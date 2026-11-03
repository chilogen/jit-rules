# Just-in-time rules: design decisions

Why this project exists is in [../README.md](../README.md). This document records only how it is done: the decisions and the principles that govern them. The coding agent receives only [../runbook/](../runbook/README.md); this document and [verification.md](verification.md) are for people.

## Who the user is decides where things land

The end user is an ordinary person: they cannot write scripts and are not good at writing judgment questions. So **the rules are written by the user's own AI**; the person only states the requirement and confirms the result. It follows that:

- **The artifact is a file a person can read**: one judgment unit is one Markdown file, with the rule text in the body and the triggers and examples in the frontmatter. `jitr unit show` renders it as a plain-language description.
- **Editing, evaluating, and enabling are three separate actions**: the AI edits the file; `jitr unit test` runs the unit's own examples, `replay` replays it over real history, and `check` looks for conflicts; after the person confirms, `jitr unit enable` writes the file's hash into `jitr.lock`. If the file is edited afterwards it stops taking effect automatically until it is enabled again. Principle: the AI may edit freely, but content that has not been evaluated never reaches the AI.
- **How the AI learns this workflow**: the built-in core pack has two units that fire when the user states a new rule or corrects the AI, telling the AI to run `jitr guide` and follow it. Principle: authoring rules is itself delivered as a just-in-time rule.

## Judgment units

A unit keeps every part of a rule in one file:

| Part | Field | Meaning |
|---|---|---|
| When to check | `trigger.events` | When the user sends a message / before the AI calls a tool |
| Deterministic conditions | `trigger.tools/paths/shell/prompt/content` | Declarative; evaluation takes almost no time |
| Judgment | `judge.question` / `judge.not` / `judge.backend`, or a `jitr-judge` script block | A yes/no question for a model, or a script that inspects the environment |
| Where to deliver | `deliver.as` | `inject`: put into context; `block`: stop the tool call and return the rule as the reason so the AI fixes it |
| What to deliver | Body | The rule text |
| What it conflicts with | `topic` / `replaces` | See below |
| How to verify | `examples` | At least one that should fire and one that should not; both a test set and an explanation for people |

Decisions:

- **A unit picks its own judgment: a model backend or a script.** `judge.backend` chooses the model backend per unit (the settings default otherwise); a fenced code block marked `jitr-judge` in the Markdown is a script judge, run in the project directory, exit code 0 meaning a hit. Principle: some conditions depend on environment state the event does not carry (how far a branch has diverged, whether a service is running); a declarative trigger cannot see it and a model can only guess. The script lives in the Markdown next to the rule so a person reads one file; it is cut out of the body because the body is what reaches the AI. The user's AI writes the script, as it writes everything else in the unit.
- **Script judges run where hooks run, with the user's permissions.** Installing a unit or a pack is trusting it, as with a skill or the user's own hooks. Principle: the harness runs hooks outside the agent's sandbox on purpose: hooks are the user's machinery for constraining the model, not actions of the model. jitr follows that rather than adding a sandbox of its own.
- **Script judges are not cached by default and never run during evaluation.** A unit can set `judge.cache`; scripts default to off, models to on. Principle: a script reads live state, so the same event can get a different answer later; evaluation runs on synthetic or historical events whose environment no longer exists, so running the script there would test the current repository, not the example.
- **Examples are required.** Principle: without examples nothing can be evaluated; a judgment question written by an AI must first prove itself on examples.
- **There are only two delivery modes.** Injection on a user message takes effect before the model starts the turn; injection on a tool call takes effect before the model's next generation (the call itself proceeds); blocking returns the rule as the failure reason of the tool call. Principle: a rule must arrive before the model generates; content that is already generated can only be fixed by blocking and making the model rewrite it.
- **Blocking applies to every call that meets the condition, so a blocking condition must describe the violation, not "doing a certain kind of thing".** Once fixed, the condition no longer holds and the call goes through; when the AI is sure the rule does not apply, it stops and hands over to the user instead of retrying unchanged. Principle: whether a call goes through must be decided only by whether the content complies, never by "it was already blocked once"; otherwise an unchanged retry bypasses the rule and the decision goes back to the model. Validation requires a blocking unit to have an "after the fix" miss example, so a test proves that the fixed content is allowed.

