# Spec: harness definitions and installation

Each harness is described by a YAML definition file. The four built-in definitions are in `assets/harnesses/`; copy them as they are. To support another harness, the community only writes another such file. `Definition` and related types are in [contracts.md](contracts.md).

Loading order: built-in definitions first, then `{config}/harnesses/*.yaml`; on a name clash the latter overrides the former. Parse with `KnownFields(true)`; unknown fields are errors.

## Detection (`harness.DetectDef(defs, raw)`)

Go through all definitions in ascending `Detect.Order` and return the first that matches:
- `any_fields` written: the payload has at least one of these keys;
- `field_equals` written: every key exists with an equal value (both compared as strings);
- both written: both must hold;
- `fallback: true`: always matches.

Return nil when none matches. The `--harness` argument is accepted and ignored for compatibility with installed hook commands. Detection uses only the payload; the journal records the detected name in `event.harness`.

## Decoding (`harness.Decode(def, raw, now)`)

1. Parse the JSON, read `hook_event_name`, and look up its kind in `def.Events`; if not found, return `ErrIgnored`. `HookEvent` = `hook_event_name`.
2. If the kind is `session_start` and the payload's `source` is in `CompactSources`, change the kind to `compact`.
3. Common fields: `Harness` = def.Name, `session_id`, `cwd`, `prompt`, `TS` = now; `Root` = `paths.ProjectRoot(cwd)`.
4. If the kind is `stop`: `Content` = `last_assistant_message`, `StopActive` = `stop_hook_active` (false when absent or not a boolean).
   If the kind is tool or tool_result: `RawTool` = `tool_name`; use `Extract` (with `exists` implemented by `os.Stat`) to get Tool, Effect, Paths, Content, ContentUnknown, Shell; store `tool_input` unchanged in `Input`.
5. Compute Size and DedupeKey (see event.md).

## Encoding (`harness.Encode(def, ev, res)`)

| res.Action | Output |
|---|---|
| `none` with no Notice | Exit 0, Stdout and Stderr both empty |
| `inject` | Exit 0, Stdout = `{"hookSpecificOutput":{"hookEventName":"<ev.HookEvent>","additionalContext":"<res.Text>"}}` |
| `block` | Exit 2, Stderr = res.Text, Stdout empty |
| `inject` when ev.Kind is `stop` | Exit 0, Stdout = `{"decision":"block","reason":"<res.Text>"}` |

Reason for the `stop` row: the turn-end event has no field for injecting context; the only way to put text in front of the AI is to refuse the end, and the harness hands `reason` to the AI as a new instruction. The JSON form is the one measured on Claude Code and Codex, and keeps exit 2 reserved for block.

When `res.Notice` is non-empty, `def.Supports.SystemMessage` is true, and Action is not block, add `"systemMessage": "<Notice>"` at the top level of the Stdout JSON (when Action is none, Stdout is just `{"systemMessage": ...}`). The JSON is produced by `encoding/json`, never by concatenating strings by hand.

## Installation

### Common flow (`cli install` / `cli uninstall`)

1. Make sure jitr is at the stable path `{bin}`: do nothing if the current executable is already at `{bin}`; otherwise copy it there with `fileutil.ReplaceExecutable`. Skipped on uninstall.
2. Read the definition, find the Installer for `Install.Kind`, and call `Plan` to get a list of Changes.
3. Print each Change's path and a unified diff (a simple line-based diff of your own is enough). With `--dry-run`, stop here.
4. `installer.Apply(changes, now)`.
5. On install, print the definition's `Notes`.

**Command string** (`harness.CommandString`): use `Command.Windows` when `goos` is `windows`, otherwise `Command.Unix`; replace `{bin}` with the stable path and `{name}` with the definition name. This string must be exactly the same on every install, because Codex binds its trust in a hook to the hook definition.

