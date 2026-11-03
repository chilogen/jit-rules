# Spec: one hook call (`jitr hook`)

Every time a harness fires a hook, it starts `jitr hook --harness <name>` and passes the payload on stdin. The whole flow is done by `pipeline.Run(stdin, now)`, with each step written as its own function. Types are in [contracts.md](contracts.md).

## Iron rule

**Whatever goes wrong, the exit code of `jitr hook` can only be 0 or 2, and 2 can only come from `Action == block`.** Any error is treated as "empty output, exit 0" and written to `{state}/errors.jsonl` with `journal.LogError` (one line `{"ts","stage","err"}`).

To guarantee this:
- The `hook` command sets `DisableFlagParsing: true` and ignores its arguments, including `--harness <name>` and `--harness=<name>`, for compatibility with installed hook commands. Missing or unknown arguments cannot make the hook fail; the payload alone determines the harness.
- The `hook` command writes `Run`'s Stdout and Stderr unchanged; when Exit is 2 it returns `cli.Silent(2)`, otherwise nil.
- `cmd/jitr/main.go` catches panics with `recover`: when the first argument is `hook`, call `journal.LogError("panic", ...)`, print nothing, exit 0; for other commands, print the panic to stderr and exit 3.

## Steps

0. **Master switch**: when the environment variable `JITR_DISABLE` is non-empty, read nothing, print nothing, exit 0, and write no log.
1. **Read**: read stdin (capped at 10MB; exceeding it is an error).
2. **Detect**: `harness.LoadAll`, then `harness.DetectDef`; nil is an error.
3. **Decode**: call `harness.Decode`; on `ErrIgnored`, print nothing and exit (not an error).
4. **Accounting-only events**: when the kind is `session_start` or `compact`:
   - update the session state under the lock: `compact` clears `delivered`; `session_start` changes nothing and only makes sure the state file exists.
   - write one journal record (`result.action = none`, no units), print nothing, and exit.
5. **Claim (dedupe)**: skip when DedupeKey is empty. Otherwise call `Claim`: on dup, write one journal record (`result.action = none`, `err` `"dedupe"`), print nothing, and exit. If taking the lock takes more than 200ms, skip dedupe and neither read nor write state in any later step ("stateless mode").
   - For `tool_result`, claim `"result:" + ev.DedupeKey`; for other kinds, claim `ev.DedupeKey` unchanged. Pre-tool and post-tool hooks with the same tool id have separate claims.
   - After a non-duplicate `tool_result` claim (or when its key is empty), account for `ev.Size` response bytes under the state lock before resolution and rule evaluation. A repeated post-tool hook must not add these bytes again. Continue through gate, judge, and delivery; `tool_result` is not an accounting-only event.
6. **Resolve**: call `profile.Resolve(ev.Cwd)`. An error is handled as an error. When `Resolved.Errors` is non-empty, record the notice "jit-rules：有规则加载失败，运行 jitr status 查看" ("jit-rules: some rules failed to load; run jitr status"), and write each error to errors.jsonl (stage `resolve`).
7. **Gate**: for each unit whose `Trigger.Events` contains ev.Kind, call `gate.Match(u, ev)`. Units that fail are counted (`gate_failed`); in addition, when not in stateless mode and the unit has an entry in `state.delivered`, it is recorded in the journal with `gate` = `Outcome.Failed` and goes no further. Reason: after a delivery, a later event where the same unit fails only at `content` or `shell` is the trace of the AI having fixed the violation; without the record it looks the same as an unrelated event. Limiting it to delivered units keeps the journal small.
8. **Judge**: pass the units that passed the gate and have a Judge to `judge.Decide` together (see judge.md). The whole hook may use at most `deadline_ms` milliseconds from its start: call Decide with a ctx carrying that deadline.
   - When a unit's Verdict is `Unconfigured`: the unit **does not count as a hit**, and record the notice "jit-rules：judge 后端没有配置好（<Verdict.Err>），带模型判断的规则不会生效。运行 jitr status 查看" ("jit-rules: the judge backend is not configured (<Verdict.Err>); rules that need model judgment will not take effect. Run jitr status"), once per hook call however many units are affected. Reason: a missing key is a lasting state; treating it as a hit would deliver every model-judged rule on every event, which is worse than not delivering; "prefer extra delivery" applies only to temporary errors and timeouts.
   - Otherwise: a unit without a Judge is a hit once it passes the gate; a unit with a Judge is a hit only when `Verdict.Hit` is true.
