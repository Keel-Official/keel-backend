# Could Keel have warned about the Blend incident of February 2026?

**Status: DRAFT. Every section now holds its measurement, and one sentence is still
missing.** Sections 5 and 6 are the ones that answer the title and both were filled on
14 September 2026 from the run named in section 9. What is still open is section 6.5, the
statement of what those numbers MEAN, which is not written by the hand that produced them.
Section 10 lists that and the four other open items, with an owner against each.

**Version:** draft, 5 September 2026, sections 5 and 6 filled 14 September 2026
**Methodology version:** `1.0.8-draft`, the version the engine stamps on every
result quoted here. The methodology documents are at `1.1.0-draft`, except
`01-data-sources.md` and `11-limitations.md` which moved to `1.2.0-draft` on
14 September 2026 under DEC-021, and they describe a multi-pair rule the engine does not
implement yet; DEC-014 and DEC-015 are where that gap is recorded. Nothing in this report depends on it: USTRY is measured
against USDC and USDC is the unit the thresholds are in.
**Asset:** `USTRY`, issuer `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`,
against `USDC`, issuer `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`.
The identity is fixed by `docs/decisions/DEC-001-ustry-identity.md` and an asset is
the pair (code, issuer), never the ticker.

---

## 1. What this report claims, and what it does not

**The answer, in one sentence:** measured once a day, USTRY/USDC read `LOW` on every sample of February 2026; measured at the exploit's own ledger, the same methodology reads `CRITICAL`, because the book's only maker withdrew its ladder for 79 seconds at the same minute every day and the exploit landed inside that window. Section 6 holds the evidence and 6.5 says what it means.

**It claims** that the state of the USTRY/USDC order book in February 2026 can be
rebuilt from public Stellar data, that Keel's published methodology applied to that
state produces a risk band on each day of the month, and that the resulting series
either does or does not cross into `CRITICAL` before the exploit date. Which of
those two it is, is section 6.

**It does not claim** that anybody would have acted on the warning, that Keel
existed at the time, or that the thresholds it uses are calibrated. They are chosen,
and `docs/methodology/11-limitations.md` says so in those words.

**It especially does not claim to be free of hindsight.** Section 7 is about that
and it is not a formality: this report knows the date of the attack, and a backtest
that knows the outcome can find a signal in almost anything. What makes the claim
checkable rather than rhetorical is that every threshold used here was written down
before the series was computed, and section 9 says exactly how a reader can confirm
that from the repository's own history.

**If the series shows no clear signal before the exploit date, that is the finding
and it is reported as one.** The PRD says so first, at section 10: "If the backtest
does not show a clear signal, that is not a project failure but a finding that has
to be reported honestly. Reporting it as it is does far more for long term
credibility than tuning thresholds until the result looks good."

## 2. The incident, from the chain

| | |
|---|---|
| Date | 22 February 2026 |
| Ledger the manipulation executed in | **61340263**, closed 2026-02-22T00:10:21Z |
| What happened | the USTRY price was pushed up roughly 100 times through a thinly traded feed and the position was then used as collateral to borrow about $61 million in XLM |
| Where that is stated | `docs/context/Keel_PRD.md` section 1. The date correction from May to February is `docs/decisions/DEC-001-ustry-identity.md` section 1 |

The order book immediately before that trade, at the end of ledger **61340262**, is
in `testdata/fixtures/ustry_pre_exploit.md`. Every figure in it was computed by hand
in a spreadsheet before any implementation existed, and that file is in a directory
the engine's authors cannot write to. The two levels were the whole book:

```
Asks: [ { price_r: {266843207, 2500000}, amount: 1.2185312 } ]   price 106.7372828
Bids: [ { price_r: {1057, 1000},         amount: 0.0001000 } ]   price   1.0570000
Pools: []
```

**One ask and one bid, and 105 dollars of nothing between them.**

## 3. What Keel says about that state

Applying the methodology to the book above, at ledger 61340262:

| Quantity | Value | Where the rule is written |
|---|---|---|
| reference price `P0` | 53.8971414 | `03-reference-price.md` |
| `spreadPct` | 196.0777141 per cent | `03-reference-price.md` |
| depth at ±2, ±5, ±10 per cent, both sides | **0** on all six | `04-depth.md` |
| manipulation cost to move the price 50 per cent | **0**, and the target is **reachable** | `05-manipulation-cost.md` |
| `maxReachablePrice` | 106.7372828 | `05-manipulation-cost.md` section 5 |
| cost to reach it | **0** | |
| ratio of that price to the real price of 1.057 | **100.98** | |
| band | **CRITICAL** | `09-flags-and-bands.md` |
| flags | `ZERO_DEPTH_2PCT`, `THIN_DEPTH_5PCT`, `SPREAD_EXTREME`, `MANIPULATION_CHEAP` | `09-flags-and-bands.md` |

