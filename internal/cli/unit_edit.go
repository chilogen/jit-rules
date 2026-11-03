package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/chilogen/jit-rules/internal/evaluate"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
	"github.com/spf13/cobra"
)

func newUnitEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <id>",
		Short: "启用判断单元",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, resolved, err := requireParsedUnit(args[0])
			if err != nil {
				return err
			}
			selected, err := selectProfile(state.Profile)
			if err != nil {
				return err
			}
			if state.Source != "own" {
				return enablePackUnit(cmd, state, selected)
			}
			return enableOwnUnit(cmd, state, resolved, selected)
		},
	}
}

func enablePackUnit(
	cmd *cobra.Command,
	state profile.UnitState,
	selected *selectedProfile,
) error {
	if state.Status != "disabled" {
		return fmt.Errorf("单元 %s 已经生效", state.ID)
	}
	if err := profile.RemoveDisable(selected.config.Dir, state.ID); err != nil {
		return fmt.Errorf("启用包单元: %w", err)
	}
	warnings := commitWarning(selected.config.Dir, "jitr: 启用单元 "+state.ID)
	Output(cmd, true, map[string]string{"id": state.ID}, "已启用单元 "+state.ID, nil, warnings)
	return nil
}

func enableOwnUnit(
	cmd *cobra.Command,
	state profile.UnitState,
	resolved *profile.Resolved,
	selected *selectedProfile,
) error {
	validationErrors, validationWarnings := unit.Validate(state.Unit)
	if len(validationErrors) > 0 {
		return fmt.Errorf("单元校验失败: %w", errors.Join(validationErrors...))
	}
	record, err := evaluate.LoadTest(state.ID)
	if err != nil {
		return fmt.Errorf("读取测试结果: %w", err)
	}
	if record == nil || record.Hash != state.Unit.Hash {
		return fmt.Errorf("单元 %s 尚未通过当前版本测试，请先运行 jitr unit test %s", state.ID, state.ID)
	}
	counts, failed, skipped := testCounts(record.Cases)
	if failed {
		return fmt.Errorf("单元 %s 的测试有失败，请先运行 jitr unit test %s", state.ID, state.ID)
	}
	conflicts := evaluate.Conflicts(
		state.Unit, resolved.Units, resolved.Settings, evaluate.LoadCoHits(),
	)
	human, conflictWarnings, confirmed := conflictOutput(conflicts, state.ID)
	if confirmed {
		return errors.New(human)
	}
	selected.lock.Units[state.ID] = profile.LockUnit{
		Hash: state.Unit.Hash, EnabledAt: time.Now().UTC(), Tests: counts,
	}
	if err := profile.SaveLock(selected.config.Dir, selected.lock); err != nil {
		return fmt.Errorf("保存 lock: %w", err)
	}
	warnings := append(validationWarnings, conflictWarnings...)
	if skipped {
		warnings = append(warnings, "测试中有跳过的模型判断；当前环境未配置 judge，仍允许启用")
	}
	warnings = append(warnings, commitWarning(
		selected.config.Dir, "jitr: 启用单元 "+state.ID,
	)...)
	Output(cmd, true, map[string]any{"id": state.ID, "tests": counts},
		"已启用单元 "+state.ID, nil, warnings)
	return nil
}

func newUnitDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <id>",
		Short: "停用判断单元",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := findUnitState(args[0])
			if err != nil {
				return err
			}
			selected, err := selectProfile(state.Profile)
			if err != nil {
				return err
			}
			if state.Source == "own" {
				delete(selected.lock.Units, state.ID)
				err = profile.SaveLock(selected.config.Dir, selected.lock)
			} else {
				err = profile.AddDisable(selected.config.Dir, state.ID)
			}
			if err != nil {
				return fmt.Errorf("停用单元: %w", err)
			}
			warnings := commitWarning(selected.config.Dir, "jitr: 停用单元 "+state.ID)
			Output(cmd, true, map[string]string{"id": state.ID},
				"已停用单元 "+state.ID, nil, warnings)
			return nil
		},
	}
}

func newUnitRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "删除 profile 自有单元",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, _, err := findUnitState(args[0])
			if err != nil {
				return err
			}
			if state.Source != "own" {
				return fmt.Errorf("单元 %s 来自包，不能删除", state.ID)
			}
			selected, err := selectProfile(state.Profile)
			if err != nil {
				return err
			}
			if err := os.Remove(state.File); err != nil {
				return fmt.Errorf("删除单元文件: %w", err)
			}
			delete(selected.lock.Units, state.ID)
			if err := profile.SaveLock(selected.config.Dir, selected.lock); err != nil {
				return fmt.Errorf("保存 lock: %w", err)
			}
			warnings := commitWarning(selected.config.Dir, "jitr: 删除单元 "+state.ID)
			Output(cmd, true, map[string]string{"id": state.ID},
				"已删除单元 "+state.ID, nil, warnings)
			return nil
		},
	}
}
