# The book of 22 February was not lost by the fold. Its market maker deleted it, twelve seconds before the manipulation

**Read on:** 2026-09-14, from public Horizon mainnet, no account required.
**Pair:** `USTRY` `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`
against `USDC` `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`,
the identity fixed by DEC-001.
**MethodologyVersion:** none, and that is deliberate. Every figure below is either
a Horizon reading taken today or a cell of a CSV already committed to this
repository. Nothing passed through `internal/domain`, so nothing here depends on
any threshold this project chose.

**What this closes.** `tugas-b.md` item B2 sets out two explanations for the
discontinuity of 22 February and asks that one be supported by evidence and the
other RULED OUT. This document rules out the second. The bids did not go missing
from the reconstruction. They were cancelled on-chain by the account that posted
them, in three transactions this repository's fold read correctly, and the fold's
own counts change by exactly the number of offers those transactions deleted.

**What it does not close.** Why the maker withdrew is not established here and is
not establishable from these readings. Whether the withdrawal was ROUTINE is a
separate measurement and it is answered in
`docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md`, written beside this
one: it was, on all 28 days of the month. Section 6 carries what neither document
claims.

---

## 1. The discontinuity, as it stood

Two rows of `docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400.csv`,
ninety ledgers and about nine minutes apart:

| Ledger | Time UTC | Bids | Asks | Bid total | Ask total | Depth buy at 2% | Band |
|---|---|---|---|---|---|---|---|
| 61340172 | 00:01:24 | 17 | 6 | 212,492.602 | 214,262.008 | 227,479.655 | `LOW` |
| 61340262 | 00:10:15 | 14 | 3 | 694.047 | 1.2185315 | 0.0000001 | `CRITICAL` |

Both times are ledger close times as Horizon reports them, `GET /ledgers/61340172` and
`GET /ledgers/61340262`, not interpolations.

The 8 September reading called this unresolved and the 12 September reading left
it open. The two explanations it left open were that the market really emptied,
or that the fold loses the offers holding the earlier row up.

---

## 2. Nothing traded them away

`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv`
holds every trade Horizon reports for the pair that month. Between the two rows
above there are **four**, and this is the whole of them:

| Ledger | Time UTC | Price | Base amount |
|---|---|---|---|
| 61340172 | 00:01:24 | 1.0574269436 | 0.0618419 |
| 61340173 | 00:01:30 | 1.0574269436 | 0.1225355 |
| 61340173 | 00:01:30 | 1.0574269436 | 0.0973814 |
| 61340224 | 00:06:31 | 1.0574269436 | 0.0273371 |

**0.3090959 USTRY in total.** The bid side lost about 211,799 units in the same
window. Trading accounts for 0.00015 per cent of it, so whatever removed the book
was not a buyer.

---

## 3. Three transactions deleted it, and they are named

At ledger **61340261**, `2026-02-22T00:10:09Z`, the account
`GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN` submitted three
transactions. Each holds six offer operations, each operation has
`amount: 0.0000000`, which on `manage_buy_offer` and `manage_sell_offer` is a
delete, and all three succeeded.

| Transaction | Pair | Operations |
|---|---|---|
| `8f8ae8499e03f42343744a3278ac03360fa328790f1e3af990637e87fef0467e` | USTRY/USDC | 6, all deletes |
| `3b350d1a0b1a…` | TESOURO/USDC | 6, all deletes |
| `3cb72e4c37ab…` | USDC/ZUSD | 6, all deletes |

The six USTRY operations, in the order Horizon returns them:

| Operation id | Type | Offer id | Price |
|---|---|---|---|
| 263454414923366401 | `manage_buy_offer` | 1824767559 | 1.0574269 |
| 263454414923366402 | `manage_sell_offer` | 1824767560 | 1.0583791 |
| 263454414923366403 | `manage_buy_offer` | 1824767561 | 1.0563161 |
| 263454414923366404 | `manage_sell_offer` | 1824767562 | 1.0594899 |
| 263454414923366405 | `manage_buy_offer` | 1824767563 | 1.0533011 |
| 263454414923366406 | `manage_sell_offer` | 1824767564 | 1.0625049 |

**A three-level ladder on each side, removed in one transaction.** Offer
`1824767559` is the same offer the 12 September document proved was resting at the
daily sample, by two fills that bracket it: first at ledger 61337700, last at
61340224. It was the best bid on that row and it is the first line of this table.

The maker's CETES ladder went the same way over the following eleven ledgers,
61340266 to 61340272, one operation at a time.

The other account holding this book, `GBPFB6XN`, performed **no USTRY offer
operation at all** in the window; a walk of 200 of its operations from ledger
61340100 returns none.

---

## 4. The fold's own arithmetic closes it

This is the part that makes the conclusion a proof rather than a story, and it
needs no new reading: the numbers are already in the committed CSV.

| Quantity | 00:01:24 | Deleted at 00:10:09 | Expected after | CSV at 00:10:15 |
|---|---|---|---|---|
| Bid levels | 17 | 3 | 14 | **14** |
| Ask levels | 6 | 3 | 3 | **3** |
| Bid total | 212,492.602 | the maker's three | ~694 | **694.047** |
| Ask total | 214,262.008 | the maker's three | ~1.2 | **1.2185315** |

