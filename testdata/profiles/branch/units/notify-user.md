---
title: 分支大幅领先时通知用户
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
提交前告诉用户当前分支已大幅领先 master。

```sh jitr-judge
test "$(git rev-list --count master..HEAD)" -gt 5
```
