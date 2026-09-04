# Keel: Overview and Notation

**Methodology version:** 1.0.8-draft
**Status:** complete. Definitions locked, thresholds not calibrated.

This document states the question Keel answers and the notation every other file in
this folder uses. Every definition here must be defensible without reference to the
code. Where the code and these documents disagree, the documents are right and the
code has a bug.

**Split of authority.** This folder is the source of truth, one file per subject, and
the file that owns a subject wins where two files touch it. Flags, bands, and every
threshold belong to `09-flags-and-bands.md`. Two homes for one definition guarantee
that both drift, so the split has to be kept sharp.

**Validated against:** the YieldBlox DAO pool incident on Blend V2, 22 February 2026.
The full case is in `10-validation.md`.

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

**The percent convention.** Every quantity and threshold whose name ends in `Pct` is
expressed in PERCENT, not as a fraction. A `spreadPct` of 196.0777141 means 196 percent
and is compared directly against a `SpreadExtremePct` of 20.0. Conversely `δ` is always a
fraction, so `δ = 0.02` means 2 percent. The two conventions differ deliberately: `δ` is an
input to a formula, `Pct` is a reported quantity.

---

## 3. Where everything else lives

| File | Subject |
|---|---|
| `01-data-sources.md` | where every number comes from, and how each source fails silently |
| `02-pair-selection.md` | which quote asset and which pair Keel reports for an asset |
| `03-reference-price.md` | `P0`, the price source ladder, `spreadPct`, price divergence |
| `04-depth.md` | SDEX depth, AMM depth, and the rule that combines them |
| `05-manipulation-cost.md` | `MC`, `Reachable`, the two venue forms, `MaxReachablePrice` |
| `06-oracle-resilience.md` | the VWAP window term, and why arbitrage does not correct quotes |
| `07-supporting-metrics.md` | genuine trades, holder concentration, volume to supply |
| `08-collateral.md` | `C_max` and its two terms |
| `09-flags-and-bands.md` | every flag, every band, every threshold value |
| `10-validation.md` | the three validation layers and the 22 February 2026 case |
| `11-limitations.md` | the conservative principle and what Keel cannot see |


---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
| 1.0.8-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** `07` had run to 1.0.8-draft alone; Al ratified one version for the whole set so that a reader cannot cite two. README section 4 and DEC-014 carry the reasoning |
