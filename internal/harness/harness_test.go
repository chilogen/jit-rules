package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
)

func TestGoldenPayloads(t *testing.T) {
	defs, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	expected := readExpected(t)
	assertHarnessSets(t, expected)
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for harnessName, cases := range expected {
		dir := filepath.Join("..", "..", "testdata", "payloads", harnessName)
		fixtures, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(fixtures) != len(cases) {
			t.Fatalf("%s: %d fixtures, %d expected", harnessName, len(fixtures), len(cases))
		}
		for _, fixture := range fixtures {
			name := strings.TrimSuffix(filepath.Base(fixture), ".json")
			want, ok := cases[name]
			if !ok {
				t.Errorf("%s/%s missing expected entry", harnessName, name)
				continue
			}
			t.Run(harnessName+"/"+name, func(t *testing.T) {
				testGoldenPayload(t, defs, fixture, want, now)
			})
		}
	}
}

func assertHarnessSets(t *testing.T, expected map[string]map[string]map[string]any) {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join("..", "..", "testdata", "payloads", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != len(expected) {
		t.Fatalf("%d payload directories, %d expected harnesses", len(dirs), len(expected))
	}
	for _, dir := range dirs {
		if _, ok := expected[filepath.Base(dir)]; !ok {
			t.Errorf("%s missing from expected", filepath.Base(dir))
		}
	}
}

func testGoldenPayload(t *testing.T, defs []*Definition, fixture string, want map[string]any, now time.Time) {
	raw := readFile(t, fixture)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	def := DetectDef(defs, payload)
	if def == nil {
		t.Fatal("payload not detected")
	}
	ev, err := Decode(def, raw, now)
	if err != nil {
		t.Fatal(err)
	}
	got := eventFields(ev)
	for field, expectedValue := range want {
		if field == "paths" && emptySlice(expectedValue) && got[field] == nil {
			continue
		}
		if !reflect.DeepEqual(got[field], expectedValue) {
			t.Errorf("%s: got %#v, want %#v", field, got[field], expectedValue)
		}
	}
	if ev.HookEvent != payload["hook_event_name"] {
		t.Errorf("HookEvent = %q, want %q", ev.HookEvent, payload["hook_event_name"])
	}
}

func eventFields(ev *event.Event) map[string]any {
	paths := make([]any, len(ev.Paths))
	for i, path := range ev.Paths {
		paths[i] = path
	}
	return map[string]any{
		"harness": ev.Harness, "kind": ev.Kind, "tool": ev.Tool,
		"raw_tool": ev.RawTool, "effect": ev.Effect, "paths": paths,
		"shell": ev.Shell, "content": ev.Content,
		"content_unknown": ev.ContentUnknown, "prompt": ev.Prompt,
		"dedupe_key": ev.DedupeKey, "stop_active": ev.StopActive,
	}
}

func readExpected(t *testing.T) map[string]map[string]map[string]any {
	t.Helper()
	var expected map[string]map[string]map[string]any
	data := readFile(t, filepath.Join("..", "..", "testdata", "expected.json"))
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	return expected
}

func emptySlice(value any) bool {
	slice, ok := value.([]any)
	return ok && len(slice) == 0
}

type shellWriteTable struct {
	Cwd   string           `json:"cwd"`
	Cases []shellWriteCase `json:"cases"`
}

type shellWriteCase struct {
	Name     string         `json:"name"`
	Command  string         `json:"command"`
	Existing []string       `json:"existing"`
	Expect   map[string]any `json:"expect"`
}

func TestShellWrites(t *testing.T) {
	var table shellWriteTable
	data := readFile(t, filepath.Join("..", "..", "testdata", "shell_writes.json"))
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatal(err)
	}
	def := findLoaded(t, "codex")
	for _, test := range table.Cases {
		t.Run(test.Name, func(t *testing.T) {
			existing := make(map[string]bool)
			for _, name := range test.Existing {
				existing[filepath.Join(table.Cwd, name)] = true
			}
			got := Extract(def, "Bash", map[string]any{"command": test.Command},
				table.Cwd, table.Cwd, func(path string) bool { return existing[path] })
			assertExtracted(t, got, test.Expect)
		})
	}
}

