# YELLOW ZONE: internal/horizon

Adapter for live data from the Horizon API.

The output MUST be a `domain.Snapshot`, identical to what `internal/hubble`
produces. If the two adapters produce different types the design has failed and
`computeDepth()` ends up changing with them.

## Endpoints

- `GET /order_book` with the selling_asset_* and buying_asset_* parameters
- `GET /liquidity_pools` with the `reserves=A,B` filter
- `GET /assets?asset_code=...` to verify asset identity, and to read the issued
  supply and Horizon's own holder count
- `GET /ledgers/{sequence}` for the ledger close time, which `/order_book` does
  not carry at all
- `GET /accounts?asset=CODE:ISSUER` for the trustline holders, paged. This is the
  only endpoint here that pages, and the only one whose request cost grows with
  the asset rather than being fixed. See `holders.go`

## Traps that keep happening

1. The old import path `github.com/stellar/go/...`. The correct one is
   `github.com/stellar/go-stellar-sdk/...`.
2. Reading the `price` string field. The correct one is `price_r`, shaped
   `{"n": 1, "d": 10}`. The string field has already lost precision.
3. Horizon does NOT serve historical data. If you need the past, that is
   internal/hubble's job. Never invent a historical endpoint. The trustline
   balance is the sharpest form of this: an order book can at least be
   reconstructed from the operations that posted it, which is what
   `offers-implied` means in the golden fixture, and a balance at a past ledger
   cannot be reconstructed from anything Horizon exposes. That is why
   `holders.go` exists as a recorder rather than as something `scan` calls.
4. Asset type must be passed explicitly and never inferred from code length.
   USTRY has a five character code and is `credit_alphanum12`; querying it as
   `credit_alphanum4` returns an empty result and no error. Two decision records
   in this repository contain exactly that mistake.

## After writing code here

Explain in three sentences: what design decision you took, one alternative you
rejected, and why.

## Two more traps, both found by running against live Horizon

5. **The two sides of `/order_book` are not denominated in the same asset.** An
   ask's `amount` is in the base asset. A bid's `amount` is in the QUOTE asset,
   because Horizon inverts the bid price into quote-per-base but leaves the
   amount as the underlying offer's selling amount, and a bid is an offer selling
   the quote asset. `domain.Level.Amount` is defined in base units, so a bid has
   to be converted. Reading it as base overstates sell-side depth by the price
   factor, and sell-side depth is the liquidation term of `C_max`. Measured, not
   assumed: `docs/evidences/order_book_amount_units_2026-08-24.txt`.
6. **`Latest-Ledger` is not on every response.** The collection endpoints send
   it, `/ledgers/{sequence}` does not, and it does not need to because it carries
   its own sequence in the body. The first version of this client required the
   header everywhere and failed on its first real request. `/order_book` carries
   no ledger sequence at all, so that header is the only honest stamp for a
   snapshot and a guess is not an acceptable substitute.

## A seventh trap, found while building the schema 2 recorder

7. **`/liquidity_pools` takes no asset type at all, and `/order_book` does.** The
   `reserves=` filter is ONE parameter holding two canonical asset strings
   separated by a comma, `native` for XLM and `CODE:ISSUER` otherwise, with no
   type anywhere in it. So the two endpoints fail DIFFERENTLY on the same bad
   configuration: `/order_book` with the wrong type returns an empty book and no
   error, which is trap 4, while `/liquidity_pools` never sees the type and
   answers correctly. A pair whose type is wrong therefore looks like a market
   with no orders and a working pool, which is a plausible reading of a real
   market rather than an obvious bug. The order of the two assets does not
   matter, the filter is an AND over both, and an empty result is HTTP 200 with
   `"records": []`. All measured, not assumed:
   `docs/evidences/liquidity_pools_reserves_2026-08-25.txt`.

## What exists here now

`client.go` is the read side: `GetSnapshot` assembles one `domain.Snapshot` from
`/order_book`, `/liquidity_pools`, and `/ledgers/{seq}`, and `VerifyAsset` closes
trap 4 once per asset rather than once per snapshot. `decode.go` holds the wire
shapes. `recorder.go` is the cross-validation recorder, one file per pair per
ledger, never overwritten. `tick.go` is recording schema 2, which is the same
recorder writing raw bytes and nothing else; it is the default since 25 August
2026 and `recorder.go` still writes schema 1 under `-schema 1`. Each file's own
header lists the design decisions and the alternative rejected, which is what
this zone asks for; the three that matter most across all of them are below.

## The three sentences this zone asks for

The decision: Horizon is read with `net/http` and hand-written response structs,
every snapshot keeps the raw bytes it was parsed from, and the recorder writes one
never-overwritten file per ledger. The alternative rejected: using
`horizonclient` from the Stellar SDK and recording only the parsed
`domain.Snapshot`, which is less code today. Why it was rejected: the SDK returns
its own structs that would need converting into `domain.Snapshot` anyway, so it
buys a dependency and saves no layer, and recording only the parsed form would
bake one reading of the order book into weeks of evidence, which is exactly the
reading that turned out to be wrong for bids until it was measured.

## The same three sentences for schema 2, added 25 August 2026

The decision: a tick stores the raw response bytes of `/order_book` and
`/liquidity_pools` as strings with their sha256, brackets them with the
`Latest-Ledger` of the first and last request, and parses, converts and selects
NOTHING, so a non-2xx and an empty pool list are both stored and kept. The
alternative rejected: extending `GetSnapshot` so one call returned the parsed
snapshot and the pool bytes together, which is far less code and reuses a path
that already works. Why it was rejected: the paragraph above is the reason in its
own words, because the parsed half of a schema 1 recording is precisely the half
that had to be revised once the bid amount unit was measured, and a recording
that claims nothing cannot be wrong in that way later.

The selection point is worth stating on its own, because it is the one that will
be under pressure. AQUA sits in 1308 pools and `reserves=AQUA,USDC` still returns
one record, since all 1308 are distinct asset pairs. If a pair ever returns more
than one, this package must still store all of them: CHOOSING AMONG CANDIDATE
POOLS IS A METHODOLOGY DECISION and methodology is the red zone. Summing their
reserves would be wrong anyway, because two pools at different prices are not one
deeper pool. Measured in `docs/evidences/aqua_identity_and_pools_2026-08-25.txt`.
