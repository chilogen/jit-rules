// Package event defines normalized hook events.
package event

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Event is the harness-independent representation of one hook payload.
type Event struct {
	V              int            `json:"v" yaml:"-"`
	Harness        string         `json:"harness" yaml:"harness,omitempty"`
	HookEvent      string         `json:"hook_event" yaml:"-"`
	Kind           string         `json:"kind" yaml:"kind"`
	SessionID      string         `json:"session_id" yaml:"session_id,omitempty"`
	DedupeKey      string         `json:"dedupe_key" yaml:"-"`
	Cwd            string         `json:"cwd" yaml:"cwd,omitempty"`
	Root           string         `json:"root" yaml:"-"`
	TS             time.Time      `json:"ts" yaml:"-"`
	Tool           string         `json:"tool,omitempty" yaml:"tool,omitempty"`
	RawTool        string         `json:"raw_tool,omitempty" yaml:"raw_tool,omitempty"`
	Effect         string         `json:"effect,omitempty" yaml:"effect,omitempty"`
	Paths          []string       `json:"paths,omitempty" yaml:"paths,omitempty"`
	Shell          string         `json:"shell,omitempty" yaml:"shell,omitempty"`
	Content        string         `json:"content,omitempty" yaml:"content,omitempty"`
	ContentUnknown bool           `json:"content_unknown,omitempty" yaml:"content_unknown,omitempty"`
	Prompt         string         `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	StopActive     bool           `json:"stop_active,omitempty" yaml:"-"`
	Input          map[string]any `json:"-" yaml:"-"`
	Size           int            `json:"size" yaml:"-"`
}

// CanonicalTool maps a native tool name to its normalized name.
func CanonicalTool(raw string, table map[string]string) string {
	if canonical, ok := table[raw]; ok {
		return canonical
	}
	if strings.HasPrefix(raw, "mcp__") {
		parts := strings.SplitN(strings.TrimPrefix(raw, "mcp__"), "__", 3)
		if len(parts) == 3 {
			return "mcp/" + parts[0] + "/" + parts[1] + "__" + parts[2]
		}
		if len(parts) == 2 {
			return "mcp/" + parts[0] + "/" + parts[1]
		}
	}
	return strings.ToLower(raw)
}

// SizeOf returns the byte size used for session accounting.
func SizeOf(kind string, payload map[string]any) int {
	var value any
	switch kind {
	case "prompt":
		prompt, _ := payload["prompt"].(string)
		return len(prompt)
	case "tool":
		value = payload["tool_input"]
	case "tool_result":
		value = payload["tool_response"]
	case "stop":
		message, _ := payload["last_assistant_message"].(string)
		return len(message)
	default:
		return 0
	}
	data, _ := json.Marshal(value)
	return len(data)
}

// DedupeKey derives the stable claim key for an event.
func DedupeKey(kind string, fields []string, payload map[string]any, prompt string) string {
	switch kind {
	case "tool", "tool_result":
		if value, ok := firstField(fields, payload); ok {
			return "tool:" + value
		}
	case "prompt":
		if value, ok := firstField(fields, payload); ok {
			return "prompt:" + value
		}
		sum := sha1.Sum([]byte(prompt))
		return "prompt:sha1:" + hex.EncodeToString(sum[:])
	case "stop":
		active := "0"
		if value, _ := payload["stop_hook_active"].(bool); value {
			active = "1"
		}
		message, _ := payload["last_assistant_message"].(string)
		sum := sha1.Sum([]byte(active + "\n" + message))
		return "stop:sha1:" + hex.EncodeToString(sum[:])
	}
	return ""
}

func firstField(fields []string, payload map[string]any) (string, bool) {
	for _, field := range fields {
		if value, ok := payload[field]; ok {
			return fmt.Sprint(value), true
		}
	}
	return "", false
}
