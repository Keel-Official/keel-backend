# Keel: Core Methodology

**Methodology version:** 1.0.2-draft
**Applies to:** all of `internal/depth`, using the types from `internal/domain`
**Status:** definitions locked, thresholds not calibrated
**Validated against:** the YieldBlox DAO pool incident on Blend V2, 22 February 2026

This document defines what Keel computes and why. Every definition here has to be
defensible without referring to the code. Where the code and this document
disagree, this document is right and the code has a bug.

**Split of authority.** This file defines the quantities that are computed. The
definitions of flags, bands, and every threshold live in `09-flag-dan-band.md`,
and that file wins where the two differ. Two homes for one definition guarantee
that both drift, so the split has to be kept sharp.

**The percent convention.** Every quantity and threshold whose name ends in `Pct`
is expressed in PERCENT, not as a fraction. A `spreadPct` of 196.0777141 means 196
percent, and it is compared directly against a `SpreadExtremePct` of 20.0.
Conversely `δ` is always a fraction: `δ = 0.02` means 2 percent. The two
conventions differ deliberately, because `δ` is an input to a formula while `Pct`
is a reported quantity.

---

## 1. The question this answers

An oracle answers: what is the price of this asset.
Keel answers: **how large a transaction can that price support, and what does it
cost to move it.**

Two derived questions that must be kept firmly apart, because they are constantly
confused:

| Risk | Side of the book | Question |
| --- | --- | --- |
| **Liquidation** | bid (sell side) | If this collateral has to be liquidated, can the market absorb it? |
| **Oracle manipulation** | ask (buy side) | What does it cost to push the price up until the collateral looks far more valuable? |

An asset can be safe on one side and dangerous on the other. Keel reports both
separately and never collapses them into a single number.

---

## 2. Notation and units

| Symbol | Meaning |
| --- | --- |
| `base` | the asset being assessed |
| `quote` | the asset of measurement (the primary pair) |
| `P0` | the reference price, quote per base |
| `δ` | relative price shift, 0.02 means 2 percent |
| `P_target` | the target price, `P0 × (1 + δ)` |
| `X`, `Y` | the base and quote reserves of one AMM pool |
| `f` | the pool fee, 0.003 on Stellar |

Unit rules:

1. Every depth and cost figure is a **notional in the quote asset**, not a count
   of tokens. The business question is "how many dollars", not "how many coins".
2. Keel does **not** convert to USD using an external price feed. The entire
   premise of this product is to question whether a reported price can be trusted.
   Using a feed inside the computation makes the argument circular.
3. Stellar amounts are int64 stroops with 7 decimals. Prices are read from the
   rational fraction `price_r`, shaped `{n, d}`, never from the already-rounded
   `price` string. All arithmetic is decimal, never floating point.

---

## 3. The reference price `P0`

Fallback order, stopping at the first condition that is met:

| # | Condition | `P0` | `priceSource` |
| --- | --- | --- | --- |
| 1 | A bid and an ask both exist | `(best_bid + best_ask) / 2` | `book` |
| 2 | Only one side of the book, a pool exists | `Y / X` from the pool with the largest quote reserve | `pool` |
| 3 | The book is empty, a pool exists | `Y / X` | `pool` |
| 4 | Neither a book nor a pool | undefined | `none` |

**Case 4 is not an error and must not raise an exception.** An asset with no
executable price is the highest-value finding Keel can produce. It is reported as
a legitimate result with the flag `NO_EXECUTABLE_PRICE` and band `CRITICAL`.

**A warning found in real data.** A `P0` taken from the orderbook mid can be
influenced by the attacker's own orders. During the 22 February 2026 incident the
attacker placed a buy order for 0.0001 USTRY at a price of 1.057 at 23:39:31, one
minute after placing the manipulation offer. An order that small still shapes `P0`
while representing no real liquidity whatsoever. `P0` therefore **must not** be
used on its own as a health indicator, and must always be read together with
depth.

---

## 3a. Spread, and the limit of `P0`'s meaning

Discovered while building the golden fixture, and the reason version 1.0.1 exists.

```
spreadPct = (best_ask - best_bid) / P0 × 100
```

`spreadPct` is null when either side of the book is empty, because the difference
is undefined. Null means unknown, not zero.

