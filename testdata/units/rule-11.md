---
title: question without question mark
trigger:
  events: [prompt]
judge:
  question: This should trigger
examples:
  - expect: hit
    event: {kind: prompt}
  - expect: miss
    event: {kind: prompt}
---
body
