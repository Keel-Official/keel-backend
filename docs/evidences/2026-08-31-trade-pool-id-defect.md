# The pool identifier is empty on every trade, and the cause is one struct tag

**Date:** 31 August 2026
**Subject:** `docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv`
**Raw probe:** `docs/evidences/2026-08-31-trade-pool-id-probe/`
**Status:** finding only. Nothing is fixed. The proposed patch in section 6 is unapplied.

---

## 0. Verdict

| Candidate | Verdict |
|---|---|
| `internal/horizon/trades.go` | **THIS IS THE DEFECT.** Line 229 reads a field name Horizon never sends |
| `cmd/keel/backtest.go` | not at fault. It writes the domain field through verbatim |
| a property of what Horizon returns for this pair | **no.** Horizon sends the pool id on every pool trade, under two side-specific names |

One line is wrong:

```go
// internal/horizon/trades.go:229
LiquidityPoolID string `json:"liquidity_pool_id"`
```

Horizon has no `liquidity_pool_id` field on a trade. It sends
`base_liquidity_pool_id` or `counter_liquidity_pool_id`, depending on which side
of the trade the pool was. The tag matches neither, `encoding/json` ignores keys
it was not asked for and leaves the target at its zero value, and the decode
returns no error. The field has been the empty string on every trade the
repository has ever read.

---

## 1. Two of the three numbers do agree, and the premise needs correcting first

The three figures were reported as three numbers that should agree. Two of them
already do, and seeing why is what locates the defect.

Counted over all 13,547 rows:

| Measure | Count |
|---|---|
| `trade_type = liquidity_pool` | 58 |
| pool trades with `base_account` empty and `counter_account` set | 49 |
| pool trades with `counter_account` empty and `base_account` set | 9 |
| pool trades with BOTH accounts empty | 0 |
| pool trades with NEITHER account empty | 0 |
| **orderbook** trades with any empty account | **0** |

49 + 9 = 58, exactly. The account columns are not disagreeing with the trade type;
they are agreeing with it perfectly. A pool trade has an account on one side and a
pool on the other, so exactly one account field is empty, and which one depends on
whether the pool was the base or the counter. 49 of these trades had the pool on
the base side and 9 on the counter side.

So `base_account` being empty on 49 rows rather than 58 is correct behaviour and
not a symptom. **The account columns are currently the only way to tell which side
of a pool trade the pool was on**, and they do that job.

That leaves one number genuinely wrong: `liquidity_pool_id` is empty on all 58.

---

## 2. Horizon does send it. Evidence.

Probe: `/trades` for this exact pair, filtered `trade_type=liquidity_pool`, 200
records, cursor pinned. Raw body and headers in
`docs/evidences/2026-08-31-trade-pool-id-probe/`, sha256 recorded there.

Counted with `jq` over those 200 records:

| | |
|---|---|
| records with a top-level `liquidity_pool_id` | **0** |
| records with `base_liquidity_pool_id` | 171 |
| records with `counter_liquidity_pool_id` | 29 |
| records with both side ids | 0 |
| records with neither | 0 |

171 + 29 = 200. Every pool trade names its pool. None of them uses the name the
code is looking for.

One real record, base side is the pool:

```json
{
  "id": "262289800181637121-0",
  "trade_type": "liquidity_pool",
  "liquidity_pool_fee_bp": 30,
  "base_liquidity_pool_id": "27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb",
  "base_amount": "0.0000527",
  "counter_offer_id": "4873975818609025025",
  "counter_account": "GBQ7SRTP7PNNSCNZFU3QMI4MJHC7CFAYITF4B2ZRLPUK7R62DJJQDP7K",
  "base_is_seller": true
}
```

`base_account` is not empty in that body. It is **absent**. The CSV shows an empty
string because Go decoded a struct field that nothing populated, which is the same
mechanism as the defect itself.

The distinct pool id across all 200 records is a single value:

```
27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb
```

