# DEC-017: `MANIPULATION_RATIO_LOW` is a percentage, and the threshold is 0.1

**Status:** Accepted
**Date:** 2026-09-10
**Kind:** Definition and threshold. It repairs a rule that could not be implemented as
written, defines a term the methodology used once and never defined, and sets the
threshold value. It changes no other formula and no contract schema.
**Drafted by:** Claude
**Decided by:** Al, 10 September 2026
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. Section 1 is Al's; the measurements in sections 3 and
4 are Claude's and are evidence for it, not part of it.
**Methodology version:** this record moves it. The text change belongs to
`09-flags-and-bands.md` and under DEC-014's one-version rule the whole set moves with it.

---

## 1. The decision

Three parts, and only the third is a judgement.

1. **`circulating_supply_value` is a quote-denominated value**: circulating supply
   multiplied by `P0`. It is defined in `09-flags-and-bands.md` and used nowhere else.
2. **The rule is a percentage comparison** and gains the `* 100` it was missing:

   ```
   THERE EXISTS delta d in ManipulationCostOrderbookOnly such that:
       Reachable(d) == true
       AND  (Cost(d) / circulating_supply_value) * 100 < Thresholds.ManipulationRatioLowPct
   ```
3. **`ManipulationRatioLowPct` is `0.1`**, down from the `1.0` that was never applied.

## 2. What was wrong, and why nothing was implemented against it

The rule at `09-flags-and-bands.md` section 4 read:

```
Cost(d) / circulating_supply_value < Thresholds.ManipulationRatioLowPct
```

The left side is a bare ratio. Section 6 states the threshold's unit as `percent` with
value `1.0`. Those two cannot both be right: read literally the rule fires below a ratio
of 1.0, which is a threshold of 100 per cent, and nobody writes 100 per cent as
"1.0 percent". The two readings differ by a factor of a hundred.

`internal/domain/flags.go` refused to guess. It left the flag `stateUnevaluated` with the
ambiguity written into a comment beside it. That was the right call and it is the reason
this record exists rather than a silent hundredfold error, but it had a cost: the flag is
HIGH tier, and `bandConfidence` degrades to `partial` whenever any HIGH or CRITICAL tier
flag is unevaluated. So **every asset Keel has ever scored carries `partial` confidence**,
and this one rule is why.

**Part 1 of the decision is forced rather than chosen**, and the check is dimensional.
`Cost` is quote-denominated: section 6 lists `ManipulationCheapAbsolute` in "quote asset"
and compares it directly against `Cost`. A quote numerator over a quote denominator is
dimensionless and may be compared against a percentage. A quote numerator over a
base-quantity denominator is a price, and comparing a price against a percentage means
nothing. So the denominator is a value.

**Part 2 is forced by the same table.** Every other `Pct` threshold in section 6 is
compared against a left side already expressed in per cent: `SpreadExtremePct` against
`spreadPct`, `HolderTop1ExtremePct` against `Top1Pct`, which
`internal/domain/supporting.go` line 793 multiplies by 100. This rule was the only one
whose left side was a bare ratio. The defect is a missing `* 100` in the formula, not a
second legitimate reading of the threshold.

## 3. Why 0.1 and not 1.0, measured rather than argued

Measured on 10 September 2026 over the 64-asset scan of ledgers 64344122 to 64344124,
methodology `1.0.8-draft`, with `circulating_supply_value` taken as Horizon's
`balances.authorized` multiplied by the stored `mid_price`. 48 of the 64 could be scored;
the other 16 carry no supply figure, no mid price, or no reachable rung.

**This flag is mostly redundant, and its value is only in what it adds.** Against
`MANIPULATION_CHEAP` at a threshold of 1.0 per cent:

| | Assets |
|---|---|
| both flags fire | 36 |
| `MANIPULATION_CHEAP` alone | 1 |
| **`MANIPULATION_RATIO_LOW` alone** | **4** |
| neither | 7 |

On 36 of 48 the two flags say the same thing, because `Cost(δ=0.5)` is 0 with
`Reachable = true` on 16 assets and under 10,000 quote on many more. The rule earns its
place only on the assets an absolute threshold misses: large assets whose manipulation
cost is high in absolute terms and trivial against their own size.

How that added set moves with the threshold:

| Threshold | Fires, of 48 | Adds over `MANIPULATION_CHEAP` |
|---|---|---|
| 0.01 % | 30 | nothing |
| **0.1 %** | **35** | **VELO, AQUA** |
| 1 % | 40 | VELO, AQUA, BTC, ETH |
| 10 % | 45 | those four plus PYUSD, USTRY, BTCLN, EURC |

**At 0.01 per cent and below the flag adds nothing at all**, so that is the floor: a
threshold there would make the rule dead weight and the honest action would be to delete
it rather than lower it.

The four candidates the choice was made over:

