---
title: no corrected miss example
trigger:
  events: [tool]
  tools: [write]
deliver:
  as: block
examples:
  - expect: hit
    event: {kind: tool, tool: write}
  - expect: miss
    event: {kind: tool, tool: edit}
---
body
