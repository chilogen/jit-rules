# Spec: model judgment and evaluation

Types are in [contracts.md](contracts.md).

## judge.Decide

1. When `units` is empty, return an empty map right away.
2. **Route**: a unit with `Judge.Script` is a script judge (next section). Every other unit uses the model backend `u.Judge.Backend`, or `s.Judge.Backend` when that is empty; units are grouped by backend name. Every group and every script runs concurrently; Decide returns when all are done. Steps 3–7 apply to each group.
3. Get the backend by name from the registry; if it is not found, every unit of the group gets `{Unconfigured: true, Backend: <name>, Err: "unknown backend"}`.
4. **state** (the event summary sent to the model, the same for every group): a map with the non-empty fields among `kind`, `tool`, `raw_tool`, `effect`, `paths`, `shell`, `content`, `prompt`, each string field truncated to `max_field_chars` runes; when `send_content` is false, `shell` and `content` are left out. Each unit produces one Question: `{ID: u.ID, Text: u.Judge.Question, Not: u.Judge.Not}`.
5. **Cache** (only for units whose `judge.cache` is unset or true): for each Question compute key = sha256(backend name + "\n" + model + "\n" + Text + "\n" + Not + "\n" + JSON of state), where model is the string value of `Backends[backend name]["model"]`. Cache hits use the cached P and Model directly; only the Questions that missed, plus those of units with `cache: false`, are sent to the backend, in one batch. Successful results of cached units are written to the cache (`{"p":..,"model":..}`). Cache entries never expire.
6. Call `Backend.Judge(ctx, state, questions, s.Backends[backend name])`, with the ctx timeout = min(`judge.timeout_ms`, the time remaining in the incoming ctx).
7. Results (the threshold is `s.ThresholdOf(backend name)`):
   - Success: Hit is true when `P >= threshold`. An ID missing from the result is treated as `{Hit:true, Fallback:true, Err:"missing"}`.
   - An error wrapping `backend.ErrNotConfigured`: the whole batch is `{Unconfigured:true, Err:<error>}`, with Hit false.
   - Any other error (timeout, network error, non-2xx status, parse failure): the whole batch is `{Hit:true, Fallback:true, Err:<error>}`.
   - No retries.
   - Every Verdict carries `Backend`, `Model` (from the result or the cache), `ConnMs` (from `Result.ConnMs`, also on errors; 0 on a cache hit), and `Ms` (total time of this backend call; 0 on a cache hit).
- **Timing**: backends record connection time with `httptrace`: in the `GotConn` callback, only when `info.Reused` is false, record `time of GotConn - start time`, and only the first time. The callback may run on another goroutine, so use `sync.Once` plus an atomic. Reason: every hook is a new process that has to set up a TLS connection again; we need to know whether time goes into connecting or into inference.

## Script judge (`internal/judge`)

A script judge is not a `backend.Backend`: it needs the event's directory and its own script, which the backend interface does not carry, and it never goes to the network. Its Verdict has `Backend` = `"script"` and an empty `Model`.

