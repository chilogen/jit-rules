# Spec: command line

## Common rules

- Every command supports `--json` (a persistent flag on the root command): it prints exactly one JSON object `{"ok": bool, "data": ..., "errors": [...], "warnings": [...]}` and nothing else (`errors` and `warnings` are string arrays, `[]` when empty). Without `--json`, output is human-readable text in Chinese: normal command output goes to stdout, while errors and warnings go to stderr.
- **Exit codes**: 0 for success, 1 for user input errors, 3 for internal errors. `jitr hook` is the exception, see pipeline.md.
- **Exit codes and error output are all handled by `cli.Execute`** (implemented completely in the shared contracts task; afterwards each command just returns errors by the conventions below):
  - The root command sets `SilenceErrors` and `SilenceUsage`; Execute prints errors.
  - A command returns nil → 0.
  - Returns `cli.Silent(code)` → print nothing more, exit with code. For "the result has been printed but the exit code must be non-zero", for example `unit test` with failing examples.
  - Returns `cli.Internal(err)` (or an error wrapping it) → print the error, exit 3. For environment problems the user cannot fix by changing input: failed file reads or writes, git failures, network failures.
  - The error chain (checked with `errors.As`) contains any of `*fs.PathError`, `*os.LinkError`, `*os.SyscallError`, `*exec.Error`, `*exec.ExitError`, or `*url.Error` → print the error, exit 3. **Exception**: when the chain satisfies `errors.Is(err, fs.ErrNotExist)`, it does not count and falls to the next rule (a missing file usually means the user gave a wrong path). Reason: packages such as pack, profile, and installer cannot import cli (cli imports them), so they cannot mark errors as Internal themselves; with this rule they get the right exit code just by wrapping the underlying error with `%w`.
  - Any other error (including cobra's own argument errors, unknown commands, unknown flags) → print the error, exit 1.
  - **Every package** wraps underlying errors with `fmt.Errorf("...: %w", err)`, never `%v` or a fresh `errors.New`, otherwise the previous rule cannot see them.
  - cobra's built-in parts must follow these rules too (written once in `Execute` by the shared contracts task): disable the generated `completion` command (`CompletionOptions.DisableDefaultCmd = true`); the `help` command and `-h/--help` with `--json` print `Output(cmd, true, {"help": <help text>}, ...)`, and print normally without it; before running, `Execute` walks the whole command tree and gives every command that has subcommands but no RunE (such as `profile`, `pack`, `unit`, registered by later tasks) `Args: cobra.NoArgs` and a RunE that calls `cmd.Help()`, so that `jitr pack foo` is an argument error (exit 1) rather than printing help and returning 0. Tasks registering parent commands need not care about this.
  - When printing an error: if `--json` or `--json=true` appears on the command line (judged from the raw arguments even when parsing failed) → print JSON with `Output(cmd, false, nil, "", []error{err}, nil)`; otherwise write the error to stderr.
- **Unit id arguments**: accept a global id (`me/git.commit-zh`); also accept a short id (`git.commit-zh`) when exactly one unit with that short id is in the `States` of `Resolve` for the current cwd.
- **`<profile>` arguments**: accept the name or directory path of a registered profile; when omitted, use the highest-ranked profile for the current cwd; when there is none, it is an error telling the user to run `jitr profile init` first.
- **Commands that change a profile** call `profile.Commit` afterwards and put the returned warning into warnings.

## Commands

### Installation

| Command | Behavior |
|---|---|
| `jitr install <harness>... [--scope user\|project] [--dry-run]` | Install following harness.md. `all` means every definition. The default scope is user |
| `jitr uninstall <harness>... [--scope ...] [--dry-run]` | Uninstall |
| `jitr upgrade <path to new binary>` | `fileutil.ReplaceExecutable(<path>, {bin})`. Changes no harness configuration |
| `jitr purge [--yes]` | Delete `{config}`, `{state}`, `{data}`, `{cache}`. Without `--yes`, confirm in the terminal first; `--json` without `--yes` is an error |

### profile

| Command | Behavior |
|---|---|
| `jitr profile init <dir> --name <name>` | Require a `.jitr` directory, then `profile.Init` and register (appended to the end of the registry). An error if `<dir>` already has profile.yaml or is already registered |
| `jitr profile register <dir>` | Require a `.jitr` directory and validate the profile (profile.yaml loads; units active in the lock and units in every active pack pass Validate), and only then write it into the registry. An error if already registered. Scope is derived from the path as in profile-pack.md; there is no `--scope` flag |
| `jitr profile unregister <name\|dir>` | Remove from the registry. Does not delete the directory |
| `jitr profile list` | List every registered profile: name, path, derived scope (`~` for the global profile, the parent directory for a project profile), number of active units, whether it applies to the current cwd. With `--json`, data is an array of `{"name","path","scope","units","applies"}` |
| `jitr profile show [<profile>]` | Print `unit.Describe` for each active unit (threshold from `Settings.ThresholdOf(<the unit's backend>)`, the unit's backend being `judge.backend` or else `settings.judge.backend`), separated by blank lines |

### Packs

| Command | Behavior |
|---|---|
| `jitr pack add <source> [--ref <ref>] [--profile <p>] [--include <glob>]... [--exclude <glob>]... [--force]` | Fetch (profile-pack.md) → `pack.Validate` and Validate on each unit → an error if the pack is named core or is already in the profile → run `evaluate.Test` on every unit in the pack (automatically skipped without a key) → conflict check for the new units → `profile.AddPack` appends to the end of `packs`, and the lock gets `{source, ref, commit, hash, added_at}` → print each unit's plain-language description. If tests fail or error, or there is a confirmed conflict, report it and write nothing, unless `--force` is given (then they become warnings) |
| `jitr pack remove <name> [--profile <p>]` | `profile.RemovePack`, and delete the lock record. Does not delete files under `{data}`. Allowed for core too |
| `jitr pack update [<name>] [--ref <ref>] [--profile <p>] [--force]` | For the given pack (all non-builtin packs when name is omitted): git sources are fetched again with the new ref (the original ref if none is given); local sources get their hash recomputed; the remaining checks are as for add; on success the lock is updated (with `--ref`, also `profile.SetPackRef`). Builtin packs are skipped |
| `jitr pack list [--profile <p>]` | List pack name, source, ref, commit (first 12 characters), number of units, and status (`ok` / `modified` / `missing`) |

### Units (the commands the AI mainly uses)

| Command | Behavior |
|---|---|
| `jitr unit list [--profile <p>] [--all]` | List the `States` of `Resolve`: global id, title, status, file path. Without `--all`, only `active`; with it, everything. With `--json`, data is an array of `{"id","title","status","file","profile","source","err"}` |
| `jitr unit new <profile> <short id> [--from-stdin]` | Create `<profile dir>/units/<short id>.md`. An error if the file exists. Without `--from-stdin`, write the embedded `assets/unit-template.md`. Afterwards run Parse and Validate; on errors, delete the file, print the errors, exit 1 |
| `jitr unit test <id> [--offline]` | Run `evaluate.Test`, `SaveTest`, and print a table of each example's result; exit 1 if anything fails or errors |
| `jitr unit replay <id> [--since 14d] [--live]` | Run `evaluate.Replay`. Empty history succeeds with an empty report; journal read or parse failures exit 3 (with `--json`, `ok: false` and the error in `errors`), without a success report. `--since` accepts the suffixes `d` (days), `h`, `m` |
| `jitr unit check <id>` | Run `evaluate.Conflicts` against all currently active units; exit 1 on a confirmed conflict |
| `jitr unit show <id>` | Print `unit.Describe` (threshold as in `profile show`), followed by a line with the status and the latest test result (noting "测试结果已过期", "test result is stale", when its hash is not the current hash) |
| `jitr unit enable <id>` | For a profile's own unit, require in order: Validate has no errors; the latest test result's hash equals the current hash, with no fail or error (print a warning when some are skipped); no confirmed conflict. When all hold, write to the lock (`tests` holds the test counts). Otherwise say which step failed and which command to run. For a pack unit: if it is in the profile's `disable` list, `profile.RemoveDisable`; otherwise say it is already in effect |
| `jitr unit disable <id>` | For a profile's own unit: delete it from the lock. For a pack unit: `profile.AddDisable` |
| `jitr unit rm <id>` | Only for a profile's own units: delete the file and its lock entry |

Statuses: `active` in effect; `modified` in the lock but the hash differs; `draft` not in the lock; `disabled` in the disable list; `replaced` replaced by another unit's replaces; `shadowed` the same id also exists in a higher-ranked profile; `invalid` the file cannot be parsed or fails validation.

### Diagnostics

| Command | Behavior |
|---|---|
| `jitr status` | Prints: whether each harness is installed (calling `Installer.Installed` for both the user and project scopes, with the project Root being the project root of the current cwd); an `Installed` error is listed under that harness and does not stop checks for later harnesses or scopes; for each installed harness, whether the journal of the last 24 hours has events whose `event.harness` equals it, and if installed with no events, the note "已安装但最近没有收到事件" ("installed but no events received recently") plus the notes in that harness's definition (no harness-specific logic); registered profiles and number of active units; the Errors of `Resolve`; the number of lines in errors.jsonl over the last 24 hours and the last 3; for each model backend used by an active unit with a question judge (its `judge.backend`, or else `settings.judge.backend`), whether the environment variable named by its `api_key_env` is set; for each interpreter used by an active script judge, whether it is found in PATH |
| `jitr check` | For all active units: Validate plus pairwise conflict checks |
| `jitr explain [--last N \| --event <file>]` | Take one event: `--event` reads a raw payload file and decodes it with DetectDef + Decode; `--last N` (default 1) takes the event of the N-th most recent journal record whose kind is prompt, tool, tool_result, or stop and whose `err` is not `dedupe`. Then call `pipeline.Explain` and print, unit by unit: the gate result (and which field failed, if any), the judgment, NoBlock, the skip reason, and finally the action and the delivered text. It uses the current rules, which may differ from those at the time; the output says so |
| `jitr guide` | Print the embedded guide.md verbatim |
| `jitr version` | Print `Version` and `Commit` |

### Evaluation

| Command | Behavior |
|---|---|
| `jitr isolate <harness> [--no-jitr] -- <harness args>...` | Launch the harness in isolated mode: its own memory, skills, and instruction files are disabled. Without `--no-jitr`, only jitr remains; with it, jitr is disabled too (control group). The harness name must come before `--`, and everything after `--` is passed to the harness unchanged. Details are in the "Isolated launch" section of harness.md. The child process uses the terminal directly and its output cannot be wrapped in one JSON object, so `--json` is rejected as a user error (exit 1, printing one JSON object per the common rules) and nothing is started |

### Hook entry

| Command | Behavior |
|---|---|
| `jitr hook [--harness <name>]` | Detect the harness from stdin; `--harness` is accepted and ignored for compatibility with installed hooks. See pipeline.md |
