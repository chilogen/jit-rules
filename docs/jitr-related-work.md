# JITR：团队级可执行记忆、相关工作与证据路线

更新日期：2026-11-03

文档状态：持续更新。后续将结合日常工程 trace、规则迭代记录和公开评测任务补充实证结果。

## 执行摘要

JITR 当前更适合被定义为一项工程实践，而不是一种新的基础思想：

> **JITR 是团队级可执行策略记忆的运行时分发与约束层。它保存团队统一的反模式和工程规则，在相关事件发生时即时检索、注入或阻断，并用 trace 支持离线的组织学习。**

模型厂商、agent harness 和公开研究已经分别实现了按需上下文、生命周期 hooks、tool guardrails、trajectory logging、LLM grading 和在线 monitor 等组件。公开资料没有显示 JITR 发明了这些机制；目前也没有发现一个公开系统完整覆盖 JITR 的跨 harness 规则协议、确定性 gate、per-rule judge、执行前干预、真实事件 replay、冲突检查和离线规则治理。这里的价值首先来自组合、可运营性和后续实证，而不是思想命名。

JITR 试图解决的是团队使用 coding agents 后出现的组织问题：

- 每个成员和 harness 形成不同的个人记忆，导致同一团队的产出标准不一致；
- 把所有规范和反模式常驻在 `CLAUDE.md`、`AGENTS.md` 或 system prompt 中，会持续占用上下文，并在长任务中被稀释；
- 某个成员踩过的坑不会自动成为其他成员和其他 harness 的经验；
- 仅有最终结果评测，无法解释规则何时被需要、是否成功纠偏，以及团队是否逐渐减少重复犯错。

JITR 不应被描述为通用 Process Reward Model。它的模型 judge 当前只回答一个更窄的问题：**当前可观测事件是否匹配某条团队规则**。`unit test` 和 `replay` 主要验证路由行为，尚不能证明 JITR 提高了整体任务成功率或过程合规率。[cite:1][cite:2]

后续故事需要从实践中长出来，而不是先假设结论。最核心的待验证命题是：

1. 按事件交付规则，能否在减少常驻上下文的同时保持或提高合规率；
2. 团队共享规则，能否降低不同成员、模型和 harness 之间的产出差异；
3. 同一规则反复触发后，harness 自身的记忆机制是否会使违规复发率下降；
4. 聚合 trace 能否稳定发现共性反模式，并经过人工治理转化为有效的新规则；
5. 上述收益能否覆盖 judge 延迟、额外 token、误注入和误阻断的成本。

## 问题定义：团队记忆而不是个人记忆

Agent harness 普遍提供 instruction、skill、memory 或 session continuation 等机制，但这些机制通常服务于单个用户、单个项目或单个 harness。即使它们都被称为“记忆”，保存的对象、更新时机和传播边界也不同。[cite:3][cite:8]

| 记忆层 | 典型载体 | 主要作用 | 主要局限 |
|---|---|---|---|
| 个人/会话记忆 | harness memory、session summary | 适应个人习惯和当前任务 | 成员之间不一致，难以审计和迁移 |
| 项目常驻记忆 | `CLAUDE.md`、`AGENTS.md` | 为仓库提供稳定背景 | 所有规则持续占用上下文 |
| 团队策略记忆 | JITR units / packs | 统一反模式，并按事件交付或执行 | 需要可靠触发、治理和版本控制 |
| 组织证据 | journal、trace、评测结果 | 发现重复问题并评估规则效果 | 不能未经审核直接变成运行时政策 |

因此，JITR 不是另一个通用 memory store。它更接近一种**可执行的程序性记忆**：规则不仅描述团队知道什么，还描述它何时适用、怎样交付、何时必须阻断，以及如何通过正反例和 replay 验证。

真正无法定义触发条件、必须始终成立的少量原则仍应常驻；能够绑定到 prompt、tool、path、shell、content 或环境状态的规则才适合进入 JITR。[cite:1]

## 规则的分层承载

JITR 不应简单地被设定为所有 `Skill`、`AGENTS.md` 或 `CLAUDE.md` 规则的重复兜底，也不应机械地独占所有团队知识。更合理的方式是按规则性质确定唯一的主要承载层：

| 规则类型 | 主要承载位置 | 原因 |
|---|---|---|
| 无法定义触发条件、始终需要知道的短原则 | `AGENTS.md` / `CLAUDE.md` | 任何时刻都不能缺席 |
| 从规划阶段开始持续需要的完整任务流程 | Skill | Agent 需要主动选择并执行一整套步骤 |
| 能绑定到 prompt、tool、path、shell、content 的反模式 | JITR | 只在相关事件送达，避免长期占用上下文 |
| 高代价、不可逆的违规 | Skill 或常驻规则负责正向指导，JITR 负责执行前检查 | 两层承担不同职责，形成纵深防御 |

