# DEC-009: Manipulation cost is bounded by price, not gated by venue reach

**Status:** Accepted
**Date:** 2026-08-28
**Decided by:** Al
**Drafted by:** Claude, acting as reviewer
**Zone:** YELLOW (`docs/decisions/`)
**Methodology version at the time of this decision:** `1.0.3-draft`, unchanged by
this decision. See section 7.
**Owning file for the rule itself:** `docs/methodology/05-manipulation-cost.md`
section 1

---

## 1. Context

Methodology 1.0.3 split manipulation cost into two venue forms,
`manipulationCostCombined` and `manipulationCostOrderbookOnly`, and recorded that
split in `05-manipulation-cost.md` section 3, in `internal/domain/types.go`, in
contract 1.3.0, and in migration `0003_venue_split_and_offers_implied.sql`.

Section 1 of the same methodology file was not revised alongside it. Its definition
block is still written in terms of asks alone. The consequence is that the
orderbook-only form has a written definition and the combined form does not, while
`internal/domain/compute.go` has an implementation of both.

R-7 step 3 asked what the combined form does with a venue that cannot attain
`P_target` on its own. The question was raised as a question about `Reachable`. On
review it is not one. `Reachable` is an existential statement over venues, and a
venue that fails to attain the target does not change the truth value of an
existential statement. The undefined quantity is `MC`, not `Reachable`.

---

## 2. Decision

Two readings of the combined form were put to Al.

| Reading | Treatment of a venue that cannot attain `P_target` on its own |
| --- | --- |
| A, bounded | The venue still contributes all of its liquidity priced below `P_target` |
| B, gated | The venue contributes nothing to the combined figure |

**Al chose reading A.**

`Reachable` is confirmed unchanged. It remains an existential statement, and the
already recorded consequence that an active pool makes it unconditionally true on
the buy side, currently stated in DEC-003 (API contract v1.1) section 4.3 and in
`compute.go`, is carried into the owning methodology file rather than left in those
two places.

---

## 3. What this record does not do

This record names a reading. It does not state the rule.

`docs/methodology/05-manipulation-cost.md` section 1 owns the rule, and this
repository has lost five times to the pattern of a definition acquiring a second
home next to its locked one. A decision record that restated the formula would be
that pattern in its most respectable disguise. Anyone implementing against this
document must read section 1 of the methodology file, not this section.

---

## 4. Evidence

All three items below were in the repository before this decision was taken. None
of them was produced by running the implementation.

### 4.1 The hand computed fixture already encodes reading A

DEC-006 section 2 records combined cost as the orderbook term plus the AMM term at
two rungs of the ladder: 130.0627 + 149.2224 = 279.2851, and 130.0627 + 372.0029 =
502.0656. The 130.0627 term is the whole of the order book. On the golden fixture
the highest ask is 106.7372828 while the ladder targets above delta 0.5 are
107.7942828, 592.8685554 and 5443.6112814, so at those rungs the book cannot attain
the target and its entire contents are nonetheless counted. Reading B would delete
that term from those rows and invalidate a table that was computed by hand before
any implementation existed.

The rung at delta 0.5, target 80.8457121, does not discriminate between the two
readings. Its orderbook term is zero because the single ask sits above the target,
so both readings agree there. It is a consistency check, not evidence.

### 4.2 The quantifiers over venues are different, and that is the whole point

For the price a consumer reads to arrive at `P_target`, no venue may still be
offering below `P_target`, because the price available in the market is the best
price available anywhere. A venue left un-walked keeps offering cheaper liquidity
and the price has not arrived. So the cost is paid at **every** venue, while the
target only has to be attainable at **some** venue.

`MC` therefore ranges over all venues and `Reachable` over one. A venue whose whole
book sits below `P_target` contributes cost without contributing reach, and that is
not an anomaly to be gated away, it is the shape the two quantifiers give it.

DEC-006 section 8 reaches the same conclusion from the other direction, in its own
words: both venues have to be moved to the same target for the market price to
arrive there, so the two amounts are independent and they do add.

**Non-negotiable rule 4 is deliberately not cited here.** An earlier draft of this
record did cite it. That was wrong, and DEC-006 section 8 says so in advance: rule 4
governs DEPTH, where the venues compete at one shared marginal price and a pool
priced above the target must contribute exactly zero. Manipulation cost to reach a
target is a different quantity, and borrowing rule 4's authority for it is precisely
the conflation DEC-006 wrote that paragraph to prevent.

### 4.3 Reading A is already what `ComputeDepth` does

`ComputeDepth` sums every ask at or below the buy target and then adds the pool
term, with no test of whether the book alone could attain the target. This is
offered as corroboration and not as an argument from rule 4: it shows that the
bounded treatment is already the habit of this codebase on the quantity next door,
so reading B would introduce an inconsistency rather than remove one.

---

## 5. Rejected alternative

