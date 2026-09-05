# Keel: the Horizon-only phase (deferring BigQuery)

**Decision:** BigQuery is deferred until it is proven to be genuinely needed.
**Supersedes:** the Day 0 ordering in the Readiness Checklist, which put the Hubble spike on the critical path.

> **The section 3 spike was finally run on 21 August 2026, and its answer is not
> the one this document expected.** Section 7 holds the result. The premise of the
> spike, that the USTRY/USDC trade history is thin, is false on trade count and true
> only on value. The curl in section 3 also gave USTRY the wrong asset type, which is
> fixed in place.

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

Document it in `docs/methodology/01-data-sources.md` section 6, including the
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

# 2. Pull the full USTRY/USDC trade history.
#    USTRY is credit_alphanum12; the wrong type returns an empty result and no error.
curl -s "https://horizon.stellar.org/trades\
?base_asset_type=credit_alphanum12&base_asset_code=USTRY&base_asset_issuer=<ISSUER_USTRY>\
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

---

## 7. The spike result, 21 August 2026

The DoD in section 3 asked for one table: trade count, unique account count, and the
size of the manipulation trade. Here it is, and two of the three answers change what
this document concluded.

| Question | Answer |
|---|---|
| Total trades in this market | **at least 12,000, and the count is unfinished** |
| Pages pulled before stopping | 60 at `limit=200`, a self-imposed cap |
| Time span those 60 pages covered | 2025-06-28 to 2025-07-01, **four days** |
| Unique accounts | **89** |
| Trade types | 11,545 orderbook, **455 liquidity pool** |
| Size of the manipulating trade | **5.3475699 USDC** for 0.0501003 USTRY, `trade_type: orderbook` |

### 7.1 The premise was wrong, and in an interesting way

Section 3 asked "how thin is the USTRY/USDC trade history?" and set the bar at "if it
is under a few thousand, the whole backtest can be done from Horizon alone".

Twelve thousand trades fit into four days of 2025. The full history is far larger and
this exercise did not reach the end of it. On **trade count** this market is not thin
at all; it is one of the busiest thin markets imaginable.

On **value** it is exactly as thin as reported. The amounts are dust: individual
trades of 0.0096631 and 0.0148813 USTRY, fractions of a cent. That is entirely
consistent with the "under $1 per hour" figure in DEC-001.

So the market is thick in count and negligible in value. Two accounts account for
almost all of it: `GB37DH4CM64RFUJ4LVNGTECDITMYELOBFUW7CR36644JZMFYZA3UBHQW` appears
on 11,670 trade sides and `GBMMYPWILFTPY5GCZ5Z63DP6Q72SUKB46E3VORXUDN2WI267O43LKF6O`
on 10,364, out of 24,000 sides in the sample. Two accounts trading dust with each
other thousands of times is the shape wash trading has.

That makes three things concrete rather than theoretical:

- **Counting trades is the wrong measure of liquidity**, which is the whole reason
  the genuine trade rules in the execution plan D-4 exist.
- **`WASH_TRADE_SUSPECTED` has a real subject.** This is not a hypothetical flag.
- **"Time since the last genuine trade" is the metric that matters here**, because
  time since the last *trade* would read "seconds ago" on a market that is dead.

### 7.2 Revisit condition 1 of section 6 may now be triggered

Section 6 says to activate Phase 3 if "the spike shows the USTRY/USDC trade history
is too large to pull through Horizon in reasonable time". On raw count it is: 60
requests covered four days out of a history spanning at least fourteen months.

But Phase 3 is about *precise historical depth*, and the trade count does not decide
that. The backtest needs the trades in a window around February 2026, not the whole
history, and Horizon's cursor is a TOID so a ledger range can be addressed directly
rather than walked from the beginning. Section 5.2 of DEC-001 shows how.

Recommendation: **do not activate Phase 3 on this evidence.** Bound the pull by
ledger range instead of pulling everything, and record the bound in the report. The
condition was written expecting a small history; the real situation is a large history
that can be sliced cheaply.

### 7.3 The finding that outweighs the DoD

455 of the 12,000 trades in the sample were `liquidity_pool` trades. That means a
USTRY/USDC AMM pool exists, and the golden fixture records `Pools: []`.

That is now `docs/decisions/DEC-006-amm-pool-in-the-fixture.md`, and it matters more
than anything else in this section. Section 1 of this document lists
`/liquidity_pools/{id}/operations` as available and says historical pool reserves are
"cleaner" than offers. That was right, and nobody had used it.