That is the USTRY/USDC pool this repository already has decisions about. It is
named in `docs/methodology/01-data-sources.md`, `docs/methodology/10-validation.md`,
`DEC-003`, `DEC-006` and `DEC-007`. **The one identifier the CSV drops is the one
identifier the methodology is built on.**

---

## 3. Reproduction

`scratchpad/repro/main.go`, decoding the two real bodies above with the current
struct shape and with the proposed one:

```
base side is the pool
  CURRENT  LiquidityPoolID=""  (err=<nil>)
  PROPOSED base="27480d04...cccb"  counter=""  fee_bp=30

counter side is the pool
  CURRENT  LiquidityPoolID=""  (err=<nil>)
  PROPOSED base=""  counter="27480d04...cccb"  fee_bp=30

Unmarshal returned err=<nil> for a body containing NO liquidity_pool_id key
```

The last line is why this survived. A wrong `json` tag is not a decode error in
Go. It is silence.

---

## 4. Why no test caught it

`internal/horizon/trades_test.go` contains no occurrence of `liquidity_pool` or
`LiquidityPool`. The decode path is exercised only with orderbook fixtures, and an
orderbook trade legitimately has no pool id, so every existing assertion is
satisfied by the empty string that the defect also produces. The bug and the
fixtures are indistinguishable.

`liquidity_pool_fee_bp` is dropped for a different reason: no struct field claims
it at all. It is 30 on every pool trade in the probe.

---

## 5. Blast radius: smaller than it looks

`LiquidityPoolID` appears in exactly four places in the repository:

| Location | Role |
|---|---|
| `internal/horizon/trades.go:229` | the wrong tag |
| `internal/horizon/trades.go:292` | copied into `domain.Trade` |
| `internal/domain/trades.go:144` | the field declaration |
| `cmd/keel/backtest.go:360` | written to the CSV |

It is **write-only**. Nothing reads it, nothing branches on it, no computation
consumes it. `domain.Trade.Type` carries `"liquidity_pool"` verbatim and is what
any existing venue logic uses.

Two consequences, and they point in opposite directions:

- No stored number is wrong because of this. Depth, manipulation cost and the
  daily series never touched the field. **The daily CSV does not change.**
- Nothing will start working merely because the field gets populated. It becomes
  correct evidence, available for a rule that does not exist yet.

---

## 6. Proposed fix, UNAPPLIED

### 6.1 `internal/horizon/trades.go`, replacing line 229

```go
	// THE POOL IS NAMED BY SIDE AND THERE IS NO BARE liquidity_pool_id. Horizon
	// sends base_liquidity_pool_id or counter_liquidity_pool_id and never both:
	// 171 and 29 of 200 pool trades on this pair, 0 with both, 0 with neither.
	// A tag reading `liquidity_pool_id` matches nothing, and encoding/json
	// reports no error for a key it was never asked for, which is why this was
	// the empty string on all 58 pool trades of the February CSV. See
	// docs/evidences/2026-08-31-trade-pool-id-defect.md.
	BaseLiquidityPoolID    string `json:"base_liquidity_pool_id"`
	CounterLiquidityPoolID string `json:"counter_liquidity_pool_id"`

	// Fee in basis points, sent only on a pool trade. A pool trade's price is
	// quoted after this fee, so the genuine-trade rule in 07 section 1 has to
	// decide whether it cares. Recorded rather than interpreted.
	LiquidityPoolFeeBP int `json:"liquidity_pool_fee_bp"`
```

### 6.2 `internal/horizon/trades.go`, two helpers

```go
// poolID returns the pool this trade touched, or "" for an orderbook trade.
//
// It does NOT infer the pool from an empty account. An orderbook trade with a
// missing account would then be read as a pool trade, and the two are not the
// same claim. Horizon states the side; this reads what it stated.
func (r tradeRecord) poolID() string {
	if r.BaseLiquidityPoolID != "" {
		return r.BaseLiquidityPoolID
	}
	return r.CounterLiquidityPoolID
}

// poolSide reports which side of the trade the pool was, "base", "counter", or
// "" when no pool was involved.
func (r tradeRecord) poolSide() string {
	switch {
	case r.BaseLiquidityPoolID != "":
		return "base"
	case r.CounterLiquidityPoolID != "":
		return "counter"
	default:
		return ""
	}
}
```

