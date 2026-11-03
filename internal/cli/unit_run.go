package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/evaluate"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
	"github.com/spf13/cobra"
)

func newUnitTestCommand() *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "运行单元示例",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetCommandFlags(cmd)
			state, resolved, err := requireParsedUnit(args[0])
			if err != nil {
				return err
			}
			cases := evaluate.Test(state.Unit, resolved.Settings, offline)
			if err := evaluate.SaveTest(state.Unit, cases, time.Now().UTC()); err != nil {
				return fmt.Errorf("保存测试结果: %w", err)
			}
			_, failed, _ := testCounts(cases)
			Output(cmd, !failed, cases, strings.Join(caseLines(cases), "\n"), nil, nil)
			if failed {
				return Silent(1)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "跳过模型判断")
	return cmd
}

func newUnitReplayCommand() *cobra.Command {
	var sinceText string
	var live bool
	cmd := &cobra.Command{
		Use:   "replay <id>",
		Short: "回放历史事件",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			defer resetCommandFlags(cmd)
			state, resolved, err := requireParsedUnit(args[0])
			if err != nil {
				return err
			}
			since, err := parseSince(sinceText)
			if err != nil {
				return err
			}
			report, err := evaluate.Replay(
				state.Unit, resolved.Settings, since, live, time.Now().UTC(),
			)
			if err != nil {
				return Internal(fmt.Errorf("回放历史事件失败: %w", err))
			}
			human := formatReplay(report, !resolved.Settings.Journal.StoreContent)
			Output(cmd, true, report, human, nil, nil)
			return nil
		},
	}
	cmd.Flags().StringVar(&sinceText, "since", "14d", "回放时间范围")
	cmd.Flags().BoolVar(&live, "live", false, "实时调用模型补充判断")
	return cmd
}

func parseSince(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
		if err != nil || days < 0 {
			return 0, fmt.Errorf("无效的 --since %q", value)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf("无效的 --since %q", value)
	}
	return duration, nil
}

func formatReplay(report evaluate.ReplayReport, redacted bool) string {
	lines := make([]string, 0, min(len(report.Hits), 50)+3)
	for index, hit := range report.Hits {
		if index == 50 {
			break
		}
		lines = append(lines, hit.Summary)
	}
	lines = append(lines, fmt.Sprintf(
		"总命中：%d；命中率：%d/%d；未知：%d",
		len(report.Hits), len(report.Hits), report.Judged, report.Unknown,
	))
	if redacted {
		lines = append(lines, "提示：日志未保存原始内容，依赖内容的条件可能无法命中")
	}
	return strings.Join(lines, "\n")
}

func newUnitCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check <id>",
		Short: "检查单元冲突",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, resolved, err := requireParsedUnit(args[0])
			if err != nil {
				return err
			}
			conflicts := evaluate.Conflicts(
				state.Unit, resolved.Units, resolved.Settings, evaluate.LoadCoHits(),
			)
			human, warnings, confirmed := conflictOutput(conflicts, state.ID)
			Output(cmd, !confirmed, conflicts, human, nil, warnings)
			if confirmed {
				return Silent(1)
			}
			return nil
		},
	}
}

func newUnitShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "显示判断单元",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			state, resolved, err := requireParsedUnit(args[0])
			if err != nil {
				return err
			}
			record, err := evaluate.LoadTest(state.ID)
			if err != nil {
				return fmt.Errorf("读取测试结果: %w", err)
			}
			testStatus := describeTestRecord(state.Unit, record)
			description := strings.TrimSuffix(
				unit.Describe(state.Unit, unitThreshold(state.Unit, resolved.Settings)), "\n",
			)
			human := fmt.Sprintf("%s\n状态：%s；%s", description, state.Status, testStatus)
			Output(cmd, true, map[string]any{
				"id": state.ID, "status": state.Status,
				"description": description, "test": record,
			}, human, nil, nil)
			return nil
		},
	}
}

func describeTestRecord(parsed *unit.Unit, record *evaluate.TestRecord) string {
	if record == nil {
		return "尚未运行测试"
	}
	counts, _, _ := testCounts(record.Cases)
	status := fmt.Sprintf(
		"最近测试：通过 %d，失败 %d，跳过 %d",
		counts.Passed, counts.Failed, counts.Skipped,
	)
	if record.Hash != parsed.Hash {
		status += "（测试结果已过期）"
	}
	return status
}

func unitThreshold(parsed *unit.Unit, settings profile.Settings) float64 {
	backendName := settings.Judge.Backend
	if parsed.Judge != nil && parsed.Judge.Backend != "" {
		backendName = parsed.Judge.Backend
	}
	return settings.ThresholdOf(backendName)
}
