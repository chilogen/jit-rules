package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/installer"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/spf13/cobra"
)

type installOptions struct {
	scope  string
	dryRun bool
}

type plannedHarness struct {
	def     *harness.Definition
	changes []installer.Change
}

type changeOutput struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

func init() {
	Register(newInstallCommand(false))
	Register(newInstallCommand(true))
	Register(newUpgradeCommand())
	Register(newPurgeCommand())
}

func newInstallCommand(uninstall bool) *cobra.Command {
	options := installOptions{scope: "user"}
	name, short := "install", "安装 harness 钩子"
	if uninstall {
		name, short = "uninstall", "卸载 harness 钩子"
	}
	command := &cobra.Command{
		Use:   name + " <harness>...",
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd, args, options, uninstall)
		},
	}
	command.Flags().StringVar(&options.scope, "scope", "user", "安装作用域（user 或 project）")
	command.Flags().BoolVar(&options.dryRun, "dry-run", false, "只显示计划，不写文件")
	return command
}

func runInstall(cmd *cobra.Command, names []string, options installOptions, uninstall bool) error {
	if options.scope != "user" && options.scope != "project" {
		return fmt.Errorf("scope 必须是 user 或 project")
	}
	definitions, err := selectDefinitions(names)
	if err != nil {
		return err
	}
	root, err := installRoot(options.scope)
	if err != nil {
		return err
	}
	plans, err := planHarnesses(definitions, options.scope, root, uninstall)
	if err != nil {
		return err
	}
	if !options.dryRun {
		if !uninstall {
			if err := ensureStableExecutable(); err != nil {
				return err
			}
		}
		for _, plan := range plans {
			if err := installer.Apply(plan.changes, time.Now()); err != nil {
				return err
			}
		}
	}
	data, human := renderPlans(plans, !uninstall && !options.dryRun)
	Output(cmd, true, data, human, nil, nil)
	return nil
}

func selectDefinitions(names []string) ([]*harness.Definition, error) {
	all, err := harness.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("load harness definitions: %w", err)
	}
	requested := make(map[string]bool)
	for _, name := range names {
		requested[name] = true
	}
	if requested["all"] {
		return all, nil
	}
	var result []*harness.Definition
	for _, name := range names {
		if requested[name] {
			def, err := findDefinition(all, name)
			if err != nil {
				return nil, err
			}
			result = append(result, def)
			requested[name] = false
		}
	}
	return result, nil
}

func findDefinition(definitions []*harness.Definition, name string) (*harness.Definition, error) {
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, nil
		}
	}
	return nil, fmt.Errorf("unknown harness %q", name)
}

func installRoot(scope string) (string, error) {
	if scope != "project" {
		return "", nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return paths.ProjectRoot(cwd), nil
}

func planHarnesses(definitions []*harness.Definition, scope, root string, uninstall bool) ([]plannedHarness, error) {
	result := make([]plannedHarness, 0, len(definitions))
	for _, definition := range definitions {
		implementation, ok := installer.Get(definition.Install.Kind)
		if !ok {
			return nil, fmt.Errorf("installer %q is not registered", definition.Install.Kind)
		}
		changes, err := implementation.Plan(installer.Request{
			Def: definition, Scope: scope, Root: root,
			Bin: paths.Bin(), Uninstall: uninstall,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, plannedHarness{def: definition, changes: changes})
	}
	return result, nil
}

func ensureStableExecutable() error {
	source, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return fmt.Errorf("resolve current executable: %w", err)
	}
	target, err := filepath.Abs(paths.Bin())
	if err != nil {
		return fmt.Errorf("resolve stable executable: %w", err)
	}
	if filepath.Clean(source) == filepath.Clean(target) {
		return nil
	}
	if err := fileutil.ReplaceExecutable(source, target); err != nil {
		return fmt.Errorf("install stable executable: %w", err)
	}
	return nil
}

func renderPlans(plans []plannedHarness, includeNotes bool) ([]changeOutput, string) {
	var data []changeOutput
	var human strings.Builder
	for _, plan := range plans {
		for _, change := range plan.changes {
			diff := unifiedDiff(change)
			data = append(data, changeOutput{Path: change.Path, Diff: diff})
			fmt.Fprintf(&human, "%s\n%s", change.Path, diff)
		}
		if includeNotes {
			for _, note := range plan.def.Install.Notes {
				fmt.Fprintln(&human, note)
			}
		}
	}
	return data, strings.TrimSuffix(human.String(), "\n")
}

func unifiedDiff(change installer.Change) string {
	var output strings.Builder
	fmt.Fprintf(&output, "--- %s\n+++ %s\n@@\n", change.Path, change.Path)
	for _, line := range splitLines(change.Before) {
		fmt.Fprintf(&output, "-%s\n", line)
	}
	for _, line := range splitLines(change.After) {
		fmt.Fprintf(&output, "+%s\n", line)
	}
	return output.String()
}

func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func newUpgradeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade <新二进制路径>",
		Short: "替换稳定路径上的 jitr",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := fileutil.ReplaceExecutable(args[0], paths.Bin()); err != nil {
				return fmt.Errorf("upgrade executable: %w", err)
			}
			Output(cmd, true, map[string]string{"path": paths.Bin()}, "升级完成", nil, nil)
			return nil
		},
	}
}

func newPurgeCommand() *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:   "purge",
		Short: "删除 jitr 的全部数据",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonMode && !yes {
				return fmt.Errorf("--json 模式下 purge 必须加 --yes")
			}
			if !yes && !confirmed(cmd) {
				Output(cmd, true, nil, "已取消", nil, nil)
				return nil
			}
			for _, path := range []string{paths.Config(), paths.State(), paths.Data(), paths.Cache()} {
				if err := os.RemoveAll(path); err != nil {
					return fmt.Errorf("remove %s: %w", path, err)
				}
			}
			Output(cmd, true, nil, "已删除全部数据", nil, nil)
			return nil
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "跳过确认")
	return command
}

func confirmed(cmd *cobra.Command) bool {
	fmt.Fprint(cmd.ErrOrStderr(), "删除全部 jitr 数据？[y/N] ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	return bytes.EqualFold([]byte(strings.TrimSpace(line)), []byte("y")) ||
		strings.EqualFold(strings.TrimSpace(line), "yes")
}
