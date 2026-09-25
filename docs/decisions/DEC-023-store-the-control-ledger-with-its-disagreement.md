# DEC-023: Ledger 61340262 is stored, and the row carries its disagreement with the fixture rather than waiting for it to be settled

**Status:** **ACCEPTED on the substance, 2026-09-17.** Al chose "store with warnings"
over settling the fixture question first and over hiding the four fields. Recorded by
Claude; the choice is Al's, the transcription is not. **Three details in section 7 are
Claude's fill-in and need Al's eye before the run**, and they are marked as such rather
than presented as decided.
**Date drafted:** 2026-09-17
**Kind:** Scope. It names which ledger may be stored. It decides no mechanism, changes
no formula and settles no disputed number.
**Depends on:** DEC-022, which is the mechanism. Nothing here can happen until that
code exists, because the reconstruction of this ledger cannot pass the old gate.
**Does NOT settle:** `docs/report/blend-february-2026.md` section 5.4. That question
belongs to the golden fixture, the fixture is RED, and storing a row is not ratifying
it.
**Zone:** `docs/decisions/` (YELLOW).

---

## 1. The decision

**Ledger 61340262 of the USTRY/USDC pair is authorised for storage as an
`offers-implied` row in production.**

It is stored under DEC-022's mechanism, with `-accept-incomplete`, its walk counters in
the `reconstruction` field, and its pool coverage from DEC-013. **In addition it carries
warnings naming the four quantities on which the reconstruction and the hand-computed
fixture disagree, and it carries the fixture's values for those four.**

The alternative rejected was settling section 5.4 first. That question is whether a
dust offer at the sentinel price is a second silent removal or a gap in the hand
computation, it belongs to a RED file, and it has been open since 12 September. Holding
the only historical row this deployment can serve behind a question nobody is working
on would keep `?ledger=` at 503 indefinitely, which is the outcome DEC-022 section 3
already showed is not a high standard but a closed door.

The second alternative rejected was suppressing the four fields on this row. It would
have put a per-row exception into the API, and an API that hides a number on one row
and shows it on every other is harder to trust than one that shows it with a warning.

## 2. Why this ledger and not a safer one

The February series holds thirty rows. Twenty-eight of them are daily samples and every
one reads `LOW` at a price around 1.056 to 1.058. The two that read `CRITICAL` are
61340262 and 61340263, eleven seconds apart.

| Ledger | Closed | `p0` | Band |
| --- | --- | --- | --- |
| 61340172 | 2026-02-22T00:01:24Z | 1.0579 | `LOW` |
| **61340262** | 2026-02-22T00:10:15Z | **53.8971414** | **`CRITICAL`** |
| 61340263 | 2026-02-22T00:10:21Z | 53.8971414 | `CRITICAL` |
| 61355036 | 2026-02-23T00:00:33Z | 1.0580 | `LOW` |

Source: `docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.csv`.

A daily row is undisputed and shows a healthy book. It proves nothing, because what
made USTRY dangerous was never visible at daily resolution, which is the finding
`docs/evidences/2026-08-26-ustry-february-trades-implied.md` reached first and the
report repeats. **Choosing the safe ledger and choosing a ledger worth serving are the
same decision, and they point in opposite directions.** This record takes the second.

## 3. What is already settled and needs no work

1. **The gate accepts this row's shape.** The two conditions DEC-022 leaves refused
   with no override both read clean: `crossed` is `false` on all thirty rows of the
   repaired series, and its sidecar gives `earliest_offer_operation_ledger` 60988098
   above `trade_window_from_ledger` 60987032, so `MayBeInflated` is false. What fails
   today is `walks_truncated`, `walks_stopped_at_floor` and `walks_failed`, and those
   are exactly the three DEC-022 admits.
2. **Pool coverage exists.** DEC-013 section 1 gives pool
   `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb`, 16.3389179 USDC
   and 15.4791416 USTRY at 30 bps, read from the pool's own effects. Section 2 gives
   the quiet window, last effect 2026-02-10T16:59:35Z and next 2026-02-22T22:08:33Z,
   and 61340262 falls inside it. So the same audited reserves apply and no arithmetic
   reconstruction is needed. That is the `-pool-snapshots` file, and it is a
   transcription rather than a measurement.

