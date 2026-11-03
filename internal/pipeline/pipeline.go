package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"time"

	"github.com/chilogen/jit-rules/internal/deliver"
	"github.com/chilogen/jit-rules/internal/event"
	"github.com/chilogen/jit-rules/internal/gate"
	"github.com/chilogen/jit-rules/internal/harness"
	"github.com/chilogen/jit-rules/internal/journal"
	"github.com/chilogen/jit-rules/internal/judge"
	"github.com/chilogen/jit-rules/internal/profile"
	"github.com/chilogen/jit-rules/internal/session"
	"github.com/chilogen/jit-rules/internal/unit"
)

const maxInputBytes = 10 * 1024 * 1024

type invocation struct {
	raw        []byte
	now        time.Time
	started    time.Time
	def        *harness.Definition
	ev         *event.Event
	store      *session.Store
	state      *session.State
	stateless  bool
	resolved   *profile.Resolved
	gates      []GateTrace
	gateFailed int
	gated      []gatedUnit
	gateMissed []journal.UnitRecord
	hits       []deliver.Hit
	notices    []string
	result     deliver.Result
	persist    bool
	accounted  bool
}

type gatedUnit struct {
	unit    *unit.Unit
	outcome gate.Outcome
	verdict *judge.Verdict
	noBlock bool
}

func run(stdin []byte, now time.Time) harness.Output {
	if os.Getenv("JITR_DISABLE") != "" {
		return harness.Output{}
	}
	call := &invocation{raw: stdin, now: now, started: time.Now(), persist: true}
	if err := call.execute(); err != nil {
		journal.LogError(errorStage(err), errors.Unwrap(err))
		return harness.Output{}
	}
	if call.ev == nil {
		return harness.Output{}
	}
	return harness.Encode(call.def, call.ev, call.result)
}

func (call *invocation) execute() error {
	if err := call.readAndDetect(); err != nil {
		return err
	}
	if err := call.decode(); err != nil {
		if errors.Is(err, harness.ErrIgnored) {
			call.ev = nil
			return nil
		}
		return staged("decode", err)
	}
	if isAccountingOnlyKind(call.ev.Kind) {
		if err := call.accountState(); err != nil {
			return err
		}
		return call.finishAccounting()
	}
	done, err := call.claim()
	if err != nil || done {
		return err
	}
	if call.ev.Kind == "tool_result" && !call.stateless {
		if err := call.accountState(); err != nil {
			return err
		}
	}
	if err := call.resolve(); err != nil {
		return staged("resolve", err)
	}
	call.match()
	call.judge()
	call.deliver()
	if err := call.persistState(); err != nil {
		return staged("state", err)
	}
	call.appendJournal("")
	call.maybeGC()
	return nil
}

func (call *invocation) readAndDetect() error {
	if len(call.raw) > maxInputBytes {
		return staged("read", fmt.Errorf("payload exceeds 10MB"))
	}
	var payload map[string]any
	if err := json.Unmarshal(call.raw, &payload); err != nil {
		return staged("read", err)
	}
	defs, err := harness.LoadAll()
	if err != nil {
		return staged("detect", err)
	}
	call.def = harness.DetectDef(defs, payload)
	if call.def == nil {
		return staged("detect", fmt.Errorf("no harness matched payload"))
	}
	return nil
}

func (call *invocation) decode() error {
	ev, err := harness.Decode(call.def, call.raw, call.now)
	if err != nil {
		return err
	}
	call.ev = ev
	call.store = session.Open(ev.Harness, ev.SessionID)
	return nil
}

func isAccountingOnlyKind(kind string) bool {
	return kind == "session_start" || kind == "compact"
}

func (call *invocation) accountState() error {
	locked, err := call.store.Update(call.now, func(state *session.State) {
		switch call.ev.Kind {
		case "tool_result":
			state.Bytes += call.ev.Size
		case "compact":
			state.Delivered = make(map[string]*session.Mark)
		}
	})
	if err != nil {
		return staged("state", err)
	}
	call.stateless = !locked
	call.accounted = locked
	return nil
}

func (call *invocation) finishAccounting() error {
	call.result = deliver.Result{Action: "none"}
	call.appendJournal("")
	call.maybeGC()
	return nil
}

func (call *invocation) claim() (bool, error) {
	if call.ev.DedupeKey == "" {
		return false, nil
	}
	key := call.ev.DedupeKey
	if call.ev.Kind == "tool_result" {
		key = "result:" + key
	}
	duplicate, locked, err := call.store.Claim(key, call.now)
	if err != nil {
		return false, staged("claim", err)
	}
	if !locked {
		call.stateless = true
		return false, nil
	}
	if !duplicate {
		return false, nil
	}
	call.result = deliver.Result{Action: "none"}
	call.appendJournal("dedupe")
	return true, nil
}

