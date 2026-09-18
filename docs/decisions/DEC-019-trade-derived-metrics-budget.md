# DEC-019: The trade-derived metrics cost 8.6 hours of Horizon per refresh, and that decides their shape

**Status:** Accepted by Al, 18 September 2026. **OPTION B at a threshold of 20,000
trades in 30 days**, which section 5 recommends: pairs at or below the threshold have
the whole 30 day window classified, and pairs above it report the volume half
unevaluated WITH the reason string section 5 asks for. Section 9 records what was
built against it and what the record still does not decide.
**Date drafted:** 2026-09-12
**Kind:** Request budget and metric cadence. It changes no formula and no threshold. It
decides which assets can carry the trade-derived half of `SupportingMetrics` at all.
**Drafted by:** Claude
**Decided by:** Al, 18 September 2026, Option B at 20,000. Between 12 and 18 September
this line read "pending" and the whole record was marked DRAFT, with the sentence
"Nothing in this record is in force. Every section is a proposal and section 4 is a
choice nobody has made." That is kept here rather than deleted, because the reason the
record was filed before an implementation is the part worth carrying: the arithmetic in
section 2 rules out the obvious shape, and building the obvious shape first and
measuring afterwards is how the six days in the `compute.go` zone row happened.
**Extends:** DEC-018, whose point 2 is accepted and whose provenance pattern this record
reuses rather than reinvents. DEC-018 solved the same problem for holder concentration;
this is the same problem for the trade stream, and it is worse by two orders of magnitude.

---

## 1. What is not computed today, and where the expensive half of it starts

`keel scan` attaches holder concentration as of commit `9ca196a`. It attaches nothing
derived from trades. In production every asset therefore reports five flags
`unevaluated`:

| Requirement or flag | PRD prio | What it needs |
|---|---|---|
| FR-9, volume-to-supply over 24h, 7d, 30d | S | every trade in the window, classified |
| FR-10, time since the last genuine trade | S | trades back to the last genuine one |
| `NO_GENUINE_TRADE_7D` | | the same |
| `NO_GENUINE_TRADE_30D` | | the same |
| `WASH_TRADE_SUSPECTED` | | every trade in 30 days, classified |

**Not one of the five can use an aggregate, and that is one sentence in the methodology.**
`07-supporting-metrics.md` section 3 defines the ratio as **"genuine volume ÷ circulating
supply"**, and section 3's numerator paragraph says "the trades that count are those
marked genuine".

That sentence is what closes the cheap road. Horizon's `/trade_aggregations` returns
volume per time bucket for one or two requests per pair, which would serve a
volume-to-supply ratio built on TOTAL volume in about 180 requests for the whole set.
It cannot serve one built on GENUINE volume, because exclusion is a per-trade verdict and
an aggregate has already thrown the trades away.

**THE FAMILY SPLITS IN TWO, AND THE SPLIT IS BY HOW FAR BACK THE ANSWER LIVES rather
than by which requirement it serves.** An earlier draft of this record said the five
stand or fall together. That is wrong, and the correction is what makes option B in
section 4 smaller than it first looks:

| | How far back | Cost shape |
|---|---|---|
| FR-10 and the two `NO_GENUINE_TRADE` flags | only to the LAST genuine trade | about one page per pair, because the answer is found at the recent end and the walk stops there |
| FR-9 and `WASH_TRADE_SUSPECTED` | the WHOLE window, 24h, 7d and 30d | every trade classified, which is section 2's arithmetic |

The first row does not need section 2's budget at all and should not wait on this record.

> **AMENDED 17 September 2026. The first row is still cheap and "about one page per
> pair" is wrong, because the unit is a DAY and not a page.** See section 8. The
> conclusion that it need not wait on this record survives; the arithmetic under it does
> not.

The second row is what this record is actually about. Sections 2 through 6 below concern
the second row only.

---

## 2. The arithmetic, from measurements already in this repository

**Per-pair trade counts** are the `N trades in 30 days` figures recorded in
`configs/demonstration-set.json`, measured on Horizon on 26 August 2026. 60 of the 61
pairs carry one. **Page size** is Horizon's 200 records. **The cap** is NFR-6, 3000
requests an hour.

