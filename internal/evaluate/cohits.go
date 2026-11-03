package evaluate

import (
	"time"

	"github.com/chilogen/jit-rules/internal/journal"
)

// Unit hashes are ignored on purpose: a co-hit only nominates a pair, and the
// contradiction itself is always judged on the current bodies.
func loadCoHits() CoHits {
	records, err := journal.Since(time.Time{})
	if err != nil {
		return CoHits{}
	}
	history := CoHits{}
	for _, record := range records {
		ids := hitIDs(record)
		for i := range ids {
			for j := i + 1; j < len(ids); j++ {
				key := pairKey(ids[i], ids[j])
				if _, seen := history[key]; !seen {
					history[key] = replaySummary(record)
				}
			}
		}
	}
	return history
}

func hitIDs(record journal.Record) []string {
	var ids []string
	for _, saved := range record.Units {
		if saved.Gate != "" {
			continue
		}
		if saved.Judge == nil || (saved.Judge.Hit && !saved.Judge.Unconfigured) {
			ids = append(ids, saved.ID)
		}
	}
	return ids
}

func pairKey(a, b string) [2]string {
	if b < a {
		a, b = b, a
	}
	return [2]string{a, b}
}

func (history CoHits) lookup(a, b string) (string, bool) {
	summary, ok := history[pairKey(a, b)]
	return summary, ok
}
