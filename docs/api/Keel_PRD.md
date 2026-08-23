# Keel: Product Requirements Document

**Version:** 1.0
**Date:** August 2026
**Status:** Active for the 30 day Instawards sprint
**Owner:** Ciganytry (Rafli Ahmad Denistri)

This document complements the SOW, it does not replace it. The SOW is the
commitment to the funder about what gets delivered. This PRD is the product
definition for the builders: who the users are, what gets built, and how we know it
is right.

> **Translation and staleness note.** Translated to English under DEC-005 with its
> content unchanged. Section 5 is stale: `09-flags-and-bands.md` states that it
> supersedes PRD sections 5.1 and 5.2 and that the PRD now only points to it, but
> sections 5.1 and 5.2 below still carry full definitions, and they predate
> `SPREAD_EXTREME` and the three-state flag model. Two homes for one definition
> guarantee both drift, which is exactly what happened. Reconciling this is a
> content change, not a translation.
>
> Note also that this document still says the incident was in May 2026. DEC-001
> corrected that to 22 February 2026. Section 12 of that decision record calls for
> changing every "May 2026" reference here, and that has not been done.

---

## 1. Product summary

Keel is a liquidity risk engine for Stellar assets. It answers one question nobody
in the Stellar ecosystem answers today:

> This asset's price is X. How large a transaction can that price support?

An oracle answers "what is the price". Keel answers "how deep is the market". That
difference is what went missing during the Blend incident of May 2026, when the
USTRY price was pushed up 100 times through a thinly traded feed and then used as
collateral to borrow $61 million in XLM.

---

## 2. Users

### 2.1 The real users during these 30 days

Honesty is needed here, because it determines the design priorities. During the
Instawards sprint, Keel's primary users are **not** lending protocols. They are:

| User | Need | Design implication |
|---|---|---|
| **P1. The Ambassador Chapter Lead** | Verify the deliverable is complete without technical expertise | The dashboard has to be understandable with no explanation. The evidence has to be a clickable link |
| **P2. An SCF Build reviewer** | Judge whether the methodology is sound and the gap is real | A reproducible methodology document and backtest matter more than features |
| **P3. A prospective technical user** (protocols, vault curators, RWA issuers) | Judge whether the metrics are useful for their decisions | An API they can try in 5 minutes, with no registration |

The conclusion to hold on to: **clarity beats completeness.** A clear dashboard of
30 assets beats a confusing dashboard of 50. A backtest somebody else can reproduce
beats three extra features.

### 2.2 The intended users after SCF Build

Lending protocols setting collateral risk parameters, vault curators selecting
assets, and RWA issuers wanting to prove their asset is liquid. Their needs shape
the product direction but do **not** shape the 30 day scope.

---

## 3. Product principles

Four principles that settle design arguments without needing to reopen them.

**P-1. Keel does not depend on an external price oracle.**
Keel's entire premise is to question whether a reported price can be trusted. If
Keel used a third party price feed to convert to USD, the argument would be
circular and easy to break. Every value is expressed in **its own quote asset**
(XLM or USDC), not in converted USD. If a USD display is needed in the dashboard,
mark it clearly as an indicative conversion with its source named, and never use it
in a computation.

**P-2. A conservative bias in every ambiguous case.**
Where two reasonable interpretations exist, choose the one that yields lower depth
and a higher risk assessment. A warning product that is too optimistic is useless.
Write every one of these conservative choices into the methodology document.

**P-3. Every number must be traceable.**
Every output carries `ledgerSeq`, `methodologyVersion`, and `dataSource`. A number
without all three cannot be re-verified and is therefore worthless for Deliverable
1.

**P-4. Absence of data is a finding, not an error.**
An asset with no orderbook, no pool, or no trades is the most dangerous asset that
can be found. It has to appear in the output with the highest risk marker, not
vanish from the list or raise an exception.

---

## 4. Functional requirements

Priority: **M** = must (without it the deliverable fails), **S** = should (cut only
under duress), **C** = could (cut first).

### 4.1 Core computation

