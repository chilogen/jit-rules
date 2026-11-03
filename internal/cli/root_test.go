package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"syscall"
	"testing"

	"github.com/spf13/cobra"
)

func TestOutputJSONEnvelope(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	var output bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&output)
	jsonMode = true
	defer resetJSONFlag()
	Output(cmd, true, map[string]string{"value": "x"}, "", nil, nil)
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "data", "errors", "warnings"} {
		if _, exists := got[key]; !exists {
			t.Errorf("JSON output lacks %q", key)
		}
	}
	if errorsValue, ok := got["errors"].([]any); !ok || len(errorsValue) != 0 {
		t.Errorf("errors = %#v, want []", got["errors"])
	}
	if warnings, ok := got["warnings"].([]any); !ok || len(warnings) != 0 {
		t.Errorf("warnings = %#v, want []", got["warnings"])
	}
}

func TestExecuteExitCodes(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	exitErr := commandExitError(t)
	cases := []struct {
		name string
		err  error
		code int
		text string
	}{
		{name: "ok", code: 0},
		{name: "ordinary", err: errors.New("x"), code: 1, text: "x"},
		{name: "internal", err: Internal(errors.New("x")), code: 3, text: "x"},
		{name: "silent", err: Silent(5), code: 5},
		{name: "path-permission", err: fmt.Errorf("a: %w", &fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}), code: 3, text: "/x"},
		{name: "path-missing", err: fmt.Errorf("a: %w", &fs.PathError{Op: "open", Path: "/x", Err: fs.ErrNotExist}), code: 1, text: "/x"},
		{name: "exit-error", err: fmt.Errorf("a: %w", exitErr), code: 3, text: "a:"},
		{name: "broken-chain", err: fmt.Errorf("a: %v", exitErr), code: 1, text: "a:"},
	}
	for index, tc := range cases {
		commandName := fmt.Sprintf("exit-case-%d", index)
		Register(errorCommand(commandName, tc.err))
		stdout, stderr, code := runCLI(t, commandName)
		if code != tc.code {
			t.Errorf("%s code = %d, want %d", tc.name, code, tc.code)
		}
		if tc.name == "silent" && (stdout != "" || stderr != "") {
			t.Errorf("silent output = %q / %q", stdout, stderr)
		}
		if tc.text != "" && !bytes.Contains([]byte(stderr), []byte(tc.text)) {
			t.Errorf("%s stderr = %q, want %q", tc.name, stderr, tc.text)
		}
	}
}

func TestExecuteInputErrors(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	required := &cobra.Command{
		Use:  "needs-argument",
		Args: cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error { return nil },
	}
	Register(required)
	cases := [][]string{
		{"unknown-command-t01"},
		{"needs-argument", "--unknown"},
		{"needs-argument"},
		{"completion"},
	}
	for _, args := range cases {
		_, _, code := runCLI(t, args...)
		if code != 1 {
			t.Errorf("%v code = %d, want 1", args, code)
		}
	}
}

func TestExecuteJSONErrors(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	exitErr := commandExitError(t)
	errorsToTest := []error{
		errors.New("x"),
		Internal(errors.New("x")),
		fmt.Errorf("a: %w", &fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}),
		fmt.Errorf("a: %w", &fs.PathError{Op: "open", Path: "/x", Err: fs.ErrNotExist}),
		fmt.Errorf("a: %w", exitErr),
		fmt.Errorf("a: %v", exitErr),
	}
	for index, commandErr := range errorsToTest {
		name := fmt.Sprintf("json-error-%d", index)
		Register(errorCommand(name, commandErr))
		stdout, stderr, _ := runCLI(t, name, "--json")
		assertFailedJSON(t, stdout)
		if stderr != "" {
			t.Errorf("%s wrote stderr %q", name, stderr)
		}
	}
	stdout, _, _ := runCLI(t, "json-error-0", "--unknown", "--json")
	assertFailedJSON(t, stdout)
	stdout, _, _ = runCLI(t, "unknown-json-command", "--json")
	assertFailedJSON(t, stdout)
}

func TestExecuteSilentJSONHasNoOutput(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	Register(errorCommand("silent-json", Silent(5)))
	stdout, stderr, code := runCLI(t, "silent-json", "--json")
	if code != 5 || stdout != "" || stderr != "" {
		t.Fatalf("silent JSON = code %d, %q / %q", code, stdout, stderr)
	}
}

func TestParentAndHelpBehavior(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	group := &cobra.Command{Use: "grp-t01"}
	group.AddCommand(&cobra.Command{
		Use:  "child",
		RunE: func(*cobra.Command, []string) error { return nil },
	})
	Register(group)
	if _, _, code := runCLI(t, "grp-t01", "foo"); code != 1 {
		t.Fatalf("group extra argument code = %d", code)
	}
	stdout, _, code := runCLI(t, "grp-t01")
	if code != 0 || stdout == "" {
		t.Fatalf("group help = code %d, output %q", code, stdout)
	}
	stdout, _, code = runCLI(t, "grp-t01", "--json")
	assertHelpJSON(t, stdout, code)
	stdout, _, code = runCLI(t, "help", "--json")
	assertHelpJSON(t, stdout, code)
	stdout, _, code = runCLI(t, "grp-t01", "-h", "--json")
	assertHelpJSON(t, stdout, code)
}

func errorCommand(name string, result error) *cobra.Command {
	return &cobra.Command{
		Use: name,
		RunE: func(*cobra.Command, []string) error {
			return result
		},
	}
}

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)
	code := Execute()
	return stdout.String(), stderr.String(), code
}

func assertFailedJSON(t *testing.T, output string) {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("invalid JSON %q: %v", output, err)
	}
	if ok, _ := got["ok"].(bool); ok {
		t.Fatalf("ok = true in %q", output)
	}
}

func assertHelpJSON(t *testing.T, output string, code int) {
	t.Helper()
	if code != 0 {
		t.Fatalf("help code = %d", code)
	}
	var got struct {
		OK   bool `json:"ok"`
		Data struct {
			Help string `json:"help"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("invalid help JSON %q: %v", output, err)
	}
	if !got.OK || got.Data.Help == "" {
		t.Fatalf("help JSON = %#v", got)
	}
}

func commandExitError(t *testing.T) *exec.ExitError {
	t.Helper()
	err := exec.Command("sh", "-c", "exit 7").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("command error = %T, want *exec.ExitError", err)
	}
	return exitErr
}