| | |
|---|---|
| trades in 30 days, across the 60 measured pairs | **5,149,904** |
| pages at 200 per page | **25,782** |
| hours at the NFR-6 cap | **8.6** |

**The cost is not spread evenly and that is the whole of the opportunity:**

| Pair | Trades in 30 days | Pages | Minutes at the cap |
|---|---|---|---|
| XLM | 1,934,524 | 9,673 | **193** |
| HU | 666,311 | 3,332 | 67 |
| TGM | 427,191 | 2,136 | 43 |
| yXLM | 416,382 | 2,082 | 42 |
| XRP | 200,701 | 1,004 | 20 |

**XLM alone is 193 minutes**, which is more than half the total of every other pair
combined. Eight pairs carry most of the bill.

**A walk of this shape has been run once and it is the only wall-clock figure that is
real.** `keel backtest` over USTRY/USDC for February 2026, re-run on 12 September 2026:
13,547 trades in the window, **521 pages, 104,050 records read, 2 minutes 47 seconds**.
The 7.7x between records read and trades kept is the overshoot of a window six months in
the past and does not apply to a rolling window ending now, so the page figures above are
not inflated by it.

---

## 3. What this rules out

**Per scan round is impossible and not by a small margin.** `scan` runs every 15 minutes
in production. One refresh is 8.6 hours. Even one pair, XLM, does not fit in a round.

**Daily for the whole set consumes the budget.** 8.6 hours of continuous requests at the
cap leaves 15 hours a day for the depth scans, the holder pull that DEC-018 point 1
describes, and any replay. It is arithmetically possible and it is the whole machine
doing one thing.

So the shape is forced in the same way DEC-018 section 2 forced the holder shape: a cache
filled on its own schedule, read by `scan`, carrying its own provenance ledger under
DEC-018 point 2's accepted rule. **What is NOT forced, and is the actual decision, is
which assets go in it.**

---

## 4. The options, priced

**A. Every asset, once a day.** 8.6 hours a day of Horizon. Complete coverage. Leaves
little budget for anything else and no headroom for a retry storm.

**B. Every asset below a trade-count threshold; the rest report `unevaluated`.**

| Threshold, trades in 30 days | Assets covered | Pages | Minutes at the cap |
|---|---|---|---|
| 5,000 | 20 of 60 | 287 | **6** |
| 20,000 | **39 of 60** | 1,164 | **23** |
| 50,000 | 45 of 60 | 2,093 | 42 |
| 100,000 | 51 of 60 | 4,467 | 89 |

**C. Change the methodology so the ratio uses total volume.** `/trade_aggregations` then
serves FR-9 for the whole set in about 180 requests. It does nothing for FR-10 or for the
three flags, and `docs/methodology/` is RED, so this is Al's alone and it is a change to
the paid deliverable rather than to the code.

**D. Raise the request budget above NFR-6.** Public Horizon allows about 3600 an hour per
IP, so the headroom is 20 percent and not a multiple. It does not change the shape.

---

## 5. The recommendation, and the reason is an asymmetry rather than a preference

**Option B at a threshold, and the case for it is that the cost and the value run in
opposite directions.**

The two measures in the expensive row are `WASH_TRADE_SUSPECTED` and the volume-to-supply
ratio, and both are least informative exactly where they cost most. `WASH_TRADE_SUSPECTED`
fires when more than half of 30 day volume is excluded by the genuine-trade rules. **XLM,
at 1,934,524 trades in 30 days, is the single most expensive pair in the set** and is also
the one whose volume nobody suspects of being wash. The two pairs that trade fewer than 30
times a month, `EMN` and `FUNT`, are where an exclusion ratio actually says something, and
they cost **zero full pages between them**.

So the assets that make these measures informative are nearly free, and the assets that
consume the budget are the ones whose answer is least in doubt. A threshold buys 39 of 60
assets for 23 minutes instead of 60 for 8.6 hours.

**What it costs, stated rather than buried:** an asset above the threshold reports a nil
volume-to-supply ratio and `WASH_TRADE_SUSPECTED` unevaluated, which is the same answer it
gives today, so option B is strictly better than the current state and not a compromise
against it. `bandConfidence` stays `partial` for those assets. A reader cannot tell from
`unevaluated` alone whether the asset was too busy to classify or too quiet to have data,
which is why the threshold, if
adopted, needs its own reason string in the way DEC-018 point 3's staleness bound does.

