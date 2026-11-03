# 如何编写判断单元（给 AI 看）

你在帮用户把一条"以后都要遵守的要求"做成判断单元。jitr 会在合适的时刻把单元里的规则自动送到 AI 面前。用户不会写脚本，也不会自己写判断问题，这些都由你来做；用户只负责最后确认。

## 流程（按顺序做，不要跳步）

1. **先找有没有现成的。** 运行 `jitr unit list --all --json`。已经有相近的单元时，修改它，不要新建。用 `jitr profile list --json` 查看 profile 的名字；有多个时问用户放进哪一个。
2. **写单元。**
   - 新建：`jitr unit new <profile> <名字> --from-stdin`，从 stdin 传入完整文件内容（格式见下文）。名字只能用小写字母、数字、`.`、`-`，例如 `git.commit-zh`。
   - 修改：直接编辑 `jitr unit list --all --json` 输出的 `file` 路径指向的文件。
   - 新建或修改后，这个单元都处于**未生效**状态，不会送达。
3. **测试。** 运行 `jitr unit test <profile>/<名字>`，直到全部示例通过。没通过时，修改触发条件、判断问题或示例，不要删掉没通过的示例来凑数。
4. **回放历史。** 运行 `jitr unit replay <profile>/<名字> --since 14d`，查看它在过去真实的事件中会在哪里触发。如果触发在明显不相关的地方，就收紧触发条件；如果该触发的地方没触发，就放宽。回放只覆盖记录过的事件；约束写出内容的单元，还要把条件（或脚本）在项目里现有的同类文件上跑一遍，逐条看命中的是不是真的违规。改完回到第 3 步。
5. **检查冲突。** 运行 `jitr unit check <profile>/<名字>`。报告了冲突时，按报告里的建议处理（设置相同的 `topic`、加 `replaces`，或者收紧触发条件），然后回到第 3 步。
6. **给用户确认。** 运行 `jitr unit show <profile>/<名字>`，把输出原样给用户看，问："这样可以吗？"
7. **生效。** 用户同意后，运行 `jitr unit enable <profile>/<名字>`。用户不同意时，按用户的意见修改，再回到第 3 步。

## 单元文件格式

```markdown
---
title: commit message 用中文写
trigger:
  events: [tool]
  tools: [shell]
  shell: ['git\s+commit\b.*\s-[a-z]*m\s*["'']?[A-Za-z]']
deliver:
  as: block
topic: git.commit-language
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m "fix bug"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git commit -m "修复登录错误"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git log'}
---
commit message 用中文写，第一行不超过 50 个字。
```

frontmatter 下面的正文就是会被送达的规则原文：写一两句话、直接写要求，不要写原因。

### 字段说明

