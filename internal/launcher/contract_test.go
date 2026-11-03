package launcher

import (
	"reflect"
	"testing"

	"github.com/chilogen/jit-rules/internal/harness"
)

type fakeLauncher struct{ kind string }

func (f fakeLauncher) Kind() string { return f.kind }
func (fakeLauncher) Prepare(Request) (Launch, error) {
	return Launch{}, nil
}

func TestRegisterAndGet(t *testing.T) {
	value := fakeLauncher{kind: "contract-test"}
	Register(value)
	got, ok := Get(value.kind)
	if !ok || got.Kind() != value.kind {
		t.Fatalf("Get(%q) = %#v, %v", value.kind, got, ok)
	}
}

func TestMergeArgs(t *testing.T) {
	def := &harness.Definition{Isolate: harness.Isolate{
		Subcommands: []string{"exec"},
		Args:        []string{"base"},
		SubcommandArgs: map[string][]string{
			"exec": {"subcommand"},
		},
	}}
	cases := []struct {
		name string
		user []string
		want []string
	}{
		{
			name: "subcommand",
			user: []string{"exec", "prompt"},
			want: []string{"exec", "base", "subcommand", "generated", "prompt"},
		},
		{
			name: "interactive",
			user: []string{"prompt"},
			want: []string{"base", "generated", "prompt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MergeArgs(def, tc.user, []string{"generated"})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("MergeArgs() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
