---
title: block on prompt event
trigger:
  events: [prompt]
judge:
  question: Should this trigger?
deliver:
  as: block
examples:
  - expect: hit
    event: {kind: prompt, tool: write}
  - expect: miss
    event: {kind: prompt, tool: write}
---
body
