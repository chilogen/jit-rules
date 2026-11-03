package evaluate

import (
	"context"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/gate"
	"github.com/chilogen/jit-rules/internal/judge"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func testUnit(u *unit.Unit, settings profile.Settings, offline bool) []CaseResult {
	results := make([]CaseResult, 0, len(u.Examples)+1)
	if u.Deliver.As == "block" && !hasCompliantMiss(u.Examples) {
		results = append(results, CaseResult{
			Index: -1, Result: "fail",
			Note: "规则 12：拦截单元需要一条'改对之后'的 miss 示例：和某条 hit 示例是同一个工具，只是内容合规",
		})
	}
	for index, example := range u.Examples {
		results = append(results, testExample(index, example, u, settings, offline))
	}
	return results
}

func testExample(
	index int,
	example unit.Example,
	u *unit.Unit,
	settings profile.Settings,
	offline bool,
) CaseResult {
	result := CaseResult{Index: index, Expect: example.Expect}
	ev := example.Event
	ev.Root, ev.Cwd = "/work/proj", "/work/proj"
	if !contains(u.Trigger.Events, ev.Kind) || !gate.Match(u, &ev).OK {
		result.Got = "miss"
		return compareCase(result)
	}
	if u.Judge == nil {
		result.Got = "hit"
		return compareCase(result)
	}
	if u.Judge.Script != "" {
		result.Result = "skipped"
		result.Note = "脚本判断依赖真实环境，示例测试不运行脚本"
		return result
	}
	if offline {
		result.Result = "skipped"
		result.Note = "离线模式跳过模型判断"
		return result
	}
	verdict := decide(&ev, u, settings)
	switch {
	case verdict.Unconfigured:
		result.Result, result.Note = "skipped", verdict.Err
	case verdict.Fallback:
		result.Result, result.Note = "error", verdict.Err
	case verdict.Hit:
		result.Got = "hit"
	default:
		result.Got = "miss"
	}
	if result.Got != "" {
		return compareCase(result)
	}
	return result
}

func decide(ev *event.Event, u *unit.Unit, settings profile.Settings) judge.Verdict {
	timeout := time.Duration(settings.Judge.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return judge.Decide(ctx, ev, []*unit.Unit{u}, settings)[u.ID]
}

func compareCase(result CaseResult) CaseResult {
	result.Result = "fail"
	if result.Got == result.Expect {
		result.Result = "pass"
	}
	return result
}

func hasCompliantMiss(examples []unit.Example) bool {
	hitTools := map[string]bool{}
	for _, example := range examples {
		if example.Expect == "hit" && example.Event.Tool != "" {
			hitTools[example.Event.Tool] = true
		}
	}
	for _, example := range examples {
		if example.Expect == "miss" && hitTools[example.Event.Tool] {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