| ID | Requirement | Prio |
|---|---|---|
| FR-1 | Compute effective depth at +/-2%, +/-5%, +/-10% from the SDEX orderbook | M |
| FR-2 | Compute effective depth from AMM pool reserves using the constant product formula with fee | M |
| FR-3 | Combine SDEX and AMM at the same final marginal price, not by independent summation | M |
| FR-4 | Report the buy side and the sell side separately at every delta level | M |
| FR-5 | Handle every edge case (empty book, one side, empty pool, several pools, no price) without an exception | M |
| FR-6 | Compute the manipulation cost: the notional needed to shift the price by delta | M |
| FR-7 | Compute a maximum safe collateral size recommendation, with configurable parameters | M |
| FR-8 | Compute holder concentration: top-1 share, top-10 share, HHI | S |
| FR-9 | Compute the volume-to-supply ratio over 24 hour, 7 day, and 30 day windows | S |
| FR-10 | Compute the time since the last genuine trade, with documented wash trade exclusion rules | S |
| FR-11 | Compute depth for every quote pair that has any liquidity, and designate a primary pair | S |

### 4.2 Data and replay

| ID | Requirement | Prio |
|---|---|---|
| FR-12 | Read the current orderbook and pools from Horizon mainnet, read-only | M |
| FR-13 | Read a historical ledger snapshot from Hubble for a given `ledgerSeq` | M |
| FR-14 | Both sources return an identical `Snapshot` shape | M |
| FR-15 | Record Horizon snapshots periodically as cross-validation ground truth | M |
| FR-16 | Produce a metric time series for one asset over a ledger range | M |
| FR-17 | Run the engine over at least 50 active Stellar assets and store the results | M |

### 4.3 Public API

| ID | Requirement | Prio |
|---|---|---|
| FR-18 | `GET /v1/asset/{code}:{issuer}/depth` returns the current metrics | M |
| FR-19 | The `?ledger=` parameter returns the metrics at a historical ledger state | M |
| FR-20 | `GET /v1/assets` returns the monitored assets with their risk bands | M |
| FR-21 | Every response carries `ledgerSeq`, `computedAt`, `methodologyVersion`, `dataSource`, `warnings` | M |
| FR-22 | API documentation (OpenAPI) that can be tried without registration | M |
| FR-23 | A `?quote=` parameter to select a quote asset other than the primary pair | C |

### 4.4 Dashboard

| ID | Requirement | Prio |
|---|---|---|
| FR-24 | A table of every monitored asset with its risk band and triggered flags | M |
| FR-25 | A per-asset detail page: depth curve, supporting metrics, C_max | M |
| FR-26 | A Blend case study page with a manipulation cost ratio chart across May 2026 and a marker on the exploit date | M |
| FR-27 | Historical trend per asset | S |
| FR-28 | A short explanation of each metric readable by a non-technical audience | S |
| FR-29 | Asset search and filtering | C |

---

## 5. Risk band definitions

This section settles what the SOW only called "risk scores".

**Decision: rule based classification, not a weighted composite score.**

The reason: a weighted 0 to 100 score obliges you to justify every weight, and
there is no empirical basis for justifying weights from a single incident. Rule
based classification only obliges you to justify the thresholds, and every flag can
be inspected separately by a user with their own policy.

### 5.1 Flags

Every asset is evaluated against the following flags. **Flags are reported
individually in the API, not just the band**, so a protocol can apply its own
policy.

| Flag | Condition |
|---|---|
| `NO_EXECUTABLE_PRICE` | `priceSource = none`. No orderbook and no pool |
| `ZERO_DEPTH_2PCT` | Depth at +/-2% is zero on either side |
| `MANIPULATION_CHEAP` | MANIPULATION_CHEAP fires when: THERE EXISTS a delta d with Reachable(d) == true AND Cost(d) < ManipulationCheapAbsolute |
| `MANIPULATION_RATIO_LOW` | The cost of raising the price 50% is less than 1% of the circulating supply value |
| `NO_GENUINE_TRADE_30D` | No genuine trade in 30 days |
| `NO_GENUINE_TRADE_7D` | No genuine trade in 7 days |
| `HOLDER_CONCENTRATION_EXTREME` | The single largest holder controls more than 50% of non-issuer circulating supply |
| `HOLDER_CONCENTRATION_HIGH` | The ten largest holders control more than 80% |
| `THIN_DEPTH_5PCT` | Depth at +/-5% is below an absolute threshold (default: the equivalent of 50,000 XLM) |
| `WASH_TRADE_SUSPECTED` | More than 50% of 30 day volume is excluded by the genuine trade rules |

### 5.2 Bands

The band is the worst triggered flag. No weighting, no averaging.

