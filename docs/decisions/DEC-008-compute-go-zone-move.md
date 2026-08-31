# DEC-008: `compute.go` moved from RED to YELLOW, and what replaced its lock

**Status:** **Accepted** by Al on 2026-08-31, the day it was drafted
**Date:** 2026-08-31
**Kind:** Retrospective ratification. The move was decided and executed by Al on
25 August 2026. This record does not make the decision and could not; it writes
down the terms of one already in force and asks Al to confirm them.
**Decided by:** Al, 25 August 2026
**Drafted by:** Claude, six days after the fact
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and
must not create or reverse a decision, which is why the paragraph above is the
first thing in this file.
**Methodology version at the time of this record:** `1.0.3-draft`, unchanged by
this record. A zone is a rule about who writes a file, not a definition.
**Number:** reserved for this subject by the deliverable 1 breakdown of
2026-08-28 and again by DEC-009 section 9 item 2, which asks that `compute.go`
not be touched under any other record until this one exists.

---

## 1. Why this record exists, and what it is not

On 25 August 2026 `internal/domain/compute.go` moved from RED to YELLOW so that
Deliverable 1 could be finished by two hands instead of one. It is the largest
loosening of permission in this repository. **No decision record governed it for
six days.**

The root `CLAUDE.md` row for the file says so outright, and its wording records
one attempt at a fix that was correctly abandoned: the row carried a placeholder
decision number until 26 August 2026, when all records then in
`docs/decisions/` were searched, none was found to cover a zone reclassification,
and the pointer was **deleted rather than aimed at the nearest plausible
record**, because a wrong number reads as authority the move does not have.

So the largest loosening here has been written in prose only, in three places:

| Where | What it carries |
| --- | --- |
| root `CLAUDE.md`, the `internal/domain/compute.go` row | the move, the date, the reason, and the admission that no record governs it |
| the header of `internal/domain/compute.go` | the move, what replaced the lock, and which functions have a hand computed oracle |
| `internal/domain/CLAUDE.md` | the long account, including the day this file itself was stale |

This record is the fourth place, and it is meant to be the one the other three
point at rather than a fifth home for the same facts. **It states the terms of the
move. It does not restate the ordering rule as a definition**, because
`CLAUDE.md` owns that sentence and this repository has lost five times to a rule
acquiring a second home next to its first.

What this record is NOT: it is not permission for a further loosening, and it is
not a reversal. Nothing in `.claude/` changes under it.

---

## 2. Ratified: the move, and the reason given for it

`internal/domain/compute.go` is YELLOW as of 25 August 2026. Claude may write it,
under the three obligations in section 5.

**The reason, as recorded on the day:** Deliverable 1 was gated on a single
writer. The methodology computations are the bulk of FR-1 through FR-7, the
red zone made Al the only person who could write them, and the sprint is 30 days.

**What the zone had been.** RED, and before that the zone was a different path
entirely. It was `internal/depth` until methodology 1.0.3 moved the computations
into `internal/domain`, at which point the lock pointed at a directory that held
nothing. Al removed the empty directory on 23 August 2026. The zone follows the
code and not the name, and this is the third version of that map.

---

## 3. What came out with the lock

| Mechanism | Before | After |
| --- | --- | --- |
| `.claude/settings.json`, `Edit`/`Write` on `internal/domain/compute.go` | `deny` | **`ask`**, not removed. See section 4 |
| `.claude/hooks/lindungi-zona-merah.sh`, the file-name rule for `compute.go` | present | removed |
| the same hook, the DIRECTORY rule matching `internal/domain` in directory form | present | removed |

The directory rule had to go with the file rule. A lock over a package where
every file is writable refuses ordinary work while protecting nothing, and P2-6b
in `scripts/audit-verification.sh` is the line that asserts `types.go` next door
is not refused.

---

## 4. What deliberately did NOT come out, and it is worth as much as what did

A zone change is a licence to loosen what the zone change required, and nothing
else. Four things were kept:

1. **The deny entry became `ask` rather than disappearing.** Every write to
   `compute.go` still surfaces to Al. The move removed the refusal, not the
   visibility.
2. **`testdata/fixtures/` did not move.** It is still `deny` in the permission
   file and still closed in the hook. Section 5 is why this now matters more than
   it did before the move, not less.