同一条规范不应在多个位置手工维护完全相同的正文，否则会产生版本漂移、冲突和无法确认来源的问题。关键规则可以出现在两层，但语义角色应不同：

```text
Skill / AGENTS.md：告诉 agent 应该如何完成任务
JITR：检查即将发生的具体动作是否已经违规
```

例如，“发布版本的完整流程”适合放在 release skill；“禁止未通过测试就执行发布命令”适合由 JITR 在命令执行前检查。“Markdown 第一行必须是一级标题”可以只由 JITR 在写文件时检查，没有必要常驻。

新发现的反模式默认先进入 JITR，而不是立即增加所有人的常驻上下文。后续根据 trace 调整承载层：

1. 低频且触发条件明确：继续只由 JITR 承载；
2. 高频并影响整个任务规划：把正向流程沉淀到 Skill，必要时保留 JITR 违规检查；
3. 几乎所有任务都需要：提炼成很短的常驻原则；
4. 违规代价很高：即使已有 Skill 或常驻指导，也保留 JITR 执行前约束。

Harness 自身 memory 可以逐渐吸收高频反馈，但只能视为本地缓存。JITR 仍是团队规则的 source of truth；是否晋升、降级或退役一条规则，应由离线 trace 和人工审核决定。

## 当前触发与交付能力

JITR 当前是一个灵活的**事件级规则系统**。它先用低成本确定性条件缩小候选集，再按需执行模型或脚本判断：

```text
prompt / tool / tool_result event
  -> event kind
  -> deterministic gate
  -> optional LLM or script judge
  -> topic / priority arbitration
  -> redelivery filter
  -> inject / block / none
```

### 确定性触发条件

一个 unit 可组合以下条件：[cite:1][cite:2]

| 条件 | 当前语义 |
|---|---|
| `events` | `prompt`、`tool`、`tool_result`，可以同时声明 |
| `tools` | 对归一化工具名做 doublestar glob；也会匹配 shell 分析得到的 `write` / `edit` effect |
| `paths` | 事件中任一路径与任一 doublestar glob 匹配 |
| `shell` | shell 命令匹配任一 Go RE2 正则 |
| `prompt` | 用户消息匹配任一 Go RE2 正则 |
| `content` | 即将写入的内容匹配任一 Go RE2 正则 |

不同字段之间是 **AND**：写出的每类条件都必须成立。同一字段的多个 pattern 之间是 **OR**：命中任意一个即可。没有 judge 时，通过 gate 就算命中。

Harness adapter 会把原生工具归一化为 `shell`、`write`、`edit`、`read`、`search`、`web`、`agent`、`skill` 或 `mcp/...`。对于重定向、heredoc、`printf`、`echo` 和 `tee` 等 shell 写文件方式，JITR 会保守地提取目标路径、写入内容和 `write` / `edit` effect，因此文件规则不只依赖 harness 的专用写文件工具。[cite:1]

### 模型与脚本判断

确定性 gate 之后，每条 unit 可以选择一种进一步判断：

- **LLM judge**：回答该 unit 的 yes/no `question`，支持 `not` 排除说明、per-unit backend、概率阈值和可选 cache；
- **Script judge**：在项目目录运行 unit 内的 `jitr-judge` 代码块，事件 JSON 从 stdin 传入；退出码 0 表示命中，1 表示不命中。

脚本判断可以读取 git 分支、文件系统、服务状态等事件本身没有携带的环境信息，表达能力很强，但它以用户权限运行，需要像普通 hook 一样审阅。模型或脚本临时失败时可以降级为提醒，但不能据此阻断；后端长期未配置时，相关模型规则不生效并通知用户。[cite:1][cite:2]

### 交付与冲突语义

- `inject`：在用户 prompt 事件上可影响当前一轮生成；在 tool 或 tool_result 事件上只能影响下一次生成，不能撤回已经发起或执行的工具调用；
- `block`：用于 tool 事件时在动作执行前退出并把规则作为纠正理由交给 agent；用于 tool_result 时工具已经执行，只能要求 agent 先修复结果再继续，不能撤销工具调用；
- 同一 `topic` 下只保留优先级最高的 unit；
- 用户 unit 高于 pack，profile 和 pack 顺序继续决定优先级；
- `replaces` 可以让一个 unit 完全替换另一个 unit；
- injection 按 session byte distance 控制重复送达，避免频繁占用上下文；
- blocking rule 不受 redelivery 抑制，每一次仍然违规的调用都会被阻断；
- 内容不可见或 judge 结果不确定时，只允许 inject，不允许 block。[cite:1][cite:2]