---

## 6. What this record does NOT decide

1. **The threshold number.** Nothing in section 2 derives 20,000; it is a row in a table
   that a reader can move. It is proposed exactly as DEC-018 point 3 proposes 48 hours,
   and if adopted it belongs behind a flag for the same reason.
2. **Whether the methodology moves.** Option C is Al's and is a RED-zone change.
3. **The cadence.** Daily is assumed throughout section 4 because DEC-018 point 1 assumes
   it for holders, and that point is itself still a proposal.
4. **Whether the API exposes the trade half's own ledger and age.** That is DEC-018 point
   4, which is a contract change and is unaccepted; this family would land under whatever
   that point settles rather than inventing a second answer.

---

## 7. Reproducing section 2

```bash
python3 - <<'EOF'
import json, re
d = json.load(open('configs/demonstration-set.json'))
rows = []
for p in d['pairs']:
    m = re.search(r'(\d+) trades in 30 days', p.get('note', ''))
    if m:
        rows.append((p['base']['code'] or 'XLM', int(m.group(1))))
pages = sum((n + 199) // 200 for _, n in rows)
print(len(rows), 'pairs', sum(n for _, n in rows), 'trades', pages, 'pages', round(pages/3000, 1), 'h')
EOF
```

The wall-clock figure:

```bash
go run ./cmd/keel backtest -pairs scripts/record-pairs.example.json \
  -from 2026-02-01 -to 2026-03-01 -out <a directory>
```

---

## 8. Version history

| Date | Change |
|---|---|
| 12 September 2026 | Drafted. Nothing in force. The 8.6 hour figure and the threshold table are measured from `configs/demonstration-set.json` and from the February `backtest` re-run of the same day |
| 12 September 2026, later | Section 1 corrected before this record was ever committed. The first draft claimed the five measures stand or fall together; they do not. FR-10 and the two `NO_GENUINE_TRADE` flags stop walking at the last genuine trade and cost about one page per pair, so they are outside this record's budget problem entirely. Section 5's argument was rebuilt on `WASH_TRADE_SUSPECTED` and the volume-to-supply ratio, which are the two that really do need the whole window |

---

## 8. AMENDMENT, 17 September 2026: the cheap half cannot stop mid-day

**This section AMENDS and decides nothing.** It corrects an arithmetic claim this
record makes about its own cheap half, found while starting to build that half. The
recommendation in section 5 and the options in section 4 are untouched: they concern
the expensive row, and nothing here reaches them.

### 8.1 What the record claims and why it is wrong

Section 1 splits the family and prices the cheap row at "about one page per pair,
because the answer is found at the recent end and the walk stops there". The walk can
indeed stop at the recent end. **It cannot stop at an arbitrary page, and a page is
therefore the wrong unit.**

`domain.ClassifyTrades` computes `dailyOrderBookMedians(trades)` and
`orderBookPricesByTime(trades)` over the WHOLE slice it is given, and two of the five
conditions read them:

| Condition | Depends on | Safe to judge from one page? |
|---|---|---|
| 1, self trade | the trade alone | yes |
| 2, dust | the trade alone | yes |
| 3, issuer leg | the trade alone | yes |
| 4, off-book pool fill | the order book prices around it, ±15 minutes | no |
| 5, price outlier | the median of that UTC DAY | **no** |

So a backwards walk that stops part way through a day computes condition 5 against the
median of a partial day, which is a different statistic from the one the rule names.
`dailyOrderBookMedians` carries that warning in its own header, about a sliding subset,
and this is that case arriving from the other direction.

### 8.2 The error runs in the optimistic direction, which is what makes it disqualifying

A trade that passes conditions 1 to 3 can look genuine against a partial day's median
and be excluded once the whole day is known. The verdict can only move one way as more
of the day arrives: from genuine to excluded, never back. **So a partial walk reports a
last genuine trade that is NEWER than the true one, and the asset reads as fresher than
it is.**

`NO_GENUINE_TRADE_7D` and `NO_GENUINE_TRADE_30D` are the flags that fire on staleness.
An implementation that can only err towards "traded more recently than it did" is an
implementation that can only fail to raise those flags. That is the direction DEC-022
section 5.1 refuses without an override for the reconstruction gate, and the same
reasoning applies here: a warning product may fail towards more dangerous and not
towards safer.