1. **Interpreter**: `exec.LookPath(u.Judge.Lang)`; when it fails → `{Unconfigured: true, Err: "<Lang> 不在 PATH 中"}` ("<Lang> is not in PATH"). Reason: like a missing api key, a missing interpreter is a lasting state.
2. **Directory**: `ev.Cwd`; when it is empty or not a directory → `{Hit: true, Fallback: true, Err: <reason>}`.
3. **Cache** (only when `judge.cache` is true): key = sha256("script\n" + Lang + "\n" + Script + "\n" + ev.Cwd + "\n" + JSON of state), state as in step 4 above; a hit uses the cached P. Exit codes 0 and 1 are written to the cache.
4. **Script file**: write the script once to `{cache}/scripts/<sha256 of Script>.<ext>` (`fileutil.WriteAtomic`, mode 0600, skipped when the file exists). `<ext>` is `Lang`, except `ps1` when `Lang` is `pwsh` or `powershell`. Reason: passing the script as a file works for every interpreter that takes a script path, without per-language flags; PowerShell is the one common interpreter that refuses a file without its own extension.
5. **Run** `<interpreter> <script file>` with `exec.CommandContext` on the incoming ctx (no timeout of its own; the hook deadline bounds it) and `WaitDelay` 100ms; working directory `ev.Cwd`; environment = the current one plus `JITR_UNIT=<global id>`; stdin = the JSON of `ev` (the same encoding as the journal's event, with full content); stdout discarded; stderr kept, trimmed, first 500 runes.
6. **Result**:
   - exit 0 → `{Hit: true, P: 1}`; exit 1 → `{Hit: false, P: 0}`;
   - exit 9009 (Windows: the command could not be run, as the Microsoft Store placeholder `python3` does) → `{Unconfigured: true, Err: "<Lang> 无法运行"}` ("<Lang> cannot run"). Reason: an interpreter that is found but cannot run is as lasting as one that is missing. Exit codes on Unix never exceed 255, so this check needs no platform switch;
   - any other exit code, a failure to start, or ctx done → `{Hit: true, Fallback: true, Err: <reason>}`.
   - Every Verdict carries `Ms` (time from start to exit; 0 on a cache hit), `Exit` (the exit code; nil when the process did not exit by itself or on a cache hit), and `Stderr`.

Reason for "fallback": a script that errors or is killed gave no verdict; as with a model timeout, deliver but never block.

## Backend: jev (`internal/backend/jev`)

- Configuration (from cfg): `model`, `base_url`, `api_key_env`. Missing any of them is an error.
- When the api key variable is empty, send no request and return `fmt.Errorf("%w: <variable name> 未设置", backend.ErrNotConfigured)` ("<variable name> is not set").
- Request: `POST {base_url}/v1/systemone` with headers `Authorization: Bearer <key>` and `Content-Type: application/json`.
- Request body:

  ```json
  {"model": "<model>", "state": <state>, "questions": {
     "<ID>": {"type": "noul", "instructions": "<Text>", "criteria": {"true": "<Text>", "false": "<Not>"}}
  }}
  ```

  When Not is empty, omit `criteria`.
- Response: `{"model": "jev-1.14.0", "answers": {"<ID>": {"type": "noul", "noul": 0.95}}, "usage": {...}}`. P[ID] = `answers[ID].noul`; Model = the response's `model` (the configured model when empty).

## Backend: openai (`internal/backend/openai`)

- Configuration: `model`, `base_url`, `api_key_env`; an empty api key is handled as for jev.
- Request: `POST {base_url}/decisions` with headers `Authorization: Bearer <key>` and `Content-Type: application/json`.
- Request body:

  ```json
  {
    "model": "<model>",
    "input": "<JSON of state>",
    "questions": [
      {
        "type": "predicate",
        "name": "<ID>",
        "instructions": "<Text>\nAnswer false when: <Not>"
      }
    ]
  }
  ```

  When Not is empty, omit the `Answer false when:` line.
- Response: for each predicate answer, P[`name`] = `probability`, clamped to [0,1]. Ignore refusal and other answer types. Model = the response's `model`.

## Backend: fake (`internal/backend/fake`, tests only)

Exports a struct with configurable fields: name, P per ID, default P, an optional `func(state map[string]any, q backend.Question) float64` (when set, it computes P, so different events can get different P), delay, error to return, ConnMs; plus accessors for the call count and the last request. Used to test fallback, the cache, batching, and evaluate.

## Evaluation (`internal/evaluate`)

### Unit test `evaluate.Test(u, s, offline)`

First a structural check: when `deliver.as` is block, recheck rule 12 of unit.md and, if it fails, add `{Index: -1, Result: "fail", Note: <rule 12's message>}`.

Then for each example:
1. Build the Event: copy from example.event, with `Root` = `Cwd` = `/work/proj`.
2. Kind not in `Trigger.Events`, or `gate.Match` fails → Got is miss.
3. Passed the gate and has a script judge: record `skipped` with Note "脚本判断依赖真实环境，示例测试不运行脚本" ("script judges depend on the real environment; example tests do not run scripts"). Reason: the example's directory `/work/proj` is synthetic; running the script would test whatever repository the command happens to run in.
4. Passed the gate and has a question judge: when `offline` is true, record `skipped`; otherwise call `judge.Decide` (ctx timeout `judge.timeout_ms`):
   - `Unconfigured` → `skipped`, with the reason in Note;
   - `Fallback` → `error`, with Err in Note;
   - otherwise Got is hit / miss according to `Hit`.
5. When there is a Got, compare it with `expect` to get `pass` or `fail`.

`SaveTest` writes the results to `{state}/tests/<global id with / replaced by __>.json`: `{"hash": <unit hash>, "ts":..., "cases": [...]}`.

### Replay `evaluate.Replay(u, s, since, live, now)`

1. Read records with `journal.Since(now - since)`, skipping those whose kind is not prompt / tool / tool_result / stop and those whose `err` is `dedupe`. Return an empty report and nil error when there are no records. If reading fails, return an error wrapping the cause; do not report a successful empty or partial replay. The CLI treats journal read or parse failures as internal errors (exit 3), with no success summary.
2. For each event: Kind not in `Trigger.Events` or the gate fails → not a hit. Passed the gate without a Judge → hit. With a Judge: if the journal has a record for this unit with the same hash as now, and it is neither fallback nor unconfigured, reuse its hit; otherwise a script judge counts as Unknown (even when `live` is true: the environment of a past event no longer exists), and a question judge calls `judge.Decide` when `live` is true (an Unconfigured or Fallback result counts as Unknown), or count it as Unknown when false.
3. `Judged` = the number of events that passed the Kind check and got a definite verdict (hit or not).
4. Each hit carries a one-line summary: `<time RFC3339> <harness> <kind> <tool> <first path, or the first 60 characters of shell, or the first 60 characters of prompt>`.
5. The CLI shows at most 50 entries, plus the total, the hit rate (hits / Judged), and the Unknown count. When the journal was written with `store_content: false`, content, shell, and prompt are only hashes, so conditions depending on them never hit; the CLI prints a one-line note in that case.

### Conflict check `evaluate.Conflicts(target, all, s, history)`

For each other unit B in all (skipping target itself, units sharing a non-empty topic with target, and units related to target by replaces):

1. **Do they fire together?** Give each of target's hit examples to B (Kind check + gate + judge), then each of B's hit examples to target. If any example makes both hit, they "fire together". When a judge result is Unconfigured or Fallback, or the unit has a script judge (never run here, as in unit tests), passing both gates counts as "may fire together". When the examples do not show that they fire together but `history` contains the pair, they also count as firing together, with the example summary written as `真实事件 <replay summary>` ("real event ...").

   `LoadCoHits` reads all retained journal records: the units that hit in each record (`gate` empty, and `judge` null or `hit` true and not unconfigured) are paired, and each pair keeps the replay summary of its first occurrence. Reason: overlaps the examples do not cover do show up in real events. Unit hashes are not compared: this step only nominates candidate pairs, and whether they contradict is always judged on both units' current bodies; after a rule is edited, old records still show that the two triggers overlap.
2. For each pair that fires together, call the backend once: state = `{"instruction_a": target.Body, "instruction_b": B.Body}`, questions = `[{ID:"conflict", Text:"Would following instruction_a make it impossible or unclear to follow instruction_b, or do they ask for contradictory things?"}]`. P ≥ 0.5 is a conflict (`Confirmed: true`). When the call returns an error (including a missing key), record a possible conflict (`Confirmed: false`). A pair that only "may fire together" is recorded as a possible conflict without calling the backend.
3. Output of `FormatConflict` (in Chinese, verbatim):

   ```
   冲突：<target id> ↔ <B id>（同时触发的示例：<example summary>）
   可选处理：
     1. 两条是同一件事的不同要求 → 给它们设置相同的 topic，只让排位高的生效（当前排位高的是 <id>）
     2. 用 <target id> 完全替换 <B id> → 在 <target id> 里加上 replaces: [<B id>]
     3. 不需要 <B id> → jitr unit disable <B id>
     4. 两者其实不冲突，只是触发范围太宽 → 收紧其中一条的 trigger
   ```

   That is: "Conflict: A ↔ B (example that fires both: …). Options: 1. Two requirements about the same thing → give them the same topic so only the higher-ranked one fires (currently <id>); 2. Replace B entirely with A → add replaces: [B] to A; 3. B is not needed → jitr unit disable B; 4. They do not really conflict, the trigger is just too broad → narrow one of the triggers."

   When `Confirmed` is false, the first line becomes "可能冲突（无法调用模型确认）：" ("Possible conflict (could not confirm with the model):"), the rest unchanged.

A confirmed conflict makes `unit check`, `unit enable`, and `pack add` fail; a possible conflict is printed only as a warning. `jitr check` (without an id) checks all active units pairwise; `unit enable` and `pack add` check only the newly added units.
