package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/assets"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/installer"
	"github.com/chilogen/jit-rules/internal/journal"
)

type statusTestInstaller struct {
	kind      string
	installed bool
	err       error
	called    bool
}

func (i *statusTestInstaller) Kind() string { return i.kind }
func (i *statusTestInstaller) Plan(installer.Request) ([]installer.Change, error) {
	return nil, nil
}
func (i *statusTestInstaller) Installed(installer.Request) (bool, error) {
	i.called = true
	return i.installed, i.err
}

func TestUnitReplayShowsTwoOfThree(t *testing.T) {
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	writeUnitThroughCLI(t, "me", "replay", replayUnit())
	commands := []string{
		`git commit -m "first"`,
		"git status",
		`git commit -m "second"`,
	}
	for index, command := range commands {
		if _, _, code := runHookPayload(
			t, command, "replay-"+string(rune('a'+index)),
		); code != 0 {
			t.Fatalf("hook %d = %d", index, code)
		}
	}
	stdout, stderr, code := runCLI(t, "unit", "replay", "replay", "--since", "1h")
	if code != 0 || !strings.Contains(stdout, "命中率：2/3") {
		t.Fatalf("replay = %d, %q / %q", code, stdout, stderr)
	}
}

func TestExplainEventAndLastFailedField(t *testing.T) {
	home, _ := cliEnvironment(t)
	smoke := filepath.Join(home, ".jitr")
	copyTree(t, testDataPath(t, "smoke", "profile"), smoke)
	if _, stderr, code := runCLI(t, "profile", "register", smoke); code != 0 {
		t.Fatalf("register smoke = %d, %q", code, stderr)
	}
	payload := testDataPath(t, "payloads", "codex", "pre_shell_write.json")
	stdout, stderr, code := runCLI(t, "explain", "--event", payload)
	if code != 0 || !strings.Contains(stdout, "动作：block") {
		t.Fatalf("explain event = %d, %q / %q", code, stdout, stderr)
	}
	if _, _, code := runHookPayload(t, "cat note.txt", "explain-last"); code != 0 {
		t.Fatalf("hook = %d", code)
	}
	stdout, stderr, code = runCLI(t, "explain", "--last", "1")
	if code != 0 || !strings.Contains(stdout, "未通过门槛：tools") {
		t.Fatalf("explain last = %d, %q / %q", code, stdout, stderr)
	}
}

func TestExplainLastIncludesToolResult(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	want := event.Event{
		Harness: "fake", Kind: "tool_result", Tool: "shell",
		Shell: "git status",
	}
	if err := journal.Append(journal.Record{TS: time.Now().UTC(), Event: want}, true); err != nil {
		t.Fatal(err)
	}
	got, err := explainEvent("", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != want.Kind || got.Shell != want.Shell {
		t.Fatalf("explainEvent() = %#v, want %#v", got, want)
	}
}

func TestGuideMatchesEmbeddedAsset(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	want, err := assets.FS.ReadFile("guide.md")
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(t, "guide")
	if code != 0 || stderr != "" || stdout != string(want) {
		t.Fatalf("guide = %d, %q / %q", code, stdout, stderr)
	}
}

func TestStatusReportsInstalledHarnessAndMissingAPIKey(t *testing.T) {
	_, home := installFakeHomes(t)
	initCLIProfile(t, home, "me")
	t.Setenv("TYPESAFE_API_KEY", "")
	if _, stderr, code := runCLI(
		t, "install", "claude-code", "--scope", "user", "--dry-run=false",
	); code != 0 {
		t.Fatalf("install = %d, %q", code, stderr)
	}
	stdout, stderr, code := runCLI(t, "status")
	if code != 0 {
		t.Fatalf("status = %d, %q", code, stderr)
	}
	for _, text := range []string{
		"Claude Code：user=已安装",
		"Codex CLI：user=未安装",
		"TYPESAFE_API_KEY：未设置",
	} {
		if !strings.Contains(stdout, text) {
			t.Errorf("status lacks %q:\n%s", text, stdout)
		}
	}
}

func TestStatusContinuesAfterHarnessCheckFailure(t *testing.T) {
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	failing := &statusTestInstaller{kind: "status-failing", err: errors.New("broken config")}
	succeeding := &statusTestInstaller{kind: "status-succeeding", installed: true}
	installer.Register(failing)
	installer.Register(succeeding)
	dir := filepath.Join(home, "config", "harnesses")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	definitions := map[string]string{
		"90-failing.yaml":    statusHarnessDefinition("status-failing", "Status Failing"),
		"91-succeeding.yaml": statusHarnessDefinition("status-succeeding", "Status Succeeding"),
	}
	for name, definition := range definitions {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(definition), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stdout, stderr, code := runCLI(t, "status")
	if code != 0 || stderr != "" {
		t.Fatalf("status = %d, %q / %q", code, stdout, stderr)
	}
	if !failing.called || !succeeding.called {
		t.Fatalf("checks called: failing=%t succeeding=%t", failing.called, succeeding.called)
	}
	for _, text := range []string{
		"Status Failing 检查失败：user: broken config",
		"Status Succeeding：user=已安装",
	} {
		if !strings.Contains(stdout, text) {
			t.Errorf("status lacks %q:\n%s", text, stdout)
		}
	}
}

func statusHarnessDefinition(name, title string) string {
	return "name: " + name + "\n" +
		"title: " + title + "\n" +
		"detect: {order: 900}\n" +
		"events: {}\n" +
		"tools: {}\n" +
		"dedupe: {}\n" +
		"supports: {}\n" +
		"install:\n" +
		"  kind: " + name + "\n" +
		"  targets:\n" +
		"    user: {paths: [settings.json], mode: first}\n"
}

func TestPackAddFailedTestNeedsForce(t *testing.T) {
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	source := filepath.Join(home, "failing-pack")
	copyTree(t, fixturePath(t, "good"), source)
	path := filepath.Join(source, "units", "greet.md")
	data := strings.Replace(
		string(readFile(t, path)),
		`event: {kind: prompt, prompt: "你好"}`,
		`event: {kind: prompt, prompt: "再见"}`,
		1,
	)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runCLI(t, "pack", "add", source); code != 1 ||
		!strings.Contains(stderr, "测试未通过") {
		t.Fatalf("pack add = %d, %q", code, stderr)
	}
	if _, stderr, code := runCLI(t, "pack", "add", source, "--force"); code != 0 ||
		!strings.Contains(stderr, "测试未通过") {
		t.Fatalf("pack force = %d, %q", code, stderr)
	}
}

func replayUnit() string {
	return `---
title: replay commit
trigger:
  events: [tool]
  tools: [shell]
  shell: ['git\s+commit\b']
deliver:
  as: block
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m "fix"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
commit message must follow the rule.
`
}

func testDataPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	values := []string{filepath.Dir(file), "..", "..", "testdata"}
	values = append(values, parts...)
	return filepath.Clean(filepath.Join(values...))
}
