# The daily book-rebuild is real beyond the USTRY maker, and it is not what every maker does

**Read on:** 2026-09-17, from public Horizon mainnet, no account required.
**Window:** ledger `61027032` to `61429800`, which is 2026-02-01 to 2026-02-28
inclusive. The same window as
`docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md`, so the figures here and
the figures there are comparable without adjustment.
**MethodologyVersion:** none. Every figure is a count or a subtraction over Horizon
operation records. Nothing passed through `internal/domain`, so no threshold this project
chose can move any number here, and nothing passed through `internal/horizon`'s fold
either, so no defect in the reconstruction can reach these numbers.

**What this answers.** The draft of the backtest report's section 6.5 offers a sentence
for Al that reads, in its strongest form, "any lending protocol or vault consuming a
once-a-day depth figure inherits that blind spot". Section 6 of that draft flags the
sentence as wider than the measurement: the daily 00:10 UTC rebuild was established on
**one** account, on **one** pair. This document is the requested widening. Three further
market-maker accounts on three unrelated pairs were walked over the same month with the
same rule.

**The answer is split and the split is the finding.** One of the three rebuilds its
ladder on a fixed daily schedule, the same as the USTRY maker and within three minutes of
the same clock time. The other two do not: one is essentially never absent because it
edits its offers in place and never deletes them, and one is absent often but at no
regular hour. **So the pattern is real and recurrent, and it is not universal.** A
sentence that says every maker does this would be false, and this document is what makes
the narrower sentence available.

**A second thing came out of the control walk and it is a correction owed to the report.**
Re-walking the USTRY maker with this script reproduces the 14 September figures exactly,
which is section 3. It also shows that the 7,462 seconds that document reports is a sum
over windows that overlap, so the same seconds are counted more than once. The union is
6,847 seconds, which is 0.2830 per cent of February and about one in 353 rather than one
in 324. No finding moves. Two numbers the backtest report quotes do, and section 3.2 is
where that is set out.

---

## 1. Choosing the accounts, and a trap that had to be walked around first

The candidate pool was the monitored set the live API serves, 61 assets at
`https://api.keels.app/v1/assets`. For each asset both sides of its book against USDC
were read from `/offers`, and the offers were counted per `seller`. That yields
**106 distinct accounts** holding a top-three position on some book on 2026-09-17.

**Most of them are no use for a February measurement, and finding that out was the first
result.** Each of the 106 was probed with a single request for its first 200 operations
from ledger `61027032`. Only **25** were posting or cancelling offers inside the February
window at all. Several of the largest makers on today's books, including the account that
is simultaneously top maker on ETH, SHX and VELO, held no offers whatever in February 2026
and were doing payments and claimable balances instead. Two had no operations before April
and June 2026 respectively, so the accounts did not yet exist as makers.

**This is worth recording because picking today's biggest maker and walking it backwards
is the obvious way to do this and it silently produces an empty month.** The maker
population on these books turned over between February and September 2026.

The three walked here were chosen from the 25 survivors, on unrelated pairs and unrelated
to each other.

| Account | Book | Share of that book's offers on 2026-09-17 | Offer ops in the first 200 records of February |
|---|---|---|---|
| `GCXSDGZIDUNDEBEPZXRIQEOUPS7SQU2L5KBMF5UE6LDJKTNNH4EVVNSS` | sUSD/USDC | 26 of 64, **41 %** | 115 of 200 |
| `GCSPJLGUAMWACHKIGMZEH2N62JVONEUV3INOA6A5BWCDP53YX6H5YYP5` | LSP/USDC | 11 of 232, **5 %** | 200 of 200 |
| `GAXOUC6QADC35WEGCLK2VAWLUYBBB27VOAEQHYXNFQJPXIZVTIU2UNA4` | yXLM/USDC | 9 of 111, **8 %** | 190 of 200 |

**ONLY THE FIRST OF THE THREE IS DOMINANT ON ITS BOOK, AND THE SHARES ABOVE ARE
SEPTEMBER'S RATHER THAN FEBRUARY'S.** The USTRY measurement is meaningful partly because
that maker held 99.67 per cent of its book's bid side at the sampled ledger, so its
absence WAS the book's absence. Nothing here establishes that for the LSP and yXLM
accounts, in September or in February: the share column counts offers, not depth, and it
was read seven months after the window that was walked.

