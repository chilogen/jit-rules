package pipeline

import (
	"fmt"
	"time"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
)

func explain(ev *event.Event, now time.Time) (*Trace, error) {
	if ev == nil {
		return nil, fmt.Errorf("event is nil")
	}
	call := &invocation{
		now:       now,
		started:   time.Now(),
		ev:        ev,
		stateless: true,
	}
	if err := call.resolve(); err != nil {
		return nil, err
	}
	call.match()
	call.judge()
	call.result = deliver.Decide(call.hits, call.ev, nil, call.resolved.Settings)
	return &Trace{
		Gates:  append([]GateTrace(nil), call.gates...),
		Record: call.makeRecord(""),
		Result: call.result,
	}, nil
}
