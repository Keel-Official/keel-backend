# Keel: Quote Asset and Pair Selection

**Methodology version:** 1.1.0-draft
**Status:** DECIDED. Section 5 defines criteria; the list itself is produced by a
selection run and recorded in `docs/evidences/`.
**Settles:** open question Q7, and PRD open question Q6.

Every figure Keel publishes is denominated in a quote asset. "The depth of USTRY" is
meaningless until the counter asset is named. This document must state which pair is
measured and why, because a reviewer will ask, and because measuring the wrong pair
answers the wrong question.

Each section states the constraints that bound the answer, then records the decision and
its reason.

---

## 1. Which quote asset

**Constraints that bound this decision**

- A Stellar asset may trade against many counter assets at once. XLM and USDC are the
  common ones, but nothing forbids others.
- On the incident, the oracle read the **USTRY/USDC** market. Measuring USTRY/XLM would
  have answered a question nobody was asking.
- Thresholds in `09-flags-and-bands.md` are absolute values in the quote asset. If assets
  are measured against different quotes, their bands are not comparable, and an asset's
  band can move purely because the XLM price moved. This is open question Q7.
- Denominating everything in USDC embeds an assumption that USDC is stable, which is
  awkward for a product whose premise is questioning price assumptions.

**Questions to answer**

1. Is there a single global quote asset, or is it chosen per asset?
2. If per asset, what rule chooses it?
3. How is the Q7 comparability problem handled in the meantime?

**Decision**

> **The quote asset is global and it is USDC**, issuer
> `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`. Every published depth,
> manipulation cost, `maxSafeCollateral` and threshold comparison is denominated in that
> asset. It is not chosen per asset, so question 2 does not arise. `quote` remains
> present in every response, because a unit that is currently global is not a unit that
> may be assumed forever.
>
> **This resolves Q7 rather than deferring it.** The thresholds in
> `09-flags-and-bands.md` section 6, `ManipulationCheapAbsolute = 10,000` and
> `ThinDepth5PctAbsolute = 50,000`, are USDC figures, and section 6 of that document is
> amended from "unresolved limitation of units" to a statement of this decision.
>
> **The consequence, stated as PRD section 11 requires.** Keel now carries the
> assumption that USDC holds its peg. If USDC depegs, every band in the system moves
> without any asset's liquidity changing. Two things make that assumption auditable
> rather than hidden. First, XLM/USDC is itself a monitored pair and the deepest market
> on the network, so a USDC dislocation appears inside Keel's own output instead of
> silently corrupting it. Second, the flags are published individually per
> `09-flags-and-bands.md` section 1, so a consumer who rejects the assumption can apply
> their own thresholds to the raw figures.

**Rationale**

Three reasons, in order of weight.

**A single global quote is the only form under which the absolute thresholds mean
anything.** `09-flags-and-bands.md` expresses `ManipulationCheapAbsolute` and
`ThinDepth5PctAbsolute` in the quote asset. Under a per-asset quote, two assets compared
against the same number are being compared against two different amounts of money, and
the band table in section 5 of that document stops being a ranking. Section 6 of the same
document requires Q7 settled before version 1.1, and settling it is what promotes the
methodology set to 1.1.0-draft.

**USDC rather than XLM, because a band must not move when no liquidity moved.** That is
the failure PRD section 11 names first, and it is the more damaging of the two, because
it is invisible to the reader. A USDC depeg is a public event that a reviewer can date. A
gradual XLM drift silently reclassifies assets and leaves no trace in the output. Between
an assumption that fails loudly and one that fails quietly, a warning product takes the
loud one. It is also the unit the incident was denominated in, the unit of every stored
artifact in this repository, and the unit a lending protocol setting collateral parameters
already thinks in.

**Per asset was rejected, not overlooked.** It is the more sophisticated answer and it is
the wrong one for a 30 day sprint whose acceptance criteria include a reviewer predicting
Keel's behaviour from the documents alone. A per-asset rule requires a tiebreak, a
comparability caveat on every cross-asset statement, and a conversion story for the
thresholds. Section 2 recovers the part of it that actually protects the user, which is
the cheapest-path check, without paying for the part that only adds ambiguity.

