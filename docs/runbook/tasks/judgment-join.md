# Change: wiring, evaluation, commands, acceptance (sequential, on main, after both lanes are merged)

**Read first**: the journal section of spec/pipeline.md; "Evaluation" of spec/judge.md; `profile show`, `unit show`, `status` in spec/cli.md; "Script judge" of spec/judge.md.

**Change only**: `internal/pipeline/journal.go`, `internal/evaluate/`, `internal/cli/` (`profile.go`, `unit_run.go`, `pack.go`, `diag.go`, `diag_status.go`), their tests, `testdata/profiles/`, `docs/runbook/ACCEPTANCE.md` (a new section only).

## Steps

1. Pipeline: copy `Verdict.Exit` and `Verdict.Stderr` into `JudgeRecord`.
2. Evaluate: script units in `Test` (skipped), `Replay` (Unknown unless reused from the journal), `Conflicts` (may fire together).
3. CLI: every `unit.Describe` caller passes `ThresholdOf(<the unit's backend>)`; `status` lists each backend and interpreter in use.
4. End-to-end test and real-harness check below.

## Required tests

- **The two-rule scenario** (end-to-end through `cli.Execute` and `hook`, skip on Windows or without git): a `t.TempDir()` repository with `master` and a branch; profile `testdata/profiles/branch/` (registered and its units enabled through the CLI inside the test) with two inject units, both `tools: [shell]`, `shell: ['^git\s+commit\b']`, no topic:
  - `merge-master`: script `-gt 3`, body asks to merge master;
  - `notify-user`: script `-gt 5`, body asks to tell the user.
  Feed claude-code `pre_shell` payloads (built in the test from `testdata/payloads/claude-code/pre_shell.json` with `cwd` set to the repository and the command `git commit -m x`; a new `tool_use_id` and session each time so redelivery does not interfere):
  - branch 2 commits ahead → no output;
  - 4 ahead → inject with `merge-master` only;
  - 6 ahead → inject with both, `merge-master` first (same profile, ties broken by id).
  The journal records for the 6-commit call have `backend` `script` and `exit` 0 for both units; for the 4-commit call `notify-user` has `exit` 1.
- A script unit with `deliver.as: block` whose script exits 3 → the call is not blocked (Exit 0, inject), journal `fallback` true.
- `deadline_ms: 300` and a script `sleep 5` → `hook` returns within 1s with exit 0.
- `unit test` of a script unit → examples passing the gate are `skipped` with the note from judge.md, exit 0; `unit enable` succeeds with a warning.
- `unit replay` of a script unit: a journal record with the same hash is reused; without one the event counts as Unknown even with `--live`, and the script is not run (check with a marker file).
- `unit check` of two script units whose examples both pass the gates → "possible conflict", exit 0.
- `unit show` of a script unit contains the script; `profile show` of a question unit with `backend: openai` shows threshold 0.5 while the default backend's units show 0.3.
- `status` with one jev unit, one openai unit, and one `sh` script unit lists both key variables and `sh`.

## Real-harness check

With temporary HOME and JITR_HOME, install into Claude Code, use the `branch` profile in a scratch repository 6 commits ahead, and ask the agent to commit. Pass: the agent's context contains both rule bodies, and the journal shows both script runs with their `ms` and `exit`. Record the result as a new section of ACCEPTANCE.md.

## Done when

`make check` passes. Commit `judgment join`.
