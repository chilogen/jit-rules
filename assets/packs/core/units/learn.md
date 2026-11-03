---
title: 用户纠正或重复强调时，提议记成规则
trigger:
  events: [prompt]
judge:
  question: Does the current message explicitly correct the agent's prior behavior, or explicitly say that a work instruction is being repeated?
  not: The message states a requirement without framing it as a correction or repetition.
deliver:
  as: inject
topic: core.authoring
examples:
  - expect: hit
    event: {kind: prompt, prompt: "我说过了，设计文档里不要写不做什么"}
  - expect: hit
    event: {kind: prompt, prompt: "Again, don't use relative imports."}
  - expect: miss
    event: {kind: prompt, prompt: "帮我加一个登录页面"}
---
如果用户在当前消息中明确纠正你，或者明确表示某项要求已经说过：先按要求完成当前任务；完成后问用户一句，要不要把这条要求记成规则、以后自动提醒。用户同意后，运行 `jitr guide` 并按其中的流程操作。如果不是，忽略这条提示。
