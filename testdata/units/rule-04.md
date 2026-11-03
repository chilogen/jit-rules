---
title: prompt condition on tool event
trigger:
  events: [tool]
  tools: [write]
  prompt: [review]
examples:
  - expect: hit
    event: {kind: tool, tool: write}
  - expect: miss
    event: {kind: tool, tool: write}
---
body
