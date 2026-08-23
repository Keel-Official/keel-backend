# Keel: Risk Flags and Bands

**Methodology version:** 1.0.3-draft
**Supersedes:** PRD sections 5.1 and 5.2, which now simply point here
**Implemented in:** `internal/domain/flags.go`

Flag definitions previously lived in the PRD while also being alluded to in the
methodology. Two homes for one definition guarantees the two will drift apart. From this
version onward this document is the single source of truth, and the PRD only points here.

---

## 1. Why rules rather than a weighted score

Keel does not publish a 0 to 100 weighted score.

A weighted score obliges its author to justify every weight, and there is no empirical
basis for doing so from a single incident. A rule-based classification only requires
justifying its thresholds, and each flag can be inspected independently by a consumer
with their own policy.

For that reason **flags are always reported individually in the API**, not only the band
derived from them. A consumer who disagrees with Keel's thresholds can still use the raw
flags.

---

## 2. Three states per flag

This was corrected in version 1.0.2. Previously a flag had only two states, which made
assets with incomplete data look safer than they were.

| State         | Meaning                                                                 |
| ------------- | ----------------------------------------------------------------------- |
| `triggered`   | the condition is met                                                    |
| `clear`       | the condition was checked and is not met                                |
| `unevaluated` | the required data is unavailable, so the condition could not be checked |

`unevaluated` is **not** a synonym for `clear`. An asset with no trustline data must not
look identical to one whose holder distribution was checked and found safe.

Consequences for the output:

- `flags` holds the flags that are `triggered`
- `unevaluatedFlags` holds the flags that could not be checked
- `bandConfidence` is `partial` when any CRITICAL or HIGH tier flag is `unevaluated`, and
  `full` when every flag at those tiers could be checked

Dashboards must surface `bandConfidence`. A `LOW` band with `partial` confidence is a far
weaker statement than `LOW` with `full` confidence, and that difference must not be
hidden.

---

## 3. Data required per flag

| Flag                           | Requires                         |
| ------------------------------ | -------------------------------- |
| `NO_EXECUTABLE_PRICE`          | book and pool snapshot           |
| `ZERO_DEPTH_2PCT`              | book and pool snapshot           |
| `SPREAD_EXTREME`               | book snapshot                    |
| `PRICE_SOURCE_CONFLICT`        | book and pool snapshot           |
| `THIN_DEPTH_5PCT`              | book and pool snapshot           |
| `MANIPULATION_CHEAP`           | book and pool snapshot           |
| `MANIPULATION_RATIO_LOW`       | snapshot plus circulating supply |
| `NO_GENUINE_TRADE_7D`          | trade history                    |
| `NO_GENUINE_TRADE_30D`         | trade history                    |
| `WASH_TRADE_SUSPECTED`         | trade history                    |
| `HOLDER_CONCENTRATION_HIGH`    | trustline distribution           |
| `HOLDER_CONCENTRATION_EXTREME` | trustline distribution           |

The first six can be evaluated from a `Snapshot` alone. The remaining six need additional
input and become `unevaluated` when that input is absent.

---

## 4. Flag definitions

All thresholds refer to `Thresholds` in `types.go` and are expressed in the **quote
asset**, never in USD.

### CRITICAL tier

**`NO_EXECUTABLE_PRICE`**

```
priceSource == none
```

No order book and no pool. The asset has no executable price at all.

**`ZERO_DEPTH_2PCT`**

```
depth(0.02).BuySide == 0  OR  depth(0.02).SellSide == 0
```

One side alone is enough. An asset that cannot be sold is as dangerous as one that cannot
be bought.

**`MANIPULATION_CHEAP`**

```
THERE EXISTS delta d in ManipulationCostOrderbookOnly such that:
    Reachable(d) == true  AND  Cost(d) < Thresholds.ManipulationCheapAbsolute
```

Evaluated on the **orderbookOnly** variant, not combined. An attacker takes the cheapest
path, and `orderbookOnly <= combined` always holds.

**The `Reachable == true` condition was added in 1.0.2 and must not be removed.**

The reason is concrete. On the USTRY fixture, `Cost` is 130.0627093 for δ = 1, 10 and 100,
yet all three have `Reachable = false` because no ask sits above 106.7372828. Without this
condition those three rows would be counted, even though they prove an attack to such a
price is **impossible**. Labelling an impossible state as "cheap" inverts the truth.

