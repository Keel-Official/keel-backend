# API mocks

Every example response in `docs/api/keel-openapi.yaml`, written out as standalone
JSON so the frontend can be built before the API exists. `internal/api` is empty
and `keel serve` exits with code 3, so these files are the only thing to build
against today, and that is the plan rather than a shortfall (DEC-002 phase 1).

**These files are generated. Do not edit them.** Regenerate with `make api-mocks`.
Prove they still match the contract with `make api-mocks-check`, which fails if the
contract moved and the mocks did not.

## The files

| File | What it is for |
|---|---|
| `asset-healthy.json` | A liquid asset. The ordinary case, `band: LOW`, no flags |
| `asset-pool-only.json` | No orderbook, AMM only. `spreadPct` and `maxReachablePrice` are both null, for different reasons |
| `asset-no-price.json` | No orderbook and no pool. `priceSource: none`, `band: CRITICAL`, HTTP 200 |
| `asset-broken-book.json` | A spread of 196 percent. Real on-chain state, every number verified |
| `asset-historical.json` | A historical replay, `dataSource: hubble` |
| `asset-list-mixed.json` | The list endpoint, three rows across three bands |
| `history-ustry.json` | A time series with a gap in it |
| `health.json`, `methodology.json` | The two meta endpoints |

## Four states, not two

The thing most likely to go wrong in a Keel frontend is treating this as "data" and
"error". There are four distinct states and each needs its own display:

1. **Healthy.** `asset-healthy.json`. Show the numbers.
2. **No executable price.** `asset-no-price.json`. `priceSource: none`, `midPrice`
   null, `band: CRITICAL`, and it arrives as **HTTP 200**. This is the highest-value
   finding the product can produce. An error screen here is a bug.
3. **Broken book.** `asset-broken-book.json`. `midPrice` is populated and
   meaningless: it is the midpoint of an ask at 106.74 and a bid at 1.06, two prices
   with nothing to do with each other. Do not show it as a price without a marker.
   Damp the 2/5/10 percent depth ladder, which is derived from it. Promote the
   manipulation rungs at delta 10 and 100, and `maxReachablePrice`.
4. **Lower bound rather than measurement.** Any response with
   `dataSource: trades-implied`. The depth figures are lower bounds, reconstructed
   from trades that executed. Never display one as equivalent to a `horizon` result.

## Two pairs that must never be read apart

**`cost` and `reachable`**, inside every `manipulationCost` entry:

| cost | reachable | meaning |
|---|---|---|
| 0 | true | the target price is free. **The most dangerous state the product can report** |
| 0 | false | there is no liquidity at all in that range |
| > 0 | true | a real cost to reach the target |
| > 0 | false | the cost exhausts every ask and still does not reach the target |

`asset-broken-book.json` holds both zero cases side by side: delta 0.5 is
`cost: 0, reachable: true`, and delta 1, 10, and 100 are `cost: 130.06,
reachable: false`. Showing 130.06 as "expensive to reach" would be exactly
backwards; that price cannot be reached at all.

**`band` and `bandConfidence`.** `partial` means at least one flag at the CRITICAL
or HIGH level could not be evaluated, so the band is a floor: it can only be worse
than shown, never better. `asset-broken-book.json` is `CRITICAL` with `partial`.
A `LOW` with `partial` would be the dangerous one, and displaying it identically to
`LOW` with `full` is the failure the methodology explicitly forbids.

Also note `unevaluatedFlags`. A flag absent from both `flags` and
`unevaluatedFlags` was checked and does not apply. A flag inside
`unevaluatedFlags` was never checked. Those are different claims and must not
render the same.

## Numbers

Every numeric value is a **string**, except `delta` and integers such as
`ledgerSeq`. Stellar amounts are int64 stroops with 7 decimals, and a JSON number
is an IEEE 754 double. Use decimal.js or big.js. `parseFloat` will silently lose
precision on values this product exists to be precise about.

Fields ending in `Pct` are on a percent scale: `spreadPct: '196.0777141'` means 196
percent, not 19607 percent.

`null` means unknown, never zero. `spreadPct` is null when the book has only one
side. `maxReachablePrice` is null when there is no ask at all, or when all the
liquidity is in an AMM, because a constant product curve has no upper price bound.

## How to prove the handoff worked

The contract being valid is not the same as the contract being usable. The check
that actually means something:

```bash
# 1. the contract is structurally valid OpenAPI 3.1
npx --yes @redocly/cli lint docs/api/keel-openapi.yaml

# 2. the mocks still match the contract
make api-mocks-check
```

Then, on the frontend side: render all four states from these files **without
patching the data**. If a component needs a field edited, added, or reshaped to
render, that is a contract gap and it should come back as a question rather than a
local fix. That is the frontend equivalent of the golden fixture: the data is fixed
first, and the code is made to match it.

## What is still open

Read `docs/decisions/DEC-003-api-contract-v1-1.md` section 6 before starting. Field
names, types, and nullability are settled and safe. One thing is not:

- **No list example holds a `partial` row.** `asset-list-mixed.json` is all `full`.
  The partial case is real on that endpoint; design for it using
  `asset-broken-book.json`.

**`criticalDelta` is settled, as of 25 August 2026.** This entry used to say the
methodology implied 1.0 while every example used 0.5. It is 0.5, the value these files
already carried, so nothing here changed and no mock was regenerated.

Keep reading it from the response anyway. It is a caller-configurable parameter that
the API reports precisely so a consumer does not have to know it, and a UI that
hardcodes 0.5 will silently mislabel every response the day somebody runs Keel with a
different one.