3. **`docs/context/` did not move.** The inputs from outside stay closed.
4. **The formatting rule survived the zone it used to serve.** `make fmt`,
   `gofmt -l -w .` and `gofmt -w some/directory/` are still refused because none
   of them names a file, even though no RED zone holds a `.go` file any more. It
   protects nothing now and is kept on purpose, so that formatting has one owner
   and CI's gofmt check has one fix. P2-6c is the line that would have to change
   if this is ever dropped, and dropping it would be Al's.

Two lessons from the FIRST loosening, P2-6d on 24 August 2026, were applied to
this one and are recorded here because they are the transferable part:

- **Probe a loosening before it lands, not after.** P2-6d reopened eight routes,
  two of them ordinary forms of a command rather than exotic ones, and they were
  found by probing.
- **Give every loosening a check of its own.** This one got two, P2-12 and P2-13,
  and five existing checks were re-anchored rather than deleted.

---

## 5. What replaced the lock, and no mechanism enforces it

The rule is one sentence and `CLAUDE.md` owns it: **a function in `compute.go`
may only be written after its expected values exist in `testdata/fixtures`.**

No permission layer can tell whether a number was computed before or after the
code that satisfies it. This rule is therefore enforced by nothing, and that is
the honest position rather than an oversight. P2-13 proves the sentence exists in
the zone map. It cannot prove the sentence was followed, and its own comment in
the audit script calls it the weakest check in that file.

The consequence is the part that deserves ratification, because it is a real cost
accepted in exchange for a real gain:

> While one person wrote both the fixture and the code, the independence of the
> expected values was a property of the arrangement. Since the move it is a
> property of a rule nobody can check. **The fixture lock is now the only
> structural reason to believe the expected values are independent of the
> implementation**, which is why item 2 of section 4 is not a detail.

Three obligations travel with the write permission, and they are what is left of
the lock. They are stated in `internal/domain/CLAUDE.md` and are referenced here
rather than redefined:

1. Name, out loud, which fixture value the function being written is judged
   against. If the answer is none, say so in the code and in the report.
2. Never edit `testdata/fixtures/` or the expected values in
   `internal/conformance/expected.go` to make a test pass. A disagreement between
   the two IS the finding, and editing either side destroys it.
3. Explain each design decision in three sentences and name one rejected
   alternative, the standing YELLOW rule.

---

## 6. What is now unverified, named rather than implied

The move is defensible only if the gap it opened is visible. The header of
`compute.go` carries this list and it is reproduced here because a reader of the
decision should not have to open the code to learn the price of it:

| Function | Hand computed oracle |
| --- | --- |
| `MidPrice` | YES for the two-sided-book rung, `P0 = 53.8971414` |
| `ComputeDepth` | SDEX walk YES, as a correct zero. **AMM term NO** |
| `ComputeManipulationCost` | `orderbookOnly` YES, four rows. **`includeAMM` NO** |
| `ComputeMaxSafeCollateral` | **NO.** `08-collateral.md` is complete and the fixture is silent |

The AMM half is implemented from `04-depth.md` sections 2 and 3 and is checked
only by invariants, never by a number computed by hand. For the first three rows
the cause is a single line: `testdata/fixtures/ustry_pre_exploit.md` line 30 still
records `Pools: []` while `GoldenSnapshot()` carries the pool that genuinely
existed. That is handoff item B-4, it is Al's, and until it closes, agreement
between `compute.go` and DEC-006 section 2 proves only that two readings of the
same document agree.

**The fourth row has a different cause and B-4 does not touch it.** The fixture
carries expected values under `Reference price`, `Depth`, `Manipulation cost`,
`Maximum reachable price` and `Flags and band`, and **no collateral section at
all**. So `C_max` has no hand computed oracle for any snapshot, with or without a
pool, while `08-collateral.md` is marked complete in the methodology index. Adding
the pool to the fixture will not produce one. This is recorded as a separate open
item in section 11 rather than folded into B-4, because folding it in is how it
would be missed.

---

## 7. Rejected alternative

**Keep `compute.go` RED and have Al write all of it.** Rejected on 25 August for
the reason in section 2: it gates the bulk of FR-1 through FR-7 on one writer
inside a 30 day sprint, and the deliverable was the thing at risk.

The cost of the rejection is section 5, stated plainly: the independence of the
expected values moved from a property of the arrangement to a rule enforced by
nothing. That trade is the decision. Anyone who thinks it was the wrong trade
should reverse it as a reversal, recorded as one, and not by editing this text
away.

A second alternative, **move the whole of `internal/domain` and say so**, is not
rejected here because it is what happened: every file in the package is YELLOW,
which `internal/domain/CLAUDE.md` states in its first line. The record of that is
that document, and the root map's row for the package agrees with it.

