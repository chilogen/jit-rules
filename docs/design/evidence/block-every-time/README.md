# Evidence: block every violation

Measured on 2026-11-02 during the second design revision; conclusions are in the "Blocking and shell writes" section of [../../verification.md](../../verification.md).

- `hook.py`: the hook used for the experiment. On PreToolUse, if a Write, an apply_patch (Add File), or a shell redirection writes a `.md` file whose content does not start with `# `, it exits 2 and writes the block message to stderr. Written only for the experiment; its recognition is cruder than the runbook's. Its messages are in Chinese, exactly as used in the runs.
- `claude-code/PreToolUse-Write.json`: the payload of Claude Code writing notes.md for a non-conflicting prompt (content starts with `# Notes`, not blocked). Payloads from the two blocked runs were not kept; their results are recorded in verification.md.
- `codex/01-PreToolUse-Bash.json`, `codex/03-PreToolUse-apply_patch.json`: raw Codex payloads.
- `codex/02-PreToolUse-Bash-printf.reconstructed.json`: the call in which Codex created a file with `printf ... >> notes.md`. The original file was accidentally overwritten during the experiment; this copy was reconstructed from the Codex session log of the same session (`~/.codex/sessions/2026/11/02/rollout-2026-11-02T03-28-21-*.jsonl`, lines 21 and 23): fields other than `tool_input` and `tool_use_id` come from another payload of the same turn, and `tool_input.command` and `tool_use_id` come from the command and id of this call in the session log.
- Local absolute paths in all payloads were replaced: the temporary experiment directory is written as `/tmp/exp`, and paths under the home directory as `~/...`. Nothing else was changed.
