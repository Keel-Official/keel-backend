# Keel: Technical Design Document

**Version:** 0.2 (draft)
**Date:** August 2026, annotated 20 August 2026
**Author:** drafted together with Claude, the decisions are owned by the team
**Audience:** both builders, SCF Build reviewers

> **STALENESS WARNING.** This document was written when Keel was planned in
> TypeScript. The implementation is in **Go**. Any part naming a `.ts` file,
> `decimal.js`, `big.js`, Fastify, Hono, ESLint, or dependency-cruiser is a
> leftover and **does not apply**. The shape of the architecture still applies;
> only the tooling changed.
>
> | Section | What it says | What applies now |
> |---|---|---|
> | 2.1 | dependency-cruiser or the ESLint `import/no-restricted-paths` rule | `internal/domain/arch_test.go`, run through `make arch` in CI |
> | 3.3 | `decimal.js` or `big.js` | `github.com/shopspring/decimal`, decision T4 closed |
> | 4 | `.ts` modules under `domain/` | Go packages: types in `internal/domain/types.go`, formulas in `internal/domain/compute.go`, conformance tests in `internal/conformance` |
> | 5 | the `assets`, `metrics`, `runs` schema | present as `migrations/0001_core.sql`, reconciled with this section on 20 August 2026, and extended by `0002_methodology_103.sql` and `0003_venue_split_and_offers_implied.sql`. The schema in this document predates the v1.0.2 columns (`unevaluated_flags`, `band_confidence`, `spread_pct`, `max_reachable_price`); the migrations carry them |
> | 7 | Fastify or Hono | not decided for Go yet, see the open decisions |
> | 9 item 6 | the JSON determinism test | exists, `TestInvarianDeterminisme` in `internal/conformance` |
> | 11 and 13 T1/T2 | the fallback plan and the BigQuery budget | deferred entirely, see `docs/decisions/DEC-002-hold-bigquery.md` |
> | 6.4 | the request budget, and holder data coming from Hubble | annotated in place on 25 August 2026. Holders are read from Horizon now, and the recorder line is 48 per hour rather than 32 |

This document explains **how** Keel is built. What is built is in the PRD. What was
promised is in the SOW.

Sections marked **[AWAITING SPIKE]** cannot be finalised before the results of the
Day 0 Hubble spike.

---

## 1. Goals and non-goals

### Technical goals
1. Computation that is **deterministic and reproducible** at the same `ledgerSeq`
2. One computation implementation used by both the live and the historical path
3. The historical path can be swapped without touching computation code
4. Absolutely read-only, enforced mechanically rather than by convention

### Non-goals
- High availability. There is no SLA (NFR-4)
- Low latency for historical computation. It is batch, and it is stated as such
- Scale beyond a few hundred assets
- Multi-tenancy, authentication, user management

---

## 2. Architecture

```
                   ┌──────────────────┐
   Horizon ───────▶│                  │
   (current state) │                  │
                   │    ADAPTERS      │──▶ Snapshot ──┐
   Hubble ────────▶│                  │              │
   (historical)    └──────────────────┘              │
                                                      ▼
                                           ┌────────────────────┐
                                           │      DOMAIN        │
                                           │  depth, metrics,   │
                                           │  collateral, flags │
                                           │  (pure functions)  │
                                           └────────┬───────────┘
                                                    │ AssetRisk
                                                    ▼
   ┌──────────────┐      ┌───────────────┐   ┌──────────────┐
   │ ORCHESTRATOR │─────▶│    STORE      │◀──│  API (read)  │
   │ scan, replay │      │  PostgreSQL   │   └──────┬───────┘
   └──────────────┘      └───────────────┘          │
                                                    ▼
                                             ┌──────────────┐
                                             │  DASHBOARD   │
                                             └──────────────┘
```

### 2.1 Dependency rules (lint must enforce these)

```
domain/          must not import anything from adapters/, store/, api/
adapters/        may import types from domain/, must contain no logic
orchestrator/    may import everything
api/             reads store/ only, never calls adapters directly
```

The first rule is the decisive one. `domain/` must not know where a snapshot came
from. That is what makes historical replay a matter of swapping an adapter.

Enforce it with `dependency-cruiser` or the `import/no-restricted-paths` rule in
ESLint, and run it in CI. A rule that lives only in a document gets broken within
two weeks.

### 2.2 Why the API does not call adapters directly

If an API endpoint called Horizon when a request arrived, one popular asset could
burn your Horizon rate limit budget in minutes, and users would get unpredictable
latency. The API only reads results already computed by the orchestrator. The
consequence is that metrics always lag slightly, and that is accepted explicitly in
NFR-1 (at most 15 minutes).

---

## 3. Data sources

### 3.1 Horizon (the live path)

