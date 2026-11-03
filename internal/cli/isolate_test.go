package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func TestIsolateManualPrintsNotes(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	stdout, _, code := runCLI(t, "isolate", "trae", "--")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "无法从命令行启动一个隔离的会话") {
		t.Fatalf("notes not printed: %q", stdout)
	}
}

func TestIsolateRejectsJSONWithoutStartingChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell script")
	}
	home := t.TempDir()
	t.Setenv("JITR_HOME", home)
	marker := filepath.Join(t.TempDir(), "started")
	program := writeScript(t, "#!/bin/sh\nprintf started > \"$TEST_MARKER\"\n")
	t.Setenv("TEST_MARKER", marker)
	writeHarnessOverride(t, home, program)

	stdout, stderr, code := runCLI(
		t, "isolate", "claude-code", "--json", "--", "-p", "hi",
	)
	if code != 1 || stderr != "" {
		t.Fatalf("JSON isolate = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	assertFailedJSON(t, stdout)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child started: %v", err)
	}
}

func TestIsolateReturnsChildExitAndRemovesTempDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell script")
	}
	home := t.TempDir()
	t.Setenv("JITR_HOME", home)
	resultDir := t.TempDir()
	envFile := filepath.Join(resultDir, "env")
	tempFile := filepath.Join(resultDir, "temp")
	t.Setenv("TEST_ENV_FILE", envFile)
	t.Setenv("TEST_TEMP_FILE", tempFile)
	program := writeScript(t, `#!/bin/sh
printf '%s/%s' "$JITR_ISOLATED" "$JITR_DISABLE" > "$TEST_ENV_FILE"
dirname "$2" > "$TEST_TEMP_FILE"
exit 7
`)
	writeHarnessOverride(t, home, program)

	_, _, code := runCLI(t, "isolate", "claude-code", "--no-jitr", "--")
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if got := readText(t, envFile); got != "1/1" {
		t.Fatalf("child environment = %q, want 1/1", got)
	}
	tempDir := strings.TrimSpace(readText(t, tempFile))
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory still exists: %v", err)
	}
}

func TestRunIsolateAppliesLaunchEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	result := filepath.Join(t.TempDir(), "environment")
	strategy := &environmentLauncher{result: result}
	launcher.Register(strategy)
	def := &harness.Definition{Isolate: harness.Isolate{Kind: strategy.Kind()}}
	command := &cobra.Command{}
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})

	code, err := runIsolate(command, def, nil, false)
	if err != nil || code != 0 {
		t.Fatalf("runIsolate() = %d, %v", code, err)
	}
	if got := readText(t, result); got != "1/1" {
		t.Fatalf("child environment = %q, want 1/1", got)
	}
}

type environmentLauncher struct {
	result  string
	tempDir string
}

func (*environmentLauncher) Kind() string { return "environment-test" }

func (f *environmentLauncher) Prepare(req launcher.Request) (launcher.Launch, error) {
	f.tempDir = req.TempDir
	script := fmt.Sprintf(`printf '%%s/%%s' "$JITR_ISOLATED" "$X_TEST" > %q`, f.result)
	return launcher.Launch{
		Program: "sh",
		Args:    []string{"-c", script},
		Env:     map[string]string{"X_TEST": "1"},
	}, nil
}

func writeHarnessOverride(t *testing.T, home, program string) {
	t.Helper()
	def := harness.Definition{
		Name: "claude-code",
		Isolate: harness.Isolate{
			Kind: "claude-settings", Program: program, Settings: map[string]any{},
		},
	}
	data, err := yaml.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "config", "harnesses", "override.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "child.sh")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