**The line that matters most is the cost of zero.** An attacker does not pay for the
trade that moves the price. They pay for the third-party liquidity they have to
consume on the way to it, and on this book there was none to consume: no ask was
cheaper than the target, so nothing had to be bought, and the one ask sitting at
106.74 was enough to make the target reachable. `05-manipulation-cost.md` section 1
is the definition and this report does not restate it in its own words.

`bandConfidence` is **partial**, not full. Six flags need supply data, trade history
or trustline distribution that a book snapshot cannot carry, and they are reported as
`unevaluated` rather than as clear. A metric that could not be assessed is never
counted as a passing metric; `09-flags-and-bands.md` section 2 is the rule.

### 3.1 The live API answers a different price for this ledger, and both are right

Since 17 September 2026 Keel serves this ledger directly, at
`GET /v1/asset/USTRY:GCRYUGD5.../depth?ledger=61340262`. **It answers a reference
price of 1.0555441846982006, not the 53.8971414 in the table above.** A reader
holding this report beside that response is entitled to an explanation, and it is not
that one of them is a mistake.

**The two rows are the same methodology over different inputs, and the difference is
one pool.** The table above is computed from the ORDER BOOK alone, because the golden
fixture it comes from records `Pools: []`. The API row carries the USTRY/USDC constant
product pool that genuinely held reserves at that ledger, 16.3389179 USDC against
15.4791416 USTRY at 30 bps, verified in `docs/decisions/DEC-013-USTRY-USDC-pool-ledger-61340263.md`
section 1 and supplied to the run from
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-pool-evidence-2026-02-22.json`.

With a pool present and a two-sided book, `03-reference-price.md` section 1 compares
the book mid against the pool spot and takes the pool when they diverge past
`Thresholds.PriceDivergencePct`. Here they diverge by a factor of fifty, so the ladder
takes the pool branch. That rule is methodology 1.0.3, and
`docs/decisions/DEC-006-amm-pool-in-the-fixture.md` section 8 item 3 named this fixture
as the reason it was written, listed every figure the branch would move, and left them
uncomputed. They are computed now.

| Quantity at ledger 61340262 | This report, book only | The API, with the pool |
|---|---|---|
| `P0` | 53.8971414 | 1.0555441846982006 |
| `priceSource` | `book` | `pool` |
| `spreadPct` | 196.0777141 | 10011.9241176 |
| depth at ±2 per cent | 0 on both sides | 0.1630695 buy, 0.1638274 sell |
| manipulation cost to 50 per cent | 0 | 3.6831374 combined, 0 through the book alone |
| `maxReachablePrice` | 106.7372828 | `null`, because an active pool has no highest price |
| flags | `ZERO_DEPTH_2PCT` among four | `PRICE_SOURCE_CONFLICT` in its place |
| **band** | **`CRITICAL`** | **`CRITICAL`** |

**Three things do not move, and they are the three this report is about.** The book is
the same book: best bid 1.057 for 0.0001, best ask 106.7372828 for 1.2185312. The band
is `CRITICAL` either way. And the manipulation cost through the order book alone is
still zero, because no ask sat below any target, which is the sentence section 3 calls
the line that matters most.

**Reproduce the difference in one command each.** The API row:
`curl "https://api.keels.app/v1/asset/USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC/depth?ledger=61340262"`.
The run that produced it, with its full diagnostics, is
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-replay-61340262-with-pool-2026-09-17.log`
and its sidecar. `docs/decisions/DEC-023-store-the-control-ledger-with-its-disagreement.md`
section 4.2 records what was stored and when.

> **[FOR AL. This paragraph states what the difference MEANS and is the one part of
> this section that is not Claude's under the zone map. Drafted for ratification,
> amendment or deletion.]**
>
> The figure this report quotes for `P0` is the one worked by hand before any of this
> code existed, and it describes the order book that the oracle read. The figure the
> API returns describes the whole market at that ledger, order book and pool together,
> which is what Keel exists to measure. Neither is wrong; the report's is narrower on
> purpose, because the incident is a story about what the order book alone could
> support. **A reader who wants the price the oracle acted on should take
> 53.8971414. A reader who wants the price the market as a whole would have supported
> should take 1.0555441847, and should notice that the honest venue was quoting it the
> whole time.**

## 4. Where the historical data comes from, and why it is not a measurement

Horizon serves no order book at a past ledger. It serves every operation and the
result of every operation for ever, and a book is what those operations left behind.
Keel rebuilds the book by replaying `manage_sell_offer` and `manage_buy_offer`
operations up to a target ledger and applying the trades that consumed them.

