# Keel: the Horizon-only phase (deferring BigQuery)

**Decision:** BigQuery is deferred until it is proven to be genuinely needed.
**Supersedes:** the Day 0 ordering in the Readiness Checklist, which put the Hubble spike on the critical path.

> **Translation note.** Translated to English under DEC-005 with its content
> unchanged. One known error is left in place and fixed under task T5: the curl
> command in section 3 gives USTRY the type `credit_alphanum4`, and USTRY is
> `credit_alphanum12`. See finding P1-22 in
> `docs/internal/audit-2026-08-20.md`.

---

## 1. What is actually blocked without BigQuery

Exactly one thing: **the orderbook state at a past ledger.**

What is **not** blocked, and this is the majority of the work:

| Need | Horizon source | Historical? |
|---|---|---|
| Current orderbook | `/order_book` | not needed |
| Current pool reserves | `/liquidity_pools` | not needed |
| Full trade history | `/trades` filtered by asset pair | **Yes, complete** |
| Price and volume series | `/trade_aggregations` | **Yes, complete** |
| Account operation history | `/accounts/{id}/operations` | **Yes, complete** |
| Pool operation history | `/liquidity_pools/{id}/operations` | **Yes, complete** |
| Holder list and balances | `/accounts?asset=` | current only |
| Asset supply | `/assets` | current only |

That means all of Deliverable 1 except replay, all of Deliverable 3, and **the
central claim of Deliverable 2** can be built without touching BigQuery.

---

## 2. Three substitutes for historical state, cheapest first

### 2.1 Manipulation cost read directly from the trades that happened

The strongest claim in your backtest report is not "USTRY depth at 2% was X on 20
February". The strongest claim is:

> A trade of size X moved the USTRY price from about $1.05 to about $107. The
> measured manipulation cost is X. The value borrowed against that manipulated price
> was $10.97 million.

X is **read directly** from `/trades`. No orderbook needed, no reconstruction
needed, no BigQuery needed. It is an on-chain fact anyone can verify with a single
curl.

If the circulating figures are right, the ratio is roughly 1 to tens of millions.
That is one sentence that sells Keel's entire premise, and you can own it this week.

### 2.2 An upper bound on depth implied by trades (a new piece of methodology, worth documenting)

This is the mathematically honest substitute for historical depth.

**The claim:** if a trade worth `S` shifts the marginal price by `δ`, then depth at
`δ` **cannot be larger than `S`**.

```
depth(δ) <= S,  for δ = |P_after / P_before - 1|
```

The reason is simple: if there were more liquidity in that price range, a trade of
size `S` could not have pushed through it.

This produces an **upper bound**, not an exact value. But for Keel's purpose an
upper bound is sufficient and in fact rhetorically stronger. You do not need to
prove USTRY depth was exactly $41. You need to prove it was **below the safe
threshold**, and an upper bound does that.

Its bias runs in the right direction: it can never make an asset look more dangerous
than reality, only potentially less. That is a bias you can defend in front of a
reviewer.

Document it as `docs/methodology/11-depth-tersirat-dari-trade.md`, including the
statement that this is an upper bound and not a direct measurement.

### 2.3 Full offer reconstruction from account operations

Only attempt this if 2.1 and 2.2 prove insufficient.

The procedure:
1. Pull every USTRY/USDC trade from `/trades`. Collect the set of accounts that ever
   participated
2. For each account, pull `/accounts/{id}/operations` over the February 2026 range
3. Filter for `manage_sell_offer`, `manage_buy_offer`, and
   `create_passive_sell_offer`
4. Build an offer state machine and apply the operations in order up to the target
   ledger

**A gap that must be documented:** an account that placed an offer and then
cancelled it without ever trading never appears in `/trades`, so it never enters the
account set. For a market with volume under $1 per hour the account count is small
and the gap is probably small too, but its existence has to be stated rather than
hidden.

Historical pool reserves are cleaner: `/liquidity_pools/{id}/operations` gives the
deposit, withdraw, and trade history per pool directly, with no such gap.

