# DEC-003: USTRY/USDC Pool at Ledger 61340263

**Status:** VERIFIED from on-chain data
**Date:** August 2026
**Impact:** golden fixture, reference price rule, manipulation cost definition, limitations section

---

## 1. Verification result

```
Pool ID   : 27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb
Fee       : 30 bps
Created   : active at least since 2026-07-01 (earliest indexed operation)

Reserves at ledger 61340263 (2026-02-22T00:10:21Z):
  USDC  : 16.3389179
  USTRY : 15.4791416
  spot  : 1.0555442 USDC per USTRY
  k     : 252.9124238
```

### Provenance

Obtained from `/liquidity_pools/{id}/effects`, walking back from the current state until
past the target ledger, filtered by `liquidity_pool.id`.

The last effect touching this pool before the attack is dated
**2026-02-10T16:59:35Z**, and the next effect is dated **2026-02-22T22:08:33Z**.
Ledger 61340263 falls between the two, so the reserve values from the February 10 effect
apply exactly at the moment of the attack. No arithmetic reconstruction is required.

### Cross-validation

| Check | Result |
|---|---|
| `k` rose from February to now | 252.912 to 256.237, up 3.32. Consistent with 30 bps fee accumulation |
| Reserves read directly from the effect, not computed | yes, so there is no risk of inversion error |
| Pool existed before the incident | yes, operations indexed since 1 July 2025 |

Residual risk worth noting: this method assumes that every pool state change produces an
effect indexed at that endpoint. For deposits, withdrawals, and trades this holds. Rare
cases such as trustline authorization revocation have not been checked separately.

---

## 2. Pool quiet window

| | |
|---|---|
| Last pool effect before the attack | 2026-02-10T16:59:35Z |
| Attack | 2026-02-22T00:10:21Z |
| Next pool effect | 2026-02-22T22:08:33Z |
| Quiet before the attack | 11 days 7 hours |
| Quiet after the attack | 21 hours 58 minutes |
| Total quiet window | 12 days 5 hours |

The pool held an honest price of 1.0555 undisturbed throughout the attack, and for almost
a full day afterward.

---

## 3. Key finding: upward manipulation faces no arbitrage pressure

This explains the quiet window above, and it is sharper than the zero-cost finding.

Markets are assumed to have a self-correcting mechanism in the form of arbitrage. That
mechanism **did not work** in this attack, and the reason is structural.

The book state at the time of the attack:

| Source | Price | Nature |
|---|---|---|
| Pool | 1.0555442 | executable in both directions |
| Bid orderbook | 1.0570000 | executable, size 0.0001 USTRY |
| Ask orderbook | 106.7372828 | executable, but no one wants it |

Arbitrage requires a profitable transaction. The only candidate is to buy from the pool
at 1.0555 then sell into the bid at 1.0570, a spread of 0.14 percent, below the pool fee
of 0.30 percent. Not profitable.

The ask at 106.74 creates no opportunity at all, because no party is offering to **buy**
at that price. An absurdly priced sell order is not an arbitrage opportunity; it is merely
an order no one takes.

**General formulation:**

> Arbitrage only corrects mispricing that is **executed**. It does not correct mispricing
> that is merely **quoted** or **reported**. An oracle that reads quotes or last-trade
> prices is reading exactly the part of the market that no mechanism defends.

This complements the zero-cost finding. This attack is cheap **and** uncorrected, and both
properties stem from the same root: the attacker moved the reported number without moving
the market.

### Consequences for Keel

Depth measures executable liquidity, which is exactly the part of the market that
arbitrage guards. The gap between the quoted price and the depth-backed price is the
attack surface. That is what Keel measures, and the reason this metric is correct, not
merely useful.

---

## 4. What must be updated

### 4.1 Reference price rule (methodology section 3)

The current rule mandates using the orderbook mid when both sides are filled. On this
fixture that rule produces 53.8971414 for an asset whose real liquidity sits at 1.0555442,
off by 50x.

