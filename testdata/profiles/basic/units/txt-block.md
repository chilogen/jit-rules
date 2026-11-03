---
title: Lowercase hello restriction
trigger:
  events: [tool]
  tools: [write, edit]
  paths: ["**/*.txt"]
  content: ["hello"]
deliver:
  as: block
examples:
  - expect: hit
    event: {kind: tool, tool: write, paths: [note.txt], content: "hello world"}
  - expect: miss
    event: {kind: tool, tool: write, paths: [note.txt], content: "HELLO WORLD"}
---
Do not write lowercase hello to text files.
