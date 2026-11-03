package codexconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
)

func TestPrepareCodexArguments(t *testing.T) {
	def := codexDefinition(t)
	got, err := (implementation{}).Prepare(launcher.Request{
		Def: def, Bin: "/tmp/jitr", UserArgs: []string{"exec", "hi"}, TempDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"exec"}, def.Isolate.Args...)
	want = append(want, "--ignore-user-config")
	events := []string{"PostToolUse", "PreToolUse", "SessionStart", "Stop", "UserPromptSubmit"}
	for _, event := range events {
		want = append(want, "-c")
		prefix := "hooks." + event + "="
		found := ""
		for _, arg := range got.Args {
			if len(arg) >= len(prefix) && arg[:len(prefix)] == prefix {
				found = arg
				break
			}
		}
		if found == "" {
			t.Fatalf("Args missing %s: %#v", prefix, got.Args)
		}
		want = append(want, found)
	}
	want = append(want, "hi")
	if !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("Args = %#v, want %#v", got.Args, want)
	}
}

func TestPrepareCodexInteractive(t *testing.T) {
	def := codexDefinition(t)
	got, err := (implementation{}).Prepare(launcher.Request{
		Def: def, Bin: "/tmp/jitr", UserArgs: []string{"hi"}, TempDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Args[:len(def.Isolate.Args)], def.Isolate.Args) {
		t.Fatalf("Args = %#v, want isolation args first", got.Args)
	}
	for _, arg := range got.Args {
		if arg == "--ignore-user-config" {
			t.Fatalf("interactive Args contain --ignore-user-config: %#v", got.Args)
		}
	}
}

func TestPrepareCodexCopiesOnlyKeptFiles(t *testing.T) {
	def := codexDefinition(t)
	source := t.TempDir()
	t.Setenv("CODEX_HOME", source)
	auth := []byte(`{"tokens":"test"}`)
	if err := os.WriteFile(filepath.Join(source, "auth.json"), auth, 0o640); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.toml", "hooks.json"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, noJitr := range []bool{false, true} {
		tempDir := t.TempDir()
		got, err := (implementation{}).Prepare(launcher.Request{
			Def: def, Bin: "/tmp/jitr", NoJitr: noJitr, TempDir: tempDir,
		})
		if err != nil {
			t.Fatal(err)
		}
		home := filepath.Join(tempDir, "home")
		if got.Env["CODEX_HOME"] != home {
			t.Fatalf("CODEX_HOME = %q, want %q", got.Env["CODEX_HOME"], home)
		}
		entries, err := os.ReadDir(home)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "auth.json" {
			t.Fatalf("isolated home entries = %v", entryNames(entries))
		}
		copied, err := os.ReadFile(filepath.Join(home, "auth.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(copied, auth) {
			t.Fatalf("copied auth = %q, want %q", copied, auth)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(filepath.Join(home, "auth.json"))
			if err != nil || info.Mode().Perm() != 0o640 {
				t.Fatalf("copied mode = %v, %v", info.Mode().Perm(), err)
			}
		}
	}
}

func TestPrepareCodexWithoutAuthentication(t *testing.T) {
	def := codexDefinition(t)
	t.Setenv("CODEX_HOME", t.TempDir())
	tempDir := t.TempDir()
	got, err := (implementation{}).Prepare(launcher.Request{
		Def: def, Bin: "/tmp/jitr", TempDir: tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(got.Env["CODEX_HOME"])
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty home entries = %v, %v", entryNames(entries), err)
	}
}

func TestCodexWindowsTOMLEscaping(t *testing.T) {
	def := codexDefinition(t)
	got, err := (implementation{}).Prepare(launcher.Request{
		Def: def, Bin: `C:\Users\a b\jitr.exe`, UserArgs: []string{"hi"},
		TempDir: t.TempDir(), GOOS: "windows",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `hooks.PostToolUse=[{matcher="*",hooks=[{type="command",command="\"C:\\Users\\a b\\jitr.exe\" hook --harness codex",timeout=10}]}]`
	if !contains(got.Args, want) {
		t.Fatalf("Args do not contain exact TOML value %q:\n%#v", want, got.Args)
	}
}

func codexDefinition(t *testing.T) *harness.Definition {
	t.Helper()
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	def, err := harness.Find("codex")
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
