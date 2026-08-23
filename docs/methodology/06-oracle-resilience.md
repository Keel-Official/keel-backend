# Keel: Oracle Resilience and Arbitrage Asymmetry

**Methodology version:** 1.0.3-draft
**Status:** partial. The VWAP window length is an assumption and is item 6 of the handoff.

Moving the marginal price is not the same as moving what an oracle reports. This file
covers the gap: the volume an averaging oracle forces an attacker to outweigh, and the
reason the market's own correction mechanism never arrived.

---

## 1. The VWAP window term

The oracle that was manipulated is VWAP-based, not last-price. Moving the marginal price
is not enough on its own; the attacker's trade must also dominate the volume-weighted
average.

```
MR(P_target, W) = MC_orderbookOnly(P_target) + V_genuine(W)
```

The second term is the invisible defence. An actively traded market forces the attacker
to outweigh real volume. A market with no trading has no such defence. On 22 February
both terms were zero or near zero at the same time.

`W` is a parameter. The 15-minute default follows Script3's statement that no other trade
occurred within 15 minutes before the manipulation. That figure is **not confirmed** as
Reflector's actual window and is marked as an assumption.

---

## 2. Arbitrage asymmetry

**New in 1.0.3.** Explains why the attack was not merely cheap, but also went uncorrected
for 22 hours.

State at the time of the attack:

| Source   | Price       | Character                       |
| -------- | ----------- | ------------------------------- |
| Pool     | 1.0555442   | executable in both directions   |
| Book bid | 1.0570000   | executable, size 0.0001 USTRY   |
| Book ask | 106.7372828 | executable, but nobody wants it |

The only arbitrage candidate was buying from the pool at 1.0555 and selling into the bid
at 1.0570, a spread of 0.14 percent, below the pool fee of 0.30 percent. Not profitable.

The ask at 106.74 creates no opportunity at all, because nobody is offering to **buy** at
that price.

> Arbitrage only corrects mispricing that is **executed**. It does not correct mispricing
> that is merely **quoted** or **reported**. An oracle reading quotes or last-trade prices
> is reading precisely the part of the market that no mechanism defends.

The consequence: depth measures executable liquidity, which is exactly the part of the
market arbitrage protects. The gap between the quoted price and the depth-supported price
is the attack surface.

---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