### 8.3 The corrected unit and the corrected cost

**The smallest honest unit is one whole UTC day.** Priced from the same
`configs/demonstration-set.json` counts section 2 uses, at 200 records a page:

| | Pages | Minutes at the NFR-6 cap |
|---|---|---|
| One whole UTC day, every pair | **890** | **17.8** |

and the shape works in this half's favour, unlike the expensive half's:

| Pair | Trades per day | Pages for one day |
|---|---|---|
| XLM | 64,484 | 323 |
| HU | 22,210 | 112 |
| TGM | 14,240 | 72 |
| yXLM | 13,879 | 70 |

| Pair | Trades in 30 days | Pages for the WHOLE month |
|---|---|---|
| RCW | 2,203 | 12 |
| GROG | 1,706 | 9 |
| TRNPC | 1,649 | 9 |
| EMN, FUNT | 0 | 0 |

**A busy pair resolves on the first day walked, and a pair that needs many days walked
is quiet by definition and costs almost nothing per day.** The two ends of the set pay
for opposite reasons and both are small. A realistic figure for the whole set is 20 to
30 minutes rather than section 1's implied 61 pages, and still nothing like section 2's
8.6 hours.

> **AMENDED 18 September 2026, AND THE SENTENCE IN BOLD ABOVE IS FALSE.** The two
> properties are independent and one pair class has both. HU/USDC trades 22,210 times a
> day AND had no genuine day for at least nineteen of them, so it spent about 2,100
> requests on its own in the first pass run under this decision and starved the forty
> pairs behind it. See section 9.8 for the measurement, why the threshold selects for
> exactly this pair class, and the page bound added in response. The worst case stated
> two paragraphs below is therefore not the worst case.

**The worst case is worth stating rather than hiding**: a pair with no genuine trade in
30 days walks all 30 days before answering, and for the noisiest such pair that is the
expensive half's bill. It is bounded by whatever horizon the walk is allowed, and that
bound is a number this record does not choose, exactly as it does not choose the
threshold in section 6 item 1.

### 8.4 What this changes for whoever builds it

1. The walk fetches and classifies in whole UTC days, and a day is either complete or
   not used.
2. It walks back day by day until a day contains a genuine trade, or until a bound is
   reached. Reaching the bound reports `unevaluated`, never "no genuine trade", because
   those are different answers and only one of them is a finding.
3. `internal/horizon`'s existing `Trades` walks `order=asc` from a cursor, which suits
   the backtest's forward window. A backwards day walk is a second shape over the same
   endpoint and should say so rather than overload `TradeQuery`.
4. Conditions 1 to 3 remain per-trade and could in principle answer a cheaper question,
   but not this one. FR-10 asks for the last GENUINE trade, and genuine is the verdict
   of all five conditions together.

---

## 9. ACCEPTED, 18 September 2026, and what was built the same day

**Al chose option B at 20,000.** Section 5 argued for it on an asymmetry rather than a
preference, and nothing in the implementation changed that argument: the pairs that make
`WASH_TRADE_SUSPECTED` and the volume-to-supply ratio informative are nearly free, and the
pairs that consume the budget are the ones whose answer is least in doubt.

### 9.1 What exists now

| Piece | Where | Zone |
|---|---|---|
| The backward walk in whole UTC days | `internal/horizon/tradedays.go`, `WalkTradeDays` | YELLOW |
| The cache table and its constraints | `migrations/0009_trade_readings.sql` | GREEN |
| Storing and reading a row | `internal/store/trades.go` | GREEN |
| The pass, the threshold gate and the bound | `cmd/keel/trades.go` | GREEN |
| Reading the cache into a scan round | `cmd/keel/scan.go`, `attachTradeHalf` | GREEN |

Section 8.4 listed four things it said whoever built this would have to do. All four are
done: the walk fetches and classifies in whole UTC days, it walks back day by day until a
day holds a genuine trade or the bound is reached, reaching the bound reports unevaluated
rather than "no genuine trade", and the backward shape is a second walk beside
`TradeQuery` rather than an overload of it.

