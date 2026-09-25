# DEC-024: The February 2026 daily USTRY/USDC series is stored, one replay per day

**Status:** **ACCEPTED, 2026-09-25.** Decided by the project owner, who chose "option A"
when offered three ways to extend the historical path. Drafted by Claude; the choice is
the owner's, the transcription is not.
**Date drafted:** 2026-09-25
**Kind:** Scope. It names which ledgers may be stored. It changes no mechanism, no
formula, no threshold and no contract schema.
**Depends on:** DEC-022 (the mechanism: a reconstruction may be stored when it names its
gaps) and commit `212bb83` (`keel replay -pools-from-effects -trade-metrics`, which
reconstructs pool reserves and the trade half at the target).
**Extends:** DEC-023, whose section 6 says "this record authorises one ledger, not a
February series". This record is the separate decision that sentence asks for. DEC-023
is not edited; its amendment table points here.
**Zone:** `docs/decisions/` (YELLOW).

---

## 1. The decision

**Twenty-seven more ledgers of the USTRY/USDC pair are authorised for storage as
`offers-implied` rows**, one per UTC day of February 2026 except 22 February, whose
daily sample (61340172) DEC-023 already stored. Each is the daily sample ledger of the
repaired book series,
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.csv`:
the first trade at or after each UTC midnight.

| Day | Ledger | Day | Ledger | Day | Ledger |
|---|---|---|---|---|---|
| 01 | 61027032 | 10 | 61161968 | 19 | 61295329 |
| 02 | 61041997 | 11 | 61176868 | 20 | 61310263 |
| 03 | 61056831 | 12 | 61191677 | 21 | 61325141 |
| 04 | 61071606 | 13 | 61206579 | 23 | 61355036 |
| 05 | 61086706 | 14 | 61221451 | 24 | 61369919 |
| 06 | 61101452 | 15 | 61236743 | 25 | 61385497 |
| 07 | 61116508 | 16 | 61250942 | 26 | 61400035 |
| 08 | 61132067 | 17 | 61265749 | 27 | 61415216 |
| 09 | 61147341 | 18 | 61280403 | 28 | 61429800 |

With DEC-023's three, the stored set becomes **thirty ledgers**: a reading every day of
February, plus the control ledger and the incident ledger on 22 February.

## 2. Why now, and why this and not the cheaper route

**What it gives a reader.** `/history?source=offers-implied` for USTRY answers three
points today, all inside nine minutes. The report's central finding is that the book
read `LOW` on every daily sample of February and `CRITICAL` at the exploit's ledger.
Today a reader can check the second half of that sentence through the API and must take
the first half from a CSV. After this, both halves are served, and the dashboard's
"Offers implied" chart becomes a month.

**Why replay and not `keel bookseries`.** DEC-022 section 6 refused a persist path for
`bookseries` because it reconstructs no pool, so every row would be order book only and
would understate depth. That objection is about the route, not the series. Since
`212bb83`, `keel replay -pools-from-effects` reconstructs each pool's reserves at the
target from the pool's own effects, so each of these rows is a combined-depth result
like the three already stored. The cost is paid per ledger instead of once.

**Why not other assets.** A replay walks every account that ever traded the pair
backwards through its operations. USTRY/USDC is quiet enough for that to fit in about
45 minutes. XLM/USDC traded 1,934,524 times in 30 days, so the same walk does not fit
inside any budget public Horizon offers. Other assets' past ledgers need captive-core,
which the SOW excludes.

## 3. What each row carries, and how it differs from DEC-023's three

The three rows of 17 September were written before `212bb83`, from a hand-audited pool
file and with no trade half, so they carry no oracle window and six unevaluated flags.
The twenty-seven written under this record carry reconstructed pools and the trade half,
so they will generally be MORE complete than the three beside them. **That is expected
and must not be "fixed" by rewriting the three**: `SaveMetrics` is `ON CONFLICT DO
NOTHING` and decision 2 in `internal/store/store.go` forbids rewriting a stored result.
Whether to replace them is close-out item 6, still open, and not decided here.

## 4. The run, and the rehearsal that preceded it

Run by the owner on the production box, one ledger at a time, as RUNBOOK section 3.10
describes. About 45 minutes and about 1,000 Horizon requests per ledger on the 25
September rehearsal (section 6), so the series is roughly **20 hours** and must not run
in parallel with itself.

Parameters, per ledger `L`: the same walk bounds as DEC-023's runs
(`-lookahead 5000 -max-pages-per-account 60 -max-pages-per-offering-account 400`),
the walk floor and trade seek both at `L - 40000` (the 40,172-ledger gap DEC-023's runs
used, rounded), `-known-removals configs/known-removals.json`, and
`-pools-from-effects -trade-metrics -accept-incomplete -compute -persist`.

**Rehearsal, 25 September 2026.** Section 6 records one ledger replayed with exactly
these flags minus `-persist`, before any row was written.

## 5. What stays refused

No ledger outside the table in section 1 and DEC-023's three. No other pair. No
`bookseries` persist path. No change to DEC-002. A run that refuses with `crossed` or
`inflated` is a finding: the ledger is skipped and reported, never retried with fewer
flags.

## 6. Rehearsal result

Ledger 61147341, 9 February 2026, the first day the known removal applies. Run on
25 September 2026 with exactly section 4's flags minus `-persist`, from a workstation
against public Horizon. **No refusal: no `crossed`, no `inflated`.** It finished in
under 41 minutes.

| | Result |
|---|---|
| Walk | 86 accounts (78 from trades, 8 only from the live book), 637 trades, 522 offer operations applied, **1,023 Horizon requests** |
| Completeness | 6 walks truncated, 76 stopped at the floor, 0 failed, 0 unsizable, 2 offers named by trades but never seen. Incomplete, so `-accept-incomplete` is required and the row records every gap |
| Known removal | 1822775941 applied, the same one the repaired series applies from this day |
| Pool | 1 of 1 reconstructed from its own effects: 15.4791425 USTRY and 16.3389169 USDC, last effect at ledger 61069106 |
| Trade half | computed: last genuine trade at the target itself, 1 trade in the 15 minute window, genuine volume 0.0225665 USDC. Volume-to-supply is not computed at a past ledger, by design |
| Result | `P0` 1.056766 from the book, spread 0.135 percent, band `LOW` (partial), no flags, three flags unevaluated |

**It reproduces the repaired series.** `P0`, best bid 1.0560527, best ask 1.0574793,
the spread and the known removal are identical to that CSV's 9 February row. Depth at
±2 percent buy is 213,175.64 against the CSV's 213,175.47: the difference is the pool's
contribution, which the series (order book only) could not include. That is the
improvement section 2 claims, measured.

**One correction to section 4.** The cost is about **1,000 requests per ledger**, not
374. The 374 of 17 September was measured for 22 February over 65 accounts; this walk
reached 86. Why the request count is nearly three times higher is not measured here, and
the count should be read from each ledger's log rather than assumed. At
about 1,000 requests in 40 minutes the run uses about 1,500 an hour, inside the 3,000
budget but not beside the scan's own use of the same IP, so the time estimate of about
20 hours stands and the "one at a time" rule matters more than section 4 said.

**Also seen, and not a defect.** The book carries a 0.0000002 USTRY ask at price
2147483647 and a 429.4967294 bid at 1/2147483647. Those are the sentinel dust levels
close-out item 4 already names at 61340263, present here thirteen days earlier. Whether
resting dust counts towards `Reachable` is close-out item 5, still open.

**Conclusion: the series may run as RUNBOOK section 3.10 writes it.**

## 7. Amendment history

This record is append-only from its first commit.

| Date | Amendment |
| --- | --- |
| 25 September 2026 | Record created. The owner's decision in section 1 |
| 25 September 2026 | Section 6 filled from the rehearsal of 61147341: no refusal, matches the repaired series, about 1,000 requests per ledger rather than 374 |
