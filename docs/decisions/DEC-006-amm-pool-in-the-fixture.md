# DEC-006: The golden fixture is missing an AMM pool, and what that costs

**Status:** OPEN. This document records a finding and its consequences. The decision
is Al's, because closing it means editing the methodology and the golden fixture,
and both are the paid deliverable.
**Found:** 21 August 2026, while closing the DEC-002 spike DoD.
**Evidence:** `docs/evidences/pool_ustry_usdc_2026-02.txt`, reproducible with curl
and no account.

---

## 1. The finding

`testdata/fixtures/ustry_pre_exploit.md` records `Pools: []` for USTRY/USDC at
ledger 61340263. A liquidity pool for exactly that pair existed and held reserves
at that ledger.

| | |
|---|---|
| Pool | `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb` |
| Fee | 30 bp |
| Reserve quote, USDC | **16.3389179** |
| Reserve base, USTRY | **15.4791416** |
| Spot price | **1.0555441847** USDC per USTRY |

The USTRY issuer in that pool is `GCRYUGD5...`, the same issuer the fixture uses, so
this is not a different USTRY.

**How the reserve figures are known, link by link.** The last transaction to touch
this pool before the incident ledger was `fa012177...` at ledger 61172481, on
2026-02-10T16:59:35Z, twelve days earlier. Its `liquidity_pool_trade` effect records
the reserves it left behind. Pool reserves change only through a transaction that
touches the pool, and no transaction touched this one between then and ledger
61340264. So the reserves above are the reserves at ledger 61340263.

Two of those four links are direct observation from Horizon, one is arithmetic, and
one is an inference from how Stellar pools work. The inference is the third link and
it is the only one worth arguing about.

---

## 2. What it changes in the fixture

Recomputed with the methodology's own formulas, section 6 for depth and section 7
for manipulation cost, using `n_amm = Y x (sqrt(P_target / P_pool) - 1)` grossed up
by the fee.

**Depth, buy side.** The fixture records zero on all three rungs.

| δ | buy target | fixture | with the pool |
|---|---|---|---|
| 0.02 | 54.975084 | 0 | **101.8814** USDC |
| 0.05 | 56.591998 | 0 | **103.6081** USDC |
| 0.10 | 59.286856 | 0 | **106.4319** USDC |

**Manipulation cost.** The fixture records 0, then 130.0627093 three times.

| δ | target | fixture | orderbook | AMM | with the pool |
|---|---|---|---|---|---|
| 0.5 | 80.845712 | 0 | 0 | 127.0348 | **127.0348** |
| 1 | 107.794283 | 130.0627093 | 130.0627 | 149.2224 | **279.2851** |
| 10 | 592.868555 | 130.0627093 | 130.0627 | 372.0029 | **502.0656** |
| 100 | 5443.611281 | 130.0627093 | 130.0627 | 1160.4954 | **1290.5581** |

**What does not change.** `P0` stays 53.8971414, because the fallback ladder in
methodology section 3 stops at case 1 as soon as the book has two sides, so the pool
has no influence on the reference price at all. `spreadPct` stays 196.08.
`ZERO_DEPTH_2PCT` still fires, because the pool quotes 1.0555 and the sell side
target is 52.82, so the pool absorbs no selling anywhere in that band and the sell
side is still zero. The band is still `CRITICAL`.

So the conclusion of the fixture survives. Two of its headline numbers do not.

---

## 3. The claim this breaks

`keel-methodology-core.md` section 10.4 states:

> `MC(100 × P0) for USTRY/USDC on 21 February 2026 = 0 USDC`
>
> Not thin. Zero.

Under Keel's own combining rule that is false. With the pool included it is
**1290.5581 USDC**, and at the critical delta it is 127.0348 USDC.

This matters more than a wrong number in a table. "Zero" is the sentence the
methodology is built around, and it is the sentence a reviewer will test first.

---

## 4. The part that is genuinely interesting

The attacker paid **5.3475699 USDC**, and the trade record says
`"trade_type": "orderbook"`.

On Stellar, an orderbook match reaches offers only. Pool liquidity is reachable
through path payments, not through an orderbook match. So the attacker never touched
the pool, never paid its liquidity providers, and lifted the orderbook's best ask to
106.74 while the pool went on quoting 1.0555.

