# Harness definitions: detection, decoding, shell write recognition, encoding (harness lane)

**Read first**: the "Detection", "Decoding", and "Encoding" sections of spec/harness.md and the "Hook entries" paragraph under "hooksjson Installer", all of spec/event.md, and the event and harness sections of spec/contracts.md.

**Change only**: `internal/harness/`.

## Steps

1. Load built-in definitions and definitions from the user directory; `DetectDef`, `Decode`, `Extract`, `Encode`.
2. Put shell write recognition in its own `shellwrite.go` (plus `shellwrite_lex.go` if needed), implementing the four steps of event.md exactly; add no recognition rules that are not in the spec.
3. `HookEntries`, `CommandString`.

## Required tests

- **Table-driven golden test**: iterate over `testdata/payloads/<harness>/*.json`, run DetectDef + Decode on each (the fixtures' cwd `/work/proj` does not exist, so `exists` is always false), and compare the result field by field with `testdata/expected.json[harness][file name without .json]` (only the fields present in expected; `paths: []` equals nil). Every fixture must have an entry in expected, and vice versa.
- **Shell write test table**: per the "Test baseline" section of event.md, run every case of `testdata/shell_writes.json`, using the case name as the subtest name.
- Decode sets `HookEvent` (for example `PreToolUse` for claude-code's `pre_write`).
- Put a definition with `name: codex`, `detect.order: 1`, and tools mapping Bash to `x` in the user directory. Afterwards codex's `pre_shell` decodes with Tool `x`.
- A definition with an unknown field fails to load.
- `HookEntries`: for the codex definition, the PreToolUse handler has `additionalContextLimit: 5000`, and the SessionStart group has no matcher. `CommandString` under `linux` and `windows` equals hand-written expected strings.
- Encode: the three Actions with and without a Notice, under `system_message` true and false, 12 combinations in all, each compared with a hand-written expected string; inject's `hookEventName` comes from `ev.HookEvent`; when Text contains quotes, newlines, or Chinese, the output is still valid JSON.

## Done when

`make check` passes. Commit `harness definitions`.