func (call *invocation) resolve() error {
	resolved, err := profile.Resolve(call.ev.Cwd)
	if err != nil {
		return err
	}
	call.resolved = resolved
	if len(resolved.Errors) > 0 {
		call.notices = append(call.notices, "jit-rules：有规则加载失败，运行 jitr status 查看")
	}
	if call.persist {
		for _, resolveErr := range resolved.Errors {
			journal.LogError("resolve", resolveErr)
		}
	}
	if call.stateless {
		return nil
	}
	call.state, err = call.store.Read()
	return err
}

func (call *invocation) match() {
	for _, candidate := range call.resolved.Units {
		if !slices.Contains(candidate.Trigger.Events, call.ev.Kind) {
			continue
		}
		outcome := gate.Match(candidate, call.ev)
		call.gates = append(call.gates, GateTrace{
			ID: candidate.ID, OK: outcome.OK, Failed: outcome.Failed,
		})
		if !outcome.OK {
			call.gateFailed++
			call.recordGateMiss(candidate, outcome)
			continue
		}
		call.gated = append(call.gated, gatedUnit{unit: candidate, outcome: outcome})
	}
}

// recordGateMiss keeps a unit delivered earlier in this session in the
// journal even when it fails the gate (pipeline.md step 7).
func (call *invocation) recordGateMiss(candidate *unit.Unit, outcome gate.Outcome) {
	if call.stateless || call.state == nil || call.state.Delivered[candidate.ID] == nil {
		return
	}
	call.gateMissed = append(call.gateMissed, journal.UnitRecord{
		ID: candidate.ID, Hash: candidate.Hash, Gate: outcome.Failed,
	})
}

func (call *invocation) judge() {
	judged := make([]*unit.Unit, 0, len(call.gated))
	for _, candidate := range call.gated {
		if candidate.unit.Judge != nil {
			judged = append(judged, candidate.unit)
		}
	}
	deadline := time.Duration(call.resolved.Settings.DeadlineMs) * time.Millisecond
	ctx, cancel := context.WithDeadline(context.Background(), call.started.Add(deadline))
	defer cancel()
	verdicts := judge.Decide(ctx, call.ev, judged, call.resolved.Settings)
	for index := range call.gated {
		call.applyVerdict(index, verdicts)
	}
}

func (call *invocation) applyVerdict(index int, verdicts map[string]judge.Verdict) {
	candidate := &call.gated[index]
	if candidate.unit.Judge == nil {
		candidate.noBlock = candidate.outcome.ContentUncertain
		call.hits = append(call.hits, deliver.Hit{
			Unit: candidate.unit, NoBlock: candidate.noBlock,
		})
		return
	}
	verdict := verdicts[candidate.unit.ID]
	candidate.verdict = &verdict
	if verdict.Unconfigured {
		notice := fmt.Sprintf(
			"jit-rules：judge 后端没有配置好（%s），带模型判断的规则不会生效。运行 jitr status 查看",
			verdict.Err,
		)
		if !slices.Contains(call.notices, notice) {
			call.notices = append(call.notices, notice)
		}
		return
	}
	if !verdict.Hit {
		return
	}
	candidate.noBlock = verdict.Fallback ||
		candidate.outcome.ContentUncertain ||
		(call.ev.ContentUnknown && candidate.unit.Judge != nil)
	call.hits = append(call.hits, deliver.Hit{
		Unit: candidate.unit, Verdict: &verdict, NoBlock: candidate.noBlock,
	})
}

func (call *invocation) deliver() {
	state := call.state
	if call.stateless {
		state = nil
	}
	call.result = deliver.Decide(call.hits, call.ev, state, call.resolved.Settings)
	if len(call.notices) > 0 && !call.stateless &&
		call.state != nil && !call.state.Notified {
		call.result.Notice = joinNotices(call.notices)
	}
}

func (call *invocation) persistState() error {
	if call.stateless {
		return nil
	}
	locked, err := call.store.Update(call.now, func(state *session.State) {
		before := state.Bytes
		eventBytes := call.ev.Size
		if call.accounted {
			eventBytes = 0
		}
		state.Bytes += eventBytes + len(call.result.Text)
		for _, id := range call.result.Delivered {
			state.Delivered[id] = &session.Mark{At: before}
		}
		if call.result.Notice != "" {
			state.Notified = true
		}
	})
	if !locked {
		call.stateless = true
	}
	return err
}

func (call *invocation) maybeGC() {
	if rand.IntN(100) != 0 {
		return
	}
	session.GC(call.now)
	keepDays := profile.Defaults().Journal.KeepDays
	if call.resolved != nil {
		keepDays = call.resolved.Settings.Journal.KeepDays
	}
	journal.GC(call.now, keepDays)
}

func joinNotices(notices []string) string {
	result := ""
	for _, notice := range notices {
		if result != "" {
			result += "\n"
		}
		result += notice
	}
	return result
}

type pipelineError struct {
	stage string
	err   error
}

func (err *pipelineError) Error() string { return err.stage + ": " + err.err.Error() }
func (err *pipelineError) Unwrap() error { return err.err }

func staged(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &pipelineError{stage: stage, err: err}
}

func errorStage(err error) string {
	var target *pipelineError
	if errors.As(err, &target) {
		return target.stage
	}
	return "pipeline"
}