### 能力边界

这种设计对当前事件的工具、文件、命令、内容和语义组合很灵活，但当前原生 DSL 仍有明确边界：

- 不支持任意嵌套的布尔表达式；字段间固定为 AND、字段内固定为 OR；
- 可触发规则的原生事件包括 `prompt`、`tool` 和 `tool_result`；`tool_result` 的工具、路径、命令和内容条件仍从 `tool_input` 归一化，`tool_response` 只用于字节计数，不能据此检查工具成功、失败或输出内容；
- 部分 harness 的原始 `PostToolUse` payload 确实带有 `tool_response` 和 `tool_use_id`，但格式不统一，失败调用是否产生同类事件也不能由当前协议保证；
- 当 harness payload 提供调用 id 时，journal 保存由它得到的 dedupe key，可关联对应的 pre/post event；该能力依赖各 harness 的 payload，仍没有 assistant message、计划或完成声明事件；
- 不直接表达“最近 K 步”“连续失败 N 次”“先发生 A 再发生 B”等时序条件；
- 不原生读取完整 trajectory，也不判断循环、目标漂移或长期遗忘；
- shell 写入识别是保守静态分析，动态变量和复杂脚本可能只能得到 `content_unknown`；
- `edit` 的 content 是修改片段，不是整个文件；约束完整文件时需要限定适当工具；
- tool_result 发生时工具已经执行，任何交付都无法撤回该调用；需要执行前约束时必须在 tool 事件上 block。

因此，当前 JITR 可以概括为：**event-local 很灵活，trajectory-level 首先受限于 observability 和 event correlation，而不只是缺少状态字段。** 只有 harness 确实提供并由 adapter 归一化了所需信号，bounded process state 才有可靠的数据来源；拿不到的 assistant 行为和环境结果不能靠计数器补出来。

## 三个时间尺度的记忆循环

JITR 将团队学习拆成三个不同时间尺度，避免在运行时自动修改团队规则。

```text
快速循环：运行时执行
event -> deterministic gate -> per-rule judge -> allow / inject / block -> trace

中速循环：Harness 个体学习
反复接收规则反馈 -> harness 更新个人或项目记忆 -> 后续主动规避

慢速循环：团队组织学习
trace 聚合 -> 发现共性反模式 -> 候选规则 -> 人工审核
           -> shadow / replay -> 版本化发布 -> 全团队生效
```

### 快速循环：运行时只读

运行时只读取已经审核和版本化的规则。事件先经过工具、路径、命令、内容等确定性 gate，只有剩余语义条件才交给模型或脚本 judge。命中后规则以 `inject` 或 `block` 方式交付；所有候选、判断、延迟和结果写入 journal。[cite:1][cite:2]

运行时不应根据单次事件自动创建或修改团队规则。这个约束避免偶然行为、prompt injection、judge 误报或局部偏好直接污染团队政策。

### 中速循环：Harness 自身记忆

JITR 不需要负责写入每个 harness 的私有 memory。一个 harness 如果会从重复反馈中更新自身记忆，那么高频规则可能逐渐成为它的本地习惯，后续无需等待 JITR 阻断才会遵守。

这是一项需要实测的行为假设，而不是可直接依赖的协议保证。不同 harness 是否自动记忆、记忆保存多久、能否跨项目复用以及是否会被新上下文覆盖，都可能不同。JITR 仍然是团队规则的 source of truth，本地记忆只能视为可能命中的缓存。

### 慢速循环：Trace 驱动的离线治理

团队定期分析 trace，从多人、多项目和多 harness 的重复事件中发现候选反模式。候选规则需要补充触发条件、正反例、交付方式和独立 scorer，再经过 replay、shadow 和人工审核后发布。

这使信息形成双向但非实时的流动：

- **JITR → Harness**：团队规则通过运行时反馈传播到个人行为和本地记忆；
- **Harness → 团队**：个体行为通过 trace 汇总，成为候选组织经验；
- **团队 → JITR**：经验证的经验被版本化为新规则，再传播给所有成员。

严格地说，这不是 memory 文件之间的直接同步，而是一个**异步收敛的组织学习过程**。

## 产品与模型厂商实践

公开产品已经验证了 JITR 使用的大部分基础机制，但它们服务于不同抽象层。

### Claude Code conditional rules 与 hooks

Claude Code 的 `.claude/rules/*.md` 支持按路径加载规则；官方 hooks 可以在生命周期事件上运行脚本、prompt 或 agent，并允许 `PreToolUse` 在执行前阻断调用并把原因反馈给模型。[cite:3][cite:4]