| Need | Endpoint | Note |
|---|---|---|
| Orderbook | `/order_book` | **Current state only. It takes no ledger parameter** |
| Pool reserves | `/liquidity_pools` | Current state only |
| Price series | `/trade_aggregations` | Historical, native, free |
| Trades | `/trades` | Historical, for genuine trade detection |
| Asset supply | `/assets` | Current state |

The `Latest-Ledger` response header is used as the snapshot's `ledgerSeq`. Do not
use the system clock.

### 3.2 Hubble (the historical path) [AWAITING SPIKE]

The `crypto-stellar.crypto_stellar` dataset on BigQuery. The tables needed:
`offers`, `liquidity_pools`, `trust_lines`, `history_trades`.

Every query must:
- prune partitions with `WHERE batch_run_date BETWEEN ...`
- never use `SELECT *`
- set `maximum_bytes_billed` so that a wrong query fails rather than bills

If the spike shows the `offers` snapshots are too sparse for May 2026, activate the
fallback plan in section 11.

### 3.3 Numeric precision (important, easy to get wrong)

This is the most common cause of cross-validation failing for no visible reason.

- Stellar amounts are **int64 in stroops**, 7 decimals. Store them as `bigint` or a
  string, never as `number`
- Horizon returns prices as a **rational fraction** `price_r: { n, d }`. Use that.
  The `price` string is already rounded and must not be used in a computation
- All depth arithmetic uses `decimal.js` or `big.js`, not IEEE 754 floats
- Convert to `number` only in the presentation layer, immediately before sending to
  the API

Break these and the Horizon path and the Hubble path will produce numbers that
differ in some later digit, cross-validation will fill up with false mismatches,
and you will chase a bug that does not exist.

---

## 4. Domain modules

> **TYPESCRIPT LEFTOVER.** The list of `.ts` files below does not apply. The real
> split is:
>
> | Go file or package | Contents | Zone |
> |---|---|---|
> | `internal/domain/types.go` | shared types only, no computation | yellow |
> | `internal/domain/compute.go` | every formula: reference price, SDEX and AMM depth, the combination, manipulation cost, flags, bands, C_max | red |
> | `internal/conformance` | the golden fixture and conformance tests, black-box against `internal/domain` | green |
>
> The split is by FILE and not by package, and it was by package until methodology
> 1.0.3. The formulas lived in `internal/depth`, that directory was emptied by 1.0.3
> and removed on 23 August 2026. A type is a shape and a formula is a claim, and only
> the second one has to be defended to a funder, which is why the two sit side by side
> in one package under different owners.
>
> The purity rules in the final paragraph of this section still apply in full, and
> are now enforced mechanically by `internal/domain/arch_test.go`.

```
domain/
  types.ts        Asset, Level, PoolReserves, Snapshot, AssetRisk, Flag
  price.ts        midPrice() and the fallback order (decision D-2)
  depthSdex.ts    walk the book
  depthAmm.ts     the constant product formula with fee
  depthCombine.ts combination by marginal price (decision D-3)
  manipulation.ts manipulation cost
  collateral.ts   C_max
  concentration.ts holder concentration
  activity.ts     genuine trades and volume-to-supply
  flags.ts        flag evaluation and band determination
  version.ts      METHODOLOGY_VERSION
```

Every file under `domain/` has to pass three conditions: no I/O, no `Date.now()`,
no `Math.random()`. If it needs the time, it takes it as an argument.

`flags.ts` holds every threshold as a named constant in one place, not scattered.
Thresholds are part of the methodology, and changing one has to change
`METHODOLOGY_VERSION`.

---

## 5. Storage schema

PostgreSQL. One managed instance on a free tier (Neon or Supabase) is enough.

```sql
create table assets (
  id             serial primary key,
  code           text not null,
  issuer         text,                      -- null for native XLM
  quote_code     text not null,             -- the primary pair
  quote_issuer   text,
  active         boolean not null default true,
  selection_note text,                      -- why this asset is in the demonstration set
  added_at       timestamptz not null default now(),
  unique (code, issuer, quote_code, quote_issuer)
);

create table metrics (
  id                  bigserial primary key,
  asset_id            int not null references assets(id),
  ledger_seq          bigint not null,
  ledger_closed_at    timestamptz not null,
  computed_at         timestamptz not null,
  methodology_version text not null,
  data_source         text not null,        -- 'horizon' | 'hubble'
  mid_price           numeric,
  price_source        text not null,        -- 'book' | 'pool' | 'none'
  depth               jsonb not null,       -- [{delta, buySide, sellSide, fromSdex, fromAmm}]
  manipulation_cost   jsonb not null,
  max_safe_collateral numeric,
  holder_top1_pct     numeric,
  holder_top10_pct    numeric,
  holder_hhi          numeric,
  volume_to_supply    jsonb,
  last_genuine_trade  jsonb,
  trades_excluded_pct numeric,
  flags               text[] not null default '{}',
  band                text not null,
  warnings            text[] not null default '{}',
  unique (asset_id, ledger_seq, methodology_version, data_source)
);

create index on metrics (asset_id, ledger_seq desc);
create index on metrics (band) where band in ('HIGH','CRITICAL');

create table runs (
  id            bigserial primary key,
  kind          text not null,              -- 'scan' | 'replay'
  started_at    timestamptz not null,
  finished_at   timestamptz,
  assets_ok     int not null default 0,
  assets_failed int not null default 0,
  notes         text
);
```

