package evaluate

import (
	"context"
	"fmt"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/gate"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

type matchResult int

const (
	matchMiss matchResult = iota
	matchHit
	matchUnknown
)

func conflicts(
	target *unit.Unit,
	all []*unit.Unit,
	settings profile.Settings,
	history CoHits,
) []Conflict {
	var result []Conflict
	for _, other := range all {
		if skipConflictPair(target, other) {
			continue
		}
		example, possible, definite := overlap(target, other, settings)
		if summary, ok := history.lookup(target.ID, other.ID); ok && !definite {
			example, possible, definite = "真实事件 "+summary, true, true
		}
		if !possible {
			continue
		}
		confirmed := false
		if definite {
			var conflict bool
			confirmed, conflict = confirmConflict(target, other, settings)
			if !conflict {
				continue
			}
		}
		result = append(result, Conflict{
			A: target.ID, B: other.ID, Example: example, Confirmed: confirmed,
		})
	}
	return result
}

func skipConflictPair(a, b *unit.Unit) bool {
	if a == nil || b == nil || a.ID == b.ID {
		return true
	}
	if a.Topic != "" && a.Topic == b.Topic {
		return true
	}
	return contains(a.Replaces, b.ID) || contains(b.Replaces, a.ID)
}

func overlap(a, b *unit.Unit, settings profile.Settings) (string, bool, bool) {
	for _, pair := range [][2]*unit.Unit{{a, b}, {b, a}} {
		for _, example := range pair[0].Examples {
			if example.Expect != "hit" {
				continue
			}
			ev := example.Event
			ev.Root, ev.Cwd = "/work/proj", "/work/proj"
			left := matchUnit(pair[0], &ev, settings)
			right := matchUnit(pair[1], &ev, settings)
			if left == matchMiss || right == matchMiss {
				continue
			}
			definite := left == matchHit && right == matchHit
			return eventSummary(&ev), true, definite
		}
	}
	return "", false, false
}

func matchUnit(u *unit.Unit, ev *event.Event, settings profile.Settings) matchResult {
	if !contains(u.Trigger.Events, ev.Kind) || !gate.Match(u, ev).OK {
		return matchMiss
	}
	if u.Judge == nil {
		return matchHit
	}
	if u.Judge.Script != "" {
		return matchUnknown
	}
	verdict := decide(ev, u, settings)
	if verdict.Unconfigured || verdict.Fallback {
		return matchUnknown
	}
	if verdict.Hit {
		return matchHit
	}
	return matchMiss
}

func confirmConflict(a, b *unit.Unit, settings profile.Settings) (bool, bool) {
	implementation, ok := backend.Get(settings.Judge.Backend)
	if !ok {
		return false, true
	}
	timeout := time.Duration(settings.Judge.TimeoutMs) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := implementation.Judge(ctx, map[string]any{
		"instruction_a": a.Body,
		"instruction_b": b.Body,
	}, []backend.Question{{
		ID:   "conflict",
		Text: "Would following instruction_a make it impossible or unclear to follow instruction_b, or do they ask for contradictory things?",
	}}, settings.Backends[settings.Judge.Backend])
	if err != nil {
		return false, true
	}
	probability, ok := result.P["conflict"]
	if !ok {
		return false, true
	}
	if probability < 0.5 {
		return false, false
	}
	return true, true
}

func eventSummary(ev *event.Event) string {
	switch {
	case len(ev.Paths) > 0:
		return ev.Kind + " " + ev.Tool + " " + ev.Paths[0]
	case ev.Shell != "":
		return ev.Kind + " " + ev.Tool + " " + truncate(ev.Shell, 60)
	case ev.Prompt != "":
		return ev.Kind + " " + truncate(ev.Prompt, 60)
	default:
		return ev.Kind + " " + ev.Tool
	}
}

func formatConflict(conflict Conflict, higher string) string {
	prefix := "冲突："
	if !conflict.Confirmed {
		prefix = "可能冲突（无法调用模型确认）："
	}
	return fmt.Sprintf(
		"%s%s ↔ %s（同时触发的示例：%s）\n"+
			"可选处理：\n"+
			"  1. 两条是同一件事的不同要求 → 给它们设置相同的 topic，只让排位高的生效（当前排位高的是 %s）\n"+
			"  2. 用 %s 完全替换 %s → 在 %s 里加上 replaces: [%s]\n"+
			"  3. 不需要 %s → jitr unit disable %s\n"+
			"  4. 两者其实不冲突，只是触发范围太宽 → 收紧其中一条的 trigger",
		prefix, conflict.A, conflict.B, conflict.Example, higher,
		conflict.A, conflict.B, conflict.A, conflict.B, conflict.B, conflict.B,
	)
}
