// Package backend defines and registers model judgment backends.
package backend

import (
	"context"
	"errors"
	"net/http/httptrace"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ErrNotConfigured marks missing required backend configuration.
var ErrNotConfigured = errors.New("judge backend not configured")

// Question is one unit question sent to a backend.
type Question struct {
	ID   string
	Text string
	Not  string
}

// Result is one batch backend response.
type Result struct {
	P      map[string]float64
	Model  string
	ConnMs int
}

// Backend judges a batch of unit questions.
type Backend interface {
	Name() string
	Judge(ctx context.Context, state map[string]any, qs []Question, cfg map[string]any) (Result, error)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Backend)
)

// Register makes a backend available by name.
func Register(b Backend) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[b.Name()] = b
}

// Get looks up a registered backend.
func Get(name string) (Backend, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	registered, ok := registry[name]
	return registered, ok
}

// Names returns registered backend names in lexical order.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ConnTimer records new connection setup time.
type ConnTimer struct {
	start time.Time
	once  sync.Once
	ms    atomic.Int64
}

// NewConnTimer starts a connection timer.
func NewConnTimer() *ConnTimer {
	return &ConnTimer{start: time.Now()}
}

// Trace returns the HTTP trace used to detect connection reuse.
func (t *ConnTimer) Trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Reused {
				return
			}
			t.once.Do(func() {
				t.ms.Store(time.Since(t.start).Milliseconds())
			})
		},
	}
}

// Ms returns recorded new connection latency.
func (t *ConnTimer) Ms() int {
	return int(t.ms.Load())
}