**Coverage risk, and why it is acceptable.** A global USDC quote makes an asset that
trades only against XLM look priceless, and under `09-flags-and-bands.md` that is
`NO_EXECUTABLE_PRICE`, a CRITICAL flag. That would be a measurement artifact reported as
a finding, which PRD principle P-4 does not license. Section 2 is what prevents it: the
XLM pair is evaluated, and an asset with real XLM liquidity and no USDC market is
reported as such rather than as a dead asset. All 64 assets in the 26 August scan
returned a non-null `mid_price` against USDC, so the artifact is expected to be rare, but
"expected to be rare" is not a reason to leave it uncovered.

---

## 2. Multiple pairs for one asset

**Constraints**

- An attacker uses the cheapest available path. Ignoring a secondary pair makes Keel
  optimistic, and optimism is the failure mode this product exists to prevent.
- Computing every pair for 50 assets multiplies the Horizon request budget. Section 6.4
  of the technical design allocates 3 requests per asset per scan against a ceiling of
  3000 per hour.
- The API contract currently returns one `quote` per response, with an optional `?quote=`
  parameter.

**Questions to answer**

1. Are all pairs with any liquidity computed, or only the primary?
2. What rule designates the primary pair?
3. Are secondary pairs reported, and if so where?
4. If an asset is safe on its primary pair and dangerous on a secondary one, what band
   does the asset carry?

**Decision**

> **1. Candidate set, not every pair.** Depth is computed against a declared candidate
> quote set, which in this version is exactly two members: USDC and native XLM. Every
> member with any liquidity is computed. FR-11 says "every quote pair that has any
> liquidity"; this is that requirement bounded to a declared set, and the bound is a
> stated limitation of the version, not an unstated shortfall.
>
> **2. The primary pair is USDC, always.** It follows from section 1 and requires no
> rule. `primaryQuote` is nonetheless emitted as an explicit field rather than left
> implicit, so that widening the candidate set later does not silently change what a
> stored row meant.
>
> **3. Secondary pairs are reported in three places.** In the store, one `metrics` row per
> evaluated pair, which is where the published CSV draws from. In the API, through
> `?quote=` (FR-23). And in the primary response, as a `pairsEvaluated` array carrying
> the quote, band and `bandConfidence` of each pair, without its depth figures. The third
> of these exists because FR-23 is priority C and is first out under PRD section 12; the
> cheapest-path finding must survive that cut, and an array of three fields does.
>
> **4. The asset carries the worse band.** The band is the highest tier triggered on any
> evaluated pair, which is principle P-2 applied to pair choice. When a pair other than
> the primary sets the band, `bandDrivenBy` names that quote and the warning
> `SECONDARY_PAIR_WORSE` is emitted. Depth, cost and `maxSafeCollateral` in the headline
> response remain the primary pair's figures and are never mixed across pairs.
>
> **The XLM pair is converted before it is judged, and never after.** Thresholds are USDC
> figures per section 1, so an XLM-denominated depth cannot be compared against them
> directly. The conversion uses Keel's own XLM/USDC mid price at the same `ledgerSeq`,
> emitted as `xlmUsdcRate` with the `ledgerSeq` it came from. This is not a price oracle
> and does not breach principle P-1: the rate is read from the same order book and pool
> data Keel already reads, from the deepest market on the network, at the same ledger,
> and it is published alongside the number it produced so a reader can recompute with a
> different rate.
>
> **When the rate is not trustworthy, the XLM pair goes `unevaluated`, not `clear`.** If
> XLM/USDC has no executable price at that ledger, or its own `SPREAD_EXTREME` or
> `PRICE_SOURCE_CONFLICT` fires, the XLM pair's flags are recorded as `unevaluated` under
> `09-flags-and-bands.md` section 2 and `bandConfidence` falls to `partial`. An
> unconverted secondary pair must not be able to make an asset look safer than a
> converted one would have.

