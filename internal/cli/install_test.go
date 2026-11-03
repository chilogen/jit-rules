package cli

import (
	"os"
	"path/filepath"
	"testing"

	_ "github.com/chilogen/jit-rules/internal/installer/dshpatch"
	_ "github.com/chilogen/jit-rules/internal/installer/hooksjson"
	"github.com/chilogen/jit-rules/internal/paths"
)

func TestInstallDryRunWritesNothing(t *testing.T) {
	jitrHome, home := installFakeHomes(t)
	stdout, stderr, code := runCLI(t, "install", "codex", "--scope", "user", "--dry-run")
	if code != 0 {
		t.Fatalf("install dry-run code = %d, stderr = %q", code, stderr)
	}
	if stdout == "" {
		t.Fatal("install dry-run printed no plan")
	}
	for _, path := range []string{
		filepath.Join(jitrHome, "bin", "jitr"),
		filepath.Join(home, ".codex", "hooks.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("dry-run wrote %s: %v", path, err)
		}
	}
}

func TestInstallAndUninstallCLI(t *testing.T) {
	jitrHome, home := installFakeHomes(t)
	target := filepath.Join(home, ".codex", "hooks.json")
	_, stderr, code := runCLI(t, "install", "codex", "--scope", "user", "--dry-run=false")
	if code != 0 {
		t.Fatalf("install code = %d, stderr = %q", code, stderr)
	}
	for _, path := range []string{filepath.Join(jitrHome, "bin", "jitr"), target} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("installed path %s: %v", path, err)
		}
	}
	_, stderr, code = runCLI(t, "uninstall", "codex")
	if code != 0 {
		t.Fatalf("uninstall code = %d, stderr = %q", code, stderr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("uninstall left target: %v", err)
	}
}

func TestDeepseekProjectScopeReturnsUserError(t *testing.T) {
	installFakeHomes(t)
	_, _, code := runCLI(t, "install", "deepseek", "--scope", "project", "--dry-run=false")
	if code != 1 {
		t.Fatalf("project install code = %d, want 1", code)
	}
}

func TestUpgradeOnlyReplacesBinary(t *testing.T) {
	jitrHome, home := installFakeHomes(t)
	source := filepath.Join(t.TempDir(), "new-jitr")
	if err := os.WriteFile(source, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, "upgrade", source)
	if code != 0 {
		t.Fatalf("upgrade code = %d, stderr = %q", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(jitrHome, "bin", "jitr"))
	if err != nil || string(data) != "new binary" {
		t.Fatalf("upgraded binary = %q, %v", data, err)
	}
	if entries, err := os.ReadDir(home); err != nil || len(entries) != 0 {
		t.Fatalf("upgrade changed fake HOME: %v, %v", entries, err)
	}
}

func TestPurgeJSONRequiresYesThenDeletesData(t *testing.T) {
	installFakeHomes(t)
	for _, path := range []string{paths.Config(), paths.State(), paths.Data(), paths.Cache()} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "value"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_, _, code := runCLI(t, "purge", "--json")
	if code != 1 {
		t.Fatalf("purge --json code = %d, want 1", code)
	}
	_, stderr, code := runCLI(t, "purge", "--json=false", "--yes")
	if code != 0 {
		t.Fatalf("purge --yes code = %d, stderr = %q", code, stderr)
	}
	for _, path := range []string{paths.Config(), paths.State(), paths.Data(), paths.Cache()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("purge left %s: %v", path, err)
		}
	}
}

func installFakeHomes(t *testing.T) (string, string) {
	t.Helper()
	jitrHome := t.TempDir()
	home := t.TempDir()
	t.Setenv("JITR_HOME", jitrHome)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return jitrHome, home
}