这是与 JITR 最接近的公开产品机制。共同点包括按需加载、事件触发、语义判断、执行前阻断和纠正反馈。主要差异是：

- Claude 的能力属于单一 harness；JITR 把规则做成跨 Claude Code、Codex、TRAE 和 DeepSeek 的外部协议；
- JITR 同时处理 prompt、tool、path、shell、content 和环境脚本，而不是主要依赖路径；
- JITR 把 rule body、trigger、examples、judge、delivery、测试、replay、冲突和版本 hash 放进同一个可审阅单元；
- JITR 将团队 profile 和 pack 作为规则传播边界，而不是逐个项目配置 hook。

因此，Claude Code 证明了运行时机制可行，但没有直接提供团队级、跨 harness 的策略记忆。

### OpenAI Agents SDK Tool Guardrails

OpenAI Agents SDK 的 tool guardrails 可在 function tool 调用前后运行。输入 guardrail 能在执行前拒绝调用，并把拒绝内容返回给模型；应用也可以用另一个模型做分类。[cite:5]

它与 JITR 都在副作用边界做外部判断。区别在于 OpenAI guardrail 是应用开发者写入 workflow 的代码，而 JITR 面向现成 coding harness，把政策表达为用户和团队可安装、测试及版本化的规则数据。

### Just-in-time context engineering

Anthropic 将 context engineering 描述为对有限注意力预算的管理，并建议保留轻量引用、运行时再加载所需信息，而不是预先把全部材料塞入上下文；Claude Code 的文件检索被用作 just-in-time context 的例子。[cite:6]

JITR 沿用了相同动机，但把“是否检索”从执行模型移到外部 hook：模型不需要先记得存在某条不可见规则，规则由事件和 judge 主动送达。

## 模型长任务评测与在线监控

模型厂商做 agent 长任务评测时，通常会在模型调用、工具调用、handoff、guardrail 和环境变化上采集完整 trace；但公开评测框架大多在试次结束后读取 trace 或最终环境状态评分，而不是把 grader 的意见实时注回当前 rollout。

OpenAI Agent Evals 将 model calls、tool calls、guardrails 和 handoffs 记录为端到端 trace，再由 grader 做结构化评分，用于发现回归并改进后续版本。[cite:7] Anthropic 也把 transcript 定义为一次 trial 的完整记录，说明 coding-agent eval 通常先完成 agent loop，再用单测、环境状态或 transcript grader 评分。[cite:8] METR 使用 Inspect/Hawk 运行自主任务并记录 agent 行为；其旧平台 Vivaria 还支持中途编辑运行以观察反事实，但这主要用于诊断，不是标准能力分数。[cite:9]

在线干预并非不存在。Anthropic 公开披露，其内部研发 agent 的动作会经过在线 monitor，在执行前检查并可 block 或 steer；离线 monitor 则事后分析活动并升级异常。[cite:10] 这一控制结构与 JITR 非常接近：

```text
action event -> hook -> policy / judge -> allow / block / steer -> execute
```

判断目标不同：安全 monitor 关注危险行为，JITR 关注当前事件是否触发团队工程规则。这个证据说明 JITR 的控制拓扑不是新的，但将其用于跨 harness 的团队反模式传播仍是一个有待实证的产品方向。

纯能力评测与在线干预必须分开。grader 如果在 rollout 中提供额外提示，评测对象就从“模型能力”变成“模型 + harness + monitor”的完整系统。JITR 后续实验因此需要同时保留 shadow 模式和 intervention 模式。

## 相关研究及证据权重

相关论文的存在不能自动证明 JITR 值得写成论文。这个领域包含大量新近预印本和 workshop 工作，方法增量、复现程度与引用影响差异很大。相关工作应按“能提供什么证据”使用，而不应平铺成同等可靠的学术背书。

| 工作 | 可借鉴内容 | 与 JITR 的关键差异 | 当前证据定位 |
|---|---|---|---|
| SWE-PRM [cite:11] | 周期性观察近期轨迹并注入纠偏反馈 | 固定错误 taxonomy、trajectory window、只 steer | 相邻 workshop 工程实践，不能作为核心学术背书 |
| OctoBench [cite:14] | 用二元 checklist 评分 repository-grounded agent 的过程合规 | 事后完整轨迹评分，不做在线交付 | 最适合借鉴的公开过程评测结构 |
| AgentProcessBench [cite:15] | 人工 step labels 与 judge 元评测 | 判断步骤质量，不判断团队规则适用性 | 用于设计 judge 校准和错误切片 |
| ToolPRMBench [cite:16] | tool-use trajectory 的局部与多步错误 | 面向 PRM benchmark，不是规则系统 | 支持“确定性 gate 优先、语义 judge 补充” |
| AgentRewardBench [cite:17] | 专家标注任务成功、副作用和循环，并比较 judges | web agent、事后评分 | 说明 judge 必须单独验证，不能视为真值 |
| Agent-as-a-Judge [cite:18] | evaluator agent 查看轨迹并检查环境产物 | 完整任务评估，不做执行前规则匹配 | 支持环境证据增强评测 |
| ProcessBench [cite:19] | 定位最早错误步骤 | 数学推理、封闭状态、无工具副作用 | 提供 first-error 诊断思想 |
| MAST [cite:20] | 从 traces 提炼多 agent 失败 taxonomy | post-hoc 诊断，不在线干预 | 参考离线反模式发现方法 |

