---
title: commit message 用中文写
trigger:
  events: [tool]
  tools: [shell]
  shell: ['git\s+commit\b.*\s-[a-z]*m\s*["'']?[A-Za-z]']
deliver:
  as: block
topic: git.commit-language
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m "fix bug"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git commit -m "修复登录错误"'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git log'}
---
commit message 用中文写，第一行不超过 50 个字。
