# Keel: Risk Flags and Bands

**Methodology version:** 1.0.2-draft
**Supersedes:** PRD sections 5.1 and 5.2, which now simply point here
**Implemented in:** `internal/depth`

Flag definitions used to live in the PRD while also being touched on in the
methodology. Two homes for one definition guarantee that both drift. From this
version on, this document is the single source of truth and the PRD only points
here.

---

## 1. Why rules and not a weighted score

Keel does not publish a weighted 0 to 100 score.

A weighted score obliges its author to justify every weight, and there is no
empirical basis for justifying weights from a single incident. Rule based
classification only obliges a justification of the thresholds, and every flag can
be inspected separately by a consumer with their own policy.

For that reason **flags are always reported individually in the API**, not just
the band derived from them. A consumer who disagrees with Keel's thresholds can
still use the raw flags.

---

## 2. The three states of every flag

This is an important correction in version 1.0.2. Previously a flag had only two
states, and that made an asset with incomplete data look safer than it was.

| State | Meaning |
| --- | --- |
| `triggered` | the condition is met |
| `clear` | the condition was checked and is not met |
| `unevaluated` | the data needed is unavailable, the condition could not be checked |

`unevaluated` is **not** a synonym for `clear`. An asset with no trustline data
must not look the same as an asset whose holder distribution was examined and
found safe.

Consequences in the output:

- `flags` holds the flags that are `triggered`
- `unevaluatedFlags` holds the flags that could not be checked
- `bandConfidence` is `partial` when any flag at the CRITICAL or HIGH level is
  `unevaluated`, and `full` when every flag at those two levels could be checked

The dashboard is required to display `bandConfidence`. A `LOW` band with `partial`
confidence is a far weaker statement than `LOW` with `full` confidence, and that
difference must not be hidden.

---

## 3. The data each flag needs

| Flag | Needs |
| --- | --- |
| `NO_EXECUTABLE_PRICE` | a book and pool snapshot |
| `ZERO_DEPTH_2PCT` | a book and pool snapshot |
| `SPREAD_EXTREME` | a book snapshot |
| `THIN_DEPTH_5PCT` | a book and pool snapshot |
| `MANIPULATION_CHEAP` | a book and pool snapshot |
| `MANIPULATION_RATIO_LOW` | a snapshot and circulating supply |
| `NO_GENUINE_TRADE_7D` | trade history |
| `NO_GENUINE_TRADE_30D` | trade history |
| `WASH_TRADE_SUSPECTED` | trade history |
| `HOLDER_CONCENTRATION_HIGH` | trustline distribution |
| `HOLDER_CONCENTRATION_EXTREME` | trustline distribution |

The first five can be evaluated from a `Snapshot` alone. The other six need extra
input and become `unevaluated` when that input is absent.

---

## 4. Flag definitions

Every threshold refers to `Thresholds` in `types.go` and is expressed in the
**quote asset**, not in USD.

### CRITICAL level

**`NO_EXECUTABLE_PRICE`**
```
priceSource == none
```
No orderbook and no pool. The asset has no executable price at all.

**`ZERO_DEPTH_2PCT`**
```
depth(0.02).BuySide == 0  OR  depth(0.02).SellSide == 0
```
One side alone is enough. An asset that cannot be sold is as dangerous as an asset
that cannot be bought.

**`MANIPULATION_CHEAP`**
```
THERE EXISTS a delta d such that:
    Reachable(d) == true  AND  Cost(d) < Thresholds.ManipulationCheapAbsolute
```

**The `Reachable == true` requirement is the version 1.0.2 correction and must not
be dropped.**

The reason is concrete. On the USTRY fixture, `Cost` is 130.0627093 for δ = 1, 10,
and 100, but all three are `Reachable = false` because no ask exists above
106.7372828. Without this requirement those three rows would be assessed, when
what they actually prove is that an attack to a price that high is **impossible**.
Marking an impossible state as "cheap" is the opposite of the truth.

Conversely `Cost(δ=0.5) = 0` with `Reachable = true` on the same fixture is the
most dangerous condition that can exist, and that is precisely what must be
caught.

### HIGH level

**`MANIPULATION_RATIO_LOW`**
```
THERE EXISTS a delta d such that:
    Reachable(d) == true
    AND  Cost(d) / circulating_supply_value < Thresholds.ManipulationRatioLowPct
```
The `Reachable` requirement applies for the same reason as above.

**`SPREAD_EXTREME`**

Both sides of this comparison are in PERCENT, not fractions. A `spreadPct` of
196.08 is compared against a `SpreadExtremePct` of 20.0. If either one were
written as a fraction, this flag would silently never fire and nothing would fail.

```
spreadPct > Thresholds.SpreadExtremePct
    where spreadPct = (best_ask - best_bid) / P0 × 100
```
A new flag in version 1.0.2. When the spread reaches hundreds of percent, `P0` and
every metric derived from it lose their meaning. On the USTRY fixture the value is
196.08 percent, which means a reference price of 53.90 for an asset worth about
1.06.

Other flags do also fire on that case, but that is a coincidence and not the
design. `spreadPct` is also reported as a number rather than only as a triggered
status, because its magnitude is informative.