### SWE-PRM 的合理位置

SWE-PRM 的结构与 JITR 都包含“观察进行中的行为 → 独立 judge → 反馈给原 agent”，但“定期调用另一个模型纠偏”并不是新的基础思想。该工作公开状态是 NeurIPS 2025 SEA Workshop paper，而不是 NeurIPS 主会；截至本次更新，OpenAlex 记录的引用量也很低。[cite:11][cite:12][cite:13]

它可以作为相邻实现和实验设计参考，但不应被称为权威基线，也不能用它的存在反推 JITR 具备论文价值。更重要的问题是：JITR 是否能在团队规则和跨 harness 场景中产生可复现、可推广的经验结论。

### 过程评测工作的共同启示

OctoBench 将任务成功与逐项约束合规分开；AgentProcessBench 和 ToolPRMBench 关注 step-level 判断；AgentRewardBench 表明不同 judge 在不同 benchmark 上表现不稳定。[cite:14][cite:15][cite:16][cite:17]

这些工作共同支持四个评测原则：

1. 最终产物正确不代表过程符合约束；
2. judge 自身需要人工标注和独立 benchmark；
3. 任务成功、规则合规、副作用、循环和成本应拆开计量；
4. 低层工具约束应优先使用确定性检查，LLM 只处理剩余语义。

完整 trajectory 有利于事后诊断，但不意味着每次热路径判断都应加载完整历史。JITR 当前 event-level judge 更适合原子规则。循环、漂移和未经验证就结束等跨步问题，只有在 harness 暴露 assistant message、工具结果或稳定 transcript 时才能可靠分析；否则无论增加多少 session state 都只能得到不完整推断。

## 定位边界

JITR 的差异不在“使用 judge”，而在它管理的对象和组织边界。

| 机制 | 保存或判断的对象 | 传播范围 | 是否在线干预 | 与 JITR 的关系 |
|---|---|---|---|---|
| Harness memory | 个体习惯、会话和项目经验 | 单用户或单 harness | 通常通过后续生成体现 | JITR 的潜在本地缓存 |
| 常驻 instruction | 始终加载的项目和团队要求 | 项目或用户 | 通过上下文软约束 | JITR 要减少的常驻负担 |
| Claude hooks / OpenAI guardrails | 生命周期事件和工具风险 | 单一 harness / workflow | 是 | JITR 的底层机制先例 |
| 厂商 trace grader | 完整轨迹或最终状态 | 评测系统 | 通常否 | JITR 的事后评测先例 |
| 厂商 online monitor | 单个动作的安全风险 | 内部 agent 平台 | 是 | 与 JITR 最接近的控制拓扑 |
| PRM / process judge | 一步或一段轨迹的质量 | 模型训练或评测 | 视系统而定 | 相邻研究，不等同于 JITR |
| JITR | 事件与团队规则的适用关系 | 团队、项目和多个 harness | `inject` / `block` | 团队策略记忆的执行层 |

### JITR 是什么

- 团队统一反模式和程序性知识的 source of truth；
- 按事件检索规则、控制上下文占用的 delivery layer；
- 在工具副作用发生前实施确定性或模型辅助检查的 enforcement layer；
- 收集规则触发、采纳、修正和成本信息的 trace layer；
- 支撑离线规则发现、审核、回放和发布的治理基础设施。

### JITR 不是什么

- 不是新的通用记忆理论；
- 不是自动修改自身政策的在线学习系统；
- 不是对隐藏思维链逐步打分的通用 PRM；
- 不是每个 harness 私有 memory 的替代品；
- 不是所有团队文档的检索系统；
- 不是仅凭已有实现就已经成立的论文贡献。

能够留下独立于实现的普遍结论，才构成研究价值。例如：常驻规则在多长轨迹后开始失效、JIT delivery 在什么条件下产生净收益、多少次运行时反馈会被 harness 的本地记忆吸收，以及怎样的 trace 信号足以发现稳定的团队反模式。

## 待验证假设

