package judge

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestScriptExitMappingAndStderr(t *testing.T) {
	requireShell(t)
	tests := []struct {
		name        string
		script      string
		hit         bool
		probability float64
		fallback    bool
		exit        int
		stderr      string
	}{
		{name: "hit", script: "exit 0", hit: true, probability: 1, exit: 0},
		{name: "miss", script: "exit 1", exit: 1},
		{name: "fallback", script: "exit 3", hit: true, fallback: true, exit: 3},
		{name: "stderr", script: "echo oops >&2\nexit 1", exit: 1, stderr: "oops"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("JITR_HOME", t.TempDir())
			got := runScriptUnit(t, t.TempDir(), scriptUnit(test.name, test.script, nil), &event.Event{})
			if got.Hit != test.hit || got.P != test.probability || got.Fallback != test.fallback {
				t.Fatalf("verdict = %#v", got)
			}
			if got.Exit == nil || *got.Exit != test.exit {
				t.Fatalf("Exit = %#v, want %d", got.Exit, test.exit)
			}
			if got.Backend != "script" || got.Stderr != test.stderr {
				t.Fatalf("verdict = %#v", got)
			}
		})
	}
}

func TestScriptRunsInEventCwd(t *testing.T) {
	requireShell(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not available")
	}
	t.Setenv("JITR_HOME", t.TempDir())
	repo := t.TempDir()
	runGit(t, git, repo, "init", "-q")
	runGit(t, git, repo, "config", "user.email", "test@example.com")
	runGit(t, git, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, git, repo, "add", "file")
	runGit(t, git, repo, "commit", "-qm", "one")
	if err := os.WriteFile(filepath.Join(repo, "file"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, git, repo, "commit", "-qam", "two")
	runGit(t, git, repo, "checkout", "-q", "HEAD~1")

	got := runScriptUnit(
		t,
		repo,
		scriptUnit("cwd", `test "$(git rev-list --count HEAD)" -eq 1`, nil),
		&event.Event{},
	)
	if !got.Hit || got.Fallback {
		t.Fatalf("verdict = %#v", got)
	}
}

func TestScriptReadsEventAndUnitEnvironment(t *testing.T) {
	requireShell(t)
	t.Setenv("JITR_HOME", t.TempDir())
	cwd := t.TempDir()
	tests := []*unit.Unit{
		scriptUnit("stdin", `grep -q '"tool":"shell"'`, nil),
		scriptUnit("unit-env", `test "$JITR_UNIT" = "unit-env"`, nil),
	}
	ev := &event.Event{Kind: "tool", Tool: "shell"}
	got := Decide(context.Background(), evWithCwd(ev, cwd), tests, scriptSettings())
	for _, id := range []string{"stdin", "unit-env"} {
		if !got[id].Hit || got[id].Fallback {
			t.Fatalf("%s verdict = %#v", id, got[id])
		}
	}
}

func TestScriptUnconfiguredAndMissingDirectory(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	missingInterpreter := &unit.Unit{
		ID: "missing-interpreter",
		Judge: &unit.Judge{
			Lang:   "no-such-interp",
			Script: "exit 0",
		},
	}
	got := Decide(
		context.Background(),
		&event.Event{Cwd: t.TempDir()},
		[]*unit.Unit{missingInterpreter},
		scriptSettings(),
	)["missing-interpreter"]
	if !got.Unconfigured || got.Hit || !strings.Contains(got.Err, "no-such-interp") {
		t.Fatalf("missing interpreter verdict = %#v", got)
	}

	requireShell(t)
	missingDir := scriptUnit("missing-dir", "exit 0", nil)
	got = Decide(
		context.Background(),
		&event.Event{Cwd: filepath.Join(t.TempDir(), "missing")},
		[]*unit.Unit{missingDir},
		scriptSettings(),
	)["missing-dir"]
	if !got.Hit || !got.Fallback {
		t.Fatalf("missing directory verdict = %#v", got)
	}
}

func TestScriptDeadlineKillsProcess(t *testing.T) {
	requireShell(t)
	t.Setenv("JITR_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	got := Decide(
		ctx,
		&event.Event{Cwd: t.TempDir()},
		[]*unit.Unit{scriptUnit("timeout", "sleep 5", nil)},
		scriptSettings(),
	)["timeout"]
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("elapsed = %v, want under 1s", elapsed)
	}
	if !got.Hit || !got.Fallback || got.Exit != nil {
		t.Fatalf("verdict = %#v", got)
	}
}

