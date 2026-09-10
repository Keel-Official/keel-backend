# Manual verification: MANIPULATION_RATIO_LOW on AQUA/USDC

Computed by hand to verify `internal/domain/flags.go` against
`docs/methodology/09-flags-and-bands.md` section 4 (HIGH tier), following the
same "computed by hand before trusting the implementation" convention as
`testdata/fixtures/ustry_pre_exploit.md`.

**Threshold in force:** `ManipulationRatioLowPct = 0.1` (percent).

## Inputs

| Quantity | Value | Source |
| --- | --- | --- |
| Manipulation cost — `orderbookOnly`, cheapest delta with `Reachable == true` | 24,121 USDC | manipulation cost curve (snapshot) |
| Circulating supply | `<<FILL FROM HOLDER PULL — raw AQUA units>>` | trustline distribution pull |
| Price `P0` | `<<FILL FROM SNAPSHOT — USDC per AQUA>>` | snapshot |

## Computation

```
supply_value = circulating_supply × price
             = <<supply>> × <<price>>
             ≈ 27,800,000 USDC         ← cross-check: must match the code's measured supply value

ratio_pct    = cost / supply_value × 100
             = 24,121 / 27,800,000 × 100
             = 0.0868 %
```

## Decision

```
0.0868 % < 0.1 %  →  MANIPULATION_RATIO_LOW = triggered
```

`Reachable == true` holds for the delta whose cost is used, so the flag is not
suppressed (section 4). A 27.8M-USDC market moved for ~24k USDC — the same
"large asset, trivial relative cost" pattern as VELO, just less extreme.