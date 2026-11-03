---
title: Go 文件超过 400 行时进行模块化拆分
trigger:
  events: [tool_result]
  tools: [write, edit, shell]
deliver:
  as: inject
topic: go.structure
examples:
  - expect: hit
    event: {kind: tool_result, tool: write, paths: [internal/example.go]}
  - expect: hit
    event: {kind: tool_result, tool: shell, shell: "sed -i 's/a/b/' internal/example.go"}
  - expect: miss
    event: {kind: tool_result, tool: read, paths: [internal/example.go]}
---
本次改动的 Go 文件超过 400 行。请进行模块化拆分，把职责清晰、可独立测试的类型和函数移到合适的文件中；不要为了满足行数限制机械切割紧密相关的代码。

```python3 jitr-judge
import json
import os
import subprocess
import sys


def changed_go(root):
    # shell writes are often invisible to jitr (sed -i, scripts): fall back to the working tree vs HEAD
    result = subprocess.run(
        ["git", "status", "--porcelain", "-z", "--untracked-files=all"],
        cwd=root, capture_output=True, text=True,
    )
    if result.returncode != 0:
        return []
    entries = result.stdout.split("\0")
    paths, i = [], 0
    while i < len(entries):
        entry = entries[i]
        i += 1
        if len(entry) < 4:
            continue
        if entry[0] in "RC":
            i += 1  # skip the rename source
        paths.append(entry[3:])
    return paths


event = json.load(sys.stdin)
root = event.get("root") or event.get("cwd") or os.getcwd()
paths = list(event.get("paths") or [])
if event.get("tool") == "shell":
    paths += changed_go(root)
for path in dict.fromkeys(p for p in paths if p.lower().endswith(".go")):
    candidate = path if os.path.isabs(path) else os.path.join(root, path)
    try:
        with open(candidate, encoding="utf-8") as source:
            if len(source.read().splitlines()) > 400:
                sys.exit(0)
    except (OSError, UnicodeError):
        pass

sys.exit(1)
```
