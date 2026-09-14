# The USTRY book was deleted and re-posted at 00:10 UTC on every day of February 2026, and the manipulation landed twelve seconds inside that window

**Read on:** 2026-09-14, from public Horizon mainnet, no account required.
**Account:** `GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN`, the market
maker whose ladder is the USTRY/USDC book. The pair identity is DEC-001's.
**Window:** ledger 61027032 to 61429800, which is 2026-02-01 to 2026-02-28 inclusive.
**MethodologyVersion:** none. Every figure is a count over Horizon operation records.
Nothing passed through `internal/domain`, so no threshold this project chose can move
any number here.

**What this answers.** `docs/evidences/2026-09-14-february-22-withdrawal-timeline.md`
proves the book of 22 February was deleted by its own maker twelve seconds before the
manipulation, and closes by saying the one thing it cannot say is whether that
withdrawal was routine. This is that measurement. **It was routine. It happened at the
same minute of every single day of the month.**

**What it still does not say.** Nothing here establishes that the attacker knew the
schedule. Section 5 gives the number a reader needs to weigh that and stops there,
because under the zone map what the number MEANS is Al's sentence and not Claude's.

---

## 1. How it was measured

One forward walk of the account's own operation stream:

```bash
A=GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN
curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61027032*4294967296))&order=asc&limit=200"
# then page on paging_token until the ledger exceeds 61429800
```

| | |
|---|---|
| Pages | 231 |
| Operation records read | 46,033 |
| First record | 2026-02-01T00:10:04Z |
| Last record | 2026-02-28T00:11:53Z |
| Offer operations on USTRY/USDC | 1,282 |

**The walk is complete for this account over this window.** It is not sampled, it is not
capped, and it is not the reconstruction's walk: it reads the operation stream directly
rather than folding it, so nothing in this document depends on
`internal/horizon/replay.go` being right.

---

## 2. What a cycle looks like

The account's USTRY operations group into transactions of six operations each, a
three-level ladder on each side. Every transaction is one of two shapes:

- a **delete-all**: six operations, every one with `amount: 0.0000000`, which on
  `manage_buy_offer` and `manage_sell_offer` deletes the offer;
- a **post**: six operations, every one with a non-zero amount.

Over February: **116 delete-all transactions and 114 posts.** There are no mixed
transactions. The maker never edits its ladder in place; it removes the ladder and
posts a new one.

An **absence window** is the interval from a delete-all to the next post. During it the
maker holds no USTRY offer, and the maker is essentially the whole book: at the sampled
ledger of 22 February its three bids were 211,798.555 of the 212,492.602 units the
reconstruction reports on that side, which is 99.67 per cent.

---

## 3. The cadence, over the whole month

| | Seconds |
|---|---|
| Windows | 116 |
| Shortest | 5 |
| Median | 71 |
| Longest | 946 |
| Total absent | 7,462 |

**7,462 seconds out of 2,419,200 is 0.3084 per cent of February.** For 99.69 per cent of
the month this maker was quoting both sides.

---

## 4. The daily window, which is the finding

Thirty-three of the 116 windows open between 00:09 and 00:11 UTC, and they fall on
**28 distinct days out of 28.** There is no day in February 2026 without one.

| Day | Opened | Duration | Delete ledger |
|---|---|---|---|
| 1 Feb | 00:10:04 | 79 s | 61027099 |
| 2 Feb | 00:10:08 | 75 s | 61042050 |
| 3 Feb | 00:10:05 | 103 s | 61056906 |
| 4 Feb | 00:10:05 | 80 s | 61071694 |
| 5 Feb | 00:10:07 | 12 s | 61086676 |
| 6 Feb | 00:10:05 | 77 s | 61101515 |
| 7 Feb | 00:10:06 | 72 s | 61116582 |
| 8 Feb | 00:10:07 | 75 s | 61131952 |
| 9 Feb | 00:10:08 | 74 s | 61147098 |
| 10 Feb | 00:10:03 | 108 s | 61162013 |
| 11 Feb | 00:10:05 | 77 s | 61176902 |
| 12 Feb | 00:10:05 | 82 s | 61191718 |
| 13 Feb | 00:10:06 | 87 s | 61206486 |
| 14 Feb | 00:10:03 | 81 s | 61221332 |
| 15 Feb | 00:10:04 | 85 s | 61236185 |
| 16 Feb | 00:10:05 | 81 s | 61250961 |
| 17 Feb | 00:10:07 | 100 s | 61265694 |
| 18 Feb | 00:10:07 | 17 s | 61280501 |
| 19 Feb | 00:10:02 | 82 s | 61295347 |
| 20 Feb | 00:10:06 | 76 s | 61310249 |
| 21 Feb | 00:10:04 | 88 s | 61325181 |
| **22 Feb** | **00:10:09** | **79 s** | **61340261** |
| 23 Feb | 00:10:08 | 78 s | 61355135 |
| 24 Feb | 00:10:06 | 110 s | 61370021 |
| 25 Feb | 00:10:07 | 68 s | 61384994 |
| 26 Feb | 00:10:07 | 76 s | 61399897 |
| 27 Feb | 00:10:05 | 109 s | 61414848 |
| 28 Feb | 00:10:06 | 78 s | 61429741 |

