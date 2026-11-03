package installer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/harness"
)

type fakeInstaller struct {
	kind string
}

func (f fakeInstaller) Kind() string { return f.kind }
func (fakeInstaller) Plan(Request) ([]Change, error) {
	return nil, nil
}
func (fakeInstaller) Installed(Request) (bool, error) {
	return false, nil
}

func TestRegisterAndGet(t *testing.T) {
	value := fakeInstaller{kind: "test-kind"}
	Register(value)
	got, ok := Get(value.kind)
	if !ok || got.Kind() != value.kind {
		t.Fatalf("Get(%q) = %#v, %v", value.kind, got, ok)
	}
}

func TestTargetPaths(t *testing.T) {
	_, home := fakeHomes(t)
	existing := filepath.Join(home, ".second")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	def := &harness.Definition{
		Title: "Example",
		Install: harness.Install{Targets: map[string]harness.Target{
			"user": {
				Paths: []string{"~/.missing/hooks.json", "~/.second/hooks.json"},
				Mode:  "existing",
			},
			"project": {Paths: []string{".tool/hooks.json"}, Mode: "first"},
		}},
	}
	got, err := TargetPaths(def, "user", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != filepath.Join(existing, "hooks.json") {
		t.Fatalf("TargetPaths user = %#v", got)
	}
	root := t.TempDir()
	got, err = TargetPaths(def, "project", root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, ".tool", "hooks.json"); len(got) != 1 || got[0] != want {
		t.Fatalf("TargetPaths project = %#v, want %q", got, want)
	}
}

func TestTargetPathsErrors(t *testing.T) {
	fakeHomes(t)
	def := &harness.Definition{
		Title: "Example",
		Install: harness.Install{Targets: map[string]harness.Target{
			"user": {Paths: []string{"~/.missing/hooks.json"}, Mode: "existing"},
		}},
	}
	if _, err := TargetPaths(def, "user", ""); err == nil {
		t.Fatal("TargetPaths existing without directories succeeded")
	}
	if _, err := TargetPaths(def, "project", t.TempDir()); err == nil {
		t.Fatal("TargetPaths unsupported project scope succeeded")
	}
}

func TestApplyBacksUpAndDeletes(t *testing.T) {
	fakeHomes(t)
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	before := []byte("before\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 10, 11, 12, 0, time.UTC)
	if err := Apply([]Change{{Path: path, Before: before, After: []byte("after\n")}}, now); err != nil {
		t.Fatal(err)
	}
	backup := path + ".bak-jitr-20261102101112"
	if got, err := os.ReadFile(backup); err != nil || string(got) != string(before) {
		t.Fatalf("backup = %q, %v", got, err)
	}
	updated, err := os.ReadFile(path)
	if err != nil || string(updated) != "after\n" {
		t.Fatalf("updated file = %q, %v", updated, err)
	}
	if err := Apply([]Change{{Path: path, Before: updated, After: nil}}, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("deleted file stat error = %v", err)
	}
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
