// Package judge coordinates model-backed unit decisions.
package judge

import (
	"context"

	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

// Verdict is the decision and diagnostics for one unit.
type Verdict struct {
	Hit          bool
	P            float64
	Fallback     bool
	Unconfigured bool
	Backend      string
	Model        string
	ConnMs       int
	Ms           int
	Err          string
	Exit         *int
	Stderr       string
}

// Decide judges all supplied units.
func Decide(ctx context.Context, ev *event.Event, units []*unit.Unit, s profile.Settings) map[string]Verdict {
	return decide(ctx, ev, units, s)
}