**Rationale**

The cheapest-path argument runs one way only, and that is what decides question 4. If an
asset is thin on USDC and deep on XLM, reporting the USDC figure understates its true
liquidity, which overstates its risk, which is the safe direction under P-2. If it is
deep on USDC and thin on XLM, reporting only USDC says an asset is safe while a cheaper
attack sits next to it, and that is the exact shape of the Blend failure this product
exists to name. Only the second case needs machinery, and taking the worst band across
pairs is the smallest machinery that covers it.

Two members, not more, because the budget is real and the marginal value falls off fast.
Fifty assets at three Horizon requests per pair per scan is 150 requests for one quote and
300 for two. At the four scans per hour that NFR-1's fifteen minute freshness implies,
that is 1200 requests per hour against the 3000 ceiling in NFR-6, leaving room for the
recorder in FR-15. A third candidate quote would take it to 1800 and buy coverage of pairs
that the 26 August scan gives no evidence anyone trades.

Reporting the band but not the depth of secondary pairs in the primary response is
deliberate. A depth figure in a different unit invites exactly the incomparable
arithmetic section 1 exists to prevent, while a band and a confidence are already unitless
by construction.

**Changes this decision requires**

| File | Change |
|---|---|
| `internal/domain/types.go` | add `PrimaryQuote`, `PairsEvaluated []PairSummary`, `BandDrivenBy`, `XlmUsdcRate` to `AssetRisk`; add warning `SECONDARY_PAIR_WORSE` |
| `docs/api/keel-openapi.yaml` | add `primaryQuote`, `pairsEvaluated`, `bandDrivenBy`, `xlmUsdcRate`; regenerate mocks |
| `internal/store` | the `assets` unique constraint already covers `(code, issuer, quote_code, quote_issuer)`, so a second quote is a second row and needs no migration |
| `09-flags-and-bands.md` section 6 | replace "An unresolved limitation of units" with the section 1 decision and its consequence |
| PRD section 11 | mark Q6 and Q7 answered, pointing here |
| PRD FR-11 | note the candidate-set bound |

---

## 3. The backtest pair

**Constraint**

The oracle read USTRY/USDC. This is not a free choice.

**Decision**

> **The backtest measures USTRY/USDC**, issuer
> `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` against the Circle USDC
> issuer, which is the pair in `testdata/fixtures/ustry_pre_exploit.md` and in
> `09-flags-and-bands.md` section 7.
>
> The reason, stated rather than assumed: the claim the backtest makes is that Keel would
> have flagged the market Blend actually relied on before Blend relied on it. That claim
> is only true of the pair the oracle read. Run against USTRY/XLM the same numbers would
> describe a market no lending decision depended on, and the finding would be about
> nothing, however good the chart looked.
>
> **USTRY/XLM is computed alongside it as a published control.** It is reported
> separately, never merged into the headline series, and its purpose is to let a reader
> see whether the conclusion depends on the pair choice. This costs one extra series and
> answers in advance the first question a sceptical reviewer asks, which is whether the
> pair was chosen because it produced the result.

---

## 4. Path payments through intermediate assets

**Constraints**

- Stellar routes path payments across multiple books and pools in a single operation.
  This was observed directly in the pool effects: `trade` and `liquidity_pool_trade`
  interleaved within one operation.
- True effective liquidity is therefore larger than any single pair suggests.
- Implementing path finding is out of scope for the 30 day sprint.

**Questions to answer**

1. Is this stated as a known limitation, or partially approximated?
2. In which direction does ignoring it bias the result, and is that direction safe?

**Decision**

