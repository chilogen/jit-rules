package judge

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/backend/fake"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestDecideRoutesBackendGroupsWithIndependentThresholds(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	a := &fake.Backend{BackendName: "route-a", DefaultP: 0.5}
	b := &fake.Backend{BackendName: "route-b", DefaultP: 0.5}
	backend.Register(a)
	backend.Register(b)
	settings := routingSettings(a.Name(), b.Name())
	units := []*unit.Unit{
		questionUnit("a1", "", nil),
		questionUnit("a2", "", nil),
		questionUnit("b1", b.Name(), nil),
	}

	got := Decide(context.Background(), &event.Event{Kind: "tool"}, units, settings)
	if a.Calls() != 1 || b.Calls() != 1 {
		t.Fatalf("calls = A:%d B:%d, want 1 each", a.Calls(), b.Calls())
	}
	_, questionsA := a.LastRequest()
	_, questionsB := b.LastRequest()
	if len(questionsA) != 2 || len(questionsB) != 1 {
		t.Fatalf("question counts = A:%d B:%d", len(questionsA), len(questionsB))
	}
	if !got["a1"].Hit || !got["a2"].Hit || got["b1"].Hit {
		t.Fatalf("verdicts = %#v", got)
	}
	for _, id := range []string{"a1", "a2"} {
		if got[id].Backend != a.Name() {
			t.Fatalf("%s backend = %q", id, got[id].Backend)
		}
	}
	if got["b1"].Backend != b.Name() {
		t.Fatalf("b1 backend = %q", got["b1"].Backend)
	}
}

func TestDecideBackendFailureIsIsolated(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	a := &fake.Backend{
		BackendName: "route-unconfigured",
		Err:         fmt.Errorf("missing key: %w", backend.ErrNotConfigured),
	}
	b := &fake.Backend{BackendName: "route-healthy", DefaultP: 1}
	backend.Register(a)
	backend.Register(b)
	settings := routingSettings(a.Name(), b.Name())
	units := []*unit.Unit{
		questionUnit("bad", "", nil),
		questionUnit("good", b.Name(), nil),
	}

	got := Decide(context.Background(), &event.Event{Kind: "tool"}, units, settings)
	if !got["bad"].Unconfigured || got["bad"].Hit {
		t.Fatalf("bad verdict = %#v", got["bad"])
	}
	if !got["good"].Hit || got["good"].Fallback || got["good"].Unconfigured {
		t.Fatalf("good verdict = %#v", got["good"])
	}
}

func TestDecideCacheFalseResendsOnlyThatUnit(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{BackendName: "route-cache", DefaultP: 1}
	backend.Register(fakeBackend)
	noCache := false
	units := []*unit.Unit{
		questionUnit("cached", "", nil),
		questionUnit("uncached", "", &noCache),
	}
	settings := routingSettings(fakeBackend.Name(), "")
	ev := &event.Event{Kind: "tool"}

	Decide(context.Background(), ev, units, settings)
	Decide(context.Background(), ev, units, settings)
	if fakeBackend.Calls() != 2 {
		t.Fatalf("calls = %d, want 2", fakeBackend.Calls())
	}
	_, questions := fakeBackend.LastRequest()
	if len(questions) != 1 || questions[0].ID != "uncached" {
		t.Fatalf("second questions = %#v", questions)
	}
}

func TestDecideRunsScriptAndBackendConcurrently(t *testing.T) {
	requireShell(t)
	t.Setenv("JITR_HOME", t.TempDir())
	fakeBackend := &fake.Backend{
		BackendName: "route-concurrent",
		DefaultP:    1,
		Delay:       300 * time.Millisecond,
	}
	backend.Register(fakeBackend)
	units := []*unit.Unit{
		questionUnit("question", "", nil),
		scriptUnit("script", "sleep 0.3\nexit 0", nil),
	}
	started := time.Now()
	got := Decide(
		context.Background(),
		&event.Event{Kind: "tool", Cwd: t.TempDir()},
		units,
		routingSettings(fakeBackend.Name(), ""),
	)
	elapsed := time.Since(started)
	if elapsed >= 550*time.Millisecond {
		t.Fatalf("elapsed = %v, want concurrent completion under 550ms", elapsed)
	}
	if !got["question"].Hit || !got["script"].Hit {
		t.Fatalf("verdicts = %#v", got)
	}
}

func routingSettings(defaultName, secondName string) profile.Settings {
	backends := map[string]map[string]any{
		defaultName: {"model": "fake-model", "threshold": 0.3},
	}
	if secondName != "" {
		backends[secondName] = map[string]any{"model": "fake-model", "threshold": 0.8}
	}
	return profile.Settings{
		Judge: profile.JudgeSettings{
			Backend:       defaultName,
			TimeoutMs:     1000,
			MaxFieldChars: 100,
			SendContent:   true,
		},
		Backends: backends,
	}
}

func questionUnit(id, backendName string, cache *bool) *unit.Unit {
	return &unit.Unit{
		ID: id,
		Judge: &unit.Judge{
			Question: "question-" + id,
			Backend:  backendName,
			Cache:    cache,
		},
	}
}
