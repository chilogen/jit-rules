package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chilogen/jit-rules/internal/fileutil"
	"github.com/chilogen/jit-rules/internal/paths"
	"github.com/gofrs/flock"
)

const (
	lockRetries = 20
	lockDelay   = 10 * time.Millisecond
)

func open(harness, sessionID string) *Store {
	return &Store{path: filepath.Join(paths.State(), "sessions", harness, sessionID+".json")}
}

func (s *Store) update(now time.Time, fn func(*State)) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return false, err
	}
	fileLock := flock.New(s.path + ".lock")
	locked, err := acquire(fileLock)
	if err != nil || !locked {
		return locked, err
	}
	updateErr := s.updateLocked(now, fn)
	unlockErr := fileLock.Unlock()
	if updateErr != nil {
		return true, updateErr
	}
	return true, unlockErr
}

func acquire(fileLock *flock.Flock) (bool, error) {
	for attempt := 0; attempt <= lockRetries; attempt++ {
		locked, err := fileLock.TryLock()
		if err != nil || locked {
			return locked, err
		}
		if attempt < lockRetries {
			time.Sleep(lockDelay)
		}
	}
	return false, nil
}

func (s *Store) updateLocked(now time.Time, fn func(*State)) error {
	state, err := readState(s.path)
	if err != nil {
		return err
	}
	fn(state)
	state.V = 1
	state.Updated = now
	cutoff := now.Add(-10 * time.Minute)
	for key, seenAt := range state.Seen {
		if seenAt.Before(cutoff) {
			delete(state.Seen, key)
		}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return fileutil.WriteAtomic(s.path, data, 0o600)
}

func (s *Store) claim(key string, now time.Time) (dup, locked bool, err error) {
	locked, err = s.update(now, func(state *State) {
		ttl := 10 * time.Minute
		if strings.HasPrefix(key, "prompt:sha1:") || strings.HasPrefix(key, "stop:sha1:") {
			ttl = 5 * time.Second
		}
		if seenAt, ok := state.Seen[key]; ok && now.Sub(seenAt) < ttl {
			dup = true
			return
		}
		state.Seen[key] = now
	})
	return dup, locked, err
}

func readState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, err
	}
	state := newState()
	if len(data) > 0 {
		if err := json.Unmarshal(data, state); err != nil {
			return nil, err
		}
	}
	if state.Delivered == nil {
		state.Delivered = make(map[string]*Mark)
	}
	if state.Seen == nil {
		state.Seen = make(map[string]time.Time)
	}
	return state, nil
}

func newState() *State {
	return &State{
		V:         1,
		Delivered: make(map[string]*Mark),
		Seen:      make(map[string]time.Time),
	}
}

func gc(now time.Time) {
	root := filepath.Join(paths.State(), "sessions")
	cutoff := now.Add(-7 * 24 * time.Hour)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		info, statErr := entry.Info()
		if statErr == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
			_ = os.Remove(path + ".lock")
		}
		return nil
	})
}