On 21 February 2026 the USTRY/USDC book held an ask at 106.7372828 and a bid at
1.057, which makes `P0` equal to 53.8971414 for an asset actually worth about
1.06. That number is not a bug; it is what a mid price does when the spread runs
into the hundreds of percent.

The consequence is harsh: **`P0` and every metric derived from it lose their
meaning at an extreme spread.** The ±2/5/10 percent depth ladder is included in
that. It is still reported because the SOW promised it, but on a book that broken
it is not an oracle safety metric. What saves the analysis is the large delta
ladder in section 7.3 and the `SPREAD_EXTREME` flag.

Other flags do also fire on the USTRY case, but that is a coincidence and not the
design. This is why `spreadPct` is reported as a number in the API response, not
merely as a triggered or not-triggered status.

---

## 4. SDEX orderbook depth

Buy side depth at `δ` is the total notional that can be absorbed by buying from
the asks before the marginal price passes `P_target`.

```
P_target = P0 × (1 + δ)
depth_sdex_buy(δ) = Σ (price_i × amount_i)  for every ask with price_i <= P_target
```

The sell side is symmetric, using the bids and `P0 × (1 - δ)`.

**Decision: a level that crosses the limit is discarded entirely, not taken
partially.** This produces a figure slightly below the theoretical value. The
choice is deliberate and consistent with the Conservative Principle in section 12:
in every ambiguous case, Keel picks the interpretation that makes the asset look
riskier, not safer.

---

## 5. Constant product AMM depth

Stellar pools use `X × Y = k` with a 30 bps fee. The spot price is `P = Y / X`.

The derivation. Buying `Δx` of the base leaves a reserve of `X - Δx`, so the new
marginal price is:

```
P' = k / (X - Δx)²
P' / P = X² / (X - Δx)²
```

For `P' / P = 1 + δ`:

```
X - Δx = X / √(1 + δ)
```

Which gives:

```
depth_amm_buy(δ)  = Y × (√(1 + δ) - 1)      quote that must be paid
depth_amm_sell(δ) = Y × (1 - √(1 - δ))      quote that is received
gross_input       = net_input / (1 - f)
```

A sanity check that must exist in the tests: `depth_amm ≈ (δ / 2) × Y`. A large
deviation from that means there is a bug.

| δ | up, percent of Y | down, percent of Y |
| --- | --- | --- |
| 2% | 0.995% | 1.005% |
| 5% | 2.47% | 2.53% |
| 10% | 4.88% | 5.13% |

The square root is computed at a fixed decimal precision, not in `float64`. That
precision and its tolerance are named constants and are part of the methodology,
not an implementation detail.

---

## 6. Combining SDEX and AMM

**The wrong way:** `depth_total = depth_sdex + depth_amm`, each computed
separately. The two compete over the same price range, so summing them
independently overstates liquidity.

**The right way:** both are bounded by the same final marginal price.

```
combined_depth(δ):
    P_target = P0 × (1 + δ)
    n_sdex   = Σ (price × amount) for asks with price <= P_target
    n_amm    = Σ over every pool:
                   0                               if P_pool >= P_target
                   Y × (√(P_target / P_pool) - 1)  if P_pool <  P_target
    return n_sdex + n_amm
```

The SDEX and AMM contributions are still reported separately in the output
(`fromSdex`, `fromAmm`) so that a third party can verify this combination without
reading the code.

A discriminating test that must exist: build a fixture whose pool price sits 5
percent above `P0`, then ask for depth at 2 percent. The correct answer is
`fromAmm` exactly zero. An implementation that sums separately returns a non-zero
value.

---

## 7. Manipulation cost

This section is Keel's main contribution, and its definition was corrected after
examining the ledger data of the 22 February 2026 incident.

### 7.1 Definition

The manipulation cost of reaching a price `P_target` is the notional that has to
be paid **to other parties** in order to push the marginal price up to it:

```
MC(P_target) = Σ (price_i × amount_i)
               over every ask with price_i <= P_target
               that is NOT owned by the attacker
```

### 7.2 Why that ownership clause decides everything

This is the mistake that almost entered this methodology, and only the data
exposed it.

