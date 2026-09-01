# Keel: Supporting Metrics

**Methodology version:** 1.0.5-draft
**Status:** Sections 1–5 defined and run over the USTRY history where data allows. Holder-dependent results (holder concentration, volume-to-supply ratio) are pending the first holder pull. Ready for review.

The SOW promises holder concentration, volume-to-supply ratio, and time since the last
genuine trade. It defines none of them. The definitions are the intellectual content of
these metrics; the code is not.

This file has an advantage the rest of the methodology lacks: **real specimens to test
against.** Write each rule, then run it over the USTRY/USDC trade history already pulled
in `evidences/`. A rule that survives that test is not invented.

---

## 1. What makes a trade genuine

The word *genuine* is what gives this metric value. Without it, the metric is
`/trades` sorted descending, which anyone can do.

### Specimens the rule must handle correctly

All three are on-chain and already in hand.

| # | Specimen | Expected treatment |
|---|---|---|
| A | 22 Feb 00:10:57, 0.0000080 USTRY at 1.057, both sides attacker controlled | must be excluded |
| B | 22 Feb 00:10:21, 0.0501003 USTRY at 106.7372828, matched against the seller's own offer | must be excluded |
| C | 19 Aug, 604 trades of 0.004 to 0.3 USTRY (67 against the pool), repeated accounts, same ledger close times | **the rule must take a defensible position** |

Specimen B looked like the hard one — the two sides are different accounts, so a same-account rule cannot reach it. It is caught cheaply anyway: at 106.74 against a daily median near 1.06 it is the single February trade the price-deviation criterion removes (1 trade, 0.0014% of volume). The same-account rule missing it costs nothing here, because a later, equally cheap condition catches it.

Specimen C is the real judgment call. Its shape rules out wash trading — no account sits on both sides of any ledger — but the same evidence rules out two-sided market making, since the flow is one-directional (4 sellers against 336 buyers). Deciding wrongly makes a dead market look alive or a functioning one look rigged; the defensible move is to count C's genuine matches and state that what the pattern *is* remains unestablished.

### Candidate exclusion criteria

Each must be accepted, rejected, or deferred, with a reason.

| Criterion | Cost of including it | Decision |
|---|---|---|
| Buyer account equals seller account | Feb 0 trades, 0% vol. Aug 0 trades, 0% vol | Accepted |
| Notional below a dust threshold | Feb 9.85% trades / 0.0012% vol. Aug 74.51% / 1.6971% | Accepted |
| Either side is the asset issuer | Feb 0, Aug 0. Issuer never appears as a counterparty | Accepted |
| Both sides funded from a common source | Cheapest proxy is counterparty exclusivity: Feb 199 trades / 86.3960% vol | Rejected — no work left: A and B are already caught by cheaper criteria (dust, price-outlier) |
| Trade matched against an offer owned by the taker's affiliate | Requires defining affiliation; no measurement was attempted | Rejected — not measured because not needed: its target, B, is already caught by condition 5 |
| Pool trade priced against the order book | Aug at ±15 min: 389 dearer, 57 cheaper, 150 unevaluated. Aug 29 is 315 of 315 dearer. Volume under 0.02% | Accepted |
| Price deviates from the daily median | At 1.5x: Feb 1 trade / 0.0014% vol. Aug 0 trades / 0% vol | Accepted |

### Required output

Whatever rule is chosen, the metric reports **how much volume was excluded and why**. "1.70 percent of August volume excluded as non-genuine, almost all of it dust" is a far stronger statement than a date, and it lets an outsider audit the rule.

**Definition**

A trade is counted as **genuine** if it is a distinct on-ledger offer match between two different accounts, neither of which is the asset issuer, at a price consistent with the contemporaneous order book. The rule tests **account identity and price**, not economic independence — common funding of the two sides is not measured (see Rationale).

**Unit of counting.** The unit is the trade — a single offer match — not the operation. One order that sweeps four offers is counted as four trades. All totals below use the trade count (ratio 1.429), not the operation count (1.014); the two are not interchangeable.

