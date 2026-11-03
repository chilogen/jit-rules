# Unit commands, evaluation, diagnostics (joins after the hook pipeline, profile and pack commands, and installers)

**Read first**: the "Evaluation" section of spec/judge.md, the "Units" and "Diagnostics" sections of spec/cli.md, and the evaluate section of spec/contracts.md.

**Change only**: `internal/evaluate/`, `internal/cli/unit.go`, `internal/cli/diag.go` (check, explain, guide, status), plus `checkPack` in `internal/cli/pack.go` and `testdata/evaluate/`. Split into `unit_*.go` and `diag_*.go` when a file gets too long.

## Steps

1. `internal/evaluate`: Test, SaveTest, LoadTest, Replay, LoadCoHits, Conflicts, FormatConflict.
2. CLI: `unit list/new/test/replay/check/show/enable/disable/rm`, plus `check`, `explain`, `guide`, `status`.
3. Replace the `checkPack` placeholder in `internal/cli/pack.go` with the real tests and conflict check: the conflict check covers the new units among themselves, and the new units against `active`.

## Required tests (judge always uses the fake backend; every command is driven through `cli.Execute`)

Walk through the AI's whole authoring workflow (the most important test, named `TestAuthoringLoop`):
1. `profile init`;
2. `unit new me git.commit-zh --from-stdin` with a valid unit: events [tool], tools [shell], the shell condition from `assets/unit-template.md`, plus a judge; block; 2 hit examples and 2 miss examples (one of them the "after the fix" commit) → the file is created, `unit list --all` shows draft;
3. `hook` with a shell event `git commit -m "fix bug"` → nothing delivered (draft);
4. `unit enable` → fails with exit code 1, telling you to run `unit test` first;
5. `unit test` (the fake gives hit examples 0.9 and miss examples 0.1) → all pass, exit code 0;
6. `unit check` → no conflicts;
7. `unit enable` → succeeds; jitr.lock has a record with the right `tests` counts;
8. `hook` with the same event again (another tool_use_id) → Exit 2;
9. `hook` with `git commit -m "修复登录错误"` (another tool_use_id) → not blocked;
10. change one character in the unit file → `unit list --all` shows modified; `hook` with the event from step 8 → no longer delivered;
11. `unit test` + `unit enable` → active again.

Other tests:
- `unit new` with a block unit violating rule 12 (every miss example uses a different tool from the hits) → fails with exit code 1, and the file is deleted.
- With no key configured (the fake returns `ErrNotConfigured`), `unit test` of a unit with a judge → examples that pass the gate are recorded as skipped, exit code 0; `unit enable` succeeds and prints a warning.
- Conflicts: two units whose bodies say "commit in Chinese" and "commit in English", with examples that fire together, and the fake returns 0.9 for the conflict question → `unit check` reports the conflict with the 4 suggestions, exit code 1; after giving them the same topic, checking again → no conflict. When the fake returns `ErrNotConfigured` for the conflict question → reports "possible conflict", exit code 0.
- Conflicts from the journal: two units whose examples do not overlap, plus a journal record where both hit on the same event → `Conflicts` with `LoadCoHits()` reports the pair, with the example summary starting with `真实事件 `; a unit the journal recorded as a judge miss on that event is not paired.
- `unit replay`: first produce 3 journal records through the pipeline, 2 of which should hit → prints 2 entries with a hit rate of 2/3.
- `explain --last` names the field that failed the gate; with the smoke profile registered, `explain --event testdata/payloads/codex/pre_shell_write.json` prints block.
- `unit disable` a pack unit → it appears in profile.yaml's disable list; `unit enable` it again → it is gone from the list.
- `unit list --all` lists `invalid` units (an unparseable file), and `hook` is unaffected.
- `status`: after installing claude-code in a fake HOME with the installer from the installers task, the output shows claude-code installed and codex not installed; when the api key variable is not set, it says so.
- `pack add` of a local pack with a unit whose tests fail → fails with exit code 1; with `--force` → succeeds and prints a warning.
- With "commit in Chinese" active in the profile, `pack add` of a local pack with a "commit in English" unit, the fake returning 0.9 for the conflict question → fails with exit code 1 and reports the conflict; with `--force` → succeeds.
- The output of `guide` is byte-identical to assets/guide.md.

## Done when

`make check` passes. Commit `unit commands and evaluation`.
