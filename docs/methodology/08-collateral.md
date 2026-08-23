# Keel: Maximum Safe Collateral Size

**Methodology version:** 1.0.3-draft
**Status:** complete for 1.0.x. A better form with no chosen constant is a candidate for 1.1.

`C_max` is the only number in this methodology that tells a lender what to do rather
than what is true. It is the minimum of two limits, and both of them have to be
reported, because which one binds is the part that decides the response.

---

## 1. Definition

```
δ_critical = Params.ManipulationCriticalDelta          (default 0.5)

if Reachable_orderbookOnly(δ_critical):
    C_max = min( D_sell(δ_liquidation) × h , MC_orderbookOnly(δ_critical) × m )
else:
    C_max = D_sell(δ_liquidation) × h
    warning "manipulation to δ_critical is unreachable through the order book;
             the manipulation term was not applied"
```

| Symbol          | Meaning                     | Default |
| --------------- | --------------------------- | ------- |
| `δ_liquidation` | liquidation discount        | 0.10    |
| `h`             | liquidation haircut         | 0.5     |
| `δ_critical`    | critical manipulation delta | **0.5** |
| `m`             | manipulation safety margin  | 0.25    |

**Why `δ_critical = 0.5`, changed in 1.0.3.** Because `MC` is monotonically increasing in
δ and `Reachable` is monotonically decreasing, a lower value always yields a tighter bound
while relying less often on an unreachable target. On the USTRY fixture, δ=1 produces
`Reachable = false` with `Cost = 130.0627093`, so the manipulation term would yield a
positive collateral allowance derived from an **impossible** attack. At δ=0.5 the result
is zero, which is correct. A 50 percent price inflation is already more than enough to
push a position under water at any sane LTV.

**Why the `Reachable` guard.** When the target is unreachable, `MC` is not the cost of
reaching anything, and multiplying it by `m` produces a meaningless number.

**Why `orderbookOnly`.** An attacker takes the cheapest path, and `orderbookOnly` is
always less than or equal to `combined`. Using the smaller figure is the conservative
choice.

Both terms must be reported separately, not only their minimum. Every parameter is
caller-configurable.

Default values are **chosen, not calibrated**. That statement must appear on the dashboard
and in API responses.

**A more correct form, candidate for v1.1.** Fixing a single δ remains arbitrary. The
form with no chosen constant lets the attacker pick their best δ:
`C_max_manipulation = min over Reachable δ of [ MC(δ) / ((1 + δ) × LTV − 1) ]`.
Deferred out of 1.0.x because it changes the shape of `C_max` and introduces an `LTV`
parameter.

---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
