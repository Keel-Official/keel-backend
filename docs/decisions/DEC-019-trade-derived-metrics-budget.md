# DEC-019: The trade-derived metrics cost 8.6 hours of Horizon per refresh, and that decides their shape

**Status:** **DRAFT. Nothing in this record is in force.** Every section is a proposal
and section 4 is a choice nobody has made. It is filed now rather than after an
implementation because the arithmetic in section 2 rules out the obvious shape, and
building the obvious shape first and measuring afterwards is how the six days in the
`compute.go` zone row happened.
**Date drafted:** 2026-09-12
**Kind:** Request budget and metric cadence. It changes no formula and no threshold. It
decides which assets can carry the trade-derived half of `SupportingMetrics` at all.
**Drafted by:** Claude
**Decided by:** pending
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