> **1. Stated as a known limitation. No approximation is attempted.** PRD section 8 places
> path finding out of scope, and a partial approximation would be worse than the
> limitation: it would produce a number carrying an accuracy it does not have, and
> NFR-9's reproducibility promise would then cover a figure nobody can check. The
> limitation is named in the methodology limitations section, on the `/methodology`
> endpoint, and in the backtest report.
>
> **2. The bias is toward over-warning in the general case, which is the safe direction,
> with one exception that must be stated with it.**
>
> General case. Ignoring routed liquidity understates the liquidity actually reachable,
> which understates the cost of moving a price, which makes `MANIPULATION_CHEAP` and
> `MANIPULATION_RATIO_LOW` fire more readily than a path-aware engine would. Keel
> therefore calls some assets more dangerous than they are. Under principle P-2 that is
> the direction to be wrong in, and it is consistent with `09-flags-and-bands.md`
> evaluating manipulation cost on the `orderbookOnly` variant for the same reason.
>
> **The exception, which is not a technicality.** The bias reverses whenever the consumer
> being protected prices the asset through a route rather than through one book. In that
> case the reachable price Keel computes from a single pair is not the price the consumer
> would see, and Keel can call an asset safe while a cheaper routed path exists. Keel does
> not detect this case and cannot flag it. Anyone using Keel to set parameters against a
> path-priced feed is outside what these numbers cover, and that sentence belongs in the
> limitations section verbatim rather than in a footnote.

---

## 5. Asset selection for the demonstration set

**Constraints**

- The SOW promises at least 50 active Stellar assets.
- A reviewer will ask why these 50 and not others.
- Layer 3 of the validation protocol requires 8 recorder assets spanning the liquidity
  range, and a demonstration set consisting only of healthy assets never exercises the
  code paths that matter most.
- The 64 assets stored on 26 August are the union of `configs/demonstration-set.json` and
  `configs/recorder-pairs.json`, which is an artifact of which two files were loaded and
  not a set anybody selected. Both files declare themselves PROVISIONAL and name this
  section as what supersedes them.

**Questions to answer**

1. What are the inclusion criteria, stated before the list is built?
2. Is the set balanced across the liquidity range, or is it the top 50 by some measure?
3. Are known-dangerous assets included deliberately?

**Decision**

