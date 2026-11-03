# Verification record

> Verified 2026-11-02. The section "Blocking and shell writes" at the end was measured during the second design revision. Every claim the design makes about an external system is registered here with its source. Evidence comes in four kinds: **measured** (run for real on the author's machine), **source** (read the other project's code), **docs** (official documentation), and **third party** (another project's implementation).

## Summary

| Item | Conclusion | Evidence |
|---|---|---|
| Claude Code: does exit-2 stderr enter the model context, and does the model rewrite according to the rule? | Yes. After the first Write was blocked, the model changed the content to uppercase as stderr's rule required, and the second Write went through | Measured: [evidence/payloads/claude-code/](evidence/payloads/claude-code/) |
| Claude Code: does `additionalContext` on UserPromptSubmit take effect? | Yes. Following the injected rule, the model ended its reply with `JITR-OK` | Measured |
| Claude Code: payload fields | Has `session_id`, `transcript_path`, `cwd`, `prompt_id`, `permission_mode`, `tool_use_id`; Write/Edit use `file_path`; paths are **absolute** | Measured |
| Codex: exit 2 and injection | Same as Claude Code; both work | Measured: [evidence/payloads/codex/](evidence/payloads/codex/) |
| Codex: payload fields | Every event has `model` (SessionStart too); events within a turn have `turn_id`; no `prompt_id`; the apply_patch patch is in `tool_input.command`, with absolute paths in the patch headers | Measured |
| Codex: hook trust | Untrusted hooks are **silently skipped**. Trust is bound to the hash of the hook definition; changing the definition requires trusting it again | Measured (without `--dangerously-bypass-hook-trust`, no hook fired) + docs |
| Codex: hooks.json and `[hooks]` in the same layer | They are merged with a warning at startup; hooks from every layer run, none override each other | Docs |
| Does `tool_use_id` change on a retry after a block? | Yes, on both Claude Code and Codex, so deduplicating by call id never mistakes a retry for a duplicate | Measured |
| TRAE: configuration paths | International edition `~/.trae/hooks.json`, CN edition `~/.trae-cn/hooks.json`, project level `<root>/.trae/hooks.json` | Docs (CN edition) + third party ([OpenViking](https://github.com/volcengine/OpenViking)'s installer writes both paths) |
| TRAE: `systemMessage` support | No. The only common output fields are `continue` and `stopReason` | Docs |
| TRAE: exit-2 behavior | On PreToolUse it is equivalent to deny, and stderr enters the model context as the reason | Docs |
| TRAE: `tool_input` field names for Write/Edit | **Could not be confirmed.** The docs list only tool names; no runnable environment or public capture was found. The design therefore does not depend on specific field names: all common names are accepted (runbook/spec/event.md) | — |
| DeepSeek: hook bridge | `@deepseek-ai/dsh-hooks-claude-code` ships with the CLI; the `configPath` option is read once at process start; only user-level configuration is supported, per-project hook discovery is still a TODO | Source |
| DeepSeek: payload | `transcript_path` is always `''`; `cwd` is the session's current directory; `tool_response` is concatenated plain text | Source |
| DeepSeek: native tool names and fields | `bash`/`pwsh` (`command`), `write` (`file_path`, `content`), `edit` (`file_path`, `new_string`), `read` (`file_path`), `apply_patch` (**`input`**, not `command`), `str_replace_editor` (`command`, `path`, `file_text`, `new_str`), `glob`, `grep`, `mcp__<server>__<tool>` | Source |
| DeepSeek: where plugin configuration lives | The user-level patch layer is `$DSH_HOME/cordis.patch.yml` (`DSH_HOME` defaults to `~/.dsh`) and applies to every DSH profile. The format is a list of patches, each `- insert: [{id, name, config}]` | Source + official docs |
| JEV: can question keys contain `:` and `/`? | Yes, returned unchanged | Measured: [evidence/jev/](evidence/jev/) |
| JEV: conditions written in Chinese | Same result as the English version (both 0.98); an unrelated condition got 0.09 | Measured (one sample only) |
| JEV: latency | Inference itself is fast (around 50 ms as measured by the user). The 1–6 s measured on the author's machine was mostly the machine's outbound network: the TLS handshake alone took 1–4 s, and even the inference-free `/v1/models` endpoint took 1.2–3.1 s. The resulting design constraint: every hook is a new process that redoes the TLS handshake, and on a poor network the handshake costs far more than inference | Measured (re-measured and corrected 2026-11-02: first recorded as "about 2 s each", which mistook network time for JEV time) |
| JEV: aliases move | The docs say `jev-latest` → 1.13.0; the actual response was `jev-1.14.0` | Measured |
| GPT-6 Luna | Model id `gpt-6-luna`; supports both Chat Completions and Responses; supports Structured Outputs; `reasoning_effort` defaults to medium and can be set to none | Docs: [model page](https://developers.openai.com/api/docs/models/gpt-6-luna.md) |
| GPT-6 Luna latency and real behavior of strict schemas | **Not measured**; no `OPENAI_API_KEY` on the machine | — |
| Claude Code: isolated launch | `--setting-sources "" --disable-slash-commands --settings <file with only hooks and autoMemoryEnabled:false>` disables the project CLAUDE.md, auto memory, and skills together, while hooks in `--settings` still run. Neither `--bare` nor `--safe-mode` works: both also disable hooks, and `--bare` only accepts API-key login | Measured (MDMARK, MEMMARK, skill list, each compared with a non-isolated run) |
| Codex: isolated launch | `exec --ignore-user-config -c project_doc_max_bytes=0 -c memories.use_memories=false -c memories.generate_memories=false -c skills.include_instructions=false -c skills.bundled.enabled=false`, plus hooks passed inline with `-c hooks.*`: AGENTS.md and skills are disabled while hooks still run. Only the exec subcommand has `--ignore-user-config`. **Correction (2026-11-02)**: the user's own hooks were not tested here; acceptance of the first implementation found that Codex 0.163 still runs hooks in `$CODEX_HOME/hooks.json` with `--ignore-user-config`, so isolation now uses a temporary `CODEX_HOME` (see runbook/spec/harness.md) | Measured (MDMARK, skill list, each compared with a non-isolated run) + source (configuration struct definitions) |
| DeepSeek: isolated launch | An entry can be disabled by writing `{id, disabled: true}` in a `--patch` file; the entry ids for instructions and skills are `agent-instructions`, `skill`, `skill-filesystem`, `tool-skill`; there is no built-in memory, memory comes from memory-* MCP entries inserted in the user patch | Source (not run) |
| TRAE: isolated launch | A GUI IDE; no command-line method found | — |

## Blocking and shell writes (measured 2026-11-02 during the second revision)

Method: a hook script that checks "the first line of a Markdown file must start with `# `", exits 2 on every violation, and uses the same block message as the new runbook ([evidence/block-every-time/](evidence/block-every-time/)). Claude Code got the hook through `--setting-sources "" --settings <file>`, Codex through `exec --dangerously-bypass-hook-trust -c hooks.PreToolUse=...`; both ran in temporary directories without touching user configuration. Local Claude Code, Codex 0.166.0.

| Item | Conclusion | Evidence |
|---|---|---|
| When the user's request conflicts with the rule ("create notes.md whose first line is Shopping list:"), what happens after a block? | Neither Claude Code (Write) nor Codex (apply_patch) retried unchanged or tried another way around; both stopped, explained the conflict to the user, and offered options. The file was not created | Measured (once each) |
| When the rule can be satisfied, what happens after a block? | Claude Code fixes the content according to the rule in stderr and writes again; the second write goes through | Measured (with the uppercase rule from the first verification; in this run, Claude Code wrote a heading right away for the non-conflicting prompt and was never blocked) |
| Which tools does Codex use to write files? | Both: apply_patch for creating todo.md; `printf '%s\n' '...' >> notes.md` for "note a sentence in notes.md", where the file did not exist before. That shell command went through the PreToolUse hook with tool name `Bash` | Measured; payload in evidence (this one was reconstructed from the Codex session log, see the evidence README) |
| Do file-writing tools alone cover every write? | No. The `printf ... >>` above goes through no file-writing tool | Same as above |
| Is the shell write recognition test table correct? | All 43 cases agree with a reference implementation written from event.md; 22 of them, which have known content and write a single file, were executed with real bash and produced files matching the expected content (one more writes to the absolute path `/work/proj`, which does not exist on the machine, and was not executed) | Measured (the reference implementation and checking script are not in the repository; they can be rewritten from event.md) |

## Turn-end reminders (measured 2026-11-04)

Method: first a logging `Stop` hook (Claude Code through `--setting-sources "" --settings <file>`, Codex through a temporary `CODEX_HOME` with `--dangerously-bypass-hook-trust`), then the implemented `stop` event through `jitr isolate` with one project unit "改了文件就留档" (inject on `stop`, script judge `[ -n "$(git status --porcelain)" ]`). Local Claude Code (haiku), Codex 0.166.0; temporary repositories and `JITR_HOME`.

| Item | Conclusion | Evidence |
|---|---|---|
| Does `Stop` fire once per turn? | Yes on both, for plain replies and for turns with tool calls | Measured (3 runs each) |
| What does the payload carry? | `last_assistant_message` (final text) and `stop_hook_active`; Claude Code adds `prompt_id`, Codex `turn_id`. TRAE documents the same two fields plus `loop_count` | Measured; TRAE docs |
| Does `{"decision":"block","reason":...}` keep the AI working? | Yes on both: the AI follows `reason`, and the next `Stop` of the same turn has `stop_hook_active: true` and the same turn id | Measured (once each) |
| Does the reminder work end to end? | Edit task: reminded once, the AI appended to `docs/log/2026-11-04.md` itself, the re-sent end was skipped (`redeliver`). Pure question on a clean tree: script exit 1, nothing delivered. Same on both harnesses; no internal errors | Measured (once each) |
