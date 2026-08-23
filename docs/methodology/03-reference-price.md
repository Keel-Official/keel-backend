# Keel: Reference Price

**Methodology version:** 1.0.3-draft
**Status:** complete. The `P0` ladder changed in 1.0.3 and the reason is recorded below.

`P0` is the price every other quantity in this methodology is measured against, so a
wrong `P0` is not a wrong number, it is a wrong document. This file defines how it is
chosen, what it means when the two available sources disagree, and the point past which
`P0` stops meaning anything at all.

---

## 1. The price source ladder

**Changed in 1.0.3.** The previous rule always preferred the order book mid whenever both
sides were populated. On the USTRY fixture that rule produced 53.8971414 for an asset
whose real liquidity sat at 1.0555442, off by a factor of 50.

```
1. Pool present AND two-sided book
     divergence = |book_mid − pool_spot| / pool_spot × 100
     if divergence > Thresholds.PriceDivergencePct
         P0 = pool_spot,  priceSource = "pool",  raise PRICE_SOURCE_CONFLICT
     else
         P0 = book_mid,   priceSource = "book"
2. Two-sided book, no pool        -> book_mid,  "book"
3. One-sided book, pool present   -> pool_spot, "pool"
4. Pool only                      -> pool_spot, "pool"
5. Neither book nor pool          -> undefined, "none"
```

Rationale: when two price sources disagree, trust the one backed by executable
liquidity, and **tell the consumer that they disagree**. An order book mid with a spread
of several hundred percent is not executable by anyone. Hiding the conflict is worse
than picking one side of it.

When more than one pool exists, `pool_spot` is taken from the pool with the largest
quote reserve. `poolSpotPrice` and `priceDivergencePct` are always reported whenever a
pool is present, regardless of which branch was taken.

**Case 5 is not an error.** An asset with no executable price is the highest-value
finding Keel can produce. It is reported as a valid result carrying the
`NO_EXECUTABLE_PRICE` flag and band `CRITICAL`.

**A warning drawn from real data.** A book-derived `P0` can be shaped by the attacker's
own orders. On 22 February 2026 the attacker placed a buy order for 0.0001 USTRY at
1.057, one minute after placing the manipulation offer. An order that small contributes
to `P0` while representing no liquidity at all. `P0` must never be read on its own.

---

## 2. Spread, and the limit of `P0`'s meaning

**Restored in the road 1 split.** The 1.0.3 rewrite of the core file dropped the
definition of `spreadPct` while continuing to report it, leaving the quantity defined
only inside the `SPREAD_EXTREME` flag rule. A reported quantity has to be defined where
quantities are defined. `09-flags-and-bands.md` still owns the threshold it is compared
against.

```
spreadPct = (best_ask − best_bid) / P0 × 100
```

`spreadPct` is null when either side of the book is empty, because the difference is
undefined. Null means unknown, not zero.

On 21 February 2026 the USTRY/USDC book held an ask at 106.7372828 and a bid at 1.057,
which puts the mid at 53.8971414 for an asset actually worth about 1.06. That number is
not a bug. It is what a mid price does when the spread runs into the hundreds of
percent, and in 1.0.3 it is also what makes the divergence rule above fire.

The consequence is harsh: **`P0` and every metric derived from it lose their meaning at
an extreme spread.** The ±2/5/10 percent depth ladder is included in that. It is still
reported because the SOW promised it, but on a book that broken it is not an oracle
safety metric. What saves the analysis is the large delta ladder in
`05-manipulation-cost.md` section 4 and the `SPREAD_EXTREME` flag.


---

## Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Split out of `keel-methodology-core.md` under the road 1 decision. Content unchanged except where noted in the section itself |
