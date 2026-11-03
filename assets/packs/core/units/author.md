---
title: 用户要新增或修改规则时，按 jitr 的流程来做
trigger:
  events: [prompt]
judge:
  question: Is the user asking to add, change, remove, or review a rule, habit, or preference that the agent should follow in future work?
  not: The user only asks for a one-off change to the current task.
deliver:
  as: inject
topic: core.authoring
examples:
  - expect: hit
    event: {kind: prompt, prompt: "以后写 commit message 都用中文"}
  - expect: hit
    event: {kind: prompt, prompt: "Remember to always run the tests before committing."}
  - expect: miss
    event: {kind: prompt, prompt: "把这个函数改成异步的"}
---
如果用户是在要求新增、修改或删除一条以后都要遵守的规则：不要把它写进 CLAUDE.md、AGENTS.md 或记忆文件；先运行 `jitr guide`，然后严格按其中的流程把它做成一个判断单元。如果不是，忽略这条提示。