**The fold did not lose three bids and three asks. It applied three deletes.** A
reconstruction that had missed those operations would have carried the ladder
forward and shown a deep book at 61340262, and the one thing every reader of this
project already knows about that ledger is that the book there was almost empty:
`testdata/fixtures/ustry_pre_exploit.md`, computed by hand before this code
existed, gives one ask of 1.2185312 and one bid of 0.0001.

So the reconstruction agrees with the hand computation at the one ledger where a
hand computation exists, and the difference between the two rows is fully
accounted for by operations both of them saw.

**Explanation 2 is ruled out. Explanation 1 is what happened.**

---

## 5. The timeline, in the order it occurred

| Ledger | Time UTC | Gap | What |
|---|---|---|---|
| 61340172 | 00:01:24 | | the daily sample. 227,479 USDC of executable depth at 2%, band `LOW` |
| 61340224 | 00:06:31 | +5m 07s | the last ordinary trade, 0.0273371 USTRY at 1.0574 |
| **61340261** | **00:10:09** | **+3m 38s** | **the maker deletes its whole USTRY ladder, and its TESOURO and ZUSD ladders in the same ledger** |
| 61340263 | 00:10:21 | **+12s** | the manipulation. One trade, 0.0501003 USTRY for 5.3475699 USDC, price 106.7372828, against resting offer 1824788980 |
| 61340274 | 00:11:28 | +1m 07s | the maker re-posts all four ladders. The USTRY side comes back as 4,962.23 / 51,372.42 / 156,427.15 buying and 4,935.87 / 48,660.70 / 148,720.39 selling |

**The exposure window is 79 seconds wide** and the attack landed 12 seconds into
it. Outside it, on this evidence, the book carried about 212,000 units on each
side all day.

---

## 6. What this does not say, and it is the half a reader will want

**It does not say the withdrawal was targeted, coordinated or known in advance.**
Three separate transactions in one ledger followed by a re-post of everything 79
seconds later is also the exact shape of a market maker restarting: cancel the
book, recompute, re-post. Whether this maker did that hourly all February or did
it once, on the night of the incident, is a counting problem over its operation
stream and is NOT answered here.

**It is answered next door.** `docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md`
walks the same account's whole February and finds 116 of these windows, one of them
between 00:09 and 00:11 UTC on every one of the 28 days. The withdrawal was routine and
it was scheduled. What that means for the report's headline is still Al's, and the
cadence document says so in the same words.

**It does not settle what the numbers MEAN.** Under the zone map that is Al's, and
the specific sentence at stake is the report's section 6 headline. What this
document establishes is the fact the sentence has to be built on: the threshold
was crossed 12 seconds before the exploit, not days before it, and a daily sample
of this asset would have read `LOW` on the morning of 22 February.

**It does not repair the phantom ask.** `best_ask` on this row and on every row
from 9 February is still the one-stroop ghost that
`docs/evidences/2026-09-12-crossed-book-ustry-february.md` names, and
`p0`, `spread_pct` and both depth ladders inherit it. The BID figures this
document rests on do not: they come from offers whose operations are quoted above.

---

## 7. The reading decision, in three sentences

The decision: the discontinuity was tested by asking the operation stream what
happened between the two rows, rather than by re-running the reconstruction with
deeper caps. The alternative rejected: raising `max-pages-per-offering-account`
again and comparing the two runs, which is what the 8 September document priced at
three hours. It was rejected because a deeper walk can only ADD offers to the
earlier row, and the earlier row was already the deep one, so the experiment could
not have distinguished the two explanations no matter which way it came out.

---

## 8. Reproducing this

Sections 1, 2 and 4 are offline, against files already in the repository:

```bash
python3 - <<'EOF'
import csv
rows=list(csv.DictReader(open(
  "docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400.csv")))
for r in rows:
    if r['target_ledger'] in ('61340172','61340262'):
        print(r['target_ledger'], r['bids'], r['asks'],
              r['bid_amount_total'][:12], r['ask_amount_total'][:12], r['band'])

t=list(csv.DictReader(open(
  "docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv")))
w=[x for x in t if 61340172 <= int(x['ledger_seq']) <= 61340262]
print(len(w), "trades", sum(float(x['base_amount']) for x in w), "USTRY")
EOF
```

Section 3 is one request per transaction, and one per account:

```bash
curl -s "https://horizon.stellar.org/transactions/8f8ae8499e03f42343744a3278ac03360fa328790f1e3af990637e87fef0467e/operations?limit=200"

A=GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN
curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61340250*4294967296))&order=asc&limit=200"

B=GBPFB6XNLDMXQKOFJAH6IRTOMTEUU4ZWFHNRMYWZNXCZEDNE6UU66WSG
curl -s "https://horizon.stellar.org/accounts/$B/operations?cursor=$((61340100*4294967296))&order=asc&limit=200"
```

The cursor arithmetic is the TOID convention: a ledger's first operation sits at
`ledger << 32`, so seeking to `ledger * 4294967296` starts the page there.
