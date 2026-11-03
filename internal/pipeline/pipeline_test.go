package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
	"github.com/chilogen/jit-rules/internal/backend/fake"
	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/gofrs/flock"
)

var testNow = time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)

func TestRunDeliveryStateAndDedupe(t *testing.T) {
	home := setupFixtureProfile(t, "testdata/profiles/basic", "")
	prompt := fixture(t, "testdata/payloads/claude-code/prompt.json")
	out := Run(prompt, testNow)
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), `"hookEventName":"UserPromptSubmit"`) ||
		!strings.Contains(string(out.Stdout), "basic/prompt-all") {
		t.Fatalf("prompt output = %+v", out)
	}

	write := fixture(t, "testdata/payloads/claude-code/pre_write.json")
	out = Run(write, testNow.Add(time.Second))
	if out.Exit != 2 || !strings.HasPrefix(string(out.Stderr), "jit-rules 拦截了这次调用") ||
		!strings.Contains(string(out.Stderr), "basic/txt-block") ||
		!strings.Contains(string(out.Stderr), "basic/txt-inject") {
		t.Fatalf("block output = %+v", out)
	}
	out = Run(changeJSON(t, write, "tool_use_id", "second"), testNow.Add(2*time.Second))
	if out.Exit != 2 || !strings.Contains(string(out.Stderr), "basic/txt-block") ||
		strings.Contains(string(out.Stderr), "basic/txt-inject") {
		t.Fatalf("repeat block output = %+v", out)
	}
	compliant := changeToolContent(t, write, "third", "HELLO WORLD")
	out = Run(compliant, testNow.Add(3*time.Second))
	assertSilent(t, out)
	if gate := gateOf(records(t), "basic/txt-block"); gate != "content" {
		t.Fatalf("delivered unit failing the gate: gate = %q", gate)
	}

	setProfileSettings(t, filepath.Join(home, ".jitr"), "redeliver_bytes: 10")
	Run(fixture(t, "testdata/payloads/claude-code/post_edit.json"), testNow.Add(4*time.Second))
	out = Run(changeToolContent(t, write, "fourth", "HELLO WORLD"), testNow.Add(5*time.Second))
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), "basic/txt-inject") {
		t.Fatalf("redelivered output = %+v", out)
	}

	duplicate := changeJSON(t, write, "tool_use_id", "duplicate")
	Run(duplicate, testNow.Add(6*time.Second))
	assertSilent(t, Run(duplicate, testNow.Add(7*time.Second)))
	records := records(t)
	if records[len(records)-1].Err != "dedupe" {
		t.Fatalf("duplicate record = %+v", records[len(records)-1])
	}
}

func TestPostToolUseDelivery(t *testing.T) {
	t.Run("inject", func(t *testing.T) {
		home := setupCustomProfile(t, map[string]string{
			"post.md": toolResultUnit("Post check", "inject"),
		}, "")
		pre := fixture(t, "testdata/payloads/claude-code/pre_edit.json")
		post := fixture(t, "testdata/payloads/claude-code/post_edit.json")
		assertSilent(t, Run(pre, testNow))
		out := Run(post, testNow)
		if out.Exit != 0 || !strings.Contains(string(out.Stdout), `"hookEventName":"PostToolUse"`) ||
			!strings.Contains(string(out.Stdout), "custom/post") {
			t.Fatalf("post inject output = %+v", out)
		}
		got := records(t)
		record := got[len(got)-1]
		if record.Event.Kind != "tool_result" || record.Result.Action != "inject" {
			t.Fatalf("post inject record = %+v", record)
		}
		bytes := sessionBytes(t, home)
		assertSilent(t, Run(post, testNow))
		got = records(t)
		if got[len(got)-1].Err != "dedupe" {
			t.Fatalf("duplicate post record = %+v", got[len(got)-1])
		}
		if duplicateBytes := sessionBytes(t, home); duplicateBytes != bytes {
			t.Fatalf("duplicate post bytes = %d, want %d", duplicateBytes, bytes)
		}
	})
	t.Run("block", func(t *testing.T) {
		setupCustomProfile(t, map[string]string{
			"post.md": toolResultUnit("Post check", "block"),
		}, "")
		out := Run(fixture(t, "testdata/payloads/claude-code/post_edit.json"), testNow)
		if out.Exit != 2 || !strings.HasPrefix(string(out.Stderr), "jit-rules 要求先修复工具结果") {
			t.Fatalf("post block output = %+v", out)
		}
	})
}

