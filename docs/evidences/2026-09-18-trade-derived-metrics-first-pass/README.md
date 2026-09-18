# The first pass of the trade-derived metrics, 18 September 2026

**What this records.** The first execution of `keel trades` under DEC-019, accepted by Al
the same day as option B at a threshold of 20,000 trades in 30 days. It measures what the
accepted option actually costs and what it actually answers, against the demonstration set
on public Horizon.

**Why it is filed.** Three of the figures below contradict figures in DEC-019, and one of
them contradicts a sentence the record uses to justify the shape of the cheap half. A
measurement that disagrees with a decision record is the finding, not an inconvenience.

| | |
|---|---|
| **Run** | `go run ./cmd/keel trades`, three passes, 16:30, 17:01 and 18:47 UTC on 18 September 2026 |
| **Anchor** | `2026-09-18T00:00:00Z`, the start of the day all three passes ran in |
| **Methodology** | `1.0.8-draft`, the version the engine stamps |
| **Horizon** | `https://horizon.stellar.org`, public, budget 3,000 requests an hour (NFR-6) |
| **Set** | 64 active pairs in the local `assets` table |
| **Stored** | **64 of 64.** The first two passes stored 55 and the third finished the rest |
| **Raw** | `trade-readings-2026-09-18.csv`, one row per pair, exported from `trade_readings` |

Every figure below is reproducible from the CSV beside this file. The CSV is the table as
stored, not a summary of it.

---

## 1. What the pass answered

Over all 64 pairs:

| Outcome | Pairs | Pages | What it is |
|---|---|---|---|
| Has a last genuine trade | **35** | 1,816 | a measurement |
| None, over a completed 30 day search | **25** | 798 | a measurement, and it fires the flag |
| None, stopped on the 400 page bound | **4** | 1,600 | genuinely unevaluated |
| None, and the whole history was seen | 0 | 0 | |

| Scope | Pairs | Pages | Pages per pair |
|---|---|---|---|
| `full-window` | 39 | 1,697 | 44 |
| `last-genuine-only` | 25 | 2,517 | **101** |

**60 of 64 pairs carry an answer to FR-10 and 4 do not.** That sentence changed on the same
day: before the reversal recorded in DEC-019 section 9.11, the 25 pairs in the second row
reported "not checked" rather than a fired flag, and the answered figure was 35 of 64.

**The 39 full-window pairs are exactly the count DEC-019's own table predicts for a
threshold of 20,000**, which is the one prediction in that record the run confirmed. Its
page figure did not survive: the record prices those pairs at 1,164 pages and they cost
1,697, 46 per cent more. Part of that is the set being 64 here and 60 there, and part is
that the counts the gate reads were measured on 26 August and trade volumes have moved
since. The gate is applied on a figure three weeks stale, which section 9.3 of the record
already states.

**The four page-capped pairs cost 1,600 pages, 38 per cent of the whole pass, and returned
no answer at all.** That is the bound working rather than failing: without it one of them
would have spent the budget and starved the rest, which is what happened to HU/USDC in the
first pass. It is also the honest price of the cheap half's worst case.

---

## 2. The cheap half cost MORE than the expensive half, which inverts DEC-019 section 8.3

**2,517 pages for 25 pairs against 1,697 pages for 39.** Per pair that is 101 against 44,
so the half the record calls cheap costs more than twice the half it calls expensive.

The record's section 8.3 argues the opposite in one sentence: "A busy pair resolves on the
first day walked, and a pair that needs many days walked is quiet by definition and costs
almost nothing per day." The two properties are independent, and the threshold selects for
the pair class that has both.

**HU/USDC is the case that proved it and it cost a whole pass.** 22,210 trades a day, and
no genuine day in the first nineteen walked. In the 16:30 pass it ran from 16:43:47 to
16:57:00, consumed the rest of the 3,000 request budget alone, failed, and the forty pairs
behind it failed instantly on a spent budget. **One pair spent more than section 2 prices
for the entire under-threshold set**, which is 1,164 pages.

