# Keel: Principles and Limitations

**Methodology version:** 1.2.0-draft
**Status:** complete, and deliberately unflattering.

This is the section a reviewer looks for first. It is honest, and it stays that way.
Every limitation below either has evidence behind it or is named as an assumption.

---

## 1. Conservative principle

In every ambiguous case, choose the interpretation that yields lower depth and a higher
risk assessment.

---

## 2. Known limitations

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
8. **Known removals repair only what was seen.** When an offer leaves the book without
   emitting an event, the historical fold keeps a phantom value until it is told
   otherwise. The known-removals mechanism (`configs/known-removals.json`, DEC-021)
   corrects this only where evidence exists that a specific offer left — that is, only for
   offers a scan or a trade happened to touch. An offer that left silently, and that
   nothing ever looked at, keeps its phantom value, and the reconstruction cannot know it
   is there. The correction only ever removes depth, never adds it, so it stays on the
   conservative side of principle 1 — but it is a floor on known removals, not a guarantee
   that the book is clean.

---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
| 1.0.8-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** `07` had run to 1.0.8-draft alone; Al ratified one version for the whole set so that a reader cannot cite two. README section 4 and DEC-014 carry the reasoning |
| 1.1.0-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** Al resolved Q7 in `02-pair-selection.md` section 1: the quote asset is global and it is USDC, so every absolute threshold is a USDC figure. Under the one-version rule of DEC-014 the whole set moves with the one file whose content changed. README section 4 and DEC-015 carry the reasoning |
| 1.2.0-draft | 2026-09-14 | Added limitation 8 (known removals repair only what was seen) per DEC-021. |
| 1.2.0-draft | 2026-09-25 | Header corrected from `1.2.0` to `1.2.0-draft`. The suffix had been dropped without a recorded decision, and the thresholds remain chosen rather than calibrated, so `-draft` stays. No content change. DEC-014 section 10. |
