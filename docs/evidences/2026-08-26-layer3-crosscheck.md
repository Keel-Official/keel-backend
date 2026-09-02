# Validation Layer 3, executed: 60 recordings against 60 rebuilt books

**Run on:** 26 August 2026, from Horizon mainnet, no account required.
**Protocol:** `docs/methodology/10-validation.md` section 3.
**Recordings:** the sixty committed under `recordings/samples/`, taken at ledgers
64129586 to 64129592, recorded 2026-08-26T07:09:21Z.
**Rebuilt at:** ledger 64134034 and after, about seven hours later.
**Raw results:** `docs/evidences/layer3-crosscheck-2026-08-26.csv`, one row per
recording with every count behind every verdict.

**AMENDED 2 September 2026.** Section 4 predicted what a crosscheck run minutes
after the recording would produce. That experiment was run on 31 August and the
prediction is about half right. Section 4.1 below carries the correction and
`docs/evidences/2026-08-31-samehour-crosscheck.md` carries the measurement. The
original text of section 4 is left exactly as it was written.

**Reproduce it:**

```bash
go run ./cmd/keel crosscheck -out docs/evidences/layer3-crosscheck-2026-08-26.csv
```

It will NOT reproduce these numbers, and that is a property of the method rather
than a defect. Every hour that passes moves more offers, so the same command run
tomorrow compares fewer pairs. Section 4 is what follows from that.

---

## 1. The result

```
60 recording(s): 37 match, 0 mismatch, 23 partial, 0 error
```

| Verdict | Count | What it means |
|---|---|---|
| **MATCH** | 37 | the two paths agree at all four comparison depths, and the rebuild claimed no gap |
| **MISMATCH** | **0** | they disagree, with the rebuild claiming no gap |
| PARTIAL | 23 | the rebuild could not carry every offer back, so it says nothing either way |
| ERROR | 0 | |

**Not one comparable pair disagreed.** 533 order book levels across 37 pairs, each
compared at four depths, with every price matched as an exact rational and every
amount inside the 0.0000001 tolerance of section 4.

The deepest agreement is AIus at 153 levels, one bid and 152 asks. The most
important agreements are arguably the twelve pairs whose recorded book is EMPTY:
an asset with no executable price is this product's most interesting finding, and
a rebuild that invented a level on one of them would be the worst kind of error.
All twelve rebuilt empty.

## 2. What the four depths are

`10-validation.md` section 3 requires them reported separately, because aggregate
agreement hides a discrepancy in a single large offer.

| Depth | What is compared | How |
|---|---|---|
| 1 | the number of bid and ask levels | exactly, except on a side the recording truncated. See 2.1 |
| 2 | the price of each level | EXACTLY, as the rational. Section 4 of the protocol allows a tolerance only on derived decimals, and a price is not one |
| 3 | the amount of each level | within `Tolerance`, 0.0000001. The bid side has been through a division on both paths and cannot compare exactly |
| 4 | `ComputeAssetRisk` over both | `P0`, `spreadPct`, the whole depth ladder within tolerance, `priceSource`, the band, and the flag SET compared order-independently |

### 2.1 One thing the protocol does not mention and the data forced

`/order_book` serves at most 200 levels a side and does not say when it has
truncated. Five of the sixty recordings have a side at exactly 200, so the
recording is a PREFIX of the real book rather than the whole of it. Requiring the
same count on both sides would call every deep market a mismatch, so a capped side
is checked as "the rebuild has at least as many", the prefix is compared level by
level, and depth 4 is skipped on that pair: the engine reads the whole book, so
running it over a prefix on one side and the full book on the other measures the
truncation and not the reconstruction. Every such row says so in its explanation.

### 2.2 Pools are excluded from both sides

The recordings carry the pool response and the rewind reconstructs no pool.
Comparing risk with a pool on one side only would measure the missing pool rather
than the book, so depth 4 runs over book-only snapshots on both sides. What this
run validates is the BOOK path.

## 3. Which historical path this used, and why not the other one

There are two, and they are priced completely differently.

`keel replay` walks operations forward from nothing. It is the only route to a book
six months old, and `docs/evidences/2026-08-26-ustry-book-replayed-from-operations.md`
is it reproducing the golden fixture at the incident ledger. Its cost is set by how
busy the ACCOUNTS are and not by how busy the pair is: three of the QUIETEST pairs
in the demonstration set, at a target seven hours old, did not finish in ten
minutes. The bots that trade a sleepy pair are not sleepy.

`keel crosscheck` uses the rewind instead, and one field is why it works.
`/offers` reports every resting offer with its exact price, its amount and its
`last_modified_ledger`. An offer whose last modification is at or before the target
has not moved since, so what Horizon reports now IS its state then. No walk, no
decoding, no inference. Measured across these sixty pairs: **9,699 of the 10,265
offers resting on the live book had not moved in seven hours**, which is 94.5
percent of the work done by reading one field.

The whole run cost **753 Horizon requests**, an average of 12.6 per pair. The
forward walk did not finish three pairs in ten minutes.

## 4. Why 37 and not 60, and what that costs the SOW

The SOW promises cross-validation on at least 50 pairs. This run produced 37
comparable ones, so it does not meet that yet, and the reason is one number:
**seven hours passed between the recording and the comparison.**

In those seven hours, across the sixty pairs:

| | Offers |
|---|---|
| Carried back unchanged | 9,699 |
| Changed after the target, so left off the book and counted | 566 |
| Gone from the live book, named by a trade, counted and never put back | 2,565 |