### 7.4 One caveat on the headline number

5.3475699 USDC is what the attacker **paid**. It is not the same quantity as Keel's
manipulation cost, and the two must not be conflated in the report.

Methodology section 7.2 is explicit: the cost is the notional paid to **other
parties**, and this payment went to an offer the attacker owned, so it returned to
them. What 5.3475699 USDC measures is the size of the trade needed to move the
oracle's reading, which is a different and also useful quantity, because it is the
capital an attacker has to be able to move rather than to spend.

Both belong in the report, labelled differently.

---

## 8. AMENDMENT, 3 September 2026: two acceptance criteria name Hubble, and this record does not know it

**This section AMENDS and does not reverse.** The deferral in section 1 stands as
written until Al says otherwise. `docs/decisions/` is YELLOW: Claude drafts and
amends a record and must not create or reverse a decision. What follows is a finding
and a re-costing handed over for a decision, and the decision is item B-8.

### 8.1 What this record claims, and what the PRD says

Section 1: "Exactly one thing is blocked without BigQuery: **the orderbook state at
a past ledger**." Section 4: "Phases 1 and 2 cover **all of Deliverable 1 except
precise replay**."

Both sentences are about capability. Neither is about the acceptance criteria, and
the acceptance criteria are what the deliverable is scored against. The PRD, at
`docs/context/Keel_PRD.md` since 5 September 2026 and at `docs/api/Keel_PRD.md` when
this section was written, section 9, reads verbatim:

> - [ ] **Horizon versus Hubble** cross-validation on at least 50 pairs, results tabulated

and section 4.2 reads:

> | FR-13 | Read a historical ledger snapshot from **Hubble** for a given `ledgerSeq` | **M** |

FR-13 is priority **M**, which the PRD defines as "without it the deliverable fails".
`internal/hubble/` holds one file, `CLAUDE.md`, and no Go at all. So a MUST is at zero
and an acceptance criterion names a source that does not exist in the repository.

### 8.2 Where the word was lost, because it was not lost here

This record never claimed the criteria were satisfiable without Hubble. The claim
entered somewhere else and by paraphrase. `docs/internal/deliverable-1-breakdown-2026-08-28.md`
renders criterion 3 as "Cross-validation on at least 50 pairs, tabulated", dropping
the two words that name the second source, and the 31 August and 2 September
breakdowns carry that paraphrase forward and score against it. Criterion 3 was scored
85, then 90, then 91 on a reading that had the source removed from it.

The 2 September breakdown also states that the PRD "lives in `docs/context/`, which
is gitignored and is not on disk". That is false: it is tracked at
`docs/api/Keel_PRD.md`, and section 9 is at line 312. The question the breakdown's
section 4 describes as needing ten minutes with the PRD could have been answered on
2 September from a committed file.

### 8.3 The one thing this amendment cannot settle

`10-validation.md` section 3 defines Layer 3 as "recorded Horizon versus
reconstructed history" and says of it: "This is the layer that satisfies the SOW
promise." So the methodology asserts a substitution. Whether the SOW permits it
cannot be checked here, because `docs/context/` is not on disk in this working copy.

Three readings, and only Al can close it:

1. The SOW says cross-validation without naming a source, the PRD narrowed it to
   Hubble on its own, and Layer 3 satisfies the SOW. Then criterion 3 is close to
   done and the PRD's wording is what needs amending, with the client informed.
2. The SOW names Hubble too. Then criterion 3 is not close to done and no amount of
   Layer 3 work closes it.
3. The SOW is silent and the PRD is the operative contract. Then the criterion means
   what it says.

**Under reading 1 or 3 the fix is a wording change to a client-facing document, which
is not a repository matter and is not Claude's.** Lowering a bar because the work did
not clear it is the move `internal/conformance` has a written rule against, and the
difference between amending a criterion the client agreed and quietly rescoring it is
the whole of the distinction.

### 8.4 The re-costing, which is the reason this section exists

The 2 September breakdown priced closing criterion 3 at **+1.3 overall points**, on
the ground that criterion 3 was already at 91. With the criterion read as written it
is not at 91, and the price changes:

| Criterion | Scored 2 Sep | Read as written | What one Hubble adapter does |
|---|---|---|---|
| 2, FR-12 to FR-17 | 88 | ~70, FR-13 is an M at zero and FR-14 has no second source to compare | → ~95 |
| 3, Horizon versus Hubble on 50+ pairs | 91 | ~40, the harness, recorder, tabulation and discipline all exist and carry over unchanged; the second reader does not exist | → ~100 |