**Path placeholders** (expanded with `paths.Expand` inside `installer.TargetPaths`): `~` is the user's home directory, `{config}` is as in layout.md, and `{env:NAME:default}` means the value of environment variable NAME when non-empty, otherwise the default (which may itself contain `~`). Paths in the project scope are relative to `Root`.

**Target.Mode**:
- `first`: write only `Paths[0]`, creating parent directories as needed;
- `existing`: write every path whose parent directory already exists; if none exists, the error is "没有找到 <Title> 的配置目录，请先安装并启动一次 <Title>" ("no configuration directory for <Title> found; install and start <Title> once first").

With `--scope project`, if the definition has no `targets.project`, the error is "<Title> 不支持项目级安装" ("<Title> does not support project-level installation").

### hooksjson Installer (kind: `hooks-json`)

The target file is a JSON object. If the file does not exist, start from a copy of `FileDefaults`; if it exists but is not a valid JSON object, fail without writing.

**Hook entries** (`harness.HookEntries`): for each native event name E in `def.Events`, generate one item:
- `PreToolUse`, `PostToolUse`: `{"matcher":"*","hooks":[H]}`
- other events: `{"hooks":[H]}`
- where H = `{"type":"command","command":<command string>,"timeout":<TimeoutSeconds>}`, merged with the fields in `HandlerExtra[E]`.

**Install**:
1. First run the uninstall logic to remove old entries;
2. Append each event's entries from `HookEntries` to the end of the array `obj["hooks"][E]`;
3. For each key in FileDefaults, add it if the file does not have it yet.

**Uninstall**: go through every group in every event array under `obj["hooks"]` and remove handlers whose `command` contains both `jitr` and ` hook --harness `; remove a group whose `hooks` becomes empty; remove an event key whose array becomes empty; remove the `hooks` key when the `hooks` object becomes empty. Finally, if every remaining top-level key is in `FileDefaults` with an equal value (including `{}` with no keys left), the file was created by jitr's install, so delete it (`Change.After = nil`) instead of leaving an empty shell.

**Installed**: true when some file in `TargetPaths` exists, is valid JSON, and contains a handler meeting the uninstall condition above. In `existing` mode, when no parent directory exists, return false without an error.

**Output format**: keep the original order of keys (write your own ordered JSON object type, parse with `json.Decoder.Token`, and call `Decoder.UseNumber()` so numbers are kept verbatim instead of becoming float64), indent with 2 spaces, and end the file with a newline. Encoding must use `json.Encoder` with `SetEscapeHTML(false)`: by default Go writes `<`, `>`, `&` in strings as `<`, `>`, `&`, which mangles the `2>/dev/null` and `&&` common in user configuration (the first implementation did exactly this to real configuration). **Acceptance requirement**: for a file containing other hooks, install followed by uninstall must give a file byte-identical to the original (when the original uses 2-space indentation and ends with a newline).

### dshpatch Installer (kind: `dsh-patch`)

Produces two Changes:
1. The hooks.json given by `Targets.user`: the same content as a hooks-json install (starting from an empty object); deleted on uninstall;
2. `Options.patch_file` (with placeholders expanded): a YAML file whose content is a list.
   - Install: in the list, find the item in an `insert` array whose `id == Options.entry_id`; replace it entirely if found, otherwise append to the end of the list:
     ```yaml
     - insert:
         - id: <entry_id>
           name: '<plugin>'
           config:
             configPath: <absolute path of file 1>
     ```
   - Uninstall: delete that item; if its `insert` array becomes empty, delete the whole list element.
   - Modify through the yaml.v3 Node API, without touching other content or comments in the file.
   - If the file does not exist, start from an empty list.

**Installed**: the patch file exists and has an insert item with `id == Options.entry_id`.

## Isolated launch (`jitr isolate`)

Purpose: for evaluation, disable the harness's own memory, skills, and instruction files (CLAUDE.md, AGENTS.md, etc.) so that only jitr's influence remains (or disable jitr too, as the control group).

