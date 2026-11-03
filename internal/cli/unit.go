package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
	"github.com/spf13/cobra"
)

var unitShortIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,63}$`)

type unitListItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	File    string `json:"file"`
	Profile string `json:"profile"`
	Source  string `json:"source"`
	Err     string `json:"err"`
}

func init() {
	command := &cobra.Command{Use: "unit", Short: "管理判断单元"}
	command.AddCommand(
		newUnitListCommand(),
		newUnitNewCommand(),
		newUnitTestCommand(),
		newUnitReplayCommand(),
		newUnitCheckCommand(),
		newUnitShowCommand(),
		newUnitEnableCommand(),
		newUnitDisableCommand(),
		newUnitRemoveCommand(),
	)
	Register(command)
}

func newUnitListCommand() *cobra.Command {
	var target string
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出判断单元",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			defer resetCommandFlags(cmd)
			resolved, filter, err := resolveUnits(target)
			if err != nil {
				return err
			}
			items, lines := listUnitStates(resolved.States, filter, all)
			Output(cmd, true, items, strings.Join(lines, "\n"), resolved.Errors, nil)
			if len(resolved.Errors) > 0 {
				return Silent(1)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "profile", "", "目标 profile")
	cmd.Flags().BoolVar(&all, "all", false, "显示所有状态")
	return cmd
}

func listUnitStates(
	states []profile.UnitState,
	filter string,
	all bool,
) ([]unitListItem, []string) {
	var items []unitListItem
	var lines []string
	for _, state := range states {
		if filter != "" && state.Profile != filter {
			continue
		}
		if !all && state.Status != "active" {
			continue
		}
		title := ""
		if state.Unit != nil {
			title = state.Unit.Title
		}
		item := unitListItem{
			ID: state.ID, Title: title, Status: state.Status, File: state.File,
			Profile: state.Profile, Source: state.Source, Err: state.Err,
		}
		items = append(items, item)
		lines = append(lines, fmt.Sprintf(
			"%s\t%s\t%s\t%s", item.ID, item.Title, item.Status, item.File,
		))
	}
	return items, lines
}

func newUnitNewCommand() *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "new <profile> <短 id>",
		Short: "新建判断单元",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetCommandFlags(cmd)
			return runUnitNew(cmd, args[0], args[1], fromStdin)
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "from-stdin", false, "从标准输入读取单元")
	return cmd
}

func runUnitNew(cmd *cobra.Command, target, shortID string, fromStdin bool) error {
	if !unitShortIDPattern.MatchString(shortID) {
		return fmt.Errorf("短 id %q 必须匹配 ^[a-z0-9][a-z0-9.-]{0,63}$", shortID)
	}
	selected, err := selectProfile(target)
	if err != nil {
		return err
	}
	data, err := newUnitContent(cmd, fromStdin)
	if err != nil {
		return err
	}
	path := filepath.Join(selected.config.Dir, "units", shortID+".md")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("创建单元 %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("写入单元 %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("关闭单元 %s: %w", path, err)
	}
	parsed, parseErr := unit.Parse(data, path, selected.config.Name)
	if parseErr == nil {
		errs, _ := unit.Validate(parsed)
		parseErr = errors.Join(errs...)
	}
	if parseErr != nil {
		_ = os.Remove(path)
		return fmt.Errorf("单元无效: %w", parseErr)
	}
	warnings := commitWarning(selected.config.Dir, "jitr: 新建单元 "+parsed.ID)
	Output(cmd, true, map[string]string{"id": parsed.ID, "file": path},
		fmt.Sprintf("已创建单元 %s：%s", parsed.ID, path), nil, warnings)
	return nil
}

func newUnitContent(cmd *cobra.Command, fromStdin bool) ([]byte, error) {
	if fromStdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("读取标准输入: %w", err)
		}
		return data, nil
	}
	data, err := assets.FS.ReadFile("unit-template.md")
	if err != nil {
		return nil, fmt.Errorf("读取单元模板: %w", err)
	}
	return data, nil
}