Going through the combined price would have cost 127.03 USDC at the critical delta.
Going through the orderbook alone cost 5.35. **Using one venue instead of the
combined book was 23.8 times cheaper.**

That is a problem for non-negotiable rule 4, which says SDEX and AMM depth are
combined through a shared marginal price and never summed separately. The rule is
right for the question "if this collateral is liquidated, can the market absorb it",
because a liquidator will take whatever liquidity exists wherever it is. It appears
to be wrong for the question "can the oracle be fooled", because an attacker only
has to move the venue the oracle reads, and the combined price then overstates what
they must pay.

Keel already separates those two questions by side of the book, buy side for
manipulation and sell side for liquidation. This finding says they may also need
separating by **venue**, not only by side.

There is a second, quieter finding in the same place. The pool was quoting the
honest price of 1.0555 throughout, and `P0` still came out 53.8971414, because the
fallback ladder consults a pool only when the book has fewer than two sides. A
venue quoting the true price sat there and the reference price ignored it.

---

## 5. What is not being done here, and why

The fixture is not being edited, and neither is the methodology.

The golden fixture's whole value is that its numbers were computed by hand before
any implementation existed. Rewriting its inputs would invalidate every expected
value in it, and having Claude recompute them would destroy exactly the safeguard
the fixture exists to provide: `internal/conformance/fixture.go` says in its own
header, "Do not adjust these numbers to match the code. Adjust the code to match
these numbers." The same logic protects the inputs.

`scripts/audit-verification.sh` now carries a check for this finding, so it cannot be
quietly forgotten.

---

## 6. The options, with a recommendation

**A. Correct the fixture, recompute by hand, soften the claim.**
Add the pool to `GoldenSnapshot()`, recompute the depth and manipulation tables by
hand, and rewrite section 10.4 to say what is actually true: the cost through the
orderbook was zero, the cost through the combined price was 127.03 USDC at the
critical delta, and the attacker paid 5.35 USDC by using the orderbook alone.
Raises `MethodologyVersion`.

**B. Keep the fixture as it is and rename what it claims to be.**
Relabel it an orderbook-only fixture, state explicitly that pools are excluded, and
add a second fixture that includes the pool. Cheaper, and it keeps the hand
computed numbers valid. But it leaves the methodology's "zero" sentence standing
next to a fixture that quietly assumes away a venue.

**C. Split the model by venue, not only by side.**
Report manipulation cost per venue as well as combined: the cost to move the
orderbook alone, the cost to move the pool alone, and the cost to move the combined
price. The oracle-relevant number is then whichever venue the oracle reads, and
Keel stops having to guess.

Recommendation: **A and C together.** A is not optional, because a false headline
number is the one thing that cannot survive review. C is what turns this from an
embarrassment into the most interesting result in the deliverable: Keel would be
reporting that the cheapest attack path is the one that ignores half the liquidity,
and that combining venues can understate oracle risk. No monitoring tool in this
ecosystem says that today.

B is the fallback if week 3 runs out of time, and it must not be chosen silently.

---

## 7. The inference was checked from both directions

This finding rested on one inference: that no transaction touched the pool between
ledger 61172481 and ledger 61340264, so the reserves did not move. It was checked
rather than assumed, by walking Horizon's transactions-for-pool endpoint both ways.

| Direction | Cursor | Nearest entry |
|---|---|---|
| Backwards from the incident ledger | `61340264 << 32` | ledger **61172481**, 2026-02-10T16:59:35Z |
| Forwards from just after that one | `61172482 << 32` | ledger **61351813**, 2026-02-22T18:48:02Z |

The gap between 61172481 and 61351813 contains ledger 61340263, and nothing touched
the pool inside it. The reserves at the incident ledger are therefore the ones that
transaction `fa012177...` left behind, and they were not zero.

What would still overturn this: a class of reserve-changing event that does not
produce a transaction associated with the pool. None is known, and Stellar has no
mechanism for changing a pool's reserves without an operation against it, but that is
an argument from the protocol rather than from this data.
