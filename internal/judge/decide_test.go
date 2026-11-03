package judge

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/backend/fake"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestDecideBatchesCachesAndUsesThreshold(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{
		BackendName: "decide-batch",
		P:           map[string]float64{"a": 0.5, "b": 0.49, "c": 0.9},
		ConnMs:      7,
	}
	backend.Register(fakeBackend)
	settings := testSettings(fakeBackend.Name())
	settings.Judge.MaxFieldChars = 4
	units := testUnits("a", "b", "c")
	ev := &event.Event{
		Kind:    "tool",
		Tool:    "write",
		RawTool: "WriteFile",
		Effect:  "write",
		Paths:   []string{"/very/long"},
		Shell:   "echo long",
		Content: "alpha",
		Prompt:  "你好世界五",
	}

	first := Decide(context.Background(), ev, units, settings)
	if got := fakeBackend.Calls(); got != 1 {
		t.Fatalf("Calls() = %d, want 1", got)
	}
	if !first["a"].Hit || first["b"].Hit || !first["c"].Hit {
		t.Fatalf("threshold verdicts = %#v", first)
	}
	for id, verdict := range first {
		if verdict.Backend != fakeBackend.Name() || verdict.Model != "fake-model" || verdict.ConnMs != 7 {
			t.Fatalf("verdict %s = %#v", id, verdict)
		}
	}
	state, questions := fakeBackend.LastRequest()
	if len(questions) != 3 {
		t.Fatalf("questions = %#v", questions)
	}
	assertState(t, state, map[string]any{
		"kind":     "tool",
		"tool":     "writ",
		"raw_tool": "Writ",
		"effect":   "writ",
		"paths":    []string{"/very/long"},
		"shell":    "echo",
		"content":  "alph",
		"prompt":   "你好世界",
	})

	second := Decide(context.Background(), ev, units, settings)
	if got := fakeBackend.Calls(); got != 1 {
		t.Fatalf("Calls() after cache hit = %d, want 1", got)
	}
	for id, verdict := range second {
		if verdict.Model != first[id].Model || verdict.P != first[id].P || verdict.ConnMs != 0 || verdict.Ms != 0 {
			t.Fatalf("cached verdict %s = %#v, first %#v", id, verdict, first[id])
		}
	}

	ev.Content = "bravo"
	Decide(context.Background(), ev, units, settings)
	if got := fakeBackend.Calls(); got != 2 {
		t.Fatalf("Calls() after content change = %d, want 2", got)
	}
}

func TestDecideOmitsContentWhenDisabled(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{BackendName: "decide-no-content", DefaultP: 1}
	backend.Register(fakeBackend)
	settings := testSettings(fakeBackend.Name())
	settings.Judge.SendContent = false
	ev := &event.Event{Kind: "tool", Shell: "secret shell", Content: "secret content", Prompt: "visible"}

	Decide(context.Background(), ev, testUnits("a"), settings)
	state, _ := fakeBackend.LastRequest()
	if _, ok := state["shell"]; ok {
		t.Fatalf("state contains shell: %#v", state)
	}
	if _, ok := state["content"]; ok {
		t.Fatalf("state contains content: %#v", state)
	}
	if state["prompt"] != "visible" {
		t.Fatalf("state = %#v", state)
	}
}

func TestDecideTimeoutFallsBack(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{
		BackendName: "decide-timeout",
		Delay:       50 * time.Millisecond,
		ConnMs:      13,
	}
	backend.Register(fakeBackend)
	settings := testSettings(fakeBackend.Name())
	settings.Judge.TimeoutMs = 5

	verdicts := Decide(context.Background(), &event.Event{Kind: "tool"}, testUnits("a", "b", "c"), settings)
	for id, verdict := range verdicts {
		if !verdict.Hit || !verdict.Fallback || verdict.Unconfigured || verdict.ConnMs != 13 {
			t.Fatalf("verdict %s = %#v", id, verdict)
		}
	}
}

func TestDecideMissingResultFallsBack(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{
		BackendName: "decide-missing",
		DefaultP:    0.1,
		Missing:     map[string]bool{"b": true},
	}
	backend.Register(fakeBackend)
	verdicts := Decide(
		context.Background(),
		&event.Event{Kind: "tool"},
		testUnits("a", "b", "c"),
		testSettings(fakeBackend.Name()),
	)
	if verdicts["a"].Fallback || !verdicts["b"].Fallback || verdicts["b"].Err != "missing" || verdicts["c"].Fallback {
		t.Fatalf("verdicts = %#v", verdicts)
	}
	if !verdicts["b"].Hit {
		t.Fatalf("missing verdict = %#v, want hit", verdicts["b"])
	}
}

func TestDecideUnconfiguredError(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{
		BackendName: "decide-unconfigured",
		Err:         fmt.Errorf("key: %w", backend.ErrNotConfigured),
		ConnMs:      3,
	}
	backend.Register(fakeBackend)
	verdicts := Decide(
		context.Background(),
		&event.Event{Kind: "tool"},
		testUnits("a", "b", "c"),
		testSettings(fakeBackend.Name()),
	)
	for id, verdict := range verdicts {
		if verdict.Hit || verdict.Fallback || !verdict.Unconfigured || verdict.ConnMs != 3 {
			t.Fatalf("verdict %s = %#v", id, verdict)
		}
		if !strings.Contains(verdict.Err, "key") {
			t.Fatalf("Err = %q", verdict.Err)
		}
	}
}

func TestDecideUnknownBackend(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	settings := testSettings("not-registered")
	verdicts := Decide(context.Background(), &event.Event{}, testUnits("a", "b"), settings)
	for id, verdict := range verdicts {
		if !verdict.Unconfigured || verdict.Hit || verdict.Backend != "not-registered" || verdict.Err != "unknown backend" {
			t.Fatalf("verdict %s = %#v", id, verdict)
		}
	}
}

func TestDecideConcurrentCallsAreRaceSafe(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{BackendName: "decide-concurrent", DefaultP: 0.9}
	backend.Register(fakeBackend)
	settings := testSettings(fakeBackend.Name())
	units := testUnits("a", "b", "c")

	var wait sync.WaitGroup
	for index := range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ev := &event.Event{Kind: "tool", Content: fmt.Sprintf("event-%d", index)}
			verdicts := Decide(context.Background(), ev, units, settings)
			for _, verdict := range verdicts {
				if !verdict.Hit {
					t.Errorf("verdict = %#v, want hit", verdict)
				}
			}
		}()
	}
	wait.Wait()
}

func TestDecideEmptyUnits(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	if got := Decide(context.Background(), &event.Event{}, nil, profile.Settings{}); got == nil || len(got) != 0 {
		t.Fatalf("Decide() = %#v, want empty map", got)
	}
}

func testSettings(name string) profile.Settings {
	return profile.Settings{
		Judge: profile.JudgeSettings{
			Backend:       name,
			TimeoutMs:     100,
			MaxFieldChars: 100,
			SendContent:   true,
		},
		Backends: map[string]map[string]any{
			name: {"model": "fake-model", "threshold": 0.5},
		},
	}
}

func testUnits(ids ...string) []*unit.Unit {
	units := make([]*unit.Unit, len(ids))
	for index, id := range ids {
		units[index] = &unit.Unit{
			ID: id,
			Judge: &unit.Judge{
				Question: "question-" + id,
				Not:      "not-" + id,
			},
		}
	}
	return units
}

func assertState(t *testing.T, got, want map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state = %#v, want %#v", got, want)
	}
}
