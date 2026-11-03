package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/backend/fake"
	"github.com/chilogen/jit-rules/internal/profile"
	"gopkg.in/yaml.v3"
)

func TestAuthoringLoop(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	registerCLIFake(t, dir, "authoring", func(state map[string]any, _ backend.Question) float64 {
		shell, _ := state["shell"].(string)
		if strings.Contains(shell, "fix bug") || strings.Contains(shell, "docs") {
			return 0.9
		}
		return 0.1
	}, nil)

	content := authoringUnit("")
	stdout, stderr, code := runCLIInput(
		t, content, "unit", "new", "me", "git.commit-zh", "--from-stdin",
	)
	if code != 0 {
		t.Fatalf("unit new = %d, %q / %q", code, stdout, stderr)
	}
	assertUnitStatus(t, "me/git.commit-zh", "draft")

	stdout, _, code = runHookPayload(t, "git commit -m \"fix bug\"", "draft-1")
	if code != 0 || stdout != "" {
		t.Fatalf("draft hook = %d, %q", code, stdout)
	}
	_, stderr, code = runCLI(t, "unit", "enable", "git.commit-zh")
	if code != 1 || !strings.Contains(stderr, "unit test") {
		t.Fatalf("early enable = %d, %q", code, stderr)
	}
	stdout, stderr, code = runCLI(t, "unit", "test", "git.commit-zh")
	if code != 0 || !strings.Contains(stdout, "pass") {
		t.Fatalf("unit test = %d, %q / %q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "unit", "check", "git.commit-zh")
	if code != 0 || !strings.Contains(stdout, "没有冲突") {
		t.Fatalf("unit check = %d, %q / %q", code, stdout, stderr)
	}
	if _, stderr, code = runCLI(t, "unit", "enable", "git.commit-zh"); code != 0 {
		t.Fatalf("enable = %d, %q", code, stderr)
	}
	assertLockCounts(t, dir, "me/git.commit-zh", 4, 0, 0)

	_, _, code = runHookPayload(t, "git commit -m \"fix bug\"", "active-1")
	if code != 2 {
		t.Fatalf("active hook code = %d, want 2", code)
	}
	stdout, _, code = runHookPayload(t, "git commit -m \"修复登录错误\"", "active-2")
	if code != 0 || stdout != "" {
		t.Fatalf("compliant hook = %d, %q", code, stdout)
	}

	path := filepath.Join(dir, "units", "git.commit-zh.md")
	data := readFile(t, path)
	data = bytes.Replace(data, []byte("第一行"), []byte("首行"), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	assertUnitStatus(t, "me/git.commit-zh", "modified")
	stdout, _, code = runHookPayload(t, "git commit -m \"fix bug\"", "modified-1")
	if code != 0 || stdout != "" {
		t.Fatalf("modified hook = %d, %q", code, stdout)
	}
	if _, _, code = runCLI(t, "unit", "test", "git.commit-zh"); code != 0 {
		t.Fatalf("retest code = %d", code)
	}
	if _, _, code = runCLI(t, "unit", "enable", "git.commit-zh"); code != 0 {
		t.Fatalf("reenable code = %d", code)
	}
	assertUnitStatus(t, "me/git.commit-zh", "active")
}

func authoringUnit(topic string) string {
	topicLine := ""
	if topic != "" {
		topicLine = "topic: " + topic + "\n"
	}
	return fmt.Sprintf(`---
title: commit message 用中文写
trigger:
  events: [tool]
  tools: [shell]
  shell: ['git\s+commit\b']
judge:
  question: Is the commit message written in English?
deliver:
  as: block
%sexamples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m "fix bug"'}
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m "docs"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git commit -m "修复登录错误"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
commit message 用中文写，第一行不超过 50 个字。
`, topicLine)
}

func registerCLIFake(
	t *testing.T,
	dir, name string,
	fn func(map[string]any, backend.Question) float64,
	judgeErr error,
) {
	t.Helper()
	backend.Register(&fake.Backend{
		BackendName: name, Func: fn, Err: judgeErr,
		P: map[string]float64{"conflict": 0.9},
	})
	loaded, err := profile.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Settings = map[string]any{
		"judge": map[string]any{"backend": name},
		"backends": map[string]any{
			name: map[string]any{
				"threshold": 0.5, "model": "fake", "api_key_env": "FAKE_API_KEY",
			},
		},
	}
	data, err := yaml.Marshal(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runCLIInput(t *testing.T, input string, args ...string) (string, string, int) {
	t.Helper()
	rootCmd.SetIn(strings.NewReader(input))
	defer rootCmd.SetIn(nil)
	return runCLI(t, args...)
}

func runHookPayload(t *testing.T, command, toolUseID string) (string, string, int) {
	t.Helper()
	payload := map[string]any{
		"session_id": "session-authoring", "turn_id": "turn-authoring",
		"cwd": "/work/proj", "hook_event_name": "PreToolUse",
		"model": "fake", "tool_name": "Bash",
		"tool_input":  map[string]any{"command": command},
		"tool_use_id": toolUseID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return runCLIInput(t, string(data), "hook", "--harness", "codex")
}

func assertUnitStatus(t *testing.T, id, status string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "unit", "list", "--all", "--json")
	if code != 0 {
		t.Fatalf("unit list = %d, %q", code, stderr)
	}
	var result struct {
		Data []unitListItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	for _, item := range result.Data {
		if item.ID == id && item.Status == status {
			return
		}
	}
	t.Fatalf("%s status %s not found in %#v", id, status, result.Data)
}

func assertLockCounts(
	t *testing.T,
	dir, id string,
	passed, failed, skipped int,
) {
	t.Helper()
	lock, err := profile.LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := lock.Units[id].Tests
	if got.Passed != passed || got.Failed != failed || got.Skipped != skipped {
		t.Fatalf("test counts = %#v", got)
	}
}
