# Release and real-environment acceptance (joins after unit commands and evaluation, and isolated launch)

**Read first**: all of spec/cli.md (to check for missing commands).

## Steps

1. Check spec/cli.md item by item and add any command not yet implemented.
2. Write the repository `README.md` (for ordinary users). In this order:
   - a one-sentence introduction;
   - Install: download the binary for the platform and run `jitr install <harness>`;
   - Get started: `jitr profile init ~/jitr-profile --name me`, set `TYPESAFE_API_KEY` (without it, rules that need model judgment do not take effect, including the built-in core pack);
   - Usage: just tell the AI "from now on, …"; the AI follows the jitr guide workflow to turn it into a rule, and you only confirm;
   - Install community packs: `jitr pack add <git-url>`;
   - Uninstall: `jitr uninstall all && jitr purge`;
   - Notes for each harness (taken directly from the notes in the definition files).
3. `make dist`.
4. On a real machine, go through the acceptance checklist below for each harness and write the results into `docs/runbook/ACCEPTANCE.md`.

## Acceptance rules

- For each item write pass / fail / not testable, plus one sentence. For not testable, give the reason; never pretend it passed.
- **Every failed item must come with diagnostics**: the output of `jitr explain --last` and the relevant journal lines. Do not describe only the symptom.
- If the code changes after acceptance, rerun every affected acceptance item (including the hook pipeline smoke test) and update ACCEPTANCE.md; its results must correspond to the final committed code.
- If the root cause is in the spec, report it; do not change the semantics.

## Real-environment acceptance checklist (for each harness: claude-code, codex, trae, deepseek)

1. `jitr install <h>`, then complete the follow-up steps from the notes (codex: trust in /hooks; deepseek: restart).
2. In the harness, say "from now on, the first line of every markdown file must be a level-1 heading". Expected: the AI runs `jitr guide`, creates a unit following the workflow (block, the condition is "the content does not start with `# `", `tools` includes `write`, with an "after the fix" miss example), shows the output of `unit show`, and waits for your confirmation. This item needs `TYPESAFE_API_KEY` (the core pack fires through model judgment); without it, write "not testable", then copy `testdata/smoke/profile/units/md.first-line-h1.md` into your profile by hand, enable it with `unit test` and `unit enable`, and continue with item 3.
3. After confirming, ask the AI to write a md file twice:
   - a. "Create notes.md whose first line is Shopping list:, followed by milk, eggs, bread". Expected: the write is blocked; the AI either changes it to start with a `# ` heading and rewrites, or stops and explains the conflict with the rule and asks you to decide. Either way, `notes.md` must never start with a non-heading line.
   - b. "Note in notes2.md: buy milk and eggs tomorrow". Expected: the final file's first line is a heading starting with `# ` (written right the first time, or fixed after a block).
   - For both, record which tool the AI used to write the file and the `tool`, `effect`, and `result.action` of the corresponding journal records.
4. `jitr status` shows this harness installed with recent events.
5. After `jitr uninstall <h>`, the harness configuration has no jitr entries and the other entries are unchanged; `jitr status` shows not installed.
6. On Windows, complete items 1, 3, and 5 for at least one harness.

## Isolated launch acceptance (claude-code, codex, deepseek each; for trae only check that instructions are printed)

First install jitr at user level with `jitr install <h>` (otherwise you cannot see whether isolation runs it twice), then prepare four things in a temporary project:
- a project instruction file (`CLAUDE.md` for Claude Code, `AGENTS.md` for Codex) saying "always make MDMARK the last word of every reply";
- a skill named `isotest` in the harness's user-level skill directory;
- a jitr rule that injects "always make HOOKMARK the last word of every reply" when the user sends a message;
- a user-level hook that is not jitr's: in the harness's user-level hook configuration (`~/.claude/settings.json` for Claude Code, `$CODEX_HOME/hooks.json` for Codex) add a command hook fired when the user sends a message, running `touch <temporary project>/USERMARK`. Remove this hook afterwards and confirm the user configuration is byte-identical to before the test.

Then run each harness three times (deleting the USERMARK file before each run), always asking "Say hi. Then list the names of any skills you have, or say NOSKILLS.":

| How it runs | Expected |
|---|---|
| The harness directly (not through jitr isolate) | The reply has MDMARK, lists isotest, and the USERMARK file is created |
| `jitr isolate <h> -- <non-interactive arguments>` | No MDMARK, replies NOSKILLS, has HOOKMARK, no USERMARK file; the journal has exactly one prompt record for this message (counting records whose err is dedupe; two means both the installed hook and the hook generated for isolation ran) |
| `jitr isolate <h> --no-jitr -- <non-interactive arguments>` | No MDMARK, replies NOSKILLS, no HOOKMARK, no USERMARK file |

Non-interactive arguments: `-p "<question>"` for claude-code; `exec --skip-git-repo-check "<question>"` for codex; for deepseek, pick the non-interactive mode from its CLI help.

For claude-code, add one more item: write the same requirement (with the marker MEMMARK) in `~/.claude/projects/<project path with / and . replaced by ->/memory/MEMORY.md`. Expected: MEMMARK appears in the direct run and not in the isolated run. Delete this directory afterwards.

Write the results into the "Isolated launch" section of ACCEPTANCE.md.

## Done when

ACCEPTANCE.md is filled in and corresponds to the final committed code; `make check` passes. Commit `release and acceptance`.
