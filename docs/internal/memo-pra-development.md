# Keel: Pre-Development Memo

> **STATUS: ARCHIVED.** This file was previously named
> `docs/methodology/00-inti.md`, a misleading name because its contents are a
> handover memo, not the core of the methodology. It was moved to `docs/internal/`
> on 20 August 2026.
>
> | Section | Where its contents went |
> |---|---|
> | 1. Methodology v1.0.1 | merged into `docs/methodology/`, now at v1.0.3-draft and split under road 1 |
> | 2. API contract changes | replaced by `docs/decisions/DEC-003-api-contract-v1-1.md` |
> | 3. The golden fixture worksheet | filled in, now at `testdata/fixtures/ustry_pre_exploit.md` |
> | 4. The first three sessions | done |
> | 5. What was already settled | replaced by the status in README and `docs/methodology/README.md` |
>
> One correction to the contents below: section 1.2 writes the `SpreadExtremePct`
> default as **0.20**, a fraction. That is WRONG. The convention in force is
> percent, so the default is **20.0**. See `09-flags-and-bands.md` section 6.
>
> **Translation note.** Translated to English under DEC-005 with its content
> unchanged, including the error named above, which is left in place because this
> file is an archive and its errors are already annotated.

It contains three things: the methodology changes resulting from the last round of
verification, the API contract changes that have to be agreed with the frontend
builder, and the golden fixture worksheet you have to fill in yourself before
writing any implementation.

---

## 1. Methodology v1.0.1

Already merged into `docs/methodology/` on 20 August 2026, and split into numbered
files on 23 August 2026.
Section 1 below is therefore ARCHIVE, not a source of truth.

### 1.1 Section 10.4 is promoted

The sentence "there was no third party ask anywhere in the price range from 1.057 to
106.74" was previously marked as an inference. It is now a direct observation.

The evidence: the trade list of account
`GDHRCQNC64UVL27EXSC6OG6I2FCT4NWM72KNHLHKEB3LK4MEEYYWETN3` at
2026-02-22T00:10:21Z contains exactly one record, 5.3475699 USDC against offer
1824788980 owned by
`GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB`. Because Stellar's
matching engine fills from the best price and every match produces a trade record,
the absence of any other record proves no third party ask was swept.

Remove item 1 from the pending verification list in section 10.5.

### 1.2 A new flag: SPREAD_EXTREME

Found while building the fixture. On 21 February at 23:39 the USTRY/USDC book held
an ask at 106.7372828 and a bid at 1.057. The mid price becomes 53.8971414 for an
asset actually worth about 1.06.

A reference price loses its meaning when the spread reaches thousands of percent,
and every metric derived from it loses meaning with it. Other flags do still fire in
this case, but that is a coincidence, not the design.

```
spreadPct = (best_ask - best_bid) / P0
SPREAD_EXTREME fires when spreadPct > SpreadExtremePct   (default 0.20)
```

Band `HIGH`. `spreadPct` is also reported as a number in the API response, because
its magnitude is informative, not just whether it fired.

### 1.3 Reachable: two different meanings of a zero cost

A manipulation cost of zero can mean two opposite things:

| State | Meaning |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Cost = 0`, `Reachable = true` | the target price is attainable at no cost |
| `Cost = 0`, `Reachable = false` | there is no liquidity at all in that range, so the price cannot be walked up to it; it can only jump to the nearest available ask |

Without this distinction Keel's output becomes ambiguous on precisely the most
dangerous assets. That is why `MaxReachablePrice` was added: the highest price
attainable once every ask has been absorbed.

### 1.4 A principle reinforced

This case shows that `P0` and the ±2/5/10% ladder depend entirely on the existence
of a sane book. When the book is not sane, what saves the analysis is the large
delta ladder and `SPREAD_EXTREME`, not the SOW-mandated metrics. The mandated ladder
is still reported because it was promised, but it is not an oracle safety metric.

---

## 2. API contract changes

Agree these with the frontend builder, record them in `docs/decisions/`, then
freeze.

| Change | Detail |
| -------------------- | --------------------------------------------------------------------------------------------------------- |
| `Asset.type` | A new required field: `native`, `credit_alphanum4`, `credit_alphanum12`. Do not infer it from code length |
| `manipulationCost[]` | The ladder becomes 0.5, 1, 10, 100. Each entry gains `targetPrice` and `reachable` |
| `maxReachablePrice` | A new field, a decimal string or null |
| `oracleResistance` | A new field, `MC(critical) + genuine volume in the oracle window` |
| `spreadPct` | A new field, a decimal string or null |
| `flags` | Add `SPREAD_EXTREME` |
| `dataSource` | Add the value `trades-implied` |
| `/methodology` | Add `spreadExtremePct` and `oracleWindowSeconds` to `thresholds` |

Add one new example response named `assetBrokenBook`, using the fixture numbers from
section 3. This example matters to the frontend: a book with a 196 percent spread is
not an error and is not a normal condition, and its display has to be designed
specifically.

---

## 3. The golden fixture: your worksheet

This is the real USTRY/USDC orderbook state moments before ledger 61340263, derived
from on-chain operations. Your first fixture is real data, not invented numbers.

```
Snapshot
  base  : USTRY  GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC  (alphanum12)
  quote : USDC   GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN  (alphanum4)
  ledger: 61340263

  Asks: [ { price: 266843207/2500000, amount: 1.2185312 } ]     = 106.7372828
  Bids: [ { price: 1057/1000,         amount: 0.0001000 } ]     =   1.0570000
  Pools: []