**Every figure in section 5 therefore carries `dataSource: offers-implied` and is a
reconstruction rather than a reading.** It is a stronger source than the trade stream
would be, because an offer proves liquidity that was *posted* while a trade proves
only liquidity that was *consumed*, but it is not the same thing as a snapshot and
this report does not present it as one.

**The method was checked against the hand-computed fixture before it was trusted.**
On 5 September 2026 the book at control ledger 61340262 was rebuilt this way and the
methodology run over it, and every quantity in section 3 came back identical to the
figures worked by hand. The reading is
`docs/evidences/2026-09-05-control-ledger-validation.md`, and the artefact and its
provenance sidecar are beside it.

**Three limits of the method, each counted on every run rather than assumed away:**

1. An offer whose owner never traded and is not resting today is not discovered.
2. An account walk that fails or hits its page cap loses that account's offers.
3. No AMM pool is reconstructed at all, so every figure here is order book only.

The first two make the rebuilt book **thinner** than the market was, which overstates
risk rather than understating it. That is the conservative direction and it is
principle P-2 in the PRD. The third is a genuine gap and section 8 carries it.

## 5. The book, day by day, through February 2026

**Source:** `docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.csv`
and its provenance sidecar, both in this repository. The command that produced them is in
section 9. **Nothing in these tables was typed by hand**: every figure is a cell of that
CSV, rounded as each table states, and the CSV carries the unrounded value.

**The sample rule, fixed in code before the run: the first trade at or after each UTC
midnight.** Each row reports the instant actually sampled. First trade rather than nearest
to midnight, so a row labelled with a day never describes the state the market was in the
evening before. Two rows sit outside the daily grid and are shown in bold: control ledger
61340262 and the incident ledger 61340263, which closed at 00:10:15 and 00:10:21 on
22 February.

**This run is REPAIRED and says so in its own provenance.** One offer, `1822775941`, is
removed from the fold at every target from 9 February, because it was proven absent from
ledger 61143682 onward while emitting neither an operation nor a trade. The proof is
`docs/evidences/2026-09-12-crossed-book-ustry-february.md`, the mechanism and the decision
are DEC-021, and the CSV's `known_removals` column names the offer on every row where it
applied. An unrepaired run of the same command is in this repository too, without the
`-repaired` suffix, and section 5.5 measures the difference between them.

### 5.1 The book

Prices in USDC per USTRY, to six decimals. `spreadPct` is defined in
`03-reference-price.md`.

| Day | Ledger | Sampled UTC | Bids / asks | Best bid | Best ask | `P0` | Spread % |
|---|---|---|---|---|---|---|---|
| 02-01 | 61027032 | 00:03:32 | 8 / 4 | 1.055570 | 1.056520 | 1.056045 | 0.090 |
| 02-02 | 61041997 | 00:05:00 | 9 / 4 | 1.055581 | 1.056531 | 1.056056 | 0.090 |
| 02-03 | 61056831 | 00:02:44 | 10 / 4 | 1.055745 | 1.056695 | 1.056220 | 0.090 |
| 02-04 | 61071606 | 00:01:44 | 11 / 4 | 1.055851 | 1.056801 | 1.056326 | 0.090 |
| 02-05 | 61086706 | 00:12:59 | 13 / 5 | 1.055953 | 1.056872 | 1.056412 | 0.087 |
| 02-06 | 61101452 | 00:03:59 | 14 / 4 | 1.055953 | 1.057438 | 1.056695 | 0.141 |
| 02-07 | 61116508 | 00:02:56 | 13 / 4 | 1.055953 | 1.057269 | 1.056611 | 0.125 |
| 02-08 | 61132067 | 00:21:02 | 14 / 5 | 1.055963 | 1.057330 | 1.056646 | 0.129 |
| 02-09 | 61147341 | 00:33:22 | 14 / 4 | 1.056053 | 1.057479 | 1.056766 | 0.135 |
| 02-10 | 61161968 | 00:05:45 | 15 / 5 | 1.056360 | 1.057310 | 1.056835 | 0.090 |
| 02-11 | 61176868 | 00:06:47 | 15 / 4 | 1.056443 | 1.057395 | 1.056919 | 0.090 |
| 02-12 | 61191677 | 00:06:04 | 15 / 4 | 1.056521 | 1.057473 | 1.056997 | 0.090 |
| 02-13 | 61206579 | 00:19:24 | 15 / 4 | 1.056641 | 1.057593 | 1.057117 | 0.090 |
| 02-14 | 61221451 | 00:21:36 | 15 / 4 | 1.056733 | 1.057685 | 1.057209 | 0.090 |
| 02-15 | 61236743 | 01:04:33 | 15 / 4 | 1.056827 | 1.057779 | 1.057303 | 0.090 |
| 02-16 | 61250942 | 00:08:17 | 16 / 4 | 1.056827 | 1.057779 | 1.057303 | 0.090 |
| 02-17 | 61265749 | 00:15:32 | 16 / 4 | 1.057006 | 1.057958 | 1.057482 | 0.090 |
| 02-18 | 61280403 | 00:00:35 | 16 / 4 | 1.057033 | 1.057985 | 1.057509 | 0.090 |
| 02-19 | 61295329 | 00:08:18 | 16 / 5 | 1.057169 | 1.058049 | 1.057609 | 0.083 |
| 02-20 | 61310263 | 00:11:28 | 16 / 5 | 1.057260 | 1.058212 | 1.057736 | 0.090 |
| 02-21 | 61325141 | 00:06:22 | 16 / 4 | 1.057344 | 1.058296 | 1.057820 | 0.090 |
| 02-22 | 61340172 | 00:01:24 | 17 / 5 | 1.057427 | 1.058379 | 1.057903 | 0.090 |
| **61340262** | 61340262 | 00:10:15 | 14 / 2 | 1.057000 | 106.737283 | 53.897141 | 196.078 |
| **61340263** | 61340263 | 00:10:21 | 14 / 2 | 1.057000 | 106.737283 | 53.897141 | 196.078 |
| 02-23 | 61355036 | 00:00:33 | 19 / 6 | 1.057515 | 1.058423 | 1.057969 | 0.086 |
| 02-24 | 61369919 | 00:00:10 | 19 / 9 | 1.057546 | 1.058423 | 1.057984 | 0.083 |
| 02-25 | 61385497 | 00:58:31 | 19 / 10 | 1.057719 | 1.058423 | 1.058071 | 0.067 |
| 02-26 | 61400035 | 00:23:34 | 20 / 9 | 1.057811 | 1.058423 | 1.058117 | 0.058 |
| 02-27 | 61415216 | 00:45:26 | 21 / 9 | 1.057875 | 1.058423 | 1.058149 | 0.052 |
| 02-28 | 61429800 | 00:15:49 | 22 / 10 | 1.057967 | 1.058423 | 1.058195 | 0.043 |

