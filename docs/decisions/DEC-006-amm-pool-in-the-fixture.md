# DEC-006: The golden fixture is missing an AMM pool, and what that costs

**Status:** DECIDED 25 August 2026, options A and C together, by Al. See section 8,
which also records that most of both had already been executed on 23 August without
this document noticing. What remains is in that section and it is smaller than
section 6 suggests.
**Found:** 21 August 2026, while closing the DEC-002 spike DoD.
**Evidence:** `docs/evidences/pool_ustry_usdc_2026-02.txt`, reproducible with curl
and no account.

---

## 1. The finding

`testdata/fixtures/ustry_pre_exploit.md` records `Pools: []` for USTRY/USDC at
ledger 61340263. A liquidity pool for exactly that pair existed and held reserves
at that ledger.

| | |
|---|---|
| Pool | `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb` |
| Fee | 30 bp |
| Reserve quote, USDC | **16.3389179** |
| Reserve base, USTRY | **15.4791416** |
| Spot price | **1.0555441847** USDC per USTRY |

The USTRY issuer in that pool is `GCRYUGD5...`, the same issuer the fixture uses, so
this is not a different USTRY.

**How the reserve figures are known, link by link.** The last transaction to touch
this pool before the incident ledger was `fa012177...` at ledger 61172481, on
2026-02-10T16:59:35Z, twelve days earlier. Its `liquidity_pool_trade` effect records
the reserves it left behind. Pool reserves change only through a transaction that
touches the pool, and no transaction touched this one between then and ledger
61340264. So the reserves above are the reserves at ledger 61340263.

Two of those four links are direct observation from Horizon, one is arithmetic, and
one is an inference from how Stellar pools work. The inference is the third link and
it is the only one worth arguing about.

---

## 2. What it changes in the fixture

Recomputed with the methodology's own formulas, section 6 for depth and section 7
for manipulation cost, using `n_amm = Y x (sqrt(P_target / P_pool) - 1)` grossed up
by the fee.

**Depth, buy side.** The fixture records zero on all three rungs.

| δ | buy target | fixture | with the pool |
|---|---|---|---|
| 0.02 | 54.975084 | 0 | **101.8814** USDC |
| 0.05 | 56.591998 | 0 | **103.6081** USDC |
| 0.10 | 59.286856 | 0 | **106.4319** USDC |

**Manipulation cost.** The fixture records 0, then 130.0627093 three times.

| δ | target | fixture | orderbook | AMM | with the pool |
|---|---|---|---|---|---|
| 0.5 | 80.845712 | 0 | 0 | 127.0348 | **127.0348** |
| 1 | 107.794283 | 130.0627093 | 130.0627 | 149.2224 | **279.2851** |
| 10 | 592.868555 | 130.0627093 | 130.0627 | 372.0029 | **502.0656** |
| 100 | 5443.611281 | 130.0627093 | 130.0627 | 1160.4954 | **1290.5581** |

**What does not change.** `P0` stays 53.8971414, because the fallback ladder in
methodology section 3 stops at case 1 as soon as the book has two sides, so the pool
has no influence on the reference price at all. `spreadPct` stays 196.08.
`ZERO_DEPTH_2PCT` still fires, because the pool quotes 1.0555 and the sell side
target is 52.82, so the pool absorbs no selling anywhere in that band and the sell
side is still zero. The band is still `CRITICAL`.

So the conclusion of the fixture survives. Two of its headline numbers do not.

---

## 3. The claim this breaks

Section 10.4 of the pre-split core file, whose content is now in `10-validation.md`,
stated:

> `MC(100 × P0) for USTRY/USDC on 21 February 2026 = 0 USDC`
>
> Not thin. Zero.

Under Keel's own combining rule that is false. With the pool included it is
**1290.5581 USDC**, and at the critical delta it is 127.0348 USDC.

This matters more than a wrong number in a table. "Zero" is the sentence the
methodology is built around, and it is the sentence a reviewer will test first.

---

## 4. The part that is genuinely interesting

The attacker paid **5.3475699 USDC**, and the trade record says
`"trade_type": "orderbook"`.

On Stellar, an orderbook match reaches offers only. Pool liquidity is reachable
through path payments, not through an orderbook match. So the attacker never touched
the pool, never paid its liquidity providers, and lifted the orderbook's best ask to
106.74 while the pool went on quoting 1.0555.

Going through the combined price would have cost 127.03 USDC at the critical delta.
Going through the orderbook alone cost 5.35. **Using one venue instead of the
combined book was 23.8 times cheaper.**

That is a problem for non-negotiable rule 4, which says SDEX and AMM depth are
combined through a shared marginal price and never summed separately. The rule is
right for the question "if this collateral is liquidated, can the market absorb it",
because a liquidator will take whatever liquidity exists wherever it is. It appears
to be wrong for the question "can the oracle be fooled", because an attacker only
has to move the venue the oracle reads, and the combined price then overstates what
they must pay.

