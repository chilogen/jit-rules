package dshpatch

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/launcher"
	"gopkg.in/yaml.v3"
)

func TestPrepareDeepSeekPatch(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	dshHome := t.TempDir()
	t.Setenv("DSH_HOME", dshHome)
	writePatch(t, filepath.Join(dshHome, "cordis.patch.yml"), `
- insert:
    - id: memory-engram
    - id: other
`)
	writePatch(t, filepath.Join(dshHome, "profiles", "web", "cordis.patch.yml"), `
- insert:
    - id: memory-x
`)
	def, err := harness.Find("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	got, err := (implementation{}).Prepare(launcher.Request{
		Def: def, Bin: "/tmp/jitr", UserArgs: []string{"chat"}, TempDir: tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	patchPath := filepath.Join(tempDir, "isolate.patch.yml")
	wantArgs := []string{"--patch", patchPath, "chat"}
	if !reflect.DeepEqual(got.Args, wantArgs) {
		t.Fatalf("Args = %#v, want %#v", got.Args, wantArgs)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "hooks.json")); err != nil {
		t.Fatalf("hooks.json missing: %v", err)
	}
	sequence := readPatch(t, patchPath)
	wantDisabled := []string{
		"agent-instructions", "skill", "skill-filesystem", "tool-skill",
		"memory-engram", "memory-x",
	}
	if got := disabledIDs(sequence); !reflect.DeepEqual(got, wantDisabled) {
		t.Fatalf("disabled ids = %#v, want %#v", got, wantDisabled)
	}
	last := sequence.Content[len(sequence.Content)-1]
	insert := mappingValue(last, "insert")
	if insert == nil || len(insert.Content) != 1 ||
		mappingValue(insert.Content[0], "id").Value != "jit-rules-hooks" {
		t.Fatalf("last patch item is not hooks bridge: %#v", last)
	}
}

func TestPrepareDeepSeekPatchWithoutJITR(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	t.Setenv("DSH_HOME", t.TempDir())
	def, err := harness.Find("deepseek")
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	if _, err := (implementation{}).Prepare(launcher.Request{
		Def: def, Bin: "/tmp/jitr", NoJitr: true, TempDir: tempDir,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("hooks.json exists with NoJitr: %v", err)
	}
	sequence := readPatch(t, filepath.Join(tempDir, "isolate.patch.yml"))
	for _, item := range sequence.Content {
		if mappingValue(item, "insert") != nil {
			t.Fatalf("patch contains hooks bridge with NoJitr: %#v", item)
		}
	}
}

func writePatch(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPatch(t *testing.T, path string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.SequenceNode {
		t.Fatalf("patch is not a sequence:\n%s", data)
	}
	return document.Content[0]
}

func disabledIDs(sequence *yaml.Node) []string {
	var result []string
	for _, item := range sequence.Content {
		id := mappingValue(item, "id")
		disabled := mappingValue(item, "disabled")
		if id != nil && disabled != nil && disabled.Value == "true" {
			result = append(result, id.Value)
		}
	}
	return result
}