| 假设 | 期望现象 | 主要反证 |
|---|---|---|
| H1 上下文效率 | JITR 显著减少常驻规则 token | judge 和重复注入成本抵消节省 |
| H2 合规不降 | JITR 的规则合规率不低于 resident rules | 漏触发或送达过晚导致更多违规 |
| H3 团队一致性 | 不同成员和 harness 的输出方差下降 | harness 差异仍主导最终行为 |
| H4 本地吸收 | 同一 harness 反复收到规则后复发率下降 | 私有 memory 不更新或不稳定 |
| H5 组织学习 | 跨成员 trace 能稳定产生高价值候选规则 | 大多数聚类只反映任务特例或误报 |
| H6 在线约束有效 | block 后出现合规重试且绕过率低 | agent 放弃任务、循环或改用其他工具绕过 |

这些假设目前都不能由 replay 单独证明。Replay 只能回答历史事件在当前规则下是否会命中，属于 routing evaluation；它不能构造“没有注入时会怎样”的反事实。

## 证据积累路线

证据路线分为日常工程实践和公开评测。二者共享同一套事件定义、规则版本和结果指标：日常实践发现问题与生成假设，公开评测提供可控环境和可复现实验。

### 日常工程实践

#### 规则生命周期

新规则只从离线流程进入运行时：

```text
trace cluster
  -> candidate anti-pattern
  -> human review
  -> examples + independent scorer
  -> replay
  -> shadow
  -> inject
  -> block（仅限高置信度、可逆性差的违规）
  -> versioned release / rollback
```

`shadow` 阶段运行 gate 和 judge，但不向 agent 注入任何内容，用于估计命中率和误报。`inject` 可以容忍一定程度的 over-trigger；`block` 必须以高 precision 和低 false-block rate 为前提。运行时始终只消费已经发布的版本。

#### 高频触发的解释

高频不等于规则重要，也不应自动升级为 block。需要结合分布和后续行为解释：

| 观察 | 可能含义 | 后续检查 |
|---|---|---|
| 单个 harness 反复触发 | 本地 memory 没有吸收，或规则表达不清 | 复发间隔、是否同类任务、反馈是否被采纳 |
| 多成员、多 harness 同时高频 | 团队存在系统性反模式 | 跨项目复现、是否可改进默认工具或模板 |
| 首次触发后复发率下降 | harness 可能形成了本地记忆 | 排除任务变化和模型版本变化 |
| 高频触发但很少发现真实违规 | gate 过宽或 judge 误报 | 人工标注 precision、收窄 trigger |
| block 后频繁换工具重试 | 规则可被绕过或事件归一化不完整 | 关联 shell、write、patch 等等价副作用 |
| 长期稳定零触发 | 规则已被吸收、场景消失或规则失效 | shadow 抽样和规则退役评审 |

#### 日常观测指标

| 维度 | 指标 | 回答的问题 |
|---|---|---|
| 团队一致性 | 同任务跨成员/模型/harness 的 compliance variance | 团队产出是否收敛 |
| 本地吸收 | 首次反馈后的 recurrence curve | harness memory 是否逐渐接管 |
| 在线纠偏 | rewrite success、time to recovery、bypass rate | block 是否真正纠正行为 |
| 规则质量 | applicable recall、precision、false-block rate | gate 和 judge 是否可靠 |
| 上下文效率 | resident tokens avoided、injected tokens、redelivery | 是否真的节省注意力预算 |
| 运行成本 | judge token、金额、P50/P95 latency、fallback rate | 收益是否覆盖成本 |
| 组织学习 | 候选规则数、通过审核率、发布后有效命中率 | trace 是否产生可复用知识 |
| 规则健康度 | 冲突、重复、退役和 rollback 次数 | 规则库是否可长期维护 |

需要特别记录“首次收到规则后，后续同类事件是否仍违规”。仅统计总 hit count 无法区分真正的组织学习、任务分布变化和过宽触发器。

### 公开评测任务

公开评测分三个阶段推进，而不是一次性追求完整结论。

#### 阶段一：Judge 与路由评测

从真实 journal 和公开任务中分层抽样事件，由人工标注“某条规则是否适用”。样本覆盖 prompt/tool、不同 harness、gate hit/near miss、inject/block、内容可见/未知，以及 judge 的高低置信区间。

核心指标是 applicable recall、precision、false-block rate、Brier score/ECE、gate reduction 和最差规则切片。模型 judge、prompt、阈值、规则 hash 和 cache namespace 必须版本化。

#### 阶段二：逐事件干预评测

关联一次阻断前后的工具调用，重建可观察链路：

```text
违规候选 -> block -> 合规重试 / 放弃 / 绕路 -> 最终结果
```