9. **Compute NoBlock for each hit**: true when any of these holds: `Verdict.Fallback`; `Outcome.ContentUncertain`; ev.ContentUnknown is true and the unit has a Judge. Reason: the hit depends on content that cannot be seen, or the model gave no verdict, so it cannot be used to block.
10. **Delivery decision**: call `deliver.Decide` (see below) to get the Result.
11. **Notice**: when steps 6 or 8 recorded notices, the call is not in stateless mode, and `notified` in the state is false, join the notices with `\n` into `Result.Notice`, and set `notified` to true in step 12. At most one notice per session.
12. **Write state**: unless in stateless mode, update under the lock: first `before = bytes`, then add `len(res.Text)` plus `ev.Size` only if it was not already accounted for in step 5; for each unit in `res.Delivered` set `delivered[id] = {at: before}`. A post-tool response's bytes are counted once, before rule evaluation, and are not added again here.
13. **Write the journal**, then **encode and print**. A failed journal write is only recorded in errors.jsonl and does not affect the output.

## gate.Match

Check in the order tools → paths → shell → prompt → content, skipping fields that are not written; the name of the first field that fails goes into `Failed`:
- `Tools`: any glob (`doublestar.Match`) matches `ev.Tool`, or `ev.Effect` is non-empty and matches. Reason: when a shell command writes a file, a `tools: [write]` rule should check it too.
- `Paths`: any path of the event matches any glob; an event without paths fails.
- `Shell` / `Prompt`: the corresponding field is matched by any regular expression (`MatchString`).
- `Content`: when `ev.ContentUnknown` is true, it passes with `ContentUncertain = true`; otherwise `ev.Content` must be matched by any regular expression.

The gate compiles regular expressions itself and caches them in a package-level `sync.Map` keyed by the expression; an expression that fails to compile does not match.

## deliver.Decide

Input: the hit units (already sorted by rank), ev, the session state, settings. Below, "a unit that can block" means: ev.Kind is `tool` or `tool_result`, the unit's `deliver.as == block`, and `NoBlock` is false.

For `tool_result`, the tool has already executed. Injection advises on the materialized result; blocking requires the AI to fix that result before continuing the workflow. It does not prevent or roll back the completed tool call. Tool, path, shell, and content conditions still use the normalized fields extracted from `tool_input`; response bytes are used for accounting as specified in event.md.

1. **Topic arbitration**: under the same non-empty topic, keep only the highest-ranked unit and mark the rest `skip: topic`.
2. **Redelivery filter**: units that can block **do not take part** in this step. When ev.Kind is `stop`, the byte distance is not used: every remaining unit is marked `skip: redeliver` when `ev.StopActive` is true, and kept otherwise. Reason: a turn-end reminder is due once per turn however short the turn; the harness's own flag says this end was already sent back once, so reminding again would loop. On other kinds, the remaining units are marked `skip: redeliver` when `state.bytes - delivered[id].at < redeliver_bytes`; a unit with no entry in the state can be delivered. Reason: blocking must apply to every violating call, otherwise an unchanged retry would bypass the rule; injection only needs a reminder every so much distance.
3. **Choose the action**: if any remaining unit can block → `block`; otherwise if any unit remains → `inject`; otherwise → `none`.
4. **Assemble the text**:
   - block: first all units that can block, by rank (never truncated), then the other remaining units, by rank (subject to truncation). `Blocked` = the units that can block.
   - inject: all remaining units, by rank (subject to truncation).
   - Truncation: starting from the first unit subject to truncation, accumulate the rune count of `[<global id>] <body>`; when adding a unit makes the total exceed `max_inject_chars`, that unit and every later unit subject to truncation are marked `skip: truncated`. For inject, the first unit is always kept however long it is.
   - Format:
     - inject:
       ```
       <jit-rules>
       [<global id>] <body>
       [<global id>] <body>
       </jit-rules>
       ```
     - block for `tool` (the introduction is verbatim):
       ```
       jit-rules 拦截了这次调用：即将执行的内容违反了下列规则。请按规则改正后重新发起；如果你确认规则不适用于这次操作，停下来告诉用户，不要原样重试。
       [<global id>] <body>
       ```
       The introduction says: "jit-rules blocked this call: what was about to run violates the rules below. Fix it according to the rules and try again; if you are sure a rule does not apply to this operation, stop and tell the user instead of retrying unchanged."
     - block for `tool_result` (the introduction is verbatim):
       ```
       jit-rules 要求先修复工具结果：工具已经执行，但产生的结果违反了下列规则。请先按规则修复结果再继续；如果你确认规则不适用于这次操作，停下来告诉用户。
       [<global id>] <body>
       ```
     - inject for `stop` (the introduction is verbatim):
       ```
       jit-rules 在你结束本轮之前提醒：请检查下列规则是否适用于本轮的工作；适用就照做后再结束，不适用就直接结束，不需要解释。
       [<global id>] <body>
       ```
       The introduction says: "jit-rules reminds you before you end this turn: check whether the rules below apply to this turn's work; if so, follow them and then end; if not, just end without explaining."
     Each unit starts a line, and newlines within a body are kept; lines are joined with `\n`, with no trailing newline.
   - `Delivered` = the units whose bodies made it into Text.

