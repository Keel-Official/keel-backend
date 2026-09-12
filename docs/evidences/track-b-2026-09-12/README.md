# Track B: reconstruction findings and reproducible checks

Read on 12 September 2026. This is engineering evidence, not an accepted backtest
conclusion or a methodology amendment. The backend checkout used for this work was
`07c815b`. No monthly reconstruction was rerun and no disputed historical row was
loaded into an application database.

Pair throughout: USTRY, issuer
`GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`, against USDC, issuer
`GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`.
Raw operation observations have no methodology version: no risk engine produced
them. The CSV observations retain their original `1.0.8-draft` / `offers-implied`
labels. `manifest.json` records capture URLs and SHA-256 hashes.

## 1. B1: the residual is characterized, not repaired

The cap400 input has 30 rows: 28 daily samples and two additional control ledgers.
All 30 say `fold_complete=false` and report three missing offer IDs. Its sidecar
reports 36 truncated account walks, five failed walks and 167 walks stopped at the
floor. Those counters belong to the shared acquisition; the capture does not
identify the three missing IDs or attribute each walk failure to particular rows.
An exact per-row attribution of those acquisition gaps cannot be recovered from
this CSV alone.

Seven daily rows are crossed, 22 to 28 February inclusive. The earlier
[phantom-ask investigation](../2026-09-12-crossed-book-ustry-february.md) also
implicates every sample after 8 February, including uncrossed samples. The earlier
days are outside this particular phantom interval, not verified correct.

This run is materially different from the shallower run discussed in the original
report: **all 28 daily cap400 rows say LOW/partial and have no triggered flags**.
Only the two added control rows say CRITICAL/partial. These are facts about a
disputed program output, not evidence that the asset was safe or that Keel warned.
The [generated appendix](daily-output-review.md) lists every row, labels the known
problem, preserves reachability/confidence, and reports control flags separately.

### The subtraction-only fold misses protocol adjustment

The fresh `dust-owner-operations.json` records five successful updates of offer
`1822775941` at ledger `61143618`. The final update is operation
`262609839670583297`, amount `0.0982607` USTRY at ratio
`1981860307/1874295956`. `dust-last-fill-operation.json` records the next ledger's
path payment, operation `262609843965321217`; the existing pair trade CSV records
its fill of `0.0982606` USTRY against this offer.

Keel's `consume` subtracts those amounts and retains `0.0000001`. That subtraction
is exact, but it is not a complete implementation of Stellar offer settlement.