**Exclusion conditions.** The five conditions below are evaluated in the order listed. A trade stops at the first condition it meets and is excluded under that condition; a pool trade that passes all five but cannot be compared to an order book within ±15 min is **Unevaluated**. Order is load-bearing: 133 of the 150 August pool fills that lack a contemporaneous book are also dust, and because dust (condition 2) is tested first they resolve as Excluded, not Unevaluated — which is why August has 17 Unevaluated, not 150. A trade is excluded if it meets any of:

1. **Self-trade** — the buyer and seller are the same account.
2. **Dust** — the counter notional is below 0.01 USDC.
3. **Issuer leg** — either side of the trade is the asset issuer.
4. **Off-book pool fill** — a pool trade whose price is worse than the contemporaneous order book, evaluated at ±15 min. Only fills *dearer* than the book are excluded; a fill *cheaper* than the book is kept, because it is real price improvement. This asymmetry is deliberate: 55 of the 58 February pool trades were arbitrage that closed a 12-day gap, and a rule that discarded cheaper-than-book fills would remove the most economically useful activity in both periods.
5. **Price outlier** — the execution price deviates by more than 1.5x from the daily median, computed from **order-book trades only** (pool fills do not enter the median, so this does not circle back into condition 4). If a day has no order-book trades there is no median and the trade passes this condition rather than falling to Unevaluated — Unevaluated is reserved for pool fills that cannot be compared to a book, not a catch-all for anything unmeasured. In August this case occurs zero times.

**Outputs.** Every trade resolves to exactly one of three states, and the metric reports the count and counter-notional volume of each. Over the 30-day August window (56,863 trades, 6,246.5452279 USDC):

- **Genuine** — an evaluated trade meeting none of the exclusion conditions. Aug: 14,478 trades, 6,139.9850386 USDC (98.2941%).
- **Excluded** — meets one or more conditions, reported per condition. Aug: 42,368 trades, 106.0070905 USDC (1.6971%) — entirely dust. Conditions 1, 3, 4, and 5 catch nothing in August; the 389 pool fills dearer than the book, including all 315 on 29 August, stop at condition 2 because they sit below the dust threshold.
- **Unevaluated** — a pool fill that passes all five conditions but has no order book within ±15 min. Aug: 17 trades, 0.5530988 USDC (0.0089%). All 17 fall on 18–19 August at prices inside the 1.5x band.

**Time since last genuine trade** (the SOW metric) is the output ledger's close time minus the close time of the most recent Genuine trade. It is **Unevaluated** when no Genuine trade exists in the fetched history — there is no timestamp to measure from. This is distinct from a large but finite gap, which is a measured value and the very signal the metric exists to surface.

**Rationale**

*Dust at 0.01 USDC.* The threshold removes three-quarters of August trades (42,368 of 56,863) but only 1.71% of value; 98.29% survives. A market can be flooded with sub-cent fills without moving real volume, and a genuine-volume metric that counted them would report motion where there is none.

*The ±15-minute window.* A pool fill can only be judged against a book that existed near it in time. When no book trade falls within ±15 minutes, the rule marks the fill Unevaluated rather than scoring it against an hour-stale price. Failing loud is safer than being silently wrong — 17 August fills are held out this way instead of measured against data that no longer describes the market.

*Resolution order.* Conditions are tested cheapest-and-certain first, so a fill that is both dust and uncomparable resolves as dust. Reversing the order would route 133 of the 150 uncomparable August fills into Unevaluated (17 → 150), inflating the "unknown" bucket and hiding sub-cent dust behind a label that should mean "could not measure."

*Common funding, rejected.* The criterion has no demonstrated work left. Specimen A is caught by dust and Specimen B by the price-outlier condition; meanwhile the cheapest proxy for shared funding, counterparty exclusivity, would exclude 86.40% of February volume — a large cost with no matching benefit on the specimens in hand.

*Economic independence is not tested.* The opening promises account identity and price, not independence, because the rule measures neither funding source nor affiliation. This is an acknowledged limit, stated plainly so a reader knows what the metric does not cover rather than discovering it against a case the rule misses.

**Result when run over the USTRY history**