The faulty intuition is to treat the size of the manipulating transaction as its
cost. In the 22 February incident that transaction was worth 5.3475699 USDC. That
figure is not the cost, because the offer it executed against belonged to the
attacker. The money moved from one attacker account to another attacker account.

The real cost is the value of the **third party** orders that have to be swept on
the way from `P0` to `P_target`. Stellar's matching engine always fills from the
best price. For a buy order to reach the offer at 106.74, every cheaper ask has to
be consumed first.

And that amount is **exactly the buy side depth between `P0` and `P_target`.**

```
MC(P_target) = buy side depth up to P_target, minus the attacker's own orders
```

Keel cannot know order ownership ahead of the event. It therefore reports the
unfiltered version, which is an **upper bound** on the manipulation cost. The
direction of that bias is safe: Keel will say manipulation is more expensive than
it really is, never cheaper.

### 7.3 Consequence: the large delta ladder

Depth at ±2, ±5, and ±10 percent measures market quality. It is **not enough** to
measure resistance to oracle manipulation, because an attacker does not need to
move the price by 10 percent. In this incident the attacker moved it by a factor
of 100.98.

Keel therefore computes two ladders:

| Ladder | δ values | Purpose |
| --- | --- | --- |
| Market quality | 2%, 5%, 10% | mandated by the SOW, describes ordinary depth |
| Manipulation resistance | 50%, 100%, 1000%, 10000% | describes the cost of an attack |

The most useful derived metric is the inverse function:

```
P_reachable(C) = the highest price reachable with capital C
```

For USTRY on 21 February 2026, `P_reachable(0)` already exceeded 100 times `P0`.

### 7.4 `Reachable`, and the two opposite meanings of a zero cost

This is the 1.0.1 correction and the most easily misread part of the whole
methodology. The first test written against the USTRY fixture did misread it, on
two lines.

Every row of the manipulation ladder carries two quantities computed from
**different** sets of asks:

```
Cost(P_target)      = Σ (price_i × amount_i)  for asks with price_i <  P_target
Reachable(P_target) = an ask exists with price_i >= P_target
```

An ask never belongs to both at once. The attacker has to sweep every ask cheaper
than the target, then touch only a sliver of the first ask above it. That final
touch is what sets the price the oracle reads, and it can cost arbitrarily little.

A zero cost therefore means two opposite things:

| State | Meaning |
| --- | --- |
| `Cost = 0`, `Reachable = true` | the target price can be reached without paying anything to a third party. **This is the most dangerous condition that can exist** |
| `Cost = 0`, `Reachable = false` | there is no liquidity at all in that range, so the price cannot be walked up to it. That is not bad news |

Reading `Cost` on its own, without `Reachable`, produces a misleading number and
nothing fails. On the USTRY fixture, `Cost` is 130.0627093 for δ = 1, 10, and 100,
and all three are `Reachable = false`. The 130.06 there does **not** mean "that
price is expensive to reach"; that price cannot be reached at all, because the
book runs out long before it.

### 7.5 Maximum reachable price

A discrete delta ladder can miss an attack that lands between two rungs. Keel
therefore also reports a pair of numbers that does not depend on the ladder:

```
MaxReachablePrice       = the highest ask price on the book
CostToMaxReachablePrice = Σ ask notionals with price < MaxReachablePrice
```

For USTRY on 21 February 2026 those values were 106.7372828 at a cost of **zero**.
The real attack landed in the gap between δ = 0.5 and δ = 1, so every rung of
every ladder missed it and only this pair of numbers caught it.

---

## 8. Oracle resistance against a VWAP window

The oracle manipulated in this incident is VWAP based, not spot based. Moving the
marginal price is not sufficient; the attacker's transaction also has to dominate
the volume weighted average over the oracle's window.

```
MR(P_target, W) = MC(P_target) + V_genuine(W)
```

where `V_genuine(W)` is the genuine trade volume within the window `W`.

The second term is the invisible defence. A market with real trading forces an
attacker to outweigh real volume in order to move the average. A market with no
trading has no such defence at all.

On 22 February 2026 both terms were zero or close to zero at the same time. That
is what made the attack effectively free.

`W` is a parameter, not a universal constant. Keel's default assumes 15 minutes,
following Script3's statement that no other trade occurred in the 15 minutes
before the manipulation. That figure is **not confirmed** as Reflector's actual
window and is marked as an assumption.

