# Packs, registry, and profiles (rules lane, after judgment units)

**Read first**: all of spec/profile-pack.md, and the pack, registry, and profile sections of spec/contracts.md.

**Change only**: `internal/pack/`, `internal/registry/`, `internal/profile/`. All test data is built in t.TempDir.

## Steps

1. `internal/pack`: `ReadManifest`, `Validate`, `Units`, `Hash`, `Builtin`, `GitDir`, `Fetch` (local directory and git sources; git through `exec.Command("git", ...)`).
2. `internal/registry`: `Load`, `Save`, `Scope`, `IsGlobal`, `Applicable`.
3. `internal/profile`: `Load`, `LoadLock`, `SaveLock`, the five functions that modify profile.yaml, `Init`, `Commit`, `Resolve` (including settings merging and the api_key check).

## Required tests

Build data in t.TempDir and do not depend on the network. For git sources, `git init` a local repository in a temporary directory (commit with `git -c user.name=t -c user.email=t@t commit` in tests) and test through a `file://` URL; `t.Skip` when the machine has no git.

pack:
- With `run.sh` in the pack, Validate fails and lists it; a `.git/` directory in the pack is not an error.
- `Hash`: changing any byte under units changes the hash; changing only README.md does not.
- `Builtin()` returns the two units of the core pack, with `File` starting with `builtin/core/units/`.
- git: `Fetch(url, "")` gets HEAD's commit, in directory `GitDir(<pack name>, <commit>)`, with no `.git` inside; `Fetch(url, "v1")` (a tag) and `Fetch(url, <commit sha>)` both get the corresponding commit.
- **Branch updates**: the repository's `main` branch first has commit A, and `Fetch(url, "main")` gets A; after committing B, `Fetch(url, "main")` gets B, and B's directory exists with the new content.
- A non-existent ref is an error, and nothing is left behind under `{data}/packs/.tmp/`.

registry:
- Ranking: with registered global `~/.jitr`, parent `<parent>/.jitr`, child `<parent>/child/.jitr`, and unrelated `<other>/.jitr`, a cwd under child gets child, parent, global; a cwd outside all project scopes gets only global. Set HOME and JITR_HOME to temporary directories.
- Loading or saving an entry whose path does not name a `.jitr` directory fails. Saved entries contain only `path`, with no stored `scope`.

profile:
- When the same global id appears in A and B, B's is kept and A's has status `shadowed`.
- `replaces`: a unit in B replaces a unit in A; A's unit is no longer in the result, with status `replaced`.
- `disable`: a disabled unit is not in `Units`, with status `disabled`.
- include/exclude filtering.
- lock: after one byte of an own unit changes, it is no longer in `Units`, with status `modified`; a unit absent from the lock is `draft`; an unparseable one is `invalid`, and (when not in the lock) `Errors` is empty.
- Local packs: take effect after being added to profile.yaml and the lock (with the right hash); after one unit file in the pack changes, none of the pack's units take effect, and `Errors` has an entry mentioning `jitr pack update`.
- settings: A sets `redeliver_bytes: 1`, B sets `redeliver_bytes: 2`, the result is 2; after B sets `backends: {jev: {threshold: 0.6}}`, jev's `model` keeps its default and the threshold is 0.6; unset keys take default values.
- With `backends.jev.api_key: x` in settings, that profile fails to load, `Errors` has an entry, and the other profiles still take effect.
- When one profile's profile.yaml is corrupt, `Errors` has an entry and the other profiles still take effect.
- **Modifying profile.yaml**: prepare a profile.yaml with comments (a comment on the first line, a comment above `packs`, and a trailing comment after one pack), then in order: `AddPack` two packs → `SetPackRef` on one of them → `RemovePack` the other → `AddDisable` an id → `AddDisable` the same id → `RemoveDisable` that id. After each step, the structure read back with `Load` is as expected and all three comments are still there. Finally, `RemovePack` of a non-existent pack is an error.
- `SaveLock` then `LoadLock` gives the same content; the file's first line is the prescribed comment.
- The profile.yaml created by `Init` can be read by `Load`, with only core in packs; `Init` in a subdirectory of an existing git repository does not run `git init`.

## Done when

`make check` passes. Commit `packs and profiles`.