The rule was run over both periods already pulled in `evidences/`. Every figure below is measured, not asserted.

| State / condition | February (13,547 trades, 375,320.8368055 USDC) | August (56,863 trades, 6,246.5452279 USDC) |
|---|---|---|
| **Genuine** | 12,204 / 375,310.2438969 USDC (99.9972%) | 14,478 / 6,139.9850386 USDC (98.2941%) |
| Excluded — 1 self-trade | 0 / 0 | 0 / 0 |
| Excluded — 2 dust | 1,334 / 4.4477211 | 42,368 / 106.0070905 |
| Excluded — 3 issuer leg | 0 / 0 | 0 / 0 |
| Excluded — 4 off-book dearer fill | 8 / 0.7976176 | 0 / 0 |
| Excluded — 5 price outlier | 1 / 5.3475699 | 0 / 0 |
| **Unevaluated** | 0 / 0 | 17 / 0.5530988 (0.0089%) |

August exclusion is entirely dust; conditions 1, 3, 4, and 5 net nothing, because the 389 pool fills dearer than the book — including all 315 on 29 August — sit below the dust threshold and stop at condition 2. February is the only period where more than one condition fires: dust, off-book, and price-outlier are each active.

The two periods are not one market seen twice. Total trades roughly quadrupled (13,547 → 56,863, 4.2x), but **genuine trades rose only 19%** (12,204 → 14,478). Over the same span **genuine volume fell 61-fold** (375,310 → 6,140 USDC), so the average genuine trade collapsed from 30.75 USDC to 0.42 USDC. The market did not get busier; it got flooded with sub-cent fills.

**Specimen outcomes.**

