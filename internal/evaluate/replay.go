package evaluate

import (
	"fmt"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/gate"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func replay(
	u *unit.Unit,
	settings profile.Settings,
	since time.Duration,
	live bool,
	now time.Time,
) (ReplayReport, error) {
	records, err := journal.Since(now.Add(-since))
	if err != nil {
		return ReplayReport{}, fmt.Errorf("read replay journal: %w", err)
	}
	var report ReplayReport
	for _, record := range records {
		replayRecord(u, settings, live, record, &report)
	}
	return report, nil
}

func replayRecord(
	u *unit.Unit,
	settings profile.Settings,
	live bool,
	record journal.Record,
	report *ReplayReport,
) {
	ev := &record.Event
	if (ev.Kind != "prompt" && ev.Kind != "tool" && ev.Kind != "tool_result" && ev.Kind != "stop") ||
		record.Err == "dedupe" {
		return
	}
	if !contains(u.Trigger.Events, ev.Kind) {
		return
	}
	if !gate.Match(u, ev).OK {
		report.Judged++
		return
	}
	hit, known := replayDecision(u, settings, live, record)
	if !known {
		report.Unknown++
		return
	}
	report.Judged++
	if hit {
		report.Hits = append(report.Hits, ReplayHit{
			TS: record.TS, Summary: replaySummary(record),
		})
	}
}

func replayDecision(
	u *unit.Unit,
	settings profile.Settings,
	live bool,
	record journal.Record,
) (bool, bool) {
	if u.Judge == nil {
		return true, true
	}
	for _, saved := range record.Units {
		if saved.ID != u.ID || saved.Hash != u.Hash || saved.Judge == nil {
			continue
		}
		if !saved.Judge.Fallback && !saved.Judge.Unconfigured {
			return saved.Judge.Hit, true
		}
	}
	if u.Judge.Script != "" {
		return false, false
	}
	if !live {
		return false, false
	}
	verdict := decide(&record.Event, u, settings)
	if verdict.Fallback || verdict.Unconfigured {
		return false, false
	}
	return verdict.Hit, true
}

func replaySummary(record journal.Record) string {
	ev := record.Event
	detail := ""
	switch {
	case len(ev.Paths) > 0:
		detail = ev.Paths[0]
	case ev.Shell != "":
		detail = truncate(ev.Shell, 60)
	default:
		detail = truncate(ev.Prompt, 60)
	}
	return fmt.Sprintf(
		"%s %s %s %s %s",
		record.TS.Format(time.RFC3339), ev.Harness, ev.Kind, ev.Tool, detail,
	)
}

func truncate(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