---

## 3. The new Day 0 spike

The old spike: "are the Hubble snapshots dense enough for February 2026?" It needs a
Google account, a quota, and learning BigQuery.

The new spike: **"how thin is the USTRY/USDC trade history?"** Free, no account, done
in 30 minutes.

```bash
# 1. Find the USTRY issuer from the attacker's burner account balances
curl -s "https://horizon.stellar.org/accounts/GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB" \
  | jq '.balances'

# 2. Pull the full USTRY/USDC trade history
curl -s "https://horizon.stellar.org/trades\
?base_asset_type=credit_alphanum4&base_asset_code=USTRY&base_asset_issuer=<ISSUER_USTRY>\
&counter_asset_type=credit_alphanum4&counter_asset_code=USDC&counter_asset_issuer=<ISSUER_USDC>\
&order=asc&limit=200" \
  | jq '._embedded.records[] | {ledger_close_time, base_amount, counter_amount, price, base_account, counter_account}'

# 3. The ledger sequence of the manipulation offer transaction
curl -s "https://horizon.stellar.org/transactions/09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb" \
  | jq '{ledger, created_at, successful}'
```

**Questions this spike answers:**

- How many trades exist across this market's entire history? If it is under a few
  thousand, the whole backtest can be done from Horizon alone
- How many unique accounts ever participated? If it is under a hundred, full offer
  reconstruction (2.3) is tractable too
- Exactly how large was the manipulation trade? This is the headline number of your
  report

**Definition of done:** one table holding the trade count, the unique account count,
and the size of the manipulation trade. Tell Kenny the result the same day.

---

## 4. The revised order of work

| Phase | Contents | BigQuery? |
|---|---|---|
| **Phase 1** (weeks 1 to 2) | Horizon reader, depth engine, supporting metrics, flags, C_max, a 50 asset scan, the recorder, the API, the dashboard against mocks | No |
| **Phase 2** (week 3) | Backtest from trade data: measured manipulation cost, implied depth, a ledger based chronology | No |
| **Phase 3** (if needed) | Precise historical depth via offer reconstruction or Hubble | Decided in week 3 |

Phases 1 and 2 cover all of Deliverable 1 except precise replay, all of Deliverable
3, and the central claim of Deliverable 2. If the sprint gets tight, Phase 3 is what
gets cut, and cutting it damages nothing essential.

---

## 5. Changes to other documents

| Document | Change |
|---|---|
| Readiness Checklist, block A | The Google Cloud and BigQuery items are **removed from the critical path**. There is no longer any third party dependency at Day 0 |
| Checklist, block B | The Hubble spike is replaced by the trade history spike in section 3 of this document |
| TDD section 3.2 | The Hubble adapter is still defined as an interface, its implementation deferred. The **[AWAITING SPIKE]** marker stays |
| TDD section 4 | Add `internal/domain/implied_depth.go` for the methodology in 2.2 |
| PRD | Add a note that historical metrics in v1 may be upper bounds rather than direct measurements, and that this is marked in the API response |
| OpenAPI | Add the value `dataSource: "trades-implied"` alongside `horizon` and `hubble`, so consumers know the nature of the number |
| Execution plan D1, D1.5 | Historical replay goes through the trade path first; Hubble becomes an optional improvement |

Adding `dataSource: "trades-implied"` matters. A number that is an upper bound must
not look the same as a number that was measured directly. That honesty shows up in
the API, not only in a document.

---

## 6. When to revisit this decision

Activate Phase 3 if any of these happens:

1. The spike shows the USTRY/USDC trade history is too large to pull through Horizon
   in reasonable time
2. A reviewer or the Ambassador explicitly asks for precise historical depth rather
   than an upper bound
3. Phases 1 and 2 finish ahead of schedule and week 3 has time left over

If none of those happens, finish the sprint without BigQuery at all. That is a valid
outcome and in fact easier for someone else to reproduce, because a third party can
verify every one of your numbers with curl alone.
