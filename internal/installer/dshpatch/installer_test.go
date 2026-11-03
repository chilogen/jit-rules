package dshpatch

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/installer"
	_ "github.com/chilogen/jit-rules/internal/installer/hooksjson"
)

func TestLifecyclePreservesOtherPatchAndComments(t *testing.T) {
	jitrHome, home := fakeHomes(t)
	dshHome := filepath.Join(home, ".dsh")
	t.Setenv("DSH_HOME", dshHome)
	patchPath := filepath.Join(dshHome, "cordis.patch.yml")
	original := []byte(`# keep this comment
- insert:
    - id: other-entry # keep entry comment
      name: other-plugin
      config:
        enabled: true
`)
	writeFile(t, patchPath, original)
	def := findDefinition(t)
	request := installer.Request{
		Def: def, Scope: "user", Bin: filepath.Join(jitrHome, "bin", "jitr"), GOOS: "linux",
	}
	assertInstalled(t, request, false)
	changes := plan(t, request)
	if len(changes) != 2 {
		t.Fatalf("install changes = %d, want 2", len(changes))
	}
	apply(t, changes, 0)
	assertInstalled(t, request, true)
	installed := readFile(t, patchPath)
	for _, text := range []string{"keep this comment", "keep entry comment", "other-entry", "jit-rules-hooks"} {
		if !bytes.Contains(installed, []byte(text)) {
			t.Errorf("installed patch missing %q:\n%s", text, installed)
		}
	}
	if count := strings.Count(string(installed), "jit-rules-hooks"); count != 1 {
		t.Fatalf("entry count = %d", count)
	}
	if second := plan(t, request); len(second) != 0 {
		t.Fatalf("second install produced %d changes", len(second))
	}
	request.Uninstall = true
	changes = plan(t, request)
	apply(t, changes, 1)
	assertInstalled(t, request, false)
	uninstalled := readFile(t, patchPath)
	for _, text := range []string{"keep this comment", "keep entry comment", "other-entry"} {
		if !bytes.Contains(uninstalled, []byte(text)) {
			t.Errorf("uninstalled patch missing %q:\n%s", text, uninstalled)
		}
	}
	if bytes.Contains(uninstalled, []byte("jit-rules-hooks")) {
		t.Fatalf("jitr entry remains:\n%s", uninstalled)
	}
	hookPath := filepath.Join(jitrHome, "config", "harness", "deepseek", "hooks.json")
	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Fatalf("hooks target remains: %v", err)
	}
}

func TestProjectScopeUnsupported(t *testing.T) {
	fakeHomes(t)
	def := findDefinition(t)
	request := installer.Request{Def: def, Scope: "project", Root: t.TempDir(), Bin: "/bin/jitr"}
	if _, err := (implementation{}).Plan(request); err == nil {
		t.Fatal("project Plan succeeded")
	}
}

func TestUninstallAbsentDoesNotCreateFiles(t *testing.T) {
	jitrHome, home := fakeHomes(t)
	dshHome := filepath.Join(home, ".dsh")
	t.Setenv("DSH_HOME", dshHome)
	def := findDefinition(t)
	request := installer.Request{
		Def: def, Scope: "user", Bin: filepath.Join(jitrHome, "bin", "jitr"), Uninstall: true,
	}
	changes := plan(t, request)
	if len(changes) != 0 {
		t.Fatalf("absent uninstall changes = %#v", changes)
	}
	if _, err := os.Stat(dshHome); !os.IsNotExist(err) {
		t.Fatalf("absent uninstall wrote DSH_HOME: %v", err)
	}
}

func plan(t *testing.T, request installer.Request) []installer.Change {
	t.Helper()
	changes, err := (implementation{}).Plan(request)
	if err != nil {
		t.Fatal(err)
	}
	return changes
}

func apply(t *testing.T, changes []installer.Change, second int) {
	t.Helper()
	now := time.Date(2026, 11, 2, 11, 0, second, 0, time.UTC)
	if err := installer.Apply(changes, now); err != nil {
		t.Fatal(err)
	}
}

func assertInstalled(t *testing.T, request installer.Request, want bool) {
	t.Helper()
	got, err := (implementation{}).Installed(request)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Installed() = %v, want %v", got, want)
	}
}

func findDefinition(t *testing.T) *harness.Definition {
	t.Helper()
	def, err := harness.Find("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	return def
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

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