### 5.2 What the methodology says about each book

Depth is notional in USDC, per `04-depth.md` section 1:
`depth_sdex_buy(δ) = Σ (price_i × amount_i)` over asks priced at or below `P0 × (1 + δ)`.
`Cost(δ=0.5)` and `Reachable` are `05-manipulation-cost.md`. Rounded to whole USDC.

| Day | Depth buy ±2% | Depth sell ±2% | `Cost(δ=0.5)` | Reachable | Band | Flags |
|---|---|---|---|---|---|---|
| 02-01 | 218,855 | 223,890 | 218,855 | true | LOW | (none) |
| 02-02 | 220,569 | 210,631 | 220,569 | true | LOW | (none) |
| 02-03 | 220,275 | 223,189 | 220,275 | true | LOW | (none) |
| 02-04 | 227,017 | 218,155 | 227,017 | true | LOW | (none) |
| 02-05 | 215,549 | 223,243 | 215,549 | true | LOW | (none) |
| 02-06 | 228,473 | 224,077 | 228,473 | true | LOW | (none) |
| 02-07 | 219,034 | 217,311 | 219,034 | true | LOW | (none) |
| 02-08 | 226,756 | 212,692 | 226,756 | true | LOW | (none) |
| 02-09 | 213,175 | 217,186 | 213,175 | true | LOW | (none) |
| 02-10 | 221,252 | 219,180 | 221,252 | true | LOW | (none) |
| 02-11 | 224,208 | 219,228 | 224,208 | true | LOW | (none) |
| 02-12 | 229,364 | 220,149 | 229,364 | true | LOW | (none) |
| 02-13 | 219,295 | 219,016 | 219,295 | true | LOW | (none) |
| 02-14 | 228,395 | 224,871 | 228,395 | true | LOW | (none) |
| 02-15 | 212,760 | 218,369 | 212,760 | true | LOW | (none) |
| 02-16 | 217,064 | 222,168 | 217,064 | true | LOW | (none) |
| 02-17 | 225,355 | 223,146 | 225,355 | true | LOW | (none) |
| 02-18 | 214,669 | 217,046 | 214,669 | true | LOW | (none) |
| 02-19 | 222,684 | 214,425 | 222,684 | true | LOW | (none) |
| 02-20 | 213,535 | 221,243 | 213,535 | true | LOW | (none) |
| 02-21 | 223,707 | 213,701 | 223,707 | true | LOW | (none) |
| 02-22 | 227,480 | 223,262 | 227,480 | true | LOW | (none) |
| **61340262** | 0 | 0 | 0 | true | CRITICAL | MANIPULATION_CHEAP SPREAD_EXTREME THIN_DEPTH_5PCT ZERO_DEPTH_2PCT |
| **61340263** | 0 | 0 | 0 | true | CRITICAL | MANIPULATION_CHEAP SPREAD_EXTREME THIN_DEPTH_5PCT ZERO_DEPTH_2PCT |
| 02-23 | 216,472 | 221,887 | 216,472 | true | LOW | (none) |
| 02-24 | 221,299 | 222,800 | 221,299 | true | LOW | (none) |
| 02-25 | 225,347 | 216,186 | 225,347 | true | LOW | (none) |
| 02-26 | 213,845 | 218,238 | 213,845 | true | LOW | (none) |
| 02-27 | 214,413 | 224,042 | 214,413 | true | LOW | (none) |
| 02-28 | 216,307 | 215,205 | 216,307 | true | LOW | (none) |