## 4. What the row will say about itself

Two groups of warnings, and they are different in kind.

**The walk's own gaps**, from DEC-022, one per nonzero counter. These say the book may
be too thin.

**The disagreement with the fixture**, which is this record's addition:

| Quantity | Fixture, by hand | This run |
| --- | --- | --- |
| `maxReachablePrice` | 106.7372828 | 2147483647 |
| `costToMaxReachablePrice` | 0 | 124.715139 |
| `Reachable` at δ = 1, 10, 100 | false | true |
| Asks on the book | 1 | 2 |

**`maxReachablePrice` needs its own warning and not a shared one.** 2147483647 is a
sentinel price carried by a dust offer, not a price anything can reach. A dashboard
that renders it as a number renders a reachable price of 2.1 billion USDC. The warning
must say the word sentinel.

**What the two sides AGREE on is the larger half and the record states it, so that the
disagreement is not read as a failed reconstruction.** Section 5.4 of the report is
explicit: the repaired run reproduces `best_bid`, `best_ask`, `P0`, `spreadPct`, the
zero depth ladder, all four flags and the `CRITICAL` band exactly. The case this row
exists to show is intact. What disagrees is four quantities about an upper bound.

### 4.1 THE PARAGRAPH ABOVE IS FALSE FOR THE RUN THIS RECORD AUTHORISES, measured 17 September 2026

**This subsection AMENDS and decides nothing.** It records a rehearsal run and hands
the consequence back, because the consequence is a claim about what a number means and
those are Al's.

