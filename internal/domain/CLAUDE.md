# Zones inside internal/domain

**THIS PACKAGE HOLDS NO RED FILE ANY MORE.** Every file here is YELLOW.

**The move that made that true is governed by DEC-008 since 31 August 2026.** This
document is the long account and stays the long account; DEC-008 is the record, and
where the two disagree the record wins. It was written six days after the move, and
this file's own account of being stale for a day is why nobody assumed a record
existed.

| File | Zone | Owner |
| --- | --- | --- |
| `compute.go` | YELLOW since 25 August 2026 | Al and Claude both. Every write still surfaces: the entry in `.claude/settings.json` became `ask`, not nothing |
| `flags.go` | YELLOW | added 26 August 2026, at the path `docs/methodology/09-flags-and-bands.md` line 5 had already been naming for days |
| `types.go` | YELLOW | Claude may write, and must explain each design decision in three sentences and name one rejected alternative |
| `arch_test.go`, `*_test.go` | YELLOW | same |

The three-sentence rule applies to all of them, and it applies hardest to
`compute.go`, because a formula is a claim that has to be defended to a reviewer or
a funder while a type is only a shape.

## What this document said until 26 August 2026, and why that matters

It said `compute.go` was RED, that Claude had no write permission, and that the
correct response to being asked to write it was to refuse. All three had been false
since 25 August, when Al moved the file to YELLOW so that Deliverable 1 would not be
gated on a single writer.

That is the exact failure this repository keeps recording about itself, running the
other way. A stale lock pointing at a path that no longer exists reports success
while protecting nothing; a stale zone document pointing at a rule that no longer
exists refuses work the map permits. Neither one fails loudly. The root `CLAUDE.md`
was updated on the 25th and this file was not, so for one day the package's own zone
document contradicted the map that governs it.

## Why the zone was a file and not a directory, while it lasted

It used to be a directory, `internal/depth`. Methodology 1.0.3 moved the
computations into this package and left the lock pointing at a directory that no
longer held anything, so for a while the red zone existed in `CLAUDE.md` and
nowhere else. The zone follows the code, not the name. The empty directory was
removed on 23 August 2026.

When `compute.go` went yellow the hook's file rule and its directory rule for
`internal/domain` came out with it, because a lock over a package where every file
is writable refuses ordinary work while protecting nothing. What did NOT come out:
`testdata/fixtures/` and `docs/context/`, both still denied in `.claude/settings.json`
and still closed in `.claude/hooks/lindungi-zona-merah.sh`.

## What replaced the lock

**A function in `compute.go` may only be written after its expected values exist in
`testdata/fixtures`.** No permission layer can tell whether a number was computed
before or after the code that satisfies it, so this rule is enforced by nothing.
That is why the fixture lock matters more now than it did while one person wrote
both sides: it is the only remaining structural reason to believe the expected
values are independent of the implementation.

Read the header of `compute.go` before adding to it. It lists which functions have
a hand computed oracle and which do not, and the second list is not empty: the AMM
half of the depth and manipulation formulas, and the whole of
`ComputeMaxSafeCollateral`, are implemented from the methodology and checked only by
invariants. `testdata/fixtures/ustry_pre_exploit.md` line 30 still records
`Pools: []` while `GoldenSnapshot()` carries the pool that genuinely existed, which
is handoff item B-4 and is Al's.

## The formatting rule survived the zone it used to serve

Three routes once reached this package: naming a file, naming the directory, or
sweeping a tree that contained it. All three were closed by 24 August and the first
two reopened on the 25th along with the zone. **The formatting rule stayed anyway**,
deliberately, because formatting having one owner and CI's gofmt check having one
fix is a workflow rule rather than a zone rule. `gofmt -l .` is read only and always
allowed; a write has to name its files.

| Want to | Do this | Not this |
| --- | --- | --- |
| format | `gofmt -w internal/domain/types.go` | `gofmt -w internal/domain/` |
| check formatting | `gofmt -l .`, which is read only | `make fmt`, which writes |

`make fmt` is refused for Claude and unchanged for Al, whose terminal this hook does
not sit in. So CI's gofmt check still has an owner and a one command fix, and Claude
formats by naming files. Finding P2-6c.

## Claude's role in compute.go now

Writing it is permitted. Three obligations come with that, and they are what is left
of the lock:

1. **Check the ordering rule first.** Name, out loud, which fixture value the
   function you are about to write is judged against. If the answer is "none", say
   so in the code and in the report rather than writing the number and moving on.
2. **Never touch `testdata/fixtures/` or the expected values in
   `internal/conformance/expected.go` to make a test pass.** `fixture.go` says it in
   its own header: adjust the code to match those numbers, never the reverse. A
   disagreement between the two IS the finding, and editing either side destroys it.
3. **Explain each design decision in three sentences and name one rejected
   alternative**, the same rule `types.go` has always carried.

Still worth doing before writing anything: read the code, run the tests, and point
out an edge case that is not handled. The question below is the kind that finds one,
and it is kept here because it is a better use of five minutes than a first draft.

> If the best SDEX ask sits at 0.101 and the AMM curve only crosses 0.101 after 400
> units, how many units can be absorbed before the combined marginal price moves?
> Which one runs out first?

## Conceptual traps to watch for

- Summing SDEX and AMM depth separately. This is WRONG. Both compete at the same
  marginal price. See `docs/methodology/04-depth.md` section 3.
- Using `float64` anywhere, including as an intermediate value.
- Assuming the order book always has both sides.
- Forgetting that an AMM is a continuous curve, not a set of discrete levels.
- Reading `Cost` without reading `Reachable`. The two are computed from disjoint
  sets of asks and a cost to an unreachable target is not a cost.

## The cleanup Claude could not do, done

Al ran `git rm -r internal/depth` on 23 August 2026. The directory held nothing
but its own `CLAUDE.md` in Indonesian, and Claude could not remove it, because the
lock that makes a zone real also makes it unremovable from this side.

Three things went with it, and each one had to be retired by hand rather than
disappearing along with the directory:

- The `internal/depth/**` denials in `.claude/settings.json` and the path in
  `.claude/hooks/lindungi-zona-merah.sh`. A lock on a path that does not exist
  cannot be distinguished from a lock that is working.
- The `../depth` entry in `paketMurni` in `arch_test.go`, and the
  `os.IsNotExist` tolerance that existed only to let that entry point at nothing.
- The single untranslatable exception DEC-005 recorded, which was that Indonesian
  file. DEC-005 section 2 records it as retired by deletion rather than by
  translation.

That list is the point. A directory is deleted in one command and the things that
referred to it are not, and every one of them fails silently rather than loudly:
a deny rule matching no path, a linter exclusion matching no file, and a purity
scan over a directory that is not there all report success.
