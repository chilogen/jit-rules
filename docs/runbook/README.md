# jit-rules implementation manual

> Revised 2026-11-02. After the first implementation built from this manual, the core acceptance check failed and six fixes followed; most causes were in the spec itself (see "Problems exposed by the first implementation" in [../design/README.md](../design/README.md)). This revision changed the blocking semantics, added shell write recognition, rewrote how packs are pinned, gathered every type into contracts.md, and moved the real-harness smoke test forward into the hook pipeline task.

You are implementing a command-line tool called `jitr`. It installs as a hook into AI coding tools (Claude Code, Codex, TRAE, DeepSeek Harness). Whenever the user sends a message or the AI is about to call a tool, the harness runs `jitr hook` once. jitr decides which of the user's rules apply at that moment and puts the rule text in front of the AI: either into its context, or by blocking the tool call and asking it to fix the call according to the rule.

## Task order

```
repository skeleton → shared contracts ─┬─ rules lane:     judgment units → packs and profiles → profile and pack commands ─┐
                                         ├─ harness lane:   harness definitions → installers → isolated launch ──────────────┤
                                         ├─ judgment lane:  model judgment ─────────────────────────────────────────────────┤
                                         └─ runtime lane:   gate, session, delivery, journal ────────────────────────────────┘
hook pipeline (needs packs and profiles, harness definitions, model judgment, runtime; includes the real-harness smoke test)
  → unit commands and evaluation (also needs profile and pack commands, installers)
  → release and acceptance (also needs isolated launch)
```

| Task | File |
|---|---|
| Repository skeleton | [tasks/repository-skeleton.md](tasks/repository-skeleton.md) |
| Shared contracts | [tasks/shared-contracts.md](tasks/shared-contracts.md) |
| Judgment units | [tasks/units.md](tasks/units.md) |
| Packs and profiles | [tasks/packs-and-profiles.md](tasks/packs-and-profiles.md) |
| Profile and pack commands | [tasks/profile-and-pack-commands.md](tasks/profile-and-pack-commands.md) |
| Harness definitions | [tasks/harness-definitions.md](tasks/harness-definitions.md) |
| Installers | [tasks/installers.md](tasks/installers.md) |
| Isolated launch | [tasks/isolated-launch.md](tasks/isolated-launch.md) |
| Model judgment | [tasks/model-judgment.md](tasks/model-judgment.md) |
| Gate, session, delivery, journal | [tasks/runtime.md](tasks/runtime.md) |
| Hook pipeline | [tasks/hook-pipeline.md](tasks/hook-pipeline.md) |
| Unit commands and evaluation | [tasks/unit-commands.md](tasks/unit-commands.md) |
| Release and acceptance | [tasks/release.md](tasks/release.md) |

- **The repository skeleton and shared contracts run in sequence**: the shared contracts task writes every cross-package type and signature (spec/contracts.md); this is what lets the four lanes start at the same time.
- **The rules, harness, judgment, and runtime lanes can run in parallel**; within a lane, tasks run in the listed order.
- **The hook pipeline, unit commands, and release tasks run in sequence**, each starting after everything it depends on is merged. The hook pipeline task ends with a smoke test on real Claude Code and Codex; the unit commands task starts only after the smoke test passes.

## Change: per-unit judgment (2026-11-03)

The tasks above are done. This change lets each unit choose its model backend, adds script judges written as a `jitr-judge` code block in the unit's Markdown, and makes caching a per-unit option. Spec changes are in unit.md, judge.md, contracts.md, pipeline.md, cli.md, layout.md, and `assets/guide.md`.

```
judgment contracts ─┬─ units lane:     judgment units ──┐
                    └─ judgment lane:  judgment routing ┘
judgment join (on main, after both lanes are merged)
```

| Task | File |
|---|---|
| Judgment contracts | [tasks/judgment-contracts.md](tasks/judgment-contracts.md) |
| Judgment units | [tasks/judgment-units.md](tasks/judgment-units.md) |
| Judgment routing | [tasks/judgment-routing.md](tasks/judgment-routing.md) |
| Judgment join | [tasks/judgment-join.md](tasks/judgment-join.md) |

- **Judgment contracts runs first, alone**: it is the only task allowed to change frozen contracts, so both lanes compile against the same types.
- **The two lanes run in parallel** under the rules below; they share no files (`internal/unit/` vs `internal/judge/`), and the judgment lane builds its test units by hand.
- **Judgment join runs last**: it touches the packages that consume both lanes (pipeline journal, evaluate, cli) and holds the end-to-end and real-harness checks.