Keel already separates those two questions by side of the book, buy side for
manipulation and sell side for liquidation. This finding says they may also need
separating by **venue**, not only by side.

There is a second, quieter finding in the same place. The pool was quoting the
honest price of 1.0555 throughout, and `P0` still came out 53.8971414, because the
fallback ladder consults a pool only when the book has fewer than two sides. A
venue quoting the true price sat there and the reference price ignored it.

---

## 5. What is not being done here, and why

The fixture is not being edited, and neither is the methodology.

The golden fixture's whole value is that its numbers were computed by hand before
any implementation existed. Rewriting its inputs would invalidate every expected
value in it, and having Claude recompute them would destroy exactly the safeguard
the fixture exists to provide: `internal/conformance/fixture.go` says in its own
header, "Do not adjust these numbers to match the code. Adjust the code to match
these numbers." The same logic protects the inputs.

`scripts/audit-verification.sh` now carries a check for this finding, so it cannot be
quietly forgotten.

---

## 6. The options, with a recommendation

**A. Correct the fixture, recompute by hand, soften the claim.**
Add the pool to `GoldenSnapshot()`, recompute the depth and manipulation tables by
hand, and rewrite section 10.4 to say what is actually true: the cost through the
orderbook was zero, the cost through the combined price was 127.03 USDC at the
critical delta, and the attacker paid 5.35 USDC by using the orderbook alone.
Raises `MethodologyVersion`.

**B. Keep the fixture as it is and rename what it claims to be.**
Relabel it an orderbook-only fixture, state explicitly that pools are excluded, and
add a second fixture that includes the pool. Cheaper, and it keeps the hand
computed numbers valid. But it leaves the methodology's "zero" sentence standing
next to a fixture that quietly assumes away a venue.

**C. Split the model by venue, not only by side.**
Report manipulation cost per venue as well as combined: the cost to move the
orderbook alone, the cost to move the pool alone, and the cost to move the combined
price. The oracle-relevant number is then whichever venue the oracle reads, and
Keel stops having to guess.

Recommendation: **A and C together.** A is not optional, because a false headline
number is the one thing that cannot survive review. C is what turns this from an
embarrassment into the most interesting result in the deliverable: Keel would be
reporting that the cheapest attack path is the one that ignores half the liquidity,
and that combining venues can understate oracle risk. No monitoring tool in this
ecosystem says that today.

B is the fallback if week 3 runs out of time, and it must not be chosen silently.

---

## 7. The inference was checked from both directions

This finding rested on one inference: that no transaction touched the pool between
ledger 61172481 and ledger 61340264, so the reserves did not move. It was checked
rather than assumed, by walking Horizon's transactions-for-pool endpoint both ways.

| Direction | Cursor | Nearest entry |
|---|---|---|
| Backwards from the incident ledger | `61340264 << 32` | ledger **61172481**, 2026-02-10T16:59:35Z |
| Forwards from just after that one | `61172482 << 32` | ledger **61351813**, 2026-02-22T18:48:02Z |

The gap between 61172481 and 61351813 contains ledger 61340263, and nothing touched
the pool inside it. The reserves at the incident ledger are therefore the ones that
transaction `fa012177...` left behind, and they were not zero.

What would still overturn this: a class of reserve-changing event that does not
produce a transaction associated with the pool. None is known, and Stellar has no
mechanism for changing a pool's reserves without an operation against it, but that is
an argument from the protocol rather than from this data.

---

## 8. Decided 25 August 2026: A and C, and most of both was already done

Al chose **A and C together**, which is what section 6 recommends. This section records
the decision and one thing that had to be checked before executing it: how much of A
and C already existed.

**Most of both landed on 23 August, and this document did not notice.** DEC-006 was
found on the 21st. Methodology 1.0.3, contract 1.3.0 and migration 0003 all answered
it on the 23rd, and none of them amended this file, so it went on saying OPEN and went
on saying the fixture is missing a pool for two days after it stopped being true.

| Piece | Where it is | Landed |
|---|---|---|
| The pool in the fixture INPUT | `GoldenSnapshot()` carries `PoolUSTRYUSDC`, reserves 16.3389179 USDC | `d0d461e`, 23 Aug |
| The two-venue manipulation split | `05-manipulation-cost.md` section 3, written as a 1.0.3 addition and naming this finding as its cause | 1.0.3 |
| Its storage | `manipulation_cost_combined` and `manipulation_cost_orderbook_only` | migration `0003`, 23 Aug |
| Its contract fields | `manipulationCostCombined`, `manipulationCostOrderbookOnly` | contract 1.3.0, `7011e1c`, 23 Aug |
| Its type | `AssetRisk.ManipulationCostCombined` and `.ManipulationCostOrderbookOnly` | types.go |
| Its signature | `ComputeManipulationCost(s, p0, deltas, includeAMM)` | compute.go |
| Per-venue DEPTH | `DepthPoint.FromSdex` and `.FromAmm` | types.go |
| The "zero" claim, corrected | `05-manipulation-cost.md` section 3 separates the venues explicitly | 1.0.3 |