核心指标包括 violation prevention、rewrite success、time to recovery、bypass、unnecessary interruption、repeated block 和冲突升级。评价对象是可观察动作和环境变化，不依赖模型私有思维链。

#### 阶段三：长轨迹因果实验

OctoBench 的 checklist 结构适合转成 JITR units；SWE-bench Verified、Terminal-Bench 或其他公开 coding tasks 可用于测量任务完成，但需要额外构造明确的过程规则和 scorer。[cite:8][cite:14]

每个任务至少保留三个实验 arm：

| Arm | 规则呈现方式 | 目的 |
|---|---|---|
| No-rule | 不提供目标规则 | 测量规则本身的增益 |
| Resident | 规则常驻 instruction 文件 | 现有团队实践基线 |
| JITR | 规则只通过 JITR 交付 | 核心 treatment |

可增加 `JITR-shadow` 作为诊断组，但不应混入核心效果比较。实验使用 `jitr isolate` 排除 harness 自带 memory、skills 和 instruction，正式测量 harness 本地吸收时再单独开启这些机制。[cite:1]

实验应固定模型版本、预算、环境快照和规则版本，每个任务重复运行；scorer 对 arm 盲测，并同时报告：

- Task Success：环境目标或测试是否完成；
- Process Compliance：适用 checklist items 的合规率；
- Instance Compliance：一次运行是否零违规；
- Context Saving：减少的常驻 token 与实际注入 token；
- Intervention Cost：judge token、金额和延迟；
- Behavioral Cost：误阻断、放弃、循环和任务成功率下降；
- Cross-harness Variance：同一规则在不同 harness 的效果差异。

### 建议的 Trace 数据

现有 journal 已记录规范化事件、候选 unit、judge 结果、delivery 和延迟。[cite:2] 为支持团队学习和因果评测，还需要逐步补充稳定的运行关联字段。以下是目标结构，不代表当前字段已经全部实现：

```json
{
  "run_id": "...",
  "task_id": "...",
  "actor_hash": "...",
  "harness": "claude-code",
  "model_exact": "...",
  "arm": "resident|jitr|no-rule|shadow",
  "event_id": "...",
  "retry_of": "...",
  "step": 12,
  "rule_id": "team/no-direct-main-push",
  "rule_hash": "sha256:...",
  "judge_prompt_hash": "sha256:...",
  "judge_p": 0.82,
  "decision": "allow|inject|block",
  "human_applicable": true,
  "pre_action_compliant": false,
  "post_action_compliant": true,
  "task_success": true
}
```

`actor_hash` 只用于跨时间统计，不应保留不必要的个人身份；代码、prompt 和 tool arguments 的存储需要遵守仓库敏感信息策略。实验 `arm` 应与给 scorer 的轨迹分离，避免从 JITR 标记推断 treatment。

## 证据成熟度

JITR 的叙事应随证据升级，不提前使用更强结论。

| 等级 | 证据 | 可以声称什么 |
|---|---|---|
| E0 设计 | 协议、实现、单元测试 | 系统具备某项机制 |
| E1 真实 smoke test | 少量真实 harness 运行 | 机制在特定环境可工作 |
| E2 日常观测 | 持续 trace、人工抽样、规则迭代 | 描述真实分布和相关性 |
| E3 内部对照 | Resident / JITR / No-rule 随机或配对实验 | 在控制条件下估计因果效果 |
| E4 公开复现 | 公开任务、脚本、规则和匿名 trace | 外部可验证的经验结论 |
| E5 跨团队复现 | 独立团队、模型和 harness | 具有更强普适性的系统发现 |

当前已有实现、单元测试和少量真实 harness smoke evidence，接近 E1；关于团队记忆、上下文收益和组织学习的结论仍处于待验证状态。[cite:1]

## 论文与产品叙事

JITR 目前首先是一套工程系统。是否值得形成论文，取决于实践后能否留下独立于代码实现的知识，而不是能否找到若干相似论文。

可能成立的经验贡献包括：

- 常驻团队规则随任务长度和上下文距离衰减的可量化规律；
- JIT delivery 相比 resident instructions 的适用区间和失败边界；
- 不同 harness 吸收重复反馈的速度和稳定性；
- 团队 trace 中哪些信号能预测一条候选反模式是否值得发布；
- inject 与 block 在 recall、precision 和用户中断成本上的不同阈值；
- 团队策略记忆如何降低跨成员和跨 harness 的行为方差。

如果实践只能证明“hook 可以调用 judge 并注入规则”，更合适的产物是开源工具、设计文档和工程经验报告。如果公开评测能够稳定回答上述问题，才形成有价值的 empirical systems 或 software engineering research。

