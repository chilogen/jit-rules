package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/evaluate"
	"github.com/chilogen/jit-rules/internal/paths"
)

func TestUnitReplayJournalFailuresAndEmptyHistory(t *testing.T) {
	for _, state := range []string{"missing", "empty", "malformed", "read-error"} {
		for _, mode := range []string{"human", "json"} {
			t.Run(state+"/"+mode, func(t *testing.T) {
				home, _ := cliEnvironment(t)
				initCLIProfile(t, home, "me")
				writeUnitThroughCLI(t, "me", "replay", replayUnit())
				prepareReplayJournal(t, state)
				args := []string{"unit", "replay", "replay"}
				if mode == "json" {
					args = append(args, "--json")
				}
				stdout, stderr, code := runCLI(t, args...)
				failed := state == "malformed" || state == "read-error"
				wantCode := 0
				if failed {
					wantCode = 3
				}
				if code != wantCode {
					t.Fatalf("replay = %d, %q / %q", code, stdout, stderr)
				}
				if mode == "json" {
					checkReplayJSON(t, stdout, stderr, failed)
				} else if failed {
					if stdout != "" || !strings.Contains(stderr, "回放历史事件失败") {
						t.Fatalf("failure output = %q / %q", stdout, stderr)
					}
				} else if stderr != "" || !strings.Contains(stdout, "命中率：0/0") {
					t.Fatalf("empty history = %q / %q", stdout, stderr)
				}
			})
		}
	}
}

func prepareReplayJournal(t *testing.T, state string) {
	t.Helper()
	if state == "missing" {
		return
	}
	dir := filepath.Join(paths.State(), "journal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, time.Now().UTC().Format("2006-01-02")+".jsonl")
	if state == "read-error" {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return
	}
	var data []byte
	if state == "malformed" {
		data = []byte("not json\n")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func checkReplayJSON(t *testing.T, stdout, stderr string, failed bool) {
	t.Helper()
	var result struct {
		OK     bool                   `json:"ok"`
		Data   *evaluate.ReplayReport `json:"data"`
		Errors []string               `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatal(err)
	}
	if stderr != "" || result.OK == failed {
		t.Fatalf("JSON replay = %q / %q", stdout, stderr)
	}
	if failed {
		if result.Data != nil || len(result.Errors) != 1 ||
			!strings.Contains(result.Errors[0], "回放历史事件失败") {
			t.Fatalf("JSON failure = %+v", result)
		}
		return
	}
	if len(result.Errors) != 0 || result.Data == nil || len(result.Data.Hits) != 0 ||
		result.Data.Judged != 0 || result.Data.Unknown != 0 {
		t.Fatalf("JSON empty history = %+v", result)
	}
}