That limit is narrower than it first looks, and section 5 is where it is priced. Whether
a maker rebuilds its ladder on a fixed daily schedule is a fact about that maker's own
operation stream and needs no dominance at all. Turning maker-absence into BOOK-absence is
what needs dominance, and this document claims that step only for the sUSD account.

---

## 2. How it was measured

The rule is a transcription of
`docs/evidences/2026-09-14-maker-withdrawal-cadence-february.md` sections 1 and 8, not a
new one, because a second rule would make the comparison to the USTRY maker meaningless.

One forward walk of each account's own operation stream:

```bash
A=GCXSDGZIDUNDEBEPZXRIQEOUPS7SQU2L5KBMF5UE6LDJKTNNH4EVVNSS
curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61027032*4294967296))&order=asc&limit=200"
# then page on paging_token until the ledger exceeds 61429800
```

Offer operations are grouped by (transaction, pair). A group is a **delete-all** when
every `amount` is `0.0000000`, which is what deletes an offer on `manage_buy_offer` and
`manage_sell_offer`, and a **post** when no amount is zero. An **absence window** runs
from a delete-all to the next post on the same pair, and its duration is the difference of
the two `created_at` values.

`cadence.py` beside this file is that walk and that arithmetic, and it is the script that
produced every number below.

### The four decisions inside it, because `docs/evidences/` is YELLOW

1. **`create_passive_sell_offer` is read as an offer operation**, not assumed absent. None
   of the four accounts used it. Had one used it and not been read, its book would have
   read as never emptying, which is the wrong direction for a document arguing that books
   do empty.
2. **A second delete-all arriving while a window is open does not open a second window.**
   The maker is already absent and counting it twice would double-count one absence.
3. **A transaction mixing zero and non-zero amounts is classified `mixed` and opens
   nothing.** None of the four accounts produced one. An account producing many is editing
   its ladder rather than replacing it, and that is reported rather than forced into the
   delete/post shape. This is exactly what the LSP maker in section 4 turned out to do, by
   a different route.
4. **A delete-all whose closing post falls beyond the last ledger of the window is
   excluded from every count.** It is reported separately as an unclosed delete. This
   matters for the sUSD maker, whose 28 February rebuild is exactly that case.

Decisions 2, 3 and 4 all make the measured absence **shorter** rather than longer. Every
absence figure below is therefore a lower bound, which is the safe direction for a
document whose use is to argue that a book was empty when somebody traded against it.

### The pair label

An offer names a selling and a buying asset, and the two sides of one book arrive with
those two fields swapped. The pair label sorts the two asset strings, so a bid and an ask
on the same book land on the same pair. Without that, every book splits into two
half-books and no window ever closes.

---

## 3. Control: the script reproduces the published USTRY figures, and finds one thing in them

`GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN` was walked again with this
script before any number it produced for a new account was believed.

**The walk is identical, to the record.**

| | Published 2026-09-14 | This script |
|---|---|---|
| Pages | 231 | **231** |
| Operation records | 46,033 | **46,033** |
| First record | 2026-02-01T00:10:04Z | **2026-02-01T00:10:04Z** |
| Last record | 2026-02-28T00:11:53Z | **2026-02-28T00:11:53Z** |
| Offer ops on USTRY/USDC | 1,282 | **1,282** |
| Delete-all transactions | 116 | **116** |
| Post transactions | 114 | **114** |
| Days in the 00:09–00:11 band | 28 of 28 | **28 of 28** |

The reader, the pair label, the TOID arithmetic and the delete/post classifier all agree
with a result produced independently three days earlier.

**The four-pair claim checks out too.** Section 4 of that document says the 00:10 rebuild
is the account restarting rather than a USTRY behaviour, and names four pairs deleted
inside the band on 28 of 28 days. The same walk, grouped per pair:

| Pair | Offer ops | Windows | Modal band | Days in band |
|---|---|---|---|---|
| `CETES/USDC` | 31,370 | 4,142 | 00:10 | 27 of 28 |
| `TESOURO/USDC` | 12,348 | 106 | 00:10 | 28 of 28 |
| `USTRY/USDC` | 1,282 | 106 | 00:10 | 28 of 28 |
| `ZUSD/USDC` | 996 | 83 | 00:10 | 28 of 28 |

All four land on 00:10, which is why the whole account's raw operation stream is kept
beside this file rather than only the USTRY slice of it.

### 3.1 The window counts differ, 116 against 106, and the cause is a rule rather than data