Why the two properties combine: a pair is over the threshold because it is BUSY, and a
pair filled from a liquidity pool has no genuine day at all under condition 4 of the
genuine-trade rules, which is section 3 below. The threshold supplies the first property
and the methodology supplies the second.

`-max-pages`, default 400, was added in response. It is recorded in DEC-019 section 9.8.

---

## 3. Condition 4 is why 29 pairs have no genuine trade, and it is the methodology working

`docs/methodology/07-supporting-metrics.md` section 1 gives the genuine-trade rule three
outcomes and not two. A liquidity-pool fill with no order-book trade inside the ±15 minute
comparison window is **Unevaluated**, never Genuine, because scoring it against an
hour-stale book is the failure the rule exists to avoid.

**A pair filled entirely from a pool therefore reports no last genuine trade however far
back it is walked.** GROG/USDC is the clean example: 30 whole days, 14 pages,
`trades_excluded_pct` **2.69 per cent**, and not one genuine trade. 97 per cent of its
volume is Unevaluated rather than excluded, which is why the exclusion share looks healthy
beside an empty answer.

This is the rule behaving as written. It is not a defect in the walk and it is not an
argument to widen the comparison window: `07` section 1 already measured what widening
does, moving 133 comparable August fills into Unevaluated.

**What is owed and is not this file's to write.** Whether `07-supporting-metrics.md`
should state the consequence for pool-dominated pairs, beside the USTRY figures it already
carries, is a change to a RED document and is Al's.

---

## 4. The anchor's own day is pure cost, and for a bursty pair it is the whole bill

**TGM/USDC spent all 400 pages without completing a single day.** Its row reads
`days_walked = 0, pages = 400`.

The walk discards the anchor's own day, because a partial day cannot be classified:
condition 5 reads the median of the trade's own UTC day, and DEC-019 section 8.2 shows the
error runs towards "traded more recently than it did". Those trades are still fetched. The
walk cannot seek past them, because seeking to a day boundary means turning a time into a
ledger sequence and `00-overview.md` section 2 rule 4 forbids exactly that.

Measured on the pair the same evening, paging `order=desc`:

| Page | Span covered |
|---|---|
| 1 | 2026-09-18T09:08:38Z to 16:46:13Z, seven and a half hours |
| 2 | 08:56:08Z to 09:08:38Z, twelve minutes |
| 3 | 08:43:23Z to 08:56:08Z, thirteen minutes |
| 4 | 08:35:08Z to 08:43:23Z, eight minutes |

A burst between roughly 08:00 and 09:10 put tens of thousands of trades inside the
anchor's own day. 400 pages is 80,000 records and none of them was usable.

**The consequence for the bound.** A page bound has to cover the anchor's partial day PLUS
one complete day, and a burst makes the first of those unknowable in advance. 400 was
chosen so the busiest pair in the set by the August count, XLM at 323 pages a day, could
complete its newest day; a burst defeats that reasoning. Whether the default moves is Al's,
and the row now says which of the two page-bound states it is in rather than reporting one
number for both.

---

## 5. The loop closes, and one asset proves it

**`bandConfidence` read `full` for the first time in this project's history**, on ACT/USDC
at 17:27 UTC. The engine has stored 17,663 metrics rows since 25 August 2026. 17,662 of
them read `partial`. This is the other one.

```
code                | ACT
holder_top1_pct     | 41.5338257989363115656336996
last_genuine_trade  | {"at": "2026-09-10T21:36:38Z", "ledgerSeq": 64368545}
trades_excluded_pct | 0.63193442695612583294716994
volume_to_supply    | {"d1": "0", "d7": "0", "d30": "0.0000015831455508177976463988"}
band                | CRITICAL
band_confidence     | full
```

**Why it took two commands and a scan.** `bandConfidence` drops to `partial` on any
unevaluated HIGH or CRITICAL flag, and the trade-derived half supplied three of them. The
sequence that produced the row above is `keel trades`, then `keel holders` for this asset,
then one `keel scan` round. Neither cache alone is enough: the volume-to-supply ratio
divides a numerator the trade pull measured by a denominator the holder pull measured, and
`domain.VolumeToSupply` declines rather than guessing when either is missing.

