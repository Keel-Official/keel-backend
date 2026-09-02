# DEC-012: Accounts in USTRY trade records that hold no USTRY trustline are path-payment routers

**Status:** Proposed. Drafted by the assistant; not in effect until Al accepts.
**Date drafted:** 2026-09-02
**Zone:** YELLOW (`docs/decisions`)
**Supersedes / reverses:** nothing. This record does not delete or rewrite any earlier text.

---

## Decision

Three parts, to be accepted or rejected together or individually.

1. **Record as measured evidence** that a large majority of the accounts appearing on either
   side of the recorded USTRY/USDC trade history hold no USTRY trustline and never held one,
   and that they reach those trades through path payments rather than by buying or selling
   USTRY for their own account.

2. **Rename the third classification column** emitted by `scripts/holderstats` from
   `no trustline at pull` to `not in trustline pull`, and remove the inference paragraph
   beneath it. The column reports an observation. It must not report a cause.

3. **Flag, without editing, three claims** in `docs/methodology/07-supporting-metrics.md`
   section 1 as resting on a premise this evidence undermines. The claims stay in place until
   Al revises them; this record does not revise methodology text.

---

## Reason

The classification column as originally written asserted a cause the evidence does not
support, and the first specimen tested falsified it. Renaming the column to what the script
actually measures makes the output correct regardless of which cause applies to which
account, which matters because the causes are almost certainly mixed across 372 accounts and
only 29 have been examined. Recording the finding separately, rather than folding it into a
methodology edit made under time pressure, keeps the evidence and its limits auditable while
leaving the methodology judgement where it belongs.

**Rejected alternative:** rename the column to `path payment router`. That label is supported
for the 29 accounts sampled and unmeasured for the remaining 343. It trades one unverified
cause for another and would have to be revised again the first time a counter-example turns
up, which is precisely the failure this record exists to correct.

---

## Cause / Basis

### The premise that failed

The original column name rested on the claim that holding an asset requires a trustline, so
any account appearing in a trade record must have held a trustline at the time of the trade.
That claim does not hold for path payments. In `path_payment_strict_send` and
`path_payment_strict_receive`, the intermediate asset is never held by the sending account:
only the send asset and the destination asset require trustlines. An account can therefore
appear on either side of a USTRY trade record with no USTRY trustline at any point in its
existence.

### Measurement

Source files, all under `docs/evidences/`:

- `2026-08-31-USTRY.GCRYUGD5-holders-and-supply/holders.csv` (875 trustline rows,
  `latest_ledger` 64211133..64211152, read 2026-08-31T16:00:22Z..16:01:53Z)
- `USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv` (13,547 rows)
- `USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-08-01_2026-09-01.csv` (56,863 rows)

Cross-check output (`scripts/holderstats`):

| Quantity | Value |
|---|---|
| Distinct accounts on either side of a recorded trade | 431 |
| Of those, holding a non-zero USTRY balance at the pull | 38 |
| Of those, holding an open USTRY trustline at zero | 21 |
| Of those, absent from the trustline pull | 372 (86.3%) |
| Side appearances attributable to the 372 | 60,622 of 140,166 (43.2%) |

The reader was validated independently before these figures were trusted: total side slots
(2 x 70,410 = 140,820) minus counted account appearances (140,166) leaves 654 empty account
cells, which equals the 58 February pool trades plus the 596 August pool fills already
recorded in section 1. The match is exact.

The pull was checked for truncation and shows none. Accounts absent from it are distributed
evenly across the account-ID space (second character A 98, B 97, C 87, D 90) in the same
proportions as the pull itself (A 210, B 235, C 212, D 218), so a cursor-truncated page is
ruled out as the explanation.

### Account-level verification

A stratified sample of 30 accounts from the 372 was queried against Horizon
`/accounts/{id}/operations?limit=200&order=asc`: the 10 with the most trade appearances, 10
from the middle of the distribution, and 10 with a single trade. Twenty-nine returned data;
one initial call failed transiently and succeeded on retry.