Every input above matches, so the ten-window gap is entirely in how a delete-all is paired
with a post.

- **The published rule opens a window at every delete-all.** When several deletes land
  before one post, each of them produces a window, and all of those windows end at the
  same post. Section 4 of that document says so in as many words about 20 February:
  "that day's deletes were spread over six consecutive ledgers … the window is the same
  window counted from each of them."
- **This script opens one window per absence**, decision 2 in section 2 above.

Running this script's walk under the published rule reproduces the published figures
exactly:

| Under the published rule | Published | This script |
|---|---|---|
| Windows | 116 | **116** |
| Shortest | 5 s | **5 s** |
| Longest | 946 s | **946 s** |
| Total absent | 7,462 s | **7,462 s** |
| Per cent of February | 0.3084 % | **0.3084 %** |
| One in | 324 | **324** |
| Windows in the 00:09–00:11 band | 33 | **33** |

So the two documents disagree about nothing. The only figure that does not land is the
median, 71 s published against 70 s here, which is a tie-break convention on an
even-length list of 116 values and moves no argument.

### 3.2 The consequence, which belongs to the report rather than to this file

**Those overlapping windows are nested, so summing them counts the same seconds more than
once.** Windows `[D1,P]`, `[D2,P]`, … `[Dk,P]` all end at the same post, and their union is
just `[D1,P]`. The 7,462 s figure is the sum; the union is **6,847 s**.

| | Sum of windows (published) | Union of absences |
|---|---|---|
| Total absent in February | 7,462 s | **6,847 s** |
| Per cent of the month | 0.3084 % | **0.2830 %** |
| A moment falls inside one about | 1 in 324 | **1 in 353** |

**This is a correction to an arithmetic figure and not to any finding.** Every conclusion
in the 14 September document survives it untouched: the maker still deleted its ladder on
28 of 28 days, the manipulation still landed 12 seconds into a 79-second window, and the
withdrawal was still routine. What changes is a number the backtest report quotes twice.
`docs/report/blend-february-2026.md` section 6.4 carries "0.3084 per cent" and "about one
in 324", and version C of the section 6.5 draft repeats both. The report itself calls that
figure "arithmetic rather than an accusation", and arithmetic offered to a client in those
terms should be the union rather than the sum.

**Which of the two to print is Al's**, because the choice is about what the sentence
claims rather than about what the chain says, and both numbers are defensible if the rule
is stated beside them. The sum answers "how many maker-absences were open, summed over
their durations". The union answers "how much of February had this maker off the book",
which is the question the one-in-N sentence actually asks. Section 5 below and the tables
in section 4 use the union throughout, so the three new accounts are comparable to this
one.

---

## 4. The three makers

Every column was produced by the same script under the same rule, the union of absences of
section 3.2, so the four are comparable without adjustment.

| | USTRY maker (control) | sUSD maker | LSP maker | yXLM maker |
|---|---|---|---|---|
| Account | `GABFRFPY` | `GCXSDGZI` | `GCSPJLGU` | `GAXOUC6Q` |
| Pages walked | 231 | 27 | 12 | 15 |
| Operation records | 46,033 | 5,276 | 2,358 | 2,841 |
| Offer ops on the pair | 1,282 | 3,450 | 2,241 | 1,219 |
| Delete-all transactions | 116 | 651 | **1** | 187 |
| Post transactions | 114 | 2,799 | 2,240 | 1,032 |
| Mixed transactions | 0 | 0 | 0 | 0 |
| Absence windows | 106 | 623 | **1** | 138 |
| Shortest | 5 s | 18 s | 7,885 s | 11 s |
| Median | 74 s | 29 s | 7,885 s | 28 s |
| Longest | 946 s | 7,233 s | 7,885 s | 53,876 s |
| Total absent | 6,847 s | 127,618 s | 7,885 s | 73,972 s |
| Per cent of February | **0.2830 %** | **5.2752 %** | **0.3259 %** | **3.0577 %** |
| Modal opening minute | 00:10 UTC | **00:12 UTC** | 23:37 | 10:59 |
| Closed windows in that band | 28 | 27 | 1 | 5 |
| Distinct days in that band | **28 of 28** | **27 of 28**, plus an unclosed delete on the 28th | 1 of 28 | 2 of 28 |

The USTRY column is this script's own walk, not a quotation. Under the published pairing
rule the same walk gives 116 windows and 7,462 s, which is section 3.1.