### 9.2 Three numbers this record left open, and where each one now lives

1. **The threshold**, section 6 item 1. `keel trades -threshold`, default 20,000. It is a
   flag and not a constant for the reason that section gives: nothing in section 2 derives
   20,000, it is a row in a table a reader can move.
2. **The walk bound**, section 8.3's "a number this record does not choose".
   `keel trades -max-days`, default 30. Thirty is not an arbitrary budget: it is the window
   `NO_GENUINE_TRADE_30D` and `WASH_TRADE_SUSPECTED` are defined over, so a walk that
   completes thirty whole days and finds nothing has MEASURED both rather than run out of
   road. Any smaller default would make the commonest outcome an unevaluated one.
3. **The staleness bound on the cache**, which is DEC-018 point 3's question asked of this
   table. `keel scan -max-trade-age`, default 36 hours, measured from the ANCHOR and not
   from the pull. It is tighter than the holder bound's 48 hours because a trade reading is
   already up to 24 hours behind by construction, so 48 would admit a window that ended
   nearly three days ago.

### 9.3 Two things the implementation does that this record should be read as having decided

**The gate reads the 30 day count out of the selection note**, which is the field section 7
reproduces its own arithmetic from, with the same regular expression. That count was
measured on Horizon on 26 August 2026 and is not refreshed by the pass, so the gate is
applied on a figure that may be three weeks stale, and the row's reason string says so in
as many words. The alternative was counting during the walk, which is self-updating and
would cost up to 100 pages per over-threshold pair before aborting, roughly forty minutes
the accepted option does not include. **A pair whose note carries no count is treated as
ABOVE the threshold**, because a missing count read as zero would buy the most expensive
walk for the pair nobody has measured.

**Coverage is checked separately from the threshold.** A pair under the threshold whose walk
was cut short by the bound has no complete window either, and its row is stored as
`last-genuine-only` with a reason naming the days it did cover. Storing partial sums as a
measured volume is the error this whole record is about, and the threshold alone does not
prevent it.

### 9.4 One understatement that is open, named rather than left to be discovered

A walk that reached the end of a pair's history without meeting a genuine trade has
MEASURED that the pair never genuinely traded, and the row records it with `exhausted`.
The flag rules in `internal/domain/flags.go` evaluate `NO_GENUINE_TRADE_7D` and
`NO_GENUINE_TRADE_30D` only when `sup.LastGenuineTrade` is non-nil, so that measurement
currently surfaces as unevaluated rather than as the flag firing.

The direction is safe and it is the same direction section 8.2 permits, so this is a
missed finding and never a false reassurance. Closing it means teaching the flag rules to
read "measured absence" apart from "not measured", which is a change in a YELLOW package
and a shape question of its own: it needs a second field beside the reference, because a
nil pointer cannot carry the difference. It is not done here and it is not forgotten.

### 9.5 What is still not decided

Section 6 items 2, 3 and 4 are untouched. The methodology does not move, so option C
remains unchosen and `07-supporting-metrics.md` keeps defining the ratio over genuine
volume. The cadence is assumed daily and is a flag rather than a schedule until DEC-018
point 1 is accepted. And whether the API exposes the trade half's own anchor and age is
DEC-018 point 4, still a draft, so this family carries its provenance in the database and
not yet in the contract.

### 9.6 A data-source fact this record's implementation had to discover

**Public Horizon sends no `Latest-Ledger` header on `/trades`**, measured 18 September
2026. `internal/horizon/CLAUDE.md` trap 6 said the collection endpoints send it and is
corrected in the same change.

It reaches this record because it changed the schema built under it.
`trade_readings.ledger_seq` is nullable rather than `NOT NULL`, and the sequence is taken
from the paging token of a trade in the walk, whose high 32 bits are the ledger. That is
decoding an identifier rather than deriving one from a time, which is the distinction
`00-overview.md` section 2 rule 4 turns on. A pair with no trade at all in the walked
window has no token to take it from and therefore no ledger to cite, so the column admits
NULL and a second constraint stops the absence spreading: a row that names a last genuine
trade must name a ledger, because a walk that met a genuine trade met a trade.

