# Keel methodology: index and status

**Methodology version in force:** `1.0.2-draft`
**In sync with:** `internal/domain.MethodologyVersion`

This file is a map. It contains no definitions of its own, so that it cannot
become a second place that diverges from the first.

---

## 1. Sources of truth

| File | Contents | Applies to |
| --- | --- | --- |
| `keel-methodology-core.md` | the computed quantities: `P0`, `spreadPct`, SDEX and AMM depth, the combination, manipulation cost, `Reachable`, `MaxReachablePrice`, oracle resistance, `C_max`, empirical validation, limitations | `internal/depth` |
| `09-flag-dan-band.md` | flag definitions, the three flag states, band derivation, `bandConfidence`, every threshold value | `internal/depth` |

Where the two disagree about flags, bands, or thresholds, `09-flag-dan-band.md`
wins. Where they disagree about a computed quantity, `keel-methodology-core.md`
wins.

The golden fixture and all of its expected values live in
`testdata/fixtures/ustry_pre_exploit.md`, computed by hand before any
implementation existed, and translated into Go in `internal/conformance/`.

---

## 2. Unit conventions

Two different conventions live side by side on purpose. Mixing them is a source of
silent bugs that fail nothing.

| Form | Example | Meaning |
| --- | --- | --- |
| `δ`, an input to a formula | `δ = 0.02` | a fraction, meaning 2 percent |
| ending in `Pct`, a reported quantity | `spreadPct = 196.0777141` | percent, meaning 196 percent |

`SpreadExtremePct` is 20.0 and is compared directly against `spreadPct`. The
archived `docs/internal/memo-pra-development.md` section 1.2 writes it as 0.20, a
fraction. That is wrong and has been corrected there.

---

## 3. Against the Deliverable 1 Definition of Done

The DoD in `docs/internal/Keel_Deliverable_1_Rencana_Eksekusi.md` section 6
promises **eleven files** in this folder, numbered `00` through `10`. The current
structure differs, and that difference has to be decided rather than left alone.

| Promised by the DoD | What exists now | Status |
| --- | --- | --- |
| `00-ikhtisar.md` | `keel-methodology-core.md` sections 1 and 2 | content exists, different name |
| `01-sumber-data.md` | spread across TDD section 3 and `DEC-002` | **not written** as methodology |
| `02-harga-acuan.md` | `keel-methodology-core.md` sections 3 and 3a | content exists, different name |
| `03-depth-sdex.md` | `keel-methodology-core.md` section 4 | content exists, different name |
| `04-depth-amm.md` | `keel-methodology-core.md` section 5 | content exists, different name |
| `05-penggabungan.md` | `keel-methodology-core.md` section 6 | content exists, different name |
| `06-pemilihan-pasangan.md` | decision D-1 in the execution plan | **not written** as methodology |
| `07-metrik-pendukung.md` | decisions D-4 through D-6 in the execution plan | **not written** as methodology |
| `08-collateral.md` | `keel-methodology-core.md` section 9 | content exists, but the origin of the default parameters is not yet traced to Blend's real parameters |
| `09-validasi.md` | does not exist, and the number 09 is already taken by `09-flag-dan-band.md` | **not written**, and the **number collides** |
| `10-keterbatasan.md` | `keel-methodology-core.md` section 12 | content exists, different name |

There is no file for flags and bands in the DoD list at all, even though
`09-flag-dan-band.md` exists and holds binding definitions. The DoD numbering no
longer describes reality.

### The decision that has to be made

There are two valid roads. What is not valid is leaving it as it is, because this
DoD is what an SCF Build reviewer will read.

1. **Split to match the DoD.** `keel-methodology-core.md` is broken into ten
   numbered files, `09-flag-dan-band.md` is renumbered, and the three missing
   files are written. This is expensive and `09-flag-dan-band.md` is already
   referenced from six places.
2. **Amend the DoD.** The current structure is kept, DoD section 6 is rewritten to
   describe the real structure, and the three missing pieces of content are still
   required wherever they end up living.

Recommendation: road 2. A reviewer reads content, not file names, and the four real
gaps (data sources, pair selection, supporting metrics, and the validation
protocol) still have to be filled on either road. Splitting files removes none of
them and only adds renumbering work.

Note that file names are not being translated under DEC-005 either, for the same
reason: `09-flag-dan-band.md` is referenced by path from six documents.

---

## 4. Version history

It lives in `keel-methodology-core.md` section 13 and `09-flag-dan-band.md`
section 9. Both must be raised together.
