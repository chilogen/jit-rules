// Package evaluate tests, replays, and compares judgment units.
package evaluate

import (
	"time"

	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/unit"
)

// CaseResult is one unit example outcome.
type CaseResult struct {
	Index  int
	Expect string
	Got    string
	Result string
	Note   string
}

// TestRecord is the saved test result for one unit hash.
type TestRecord struct {
	Hash  string       `json:"hash"`
	TS    time.Time    `json:"ts"`
	Cases []CaseResult `json:"cases"`
}

// ReplayHit summarizes one matching historical event.
type ReplayHit struct {
	TS      time.Time
	Summary string
}

// ReplayReport summarizes replay evaluation.
type ReplayReport struct {
	Hits    []ReplayHit
	Judged  int
	Unknown int
}

// Conflict describes an overlap between two units.
type Conflict struct {
	A, B      string
	Example   string
	Confirmed bool
}

// Test evaluates all examples in a unit.
func Test(u *unit.Unit, s profile.Settings, offline bool) []CaseResult {
	return testUnit(u, s, offline)
}

// SaveTest persists unit test results.
func SaveTest(u *unit.Unit, cases []CaseResult, now time.Time) error {
	return saveTest(u, cases, now)
}

// LoadTest reads the latest saved test result.
func LoadTest(id string) (*TestRecord, error) { return loadTest(id) }

// Replay evaluates a unit against journal history.
func Replay(u *unit.Unit, s profile.Settings, since time.Duration, live bool, now time.Time) (ReplayReport, error) {
	return replay(u, s, since, live, now)
}

// CoHits maps a sorted pair of unit ids to a summary of one real event on which both hit.
type CoHits map[[2]string]string

// LoadCoHits collects co-hit pairs from all retained journal records.
func LoadCoHits() CoHits { return loadCoHits() }

// Conflicts checks a target against all active units.
func Conflicts(target *unit.Unit, all []*unit.Unit, s profile.Settings, history CoHits) []Conflict {
	return conflicts(target, all, s, history)
}

// FormatConflict renders a conflict report.
func FormatConflict(c Conflict, higher string) string { return formatConflict(c, higher) }
