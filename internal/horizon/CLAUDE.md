# YELLOW ZONE: internal/horizon

Adapter for live data from the Horizon API.

The output MUST be a `domain.Snapshot`, identical to what `internal/hubble`
produces. If the two adapters produce different types the design has failed and
`computeDepth()` ends up changing with them.

## Endpoints

- `GET /order_book` with the selling_asset_* and buying_asset_* parameters
- `GET /liquidity_pools`
- `GET /assets?asset_code=...` to verify asset identity

## Traps that keep happening

1. The old import path `github.com/stellar/go/...`. The correct one is
   `github.com/stellar/go-stellar-sdk/...`.
2. Reading the `price` string field. The correct one is `price_r`, shaped
   `{"n": 1, "d": 10}`. The string field has already lost precision.
3. Horizon does NOT serve historical data. If you need the past, that is
   internal/hubble's job. Never invent a historical endpoint.
4. Asset type must be passed explicitly and never inferred from code length.
   USTRY has a five character code and is `credit_alphanum12`; querying it as
   `credit_alphanum4` returns an empty result and no error. Two decision records
   in this repository contain exactly that mistake.

## After writing code here

Explain in three sentences: what design decision you took, one alternative you
rejected, and why.
