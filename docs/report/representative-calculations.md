# Representative calculation review

Prepared 12 September 2026. **Awaiting human review.** These assistant-prepared
calculations use a separate Python `Fraction` implementation, with no backend
imports. They supplement, and do not satisfy or replace, the human-authored
Layer 1 records in `testdata/manual` or the locked golden fixtures.

The [exact-rational sheet](representative-calculations.json) was generated before
the backend comparison test was run. Its [generator](representative-calculations.py)
uses the SDEX formulas in methodology [03](../methodology/03-reference-price.md),
[04](../methodology/04-depth.md), and [05](../methodology/05-manipulation-cost.md).
All notionals below are USDC and prices USDC/USTRY. The scenarios use the full pair
identity in the [report](blend-february-2026.md) solely for controlled tests.
Their ledger/time fields are fixture metadata, not a claim of historical observation.

## Normal synthetic book

Bid: 1,000,000 USTRY at 99/100. Ask: 1,000,000 USTRY at 101/100.
No pools by construction. "Normal" describes this balanced, narrow-spread input,
not a promised risk band or a historical classification.

`P0 = (0.99 + 1.01) / 2 = 1`; spread `= (1.01 - 0.99) / 1 * 100 = 2%`.
At each of 2%, 5%, and 10%, both levels fall inside their side's price boundary.
Buy depth is `1.01 * 1000000 = 1010000`; sell depth is
`0.99 * 1000000 = 990000`.

For upward deltas 0.5, 1, 10, and 100, cost is 1,010,000 but reachability is
**false**: the only ask is below every target. Cost must not be presented without
that reachability result.

## Broken synthetic book

The existing fixture's book is reused with the real pool deliberately omitted.
This is a **synthetic book-only scenario**, not the actual February combined
market; `internal/conformance.BookOnlySnapshot` documents the same distinction.
Its original input provenance is in the [existing fixture](../../testdata/fixtures/ustry_pre_exploit.md).

Bid: 0.0001000 USTRY at 1057/1000. Ask: 1.2185312 USTRY at 266843207/2500000.

`P0 = (1.057 + 106.7372828) / 2 = 53.8971414`.
Spread is approximately 196.0777140585%. At 2%, 5%, and 10%, neither level falls
within its boundary, so both depths are exactly zero for this constructed input.

| Upward delta | Cost | Reachable |
|---|---|---|
| 0.5 | 0 | true |
| 1 | 130.06270929502336 | false |
| 10 | 130.06270929502336 | false |
| 100 | 130.06270929502336 | false |

The cost follows `106.7372828 * 1.2185312` when the ask is below the target.
At delta 0.5 the ask is above the target, so no cheaper ask is consumed. This
illustrates the specified cost model; it does not assert that a real transaction
can execute for zero stroops or that pool-inclusive historical cost was zero.

## Incomplete historical evidence

Two variants remove evidence from a controlled replay: an unresolved offer ID,
and unknown (`nil`) pool coverage. Expected outcome: **no stored result**, with
an error before any metrics write. The comparison test verifies both variants.
This is an availability check, not a numerical calculation on guessed reserves.

The separate existing API tests verify HTTP 503 while historical serving is off
and HTTP 404 for an unreplayed ledger while it is on. They do not prove the state
of a deployed service. No new frontend screen is part of these tests.

## Comparison result and limits

`TestReportRepresentativeCalculations` passed on 12 September 2026 against backend
`1.0.8-draft`. Both mid prices, spreads, all six depth values per case, and four
order-book cost/reachability pairs per case matched the rational expectations to
an absolute tolerance of 0.0000001. Both incomplete variants refused persistence.
The tolerance is for this comparison only, not a new methodology constant.

Methodology documents remain labeled `1.1.0-draft`; A7 reconciliation is pending.
These cases do not validate AMM routing, oracle resistance, holder/wash metrics,
collateral, flags, bands, or the completeness of real February state. Do not infer
coverage of those areas from the passing check.

Final verification for this report update: `go test ./... -count=1`, `go vet ./...`,
the explicit API 503/404 tests, both Python reproduction checks, capture-manifest
hashes, and local document links passed. The full Go run in this report phase did
not set database DSNs, so database integration tests were skipped; the earlier
separate disposable-database run is recorded in the engineering evidence notes.
An independent reviewer found no concrete issues in the report and comparison
artifacts. Human approval and race-enabled verification remain outstanding.

```sh
python docs/report/representative-calculations.py --check
go test ./cmd/keel -run TestReportRepresentativeCalculations -count=1 -v
```

Independent rational arithmetic makes inputs and rounding inspectable without
calling the backend to produce expected answers. Generating expectations from
backend output was rejected because that would only reproduce its mistakes.
Keeping these checks outside the locked manual records preserves the distinction
between assistant preparation and the owner's independent human acceptance.
