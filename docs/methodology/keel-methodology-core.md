# Keel: Core Methodology

**Methodology version:** 1.0.3-draft
**Applies to:** all of `internal/domain`
**Validated against:** the YieldBlox DAO pool incident on Blend V2, 22 February 2026

This document defines what Keel computes and why. Every definition here must be
defensible without reference to the code. If the code and this document disagree, this
document is correct and the code is wrong.

Flag and band definitions live in `09-flags-and-bands.md`.

---

## 1. The question being answered

An oracle answers: what is the price of this asset.
Keel answers: **how much can be transacted at that price, and what does it cost to move
it.**

Two risks that must be kept strictly apart, because they are routinely confused:

| Risk                    | Book side | Question                                                                           |
| ----------------------- | --------- | ---------------------------------------------------------------------------------- |
| **Liquidation**         | bids      | If this collateral is liquidated, can the market absorb it?                        |
| **Oracle manipulation** | asks      | What does it cost to raise the price until the collateral looks far more valuable? |

An asset can be safe on one side and dangerous on the other. Keel reports both
separately and never collapses them into a single number.

---

## 2. Notation and units

| Symbol     | Meaning                                                       |
| ---------- | ------------------------------------------------------------- |
| `base`     | the asset being assessed                                      |
| `quote`    | the asset used as the unit of measurement                     |
| `P0`       | reference price, quote per base                               |
| `δ`        | relative price move; 0.02 means 2 percent                     |
| `P_target` | `P0 × (1 + δ)`                                                |
| `X`, `Y`   | base and quote reserves of a single pool                      |
| `f`        | pool fee, read from `fee_bp` in the response, never hardcoded |

1. All depth and cost figures are expressed as **notional in the quote asset**, not as a
   count of tokens.
2. Keel does **not** convert to USD using an external price feed. The premise of this
   product is to question whether a reported price can be trusted; using a feed to
   compute would make the argument circular.
3. Stellar amounts are int64 stroops with 7 decimals. Prices are read from the rational
   fraction `price_r`, never from the rounded `price` string. All arithmetic is decimal,
   never floating point.
4. Time is never derived from a ledger sequence arithmetically. Assuming five seconds
   per ledger drifts by roughly three weeks over a six-month span.

---

## 3. Reference price `P0`

**Changed in 1.0.3.** The previous rule always preferred the order book mid whenever both
sides were populated. On the USTRY fixture that rule produced 53.8971414 for an asset
whose real liquidity sat at 1.0555442, off by a factor of 50.

```
1. Pool present AND two-sided book
     divergence = |book_mid − pool_spot| / pool_spot × 100
     if divergence > Thresholds.PriceDivergencePct
         P0 = pool_spot,  priceSource = "pool",  raise PRICE_SOURCE_CONFLICT
     else
         P0 = book_mid,   priceSource = "book"
2. Two-sided book, no pool        -> book_mid,  "book"
3. One-sided book, pool present   -> pool_spot, "pool"
4. Pool only                      -> pool_spot, "pool"
5. Neither book nor pool          -> undefined, "none"
```

Rationale: when two price sources disagree, trust the one backed by executable
liquidity, and **tell the consumer that they disagree**. An order book mid with a spread
of several hundred percent is not executable by anyone. Hiding the conflict is worse
than picking one side of it.

When more than one pool exists, `pool_spot` is taken from the pool with the largest
quote reserve. `poolSpotPrice` and `priceDivergencePct` are always reported whenever a
pool is present, regardless of which branch was taken.

**Case 5 is not an error.** An asset with no executable price is the highest-value
finding Keel can produce. It is reported as a valid result carrying the
`NO_EXECUTABLE_PRICE` flag and band `CRITICAL`.

**A warning drawn from real data.** A book-derived `P0` can be shaped by the attacker's
own orders. On 22 February 2026 the attacker placed a buy order for 0.0001 USTRY at
1.057, one minute after placing the manipulation offer. An order that small contributes
to `P0` while representing no liquidity at all. `P0` must never be read on its own.

---

## 4. SDEX order book depth

```
P_target           = P0 × (1 + δ)
depth_sdex_buy(δ)  = Σ (price_i × amount_i)  over asks with price_i ≤ P_target
depth_sdex_sell(δ) = Σ (price_i × amount_i)  over bids with price_i ≥ P0 × (1 − δ)
```

