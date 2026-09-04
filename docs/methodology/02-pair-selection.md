# Keel: Quote Asset and Pair Selection

**Methodology version:** 1.0.8-draft
**Status:** WORKSHEET. The decisions below are unmade. Do not ship this file with blanks.

Every figure Keel publishes is denominated in a quote asset. "The depth of USTRY" is
meaningless until the counter asset is named. This document must state which pair is
measured and why, because a reviewer will ask, and because measuring the wrong pair
answers the wrong question.

Each section states the constraints that bound the answer, then leaves the decision to be
written. Fill it, then delete the "unmade" marker above.

---

## 1. Which quote asset

**Constraints that bound this decision**

- A Stellar asset may trade against many counter assets at once. XLM and USDC are the
  common ones, but nothing forbids others.
- On the incident, the oracle read the **USTRY/USDC** market. Measuring USTRY/XLM would
  have answered a question nobody was asking.
- Thresholds in `09-flags-and-bands.md` are absolute values in the quote asset. If assets
  are measured against different quotes, their bands are not comparable, and an asset's
  band can move purely because the XLM price moved. This is open question Q7.
- Denominating everything in USDC embeds an assumption that USDC is stable, which is
  awkward for a product whose premise is questioning price assumptions.

**Questions to answer**

1. Is there a single global quote asset, or is it chosen per asset?
2. If per asset, what rule chooses it?
3. How is the Q7 comparability problem handled in the meantime?

**Decision**

> _to be written_

**Rationale**

> _to be written_

---

## 2. Multiple pairs for one asset

**Constraints**

- An attacker uses the cheapest available path. Ignoring a secondary pair makes Keel
  optimistic, and optimism is the failure mode this product exists to prevent.
- Computing every pair for 50 assets multiplies the Horizon request budget. Section 6.4
  of the technical design allocates 3 requests per asset per scan against a ceiling of
  3000 per hour.
- The API contract currently returns one `quote` per response, with an optional `?quote=`
  parameter.

**Questions to answer**

1. Are all pairs with any liquidity computed, or only the primary?
2. What rule designates the primary pair?
3. Are secondary pairs reported, and if so where?
4. If an asset is safe on its primary pair and dangerous on a secondary one, what band
   does the asset carry?

**Decision**

> _to be written_

**Rationale**

> _to be written_

---

## 3. The backtest pair

**Constraint**

The oracle read USTRY/USDC. This is not a free choice.

**Decision**

> _to be written, with the reason stated explicitly rather than assumed_

---

## 4. Path payments through intermediate assets

**Constraints**

- Stellar routes path payments across multiple books and pools in a single operation.
  This was observed directly in the pool effects: `trade` and `liquidity_pool_trade`
  interleaved within one operation.
- True effective liquidity is therefore larger than any single pair suggests.
- Implementing path finding is out of scope for the 30 day sprint.

**Questions to answer**

1. Is this stated as a known limitation, or partially approximated?
2. In which direction does ignoring it bias the result, and is that direction safe?

**Decision**

> _to be written_

---

## 5. Asset selection for the demonstration set

**Constraints**

- The SOW promises at least 50 active Stellar assets.
- A reviewer will ask why these 50 and not others.
- Layer 3 of the validation protocol requires 8 recorder assets spanning the liquidity
  range, and a demonstration set consisting only of healthy assets never exercises the
  code paths that matter most.

**Questions to answer**

1. What are the inclusion criteria, stated before the list is built?
2. Is the set balanced across the liquidity range, or is it the top 50 by some measure?
3. Are known-dangerous assets included deliberately?

**Decision**

> _to be written_

---

## 6. Checklist before this file ships

- [ ] Every "to be written" replaced
- [ ] Each decision has a stated reason, not only a stated choice
- [ ] Q7 either resolved or explicitly deferred with its consequence stated
- [ ] The selection criteria were written before the 50 asset list was built
- [ ] Someone outside the team can predict, from this file alone, which pair Keel will
      report for an asset they name

## 7. Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Worksheet created. No decisions recorded yet |
| 1.0.8-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** `07` had run to 1.0.8-draft alone; Al ratified one version for the whole set so that a reader cannot cite two. README section 4 and DEC-014 carry the reasoning |