func TestPostToolUseDoesNotReacquireStateAfterClaimTimeout(t *testing.T) {
	home := setupCustomProfile(t, map[string]string{
		"post.md": toolResultUnit("Post check", "inject"),
	}, "")
	statePath := filepath.Join(home, "state", "sessions", "claude-code",
		"40002396-c1ff-4904-bcd2-d2592595627b.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	stateLock := flock.New(statePath + ".lock")
	locked, err := stateLock.TryLock()
	if err != nil || !locked {
		t.Fatalf("lock state: locked=%t, err=%v", locked, err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = stateLock.Unlock()
		close(released)
	}()

	out := Run(fixture(t, "testdata/payloads/claude-code/post_edit.json"), testNow)
	<-released
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), "custom/post") {
		t.Fatalf("stateless post output = %+v", out)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("stateless post wrote state: %v", err)
	}
}

func TestCompactTopicAndTruncation(t *testing.T) {
	t.Run("compact", func(t *testing.T) {
		setupFixtureProfile(t, "testdata/profiles/basic", "")
		raw := fixture(t, "testdata/payloads/claude-code/prompt.json")
		Run(raw, testNow)
		Run(fixture(t, "testdata/payloads/claude-code/session_compact.json"), testNow.Add(time.Second))
		out := Run(changeJSON(t, raw, "prompt_id", "after-compact"), testNow.Add(2*time.Second))
		if !strings.Contains(string(out.Stdout), "basic/prompt-all") {
			t.Fatalf("after compact = %+v", out)
		}
	})
	t.Run("topic", func(t *testing.T) {
		setupCustomProfile(t, map[string]string{
			"first.md":  toolUnit("First", "inject", "same", ""),
			"second.md": toolUnit("Second", "block", "same", ""),
		}, "")
		out := Run(fixture(t, "testdata/payloads/claude-code/pre_write.json"), testNow)
		if out.Exit != 0 || !strings.Contains(string(out.Stdout), "custom/first") {
			t.Fatalf("topic output = %+v", out)
		}
		got := records(t)[0]
		if len(got.Units) != 2 || got.Units[1].Skip != "topic" {
			t.Fatalf("topic units = %+v", got.Units)
		}
	})
	t.Run("tree-topic", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("JITR_HOME", home)
		t.Setenv("HOME", home)
		parent := filepath.Join(home, "a")
		child := filepath.Join(parent, "b")
		dirs := []string{
			filepath.Join(home, ".jitr"),
			filepath.Join(parent, ".jitr"),
			filepath.Join(child, ".jitr"),
		}
		for index, name := range []string{"root", "parent", "child"} {
			writeActiveProfile(t, dirs[index], name, toolUnit(name, "inject", "same", ""))
		}
		writeFile(t, filepath.Join(home, "config", "registry.yaml"), fmt.Sprintf(
			"version: 1\nprofiles:\n  - path: %s\n  - path: %s\n  - path: %s\n",
			dirs[0], dirs[1], dirs[2],
		))
		raw := replaceFixtureCWD(
			fixture(t, "testdata/payloads/claude-code/pre_write.json"),
			filepath.Join(child, "work"),
		)
		out := Run(raw, testNow)
		if out.Exit != 0 || !strings.Contains(string(out.Stdout), "child/rule") ||
			strings.Contains(string(out.Stdout), "parent/rule") ||
			strings.Contains(string(out.Stdout), "root/rule") {
			t.Fatalf("tree topic output = %+v", out)
		}
		got := records(t)[0]
		if len(got.Units) != 3 ||
			got.Units[0].ID != "child/rule" ||
			got.Units[1].Skip != "topic" ||
			got.Units[2].Skip != "topic" {
			t.Fatalf("tree topic units = %+v", got.Units)
		}
	})
	t.Run("truncated", func(t *testing.T) {
		setupFixtureProfile(t, "testdata/profiles/basic", "max_inject_chars: 10")
		Run(fixture(t, "testdata/payloads/claude-code/pre_write.json"), testNow)
		got := records(t)[0]
		if len(got.Units) != 2 || got.Units[1].Skip != "truncated" {
			t.Fatalf("truncated units = %+v", got.Units)
		}
	})
}

