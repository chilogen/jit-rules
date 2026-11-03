---
title: Go 函数超过 60 行时进行职责拆分
trigger:
  events: [tool_result]
  tools: [write, edit, shell]
deliver:
  as: inject
topic: go.function-size
examples:
  - expect: hit
    event: {kind: tool_result, tool: edit, paths: [internal/example.go]}
  - expect: hit
    event: {kind: tool_result, tool: shell, shell: "python3 patch.py"}
  - expect: miss
    event: {kind: tool_result, tool: read, paths: [internal/example.go]}
---
本次改动的 Go 代码包含超过 60 行的函数。请按职责提取命名清晰、可独立测试的辅助函数；不要通过压缩格式、合并语句或无意义切割来规避限制。

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


def checker(root):
    # build once and reuse: `go run` on every shell command is too slow for the hook deadline
    source = os.path.join(root, "scripts", "funclen")
    git_dir = subprocess.run(
        ["git", "rev-parse", "--absolute-git-dir"], cwd=root, capture_output=True, text=True,
    ).stdout.strip()
    binary = os.path.join(git_dir or root, "jitr-funclen")
    newest = max(os.path.getmtime(os.path.join(source, f)) for f in os.listdir(source))
    if not os.path.exists(binary) or os.path.getmtime(binary) < newest:
        partial = f"{binary}.{os.getpid()}"
        built = subprocess.run(
            ["go", "build", "-o", partial, "./scripts/funclen"], cwd=root, capture_output=True, text=True,
        )
        if built.returncode != 0:
            sys.stderr.write(built.stderr)
            sys.exit(2)
        os.replace(partial, binary)
    return binary


event = json.load(sys.stdin)
root = event.get("root") or event.get("cwd") or os.getcwd()
paths = list(event.get("paths") or [])
if event.get("tool") == "shell":
    paths += changed_go(root)
targets = []
for path in dict.fromkeys(p for p in paths if p.lower().endswith(".go")):
    candidate = path if os.path.isabs(path) else os.path.join(root, path)
    if os.path.isfile(candidate):
        targets.append(candidate)
if not targets:
    sys.exit(1)
result = subprocess.run(
    [checker(root), *targets], cwd=root, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True,
)
if result.returncode == 0:
    sys.exit(1)
if result.returncode == 1:
    sys.exit(0)
# a .go file that does not parse yet (mid-edit) gives no verdict on function size; the compiler reports it
sys.stderr.write(result.stderr)
sys.exit(1)
```
