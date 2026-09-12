# USTRY/USDC: February 2026 historical evidence report

**Status:** review draft, 12 September 2026. Limited-evidence scope approved by the
project owner; this report and its external message still require final approval.
No real historical risk rows have been loaded by this work.

Keel measures executable liquidity and the volume a quoted price can support.
This investigation cannot establish a reliable daily risk series or a first-warning
date before the 22 February incident. It does establish specific offer cancellations
and identifies why the available replay output is not suitable for that claim.

## Scope and identity

The requested window is 1 February 2026 00:00 UTC through 1 March 2026 00:00 UTC,
exclusive. Available diagnostics contain 28 daily samples and two additional
control samples; they are not continuous observations. Exact sample ledgers and
times appear in the [diagnostic appendix](../evidences/track-b-2026-09-12/daily-output-review.md).

| Asset | Issuer | Type |
|---|---|---|
| USTRY | `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` | `credit_alphanum12` |
| USDC | `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` | `credit_alphanum4` |

Risk notionals use USDC; prices use USDC per USTRY. The [pool coverage inventory](pool-coverage.md)
identifies the one known pair pool and the periods for which reserve evidence is
missing or conditional. It precedes any historical load and does not certify that
the pool list is exhaustive.

## What the evidence supports

| Finding | Evidence class | Confidence and limit |
|---|---|---|
| Six USTRY/USDC offers were canceled at ledger 61340261, closed 22 February 00:10:09 UTC | Direct operation observations; reconstructed offer-membership check | Successful transaction results identify the cancellations. This does not establish remaining quantities or a complete market snapshot |
| The control ledger 61340262 closed at 00:10:15 UTC; the incident ledger is 61340263 at 00:10:21 UTC | Ledger timing observations and existing incident fixture | Distinguish pre-operation state from end-of-ledger state; do not relabel an intra-ledger fixture as an atomic historical snapshot |
| A subtraction-only replay retained a one-stroop residual of offer 1822775941 after its last recorded fill at ledger 61143619 | Reconstructed finding with protocol-source analysis | Protocol adjustment explains a route to removal; no decoded historical transaction-meta proof of the exact deletion point |
| A named pool has a recorded reserve candidate of 15.4791416 USTRY and 16.3389179 USDC following ledger 61172481 | Transcript observation; later continuity is reconstructed | Limited confidence, conditional on retained transcript assertions. This is not a daily reserve series |
| Daily depth, collateral, manipulation cost, risk band, and first-warning date | Unavailable as accepted historical findings | Incomplete replay, phantom-offer risk, and missing aligned pool coverage prevent acceptance |

The cancellation transaction is
`8f8ae8499e03f42343744a3278ac03360fa328790f1e3af990637e87fef0467e`.
Its six pair-specific operations cancel offers `1824767559` through `1824767564`.
Other cancellations in the same ledger concern different pairs and are excluded.
The [engineering evidence](../evidences/track-b-2026-09-12/README.md) gives exact
operation IDs, account identity, raw sources, and the limits of the folding test.

The six cancellations are consistent with the observed level-count transition,
but they do not validate every other level in the reconstructed book. A real
withdrawal and a reconstruction defect can coexist. The report therefore makes
no claim that a warning would have preceded the event by a particular duration.

## Why daily outputs are withheld

Every row in the available cap400 export reports an incomplete fold. All 28 daily
rows show `LOW/partial` with no triggered flags; the two extra control rows show
`CRITICAL/partial`. Those labels describe disputed program output, not accepted
historical market findings. A prior shallower export gives different results and
must not be blended into this series to manufacture a warning narrative.

The [row-by-row appendix](../evidences/track-b-2026-09-12/daily-output-review.md)
retains exact input provenance, ledger/time, confidence, reachability, and displayed
rounding rules. Seven daily books are crossed; the earlier phantom investigation
also implicates uncrossed samples from 9 February onward. Samples from 1 to 8
February are outside that particular interval but still incomplete.