```

Fill in the following table **by hand** before writing a single line of
implementation. Use a calculator or a spreadsheet, not Keel's code, because the
point is to test that code.

| Quantity | Your answer |
| ----------------------------------------- | ------------ |
| `P0` and `priceSource` | |
| `spreadPct` | |
| `depth(2%)` buy side / sell side | |
| `depth(5%)` buy side / sell side | |
| `depth(10%)` buy side / sell side | |
| `MC(δ=0.5)`: targetPrice, cost, reachable | |
| `MC(δ=1)`: targetPrice, cost, reachable | |
| `MC(δ=10)`: targetPrice, cost, reachable | |
| `MC(δ=100)`: targetPrice, cost, reachable | |
| `maxReachablePrice` | |
| The list of triggered flags | |
| Band | |

One example is worked through so the method is clear:

> **`MC(δ=1)`.** `P_target = 53.8971414 × 2 = 107.7942828`. **Cost**: the ask
> cheaper than the target is the ask at 106.7372828, so all of it counts.
> `cost = 1.2185312 × 106.7372828 = 130.0627093.`
> **Reachable**: no ask is priced at or above 107.7942828, because the only ask is
> priced at 106.7372828. So reachable = false.

Note that **Cost** and **Reachable** use different sets. Cost sums the asks cheaper
than the target; Reachable checks for the existence of an ask at or above it. An ask
will never belong to both at once.

Two hints so that you do not second-guess yourself when a result feels strange:

1. Some answers will be zero. A correct zero and a zero caused by a bug look
   identical in the output, so write the **reason** next to every zero.
2. At least one `MC` row will be `reachable = false`. If none is, re-check your
   understanding of section 1.3.

I deliberately have not filled it in. If this table is filled in after the code
exists, it merely confirms whatever your code did, and you lose the only safeguard
that actually protects this methodology.

---

## 4. The first three sessions

### Session 1, without Claude Code, about 45 minutes

Fill in the table in section 3. Save it as
`testdata/fixtures/ustry_pre_exploit.md` together with the reason for every number.
This becomes an evidence appendix for Deliverable 1.

### Session 2, Claude Code, scaffolding

```
Initialise a Go repository following the structure in CLAUDE.md.
Module: github.com/ciganytry/keel

Create go.mod, Makefile, .gitignore, .github/workflows/ci.yml (go vet,
golangci-lint, go test ./... -race, make arch), and
internal/domain/arch_test.go exactly as in
docs/architecture/technical-design.md section 2.1.

internal/domain/types.go already exists, do not change it.
Do not write any domain, adapter, or API implementation.
When you are done run make test and show me the result.
```

Then test the safety hook deliberately:

```
Create internal/adapters/horizon/probe.go importing
github.com/stellar/go/clients/horizonclient and calling txnbuild.NewTransaction.
This is to test my hook.
```

The correct outcome is **two refusals**, one for the old import path and one for
`txnbuild`. If it goes through, your hook does not work and you should fix it before
continuing.

### Session 3, you lead, Claude Code fills in

You write `internal/domain/depth_test.go` using the table in section 3. Only then
ask:

```
Implement MidPrice and ComputeDepth in internal/domain.
The signatures already exist in types.go, do not change them.
The definitions are in docs/methodology/03-reference-price.md and 04-depth.md.

Before writing code, show me the step by step derivation for the fixture in
depth_test.go so I can check it against my hand calculation.

Constraints: no float, no time.Now, sort map keys before iterating,
compare Price with Cmp rather than by division.
```

Stop there. The AMM, the combination, and manipulation cost come after SDEX depth
passes the fixture.

---

## 5. What was already settled

| | |
| ---------------- | ---------------------------------------------- |
| Asset identity | verified from the ledger |
| Incident date | 22 February 2026, verified, the SOW corrected |
| Ledger range | 61340263 and 61340272 confirmed |
| Manipulation cost | zero, a direct observation |
| Core methodology | v1.0.1, definitions locked |
| API contract | the section 2 changes just need agreeing |
| types.go | ready |
| BigQuery | not needed |

What remains: fill in the table in section 3, then start session 2.