### 5.3 Four things this table says that a reader should not have to hunt for

1. **The band is `LOW` on all 28 daily rows and `CRITICAL` on both control rows.** No flag
   fires on any daily sample in the entire month. The two `CRITICAL` readings are 8 minutes
   51 seconds and 8 minutes 57 seconds after the daily sample that preceded them.

2. **Depth at ±2%, ±5% and ±10% is identical on every daily row**, which is why only the
   ±2% column is shown. The whole resting ladder sits inside 2 per cent of `P0`, so
   widening the window reaches nothing further. The full ladder is in the CSV.

3. **The book grows through the month.** Eight bids and four asks on 1 February, twenty-two
   and ten on the 28th, with buy-side depth between 212,760 and 229,364 USDC throughout. On
   this evidence the market did not thin out before the incident and did not heal after it.

4. **The spread narrows from 0.090 per cent to 0.043 per cent** across the month, and reads
   0.090 on the morning of the incident. Nothing in the daily series points at 22 February.

### 5.4 Where this table disagrees with the hand-computed fixture, and it is not hidden

The golden fixture gives the book at ledger 61340263 worked by hand before any of this code
existed. The repaired run reproduces its `best_bid`, `best_ask`, `P0`, `spreadPct`, the
zero depth ladder, all four flags and the `CRITICAL` band exactly.

**It does not reproduce four quantities, and one offer produces all four:**

| Quantity at ledger 61340263 | Fixture, by hand | This run |
|---|---|---|
| `maxReachablePrice` | 106.7372828 | 2147483647 |
| `costToMaxReachablePrice` | 0 | 124.715139 |
| `Reachable` at δ = 1, 10, 100 | false | true |
| Asks on the book | 1 | 2 |

The second ask is a dust offer at the sentinel price 2147483647. It is visible to this run
because the operation floor reaches back to ledger 60987032, and invisible to the shallower
control run of 5 September 2026. With such an ask resting, every manipulation target is
satisfied by something priced at or above it, so `Reachable` is true at every rung and the
maximum reachable price becomes the sentinel.

**This report does not resolve which side is right and adjusts neither.** Either the
sentinel ask is a second offer that left the book without emitting an event, which is the
class DEC-021 exists for, or the hand computation did not include an offer created long
before the two it was built from. The consequence for a reader is narrow and specific: the
`Cost(δ=0.5)` and `Reachable` columns are sound on the daily rows and must not be quoted
for the two control rows until this is settled. Section 3 of this report reads the incident
ledger from the fixture, not from this run, and is unaffected.

### 5.5 What the repair changed, measured

Against the unrepaired run of the same command, same floor, same caps:

| | Unrepaired | Repaired |
|---|---|---|
| Rows whose book crosses, which no ledger can hold | 6 | **0** |
| `P0` on the 20 daily rows from 9 February | led by the phantom ask | moves in the fifth decimal |
| Depth at ±2% on those rows | | moves by about one part in 10^12 |
| `P0` at the control ledgers | 1.0572 | **53.8971414**, the hand-computed value |
| Depth at ±2% at the control ledgers | 1.06e-7 | **0**, the hand-computed value |
| Bands, anywhere in the series | | **unchanged** |

**The phantom mattered enormously where the book was thin and almost not at all where it
was deep.** That is the opposite of how a one-stroop offer sounds, and it is why the daily
narrative in 5.3 reads the same with or without the repair while the incident ledger does
not.

### 5.6 What the run reports against itself

From the sidecar, and each of these belongs beside any figure quoted above: `fold_complete`
is false on every row, `missing_offer_ids` is 3, `walks_truncated` is 35, `walks_failed` is
6, `walks_stopped_at_floor` is 167, and no AMM pool is reconstructed at all. Every one of
those loses offers, and a lost offer reads as a **thinner** book rather than as an error,
which is the conservative direction named in section 4. The run cost 1,719 requests and
8,312 seconds across 222 accounts.

