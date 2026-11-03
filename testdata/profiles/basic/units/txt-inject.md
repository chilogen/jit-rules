---
title: Text file guidance
trigger:
  events: [tool]
  paths: ["**/*.txt"]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: tool, tool: write, paths: [note.txt]}
  - expect: miss
    event: {kind: tool, tool: write, paths: [note.md]}
---
Keep text files concise.
