package deliver

import (
	"strings"
	"testing"

	"github.com/chilogen/jit-rules/internal/event"
)

func TestDecideStopRemindsOncePerTurn(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	hits := []Hit{hit("p/log", "archive the change", "inject")}
	// delivered a moment ago in bytes: the distance filter must not apply on stop
	st := state(100, "p/log", 99)
	got := Decide(hits, &event.Event{Kind: "stop"}, st, settings(1000))
	if got.Action != "inject" || got.Text != stopIntroduction+"\n[p/log] archive the change" {
		t.Fatalf("first end = %+v", got)
	}
	assertStrings(t, got.Delivered, "p/log")
	again := Decide(hits, &event.Event{Kind: "stop", StopActive: true}, st, settings(1000))
	if again.Action != "none" || again.Skips["p/log"] != "redeliver" || len(again.Delivered) != 0 {
		t.Fatalf("re-sent end = %+v", again)
	}
}

func TestDecideStopNeverBlocks(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	got := Decide([]Hit{hit("p/b", "x", "block")}, &event.Event{Kind: "stop"}, nil, settings(100))
	if got.Action != "inject" || len(got.Blocked) != 0 || !strings.HasPrefix(got.Text, stopIntroduction) {
		t.Fatalf("Decide() = %+v", got)
	}
}
