# GREEN ZONE: internal/api

Read-only HTTP handlers. Write freely, this is plumbing.

The contract already exists and changing it is not this package's business:
`docs/api/keel-openapi.yaml`. If a handler needs a different response shape, the
contract changes through a decision record, not the handler quietly.

## Rules

1. **The API never calls an adapter.** It only reads results already computed,
   out of `internal/store`. One popular asset triggering a Horizon call per
   request would burn the rate limit budget in minutes, and users would get
   unpredictable latency. The consequence is that metrics always lag slightly,
   and that is accepted explicitly in NFR-1.
2. **Every numeric value is sent as a string.** Stellar amounts are int64 stroops
   with 7 decimals, and a JSON number is an IEEE 754 double. The only exceptions
   are `delta` and integers such as `ledgerSeq`.
3. **Scale conversion happens here, in one obvious place.** Fields ending in
   `Pct` are on a PERCENT scale in the API. `spreadPct: '196.0777141'` means 196
   percent. If one fractional field hides among the percent fields, someone will
   eat it.
4. Every response carries `ledgerSeq` and `methodologyVersion`, plus the
   `X-Keel-Staleness-Seconds` and `X-Keel-Methodology-Version` headers.
5. **An asset with no price is not an error.** `priceSource: none` with band
   `CRITICAL` is an HTTP 200. A ledger that is not available yet is a 404 with
   code `LEDGER_NOT_AVAILABLE`, not a 500. A book with a spread of several hundred
   percent is also a 200. All three are findings, not failures.

## Settle this before writing handlers

The contract still lags `internal/domain/types.go` in four places:
`costToMaxReachablePrice`, `unevaluatedFlags`, and `bandConfidence` exist in the
code but not in the contract, and `oracleResistance` is a scalar in the code but
an object in the contract. Writing handlers against a contract that is not in
sync means writing them twice.

See findings P1-6 through P1-12 in `docs/internal/audit-2026-08-20.md`.
