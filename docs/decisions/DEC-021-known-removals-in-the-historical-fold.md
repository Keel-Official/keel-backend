# DEC-021: An offer can leave the book without emitting an event, so the historical fold is told about it by hand or it stays wrong

**Status:** **Accepted (was DRAFT)**
**Date drafted:** 2026-09-14
**Kind:** Method. It changes what the historical reconstruction is allowed to be told,
and therefore what `dataSource: offers-implied` claims about itself. It changes no
formula, no threshold and no contract schema.
**Drafted by:** Claude
**Decided by:** Al
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. Sections 2, 3 and 5 are measurements and code
readings and are evidence, not decision.
**Rests on:** `docs/evidences/2026-09-12-crossed-book-ustry-february.md`, which proves
the defect and eliminates every cause the fold could have seen.
**Relates to:** DEC-002, which deferred the second data source and left this path as the
only route to a past book; DEC-010, which requires a provenance sidecar beside every
backtest artefact, and which this record extends rather than amends.

---

## 1. The decision

**Al, 14 September 2026. Option A adopted. The four points below are taken.** The section 5
acceptance test passed on all pre-registered criteria: `best_ask` 106.7372828, `p0`
53.8971414, `crossed_points` 0, and the `known_removals` boundary exact at 9 February.
Artefact:
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.csv`
and its sidecar.

Two findings are recorded and NOT folded in: `max_reachable_price` reads the sentinel on
the control rows, and a dust offer at that sentinel resembles a second silent removal. It
is named, not added, per section 4. The fold remains incomplete, `fold_complete` is false.
Both are carried in `11-limitations.md` rather than corrected here.

### 1.1 What was adopted, restored

**These four points were the proposal this record was drafted around, and the decision
above overwrote them.** They are restored verbatim beneath the decision rather than left
implicit, because a record whose decision reads "the four points of section 1 are taken"
above a section 1 that no longer lists any is a record that cannot be audited by the
person it was written for. Nothing here changes the decision; it states what the decision
adopted. Amending a record is Claude's under the zone map, and creating or reversing one
is not.

1. **A removal list is admissible input.** One file, `configs/known-removals.json`,
   holding one entry per offer: the offer id, the ledger by which it is proven absent,
   the evidence document, and one sentence of why.

2. **It is never applied unless asked.** Both commands need `-known-removals` on the
   command line. A run without it reconstructs exactly what it reconstructed before this
   record existed.

3. **Every artefact says which run it was.** The sidecar names the file and lists each
   declared removal; the CSV carries a per-row `known_removals` column, because a
   removal proven at one ledger leaves every earlier row untouched and a reader
   comparing two rows is entitled to know which of them was repaired.

4. **`offers-implied` keeps its name, and the methodology says what it now means.**
   `docs/methodology/01-data-sources.md` gains one paragraph: the historical book is
   reconstructed from operations and trades, PLUS any removal this repository has proven
   by hand, and a run that used one is identified in its own provenance.

**What this record does NOT adopt** is inferring removals. Nothing in it lets the code
decide for itself that an offer is a phantom. Section 4 is why.

---

## 2. What the fold cannot see, stated once

`internal/horizon/replay.go` folds two event streams: offer operations and trades. An
offer leaves the reconstructed state only when one of them says so.

On 12 September 2026 offer `1822775941` was proven to have left without either. One
stroop of USTRY at price `1.0573892029461328`. Horizon's per-offer trade index returns
exactly two trades for it, both on 8 February. A forward walk of its owner's operations
from ledger 61143617 to 61344294 names it in five operations, all at ledger 61143618,
and never again. The issuer performed no `allow_trust`, no `set_trust_line_flags` and no
`clawback` in the range. And at ledger 61143682 a trade executed ABOVE its price, which
a buy could not have done while a cheaper ask rested, so it was gone by then.

**The candidate mechanism is the matching engine's own dust handling** and that document
declines to call it proven. What IS proven is the elimination, and the elimination is
what decides this record: **a deeper walk cannot fix this and neither can a wider trade
window, because there is no page to find.**

**The cost of leaving it unrepaired is twenty days, not six rows.** The phantom sat below
the real ask from 9 February to 28 February. Six rows crossed and announced themselves.
Thirteen did not, and on those `best_ask`, `p0`, `spread_pct` and both depth ladders are
wrong in exactly the same way while looking entirely ordinary.

---

## 3. What is built, and what it deliberately does not do

`internal/horizon/removals.go`, with tests in `removals_test.go`.

| Piece | What it does |
|---|---|
| `KnownRemoval` | offer id, `gone_by_ledger`, evidence path, why. `LoadKnownRemovals` REFUSES an entry missing any of the four |
| the fold | a removal enters as a synthetic delete event at TOID `gone_by_ledger << 32`, in TOID order with the real events |
| `ReplayResult.KnownRemovalsApplied`, `SeriesPoint.KnownRemovalsApplied` | which ids actually took an offer OFF that book, sorted. Not which ones were eligible: see section 5 |
| `keel replay -known-removals`, `keel bookseries -known-removals` | opt in, per run. Both print the count; neither defaults to a file |
| the CSV | a `known_removals` column beside `crossed` |
| the sidecar | `known_removals_file`, `known_removals_declared`, and one line per declared removal naming its evidence |

**A synthetic EVENT rather than a filter over the result, and the two differ in exactly
one case:** an offer proven gone at ledger L may be re-created above L by an operation
the fold can see, and a filter would delete the re-creation too. As an event it deletes
what was resting and nothing that arrives afterwards. There is a test for that case.

**The TOID is one operation earlier than the proof strictly supports.** What the evidence
establishes is absence AT ledger 61143682, and the event is placed at the start of it.
The difference can only appear at a target exactly equal to that ledger, and no target in
this repository is one.

---

## 4. The objection, which is real, and why the shape answers it

**The objection: a file the fold obeys is a file somebody can put anything into.** A
reconstruction that can be handed corrections is a reconstruction whose output can be
steered, and this one produces the evidence for a paid deliverable about an incident.
That is not a hypothetical risk in a repository whose whole zone map exists because
numbers that check the code must not be produced by the code's author.

Four properties answer it, and none of them is a promise:

1. **Evidence is a required field.** An entry without a document is refused at load.
   `TestTheRepositoryRemovalListLoadsAndNamesItsEvidence` fails if a cited document is
   not in the repository.
2. **Off by default.** Every figure this repository has already published is reproducible
   without the flag, unchanged.
3. **Declared in the artefact.** The sidecar and the per-row column mean a repaired
   reading cannot be presented as an unrepaired one by accident.
4. **It can only ever REMOVE.** The mechanism cannot add an offer, so it cannot make a
   book deeper, and the direction it can push a reading in is the conservative one for a
   product whose job is to warn.

**What no property fixes:** it removes offers a person chose to look at. Offers nobody
looked at keep whatever the fold gave them. That is the honest limit and section 7 says
where it has to be written down.

**The alternative rejected: let the fold repair a crossed book itself**, by dropping
whichever level crosses. Rejected because nothing in a book alone says WHICH of the two
crossing offers is the phantom, and resolving the February pair took a separate walk of
each side; an automatic dropper would be guessing, and it would repair only the six rows
that announce themselves while leaving the thirteen quiet wrong ones untouched and
declaring the run clean.

---

## 5. The acceptance test, and its result

**The test is the golden fixture, and it is the only test available that was not produced
by this code.** `testdata/fixtures/ustry_pre_exploit.md` gives the book at ledger
61340263 by hand: one ask of 1.2185312 at 106.7372828, one bid of 0.0001 at 1.057,
`P0` 53.8971414, `spreadPct` 196.0777141.

The unrepaired cap400 run of 10 September reports, at that ledger, `best_bid` 1.057 and
`ask_amount_total` 1.2185315, both agreeing, and `best_ask` 1.0573892029461328, which is
the phantom rather than the fixture's 106.7372828. `p0` is 1.0571946 against the
fixture's 53.8971414.

**A repaired run must move `best_ask` to the fixture's value and `p0` with it, and must
change nothing on the bid side.**

**A SHALLOW-FLOOR CONTROL RUN DOES NOT TEST THIS, AND ON 14 SEPTEMBER 2026 ONE WAS
BRIEFLY READ AS THOUGH IT DID.** The control command in the report's section 9 uses
`-since-ledger 61300000`. The phantom was created at ledger 61143618, which is below that
floor, so it is invisible to that run whether or not a removal is declared: the run
reproduced the fixture exactly, as it already did on 5 September 2026 without any repair
existing. The misreading is recorded here rather than quietly corrected, and it produced
one code change: `KnownRemovalsApplied` now reports the removals that actually took an
offer OFF the book instead of the ones that were merely eligible at that target, so the
column can no longer credit a repair with a book it did not touch.

**The test therefore needs the DEEP floor, 60987032**, which is the month run's floor and
the only configuration in which the fold can see the phantom's create at all.

### 5.1 The result, 14 September 2026

The deep-floor repaired run finished at 16:00, 8,312 seconds and 1,719 requests over 222
accounts. Artefact:
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.csv`
and its sidecar. The four criteria were written down before the run finished, in that
day's session and in this section, and none was adjusted afterwards.