The unique constraint on `metrics` includes `methodology_version` and
`data_source`. That is deliberate: a result from a different methodology or a
different source is a different row, not an overwrite. It is what makes
cross-validation a single SQL query, and what stops a mid-sprint methodology change
from silently corrupting the time series.

**Raw snapshots are not stored in the database.** For 50 assets every 15 minutes
across 30 days that is tens of gigabytes for no benefit. What is stored is only the
recordings needed for cross-validation: 8 selected assets, as gzipped JSON files
under `recordings/`, 60 of which go into git as evidence.

---

## 6. Orchestrator

### 6.1 The scan job (live)

Runs every 15 minutes.

```
for every active asset:
  snapshot = horizonAdapter.getSnapshot(asset, quote)
  metrics  = computeAssetRisk(snapshot, params)
  store into metrics
  record per-asset failures, do not halt the whole job
```

One asset failing must not fail the scan. Failures are recorded in
`runs.assets_failed` and that asset keeps its previous result, marked stale.

### 6.2 The recorder job (cross-validation)

Runs every 30 minutes for 8 selected assets. It writes raw Horizon snapshots to
disk. **Starting Day 2**, not week 3, because a comparison baseline cannot be
created retroactively.

### 6.3 The replay job (historical)

Run manually with a ledger range. It uses `hubbleAdapter`. It writes to the same
table with `data_source = 'hubble'`.

### 6.4 The Horizon rate limit budget

> **ANNOTATED 25 AUGUST 2026. The table below is kept as written and two of its
> premises no longer hold.** The ceiling and the target are still right, and so is
> the instruction to recompute rather than raise.
>
> | Line | What it says | What is true now |
> |---|---|---|
> | "Holder data comes from Hubble" | holders cost this budget nothing | holders are read from **Horizon**, because DEC-002 defers Hubble and a trustline balance cannot be reconstructed for a past ledger from anything Horizon serves. See `internal/horizon/holders.go` |
> | "The recorder, 8 assets every 30 minutes = 32/hour" | 32 | **48**. A pair snapshot is 3 requests, and 8 pairs twice an hour is 48. The arithmetic was never rechecked after the recorder existed |
>
> **The line this table has no row for is the holder reading itself, and it is the
> only cost here that is not fixed per asset.** One request per 200 accounts, so an
> asset with 873 trustlines costs 5 and one at the 25 page cap costs 25. That is why
> `keel record` has `-holder-pages` and `-holder-interval`: the cap bounds one
> reading and the interval bounds how often the unbounded-in-principle cost is paid.
> A recorder holding 8 base assets at a 6 hour holder interval adds well under 200
> requests per hour at USTRY's size, and the honest statement is that this figure
> depends on which assets are chosen, which is why it is not written as a constant.
>
> Recompute this whole table when the demonstration set is settled, which is decision
> D-1 in `docs/methodology/02-pair-selection.md`.

Public Horizon limits roughly 3600 requests per hour per IP. Our target is under
3000.

```
Per asset per scan:  1 orderbook + 1 pool list + 1 trade_aggregations  = 3
50 assets                                                              = 150
A scan every 15 minutes                                                = 600/hour
The recorder, 8 assets every 30 minutes                                = 32/hour
Retry headroom                                                         = ~100/hour
                                                          Total        ≈ 750/hour
```

The headroom is large, and that is deliberate. Holder data comes from Hubble rather
than Horizon precisely in order to protect this budget. If the asset count rises to
200, recompute before raising it.

---

## 7. The API layer

- Framework: Fastify or Hono. Light, sufficient
- Reads from PostgreSQL only. Never calls an adapter
- 60 second response cache
- A rate limit of 60 requests per minute per IP
- No authentication. There is no user data
- CORS open. This is a public read-only API
- Every response carries `ledgerSeq`, `computedAt`, `methodologyVersion`,
  `dataSource`, `warnings`

The response headers include `X-Keel-Staleness-Seconds` so that a consumer knows
how far behind the data is without computing it themselves.

---

## 8. Degraded modes

