# DEC-020: Condition 4 asks for a book and is given only trades, and that is now the last thing between Keel and a full band confidence

**Status:** **DRAFT. Nothing in this record is in force.** Section 1 is a proposal and
section 6 is a choice nobody has made.
**Date drafted:** 2026-09-12
**Kind:** Definition. It changes WHEN a flag fires, which makes it a methodology change
rather than a wiring one, and `docs/methodology/` is RED. Section 1 is Al's to accept,
amend or reject, and Al's to write if accepted.
**Drafted by:** Claude
**Decided by:** pending
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must not
create or reverse a decision. Sections 3, 4 and 5 are measurements and code readings and
are evidence, not decision.
**Relates to:** DEC-019, which prices the trade-derived family by request budget. This
record is about a case that budget does not reach: a measure that stays unevaluated no
matter how many requests are spent on it.
**Coordination:** `docs/methodology/` is Track B's under `tugas-a.md` section 7. This
record proposes an edit inside it and does not make one.

---

## 1. The proposed decision

**Condition 4 may compare a pool fill against a RECORDED ORDER BOOK, and not only
against other order-book trades.** The rule's own words ask for "a book that existed near
it in time". Today the only thing it will accept as evidence of that book is another
trade, which is an inference from the book rather than the book itself, and Keel records
the book directly every fifteen minutes.

Three things are proposed and they are separable. Al may take any of them.

1. **A recorded book mid within ±15 minutes of a fill satisfies condition 4.** The
   existing window, the existing asymmetry, the existing Unevaluated fallback: only the
   admissible evidence widens.

2. **Trade evidence keeps priority where both exist.** Where an order-book trade falls
   inside the window it is used, exactly as today, so every figure this repository has
   already published stays reproducible. The recorded book is the fallback, not the
   replacement.

3. **A fill judged against a recorded book is labelled as such.** The classification
   already distinguishes Genuine, Excluded and Unevaluated; a fill judged against a book
   that was recorded rather than traded is a slightly weaker measurement and a reader is
   entitled to see which it was.

**What this record does NOT propose** is widening the ±15 minute window, which is
addressed and rejected in section 6.

---

## 2. What the rule says, and what the implementation does with it

`docs/methodology/07-supporting-metrics.md` section 1, condition 4:

> **Off-book pool fill** — a pool trade whose price is worse than the contemporaneous
> order book, evaluated at ±15 min.

and the paragraph that explains the fallback:

> *The ±15-minute window.* A pool fill can only be judged against **a book that existed
> near it in time**. When no **book trade** falls within ±15 minutes, the rule marks the
> fill Unevaluated rather than scoring it against an hour-stale price. Failing loud is
> safer than being silently wrong.

**The two halves of that paragraph are not the same requirement.** The first sentence asks
for a book. The second accepts only a trade as proof of one. When the rule was written
those were the same thing, because a recorded book did not exist to consult.

`internal/domain/supporting.go` implements the second sentence exactly, and its own
comments say so:

- `orderBookPricesByTime` collects "every ORDER-BOOK trade's price", pool fills excluded.
- `contemporaneousBookPrice` returns false "when no order-book trade falls inside it,
  which is the Unevaluated case".
- `classifyOne` turns that false into `GenuineStateUnevaluated`.

So a pair with a live, two-sided, recorded book and no recent book TRADES is treated
identically to a pair with no book at all. Those two are not the same market and the rule
does not mean them to be.

---

## 3. How often it cannot answer, measured on the live set

**Two thirds of the demonstration set cannot produce a genuine trade at all today.** In a
`keel scan` round run against Horizon on 12 September 2026, walking recent trades and
classifying them, **26 of 64 pairs yielded a last genuine trade and 38 did not**. Pairs
that widened the lookback to 168 hours and read between 500 and 3,900 trades still found
none genuine. The failure is not a short window.

**Because the set is pool-selected by construction.** `configs/demonstration-set.json`
records its own method: every constant-product pool holding USDC was enumerated, 879 of
them, and the 60 with the largest USDC reserve were kept. Sampled trade composition over
the most recent 200 trades per pair:

