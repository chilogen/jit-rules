package journal

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/paths"
)

func TestAppendAndSince(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	first := sampleRecord(start, "one")
	second := sampleRecord(start.Add(time.Minute), "two")
	if err := Append(first, true); err != nil {
		t.Fatal(err)
	}
	if err := Append(second, true); err != nil {
		t.Fatal(err)
	}
	got, err := Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Event.Content != "one" || got[1].Event.Content != "two" {
		t.Fatalf("Since() = %+v", got)
	}
	if got[0].Event.Harness != first.Event.Harness || got[0].Units[0] != first.Units[0] {
		t.Fatalf("fields changed: %+v", got[0])
	}
}

func TestSinceReadsLegacyHarnessFlag(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	record := sampleRecord(time.Now().UTC(), "legacy")
	if err := Append(record, true); err != nil {
		t.Fatal(err)
	}
	path := journalPath(record.TS)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, present := fields["harness_flag"]; present {
		t.Fatal("new journal records still contain harness_flag")
	}
	legacy := append([]byte(`{"harness_flag":"legacy-config",`), data[1:]...)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Since(record.TS.Add(-time.Second))
	if err != nil || len(got) != 1 {
		t.Fatalf("Since() = %+v, %v", got, err)
	}
	if got[0].Event.Harness != record.Event.Harness || got[0].Event.Content != "legacy" {
		t.Fatalf("legacy record changed: %+v", got[0])
	}
}

func TestAppendHashesSensitiveEventFields(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	record := sampleRecord(time.Now(), "secret-content")
	record.Event.Shell = "secret-shell"
	record.Event.Prompt = "secret-prompt"
	record.Event.Input = map[string]any{"token": "secret-input"}
	if err := Append(record, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(journalPath(record.TS))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-content", "secret-shell", "secret-prompt", "secret-input"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("journal contains %q: %s", secret, data)
		}
	}
	got, err := Since(record.TS.Add(-time.Second))
	if err != nil || len(got) != 1 {
		t.Fatalf("Since() = %+v, %v", got, err)
	}
	for name, value := range map[string]string{
		"content": got[0].Event.Content, "shell": got[0].Event.Shell, "prompt": got[0].Event.Prompt,
	} {
		if !strings.HasPrefix(value, "sha256:") {
			t.Fatalf("%s = %q", name, value)
		}
	}
}

func TestAppendKeepsEmptyFieldsEmptyWhenHashing(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	record := sampleRecord(time.Now(), "")
	if err := Append(record, false); err != nil {
		t.Fatal(err)
	}
	got, err := Since(record.TS.Add(-time.Second))
	if err != nil || len(got) != 1 {
		t.Fatalf("Since() = %+v, %v", got, err)
	}
	if got[0].Event.Content != "" || got[0].Event.Shell != "" || got[0].Event.Prompt != "" {
		t.Fatalf("empty fields hashed: %+v", got[0].Event)
	}
}

func TestAppendUsesUTCDay(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	before := time.Date(2026, 11, 2, 23, 59, 59, 0, time.UTC)
	after := before.Add(2 * time.Second)
	if err := Append(sampleRecord(before, "before"), true); err != nil {
		t.Fatal(err)
	}
	if err := Append(sampleRecord(after, "after"), true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2026-11-02.jsonl", "2026-11-03.jsonl"} {
		if _, err := os.Stat(filepath.Join(paths.State(), "journal", name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestGCAndLogError(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	if err := Append(sampleRecord(now.AddDate(0, 0, -8), "old"), true); err != nil {
		t.Fatal(err)
	}
	if err := Append(sampleRecord(now.AddDate(0, 0, -7), "kept"), true); err != nil {
		t.Fatal(err)
	}
	GC(now, 7)
	if _, err := os.Stat(journalPath(now.AddDate(0, 0, -8))); !os.IsNotExist(err) {
		t.Fatalf("old journal still exists: %v", err)
	}
	if _, err := os.Stat(journalPath(now.AddDate(0, 0, -7))); err != nil {
		t.Fatalf("boundary journal removed: %v", err)
	}
	LogError("append", errors.New("failed"))
	data, err := os.ReadFile(filepath.Join(paths.State(), "errors.jsonl"))
	if err != nil || !strings.Contains(string(data), `"stage":"append"`) {
		t.Fatalf("errors.jsonl = %q, %v", data, err)
	}
}

func sampleRecord(ts time.Time, content string) Record {
	return Record{
		V: 1, TS: ts, Isolated: true,
		Event:    event.Event{V: 1, Harness: "test", Kind: "tool", Content: content},
		Profiles: []string{"me"}, GateFailed: 2,
		Units:  []UnitRecord{{ID: "me/test", Hash: "sha256:abc", Delivered: "inject"}},
		Result: ResultRecord{Action: "inject", Chars: 3}, Ms: 4,
	}
}
