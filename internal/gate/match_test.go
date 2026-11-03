package gate

import (
	"testing"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/unit"
)

func TestMatchTriggerFields(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	tests := []struct {
		name    string
		trigger unit.Trigger
		event   event.Event
	}{
		{"tools", unit.Trigger{Tools: []string{"read"}}, event.Event{Tool: "read"}},
		{"paths", unit.Trigger{Paths: []string{"**/*.go"}}, event.Event{Paths: []string{"internal/a.go"}}},
		{"shell", unit.Trigger{Shell: []string{`git\s+push`}}, event.Event{Shell: "git push origin"}},
		{"prompt", unit.Trigger{Prompt: []string{`deploy`}}, event.Event{Prompt: "please deploy"}},
		{"content", unit.Trigger{Content: []string{`secret`}}, event.Event{Content: "no secret here"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("JITR_HOME", t.TempDir())
			if got := Match(&unit.Unit{Trigger: tt.trigger}, &tt.event); !got.OK {
				t.Fatalf("matching %s failed: %+v", tt.name, got)
			}
			tt.event.Tool = ""
			tt.event.Paths = nil
			tt.event.Shell = ""
			tt.event.Prompt = ""
			tt.event.Content = ""
			got := Match(&unit.Unit{Trigger: tt.trigger}, &tt.event)
			if got.OK || got.Failed != tt.name {
				t.Fatalf("mismatch = %+v, want Failed %q", got, tt.name)
			}
		})
	}
}

func TestMatchPathsRequireEventPath(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := &unit.Unit{Trigger: unit.Trigger{Paths: []string{"**"}}}
	got := Match(u, &event.Event{})
	if got.OK || got.Failed != "paths" {
		t.Fatalf("Match() = %+v", got)
	}
}

func TestMatchToolsIncludesEffect(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := &unit.Unit{Trigger: unit.Trigger{Tools: []string{"write"}}}
	if got := Match(u, &event.Event{Tool: "shell", Effect: "write"}); !got.OK {
		t.Fatalf("write effect should match: %+v", got)
	}
	got := Match(u, &event.Event{Tool: "shell"})
	if got.OK || got.Failed != "tools" {
		t.Fatalf("empty effect should not match: %+v", got)
	}
	got = Match(&unit.Unit{Trigger: unit.Trigger{Tools: []string{""}}}, &event.Event{Tool: "shell"})
	if got.OK || got.Failed != "tools" {
		t.Fatalf("empty effect matched wildcard: %+v", got)
	}
}

func TestMatchUnknownContent(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	ev := &event.Event{ContentUnknown: true}
	got := Match(&unit.Unit{Trigger: unit.Trigger{Content: []string{"x"}}}, ev)
	if !got.OK || !got.ContentUncertain {
		t.Fatalf("content condition = %+v", got)
	}
	got = Match(&unit.Unit{}, ev)
	if !got.OK || got.ContentUncertain {
		t.Fatalf("no content condition = %+v", got)
	}
}

func TestMatchChecksInSpecifiedOrder(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	u := &unit.Unit{Trigger: unit.Trigger{
		Tools: []string{"read"}, Paths: []string{"*.go"}, Shell: []string{"ok"},
	}}
	got := Match(u, &event.Event{})
	if got.Failed != "tools" {
		t.Fatalf("Failed = %q, want tools", got.Failed)
	}
}