## 6. When the unsafe threshold was crossed

The Statement of Work asks this report to "identify when the unsafe threshold was crossed
relative to the exploit". This section holds the facts that answer it. **The sentence that
says what those facts MEAN is marked in 6.5 and is not written by the same hand that
produced them**, which is the rule this repository works under and is the reason the
number and its interpretation are separated here rather than blended.

### 6.1 The first day each flag fired

**None of them, on any daily sample, in the whole month.**

| Flag | First daily row it fires on |
|---|---|
| `ZERO_DEPTH_2PCT` | never |
| `THIN_DEPTH_5PCT` | never |
| `SPREAD_EXTREME` | never |
| `MANIPULATION_CHEAP` | never |
| `PRICE_SOURCE_CONFLICT` | never |
| every other flag in `09-flags-and-bands.md` | never |

Six flags read `unevaluated` on every row of this series, because they need supply,
trustline or trade-history data that the historical path does not reconstruct:
`MANIPULATION_RATIO_LOW`, `NO_GENUINE_TRADE_7D`, `NO_GENUINE_TRADE_30D`,
`WASH_TRADE_SUSPECTED`, `HOLDER_CONCENTRATION_HIGH` and `HOLDER_CONCENTRATION_EXTREME`.
`bandConfidence` is therefore `partial` on every row, and `09-flags-and-bands.md` section 2
requires that word to be surfaced wherever a band is.

### 6.2 The first row that reaches CRITICAL

| | Ledger | Time UTC | Band | Flags |
|---|---|---|---|---|
| Last daily sample before the incident | 61340172 | 22 Feb 00:01:24 | `LOW` | none |
| Control ledger | 61340262 | 22 Feb 00:10:15 | **`CRITICAL`** | `ZERO_DEPTH_2PCT`, `THIN_DEPTH_5PCT`, `SPREAD_EXTREME`, `MANIPULATION_CHEAP` |
| Incident ledger | 61340263 | 22 Feb 00:10:21 | **`CRITICAL`** | the same four |
| First daily sample after | 61355036 | 23 Feb 00:00:33 | `LOW` | none |

**All four flags fire at once, and no flag fires before or after.** There is no row in this
series on which one or two of them fire and the others do not.

### 6.3 So the answer is not a date, and the series says so in its own numbers

**A step, a drift and a spike are three different shapes, and this is the third.** A drift
would show the depth ladder thinning over days; the ladder holds between 212,760 and
229,364 USDC of buy-side depth on all 28 daily rows including the morning of the incident.
A step would show the band changing and staying changed; the band reads `LOW` again 23
hours 50 minutes later and every day after.

The transition Keel measures is **8 minutes 51 seconds wide on the reading side and 79
seconds wide on the chain**, and it reverts. The daily grid brackets it without touching
it.

### 6.4 What produced the window, which is measured rather than inferred

`docs/evidences/2026-09-14-february-22-withdrawal-timeline.md` and
`docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md` establish the mechanism
from the operation stream, without passing through any threshold this project chose:

| | |
|---|---|
| The book's market maker deletes its entire USTRY ladder | ledger 61340261, 22 Feb 00:10:09 |
| The manipulation trade executes | ledger 61340263, 00:10:21, **12 seconds later** |
| The maker re-posts its ladder | ledger 61340274, 00:11:28 |
| Withdrawal windows of this kind in February 2026 | **116** |
| Of those, opening between 00:09 and 00:11 UTC | 33, falling on **28 of 28 days** |
| Total time this maker was absent from the book | 7,462 seconds, **0.3084% of the month** |
| Probability a moment chosen without reference to that schedule falls inside a window | about **1 in 324** |

**The withdrawal was routine, scheduled, and the same minute every day.** Trading does not
explain the empty book: four trades totalling 0.3090959 USTRY occurred between the daily
sample and the control ledger. The maker cancelled and re-posted, as it did every day.

### 6.5 What the facts mean

**Keel would not have warned on a daily cadence, and would have read CRITICAL at the ledger the exploit executed in.** Both halves come from the same engine and the same thresholds, written down before the series was computed.

On all 28 daily samples the USTRY/USDC book carried between 212,760 and 229,364 USDC of executable buy-side depth and read `LOW`. At ledger 61340262, six seconds before the manipulation trade, the same methodology reads `CRITICAL` with four flags at once, and it reads `LOW` again on the next daily sample.

The finding is therefore about **sampling, not about the asset's average condition**. The market was deep almost all of the time and empty for 79 seconds a day, at the same minute every day, when its only maker withdrew and re-posted. A monitor that samples once a day brackets that window without touching it. A monitor that reads at the ledger a lending protocol prices collateral would have seen it.

