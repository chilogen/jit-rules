---
title: CRLF file
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
line one
line two