### Common flow (`cli isolate`)

1. Read the definition's `Isolate`, find the Launcher for its `Kind`, and call `Prepare` in a temporary directory created with `os.MkdirTemp`.
2. Print `Notes`.
3. Start `Launch.Program` with `Launch.Args` through `exec.Command`, inheriting stdin, stdout, and stderr; the environment is the current one with `Launch.Env` applied first (same names override), then:
   - always append `JITR_ISOLATED=1`;
   - with `--no-jitr`, also append `JITR_DISABLE=1`.
4. Wait for the process to exit, delete the temporary directory, and exit with its exit code (through `cli.Silent(code)`).

**Where arguments are inserted** (`launcher.MergeArgs`): if the first user argument is in `Subcommands`, the isolation arguments go right after it; otherwise they go first. That is, the final arguments = `[subcommand] + Args + SubcommandArgs[subcommand] + arguments generated by the Launcher + the remaining user arguments`.

### Launchers

**`claude-settings`**: write `settings.json` into the temporary directory with a copy of `Settings`; without `--no-jitr`, add a `hooks` key (the result of `HookEntries`). The generated arguments are `["--settings", <path of this file>]`.

**`codex-config`**: without `--no-jitr`, for each native event E in `Events` (sorted by event name), generate two arguments, `-c` and `hooks.E=[{...}]`. The value is inline TOML:
- PreToolUse, PostToolUse: `[{matcher="*",hooks=[{type="command",command="<command>",timeout=<TimeoutSeconds>}]}]`
- other events: `[{hooks=[{type="command",command="<command>",timeout=<TimeoutSeconds>}]}]`
- `<command>` is escaped by TOML basic string rules: `\` → `\\`, `"` → `\"`.
- Fields in HandlerExtra[E] are also added to the handler (numbers written directly, strings quoted), sorted by key after timeout.

With `--no-jitr`, these arguments are not generated.

In addition (with or without `--no-jitr`): create an empty directory `<TempDir>/home`; get the user's real harness home with `paths.Expand(Options.home, "")`, and copy the files listed in `Options.home_keep` from it (regular files only, skipped if missing), keeping permissions; set `Launch.Env` to `{Options.home_env: <TempDir>/home}`. Reason: Codex 0.163 was measured to still read and run the user hooks in `$CODEX_HOME/hooks.json` even with `--ignore-user-config`, so an installed jitr user hook ran again alongside the hooks generated for isolation (evidence: `docs/runbook/ACCEPTANCE.md` at commit `3f23dc6` of the first implementation, and the comparison after switching to a temporary home in `docs/runbook/evidence/codex-isolation-markers.txt` at commit `8097cfd`; those commits are not kept in this repository's history). Only the credentials file is copied, so the user's hooks, config, and skills are never loaded, and the user's files never need to be changed or temporarily replaced.

**`dsh-patch`**: write into the temporary directory:
1. Without `--no-jitr`, a `hooks.json` with the same content as a hooks-json install (starting from an empty object);
2. An `isolate.patch.yml` whose content is a list:
   - one item `{id: <id>, disabled: true}` for each id in `Options.disable_entries`;
   - read `$DSH_HOME/cordis.patch.yml` and `$DSH_HOME/profiles/*/cordis.patch.yml` (`~/.dsh` when `DSH_HOME` is empty; skip missing files), collect every `insert` item id starting with any prefix in `Options.disable_entry_prefixes`, deduplicate, and add one item `{id: <id>, disabled: true}` for each;
   - without `--no-jitr`, append one more item `{insert: [{id: <install.options.entry_id>, name: '<install.options.plugin>', config: {configPath: <path of file 1>}}]}`.

The generated arguments are `["--patch", <path of isolate.patch.yml>]`.

**`manual`**: starts no program; prints Notes and ends with exit code 1 (`cli.Silent(1)`).
