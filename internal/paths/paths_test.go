package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJITRHomeLocations(t *testing.T) {
	t.Setenv("JITR_HOME", "/x")
	want := map[string]string{
		"config": Config(),
		"state":  State(),
		"data":   Data(),
		"cache":  Cache(),
		"bin":    Bin(),
	}
	for name, got := range want {
		if rel, err := filepath.Rel("/x", got); err != nil || rel == ".." ||
			filepath.IsAbs(rel) && rel != "." {
			t.Errorf("%s path %q is not under /x", name, got)
		}
	}
}

func TestXDGConfigHome(t *testing.T) {
	t.Setenv("JITR_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "/c")
	if got, want := Config(), filepath.Join("/c", "jit-rules"); got != want {
		t.Fatalf("Config() = %q, want %q", got, want)
	}
}

func TestExpand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("DSH_HOME", "")
	if got, want := Expand("{env:DSH_HOME:~/.dsh}/a", ""), filepath.Join(home, ".dsh", "a"); got != want {
		t.Fatalf("fallback Expand() = %q, want %q", got, want)
	}
	t.Setenv("DSH_HOME", "/d")
	if got, want := Expand("{env:DSH_HOME:~/.dsh}/a", ""), filepath.Join("/d", "a"); got != want {
		t.Fatalf("environment Expand() = %q, want %q", got, want)
	}
	if got, want := Expand(".claude/settings.json", "/r"), filepath.Join("/r", ".claude", "settings.json"); got != want {
		t.Fatalf("rooted Expand() = %q, want %q", got, want)
	}
}

func TestProjectRoot(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	base := t.TempDir()
	root := filepath.Join(base, "a")
	deep := filepath.Join(root, "b", "c")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(deep); got != root {
		t.Fatalf("ProjectRoot() = %q, want %q", got, root)
	}
	missing := filepath.Join(base, "missing")
	if got := ProjectRoot(missing); got != missing {
		t.Fatalf("missing ProjectRoot() = %q, want itself", got)
	}
}
