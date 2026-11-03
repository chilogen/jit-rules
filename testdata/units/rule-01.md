---
trigger:
  events: [prompt]
judge:
  question: Is this a valid question?
examples:
  - expect: hit
    event: {kind: prompt}
  - expect: miss
    event: {kind: prompt}
---
missing title