This section is what separates a system that can be trusted from one that cannot.
Define it now, not when it happens.

| Condition | Behaviour |
|---|---|
| Horizon slow or down | The scan job partially fails, the API keeps serving the last result with a high `X-Keel-Staleness-Seconds` and an explicit warning |
| Hubble passes the cost budget | The historical endpoint returns 503 with a clear message. The live path is unaffected |
| An asset has no price at all | **Not an error.** `priceSource: 'none'`, flag `NO_EXECUTABLE_PRICE`, band `CRITICAL` |
| A new asset with no metric history | 404 with a message that the asset is not monitored, not a 500 |
| A historical query for a ledger not yet in Hubble | 404 with an explanation of data availability, not a 500 |

The third row is the most important and the easiest to implement wrongly. The
frontend has to receive it as a high-value finding, not as an error screen.

---

## 9. Reproducibility

NFR-9 states that re-running at the same `ledgerSeq` and `methodologyVersion` must
produce identical numbers. The mechanisms:

1. No `Date.now()`, `Math.random()`, or non-deterministic iteration order in
   `domain/`
2. Pools are sorted by `poolId` before processing
3. Orderbook levels are sorted by price, tie-broken by original order
4. All arithmetic is decimal, not float
5. `METHODOLOGY_VERSION` rises whenever a threshold or a formula changes
6. **An automated test:** run `computeAssetRisk` twice on the same fixture snapshot
   and compare the results as JSON strings. They must be identical byte for byte

Item 6 is cheap and catches violations of items 1 through 4 automatically.

---

## 10. Deployment

| Component | Platform | Note |
|---|---|---|
| API + orchestrator | One container on Railway, Fly.io, or Render | One process, jobs scheduled by an internal cron |
| Database | Managed PostgreSQL (Neon or Supabase) | A free tier is enough at this scale |
| Dashboard | Vercel or Netlify | Static, calls the API |
| Recorder | The same process as the orchestrator | Writes to a persistent volume |

No Kubernetes. No Terraform. No layered CI/CD. The deliverable is judged on
verifiable evidence, not on infrastructure sophistication.

Environment variables: the Horizon URL, BigQuery credentials, the database URL, the
query budget limit. All through platform secrets, never in the repository.

---

## 11. The fallback plan if the Day 0 spike fails [AWAITING SPIKE]

If the `offers` snapshots in Hubble are too sparse for May 2026:

**Reconstruction from events.** Take the nearest state snapshot before the target
ledger, then apply in order every operation that mutates the orderbook
(`manage_buy_offer`, `manage_sell_offer`, `create_passive_sell_offer`,
`path_payment_*`) plus the trades from `history_operations` and `history_trades` up
to the target ledger.

That is a straightforward state machine but it costs 3 to 5 working days, and it has
to be paid for by cutting Deliverable 3 scope in the order given in PRD section 12.

The adapter stays behind the same single interface. All that changes is the body of
`hubbleAdapter.getSnapshot()`. That is exactly why the dependency rules in section
2.1 must not be compromised.

---

## 12. Rejected alternatives

This section exists so that decisions are not re-argued in week 3.

| Alternative | Why it was rejected |
|---|---|
| Running our own captive core | Explicitly excluded in the SOW. The setup time and infrastructure cost are not worth it for 30 days |
| Computing metrics when a request arrives | Burns the Horizon quota, gives unpredictable latency, and is not reproducible |
| Storing every raw snapshot in the database | Tens of gigabytes for no benefit. A limited set of recordings for cross-validation is enough |
| Summing SDEX and AMM depth separately | Overstates liquidity. Both compete over the same price range |
| Using an external price feed for USD conversion | Violates principle P-1 in the PRD. Keel's argument becomes circular |
| Floats for amounts and prices | Breaks determinism and cross-validation |
| Kubernetes for deployment | The time spent produces no deliverable evidence whatsoever |
| A weighted composite risk score | The weights cannot be justified from a single incident. Replaced by rule based classification |

---

## 13. Decisions still open

| # | Decision | What it needs | Deadline |
|---|---|---|---|
| T1 | Hubble snapshot density, and whether the section 11 fallback is activated | The Day 0 spike result | Day 1 |
| T2 | The `maximum_bytes_billed` value and the monthly BigQuery budget | A cost estimate from the spike | Day 2 |
| T3 | The primary quote pair per asset: XLM or USDC | Decision D-1 in the Deliverable 1 plan | Week 1 |
| ~~T4~~ | ~~The decimal library~~ **CLOSED**: `github.com/shopspring/decimal`, present in `go.mod` | | done |
| T5 | The scan interval: 15 minutes or wider | The rate limit budget after real measurement | Week 2 |
| T6 | The final hosting platform | Team preference and free tier limits | Week 2 |
