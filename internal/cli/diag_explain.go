package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/pipeline"
	"github.com/spf13/cobra"
)

func newExplainCommand() *cobra.Command {
	var last int
	var eventFile string
	cmd := &cobra.Command{
		Use:   "explain",
		Short: "解释事件的规则处理结果",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			defer resetCommandFlags(cmd)
			if eventFile != "" && cmd.Flags().Changed("last") {
				return fmt.Errorf("--last 和 --event 不能同时使用")
			}
			ev, err := explainEvent(eventFile, last)
			if err != nil {
				return err
			}
			trace, err := pipeline.Explain(ev, time.Now().UTC())
			if err != nil {
				return fmt.Errorf("解释事件: %w", err)
			}
			Output(cmd, true, trace, formatTrace(trace), nil, nil)
			return nil
		},
	}
	cmd.Flags().IntVar(&last, "last", 1, "倒数第 N 条事件")
	cmd.Flags().StringVar(&eventFile, "event", "", "原始事件 payload 文件")
	return cmd
}

func explainEvent(path string, last int) (*event.Event, error) {
	if path != "" {
		return decodeEventFile(path)
	}
	if last < 1 {
		return nil, fmt.Errorf("--last 必须大于 0")
	}
	records, err := journal.Since(time.Time{})
	if err != nil {
		return nil, fmt.Errorf("读取日志: %w", err)
	}
	var events []event.Event
	for _, record := range records {
		if (record.Event.Kind == "prompt" || record.Event.Kind == "tool" ||
			record.Event.Kind == "tool_result" || record.Event.Kind == "stop") &&
			record.Err != "dedupe" {
			events = append(events, record.Event)
		}
	}
	if len(events) < last {
		return nil, fmt.Errorf("日志中没有倒数第 %d 条可解释事件", last)
	}
	return &events[len(events)-last], nil
}

func decodeEventFile(path string) (*event.Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取事件文件: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("解析事件文件: %w", err)
	}
	definitions, err := harness.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("读取 harness 定义: %w", err)
	}
	definition := harness.DetectDef(definitions, raw)
	if definition == nil {
		return nil, fmt.Errorf("无法识别事件来自哪个 harness")
	}
	ev, err := harness.Decode(definition, data, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("解码事件: %w", err)
	}
	return ev, nil
}

func formatTrace(trace *pipeline.Trace) string {
	lines := []string{"说明：以下结果使用当前规则，可能与事件发生时不同。"}
	for _, gate := range trace.Gates {
		status := "通过门槛"
		if !gate.OK {
			status = "未通过门槛：" + gate.Failed
		}
		lines = append(lines, fmt.Sprintf("%s：%s", gate.ID, status))
	}
	for _, record := range trace.Record.Units {
		line := record.ID
		if record.Judge != nil {
			line += fmt.Sprintf("：判断 hit=%t p=%g", record.Judge.Hit, record.Judge.P)
		}
		if record.NoBlock {
			line += "；NoBlock=true"
		}
		if record.Skip != "" {
			line += "；跳过=" + record.Skip
		}
		lines = append(lines, line)
	}
	lines = append(lines, "动作："+trace.Result.Action)
	if trace.Result.Text != "" {
		lines = append(lines, trace.Result.Text)
	}
	return joinNonempty(lines)
}