**`NO_GENUINE_TRADE_30D`**
```
no genuine trade within the last Thresholds.GenuineTradeStaleDays days
```

**`HOLDER_CONCENTRATION_EXTREME`**
```
holderTop1Pct > Thresholds.HolderTop1ExtremePct
```

### MEDIUM level

**`THIN_DEPTH_5PCT`**
```
min(depth(0.05).BuySide, depth(0.05).SellSide) < Thresholds.ThinDepth5PctAbsolute
```

**`NO_GENUINE_TRADE_7D`**
```
no genuine trade within the last Thresholds.GenuineTradeWarnDays days
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

The band is the highest level among the `triggered` flags. No weighting, no
averaging, no summing.

| Band | Triggered when a flag exists at level |
| --- | --- |
| `CRITICAL` | CRITICAL |
| `HIGH` | HIGH |
| `MEDIUM` | MEDIUM |
| `LOW` | no flag triggered |

`bandConfidence` is determined separately, per section 2.

---

## 6. Default threshold values

| Threshold | Default | Unit |
| --- | --- | --- |
| `ManipulationCheapAbsolute` | 10,000 | quote asset |
| `ManipulationRatioLowPct` | 1.0 | percent |
| `ThinDepth5PctAbsolute` | 50,000 | quote asset |
| `SpreadExtremePct` | 20.0 | percent |
| `HolderTop1ExtremePct` | 50.0 | percent |
| `HolderTop10HighPct` | 80.0 | percent |
| `WashTradeSuspectedPct` | 50.0 | percent |
| `GenuineTradeStaleDays` | 30 | days |
| `GenuineTradeWarnDays` | 7 | days |

**Every one of these values is chosen, not calibrated against a set of
incidents.** Calibration requires more events than are available. That statement
is required to appear at the `/methodology` endpoint, in the dashboard, and in the
backtest report, not only in this document.

### An unresolved unit limitation

Absolute thresholds are expressed in the quote asset. The consequence is that an
asset measured against XLM and an asset measured against USDC cannot be compared
against the same threshold, and an asset's band can change purely because the XLM
price moved, without its liquidity changing at all.

Expressing the thresholds in USDC only relocates the problem, because it imports
the assumption that USDC is stable, which is somewhat ironic for a product built
to question price assumptions.

Version 1.0.2 does not resolve this. What is done instead: `quote` is included in
every response so a consumer knows which unit applies, and this limitation is
stated openly. This is open question Q7 and it must be resolved before version
1.1.

---

## 7. A verified example: USTRY/USDC, ledger 61340263

Taken from `testdata/fixtures/ustry_pre_exploit.md`, computed by hand before any
implementation existed.

Input: one ask of 1.2185312 USTRY at 106.7372828, one bid of 0.0001 USTRY at
1.057, no pool. `P0 = 53.8971414`, `spreadPct = 196.08%`.

**Triggered**

| Flag | Level | Reason |
| --- | --- | --- |
| `ZERO_DEPTH_2PCT` | CRITICAL | depth at ±2% is zero on both sides |
| `MANIPULATION_CHEAP` | CRITICAL | `Cost(δ=0.5) = 0` with `Reachable = true` |
| `SPREAD_EXTREME` | HIGH | 196.08% passes 20% |
| `THIN_DEPTH_5PCT` | MEDIUM | depth at 5% is zero |

**Clear**

`NO_EXECUTABLE_PRICE`, because `priceSource = book`.

**Unevaluated**

`MANIPULATION_RATIO_LOW`, `NO_GENUINE_TRADE_30D`, `NO_GENUINE_TRADE_7D`,
`WASH_TRADE_SUSPECTED`, `HOLDER_CONCENTRATION_EXTREME`, and
`HOLDER_CONCENTRATION_HIGH`. All six need supply data, trade history, or trustline
distribution, none of which is in the snapshot.

**Result**

```
band            = CRITICAL
bandConfidence  = partial
```

`partial` because `MANIPULATION_RATIO_LOW` and `HOLDER_CONCENTRATION_EXTREME` sit
at the HIGH level but are `unevaluated`. The band stays `CRITICAL` because two
CRITICAL flags are already triggered, so the incomplete data does not change the
conclusion in this case.

---

## 8. Changes that came with this version

| File | Change |
| --- | --- |
| `internal/domain/types.go` | added `UnevaluatedFlags []Flag` and `BandConfidence` to `AssetRisk` |
| `docs/api/keel-openapi.yaml` | add `unevaluatedFlags`, `bandConfidence`, `spreadPct`, `SPREAD_EXTREME` |
| PRD section 5 | replace its contents with a pointer to this document |
| `/methodology` | add `spreadExtremePct` to `thresholds` |

## 9. Version history

| Version | Change |
| --- | --- |
| 1.0.0-draft | The first ten flags, band defined as the worst triggered flag |
| 1.0.1-draft | `SPREAD_EXTREME` added after the fixture showed `P0` losing its meaning at an extreme spread |
| 1.0.2-draft | `MANIPULATION_CHEAP` and `MANIPULATION_RATIO_LOW` now require `Reachable == true`. The `unevaluated` state and `bandConfidence` added after the fixture showed that six flags cannot be assessed from a snapshot alone |
