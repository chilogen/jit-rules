package harness

import (
	"encoding/json"
	"strings"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
)

type encodedOutput struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func encode(def *Definition, ev *event.Event, res deliver.Result) Output {
	if res.Action == "block" {
		return Output{Stderr: []byte(res.Text), Exit: 2}
	}
	payload := encodedOutput{}
	if res.Action == "inject" && ev.Kind == "stop" {
		payload.Decision, payload.Reason = "block", res.Text
	} else if res.Action == "inject" {
		payload.HookSpecificOutput = &hookSpecificOutput{
			HookEventName: ev.HookEvent, AdditionalContext: res.Text,
		}
	}
	if res.Notice != "" && def.Supports.SystemMessage {
		payload.SystemMessage = res.Notice
	}
	if payload.HookSpecificOutput == nil && payload.Decision == "" && payload.SystemMessage == "" {
		return Output{}
	}
	data, _ := json.Marshal(payload)
	return Output{Stdout: data}
}

func hookEntries(def *Definition, command string) map[string][]any {
	result := make(map[string][]any, len(def.Events))
	for native := range def.Events {
		handler := map[string]any{
			"type":    "command",
			"command": command,
			"timeout": def.Install.TimeoutSeconds,
		}
		for key, value := range def.Install.HandlerExtra[native] {
			handler[key] = value
		}
		group := map[string]any{"hooks": []any{handler}}
		if native == "PreToolUse" || native == "PostToolUse" {
			group["matcher"] = "*"
		}
		result[native] = []any{group}
	}
	return result
}

func commandString(def *Definition, bin, goos string) string {
	template := def.Install.Command.Unix
	if goos == "windows" {
		template = def.Install.Command.Windows
	}
	template = strings.ReplaceAll(template, "{bin}", bin)
	return strings.ReplaceAll(template, "{name}", def.Name)
}
