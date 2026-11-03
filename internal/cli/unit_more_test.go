package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestUnitNewRejectsRule12AndRemovesFile(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	invalid := strings.Replace(
		authoringUnit(""), "- expect: miss\n    event: {kind: tool, tool: shell",
		"- expect: miss\n    event: {kind: tool, tool: write", -1,
	)
	_, stderr, code := runCLIInput(
		t, invalid, "unit", "new", "me", "bad-block", "--from-stdin",
	)
	if code != 1 || !strings.Contains(stderr, "规则 12") {
		t.Fatalf("rule 12 new = %d, %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "units", "bad-block.md")); !os.IsNotExist(err) {
		t.Fatalf("invalid file remains: %v", err)
	}
}

func TestUnitNewRejectsPathLikeShortID(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	if _, _, code := runCLIInput(
		t, authoringUnit(""), "unit", "new", "me", "../escaped", "--from-stdin",
	); code != 1 {
		t.Fatalf("path-like id code = %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped.md")); !os.IsNotExist(err) {
		t.Fatalf("escaped file exists: %v", err)
	}
}

func TestUnitTestUnconfiguredCanEnableWithWarning(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	registerCLIFake(t, dir, "unit-unconfigured", nil, backend.ErrNotConfigured)
	if _, stderr, code := runCLIInput(
		t, authoringUnit(""), "unit", "new", "me", "unconfigured", "--from-stdin",
	); code != 0 {
		t.Fatalf("new = %d, %q", code, stderr)
	}
	stdout, stderr, code := runCLI(t, "unit", "test", "unconfigured")
	if code != 0 || !strings.Contains(stdout, "skipped") {
		t.Fatalf("test = %d, %q / %q", code, stdout, stderr)
	}
	_, stderr, code = runCLI(t, "unit", "enable", "unconfigured")
	if code != 0 || !strings.Contains(stderr, "跳过") {
		t.Fatalf("enable = %d, %q", code, stderr)
	}
	assertLockCounts(t, dir, "me/unconfigured", 1, 0, 3)
}

func TestUnitCheckConflictThenSameTopic(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	registerCLIFake(t, dir, "unit-conflict", func(
		_ map[string]any, question backend.Question,
	) float64 {
		if question.ID == "conflict" {
			return 0.9
		}
		return 0.9
	}, nil)
	writeUnitThroughCLI(t, "me", "chinese", authoringUnit(""))
	english := strings.Replace(
		authoringUnit(""), "commit message 用中文写", "commit message 用英文写", -1,
	)
	writeUnitThroughCLI(t, "me", "english", english)
	lockFirstUnit(t, dir, "chinese")

	stdout, _, code := runCLI(t, "unit", "check", "english")
	if code != 1 || !strings.Contains(stdout, "冲突：") ||
		!strings.Contains(stdout, "4. 两者其实不冲突") {
		t.Fatalf("conflict check = %d, %q", code, stdout)
	}

	addTopicAndRelock(t, dir, "chinese", "commit-language")
	addTopic(t, dir, "english", "commit-language")
	stdout, stderr, code := runCLI(t, "unit", "check", "english")
	if code != 0 || !strings.Contains(stdout, "没有冲突") {
		t.Fatalf("same topic = %d, %q / %q", code, stdout, stderr)
	}
}

func TestPackAddChecksConflictsWithProfileUnits(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	registerCLIFake(t, dir, "pack-conflict", func(
		state map[string]any, question backend.Question,
	) float64 {
		shell, _ := state["shell"].(string)
		if question.ID == "conflict" || strings.Contains(shell, "fix bug") ||
			strings.Contains(shell, "docs") {
			return 0.9
		}
		return 0.1
	}, nil)
	writeUnitThroughCLI(t, "me", "chinese", authoringUnit(""))
	lockFirstUnit(t, dir, "chinese")

	packDir := filepath.Join(home, "english-pack")
	english := strings.Replace(
		authoringUnit(""), "commit message 用中文写", "commit message 用英文写", -1,
	)
	for path, content := range map[string]string{
		"pack.yaml":        "name: english\nversion: 1.0.0\ndescription: 英文 commit\n",
		"units/english.md": english,
	} {
		full := filepath.Join(packDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, stderr, code := runCLI(t, "pack", "add", packDir)
	if code != 1 || !strings.Contains(stderr, "english/english ↔ me/chinese") {
		t.Fatalf("pack add = %d, %q", code, stderr)
	}
	if _, stderr, code := runCLI(t, "pack", "add", packDir, "--force"); code != 0 {
		t.Fatalf("pack add --force = %d, %q", code, stderr)
	}
}

func TestUnitDisableAndEnablePackUnit(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	if _, stderr, code := runCLI(t, "unit", "disable", "core/author"); code != 0 {
		t.Fatalf("disable = %d, %q", code, stderr)
	}
	loaded, err := profile.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(loaded.Disable, "core/author") {
		t.Fatalf("disable list = %#v", loaded.Disable)
	}
	if _, stderr, code := runCLI(t, "unit", "enable", "core/author"); code != 0 {
		t.Fatalf("enable = %d, %q", code, stderr)
	}
	loaded, _ = profile.Load(dir)
	if containsString(loaded.Disable, "core/author") {
		t.Fatalf("disable remains = %#v", loaded.Disable)
	}
}

func TestUnitListIncludesInvalidAndHookStillWorks(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	if err := os.WriteFile(
		filepath.Join(dir, "units", "broken.md"), []byte("not yaml"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	stdout, _, code := runCLI(t, "unit", "list", "--all")
	if code != 0 || !strings.Contains(stdout, "invalid") {
		t.Fatalf("list invalid = %d, %q", code, stdout)
	}
	if stdout, _, code := runHookPayload(t, "git status", "invalid-hook"); code != 0 || stdout != "" {
		t.Fatalf("hook with invalid = %d, %q", code, stdout)
	}
}

func writeUnitThroughCLI(t *testing.T, target, id, content string) {
	t.Helper()
	if _, stderr, code := runCLIInput(
		t, content, "unit", "new", target, id, "--from-stdin",
	); code != 0 {
		t.Fatalf("new %s = %d, %q", id, code, stderr)
	}
}

func lockFirstUnit(t *testing.T, dir, id string) {
	t.Helper()
	path := filepath.Join(dir, "units", id+".md")
	parsed, err := unit.ParseFile(path, "me")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := profile.LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	lock.Units[parsed.ID] = profile.LockUnit{Hash: parsed.Hash}
	if err := profile.SaveLock(dir, lock); err != nil {
		t.Fatal(err)
	}
}

func addTopicAndRelock(t *testing.T, dir, id, topic string) {
	t.Helper()
	addTopic(t, dir, id, topic)
	lockFirstUnit(t, dir, id)
}

func addTopic(t *testing.T, dir, id, topic string) {
	t.Helper()
	path := filepath.Join(dir, "units", id+".md")
	data := readFile(t, path)
	data = []byte(strings.Replace(string(data), "deliver:\n", "topic: "+topic+"\ndeliver:\n", 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestUnitRemoveRejectsPack(t *testing.T) {
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	if _, _, code := runCLI(t, "unit", "rm", "core/author"); code != 1 {
		t.Fatalf("rm pack code = %d", code)
	}
}
