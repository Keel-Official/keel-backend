# Keel: Supporting Metrics

**Methodology version:** 1.0.3-draft
**Status:** WORKSHEET. The definitions below are unmade. Do not ship this file with blanks.

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
| C | 19 Aug, dozens of trades of 0.004 to 0.3 USTRY, repeated accounts, same ledger close times | **the rule must take a defensible position** |

Specimen B is the hard one. The two sides are different accounts, so a naive
same-account rule does not catch it. Whatever rule is written must either catch it or
state clearly why it does not and what that costs.

Specimen C is a judgment call with real consequences. Healthy market making and wash
trading look similar from the outside. Deciding wrongly in one direction makes a dead
market look alive; in the other it makes a functioning market look manipulated.

### Candidate exclusion criteria

Each must be accepted, rejected, or deferred, with a reason.

| Criterion | Cost of including it | Decision |
|---|---|---|
| Buyer account equals seller account | none, trivially correct | |
| Notional below a dust threshold | requires choosing the threshold | |
| Either side is the asset issuer | none | |
| Both sides funded from a common source | expensive, may need account graph traversal | |
| Trade matched against an offer owned by the taker's affiliate | catches specimen B, hard to define | |

### Required output

Whatever rule is chosen, the metric reports **how much volume was excluded and why**.
"87 percent of 30 day volume excluded as non-genuine" is a far stronger statement than a
date, and it lets an outsider audit the rule.

**Definition**

> _to be written_

**Rationale**

> _to be written_

**Result when run over the USTRY history**

> _to be written: how many trades excluded, by which criterion_

---

## 2. Holder concentration

### Constraints

- Source is trustlines. The issuer account holds unissued supply and is not a holder.
- Assets locked in a liquidity pool are held by the pool, not by a holder. Whatever is
  decided here must match the supply denominator in section 3.
- Custodial and exchange accounts cannot be detected reliably. Attempting it produces
  false confidence.
- Fetching holders from Horizon `/accounts?asset=` is paginated and expensive for assets
  with many trustlines. Section 6.4 of the technical design resolves this by fetching
  holder data at most once per day.

### Questions to answer

1. Which accounts are excluded from the population?
2. Do pool-held reserves count in the denominator?
3. Which measures are reported: top 1, top 10, HHI, all three?
4. What is reported when the trustline set is too large to page within budget?

**Definition**

> _to be written_

**Rationale**

> _to be written_

---

## 3. Volume to supply ratio

### Constraints

- Supply can mean issued total, total held in trustlines, or circulating after excluding
  issuer and pool holdings.
- Thin assets frequently show zero 24 hour volume but non-zero 30 day volume. Reporting
  only one window hides that.
- Volume must be computed from **genuine** trades per section 1, or the metric inherits
  whatever wash trading exists.

### Questions to answer

1. Which supply definition, and does it match section 2?
2. Which windows are reported?
3. Is volume filtered by the genuine-trade rule?

**Definition**

> _to be written_

**Rationale**

> _to be written_

---

## 4. Genuine volume in the oracle window

Feeds the `MR` term in `00-core.md` section 8.

### Constraints

- The 15 minute default window is an assumption, not a confirmed Reflector parameter.
- On the incident, genuine volume in the window was zero or near zero, which is precisely
  why one self-trade dominated the average.

### Questions to answer

1. Is this the same genuine-trade rule as section 1, or a stricter one?
2. What is reported when the window contains no trades at all?

**Definition**

> _to be written_

---

## 5. Unevaluated versus zero

Every metric here can be unavailable. `09-flags-and-bands.md` section 2 requires that
unavailable be distinguishable from measured-zero, because an asset with no trustline
data must not look identical to one that was checked and found safe.

Each metric must state which condition makes it unevaluated.

| Metric | Unevaluated when |
|---|---|
| Last genuine trade | |
| Holder concentration | |
| Volume to supply | |
| Genuine volume in window | |

---

## 6. Checklist before this file ships

- [ ] Every "to be written" replaced
- [ ] The genuine-trade rule was run over the real USTRY history and the result recorded
- [ ] Specimens A and B are excluded by the rule, or the reason they are not is stated
- [ ] Specimen C has a defensible position, either direction
- [ ] Section 2 and section 3 agree on pool-held supply
- [ ] Every metric states its unevaluated condition
- [ ] Excluded volume is reported alongside each affected metric

## 7. Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Worksheet created. No definitions recorded yet |