| Band | Fires when any of these is present |
|---|---|
| **CRITICAL** | `NO_EXECUTABLE_PRICE`, `ZERO_DEPTH_2PCT`, `MANIPULATION_CHEAP` |
| **HIGH** | `MANIPULATION_RATIO_LOW`, `NO_GENUINE_TRADE_30D`, `HOLDER_CONCENTRATION_EXTREME` |
| **MEDIUM** | `THIN_DEPTH_5PCT`, `NO_GENUINE_TRADE_7D`, `HOLDER_CONCENTRATION_HIGH`, `WASH_TRADE_SUSPECTED` |
| **LOW** | None of the above |

### 5.3 The documentation obligation on thresholds

Every threshold number above is **chosen, not calibrated**. That has to be stated
explicitly in `docs/methodology/` and on the dashboard. Suggested wording:

> These thresholds were chosen based on the magnitude of the Blend incident of May
> 2026 and on conservative judgement, not calibrated against a set of incidents.
> Calibration requires more events than are available. Every flag is reported
> separately so that users can apply their own thresholds.

Saying this strengthens your position rather than weakening it. An experienced
reviewer will look for whether you know the limits of your own claims.

---

## 6. Data contract

```typescript
type Asset = { code: string; issuer: string | null }; // null means native XLM

type Flag =
  | "NO_EXECUTABLE_PRICE"
  | "ZERO_DEPTH_2PCT"
  | "MANIPULATION_CHEAP"
  | "MANIPULATION_RATIO_LOW"
  | "NO_GENUINE_TRADE_30D"
  | "HOLDER_CONCENTRATION_EXTREME"
  | "THIN_DEPTH_5PCT"
  | "NO_GENUINE_TRADE_7D"
  | "HOLDER_CONCENTRATION_HIGH"
  | "WASH_TRADE_SUSPECTED";

type AssetRisk = {
  asset: Asset;
  quote: Asset; // the asset used as the unit
  ledgerSeq: number;
  computedAt: string;
  methodologyVersion: string; // e.g. "1.2.0"
  dataSource: "horizon" | "hubble";

  midPrice: number | null;
  priceSource: "book" | "pool" | "none";

  depth: Array<{
    delta: number; // 0.02, 0.05, 0.10
    buySide: number; // notional in the quote asset
    sellSide: number;
    fromSdex: number; // breakdown, for transparency
    fromAmm: number;
  }>;

  manipulationCost: Array<{ delta: number; cost: number }>;
  maxSafeCollateral: number | null;

  holderTop1Pct: number | null;
  holderTop10Pct: number | null;
  holderHhi: number | null;
  volumeToSupply: { d1: number; d7: number; d30: number } | null;
  lastGenuineTrade: { ledgerSeq: number; at: string } | null;
  tradesExcludedPct: number | null;

  flags: Flag[];
  band: "LOW" | "MEDIUM" | "HIGH" | "CRITICAL";
  warnings: string[];
};
```

Note that `fromSdex` and `fromAmm` are reported separately even though the headline
number is the combination. That is what lets someone else check whether your
combination is correct, and it is part of the reproducibility promise.

`methodologyVersion` is not decoration. A result computed under methodology v1.0
cannot be compared directly with v1.2. Without this field your time series
silently becomes inconsistent the moment a definition changes mid-sprint.

---

## 7. Non-functional requirements

| ID | Requirement | Target |
|---|---|---|
| NFR-1 | Live metric freshness | At most 15 minutes behind the latest ledger |
| NFR-2 | API latency for already-computed metrics | Under 500 ms p95 |
| NFR-3 | Historical metrics | Batch, with no latency guarantee. Stated in the documentation |
| NFR-4 | Availability | **No SLA.** The SOW explicitly excludes a production mainnet SLA. State it on the API landing page |
| NFR-5 | Public rate limit | 60 requests per minute per IP |
| NFR-6 | Horizon rate limit compliance | Never exceed 3000 requests per hour, under the public limit of 3600 |
| NFR-7 | BigQuery cost budget | A hard limit, reviewed weekly. Every query prunes partitions |
| NFR-8 | Absolute read-only | No transaction signing or submission code anywhere in the repository. Enforced by a hook |
| NFR-9 | Reproducibility | Re-running at the same `ledgerSeq` with the same `methodologyVersion` produces identical numbers |
| NFR-10 | Openness | A public repository, with raw backtest data available as CSV and no BigQuery access required |

NFR-9 is the requirement most easily violated without noticing. Anything depending
on the system clock, on non-deterministic iteration order, or on a default value
that changes will break it. Make it an automated test, not a hope.

---

## 8. Out of scope

