package evaluate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestReplayIncludesToolResult(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	u := plainUnit("me/post-tool-replay", "")
	u.Trigger = unit.Trigger{
		Events: []string{"tool_result"}, Tools: []string{"shell"},
		Shell: []string{`git\s+commit`},
	}
	record := journal.Record{
		TS: now.Add(-time.Minute),
		Event: event.Event{
			Harness: "fake", Kind: "tool_result", Tool: "shell",
			Shell: "git commit -m fix",
		},
	}
	if err := journal.Append(record, true); err != nil {
		t.Fatal(err)
	}
	report, err := Replay(u, profile.Defaults(), time.Hour, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Judged != 1 || len(report.Hits) != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestReplayPreservesJournalError(t *testing.T) {
	for _, failure := range []string{"malformed", "read-error"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("JITR_HOME", t.TempDir())
			now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
			previous := journal.Record{
				TS:    now.Add(-24 * time.Hour),
				Event: event.Event{Kind: "tool", Tool: "shell", Shell: "git commit"},
			}
			if err := journal.Append(previous, true); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(paths.State(), "journal", "2026-11-04.jsonl")
			if failure == "malformed" {
				if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			report, err := Replay(plainUnit("me/replay", ""), profile.Defaults(), 48*time.Hour, false, now)
			if err == nil {
				t.Fatalf("Replay() succeeded: %+v", report)
			}
			var syntaxErr *json.SyntaxError
			var pathErr *os.PathError
			if failure == "malformed" && !errors.As(err, &syntaxErr) {
				t.Fatalf("lost JSON error: %v", err)
			}
			if failure == "read-error" && !errors.As(err, &pathErr) {
				t.Fatalf("lost read error: %v", err)
			}
			if len(report.Hits) != 0 || report.Judged != 0 || report.Unknown != 0 {
				t.Fatalf("partial report returned after failure: %+v", report)
			}
		})
	}
}