**Three different shapes, and only one of them repeats the USTRY maker's.**

### 4.1 The sUSD maker rebuilds daily, at 00:12 UTC, on every day of the month

| Day | Opened | Duration | Delete ledger |
|---|---|---|---|
| 1 Feb | 00:12:50 | 28 s | 61027128 |
| 2 Feb | 00:12:49 | 29 s | 61042078 |
| 3 Feb | 00:12:45 | 35 s | 61056933 |
| 4 Feb | 00:12:45 | 34 s | 61071722 |
| 5 Feb | 00:12:48 | 29 s | 61086704 |
| 6 Feb | 00:12:50 | 30 s | 61101543 |
| 7 Feb | 00:12:48 | 30 s | 61116610 |
| 8 Feb | 00:12:48 | 28 s | 61131979 |
| 9 Feb | 00:12:49 | 29 s | 61147126 |
| 10 Feb | 00:12:47 | 30 s | 61162042 |
| 11 Feb | 00:12:44 | 34 s | 61176929 |
| 12 Feb | 00:12:49 | 28 s | 61191746 |
| 13 Feb | 00:12:48 | 29 s | 61206513 |
| 14 Feb | 00:12:44 | 37 s | 61221360 |
| 15 Feb | 00:12:44 | 33 s | 61236212 |
| 16 Feb | 00:12:49 | 28 s | 61250989 |
| 17 Feb | 00:12:47 | 29 s | 61265721 |
| 18 Feb | 00:12:45 | 33 s | 61280528 |
| 19 Feb | 00:12:46 | 28 s | 61295375 |
| 20 Feb | 00:12:48 | 29 s | 61310277 |
| 21 Feb | 00:12:52 | 27 s | 61325210 |
| **22 Feb** | **00:12:46** | **29 s** | **61340288** |
| 23 Feb | 00:12:45 | 28 s | 61355162 |
| 24 Feb | 00:12:49 | 3,626 s | 61370049 |
| 25 Feb | 00:12:48 | 31 s | 61385022 |
| 26 Feb | 00:12:45 | 30 s | 61399924 |
| 27 Feb | 00:12:47 | 2,130 s | 61414876 |
| **28 Feb** | **00:12:45** | **unclosed at walk end** | **61429768** |

Twenty-seven closed windows plus the 28 February delete whose post falls beyond ledger
`61429800`. **There is no day of February 2026 on which this account did not delete its
whole sUSD ladder within eight seconds of 00:12:47 UTC.** The clock is tighter than the
USTRY maker's: every opening lies between 00:12:44 and 00:12:52, a spread of eight
seconds across four weeks.

**The two schedules are two and a half minutes apart on the same day.** On 22 February the
USTRY maker deleted at ledger `61340261`, `00:10:09`, and the sUSD maker deleted at ledger
`61340288`, `00:12:46`. These are unrelated accounts quoting unrelated assets, and both
rebuild in the first quarter hour after UTC midnight.

**This maker is absent seventeen times more of the month than the USTRY maker**, 5.2752
per cent against 0.3084 per cent, across 623 windows rather than 116. The daily rebuild is
only 27 of those 623. So the two facts are separable and this document keeps them separate:
*a fixed daily rebuild* is one thing, and *how much of the month a book is empty* is
another, and the second does not follow from the first.

### 4.2 The LSP maker is never absent, because it never deletes

One delete-all in the entire month, against 2,240 posts. Reading the offer ids explains
it: this account holds **seven** offer ids across February and updates six of them
**348 times each**, always with a non-zero amount and never with `offer_id` `0`. It
amends its ladder in place. An amended offer is never removed from the book, so there is
no instant in February 2026 at which this book was missing THIS maker.

**This is the counter-example, and it is the reason the report's sentence has to be
narrowed rather than merely supported.** The hazard section 6.4 of the report describes is
a maker that vanishes on a schedule. This maker never vanishes, so for its contribution to
the book there is nothing for a once-a-day sample to miss. What other makers on the LSP
book were doing was not measured, so the claim stops at this account rather than at the
whole book.

The single window, 7,885 seconds on 12 February at 23:37, is a one-off and not a schedule.

### 4.3 The yXLM maker is absent often and on no schedule

138 windows, 3.0577 per cent of the month, median 28 seconds. The modal opening minute,
10:59 UTC, carries five windows falling on **two** days out of 28. Its other pairs behave
the same way: `USDC/yUSDC` has 59 windows with a modal band covering 1 day, and
`yUSDC/yXLM` has 34 windows covering 2 days.