---

## 8. Design note, required by the YELLOW zone rule

Three sentences on the one design decision in this record, which is what it
declines to contain. This record deliberately does not restate the fixture-first
ordering rule as a normative sentence, because `CLAUDE.md` owns it and P2-13
reads `CLAUDE.md` and not this file. The rejected alternative was to make this
record the home of that rule, which would have been tidier to read and would have
created exactly the second home the repository has been defeated by five times,
with the added defect that the harness would then be checking the wrong file.

---

## 9. Consequences for the harness

**P2-12 reads NOT today and it is a false pass.** The check asks whether any file
in `docs/decisions/` names `compute.go` alongside the word yellow. DEC-007
mentions `compute.go` four times about `maxReachable` and carries the word yellow
in its own zone header, and DEC-009 now does the same in its files-affected
table, so the line reported the move as governed while nothing governed it. The
check's own comment predicted that failure mode in advance and calls the weaker
form the honest one, because matching the phrasing of a record nobody had written
would have been guessing at prose.

A tightening was drafted on 28 August and **Al chose to leave the check loose.**
That decision stands and is not reopened here. What this record changes is the
thing underneath the check: once it is ratified, P2-12 reads NOT for the reason it
was written for, and the false pass stops mattering because the claim it was
falsely reporting becomes true.

**P2-13 is unaffected and stays PROVEN or NOT on its own terms**, which depend on
`CLAUDE.md` and not on this file. Nothing here should be written to satisfy it.

---

## 10. Files affected

| File | Zone | Change under this record |
| --- | --- | --- |
| `docs/decisions/DEC-008-compute-go-zone-move.md` | YELLOW | this file |
| root `CLAUDE.md`, the `compute.go` row | YELLOW | may point at this record once it is Accepted, replacing the sentence that no decision record governs the move. **Al's, because it is his brief** |
| `internal/domain/compute.go` header | YELLOW | same pointer, same condition |
| `internal/domain/CLAUDE.md` | YELLOW | same pointer, same condition |
| `.claude/settings.json` | GREEN to TIGHTEN, RED to LOOSEN | **no change.** This record ratifies a state, it does not alter one |
| `.claude/hooks/lindungi-zona-merah.sh` | GREEN to TIGHTEN, RED to LOOSEN | **no change** |
| `scripts/audit-verification.sh` | GREEN | no change. See section 9 for why not |
| `testdata/fixtures/` | RED | **no change, and this is load-bearing.** Section 4 item 2 |

The three pointer edits are listed as consequences and are deliberately not made
in the same commit as this draft. A record that is still Proposed must not already
be cited as authority by the files it governs.

---

## 11. Open items handed back

1. **Ratify or reject.** This record is Proposed. If the terms in sections 2
   through 6 are not the terms of the move Al made, the draft is wrong and the
   correction is his.
2. **B-4, the fixture with-pool numbers.** Closing it converts the AMM term of
   `ComputeDepth` and the `includeAMM` form of `ComputeManipulationCost` from NO
   to YES, which is two of the four rows in section 6 and the single largest
   reduction in the cost of this move.
3. **The fixture has no collateral section, and that is not B-4.** Section 6
   records it. `ComputeMaxSafeCollateral` is unverified by number today and stays
   unverified after B-4 closes, because the gap is a missing `C_max` table rather
   than a missing pool. Whether the golden fixture gains one, or `C_max` is
   verified some other way, or the gap is accepted and written into
   `11-limitations.md`, is Al's and no record asks it yet.
4. **The three pointer edits in section 10**, once the status changes.
5. **The `DEC-003` number is used twice**, on the API contract v1.1 record and on
   the USTRY/USDC pool ledger 61340263 record. This record cites both by subject
   for that reason. DEC-009 section 9 item 3 records the collision and it is not
   resolved here.

---

## 12. Amendment history

This record is append-only from its first commit. An amendment adds a row here and
text below the section it concerns, and no earlier sentence is edited or deleted,
which is the treatment DEC-006 section 9 and DEC-009 section 10 give themselves.
A document that quietly grows correct hides the fact that it was wrong, and the
fact that it was wrong is the finding.

| Date | Amendment |
| --- | --- |
| 2026-08-31 | Created as Proposed, six days after the move it records |
| 2026-08-31 | Accepted by Al. Section 11 item 1 is closed. The three pointer edits in section 10 were made in the commit that carries this row, which is the commit after the one that created the record, so that no file cited this record as authority while it was still Proposed |