| 字段 | 写法 |
|---|---|
| `title` | 一句话说明这条规则管什么（给人看），必填 |
| `trigger.events` | 必填。`prompt`=用户发消息时；`tool`=AI 调用工具之前；`stop`=AI 结束本轮回复之前；可以写多个 |
| `trigger.tools` | 只在这些工具上检查。可选值：`shell` `write` `edit` `read` `search` `web` `agent` `skill`，或 `mcp/<服务>/<工具>`。只有 events 包含 `tool` 时才能写 |
| `trigger.paths` | 只在操作这些文件时检查，写相对项目根目录的通配符，如 `"**/*.md"`。只有 events 包含 `tool` 时才能写 |
| `trigger.shell` | 只在命令匹配这些正则时检查。只有 events 包含 `tool` 时才能写 |
| `trigger.content` | 只在即将写出的内容匹配这些正则时检查；在 `stop` 上匹配 AI 本轮的最后一段回复。只有 events 包含 `tool` 或 `stop` 时才能写 |
| `trigger.prompt` | 只在用户消息匹配这些正则时检查。只有 events 包含 `prompt` 时才能写 |
| `judge.question` / `judge.not` | 可选：让模型判断一个是非问题，写法见下文 |
| `judge.backend` | 可选：这条单元用哪个模型后端（`settings.backends` 里的名字），不写用默认后端 |
| `judge.cache` | 可选：是否缓存判断结果。模型判断默认缓存，脚本判断默认不缓存 |
| 正文里的 ```` ```sh jitr-judge ```` 代码块 | 可选：脚本判断，见下文。和 `judge.question` 二选一 |
| `deliver.as` | `inject`（默认）=把规则放进上下文；`block`=拦下这次工具调用，让 AI 按规则改正后再试 |
| `topic` | 可选：冲突域，见下文 |
| `replaces` | 可选：用这条单元替换掉其他单元，写它们的全局 id，如 `[team/git.commit-en]` |
| `examples` | 必填，至少一条 `hit` 和一条 `miss`。`event` 里写 `kind`（`prompt`、`tool` 或 `stop`），再加上 `tool`、`effect`、`paths`、`shell`、`content`、`content_unknown`、`prompt` 中用得到的几项 |

`trigger` 里写的所有条件都满足才会检查；写了 `judge` 问题就还要模型回答"是"，写了脚本就还要脚本退出码为 0。除 `events` 外，`trigger` 至少写一项，或者写判断。

### 脚本判断

条件取决于事件里看不到的环境状态时（分支离 master 多远、某个服务有没有在跑），用脚本判断，不要让模型去猜。在正文里放一个代码块，语言后面加上 `jitr-judge`：

````markdown
---
title: 分支落后太多时先合并 master
trigger:
  events: [tool]
  tools: [shell]
  shell: ['^git\s+commit\b']
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m "x"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
```sh jitr-judge
[ "$(git rev-list --count "$(git merge-base HEAD master)"..HEAD)" -gt 3 ]
```
距离 master 分叉已超过 3 个 commit，先合并 master 再提交。
````

- 代码块的第一个词是解释器（`sh`、`bash`、`python3`、`node`、`pwsh` 等），必须在 PATH 里。jitr 在项目目录运行它，stdin 是事件的 JSON。
- 按用户实际用的平台选解释器：只在 Linux / macOS 上用，写 `sh` 就够；要在 Windows 上也生效，用 `node`、`python`（Windows 上通常叫 `python` 而不是 `python3`）或 `pwsh`。解释器找不到或无法运行时，这条规则不生效，用户会收到一次提示。
- 退出码 0 = 命中，1 = 不命中，其他 = 出错（出错时规则照样送达，但不会拦截）。
- 这个代码块不会送达给 AI，正文其余部分照常送达。
- 脚本以用户的权限运行，和用户自己的 hook 一样；`jitr unit show` 会把脚本完整展示给用户确认。
- 每次通过 trigger 的事件都会运行一次脚本，所以 trigger 要写窄。脚本可以有副作用（例如直接给用户发通知），但要能承受被反复执行。
- 示例测试和回放都不运行脚本：带脚本的示例只检查 trigger，会显示为 skipped。脚本本身请先在项目目录里手动跑一遍确认。

### 工具和内容是怎么看到的

- `write` 的 `content` 是**整个文件**；`edit` 的 `content` 只是**改进去的片段**。约束整个文件的规则（例如"第一行必须是标题"）写 `tools: [write]`，否则会把编辑片段的开头误当成文件开头。
- AI 用 shell 命令写文件时（`cat > a.md <<'EOF'`、`printf ... >> a.md`、`echo ... > a.md`、`tee`），jitr 会认出写入的文件和内容：`tool` 仍是 `shell`，`effect` 是 `write`（新建或覆盖）或 `edit`（追加到已有文件），`paths` 和 `content` 照常填写。`tools: [write]` 也会匹配这种命令。
- 能认出在写哪个文件、却看不出写了什么时（例如内容来自变量），`content_unknown` 为 true：写了 `content` 条件的单元视为满足，但只会注入、不会拦截。

## 怎么写好

- **判断交给看得到所需信息的一方。** 模型判断只看得到事件本身（工具、路径、命令、写出的内容，`edit` 只有片段），看不到项目里的文件、文档和环境状态。条件能靠查这些东西确定的，用脚本去查；还要结合项目理解才能判断的，不要做成判断条件，写进正文，让 AI 自己去对照。
- **触发条件尽量窄。** 能用 `tools`、`paths`、`shell`、`content` 写清楚的，就不要用 `judge`；取决于环境状态的，用脚本判断。`judge` 每次都要联网调用模型；`events` 包含 `tool` 时，最好同时写上 `tools` 或 `paths`，不然每次工具调用都会触发一次联网判断。
- **judge 的问题**要写成一个用英文表达的是非问题，问的是"AI 正要做的这件事"，而不是问规则本身。好的写法：`Is the agent writing a design document?`；不好的写法：`Should the agent follow the design doc rule?`。`not` 用来写容易被误判为"是"的情况。
- **inject 和 block 怎么选。**
  - 规则要在 AI 动手之前就知道（例如"设计文档只写决策"）：用 `inject`，`events` 写 `prompt`（用户提出任务时就送到），或者写 `tool` 加上相关的 `paths`。注意：工具调用时注入的规则，要等这次调用执行完才到 AI 面前，只能影响之后的操作。
  - 规则约束的是"即将写出的内容本身"（文档格式、commit message、代码风格），并且内容已经出现在工具参数里：用 `block`。
  - 条件只能近似区分违规和合规、会把合规的写法也判成命中时，只用 `inject`：误拦会让合规的操作做不下去，误注入只是多一段提醒。
  - 规则要在一轮工作做完时才适用（例如"改了文件就留档"）：`events` 写 `stop`，只能用 `inject`。AI 结束本轮前会收到一次规则原文，由它判断是否适用；同一轮只提醒一次。要避免纯聊天也被提醒，加一个看环境状态的脚本判断（例如 `git status --porcelain` 非空才命中）。
- **block 的条件必须描述"违规"，不能只描述"在做某类事"。** 拦截对每一次满足条件的调用都生效：AI 改对之后，条件必须不再成立，否则会被一直拦下去。例如"commit message 用中文"：条件写"commit message 以英文字母开头"，而不是"在执行 git commit"；"Markdown 第一行必须是一级标题"：条件写"内容的开头不是 `# `"，而不是"在写 md 文件"。用 judge 时也一样，问题要问"是否违反"。
- **示例**要像真实情况：hit 写一两个典型场景，miss 写最容易被误触发的场景。block 单元必须有一条"改对之后"的 miss 示例：和某条 hit 示例是同一个工具，只是内容合规。
- **topic**：两条单元讲的是同一件事的不同要求时（比如 commit message 用中文、commit message 用英文），给它们设置相同的 topic；同一个事件上，只有排位最高的那条会生效。排位规则：用户自己的单元 > profile 里排在前面的包 > 排在后面的包。
- **replaces**：用户想用自己的版本替换某个包里的单元时，用 `replaces` 写上那个单元的 id。被替换的单元在这个 profile 里完全不再生效。

## 被拦截时

你的工具调用被 jit-rules 拦下时，理由里写着违反的规则。按规则改正后重新发起；如果你确认规则不适用于这次操作（例如用户明确要求了与规则相反的做法），停下来告诉用户是哪条规则、为什么冲突，让用户决定，不要原样重试。