**What the scan round reported over the whole set**, which is the honest version of the
line above: `64 ok, 0 failed, 0 with holder figures, 55 with trade figures`. The trade half
reached 55 pairs. The holder half reached one, because this is a development database whose
holder cache was never filled; the production stack refreshes it daily under DEC-018. So
the remaining 54 pairs carry FR-10 and not FR-9, and they carry it for a reason that is
recorded in their rows rather than inferred.

**The band moved to CRITICAL in the same round and that is the engine working rather than a
regression.** A flag that can finally be evaluated can finally fire.

---

## 6. What is outstanding

**Nothing in the dataset.** The first two passes stored 55 of 64 and spent the 3,000
request budget before reaching the rest; the third pass, at 18:47 UTC once the hour had
cleared, finished the remaining 9 in 732 requests and stored no failure. The pass is
resumable as of the second run, which is why the third cost only the nine pairs it had
left rather than all sixty-four.

**Four pairs carry no FR-10 answer and will not get one from a larger day bound**, because
what stopped them was pages and not days. TGM/USDC is the extreme case in section 4: it
never completed a single day. Raising `-max-pages` for them is a deliberate operator
action rather than a default, and section 4 explains why the number cannot be chosen in
advance for a bursty pair.

**The three passes together sent about 6,700 requests inside two and a half hours** against
a stated budget of 3,000 an hour, and the first two of them sent about 6,000 inside one
hour, which is over NFR-6. That was not deliberate: each `go run` starts a fresh in-process
budget counter, and the second pass was started to recover from the first. It is recorded
here rather than left in a log, and the third pass waited for the hour to clear.

---

## 6b. The four page-capped pairs were retried, and three of them are unaffordable

Al asked for the four to be worked after the set was complete. HU/USDC was retried twice,
at 1,500 and 2,600 pages. **Neither reached thirty days**, and the measurement that came
out of it is worth more than the answer would have been:

| Bound | Days reached | Marginal cost of the days added |
|---|---|---|
| 400 | 11 | 36 pages a day |
| 1,500 | 20 | 122 pages a day |
| 2,600 | 24 | **275 pages a day** |

The walk restarts from the anchor every time, so these are three independent measurements
and 4,500 pages were spent on one pair without answering it. Cost per day rises with depth
because this pair's older days are busier than its recent ones.

Priced from what each pair actually reached, the four together need about **17,000 pages,
close to six hours of continuous requests**, which is two thirds of the figure DEC-019
uses to rule out running the expensive half over the whole set. Section 9.13 of that
record carries the table and what it suggests about the bound's shape.

**The retry also found a defect and a fix.** `-refresh` re-walked the pair and then threw
the result away, because the store refused a second row for one day. Migration 0010
corrects the reasoning 0009 was written on: a second walk of the same day can be DEEPER
rather than a repeat. The row is now appended, nothing is overwritten, and the reader takes
the deepest reading for the newest day rather than the newest row, because depth is monotone
for this walk while insertion order is not.

---

## 7. How to reproduce

```bash
docker compose up -d postgres
KEEL_MIGRATE_DSN="postgres://keel:keel_dev_only@localhost:5433/keel?sslmode=disable" \
  bash scripts/migrate.sh
go run ./cmd/keel assets -pairs configs/demonstration-set.json
go run ./cmd/keel trades                    # add -refresh to retake the same day

docker exec keel-postgres psql -U keel -d keel -c "
SELECT CASE WHEN last_genuine_at IS NOT NULL THEN 'has a last genuine trade'
            WHEN exhausted     THEN 'none, whole history seen'
            WHEN page_capped   THEN 'none, page bound'
            WHEN bound_reached THEN 'none, 30 day bound' END AS outcome,
       count(*), sum(pages) FROM trade_readings GROUP BY 1;"
```

The outcome column above is derived in the query rather than stored; `page_capped` is
`pages >= the bound in force`. The figures in section 1 were produced with the 400 page
bound of this run.