Reading B was rejected. Its only merit is that it makes the combined figure read as
"the cost attributable to venues that could actually have done it", which is a
sentence a reader might want. That merit is not worth the cost: it contradicts the
hand computed table in DEC-006 section 2, it collapses the distinction between the
two quantifiers set out in section 4.2, and it would report a target as attained
while cheaper liquidity remained unconsumed at another venue.

---

## 6. Design note, required by the YELLOW zone rule

The rule is being generalised in the file that already owns it rather than restated
in a new one, because one subject per file is what `docs/methodology/README.md`
section 1 establishes and because a second home for this rule is the failure this
repository has repeated most often. `Reachable` is left structurally untouched even
though it is degenerate on the combined form when a pool is active, because Al has
already decided to keep it reported there with a contract note rather than removed,
and reversing that here would be a second decision smuggled into a record about a
first. The alternative rejected is the one in section 5.

---

## 7. `MethodologyVersion` does not move

Al decided on 28 August 2026 that generalising section 1 is an editorial
clarification and not a version bump. `MethodologyVersion` stays at `1.0.3-draft`
and no `1.0.3-draft` line in any methodology file moves.

The reason is that `methodologyVersion` exists so that a consumer knows when two
stored numbers stopped being comparable, as stated in `Keel_PRD.md` section 6. No
stored number changes here: the golden fixture already encodes reading A, so a
result computed before this clarification and one computed after it are the same
result. Raising the version would send a false incomparability signal on the exact
field that exists to prevent false signals.

The argument on the other side, which was heard and rejected, is that the combined
form previously had no written definition at all, and that going from undefined to
defined is a change of definition. It is recorded here because a decision **not** to
bump is easier to question later than a decision to bump, and the reasoning should
not have to be reconstructed.

---

## 8. Files affected

| File | Zone | Change |
| --- | --- | --- |
| `docs/methodology/05-manipulation-cost.md` | RED | Section 1 is generalised across venues, and the active pool consequence moves here from DEC-003 (API contract v1.1) section 4.3. Written by Al |
| `scripts/audit-verification.sh` | GREEN | New check P2-19, a tripwire that reads PROVEN while section 1 still speaks of one venue. New check P2-20 reports a pool-only ladder arriving in a stored position, and reports rather than forbids because the identity behind it is section 9 item 1 below, which is Al's |
| `docs/api/keel-openapi.yaml` | YELLOW | The `reachable` description on `manipulationCostCombined` states the degeneracy explicitly. Deferred until section 1 is written, so that the contract quotes the methodology and not this record |
| `docs/methodology/README.md` | RED | No version history entry, per section 7 |
| `internal/domain/compute.go` | disputed, see section 9 item 2 | No change. Its behaviour is already reading A |
| `testdata/fixtures/ustry_pre_exploit.md` | RED | No change. It already encodes reading A |
| `internal/domain/types.go` | YELLOW | No change |
| `migrations/` | GREEN | No change, and see section 9 item 1 for why one may never be needed |

---

## 9. Open items handed back

1. **The additive identity is now decidable, and DEC-006 section 8 left it to Al.**
   That section asks whether `combined = orderbookOnly + poolOnly` is a definition
   or a coincidence of this one fixture, and states that if it is a definition then
   option C is finished by saying so, with no column, field or migration needed.

   Reading A makes `MC` a sum over venues of each venue's liquidity priced below one
   shared `P_target`, and a sum over venues is decomposable by venue by
   construction. The identity would then be definitional. This holds **only**
   because R-7 step 2 already locked every venue to the same `P_target` from a
   single traversal of the `03-reference-price.md` ladder; if a pool-only figure
   were ever defined against its own reference price, the identity would fail. Al
   decides, and this record does not.

2. **The zone of `compute.go` is unsettled.** The repository brief lists it as RED,
   a later note records a move to YELLOW, and the deliverable 1 breakdown of
   2026-08-28 treats it as locked and reserves DEC-008 for governing the move. No
   change was made to it under this record, and none should be until DEC-008
   exists.

3. **`DEC-003` is used twice**, on the API contract v1.1 record and on the
   USTRY/USDC pool ledger 61340263 record. This record cites both by their subject
   for that reason. The collision is not resolved here.

---

## 10. Amendment history

This record is append-only from its first commit. An amendment adds a row here and
text below the section it concerns, and no earlier sentence is edited or deleted.
That is the treatment DEC-006 section 9 gives itself, and the reason is the same: a
document that quietly grows correct hides the fact that it was wrong, and the fact
that it was wrong is the finding.

| Date | Amendment |
| --- | --- |
| 28 August 2026 | Record created. Reading A accepted by Al |
| 28 August 2026 | Section 4.2 rewritten before first commit. The draft argued from non-negotiable rule 4, which DEC-006 section 8 explicitly fences to depth and warns against borrowing for manipulation cost. The conclusion is unchanged; the argument is now the quantifier asymmetry and DEC-006's own statement. Section 4.3 demoted from argument to corroboration for the same reason |
| 28 August 2026 | Section 7 added. Al decided the change is editorial and `MethodologyVersion` does not move. Section 9 item 1 opened: reading A makes the DEC-006 section 8 additive identity decidable |
