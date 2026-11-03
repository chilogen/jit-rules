package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/evaluate"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
	"github.com/spf13/cobra"
)

func init() {
	Register(newCheckCommand())
	Register(newExplainCommand())
	Register(newGuideCommand())
	Register(newStatusCommand())
}

func newCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "检查所有生效单元",
		Args:  cobra.NoArgs,
		RunE:  runCheck,
	}
}

func runCheck(cmd *cobra.Command, _ []string) error {
	resolved, _, err := resolveUnits("")
	if err != nil {
		return err
	}
	var checkErrors []error
	var warnings []string
	for _, parsed := range resolved.Units {
		errs, warns := unit.Validate(parsed)
		checkErrors = append(checkErrors, errs...)
		warnings = append(warnings, warns...)
	}
	var conflicts []evaluate.Conflict
	history := evaluate.LoadCoHits()
	for index, parsed := range resolved.Units {
		conflicts = append(conflicts,
			evaluate.Conflicts(parsed, resolved.Units[index+1:], resolved.Settings, history)...)
	}
	for _, conflict := range conflicts {
		report := evaluate.FormatConflict(conflict, conflict.A)
		if conflict.Confirmed {
			checkErrors = append(checkErrors, errors.New(report))
		} else {
			warnings = append(warnings, report)
		}
	}
	checkErrors = append(checkErrors, resolved.Errors...)
	human := "检查通过"
	if len(checkErrors) > 0 {
		human = "检查发现问题"
	}
	Output(cmd, len(checkErrors) == 0, conflicts, human, checkErrors, warnings)
	if len(checkErrors) > 0 {
		return Silent(1)
	}
	return nil
}

func newGuideCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "guide",
		Short: "输出编写指南",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := assets.FS.ReadFile("guide.md")
			if err != nil {
				return fmt.Errorf("读取编写指南: %w", err)
			}
			if jsonMode {
				Output(cmd, true, map[string]string{"guide": string(data)}, "", nil, nil)
			} else {
				_, _ = cmd.OutOrStdout().Write(data)
			}
			return nil
		},
	}
}

func profileStatusLines(resolved *profile.Resolved) []string {
	counts := map[string]int{}
	for _, state := range resolved.States {
		if state.Status == "active" {
			counts[state.Profile]++
		}
	}
	lines := make([]string, 0, len(resolved.Profiles))
	for _, name := range resolved.Profiles {
		lines = append(lines, fmt.Sprintf("profile %s：%d 个生效单元", name, counts[name]))
	}
	return lines
}

func joinNonempty(values []string) string {
	var result []string
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return strings.Join(result, "\n")
}
