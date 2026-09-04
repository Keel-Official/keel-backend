# Keel: Manipulation Cost

**Methodology version:** 1.0.8-draft
**Status:** complete. Split into two venue forms in 1.0.3.

This is the number the case study turns on. An attacker does not pay for the trade
that moves the price, they pay for the third-party liquidity they have to consume on
the way to it, and on 22 February 2026 that was zero.

---

## 1. Definition

```
MC(P_target)        = Σ notional of asks with price < P_target
Reachable(P_target) = there EXISTS an ask with price ≥ P_target
```

An attacker must consume every ask **cheaper** than the target, then barely touch the
first ask sitting **above** it. That final touch is what sets the price the oracle reads,
and it can cost arbitrarily little.

The two sets are disjoint and exhaustive. An ask never falls into both.

| State                            | Meaning                                                   |
| -------------------------------- | --------------------------------------------------------- |
| `Cost` small, `Reachable = true` | **most dangerous**                                        |
| `Cost` large, `Reachable = true` | expensive; the market has a defence                       |
| `Reachable = false`              | the target cannot be reached at any capital. Not bad news |

`Cost` is an **upper bound**. Keel cannot know which orders belong to the attacker ahead
of time, so it does not filter them out. This bias points in the safe direction.

## 2. Why order ownership is decisive

A mistake that nearly entered this methodology and was caught only by the data.

The wrong intuition treats the size of the manipulating trade as its cost. On 22 February
that trade was 5.3475699 USDC, but the offer it executed against belonged to the attacker,
so the money returned to the attacker.

The real cost is the value of **third-party** orders that must be consumed on the way to
the target. Because Stellar's matching engine always fills from the best price, this is
exactly the buy-side depth up to `P_target`.

## 3. Two forms, two different questions

**Added in 1.0.3** after verification showed an honest pool present throughout the attack.

| Metric                          | Contents     | Answers                                          |
| ------------------------------- | ------------ | ------------------------------------------------ |
| `manipulationCostCombined`      | SDEX and AMM | cost of moving the actual market price           |
| `manipulationCostOrderbookOnly` | SDEX only    | cost of fooling an oracle that reads SDEX trades |

On the USTRY fixture, reaching 106.7372828 costs roughly 147.96 USDC through the pool and
**zero** through the order book. The attacker paid zero.

`orderbookOnly ≤ combined` always holds, since combined adds the AMM term. An attacker
takes the cheapest path, so `orderbookOnly` is the binding figure. The gap between the two
is itself a signal: an asset with a large `combined` and a small `orderbookOnly` looks
safe while it is not.

The claim that the oracle reads only SDEX trades is an **inference** drawn from the fact
that an honest pool existed and the attack still succeeded. It stays marked as an
inference until confirmed.

## 4. The large-delta ladder

Depth at ±2/5/10 percent measures market quality. It is **not sufficient** for oracle
resilience, because the attacker moved the price by a factor of 100.98, not by 10 percent.

| Ladder                  | δ values         | Purpose             |
| ----------------------- | ---------------- | ------------------- |
| Market quality          | 0.02, 0.05, 0.10 | required by the SOW |
| Manipulation resilience | 0.5, 1, 10, 100  | cost of an attack   |

## 5. Maximum reachable price

```
MaxReachablePrice       = the highest ask price in the book
CostToMaxReachablePrice = Σ notional of asks with price < MaxReachablePrice
```

This pair captures attacks that fall between two rungs of the delta ladder. On the USTRY
fixture the values are 106.7372828 at zero cost, while the actual attack landed between
δ=0.5 and δ=1.

**When an active pool is present, both are null**, accompanied by a warning. Under a
constant product curve the price tends to infinity as the base reserve tends to zero, so
every target is reachable and the notion of a highest price loses meaning. Both fields
are meaningful only for pure order book markets.

---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
| 1.0.8-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** `07` had run to 1.0.8-draft alone; Al ratified one version for the whole set so that a reader cannot cite two. README section 4 and DEC-014 carry the reasoning |
