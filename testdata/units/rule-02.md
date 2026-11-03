---
title: invalid regular expression
trigger:
  events: [prompt]
  prompt: ['[']
examples:
  - expect: hit
    event: {kind: prompt}
  - expect: miss
    event: {kind: prompt}
---
body
