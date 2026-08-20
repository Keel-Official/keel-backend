# Keel Deliverable 1: Execution Plan

**The SOW promise:** a Liquidity Depth Engine. A backend that reads the SDEX
orderbook, AMM pool reserves, and trustline distribution. Per asset it computes
effective depth at +/-2%, +/-5%, +/-10%, holder concentration, the volume-to-supply
ratio, the time since the last genuine trade, and a recommendation for the maximum
safe collateral size. It supports historical replay at a past ledger state. The
methodology is documented and reproducible.

**Budget:** 126 hours = $2,268
**Evidence to be handed over:** a public repository, a methodology document, and
cross-validation results against Horizon on a sample of ledgers.

> **Translation note.** Translated to English under DEC-005 with its content
> unchanged. The TypeScript type sketches below are leftovers from before the
> project moved to Go; the shapes are still right, the syntax is not. The DoD in
> section 6 also promises eleven numbered methodology files, and
> `docs/methodology/README.md` section 3 documents why the real structure differs
> and recommends amending this section instead.

---

## 1. Allocating the 126 hours

| Component | Hours | When |
|---|---|---|
| D1.1 Data access layer (Horizon + Hubble) | 24 | Week 1 |
| D1.2 The depth computation core | 22 | End of week 1 into week 2 |
| D1.3 Supporting metrics | 20 | Week 2 |
| D1.4 Safe collateral size | 12 | Week 2 |
| D1.5 Historical replay | 20 | Start of week 3 |
| D1.6 Validation harness + 50 sample ledgers | 16 | Running from week 1 |
| D1.7 The methodology document | 12 | Continuous, finalised in week 3 |
| **Total** | **126** | |

Note that D1.6 and D1.7 are not end-stage tasks. Both run in parallel from day one.
Deferred, neither will finish.

---

## 2. Six definitional decisions to take before coding

The SOW names metrics but does not define them. The definitions are the
intellectual work of Deliverable 1, not the code. Take these decisions on days 4
through 6, write the reasoning down, then let the implementation follow.

### D-1. Depth is measured against which asset?

A Stellar asset can trade against many counter assets at once (XLM, USDC, others).
"USTRY depth" is meaningless without naming the other side.

Recommendation: compute depth for **every pair that has any liquidity**, then
designate one primary pair (the one with the largest total depth at 10%), and report
both. The reason: an attacker will use the cheapest route, so ignoring the secondary
pairs makes you too optimistic. But the headline number on a dashboard needs a
single value.

Also note: Stellar has path payments that can route through intermediate assets.
That adds effective liquidity invisible in the direct pair. For 30 days, state it as
a known limitation; do not attempt path finding.

### D-2. The mid price when the book is empty or one sided

This case is common on thin assets, and thin assets are exactly Keel's target.

The suggested fallback order:
1. A bid and an ask both exist: `P0 = (best_bid + best_ask) / 2`
2. Only one side of the book: use the pool spot price if there is one, and record a
   warning
3. No book, only a pool: `P0 = reserveQuote / reserveBase`
4. Neither: **`priceSource = 'none'`**

Case 4 is not an error and must not throw. An asset with no executable price is the
most dangerous asset you can find. It has to appear as a result with the maximum
risk score, not as a row missing from the report.

### D-3. A level that crosses the price limit: take it whole or not at all?

Walking the book, the last level usually crosses `P_limit`. Two options: discard
that level (conservative) or take it partially up to exactly the limit.

Recommendation: **discard.** More conservative, simpler, and for a product whose
purpose is to warn about risk, a conservative bias is the right direction. Document
that this is a deliberate choice, and state that it makes SDEX depth slightly lower
than the theoretical value.

### D-4. What is a "genuine trade"?

The SOW promises "time since the last genuine trade". It is the word *genuine* that
makes the metric valuable, because wash trading is the easiest way to make a dead
asset look alive.

The minimal definition you can implement in 30 days:

A trade **does not count** if any of the following is true:
- The buying account is the same as the selling account
- The notional is below a dust threshold (say the equivalent of $10)
- Either side is the asset's own issuer account
- Both sides are accounts whose trustlines were created in the same time window and
  which have only ever traded with each other (optional, expensive, mark it as v2 if
  time is tight)

What matters: **report how many trades were excluded and why.** The statement "87%
of the last 30 days of volume was excluded as non-genuine" is far stronger than a
bare date. It also makes your method auditable by someone else.

### D-5. Holder concentration is computed over what population?

The source is trustlines. But there are exclusions to decide:

- **The issuer account:** excluded. The issuer holds supply that is not yet
  circulating.
- **Liquidity pool reserves:** an asset locked in a pool is not held by a holder.
  Decide whether it enters the denominator, and stay consistent with the supply
  definition in D-6.
- **Custodian or exchange accounts:** cannot be reliably detected automatically. Do
  not try. State it as a limitation.

The metrics reported: the top-1 share, the top-10 share, and HHI. HHI is more robust
to a long tail than Gini and easier to explain.

**An important technical note:** do not pull the holder list from Horizon
`/accounts?asset=`. For an asset with thousands of trustlines, the pagination will
consume your rate limit budget (public Horizon caps at roughly 3600 requests per
hour per IP). Pull it from the Hubble `trust_lines` table in a single query. That
also gives you the historical version for free, which is what D1.5 needs.

### D-6. Which supply is used for the volume-to-supply ratio?

The options: total issued, total held by trustlines, or circulating supply after
subtracting issuer and pool holdings.

Recommendation: **the supply held by non-issuer trustlines.** That is the closest to
"how much could somebody actually sell". Volume comes from `/trade_aggregations` (or
`history_trades` in Hubble for the historical case) over 24 hour, 7 day, and 30 day
windows. Report all three, because thin assets often have zero 24 hour volume and
non-zero 30 day volume.

---

## 3. The components, one at a time

### D1.1 The data access layer (24 hours)

Two clients, one output shape.

```typescript
type Asset = { code: string; issuer: string | null };  // null means native XLM

type Level = { price: number; amount: number };        // amount in base units

type PoolReserves = {
  poolId: string;
  reserveBase: number;
  reserveQuote: number;
  feeBp: number;                                        // 30 for Stellar pools
};

type Snapshot = {
  base: Asset;
  quote: Asset;
  ledgerSeq: number;
  closedAt: string;
  book: { bids: Level[]; asks: Level[] };
  pools: PoolReserves[];
  source: 'horizon' | 'hubble';
};
```

`horizonClient.getSnapshot(base, quote)` and
`hubbleClient.getSnapshot(base, quote, ledgerSeq)` return an identical shape. The
rest of the engine must not know where the snapshot came from. That is what makes
D1.5 cheap.

What the Horizon client needs: retry with backoff, a request counter to protect the
rate limit, a cache with a TTL, and price normalisation (Horizon returns prices as
the `price_r` numerator/denominator fraction; do not use the `price` string without
checking its precision).

What the Hubble client needs: strict partition pruning
(`WHERE batch_run_date BETWEEN ...`), no `SELECT *`, and a local disk cache of
results, because every query costs money.

### D1.2 The depth computation core (22 hours)

```typescript
function computeDepth(snapshot: Snapshot, deltas: number[]): DepthResult;
```

A pure function. No fetching, no I/O, no system clock.

The algorithm for one delta, buy side:

```
P0        = midPrice(snapshot)
P_limit   = P0 * (1 + delta)
n_sdex    = sum of (price * amount) for every ask with price <= P_limit
n_amm     = per pool: reserveQuote * (sqrt(P_limit / P_pool) - 1),
            zero when P_pool >= P_limit, then grossed up by the fee
depth     = n_sdex + n_amm
```

The sell side is symmetric, using bids and `P0 * (1 - delta)`.

Tests that must exist before this module counts as done:
1. A testnet fixture with a hand-built orderbook whose depth you computed manually
   on paper.