The expected values were NOT adjusted to fit that new input, which is the part worth
recording as correct rather than as an omission. `expected.go` was hand computed for a
market with no pool, so the assertions using it were pointed at `BookOnlySnapshot()`
instead, and the pool case is covered by invariants that need no hand computation. The
hand numbers were kept and the tests were moved. That is the right way round.

### What is left of A

1. **`testdata/fixtures/ustry_pre_exploit.md` still records `Pools: []`.** The hand
   document and the Go fixture now disagree about the input, and the hand document is
   the one that is wrong. Al's, and the fixture is locked as of 25 August.
2. **The with-pool depth and manipulation tables, computed by hand.** Section 2 of this
   document already carries those figures, and they are an ILLUSTRATION of the finding,
   not fixture inputs. Section 5 of this same document is why: numbers Claude produced
   cannot be the numbers the conformance test proves the code against. Recompute them,
   then compare against section 2. Agreement is a free confirmation and disagreement is
   a second finding; copying is neither.
3. **Section 2 of this record was computed under the pre-1.0.3 `P0` ladder, so the
   reference price its tables are built on may not be the one the current ladder
   chooses.** Recorded 27 August 2026.

   The subsection "What does not change" in section 2 says `P0` stays 53.8971414
   "because the fallback ladder in methodology section 3 stops at case 1 as soon as the
   book has two sides". That sentence describes the rule as it stood before 1.0.3.
   `03-reference-price.md` section 1 records that case 1 itself changed: with a pool
   present and a two-sided book it now compares the book mid against the pool spot, and
   when the divergence exceeds `Thresholds.PriceDivergencePct` it takes
   `P0 = pool_spot`, sets `priceSource` to `pool` and raises `PRICE_SOURCE_CONFLICT`. A
   two-sided book no longer ends the ladder on its own, and that file names this
   fixture as the reason for the change.

   What it reaches is not one cell:

   - the **target price column of both tables** in section 2, because every buy target
     is `P0 x (1 + delta)` and every sell target is `P0 x (1 - delta)`;
   - **every figure in the depth table**, which is computed against those targets;
   - **`spreadPct`**, which `03-reference-price.md` section 2 defines as
     `(best_ask - best_bid) / P0 x 100`, so it moves with `P0` while neither side of
     the book moves;
   - **the `ZERO_DEPTH_2PCT` reasoning** in the same subsection, which argues from a
     sell side target of 52.82 sitting far above the pool quote of 1.0555. If the
     ladder takes the pool branch, the sell side target is derived from the pool quote
     instead and that argument has to be made again rather than carried across;
   - **the second finding in section 4**, "a venue quoting the true price sat there and
     the reference price ignored it". That is a statement about the superseded ladder.
     1.0.3 is the answer to it, so the sentence now records history rather than the
     current rule.

   **Nothing is recomputed here and no replacement value is offered, deliberately.**
   Which branch of case 1 this fixture takes is methodology applied to the fixture, and
   both halves of that are Al's: the methodology tree is RED and so is the golden
   fixture. The material needed to settle it is already written down, the rule in
   `03-reference-price.md` section 1 and the threshold default and this fixture's own
   divergence in `09-flags-and-bands.md`. Reading those two together looks like it
   settles the branch. It is still Al's to settle and to record, because what follows
   is not one number but a chain of replacement values through both tables of section 2,
   and every one of them lands in a red zone.

### Two conflicts found afterwards, in neither A nor C

Found 27 August 2026 while checking section 2 against the 1.0.3 documents. Neither
belongs to option A or to option C. They are numbered on from the list above so that a
reference to item 4 is unambiguous inside this section.

