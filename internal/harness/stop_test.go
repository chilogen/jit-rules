package harness

import (
	"encoding/json"
	"testing"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
)

func TestEncodeStopInjectRefusesEnd(t *testing.T) {
	def := &Definition{Supports: Supports{SystemMessage: true}}
	ev := &event.Event{HookEvent: "Stop", Kind: "stop"}
	got := Encode(def, ev, deliver.Result{Action: "inject", Text: "remind \"x\"", Notice: "n"})
	if got.Exit != 0 || len(got.Stderr) != 0 {
		t.Fatalf("got %#v", got)
	}
	var out map[string]any
	if err := json.Unmarshal(got.Stdout, &out); err != nil {
		t.Fatal(err)
	}
	if out["decision"] != "block" || out["reason"] != "remind \"x\"" || out["systemMessage"] != "n" ||
		out["hookSpecificOutput"] != nil {
		t.Fatalf("stdout = %s", got.Stdout)
	}
	if silent := Encode(def, ev, deliver.Result{Action: "none"}); len(silent.Stdout) != 0 || silent.Exit != 0 {
		t.Fatalf("none = %#v", silent)
	}
}