This report does not claim that the attacker knew the maker's schedule. The 1-in-324 figure in 6.4 is the probability that a moment chosen without reference to the schedule falls inside a window. It is stated as arithmetic, and section 7 on hindsight applies to this paragraph more than to any other.

**What it implies for the product**, stated as a design consequence and not as a claim about the incident: depth that disappears on a schedule is invisible to periodic sampling, so a liquidity risk parameter for collateral has to be read at, or close to, the ledger the collateral is valued at.

## 7. Hindsight bias, named

This report knows the date of the attack. Three specific ways that could corrupt it,
and what is done about each.

**Choosing the asset.** USTRY was chosen because it was attacked. A method that
finds danger only in assets already known to have been attacked has demonstrated
nothing. What limits the damage here is that the method is not tuned to this asset:
the same engine ran over 64 active Stellar assets on 26 August 2026 with zero
failures, recorded in `docs/evidences/2026-08-26-scan-64-assets-stored.md`, and the
thresholds are the same for all of them.

**Choosing the thresholds.** The thresholds in `09-flags-and-bands.md` are chosen
rather than calibrated and that file says so. **They were written before this series
was computed**, and section 9 explains how a reader can verify that from the git
history rather than taking it on trust. Had any of them moved after seeing the
result, PRD section 10 requires this report to say so. None has.

**Finding a signal in the trade stream.** This one is a live example rather than a
hypothetical, and it is why the analysis in section 5 uses the book and not the
trades. A reading of the same month's trade stream on 26 August 2026 found exactly
one pre-exploit "signal": a dust trade on 10 February that nobody would have noticed
at the time and that only looks meaningful because the date of the attack is already
known. That reading is `docs/evidences/2026-08-26-ustry-february-trades-implied.md`
section 4.

**What the trade stream could not see, and why that is the whole argument.** USTRY
traded 13,547 times in February at a spread of a fraction of a per cent around 1.057.
Every one of those trades was small and every one stayed inside a price range where
liquidity existed. Nothing in what *traded* was unusual. What made USTRY dangerous
was what was *posted*: a single ask a hundred times above the bid with nothing in
between. A price feed sees the first. Keel is built to see the second.

## 8. Limitations

Each of these is in `docs/methodology/11-limitations.md` or in a decision record,
and is repeated here because a reader of the report should not have to go and find
them.

1. **No AMM pool is reconstructed at any historical ledger.** Section 5 is order book
   only. USTRY had a pool holding honest reserves at 1.0555 for twelve days spanning
   the attack, and it prevented nothing, which is limitation 1 of the methodology.
   Its absence from these figures does not change that conclusion and does bound
   what they measure.
2. **Resting liquidity is not executable liquidity.** An offer can be withdrawn
   instantly. Every depth figure describes what was posted at one instant.
3. **Path payments through intermediate assets are not counted**, so true effective
   liquidity may exceed what is reported.
4. **Centralised exchange liquidity is invisible.**
5. **Thresholds are chosen, not calibrated.**
6. **Order ownership cannot be known ahead of time**, so manipulation cost is always
   an upper bound on what an attacker actually pays.
7. **The collateral parameters in force at the time are not fully recoverable.** The
   Blend `c_factor` for USTRY in February 2026 could not be read from public
   unauthenticated sources; four routes were tried and each is recorded in
   `docs/evidences/2026-08-31-ustry-reserve-config-history.md` section 5. What is
   established is the sign and not the figure: it was above zero, because the
   incident transaction borrowed against a USTRY position.
8. **The reconstruction is a lower bound on the book.** Section 4 says why, and every
   row of section 5 carries the diagnostics that let a reader see how much of the
   book a given day's walk actually reached.

## 9. How to reproduce every number in this report

Nothing here requires a BigQuery account, an API key, or any registration. Public
Horizon and this repository are enough, which is NFR-10.

**The book at the control ledger, and the fixture it is checked against:**

```bash
go run ./cmd/keel bookseries \
  -pairs scripts/record-pairs.example.json \
  -also-ledger 61340262,61340263 \
  -trades-from-ledger 61300000 -since-ledger 61300000 -lookahead 5000 \
  -csv /tmp/control.csv
```

Compare against `testdata/fixtures/ustry_pre_exploit.md`. The reading of that
comparison is `docs/evidences/2026-09-05-control-ledger-validation.md`.

**The February series in section 5**, exactly as the committed artefact was produced:

```bash
go run ./cmd/keel bookseries \
  -pairs scripts/record-pairs.example.json \
  -from-trades docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv \
  -also-ledger 61340262,61340263 \
  -trades-from-ledger 60987032 -since-ledger 60987032 -lookahead 5000 \
  -max-pages-per-account 60 -max-pages-per-offering-account 400 \
  -known-removals configs/known-removals.json \
  -csv docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.csv
```

1,719 requests and 8,312 seconds over 222 accounts on 14 September 2026. Drop
`-known-removals` and it reproduces the unrepaired artefact instead, which is the file of
the same name without the `-repaired` suffix: the flag is the only difference between the
two runs and section 5.5 is the difference between their outputs.

It writes a provenance sidecar beside the CSV in the shape
`docs/decisions/DEC-010-backtest-refuses-window.md` requires. Read
`walks_truncated`, `walks_failed` and `crossed_points` in it before reading any row as a
market that emptied.

**The withdrawal cadence in section 6.4**, which needs no Keel binary at all:

```bash
A=GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN
curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61027032*4294967296))&order=asc&limit=200"
# page on paging_token to ledger 61429800: 231 pages, 46,033 operation records
```

A transaction whose USTRY offer operations all carry `amount: 0.0000000` is a delete-all;
one where none does is a post. A window runs from a delete-all to the next post. The
method, the counts and the per-day table are
`docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md`.

**The trade stream for the same month**, which is what section 7 contrasts against:
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv`,
13,547 rows, every field as Horizon sent it.

**That the thresholds predate the series.** Every threshold is a constant in
`internal/domain.DefaultParams` and a row in `09-flags-and-bands.md`. Both are under
version control, so:

```bash
git log --follow -p docs/methodology/09-flags-and-bands.md | grep -n 'Absolute\|Pct'
git log -1 --format=%cI -- docs/methodology/09-flags-and-bands.md
git log -1 --format=%cI -- docs/report/blend-february-2026.md
```

The threshold values are older than this report. A reader who finds otherwise has
found a defect and should say so.

## 10. What is outstanding on this draft

| Item | Owner | State |
|---|---|---|
| Section 5, the day-by-day table | Claude | **DONE, 14 September 2026.** Generated from the repaired run named in section 9 |
| Section 6, the facts | Claude | **DONE, 14 September 2026.** Sections 6.1 to 6.4 |
| Section 6.5, what the facts MEAN | **Al** | open. The zone map gives every claim about meaning to Al, and 6.5 names the three candidate readings and why two of them are unavailable |
| The headline sentence of section 1 | **Al** | open, same reason, and it follows from 6.5 |
| The sentinel ask at ledger 61340263 | **Al**, then Claude | open. Section 5.4: four fixture quantities disagree with the repaired run because of one dust offer, and whether that offer is a second silent removal or a gap in the hand computation is a decision, not a measurement |
| Whether an AMM reserve series can be added | **Al**, then Claude | open. Pool reserves at a past ledger are not reconstructed today, and whether that gap is closed or stated is a decision |
| Publication | **Al** | open. D3's fifth criterion is "The backtest report published openly" |

## 11. Version history

| Date | Change |
|---|---|
| 5 September 2026 | Structure drafted. Sections 2, 3, 4, 7, 8 and 9 written from evidence already in the repository. Sections 5 and 6 deliberately empty |
| 8 September 2026 | The February series ran and sections 5 and 6 stay empty, with the reason recorded in place of the blank. `docs/evidences/2026-09-08-february-book-series.md` is the reading: the fixture's ask amounts reproduce exactly at both control ledgers, and the book is crossed from 23 February onward |
| 12 September 2026 | The phantom ask resolved to offer `1822775941` and the crossing bid to offer `1824767559`, so the defect is named rather than suspected. Three corrections to the 8 September reading: `best_ask` is wrong from 9 February and not from the 23rd, the deeper-walk fix priced there cannot work, and the fixture-versus-code question is settled in the fixture's favour by the single fill in ledger 61340263. Removal dated to six minutes on 8 February. A crossed-book detector landed in `internal/domain`. Sections 5 and 6 stay empty, and section 10 is unchanged on who owns them |
| 14 September 2026 | **Sections 5 and 6 filled.** Three things landed first. The discontinuity of 22 February was resolved from the operation stream: the book's market maker deleted its whole ladder at ledger 61340261 and re-posted 79 seconds later, so the fold lost nothing. That withdrawal was then shown to be routine, 116 windows in February and one at 00:10 UTC on all 28 days. And the phantom ask of 9 to 28 February was removed under DEC-021, a repair that is off by default, declared in every artefact, and validated against the hand-computed fixture on four criteria written down before the run finished. Section 5 is generated from that run. Section 6 carries the facts and marks 6.5, the meaning, as Al's. Section 5.4 records four fixture quantities the run does not reproduce and attributes them to one dust offer rather than to the repair |
