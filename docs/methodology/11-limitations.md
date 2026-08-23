# Keel: Principles and Limitations

**Methodology version:** 1.0.3-draft
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

---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
