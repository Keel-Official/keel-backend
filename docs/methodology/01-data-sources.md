# Keel: Data Sources

**Methodology version:** 1.0.8-draft
**Status:** complete. Every claim here was verified against Horizon mainnet during this
project; none is quoted from documentation alone.

This document states where every number Keel publishes comes from, what each source can
and cannot answer, and where each source fails silently. It exists so that an outside
reader can retrace any figure without access to our machines.

---

## 1. Source inventory

### 1.1 Horizon mainnet, read only

`https://horizon.stellar.org`. No account, no key, no cost.

Horizon exposes two categories of data that behave differently, and conflating them is
the single most consequential mistake available here.

| Category | Endpoints | Historical? |
|---|---|---|
| **Event data**, append-only | `/trades`, `/operations`, `/effects`, `/trade_aggregations` | Yes, complete |
| **State data**, current only | `/order_book`, `/liquidity_pools`, `/offers`, `/accounts`, `/assets` | **No** |

State endpoints accept no ledger parameter. There is no way to ask Horizon for the USTRY
order book as it stood in February 2026. Any design that assumes otherwise will fail at
the point where historical replay is implemented, which is late enough to be expensive.

Endpoints Keel uses, and for what:

| Need | Endpoint | Notes |
|---|---|---|
| Order book | `/order_book` | current state only |
| Pool reserves, current | `/liquidity_pools?reserves=A,B` | current state only |
| Pool reserves, historical | `/liquidity_pools/{id}/effects` | reserves are embedded in each effect |
| Trade history | `/trades` with asset pair filters | complete history, free |
| Price series | `/trade_aggregations` | complete history, free |
| Account order history | `/accounts/{id}/operations` | complete history |
| Holder distribution | `/accounts?asset=` | current state, paginated |
| Circulating supply | `/assets` | current state |
| Ledger close time | `/ledgers/{seq}` | the only correct time source |

**Rate limit.** The public instance allows roughly 3600 requests per hour per IP. Keel
budgets under 3000. Holder distribution is fetched at most once per day rather than per
scan, because depth changes by the minute while holder distribution does not.

### 1.2 Hubble on BigQuery, deferred

`crypto-stellar.crypto_stellar`. The only source that stores historical snapshots of
ledger state tables.

**Not used in version 1.0.x.** See `docs/decisions/DEC-002`. The Blend backtest is served
by event reconstruction from Horizon, which is free, requires no account, and keeps every
published figure reproducible with `curl` alone. Hubble remains a documented upgrade path
if precise historical order book state is later required.

### 1.3 Trade-implied bounds

When historical order book state is unavailable, depth is bounded from executed trades.
See `00-core.md` section 12. Results derived this way carry
`dataSource: "trades-implied"` and must never be presented as measurements.

---

## 2. Silent failures

All four were encountered during this project. None produces an error. Each has a
corresponding adapter test.

### 2.1 Asset type must be explicit

USTRY has a five character code and is therefore `credit_alphanum12`. Querying with
`credit_alphanum4` returns an empty array and HTTP 200.

```bash
# returns nothing, no error
.../trades?base_asset_type=credit_alphanum4&base_asset_code=USTRY&...
```

Asset type is a stored field on `domain.Asset`, never inferred from `len(code)` at call
time.

### 2.2 Two price shapes, two field names, two directions

```json
/offers  -> "price_r": {"n": 266843207, "d": 2500000}      JSON numbers
/trades  -> "price":   {"n": "2500000", "d": "266843207"}  JSON strings
```

Different field name, different JSON type, same concept. Worse, the direction of `price`
on `/trades` depends on which asset Horizon treats as the base. On the manipulating trade
the base was USDC, so `price` reads 2500000/266843207, the inverse of what a reader
expects.

Adapters normalise both into quote-per-base before anything reaches `internal/domain`.
The rounded `price` string is never used for computation.

### 2.3 Pool effects include other pools

`/liquidity_pools/{id}/effects` returns every effect of every operation that touched the
pool, including effects on **other** pools reached by the same path payment. Reading
`reserves` without filtering on `liquidity_pool.id` yields another pool's reserves, with
no error.

This was observed directly: an unfiltered query for the USTRY/USDC pool returned reserves
for an XLM/Vol pool.

### 2.4 Reserve ordering is canonical, not query order

The `reserves` array is ordered by the protocol, not by the order assets were named in
the query. For the USTRY/USDC pool, USDC comes first. Base and quote are assigned by
reading each element's `asset` field.

