package event

import (
	"encoding/json"
	"regexp"
	"testing"
)

func TestCanonicalTool(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	cases := map[string]string{
		"mcp__fs__write_file": "mcp/fs/write_file",
		"mcp__a__b__c":        "mcp/a/b__c",
		"Foo":                 "foo",
	}
	for raw, want := range cases {
		if got := CanonicalTool(raw, nil); got != want {
			t.Errorf("CanonicalTool(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := CanonicalTool("native", map[string]string{"native": "write"}); got != "write" {
		t.Errorf("table CanonicalTool() = %q", got)
	}
}

func TestDedupeKey(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	payload := map[string]any{"second": "b", "third": "c"}
	if got, want := DedupeKey("tool", []string{"first", "second", "third"}, payload, ""), "tool:b"; got != want {
		t.Fatalf("tool DedupeKey() = %q, want %q", got, want)
	}
	got := DedupeKey("prompt", []string{"missing"}, nil, "hello")
	if matched := regexp.MustCompile(`^prompt:sha1:[0-9a-f]{40}$`).MatchString(got); !matched {
		t.Fatalf("prompt DedupeKey() = %q", got)
	}
}

func TestSizeOf(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	if got, want := SizeOf("prompt", map[string]any{"prompt": "你好"}), len("你好"); got != want {
		t.Fatalf("prompt SizeOf() = %d, want %d", got, want)
	}
	input := map[string]any{"path": "a", "content": "b"}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if got := SizeOf("tool", map[string]any{"tool_input": input}); got != len(encoded) {
		t.Fatalf("tool SizeOf() = %d, want %d", got, len(encoded))
	}
	if got := SizeOf("other", nil); got != 0 {
		t.Fatalf("other SizeOf() = %d, want 0", got)
	}
}
