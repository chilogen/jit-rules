# Repository skeleton

**Goal**: create the repository with the Go project, build, and checking tools. Afterwards `make check` runs successfully (there is no business code yet).

**Read first**: all of spec/layout.md.

## Steps

1. Create the directory `jit-rules`, run `git init`, run `go mod init github.com/chilogen/jit-rules` in it, and `go get` the four dependencies listed in layout.md.
2. Create the directories according to layout.md. `cmd/jitr/main.go` is a main that only prints `jitr dev`.
3. Copy the resources (verbatim; do not change a single byte):
   - everything under `runbook/assets/` → `assets/`
   - `runbook/fixtures/<harness>/` → `testdata/payloads/<harness>/` (four harness directories)
   - `runbook/fixtures/expected.json` → `testdata/expected.json`
   - `runbook/fixtures/shell_writes.json` → `testdata/shell_writes.json`
   - `runbook/fixtures/smoke/` → `testdata/smoke/` (its jitr.lock records the unit file's hash; changing one byte of the unit file invalidates it)
   - `runbook/README.md`, `runbook/spec/`, `runbook/tasks/` → `docs/runbook/`
4. Write `.gitattributes` with two lines: `assets/** -text` and `testdata/** -text`. Reason: the hashes of these files are recorded in locks and tests, and git's line-ending conversion on Windows would invalidate them.
5. Write the following as `AGENTS.md` at the repository root:

   ```markdown
   # AGENTS.md

   This repository implements `jitr` (just-in-time rules).

   - **When implementing, read only `docs/runbook/`.** Start with `docs/runbook/README.md`, then work through the tasks in the order it gives.
   - The spec (`docs/runbook/spec/`) is the only authority; types and signatures follow `spec/contracts.md`.
   - Do not change any file under `docs/runbook/` except `ACCEPTANCE.md`.
   - For details the spec does not cover, choose the simplest implementation.
   - When the spec contradicts itself, the contract is insufficient, or following the spec would produce something wrong: do not invent semantics and do not work around it (for example by concatenating strings to dodge a check, comparing error text, or simulating with another interface). Skip the item, continue with the rest, and report the problem at the end of the task.
   - `assets/`, `testdata/payloads/`, `testdata/expected.json`, `testdata/shell_writes.json`, and `testdata/smoke/` are part of the spec; do not change them.
   - Never change tests or test data to make tests pass; when a test fails, fix the code.
   - After each task, run `make check` and commit once it passes, with the task name as the commit message (for example `packs and profiles`).
   - Tests must set `JITR_HOME=t.TempDir()` and never read or write real user directories.
   ```

6. `assets/embed.go`: as in the assets section of contracts.md (`//go:embed guide.md defaults.yaml unit-template.md harnesses/*.yaml packs`).
7. `internal/archtest/arch_test.go`: implement the dependency and forbidden-literal checks in "Code constraints" in layout.md. No concrete implementation packages exist yet, so the test should pass. Also write a test proving it catches violations: put a .go file containing the literal `"codex"` in `t.TempDir()`, scan it with the same check function, and require an error.
8. Makefile targets:
   - `VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)`, `COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null)`, `LDFLAGS = -X github.com/chilogen/jit-rules/internal/cli.Version=$(VERSION) -X github.com/chilogen/jit-rules/internal/cli.Commit=$(COMMIT)`
   - `build`: `CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/jitr ./cmd/jitr`
   - `test`: `go test ./...`
   - `lint`: `go vet ./...` + `gofmt -l .` (fails on any output)
   - `check`: lint, then test
   - `dist`: cross-compile the five platforms in layout.md with the same LDFLAGS, into `dist/jitr-<os>-<arch>[.exe]`
   - `.gitignore` ignores `bin/` and `dist/`

Until the shared contracts task, `internal/cli` does not exist; a `-X` pointing at a missing variable still compiles, so nothing needs to be done about it.

## Done when

- `make check` passes; `make dist` produces 5 files.
- Commit: `repository skeleton`.
