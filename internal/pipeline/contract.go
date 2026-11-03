// Package pipeline executes one complete hook invocation.
package pipeline

import (
	"time"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/journal"
)

// GateTrace records deterministic matching for one candidate unit.
type GateTrace struct {
	ID     string
	OK     bool
	Failed string
}

// Trace is the explain-mode pipeline result.
type Trace struct {
	Gates  []GateTrace
	Record journal.Record
	Result deliver.Result
}

// Run processes one raw hook payload.
func Run(stdin []byte, now time.Time) harness.Output { return run(stdin, now) }

// Explain runs resolution through delivery without persistent side effects.
func Explain(ev *event.Event, now time.Time) (*Trace, error) { return explain(ev, now) }