### 2.5 Ledger sequence is not a clock

Ledger close intervals average roughly 5.8 seconds, not 5. Deriving a timestamp from a
ledger number arithmetically drifts by about three weeks over six months. Time comes from
`closed_at` on `/ledgers/{seq}`, or from `created_at` on the record itself.

---

## 3. Provenance requirements

Every figure Keel publishes carries `ledgerSeq`, `methodologyVersion` and `dataSource`.
Timestamps alone are insufficient: the ledger sequence is the reproducibility anchor.

For figures used in the backtest report, an additional standard applies. Each claimed
number must have one of:

1. On-chain data reproducible by a stated command, or
2. An official statement from a party involved

Figures from news articles or audit blogs are treated as leads to be located on-chain,
never cited as facts.

---

## 4. Reproducing the fixture from scratch

Every command below runs against public Horizon with no credentials.

```bash
USTRY_ISSUER=GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC
USDC_ISSUER=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN
BURNER=GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB
POOL=27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb

# 1. Confirm the ledger and its close time
curl -s "https://horizon.stellar.org/ledgers/61340263" | jq '{sequence, closed_at}'

# 2. The manipulation offer, still open today
curl -s "https://horizon.stellar.org/offers?seller=$BURNER" \
  | jq '._embedded.records[] | {id, price_r, price, amount, last_modified_ledger}'

# 3. Both orders as placed, with their original amounts
curl -s "https://horizon.stellar.org/accounts/$BURNER/operations?order=asc&limit=200" \
  | jq '._embedded.records[] | select(.type | test("offer"))'

# 4. The manipulating trade, and proof no third party was involved
curl -s "https://horizon.stellar.org/accounts/GDHRCQNC64UVL27EXSC6OG6I2FCT4NWM72KNHLHKEB3LK4MEEYYWETN3/trades?order=asc&limit=200" \
  | jq '._embedded.records[] | {ledger_close_time, base_amount, counter_amount, price, base_account, counter_account}'

# 5. Pool reserves at the target ledger.
#    Page backwards until an effect belonging to THIS pool predates the target.
curl -s "https://horizon.stellar.org/liquidity_pools/$POOL/effects?order=desc&limit=200" \
  | jq --arg p "$POOL" '[._embedded.records[]
      | select(.created_at < "2026-02-22T00:10:21Z")
      | select(.liquidity_pool.id? == $p)][0]
      | {created_at, type, reserves: .liquidity_pool.reserves}'
```

Command 4 is the one that establishes the headline finding. A single trade record at
`00:10:21` proves the buy matched only the attacker's own offer, so no third-party asks
existed anywhere between 1.057 and 106.74.

Command 5 returns an effect dated `2026-02-10T16:59:35Z`. The next effect touching this
pool is dated `2026-02-22T22:08:33Z`, so those reserves held unchanged across the entire
attack and no arithmetic reconstruction is required.

---

## 5. What these sources cannot answer

| Question | Why not |
|---|---|
| What was the order book at an arbitrary past ledger? | Horizon state endpoints are current only; Hubble is deferred |
| Which orders belonged to the attacker? | Not knowable ahead of time, which is why manipulation cost is an upper bound |
| Which accounts are custodians or exchanges? | Not reliably detectable; not attempted |
| Does liquidity exist off-chain? | Out of scope; Keel measures on-chain only |
| What exactly does the oracle read? | Not published; treated as an inference and marked as such |

---

## 6. Trade-implied depth as a bound

**Moved here in the road 1 split**, from `keel-methodology-core.md` section 12. It is a
technique for getting a number out of a source that cannot answer the question
directly, which is what this file is about.

When historical order book state is unavailable, depth can be bounded from trades that
did occur.

```
depth(δ) ≤ S    if a trade of size S moved the marginal price by δ
```

If liquidity within that range exceeded `S`, a trade of size `S` could not have crossed
it.

The result is an upper bound, not a measurement. That is sufficient for Keel's purpose,
because what must be shown is not the exact depth but that depth sits below the safe
threshold.

Results derived this way **must** be tagged `dataSource: "trades-implied"` in API
responses.

---

## 7. Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Initial document. Consolidates source facts previously scattered across DEC-001, DEC-002 and DEC-003 |
| 1.0.8-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** `07` had run to 1.0.8-draft alone; Al ratified one version for the whole set so that a reader cannot cite two. README section 4 and DEC-014 carry the reasoning |
