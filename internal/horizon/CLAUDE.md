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

## What exists here now

`client.go` is the read side: `GetSnapshot` assembles one `domain.Snapshot` from
`/order_book`, `/liquidity_pools`, and `/ledgers/{seq}`, and `VerifyAsset` closes
trap 4 once per asset rather than once per snapshot. `decode.go` holds the wire
shapes. `recorder.go` is the cross-validation recorder, one file per pair per
ledger, never overwritten. Each file's own header lists the design decisions and
the alternative rejected, which is what this zone asks for; the three that matter
most across all of them are below.

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
