# USTRY/USDC, February 2026: what the trade stream can and cannot show

**Read on:** 26 August 2026, from Horizon mainnet, no account required.
**Window:** 2026-02-01T00:00:00Z to 2026-03-01T00:00:00Z, exclusive on the right.
**Pair:** USTRY `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`
(credit_alphanum12) against USDC
`GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` (credit_alphanum4).

**Reproduce it:**

```bash
go run ./cmd/keel backtest \
  -pairs scripts/record-pairs.example.json \
  -from 2026-02-01 -to 2026-03-01 -mark 2026-02-22 \
  -from-ledger 60977383 -out docs/evidences
```

`-from-ledger` is a SEEK and not a boundary. Ledger 60977383 closed at
2026-01-28T15:42:58Z, read from `/ledgers/60977383` and not computed from the
date; the window itself is decided on each record's own `ledger_close_time`.
Being early costs a few extra requests and being late would silently drop trades,
which is why the command prints the first trade it kept.

**Files this produced, both in this directory:**

| File | Contents |
|---|---|
| `USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv` | 13,547 rows, one per trade, every field as Horizon sent it including the exact `price` rational |
| `USTRY.GCRYUGD5-USDC.GA5ZSEJY-daily-2026-02-01_2026-03-01.csv` | 28 rows, one per UTC day, derived from those trades |

---

## 1. Why this exists at all

Deliverable 2 promises a USTRY time series for February 2026 and a statement of
when the unsafe threshold was crossed relative to the exploit date. The direct
route is historical order book state, and DEC-002 defers the only source that
serves it. Section 2 of that record names the substitutes, and this is 2.1 and
2.2 built: the manipulation cost read straight off the trade that happened, and
the upper bound on depth implied by an operation that moved the price.

The bound is `01-data-sources.md` section 6:

```
depth(δ) ≤ S    if a trade of size S moved the marginal price by δ
```

---

## 2. The distinction that decides everything below

The word doing the work in that inequality is **moved**, and it is a claim about
cause. Two trades an hour apart at different prices establish that the price
changed, not that the second one changed it: offers can be cancelled and posted in
between, and a trade stream records none of that.

So every bound here carries one of two labels, and they are never merged.

| Kind | What it assumes | Where it comes from |
|---|---|---|
| **within-leg** | nothing | one taker, one direction, filling from the best price outward. The span between the leg's first and last fill was crossed BY that leg |
| **between-legs** | that the book did not change in the gap | the span from one leg's last fill to the next leg's first fill. The gap can be seconds or minutes, and the CSV carries it |

A **leg** is a maximal run of fills inside one operation that move the base asset
in the same direction. Most operations are one leg.

**That refinement was paid for by a real error found before this file was
written.** Grouping by OPERATION rather than by leg made operation
`263504030385864713` (2026-02-22T18:48:02Z, ledger 61351813) look like the
tightest causal bound of the whole month: an 11.4 percent price span for 1.886
USDC. It is nothing of the kind. It is
`path_payment_strict_send` USDC → USTRY → USDC, which BUYS 0.890217 USTRY on the
order book at 1.1233215 and SELLS the same 0.890217 to the liquidity pool at
0.9953083 in the same operation, returning 0.8860404 of the 1 USDC it sent. The
two prices are two venues at one instant. No liquidity was crossed between them.
`internal/domain/trades_test.go` carries this case so it cannot come back.

---

## 3. The result, and it is not the one the deliverable was hoping for

### 3.1 Causal evidence: nothing, all month

**Not one leg in the whole of February 2026 moved the USTRY/USDC price by as much
as 2 percent, which is Keel's smallest rung.**

| Largest within-leg move | Day |
|---|---|
| 0.39 percent | 2026-02-22 |
| 0.21 percent | 2026-02-25 |
| 0.16 percent | 2026-02-08 |

So the trade stream supports **zero** causal bounds at δ = 0.02, 0.05, 0.10 or at
the critical delta of 0.5, on every one of the 28 days, including the day of the
attack. Against `ThinDepth5PctAbsolute` of 50,000 and
`ManipulationCheapAbsolute` of 10,000 the honest verdict for the whole month is
not "safe" and not "unsafe". It is **unevaluated**.

### 3.2 Assuming evidence: two days, and both are dust

| Day | Rung | Bound | Source trade | Gap | What it actually is |
|---|---|---|---|---|---|
| 2026-02-10 | ≥ 5 percent | 0.000001 USDC | `262733805310210049-0` | 399 s | a 0.0000009 USTRY liquidity pool trade, 6 minutes 39 seconds after the previous one |
| 2026-02-22 | ≥ 50 percent | 0.0000084 USDC | `263454449283014657-0` | 36 s | the dust trade at 00:10:57 that put the price back to 1.057, 36 seconds after the manipulation |