**This is the third shape: a book that empties frequently but unpredictably.** A once-a-day
sample of this book is wrong some of the time, and no choice of sampling time makes it
reliably right or reliably wrong. That is a different hazard from the USTRY maker's and it
is arguably worse, because it cannot be dodged by moving the sampling clock.

---

## 5. What this licenses, and what it does not

Stated as facts, because what they MEAN is Al's sentence under the zone map and not this
document's.

1. **A fixed daily book-rebuild is not unique to the USTRY maker.** Two of four accounts
   examined do it, on unrelated pairs, both within three minutes of 00:11 UTC, on 28 of 28
   days.
2. **It is not what every maker does.** One of the four never deletes its offers at all,
   and one deletes constantly at no fixed hour.
3. **The share of the month a maker is absent does not generalise and was never claimed
   to.** Across the four accounts it ranges from 0.2830 per cent to 5.2752 per cent, a
   factor of nineteen. A daily rebuild says nothing about how much of the month a book is
   empty: the sUSD maker has both the tightest schedule and by far the most absence, and
   the two facts are produced by different behaviour.
4. **Therefore a sentence of the form "any lending protocol consuming a once-a-day depth
   figure inherits that blind spot" is still wider than the measurement**, because the LSP
   maker's book has no such blind spot.
5. **A sentence of the form "books on Stellar are rebuilt on fixed daily schedules often
   enough that a once-a-day sample can sit inside or outside the rebuild by accident, and
   which one it does is decided by the sampling clock rather than by the market" is now
   measured** on two independent accounts rather than assumed from one.

**What was NOT done, so nobody reads more into this than it carries.**

- **Four accounts is not a survey of Stellar.** They were selected for holding a top-three
  position on a monitored book in September 2026 and for being active in February, which
  is not a random sample, so no proportion of "makers on Stellar" follows from them.
- **Only the USTRY and sUSD accounts are shown to be their book.** For the LSP and yXLM
  accounts this document measures when THAT MAKER's offers were absent, not when the book
  was empty. Another maker may have been quoting throughout. Points 1 and 2 above survive
  this, because they are statements about rebuild schedules rather than about depth;
  point 4 leans on the LSP account and is stated in the weaker form it can carry: that
  maker has no daily blind spot, whoever else was on its book.
- **The reconstruction was not re-run at any intra-day ledger for the three new
  accounts.** This document says when offers were absent. It does not say what depth those
  books carried at any instant, and the depth figures in the report's section 5 remain the
  only measured ones.

---

## 6. Reproducing this

```bash
cd docs/evidences/2026-09-17-maker-cadence-generalisation

# The control. 231 pages, and it took about 50 minutes against public Horizon.
python3 cadence.py walk GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN \
    61027032 61429800 USTRY-GABFRFPY
python3 cadence.py windows USTRY-GABFRFPY "USDC:GA5ZSEJY/USTRY:GCRYUGD5"

# One walk per new account, 12 to 27 Horizon pages each. Writes <prefix>-ops.json.
python3 cadence.py walk GCXSDGZIDUNDEBEPZXRIQEOUPS7SQU2L5KBMF5UE6LDJKTNNH4EVVNSS \
    61027032 61429800 sUSD-GCXSDGZI
python3 cadence.py walk GCSPJLGUAMWACHKIGMZEH2N62JVONEUV3INOA6A5BWCDP53YX6H5YYP5 \
    61027032 61429800 LSP-GCSPJLGU
python3 cadence.py walk GAXOUC6QADC35WEGCLK2VAWLUYBBB27VOAEQHYXNFQJPXIZVTIU2UNA4 \
    61027032 61429800 yXLM-GAXOUC6Q

# The window arithmetic over a walk already on disk. Writes <prefix>-windows.csv.
python3 cadence.py windows sUSD-GCXSDGZI
python3 cadence.py windows LSP-GCSPJLGU
python3 cadence.py windows yXLM-GAXOUC6Q
```

The walks committed beside this file are the ones the tables above were computed from, so
every number can be rechecked with the second command alone and no network at all. Each
`*-windows.csv` carries a `*-windows.meta.txt` provenance sidecar per DEC-010.

Re-running the first command reads live Horizon. The ledger range is closed and in the
past, so the operation stream it returns is fixed; a different answer means the walk was
truncated, not that the chain changed.