**A level that crosses the boundary is discarded entirely**, never taken partially. This
yields a slightly lower figure than the theoretical value, which is deliberate under the
Conservative Principle in section 13.

---

## 5. Constant product AMM depth

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

## 6. Combining SDEX and AMM

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

## 7. Manipulation cost

### 7.1 Definition

```
MC(P_target)        = Σ notional of asks with price < P_target
Reachable(P_target) = there EXISTS an ask with price ≥ P_target
```

An attacker must consume every ask **cheaper** than the target, then barely touch the
first ask sitting **above** it. That final touch is what sets the price the oracle reads,
and it can cost arbitrarily little.

The two sets are disjoint and exhaustive. An ask never falls into both.

| State                            | Meaning                                                   |
| -------------------------------- | --------------------------------------------------------- |
| `Cost` small, `Reachable = true` | **most dangerous**                                        |
| `Cost` large, `Reachable = true` | expensive; the market has a defence                       |
| `Reachable = false`              | the target cannot be reached at any capital. Not bad news |

`Cost` is an **upper bound**. Keel cannot know which orders belong to the attacker ahead
of time, so it does not filter them out. This bias points in the safe direction.

### 7.2 Why order ownership is decisive

A mistake that nearly entered this methodology and was caught only by the data.

The wrong intuition treats the size of the manipulating trade as its cost. On 22 February
that trade was 5.3475699 USDC, but the offer it executed against belonged to the attacker,
so the money returned to the attacker.

The real cost is the value of **third-party** orders that must be consumed on the way to
the target. Because Stellar's matching engine always fills from the best price, this is
exactly the buy-side depth up to `P_target`.

### 7.3 Two forms, two different questions

**Added in 1.0.3** after verification showed an honest pool present throughout the attack.

| Metric                          | Contents     | Answers                                          |
| ------------------------------- | ------------ | ------------------------------------------------ |
| `manipulationCostCombined`      | SDEX and AMM | cost of moving the actual market price           |
| `manipulationCostOrderbookOnly` | SDEX only    | cost of fooling an oracle that reads SDEX trades |

On the USTRY fixture, reaching 106.7372828 costs roughly 147.96 USDC through the pool and
**zero** through the order book. The attacker paid zero.

`orderbookOnly ≤ combined` always holds, since combined adds the AMM term. An attacker
takes the cheapest path, so `orderbookOnly` is the binding figure. The gap between the two
is itself a signal: an asset with a large `combined` and a small `orderbookOnly` looks
safe while it is not.

The claim that the oracle reads only SDEX trades is an **inference** drawn from the fact
that an honest pool existed and the attack still succeeded. It stays marked as an
inference until confirmed.

### 7.4 The large-delta ladder

Depth at ±2/5/10 percent measures market quality. It is **not sufficient** for oracle
resilience, because the attacker moved the price by a factor of 100.98, not by 10 percent.

| Ladder                  | δ values         | Purpose             |
| ----------------------- | ---------------- | ------------------- |
| Market quality          | 0.02, 0.05, 0.10 | required by the SOW |
| Manipulation resilience | 0.5, 1, 10, 100  | cost of an attack   |

### 7.5 Maximum reachable price

```
MaxReachablePrice       = the highest ask price in the book
CostToMaxReachablePrice = Σ notional of asks with price < MaxReachablePrice
```

This pair captures attacks that fall between two rungs of the delta ladder. On the USTRY
fixture the values are 106.7372828 at zero cost, while the actual attack landed between
δ=0.5 and δ=1.

**When an active pool is present, both are null**, accompanied by a warning. Under a
constant product curve the price tends to infinity as the base reserve tends to zero, so
every target is reachable and the notion of a highest price loses meaning. Both fields
are meaningful only for pure order book markets.

---

## 8. Oracle resilience and the VWAP window

The oracle that was manipulated is VWAP-based, not last-price. Moving the marginal price
is not enough on its own; the attacker's trade must also dominate the volume-weighted
average.

```
MR(P_target, W) = MC_orderbookOnly(P_target) + V_genuine(W)
```

The second term is the invisible defence. An actively traded market forces the attacker
to outweigh real volume. A market with no trading has no such defence. On 22 February
both terms were zero or near zero at the same time.

`W` is a parameter. The 15-minute default follows Script3's statement that no other trade
occurred within 15 minutes before the manipulation. That figure is **not confirmed** as
Reflector's actual window and is marked as an assumption.

---

## 9. Maximum safe collateral size