4. **The methodology and the contract disagree about when `maxReachablePrice` is null,
   and this fixture is exactly the case where they disagree.**

   `05-manipulation-cost.md` section 5 keys the null on the **presence** of a pool:
   "When an active pool is present, both are null", and "Both fields are meaningful
   only for pure order book markets". `docs/api/keel-openapi.yaml` keys it on
   **exclusivity**: `maxReachablePrice` is "Null in two situations: there is no ask at
   all on this pair, or all the liquidity comes from an AMM", and
   `costToMaxReachablePrice` is null "in the same two situations".

   On a market holding a book AND a pool those two rules give opposite answers, and the
   market this record is about is that market: section 1 above establishes the pool, and
   the book holds one ask at 106.7372828. The contract does not merely imply the
   non-null answer there, it states it, because the `costToMaxReachablePrice`
   description says that on the USTRY fixture "the highest reachable price was
   106.7372828 and reaching it cost zero". Its data agrees with its prose: the
   `AssetHealthy` example, whose summary is "A liquid asset with both an orderbook and a
   pool", carries `poolSpotPrice: '2.8390000'` alongside
   `maxReachablePrice: '41.8842000'`.

   **Choosing between the two rules is a methodology decision and it is Al's.** Neither
   `05-manipulation-cost.md` nor the contract was touched. What is worth knowing before
   the choice: if the methodology wins, the contract has to change, two field
   descriptions and the two `AssetHealthy` values, and by the zone map a contract change
   means a version bump and `make api-mocks`. If the contract wins, section 5 of
   `05-manipulation-cost.md` changes and nothing generated moves. P2-16 in
   `scripts/audit-verification.sh` reads the rule out of each of the two files and fails
   while they differ, so it closes whichever side moves.

5. **The contract example and the methodology state oracle window lengths that differ by
   a factor of three, and the mocks are generated from the example.**

   The `/methodology` response example carries `oracleWindowSeconds: 300`. The
   methodology assumes fifteen minutes, which is 900 seconds: `06-oracle-resilience.md`
   section 1 states that "The 15-minute default follows Script3's statement that no
   other trade occurred within 15 minutes before the manipulation", and
   `07-supporting-metrics.md` section 4 repeats "The 15 minute default window". The same
   300 is repeated in three `oracleResistance.windowSeconds` example values and in two
   warning strings reading "the 300 second oracle window".

   `docs/api/mocks/methodology.json` is generated from that example, so 300 is the number
   a frontend builds against today while 900 is the number the deliverable assumes.

   **The number was not changed, and not for lack of an obvious candidate.** The correct
   value depends on R-6, the open question about Reflector's actual VWAP window, which
   `06-oracle-resilience.md` records as handoff item 6 and which is open against a third
   party rather than against this repository. Writing 900 into the contract would swap a
   wrong number for an assumed one and would read as a confirmed oracle parameter, which
   is precisely what `06-oracle-resilience.md` is careful not to claim. P2-17 in
   `scripts/audit-verification.sh` converts the methodology's minutes into seconds, reads
   the example's number, and fails while the two differ in either direction, so whichever
   way R-6 lands the check is what closes.

### What is left of C, and one question inside it

Option C names three venues: the orderbook alone, the pool alone, and combined. Two of
the three exist. The third does not, and before it is built there is a question.

**Is the pool-only figure a stored quantity or an arithmetic identity?** On this
document's own table in section 2, combined is exactly orderbook plus AMM at every
rung: 130.0627 + 149.2224 = 279.2851, and 130.0627 + 372.0029 = 502.0656. If that
holds generally, then a `manipulationCostPoolOnly` ladder is `combined` minus
`orderbookOnly` point by point, and storing it is a second home for a fact already
stored twice. Handoff item 17 is that same mistake in a smaller shape.

Note that this does NOT contradict non-negotiable rule 4. Rule 4 governs DEPTH, where
the venues compete at a shared marginal price and do not add, which is why the
discriminating test exists and why `FromAmm` must be exactly zero for a pool priced
above the target. Manipulation COST to reach a target price is a different quantity:
both venues have to be moved to the same target for the market price to arrive there,
so the two amounts are independent and they do add.

Whether that identity is a definition or a coincidence of this one fixture is a
methodology question and it is Al's. If it is a definition, C is finished by saying so
in `05-manipulation-cost.md` and in the contract, and no column, field or migration is
needed. If it is not, the third ladder needs its own formula in `compute.go`.

---

## 9. Amendment history

This record is append-only. An amendment adds a row here and text below the section it
concerns; no earlier sentence is edited or deleted. That is the treatment DEC-003 gives
its own section 4, and the reason is the same: a document that quietly grows correct
hides the fact that it was wrong, and the fact that it was wrong is the finding.

| Date | Amendment |
|---|---|
| 25 August 2026 | Section 8 added. A and C decided by Al, and most of both turned out to already exist |
| 27 August 2026 | Section 8 item 3: section 2 was computed under the pre-1.0.3 `P0` ladder, so its target prices, its depth figures, `spreadPct` and the `ZERO_DEPTH_2PCT` reasoning all rest on a reference price the current ladder may not choose. Section 8 items 4 and 5: the `maxReachablePrice` null-condition conflict between `05-manipulation-cost.md` and the contract, and the oracle window conflict between the contract example and the methodology. No number in this record was changed, no branch was chosen, and no replacement value was computed |
