# Spec: repository layout, extension points, paths, constraints

All types and function signatures are in [contracts.md](contracts.md) and are not repeated here.

## Technology (do not change)

- Go 1.23 or later. The module path is `github.com/chilogen/jit-rules` and the binary is `jitr`.
- Only these four third-party libraries are allowed: `gopkg.in/yaml.v3`, `github.com/bmatcuk/doublestar/v4`, `github.com/spf13/cobra`, `github.com/gofrs/flock`. Everything else (HTTP, JSON, regular expressions, hashing, running git) uses the standard library.
- Target platforms: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64. `CGO_ENABLED=0`.

## Directories (create exactly these)

```
jit-rules/
├── AGENTS.md                      # written by the repository skeleton task
├── Makefile
├── go.mod
├── cmd/jitr/main.go               # does two things only: recover fallback, os.Exit(cli.Execute())
├── assets/                        # embedded with go:embed, copied verbatim from runbook/assets/
│   ├── embed.go
│   ├── guide.md
│   ├── defaults.yaml              # default settings
│   ├── unit-template.md           # written by unit new without --from-stdin
│   ├── harnesses/*.yaml
│   └── packs/core/...
├── internal/
│   ├── paths/                     # computes every file path
│   ├── fileutil/                  # atomic writes, backups, replacing executables
│   ├── event/                     # normalized events
│   ├── unit/                      # judgment units: parse, validate, plain-language description
│   ├── pack/                      # packs: read, fetch, validate, hash
│   ├── registry/                  # registry.yaml
│   ├── profile/                   # profile.yaml, jitr.lock, active unit set, settings merge
│   ├── gate/                      # trigger evaluation
│   ├── judge/                     # judgment: route units to model backends or scripts, run scripts, fallback, cache
│   ├── backend/                   # Backend interface and registry
│   │   ├── jev/
│   │   ├── openai/
│   │   └── fake/                  # tests only
│   ├── harness/                   # harness definitions: load, detect, decode (incl. shell write recognition), encode
│   ├── installer/                 # Installer interface, registry, target paths, writing
│   │   ├── hooksjson/
│   │   └── dshpatch/
│   ├── launcher/                  # Launcher interface, registry, argument merging (isolated launch)
│   │   ├── claudesettings/
│   │   ├── codexconfig/
│   │   ├── dshpatch/
│   │   └── manual/
│   ├── session/                   # session state, file locks, dedupe, distance accounting
│   ├── deliver/                   # topic arbitration, redelivery filter, choosing the action, assembling the text
│   ├── pipeline/                  # one hook call; Explain for explain
│   ├── journal/                   # journal
│   ├── evaluate/                  # unit tests, replay, conflict checks
│   ├── cli/                       # cobra commands, one file per command group
│   ├── plugins/plugins.go         # blank-imports every backend, installer, launcher implementation
│   └── archtest/                  # architecture constraint tests
├── docs/runbook/                  # copied from runbook/ by the repository skeleton task
├── testdata/
│   ├── payloads/<harness>/*.json  # copied from runbook/fixtures/<harness>/
│   ├── expected.json              # copied from runbook/fixtures/expected.json
│   ├── shell_writes.json          # copied from runbook/fixtures/shell_writes.json
│   ├── smoke/                     # copied from runbook/fixtures/smoke/
│   └── ...                        # test data each task needs
└── scripts/funclen/main.go        # function length checker used by the project rule
```

## Extension points

| Extension point | Form | How the community extends it |
|---|---|---|
| harness | YAML data (`assets/harnesses/*.yaml`, or `{config}/harnesses/*.yaml` in the user directory) | Add a YAML file; no Go required |
| Judgment units / packs | Markdown + YAML data | Publish a git repository; users install with `jitr pack add` |
| Model backend | Go package implementing `backend.Backend` | Open a PR: add `internal/backend/<name>/`, add a line to plugins.go, add defaults to `assets/defaults.yaml` |
| Install method | Go package implementing `installer.Installer` | Open a PR: add `internal/installer/<name>/`, add a line to plugins.go |
| Isolated launch method | Go package implementing `launcher.Launcher` | Open a PR: add `internal/launcher/<name>/`, add a line to plugins.go |

Each implementation calls `Register` in its own package's `init()`. `internal/plugins/plugins.go` is the only file that lists every implementation:

```go
package plugins

import (
    _ "github.com/chilogen/jit-rules/internal/backend/jev"
    _ "github.com/chilogen/jit-rules/internal/backend/openai"
    _ "github.com/chilogen/jit-rules/internal/installer/dshpatch"
    _ "github.com/chilogen/jit-rules/internal/installer/hooksjson"
    _ "github.com/chilogen/jit-rules/internal/launcher/claudesettings"
    _ "github.com/chilogen/jit-rules/internal/launcher/codexconfig"
    _ "github.com/chilogen/jit-rules/internal/launcher/dshpatch"
    _ "github.com/chilogen/jit-rules/internal/launcher/manual"
)
```

`backend/fake` is not in plugins.go and is only imported by tests. When pipeline or evaluate tests need the fake, `backend.Register` a fake instance in the test and set the settings' `judge.backend` to its name.

## File paths (all computed by `internal/paths`)

| Name | Linux / macOS | Windows |
|---|---|---|
| `{config}` | `$XDG_CONFIG_HOME/jit-rules`, default `~/.config/jit-rules` | `%APPDATA%\jit-rules` |
| `{state}` | `$XDG_STATE_HOME/jit-rules`, default `~/.local/state/jit-rules` | `%LOCALAPPDATA%\jit-rules\state` |
| `{data}` | `$XDG_DATA_HOME/jit-rules`, default `~/.local/share/jit-rules` | `%LOCALAPPDATA%\jit-rules\data` |
| `{cache}` | `$XDG_CACHE_HOME/jit-rules`, default `~/.cache/jit-rules` | `%LOCALAPPDATA%\jit-rules\cache` |
| Stable path `{bin}` | `~/.local/bin/jitr` | `%LOCALAPPDATA%\jitr\jitr.exe` |

When the environment variable `JITR_HOME` is set, the four directories above move to `$JITR_HOME/{config,state,data,cache}` and `{bin}` moves to `$JITR_HOME/bin/jitr` (plus `.exe` on Windows). Every test must set `JITR_HOME` to `t.TempDir()` and must never read or write real user directories.

Main files:

| File | Path |
|---|---|
| Registry | `{config}/registry.yaml` |
| User-defined harnesses | `{config}/harnesses/*.yaml` |
| DeepSeek's dedicated hooks file | `{config}/harness/deepseek/hooks.json` (given by a path in deepseek.yaml, not hard-coded) |
| Packs from git | `{data}/packs/<pack name>/<commit>/` |
| Temporary directory while fetching a pack | created with `os.MkdirTemp` under `{data}/packs/.tmp/` |
| Session state | `{state}/sessions/<harness>/<session_id>.json` (`/`, `\`, `:` in session_id replaced with `_`) |
| Journal | `{state}/journal/YYYY-MM-DD.jsonl` (UTC date) |
| Internal errors | `{state}/errors.jsonl` |
| Unit test results | `{state}/tests/<global id with / replaced by __>.json` |
| Judge cache | `{cache}/judge/<first two hex digits of sha256>/<sha256>.json` |
| Script judge files | `{cache}/scripts/<sha256 of the script>.<ext>` (ext rule in judge.md) |

**Project root `root`** = the first directory found walking up from cwd that contains `.git`; if none is found, or cwd does not exist, `root` = cwd.

## Code constraints

1. `internal/archtest` uses `go list -deps` to check:
   - except `plugins`, `cli`, and `cmd`, no package imports `internal/backend/<implementation>`, `internal/installer/<implementation>`, or `internal/launcher/<implementation>`;
   - implementations do not import each other.
2. `internal/archtest` also checks: `.go` files under `internal/`, except in `backend/*/`, `installer/*/`, `launcher/*/`, `archtest/`, and `_test.go` files, contain none of the string literals `"jev"`, `"openai"`, `"codex"`, `"trae"`, `"deepseek"`, `"claude-code"`. Differences between harnesses and backends live only in YAML and in the implementations; the default backend is written in `assets/defaults.yaml`. Do not evade this check with string concatenation or similar tricks.
3. Platform-specific files (`_windows.go` / `_unix.go`) may appear only in `paths`, `fileutil`, and `session` (file locks). Anywhere else that needs to distinguish platforms takes `runtime.GOOS` as a parameter (for example the `goos` of `harness.CommandString`), so both platforms can be tested on one machine.
4. Every exported function has a one-line comment. Do not write comments explaining what the code does; write only comments explaining why.