| Asset | Manipulation cost | Supply value | Ratio |
|---|---|---|---|
| VELO `GDM4RQUQ` | 13,171.98 USDC | 124,074,865.64 USDC | 0.0106 % |
| AQUA `GBNZILST` | 24,121.09 USDC | 27,831,290.42 USDC | 0.0867 % |
| BTC `GDPJALI4` | 25,499.03 USDC | 6,183,683.02 USDC | 0.4124 % |
| ETH `GBFXOHVA` | 12,395.01 USDC | 2,063,667.99 USDC | 0.6006 % |

Al chose 0.1, which flags VELO and AQUA and leaves BTC and ETH clear. The judgement is
that a manipulation cost of roughly four tenths of one per cent of an asset's own value
is not yet the condition this HIGH tier flag exists to name.

**The threshold is chosen, not calibrated**, exactly as section 6 of
`09-flags-and-bands.md` says of every value in it. What this record adds over the
previous state is that the choice was made against a measured distribution and against a
named set of assets, rather than inherited from a placeholder.

## 4. The incident asset does not fire, and that is deliberate

USTRY's ratio is 2.35 per cent, so it fires at no threshold below 10 per cent.

That is not a gap. On `testdata/fixtures/ustry_pre_exploit.md` the pre-exploit book has
`Cost(δ=0.5) = 0` with `Reachable = true`, which section 4 of `09-flags-and-bands.md`
itself calls the most dangerous condition that can exist. `MANIPULATION_CHEAP` fires
there, `ZERO_DEPTH_2PCT` fires there, and the fixture's band is already CRITICAL. **This
flag is not what catches the February 2026 incident**, and choosing 0.1 rather than 10
weakens nothing about that detection.

Recorded because the opposite move was available and would have been wrong: a threshold
of 10 per cent would light USTRY up, and it would also fire on 45 of 48 assets. Fitting a
threshold to the one asset already caught by two other flags, at the price of a flag that
says nothing about the other 44, is the trade this record declines.

## 5. What changes, and in what order

**The ordering rule governs this and it is not satisfied yet.** A function may only be
written after its expected values exist as hand computed figures. The golden fixture
cannot supply them here, for a reason that is permanent rather than pending:

- `testdata/fixtures/ustry_pre_exploit.md` line 122 states that `MANIPULATION_RATIO_LOW`
  is "not assessable from the snapshot" and line 127 requires it reported as not
  assessable. That remains true and correct for the snapshot-only path.
- The fixture's ledger is 61340262, February 2026. Horizon serves no historical trustline
  balance and `/assets` is current-state only, so `circulating_supply_value` at that
  ledger **is not obtainable by any route**. The golden fixture can never test this rule.

So the oracle must be created at current state, from a holder pull and a book recorded at
the same ledger. That is a Layer 1 hand recomputation under `testdata/manual/`, and it
depends on the holder pull that the page-cap decision governs. **The holder pull comes
first.**

| # | Step | Owner | Where |
|---|---|---|---|
| 1 | Holder pull across the demonstration set | Al decides the page cap, Claude runs it | `docs/evidences/` |
| 2 | Write the rule and the definition into the methodology | **Al** | `09-flags-and-bands.md` §4, §6 |
| 3 | Hand compute the expected verdict for one asset | **Al** | `testdata/manual/` |
| 4 | Implement the rule | Claude | `internal/domain/flags.go` |
| 5 | Set the threshold constant | Claude | `internal/domain/types.go` |
| 6 | Version bump across the set | Claude | `types.go` line 29, 12 methodology files |

Step 3 is the gate on step 4, and step 1 is the gate on step 3.

**Section 2 of this record is the reason step 3 matters more here than usual.** The rule
was unimplementable for long enough that nothing was ever checked against it, so there is
no existing behaviour to regress against. The hand computation is the only thing that
will have checked the implementation at all.

## 6. What this record does not settle

- **Whether the flag should exist.** At 0.1 per cent it adds two assets out of 48 over a
  flag that already exists. That is a thin justification and it is worth revisiting once
  the demonstration set is rebuilt from `02-pair-selection.md` section 5 criteria rather
  than from the union of two provisional files.
- **The supply figure's own definition.** Section 3's measurement used Horizon's
  `balances.authorized`. `internal/domain.HolderConcentration` computes a different
  figure, the sum of `/accounts` balances after exclusions, which omits pool-held and
  contract-held supply because that endpoint returns only classic `G` addresses. For
  USTRY the two differ by 385 units out of 10.43 million, 0.0037 per cent, so the choice
  did not affect this decision. It is decision D-6 and it stays open.
- **Whether `bandConfidence` reaches `full` after this.** It cannot until the supporting
  metrics are wired into the scan path, which is separate work. This record removes one
  of the six unevaluated flags, not all of them.

## 7. Version history

| Version | Change |
|---|---|
| 1.0.8-draft | the state this record found: rule unimplementable, flag permanently unevaluated, `bandConfidence` `partial` on every asset ever scored |
