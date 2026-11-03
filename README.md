# jit-rules

Just-in-time rules for AI coding agents.

`jitr` keeps conditional rules out of the model's resident context. It watches
agent events, decides which rules apply, and either injects the relevant text
or blocks a violating tool call with an explanation the agent can act on.

The project currently supports Claude Code, Codex CLI, TRAE, and DeepSeek
Harness on Linux, macOS, and Windows. It is early-stage software; rule and
harness formats may still change.

## How it works

A rule is a Markdown **judgment unit** with four parts:

1. a deterministic trigger over prompts, tools, paths, commands, or content;
2. an optional LLM or local-script decision for conditions the trigger cannot
   express;
3. an action: `inject` context or `block` the current tool call;
4. examples that must pass before the rule can be enabled.

Rules are inactive drafts until they have been tested, checked for conflicts,
reviewed, and explicitly enabled. See [the design overview](docs/README.md)
for the motivation and trade-offs.

## Use cases

- **Stop deterministic violations before execution.** Block hard-coded
  secrets in write arguments, forbidden shell commands, or writes to protected
  paths, and return the exact rule so the agent can correct the action.
- **Deliver team conventions only when relevant.** Keep architecture,
  migration, release, and repository policies in versioned rule packs, then
  inject only the rules that match the current prompt, path, or tool call.
- **Reinforce agent memory with team feedback.** Every injection or block
  presents the same team rule at the decision point. Agents adapt within the
  session, and harnesses with persistent memory may also save recurring
  guidance. The shared rule pack remains the source of truth, so agents
  without persistent memory still receive the policy next time.
- **Turn corrections into shared memory.** The built-in `core` pack can detect
  an explicit correction or repeated instruction and propose converting it
  into a reviewed rule. Once committed to a team pack, the lesson applies
  across repositories, sessions, and supported agent harnesses.

## Install

Building from source requires Go 1.25 or later:

```sh
go install github.com/chilogen/jit-rules/cmd/jitr@latest
```

Ensure the Go binary directory is on `PATH`, then install hooks for one or more
agent harnesses:

```sh
jitr install claude-code
# Other values: codex, trae, deepseek, all
```

`jitr install` copies the running binary to a stable per-user path before
writing hook configuration. Use `--scope project` to install hooks only in the
current repository.

## Quick start

Create a global profile:

```sh
jitr profile init ~/.jitr --name personal
jitr status
```

For repository-specific rules, run this from the repository:

```sh
jitr profile init "$PWD/.jitr" --name project
```

Profiles apply according to the agent session's working directory. A global
`~/.jitr` profile applies everywhere; a project `.jitr` profile applies when
the agent starts in that project or below it. The nearest profile has the
highest priority.

## Create rules

<table>
<thead>
<tr>
<th width="12%">Aspect</th>
<th width="44%">Deterministic script</th>
<th width="44%">Decisions API</th>
</tr>
</thead>
<tbody>
<tr>
<td colspan="3">
Describe persistent behavior directly to the agent. The built-in <code>core</code>
pack invokes <code>jitr guide</code>, chooses the cheapest reliable decision
method, creates and tests the unit, checks conflicts, and asks for approval.
</td>
</tr>
<tr>
<th scope="row" valign="top">Create it</th>
<td valign="top">
<em>From now on, after editing a Go file, split that file if it exceeds 400
lines.</em>
</td>
<td valign="top">
<em>From now on, when I explicitly correct you or say that I am repeating an
instruction, finish the current task and ask whether I want to save that
requirement as a rule.</em>
</td>
</tr>
<tr>
<th scope="row" valign="top">How it decides</th>
<td valign="top">
Reads only the Go paths reported by the current tool result, then counts lines
in those files. Local, deterministic, and requires no API key. Exit
<code>0</code> means hit and <code>1</code> means miss.
</td>
<td valign="top">
Sends the normalized user prompt to a configured backend, which answers a
self-contained yes/no <code>judge.question</code>. It uses only explicit cues
in the current message, not unavailable conversation history. Remote,
probabilistic, and cached.
</td>
</tr>
<tr>
<th scope="row" valign="top">Best for</th>
<td valign="top">
Concrete conditions that can be checked reliably, including local injection
and blocking.
</td>
<td valign="top">
Conversation intent and workflow habits that cannot be reduced reliably to
keywords. Prefer contextual injection; do not use it as the sole safety
boundary.
</td>
</tr>
<tr>
<th scope="row" valign="top">Setup &amp; safety</th>
<td valign="top">
Runs with the user's permissions. Any exit code other than <code>0</code> or
<code>1</code> is an error. Review scripts from third-party packs before
enabling them.
</td>
<td valign="top">
JEV uses <code>TYPESAFE_API_KEY</code>. OpenAI uses
<code>settings.judge.backend: openai</code> and
<code>OPENAI_API_KEY</code>. A missing key disables model-judged units. Set
<code>settings.judge.send_content: false</code> to keep event content local.
</td>
</tr>
<tr>
<th scope="row" valign="top">Generated unit</th>
<td valign="top">
<details>
<summary>View complete script unit</summary>
<pre><code>---
title: Split edited Go files over 400 lines
trigger:
  events: [tool_result]
  tools: [write, edit]
  paths: ["**/*.go"]
