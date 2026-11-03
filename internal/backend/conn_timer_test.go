package backend

import (
	"net/http/httptrace"
	"testing"
	"time"
)

func TestConnTimerRecordsFirstNewConnection(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	timer := NewConnTimer()
	time.Sleep(20 * time.Millisecond)
	timer.Trace().GotConn(httptrace.GotConnInfo{Reused: false})
	first := timer.Ms()
	if first < 20 {
		t.Fatalf("Ms() = %d, want at least 20", first)
	}

	time.Sleep(5 * time.Millisecond)
	timer.Trace().GotConn(httptrace.GotConnInfo{Reused: false})
	if got := timer.Ms(); got != first {
		t.Fatalf("Ms() after second connection = %d, want %d", got, first)
	}
}

func TestConnTimerIgnoresReusedConnection(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	timer := NewConnTimer()
	time.Sleep(time.Millisecond)
	timer.Trace().GotConn(httptrace.GotConnInfo{Reused: true})
	if got := timer.Ms(); got != 0 {
		t.Fatalf("Ms() = %d, want 0", got)
	}
}

func TestConnTimerIsConcurrentSafe(t *testing.T) {
	t.Setenv("JITR_HOME", t.TempDir())
	timer := NewConnTimer()
	done := make(chan struct{})
	go func() {
		timer.Trace().GotConn(httptrace.GotConnInfo{Reused: false})
		close(done)
	}()
	for timer.Ms() == 0 {
		select {
		case <-done:
			return
		default:
		}
	}
	<-done
}
