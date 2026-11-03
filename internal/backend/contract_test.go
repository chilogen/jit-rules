package backend

import (
	"context"
	"testing"
)

type testBackend struct{ name string }

func (b testBackend) Name() string { return b.name }

func (b testBackend) Judge(context.Context, map[string]any, []Question, map[string]any) (Result, error) {
	return Result{}, nil
}

func TestRegistry(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	Register(testBackend{name: "t01-z"})
	Register(testBackend{name: "t01-a"})
	if got, ok := Get("t01-z"); !ok || got.Name() != "t01-z" {
		t.Fatalf("Get() = %#v, %v", got, ok)
	}
	names := Names()
	var a, z int = -1, -1
	for index, name := range names {
		switch name {
		case "t01-a":
			a = index
		case "t01-z":
			z = index
		}
	}
	if a < 0 || z < 0 || a >= z {
		t.Fatalf("Names() = %v, want t01-a before t01-z", names)
	}
}