**`docs/methodology/01-data-sources.md` says nothing about this header and is RED.** It
is marked complete and every claim in it was verified against Horizon mainnet, so whether
it gains a line about which endpoints carry the stamp is Al's call, not this record's.
Flagged here rather than left in a commit message.

### 9.7 What the threshold buys and what the genuine-trade rule then declines to judge

**This is the one consequence of option B that section 5 did not price, and it is not a
defect in anything.** The threshold decides which pairs are WALKED in full. It has no
bearing on how many of the walked trades resolve to `genuine`, and
`07-supporting-metrics.md` section 1 gives that rule three outcomes rather than two: a
liquidity-pool fill with no order-book trade inside the ±15 minute comparison window is
**Unevaluated**, never Genuine, because scoring it against an hour-stale book is the
failure the rule was written to avoid.

So a pair filled entirely from a pool reports **no last genuine trade at all**, however
far back it is walked, and FR-10 stays unevaluated for it. The first reading stored under
this decision is the example: GROG/USDC on 18 September 2026, 30 whole days walked, 14
pages, `trades_excluded_pct` 2.69 per cent, and not one genuine trade. 97 per cent of its
volume is Unevaluated rather than excluded.

**Why it belongs in THIS record.** Section 5's argument for the threshold is that the
pairs which make these measures informative are nearly free, and the quiet long tail is
exactly where pool fills dominate. The threshold buys the walk for those pairs; the rule
may then decline to judge them. Both statements are correct and they were not read
together until the pass ran.

**What it is NOT.** It is not an argument to relax condition 4, which exists because
`07` measured what a wider window does: the 8-minute-to-±15-minute change moved 133
comparable August fills into Unevaluated. It is not a bug in the walk. And it is not a
reason to move the threshold, because the threshold is not what causes it.

**What is owed, and neither half is this record's to write.** The set-wide incidence is a
measurement and is being recorded in `docs/evidences/`. Whether `07-supporting-metrics.md`
should state the consequence for pool-dominated pairs, beside the USTRY figures it already
carries, is a change to a RED document and is Al's.

### 9.8 AMENDMENT: section 8.3's cost model for the cheap half is wrong, measured

**THIS IS THE SECOND TIME THIS RECORD HAS MISPRICED ITS OWN CHEAP HALF, and the
second correction is a bigger one than the first.** Section 8 corrected section 1's
"about one page per pair" to "one whole UTC day". Section 8.3 then priced the day, and
the sentence that carries the whole argument is this one:

> A busy pair resolves on the first day walked, and a pair that needs many days walked
> is quiet by definition and costs almost nothing per day.

**It is false, and the counterexample is in the demonstration set.** The two properties
are independent, and one pair class has both:

| | Trades per day | Days walked before an answer | Pages |
|---|---|---|---|
| section 8.3's busy pair | many | 1 | ~1 day's worth |
| section 8.3's quiet pair | few | up to 30 | almost nothing |
| **HU/USDC, measured 18 September 2026** | **22,210** | **~19 and still none** | **~2,100** |

**How it was measured.** The first pass under this decision ran at 16:30 UTC on
18 September 2026 over 64 pairs. It stored 24 readings and failed 40. HU/USDC ran from
16:43:47 to 16:57:00, consumed the remaining request budget on its own, then failed, and
every pair behind it failed instantly on `request budget for this window is spent`. One
pair spent more than section 2 prices for the ENTIRE under-threshold set, which is 1,164
pages.

**Why the two properties combine rather than exclude each other, which is the part
section 8.3 missed.** A pair is over the threshold because it is BUSY. A pair filled from
a liquidity pool has no genuine day at all, because condition 4 of the genuine-trade rules
marks a pool fill with no contemporaneous order book `Unevaluated` and never Genuine, which
is section 9.7. A busy pool-filled pair therefore pays the busy page rate for the whole
span the day bound allows. The threshold selects for exactly the first property and
section 9.7's rule supplies the second.

**The mechanism added in response, and the number that is not this record's to fix.**
`keel trades -max-pages`, default 400, bounds ONE pair's walk in Horizon pages.
Reaching it is not a finding, for the reason section 8.4 item 2 gives about the day
bound: the row is stored with no last genuine trade, with `page_cap` named in its reason
string, and with the days and pages it did cover. 400 is chosen so the busiest pair in the
set, XLM at 64,484 trades a day or 323 pages, can still complete its newest day; a cap
below that would guarantee every busy pair could never answer FR-10, which is refusing the
question rather than bounding it. The number is a flag and Al's to move, exactly as the
threshold in section 6 item 1 and the day bound in section 9.2 are.

