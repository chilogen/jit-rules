# Installers (harness lane, after harness definitions)

**Read first**: the "Installation" section of spec/harness.md, the "Common rules" and "Installation" sections of spec/cli.md, and the installer section of spec/contracts.md.

**Change only**: `internal/installer/` (including both implementations), `internal/cli/install.go`, `testdata/install/`.

## Steps

1. `internal/installer`: registry, `TargetPaths`, `Apply`. Hook entries and command strings come from `harness.HookEntries` and `harness.CommandString`; do not write another copy.
2. `internal/installer/hooksjson`, `internal/installer/dshpatch`: `Plan` and `Installed`.
3. CLI: `install`, `uninstall`, `upgrade`, `purge`.

## Required tests (all under JITR_HOME and a fake HOME: set `HOME` in the test, and `USERPROFILE` on Windows)

- For claude-code, on a settings.json that already has other hooks (testdata/install/claude-settings-with-other-hooks.json, hand-written, 2-space indentation, trailing newline): after install there are entries for 4 events and the other hooks are all still there; after uninstall, the file is **byte-identical** to the original. The file must contain another hook whose command includes `2>/dev/null && echo ok`, plus a number `1.5` and a large integer `12345678901234567890`.
- For codex, where the target hooks.json does not exist: after install the file exists; after uninstall it does not (the Change returned by Plan has After nil). Same for trae (`file_defaults: {version: 1}`).
- Installing twice in a row gives the same result (idempotent).
- codex: the handler has `additionalContextLimit: 5000` (only on PreToolUse and UserPromptSubmit).
- trae: when only `~/.trae-cn/` exists, only that file is written; when neither exists, it is an error; the new file contains `"version": 1`.
- deepseek: the existing `cordis.patch.yml` has other patches and comments; after install they are all still there and a jit-rules-hooks entry was added; installing again does not duplicate it; after uninstall the comments and other entries are still there (whitespace differences from YAML serialization are allowed).
- `--scope project` with deepseek is an error, exit code 1.
- Command string: installing the same harness twice gives exactly the same command.
- `--dry-run` writes no files.
- **Installed**, tested for all four harnesses: false before install; true after install; false after uninstall; false when the target file exists with only other people's hooks; for trae with no configuration directory at all it returns false rather than an error.
- When `Apply` overwrites an existing file, a `.bak-jitr-*` backup is left.

## Done when

`make check` passes. Commit `installers`.
