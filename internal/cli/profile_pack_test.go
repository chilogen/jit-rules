package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chilogen/jit-rules/internal/pack"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/registry"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestProfileInitAndShowThroughExecute(t *testing.T) {
	home, cwd := cliEnvironment(t)
	dir := filepath.Join(home, ".jitr")
	stdout, _, code := runCLI(t, "profile", "init", dir, "--name", "me")
	if code != 0 || !strings.Contains(stdout, "已创建并注册") {
		t.Fatalf("profile init = code %d, stdout %q", code, stdout)
	}
	loaded, err := profile.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Packs) != 1 || loaded.Packs[0].Name != "core" {
		t.Fatalf("packs = %#v", loaded.Packs)
	}
	entries, err := registry.Load()
	if err != nil || len(entries) != 1 || entries[0].Path != dir ||
		!registry.IsGlobal(entries[0]) {
		t.Fatalf("registry = %#v, %v", entries, err)
	}
	t.Chdir(cwd)
	stdout, _, code = runCLI(t, "profile", "show")
	if code != 0 {
		t.Fatalf("profile show code = %d", code)
	}
	for _, text := range []string{"core/", "规则原文："} {
		if !strings.Contains(stdout, text) {
			t.Errorf("profile show output lacks %q: %s", text, stdout)
		}
	}
}

func TestProfileInitUsesJITRParentAsProjectScope(t *testing.T) {
	home, cwd := cliEnvironment(t)
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cwd, ".jitr")
	if _, stderr, code := runCLI(t, "profile", "init", dir, "--name", "project"); code != 0 {
		t.Fatalf("profile init = code %d, stderr %q", code, stderr)
	}
	entries, err := registry.Load()
	if err != nil || len(entries) != 1 {
		t.Fatalf("registry = %#v, %v", entries, err)
	}
	if entries[0].Path != dir || registry.IsGlobal(entries[0]) ||
		registry.Scope(entries[0]) != cwd {
		t.Fatalf("entry = %#v, want project scope %q", entries[0], cwd)
	}
	t.Chdir(home)
	stdout, stderr, code := runCLI(t, "profile", "list", "--json")
	if code != 0 {
		t.Fatalf("profile list = code %d, stderr %q", code, stderr)
	}
	var result struct {
		Data []profileListItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 || result.Data[0].Applies {
		t.Fatalf("profile list = %#v, want project profile not applicable outside repo", result.Data)
	}
}

func TestProfileInitRejectsDirectoryOutsideJITR(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := filepath.Join(home, "profiles", "me")
	_, stderr, code := runCLI(t, "profile", "init", dir, "--name", "me")
	if code != 1 || !strings.Contains(stderr, ".jitr") {
		t.Fatalf("profile init = code %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("profile directory was created: %v", err)
	}
}

func TestProfileRegisterRejectsProjectProfileOutsideJITR(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := filepath.Join(home, "profile")
	if err := profile.Init(dir, "me"); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, "profile", "register", dir)
	if code != 1 || !strings.Contains(stderr, ".jitr") {
		t.Fatalf("profile register = code %d, stderr %q", code, stderr)
	}
}

func TestProfileRegisterRejectsInvalidLockedUnit(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := filepath.Join(home, ".jitr")
	if err := profile.Init(dir, "bad"); err != nil {
		t.Fatal(err)
	}
	data := []byte(`---
title: 无效单元
trigger:
  events: [prompt]
---
正文
`)
	file := filepath.Join(dir, "units", "bad.md")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := unit.Parse(data, file, "bad")
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
	_, _, code := runCLI(t, "profile", "register", dir)
	if code != 1 {
		t.Fatalf("register invalid profile code = %d", code)
	}
	entries, err := registry.Load()
	if err != nil || len(entries) != 0 {
		t.Fatalf("registry changed: %#v, %v", entries, err)
	}
}

