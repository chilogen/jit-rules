---
title: Markdown 文件的第一行必须是一级标题
trigger:
  events: [tool]
  tools: [write]
  paths: ["**/*.md"]
  content: ['\A(?:[^#]|#[^ ])']
deliver:
  as: block
examples:
  - expect: hit
    event: {kind: tool, tool: write, paths: [notes.md], content: "Shopping list:\nmilk\n"}
  - expect: hit
    event: {kind: tool, tool: shell, effect: write, paths: [notes.md], content: "Buy milk and eggs tomorrow.\n"}
  - expect: miss
    event: {kind: tool, tool: write, paths: [notes.md], content: "# Shopping list\nmilk\n"}
  - expect: miss
    event: {kind: tool, tool: edit, paths: [notes.md], content: "milk\n"}
  - expect: miss
    event: {kind: tool, tool: write, paths: [notes.txt], content: "milk\n"}
---
Markdown 文件的第一行必须是一级标题（以 "# " 开头）。