A rehearsal of step 3 of section 5 was run against a disposable local database on
17 September 2026. Evidence, both files kept:
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-replay-61340262-with-pool-2026-09-17.log`
and its sidecar. The gate behaved exactly as DEC-022 designed: not crossed, not
inflated, and 4 truncated, 55 stopped at the floor, 0 failed, 28 missing offers, every
one of them written into the row's `reconstruction` field and its warnings.

**And the row does not disagree with the fixture in four quantities. It disagrees in
six, and one of them is the headline price.**

| Quantity | Fixture and report | This run |
| --- | --- | --- |
| `P0` | 53.8971414 | **1.0555441846982006** |
| `priceSource` | `book` | **`pool`** |
| `spreadPct` | 196.08 | **10011.9241176** |
| depth at 2 percent | zero | **0.16306951...** |
| flags | `ZERO_DEPTH_2PCT` among four | **`PRICE_SOURCE_CONFLICT`** in its place |
| `band` | `CRITICAL` | `CRITICAL` |

**THE CAUSE IS NOT THE RECONSTRUCTION AND NOT THIS RECORD. It is the pool that section
3 of this record calls settled.** 53.8971414 is the mid of a book with a bid at 1.057
and an ask at 106.7372828, which is what the ladder returns when NO pool is present.
The fixture records `Pools: []` although a pool held reserves at that ledger, which is
the whole subject of DEC-006. Since methodology 1.0.3 the reference price compares the
book mid against the pool spot and takes the pool when they diverge past the threshold.
So supplying DEC-013's reserves, which section 5 step 2 of this record REQUIRES, moves
`P0` to the pool spot by the rule rather than by accident.

**DEC-006 section 8 item 3 predicted every one of these on 27 August 2026** and closed
with "Nothing is recomputed here and no replacement value is offered, deliberately". It
listed the target price column, every depth figure, `spreadPct`, the `ZERO_DEPTH_2PCT`
argument, `P0`, `priceSource` and `PRICE_SOURCE_CONFLICT`. All of them moved, in the
direction it named. This run is the replacement value that record declined to compute,
arrived at sideways while rehearsing something else. The pool spot DEC-006 section 1
records is 1.0555441847; this run computed 1.0555441846982006 from DEC-013's reserves,
agreeing to ten digits.

**What this does to section 1 of this record, stated plainly.** Al decided to store
this ledger with its disagreement named in warnings, on the understanding in section 4
that the disagreement was four quantities about an upper bound and that `P0`, the
spread, the depth ladder and the band all agreed. Three of those four now do not agree.
A reader of the stored row would see a USTRY price of 1.06 where the report shows 53.90
and the fixture shows 53.90, on the same ledger, with nothing in the row explaining
which is which.

**Three readings, and choosing between them is Al's.**

1. **The row is right and the fixture is stale.** DEC-006 exists because the fixture
   omits a pool that existed, and 1.0.3 was written to fix exactly the failure that
   omission caused. Under this reading the row is stored and its warnings must name the
   fixture's 53.8971414 and say it is the no-pool answer.
2. **The row is right and the REPORT must move first.** `docs/report/` shows 53.8971414
   to a client. Publishing 1.06 for the same ledger through the API, while the report
   shows 53.90, puts two numbers for one ledger in front of the same reader. The report
   is YELLOW and what a number MEANS in it is Al's.
3. **Stop and settle DEC-006 first.** The fixture is RED, the recomputation is Al's
   under DEC-006 section 8 item 2, and storing a row that contradicts the fixture on the
   headline price before that is settled is a bigger step than the one section 1 took.

**No further ledger was run.** Section 5 asks for three walks; one was run and the other
two, about ninety minutes and 1,200 Horizon requests, are held until this is answered,
because all three would carry the same divergence.

### 4.2 IT WAS RUN ANYWAY, and this records that rather than arguing with it

**Al executed sections 5 and the RUNBOOK's 3.9 on 17 September 2026, without answering
4.1 first.** That is Al's call to make and this subsection is a record, not a dispute.
Measured against the live service the same day:

| Ledger | Closed | `midPrice` | `priceSource` | `spreadPct` | Band |
| --- | --- | --- | --- | --- | --- |
| 61340172 | 00:01:24Z | 1.05790300000000005 | `book` | 0.09 | `LOW`, no flags |
| 61340262 | 00:10:15Z | 1.0555441846982006 | `pool` | 10011.92 | `CRITICAL`, four flags |
| 61340263 | 00:10:21Z | 1.0555441846982006 | `pool` | 10011.92 | `CRITICAL`, four flags |

`historicalAvailable` is `true`, an unreplayed ledger answers 404
`LEDGER_NOT_AVAILABLE`, and `/v1/assets`, `/depth` and `/history` all still answer 200,
so the migration preceded the deploy as section 3.5 of the RUNBOOK requires.

**The walk reproduced exactly.** The production run reported truncated 4, stoppedAtFloor
55, failed 0, unsizable 0, missingOffers 28 over 65 accounts, which is the rehearsal's
result to the digit, on a different machine on a different day. That is NFR-9 holding
where it is hardest to arrange.

**61340172 is the row 4.1 could not predict and it strengthens the case.** It takes the
BOOK branch, because there the book mid and the pool spot diverge by 0.22 percent,
under the threshold. So the same rule that reads `CRITICAL` at 00:10:15 reads `LOW` nine
minutes earlier, and the contrast is produced by the market rather than by a change of
method.

**WHAT 4.1 ASKED FOR IS NOW UNFIXABLE IN PLACE, AND THAT IS THE ONE THING TO CARRY
FORWARD.** The three rows carry five warnings each: four about the walk's own gaps and
one about `maxReachablePrice`. **None of them mentions the fixture, the report, or
53.8971414.** A reader of `?ledger=61340262` sees 1.06 and a reader of
`docs/report/blend-february-2026.md` sees 53.90 for the same ledger, with nothing
joining them.

It cannot be repaired by re-running. `SaveMetrics` writes `ON CONFLICT ... DO NOTHING`
and decision 2 in `internal/store/store.go` forbids rewriting a stored result, which is
why the replay prints "existing row retained unchanged; a different reconstruction
requires investigation, not an overwrite". Adding a warning to these rows means deleting
them from production first, and that is a deliberate exception to a rule the store
exists to enforce.

Three ways to close it, and the choice is Al's:

1. **Put the bridge where the reader is.** One paragraph in `docs/report/` and one line
   on the dashboard's case study page, saying 53.8971414 is the no-pool answer the
   fixture carries and 1.0555441847 is the answer with DEC-013's pool. Cheapest, touches
   no stored row, and breaks no rule.
2. **Delete the three rows and re-run with the warning added.** Honest in the response
   itself and pays for it with an explicit exception to the never-overwrite rule.
3. **Settle DEC-006 first and then decide.** The fixture is RED and the recomputation is
   Al's under DEC-006 section 8 item 2.

Claude recommends 1. The disagreement is between a fixture and a methodology version,
which is a thing to explain in prose, and the API row is already honest about
everything the API itself can know.

## 5. Order of operations, and none of it is optional

1. DEC-022's code lands and its section 7 tests pass.
2. The `-pool-snapshots` file is written from DEC-013 and kept with its evidence.
3. `keel replay` runs at 61340262 with `-accept-incomplete` and
   `-known-removals configs/known-removals.json`. Read the diagnostics before storing
   anything; a run whose counters differ materially from the 5 September run is a
   finding, not a formality.
4. The row is written to the production database. **That step is Al's.** Claude never
   holds a credential to that box, for the reason `scripts/deploy/` splits PREPARE from
   APPLY.
5. `-historical` is added to `keel-serve`'s command in `docker-compose.prod.yml` and
   the service restarted. **Rows first, flag second.** The reverse order turns a
   truthful 503 into a 404 that blames the ledger, which RUNBOOK section 4 already
   records.

After step 5, `?ledger=61340262` answers 200 and every other ledger answers 404
`LEDGER_NOT_AVAILABLE`. That is the expected end state, not a defect.

## 6. What stays refused

Everything DEC-022 section 6 refuses stays refused except the one ledger named here.
No `keel bookseries` persist path, no second data source at that endpoint, no change to
DEC-002. This record authorises one ledger, not a February series.

## 7. Three things Claude filled in that are Al's to confirm

**Items 1 and 2 were confirmed by Al on 2026-09-17 and are no longer open.** The run
list is three ledgers. Item 3 stays open and is still Al's.

1. **CONFIRMED. 61340263 joins it.** The incident ledger six seconds later carries the
   same dispute and the same pool coverage, and a page showing the control without the
   incident shows half the story.
2. **CONFIRMED. 61340172 joins them.** The daily sample nine minutes earlier, reading
   `LOW` at 1.0579, undisputed. The page can now show healthy, then control, then
   incident, across nine minutes.

So the authorised set is **61340172, 61340262 and 61340263**, and section 1 is read as
naming all three. Three replay runs, not one. The pool evidence in section 3 covers all
three without further work: DEC-013's quiet window runs from 2026-02-10T16:59:35Z to
2026-02-22T22:08:33Z and every one of the three falls inside it.

Two consequences of taking all three rather than one, recorded so they are not
discovered later. **61340172 is undisputed**, so it carries the walk's own warnings and
none of the fixture disagreement in section 4; the three rows will not look alike and
should not be made to. **And the cost triples**: the bounded run over this shape took
374 requests and 2841 seconds, so three runs is roughly 1,100 requests against a 3,000
per hour budget, which fits in one sitting but not beside anything else that reads
Horizon.

3. **STILL OPEN. One sentence in `docs/report/` is now in tension with this decision.** Section 5.4
   closes with the instruction that the disputed columns "must not be quoted for the
   two control rows until this is settled", and serving them through a public API is a
   form of quoting even with a warning attached. The amendment is one sentence saying
   the API may serve them when the row names the disagreement. **That is a claim about
   what a number means, so it is Al's under the zone map, and this record does not make
   it.** Until it is made, the report and this decision disagree in writing, which is
   better than one of them being changed quietly.

## 8. Amendment history

| Date | Amendment |
| --- | --- |
| 25 September 2026 | Section added. Section 6's "this record authorises one ledger, not a February series" now has the separate decision it asked for: DEC-024 authorises the other twenty-seven daily samples of February 2026. Nothing above is edited, and the three rows this record stored are not rewritten |