## Packs and profiles

- **A pack** is how the community distributes units: a git repository containing only `pack.yaml` and `units/*.md`, with no executable files allowed (a unit's script judge lives inside its Markdown, where `unit show` displays it). The repository ships a `core` pack.
- **A profile** is one person's rule collection: it references several packs (optionally filtered by id, for partial installs) plus its own units. `jitr.lock` records, for each pack, the resolved commit and a content hash of the whole pack; a pack whose content does not match its hash does not take effect. The built-in core pack is pinned by the jitr version. Principle: as with one's own units, what takes effect must be the content that was checked; branches move, so record the commit, not the ref.
- **Which profiles apply is decided by the session's cwd, not by the files an event touches**; event paths are relative to the project root of that cwd (the nearest ancestor containing `.git`). Principle: a prompt event has no paths, and one session should see one rule set across all its events. Cost: an AI started at a monorepo root does not get the rules of a sub-project's profile, and path globs written relative to the sub-project do not match; start the AI inside the sub-project, or put the rules in a profile whose scope covers the cwd.
- **Installing and uninstalling are one line of configuration each**: registering or unregistering a profile is a line in the registry; adding or removing a pack is an entry in profile.yaml. The harness hooks are installed once and never need to change.

## Conflicts

There are three kinds of conflict, each resolved at a different time. Principle: **conflicts are found and resolved when a unit enters a profile; at runtime only one definite rule is executed, with no judgment on the hot path.**

| Conflict | When resolved | How |
|---|---|---|
| Two units make opposite demands about the same thing | At authoring or install time: `check` finds pairs that fire together (from examples, and from units that hit together on real events in the journal), then asks a model whether they contradict | The AI proposes options and the person chooses: put both under the same `topic` (only the highest-ranked fires at runtime), replace one with `replaces`, disable one, or narrow a trigger |
| The same id appears in several profiles | At load time | Keep the highest-ranked one; not a conflict |
| One unit blocks, another injects | At runtime | Block wins; the reason includes the text of every hit rule |

**Ranking**: the user's own units > packs listed earlier in the profile > packs listed later; when several profiles apply, the more specific scope wins, then the later-registered one. Principle: the closer an intent is to the user, the higher its priority.

## Extension points

| What | Form | Principle |
|---|---|---|
| Units and packs | Data (Markdown + YAML) | Anyone can publish; validation, tests, and conflict checks guard the user's install |
| Harnesses | Data (YAML definition: detection, event mapping, tool-name mapping, dedupe fields, install locations) | Four are built in; the community adds a harness with one YAML file, no Go required |
| Model backends | Go code (implements `Backend`) | Code goes through repository review. JEV and GPT-6 Luna are built in |
| Install methods | Go code (implements `Installer`) | Same as above. hooks.json and DeepSeek's patch file are built in |

Harnesses can be data because all four follow Claude Code's hook protocol; they differ only in naming and configuration location (measured or confirmed from source, see [verification.md](verification.md)).

## Key runtime decisions

- **Prefer extra delivery to a miss, but a block must be certain.** When the model times out or errors, treat it as a hit but inject only, never block; when it is known which file is being written but not what is written, rules that depend on content also only inject. When no model backend is configured (for example the API key is not set), model-judged rules do not count as hits, and the user is told once per session. Principle: "prefer extra delivery" targets temporary failures; a missing configuration is a lasting state, and treating it as a hit would deliver those rules on every event, which is worse than not delivering them.
- **A file written by a shell command is treated as a file write.** Shell commands get a conservative static analysis that recognizes the files and contents written by redirection, heredocs, and `tee`; anything not understood is not recognized. Principle: rules constrain "the content written to a file", not "which tool wrote it"; Codex was observed creating a file with `printf ... >> notes.md`.
- **The hook never interrupts work.** Any error is treated as "allow, no output"; exit 2 can only come from an explicit rule verdict.
- **The hook command never changes once written**: it is installed at a fixed path and upgrades replace the file in place. Principle: Codex binds its trust in a hook to the hook definition and silently skips a hook whose definition changed.
- **Redelivery distance is measured in bytes accumulated by the session itself**, without reading the transcript. Principle: some harnesses do not provide a transcript, and others do not guarantee its format.
- **The main cost of a model judgment is establishing the connection, not inference.** JEV inference is around 50 ms, but every hook is a new process that redoes the TLS handshake (measured at 1–4 s on a poor network). So the journal records connection time and inference time separately; whether connection reuse is worth it is decided once real data exists; tool rules are encouraged to have a tool or path condition, but only as a lint warning. Principle: measure first, optimize later.
- **Script judges get no time budget of their own, only the hook deadline; every run is recorded.** The journal records each script's time, exit code, and stderr. Principle: measure first, optimize later; the deadline exists only so a stuck script cannot hang the agent.

## Isolated evaluation

`jitr isolate <harness> [--no-jitr] -- <args>` launches a harness in isolated mode: its own memory, skills, and instruction files are disabled and only jitr remains (with `--no-jitr`, jitr is disabled too, as the control group).

- **Done through launch arguments, without changing the user's configuration files.** Principle: an evaluation should leave no trace; if configuration files were edited, a crash midway could not be recovered.
- **Each harness's way of disabling things is written in the `isolate` section of its definition**, and the launch method is a pluggable Go implementation (currently three: a Claude settings file, Codex `-c` arguments, and a DeepSeek patch file). TRAE is a GUI IDE, so only manual instructions are printed.
- **The control group must exclude jitr completely**: `--no-jitr` sets an environment variable that makes every `jitr hook` process exit immediately, so hooks already installed in the harness configuration do not leak into the control group.
- The journal marks such runs as `isolated`, so evaluation data can be separated from everyday use.

## Technology choices

- **Go**: the hook starts on every tool call, so cold start must be in milliseconds, and nothing may depend on an installed runtime. Both Go and Rust satisfy this; Go was chosen because the hard part of this project is the protocol and rule logic, not performance, and Go is faster to develop in and easier for a coding agent to get right.
- **Linux, macOS, and Windows are all supported**: all four harnesses have Windows users. Platform differences appear in only three places: file locks, command quoting, and paths.

## Problems exposed by the first implementation

The first implementation followed the runbook; in real-environment acceptance the core check ("the first line of a Markdown file must be a heading") failed on both Claude Code and Codex, and six fixes were committed afterwards. Traced one by one, only one was a plain implementation mistake (a YAML merge that could not delete anything); the rest came from the design and the specs:

| Problem | Root cause | Change in this revision |
|---|---|---|
| A blocking rule on Claude Code did not stop a violating write (the acceptance record had no logs; the first fix and its tests show that a block followed by an unchanged retry was allowed) | The design made a block effective only once within the redelivery distance, and the block message said "retry unchanged if it does not apply" | Block every violation; the condition must describe the violation; the message now says "if it does not apply, stop and tell the user". Measured: when the rule conflicts with the user's request, both harnesses stop and explain after being blocked, without working around it; when the rule can be satisfied, Claude Code fixes and rewrites ([verification.md](verification.md)) |
| Codex wrote Markdown through the shell and the rule did not fire | The design assumed file writes always go through file-writing tools | Shell write recognition; the Effect field lets `tools: [write]` match as well |
| Connection time in the journal was always 0 | The journal required it, but no cross-package type carried the field; the same type was written separately in several specs | Every cross-package type is defined once in contracts.md, so producers and consumers share the same contract |
| Exit codes did not distinguish user errors from internal errors, and `--json` printed no JSON on errors | The spec required the distinction, but the contract had no way to express it, and root.go was frozen after the shared-contracts task | The shared-contracts task implements error handling completely: user error by default, internal errors marked explicitly |
| `status` could not tell reliably whether a harness was installed | The interface had no "is it installed" query; the implementer guessed from the diff of an uninstall plan | `Installer` gains `Installed` |
| `pack update` on a branch source did not fetch new content | The spec said "reuse the directory if it exists", and directories were named by ref | Directories and lock entries are keyed by commit, and update always fetches again |
| The architecture test forbade backend names, yet the default settings had to name one | The spec contradicted itself; the implementer evaded the check with string concatenation | Defaults moved into the embedded `defaults.yaml` |

After the first implementation, the implementer committed 14 more fixes and acceptance records on the old runbook (commits of the first implementation, not kept in this repository's history), and passed real acceptance in the environment at the time. The items below were also left unresolved in the first draft of this revision and have since been addressed:

| Problem | Root cause | Change |
|---|---|---|
| The implementer could not implement the functions in `contract.go` and ended up inventing a 611-line "exported API freeze" gate | The spec required the shared-contracts task to write `panic` placeholders and forbade later edits to `contract.go`, but Go can only replace a function body in place | Only types and signatures are frozen; the owning task replaces function bodies in place; no new exported identifiers outside the contract |
| `isolate codex` still ran the user's own hooks, and an installed jitr hook ran twice | The design assumed `--ignore-user-config` blocks user configuration; Codex 0.163 still reads `$CODEX_HOME/hooks.json` | Isolation uses a temporary `CODEX_HOME` that copies only `auth.json`; `Launch` gains `Env` |
| After uninstall, `2>/dev/null` in user configuration became `2>/dev/null`; a hooks.json that did not exist before was left behind as an empty file | The spec only required "byte-identical with 2-space indentation", without pointing out that Go's JSON encoder escapes `<>&` by default, nor saying what to do with a file that jitr created | Require `SetEscapeHTML(false)` and `UseNumber`; delete the file on uninstall when only default content remains |
| `help --json`, `completion`, and parent commands with unknown arguments did not follow the common exit-code and output protocol; `isolate --json` could not emit a single JSON object | cobra's built-in commands bypass task code, and the spec did not cover them | `Execute` handles these cases uniformly; `isolate` rejects `--json` |

**Root cause**: the runbook spelled out implementation details for a less capable implementer, which is right in itself; but the more detailed the spec, the more correctness rests entirely on the designer, and the designer of the first version did not verify the spec like code: no field was traced from producer to consumer, the two core assumptions ("what happens after a block", "how does the AI write files") were not verified on real harnesses, and the implementer was told to "follow the spec, never change the contract". Gaps in the spec could only be worked around, and surfaced only at final acceptance.

**Process changes**:
- The designer verifies core behavior on real harnesses before handing over (this revision's measurements are in verification.md), and checks the test table for complex specs against a reference implementation (shell write recognition).
- Before handing over, contracts.md is generated verbatim into each package's `contract.go` (bodies `panic`), and must pass `go build` and `go vet` with Go 1.25; then every harness definition, the defaults, the units, and the lock shipped with the runbook are decoded strictly (`KnownFields`) into the contract types, and the fields of every fixture are checked against `event.Event`.
- Every cross-package type is defined once; the shared-contracts task verifies the data flow before the lanes run in parallel.
- The real-harness smoke test moves from final acceptance to the hook pipeline task, with commands that can be run as-is and explicit pass criteria.
- When the implementer finds the spec contradicting itself, it stops and reports instead of working around it; a failed acceptance must come with diagnostics, and acceptance must be rerun after a fix.
