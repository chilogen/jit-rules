# Change: judgment contracts (sequential; the two lanes below start only after this)

**Read first**: the unit, profile, judge, and journal sections of spec/contracts.md; "Script block" and "Frontmatter fields" of spec/unit.md.

**Change only**: `contract.go` of `internal/unit`, `internal/profile`, `internal/judge`, `internal/journal`.

This task is the only one allowed to change frozen contracts, and only to the new state of contracts.md.

## Steps

1. Add the new fields of `unit.Judge`, `judge.Verdict`, `journal.JudgeRecord` exactly as contracts.md writes them.
2. Implement fully: `Settings.ThresholdOf`, `Settings.Threshold` (now `ThresholdOf(s.Judge.Backend)`), `(*unit.Judge).Cached`.
3. Add the signature comment of `unit.Describe`; its body is unchanged here.
## Required tests

- `ThresholdOf`: defaults give 0.3 for the default backend and 0.5 for `openai`; 0.5 for a missing backend; an integer `1` gives 1. `Threshold()` equals `ThresholdOf(s.Judge.Backend)`.
- `Cached`: nil with Question → true; nil with Script → false; an explicit value wins either way.
- Every existing test still passes.

## Done when

`make check` passes. Commit `judgment contracts`.