2. A pool-only fixture with no orderbook, checked against the rule of thumb
   `depth ≈ (delta/2) * reserveQuote`.
3. Edge cases: empty book, one side empty, empty pool, two pools on the same pair, an
   asset with no price at all.
4. A monotonicity test: `depth(2%) <= depth(5%) <= depth(10%)`. If that fails there
   is a bug in the combination logic.

Test 4 is cheap and catches most combination mistakes.

### D1.3 Supporting metrics (20 hours)

Implement D-4, D-5, and D-6 above. All of them take a snapshot or historical data
with a `ledgerSeq`, and all of them return a value plus the list of exclusions
applied.

The suggested pattern: every metric returns `{ value, excluded, warnings }`. It is
that `excluded` field that makes your methodology inspectable by someone else, and
it is what separates a report that is trusted from one treated as a black box.

### D1.4 Safe collateral size (12 hours)

```
C_max = min( D_sell(delta_liquidation) * h , manipulation_cost(delta_critical) * m )
```

Both parameters need a defensible default value, not a number you invented. How to
get one: read Blend's actual risk parameters (liquidation threshold, close factor,
liquidation incentive), use those as the defaults, and **name the source in the
documentation**. The sentence "our defaults come from the Blend parameters in force
in May 2026" is far stronger than "we used a 50% haircut".

Make everything configurable. Keel is protocol agnostic, so the API has to let a
caller supply their own parameters.

Report the two sides separately, do not merge them into one number:
- **The liquidation limit**, from sell side depth, answering "if this position is
  liquidated, can the market absorb it"
- **The manipulation limit**, from the cost of pushing the price up, answering "is
  pumping this asset cheaper than the value that could be stolen"

An asset can pass one and fail the other. This is Keel's main differentiator from
the monitoring tools that already exist.

### D1.5 Historical replay (20 hours)

Because `computeDepth()` is already pure, this component only means building a
correct `hubbleClient.getSnapshot(base, quote, ledgerSeq)`.

The real work is in the validation, not the fetching. The mandatory order:

1. Pull a Hubble snapshot for a ledger you have a Horizon recording of (see section
   4).
2. Compare level by level, not just the total. A difference on one large offer can
   hide inside an aggregate figure.
3. Only after they match, run it over any date range.

If Hubble turns out not to hold `offers` snapshots densely enough, activate the
event reconstruction path: take the nearest snapshot before the target ledger, then
apply the `manage_buy_offer`, `manage_sell_offer`, and trade operations from
`history_operations` in order up to the target ledger. That adds 3 to 5 working days
and has to be paid for by cutting Deliverable 3 scope.

### D1.6 The validation harness (16 hours)

This is what produces the "cross-validation results" promised in the SOW evidence
table. Its design is described in section 4.

### D1.7 The methodology document (12 hours)

The suggested structure, one file per section under `docs/methodology/`:

```
00-ikhtisar.md              what is measured and why
01-sumber-data.md           Horizon versus Hubble, the limits of each
02-harga-acuan.md           the definition of P0 and the fallback order (D-2)
03-depth-sdex.md            walking the book, treatment of the last level (D-3)
04-depth-amm.md             the full derivation, treatment of the fee
05-penggabungan.md          why they are not summed directly
06-pemilihan-pasangan.md    numeraire and the primary pair (D-1)
07-metrik-pendukung.md      genuine trades, concentration, volume-supply (D-4 to D-6)
08-collateral.md            the C_max formula and where its defaults come from
09-validasi.md              the cross-validation protocol and its results
10-keterbatasan.md          what this method does not catch
```

`10-keterbatasan.md` is the file with the largest effect on your credibility. The
minimum contents: posted liquidity is not executable liquidity (an offer can be
withdrawn instantly), path payments through intermediate assets are not counted,
off-chain liquidity on centralised exchanges is invisible, and the safe thresholds
were chosen rather than calibrated from many incidents.