**A second mechanism, because the first pass proved the pass was not resumable.**
`SaveTradeReading` already refused a duplicate row for one (pair, anchor, methodology),
so a re-run was safe; it was not cheap, because the walk ran in full before the store
declined it. A pass that dies at pair 24 of 64 then has to buy those 24 walks again to
reach pair 25, out of a budget that is already spent, which is the same starvation
arriving a second time. The pass now checks for today's row before walking and skips it,
and `-refresh` is how a reading is deliberately retaken.

**WHAT THIS DOES NOT CHANGE.** Option B stands, the threshold stands at 20,000, and
section 5's argument is untouched: it is an argument about which pairs are worth the
expensive walk and it says nothing about what the cheap walk costs. Sections 2 and 4 price
the expensive half and are unaffected. What is corrected is section 8.3 alone, and the
lesson it carries is the one section 8 already stated about itself: an arithmetic claim in
this record has now been falsified twice by the first implementation that tried to use it,
and both times the error was in the direction of cheapness.

### 9.9 The understatement in 9.4 is measured, and it is nearly half the set

**Section 9.4 named an understatement and treated it as an edge.** The first pass measures
it and it is not an edge. Of 55 pairs read on 18 September 2026, **26 report no last
genuine trade**, and not one of them is the measurement "this pair has never genuinely
traded": 23 stopped on the 30 day bound and 3 on the page bound. FR-10 is unevaluated for
47 per cent of the set, and `NO_GENUINE_TRADE_7D` and `NO_GENUINE_TRADE_30D` with it.
`docs/evidences/2026-09-18-trade-derived-metrics-first-pass/` carries the table and the
raw CSV.

**THIS RECORD CONTRADICTS ITSELF ON THOSE 23 AND THE CONTRADICTION IS NAMED HERE RATHER
THAN RESOLVED QUIETLY.** Section 9.2 argues that 30 days is the right bound precisely
because "a walk that completes thirty whole days and finds nothing has MEASURED both of
them rather than run out of road". Section 8.4 item 2 says the opposite about the same
walk: reaching the bound reports unevaluated, never "no genuine trade". Both sentences are
in force and the implementation follows 8.4, so 23 pairs that were walked across the exact
window the two flags are defined over report nothing.

**Which one is right is not Claude's to settle**, because it decides whether a flag fires
on a real asset in a product whose whole claim is that it warns. What can be said without
deciding it: the two sentences differ only when the bound EQUALS the flag's window, which
is the default and only the default, so a reader who moves `-max-days` to 14 makes 8.4
unambiguously correct and one who leaves it at 30 does not.

**What it costs while it stands.** Every one of those 26 pairs carries `bandConfidence:
partial` for a reason that is a bound rather than a property of the asset, and a reader
cannot tell the two apart from the API response, only from the `trade_readings` row behind
it. That is the same gap DEC-018 point 4 describes for the holder half's provenance, and
it lands under whatever that point settles rather than inventing a second answer.

### 9.10 The decision delivered what it was for, and one row is the proof

**`bandConfidence` read `full` on 18 September 2026 for the first time since the engine
started storing rows on 25 August.** 17,662 of 17,663 stored metrics rows read `partial`;
ACT/USDC is the other one, after `keel trades`, a holder pull for that asset, and one scan
round.

That is the outcome section 1 of this record set out to reach. Three of the five
requirements it lists were unevaluated on every asset the engine had ever scored, and
`09-flags-and-bands.md` section 2 requires a dashboard to surface that word, so every row
of the demonstration set displayed `partial` to a reader. One row no longer does.

**It is one row and not sixty, and the reason is the holder half rather than this
decision.** The scan round that produced it reported `55 with trade figures, 0 with holder
figures` across 64 pairs: the development database's holder cache was never filled, and
the volume-to-supply ratio needs a denominator only that cache carries. The production
stack refreshes it daily under DEC-018, so the figure that matters for a deployment is the
55.

