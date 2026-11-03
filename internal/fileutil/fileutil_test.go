package fileutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAtomicCreatesParents(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "a", "b", "file")
	if err := WriteAtomic(path, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "content")
}

func TestBackup(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 2, 3, 4, 5, 0, time.Local)
	if err := Backup(path, now); err != nil {
		t.Fatal(err)
	}
	backup := path + ".bak-jitr-20261102030405"
	if !strings.Contains(filepath.Base(backup), "20261102030405") {
		t.Fatalf("backup name %q lacks timestamp", backup)
	}
	assertFileContent(t, backup, "original")
	if err := os.WriteFile(path, []byte("updated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Backup(path, now); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, backup+".1", "updated")
}

func TestReplaceExecutable(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReplaceExecutable(src, dst); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, dst, "new")
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}