An experienced reviewer will look for this section. Its absence costs more than its
contents.

---

## 4. The cross-validation protocol

The SOW promises "cross-validation passed on at least 50 sample ledgers" but does
not define what is validated against what. You have to define it, and that
definition determines how convincing the evidence is.

Three layers, cheapest to strongest:

**Layer 1: manual recomputation (5 assets).**
Take a raw orderbook, copy it into a spreadsheet, compute the depth by hand. Compare
against the engine output. Attach the spreadsheet in the repository. This proves the
formula is right.

**Layer 2: testnet fixtures (10 scenarios).**
Assets and orderbooks you build yourself on testnet with numbers you choose. The
correct result is known before the code runs. This proves the implementation is
right.

**Layer 3: live Horizon versus historical Hubble (50+ pairs).**
This is what fulfils the SOW promise and this is what needs to start now.

How it works:

```
Starting Day 2:
  cron every 30 minutes
  for 8 selected assets:
    snapshot = horizonClient.getSnapshot(...)
    save raw to recordings/{asset}/{ledgerSeq}.json

Starting Day 16 (once Hubble has caught up):
  for every recording:
    h = hubbleClient.getSnapshot(asset, the same ledgerSeq)
    compare: level count, the price of each level, the amount of each level,
             pool reserves, and the computeDepth() result from both
    record: match / differ / delta
```

Two weeks of recording 8 assets every 30 minutes produces thousands of pairs. You
choose 50 as the reported sample, but you hold all of them in reserve. The results
go into `docs/methodology/09-validasi.md` as a table: asset, ledger, result,
difference.

If something does not match, that is not a failure. A difference explained
correctly (for example, Hubble snapshots at a batch boundary so it lags a few
ledgers) actually demonstrates that you understand the data. What is dangerous is
never having compared.

---

## 5. A work order that reduces risk

Do not build components to completion one at a time. Build a thin slice that cuts
end to end first.

**Slice 1 (days 2 to 5):** one asset, one pair, one delta, SDEX only, no pools, no
supporting metrics, the result printed to the terminal. The point is to prove the
whole chain works.

**Slice 2 (days 6 to 9):** add the AMM and the combination. Add all three deltas.
Still one asset.

**Slice 3 (days 10 to 13):** add the supporting metrics and C_max. Run it over 50
assets, store to the database.

**Slice 4 (days 16 to 19):** swap the data source to Hubble. If slices 1 through 3
were designed correctly, this is only swapping one interface implementation.

If you find that slice 4 requires changes to `depth.ts`, there is an abstraction
leak and it has to be fixed immediately rather than patched around.

---

## 6. Definition of Done for Deliverable 1

Tick every box before declaring it complete to the Ambassador Chapter Lead:

**Code**
- [ ] A public repository, with a README covering how to run it from nothing
- [ ] `computeDepth()` is a pure function with no I/O inside it
- [ ] Results for 50+ assets stored and queryable
- [ ] Historical replay works and has been validated against a control ledger
- [ ] Tests pass for 10 testnet fixtures and every edge case in D1.2
- [ ] Every result carries `ledgerSeq`

**Methodology**
- [ ] All eleven files under `docs/methodology/` complete
- [ ] Every decision D-1 through D-6 written down with its reasoning
- [ ] The AMM derivation present and followable by an outside reader
- [ ] The limitations file written honestly and specifically
- [ ] An outsider can reproduce one number from scratch using the documents alone

**Validation**
- [ ] The manual recomputation spreadsheet for 5 assets present in the repository
- [ ] The 50+ pair Horizon versus Hubble comparison tabulated
- [ ] Any differences that appear are explained, not ignored

**The final and most important test**
- [ ] Both builders can explain, without notes, why SDEX and AMM depth must not be
      summed separately, and why liquidation risk lives on the sell side while
      manipulation risk lives on the buy side.

If that last item fails, Deliverable 1 is not done even if all the code runs. It is
what will be tested during the SCF Build application.