20 February is listed once here and appears six times in the raw window list, because
that day's deletes were spread over six consecutive ledgers rather than submitted in
one; the window is the same window counted from each of them.

**It is not a USTRY behaviour, it is the account restarting.** In the 00:09 to 00:12
band on 22 February the account deleted and re-posted six operations on each of
**four** pairs: USTRY/USDC, TESOURO/USDC, USDC/ZUSD and CETES/USDC. Across the month
each of those four pairs was deleted inside that band on **28 of 28 days**. This is a
scheduled process that rebuilds the account's whole book once a day, a few seconds
after 00:10 UTC.

---

## 5. Where the manipulation falls

| | |
|---|---|
| The maker deletes its ladder | ledger 61340261, 2026-02-22T00:10:09Z |
| The manipulation trade | ledger 61340263, 2026-02-22T00:10:21Z |
| The maker re-posts | ledger 61340274, 2026-02-22T00:11:28Z |

**Twelve seconds into a 79 second window that opens at the same minute every day.**

The number a reader will want, and it is arithmetic rather than a claim: a moment chosen
without reference to this schedule falls inside SOME absence window with probability
0.3084 per cent, which is about one in 324.

**That is not proof of anything and this document does not extend it into one.** A one
in 324 coincidence happens to somebody. What the number does is set the price of the
alternative explanation, and the report's section 6 is where that is weighed by the
person the zone map puts in charge of weighing it.

---

## 6. What this changes for the product, stated as facts

1. **A daily snapshot would have read `LOW`.** The sampled ledger of 22 February is
   61340172, at 00:01:24, which is 8 minutes 45 seconds before the window opened. On
   that row the reconstruction reports 227,479 USDC of executable depth at 2 per cent.
2. **A snapshot taken at any of the other 27 daily samples would also have read the
   deep book**, because every one of them was taken at a first-trade-after-midnight
   ledger and none fell inside a window.
3. **The dangerous minute is the same minute every day.** A monitor sampling once a day
   at a fixed time either always sees it or never does, and which of those is decided by
   the sampling time rather than by the market.
4. **The engine was never wrong about depth.** Both readings are correct for the ledger
   they name. What separates them is nine minutes.

---

## 7. The reading decision, in three sentences

The decision: the cadence was measured by walking one account's complete operation
stream for the month and pairing each delete-all with its next post, rather than by
re-running the book reconstruction at many intra-day ledgers. The alternative rejected:
sampling the reconstruction every few minutes through 22 February, which is what would
be needed to show the book's state continuously. It was rejected because the
reconstruction costs about 1,900 Horizon requests per run and would have answered a
narrower question than the operation stream answers directly, and because a result read
off the raw operations does not depend on the fold whose defects this project is
currently documenting.

---

## 8. Reproducing this

The walk, and the window arithmetic over it:

```bash
A=GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN
# page from ledger 61027032 to 61429800 on paging_token, 200 records a page, 231 pages
curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61027032*4294967296))&order=asc&limit=200"
```

For each transaction holding only USTRY/USDC offer operations, classify it as a
delete-all when every `amount` is `0.0000000` and as a post when none is. A window runs
from a delete-all to the next post; its duration is the difference of the two
`created_at` values. The 00:10 table is the windows whose opening time falls between
00:09 and 00:11 UTC.

The three ledgers in section 5 are also in
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400.csv`
and in `docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv`,
both already committed, so section 5 can be checked without a network at all.