func TestPackAddDuplicateScriptAndRemoveThroughExecute(t *testing.T) {
	home, cwd := cliEnvironment(t)
	dir := initCLIProfile(t, home, "me")
	good := fixturePath(t, "good")
	script := fixturePath(t, "with-script")

	_, stderr, code := runCLI(t, "pack", "add", good)
	if code != 0 {
		t.Fatalf("pack add = code %d, stderr %q", code, stderr)
	}
	loaded, err := profile.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Packs[len(loaded.Packs)-1].Name; got != "good" {
		t.Fatalf("last pack = %q", got)
	}
	lock, err := profile.LoadLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := pack.Hash(good)
	if err != nil {
		t.Fatal(err)
	}
	if lock.Packs["good"].Hash != wantHash {
		t.Fatalf("lock hash = %q, want %q", lock.Packs["good"].Hash, wantHash)
	}
	if _, _, code := runCLI(t, "pack", "add", good); code != 1 {
		t.Fatalf("duplicate add code = %d", code)
	}

	beforeProfile := readFile(t, filepath.Join(dir, "profile.yaml"))
	beforeLock := readFile(t, filepath.Join(dir, "jitr.lock"))
	if _, _, code := runCLI(t, "pack", "add", script); code != 1 {
		t.Fatalf("script add code = %d", code)
	}
	assertFileEquals(t, filepath.Join(dir, "profile.yaml"), beforeProfile)
	assertFileEquals(t, filepath.Join(dir, "jitr.lock"), beforeLock)

	if _, stderr, code := runCLI(t, "pack", "remove", "good"); code != 0 {
		t.Fatalf("pack remove = code %d, stderr %q", code, stderr)
	}
	loaded, _ = profile.Load(dir)
	for _, reference := range loaded.Packs {
		if reference.Name == "good" {
			t.Fatal("good remains in profile.yaml")
		}
	}
	lock, _ = profile.LoadLock(dir)
	if _, exists := lock.Packs["good"]; exists {
		t.Fatal("good remains in jitr.lock")
	}
	resolved, err := profile.Resolve(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, parsed := range resolved.Units {
		if strings.HasPrefix(parsed.ID, "good/") {
			t.Fatalf("removed unit remains active: %s", parsed.ID)
		}
	}
}

func TestPackUpdateLocalThroughExecute(t *testing.T) {
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	local := filepath.Join(home, "local-pack")
	copyTree(t, fixturePath(t, "good"), local)
	if _, _, code := runCLI(t, "pack", "add", local); code != 0 {
		t.Fatalf("pack add code = %d", code)
	}
	unitPath := filepath.Join(local, "units", "new.md")
	writeValidUnit(t, unitPath, "新增规则", "新增规则正文。")
	if _, stderr, code := runCLI(t, "pack", "update", "good"); code != 0 {
		t.Fatalf("pack update = code %d, stderr %q", code, stderr)
	}
	selected, err := selectProfile("")
	if err != nil {
		t.Fatal(err)
	}
	wantHash, _ := pack.Hash(local)
	if selected.lock.Packs["good"].Hash != wantHash {
		t.Fatalf("updated hash = %q, want %q", selected.lock.Packs["good"].Hash, wantHash)
	}
	resolved, err := profile.Resolve(home)
	if err != nil {
		t.Fatal(err)
	}
	if !hasUnit(resolved.Units, "good/new") {
		t.Fatalf("updated units = %v, errors = %v", unitIDs(resolved.Units), resolved.Errors)
	}
}

func TestPackUpdateGitBranchThroughExecute(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	repo := filepath.Join(home, "source.git")
	copyTree(t, fixturePath(t, "good"), repo)
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "branch", "-M", "main")
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-m", "first")
	source := "file://" + filepath.ToSlash(repo)
	if _, stderr, code := runCLI(t, "pack", "add", source, "--ref", "main"); code != 0 {
		t.Fatalf("git pack add = code %d, stderr %q", code, stderr)
	}
	first := currentPackCommit(t, "good")
	writeValidUnit(t, filepath.Join(repo, "units", "new.md"), "新增规则", "新增规则正文。")
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-m", "second")
	if _, stderr, code := runCLI(t, "pack", "update", "good"); code != 0 {
		t.Fatalf("git pack update = code %d, stderr %q", code, stderr)
	}
	second := currentPackCommit(t, "good")
	if first == second {
		t.Fatalf("commit did not change: %s", first)
	}
	resolved, err := profile.Resolve(home)
	if err != nil {
		t.Fatal(err)
	}
	if !hasUnit(resolved.Units, "good/new") {
		selected, _ := selectProfile("")
		t.Fatalf(
			"new git unit is not active: units=%v errors=%v packs=%#v lock=%#v",
			unitIDs(resolved.Units), resolved.Errors, selected.config.Packs, selected.lock.Packs,
		)
	}
}

