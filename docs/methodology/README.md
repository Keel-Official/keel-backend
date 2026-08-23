# Keel methodology: index and status

**Methodology version in force:** `1.0.3-draft`
**In sync with:** `internal/domain.MethodologyVersion`

This file is a map. It carries no definitions of its own, so that it cannot become a
second home for one, with the single exception of the consolidated version history in
section 4, which is a record rather than a definition.

---

## 1. The files, and who owns what

One subject per file. The file that owns a subject wins wherever two files touch it.

| File | Subject | Status |
|---|---|---|
| `00-overview.md` | the question Keel answers, notation, units, and this list | complete |
| `01-data-sources.md` | where every number comes from, how each source fails silently, trade-implied bounds | complete |
| `02-pair-selection.md` | quote asset and pair selection, path payment limits | worksheet, no decisions recorded yet |
| `03-reference-price.md` | `P0`, the price source ladder, price divergence, `spreadPct` | complete |
| `04-depth.md` | SDEX depth, AMM depth, and the rule that combines them | complete |
| `05-manipulation-cost.md` | `MC`, `Reachable`, the two venue forms, `MaxReachablePrice` | complete |
| `06-oracle-resilience.md` | the VWAP window term, arbitrage asymmetry | partial, the window length is an assumption |
| `07-supporting-metrics.md` | genuine trades, holder concentration, volume to supply | worksheet, no definitions recorded yet |
| `08-collateral.md` | `C_max` and its two terms | complete for 1.0.x |
| `09-flags-and-bands.md` | every flag, every band, every threshold value | complete |
| `10-validation.md` | the three validation layers, and the 22 February 2026 case | protocol complete, execution not started |
| `11-limitations.md` | the conservative principle, and what Keel cannot see | complete |

The golden fixture and all of its expected values live in
`testdata/fixtures/ustry_pre_exploit.md`, computed by hand before any implementation
existed, and translated into Go in `internal/conformance/`.

---

## 2. Unit conventions

Two different conventions live side by side on purpose. Mixing them is a source of
silent bugs that fail nothing.

| Form | Example | Meaning |
| --- | --- | --- |
| `δ`, an input to a formula | `δ = 0.02` | a fraction, meaning 2 percent |
| ending in `Pct`, a reported quantity | `spreadPct = 196.0777141` | percent, meaning 196 percent |

`SpreadExtremePct` is 20.0 and is compared directly against `spreadPct`. The archived
`docs/internal/memo-pra-development.md` section 1.2 writes it as 0.20, a fraction. That
is wrong and has been corrected there.

This convention was briefly broken in the code and not in the documents. Methodology
1.0.3 shipped `domain.DefaultParams()` with `SpreadExtremePct` at 0.20 and
`PriceDivergencePct` at 0.10, both fractions, while section 6 of `09-flags-and-bands.md`
defined both in percent. On the golden fixture `spreadPct` is 196.08, so both scales
trigger and the disagreement was invisible. The rule that resolved it is the one at the
top of `00-overview.md`: the documents are right and the code has the bug.

---

## 3. Against the Deliverable 1 Definition of Done

**Road 1 was taken on 23 August 2026.** `keel-methodology-core.md` is gone, split into
the numbered files in section 1, and `09-flag-dan-band.md` was renamed to
`09-flags-and-bands.md`. This section used to argue the decision; it now records it.

The DoD in `docs/internal/Keel_Deliverable_1_Rencana_Eksekusi.md` section 6 promises
eleven files numbered `00` through `10`, with Indonesian names, and with `09` assigned to
validation. Three things about that promise did not survive contact with the content:

1. **The names are English**, under DEC-005, and file names were the one thing DEC-005
   originally left alone. That exemption is now spent, and DEC-005 records why.
2. **`09` is flags and bands, not validation.** Validation is `10`. The flag document
   already held binding definitions and was referenced by path from sixteen places, so
   moving it would have cost more than moving validation.
3. **Eleven files were not enough.** Limitations needed its own file rather than a
   section, because it is the first thing a reviewer looks for. That makes twelve, `00`
   through `11`.

So the DoD still has to be amended, which was true on either road. What changed is that
it is now amended to describe a numbered structure rather than to abandon numbering.

The mapping, for anyone holding the old file:

| Was | Is now |
|---|---|
| core sections 1 and 2 | `00-overview.md` |
| core section 3 | `03-reference-price.md` section 1 |
| core section 3a, dropped in the 1.0.3 rewrite | `03-reference-price.md` section 2, restored |
| core sections 4, 5, 6 | `04-depth.md` |
| core section 7 | `05-manipulation-cost.md` |
| core sections 8 and 10 | `06-oracle-resilience.md` |
| core section 9 | `08-collateral.md` |
| core section 11 | `10-validation.md` section 7 |
| core section 12 | `01-data-sources.md` section 6 |
| core section 13 | `11-limitations.md` |
| core section 14 | section 4 of this file |

---

## 4. Version history

Consolidated here, because it was the one part of the core file that belonged to the
folder rather than to any subject in it. Each file also carries its own table, recording
when its content moved.

| Version     | Change                                                                                                                                                                                                                                                                                                         |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1.0.0-draft | Initial definitions. Ownership-based manipulation cost. Large-delta ladder. Oracle window volume term                                                                                                                                                                                                          |
| 1.0.1-draft | `Reachable` rule corrected. `SPREAD_EXTREME` added                                                                                                                                                                                                                                                             |
| 1.0.2-draft | `MANIPULATION_CHEAP` requires `Reachable == true`. `unevaluated` state and `bandConfidence` added                                                                                                                                                                                                              |
| 1.0.3-draft | `P0` prefers the pool on large divergence, `PRICE_SOURCE_CONFLICT` added. Manipulation cost split into `combined` and `orderbookOnly`. `MaxReachablePrice` null when a pool is active. `δ_critical` 1.0 to 0.5 with a `Reachable` guard on `C_max`. Arbitrage asymmetry section. Sell-side fee treatment fixed |

Every file in this folder must be raised together. A result produced under one version
cannot be compared with a result produced under another.