These outputs are **not certified lower bounds**. An omitted genuine level might
reduce depth, while a retained phantom can increase it or move the reference price.
There is no established direction of error for the disputed daily calculations.

## Calculations and result labels

The [prepared calculation review](representative-calculations.md) compares an
independent exact-rational implementation with the current backend. It covers a
normal synthetic market, a broken synthetic book, and incomplete-input rejection.
Those controlled calculations are not substituted for February market results.

For this report:

- **Calculated:** arithmetic on explicitly stated inputs, including controlled
  scenarios. A matching calculation does not prove historical input completeness.
- **Reconstructed:** historical state inferred from operations or effects; identify
  the covered venue, time, and missing state before interpreting the result.
- **Lower-bound:** use only where the source and methodology justify that direction
  of uncertainty. No daily risk result in this report receives this designation.
- **Unavailable:** evidence cannot support the requested result. No invented zero,
  risk band, confidence upgrade, or warning date is supplied.

These are report evidence labels, not additions to the backend's enums. Source
semantics remain governed by [data sources](../methodology/01-data-sources.md),
with price, depth, and cost definitions in [reference price](../methodology/03-reference-price.md),
[depth](../methodology/04-depth.md), and [manipulation cost](../methodology/05-manipulation-cost.md).
For example, the depth definition states: "A level that crosses the boundary is
discarded entirely". The calculations preserve that inclusion rule.

The checked backend identifies itself as **1.0.8-draft**, while the methodology
documents identify **1.1.0-draft**. This report preserves both labels and does not
claim version alignment. The owner is coordinating A7; after alignment, rerun the
comparisons and retain each historical computation's actual version. Passing the
small selected cases is not validation of all methodology definitions or thresholds.

## Application behavior and provenance

The current API already supports `503 HISTORICAL_UNAVAILABLE` when historical
serving is disabled and `404 LEDGER_NOT_AVAILABLE` when an enabled historical
lookup has no accepted row. Keep unsupported historical results on those paths.
An incomplete snapshot must not be turned into the HTTP 200 no-executable-price
finding: that finding requires an actual evaluated snapshot.

Replay persistence now requires explicit ledger-aligned pool input and rejects
known acquisition gaps, crossed books, and unknown pool coverage. An empty pool
array asserts audited absence; it is not a missing-data substitute. The API has no
per-pool coverage field that could make these disputed combined-market results
truthful merely by attaching `partial`. No new API schema or frontend behavior is
introduced by this report.

Passing structural validation is not proof of complete discovery or corrected
protocol settlement. Consequently no February row is loaded on the strength of
these guards alone. Existing stored rows elsewhere were not audited by this work.

The new [capture manifest](../evidences/track-b-2026-09-12/manifest.json) records raw
response URLs, capture times, and SHA-256 hashes. Raw capture time is distinct from
ledger close time. Older pool transcripts retain their weaker provenance; current
pool balances are not carried backwards into February.

## Reproduce and review

From the backend root:

```sh
python docs/evidences/track-b-2026-09-12/analyze.py --check
python docs/report/representative-calculations.py --check
go test ./cmd/keel -run TestReportRepresentativeCalculations -count=1 -v
go test ./internal/api -run 'TestAHistoricalRequestIs503WhileHubbleIsDeferred|TestAnUnreplayedLedgerIs404AndNot500' -count=1
go test ./internal/horizon -count=1
```

These commands use local evidence and controlled test inputs, not a historical
database load. See the [engineering notes](../evidences/track-b-2026-09-12/README.md)
for the separately verified disposable-Postgres path and environment limitations.
Race-enabled verification remains outstanding because this Windows environment
has CGO disabled. Final human review covers the prepared arithmetic, this report,
and the [unsent external message](client-message-draft.md), with A7 alignment
explicitly tracked before representing the report as a version-aligned deliverable.

This rewrite separates observable events from computed scenarios so every claim
has an inspectable scope. Retaining the old apparent daily warning narrative was
rejected because the deeper replay contradicts it and neither run is accepted.
Withholding unsupported numbers lets the report finish without pretending the
historical reconstruction is complete.