func assertExtracted(t *testing.T, got Extracted, want map[string]any) {
	t.Helper()
	actual := map[string]any{
		"tool": got.Tool, "effect": got.Effect, "content": got.Content,
		"content_unknown": got.ContentUnknown,
	}
	paths := make([]any, len(got.Paths))
	for i, path := range got.Paths {
		paths[i] = path
	}
	actual["paths"] = paths
	for field, value := range want {
		if !reflect.DeepEqual(actual[field], value) {
			t.Errorf("%s: got %#v, want %#v", field, actual[field], value)
		}
	}
}

func TestLoadOverrideAndUnknownField(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	dir := filepath.Join(os.Getenv("JITR_HOME"), "config", "harnesses")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	override := "name: codex\ntitle: Custom\ndetect: {order: 1}\nevents: {PreToolUse: tool}\ntools: {Bash: x}\n"
	if err := os.WriteFile(filepath.Join(dir, "codex.yaml"), []byte(override), 0o644); err != nil {
		t.Fatal(err)
	}
	def := findLoaded(t, "codex")
	raw := []byte(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{},"cwd":"/work/proj"}`)
	ev, err := Decode(def, raw, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Tool != "x" || def.Detect.Order != 1 {
		t.Fatalf("override not applied: tool=%q order=%d", ev.Tool, def.Detect.Order)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("name: bad\nunknown: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAll(); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestHookEntriesAndCommandString(t *testing.T) {
	def := findLoaded(t, "codex")
	entries := HookEntries(def, "run")
	pre := firstHandler(t, entries["PreToolUse"])
	if pre["additionalContextLimit"] != 5000 {
		t.Fatalf("additionalContextLimit = %#v", pre["additionalContextLimit"])
	}
	session := entries["SessionStart"][0].(map[string]any)
	if _, ok := session["matcher"]; ok {
		t.Fatal("SessionStart has matcher")
	}
	if got := CommandString(def, "/opt/jitr", "linux"); got != "'/opt/jitr' hook --harness codex" {
		t.Fatalf("linux command = %q", got)
	}
	if got := CommandString(def, `C:\bin\jitr.exe`, "windows"); got != `"C:\bin\jitr.exe" hook --harness codex` {
		t.Fatalf("windows command = %q", got)
	}
}

func firstHandler(t *testing.T, groups []any) map[string]any {
	t.Helper()
	group := groups[0].(map[string]any)
	return group["hooks"].([]any)[0].(map[string]any)
}

func TestEncode(t *testing.T) {
	actions := []string{"none", "inject", "block"}
	for _, systemMessage := range []bool{false, true} {
		for _, action := range actions {
			for _, notice := range []string{"", "notice"} {
				name := strings.Join([]string{action, boolName(systemMessage), boolName(notice != "")}, "/")
				t.Run(name, func(t *testing.T) {
					testEncodeCase(t, action, notice, systemMessage)
				})
			}
		}
	}
}

func testEncodeCase(t *testing.T, action, notice string, systemMessage bool) {
	def := &Definition{Supports: Supports{SystemMessage: systemMessage}}
	text := "line \"one\"\n中文"
	got := Encode(def, eventWithHook("PreToolUse"), deliver.Result{Action: action, Text: text, Notice: notice})
	want := expectedOutput(action, text, notice, systemMessage)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if len(got.Stdout) > 0 && !json.Valid(got.Stdout) {
		t.Fatalf("invalid JSON: %s", got.Stdout)
	}
}

func expectedOutput(action, text, notice string, systemMessage bool) Output {
	if action == "block" {
		return Output{Stderr: []byte(text), Exit: 2}
	}
	fields := make([]string, 0, 2)
	if action == "inject" {
		fields = append(fields, `"hookSpecificOutput":{"hookEventName":"PreToolUse","additionalContext":"line \"one\"\n中文"}`)
	}
	if notice != "" && systemMessage {
		fields = append(fields, `"systemMessage":"notice"`)
	}
	if len(fields) == 0 {
		return Output{}
	}
	return Output{Stdout: []byte("{" + strings.Join(fields, ",") + "}")}
}

func eventWithHook(hook string) *event.Event {
	return &event.Event{HookEvent: hook}
}

func boolName(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func findLoaded(t *testing.T, name string) *Definition {
	t.Helper()
	def, err := Find(name)
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDetectionOrder(t *testing.T) {
	defs, err := LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Detect.Order > defs[j].Detect.Order })
	if got := DetectDef(defs, map[string]any{"model": "x"}); got == nil || got.Name != "codex" {
		t.Fatalf("detected %#v", got)
	}
}