Conversely `Cost(δ=0.5) = 0` with `Reachable = true` on the same fixture is the most
dangerous condition that can exist, and that is precisely what must be caught.

### HIGH tier

**`MANIPULATION_RATIO_LOW`**

```
THERE EXISTS delta d in ManipulationCostOrderbookOnly such that:
    Reachable(d) == true
    AND  Cost(d) / circulating_supply_value < Thresholds.ManipulationRatioLowPct
```

The `Reachable` condition applies for the same reason as above.

**`PRICE_SOURCE_CONFLICT`**

```
an active pool exists AND
priceDivergencePct > Thresholds.PriceDivergencePct
    where priceDivergencePct = |book_mid - pool_spot| / pool_spot × 100
```

Added in 1.0.3. Two price sources for the same asset give materially different answers.
When triggered, `P0` is taken from the pool spot price rather than the book mid, because
the pool is executable liquidity while a mid with a several-hundred-percent spread is not.

On the USTRY fixture the divergence is a factor of 50: book mid 53.8971414 against pool
1.0555442. This flag is what prevents every downstream metric from being computed on top
of a meaningless reference price.

**`SPREAD_EXTREME`**

```
spreadPct > Thresholds.SpreadExtremePct
    where spreadPct = (best_ask − best_bid) / P0 × 100
```

Added in 1.0.1. Once the spread reaches hundreds of percent, `P0` and everything derived
from it lose meaning. On the USTRY fixture the book spread is enormous, giving a book mid
of 53.90 for an asset worth roughly 1.06.

Other flags do fire on that case as well, but by coincidence rather than by design.
`spreadPct` is also reported as a number, not only as a triggered state, because its
magnitude is informative.

**`NO_GENUINE_TRADE_30D`**

```
no genuine trade within Thresholds.GenuineTradeStaleDays days
```

**`HOLDER_CONCENTRATION_EXTREME`**

```
holderTop1Pct > Thresholds.HolderTop1ExtremePct
```

### MEDIUM tier

**`THIN_DEPTH_5PCT`**

```
min(depth(0.05).BuySide, depth(0.05).SellSide) < Thresholds.ThinDepth5PctAbsolute
```

**`NO_GENUINE_TRADE_7D`**

```
no genuine trade within Thresholds.GenuineTradeWarnDays days
```

**`HOLDER_CONCENTRATION_HIGH`**

```
holderTop10Pct > Thresholds.HolderTop10HighPct
```

**`WASH_TRADE_SUSPECTED`**

```
tradesExcludedPct > Thresholds.WashTradeSuspectedPct
```

---

## 5. Deriving the band

The band is the highest tier among the `triggered` flags. No weighting, no averaging, no
summation.

| Band       | Triggered when a flag exists at tier |
| ---------- | ------------------------------------ |
| `CRITICAL` | CRITICAL                             |
| `HIGH`     | HIGH                                 |
| `MEDIUM`   | MEDIUM                               |
| `LOW`      | no flag triggered                    |

`bandConfidence` is determined separately, per section 2.

---

## 6. Default thresholds

| Threshold                   | Default | Unit        |
| --------------------------- | ------- | ----------- |
| `ManipulationCheapAbsolute` | 10,000  | quote asset |
| `ManipulationRatioLowPct`   | 1.0     | percent     |
| `ThinDepth5PctAbsolute`     | 50,000  | quote asset |
| `SpreadExtremePct`          | 20.0    | percent     |
| `PriceDivergencePct`        | 10.0    | percent     |
| `HolderTop1ExtremePct`      | 50.0    | percent     |
| `HolderTop10HighPct`        | 80.0    | percent     |
| `WashTradeSuspectedPct`     | 50.0    | percent     |
| `GenuineTradeStaleDays`     | 30      | days        |
| `GenuineTradeWarnDays`      | 7       | days        |

**Every value here is chosen, not calibrated against a body of incidents.** Calibration
would require more events than are available. This statement must appear on the
`/methodology` endpoint, on the dashboard, and in the backtest report, not only in this
document.

### An unresolved limitation of units

