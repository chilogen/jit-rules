package fake

import (
	"context"
	"sync"
	"time"

	"github.com/chilogen/jit-rules/internal/backend"
)

// Backend is a configurable judgment backend intended for tests.
type Backend struct {
	BackendName string
	P           map[string]float64
	DefaultP    float64
	Missing     map[string]bool
	Func        func(state map[string]any, question backend.Question) float64
	Delay       time.Duration
	Err         error
	ConnMs      int

	mu        sync.Mutex
	calls     int
	lastState map[string]any
	lastQs    []backend.Question
}

// Name implements backend.Backend.
func (b *Backend) Name() string {
	return b.BackendName
}

// Judge records the request and returns configured results.
func (b *Backend) Judge(
	ctx context.Context,
	state map[string]any,
	questions []backend.Question,
	cfg map[string]any,
) (backend.Result, error) {
	b.record(state, questions)
	result := backend.Result{
		P:      make(map[string]float64, len(questions)),
		Model:  configString(cfg, "model"),
		ConnMs: b.ConnMs,
	}
	if b.Delay > 0 {
		timer := time.NewTimer(b.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return result, ctx.Err()
		}
	}

	for _, question := range questions {
		if b.Missing[question.ID] {
			continue
		}
		switch {
		case b.Func != nil:
			result.P[question.ID] = b.Func(state, question)
		case b.P != nil:
			value, ok := b.P[question.ID]
			if ok {
				result.P[question.ID] = value
			} else {
				result.P[question.ID] = b.DefaultP
			}
		default:
			result.P[question.ID] = b.DefaultP
		}
	}
	if b.Err != nil {
		return result, b.Err
	}
	return result, nil
}

// Calls returns the number of Judge calls.
func (b *Backend) Calls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// LastRequest returns copies of the most recently observed state and questions.
func (b *Backend) LastRequest() (map[string]any, []backend.Question) {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := make(map[string]any, len(b.lastState))
	for key, value := range b.lastState {
		state[key] = value
	}
	questions := append([]backend.Question(nil), b.lastQs...)
	return state, questions
}

func (b *Backend) record(state map[string]any, questions []backend.Question) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	b.lastState = make(map[string]any, len(state))
	for key, value := range state {
		b.lastState[key] = value
	}
	b.lastQs = append(b.lastQs[:0], questions...)
}

func configString(cfg map[string]any, key string) string {
	value, _ := cfg[key].(string)
	return value
}