A pair with even one of the last two is PARTIAL. It is not a disagreement; it is
the reconstruction declining to claim.

The distribution is what makes this fixable rather than fundamental. Five of the
twenty-three partials have five or fewer offers unaccounted for, and four of those
have exactly ONE: GOLD, SCROOGE, USDGLO and XTAR each failed to be comparable
because a single offer moved in seven hours. At the other end, XLM/USDC has 1,710
and yUSDC 390. Those pairs would have compared cleanly if the run had happened
minutes after the recording instead of hours.

**So the fix is operational and not a code gap: run the comparison right after the
recording.** `.github/workflows/record.yml` records every hour at seven minutes
past; a crosscheck step in the same job would compare each pair against a book
rebuilt seconds later, where nothing has moved. That would put the great majority
of the sixty into MATCH on every round and accumulate the sample the SOW asks for
rather than depending on one lucky window. Adding a step to that workflow is
GREEN, and it is the obvious next piece.

The counter-argument worth stating: a comparison made seconds after the recording
is a weaker test than one made hours later, because less has happened for the
reconstruction to have to see through. Both are worth having, which is an argument
for running the immediate one every hour AND keeping the committed recordings for
occasional deep checks, not for choosing between them.

### 4.1 AMENDMENT, 2 September 2026: the immediate run was tried and it stops at 49

Two batches were run on 31 August 2026 over the same sixty pairs, at a measured
delay of five minutes to six, an hour apart. Full measurement in
`docs/evidences/2026-08-31-samehour-crosscheck.md`.

| Run | Delay | MATCH | MISMATCH | PARTIAL |
|---|---|---|---|---|
| this document | ~7 hours | 37 | 0 | 23 |
| 31 August, batch 1, ledgers 64205568 to 64205574 | 5m00s to 5m59s | **49** | 0 | 11 |
| 31 August, batch 2, ledgers 64206210 to 64206216 | 5m00s to 6m00s | **48** | 0 | 12 |

**What section 4 got right.** The delay is a real cause and shortening it is a real
fix. The four pairs named above as failing on a single moved offer, GOLD, SCROOGE,
USDGLO and XTAR, are all MATCH in both batches. Twelve of the twenty-three convert.

**What section 4 got wrong, and it is the sentence naming two pairs.** "At the
other end, XLM/USDC has 1,710 and yUSDC 390 ... Those pairs would have compared
cleanly if the run had happened minutes after the recording instead of hours."
Neither did. XLM/USDC fell from 105 changed and 1,605 gone to 35 and 4, and stayed
PARTIAL in both batches. yUSDC fell from 42 and 348 to 7 and 0, and stayed PARTIAL
in both. Eleven pairs survive both five minute batches, and it is the same eleven
in each.

**Why, in one sentence.** The reconstruction walks live offers backwards, so an
offer that moved after the target ledger cannot be carried; that number falls with
the delay but does not reach zero on a pair whose offers move inside five minutes.
It tracks market activity and not only elapsed time, which is why the same eleven
names come back an hour later.

**What it costs.** 49 and 48 are both under the 50 comparable recordings that
`10-validation.md` section 3 requires. Criterion 3 does not close by crosschecking
faster, which is what section 4 above implied it would. That is handoff item B-8,
and the roads open to it are set out in section 6 of the 31 August document.

**Where else this correction is owed.** `docs/methodology/10-validation.md` around
line 271 states the seven hour gap "is the direct cause of all 23 PARTIAL rows".
That file is RED and the correction is Al's.

## 5. Why every gap here makes a book THINNER

This matters more than the count. The rewind can miss offers in three ways and all
three lose levels:

- an offer that moved after the target is left off, because its current state is
  not its state then;
- an offer that is gone from the live book is counted and never put back, because
  a trade cannot say whether it was on the book at the target or was created after
  it;
- an offer cancelled in the window that never traded is invisible and cannot even
  be counted.

Nothing is ever added that Horizon does not currently report as resting AND that
has not moved since the target. A book that is missing a level reads as a THINNER
market, and thin is the conservative side for a product whose job is to warn.

That property was not free. An earlier version of the rewind DID put departed
offers back, reconstructed from what they sold and the price the trade reports.
It is a real lower bound on what such an offer held, and it is still wrong,
because a trade cannot say whether the offer existed at the target. Measured
before the fix: AFR came back with 99 ask levels against a recording of 77. The
reconstruction was inventing levels. It was removed rather than bounded, and
`TestADepartedOfferIsCountedAndNeverPutBack` is what holds it out.

## 6. What this does not establish

- **It is not Layer 2.** Ten synthetic testnet scenarios are a separate item and
  none of them exists yet.
- **It says nothing about the AMM half.** Pools are excluded from both sides.
- **It says nothing about a book six months old.** Everything here turns on
  `last_modified_ledger` on a live offer, and an offer that stopped existing in
  February has no live record at all. `keel replay` is that route and its own
  evidence document is where its limits are.
- **The count of comparable pairs is bounded by market activity, not by the
  schedule.** Measured on 31 August 2026 and recorded in section 4.1. This document
  reads as though the delay were the only obstacle, and it is not the only one.
- **Zero mismatches is not proof that the two paths agree in general.** It is 37
  pairs at one moment. What would make it stronger is the hourly run in section 4,
  because a sample accumulated over weeks catches a disagreement that depends on
  market conditions, and one afternoon cannot.
