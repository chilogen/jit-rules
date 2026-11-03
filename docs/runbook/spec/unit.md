# Spec: judgment units

A judgment unit is one `.md` file in the `units/` directory of a pack or a profile. Complete examples are in `assets/guide.md`, `assets/unit-template.md`, and `assets/packs/core/units/`. Go structures are in [contracts.md](contracts.md).

## File structure

The file starts with `---\n`, followed by YAML frontmatter ending with `\n---\n`; whatever follows is the **body** (with leading and trailing whitespace trimmed), after the script block below is cut out. If the file uses `\r\n` line endings, convert `\r\n` to `\n` before parsing (the hash is still computed over the original bytes).

### Script block

A fenced code block in the body whose info string contains the word `jitr-judge` is the unit's **script judge**:

````md
```sh jitr-judge
[ "$(git rev-list --count "$(git merge-base HEAD master)"..HEAD)" -gt 3 ]
```
````

- The opening fence is a line starting with three or more backticks or tildes; its info string is split on whitespace. The block ends at the first later line made only of the same fence character, at least as many as the opening, plus optional trailing spaces.
- The first word of the info string is the **interpreter** (`Judge.Lang`), for example `sh`, `bash`, `python3`; the lines between the fences, each followed by `\n`, are the script (`Judge.Script`).
- The lines from the opening fence through the closing fence are removed before the body is trimmed. Reason: the body is the text delivered to the AI; the script is for jitr only.
- Fenced blocks without `jitr-judge` are ordinary body text.
- When a script block exists and the frontmatter has no `judge`, `Judge` is created empty to hold it.

### id

- **Short id** = the file name without `.md`. It must match `^[a-z0-9][a-z0-9.-]{0,63}$`, otherwise it is an error.
- **Global id** = `<pack name>/<short id>`. For a profile's own units, the pack name is the profile name.
- The frontmatter may not contain an `id` field (guaranteed by `KnownFields(true)` and `yaml:"-"`).

### Frontmatter fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `title` | string | yes | One line, at most 80 characters |
| `trigger.events` | []string | yes | Non-empty subset of `prompt`, `tool`, `tool_result`, `stop` |
| `trigger.tools` | []string | no | Globs over normalized tool names (see event.md); also compared with the Effect of a shell command |
| `trigger.paths` | []string | no | doublestar globs |
| `trigger.shell` | []string | no | Go RE2 regular expressions |
| `trigger.prompt` | []string | no | Go RE2 regular expressions |
| `trigger.content` | []string | no | Go RE2 regular expressions; on `stop` they match the AI's final reply |
| `judge.question` | string | exactly one of `judge.question` and a script block | A yes/no question for a model backend |
| `judge.not` | string | no | Only with `judge.question` |
| `judge.backend` | string | no | Only with `judge.question`; name of a model backend in `settings.backends`, default `settings.judge.backend` |
| `judge.cache` | bool | no | Whether verdicts are cached (judge.md); default true with `judge.question`, false with a script block |
| `deliver.as` | string | no, default `inject` | `inject` or `block` |
| `topic` | string | no | Matches `^[a-z0-9][a-z0-9.-]*$` |
| `replaces` | []string | no | Global ids of other units |
| `examples` | []Example | yes | See below |

Parse with `yaml.Decoder` and `KnownFields(true)`: unknown fields are errors. Reason: if a misspelled field were silently ignored, the rule would behave in a way its author did not expect.

### examples

```yaml
examples:
  - expect: hit          # hit or miss
    event:               # a subset of the normalized event
      kind: tool         # required, prompt, tool, tool_result, or stop
      tool: write
      paths: [docs/design.md]
      content: "## 决策"
```

Fields allowed in `event`: `kind`, `tool`, `raw_tool`, `effect`, `paths`, `shell`, `content`, `content_unknown`, `prompt`, `harness`, `session_id`, `cwd`. Fields not written take their zero values.

## Validation rules (`unit.Validate`, checked in order; all errors and warnings are collected and returned together)

Every error and warning text starts with `规则 <number>：` ("Rule <number>:"), followed by which field and how to fix it.

