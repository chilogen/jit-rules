package deliver

import (
	"strings"
	"testing"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/session"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestDecideInjectFormatting(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/one", "first", "inject"), hit("p/two", "second\nline", "inject")}
	got := Decide(hits, &event.Event{Kind: "prompt"}, nil, settings(100))
	want := "<jit-rules>\n[p/one] first\n[p/two] second\nline\n</jit-rules>"
	if got.Action != "inject" || got.Text != want {
		t.Fatalf("Decide() action/text = %q, %q", got.Action, got.Text)
	}
	assertStrings(t, got.Delivered, "p/one", "p/two")
}

func TestDecideBlockOrderingAndDelivery(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/inject", "extra", "inject"), hit("p/block", "must stop", "block")}
	got := Decide(hits, &event.Event{Kind: "tool"}, nil, settings(100))
	if got.Action != "block" || !strings.HasPrefix(got.Text, blockIntroduction+"\n[p/block] must stop") {
		t.Fatalf("Decide() = %+v", got)
	}
	assertStrings(t, got.Blocked, "p/block")
	assertStrings(t, got.Delivered, "p/block", "p/inject")
}

func TestDecidePostToolBlockRequiresFix(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/block", "split the file", "block")}
	got := Decide(hits, &event.Event{Kind: "tool_result"}, nil, settings(100))
	if got.Action != "block" || !strings.HasPrefix(got.Text, postBlockIntroduction+"\n") {
		t.Fatalf("Decide() = %+v", got)
	}
	assertStrings(t, got.Blocked, "p/block")
}

func TestDecideBlockIgnoresRedelivery(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/block", "stop", "block")}
	st := state(100, "p/block", 99)
	got := Decide(hits, &event.Event{Kind: "tool"}, st, settings(50))
	if got.Action != "block" || got.Skips["p/block"] != "" {
		t.Fatalf("Decide() = %+v", got)
	}
}

func TestDecideBlockDegradesToInject(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	for _, tt := range []struct {
		name string
		hit  Hit
		ev   event.Event
	}{
		{"no block", Hit{Unit: testUnit("p/block", "stop", "block"), NoBlock: true}, event.Event{Kind: "tool"}},
		{"prompt", hit("p/block", "stop", "block"), event.Event{Kind: "prompt"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JITR_HOME", t.TempDir())
			got := Decide([]Hit{tt.hit}, &tt.ev, nil, settings(50))
			if got.Action != "inject" {
				t.Fatalf("Action = %q", got.Action)
			}
		})
	}
	got := Decide(
		[]Hit{{Unit: testUnit("p/block", "stop", "block"), NoBlock: true}},
		&event.Event{Kind: "tool"}, state(100, "p/block", 99), settings(50),
	)
	if got.Action != "none" || got.Skips["p/block"] != "redeliver" {
		t.Fatalf("NoBlock redelivery = %+v", got)
	}
}

func TestDecideRedeliveryWindow(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/inject", "text", "inject")}
	recent := Decide(hits, &event.Event{Kind: "tool"}, state(100, "p/inject", 51), settings(50))
	if recent.Action != "none" || recent.Skips["p/inject"] != "redeliver" {
		t.Fatalf("recent = %+v", recent)
	}
	old := Decide(hits, &event.Event{Kind: "tool"}, state(100, "p/inject", 50), settings(50))
	if old.Action != "inject" {
		t.Fatalf("old Action = %q", old.Action)
	}
}

func TestDecideTopicKeepsHighestRank(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	first, second := hit("p/first", "one", "inject"), hit("p/second", "two", "inject")
	first.Unit.Topic, second.Unit.Topic = "same", "same"
	got := Decide([]Hit{first, second}, &event.Event{Kind: "prompt"}, nil, settings(100))
	assertStrings(t, got.Delivered, "p/first")
	if got.Skips["p/second"] != "topic" {
		t.Fatalf("Skips = %#v", got.Skips)
	}
}

func TestDecideTruncation(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	injected := Decide(
		[]Hit{hit("p/long", "1234567890", "inject"), hit("p/next", "x", "inject")},
		&event.Event{Kind: "prompt"}, nil, settings(1),
	)
	assertStrings(t, injected.Delivered, "p/long")
	if injected.Skips["p/next"] != "truncated" {
		t.Fatalf("inject skips = %#v", injected.Skips)
	}
	blocked := Decide(
		[]Hit{hit("p/block", strings.Repeat("x", 100), "block"), hit("p/inject", "x", "inject")},
		&event.Event{Kind: "tool"}, nil, settings(1),
	)
	assertStrings(t, blocked.Delivered, "p/block")
	if blocked.Skips["p/block"] != "" || blocked.Skips["p/inject"] != "truncated" {
		t.Fatalf("block skips = %#v", blocked.Skips)
	}
}

func TestDecideNilStateEqualsEmptyState(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/one", "text", "inject")}
	a := Decide(hits, &event.Event{Kind: "prompt"}, nil, settings(100))
	b := Decide(hits, &event.Event{Kind: "prompt"}, &session.State{}, settings(100))
	if a.Action != b.Action || a.Text != b.Text {
		t.Fatalf("nil = %+v, empty = %+v", a, b)
	}
}

func hit(id, body, as string) Hit {
	return Hit{Unit: testUnit(id, body, as)}
}

func testUnit(id, body, as string) *unit.Unit {
	return &unit.Unit{ID: id, Body: body, Deliver: unit.Deliver{As: as}}
}

func settings(max int) profile.Settings {
	return profile.Settings{RedeliverBytes: max, MaxInjectChars: max}
}

func state(bytes int, id string, at int) *session.State {
	return &session.State{Bytes: bytes, Delivered: map[string]*session.Mark{id: {At: at}}}
}

func assertStrings(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}
