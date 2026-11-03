# Shared contracts, and the small packages every lane needs (sequential, after the repository skeleton; the four lanes start only after this)

**Goal**: write every type and function signature used between packages in one go, so the four lanes can each write their own packages without waiting for each other; also fully implement a few small packages every lane needs, and the unified command-line error handling.

**Read first**: all of spec/contracts.md, "File paths" in spec/layout.md, spec/event.md, "Common rules" in spec/cli.md, "settings" in spec/profile-pack.md.

## Steps

1. **Implement fully**:
   - `paths`: every function. The Windows part goes in `paths_windows.go`.
   - `fileutil`: every function.
   - `event`: `Event`, `CanonicalTool`, `SizeOf`, `DedupeKey`.
   - `profile`: only `Settings` and its sub-structures, `Defaults()` (parses the embedded `assets/defaults.yaml`), and `Settings.Threshold()`; the other functions are left to the packs and profiles task.
   - `backend`: `ErrNotConfigured`, `Register`, `Get`, `Names` (the registry is tiny and every lane needs it).
   - `cli`: `root.go` (cobra root command, persistent flag `--json`, `Register`, `Execute`), `output.go` (`Output`), `errors.go` (`Internal`, `Silent`), and the `version` command (prints `Version` and `Commit`). The rules for `Execute` are in "Common rules" of cli.md and must be complete in one go.
   - `cmd/jitr/main.go`: import `internal/plugins`, `os.Exit(cli.Execute())`, and add the recover described in "Iron rule" of pipeline.md (at this point `journal.LogError` still panics, so wrap the call inside recover in another recover).
2. **Signatures only**: write every other type in contracts.md in full; write every function's signature with the body `panic("not implemented")`. Put them in their own packages, always in a file named `contract.go`. Later tasks only replace these bodies, never the types or signatures (see the top of contracts.md).
3. Create an empty package for each concrete implementation, containing only `doc.go` (`// Package jev ...` + `package jev`):
   - `internal/backend/{jev,openai,fake}`
   - `internal/installer/{hooksjson,dshpatch}`
   - `internal/launcher/{claudesettings,codexconfig,dshpatch,manual}`
4. Write `internal/plugins/plugins.go` (contents in layout.md). **Nobody may change this file afterwards.**
5. `go build ./...` must pass.

## Required tests

- With `JITR_HOME=/x`, all five paths are under `/x`.
- Without `JITR_HOME` and with `XDG_CONFIG_HOME=/c`, `Config()` is `/c/jit-rules`.
- `Expand("{env:DSH_HOME:~/.dsh}/a", "")`: `<home>/.dsh/a` when the variable is empty, `/d/a` when it is `/d`. `Expand(".claude/settings.json", "/r")` gives `/r/.claude/settings.json`.
- `ProjectRoot`: create `a/.git` and `a/b/c` in t.TempDir; passing `a/b/c` gives `a`; passing a non-existent path gives the path itself.
- `fileutil.WriteAtomic` succeeds into a non-existent nested directory; `Backup` produces a file whose name carries a timestamp and whose content is identical; after `ReplaceExecutable`, dst's content equals src's.
- `CanonicalTool`: `mcp__fs__write_file` → `mcp/fs/write_file`; `mcp__a__b__c` → `mcp/a/b__c`; `Foo`, absent from the table → `foo`.
- `DedupeKey`: tool uses the first field present; when a prompt has none of the fields, the key is `prompt:sha1:<hex>`.
- Every value of `Defaults()` equals `assets/defaults.yaml` referenced by profile-pack.md; `Defaults().Threshold()` is 0.3; 0.5 after setting Judge.Backend to `openai`; 0.5 for a backend that does not exist; 1 when the threshold is written as the integer `1`.
- `Output`: with `--json`, the output is valid JSON with the four keys `ok`, `data`, `errors`, `warnings`; `errors` and `warnings` are `[]` when empty.
- `Execute` (register a few temporary subcommands in the test and drive them with `SetArgs`):
  - returning nil → 0; `errors.New("x")` → 1 with x on stderr; `Internal(errors.New("x"))` → 3; `Silent(5)` → 5 with no output at all;
  - unknown command, unknown flag, missing required argument → 1;
  - `fmt.Errorf("a: %w", &fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES})` → 3; `fmt.Errorf("a: %w", &fs.PathError{Op: "open", Path: "/x", Err: fs.ErrNotExist})` → 1; `fmt.Errorf("a: %w", &exec.ExitError{})` → 3; `fmt.Errorf("a: %v", &exec.ExitError{})` (with %v, the chain is broken) → 1;
  - each case above with `--json`: stdout is one valid JSON object with `ok` false (except Silent, which prints nothing); the same holds for an unknown flag.
  - Register a temporary parent command `grp` with only subcommands and no RunE: `grp foo` → 1; `grp` → 0 and prints help; `grp --json` → stdout is a JSON object with `ok` true and non-empty `data.help`; `help --json` likewise; `completion` → 1 (unknown command).

## Done when

`make check` passes (the architecture test should pass now, since the concrete implementation packages are empty). Commit `shared contracts`.