| # | Criterion | Result |
|---|---|---|
| 1 | `best_ask` at ledger 61340263 is the fixture's 106.7372828 | **PASS.** 106.7372828 |
| 2 | `p0` is the fixture's 53.8971414 | **PASS.** 53.8971414 |
| 3 | `crossed_points` is 0 | **PASS.** 0, against 6 on the unrepaired run |
| 4 | `known_removals` is empty on 1 to 8 February and carries 1822775941 from the 9th | **PASS.** Exactly that boundary |

Three figures the criteria did not name and that also land on the fixture:
`spread_pct` 196.07771405850478 against 196.0777141, `best_ask_amount` 1.2185312 at ledger
61340262, and 1.1684309 at 61340263, which is that amount less the manipulation's own fill
of 0.0501003.

**What the repair actually moved, measured against the unrepaired run of the same
command.** On the twenty daily rows from 9 February the phantom was the best ask, so `p0`
moves in the fifth decimal and depth at 2 per cent moves by about one part in 10^12. On
the two control ledgers, where the real book is nearly empty, the same offer was the
difference between `p0` 1.0572 and `p0` 53.8971414, and between a depth of 1.06e-7 and a
depth of 0. **No band changes anywhere.** The phantom mattered enormously where the book
was thin and almost not at all where it was deep, which is the opposite of how a
one-stroop offer sounds.

