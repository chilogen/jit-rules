package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chilogen/jit-rules/internal/paths"
)

func TestSaveLoadAndApplicable(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	parent := filepath.Join(home, "a")
	child := filepath.Join(parent, "b")
	entries := []Entry{
		{Path: filepath.Join(home, ".jitr")},
		{Path: filepath.Join(parent, ".jitr")},
		{Path: filepath.Join(child, ".jitr")},
		{Path: filepath.Join(home, "other", ".jitr")},
	}
	if err := Save(entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, entries) {
		t.Fatalf("Load() = %#v, want %#v", loaded, entries)
	}
	got := Applicable(loaded, filepath.Join(child, "src"))
	want := []Entry{entries[2], entries[1], entries[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Applicable(inside) = %#v, want %#v", got, want)
	}
	got = Applicable(loaded, filepath.Join(home, "outside"))
	want = []Entry{entries[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Applicable(outside) = %#v, want %#v", got, want)
	}
}

func TestSaveRejectsProjectProfileOutsideJITR(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	entry := Entry{Path: filepath.Join(t.TempDir(), "profile")}
	if err := Save([]Entry{entry}); err == nil {
		t.Fatalf("Save(%#v) succeeded", entry)
	}
}

func TestSaveRejectsRelativeProfile(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	entry := Entry{Path: ".jitr"}
	if err := Save([]Entry{entry}); err == nil {
		t.Fatalf("Save(%#v) succeeded", entry)
	}
}

func TestLoadRejectsProjectProfileOutsideJITR(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	if err := os.MkdirAll(paths.Config(), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("version: 1\nprofiles:\n  - path: /tmp/profile\n")
	if err := os.WriteFile(filepath.Join(paths.Config(), "registry.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a project profile outside .jitr")
	}
}

func TestLoadRejectsRelativeProfile(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	if err := os.MkdirAll(paths.Config(), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("version: 1\nprofiles:\n  - path: .jitr\n")
	if err := os.WriteFile(filepath.Join(paths.Config(), "registry.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a relative profile path")
	}
}

func TestLoadMissing(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	entries, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Load() = %#v", entries)
	}
}
