---
title: content filter with inject
trigger:
  events: [tool]
  tools: [write]
  content: [secret]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: tool, tool: write}
  - expect: miss
    event: {kind: tool, tool: write}
---
body
