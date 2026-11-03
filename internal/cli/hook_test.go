package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/journal"
)

func TestHookIgnoresUnknownAndMissingHarnessFlags(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "testdata", "payloads", "claude-code", "prompt.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"hook", "--unknown-flag"},
		{"hook"},
		{"hook", "--harness=claude-code"},
		{"hook", "--harness", "claude-code"},
		{"hook", "--harness", "unrecognized"},
		{"hook", "--harness=unrecognized"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) {
			t.Setenv("JITR_HOME", t.TempDir())
			rootCmd.SetIn(bytes.NewReader(payload))
			_, stderr, code := runCLI(t, args...)
			if code != 0 || stderr != "" {
				t.Fatalf("%v = code %d, stderr %q", args, code, stderr)
			}
			records, err := journal.Since(time.Time{})
			if err != nil || len(records) != 1 {
				t.Fatalf("journal = %+v, %v", records, err)
			}
			if records[0].Event.Harness != "claude-code" {
				t.Fatalf("payload detected as %q", records[0].Event.Harness)
			}
		})
	}
}
