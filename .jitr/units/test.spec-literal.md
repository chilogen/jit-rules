---
title: 测试的字符串断言只针对 spec 规定的文本
trigger:
  events: [tool_result]
  tools: [write, edit]
  paths: ["**/*_test.go"]
deliver:
  as: inject
topic: test.spec-literal
examples:
  - expect: hit
    event: {kind: tool_result, tool: edit, paths: [internal/cli/diag_test.go], content: 'if code != 0 || !strings.Contains(stdout, "命中率：2/3") {'}
  - expect: miss
    event: {kind: tool_result, tool: edit, paths: [internal/unit/contract_test.go], content: 'if !strings.Contains(got, "工具结果已经产生；阻止工作流继续") {'}
  - expect: miss
    event: {kind: tool_result, tool: edit, paths: [internal/cli/diag_test.go], content: 'if report.Judged != 3 || len(report.Hits) != 2 {'}
  - expect: miss
    event: {kind: tool_result, tool: edit, paths: [internal/cli/diag.go], content: 'if !strings.Contains(stdout, "命中率：2/3") {'}
---
刚写入的测试用字符串断言了 spec（docs/runbook/spec、assets、testdata）里找不到的文本。逐条对照 spec 判断：
- spec 模板填入的值、测试自己的输入被回显（unit id、文件名、环境变量名）：合法，保留。
- 实现者自选的措辞或错误文本：改为断言退出码、--json 字段、返回的结构体或 errors.Is/As；spec 没给可区分的错误类型时，不要比较文本，报告契约不足。

```python3 jitr-judge
import json
import os
import re
import sys

event = json.load(sys.stdin)
root = event.get("cwd") or os.getcwd()
pattern = r'strings\.(?:Contains|HasPrefix|HasSuffix)\([^,()]+(?:\([^()]*\))?[^,()]*,\s*("(?:[^"\\]|\\.)+")\)'
literals = [json.loads(m) for m in re.findall(pattern, event.get("content") or "")]
if not literals:
    sys.exit(1)

corpus = []
for top in ("docs/runbook/spec", "assets", "testdata"):
    for dirpath, _, files in os.walk(os.path.join(root, top)):
        for name in files:
            try:
                with open(os.path.join(dirpath, name), encoding="utf-8") as f:
                    corpus.append(f.read())
            except (UnicodeDecodeError, OSError):
                pass
text = "\n".join(corpus)
sys.exit(0 if any(lit not in text for lit in literals) else 1)
```