```
δ_critical = Params.ManipulationCriticalDelta          (default 0.5)

if Reachable_orderbookOnly(δ_critical):
    C_max = min( D_sell(δ_liquidation) × h , MC_orderbookOnly(δ_critical) × m )
else:
    C_max = D_sell(δ_liquidation) × h
    warning "manipulation to δ_critical is unreachable through the order book;
             the manipulation term was not applied"
```

| Symbol          | Meaning                     | Default |
| --------------- | --------------------------- | ------- |
| `δ_liquidation` | liquidation discount        | 0.10    |
| `h`             | liquidation haircut         | 0.5     |
| `δ_critical`    | critical manipulation delta | **0.5** |
| `m`             | manipulation safety margin  | 0.25    |

**Why `δ_critical = 0.5`, changed in 1.0.3.** Because `MC` is monotonically increasing in
δ and `Reachable` is monotonically decreasing, a lower value always yields a tighter bound
while relying less often on an unreachable target. On the USTRY fixture, δ=1 produces
`Reachable = false` with `Cost = 130.0627093`, so the manipulation term would yield a
positive collateral allowance derived from an **impossible** attack. At δ=0.5 the result
is zero, which is correct. A 50 percent price inflation is already more than enough to
push a position under water at any sane LTV.

**Why the `Reachable` guard.** When the target is unreachable, `MC` is not the cost of
reaching anything, and multiplying it by `m` produces a meaningless number.

**Why `orderbookOnly`.** An attacker takes the cheapest path, and `orderbookOnly` is
always less than or equal to `combined`. Using the smaller figure is the conservative
choice.

Both terms must be reported separately, not only their minimum. Every parameter is
caller-configurable.

Default values are **chosen, not calibrated**. That statement must appear on the dashboard
and in API responses.

**A more correct form, candidate for v1.1.** Fixing a single δ remains arbitrary. The
form with no chosen constant lets the attacker pick their best δ:
`C_max_manipulation = min over Reachable δ of [ MC(δ) / ((1 + δ) × LTV − 1) ]`.
Deferred out of 1.0.x because it changes the shape of `C_max` and introduces an `LTV`
parameter.

---

## 10. Arbitrage asymmetry

**New in 1.0.3.** Explains why the attack was not merely cheap, but also went uncorrected
for 22 hours.

State at the time of the attack:

| Source   | Price       | Character                       |
| -------- | ----------- | ------------------------------- |
| Pool     | 1.0555442   | executable in both directions   |
| Book bid | 1.0570000   | executable, size 0.0001 USTRY   |
| Book ask | 106.7372828 | executable, but nobody wants it |

The only arbitrage candidate was buying from the pool at 1.0555 and selling into the bid
at 1.0570, a spread of 0.14 percent, below the pool fee of 0.30 percent. Not profitable.

The ask at 106.74 creates no opportunity at all, because nobody is offering to **buy** at
that price.

> Arbitrage only corrects mispricing that is **executed**. It does not correct mispricing
> that is merely **quoted** or **reported**. An oracle reading quotes or last-trade prices
> is reading precisely the part of the market that no mechanism defends.

The consequence: depth measures executable liquidity, which is exactly the part of the
market arbitrage protects. The gap between the quoted price and the depth-supported price
is the attack surface.

---

## 11. Empirical validation: the 22 February 2026 incident

Every figure below is derived from Horizon mainnet and reproducible without an account.

```
USTRY : GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC  credit_alphanum12
USDC  : GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN  credit_alphanum4
Pool  : 27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb  fee 30 bps
```

### Verified timeline

| Time UTC        | Event                                                                                            | Evidence                           |
| --------------- | ------------------------------------------------------------------------------------------------ | ---------------------------------- |
| 10 Feb 16:59:35 | Last pool effect before the attack. Reserves 16.3389179 USDC and 15.4791416 USTRY                | pool effect                        |
| 21 Feb 23:36:28 | Burner swaps 1 XLM for 0.1612003 USDC                                                            | op 263452928864530433              |
| 21 Feb 23:38:51 | Manipulation offer: sell 1.2185312 USTRY @ 106.7372828                                           | tx `09e1a9d1...`, offer 1824788980 |
| 21 Feb 23:39:31 | Buy order for 0.0001 USTRY @ 1.0570000                                                           | op 263453066303434753              |
| 22 Feb 00:10:21 | Manipulating trade: 5.3475699 USDC for 0.0501003 USTRY, matched against the attacker's own offer | ledger 61340263                    |
| 22 Feb 00:10:57 | Dust trade of 0.0000080 USTRY @ 1.057 between attacker accounts                                  |                                    |
| 22 Feb ~00:25   | Borrows: 1,000,196.70 USDC then 61,249,278.31 XLM                                                | secondary source                   |
| 22 Feb 22:08:33 | Next pool effect, nearly 22 hours after the attack                                               | pool effect                        |