Read literally, the 10 February row says the unsafe threshold was crossed **12
days before the exploit**. That sentence is available and it should not be used
without the rest of this paragraph attached to it: it rests on a one-millionth of
a dollar trade, on the assumption that nobody touched the book for six and a half
minutes, and on a price move that the arithmetic of a nearly empty pool produces
on its own. It is not a warning anybody could have acted on.

### 3.3 The one number that is solid, and it was already known

The manipulating trade itself, `263454423513071617-0`, ledger 61340263,
2026-02-22T00:10:21Z: **5.3475699 USDC bought 0.0501003 USTRY at 106.7372828**,
against a previous price of 1.05742694. A ratio of 100.94.

This file reproduces it from the raw stream and adds nothing to what
`10-validation.md` section 7 already established. It is a between-legs bound
because 3 minutes 50 seconds separate it from the trade before it.

---

## 4. What this means, stated plainly

**The trade-implied route gives Keel no advance warning on USTRY, and that is a
finding rather than a failure of the tool.** PRD section 10 asks for exactly this
honesty: "If the backtest does not show a clear signal, that is not a project
failure but a finding that has to be reported honestly."

The reason it gives no warning is worth more than the warning would have been.
**A trade stream can only report price ranges somebody actually crossed.** Nobody
crossed the USTRY book in February, because there was nothing in it to cross. The
market traded 13,547 times in the month at a spread of a fraction of a percent
around 1.057, and every one of those trades was tiny and every one of them stayed
inside a price range where liquidity existed.

What made USTRY dangerous was not visible in what traded. It was visible in what
was **posted**: a single ask at 106.7372828 sitting 100 times above the bid, with
nothing in between, and a spread of 196 percent. That is order book STATE, and it
is precisely the state the golden fixture captures and `ComputeAssetRisk` turns
into `SPREAD_EXTREME`, `ZERO_DEPTH_2PCT`, `THIN_DEPTH_5PCT`, `MANIPULATION_CHEAP`
and a band of CRITICAL.

Three consequences follow, and none of them is rhetorical:

1. **A real February backtest needs the order book, not the trade stream.** The
   substitute in DEC-002 section 2.3, reconstructing offers from the operations
   that posted them, is not an optimisation over this file. It is the only route
   to the claim Deliverable 2 wants to make.
2. **This measures the cost of deferring Hubble.** DEC-002 section 2.2 argued
   that a trade-implied upper bound is "sufficient and in fact rhetorically
   stronger". On this asset, in this month, it is neither. That argument should
   be amended against this evidence rather than left standing.
3. **The hindsight-bias section has its example.** The one pre-exploit "signal"
   available here is a dust trade on 10 February that nobody would have noticed
   at the time and that only looks meaningful because the date of the attack is
   already known.

---

## 5. Limits of this reading, each one named

- **No genuine-trade rule is applied.** `07-supporting-metrics.md` section 1 is a
  worksheet and applying an unwritten rule here would be that document written by
  the wrong hand. The `volume_quote` column therefore inherits whatever wash
  trading exists, and the manipulation day's 326,472 USDC of the month's 375,320
  should be read with that in mind. The depth bounds are affected differently: a
  self-matched trade still crossed the book it crossed, so the inequality holds,
  but the TIGHTEST bound on a day can be set by dust, and section 3.2 is what
  that looks like.
- **The daily rows do not carry a bound forward.** A bound describes the market at
  the moment it was observed. Liquidity added the next morning would make a
  carried bound false, so a day with no qualifying move is blank rather than
  inheriting yesterday's number.
- **58 of the 13,547 trades are `liquidity_pool` rather than `orderbook`.** They
  are counted the same way here. Whether pool fills belong in a bound alongside
  order book fills is the venue question DEC-006 section 4 opens and section 8
  leaves open, and it is Al's.
- **This file does not reproduce the strongest claim about the incident.** That
  claim is that order book depth between 1.057 and 106.74 was ZERO, not merely
  small, and it rests on the manipulating operation producing exactly one trade
  record when the engine fills from the best price first. It turns on the absence
  of records rather than on their contents, so it is not a bound and no bound can
  express it. `10-validation.md` section 7 is where it lives.
- **The window's right edge is closed and its left edge was seeked.** The command
  reported `stopped=true`, meaning the walk ran past 2026-03-01 rather than off
  the end of the data, and it reported its first kept trade at
  2026-02-01T00:03:32Z. Both are printed on every run for exactly this reason.