## Session state (`{state}/sessions/<harness>/<session_id>.json`)

```json
{"v":1,"bytes":0,"delivered":{"core/learn":{"at":0}},"seen":{"tool:abc":"2026-11-02T10:00:00Z"},"notified":false,"updated":"..."}
```

- The file lock is on `<state file>.lock` (gofrs/flock); after each failed TryLock wait 10ms and retry, for at most 200ms.
- **Dedupe lifetime**: a `seen` key starting with `prompt:sha1:` or `stop:sha1:` is a duplicate only when it appears again within 5 seconds; other keys within 10 minutes. Reason: a harness without a prompt id can only dedupe by content, and a user sending the same "continue" again a little later must not be swallowed; whereas the same submission fired once by each of two configurations arrives milliseconds apart.
- On every write, remove `seen` entries older than 10 minutes.
- Each `jitr hook` has a 1% chance of deleting files under `sessions/` not modified for more than 7 days, and files under `journal/` older than `journal.keep_days` days.

## Journal (`{state}/journal/YYYY-MM-DD.jsonl`, one file per UTC date, one line appended per call)

```json
{
  "v": 1,
  "ts": "...",
  "isolated": false,
  "event": { "...the normalized event without input; when journal.store_content is false, content, shell, and prompt become \"sha256:<hex>\"..." },
  "profiles": ["me"],
  "gate_failed": 12,
  "units": [
    {"id": "me/git.commit-zh", "hash": "sha256:...", "judge": {"p": 0.82, "hit": true, "fallback": false, "unconfigured": false, "backend": "jev", "model": "jev-1.14.0", "conn_ms": 420, "ms": 640, "err": ""}, "no_block": false, "skip": "", "delivered": "block"},
    {"id": "me/branch.merge-master", "hash": "sha256:...", "judge": {"p": 1, "hit": true, "fallback": false, "unconfigured": false, "backend": "script", "model": "", "conn_ms": 0, "ms": 18, "err": "", "exit": 0}, "no_block": false, "skip": "", "delivered": "inject"}
  ],
  "result": {"action": "block", "chars": 212},
  "ms": 702,
  "err": ""
}
```

- `event.harness` is the detected harness name. New records omit the former `harness_flag` field; old records containing it remain readable, with the extra field ignored.
- `isolated` is whether the environment variable `JITR_ISOLATED` is non-empty.
- `units` records every unit that passed the gate (including ones that missed in judgment or were skipped), plus the units that failed the gate but were delivered earlier in this session (step 7). For a unit without a Judge, and for a unit that failed the gate, the `judge` field is null.
- `gate`: empty (omitted) for units that passed the gate; for a unit that failed it, the name of the first field that failed (`Outcome.Failed`). Old records without it read as empty.
- `delivered`: `block` when delivered as a block reason, `inject` when delivered as an injection (including other units attached to a block reason), the empty string when not delivered.
- `result.chars` is the rune count of `res.Text`.
- `judge.exit` and `judge.stderr` come from `Verdict.Exit` and `Verdict.Stderr` (script judges only; omitted when empty).

## pipeline.Explain

For `jitr explain`: run steps 6–10 above on the given event with `st` nil (decide as if the state were empty); no dedupe, no reading or writing state, no journal, no notice; judge is called normally (it goes through the cache). Returns each candidate unit's gate result, the journal record assembled in the format of step 13, and the delivery result.
