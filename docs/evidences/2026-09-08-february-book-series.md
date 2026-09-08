# The February 2026 book series, and why it cannot fill the report yet

**Run on:** 2026-09-08, from public Horizon mainnet with no account.
**Command:** the one in `docs/report/blend-february-2026.md` section 9, unedited.
**Raw output:** `USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01.csv`,
30 rows, 45 columns, and its provenance sidecar `.meta.txt` beside it.
**MethodologyVersion on every row:** `1.0.8-draft`, which is the code constant.
`docs/methodology/README.md` section 4 records that the constant deliberately stays
there while the document set is at `1.1.0-draft`, so this is not a drift.

**What this run was for.** Deliverable 2 asks when the unsafe threshold was crossed
relative to the exploit date, and `docs/report/blend-february-2026.md` sections 5 and 6
have been empty since 5 September waiting for the series that answers it.

**THE SERIES LANDED AND SECTION 5 STAYS EMPTY.** The reason is section 4 of this
document and it is not a shortage of data. Six of the thirty rows describe a book that
could not have existed, and the transition this report was going to be about sits inside
those six.

---

## 1. What the walk cost, and it was not the rate limit

| Property | Value |
|---|---|
| Targets | 30, ledger `61027032` to `61429800` |
| Accounts walked | 222, of which 216 came from the trade stream and 6 from live offers |
| Operations read | 763,674 |
| Offer operations found | 12,249 |
| Trades read | 13,831 |
| Requests | 1,902 |
| Elapsed | 6,998 seconds, 1 hour 57 minutes |
| Request rate | about 978 an hour, against the 3,000 NFR-6 permits |
| `walks_stopped_at_floor` | 164 |
| `walks_truncated` | **38** |
| `walks_failed` | **6** |
| `walk_complete` | **false** |

**The bottleneck is depth, not budget, and the 5 September measurement said so in
advance.** This run used a quarter of the permitted request rate and still took two
hours, because what costs time is how far back each account has to be paged.

**Only 9 of the 222 accounts produced a single offer operation.** The other 213 were
walked because they touched the pair, and they contributed nothing to any book.

| Account | Pages | Reached back to | Offer ops | Status |
|---|---|---|---|---|
| `GBPFB6XN` | 60 | `61046747` | 11,999 | **TRUNCATED** |
| `GABFRFPY` | 60 | `61363138` | 205 | **TRUNCATED** |
| `GDJSH2NU` | 5 | `60983257` | 19 | floor |
| `GBO7VUL2` | 2 | `61231411` | 12 | ended |
| `GDHRCQNC` | 2 | `61231534` | 7 | ended |
| `GCNF5GNR` | 2 | `61339909` | 3 | ended |
| `GDSANA2X` | 6 | `60986261` | 2 | floor |
| `GCXJ4V7U` | 19 | `60986974` | 1 | floor |
| `GDDAEATA` | 4 | `60985317` | 1 | floor |

**Both accounts that hold this pair's book are truncated, and the 60 page cap chosen on
5 September was chosen to prevent exactly that.** `GBPFB6XN` stopped 19,715 ledgers
above the earliest target. `GABFRFPY` stopped at `61363138`, which is **after** the
incident ledger `61340263`, so for every row up to and including the exploit its offers
are carried by nothing.

**Raising the cap globally is not the fix.** The other 36 truncated walks each found zero
offer operations, so a cap of 400 would buy them several hundred empty pages apiece.
Horizon offers no way to filter an account's operations by type, so depth cannot be
aimed. Section 6 proposes what would work.

---

## 2. What this run PROVES, and it is the strongest reading of the historical path yet

**The reconstruction reproduces the golden fixture's ask side exactly, at both control
ledgers, to the seventh decimal.**

| Quantity | Reconstruction | `testdata/fixtures/ustry_pre_exploit.md` | Difference |
|---|---|---|---|
| `ask_amount_total` at `61340262` | `1.2185315` | `1.2185312` | `0.0000003` |
| `ask_amount_total` at `61340263` | `1.1684312` | `1.1684309` | `0.0000003` |
| The change across the manipulation | `0.0501003` | `0.0501003` | **`0`** |

`1.2185315 − 1.1684312 = 0.0501003`, which is the exact USTRY volume of the manipulating
trade in ledger `61340263`. The fixture states that arithmetic by hand in
`10-validation.md` section 7 and the reconstruction lands on it from the other
direction, from 763,674 operations folded forward, with no fixture value in the loop.

**And the manipulation cost decomposes exactly.** The engine reports
`mc_cost_100 = 130.06270940076228029461328` at `61340262`. Break it apart:

```
1.2185312 x 106.7372828 = 130.06270929502336      the attacker's own resting ask
0.0000001 x 1.0573892029461328 = 0.00000010573892  one dust ask
                                 ----------------
                                 130.06270940076228029461328   the reported figure
```

Two terms, both accounted for, nothing unexplained. The same decomposition holds at
`61340263` with the fixture's remainder `1.1684309`.

**That is a real result and it should be said plainly:** the historical path rebuilt the
incident book from raw operations and agreed with a hand computation made before any of
this code existed.

---

## 3. What it disproves, and it is about the FIXTURE as much as the code

The fixture states the book at `61340262` held **one** ask. The reconstruction finds
**three**. The two extra are dust, `0.0000001` and `0.0000002` USTRY, and they are not
noise in the output because of where they sit.

| Quantity | Fixture, by hand | This run | Consequence |
|---|---|---|---|
| asks | 1 | 3 | |
| `best_ask` | `106.7372828` | **`1.0573892029461328`** | the reference price changes |
| `spread_pct` | very large | **`0.0368`** | the spread all but vanishes |
| depth at ±2%, both sides | zero, and the fixture argues at length that this zero is CORRECT | `0.0000001` buy, `0.000109` sell | a zero becomes a non-zero |
| `max_reachable_price` | `106.7372828` | **`2147483647`** | which is `2^31 − 1` |
| flags | `ZERO_DEPTH_2PCT` `MANIPULATION_CHEAP` | `MANIPULATION_CHEAP` `THIN_DEPTH_5PCT` | **the flag sets differ** |
| band | `CRITICAL` | `CRITICAL` | the verdict survives |

**`10-validation.md` section 4 compares flag sets EXACTLY.** So under this repository's
own protocol this is a mismatch and not an agreement, whatever the band says.

**Which side is wrong is not decidable from here, and this document will not pick.**
Two readings, and both have force:

1. **The dust offers are real and old, and the fixture missed them.** They were created
   below the shallow floor the 5 September validation used, so a walk that starts closer
   to the target cannot see them and a hand computation from a Horizon snapshot at the
   time would not have listed them either. If this is right, the fixture is incomplete
   and its zero-depth argument is wrong.
2. **The dust offers are ghosts.** The `0.0000001` ask at `1.0573892029461328` appears
   unchanged in every row from 9 February to 28 February. An offer of one ten-millionth
   of a unit resting for twenty days while bids climb past it is what an offer looks like
   when the operation that consumed it was never seen. `missing_offer_ids` is **203** on
   every row, so 203 offer ids were referenced and never resolved, and one of them
   plausibly killed this offer.

**Section 4 settles it for the second half of the month and leaves the first half open.**

**A third finding, and it is Al's because it is a definition.** The fixture's key
observation in `10-validation.md` section 7 is that the order book manipulation cost was
**zero**, because the only ask between 1.057 and 106.74 belonged to the attacker, and the
methodology defines the cost as the notional paid to **other parties**. The engine has no
way to know who owns an offer, so it charged 130.06 USDC for buying the attacker's own
ask. `05-manipulation-cost.md` does not mention self-owned offers anywhere.
So the document and the incident's own headline reading disagree about the central number
of Deliverable 2, and `docs/methodology/` is RED.

---

## 4. Why section 5 of the report stays empty: six rows describe an impossible book

From 23 February onward the reconstructed book is **CROSSED**: the best bid is above the
best ask.

| Day | Ledger | `best_bid` | `best_ask` | Crossed |
|---|---|---|---|---|
| 2026-02-22 | `61340172` | `1.057` | `1.0573892029461328` | no |
| 2026-02-23 | `61355036` | `1.0575149573678337` | `1.0573892029461328` | **yes** |
| 2026-02-24 | `61369919` | `1.0575149573678337` | `1.0573892029461328` | **yes** |
| 2026-02-25 | `61385497` | `1.0577188122495003` | `1.0573892029461328` | **yes** |
| 2026-02-26 | `61400035` | `1.05781077085` | `1.0573892029461328` | **yes** |
| 2026-02-27 | `61415216` | `1.05787474205` | `1.0573892029461328` | **yes** |
| 2026-02-28 | `61429800` | `1.0579667006495002` | `1.0573892029461328` | **yes** |

**A crossed book cannot exist on a Stellar ledger.** The matching engine fills from the
best price, so the moment a bid crosses an ask the two execute. Every one of those six
rows therefore describes a state no ledger ever held, and the ask doing the crossing is
the same `0.0000001` offer at `1.0573892029461328` in every row. It is a ghost, and after
22 February it is a provable one.

