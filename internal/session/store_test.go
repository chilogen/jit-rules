package session

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestClaimDuplicate(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	store := Open("test", "session")
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	dup, locked, err := store.Claim("tool:one", now)
	if err != nil || !locked || dup {
		t.Fatalf("first Claim() = %v, %v, %v", dup, locked, err)
	}
	dup, locked, err = store.Claim("tool:one", now)
	if err != nil || !locked || !dup {
		t.Fatalf("second Claim() = %v, %v, %v", dup, locked, err)
	}
}

func TestClaimExpiry(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	prompt := Open("test", "prompt")
	_, _, _ = prompt.Claim("prompt:sha1:abc", now)
	dup, _, _ := prompt.Claim("prompt:sha1:abc", now.Add(4*time.Second))
	if !dup {
		t.Fatal("prompt claim after 4s is not duplicate")
	}
	dup, _, _ = prompt.Claim("prompt:sha1:abc", now.Add(6*time.Second))
	if dup {
		t.Fatal("prompt claim after 6s is duplicate")
	}
	other := Open("test", "other")
	_, _, _ = other.Claim("tool:abc", now)
	dup, _, _ = other.Claim("tool:abc", now.Add(9*time.Minute))
	if !dup {
		t.Fatal("tool claim after 9m is not duplicate")
	}
}

func TestUpdateRemovesOldSeen(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	store := Open("test", "cleanup")
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	locked, err := store.Update(now, func(state *State) {
		state.Seen["old"] = now.Add(-10*time.Minute - time.Second)
		state.Seen["new"] = now.Add(-10 * time.Minute)
	})
	if err != nil || !locked {
		t.Fatalf("Update() = %v, %v", locked, err)
	}
	state, err := store.Read()
	if err != nil || state.Seen["old"] != (time.Time{}) || state.Seen["new"] == (time.Time{}) {
		t.Fatalf("Read() = %+v, %v", state, err)
	}
}

func TestUpdateTimesOutWithoutCallingFunction(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	store := Open("test", "locked")
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		t.Fatal(err)
	}
	ready := make(chan error, 1)
	go func() {
		fileLock := flock.New(store.path + ".lock")
		if err := fileLock.Lock(); err != nil {
			ready <- err
			return
		}
		ready <- nil
		time.Sleep(300 * time.Millisecond)
		_ = fileLock.Unlock()
	}()
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	var called atomic.Bool
	start := time.Now()
	locked, err := store.Update(time.Now(), func(*State) { called.Store(true) })
	if err != nil || locked || called.Load() {
		t.Fatalf("Update() = %v, %v, called %v", locked, err, called.Load())
	}
	if elapsed := time.Since(start); elapsed < 190*time.Millisecond || elapsed > 280*time.Millisecond {
		t.Fatalf("Update waited %v", elapsed)
	}
}

func TestConcurrentUpdate(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	store := Open("test", "concurrent")
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			locked, err := store.Update(time.Now(), func(state *State) { state.Bytes++ })
			if err != nil {
				errs <- err
			} else if !locked {
				errs <- errNotLocked{}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	state, err := store.Read()
	if err != nil || state.Bytes != 20 {
		t.Fatalf("Read() bytes = %d, err = %v", state.Bytes, err)
	}
}

func TestGCOldSessions(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	store := Open("test", "old")
	now := time.Now()
	_, err := store.Update(now, func(*State) {})
	if err != nil {
		t.Fatal(err)
	}
	old := now.Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(store.path, old, old); err != nil {
		t.Fatal(err)
	}
	GC(now)
	if _, err := os.Stat(store.path); !os.IsNotExist(err) {
		t.Fatalf("old session still exists: %v", err)
	}
}

type errNotLocked struct{}

func (errNotLocked) Error() string { return "update did not acquire lock" }
