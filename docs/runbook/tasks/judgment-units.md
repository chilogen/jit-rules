# Change: units with backends and script blocks (units lane; parallel with the judgment lane)

**Read first**: all of spec/unit.md; the unit section of spec/contracts.md.

**Change only**: `internal/unit/`, `testdata/units/`.

## Steps

1. `Parse`: the script block (fence rules, interpreter, removal from the body, empty `Judge` created when needed), `judge.backend`, `judge.cache`.
2. `Validate`: rules 14 and 15; the new conditions of rules 6, 7, and 11 (rule 11 only for a question).
3. `Describe`: the two forms of the judgment line, the script lines, `由模型 <backend>`, `（结果会缓存）`.

## Required tests

- A unit whose body is: text, a ```` ```sh jitr-judge ```` block, more text → `Lang` = `sh`, `Script` = the block's lines each ending in `\n`, `Body` = both texts without the block; `Judge` non-nil without a `judge` field in the frontmatter.
- A fence of four backticks containing a line of three backticks → the inner line is part of the script. A `~~~` fence works the same.
- A ```` ```python ```` block without `jitr-judge` → stays in the body, no script.
- `\r\n` file → same result as `\n`; the hash is over the original bytes.
- Rule 14: question + script; neither; `not` without question; `backend` with a script → each an error starting with `规则 14：`.
- Rule 15: two blocks; unclosed block; ```` ```jitr-judge ```` (no interpreter); empty script → each an error starting with `规则 15：`.
- Rule 7: events only plus a script → no rule 7 error. Rule 6 warning for a script mentions running the script. Rule 11 is not raised for a script.
- `judge: {cache: maybe}` → rule 1 error (type).
- `Describe`: a script unit prints the script line and every script line indented by two spaces, and no threshold; with `cache: true` the line ends with `（结果会缓存）`; a question unit with `backend: openai` prints `由模型 openai 回答`; a question unit without backend is byte-identical to before this change.

## Done when

`make check` passes. Commit `judgment units`.