In [Stellar Core v25.0.0 OfferExchange.cpp](https://github.com/stellar/stellar-core/blob/v25.0.0/src/transactions/OfferExchange.cpp),
`crossOfferV10` adjusts a surviving offer after a fill and deletes a zero adjusted
amount. `adjustOffer` applies normal rounding and a price-error bound. For a
one-stroop remainder at this ratio, exchanging one stroop for one stroop fails that
bound: `100 * abs(1981860307 - 1874295956) = 10756435100`, greater than
`1981860307`. This explains a protocol path by which exact subtraction can leave
a phantom. The fetched source reference and hash are in `protocol-source.json`.
`dust-ledger.json` confirms protocol version 25 at ledger 61143619.

**Limit:** this is source-based causal analysis, not a decoded historical
transaction-meta proof. The captured Horizon transaction contains `result_xdr`
but no `result_meta_xdr`. The earlier document's six-minute removal interval is
an observational upper bound; it does not prove that the remainder survived the
last fill itself. Do not turn that interval into a precisely dated deletion.

No fixed dust cutoff, deleted offer ID or revised financial threshold was added
to the engine. A general repair must cover protocol rounding, seller limits,
path-payment behavior and historical state coverage. Matching only this one
offer's expected disappearance would not establish correctness for the month.

## 2. B2: six real cancellations explain the offer-count transition

The successful transaction
`8f8ae8499e03f42343744a3278ac03360fa328790f1e3af990637e87fef0467e`
at ledger **61340261**, closed **2026-02-22T00:10:09Z**, cancels these six
USTRY/USDC offers. The source is `transition-20-operations.json`, with the joined
transaction result for every operation.

| Operation | Offer | Selling | Buying |
|---|---|---|---|
| 263454414923366401 | 1824767559 | USDC | USTRY |
| 263454414923366402 | 1824767560 | USTRY | USDC |
| 263454414923366403 | 1824767561 | USDC | USTRY |
| 263454414923366404 | 1824767562 | USTRY | USDC |
| 263454414923366405 | 1824767563 | USDC | USTRY |
| 263454414923366406 | 1824767564 | USTRY | USDC |

The submitting account is
`GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN`.
The preceding account-operation capture contains the creates of these offers.
The existing trade CSV has 19 fills against `1824767559`, from ledger `61337700`
through `61340224`. The sample `61340172` is before that final fill and before
the cancellation. The last pre-sample account operation in the fresh backward
capture is at 2026-02-21T19:58:18Z; the forward capture starts with the cancellations.

An offline test decodes the actual operation results through the existing decoder
and folds them before and after the cancellation. All six IDs exist before and
are absent after; the creates and cancels need no trade-amount estimate to prove
that lifecycle. The six cancellations are consistent with the CSV's change from
23 to 17 reported levels/offers; the test does not certify the full book. Other cancellations in that ledger concern TESOURO and
ZUSD and are excluded by full pair identity.

`control-ledger.json` records ledger `61340262` at **00:10:15Z**. The incident
trade CSV records ledger `61340263` at **00:10:21Z**, twelve seconds after the
cancellation ledger. These timestamps are observed, not estimated from ledger
spacing.

**Resolved:** the six offers were actually cancelled. The explanation that they
disappeared *only because the fold lost them* is ruled out for those six IDs.
**Not resolved:** the exact depth change, the rest of either book, and the
validity of the reported LOW-to-CRITICAL transition. Both a real withdrawal and
a reconstruction defect are present. B2's two explanations are therefore not
mutually exclusive for every metric in the row. A daily-warning claim still
requires valid books, not just a cancellation timestamp.

## 3. Reproduction

From the backend root:

```sh
python docs/evidences/track-b-2026-09-12/analyze.py --check
go test ./internal/horizon -run TestFebruary22SixOffersAreCancelledAtLedger61340261 -count=1 -v
```

The test reports **posted amounts** from operation results. It deliberately does
not present them as remaining amounts after all intervening fills.

Refetch a capture using the URL in `manifest.json` into a new file; do not
overwrite the saved bytes. Horizon links may change their spelling, so a changed
hash is a prompt to inspect the response, not proof that ledger history changed.

## 4. B4: opt-in persistence, gated by evidence

`keel replay` now accepts `-persist`, `-dsn` and `-pool-snapshots`.
Ordinary replay remains database-free. The persistence path requires an already
declared pair, the actual ledger close time, no detected replay gaps, and explicit
pool coverage at the identical pair and ledger. It calls existing `SaveMetrics`;
no store, API, domain or migration file was changed.

```sh
go run ./cmd/keel replay -pairs scripts/record-pairs.example.json \
  -ledger 61340262 -persist -pool-snapshots /path/to/audited-pool-snapshots.json
```

This command is a usage example, not a claim that the current February data meets
the gate. The pool input is an array in the existing `domain.Snapshot` JSON shape.
Each entry must identify `Base`, `Quote`, `LedgerSeq`, `LedgerClosedAt`, `Source`
(`offers-implied`) and `Pools`. Every pool needs its real `PoolID`, `ReserveBase`,
`ReserveQuote`, and `FeeBP`. Use exact decimal strings. Null/omitted quantities
are refused; explicit zero remains zero. An explicit empty `Pools` array asserts
audited absence, and must not be used to bypass missing historical coverage.
The input's `Book` is ignored: the order book comes from replay.

Known acquisition gaps and crossed books are refused. No override flag is
provided. Passing those checks does not prove account discovery is exhaustive or
that the dust defect is repaired. Review the reconstruction and retain its raw
evidence before loading a real historical result.

The test path uses a separate disposable database and controlled fixture inputs:

```sh
KEEL_REPLAY_TEST_DSN=postgres://... go test ./cmd/keel \
  -run TestReplayPersistenceThroughPostgresAndHistoricalAPI -count=1 -v
```

Do not point this test at an application/evidence database: it deliberately
writes synthetic test rows. It verifies the actual CLI writer and the real store
and API reader. Existing rows remain unchanged on duplicate writes. Track A's
`-historical` deployment switch remains off until accepted historical rows exist.
The API uses the current methodology version, so A7's bump requires corresponding
new computations; old rows must retain their original labels.

## 5. Design choices

Persistence rejects unresolved acquisition gaps and requires explicit pool evidence
because storage has no venue-coverage field that could preserve an unknown pool.
The rejected alternative was storing order-book-only calculations with a terminal
disclaimer. That would let the API publish combined-market figures and flag states
whose missing pool coverage the consumer could not see.

The ledger timestamp comes from the ledger resource and is validated against the
requested sequence. Estimating it from average ledger spacing was rejected.
Historical bucketing must use the recorded time, not an approximation.

## 6. Verification on this checkout

- `go test ./... -count=1`: passed with the store and replay integration suites
  using separate disposable migrated databases. Conformance runs in this suite.
- `go vet ./...` and `go build ./...`: passed. The Windows sandbox emitted a
  non-fatal module stat-cache permission warning during build.
- Pinned `golangci-lint` v2.13.0 with `--new-from-rev=HEAD`: zero issues.
- Full pinned lint reports gofmt findings in untouched files on this Windows
  checkout. Those files were not reformatted as part of Track B.
- `make ci`: vet and architecture checks passed, then the race-test step stopped
  because CGO is disabled on this machine. A race-enabled run remains required.
- `analyze.py --check`: passed; every raw capture matches its manifest hash.
- `git diff --check` and file-specific `gofmt -l`: passed for the changes.
- Independent code review: the unknown-pool and omitted-numeric-field findings
  were fixed and rechecked; no remaining concrete code blockers were reported.

The database integration inputs are controlled fixtures, not a repaired February
reconstruction. No deployment or accepted historical-data load is implied by these
checks. An initial combined test run used one test database for both suites and
the store tests correctly detected the extra CLI fixture rows; rerunning against
separate databases passed without changing test expectations.
