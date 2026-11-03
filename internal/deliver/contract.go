// Package deliver selects and formats matched units.
package deliver

import (
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/judge"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/session"
	"github.com/chilogen/jit-rules/internal/unit"
)

// Hit is one unit that passed all matching stages.
type Hit struct {
	Unit    *unit.Unit
	Verdict *judge.Verdict
	NoBlock bool
}

// Result is the final delivery action and diagnostics.
type Result struct {
	Action    string
	Text      string
	Notice    string
	Delivered []string
	Blocked   []string
	Skips     map[string]string
}

// Decide applies ranking, redelivery, blocking, and truncation rules.
func Decide(hits []Hit, ev *event.Event, st *session.State, s profile.Settings) Result {
	return decide(hits, ev, st, s)
}
