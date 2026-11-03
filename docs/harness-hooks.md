# Harness support for rule delivery

> Surveyed 2026-11-01. Records only the three capabilities that affect [the decisions](README.md). "?" means not found, not unsupported.

| Harness | Pre-tool event / coverage | Block and return a reason | Inject context while allowing | Evidence |
|---|---|---|---|---|
| Claude Code | PreToolUse, `*` includes MCP | ✅ | ✅ `additionalContext` | Docs |
| Codex CLI | PreToolUse; hosted tools (WebSearch) not covered | ✅ | ✅ (since 2026-05) | [Docs](https://learn.chatgpt.com/docs/hooks); local 0.166 binary |
| TRAE IDE | PreToolUse, includes MCP; the shell tool is named `RunCommand` | ✅ | ✅ | [Docs](https://docs.trae.cn/ide_hook-configuration-reference); reads Claude Code's configuration |
| DeepSeek Harness | Runs Claude Code / Codex hooks.json through a bridge; command hooks only | ✅ | ✅ (takes effect on the next request) | [Source](https://github.com/deepseek-ai/deepseek-harness) `packages/experimental/hook-protocol`; still experimental |
| Cline | PreToolUse | ✅ | ✅ (takes effect on the next request) | [Docs](https://docs.cline.bot/features/hooks/hook-reference) |
| Cursor | preToolUse; some built-in tools emit no event | ✅ | ⚠️ confirmed on the forum, not in the docs | [Docs](https://cursor.com/docs/agent/hooks); forum reports of breakage between versions |
| Gemini CLI | BeforeTool, includes MCP | ✅ | ❌ only AfterTool can inject | [Docs](https://geminicli.com/docs/hooks/reference/) |
| Copilot CLI / VS Code | preToolUse | ✅ | ? | [Docs](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-hooks-reference) |
| Kiro | PreToolUse, `*` includes MCP | ✅ | ? | [Docs](https://kiro.dev/docs/hooks/) |
| Factory Droid | PreToolUse | ✅ | ? | [Docs](https://docs.factory.ai/reference/hooks-reference) |
| Goose | Yes; reads `.claude/settings.json` | ✅ | ? | [Docs](https://goose-docs.ai/docs/guides/context-engineering/hooks/) |
| Windsurf | Per-category events (pre_run_command, etc.); no generic tool event | ✅ | ? | [Docs](https://docs.windsurf.com/zh/windsurf/cascade/hooks) |
| OpenCode | In-process JS plugin `tool.execute.before` | ✅ by throwing | ❌ no native field | [Docs](https://opencode.ai/docs/en/plugins/) |
| Roo Code / Aider | None (Roo has only a draft PR) | — | — | — |

## Conclusions

- There are three implementation styles: an external command speaking JSON over stdin/stdout plus exit 2 (the mainstream, de facto protocol set by Claude Code); in-process plugins; and prompt hooks, where an LLM decides whether to fire.
- One judge plus one rule library, with a thin adapter per harness, covers most of the tools above. The adapter only translates event names, tool names, and output fields.
