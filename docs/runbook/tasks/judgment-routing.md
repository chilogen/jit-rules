# Change: per-unit backends and script judges (judgment lane; parallel with the units lane)

**Read first**: "judge.Decide" and "Script judge" of spec/judge.md; the judge, backend, and profile sections of spec/contracts.md; "File paths" of spec/layout.md.

**Change only**: `internal/judge/`, `internal/backend/fake/` (only if a test needs a second fake instance helper).

Build test units by hand (`&unit.Unit{ID: ..., Judge: &unit.Judge{...}}`); do not wait for the units lane.

## Steps

1. Decide: route to groups by backend and to scripts; run all concurrently; per-group threshold `ThresholdOf`; per-unit cache option.
2. Script judge: interpreter lookup, directory check, script file in `{cache}/scripts/`, run, result mapping, optional cache.

## Required tests (fake backends; scripts use `sh`, skip on Windows via `runtime.GOOS` in the test)

- Two fakes registered under two names, three units (two on fake A by default backend, one with `Backend: B`) → A called once with two questions, B once with one; each verdict's `Backend` is its group's name; thresholds differ per backend (A 0.3, B 0.8, both fakes return 0.5 → A hits, B misses).
- One group returning `ErrNotConfigured` does not affect the other group's verdicts.
- `Cache: false` on a question unit → a second identical call calls the backend again for that unit only, while the cached unit is not re-sent.
- Script `exit 0` → Hit, P 1, `Exit` 0, `Backend` `script`; `exit 1` → miss, `Exit` 1; `exit 3` → Fallback hit with `Exit` 3; `echo oops >&2; exit 1` → `Stderr` `oops`.
- The script runs in `ev.Cwd`: in a `t.TempDir()` repository (`git init`, two commits; skip when git is absent) a script comparing `git rev-list --count HEAD` to 1 hits.
- The script reads stdin: `grep -q '"tool":"shell"'` hits for a shell event.
- `JITR_UNIT` equals the unit id inside the script.
- Interpreter `no-such-interp` → Unconfigured, Err contains it; no process started.
- `ev.Cwd` missing → Fallback.
- ctx with a 200ms deadline and `sleep 5` → returns within 1s, Fallback, `Exit` nil.
- Script default: two identical calls run the script twice (count via a file the script appends to); with `Cache: true` once, and the second verdict has `Ms` 0 and `Exit` nil; a different `ev.Cwd` misses the cache.
- A script unit and a question unit in one call: total time ≈ the slower of the two, not the sum (script `sleep 0.3`, fake delay 300ms, total < 550ms).
- The script file is under `{cache}/scripts/` (with `JITR_HOME=t.TempDir()`), written once for two calls, named `<sha256>.sh` for `sh`; the extension function gives `ps1` for `pwsh` and `powershell` and `python3` for `python3`.
- The function mapping an exit code to a Verdict gives Unconfigured for 9009 (call it directly: Unix cannot produce 9009).
- All existing judge tests still pass.

## Done when

`make check` passes (including `go test -race ./internal/judge/`). Commit `judgment routing`.