| Pair | Pool fills | Order-book trades |
|---|---|---|
| GROG | **200** | **0** |
| AFR | 199 | 1 |
| SHX | 150 | 50 |
| AQUA | 134 | 66 |
| yXLM | 34 | 166 |
| EURC | 0 | 200 |

**THE DIRECT MEASUREMENT OF CONDITION 4'S FAILURE RATE, over all 60 non-native pairs,
most recent 200 trades each, 12 September 2026.** For every pool fill, does an order-book
trade fall within the ±15 minutes the rule allows?

| | |
|---|---|
| pool fills seen | **9,495** |
| pool fills with NO order-book trade inside ±15 min | **8,777** |
| share the rule cannot judge | **92.4 per cent** |
| pairs where EVERY pool fill is unjudgeable | **40 of 60** |
| pairs with no pool fills at all | 3 |

Eight pairs returned 200 of 200 pool fills and zero order-book trades: AUDD, IDRT, AFR,
GOLD, LIBRE, XTAR, SSLX and LSP. For those, condition 4 has never had anything to compare
against and never will under the present rule.

**This is not the behaviour the August analysis describes, and the difference is the
asset set rather than the rule.** Section 1 of `07-supporting-metrics.md` reports 17
Unevaluated trades in August out of tens of thousands, on USTRY/USDC, a pair with
order-book trades. Run the same rule over a set chosen by enumerating liquidity pools and
the Unevaluated case stops being an edge and becomes the common one.

**And the book those pairs lack a TRADE from frequently exists.** Across the 61 live
assets, `priceSource` reads:

| `priceSource` | Assets |
|---|---|
| `book` | **28** |
| `pool` | 33 |

Twenty-eight assets had a usable order book at the last scan round. For any of those whose
recent trades are all pool fills, condition 4 declines to judge a fill while a book that
would have judged it was read, and stored, minutes earlier.

---

## 4. Why this is now the last blocker, and it was not last week

`internal/domain/flags.go` tiers, and the state after A7 landed on 12 September 2026:

| Flag | Tier | State |
|---|---|---|
| `HOLDER_CONCENTRATION_EXTREME` | HIGH | evaluates; the cache is filling |
| `MANIPULATION_RATIO_LOW` | HIGH | evaluates, DEC-017 implemented |
| **`NO_GENUINE_TRADE_30D`** | **HIGH** | **unevaluated, and this record is why** |
| `NO_GENUINE_TRADE_7D` | MEDIUM | the same cause |
| `WASH_TRADE_SUSPECTED` | MEDIUM | DEC-019's budget problem, a different cause |

`bandConfidence` reads `partial` while ANY critical or high tier flag is unevaluated.
`NO_GENUINE_TRADE_30D` is the only high-tier flag left, and it reads a
`LastGenuineTrade` that is nil precisely when condition 4 could judge nothing.

**So every asset in the demonstration set has read `partial` since the engine was
built, zero of 61 have ever read `full`, and this is the single remaining reason.**

---

## 5. What is already stored and unused

This is the part that makes option A in section 6 cheap rather than speculative.

`migrations/0001_core.sql` gives the `metrics` table `mid_price`, `price_source`,
`ledger_seq` and `ledger_closed_at`, and `keel scan` writes a row per asset per round,
every fifteen minutes, today. A row with `price_source = 'book'` carries a book mid at a
known ledger and a known close time.

**That is a contemporaneous order book price, already recorded, at a cadence finer than
the ±15 minute window the rule asks about.** No new table, no new request, no new
Horizon budget. The data condition 4 says it cannot find has been sitting one query away
since the first scan.

Its limit is honest and belongs here: it begins when scanning began, so it cannot judge a
fill older than the metrics history, and it covers only assets whose `price_source` was
`book` in the relevant round.

---

## 6. The options, priced

**A. A recorded book mid satisfies condition 4, trades keep priority.** Uses section 5's
existing rows. The change is in `internal/domain` (an input, plus a third provenance on
the classification) and in `07-supporting-metrics.md` section 1 (the definition). No new
requests. Estimated: 3 hours of code and tests, plus Al's definition.