### 5.2 FOUR fields disagree with the fixture, from one cause, and it is not the repair

**This subsection first said one field and that was wrong.** Checking the fixture's
manipulation-cost table rather than only its price table found three more, all produced by
the same offer.

| Quantity at ledger 61340263 | Fixture, by hand | Repaired deep-floor run |
|---|---|---|
| `maxReachablePrice` | 106.7372828 | **2147483647** |
| `costToMaxReachablePrice` | 0 | **124.71513940555852** |
| `Reachable` at δ = 1, 10, 100 | **false** | **true** |
| asks on the book | 1 | **2** |

Everything else still agrees, including the four pre-registered criteria, `spread_pct`,
the whole depth ladder at zero, all four flags and the `CRITICAL` band.

**The cause is the operation floor and not the removal.** The deep floor, 60987032, makes
an ask visible that the 5 September control run at floor 61300000 never saw: a dust offer
at the sentinel price 2147483647. With such an ask resting, every manipulation target is
satisfied by an ask at or above it, so `Reachable` becomes true at every rung and the
maximum reachable price becomes the sentinel. The fixture's most important line, quoted in
its own words as "the highest price an attacker can reach is 106.74 and reaching it is
free", is exactly the line this offer breaks.

**Two readings, and neither is settled here.** Either the sentinel ask is a second silent
removal of the kind section 2 describes, or the fixture's hand computation did not include
an offer created long before the two it was built from. The fixture is the authority in
this repository and the rule is to adjust the code to it, not it to the code; but that
rule presumes the two are computing the same book, and a floor that reaches further back
than the hand computation did is a different book.

