// Package journal stores hook outcomes and internal errors.
package journal

import (
	"time"

	"github.com/chilogen/jit-rules/internal/event"
)

// JudgeRecord is the persisted judgment diagnostic.
type JudgeRecord struct {
	P            float64 `json:"p"`
	Hit          bool    `json:"hit"`
	Fallback     bool    `json:"fallback"`
	Unconfigured bool    `json:"unconfigured"`
	Backend      string  `json:"backend"`
	Model        string  `json:"model"`
	ConnMs       int     `json:"conn_ms"`
	Ms           int     `json:"ms"`
	Err          string  `json:"err"`
	Exit         *int    `json:"exit,omitempty"`
	Stderr       string  `json:"stderr,omitempty"`
}

// UnitRecord is the persisted outcome for one gated unit.
type UnitRecord struct {
	ID        string       `json:"id"`
	Hash      string       `json:"hash"`
	Judge     *JudgeRecord `json:"judge"`
	NoBlock   bool         `json:"no_block"`
	Skip      string       `json:"skip"`
	Delivered string       `json:"delivered"`
	Gate      string       `json:"gate,omitempty"`
}

// ResultRecord summarizes the encoded result.
type ResultRecord struct {
	Action string `json:"action"`
	Chars  int    `json:"chars"`
}

// Record is one hook journal entry.
type Record struct {
	V          int          `json:"v"`
	TS         time.Time    `json:"ts"`
	Isolated   bool         `json:"isolated"`
	Event      event.Event  `json:"event"`
	Profiles   []string     `json:"profiles"`
	GateFailed int          `json:"gate_failed"`
	Units      []UnitRecord `json:"units"`
	Result     ResultRecord `json:"result"`
	Ms         int          `json:"ms"`
	Err        string       `json:"err"`
}

// Append writes one journal record.
func Append(r Record, storeContent bool) error { return appendRecord(r, storeContent) }

// Since returns records at or after a timestamp.
func Since(t time.Time) ([]Record, error) { return since(t) }

// GC removes expired journal files.
func GC(now time.Time, keepDays int) { gc(now, keepDays) }

// LogError appends an internal error record.
func LogError(stage string, err error) { logError(stage, err) }