`docs/evidences/2026-09-18-trade-derived-metrics-first-pass/` carries the run, the raw
CSV, and the four findings above with their measurements.

### 9.11 REVERSAL: section 8.4 item 2 is overruled for a search of known length

**Al settled the contradiction named in section 9.9 on 18 September 2026, in favor of
section 9.2.** A search that covered a flag's own window and found no genuine trade has
MEASURED that flag, and `NO_GENUINE_TRADE_30D` now fires on it.

**THIS IS RECORDED AS A REVERSAL AND NOT AS A REINTERPRETATION.** Section 8.4 item 2 reads,
and still reads:

> It walks back day by day until a day contains a genuine trade, or until a bound is
> reached. Reaching the bound reports `unevaluated`, never "no genuine trade", because
> those are different answers and only one of them is a finding.

That sentence is **overruled in one case and left standing in every other**. The case is a
bound that reaches at least as far as the threshold being answered. Its reasoning survives
everywhere else and is why the reversal is narrow: a walk that stopped after four days
still says nothing about thirty, and still reports unevaluated.

### 9.11.1 What was built

`domain.SupportingMetrics` gained `GenuineSearchWindow *time.Duration`, "how far back from
the search anchor the trade set is known to be complete". A nil `LastGenuineTrade` used to
be two answers wearing one face, "nobody looked" and "the search covered thirty whole days
and found none", and a pointer cannot carry the difference. `internal/domain/flags.go`
reads the pair: a reference is used when there is one, its absence over a search of known
length answers each threshold the search reached, and anything shorter stays unevaluated.

The value travels from `trade_readings.days_walked`, which is exact rather than inferred:
the walk emits whole UTC days contiguously backwards and discards any partial one, so the
days it reports are complete and adjacent.

**One known over-warning, stated rather than found later.** The window is measured from the
trade search's anchor, which is the last complete UTC day and is older than the ledger being
scored. A genuine trade inside that gap is not examined, so a staleness flag can fire when
it should not. That is the direction section 8.2 permits and the opposite of the one it
refuses, and it is the same 24 hour staleness the whole walk carries by construction.

### 9.11.2 What it changed, measured on the same data

One scan round over 64 pairs, before and after, on the local database holding the 55
readings this record's section 9.9 describes:

| | Before | After |
|---|---|---|
| `NO_GENUINE_TRADE_30D` triggered | 0 | **24** |
| `NO_GENUINE_TRADE_7D` triggered | 7 | **30** |
| `NO_GENUINE_TRADE_30D` unevaluated | 35 | **12** |

The 12 that remain are the 9 pairs with no reading at all and the 3 that stopped on the
page bound, which are exactly the cases the un-reversed half of item 2 still covers. 23
assets moved from "not checked" to an answer, which is the figure section 9.9 predicted.

`internal/conformance` and `testdata/fixtures/` are untouched and were not asked to move:
USTRY carries a genuine trade reference, so it takes the same branch it always did. The
ordering rule is named out loud in the code, as `internal/domain/CLAUDE.md` requires: no
fixture value judges this change, because it computes no quantity and decides only which
of three states a flag is in.

### 9.11.3 The reversal moves the code TOWARDS the methodology, not away from it

**This was checked before the change was written and it settles whether a RED document has
to move: it does not.** `docs/methodology/09-flags-and-bands.md` states the rule as

```
no genuine trade within Thresholds.GenuineTradeStaleDays days
```

and says nothing anywhere about a reference to the last genuine trade. The phrase
"reference" appears once in that file and it is about the reference PRICE.

So the old code was not implementing that sentence. It could only answer the narrower
question "the last genuine trade was more than N days ago", which needs a trade to measure
from and is silent when there is none. "No genuine trade within N days" is answered by a
search of N days that found none, and that is the sentence the paid deliverable carries.

`flags.go`'s own header sets the rule for this case: "Where this file and that document
disagree, the document is right. It owns the three states, the tiers, and every threshold;
this file owns none of them." The file disagreed with the document from the day it was
written on 26 August 2026 and nobody noticed, because the disagreement was invisible while
no trade history existed to feed it. **Section 3 of the same document is untouched and
still governs the other direction**: the six flags that need trade history become
`unevaluated` when that input is ABSENT, which is what a pair with no reading, or a search
shorter than the threshold, still gets.
