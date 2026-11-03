package manual

import "github.com/chilogen/jit-rules/internal/launcher"

type implementation struct{}

func init() {
	launcher.Register(implementation{})
}

func (implementation) Kind() string { return "manual" }

func (implementation) Prepare(launcher.Request) (launcher.Launch, error) {
	return launcher.Launch{}, nil
}