func TestShellWritesAndUnknownContent(t *testing.T) {
	setupFixtureProfile(t, "testdata/smoke/profile", "")
	for _, name := range []string{
		"testdata/payloads/codex/pre_shell_write.json",
		"testdata/payloads/claude-code/pre_shell_heredoc.json",
	} {
		out := Run(fixture(t, name), testNow)
		if out.Exit != 2 || !strings.Contains(string(out.Stderr), "smoke/md.first-line-h1") {
			t.Fatalf("%s output = %+v", name, out)
		}
	}
	assertSilent(t, Run(fixture(t, "testdata/payloads/codex/pre_apply_patch.json"), testNow))

	raw := fixture(t, "testdata/payloads/codex/pre_shell.json")
	raw = setToolCommand(t, raw, `echo "$X" > a.md`, "unknown-content")
	raw = changeJSON(t, raw, "session_id", "unknown-content-session")
	out := Run(raw, testNow.Add(time.Second))
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), "smoke/md.first-line-h1") {
		t.Fatalf("unknown content output = %+v", out)
	}
	got := records(t)
	last := got[len(got)-1]
	if len(last.Units) != 1 || !last.Units[0].NoBlock {
		t.Fatalf("unknown content record = %+v", last)
	}
}

func TestJudgeFallbackUnconfiguredAndJournal(t *testing.T) {
	t.Run("fallback", func(t *testing.T) {
		name := uniqueName("fake-timeout")
		backend.Register(&fake.Backend{BackendName: name, Delay: 50 * time.Millisecond})
		setupCustomProfile(t, map[string]string{
			"judged.md": toolUnit("Judged", "block", "", "judge:\n  question: Is this risky?\n"),
		}, fmt.Sprintf("judge:\n  backend: %s\n  timeout_ms: 1\nbackends:\n  %s:\n    threshold: 0.5", name, name))
		out := Run(fixture(t, "testdata/payloads/claude-code/pre_write.json"), testNow)
		if out.Exit != 0 || !strings.Contains(string(out.Stdout), "custom/judged") {
			t.Fatalf("fallback output = %+v", out)
		}
	})
	t.Run("unconfigured-once", func(t *testing.T) {
		name := uniqueName("fake-unconfigured")
		backend.Register(&fake.Backend{
			BackendName: name,
			Err:         fmt.Errorf("credentials: %w", backend.ErrNotConfigured),
		})
		setupCustomProfile(t, map[string]string{
			"judged.md":  toolUnit("Judged", "inject", "", "judge:\n  question: Is this risky?\n"),
			"judged2.md": toolUnit("Judged 2", "inject", "", "judge:\n  question: Is this slow?\n"),
		}, fmt.Sprintf("judge:\n  backend: %s\nbackends:\n  %s:\n    threshold: 0.5", name, name))
		raw := fixture(t, "testdata/payloads/claude-code/pre_write.json")
		first := Run(raw, testNow)
		if !strings.Contains(string(first.Stdout), "systemMessage") ||
			strings.Count(string(first.Stdout), "judge 后端没有配置好") != 1 {
			t.Fatalf("first notice = %+v", first)
		}
		second := Run(changeJSON(t, raw, "tool_use_id", "unconfigured-2"), testNow.Add(time.Second))
		assertSilent(t, second)
	})
	t.Run("journal-fields", func(t *testing.T) {
		name := uniqueName("fake-hit")
		backend.Register(&fake.Backend{BackendName: name, DefaultP: 0.9, ConnMs: 37})
		setupCustomProfile(t, map[string]string{
			"judged.md": toolUnit("Judged", "inject", "", "judge:\n  question: Is this risky?\n"),
		}, fmt.Sprintf("judge:\n  backend: %s\nbackends:\n  %s:\n    model: fake-1\n    threshold: 0.5", name, name))
		t.Setenv("JITR_ISOLATED", "1")
		Run(fixture(t, "testdata/payloads/claude-code/pre_write.json"), testNow)
		record := records(t)[0]
		if record.V != 1 || !record.Isolated || record.Event.Kind != "tool" ||
			len(record.Profiles) != 1 || len(record.Units) != 1 ||
			record.Units[0].Judge == nil || record.Units[0].Judge.ConnMs != 37 ||
			record.Result.Action != "inject" {
			t.Fatalf("record = %+v", record)
		}
	})
}