**And that is exactly where the report's answer was going to come from.** The band series
reads `CRITICAL` on every row from 1 February through 24 February and `LOW` on 25, 26, 27
and 28 February. The naive sentence is available and it is tempting: the asset was
already critical three weeks before the exploit and only cleared afterwards.

**It cannot be written.** The flip to `LOW` happens on 25 February, inside the crossed
region, and it is driven by depth figures that jump from `0.0000011` to `225,347` in one
day while `best_ask` never moves off the ghost. A depth of two hundred thousand units in
a book whose best ask is one ten-millionth of a unit is not a market that healed. It is
an artefact with a date on it.

So section 5 keeps its placeholder, and the placeholder now says the series landed and
this is why, rather than saying it is waiting.

**Two further rows to distrust for a different reason.** On 3, 4, 6 and 7 February the
only ask is a dust offer priced at `2147483647`, which is `2^31 − 1`, and `p0` comes back
as about `1.0737e9`. The engine flags `SPREAD_EXTREME` and `ZERO_DEPTH_2PCT`, so it is
not claiming that is a real price. But a mid price of a billion, derived from an ask of
`0.0000002` units at the maximum int32, is not a reference price, and
`03-reference-price.md` does not say what to do about it. The same pattern is in the Layer
1 input for AUDD, which carries an ask of `0.0000004` at price `2147483647`, so this is a
class of input rather than one bad day.

---

## 5. What is safe to say from this run

1. **The historical path agrees with the golden fixture on the ask amounts at the
   incident, exactly, and reproduces the manipulation volume `0.0501003` independently.**
   Section 2.
2. **A book rebuilt from operations is a lower bound until every referenced offer id
   resolves**, and 203 did not resolve here. Every row says so in its own
   `missing_offer_ids` and `fold_complete` columns, which is why those columns exist.
3. **The cost of a complete month is measured now rather than estimated:** 1,902
   requests, two hours, and it still did not complete.

**What is NOT safe to say, and would have been the headline:** when the unsafe threshold
was crossed. That needs a series whose rows are all possible books.

---

## 6. What would fix it, priced

**The proposal is two caps instead of one, and it is a change to `internal/horizon`
(YELLOW) plus a flag in `cmd/keel` (GREEN).** Today `-max-pages-per-account` applies to
every account equally, so the cap has to be small enough for 213 accounts that hold no
offers and is therefore too small for the two that hold the book.

Split it. A shallow cap for an account that has produced no offer operation yet, and a
much deeper one for an account that has produced at least one. Depth then gets spent
where offers are. Every truncation stays reported per walk, and `walk_complete` still
governs.

Estimated cost from this run's own measurements: `GABFRFPY` yields about 1,194 ledgers a
page, so reaching the floor needs roughly 315 more pages, about 1.75 hours for that one
account. `GBPFB6XN` yields about 6,468 ledgers a page and needs roughly 10 more. Total
for a complete month, in the region of 2,200 requests and three hours, still under
NFR-6.

**That is a proposal and not a decision.** It changes how the reconstruction behaves,
which is the thing Layer 3 tests, so it wants its own test and its own note in the zone
map's terms. Whether to spend three hours on it, ten days before the deadline, is Al's
call. The alternative is to publish section 5 with the first 22 rows only, stop at
22 February, and say in the report that the days after the exploit are not reconstructible
at this walk depth.

**And one question that belongs to Al either way:** whether an attacker's own resting
offer counts toward manipulation cost. Section 3, third finding. Nothing in
`internal/domain` can answer it, and the answer changes the central number of
Deliverable 2.

---

## 7. Reproducing this

```bash
go run ./cmd/keel bookseries \
  -pairs scripts/record-pairs.example.json \
  -from-trades docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv \
  -also-ledger 61340262,61340263 \
  -trades-from-ledger 60987032 -since-ledger 60987032 -lookahead 5000 \
  -max-pages-per-account 60 \
  -csv <path>
```

Two hours, no account, no key. It will not reproduce these numbers exactly, because the
walk discovers accounts from a trade stream that keeps growing and because a truncated
walk truncates at a different place as history lengthens. What is reproducible is the
shape, and the sidecar records every input that shaped it.

**Launch it detached.** Two runs on 5 September died silently when the polling loops
waiting on them timed out and took the job with them.

---

## 8. Version history

| Date | Change |
|---|---|
| 8 September 2026 | Created. The first complete month of book state, and the reason it does not yet answer the question it was run for. Records that the reconstruction matches the golden fixture's ask amounts exactly at both control ledgers, that it finds three asks where the fixture states one, and that the book is provably crossed from 23 February onward |