**Nothing is done about it here, on purpose.** Adding the sentinel offer to the removal
list on a resemblance is precisely the failure mode section 4 is built to prevent, and
editing the fixture is forbidden outright. It is named, and it is the first item any
reading of the manipulation-cost columns at the control ledgers has to carry.

### 5.3 What still reads incomplete

`fold_complete` is false on every row and `walk_complete` is false for the run:
`missing_offer_ids` 3, `walks_truncated` 35, `walks_failed` 6, `walks_stopped_at_floor`
167. The repair addresses one named offer and changes none of those. Any reading of this
series still has to carry them.

---

## 6. The choice, and it was made

**DECIDED 14 September 2026: option A.** The table is kept rather than deleted, because
what was rejected is part of what was decided, and option B in particular was a real
option that a later reader is entitled to weigh against what happened.

| Option | What it means | Cost |
|---|---|---|
| **A. Take all four points of section 1** | the February series is repaired, the report can cite `best_ask` and everything derived from it, and the methodology says what `offers-implied` now includes | one paragraph in `01-data-sources.md`, which is RED and is Al's to write |
| **B. Take the mechanism, publish unrepaired** | the code stays, the flag stays off, and the report publishes the unrepaired series with `best_ask`, `p0`, `spread_pct` and both depth ladders marked not citable from 9 February onward | the report loses its price and depth series and keeps its bid-side findings, which is most of section 6's answer but not the table of section 5 |
| **C. Reject the mechanism** | `internal/horizon/removals.go`, its tests and the config file are deleted | the February reconstruction is knowingly wrong on twenty rows and stays that way, and the report says so |

**B is a real option and not a courtesy.** The finding that answers the report's title,
recorded in `docs/evidences/2026-09-14-february-22-withdrawal-timeline.md`, rests
entirely on bid-side offers whose operations are quoted, and the phantom touches none of
it. Option B publishes that finding on time and leaves the price series for the SCF Build
phase.

---

## 7. If A is taken, what else has to move

1. `docs/methodology/01-data-sources.md`: one paragraph defining what `offers-implied`
   includes, and the sentence that a run which used a removal is identified in its own
   provenance. RED, Al's.
2. `docs/methodology/11-limitations.md`: the class limit, which is the sentence option A
   makes it easy to forget. An offer can leave the Stellar order book without emitting an
   operation or a trade; this repository knows of one and has looked for it in one pair
   over one month. RED, Al's.
3. `docs/report/blend-february-2026.md` section 8: the same limit, in the client's
   language, beside the series that depends on it.
4. `CLAUDE.md`: `configs/` is described as DATA, never methodology. A removal list is
   data with a proof attached, and whether that row needs a clause is a small question
   that should be answered rather than left for a later reader to discover.

---

## 8. Version history

| Date | Change |
|---|---|
| 14 September 2026 | Drafted. The mechanism was built first and left off by default, so that the choice in section 6 could be made against a measurement rather than against a description. Carries the shallow-floor misreading that changed what `KnownRemovalsApplied` reports |
| 14 September 2026, later | Section 5 closed. The deep-floor run passed all four pre-registered criteria. Section 5.2 records the one field that disagrees with the fixture, `max_reachable_price`, and attributes it to the floor rather than to the repair |
| 14 September 2026, accepted | Al adopted option A and wrote `01-data-sources.md` section 1.4 and `11-limitations.md` point 8. The decision overwrote the proposal it adopted; Claude restored the four points as section 1.1 and marked section 6 decided. Neither edit changes the decision |
