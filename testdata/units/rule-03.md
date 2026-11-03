---
title: tool condition on prompt event
trigger:
  events: [prompt]
  tools: [write]
examples:
  - expect: hit
    event: {kind: prompt}
  - expect: miss
    event: {kind: prompt}
---
body