func TestScriptFallbackNeverBlocksAndRecordsDiagnostics(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	setupCustomProfile(t, map[string]string{
		"script.md": scriptToolUnit("Script fallback", "block", "echo failed >&2\nexit 3"),
	}, "")
	raw := changeJSON(
		t, fixture(t, "testdata/payloads/claude-code/pre_shell.json"), "cwd", t.TempDir(),
	)
	out := Run(raw, testNow)
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), "custom/script") {
		t.Fatalf("fallback output = %+v", out)
	}
	record := records(t)[0]
	judged := record.Units[0].Judge
	if judged == nil || !judged.Fallback || judged.Exit == nil || *judged.Exit != 3 ||
		judged.Stderr != "failed" {
		t.Fatalf("script journal = %+v", judged)
	}
}

func TestScriptJudgeRespectsHookDeadline(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is unavailable")
	}
	setupCustomProfile(t, map[string]string{
		"slow.md": scriptToolUnit("Slow script", "inject", "sleep 5"),
	}, "deadline_ms: 300")
	raw := changeJSON(
		t, fixture(t, "testdata/payloads/claude-code/pre_shell.json"), "cwd", t.TempDir(),
	)
	started := time.Now()
	out := Run(raw, testNow)
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("hook took %s", elapsed)
	}
	if out.Exit != 0 || !strings.Contains(string(out.Stdout), "custom/slow") {
		t.Fatalf("deadline output = %+v", out)
	}
}

func TestFailuresConcurrencyDisableAndHarnesses(t *testing.T) {
	t.Run("failures", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("JITR_HOME", home)
		assertSilent(t, Run([]byte("not json"), testNow))
		writeFile(t, filepath.Join(home, "config", "registry.yaml"), "not: [yaml")
		assertSilent(t, Run(fixture(t, "testdata/payloads/claude-code/prompt.json"), testNow))
		if countLines(t, filepath.Join(home, "state", "errors.jsonl")) != 2 {
			t.Fatal("expected two pipeline errors")
		}
	})
	t.Run("broken-profile-continues", func(t *testing.T) {
		home := setupFixtureProfile(t, "testdata/profiles/basic", "")
		broken := filepath.Join(home, "work", ".jitr")
		writeFile(t, filepath.Join(broken, "profile.yaml"), "not: [yaml")
		writeFile(t, filepath.Join(home, "config", "registry.yaml"), fmt.Sprintf(
			"version: 1\nprofiles:\n  - path: %s\n  - path: %s\n",
			broken, filepath.Join(home, ".jitr")))
		raw := replaceFixtureCWD(
			fixture(t, "testdata/payloads/claude-code/prompt.json"),
			filepath.Join(home, "work", "project"),
		)
		out := Run(raw, testNow)
		if !strings.Contains(string(out.Stdout), "basic/prompt-all") ||
			!strings.Contains(string(out.Stdout), "systemMessage") {
			t.Fatalf("broken profile output = %+v", out)
		}
	})
	t.Run("concurrent", func(t *testing.T) {
		home := setupFixtureProfile(t, "testdata/profiles/basic", "")
		raw := fixture(t, "testdata/payloads/claude-code/pre_write.json")
		var wait sync.WaitGroup
		for index := range 20 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				Run(changeJSON(t, raw, "tool_use_id", fmt.Sprintf("concurrent-%d", index)), testNow)
			}()
		}
		wait.Wait()
		path := filepath.Join(home, "state", "sessions", "claude-code",
			"40002396-c1ff-4904-bcd2-d2592595627b.json")
		var state map[string]any
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &state) != nil {
			t.Fatalf("invalid state %q: %v", data, err)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		home := setupFixtureProfile(t, "testdata/profiles/basic", "")
		t.Setenv("JITR_DISABLE", "1")
		assertSilent(t, Run(fixture(t, "testdata/payloads/claude-code/prompt.json"), testNow))
		if _, err := os.Stat(filepath.Join(home, "state", "journal")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("journal exists: %v", err)
		}
	})
	t.Run("all-harnesses", func(t *testing.T) {
		setupFixtureProfile(t, "testdata/profiles/basic", "")
		for _, name := range []string{"claude-code", "codex", "deepseek", "trae"} {
			out := Run(fixture(t, "testdata/payloads/"+name+"/prompt.json"), testNow)
			if out.Exit != 0 || !strings.Contains(string(out.Stdout), "basic/prompt-all") {
				t.Fatalf("%s output = %+v", name, out)
			}
		}
	})
}

