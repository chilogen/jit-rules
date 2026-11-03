# Spec: normalized events

`event.Event` is defined in [contracts.md](contracts.md). This document specifies how each field is computed.

## Normalized tool names

`shell`, `write`, `edit`, `read`, `search`, `web`, `agent`, `skill`, `mcp/<server>/<tool>`.

Raw tool names are processed in order:
1. Found in the `tools` table of the harness definition → use the table's value;
2. Of the form `mcp__<s>__<t>` → `mcp/<s>/<t>` (split at the first and second `__`; `<t>` may itself contain `__`);
3. Otherwise → lowercase.

Patch tools (listed in the definition's `patch_tools`): when every file in the patch is `Add File`, normalize to `write`; otherwise use the table's value (`edit`).

**write versus edit**: the Content of `write` is the file's complete content; the Content of `edit` is only the fragment being inserted. Rules that constrain "the whole file" (for example the first line) rely on this distinction by writing `tools: [write]`.

## Field extraction (`harness.Extract`, for every harness)

The input is the raw `tool_input` (a map); the output is `harness.Extracted`.

**Patch tools**: the patch text is the first non-empty string among `command` and `input`.
- Paths: scan line by line; for lines prefixed with `*** Add File: `, `*** Update File: `, `*** Delete File: `, or `*** Move to: `, take the part after the prefix (trimmed), collect in order of appearance, and deduplicate.
- Content: every line starting with `+` but not with `+++`, with the leading `+` removed, joined with `\n`.

**Tools normalized to `shell`**:
- Shell: the value of `command`.
- Effect, Paths, Content, ContentUnknown: computed by "Shell write recognition" below.

**Other tools**:
- Paths: the first non-empty string value among `file_path`, `path`, `notebook_path`, `filePath`, `target_file`; if none is found, collect the string values of every top-level key of `tool_input` ending in `_path` (sorted by key).
- Content: extracted only for normalized tool names `write`, `edit`, or names starting with `mcp/`:
  - when an `edits` array exists, join the `new_string` of each item with `\n`;
  - otherwise the first string value present among `content`, `file_text`, `new_string`, `new_str`, `new_source`.

**Path normalization** (the same for every tool):
1. A relative path is first joined to `Cwd` to make it absolute, then `filepath.Clean`ed;
2. Paths inside `Root` become relative to `Root`, always `/`-separated;
3. Paths outside `Root` stay absolute;
4. No symlink resolution.

## Shell write recognition

Reason: AIs often write files with shell commands (Codex was observed creating an md file with `printf '%s\n' '...' >> notes.md`); watching only file-writing tools would let rules miss these writes.

This is a **conservative static analysis**: it never runs the command or performs any expansion; when it meets something it does not understand, it prefers not to recognize anything. The result is deterministic: the same command and the same "does the file exist" answers give the same result. `testdata/shell_writes.json` is the complete test table for this section, and every case must pass.

### Step 1: tokenize

Scan the command string from left to right:

- Space, tab: separate words.
- "Backslash + newline" outside a word: skipped as whitespace.
- Newline: produces a "newline" token. If heredocs are pending, read their bodies in order right after it (see below), then continue scanning.
- `#` outside a word: skip to the end of the line (not including the newline).
- Operators, longest match first: `&>>` `<<<` `<<-` `&>` `>>` `>|` `>&` `<<` `<&` `&&` `||` `|&` `>` `<` `|` `;` `&` `(` `)`.
  - On `(` or `)`: **give up recognizing the whole command**.
  - For an operator starting with `>` or `<`, if the word immediately before it (no whitespace in between) consists only of digits and is unquoted, that word is no longer a word but the operator's file descriptor number (fd).
  - After `<<` and `<<-`, skip whitespace and read the next word as the heredoc delimiter; if there is no word, give up. Register a pending heredoc.
- Any other character starts a word, which continues until unquoted whitespace, a newline, or the first character of an operator. Inside a word:
  - `'...'`: the text inside the quotes, verbatim. A missing closing quote means give up.
  - `"..."`: the text inside the quotes; `\$`, `` \` ``, `\"`, `\\` become the single character, backslash-newline is removed, other `\x` keep both characters; an unescaped `$` or `` ` `` makes the word a **dynamic word**. A missing closing quote means give up.
  - Unquoted `\x`: becomes `x` (when `x` is a newline, both characters are removed).
  - Unquoted `$`, `` ` ``, `*`, `?`, `[`, `{`, and an unquoted `~` as the word's first character: make the word dynamic.
  - If the word contained any quote or backslash, it is marked "quoted".

**Heredoc bodies**: read line by line from after the newline until a line equals the delimiter (with `<<-`, remove all leading tabs from the line before comparing). The body is the lines in between (with `<<-`, each with leading tabs removed), each followed by `\n`. If the end of the command is reached before the delimiter, give up. If the delimiter word is "quoted", the body is static; otherwise, the body is dynamic when it contains `$` or `` ` ``. If heredocs are still pending at the end of the command, give up.

### Step 2: split into segments

Split at newlines and at `&&` `||` `|` `|&` `;` `&`; each segment is a simple command.

### Step 3: analyze each segment

Process the segment's tokens in order:
- Word: appended to the argument list.
- `<<`, `<<-`: this segment's standard input = the corresponding heredoc body.
- `<<<`: standard input = the next word's text plus `\n` (dynamic if the word is dynamic).
- `<`, `<&`, `>&`: skip the next word.
- `>`, `>|`, `>>`: when the fd is absent or 1, the next word is an **output target** (`>>` appends); with any other fd, skip the next word.
- `&>`, `&>>`: the next word is an output target (`&>>` appends), and its content is **unknown** (mixed with standard error).
- A redirection operator with no word after it means give up.

Ignore an output target that is a dynamic word or starts with `/dev/`, `/proc/`, or `/sys/`.

**This segment's standard output content** is determined by the first argument (the command name):
- `cat` as the only argument, with static standard input present: content = standard input.
- `echo`: from the second argument on, leading arguments of the form `-[neE]+` are options (`n`: no trailing newline; `e`: interpret escapes; `E`: do not interpret escapes; later ones override earlier ones). If any remaining argument is dynamic, the content is unknown; otherwise content = the remaining arguments joined with one space, escapes interpreted when `e` is on, plus `\n` (omitted with `n`).
- `printf`: the second argument is the format, the rest are arguments; if the format or any argument is dynamic, the content is unknown. First interpret escapes in the format, then: if the format has no `%`, content = the format when there are no other arguments, otherwise unknown; if the format equals `%s`, content = the arguments concatenated; if the format equals `%s` plus newline, content = each argument followed by `\n`, concatenated (`\n` when there are no arguments); any other format, unknown.
- Any other command: content unknown.

"Interpret escapes" handles only `\n` (newline), `\t` (tab), and `\\` (backslash); any other `\x` is kept as is.

**tee**: when the command name is `tee`, among the remaining arguments: `-a` or `--append` means append; other arguments starting with `-` are ignored; dynamic words and arguments starting with `/dev/`, `/proc/`, `/sys/` are ignored; every remaining one is an output target whose content = this segment's standard input (unknown if absent or dynamic).

### Step 4: summarize

Collect the output targets of all segments in order of appearance and deduplicate by normalized path to get Paths.

- No output target: Effect empty, Paths empty, Content empty, ContentUnknown false.
- Exactly one output target (one before deduplication): Effect = `edit` (append, and the file exists, judged by `exists` on the absolute path before normalization) or `write` (otherwise); Content = the content when known, otherwise ContentUnknown = true.
- Several output targets (including the same file more than once): Effect = `edit`, Content empty, ContentUnknown = true.

Giving up gives the same result as "no output target".

Known limitations (written into the test table to fix the behavior): `cd` is not tracked, paths are always relative to the `cwd` the hook received; in-command writes such as `sed -i` or `python -c` are not recognized.

## Stop events

A `stop` event fires when the AI has finished its reply and is about to end the turn. `Content` = the payload's `last_assistant_message` (the AI's final text); `StopActive` = the payload's `stop_hook_active`. Tool, Effect, Paths, Shell, and ContentUnknown stay empty.

A harness definition may map its turn-end event to `stop` only when that event's payload carries `stop_hook_active`. Reason: delivery on `stop` relies on it to remind at most once per turn (pipeline.md); without it, every reminder would trigger another reminder. Measured: Claude Code and Codex 0.166 fire `Stop` exactly once per turn, including turns with tool calls, and after `{"decision":"block"}` fire it again with `stop_hook_active: true` and the same turn id; TRAE documents the same fields.

## Size

| kind | Size |
|---|---|
| prompt | `len(Prompt)` (bytes) |
| tool | bytes of the raw `tool_input` serialized with `json.Marshal` |
| tool_result | bytes of the raw `tool_response` serialized with `json.Marshal` |
| stop | `len(Content)` (bytes) |
| others | 0 |

## Dedupe key

- `tool` and `tool_result`: `"tool:" + <the value of the first field present among those listed in the definition's dedupe.tool>`. Both participate in dedupe. The normalized key stays the same; when claiming a `tool_result`, the pipeline prefixes it with `result:` so the matching pre-tool claim cannot suppress the post-tool event. Duplicate post-tool events are skipped before response-byte accounting.
- `prompt`: `"prompt:" + <the value of the first field present among dedupe.prompt>`; when none is present, `"prompt:sha1:" + hex(sha1(Prompt))`.
- `stop`: `"stop:sha1:" + hex(sha1(<"1" when StopActive, else "0"> + "\n" + Content))`. Reason: the first and the re-sent end of one turn share the turn id, so the turn id alone would swallow the second; two configurations firing the same Stop produce the same key milliseconds apart.
- Other kinds: the empty string.
- A tool event with none of the fields: the key is empty, and no dedupe is done.

## Test baseline

- `testdata/expected.json` gives the fields each fixture should produce (`harness`, `kind`, `tool`, `raw_tool`, `effect`, `paths`, `shell`, `content`, `content_unknown`, `prompt`, `dedupe_key`, `stop_active`). Fields not listed in expected are not compared. In the tests, the cwd directory does not exist, so `Root` = `Cwd` = `/work/proj` and `exists` returns false for every path.
- `testdata/shell_writes.json`: compute each case with `Extract(codex definition, "Bash", {"command": <command>}, "/work/proj", "/work/proj", exists)`, where `exists` returns true only for files listed in `existing` (relative to `/work/proj`); the result's `tool`, `effect`, `paths`, `content`, and `content_unknown` must all equal `expect` (an empty `paths` array equals nil).