func TestScriptCacheDefaultAndCwdKey(t *testing.T) {
	requireShell(t)
	home := t.TempDir()
	t.Setenv("JITR_HOME", home)
	countPath := filepath.Join(home, "count")
	script := fmt.Sprintf("echo x >> %s\nexit 0", strconv.Quote(countPath))
	cwdA := t.TempDir()
	cwdB := t.TempDir()

	uncached := scriptUnit("uncached", script, nil)
	runScriptUnit(t, cwdA, uncached, &event.Event{})
	runScriptUnit(t, cwdA, uncached, &event.Event{})
	if got := lineCount(t, countPath); got != 2 {
		t.Fatalf("default execution count = %d, want 2", got)
	}

	if err := os.Remove(countPath); err != nil {
		t.Fatal(err)
	}
	cache := true
	cachedUnit := scriptUnit("cached", script, &cache)
	first := runScriptUnit(t, cwdA, cachedUnit, &event.Event{})
	second := runScriptUnit(t, cwdA, cachedUnit, &event.Event{})
	third := runScriptUnit(t, cwdB, cachedUnit, &event.Event{})
	if got := lineCount(t, countPath); got != 2 {
		t.Fatalf("cached execution count = %d, want 2", got)
	}
	if first.Exit == nil || second.Ms != 0 || second.Exit != nil || third.Exit == nil {
		t.Fatalf("verdicts = first:%#v second:%#v third:%#v", first, second, third)
	}
}

func TestScriptFileNameExtensionAndReuse(t *testing.T) {
	requireShell(t)
	home := t.TempDir()
	t.Setenv("JITR_HOME", home)
	script := "exit 0"
	judgedUnit := scriptUnit("file", script, nil)
	runScriptUnit(t, t.TempDir(), judgedUnit, &event.Event{})
	sum := sha256.Sum256([]byte(script))
	path := filepath.Join(home, "cache", "scripts", fmt.Sprintf("%x.sh", sum))
	first, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	runScriptUnit(t, t.TempDir(), judgedUnit, &event.Event{})
	second, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Fatalf("script file was rewritten: %v != %v", first.ModTime(), second.ModTime())
	}
	for lang, want := range map[string]string{
		"sh": "sh", "pwsh": "ps1", "powershell": "ps1", "python3": "python3",
	} {
		if got := scriptExtension(lang); got != want {
			t.Fatalf("scriptExtension(%q) = %q, want %q", lang, got, want)
		}
	}
}

func TestScriptExit9009IsUnconfigured(t *testing.T) {
	got := verdictForExit("python3", 9009, 4, "", fmt.Errorf("exit status 9009"))
	if !got.Unconfigured || got.Hit || got.Fallback || got.Exit == nil || *got.Exit != 9009 {
		t.Fatalf("verdict = %#v", got)
	}
}

func requireShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script tests do not run on Windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is not available")
	}
}

func scriptUnit(id, script string, cache *bool) *unit.Unit {
	return &unit.Unit{
		ID: id,
		Judge: &unit.Judge{
			Lang:   "sh",
			Script: script,
			Cache:  cache,
		},
	}
}

func scriptSettings() profile.Settings {
	return profile.Settings{
		Judge: profile.JudgeSettings{
			TimeoutMs:     1000,
			MaxFieldChars: 100,
			SendContent:   true,
		},
	}
}

func runScriptUnit(t *testing.T, cwd string, judgedUnit *unit.Unit, ev *event.Event) Verdict {
	t.Helper()
	return Decide(
		context.Background(),
		evWithCwd(ev, cwd),
		[]*unit.Unit{judgedUnit},
		scriptSettings(),
	)[judgedUnit.ID]
}

func evWithCwd(ev *event.Event, cwd string) *event.Event {
	copy := *ev
	copy.Cwd = cwd
	return &copy
}

func runGit(t *testing.T, git, cwd string, args ...string) {
	t.Helper()
	command := exec.Command(git, args...)
	command.Dir = cwd
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

func TestScriptKilledBySignalHasNoExit(t *testing.T) {
	requireShell(t)
	t.Setenv("JITR_HOME", t.TempDir())
	got := runScriptUnit(t, t.TempDir(), scriptUnit("signal", "kill -9 $$", nil), &event.Event{})
	if !got.Hit || !got.Fallback || got.Exit != nil {
		t.Fatalf("verdict = %#v", got)
	}
}
