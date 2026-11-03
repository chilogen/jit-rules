package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/journal"
)

func TestJudgmentJoinTwoRuleScenario(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses git and shell scripts")
	}
	for _, program := range []string{"git", "sh"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skip(program + " is unavailable")
		}
	}
	home, _ := cliEnvironment(t)
	repo := filepath.Join(home, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "branch", "-M", "master")
	runGitTest(t, repo, "commit", "--allow-empty", "-m", "base")
	runGitTest(t, repo, "checkout", "-b", "feature")
	addEmptyCommits(t, repo, 2)

	fixture := filepath.Join(repo, ".jitr")
	copyTree(t, testDataPath(t, "profiles", "branch"), fixture)
	t.Chdir(repo)
	if _, stderr, code := runCLI(t, "profile", "register", fixture); code != 0 {
		t.Fatalf("register = %d, %q", code, stderr)
	}
	for _, id := range []string{"merge-master", "notify-user"} {
		stdout, stderr, code := runCLI(t, "unit", "test", id)
		if code != 0 || !strings.Contains(stdout, "skipped") {
			t.Fatalf("test %s = %d, %q / %q", id, code, stdout, stderr)
		}
		_, stderr, code = runCLI(t, "unit", "enable", id)
		if code != 0 || !strings.Contains(stderr, "跳过") {
			t.Fatalf("enable %s = %d, %q", id, code, stderr)
		}
	}
	stdout, stderr, code := runCLI(t, "unit", "show", "merge-master")
	if code != 0 || !strings.Contains(stdout, "git rev-list --count master..HEAD") {
		t.Fatalf("unit show = %d, %q / %q", code, stdout, stderr)
	}
	_, stderr, code = runCLI(t, "unit", "check", "merge-master")
	if code != 0 || !strings.Contains(stderr, "可能冲突") {
		t.Fatalf("unit check = %d, %q", code, stderr)
	}

	two := runBranchHook(t, repo, "ahead-2")
	if two != "" {
		t.Fatalf("2 commits output = %q", two)
	}
	addEmptyCommits(t, repo, 2)
	four := runBranchHook(t, repo, "ahead-4")
	if !strings.Contains(four, "branch/merge-master") ||
		strings.Contains(four, "branch/notify-user") {
		t.Fatalf("4 commits output = %q", four)
	}
	addEmptyCommits(t, repo, 2)
	six := runBranchHook(t, repo, "ahead-6")
	mergeAt := strings.Index(six, "branch/merge-master")
	notifyAt := strings.Index(six, "branch/notify-user")
	if mergeAt < 0 || notifyAt < 0 || mergeAt >= notifyAt {
		t.Fatalf("6 commits output = %q", six)
	}

	records, err := journal.Since(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	assertBranchJournal(t, records, "ahead-4", map[string]int{
		"branch/merge-master": 0, "branch/notify-user": 1,
	})
	assertBranchJournal(t, records, "ahead-6", map[string]int{
		"branch/merge-master": 0, "branch/notify-user": 0,
	})
}

func TestUnitReplayScriptUnknownEvenLive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	home, cwd := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	marker := filepath.Join(home, "script-ran")
	writeUnitThroughCLI(t, "me", "marker", scriptUnit(
		"Marker", "touch "+marker+"\nexit 0",
	))
	if _, _, code := runCLI(t, "unit", "test", "marker"); code != 0 {
		t.Fatalf("unit test code = %d", code)
	}
	if _, _, code := runCLI(t, "unit", "enable", "marker"); code != 0 {
		t.Fatalf("unit enable code = %d", code)
	}
	if err := journal.Append(journal.Record{
		V: 1, TS: time.Now().UTC(),
		Event: event.Event{
			Harness: "fake", Kind: "tool", Tool: "shell",
			Shell: "git commit -m x", Cwd: cwd,
		},
	}, true); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLI(
		t, "unit", "replay", "marker", "--since", "1h", "--live",
	)
	if code != 0 || !strings.Contains(stdout, "未知：1") {
		t.Fatalf("replay = %d, %q / %q", code, stdout, stderr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("script ran during replay: %v", err)
	}
}

func TestDescribeThresholdsAndStatusListsAllJudges(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	writeUnitThroughCLI(t, "me", "default-question", questionUnit("Default", ""))
	writeUnitThroughCLI(t, "me", "other-question", questionUnit("Other", "openai"))
	writeUnitThroughCLI(t, "me", "shell-script", scriptUnit("Shell", "exit 0"))
	for _, id := range []string{"default-question", "other-question", "shell-script"} {
		lockFirstUnit(t, dir, id)
	}
	stdout, stderr, code := runCLI(t, "unit", "show", "other-question")
	if code != 0 || !strings.Contains(stdout, "概率 ≥ 0.5") {
		t.Fatalf("unit show = %d, %q / %q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "profile", "show", "me")
	if code != 0 || !strings.Contains(stdout, "概率 ≥ 0.3") ||
		!strings.Contains(stdout, "概率 ≥ 0.5") {
		t.Fatalf("profile show = %d, %q / %q", code, stdout, stderr)
	}
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	stdout, stderr, code = runCLI(t, "status")
	if code != 0 {
		t.Fatalf("status = %d, %q", code, stderr)
	}
	for _, text := range []string{"TYPESAFE_API_KEY", "OPENAI_API_KEY", "脚本解释器：sh"} {
		if !strings.Contains(stdout, text) {
			t.Errorf("status lacks %q:\n%s", text, stdout)
		}
	}
}

func runBranchHook(t *testing.T, repo, session string) string {
	t.Helper()
	data := readFile(t, testDataPath(t, "payloads", "claude-code", "pre_shell.json"))
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	payload["cwd"] = repo
	payload["session_id"] = session
	payload["tool_use_id"] = "tool-" + session
	payload["tool_input"].(map[string]any)["command"] = "git commit -m x"
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runCLIInput(
		t, string(data), "hook", "--harness", "claude-code",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("hook %s = %d, %q", session, code, stderr)
	}
	return stdout
}

func assertBranchJournal(
	t *testing.T,
	records []journal.Record,
	session string,
	want map[string]int,
) {
	t.Helper()
	for _, record := range records {
		if record.Event.SessionID != session {
			continue
		}
		got := map[string]int{}
		for _, saved := range record.Units {
			if saved.Judge == nil || saved.Judge.Backend != "script" ||
				saved.Judge.Exit == nil {
				t.Fatalf("%s unit = %#v", session, saved)
			}
			got[saved.ID] = *saved.Judge.Exit
		}
		for id, exit := range want {
			if got[id] != exit {
				t.Fatalf("%s journal exits = %#v, want %s=%d", session, got, id, exit)
			}
		}
		return
	}
	t.Fatalf("journal record for %s not found", session)
}

func addEmptyCommits(t *testing.T, repo string, count int) {
	t.Helper()
	for range count {
		runGitTest(t, repo, "commit", "--allow-empty", "-m", "ahead")
	}
}

func scriptUnit(title, script string) string {
	return `---
title: ` + title + `
trigger:
  events: [tool]
  tools: [shell]
  shell: ['^git\s+commit\b']
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m x'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
` + title + ` body.

` + "```sh jitr-judge\n" + script + "\n```\n"
}

func questionUnit(title, backend string) string {
	backendLine := ""
	if backend != "" {
		backendLine = "\n  backend: " + backend
	}
	return `---
title: ` + title + `
trigger:
  events: [tool]
  tools: [shell]
  shell: ['^git\s+commit\b']
judge:
  question: Is this a commit?` + backendLine + `
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m x'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
` + title + " body.\n"
}
