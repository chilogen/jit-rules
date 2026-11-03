# Profile and pack commands (rules lane, after packs and profiles)

**Read first**: the "Common rules", "profile", and "Packs" sections of spec/cli.md, spec/profile-pack.md, and the cli section of spec/contracts.md.

**Change only**: `internal/cli/profile.go`, `internal/cli/pack.go` (and their tests; split into `pack_*.go` when a file gets too long), `testdata/packs/`.

## Steps

Implement `profile init/register/unregister/list/show` and `pack add/remove/update/list`.

For the "tests + conflict check" step in `pack add` and `pack update`, first define a package-level placeholder in `internal/cli/pack.go`: `var checkPack = func(units, active []*unit.Unit, s profile.Settings) (errs []error, warns []string) { return nil, nil }` (`active` holds the units currently active in the target profile that do not belong to this pack: during `pack update`, the pack's old version is not a conflict candidate). The unit commands and evaluation task replaces it with the real implementation.

Return errors as cli.md prescribes: return a plain error for problems in user input; for failed file reads/writes and git runs, just wrap the underlying error with `%w` all the way up and Execute treats it as an internal error, with no case-by-case checks; use `cli.Internal` only for genuine environment problems whose error chain contains none of those underlying errors; use `cli.Silent` when you need to print a result first and then exit non-zero.

## Required tests

Every test is driven through `cli.Execute` (set the arguments, then call it) and checks the exit code and output.

- The profile.yaml created by `profile init` contains the core pack and is already registered; `profile show` in the current cwd prints the plain-language descriptions of core's two units.
- `profile register` of a profile whose lock contains an invalid unit fails (exit code 1), and the registry is unchanged.
- `pack add` of a local directory (testdata/packs/good): profile.yaml's packs gains an entry at the end, jitr.lock has a record with the right hash; `pack add` of the same directory again → error, exit code 1.
- `pack add` of testdata/packs/with-script (contains run.sh) fails with exit code 1, and both profile.yaml and jitr.lock are unchanged.
- After `pack remove`, the pack is in neither profile.yaml nor jitr.lock, and its units are no longer in `Units` from `profile.Resolve`.
- `pack update`: for a git source whose ref is a branch, after a new commit on the remote, `pack update` changes the commit in the lock to the new one, and new units appear in the `Resolve` result.
- `pack update`: after one unit in a local-source pack changes, `pack update` updates the hash in the lock and the pack takes effect again.
- `pack add ./not-exist`: exit code 1, the error contains "找不到目录"; with `--json`, valid JSON is printed with `ok` false.
- `pack add file:///not/exist`: git fails, exit code 3.
- With `--json`, every command prints exactly one valid JSON object containing `ok`.

## Done when

`make check` passes. Commit `profile and pack commands`.