Absolute thresholds are expressed in the quote asset. As a result, an asset measured
against XLM and one measured against USDC cannot be compared against the same threshold,
and an asset's band can change purely because the XLM price moved, with no change in its
liquidity whatsoever.

Expressing thresholds in USDC merely relocates the problem, since it assumes USDC is
stable, which is somewhat ironic for a product that questions price assumptions.

Version 1.0.3 does not resolve this. What is done instead: `quote` is always included in
every response so consumers know which unit applies, and the limitation is stated openly.
This is open question Q7 and must be settled before version 1.1.

---

## 7. Verified worked example: USTRY/USDC at ledger 61340263

Drawn from `testdata/fixtures/ustry_pre_exploit.md`, computed by hand before any
implementation existed.

Input: one ask of 1.2185312 USTRY at 106.7372828, one bid of 0.0001 USTRY at 1.057, and
one pool holding 16.3389179 USDC and 15.4791416 USTRY at 30 bps.

The pool spot price is 1.0555442 while the book mid is 53.8971414, a divergence of 50x.
`PRICE_SOURCE_CONFLICT` therefore triggers, `P0 = 1.0555442` and `priceSource = "pool"`.

Depth and manipulation cost values live in the fixture file.

**Triggered**

| Flag                    | Tier     | Reason                                                  |
| ----------------------- | -------- | ------------------------------------------------------- |
| `MANIPULATION_CHEAP`    | CRITICAL | `Cost_orderbookOnly(δ=0.5) = 0` with `Reachable = true` |
| `PRICE_SOURCE_CONFLICT` | HIGH     | 50x divergence between pool and book mid                |
| `SPREAD_EXTREME`        | HIGH     | book spread far beyond 20%                              |

`ZERO_DEPTH_2PCT` and `THIN_DEPTH_5PCT` depend on depth values computed with the new
pool-derived `P0` and are filled in from the fixture file.

**Clear**

`NO_EXECUTABLE_PRICE`, since `priceSource = pool`.

**Unevaluated**

`MANIPULATION_RATIO_LOW`, `NO_GENUINE_TRADE_30D`, `NO_GENUINE_TRADE_7D`,
`WASH_TRADE_SUSPECTED`, `HOLDER_CONCENTRATION_EXTREME`, `HOLDER_CONCENTRATION_HIGH`. All
six require supply, trade history or trustline distribution data absent from the snapshot.

**Result**

```
band            = CRITICAL
bandConfidence  = partial
```

`partial` because `MANIPULATION_RATIO_LOW` and `HOLDER_CONCENTRATION_EXTREME` sit at the
HIGH tier yet are `unevaluated`. The band remains `CRITICAL` since a CRITICAL flag already
fired, so the incomplete data does not change the conclusion in this case.

---

## 8. Changes accompanying this version

| File                       | Change                                                                                                                               |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `internal/domain/types.go` | add `UnevaluatedFlags []Flag`, `BandConfidence`, `PriceDivergencePct`, `PoolSpotPrice` to `AssetRisk`; add `FlagPriceSourceConflict` |
| `docs/api/keel-openapi.yaml` | add `unevaluatedFlags`, `bandConfidence`, `spreadPct`, `poolSpotPrice`, `priceDivergencePct`, `PRICE_SOURCE_CONFLICT`. Landed in contract 1.3.0 |
| PRD section 5              | replace its contents with a pointer to this document                                                                                 |
| `/methodology`             | add `spreadExtremePct` and `priceDivergencePct` to `thresholds`                                                                      |

## 9. Version history

| Version     | Change                                                                                                                                                                                                         |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.0.0-draft | Ten initial flags; band as the worst triggered flag                                                                                                                                                            |
| 1.0.1-draft | `SPREAD_EXTREME` added after the fixture showed `P0` losing meaning at extreme spreads                                                                                                                         |
| 1.0.2-draft | `MANIPULATION_CHEAP` and `MANIPULATION_RATIO_LOW` require `Reachable == true`. The `unevaluated` state and `bandConfidence` added after the fixture showed six flags could not be judged from a snapshot alone |
| 1.0.3-draft | `PRICE_SOURCE_CONFLICT` added after verification found an honest pool priced 50x away from the book mid. `MANIPULATION_CHEAP` and `MANIPULATION_RATIO_LOW` evaluated on the `orderbookOnly` variant            |
