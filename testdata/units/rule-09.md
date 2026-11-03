---
title: empty body
trigger:
  events: [prompt]
judge:
  question: Should this trigger?
examples:
  - expect: hit
    event: {kind: prompt}
  - expect: miss
    event: {kind: prompt}
---
