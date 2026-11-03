// Package gate evaluates deterministic unit triggers.
package gate

import (
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/unit"
)

// Outcome is the result of deterministic trigger evaluation.
type Outcome struct {
	OK               bool
	Failed           string
	ContentUncertain bool
}

// Match evaluates a unit trigger against an event.
func Match(u *unit.Unit, ev *event.Event) Outcome { return match(u, ev) }