That is **about +12 overall points from one piece of work**, against +1.3 as priced.
It is the largest single movement available to Claude on the whole board, and
`internal/hubble/` is YELLOW, so the writing is Claude's rather than Al's.

The three roads from B-8 are unchanged in kind and changed in weight:

1. **Fix the second cause in the reconstruction.** Now known to be structural to
   walking live offers backwards rather than a defect in it, so this road may not
   exist. It also does not touch FR-13 or the word Hubble either way.
2. **Take Hubble.** Reads the ledger as it was, walks nothing backwards, satisfies
   FR-13 and FR-14 and criterion 3 as written, all with one adapter. This is the road
   the evidence and now the criteria both point at.
3. **Change what the protocol asks for.** See 8.3: legitimate only as an amendment
   the client is told about, never as a rescoring.

### 8.5 A fourth revisit trigger, which section 6 does not list

Section 6 gives three conditions for activating Phase 3. None of them is "an
acceptance criterion names Hubble". Proposed as a fourth, for Al to accept or reject:

> 4. An acceptance criterion or a MUST-priority requirement names Hubble by name, so
>    no substitute satisfies it however good the substitute is.

Read against section 6 as it stands, trigger 2 is the closest fit and does not
actually fire: no reviewer has asked for precise historical depth. The criteria asked
first, in writing, before the sprint began, and this record's revisit list cannot see
a requirement it was written to work around.

### 8.6 What it would take to act, verified rather than assumed

Checked on 3 September 2026 on the working machine:

| Prerequisite | State |
|---|---|
| `gcloud` on PATH | absent |
| `bq` on PATH | absent |
| `~/.config/gcloud` | does not exist |
| A GCP project with the BigQuery sandbox | unknown, Al owns any account |
| `Bash(gcloud:*)` in `.claude/settings.json` | **deny** |
| `Bash(bq:*)` in `.claude/settings.json` | **ask** |

So the road is blocked twice over and only one of the two blocks is a decision. The
deny on `gcloud` is correct and should stay: authenticating a cloud account is Al's,
for the same reason `scripts/s3-archive/` splits PREPARE from APPLY. An agent that
provisions the storage its own evidence comes from has no chain of custody. The
interactive login is Al's to run.

`internal/hubble/CLAUDE.md` already carries the cost rules a sandbox needs, written
before any of this: partition filter first, explicit columns, dry run before every
query. Nothing about those changes.

### 8.7 One thing this record already asks for that is now Al's

Section 5 assigns an edit to the PRD: "Add a note that historical metrics in v1 may
be upper bounds rather than direct measurements". As of 3 September 2026
`docs/api/Keel_PRD.md` is refused to Claude in both the deny list and the hook,
because it is an input from outside and holds the criteria the work is scored
against. That edit is therefore Al's to make.

**It was never made.** Checked on 3 September 2026: `docs/api/Keel_PRD.md` contains
no occurrence of "upper bound", "trades-implied" or "trades_implied". So a note this
record asked for in August is still owed, and the contract carries the honesty that
the PRD does not: `dataSource: "trades-implied"` is in `docs/api/keel-openapi.yaml`
as section 5 also required. One of the two edits landed and the other did not, and
the one that did not is the one facing the reader who judges the deliverable.

---

### 8.6 The PRD moved on 5 September 2026, and every dated sentence above stands

Al moved the file to `docs/context/Keel_PRD.md`. The path in sections 8.1, 8.2 and 8.5
is left exactly as written, because each of those sentences is a dated claim about where
the file was on 3 September and each of them was true. Rewriting them would destroy the
finding they carry.

**Section 8.2 is the one to read twice.** It records that the 2 September breakdown
claimed the PRD "lives in `docs/context/`, which is gitignored and is not on disk", and
that three plans scored acceptance criteria against a paraphrase because of it. Half of
that sentence is now true: the file does live there. **The half that mattered is still
false, and deliberately so.** It is not gitignored and it is on disk, because a negation
line in `.gitignore` landed in its own commit before the move for exactly this reason.
Without it, a claim that cost three plans would have been made retroactively correct by
a filing decision, and every later session would have been unable to read the criteria
it is scored against.

The distinction is the whole point of that section: the error was never about which
directory the file sat in. It was about asserting a file is unreachable instead of
opening it.
