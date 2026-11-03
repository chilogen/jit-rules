---
title: 分支领先时合并 master
trigger:
  events: [tool]
  tools: [shell]
  shell: ['^git\s+commit\b']
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: tool, tool: shell, shell: 'git commit -m x'}
  - expect: miss
    event: {kind: tool, tool: shell, shell: 'git status'}
---
提交前先合并 master。

```sh jitr-judge
test "$(git rev-list --count master..HEAD)" -gt 3
```
