package hooksjson

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/installer"
)

func TestClaudeRoundTripAndIdempotency(t *testing.T) {
	_, home := fakeHomes(t)
	def := findDefinition(t, "claude-code")
	target := filepath.Join(home, ".claude", "settings.json")
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "install", "claude-settings-with-other-hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, target, fixture)
	request := requestFor(def)
	installChanges := plan(t, request)
	if len(installChanges) != 1 {
		t.Fatalf("install changes = %d, want 1", len(installChanges))
	}
	apply(t, installChanges, 0)
	installed := readFile(t, target)
	if !bytes.Contains(installed, []byte("2>/dev/null && echo ok")) ||
		!bytes.Contains(installed, []byte("12345678901234567890")) ||
		!bytes.Contains(installed, []byte("1.5")) {
		t.Fatalf("install changed preserved values:\n%s", installed)
	}
	if changes := plan(t, request); len(changes) != 0 {
		t.Fatalf("second install produced %d changes", len(changes))
	}
	request.Uninstall = true
	uninstallChanges := plan(t, request)
	if len(uninstallChanges) != 1 {
		t.Fatalf("uninstall changes = %d, want 1", len(uninstallChanges))
	}
	apply(t, uninstallChanges, 1)
	if restored := readFile(t, target); !bytes.Equal(restored, fixture) {
		t.Fatalf("restored file differs:\n%s", restored)
	}
}

func TestCodexLifecycleAndHandlerExtra(t *testing.T) {
	_, home := fakeHomes(t)
	def := findDefinition(t, "codex")
	target := filepath.Join(home, ".codex", "hooks.json")
	request := requestFor(def)
	assertInstalled(t, request, false)
	changes := plan(t, request)
	apply(t, changes, 0)
	assertInstalled(t, request, true)
	var root map[string]any
	if err := json.Unmarshal(readFile(t, target), &root); err != nil {
		t.Fatal(err)
	}
	hooks := root["hooks"].(map[string]any)
	for event, value := range hooks {
		handler := value.([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
		_, hasLimit := handler["additionalContextLimit"]
		wantLimit := event == "PreToolUse" || event == "UserPromptSubmit"
		if hasLimit != wantLimit {
			t.Errorf("%s additionalContextLimit present = %v, want %v", event, hasLimit, wantLimit)
		}
	}
	request.Uninstall = true
	changes = plan(t, request)
	if len(changes) != 1 || changes[0].After != nil {
		t.Fatalf("uninstall change = %#v", changes)
	}
	apply(t, changes, 1)
	assertInstalled(t, request, false)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target still exists: %v", err)
	}
}

func TestTraeExistingTargetsAndDefaults(t *testing.T) {
	_, home := fakeHomes(t)
	def := findDefinition(t, "trae")
	request := requestFor(def)
	assertInstalled(t, request, false)
	if _, err := (implementation{}).Plan(request); err == nil {
		t.Fatal("Plan without a config directory succeeded")
	}
	targetDir := filepath.Join(home, ".trae-cn")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	changes := plan(t, request)
	if len(changes) != 1 || changes[0].Path != filepath.Join(targetDir, "hooks.json") {
		t.Fatalf("changes = %#v", changes)
	}
	apply(t, changes, 0)
	data := readFile(t, changes[0].Path)
	if !bytes.Contains(data, []byte(`"version": 1`)) {
		t.Fatalf("missing version default:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(home, ".trae", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("unexpected alternate target: %v", err)
	}
	assertInstalled(t, request, true)
	request.Uninstall = true
	changes = plan(t, request)
	if len(changes) != 1 || changes[0].After != nil {
		t.Fatalf("uninstall change = %#v", changes)
	}
	apply(t, changes, 1)
	assertInstalled(t, request, false)
}

func TestInstalledIgnoresOtherHooks(t *testing.T) {
	_, home := fakeHomes(t)
	def := findDefinition(t, "claude-code")
	target := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, target, []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"command":"other"}]}]}}`))
	assertInstalled(t, requestFor(def), false)
}

func TestCommandStableAcrossInstallPlans(t *testing.T) {
	fakeHomes(t)
	def := findDefinition(t, "codex")
	request := requestFor(def)
	first := plan(t, request)
	apply(t, first, 0)
	before := append([]byte(nil), first[0].After...)
	if changes := plan(t, request); len(changes) != 0 {
		t.Fatalf("second plan = %#v", changes)
	}
	if got := readFile(t, first[0].Path); !bytes.Equal(got, before) {
		t.Fatal("second install changed command or output")
	}
}

func requestFor(def *harness.Definition) installer.Request {
	return installer.Request{
		Def: def, Scope: "user", Bin: filepath.Join(string(filepath.Separator), "stable", "jitr"),
		GOOS: "linux",
	}
}

func plan(t *testing.T, request installer.Request) []installer.Change {
	t.Helper()
	changes, err := (implementation{}).Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	return changes
}

func apply(t *testing.T, changes []installer.Change, seconds int) {
	t.Helper()
	now := time.Date(2026, 11, 2, 10, 0, seconds, 0, time.UTC)
	if err := installer.Apply(changes, now); err != nil {
		t.Fatal(err)
	}
}

func assertInstalled(t *testing.T, request installer.Request, want bool) {
	t.Helper()
	got, err := (implementation{}).Installed(request)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Installed() = %v, want %v", got, want)
	}
}

func findDefinition(t *testing.T, name string) *harness.Definition {
	t.Helper()
	def, err := harness.Find(name)
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func fakeHomes(t *testing.T) (string, string) {
	t.Helper()
	jitrHome := t.TempDir()
	home := t.TempDir()
	t.Setenv("JITR_HOME", jitrHome)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return jitrHome, home
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
