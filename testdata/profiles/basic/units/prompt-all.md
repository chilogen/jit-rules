---
title: Prompt guidance
trigger:
  events: [prompt]
  prompt: ["."]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: prompt, prompt: hello}
  - expect: miss
    event: {kind: prompt, prompt: ""}
---
Follow the active project rules.