Taken from the SOW and restated so nobody is tempted to add them.

- Signing or submitting transactions. Keel is permanently read-only
- Publishing a price feed as an oracle
- Real time oracle anomaly and attack pattern detection. Already served by OctoPos, Blockaid, Hypernative, Range
- Captive-core ingestion
- Alert delivery via webhook, Slack, or PagerDuty
- Asset issuer risk profiling
- Chains other than Stellar
- A production mainnet SLA
- Path finding across intermediate assets. Stated as a known limitation
- Custodian and exchange account detection. Not reliable, stated as a limitation

---

## 9. Acceptance criteria

### Deliverable 1

- [ ] FR-1 through FR-7 pass tests against testnet fixtures whose results are known
- [ ] FR-12 through FR-17 work, historical replay validated against a control ledger
- [ ] Horizon versus Hubble cross-validation on at least 50 pairs, results tabulated
- [ ] Manual recomputation for 5 assets, the spreadsheet present in the repository
- [ ] The methodology document complete, including its limitations section
- [ ] NFR-9 tested automatically
- [ ] Both builders pass an oral explanation test on the SDEX-AMM combination and on the buy versus sell side split

### Deliverable 2

- [ ] FR-18 through FR-22 live and documented
- [ ] The USTRY time series for May 2026 complete, raw CSV in the repository
- [ ] The report states when the unsafe threshold was crossed relative to the exploit date
- [ ] The limitations section names hindsight bias explicitly
- [ ] A third party can reproduce the headline numbers from the repository and the report alone

### Deliverable 3

- [ ] FR-24 through FR-26 live
- [ ] At least 50 assets in the demonstration set
- [ ] The Ambassador Chapter Lead can explain what the dashboard shows unaided
- [ ] A 3 to 5 minute demo video
- [ ] The backtest report published openly

---

## 10. Success metrics

### During the 30 days

1. Every acceptance criterion in section 9 is met
2. The backtest shows a clear signal pointing the right way before the exploit date
3. At least one third party outside the team successfully reproduces a number from the repository

Note metric 2. If the backtest does **not** show a clear signal, that is not a
project failure but a finding that has to be reported honestly. Reporting it as it
is does far more for long term credibility than tuning thresholds until the result
looks good. If a threshold was tuned after seeing the result, say so in the report.

### After the 30 days (validation for SCF Build)

4. At least 3 conversations with protocols or vault curators about whether they
   would use these metrics
5. At least 1 written statement from an ecosystem party that these metrics are useful
6. Clarity on willingness to pay, including if the answer is no

Metric 6 is already acknowledged in the SOW appendix: the absence of Gauntlet and
Chaos Labs from Stellar may indicate the market is not yet large enough. "Nobody
wants to pay" is a valid and valuable outcome of a $5,000 sprint, and far cheaper
than discovering it after SCF Build.

---

## 11. Open questions

To be answered before the end of the week named.

| # | Question | Deadline |
|---|---|---|
| Q1 | How dense are the `offers` snapshots in Hubble for May 2026? | Day 1 |
| Q2 | What does BigQuery cost for one full month-long USTRY replay? | Day 2 |
| Q3 | The Blend risk parameters actually in force in May 2026, for use as the C_max defaults | Week 1 |
| Q4 | Does supply locked in a liquidity pool count toward the holder concentration denominator? | Week 1 |
| Q5 | The final genuine trade rules, in particular whether related account detection lands in v1 or v2 | Week 2 |
| Q6 | The selection criteria for the 50 asset demonstration set | Week 2 |
| Q7 | Are the absolute flag thresholds expressed in XLM or in USDC? | Week 2 |

Q7 matters more than it looks. If the thresholds are in XLM, an asset's risk band
can change purely because the XLM price moved, without its liquidity changing at
all. If they are in USDC, you import the assumption that USDC is stable. Whichever
you choose, document the consequence.

---

## 12. The scope cutting order

Fixed in advance so that decisions under pressure are not made carelessly.

1. Cut priority **C** requirements first (FR-23, FR-29)
2. Cut FR-27 and FR-28, simplify the detail page
3. Reduce the asset count on the dashboard, the rest stays available through the API
4. Widen the historical replay interval
5. Reduce FR-8 through FR-11 to their simplest versions

**Never cut:** the Blend backtest, the methodology document, the cross-validation,
and the limitations section. Those four are the entire value of this project. A
beautiful dashboard without a verifiable backtest will not make it through to SCF
Build.
