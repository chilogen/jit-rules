---
title: 问候时提醒保持简洁
trigger:
  events: [prompt]
  prompt: ["你好|hello"]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: prompt, prompt: "你好"}
  - expect: miss
    event: {kind: prompt, prompt: "再见"}
---
回答问候时保持简洁。
