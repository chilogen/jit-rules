package pipeline

import (
	"os"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/judge"
)

func (call *invocation) appendJournal(recordError string) {
	record := call.makeRecord(recordError)
	storeContent := profileStoreContent(call)
	if err := journal.Append(record, storeContent); err != nil {
		journal.LogError("journal", err)
	}
}

func (call *invocation) makeRecord(recordError string) journal.Record {
	record := journal.Record{
		V:          1,
		TS:         call.now,
		Isolated:   os.Getenv("JITR_ISOLATED") != "",
		Event:      *call.ev,
		GateFailed: call.gateFailed,
		Result:     journal.ResultRecord{Action: call.result.Action},
		Ms:         int(time.Since(call.started).Milliseconds()),
		Err:        recordError,
	}
	if call.resolved != nil {
		record.Profiles = append([]string(nil), call.resolved.Profiles...)
	}
	record.Result.Chars = utf8.RuneCountInString(call.result.Text)
	record.Units = call.unitRecords()
	return record
}

func (call *invocation) unitRecords() []journal.UnitRecord {
	records := make([]journal.UnitRecord, 0, len(call.gated)+len(call.gateMissed))
	for _, candidate := range call.gated {
		record := journal.UnitRecord{
			ID:      candidate.unit.ID,
			Hash:    candidate.unit.Hash,
			NoBlock: candidate.noBlock,
			Skip:    call.result.Skips[candidate.unit.ID],
		}
		if candidate.verdict != nil {
			record.Judge = judgeRecord(*candidate.verdict)
		}
		if slices.Contains(call.result.Delivered, candidate.unit.ID) {
			record.Delivered = "inject"
		}
		if slices.Contains(call.result.Blocked, candidate.unit.ID) {
			record.Delivered = "block"
		}
		records = append(records, record)
	}
	return append(records, call.gateMissed...)
}

func judgeRecord(verdict judge.Verdict) *journal.JudgeRecord {
	return &journal.JudgeRecord{
		P:            verdict.P,
		Hit:          verdict.Hit,
		Fallback:     verdict.Fallback,
		Unconfigured: verdict.Unconfigured,
		Backend:      verdict.Backend,
		Model:        verdict.Model,
		ConnMs:       verdict.ConnMs,
		Ms:           verdict.Ms,
		Err:          verdict.Err,
		Exit:         verdict.Exit,
		Stderr:       verdict.Stderr,
	}
}

func profileStoreContent(call *invocation) bool {
	if call.resolved == nil {
		return false
	}
	return call.resolved.Settings.Journal.StoreContent
}
