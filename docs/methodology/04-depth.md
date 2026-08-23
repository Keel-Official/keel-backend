# Keel: Depth: SDEX, AMM, and the Combination

**Methodology version:** 1.0.3-draft
**Status:** complete. The sell-side fee treatment was corrected in 1.0.3.

Depth is the quantity the whole product exists to report: how much notional the quoted
price can actually absorb. Two venues supply it and they are not additive, which is
section 3 and the single most consequential rule in this folder.

---

## 1. SDEX order book depth

```
P_target           = P0 × (1 + δ)
depth_sdex_buy(δ)  = Σ (price_i × amount_i)  over asks with price_i ≤ P_target
depth_sdex_sell(δ) = Σ (price_i × amount_i)  over bids with price_i ≥ P0 × (1 − δ)
```

**A level that crosses the boundary is discarded entirely**, never taken partially. This
yields a slightly lower figure than the theoretical value, which is deliberate under the
conservative principle in `11-limitations.md`.

---

## 2. Constant product AMM depth

`X × Y = k`, spot price `P = Y / X`. Buying `Δx` of base leaves `X − Δx`:

```
P' = k / (X − Δx)²        →        P' / P = X² / (X − Δx)²
```

For `P' / P = 1 + δ` we need `X − Δx = X / √(1 + δ)`, giving:

```
depth_amm_buy(δ)   = Y × (√(1 + δ) − 1)
depth_amm_sell(δ)  = Y × (1 − √(1 − δ))
gross_input        = net_input / (1 − f)
```

Mandatory sanity assertion in tests: `depth_amm ≈ (δ / 2) × Y`.

| δ   | up, percent of Y | down, percent of Y |
| --- | ---------------- | ------------------ |
| 2%  | 0.995%           | 1.005%             |
| 5%  | 2.47%            | 2.53%              |
| 10% | 4.88%            | 5.13%              |

Square roots are computed at a fixed decimal precision. That precision and its tolerance
are named constants and form part of the methodology, not an implementation detail.

**Fee treatment.** The buy side is grossed up by `/(1 − f)` because the quantity being
computed is an input. The sell side returns quote that is received, so the fee reduces
it: `net_output = gross_output × (1 − f)`. Consistent on both sides: the fee always
favours the pool, never the counterparty.

---

## 3. Combining SDEX and AMM

**Wrong:** `total_depth = depth_sdex + depth_amm` computed independently. Both compete
over the same price range.

**Correct:** both are bounded by the same final marginal price.

```
combined_depth(δ):
    P_target = P0 × (1 + δ)
    n_sdex   = Σ (price × amount) over asks with price ≤ P_target
    n_amm    = Σ over all pools:
                   0                              if P_pool ≥ P_target
                   Y × (√(P_target / P_pool) − 1) if P_pool < P_target
    return n_sdex + n_amm
```

`fromSdex` and `fromAmm` are still reported separately so third parties can verify the
combination without reading the code.

Mandatory discriminating test: a fixture whose pool price sits 5 percent above `P0`,
queried for depth at 2 percent. The correct answer is `fromAmm` exactly zero.

**Empirical evidence.** Effects on the USTRY/USDC pool show `trade` and
`liquidity_pool_trade` interleaved within a single operation, that is, a path payment
routed through both the order book and the pool at once. Summing the two independently
contradicts how the protocol actually behaves.

---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