对外定位可以保持克制：

> JITR is a cross-harness runtime layer for executable team memory. It delivers and enforces versioned engineering rules at relevant agent events, while traces support offline organizational learning.

## 下一阶段

1. 在日常工程中启用 trace，先建立基线分布，不急于增加大量规则。
2. 给现有规则补充可独立判断的 examples 和 compliance scorer。
3. 建立离线 trace review 流程，记录候选规则为何被接受、拒绝、修改或退役。
4. 增加 shadow 模式和稳定的 event/retry/run 关联字段。
5. 先完成 judge/routing benchmark，再测 block 后的纠偏链路。
6. 选择一组公开 coding tasks，运行 No-rule / Resident / JITR 配对实验。
7. 按证据成熟度更新本文，不把日常 anecdote 写成普遍结论。

## 结论

JITR 的基础组件并不新，其意义也不应建立在 SWE-PRM 等低影响相邻工作的存在上。更可信的故事是：团队在使用不同 agent harness 时缺少统一、低上下文成本、可执行的组织记忆；JITR 把团队反模式变成按事件交付和约束的版本化规则，并把运行 trace 送入离线治理闭环。

这个故事目前仍是假设。接下来的重点不是继续包装概念，而是让日常实践产生真实 trace，让规则治理留下可审计记录，并用公开任务检验上下文节省、规则合规、团队一致性、harness 本地吸收和干预成本。最终叙事应由这些证据决定。

## Sources

[cite:1] JITR design overview and runtime decisions. [README.md](README.md), [design/README.md](design/README.md), [runbook/spec/judge.md](runbook/spec/judge.md)

[cite:2] JITR pipeline, journal, replay and conflict specifications. [runbook/spec/pipeline.md](runbook/spec/pipeline.md), [runbook/spec/unit.md](runbook/spec/unit.md)

[cite:3] Claude Code, “How Claude remembers your project.” https://code.claude.com/docs/en/memory

[cite:4] Claude Code hooks reference and guide. https://code.claude.com/docs/en/hooks and https://code.claude.com/docs/en/hooks-guide

[cite:5] OpenAI Agents SDK, “Guardrails.” https://openai.github.io/openai-agents-python/guardrails/

[cite:6] Anthropic, “Effective context engineering for AI agents,” 2025-09-29. https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents

[cite:7] OpenAI, “Evaluate agent workflows.” https://developers.openai.com/api/docs/guides/agent-evals

[cite:8] Anthropic, “Demystifying evals for AI agents,” 2026-01-09. https://www.anthropic.com/research/demystifying-evals-for-ai-agents

[cite:9] METR, “Resources for Measuring Autonomous AI Capabilities.” https://metr.org/measuring-autonomous-ai-capabilities/

[cite:10] Anthropic, “Measurements for understanding the pace of AI development inside frontier labs,” 2026. https://www.anthropic.com/institute/measuring-pace-of-ai-development

[cite:11] Gandhi et al., “When Agents go Astray: Course-Correcting SWE Agents with PRMs,” 2025. https://arxiv.org/abs/2509.02360

[cite:12] IBM Research, SWE-PRM publication record, marked as a NeurIPS 2025 workshop paper. https://research.ibm.com/publications/when-agents-go-astray-course-correcting-swe-agents-with-prms

[cite:13] OpenAlex record for SWE-PRM, citation snapshot retrieved 2026-11-03. https://api.openalex.org/works/https://doi.org/10.48550/arXiv.2509.02360

[cite:14] Ding et al., “OctoBench: Benchmarking Scaffold-Aware Instruction Following in Repository-Grounded Agentic Coding,” 2026. https://arxiv.org/abs/2601.10343

[cite:15] Fan et al., “AgentProcessBench: Diagnosing Step-Level Process Quality in Tool-Using Agents,” 2026. https://arxiv.org/abs/2603.14465

[cite:16] Li et al., “ToolPRMBench: Evaluating and Advancing Process Reward Models for Tool-using Agents,” 2026. https://arxiv.org/abs/2601.12294

[cite:17] Lù et al., “AgentRewardBench: Evaluating Automatic Evaluations of Web Agent Trajectories,” COLM 2025. https://arxiv.org/abs/2504.08942

[cite:18] Zhuge et al., “Agent-as-a-Judge: Evaluate Agents with Agents,” 2024. https://arxiv.org/abs/2410.10934

[cite:19] Zheng et al., “ProcessBench: Identifying Process Errors in Mathematical Reasoning,” 2025. https://arxiv.org/abs/2412.06559

[cite:20] Cemri et al., “Why Do Multi-Agent LLM Systems Fail?,” 2025. https://arxiv.org/abs/2503.13657