func TestPackInputErrorsAndJSONThroughExecute(t *testing.T) {
	home, _ := cliEnvironment(t)
	initCLIProfile(t, home, "me")
	_, stderr, code := runCLI(t, "pack", "add", "./not-exist")
	if code != 1 || !strings.Contains(stderr, "找不到目录") {
		t.Fatalf("missing dir = code %d, stderr %q", code, stderr)
	}
	stdout, stderr, code := runCLI(t, "pack", "add", "./not-exist", "--json")
	if code != 1 || stderr != "" {
		t.Fatalf("missing dir JSON = code %d, stderr %q", code, stderr)
	}
	assertJSONEnvelope(t, stdout, false)
	if _, _, code := runCLI(t, "pack", "add", "file:///not/exist"); code != 3 {
		t.Fatalf("missing git source code = %d", code)
	}
}

func TestProfileAndPackCommandsEmitJSONObjects(t *testing.T) {
	home, _ := cliEnvironment(t)
	dir := filepath.Join(home, ".jitr")
	assertCLIJSON(t, true, "profile", "init", dir, "--name", "me", "--json")
	assertCLIJSON(t, true, "profile", "list", "--json")
	assertCLIJSON(t, true, "profile", "show", "--json")
	assertCLIJSON(t, true, "pack", "list", "--json")
	assertCLIJSON(t, true, "pack", "update", "--json")
	assertCLIJSON(t, true, "pack", "add", fixturePath(t, "good"), "--json")
	assertCLIJSON(t, true, "pack", "remove", "good", "--json")
	assertCLIJSON(t, true, "profile", "unregister", "me", "--json")

	other := filepath.Join(home, "other", ".jitr")
	if err := profile.Init(other, "other"); err != nil {
		t.Fatal(err)
	}
	assertCLIJSON(t, true, "profile", "register", other, "--json")
}

func cliEnvironment(t *testing.T) (home, cwd string) {
	t.Helper()
	home = t.TempDir()
	cwd = filepath.Join(home, "work")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JITR_HOME", home)
	t.Setenv("HOME", home)
	t.Chdir(cwd)
	return home, cwd
}

func initCLIProfile(t *testing.T, home, name string) string {
	t.Helper()
	dir := filepath.Join(home, ".jitr")
	if _, stderr, code := runCLI(
		t, "profile", "init", dir, "--name", name,
	); code != 0 {
		t.Fatalf("profile init = code %d, stderr %q", code, stderr)
	}
	return dir
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "testdata", "packs", name))
}

func writeValidUnit(t *testing.T, path, title, body string) {
	t.Helper()
	content := fmt.Sprintf(`---
title: %s
trigger:
  events: [prompt]
  prompt: ["new"]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: prompt, prompt: "new"}
  - expect: miss
    event: {kind: prompt, prompt: "old"}
---
%s
`, title, body)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, source, target string) {
	t.Helper()
	err := filepath.Walk(source, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, info.Mode())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	command := exec.Command("git", commandArgs...)
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func currentPackCommit(t *testing.T, name string) string {
	t.Helper()
	selected, err := selectProfile("")
	if err != nil {
		t.Fatal(err)
	}
	return selected.lock.Packs[name].Commit
}

func hasUnit(units []*unit.Unit, id string) bool {
	for _, parsed := range units {
		if parsed.ID == id {
			return true
		}
	}
	return false
}

func unitIDs(units []*unit.Unit) []string {
	ids := make([]string, 0, len(units))
	for _, parsed := range units {
		ids = append(ids, parsed.ID)
	}
	return ids
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertFileEquals(t *testing.T, path string, want []byte) {
	t.Helper()
	got := readFile(t, path)
	if string(got) != string(want) {
		t.Fatalf("%s changed unexpectedly", path)
	}
}

func assertCLIJSON(t *testing.T, wantOK bool, args ...string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, args...)
	if code != 0 || stderr != "" {
		t.Fatalf("%v = code %d, stderr %q, stdout %q", args, code, stderr, stdout)
	}
	assertJSONEnvelope(t, stdout, wantOK)
}

func assertJSONEnvelope(t *testing.T, output string, wantOK bool) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(output), &value); err != nil {
		t.Fatalf("invalid JSON %q: %v", output, err)
	}
	ok, exists := value["ok"].(bool)
	if !exists || ok != wantOK {
		t.Fatalf("JSON ok = %#v, want %t", value["ok"], wantOK)
	}
}