- **A** (22 Feb, 0.0000080 USTRY, both sides attacker-controlled) — excluded by condition 2 at 0.0000084 USDC.
- **B** (22 Feb, matched against the seller's own offer) — excluded by condition 5, the only trade that condition catches in all of February. The same-account rule cannot reach it because the two accounts differ; its price of 106.74 against a daily median near 1.06 is what removes it.
- **C** (19 Aug) — the evidence supports one claim only: not wash trading. Round-trips are zero *within a ledger* — none of the 31,692 ledgers in the window has one account on both sides. Across the whole month only two accounts ever appear on both sides at all: `GD3EXP7GTMP7` (one buy on 6 Aug, then 172 sells from 18 Aug) and `GBLKUBEVO32T` (8 sells on 13–18 Aug, one buy on 19 Aug) — each a buy-once/sell-many pattern separated by days, not the paired reversals wash trading produces. What the flow *is* stays undetermined, and it is one-directional rather than two-sided: August shows 4 sellers against 336 buyers, and `GABFRFPYM2` appears in 45,133 trades without buying once. That rules out two-sided liquidity provision as firmly as it rules out wash trading, so C's true character is left unestablished — a stronger statement than a guess. The genuine-trade rule tests genuineness, not intent: of C's 604 trades (67 against the pool), 139 are removed as dust and 4 fall into the ±15-min Unevaluated set, leaving **461 counted as Genuine**.

---

## 2. Holder concentration

### Constraints

- Source is trustlines. The issuer account holds unissued supply and is not a holder.
- Assets locked in a pool are held by the pool, not by a holder. Whatever is decided here must match the supply denominator in section 3.
- Custodial and exchange accounts cannot be detected reliably. Attempting it produces false confidence.
- `/accounts?asset=` paginates by account ID, not by balance. The constraint is order, not cost: a partial fetch is an alphabetical slice of accounts, not the largest holders, so no meaningful partial result exists — the full trustline set must be paged or the metric is unevaluated. Technical design section 6.4 fetches it at most once per day.
- `/accounts?asset=` returns current state only; it cannot be read as of a past ledger. Holder concentration has no historical version and never will for a day already gone — every day the set is not recorded is lost permanently.

**Definition**

Holder concentration is measured over the accounts holding a **non-zero** balance of `USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`, matched on asset code **and** issuer, with three groups removed from the population:

- **The issuer account**, which holds unissued supply and is not a holder.
- **Zero-balance trustlines.** Horizon `/accounts?asset=` returns every trustline, not every holder — the first record it returns holds 0.0000000 USTRY. Accounts with a zero balance are dropped before any measure is computed.
- **Pool reserves.** Assets locked in a pool are held by the pool, not a holder. Two positions are excluded, and they are **not** `G...` account IDs:
  - the AMM pool, identified by pool ID `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb`;
  - the Blend V2 (YieldBlox) position, at Soroban contract `CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS`.

  Because neither is an account ID, how each surfaces in `/accounts?asset=` — as a contract-address record, some other form, or not at all — must be confirmed against the first real holder pull before the exclusion is wired. Whatever form they take, both must be removed from the holder population **and** subtracted from the circulating denominator in section 3; the two sections must exclude the same set.

The match is on asset code, not issuer alone: `GCRYUGD5…` also issues CETES, so a query keyed only on the issuer would fold a second asset's holders into the population — a silent error, since the numbers still look valid.

Three measures are reported together, all from the same filtered set:

- **Top 1** — the largest holder's share of circulating supply.
- **Top 10** — the combined share of the ten largest holders.
- **HHI** — the sum of squared percentage shares across the population.

**Asset scope.** The exclusion list is specific to USTRY. Run unchanged against another asset, section 2 returns wrong numbers with no warning, because that asset's pools are not on the list. Applying it to a new asset requires editing the list first — the accepted cost of choosing an auditable explicit list over pool-detection heuristics.

**Custodial holdings.** A trustline is an account, not a beneficial owner. The largest holder may be an exchange or custodian holding on behalf of thousands, which the metric reports as concentration in a single hand. Custodial and exchange accounts cannot be detected reliably, so no attempt is made to unbundle them; a high top-1 share is a statement about trustlines, not about people.

**Historical availability.** `/accounts?asset=` exposes current state only; it cannot be read as of a past ledger. Holder concentration therefore has no historical version and never will for a day already gone. This breaks symmetry with section 1, which can be recomputed for any past window: in the same API, the genuine-volume metrics accept a date parameter and holder concentration does not, and the two must not be mistaken for equivalent. Operationally, every day the trustline set is not recorded is lost permanently.

**Unevaluated condition.** The three measures are reported only when the full trustline set is paged within the daily budget (technical design section 6.4). If it is not, all three are **Unevaluated**, never partial. Because `/accounts?asset=` paginates by account ID rather than balance, a partial page yields the largest holder among whatever accounts happened to be fetched — a guess in the shape of a measurement. Failing loud here matches how section 1 treats a pool fill it cannot compare.

**Rationale**

*Zero-balance trustlines are excluded.* One API call proves the need: the first record Horizon returns for `order=desc` holds 0.0000000 USTRY. `/accounts?asset=` enumerates trustlines, not holders, and an unfiltered population would be padded with empty accounts that pull every share and the HHI toward zero — making a concentrated asset look distributed.

*An explicit exclusion list, not automated pool detection.* The list can be audited line by line; a heuristic guessing which accounts are pools cannot. Its cost is asset-specificity, accepted for the same reason section 1 accepts not testing economic independence — a stated limit beats a silent one.

*Top 1, top 10, and HHI together.* Each answers a different question: single-point control, oligopoly, and whole-distribution evenness. Any one alone can hide what the others show — a low top 1 with a high HHI is a different asset from a high top 1 with a low HHI.

*Unevaluated rather than partial.* Because the endpoint pages by account ID, a page fetched under budget is an alphabetical slice, not the largest holders. A concentration figure drawn from it would be a guess in the shape of a measurement, so when the full set cannot be paged the metric fails loud — the same choice section 1 makes for a pool fill it cannot compare.

**Result when run over the USTRY history**

Not yet run. Holder concentration requires the first daily holder pull; until `/accounts?asset=` has been fetched and recorded once, there is no population to measure. The absence is expected, not an omission. Because the endpoint is current-state only, this pull cannot be backfilled — it should begin immediately (see Historical availability).

---

## 3. Volume to supply ratio

### Constraints

- Supply is defined as **circulating**: trustline balances minus the issuer and the two pool positions, identical to section 2's population. Issued total and total-held are the alternatives; the Definition takes circulating so numerator and denominator share one set.
- The right window length is not obvious in advance — a thin asset can be active across 30 days while showing nothing in a given 24 hours, and a single window would hide that. This is why three windows are reported. It is a property the metric must survive, not a pattern seen in August, where genuine volume was non-zero every day.
- Volume must be computed from **genuine** trades per section 1, or the metric inherits whatever wash trading exists.

**Definition**

The volume-to-supply ratio is **genuine volume ÷ circulating supply, both in USTRY** — how many times circulating supply turned over in the window. Numerator and denominator are held in the same unit on purpose; a value-over-quantity ratio would move with the USTRY price even when activity and supply do not.

The numerator sums each genuine trade's size once; the same token can change hands several times inside a window, so this figure is total traded volume, not the count of distinct tokens that moved — which is exactly what "times turned over" means.

**Numerator — genuine volume in USTRY.** The trades that count are those marked Genuine by section 1, but their volume is summed as `base_amount` (USTRY), not `counter_amount` (USDC). Section 1 reports its own genuine-volume figure in USDC; section 3 deliberately takes the USTRY leg of the same trades so the ratio is dimensionless. A window whose recorded trades are all excluded or unevaluated has a numerator of zero, and the ratio is then a **measured zero**, not Unevaluated.

**Denominator — circulating supply in USTRY.** The same population as section 2, exactly: accounts holding a non-zero USTRY balance, minus the issuer, minus the two pool positions (AMM pool `27480d…`, Blend V2 contract `CCCCIQSD…`). A numerator and denominator drawn from different sets would produce a ratio that measures no real quantity; this is the agreement checklist item 5 verifies.

**Three windows, from the output ledger, half-open.** The ratio is reported over **24 hours, 7 days, and 30 days**, each counted backward from the close time of the output `LedgerSeq`, never from the wall-clock time of the request — two calls against the same ledger must return the same ratio, or they violate NFR-9. Each window is half-open with exactly one closed boundary, the same discipline a backtest `[from, to)` uses: a trade at close time `t` is in the window iff `t(L) − W < t ≤ t(L)`, where `L` is the output ledger. The recent boundary is closed so the output ledger's own trades count; the far boundary is open. A trade sitting on either edge then lands in exactly one window, and two correct implementations cannot disagree. The three windows are not redundant — the short one is responsive but volatile, the long one stable but slow, the middle one is where a real shift first becomes legible (see Rationale).

**Unevaluated conditions.** The ratio is Unevaluated — never a number — in three cases, and section 5 must show which one applies:

- **Denominator unavailable.** Section 2 is Unevaluated that day (the full trustline set could not be paged), so circulating supply is unknown.
- **Denominator zero.** Every non-zero trustline turns out to be the issuer or a pool address, leaving an empty population and zero circulating supply. The ratio is undefined, not zero — division by zero is Unevaluated, distinct from a measured-zero numerator.
- **Numerator unavailable.** The trade recording does not cover the requested window, so genuine volume cannot be computed for it.

All three yield an Unevaluated ratio, but they are different failures and are not reported as one. A window with recorded trades but zero genuine volume is none of them — it is a measured zero.

**Result when run over the USTRY history**

The numerator has been run over August; the ratio has not, because the denominator waits on the first holder pull (section 2).

Genuine volume by window, August, in USTRY:

| Window | Min | Max | Ratio max/min |
|---|---|---|---|
| 24 hours | 4.9114975 USTRY | 1,561.8888048 USTRY | 318.0x |
| 7 days (rolling) | 156.9581635 | 3,431.1261575 | 21.86x |
| 30 days (rolling) | 5,380.1440098 | 5,718.2355575 | 1.06x |

Total genuine volume for the month is 5,723.2370064 USTRY. The same 14,478 trades are 6,139.9850386 USDC — identical to section 1's Outputs, because it is the same set. The ratio between those two figures, 1.072817, is the volume-weighted average price for the month, and it is the reason the ratio is denominated in USTRY: that number moves. A USDC-denominated volume-to-supply ratio would move with it even when activity and supply are unchanged. In August the price barely drifted, so the unit choice hardly matters here; on a period like 22 February it would shift the ratio substantially. The choice earns its keep precisely when it is most needed.

Zero days in August had no genuine volume, so the measured-zero condition never fired in this data, though it is the condition that will matter most on a dead asset. The shape is the argument for the middle window: the first half of August sits flat, then rises sharply from 20 August. The 24-hour window swings 318x and reads as broken; the 30-day window is nearly flat at 1.06x and hides the shift; only the 7-day window shows it as it happens.

**Volume-to-supply ratio: not yet computable.** Once the holder pull provides circulating supply in USTRY, the ratio follows immediately from the numerator above.

**Rationale**

*Denominated in USTRY.* The month's volume-weighted price, 1.072817 USDC per USTRY, is not a fixed conversion — it moves. A ratio built from a USDC numerator over a USTRY denominator would move with it, reporting change when neither activity nor supply changed. Holding both sides in USTRY makes turnover mean what it says. August's price barely drifted, so the choice hardly shows here; on a period like 22 February it would swing the ratio on price alone — which is exactly when the discipline matters.

*Three windows.* One window cannot serve. Across August the 24-hour figure swings 318x and reads as a broken feed; the 30-day figure is nearly flat at 1.06x and would not register the 20 August rise until a month after it happened. The 7-day figure, 21.86x, is smooth enough to read and fast enough to move, and it shows the shift as it occurs. Reporting all three lets a reader separate a quiet day from a dead asset, and a real trend from noise.

*Anchored to the output ledger.* Every output carries a `LedgerSeq`, and each window is measured backward from its close time, not from the clock. Two calls against the same ledger must return the same ratio; anchoring to request time would let them drift apart, violating NFR-9 and repeating the defect class that consumed half of this work.

*Half-open windows.* A trade sitting exactly on a window edge must belong to one window, not both and not neither. Fixing the interval as `t(L) − W < t ≤ t(L)` — far edge open, recent edge closed — means two correct implementations return the same number for the same ledger, instead of differing by whatever a boundary trade happens to do.

*Three Unevaluated causes, kept apart.* A missing denominator (section 2 could not page the trustline set), a zero denominator (the holder population is empty), and a missing numerator (the trade recording does not reach the window) are three different failures with three different fixes. Collapsing them into one "unavailable" would tell an operator something is wrong without saying what; section 5 requires the cause to survive to the reader.

---

## 4. Genuine volume in the oracle window

Feeds the `MR` term in `00-core.md` section 8.

### Constraints

- The 15-minute window is an assumption, not a confirmed Reflector parameter.
- Genuine volume in the oracle window can be near zero, so a single manipulative trade can dominate the average the oracle consumes. The trade that did so on the incident was not a self-trade: its two accounts differ, and section 1 removes it through the price-outlier condition, not the same-account rule. Figures are in Result.

**Definition**

Genuine volume in the oracle window is the total genuine volume in a fixed short window ending at the output ledger. It feeds the `MR` term in `00-core.md` section 8.

*Genuine rule — identical to section 1.* No window-specific condition is added. On the only specimen this section has, the 22 February incident, section 1's condition 5 already removes the manipulative trade (106.737283 against a genuine price of 1.057427) and keeps the four genuine trades. A stricter rule has no demonstrated work to do, and a condition that never fires is the mistake section 1 rejects with common funding. Three sections now share one genuine definition — one implementation, one place to change. Two different genuine definitions in one document is how numbers begin to contradict each other.

*Window — 15 minutes, an explicit assumption.* Fifteen minutes is a default, **not a confirmed Reflector parameter**, and that carries here rather than staying in Constraints. Its impact is bounded by the incident: 15 and 30 minutes give identical genuine volume (0.3268461 USDC), and 60 minutes adds only 0.098 USDC. On a market this thin the result is nearly insensitive to window width, so the unconfirmed parameter is a measured risk, not a large one. If Reflector's true window differs, this section is revised with the confirmed number.

*Not the same as section 1's ±15 minutes.* This 15-minute aggregation window and the ±15-minute price-comparison window in section 1 are different quantities that happen to share a number. They are not linked — one is how far back oracle volume is summed, the other is how far a pool fill may look for a contemporaneous book. An implementation must not fold them into one constant, or a change to one will silently move the other.

*Anchor and bounds — identical to section 3.* The window is counted backward from the close time of the output `LedgerSeq`, half-open, `t(L) − W < t ≤ t(L)`. A different convention here would let two metrics in the same document compute their edges differently.

*Outputs — three states, each carrying the recorded trade count so the cause stays visible.*

- **A number** — genuine volume in the window.
- **Measured zero** — no genuine volume, covering two situations the trade count separates: a window with recorded trades but none genuine (active but fake), and a window with no trades at all (silent). Both are zero; with the trade count they tell different stories. A silent asset and one busy with fake trades carry different risk into `MR` — the first is cheap to move because nothing opposes it, the second is already being moved.
- **Unevaluated** — the trade recording does not cover the window.

**Rationale.** Stated inline in the Definition above (the italic clauses); section 4 carries no separate Rationale block, matching the worksheet's original structure. The reasoning is not repeated here to avoid a second home for it.

**Result when run over the USTRY history**

Run over the 22 February incident — the one specimen this section has.

The 15-minute window ending 2026-02-22T00:10:21Z held 5 trades, 4 of them genuine, for a genuine volume of **0.3268461 USDC**. The single manipulative trade — the same one section 1 removes via condition 5 — was **5.3475699 USDC, 16.4 times the entire genuine volume of the window.** The four genuine trades were all priced at 1.057427; the manipulative one at 106.737283.

Window width barely moves this:

| Window | Trades | Genuine | Genuine volume (USDC) | Manipulative trade | Dominance |
|---|---|---|---|---|---|
| 15 min | 5 | 4 | 0.3268461 | 5.3475699 | 16.4x |
| 30 min | 5 | 4 | 0.3268461 | 5.3475699 | 16.4x |
| 60 min | 8 | 7 | 0.4250511 | 5.3475699 | 12.6x |

This is why the incident ran as it did: with genuine volume this small, one fabricated trade at ~100x the real price dominated the average the oracle consumed. Section 1's rule removes that trade with no oracle-specific criterion — the rule written for one metric catches the incident that motivated another.

---

## 5. Unevaluated versus zero

Every metric here can be unavailable. `09-flags-and-bands.md` section 2 requires that
unavailable be distinguishable from measured-zero, because an asset with no trustline
data must not look identical to one that was checked and found safe.

Each metric must state which condition makes it unevaluated.

| Metric | Unevaluated when |
|---|---|
| Last genuine trade | no genuine trade exists in the fetched history — no timestamp to measure from (section 1) |
| Holder concentration | the full trustline set cannot be paged within the daily budget (section 2) |
| Volume to supply | the denominator is unavailable because section 2 is unevaluated, or zero from an empty holder population (section 2), or the trade recording does not cover the window (section 3 numerator) |
| Genuine volume in window | the trade recording does not cover the window; a window containing no trades is a measured zero, not unevaluated (section 4) |

---

## 6. Checklist before this file ships

- [x] Every "to be written" replaced
- [x] The genuine-trade rule was run over the real USTRY history and the result recorded
- [x] Specimens A and B are excluded by the rule, or the reason they are not is stated
- [x] Specimen C has a defensible position, either direction
- [x] Section 2 and section 3 agree on pool-held supply
- [x] Every metric states its unevaluated condition
- [x] Excluded volume is reported alongside each affected metric

## 7. Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Worksheet created. No definitions recorded yet |
| 1.0.4-draft | Section 1 genuine-trade rule defined; run over February and August USTRY history; per-condition results and specimen outcomes recorded |
| 1.0.5-draft | Sections 2 (holder concentration), 3 (volume-to-supply), 4 (oracle-window volume), and 5 (unevaluated conditions) defined and run over the USTRY history where data allowed; holder-dependent results pending the first pull |
