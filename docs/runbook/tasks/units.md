# Judgment units (rules lane)

**Read first**: all of spec/unit.md, the "Unit file format" section of assets/guide.md, and the unit section of spec/contracts.md.

**Change only**: `internal/unit/`, `testdata/units/`, `testdata/describe/`.

## Steps

1. `internal/unit`: `Parse`, `ParseFile`, `Validate`, `Describe`.
2. Validate compiles regular expressions and checks globs only to check validity; the results are not kept (the gate compiles and caches them itself).

## Required tests (unit files for the tests go under testdata/units/)

- The two files under `assets/packs/core/units/`, `assets/unit-template.md`, and `testdata/smoke/profile/units/md.first-line-h1.md` all parse, and Validate reports no errors.
- For every "error" rule in the validation table of unit.md (1–5, 7, 8, 9, 12) there is a file that **violates exactly that rule**, and the errors from Validate can be traced to it (error messages start with `规则 <number>：`, and the test checks this prefix); for every "warning" rule (6, 10, 11, 13) there is a file that produces only that warning and no errors.
- `id:` or a misspelled `tirgger:` in the frontmatter is an error.
- The short id `Bad_Name.md` is an error.
- A unit file with `\r\n` line endings parses, and Body contains no `\r`.
- Golden tests for `Describe`: the output for `core/author` (threshold 0.3) is identical to `testdata/describe/author.txt`; the output for `assets/unit-template.md` (pack name `me`, threshold 0.3) is identical to `testdata/describe/template.txt`. Write both golden files **by hand** following the template in unit.md; do not generate them with code.

## Done when

`make check` passes. Commit `judgment units`.
