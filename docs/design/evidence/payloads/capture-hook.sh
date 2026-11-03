#!/bin/bash
# capture stdin; deny first Write with a rule in stderr; inject on prompt
D=$(dirname "$0")/out
IN=$(cat); EV=$(echo "$IN" | python3 -I -c 'import json,sys;d=json.load(sys.stdin);print(d.get("hook_event_name","?")+"-"+d.get("tool_name",""))')
echo "$IN" > "$D/$(date +%s%N)-$EV.json"
case "$EV" in
  PreToolUse-Write)
    if [ ! -f "$D/denied" ]; then touch "$D/denied"; echo "[me:style/upper] 写入文件的正文必须全部使用大写英文字母。请按规则修改后重新发起同一操作。" >&2; exit 2; fi;;
  UserPromptSubmit-)
    echo '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"<jit-rules>\n[me:reply/tag] 回复的最后一行写 JITR-OK。\n</jit-rules>"}}';;
esac
exit 0