> **The set is rebuilt from these criteria, not justified backwards from the 64.** The
> criteria below are committed first. A selection run is then executed against them and
> its output is the demonstration set, recorded in a new evidence document that cites
> both commits. The 64 of 26 August are evidence for FR-17 and remain so; they are not
> the demonstration set and the two must not be cited as the same thing.
>
> **C1. Identity.** An asset is the pair (code, issuer) and is never matched on the
> ticker. `type` is read from Horizon `/assets` and never inferred from the code length.
> Where a ticker is contested, identity is confirmed in both directions before inclusion:
> the issuer account's `home_domain`, and that domain's SEP-1 `stellar.toml` listing the
> code against that exact account. The precedent is AQUA, where 97 distinct assets share
> the ticker.
>
> **C2. Eligibility.** At the selection ledger the asset has, against USDC, at least one
> of a non-empty order book or a constant product pool with non-zero reserves. Assets with
> neither are not excluded by this criterion; they enter through C5, deliberately.
>
> **C3. Code path coverage, which is the criterion that does the real work.** The set is
> not balanced by size alone but by which branches of the engine it exercises. Minimum
> counts, verified against the selection run before the set is accepted:
>
> | Cell | Min | Present in the 26 August 64 |
> |---|---|---|
> | Two-sided book and a pool | 12 | 16 |
> | Pool only, no two-sided book | 8 | 20 |
> | Book only, no pool | 2 | **0** |
> | `PRICE_SOURCE_CONFLICT` triggered | 3 | not measured in that run |
> | `SPREAD_EXTREME` triggered | 5 | many, e.g. ACT at 870% |
> | `maxSafeCollateral == 0` | 5 | 17 |
> | `NO_EXECUTABLE_PRICE` | 1 | 0 |
> | Native base, no issuer | 1 | 1, XLM |
> | Same ticker, two issuers, both retained | 1 pair | 2 pairs, EURC and GOLD |
>
> The third row is the one that matters. Every one of the 64 has a pool, so no stored row
> exercises the book-only path, and the divergence measurement over the overlapping
> 60-pair set reports zero pairs in that case. If the selection run also finds none on
> mainnet, that absence is recorded as a finding in the evidence document and the path is
> covered by a testnet fixture instead. It is not quietly left uncovered, and the set is
> not declared complete while the cell is empty and unexplained.
>
> The last row is kept on purpose. EURC under `GAQRF3UG` and under `GDHU6WRG` priced
> 0.60 and 1.16 in the same run, and the two GOLD issuers differ by seven orders of
> magnitude. Nothing demonstrates the (code, issuer) identity rule to a reviewer faster
> than two rows with one ticker and two prices.
>
> **C4. Liquidity stratification.** Quartiles by `min(depth(0.05).buySide,
> depth(0.05).sellSide)` in USDC at the selection ledger, at least 10 assets per quartile.
> The weaker side is used rather than the mean, for the same reason `THIN_DEPTH_5PCT`
> uses it.
>
> **C5. Known-dangerous assets are included deliberately.** USTRY is included by name, as
> the subject of the backtest and the golden fixture. At least 5 assets expected to land
> CRITICAL or HIGH are retained, including any asset found with no executable price at
> all. A demonstration set of only healthy assets proves only that the healthy path runs.
>
> **C6. The recorder subset derives from this set, not the reverse.** The 8 Layer 3
> recorder assets are drawn from the demonstration set, 2 per C4 quartile.
> `configs/recorder-pairs.json` is regenerated from the selection output and loses its
> PROVISIONAL marker at that point. Its current 8 pairs may or may not survive; they hold
> no priority by having been there first.
>
> **C7. Size.** At least 50 after every cell in C3 and C4 is satisfied. Cells overlap, so
> 50 is a floor rather than a sum.
>
> **Answering question 2 directly: it is stratified, not a top 50.** A top 50 by depth
> would fill the set with assets that trigger nothing, and the flags that carry the
> product would never fire outside the fixture.

---

## 6. Checklist before this file ships

- [x] Every "to be written" replaced
- [x] Each decision has a stated reason, not only a stated choice
- [x] Q7 either resolved or explicitly deferred with its consequence stated

  Resolved in section 1: thresholds are USDC figures. The consequence, that Keel now
  assumes the USDC peg holds, is stated there together with what makes it auditable.
  `09-flags-and-bands.md` section 6 is amended by the same decision.

- [x] The selection criteria were written before the 50 asset list was built

  Ticked on a condition that is part of the decision, not a loophole. The criteria in
  section 5 are committed before the selection run, and the resulting evidence document
  cites the criteria commit and the run commit in that order. The 64 assets of 26 August
  are not that list; they are FR-17 evidence and are named as such wherever they appear.
  If the set is ever declared from the 64 without a fresh run, this box reverts to
  unticked and the reason is recorded here.

- [x] Someone outside the team can predict, from this file alone, which pair Keel will
      report for an asset they name

  Section 1 makes the answer a constant: USDC. Section 2 makes the failure mode
  predictable too, since the reader also knows the XLM pair was checked and can tell from
  `bandDrivenBy` which one set the band.

## 7. Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Worksheet created. No decisions recorded yet |
| 1.0.8-draft | Header synced to the version in force, 5 September 2026. **No content change in this file.** `07` had run to 1.0.8-draft alone; Al ratified one version for the whole set so that a reader cannot cite two. README section 4 and DEC-014 carry the reasoning |
| 1.1.0-draft | All seven decisions recorded. Global USDC quote, candidate set of two with the worst band across pairs, USTRY/USDC backtest pair with an XLM control, path payments as a stated limitation with its bias direction and its one reversal, and stratified selection criteria for the demonstration set. Q7 resolved, which is what `09-flags-and-bands.md` section 6 named as the condition for 1.1. The minor version applies to the whole methodology set under the one-version rule, so `09` section 6 and the PRD are amended in the same pass |