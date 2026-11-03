---
title: unbounded tool judge
trigger:
  events: [tool]
  shell: [git]
judge:
  question: Should this trigger?
examples:
  - expect: hit
    event: {kind: tool, tool: shell}
  - expect: miss
    event: {kind: tool, tool: shell}
---
body