### Arithmetic consistency

```
0.0501003 × 106.7372828  = 5.3475699 USDC     matches the amount paid
1.2185312 − 0.0501003    = 1.1684309 USTRY    matches the offer remainder today
106.7372828 / 1.057      = 100.98×            matches the reported "100x"
16.3389179 / 15.4791416  = 1.0555442          the pool's honest price
61,249,278.31 × 0.1612003 + 1,000,196.70 = 10,873,599 USDC
149,876.10 × 1.057 = 158,419 USDC real,  × 106.7372828 = 15,997,368 USDC manipulated
```

### Key findings

**Order book manipulation cost was zero, verified.** The trade list for account
`GDHRCQNC...` on 22 Feb 00:10:21 contains exactly one record, against an offer owned by
the attacker. Because the matching engine fills from the best price and every match
produces a trade record, the absence of any other record proves there were no third-party
asks anywhere between 1.057 and 106.74. This is a direct observation, not an inference.

**An honest pool was present throughout and prevented nothing.** Reserves went unchanged
for 12 days spanning the entire attack. Moving the actual market price to 106.74 would
have cost roughly 147.96 USDC. The attacker did not do that.

**What Keel would report.** Band `CRITICAL` with `bandConfidence` of `partial`. The flag
breakdown is in `testdata/fixtures/ustry_pre_exploit.md`.

### Outstanding verification

1. Both borrow transactions on the YieldBlox pool, still from secondary sources
2. The YieldBlox pool risk parameters in force at the time
3. Reflector's actual VWAP window length
4. Whether Reflector considers AMM reserves at all

---

## 12. Trade-implied depth

When historical order book state is unavailable, depth can be bounded from trades that
did occur.

```
depth(δ) ≤ S    if a trade of size S moved the marginal price by δ
```

If liquidity within that range exceeded `S`, a trade of size `S` could not have crossed
it.

The result is an upper bound, not a measurement. That is sufficient for Keel's purpose,
because what must be shown is not the exact depth but that depth sits below the safe
threshold.

Results derived this way **must** be tagged `dataSource: "trades-implied"` in API
responses.

---

## 13. Principles and limitations

### Conservative principle

In every ambiguous case, choose the interpretation that yields lower depth and a higher
risk assessment.

### Known limitations

1. **Liquidity that is not traded protects nothing.** The USTRY/USDC pool held honest
   reserves at 1.0555 for 12 days spanning the entire attack and prevented nothing.
2. **Resting liquidity is not executable liquidity.** Offers can be withdrawn instantly.
   Scan frequency is an honest parameter, not a technical detail.
3. **Path payments through intermediate assets are not counted.** True effective
   liquidity may exceed what Keel reports.
4. **Centralised exchange liquidity is invisible.**
5. **Thresholds are chosen, not calibrated.**
6. **A backtest knows the outcome in advance.** If a threshold was tuned after seeing the
   result, that must be stated in the report.
7. **Order ownership cannot be known ahead of time**, so manipulation cost is always an
   upper bound.

---

## 14. Version history

| Version     | Change                                                                                                                                                                                                                                                                                                         |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.0.0-draft | Initial definitions. Ownership-based manipulation cost. Large-delta ladder. Oracle window volume term                                                                                                                                                                                                          |
| 1.0.1-draft | `Reachable` rule corrected. `SPREAD_EXTREME` added                                                                                                                                                                                                                                                             |
| 1.0.2-draft | `MANIPULATION_CHEAP` requires `Reachable == true`. `unevaluated` state and `bandConfidence` added                                                                                                                                                                                                              |
| 1.0.3-draft | `P0` prefers the pool on large divergence, `PRICE_SOURCE_CONFLICT` added. Manipulation cost split into `combined` and `orderbookOnly`. `MaxReachablePrice` null when a pool is active. `δ_critical` 1.0 to 0.5 with a `Reachable` guard on `C_max`. Arbitrage asymmetry section. Sell-side fee treatment fixed |
