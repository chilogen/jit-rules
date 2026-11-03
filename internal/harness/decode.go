package harness

import (
	"encoding/json"
	"os"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/paths"
)

func decode(def *Definition, raw []byte, now time.Time) (*event.Event, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	hookEvent, _ := payload["hook_event_name"].(string)
	kind, ok := def.Events[hookEvent]
	if !ok {
		return nil, ErrIgnored
	}
	if kind == "session_start" && contains(def.CompactSources, stringField(payload, "source")) {
		kind = "compact"
	}
	cwd := stringField(payload, "cwd")
	ev := &event.Event{
		V:         1,
		Harness:   def.Name,
		HookEvent: hookEvent,
		Kind:      kind,
		SessionID: stringField(payload, "session_id"),
		Cwd:       cwd,
		Root:      paths.ProjectRoot(cwd),
		TS:        now,
		Prompt:    stringField(payload, "prompt"),
	}
	decodeTool(def, ev, payload)
	if kind == "stop" {
		ev.Content = stringField(payload, "last_assistant_message")
		ev.StopActive, _ = payload["stop_hook_active"].(bool)
	}
	ev.Size = event.SizeOf(kind, payload)
	fields := def.Dedupe.Tool
	if kind == "prompt" {
		fields = def.Dedupe.Prompt
	}
	ev.DedupeKey = event.DedupeKey(kind, fields, payload, ev.Prompt)
	return ev, nil
}

func decodeTool(def *Definition, ev *event.Event, payload map[string]any) {
	if ev.Kind != "tool" && ev.Kind != "tool_result" {
		return
	}
	ev.RawTool = stringField(payload, "tool_name")
	input, _ := payload["tool_input"].(map[string]any)
	ev.Input = input
	got := extract(def, ev.RawTool, input, ev.Cwd, ev.Root, fileExists)
	ev.Tool = got.Tool
	ev.Effect = got.Effect
	ev.Paths = got.Paths
	ev.Content = got.Content
	ev.ContentUnknown = got.ContentUnknown
	ev.Shell = got.Shell
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

func stringField(payload map[string]any, name string) string {
	value, _ := payload[name].(string)
	return value
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
