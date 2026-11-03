# Hook pipeline and real-harness smoke test (joins after packs and profiles, harness definitions, model judgment, and runtime)

**Read first**: all of spec/pipeline.md and the pipeline section of spec/contracts.md.

**Change only**: `internal/pipeline/`, `internal/cli/hook.go`, `cmd/jitr/main.go` (only the recover part), `testdata/profiles/`, `docs/runbook/ACCEPTANCE.md` (only the "Smoke test" section).

## Steps

1. `internal/pipeline.Run` and `Explain`: following the steps in pipeline.md, wire harness, profile, gate, judge, deliver, session, and journal together. Each step is its own function.
2. `internal/cli/hook.go`: `jitr hook` (`DisableFlagParsing`, see the iron rule in pipeline.md).
3. `cmd/jitr/main.go`: call `journal.LogError` in recover.
4. Once all unit tests pass, run the **real-harness smoke test** below.

## Required tests (use the real payloads in testdata/payloads; judge uses the fake backend)

Prepare a test profile `testdata/profiles/basic/` with three units, all already in the lock (with the right hashes) and all passing Validate:
- `basic/txt-inject`: events [tool], paths ["**/*.txt"], inject;
- `basic/txt-block`: events [tool], tools [write, edit], paths ["**/*.txt"], content ['hello'], block; examples include at least a hit (write note.txt with content `hello world`) and a miss (write note.txt with content `HELLO WORLD`);
- `basic/prompt-all`: events [prompt], prompt ['.'], inject.