```
1. There is a pool AND a two-sided book
     if |book_mid − pool_spot| / pool_spot > DivergenceThreshold
         P0 = pool_spot,  priceSource = "pool",  flag PRICE_SOURCE_CONFLICT
     otherwise
         P0 = book_mid,   priceSource = "book"
2. Two-sided book only        -> book mid
3. One-sided book + pool only -> pool spot
4. Pool only                  -> pool spot
5. Neither                    -> none
```

Rationale: when two price sources conflict, trust the one backed by executable liquidity,
and surface the conflict to consumers. Hiding the conflict is worse than choosing wrong.

### 4.2 Manipulation cost split in two

| Metric | Contents | Answers |
|---|---|---|
| `manipulationCostCombined` | SDEX and AMM combined | the cost of moving the actual market price |
| `manipulationCostOrderbookOnly` | SDEX only | the cost of fooling an oracle that reads SDEX trades |

On this fixture, reaching 106.7372828 costs about 147.96 USDC via the pool and zero via
the orderbook. The attacker paid zero. The gap between the two numbers is itself a signal:
an asset with a large `combined` but a small `orderbookOnly` looks safe when it is not.

Honesty note: the claim that the oracle only reads SDEX trades is an **inference** from
the fact that an honest pool exists and the attack still succeeded. Mark it as an
inference until confirmed by Reflector.

### 4.3 MaxReachablePrice semantics

Under constant product, price tends to infinity as the base reserve tends to zero. So when
an active pool exists:

- `Reachable` is always true on the buy side
- `MaxReachablePrice` is null, with a warning

Both fields are only meaningful for pure orderbook markets. The `Reachable == true`
condition on `MANIPULATION_CHEAP` is retained, because it remains binding for assets
without a pool.

### 4.4 Limitations section

Add, with evidence from this data:

> Liquidity that exists but is not traded provides no protection against a trade-based
> oracle. The USTRY/USDC pool held honest reserves at 1.0555 for 12 days spanning the
> entire attack window, and prevented nothing.

---

## 5. Files affected

| File | Change |
|---|---|
| `testdata/fixtures/ustry_pre_exploit.md` | add the pool, recompute the entire table with `P0 = 1.0555442`, bump to v1.0.3 |
| `docs/methodology/03-reference-price.md`, `05-manipulation-cost.md`, `06-oracle-resilience.md`, `11-limitations.md` | `P0` rule, manipulation cost split, arbitrage asymmetry, limitations |
| `docs/methodology/09-flags-and-bands.md` | add `PRICE_SOURCE_CONFLICT` at HIGH severity |
| `internal/domain/types.go` | `ManipulationCostCombined`, `ManipulationCostOrderbookOnly`, `PoolSpotPrice`, `PriceDivergencePct` |
| `docs/api/openapi.yaml` | the same fields, plus an `assetPriceConflict` response example |
| `CLAUDE.md` | three silent-failure gotchas, plus a prohibition on converting ledger to time |

---

## 6. Gotchas found during this verification

All three **fail silently**, not loudly, and all three have proven real on this project's
data. Each deserves a test in the adapter.

1. **Asset type.** USTRY has a five-character code, so it is of type `credit_alphanum12`.
   Querying with `credit_alphanum4` returns an empty array with no error message.
2. **Price source.** `/offers` sends `price_r` as a JSON number, `/trades` sends `price`
   as a JSON string, and the direction depends on which asset is the base. The `price`
   string is rounded and must not be used for computation.
3. **Pool effects.** `/liquidity_pools/{id}/effects` returns all effects from operations
   touching that pool, including effects on other pools in the same path payment. Without
   filtering by `liquidity_pool.id`, you read the wrong pool's reserves with no warning at
   all.

Plus one prohibition: **do not derive time from ledger number arithmetically.** The
five-seconds-per-ledger assumption is off by about three weeks over a six-month range.
Take `closed_at` from `/ledgers/{seq}`.

### Bonus for the methodology

The effect output shows `trade` and `liquidity_pool_trade` appearing mixed within the same
operation. That is a path payment routed through both the orderbook and the pool at once.

This is direct empirical evidence for the combination rule in methodology section 6: SDEX
and AMM liquidity do compete in the same price range and are consumed together by a single
order. Summing their depths separately contradicts how the protocol actually works. Keep
one example operation like this as an appendix.
