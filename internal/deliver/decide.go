package deliver

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/session"
)

const blockIntroduction = "jit-rules 拦截了这次调用：即将执行的内容违反了下列规则。请按规则改正后重新发起；如果你确认规则不适用于这次操作，停下来告诉用户，不要原样重试。"
const stopIntroduction = "jit-rules 在你结束本轮之前提醒：请检查下列规则是否适用于本轮的工作；适用就照做后再结束，不适用就直接结束，不需要解释。"
const postBlockIntroduction = "jit-rules 要求先修复工具结果：工具已经执行，但产生的结果违反了下列规则。请先按规则修复结果再继续；如果你确认规则不适用于这次操作，停下来告诉用户。"

func decide(hits []Hit, ev *event.Event, st *session.State, settings profile.Settings) Result {
	result := Result{Action: "none", Skips: make(map[string]string)}
	remaining := chooseTopics(hits, &result)
	remaining = filterRedelivery(remaining, ev, st, settings.RedeliverBytes, &result)
	if len(remaining) == 0 {
		return result
	}
	blocked, injected := splitHits(remaining, ev)
	if len(blocked) > 0 {
		result.Action = "block"
		appendAll(blocked, &result, true)
		appendLimited(injected, settings.MaxInjectChars, false, &result)
		introduction := blockIntroduction
		if ev.Kind == "tool_result" {
			introduction = postBlockIntroduction
		}
		result.Text = introduction + "\n" + strings.Join(rendered(remaining, result.Delivered), "\n")
		return result
	}
	result.Action = "inject"
	appendLimited(injected, settings.MaxInjectChars, true, &result)
	lines := strings.Join(rendered(remaining, result.Delivered), "\n")
	if ev.Kind == "stop" {
		result.Text = stopIntroduction + "\n" + lines
		return result
	}
	result.Text = "<jit-rules>\n" + lines + "\n</jit-rules>"
	return result
}

func chooseTopics(hits []Hit, result *Result) []Hit {
	seen := make(map[string]bool)
	kept := make([]Hit, 0, len(hits))
	for _, hit := range hits {
		topic := hit.Unit.Topic
		if topic != "" && seen[topic] {
			result.Skips[hit.Unit.ID] = "topic"
			continue
		}
		if topic != "" {
			seen[topic] = true
		}
		kept = append(kept, hit)
	}
	return kept
}

func filterRedelivery(hits []Hit, ev *event.Event, st *session.State, window int, result *Result) []Hit {
	kept := make([]Hit, 0, len(hits))
	for _, hit := range hits {
		if ev.Kind == "stop" && !blockable(hit, ev) {
			if ev.StopActive {
				result.Skips[hit.Unit.ID] = "redeliver"
				continue
			}
			kept = append(kept, hit)
			continue
		}
		if blockable(hit, ev) || st == nil || st.Delivered == nil {
			kept = append(kept, hit)
			continue
		}
		mark, ok := st.Delivered[hit.Unit.ID]
		if ok && mark != nil && st.Bytes-mark.At < window {
			result.Skips[hit.Unit.ID] = "redeliver"
			continue
		}
		kept = append(kept, hit)
	}
	return kept
}

func splitHits(hits []Hit, ev *event.Event) (blocked, injected []Hit) {
	for _, hit := range hits {
		if blockable(hit, ev) {
			blocked = append(blocked, hit)
		} else {
			injected = append(injected, hit)
		}
	}
	return blocked, injected
}

func blockable(hit Hit, ev *event.Event) bool {
	toolEvent := ev.Kind == "tool" || ev.Kind == "tool_result"
	return toolEvent && hit.Unit.Deliver.As == "block" && !hit.NoBlock
}

func appendAll(hits []Hit, result *Result, blocked bool) {
	for _, hit := range hits {
		result.Delivered = append(result.Delivered, hit.Unit.ID)
		if blocked {
			result.Blocked = append(result.Blocked, hit.Unit.ID)
		}
	}
}

func appendLimited(hits []Hit, max int, keepFirst bool, result *Result) {
	chars := 0
	truncated := false
	for i, hit := range hits {
		chars += utf8.RuneCountInString(formatHit(hit))
		if truncated || (chars > max && !(keepFirst && i == 0)) {
			truncated = true
			result.Skips[hit.Unit.ID] = "truncated"
			continue
		}
		result.Delivered = append(result.Delivered, hit.Unit.ID)
	}
}

func rendered(ordered []Hit, delivered []string) []string {
	byID := make(map[string]Hit, len(ordered))
	for _, hit := range ordered {
		byID[hit.Unit.ID] = hit
	}
	lines := make([]string, 0, len(delivered))
	for _, id := range delivered {
		lines = append(lines, formatHit(byID[id]))
	}
	return lines
}

func formatHit(hit Hit) string {
	return fmt.Sprintf("[%s] %s", hit.Unit.ID, hit.Unit.Body)
}