Test cases:
1. claude-code `prompt` → Action inject; in the stdout JSON, `hookEventName` is `UserPromptSubmit` and `additionalContext` contains `basic/prompt-all`.
2. claude-code `pre_write` (writes note.txt with content `hello world`) → Exit 2; stderr starts with the block introduction and contains both units' bodies.
3. **Blocking applies every time**: immediately feed the same write with another tool_use_id → still Exit 2; stderr contains `basic/txt-block` and not `basic/txt-inject` (it is within the redelivery window).
4. **Allowed after the fix**: another tool_use_id with content changed to `HELLO WORLD` → Exit 0 with empty output (txt-block does not hit, txt-inject is within the window).
5. Set `redeliver_bytes` to 10, feed a `post_edit` (Size greater than 10), then feed step 4's write again → Action inject, containing `basic/txt-inject`.
6. The same payload fed twice → the second output is empty, and the journal's `err` for that call is `dedupe`.
7. After `session_compact`, an inject unit delivered earlier can be delivered again immediately.
8. Give two units other than txt-inject and prompt-all the same topic (in a separate test profile) → only the higher-ranked one is delivered; the other's skip is `topic`.
9. `max_inject_chars: 10` → the second unit is marked truncated.
10. **Shell writes**: with `testdata/smoke/profile` registered (not basic): codex `pre_shell_write` → Exit 2, stderr contains `smoke/md.first-line-h1`; claude-code `pre_shell_heredoc` → Exit 2; codex `pre_apply_patch` (writes note.txt) → empty output.
11. **Unknown content does not block**: with the smoke profile registered, build a codex Bash payload whose command is `echo "$X" > a.md` → Action inject (not block), and this unit's `no_block` in the journal is true.
12. **Judge fallback**: a block unit with a judge, the fake backend times out → Action inject, not block.
13. **Judge not configured**: an inject unit with a judge, the fake backend returns an error wrapping `backend.ErrNotConfigured` → the unit is not delivered; the first call's stdout has a `systemMessage` saying judge is not configured; the second call in the same session has no `systemMessage`.
14. Failure fallback: stdin is `not json`, registry.yaml is corrupt, profile.yaml is corrupt; in all three cases Exit 0, no stderr, and one more line in errors.jsonl. (When profile.yaml is corrupt, other profiles still work, and the first call's stdout has a systemMessage.)
15. Concurrency: 20 goroutines call Run on the same session at once (each with a different tool_use_id); no panic, and the state file is still valid JSON.
16. For a unit that passes the gate, its journal record matches the format in pipeline.md (every field present, `judge.conn_ms` equals the fake's configured value).
17. With `JITR_DISABLE=1`, any payload gives empty output and nothing is written to the journal.
18. With `JITR_ISOLATED=1`, `isolated` in the journal is true.
19. The `prompt` fixture of all four harnesses goes through the whole flow and gets inject.
20. `Explain`: for step 2's event, txt-block passes in the returned Gates and prompt-all is absent (wrong kind); the session state file and the journal are unchanged by the call.
21. `jitr hook --unknown-flag` and `jitr hook` (without `--harness`) both process stdin normally and never exit with 1.
22. Performance: with 100 gate-only units, Run averages under 5ms (`testing.B`; record the result, not a failure condition).
23. PostToolUse: register an enabled unit with `events: [tool_result]`; the matching pre-tool event does not suppress its post-tool injection. Repeating the post-tool payload gives `err: dedupe` and leaves session bytes unchanged. A post-tool block uses the result-repair introduction and the harness's block encoding.

## Real-harness smoke test

Purpose: before writing the unit commands and release tasks, confirm the core behavior on real Claude Code and Codex. The previous implementation touched a real harness only at final acceptance, so the core problems were found too late.

Requires Claude Code (`claude`) and Codex (`codex`) to be logged in on the machine. If one is missing, write "not testable" and the reason in ACCEPTANCE.md and do the other.

Setup (run at the repository root; everything uses temporary directories and never touches the user's own configuration):

```bash
make build
BIN="$(pwd)/bin/jitr"
SMOKE="$(mktemp -d)"
export JITR_HOME="$SMOKE/jitr"
cp -R testdata/smoke/profile "$SMOKE/.jitr"
mkdir -p "$JITR_HOME/config" "$SMOKE/cc" "$SMOKE/cx"
printf 'version: 1\nprofiles:\n  - path: %s\n' "$SMOKE/.jitr" > "$JITR_HOME/config/registry.yaml"
HOOK="JITR_HOME='$JITR_HOME' '$BIN' hook --harness"
cat > "$SMOKE/cc-settings.json" <<EOF
{"hooks": {
  "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "$HOOK claude-code", "timeout": 10}]}],
  "PreToolUse": [{"matcher": "*", "hooks": [{"type": "command", "command": "$HOOK claude-code", "timeout": 10}]}]
}}
EOF
(cd "$SMOKE/cc" && git init -q) ; (cd "$SMOKE/cx" && git init -q)
```

Run (each command once, in its directory):

```bash
P1="Create a file notes.md whose first line is 'Shopping list:' followed by the items milk, eggs, bread, one per line."
P2="Write a short note to notes2.md: buy milk and eggs tomorrow."

cd "$SMOKE/cc"
claude -p "$P1" --setting-sources "" --settings "$SMOKE/cc-settings.json" --permission-mode acceptEdits < /dev/null
claude -p "$P2" --setting-sources "" --settings "$SMOKE/cc-settings.json" --permission-mode acceptEdits < /dev/null

cd "$SMOKE/cx"
CX_HOOKS=(-c "hooks.PreToolUse=[{matcher=\"*\",hooks=[{type=\"command\",command=\"$HOOK codex\",timeout=10}]}]"
          -c "hooks.UserPromptSubmit=[{hooks=[{type=\"command\",command=\"$HOOK codex\",timeout=10}]}]")
codex exec --skip-git-repo-check --dangerously-bypass-hook-trust -s workspace-write "${CX_HOOKS[@]}" "$P1" < /dev/null
codex exec --skip-git-repo-check --dangerously-bypass-hook-trust -s workspace-write "${CX_HOOKS[@]}" "$P2" < /dev/null
```

Checks (go through the journal under `$JITR_HOME/state/journal/` record by record):

1. Each harness's P1 produced at least one record with kind tool and paths containing `notes.md`, and at least one of them has `result.action` `block`.
2. Every record meeting all of these must have `result.action` `block`: kind is tool; `tool` or `effect` is `write`; paths contain some `.md` file; `content_unknown` is not true; the first line of content does not start with `# `.
3. After the runs, `notes.md` and `notes2.md` in both directories either do not exist or start with `# `. (P1's request conflicts with the rule; the AI stopping to explain to the user without creating the file is also a correct result.)
4. `$JITR_HOME/state/errors.jsonl` does not exist or is empty.
5. Note how Codex writes the file in P2 (apply_patch or a shell command); if a shell command, the journal's `effect` should be `write`.

Write each command's final output, each check's result, and the relevant journal lines into the "Smoke test" section of `docs/runbook/ACCEPTANCE.md`. If a check fails, locate the cause with `jitr explain --event` (available after the unit commands task) or by reading the journal: if it is the code, fix it and rerun the smoke test; if it is the spec, stop and report; do not change the semantics yourself.

## Done when

`make check` passes; `bin/jitr hook < testdata/payloads/claude-code/prompt.json` prints nothing and exits 0 with no profile; all five smoke checks pass (or the reason a harness could not be tested is written down). Commit `hook pipeline`.