deliver:
  as: inject
examples:
  - expect: hit
    event: {kind: tool_result, tool: write,
            paths: [large.go]}
  - expect: miss
    event: {kind: tool_result, tool: write,
            paths: [small.go]}
---
The Go file changed by this tool call now
exceeds 400 lines. Split it into focused
modules before continuing.

&#96;&#96;&#96;python3 jitr-judge
import json
import os
import sys

event = json.load(sys.stdin)
root = event.get("root") or event["cwd"]
for changed_path in event.get("paths", []):
    if not changed_path.endswith(".go"):
        continue
    target = os.path.join(root, changed_path)
    try:
        with open(target, encoding="utf-8") as source:
            if sum(1 for _ in source) &gt; 400:
                sys.exit(0)
    except OSError:
        pass
sys.exit(1)
&#96;&#96;&#96;</code></pre>
</details>
</td>
<td valign="top">
<details>
<summary>View complete Decisions API unit</summary>
<pre><code>---
title: Offer to save repeated corrections
trigger:
  events: [prompt]
judge:
  question: Does the current message explicitly
    correct the agent's prior behavior, or say
    that a work instruction is being repeated?
  not: The message states a requirement without
    framing it as a correction or repetition.
deliver:
  as: inject
topic: core.authoring
examples:
  - expect: hit
    event:
      kind: prompt
      prompt: Again, don't use relative imports.
  - expect: miss
    event:
      kind: prompt
      prompt: Add a login page.
---
When the current message explicitly corrects
you or says a requirement was already given,
finish the correction first. Then ask whether
to save it as a rule. If the user agrees, run
jitr guide.</code></pre>
</details>
</td>
</tr>
<tr>
<td colspan="3">
Both methods run only after the deterministic trigger matches, so unrelated
events never run a script or make a network request.
</td>
</tr>
</tbody>
</table>

## Packs

Install a community rule pack from a Git repository:

```sh
jitr pack add <git-url>
```

Packs and profiles are pinned by content hash. `jitr unit show` displays the
trigger, decision, script, delivery action, and rule text before activation.

## Diagnostics

```sh
jitr status
jitr check
jitr explain --last 1
```

The journal stores event content by default to support replay. Set
`settings.journal.store_content: false` when prompts, commands, or file
content must not be retained.

## Harness notes

### Codex CLI

- Run `/hooks` and trust jitr's hooks; untrusted hooks are silently skipped.
- Project scope also requires trusting the project's `.codex` directory.

### TRAE

TRAE also runs hooks from Claude Code configuration. Installing both harness
definitions is safe because jitr deduplicates events.

### DeepSeek Harness

Restart DeepSeek Harness after installing or changing hooks.

## Development

```sh
make bootstrap
make check
make build
```

The implementation and protocol specifications live under
[`docs/runbook/`](docs/runbook/). Design rationale and measured harness
behavior live under [`docs/design/`](docs/design/).

## Uninstall

```sh
jitr uninstall all
jitr purge
```

`jitr purge` asks for confirmation. Use `jitr purge --yes` only in
non-interactive automation.

## License

Licensed under the [MIT License](LICENSE).
