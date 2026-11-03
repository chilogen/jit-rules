# AGENTS.md

This repository implements `jitr` (just-in-time rules).

- **When implementing, read only `docs/runbook/`.** Start with `docs/runbook/README.md`, then work through the tasks in the order it gives.
- The spec (`docs/runbook/spec/`) is the only authority; types and signatures follow `spec/contracts.md`.
- Do not change any file under `docs/runbook/` except `ACCEPTANCE.md`.
- For details the spec does not cover, choose the simplest implementation.
- When the spec contradicts itself, the contract is insufficient, or following the spec would produce something wrong: do not invent semantics and do not work around it (for example by concatenating strings to dodge a check, comparing error text, or simulating with another interface). Skip the item, continue with the rest, and report the problem at the end of the task.
- `assets/`, `testdata/payloads/`, `testdata/expected.json`, `testdata/shell_writes.json`, and `testdata/smoke/` are part of the spec; do not change them.
- Never change tests or test data to make tests pass; when a test fails, fix the code.
- After each task, run `make check` and commit once it passes, with the task name as the commit message (for example `packs and profiles`).
- Tests must set `JITR_HOME=t.TempDir()` and never read or write real user directories.
