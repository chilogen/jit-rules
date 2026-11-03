import json, sys, os, re, time
out = os.environ["EXP_OUT"]; os.makedirs(out, exist_ok=True)
raw = sys.stdin.read(); d = json.loads(raw)
ev = d.get("hook_event_name", "?"); tool = d.get("tool_name", "")
open(os.path.join(out, "%d-%s-%s.json" % (time.time_ns(), ev, tool)), "w").write(raw)
if ev != "PreToolUse":
    sys.exit(0)
ti = d.get("tool_input") or {}
cands = []  # (path, content or None)
if tool == "Write":
    cands.append((ti.get("file_path", ""), ti.get("content", "")))
elif tool == "apply_patch":
    p = ti.get("command") or ti.get("input") or ""
    cur = None; lines = []
    for ln in p.splitlines():
        m = re.match(r"\*\*\* Add File: (.*)", ln)
        if m:
            if cur: cands.append((cur, "\n".join(lines)))
            cur = m.group(1).strip(); lines = []
        elif cur is not None and ln.startswith("+"):
            lines.append(ln[1:])
        elif ln.startswith("*** "):
            if cur: cands.append((cur, "\n".join(lines)))
            cur = None
    if cur: cands.append((cur, "\n".join(lines)))
elif tool in ("Bash", "shell", "exec_command"):
    c = ti.get("command") or ti.get("cmd") or ""
    if isinstance(c, list): c = " ".join(c)
    m = re.search(r">\s*([^\s;&|]+\.md)", c)
    if m:
        h = re.search(r"<<-?\s*['\"]?(\w+)['\"]?\n(.*?)\n\1", c, re.S)
        cands.append((m.group(1), h.group(2) if h else None))
for path, content in cands:
    if path.endswith(".md") and content is not None and not content.startswith("# "):
        sys.stderr.write("jit-rules 拦截了这次调用：即将执行的内容违反下列规则。请按规则修改后重新发起；如果你确认规则不适用于这次操作，停下来告诉用户，不要原样重试。\n[me/md.first-line-h1] Markdown 文件的第一行必须是一级标题（以 \"# \" 开头）。\n")
        open(os.path.join(out, "BLOCKED"), "a").write(path + "\n")
        sys.exit(2)
sys.exit(0)
