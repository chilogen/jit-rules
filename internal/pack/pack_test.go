package pack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testUnit = `---
title: Test unit
trigger:
  events: [prompt]
  prompt: [test]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: prompt, prompt: test}
  - expect: miss
    event: {kind: prompt, prompt: other}
---
Test body.
`

func TestValidateAndHash(t *testing.T) {
	dir := makePack(t, "sample", testUnit)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Validate(dir); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	before, err := Hash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterReadme, err := Hash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if afterReadme != before {
		t.Fatalf("README changed hash: %s != %s", afterReadme, before)
	}
	unitPath := filepath.Join(dir, "units", "test.md")
	data, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-2] = '!'
	if err := os.WriteFile(unitPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	afterUnit, err := Hash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if afterUnit == before {
		t.Fatal("unit byte change did not change hash")
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("exit 0"), 0o700); err != nil {
		t.Fatal(err)
	}
	err = Validate(dir)
	if err == nil || !strings.Contains(err.Error(), "run.sh") {
		t.Fatalf("Validate() error = %v, want run.sh", err)
	}
}

func TestBuiltin(t *testing.T) {
	manifest, units, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "core" || len(units) != 2 {
		t.Fatalf("Builtin() = %#v, %d units", manifest, len(units))
	}
	for _, parsed := range units {
		if !strings.HasPrefix(parsed.File, "builtin/core/units/") {
			t.Errorf("File = %q", parsed.File)
		}
	}
}

func TestFetchLocal(t *testing.T) {
	dir := makePack(t, "sample", testUnit)
	fetched, err := Fetch(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	absolute, _ := filepath.Abs(dir)
	if fetched.Dir != absolute || fetched.Commit != "" {
		t.Fatalf("Fetch() = %#v", fetched)
	}
}

func TestFetchGitRefsAndMovingBranch(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("JITR_HOME", t.TempDir())
	repository := makePack(t, "sample", testUnit)
	runTestGit(t, git, repository, "init")
	runTestGit(t, git, repository, "checkout", "-b", "main")
	runTestGit(t, git, repository, "add", "-A")
	runTestGit(t, git, repository, "-c", "user.name=t", "-c", "user.email=t@t",
		"commit", "-m", "A")
	commitA := runTestGit(t, git, repository, "rev-parse", "HEAD")
	runTestGit(t, git, repository, "tag", "v1")
	url := "file://" + filepath.ToSlash(repository)

	for _, ref := range []string{"", "main", "v1", commitA} {
		fetched, err := Fetch(url, ref)
		if err != nil {
			t.Fatalf("Fetch(%q) error = %v", ref, err)
		}
		if fetched.Commit != commitA || fetched.Dir != GitDir("sample", commitA) {
			t.Fatalf("Fetch(%q) = %#v", ref, fetched)
		}
		if _, err := os.Stat(filepath.Join(fetched.Dir, ".git")); !os.IsNotExist(err) {
			t.Fatalf(".git remains after Fetch(%q): %v", ref, err)
		}
	}

	unitPath := filepath.Join(repository, "units", "test.md")
	if err := os.WriteFile(unitPath, []byte(strings.Replace(testUnit, "Test body.", "New body.", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, git, repository, "add", "-A")
	runTestGit(t, git, repository, "-c", "user.name=t", "-c", "user.email=t@t",
		"commit", "-m", "B")
	commitB := runTestGit(t, git, repository, "rev-parse", "HEAD")
	fetched, err := Fetch(url, "main")
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Commit != commitB || fetched.Dir != GitDir("sample", commitB) {
		t.Fatalf("moving branch Fetch() = %#v", fetched)
	}
	data, err := os.ReadFile(filepath.Join(fetched.Dir, "units", "test.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "New body.") {
		t.Fatal("moving branch returned stale content")
	}

	if _, err := Fetch(url, "missing"); err == nil {
		t.Fatal("Fetch(missing) error = nil")
	}
	entries, err := os.ReadDir(filepath.Join(os.Getenv("JITR_HOME"), "data", "packs", ".tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary directories remain: %v", entries)
	}
}

func makePack(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "units"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name: " + name + "\nversion: v1\ndescription: test\n"
	if err := os.WriteFile(filepath.Join(dir, "pack.yaml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "units", "test.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runTestGit(t *testing.T, git, dir string, args ...string) string {
	t.Helper()
	command := exec.Command(git, args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