func TestExplainHasNoPersistentSideEffects(t *testing.T) {
	home := setupFixtureProfile(t, "testdata/profiles/basic", "")
	def, err := harness.Find("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	ev, err := harness.Decode(def, fixture(t, "testdata/payloads/claude-code/pre_write.json"), testNow)
	if err != nil {
		t.Fatal(err)
	}
	trace, err := Explain(ev, testNow)
	if err != nil {
		t.Fatal(err)
	}
	foundBlock := false
	for _, gate := range trace.Gates {
		if gate.ID == "basic/txt-block" && gate.OK {
			foundBlock = true
		}
		if gate.ID == "basic/prompt-all" {
			t.Fatal("prompt unit must not be an explain candidate")
		}
	}
	if !foundBlock || trace.Result.Action != "block" {
		t.Fatalf("trace = %+v", trace)
	}
	if _, err := os.Stat(filepath.Join(home, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Explain wrote state: %v", err)
	}
}

func TestExplainBrokenProfileStillHasNoSideEffects(t *testing.T) {
	home := setupFixtureProfile(t, "testdata/profiles/basic", "")
	broken := filepath.Join(home, "work", ".jitr")
	writeFile(t, filepath.Join(broken, "profile.yaml"), "not: [yaml")
	writeFile(t, filepath.Join(home, "config", "registry.yaml"), fmt.Sprintf(
		"version: 1\nprofiles:\n  - path: %s\n  - path: %s\n",
		broken, filepath.Join(home, ".jitr")))
	def, err := harness.Find("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	raw := replaceFixtureCWD(
		fixture(t, "testdata/payloads/claude-code/pre_write.json"),
		filepath.Join(home, "work", "project"),
	)
	ev, err := harness.Decode(def, raw, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Explain(ev, testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "state")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Explain wrote state: %v", err)
	}
}

func BenchmarkRunHundredGateOnlyUnits(b *testing.B) {
	units := make(map[string]string, 100)
	for index := range 100 {
		units[fmt.Sprintf("unit-%03d.md", index)] = promptUnit(fmt.Sprintf("Unit %d", index))
	}
	setupCustomProfile(b, units, "")
	base := fixture(b, "testdata/payloads/codex/prompt.json")
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		b.StopTimer()
		raw := changeJSON(b, base, "turn_id", fmt.Sprintf("bench-%d", index))
		b.StartTimer()
		Run(raw, testNow)
	}
}

func setupFixtureProfile(t testing.TB, relative, settings string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("JITR_HOME", home)
	t.Setenv("HOME", home)
	destination := filepath.Join(home, ".jitr")
	copyTree(t, filepath.Join(repoRoot(t), relative), destination)
	if settings != "" {
		setProfileSettings(t, destination, settings)
	}
	writeRegistry(t, home, destination)
	return home
}

func setupCustomProfile(t testing.TB, units map[string]string, settings string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("JITR_HOME", home)
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".jitr")
	profileText := "name: custom\npacks: []\ndisable: []\nsettings: {}\n"
	writeFile(t, filepath.Join(dir, "profile.yaml"), profileText)
	lock := "version: 1\nunits:\n"
	for name, content := range units {
		writeFile(t, filepath.Join(dir, "units", name), content)
		sum := sha256.Sum256([]byte(content))
		id := strings.TrimSuffix(name, ".md")
		lock += fmt.Sprintf("  custom/%s:\n    hash: sha256:%s\n", id, hex.EncodeToString(sum[:]))
		lock += "    enabled_at: 2026-11-02T00:00:00Z\n    tests: {passed: 2, failed: 0, skipped: 0}\n"
	}
	lock += "packs: {}\n"
	writeFile(t, filepath.Join(dir, "jitr.lock"), lock)
	if settings != "" {
		setProfileSettings(t, dir, settings)
	}
	writeRegistry(t, home, dir)
	return home
}

func writeActiveProfile(t testing.TB, dir, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "profile.yaml"),
		"name: "+name+"\npacks: []\ndisable: []\nsettings: {}\n")
	writeFile(t, filepath.Join(dir, "units", "rule.md"), content)
	sum := sha256.Sum256([]byte(content))
	lock := fmt.Sprintf(
		"version: 1\nunits:\n  %s/rule:\n    hash: sha256:%s\n"+
			"    enabled_at: 2026-11-02T00:00:00Z\n"+
			"    tests: {passed: 2, failed: 0, skipped: 0}\npacks: {}\n",
		name, hex.EncodeToString(sum[:]),
	)
	writeFile(t, filepath.Join(dir, "jitr.lock"), lock)
}