| `change_trust` operations found | Accounts |
|---|---|
| 0 | 25 |
| 1 | 3 |
| 2 | 1 |

Every one of the 29 carried a large path-payment count, from 29 to 200 within the first 200
operations; on 12 of them path payments accounted for 194 to 200 of the first 200 operations.
The four accounts with any `change_trust` were inspected individually: the assets trusted were
yXLM, TscOpe3, USDC, USDC and AQUA. **No account in the sample has ever trusted USTRY.**

One account, `GA2X4GX5DEW7FANH4AFVULQJ3SAQCGYVCL6N3OA7RGCEHKIWIJCZHPWV`, returned its complete
lifetime history within the 200-operation limit, from `create_account` on 2026-04-18 to its
last operation on 2026-08-31, containing zero `change_trust` operations. It appears in the
August trade file. Its path payments carry a native send and destination asset, so the route
enters and leaves in XLM and passes through USTRY as an intermediate hop.

### Limits of this evidence

- 29 of 372 accounts were examined. The finding is a sample result, not a census.
- For the 12 busiest sampled accounts, `change_trust = 0` is verified only across the first
  200 operations, not across their full history. For the remaining 17 the limit does not
  bind and the result is complete.
- Only one account's route was inspected end to end. The specific route shape (XLM in, XLM
  out) is established for that account alone; that the others are path-payment participants
  is established, the routes they take are not.
- No claim is made here about how the 372 split between routers, closed trustlines, merged
  accounts, or any other cause. That split is unmeasured.

---

## Claims flagged for review, not edited

> **Superseded by Amendment 1 (2026-09-02).** The claims below were revised in
> `07-supporting-metrics.md` 1.0.7-draft after directional verification. The section is kept
> as written so the reasoning that prompted the revision stays on the record.

In `docs/methodology/07-supporting-metrics.md` section 1:

| Line | Claim |
|---|---|
| 33 | the flow is one-directional (4 sellers against 336 buyers) |
| 33 | the same evidence rules out two-sided market making |
| 109 | August shows 4 sellers against 336 buyers |
| 109 | that rules out two-sided liquidity provision as firmly as it rules out wash trading |
| 109 | `GABFRFPYM2` appears in 45,133 trades without buying once |

The first four rest on reading each account in the trade record as a party taking a position
in USTRY. A path-payment router takes no position: a single `XLM -> USTRY -> XLM` route makes
the same account both a buyer and a seller of USTRY inside one operation, and only the
USTRY/USDC leg lands in the recorded file. The other leg sits in a pair that was not recorded.

The fifth is a separate and smaller matter: `scripts/holderstats` counts 55,199 side
appearances for that account across both files, against section 1's figure of 45,133. The
difference is most plausibly February, since section 1's sentence describes August. It needs
confirming, not correcting on assumption.

---

## What this record does not affect

Nothing in section 1's Definition, its five exclusion conditions, its Result table, the
specimen A and B outcomes, or the count of 461 Genuine trades for specimen C. Those measure
trades, not the intent or the position of the accounts behind them.

The trades in question are real. They consumed real order-book depth and are not fabricated,
so they remain Genuine under section 1's rule as written. Section 1's dust exclusion rests on
notional value, not on authenticity, and is unaffected.

Section 2's holder figures are unaffected: population 263, circulating supply
10,432,382.3504695 USTRY, top 1 91.5406%, top 10 99.9475%, HHI 8,410.8452. Every one of these
is computed from rows present in the pull and none depends on how the 372 absent accounts are
interpreted.

---

## Reproduction

```
go run ./scripts/holderstats \
  -holders docs/evidences/2026-08-31-USTRY.GCRYUGD5-holders-and-supply/holders.csv \
  -trades docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv,docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-08-01_2026-09-01.csv \
  -trade-labels 2026-02,2026-08 \
  -out docs/evidences/derived \
  -methodology-version 1.0.5-draft
```

