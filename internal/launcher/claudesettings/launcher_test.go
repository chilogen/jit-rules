package claudesettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
)

func TestPrepareClaudeSettings(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	def, err := harness.Find("claude-code")
	if err != nil {
		t.Fatal(err)
	}
	for _, noJitr := range []bool{false, true} {
		t.Run(map[bool]string{false: "with-jitr", true: "without-jitr"}[noJitr], func(t *testing.T) {
			tempDir := t.TempDir()
			got, err := (implementation{}).Prepare(launcher.Request{
				Def: def, Bin: "/tmp/jitr", UserArgs: []string{"-p", "hi"},
				NoJitr: noJitr, TempDir: tempDir,
			})
			if err != nil {
				t.Fatal(err)
			}
			settingsPath := filepath.Join(tempDir, "settings.json")
			wantArgs := []string{
				"--setting-sources", "", "--disable-slash-commands",
				"--settings", settingsPath, "-p", "hi",
			}
			if !reflect.DeepEqual(got.Args, wantArgs) {
				t.Fatalf("Args = %#v, want %#v", got.Args, wantArgs)
			}
			data, err := os.ReadFile(settingsPath)
			if err != nil {
				t.Fatal(err)
			}
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			if settings["autoMemoryEnabled"] != false {
				t.Fatalf("autoMemoryEnabled = %#v", settings["autoMemoryEnabled"])
			}
			hooks, present := settings["hooks"].(map[string]any)
			if noJitr && present {
				t.Fatal("hooks present with NoJitr")
			}
			if !noJitr && (!present || len(hooks) != 5) {
				t.Fatalf("hooks = %#v, want five events", settings["hooks"])
			}
		})
	}
}
