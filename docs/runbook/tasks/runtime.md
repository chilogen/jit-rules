# Gate, session, delivery, journal (runtime lane)

**Read first**: the sections "gate.Match", "deliver.Decide", "Session state", and "Journal" in spec/pipeline.md, and the gate, session, deliver, and journal sections of spec/contracts.md.

**Change only**: `internal/gate/`, `internal/session/`, `internal/deliver/`, `internal/journal/`.

This task implements only these four packages and does not wire up the whole flow (that is the hook pipeline task). Units and events in tests are constructed directly in test code (`&unit.Unit{...}`); do not call the rules lane's parser.

## Steps

1. `gate.Match`.
2. `session`: Store, Update, Claim, Read, GC; file locks use gofrs/flock, retried as described in pipeline.md.
3. `deliver.Decide`: topic arbitration → redelivery filter → choose the action → assemble the text.
4. `journal`: Append, Since, GC, LogError.

## Required tests

gate:
- Each trigger field has one passing and one failing case, and `Failed` names the right field.
- An event without paths does not pass a unit that has paths.
- `tools: [write]`: an event with Tool `shell` and Effect `write` passes; with an empty Effect it does not.
- A content condition and an event with `ContentUnknown` true: passes with `ContentUncertain` true; without a content condition, `ContentUncertain` is false.

session:
- Claiming the same key twice gives dup=true the second time.
- A key starting with `prompt:sha1:` is a dup when claimed again after 4 seconds, and not after 6 seconds; other keys are still dups after 9 minutes.
- Seen entries older than 10 minutes are removed on write.
- While another goroutine holds the lock for 300ms, Update returns locked=false without calling fn.
- 20 goroutines calling Update at once (each adds 1) end with Bytes == 20.

deliver (all with hand-built hits and state):
- Two inject hits → Action inject, with text identical to the template in pipeline.md.
- One block plus one inject (tool event) → Action block, the text starts with the prescribed block introduction, the block unit comes first; `Blocked` holds only the block unit, `Delivered` holds both.
- **A block unit still blocks within the redelivery window**: the state's `at` for this unit is closer to the current bytes than the window → Action is still block, and this unit has no skip.
- A block unit with `NoBlock` true → Action inject, and the unit goes through the redelivery filter.
- A block unit on a prompt event → Action inject.
- An inject unit whose `delivered.at` is within the window → skip redeliver; beyond the window → delivered.
- Two units with the same topic → only the higher-ranked one is kept, the other is skip topic.
- `max_inject_chars` truncation: for inject, units from the second on that exceed the limit are marked truncated, while the first is always kept however long; for block, blocking units are never truncated.
- st nil → the same result as an empty state.

journal:
- Append two records, read them back with Since, fields match.
- With storeContent false, the original content, shell, and prompt are nowhere in the file, and the fields are strings starting with `sha256:`.
- The Event's `Input` does not appear in the file.
- Appending across UTC midnight writes two files.

## Done when

`make check` passes. Commit `runtime`.