## Rules for parallel work

1. **One git worktree and one branch per lane** (for example `git worktree add ../jitr-rules -b lane-rules`). Commit after each task within the lane.
2. **Change only the directories and files listed under "Change only" in the task.** After the shared contracts task, the following are off limits: `go.mod`, `internal/plugins/plugins.go`, `internal/cli/root.go`, `internal/cli/output.go`, `internal/cli/errors.go`, the type definitions and function signatures in every `contract.go` (function bodies are replaced by the task that owns the package, see the top of contracts.md), `assets/`, `testdata/payloads/`, `testdata/expected.json`, `testdata/shell_writes.json`, `testdata/smoke/`.
3. **Do not change any signature in contracts.md.** When you need functionality another lane has not finished, construct the data by hand in your tests or use a fake; do not wait, and do not implement it for the other lane.
4. **Merging**: when a lane (or a task within it) is done, rebase onto the latest main, run `make check`, and fast-forward main if it passes. When several lanes finish together, merge them one at a time and rerun `make check` after each.
5. The joining tasks (hook pipeline, unit commands, release) are done on main, without a worktree.

## Other rules

1. Each task starts with "Read first", listing the spec files it needs; those are enough.
2. The spec (`spec/`) is the only authority; types and signatures follow `spec/contracts.md`. Do what the spec says; for details the spec does not cover, choose the simplest option.
3. **When the spec contradicts itself, the contract is insufficient, or following the spec would produce something wrong: stop that item.** Skip it, continue with the rest, and report the problem at the end of the task. Do not invent semantics, do not edit `spec/`, and do not work around it (concatenating strings to dodge the architecture test, comparing error text, simulating a missing interface with another one). In the previous implementation, such workarounds hid spec problems until final acceptance.
4. The repository skeleton task copies the runbook's `assets/` and `fixtures/` into `assets/` and `testdata/` of the new repository. The copied files are part of the spec; do not change them.
5. Run `make check` at the end of each task and commit if it passes; the commit message is the task name.

## Spec files

| File | Contents |
|---|---|
| [spec/contracts.md](spec/contracts.md) | Every cross-package type and function signature (the only source, frozen after the shared contracts task) |
| [spec/layout.md](spec/layout.md) | Directory layout, extension points, file paths, code constraints |
| [spec/unit.md](spec/unit.md) | Judgment unit file format, validation rules, plain-language description format |
| [spec/profile-pack.md](spec/profile-pack.md) | Packs, profiles, lock file, registry, and how the active units are computed |
| [spec/event.md](spec/event.md) | Normalized events, tool names, field extraction, shell write recognition, dedupe keys |
| [spec/harness.md](spec/harness.md) | Harness definitions: detection, decoding, encoding, installation, isolated launch |
| [spec/pipeline.md](spec/pipeline.md) | Each step of a hook call, the gate, delivery rules, state, journal |
| [spec/judge.md](spec/judge.md) | Model judgment, request formats of both backends, testing, replay, conflict checks |
| [spec/cli.md](spec/cli.md) | All commands, exit codes, and error output |

## Test data (`fixtures/`)

In this repository, the runbook's `assets/` and `fixtures/` already live at `assets/` and `testdata/`.

| File | Contents |
|---|---|
| `fixtures/<harness>/*.json` | Hook payloads, with paths rewritten to `/work/proj`. Taken from real runs ([../design/evidence/](../design/evidence/)): Claude Code's prompt, session_start, pre_write, pre_edit, post_edit; Codex's prompt, session_start, pre_apply_patch, pre_shell, post_shell, pre_shell_write. The rest are examples synthesized from the structure of real payloads, documentation, and source code |
| `fixtures/expected.json` | The fields each payload should decode to |
| `fixtures/shell_writes.json` | The complete test table for shell write recognition (synthetic). The expected values were checked case by case against a reference implementation written from event.md, and cases with known content were also executed with real bash to check the files written |
| `fixtures/smoke/profile/` | The profile for the real-harness smoke test in the hook pipeline task: one blocking rule, "the first line of a Markdown file must be a level-1 heading", with its lock already written |

## The most important rules

1. **`jitr hook` must never interrupt the user's work.** Any error is treated as "empty output, exit 0". Exit 2 can only come from a block the rules explicitly require.
2. **Tests never read or write real user directories.** Always set `JITR_HOME=t.TempDir()`; tests that need HOME set HOME to a temporary directory.
3. **Never change tests or test data to make tests pass.**
4. **Differences between harnesses live only in the YAML definitions**; harness and backend names must not appear in Go code.