### 6.3 `internal/horizon/trades.go:292`, inside `trade()`

```go
		LiquidityPoolID:    r.poolID(),
		LiquidityPoolSide:  r.poolSide(),
		LiquidityPoolFeeBP: r.LiquidityPoolFeeBP,
```

### 6.4 `internal/domain/trades.go`, beside `LiquidityPoolID` at line 144

```go
	// LiquidityPoolSide is "base", "counter" or "". A pool trade has an account
	// on one side and a pool on the other, and which one it is changes what the
	// trade means: the pool was the seller in one case and the buyer in the
	// other. 49 of the 58 February pool trades had the pool on the base side.
	LiquidityPoolSide string

	// LiquidityPoolFeeBP is Horizon's liquidity_pool_fee_bp, 0 for an orderbook
	// trade. Not applied to anything here.
	LiquidityPoolFeeBP int
```

### 6.5 A test that would have caught it

Decoding a real pool body and asserting the id is non-empty. Both orientations,
because the base-side case alone passes with `poolID()` reading only one field.
Bodies are in `docs/evidences/2026-08-31-trade-pool-id-probe/pool-trades.json`.

### 6.6 The alternative I rejected

Keep the single `LiquidityPoolID` and infer the side from whichever account is
empty. It reconstructs from a hole something Horizon says outright, and it
converts any future orderbook trade with an absent account into a phantom pool
trade. There are 0 such rows in this file today, which makes it a silent failure
waiting rather than a visible one.

---

## 7. Does this change the February CSV? YES. Say so before regenerating it.

That file is cited as evidence in
`docs/evidences/2026-08-26-ustry-february-trades-implied.md` line 28.

**What changes:**

| | Before | After |
|---|---|---|
| Rows | 13,547 | 13,547, unchanged |
| Rows with a non-empty `liquidity_pool_id` | 0 | 58 |
| Every other column on every row | | unchanged |
| `...daily-2026-02-01_2026-03-01.csv` | | **unchanged**, per section 5 |

If the side and fee columns of 6.4 are also written, the header goes from 17
columns to 19 and every row gains two fields. That breaks any consumer indexing
by position. Populating `liquidity_pool_id` alone leaves the header untouched.
Both are defensible; they are different amounts of disruption to a file already
cited, so it is a decision rather than a detail.

**The regeneration is safe in a way the holder set was not.** `/trades` over a
closed window is addressed by cursor and returns the same records on every call,
unlike `/accounts`, which is current state and cannot be re-read for a past
ledger. Re-running the backtest for February 2026 reproduces these 13,547 rows
rather than sampling a new reality.

**One sentence in the citing document is currently false.** Line 28 describes the
CSV as carrying "every field as Horizon sent it". It does not: it drops
`base_liquidity_pool_id`, `counter_liquidity_pool_id` and
`liquidity_pool_fee_bp`. That line should be corrected whether or not the CSV is
regenerated, because it is the claim that would have stopped anyone from looking.

---

## 8. What this means for 07 section 1

The worksheet asks whether the genuine-trade rule treats pool trades differently
from orderbook trades. Today it can tell them apart, because `trade_type` is
correct on all 13,547 rows and the empty-account pattern corroborates it exactly.
It cannot name the pool, and until section 6 is applied it cannot say which side
the pool was on without the reader re-deriving it from a blank column.

Both matter for specimen C. Repeated small trades against a pool are the
signature of an arbitrage bot working a stale curve, which is a different
judgement from the same pattern between two accounts. Making that call needs the
pool named, and 58 trades is a small enough population to inspect by hand once it
is.

Nothing here proposes what the rule should be. That is section 1's job and it is
still unwritten.