func setProfileSettings(t testing.TB, dir, settings string) {
	t.Helper()
	path := filepath.Join(dir, "profile.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := "settings:\n  " + strings.ReplaceAll(settings, "\n", "\n  ")
	writeFile(t, path, strings.Replace(string(data), "settings: {}", replacement, 1))
}

func toolUnit(title, action, topic, judge string) string {
	topicYAML := ""
	if topic != "" {
		topicYAML = "topic: " + topic + "\n"
	}
	return fmt.Sprintf(`---
title: %s
trigger:
  events: [tool]
  tools: [write, edit]
  paths: ["**/*.txt"]
%sdeliver:
  as: %s
%sexamples:
  - expect: hit
    event: {kind: tool, tool: write, paths: [note.txt]}
  - expect: miss
    event: {kind: tool, tool: write, paths: [note.md]}
---
%s body.
`, title, judge, action, topicYAML, title)
}

func toolResultUnit(title, action string) string {
	return fmt.Sprintf(`---
title: %s
trigger:
  events: [tool_result]
  tools: [write, edit]
  paths: ["**/*.txt"]
deliver:
  as: %s
examples:
  - expect: hit
    event: {kind: tool_result, tool: edit, paths: [note.txt]}
  - expect: miss
    event: {kind: tool_result, tool: edit, paths: [note.md]}
---
%s body.
`, title, action, title)
}

func scriptToolUnit(title, action, script string) string {
	return fmt.Sprintf(`---
title: %s
trigger:
  events: [tool]
  tools: [shell]
  shell: ['^git\s+commit\b']
deliver:
  as: %s
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m x'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
%s body.

`+"```sh jitr-judge\n%s\n```\n", title, action, title, script)
}

func promptUnit(title string) string {
	return fmt.Sprintf(`---
title: %s
trigger:
  events: [prompt]
  prompt: ["."]
examples:
  - expect: hit
    event: {kind: prompt, prompt: hello}
  - expect: miss
    event: {kind: prompt, prompt: ""}
---
%s body.
`, title, title)
}

func fixture(t testing.TB, relative string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), relative))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func repoRoot(t testing.TB) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(cwd, "..", ".."))
}

func writeRegistry(t testing.TB, home, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(home, "config", "registry.yaml"),
		fmt.Sprintf("version: 1\nprofiles:\n  - path: %s\n", dir))
}

func replaceFixtureCWD(raw []byte, cwd string) []byte {
	return []byte(strings.ReplaceAll(string(raw), "/work/proj", filepath.ToSlash(cwd)))
}

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t testing.TB, source, destination string) {
	t.Helper()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			copyTree(t, from, to)
			continue
		}
		data, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, to, string(data))
	}
}

func changeJSON(t testing.TB, data []byte, key, value string) []byte {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object[key] = value
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func changeToolContent(t testing.TB, data []byte, id, content string) []byte {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["tool_use_id"] = id
	object["tool_input"].(map[string]any)["content"] = content
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func setToolCommand(t testing.TB, data []byte, command, id string) []byte {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	object["tool_use_id"] = id
	object["tool_input"].(map[string]any)["command"] = command
	changed, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func records(t testing.TB) []journal.Record {
	t.Helper()
	got, err := journal.Since(testNow.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func sessionBytes(t testing.TB, home string) int {
	t.Helper()
	path := filepath.Join(home, "state", "sessions", "claude-code",
		"40002396-c1ff-4904-bcd2-d2592595627b.json")
	var state struct {
		Bytes int `json:"bytes"`
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	return state.Bytes
}

func countLines(t testing.TB, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(strings.TrimSpace(string(data)), "\n") + 1
}

func assertSilent(t testing.TB, out harness.Output) {
	t.Helper()
	if out.Exit != 0 || len(out.Stdout) != 0 || len(out.Stderr) != 0 {
		t.Fatalf("output = %+v", out)
	}
}

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func gateOf(all []journal.Record, id string) string {
	for _, saved := range all[len(all)-1].Units {
		if saved.ID == id {
			return saved.Gate
		}
	}
	return "<absent>"
}

func TestGateMissRecordedOnlyForDeliveredUnits(t *testing.T) {
	setupFixtureProfile(t, "testdata/profiles/basic", "")
	write := fixture(t, "testdata/payloads/claude-code/pre_write.json")
	Run(changeToolContent(t, write, "first", "HELLO WORLD"), testNow)
	if gate := gateOf(records(t), "basic/txt-block"); gate != "<absent>" {
		t.Fatalf("undelivered unit failing the gate was recorded: gate = %q", gate)
	}
}
