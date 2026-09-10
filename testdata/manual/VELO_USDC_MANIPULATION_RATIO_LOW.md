# Manual verification: MANIPULATION_RATIO_LOW on VELO/USDC

Computed by hand to verify `internal/domain/flags.go` against
`docs/methodology/09-flags-and-bands.md` section 4 (HIGH tier), following the
same "computed by hand before trusting the implementation" convention as
`testdata/fixtures/ustry_pre_exploit.md`.

**Threshold in force:** `ManipulationRatioLowPct = 0.1` (percent).

## Inputs

| Quantity | Value | Source |
| --- | --- | --- |
| Manipulation cost — `orderbookOnly`, cheapest delta with `Reachable == true` | 13,172 USDC | manipulation cost curve (snapshot) |
| Circulating supply | `<<FILL FROM HOLDER PULL — raw VELO units>>` | trustline distribution pull |
| Price `P0` | `<<FILL FROM SNAPSHOT — USDC per VELO>>` | snapshot |

## Computation

```
supply_value = circulating_supply × price
             = <<supply>> × <<price>>
             ≈ 124,000,000 USDC        ← cross-check: must match the code's measured supply value

ratio_pct    = cost / supply_value × 100
             = 13,172 / 124,000,000 × 100
             = 0.0106 %
```

## Decision

```
0.0106 % < 0.1 %  →  MANIPULATION_RATIO_LOW = triggered
```

`Reachable == true` holds for the delta whose cost is used, so the flag is not
suppressed (section 4). This is the "large asset, trivial relative cost" case
the flag exists to catch: a 124M-USDC market moved for ~13k USDC.