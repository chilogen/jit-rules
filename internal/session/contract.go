// Package session stores per-harness delivery and deduplication state.
package session

import "time"

// Mark records the byte counter at delivery.
type Mark struct {
	At int `json:"at"`
}

// State is persisted session accounting.
type State struct {
	V         int                  `json:"v"`
	Bytes     int                  `json:"bytes"`
	Delivered map[string]*Mark     `json:"delivered"`
	Seen      map[string]time.Time `json:"seen"`
	Notified  bool                 `json:"notified"`
	Updated   time.Time            `json:"updated"`
}

// Store provides locked access to one session state file.
type Store struct {
	path string
}

// Open returns the store for a harness session.
func Open(harness, sessionID string) *Store { return open(harness, sessionID) }

// Update locks, reads, mutates, and rewrites session state.
func (s *Store) Update(now time.Time, fn func(*State)) (locked bool, err error) {
	return s.update(now, fn)
}

// Claim atomically checks and records a deduplication key.
func (s *Store) Claim(key string, now time.Time) (dup, locked bool, err error) {
	return s.claim(key, now)
}

// Read returns the current session state.
func (s *Store) Read() (*State, error) { return readState(s.path) }

// GC removes expired session state.
func GC(now time.Time) { gc(now) }