**B. As A, and also retain a dedicated book snapshot per round.** Stronger, because it
does not depend on `price_source` having chosen the book, and it would let the recorder's
own archive be used for historical classification. It is a new table and a new write
path. Estimated: 6 to 8 hours. Worth doing after the deliverable, not before it.

**C. Keep condition 4 as written and state the consequence.** Costs nothing to build and
is not nothing: `11-limitations.md` and the D2 report would have to say that for
pool-dominant assets the staleness flags are permanently unevaluable and `bandConfidence`
is permanently `partial`. That is a defensible position, honestly stated.

**D. Widen the ±15 minute window. REJECTED, and the reason is not cost.** A wider window
does not create an order-book trade where there is none, so it does nothing at all for
GROG at 200 of 200 pool fills. Where it does change an answer it makes it worse, by
scoring a fill against a staler price, which is the exact failure the paragraph quoted in
section 2 says it is avoiding. It buys nothing and spends the rule's honesty.

---

## 7. The recommendation, and the reason is the rule's own words

**Option A.** Condition 4 asks whether a comparable book existed near the fill in time.
A book that Keel read, timestamped and stored answers that question better than a trade
from which the book must be inferred. Accepting the weaker evidence and refusing the
stronger one is not conservatism, it is an artifact of the order the two became available.

Section 2's own justification survives the change intact: the rule still fails loud when
there is nothing to compare against, it still refuses an hour-stale price, and it still
keeps cheaper-than-book fills. What changes is how many fills reach a comparison at all.

**If time runs out, option C is the honest fallback** and it must be written down rather
than left as a silence. A reviewer who sees `partial` on every row of a client
demonstration will ask why, and the answer should already be in the limitations document.

---

## 8. What this record does not decide

1. **Whether a book-judged fill is Genuine on the same terms as a trade-judged one.**
   Section 1 point 3 proposes labelling it; it does not propose a different threshold, and
   somebody may reasonably want one.
2. **Anything about `WASH_TRADE_SUSPECTED`.** It shares a cause with nothing here: it is
   blocked by DEC-019's request budget, and closing this record leaves it exactly where it
   was.
3. **The ±15 minute window.** Unchanged, and section 6D says why widening it is refused.
4. **Whether the February figures are recomputed.** Every published number came from
   trade evidence, which keeps priority under section 1 point 2, so nothing already
   reported changes. Whether to re-run anything is a separate question.

---

## 9. Reproducing section 3

```bash
# trade composition per pair, most recent 200 trades
curl -s "https://horizon.stellar.org/trades?base_asset_type=credit_alphanum4\
&base_asset_code=GROG&base_asset_issuer=GBE2P44LQXAXCRGGQARWIIM6C6Y6MS3S5FE2RENGO3S5JZFA6H5YTECL\
&counter_asset_type=credit_alphanum4&counter_asset_code=USDC\
&counter_asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN\
&order=desc&limit=200" \
  | python3 -c "import json,sys; r=json.load(sys.stdin)['_embedded']['records']; \
p=sum(1 for t in r if t.get('base_liquidity_pool_id') or t.get('counter_liquidity_pool_id')); \
print(p,'pool fills of',len(r))"

# price source across the live set
curl -s "https://api.keels.app/v1/assets?limit=200" \
  | python3 -c "import json,sys,collections; \
print(collections.Counter(i['priceSource'] for i in json.load(sys.stdin)['items']))"
```

The 92.4 per cent figure is the same shape of query run over every pair in
`configs/demonstration-set.json`, counting pool fills whose closest order-book trade is
more than 900 seconds away. The 26 of 64 figure came from a `keel scan` round with a trade
walk attached, reported in its own round summary.

---

## 10. Version history

| Date | Change |
|---|---|
| 12 September 2026 | Drafted. Nothing in force. Written on the day A7 landed, which is what made `NO_GENUINE_TRADE_30D` the last high-tier flag standing between the engine and a `full` band confidence |
