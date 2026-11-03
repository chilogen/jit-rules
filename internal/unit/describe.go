package unit

import (
	"fmt"
	"strconv"
	"strings"
)

func describeEvents(values []string) string {
	events := make([]string, 0, len(values))
	for _, value := range values {
		switch value {
		case "prompt":
			events = append(events, "用户发消息时")
		case "tool":
			events = append(events, "AI 调用工具之前")
		case "tool_result":
			events = append(events, "AI 工具执行之后")
		case "stop":
			events = append(events, "AI 结束本轮回复之前")
		}
	}
	return strings.Join(events, "或")
}

func onlyStop(events []string) bool {
	for _, value := range events {
		if value != "stop" {
			return false
		}
	}
	return len(events) > 0
}

func describeConditions(trigger Trigger) string {
	var conditions []string
	if len(trigger.Tools) > 0 {
		conditions = append(conditions, "工具是 "+strings.Join(trigger.Tools, "、"))
	}
	if len(trigger.Paths) > 0 {
		conditions = append(conditions, "操作的文件匹配 "+strings.Join(trigger.Paths, "、"))
	}
	if len(trigger.Shell) > 0 {
		conditions = append(conditions, "命令匹配 "+joinRegexps(trigger.Shell))
	}
	if len(trigger.Prompt) > 0 {
		conditions = append(conditions, "用户消息匹配 "+joinRegexps(trigger.Prompt))
	}
	if len(trigger.Content) > 0 {
		conditions = append(conditions, "即将写出的内容匹配 "+joinRegexps(trigger.Content))
	}
	return strings.Join(conditions, "，且")
}

func describeJudge(judge *Judge, threshold float64) string {
	if judge == nil {
		return ""
	}
	if judge.Script != "" {
		line := fmt.Sprintf(
			"判断：在项目目录用 %s 运行下面的脚本，退出码为 0 才算命中",
			judge.Lang,
		)
		if judge.Cached() {
			line += "（结果会缓存）"
		}
		for _, scriptLine := range strings.Split(strings.TrimSuffix(judge.Script, "\n"), "\n") {
			line += "\n  " + scriptLine
		}
		return line
	}
	model := "由模型"
	if judge.Backend != "" {
		model += " " + judge.Backend + " "
	}
	line := fmt.Sprintf(
		"判断：%s回答%q，概率 ≥ %s 才算命中",
		model,
		judge.Question,
		strconv.FormatFloat(threshold, 'f', -1, 64),
	)
	if judge.Not != "" {
		line += "（排除：" + judge.Not + "）"
	}
	return line
}

func joinRegexps(patterns []string) string {
	formatted := make([]string, len(patterns))
	for i, pattern := range patterns {
		formatted[i] = "/" + pattern + "/"
	}
	return strings.Join(formatted, " 或 ")
}
