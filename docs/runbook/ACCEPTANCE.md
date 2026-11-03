# Acceptance record

## Judgment join real harness — 2026-11-03

Pass on Linux x86_64 with Claude Code 2.1.304. Evidence is retained under `/tmp/jitr-judgment-join.rtysEM`: `claude-direct.out`, setup/test/enable/status output, the installed settings, and `jitr/state/journal/2026-11-03.jsonl`; temporary credential copies were removed. The run used temporary HOME and JITR_HOME, the installed Claude Code user hook, the `branch` profile, and a scratch `feature` branch six commits ahead of `master`.

- Both script units were tested through the CLI: their gate-passing examples were skipped with `脚本判断依赖真实环境，示例测试不运行脚本`; both enabled successfully with warnings. `status` reported two active units and `sh` found.
- With `change.txt` staged, the real agent was asked to run `git commit -m acceptance`. Commit `2a234f5` succeeded. Its final response quoted both injected bodies exactly: `[branch/merge-master] 提交前先合并 master。` and `[branch/notify-user] 提交前告诉用户当前分支已大幅领先 master。`
- The matching Claude Code `PreToolUse` journal record at `2026-11-03T06:28:34.731588253Z` has action `inject`. Both units have backend `script`, hit `true`, fallback `false`, `ms` 3, and exit 0; the whole hook took 6ms. `errors.jsonl` was absent.
- Diagnostic run before the pass: the agent attempted `git add` and a combined add/commit command under `acceptEdits`; permissions rejected staging and no command began with `git commit`, so the branch script gates did not run. The passing rerun pre-staged the file and used the exact commit command with `bypassPermissions`.

## JEV and full authoring chain — 2026-11-03

Linux x86_64; Claude Code 2.1.316, Codex CLI 0.166.0. Evidence: `/tmp/jitr-full-acceptance.F4mnwn/{claude-code,codex}` (transcripts, profiles, tests, journal, status and explain output). All runs used temporary HOME/JITR_HOME; user configuration was unchanged and temporary credential copies were removed. Codex used `--dangerously-bypass-hook-trust` for the installed test hooks.

The unscoped acceptance profiles applied these settings to every CLI judgment and hook; installed test hooks had an outer timeout of 30s. Built-in defaults were not changed.

```yaml
settings:
  judge:
    timeout_ms: 20000
  deadline_ms: 25000
```

- Pass: `jitr unit test core/author --json`, 3/3 real JEV examples, no skips/errors, 3.47s total.
- Pass: both harnesses received a real JEV `core/author` injection, ran `jitr guide`, created/tested/replayed/checked a draft, showed `unit show`, waited for confirmation, then enabled it.
- Pass: Claude's final draft had 8 passing examples; Codex's had 10. Claude's initial draft missed a lone `#`; review requested a regression example, it failed, and Claude fixed the draft before confirmation.
- Pass: installation/status/uninstallation on both harnesses. Before uninstall: installed with recent events; afterwards: not installed, original non-jitr hook configuration preserved.
- Pass: `make check`.

JEV journal evidence (all `jev-1.14.0`, no fallback, unconfigured verdicts, or judgment errors):

| Event | Author P / delivery | Request / connection time |
|---|---|---|
| Claude rule request | 0.98 / inject | 1102 / 848ms |
| Codex rule request | 0.98 / inject | 1762 / 1517ms |
| Claude draft revision | 0.63 / already delivered | 4827 / 4558ms |

The 4827ms request completed with the raised timeout. These are client-side request times; subtracting connection time still includes network transfer and server work, not just inference.

## Real-harness smoke results — 2026-11-03

The enabled rules were authored by the harnesses above. P1 requested `notes.md` starting with `Shopping list:`; P2 requested a shopping reminder in `notes2.md`.

| Harness / request | Journal: tool / effect / action | Final response and file |
|---|---|---|
| Claude P1 | write (Write) / — / block | Explained the conflict and asked for a decision; no file |
| Claude P2 | write (Write) / — / none | Reported creation; first line `# Notes` |
| Codex P1 | write (apply_patch) / — / block | Reported the rule conflict; no file |
| Codex P2, first attempt | shell (Bash) / write / block | Retried with a heading |
| Codex P2, corrected | shell (Bash) / write / none | Reported creation; first line `# Note` |

Pass: all five smoke checks. Both P1 writes were blocked; every observed known-content non-heading write to these Markdown files was blocked; final files were absent or compliant; neither errors.jsonl existed; Codex shell writes had `effect=write`. Journal times: Claude 17:37:43Z / 17:38:15Z; Codex 17:38:23Z / 17:39:05Z / 17:39:10Z on 2026-11-02 UTC. Final `jitr explain --last 1 --json` on each harness showed the compliant write failing the rule's content gate, `action=none`, and no errors.

Conclusion: JEV CLI evaluation and the full Claude/Codex author → confirm → enable → block/correct → status → uninstall chain pass with the settings above.

## Earlier failures and diagnostics

- Default-timeout JEV check (2026-11-03): at 3500ms, CLI examples 0 and 1 returned `context deadline exceeded`; example 2 passed. This is superseded only for the raised-timeout configuration, not for the built-in defaults. Evidence: `/tmp/jitr-jev-acceptance.4GrQJ0/jitr-home/state/tests/core__author.json`. `unit test` creates no hook journal event. Subsequent hook journal: author P=0.98, inject, request/connection 2265/1999ms; `explain --last 1`: author P=0.98, inject, 2292/1770ms, no errors. The standalone live backend test also passed 3/3 with a 20s timeout.
- Uninstall (2026-11-02): same-second backups initially failed with `create backup: ...bak-jitr-20261102135436: file exists`. `explain --last 1` reported `action=none`; no matching hook event or errors.jsonl, since the failure was in CLI configuration backup. After adding numbered backup suffixes, install/uninstall passed on all four harnesses.

## Other release coverage — 2026-11-02

Evidence: `/tmp/jitr-j3-final.Jxfbte`; temporary user configuration throughout.

- Pass: CLI command/flag audit, `make dist` for Linux/macOS amd64+arm64 and Windows amd64, and all four harnesses' offline rule test (5/5), disable and enable.
- Pass: TRAE and DeepSeek install, fixture event, status with zero internal errors, uninstall preserving other entries.
- Not testable: real TRAE authoring/writes (no controllable IDE session), real DeepSeek authoring/writes/isolation (`dsh` absent), and Windows runtime acceptance (Linux host). These limitations remain in the latest retest.

## Isolated launch — 2026-11-02

| Harness | Result |
|---|---|
| Claude 2.1.304 | Pass: direct run had skill, instruction, memory and both hook markers; isolate retained only jitr's HOOKMARK; `--no-jitr` had none; exactly one prompt journal event |
| Codex 0.163.0 | Pass: direct run had skill, instruction and both hook markers; isolate retained only HOOKMARK; `--no-jitr` had none; exactly one prompt journal event |
| TRAE | Pass: printed IDE isolation instructions and exited 1 |
| DeepSeek | Not testable: CLI not installed |

Original user configuration was restored byte-for-byte. Isolation was not rerun in the latest authoring acceptance.