| # | Condition | Level |
|---|---|---|
| 1 | A required field is missing, there is an unknown field, a type is wrong, `deliver.as` is not inject/block, an example's `expect` is not hit/miss, or an example's `kind` is not prompt/tool/tool_result/stop | Error |
| 2 | A regular expression fails to compile or a glob is invalid (`doublestar.ValidatePattern`) | Error |
| 3 | `events` contains neither `tool` nor `tool_result`, yet `tools`, `paths`, or `shell` is written; or contains none of `tool`, `tool_result`, `stop`, yet `content` is written | Error |
| 4 | `events` does not contain `prompt`, yet `trigger.prompt` is written | Error |
| 5 | `deliver.as: block`, but `events` contains neither `tool` nor `tool_result` | Error |
| 6 | `judge` is written and `events` contains `tool` or `tool_result`, but neither `tools` nor `paths` is written | Warning: every matching tool event will go to the network for judgment (with a script block: will run the script) |
| 7 | No trigger field other than `events` is written, and there is no `judge` (question or script) either | Error (equivalent to delivering on every event) |
| 8 | `examples` has no `hit` or no `miss` | Error |
| 9 | The body is empty | Error |
| 10 | The body exceeds 600 characters (counted in runes) | Warning |
| 11 | `judge.question` does not end with `?` | Warning |
| 12 | `deliver.as: block`, but no miss example has the same `tool` as some hit example | Error: "拦截单元需要一条'改对之后'的 miss 示例：和某条 hit 示例是同一个工具，只是内容合规" ("a blocking unit needs an 'after the fix' miss example: the same tool as some hit example, only with compliant content") |
| 13 | `deliver.as: inject` with `trigger.content` written and `events` containing `tool` or `tool_result` | Warning: "content 条件只能在内容写好之后判断，inject 只能让 AI 事后修正；要在写入之前纠正，请用 block" ("a content condition can only be judged after the content is written, and inject can only make the AI fix it afterwards; to correct it before writing, use block") |
| 14 | `judge` has neither `question` nor a script block, or has both; `not` or `backend` is written without `question` | Error |
| 15 | More than one script block; a script block is not closed; its info string has no interpreter before `jitr-judge`; the script is empty | Error |

Reason for rule 12: blocking applies to every call that meets the condition. If the condition describes "doing a certain kind of thing" rather than "violating the rule", the call still meets the condition after the AI fixes it and is blocked forever. Requiring an "after the fix" miss example, and passing the test, proves the condition no longer holds once fixed.

For `stop`, only `inject` exists: before the turn ends, the rule is handed to the AI once, and the AI decides whether it applies (pipeline.md). Reason for rule 5 still excluding `stop`: blocking every violating end needs a guarantee that the AI's explanation of why a rule does not apply ends the loop, and the harnesses do not all cap repeated refusals; this is left open rather than risk a turn that never ends.

For `tool_result`, blocking means requiring a fix before the workflow continues: the tool has already run and its effects are not rolled back. Use `tool` to check before execution. See pipeline.md for post-tool delivery and dedupe semantics.

## Plain-language description (`unit.Describe`, the output of `jitr unit show`)

Printed line by line; a line whose item is absent is omitted entirely. The output is in Chinese; the template is (`<>` marks substitutions):

```
【<title>】  <global id>
何时检查：<event description>
条件：<condition list joined with "，且">
判断：由模型回答"<question>"，概率 ≥ <threshold> 才算命中（排除：<not>）
判断：在项目目录用 <interpreter> 运行下面的脚本，退出码为 0 才算命中
  <script, each line indented by two spaces>
做什么：<action description>
规则原文：
  <body, each line indented by two spaces>
冲突域：<topic>（同一时刻只生效排位最高的一条）
替换：<replaces joined with 、>
示例：<n> 条应触发，<m> 条不应触发
```

The lines are, in order: title and id; when it is checked; conditions; judgment, either by a model ("answered by the model; a hit only if the probability ≥ threshold (excluding: not)") or by a script ("run the script below with <interpreter> in the project directory; a hit only if the exit code is 0", followed by the script so the person confirming the unit sees what will run); what it does; the rule text; conflict domain ("only the highest-ranked fires at any moment"); replaces; examples ("n that should fire, m that should not").

- Event description: `prompt` → "用户发消息时" (when the user sends a message); `tool` → "AI 调用工具之前" (before the AI calls a tool); `tool_result` → "AI 工具执行之后" (after the AI tool executes); `stop` → "AI 结束本轮回复之前" (before the AI ends its reply for this turn); descriptions are joined with "或" (or), in the order they appear in `events`.
- Conditions: `tools` → "工具是 a、b" (the tool is a, b); `paths` → "操作的文件匹配 x、y" (the file operated on matches x, y); `shell` → "命令匹配 /re/" (the command matches /re/); `prompt` → "用户消息匹配 /re/" (the user message matches /re/); `content` → "即将写出的内容匹配 /re/" (the content about to be written matches /re/). Multiple regular expressions are joined with "或", for example "命令匹配 /a/ 或 /b/".
- Judgment line (model): the threshold is the caller's `Settings.ThresholdOf(<the unit's backend>)`, formatted with `strconv.FormatFloat(t, 'f', -1, 64)`; without `not`, the "（排除：…）" part is omitted; when `judge.backend` is written, "由模型" becomes "由模型 <backend>".
- Judgment line (script): the threshold is not used. When `judge.cache` is true, append "（结果会缓存）" ("results are cached") to the line.
- Action description: `inject` → "在 AI 继续之前，把规则原文放进它的上下文" (put the rule text into the AI's context before it continues); `block` → "拦下这次工具调用，把规则原文作为理由交给 AI，让它按规则改正后再试" (block this tool call, give the rule text to the AI as the reason, and have it fix the call and retry).
  When an inject unit's events contain only `stop`, use "在 AI 结束本轮之前提醒它一次，由它判断规则是否适用" (remind the AI once before it ends the turn; it decides whether the rule applies).
  When a block unit's events contain `tool_result` but not `tool`, use "工具结果已经产生；阻止工作流继续，把规则原文交给 AI，要求先修复结果" (the tool result already exists; stop the workflow and require a fix).