Per-account verification, for any account in the third classification column:

```
curl -s "https://horizon.stellar.org/accounts/{ACCOUNT}/operations?limit=200&order=asc" \
| jq -r '._embedded.records[]? | select(.type=="change_trust")
         | "\(.created_at)\t\(.asset_code // .asset_type)\t\(.limit)"'
```

An empty result across an account's full lifetime history means the account never trusted any
asset, and therefore never held USTRY.

---

## Open items this record does not settle

1. ~~Whether section 1 withdraws the two-sided-market conclusion or narrows its scope to the
   USTRY/USDC pair.~~ **Settled in Amendment 1.**
2. ~~The 45,133 against 55,199 count for `GABFRFPYM2`.~~ **Settled in Amendment 1.**
3. Whether the 372 accounts are a single population or a mixture, which would require
   sampling well beyond the 29 examined here. **Still open.**

---

# Amendment 1 (2026-09-02): directional verification

This amendment adds a measurement taken after the record was first drafted. It does not
reverse anything above; it closes two of the three open items and narrows one claim in the
body that was stated more loosely than the data warranted.

## What prompted it

The body of this record noted that side-of-trade (`base_account` / `counter_account`) is not
direction, and that direction lives in `base_is_seller`. That raised a question the record
could not answer at the time: whether section 1's seller and buyer counts had been derived
from the side columns, in which case they would be wrong independently of the routing finding.
They had not. Column 12 of both trade files is `base_is_seller`, and direction is recorded.

## Measurement

Counting `base_account` as the USTRY seller when `base_is_seller` is true and
`counter_account` when it is false, over both recorded files:

| | February 2026 | August 2026 |
|---|---|---|
| Distinct USTRY sellers | 5 | 4 |
| Distinct USTRY buyers | 189 | 336 |
| Accounts appearing on both sides | 2 | 2 |

`GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN`, the largest seller and the
second-largest holder in the 31 August pull:

| | Sells | Buys |
|---|---|---|
| February 2026 | 10,064 | 2 |
| August 2026 | 45,133 | 0 |
| **Total** | **55,197** | **2** |

Section 1's figure of 45,133 sells without a single buy is exact and scoped to August; the
account buys twice in February. Sell-side appearances across both files total 55,197, and
55,197 + 2 = 55,199, which reconciles exactly with the side-appearance count emitted by
`scripts/holderstats`. **Open item 2 is closed.**

## Correction to the body of this record

The body reasoned that an account routing `XLM -> USTRY -> XLM` would appear on both sides of
the USTRY/USDC record. That is wrong, and the count of 2 accounts on both sides is what
exposes it. Such a route carries two USTRY legs in *different pairs*: USTRY is acquired on one
pair and disposed of on another. Only the USTRY/USDC leg lands in these files, so a routing
account appears once, on one side. The observed count of 2 is therefore consistent with the
routing finding rather than evidence against it, and section 1's within-ledger round-trip test
is unaffected.

## Effect on section 1

The directional counts confirm the observation while leaving the inference withdrawn, which is
the outcome recorded in `07-supporting-metrics.md` 1.0.7-draft:

- the one-sided flow in USTRY/USDC is measured and correct, now scoped explicitly to that pair
- the inference that this rules out two-sided market making is withdrawn, because a routing
  account trades the opposite USTRY leg on a pair these files do not record
- February's split (5 against 189) is recorded alongside August's (4 against 336); the same
  shape in two separate periods makes this a pattern rather than a one-month anomaly

**Open item 1 is closed.** Open item 3 remains open: 29 of 372 accounts have been examined,
and whether the remainder are a single population is still unmeasured.

## Reproduction

```
awk -F, 'NR>1 {
  gsub(/"/,"")
  if ($12=="true") { s=$13; b=$14 } else { s=$14; b=$13 }
  if (s!="") sell[s]++
  if (b!="") buy[b]++
} END { print "sellers:", length(sell), " buyers:", length(buy) }' <trade file>
```