---

## 9. Maximum safe collateral size

```
C_max = min( D_sell(δ_liquidation) × h , MC(P_critical) × m )
```

| Symbol | Meaning | Default |
| --- | --- | --- |
| `D_sell(δ_liquidation)` | sell side depth at the liquidation discount | δ = 10% |
| `h` | liquidation haircut | 0.5 |
| `MC(P_critical)` | manipulation cost to the critical price | P_critical = 2 × P0 |
| `m` | manipulation safety margin | 0.25 |

The two terms answer different questions and both have to be reported, not only
the minimum. Every parameter is configurable by the caller, because Keel is
protocol agnostic.

The defaults above are **chosen, not calibrated.** Calibration needs more
incidents than are available. That statement is required to appear in the
dashboard and in the API response, not only in this document.

---

## 10. Empirical validation: the 22 February 2026 incident

Every number in this section is derived from Horizon mainnet and can be reproduced
by anyone with no account, no BigQuery, and no privileged access.

### 10.1 Assets

```
USTRY : GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC   (credit_alphanum12)
USDC  : GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN   (credit_alphanum4)
```

Note that USTRY is `credit_alphanum12` because its code is five characters.
Querying it as `credit_alphanum4` returns an empty result and no error message.

### 10.2 Verified chronology

| Time UTC | Event | Evidence |
| --- | --- | --- |
| 21 Feb 23:36:28 | The burner account swaps 1 XLM for 0.1612003 USDC as funding | op 263452928864530433 |
| 21 Feb 23:38:51 | **The manipulation offer is placed.** Sell 1.2185312 USTRY at `price_r` {266843207, 2500000} = 106.7372828 USDC per USTRY | tx `09e1a9d1...`, offer 1824788980 |
| 21 Feb 23:39:31 | A buy order for 0.0001 USTRY at 1.0570000 is placed by the same account | op 263453066303434753 |
| 22 Feb 00:10:21 | **The manipulation trade.** `GDHRCQNC...` pays 5.3475699 USDC for 0.0501003 USTRY, matching offer 1824788980 | trade 263454423513071617-0, ledger 61340263 |
| 22 Feb 00:10:57 | A dust trade of 0.0000080 USTRY at the normal price of 1.057, between attacker accounts | trade 263454449283014657-0 |
| 22 Feb 00:11:16 | The 1.057 buy order is cancelled | op 263454462168100865, ledger 61340272 |
| 22 Feb ~00:25 | Loans drawn from the YieldBlox pool: 1,000,196.70 USDC then 61,249,278.31 XLM | secondary sources, on-chain verification pending |
| 16 Mar 14:26:40 | The burner account swaps 5.4152411 USDC for 31.1670395 XLM | op 264910138253852673 |

### 10.3 Arithmetic consistency

Every figure locks against the others, which is what tells us the reading is
correct:

```
0.0501003 USTRY × 106.7372828  = 5.3475699 USDC     matches what was paid
1.2185312 - 0.0501003          = 1.1684309 USTRY    matches the offer remainder today
106.7372828 / 1.057            = 100.98×            matches the reported "100x"
```

### 10.4 The central finding

**The manipulation cost was close to zero.** All 5.3475699 USDC went to an offer
the attacker owned, so it returned to the attacker. No other trade is recorded on
ledger 61340263 between `P0` and `P_target`. Because Stellar's matching engine
fills from the best price, the fact that the offer at 106.74 was touched means
**there was no third party ask anywhere in the price range from 1.057 to 106.74.**

That sentence used to be marked as a strong inference. Since version 1.0.1 it is a
**direct observation**: the trade list of account
`GDHRCQNC64UVL27EXSC6OG6I2FCT4NWM72KNHLHKEB3LK4MEEYYWETN3` at
2026-02-22T00:10:21Z contains exactly one record, 5.3475699 USDC against offer
1824788980 owned by
`GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB`. Since every match
produces a trade record, the absence of any other record proves no third party ask
was swept.

```
MC(100 × P0) for USTRY/USDC on 21 February 2026 = 0 USDC
```

Not thin. Zero.

**The scale of the consequence.** With XLM at 0.1612003 USDC, the loans drawn were
worth:

```
61,249,278.31 XLM × 0.1612003 =  9,873,402 USDC
                      + USDC   =  1,000,197 USDC
                        total  = 10,873,599 USDC
```

Collateral of 149,876.10 USTRY was worth 158,419 USDC at the real price and
15,997,368 USDC at the manipulated price. A ratio of 100.98 times, exactly as
expected.

**What Keel would have reported.** Run against USTRY/USDC on 21 February 2026,
Keel would have triggered, at minimum:

| Flag | Reason |
| --- | --- |
| `MANIPULATION_CHEAP` | `MC(50%)` below any defensible absolute threshold |
| `MANIPULATION_RATIO_LOW` | manipulation cost near zero against the value of supply |
| `THIN_DEPTH_5PCT` | no ask at all inside that range |
| `NO_GENUINE_TRADE_7D` | volume below 1 USDC per hour going into the incident |

Band: `CRITICAL`. This asset would not have passed any threshold that could be
defended.

### 10.5 Verification still pending

1. Both loan transactions in the YieldBlox pool, currently from secondary sources.
2. The YieldBlox pool risk parameters in force at the time.
3. Reflector's actual VWAP window length.

The item about account `GDHRCQNC...`'s trade list was **closed** in version 1.0.1
and moved into section 10.4 as a direct observation.

---

## 11. Depth implied by trades

When historical orderbook state is unavailable, depth can be bounded from the
trades that actually happened.

**The claim.** If a trade worth `S` shifted the marginal price by `δ`, then:

```
depth(δ) <= S
```

The reason: if there were more liquidity than `S` in that price range, a trade of
size `S` could not have pushed through it.

This yields an upper bound, not a direct measurement. For Keel's purpose that is
sufficient, because what has to be shown is not the exact value of depth but that
depth sits below a safe threshold. The bias is conservative in the right
direction.

A result derived this way **must** be tagged `dataSource: "trades-implied"` in the
API response. A number that is an upper bound must not look identical to a number
that was measured.

---

## 12. Principles and limitations

### The conservative principle

In every ambiguous case, choose the interpretation that yields lower depth and a
higher risk assessment. A warning product that is too optimistic is useless.

### Known limitations

1. **Posted liquidity is not executable liquidity.** An offer can be withdrawn
   instantly. Reflector reported that this market's market maker pulled all of its
   liquidity before the incident. A Keel that scans every 15 minutes catches that
   change; a Keel that scans daily may be too late. Scan frequency is an honest
   parameter, not a technical detail.
2. **Cross-asset path payments are not counted.** Real effective liquidity can be
   larger than what Keel reports.
3. **Centralised exchange liquidity is invisible.** Keel measures on-chain only.
4. **Thresholds are chosen, not calibrated.** Calibration needs many incidents.
   Every flag is reported separately so that a consumer can apply their own
   thresholds.
5. **A backtest knows the answer in advance.** The hindsight bias risk is real. If
   a threshold was set after seeing the outcome, that has to be stated in the
   report.
6. **Order ownership cannot be known ahead of the event**, so a reported
   manipulation cost is always an upper bound.

---

## 13. Versions

`MethodologyVersion` follows semver and must be raised whenever a definition or a
threshold changes. Results from different versions cannot be compared directly and
are stored as separate rows in the database.

| Version | Change |
| --- | --- |
| 1.0.0-draft | Initial definitions. Manipulation cost corrected to be ownership based after examining the ledger data of the 22 February 2026 incident. The large delta ladder added. The oracle window volume term added. |
| 1.0.1-draft | Section 3a (`spreadPct` and `SPREAD_EXTREME`) added after the golden fixture showed `P0` losing its meaning at an extreme spread. Sections 7.4 (`Reachable`, the two meanings of a zero cost) and 7.5 (`MaxReachablePrice`) added. Section 10.4 was promoted from inference to direct observation, closing item 1 of section 10.5. |
| 1.0.2-draft | `MANIPULATION_CHEAP` and `MANIPULATION_RATIO_LOW` now require `Reachable == true`. The `unevaluated` state and `bandConfidence` added after the fixture showed that six flags cannot be assessed from a snapshot alone. Details in `09-flag-dan-band.md`. The percent unit convention stated explicitly at the head of this document. |
