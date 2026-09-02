# Layer 3 at a five minute delay: two batches, and what actually decides PARTIAL

**Run on:** 31 August 2026, from Horizon mainnet, no account required.
**Protocol:** `docs/methodology/10-validation.md` section 3.
**Pairs:** the same sixty as the 26 August run, so the three runs join row by row.
**Raw results:**
`docs/evidences/layer3-samehour-2026-08-31T071059Z.csv` and
`docs/evidences/layer3-samehour-2026-08-31T081106Z.csv`, one row per recording,
each carrying the delay that produced it.

| Run | Ledgers | Recorded at | Rebuilt at | Delay |
|---|---|---|---|---|
| 26 August | 64129586 to 64129592 | 2026-08-26T07:09:21Z | ledger 64134034 and after | about 7 hours |
| 31 August, batch 1 | 64205568 to 64205574 | 07:10:23Z to 07:10:59Z | 07:15:23Z to 07:16:58Z | 300.006 s to 359.599 s |
| 31 August, batch 2 | 64206210 to 64206216 | 08:11:34Z to 08:12:05Z | 08:16:34Z to 08:18:04Z | 300.005 s to 360.150 s |

`MethodologyVersion` is `1.0.3-draft` on every row of both batches.

**Reproduce it:**

```bash
go run ./cmd/keel record -crosscheck -crosscheck-after 5m -crosscheck-out <path>
```

It will not reproduce these numbers, for the reason the 26 August document already
gives about itself: the market moves. What is reproducible is the shape of the
result, and section 3 is the part that held across two independent batches.

---

## 1. The result

| Run | Delay | MATCH | MISMATCH | PARTIAL | ERROR | PARTIAL rate |
|---|---|---|---|---|---|---|
| 26 August | ~7 hours | 37 | **0** | 23 | 0 | 38.3 % |
| 31 August, batch 1 | 5m00s to 5m59s | 49 | **0** | 11 | 0 | 18.3 % |
| 31 August, batch 2 | 5m00s to 6m00s | 48 | **0** | 12 | 0 | 20.0 % |

**Zero MISMATCH, three times.** 134 comparable rows across the three runs and not
one of them disagreed. That is the Layer 3 result and it is the same result the
26 August run reported, now with two more samples behind it.

The totals are the least useful part of this table. The sixty pairs are identical
across the three runs, so the verdicts join pair by pair, and that is where the
answer is.

---

## 2. The pair by pair join

| 7 hours | batch 1 | batch 2 | n | Reading |
|---|---|---|---|---|
| MATCH | MATCH | MATCH | 37 | never in question |
| PARTIAL | MATCH | MATCH | **11** | the delay WAS the cause |
| PARTIAL | MATCH | PARTIAL | 1 | AUDD, on the boundary |
| PARTIAL | PARTIAL | PARTIAL | **11** | the delay was NOT the cause |
| MATCH | PARTIAL | anything | **0** | no regressions at all |

**Twelve of the twenty-three are explained by the delay and eleven are not.** The
eleven survive two independent five minute batches taken an hour apart.

**Zero regressions is a result too.** Nothing that matched at seven hours went
PARTIAL at five minutes, in either batch. The delay moves the verdict in one
direction only, which is what makes the three runs comparable at all.

**The eleven**, with the counters that decide their verdict, from batch 1:

| Asset | Issuer | Recorded bid/ask | Rebuilt bid/ask | carried | changed | gone |
|---|---|---|---|---|---|---|
| AQUA | `GBNZILST` | 49 / **200** | 49 / 302 | 478 | 3 | 0 |
| BTC | `GDPJALI4` | **200** / 32 | 406 / 29 | 1201 | 4 | 0 |
| EURC | `GAQRF3UG` | 13 / 9 | 10 / 7 | 20 | 5 | 0 |
| EURC | `GDHU6WRG` | 72 / 85 | 52 / 65 | 130 | 41 | 0 |
| USDZ | `GAKTLPC4` | 17 / 21 | 17 / 20 | 42 | 1 | 0 |
| VELO | `GDM4RQUQ` | 86 / **200** | 83 / 210 | 361 | 5 | 1 |
| XLM | native | **200** / **200** | 677 / 1136 | 4297 | 35 | 4 |
| XRP | `GBXRPL45` | 185 / 120 | 178 / 114 | 619 | 13 | 0 |
| sUSD | `GCHW7CWI` | 15 / 43 | 14 / 36 | 56 | 8 | 0 |
| yUSDC | `GDGTVWSM` | 44 / 22 | 40 / 19 | 81 | 7 | 0 |
| yXLM | `GARDNV3Q` | 56 / 51 | 52 / 49 | 108 | 6 | 0 |

Two of the eleven are EURC under different issuers, which is the reason an asset is
the pair (code, issuer) and is never matched on the ticker. Every quote is USDC
`GA5ZSEJY`.

---

## 3. The finding: the self reported gap and the real disagreement are one set

A row is marked PARTIAL by `crosscheckRow.verdict()` when the reconstruction says
it could not carry every offer back, which is `Changed > 0 || Gone > 0 || Err != ""`.
That decision is made **before** the two books are compared, and the legend the tool
prints says a PARTIAL row "says nothing either way".

Checked against the comparison result on every row of all three runs:

| Run | rows disagreeing at some depth | rows marked PARTIAL | in both | disagreeing but marked MATCH |
|---|---|---|---|---|
| 26 August | 23 | 23 | 23 | **0** |
| 31 August, batch 1 | 11 | 11 | 11 | **0** |
| 31 August, batch 2 | 12 | 12 | 12 | **0** |

**The two sets are identical, in all three runs, with nothing on either side of the
difference.** That is 180 rows.

It cuts both ways and both halves are worth stating.

**The good half, and it is the stronger Layer 3 claim.** When the rebuild claims it
carried every offer, it is right. 134 comparable rows, four comparison depths each,
prices matched as exact rationals, and not one disagreement. The reconstruction is
not merely passing where it is checked; it self reports its own reliability
accurately.

**The bad half is about the tool's own wording.** "Says nothing either way" is too
generous. On every PARTIAL row the two books actually differ, at depth 1, 2 or 4.
The self reported gap is not a claim of ignorance, it is a correct prediction of a
disagreement. The legend in `cmd/keel/crosscheck.go` line 555 should say so.

Per depth, batch 1, over the eleven: prices disagree on 11, level counts on 9,
the risk output on 7, amounts on none. Amounts never disagreed in any run.

---

## 4. It retracts finding 8 of the 28 August breakdown

That finding read, in full: "The 23 PARTIAL rows in Layer 3 are an artifact of the
seven hour gap between recording and rebuild, not a property of the historical path.
The next recorder batch should be crosschecked the same hour."

**It is about half right.** Twelve rows, and the sentence claimed twenty-three. The
same claim sits in `docs/methodology/10-validation.md` around line 271, which reads
"it is the direct cause of all 23 PARTIAL rows". That file is RED and its correction
is Al's; this document is the measurement it should cite.

The retraction is not that shortening the delay does nothing. It does a great deal:
38.3 per cent to 18.3 and 20.0. The retraction is that the first explanation was
treated as sufficient and the residue was never looked for.

---

## 5. The pagination hypothesis, tested, and it is not the second cause

The 31 August breakdown proposed that the residue was truncation: `/order_book`
returns at most 200 levels a side without saying so, and three rows were noted as
having a recorded side at that limit.

**Four of the eleven are at the limit**, not three: AQUA on the ask, BTC on the bid,
VELO on the ask, and XLM/USDC on both. The count in the breakdown came from the
explanation strings, and the explanation only names truncation when the first
failure it reports is a level count. AQUA and XLM/USDC report a price first, so
their truncation is in the data and not in the sentence.

**But truncation is already compensated, and that is why it cannot be the cause.**
`countAgrees` in `cmd/keel/crosscheck.go` accepts `rebuilt >= recorded` on a capped
side, and depth 4 is skipped entirely on a capped recording so the engine is never
run over a prefix. On BTC the capped bid side passes under that rule and the
failure is on the uncapped ask, 32 recorded against 29 rebuilt. VELO is the same
shape on the other side.

More decisively: not one of the eleven is PARTIAL because of a level count at all.
All eleven are PARTIAL because `changed > 0`, and section 3 shows that is also
exactly when the books disagree.

**What the second cause actually is.** The reconstruction reads the offers that
exist now and walks them backwards to the target ledger. An offer that changed
after the target ledger cannot be walked back with what that path has, so it is
counted in `changed` and the row is not comparable. That number falls as the delay
falls, because fewer offers move in five minutes than in seven hours, but it does
not reach zero on a pair where offers move within five minutes. It is a property of
market activity, not only of elapsed time, which is why two batches an hour apart
name the same eleven pairs. EURC `GDHU6WRG` had 41 of 130 carried offers change in
five minutes, which is 32 per cent, while XLM/USDC had 35 of 4297, which is 0.8.
Size is not what separates them.

---

## 6. What this costs criterion 3

`10-validation.md` section 3 asks for at least 50 comparable recordings. Seven
hours gave 37. Five minutes gave 49 and 48.

**Shortening the delay does not bring Layer 3 to its own threshold, and now the
reason has a name.** The eleven are what holds the count under 50, and they are the
pairs whose offers move faster than the instrument can rebuild them. Reaching 50 on
this path requires a delay short enough that no offer moves on the eleven most
active pairs of the set, which is not a delay that can be scheduled.

That is handoff item B-8 and none of its three roads is Claude's to pick. What this
document adds to it is that road 1, find and fix the second cause, is now a smaller
road than it looked: the cause is located and it is structural to the current-state
reconstruction rather than a defect in it. Road 2 is Hubble, which reads the ledger
as it was and does not walk anything backwards.

**Road 3 is the one to be careful about.** Lowering the threshold because the work
did not clear it is how a validation protocol stops meaning anything, and this
repository has a written rule against the same move in `internal/conformance`:
adjust the code to match the numbers, never the numbers to match the code.

---

## 7. What this does not establish

**Two batches are two samples.** 18.3 and 20.0 per cent agree closely enough to be
worth stating together, but the rates are the weaker evidence. The pair by pair join
is the strong part, because eleven of the same eleven pairs failed in both.

**It cannot prove which constants a run used.** `defaultCrosscheckDelay` and
`maxCrosscheckDelay` exist in the code and finding P2-22 says so, but a check can
prove a constant exists and never that a run honoured it. What these files carry
instead is the measured delay on every row, from the records rather than from a
constant, which is why the delay column is in the CSV at all.

**One row raises a question this document does not answer.** AQUA's first reported
failure in batch 1 is a price: recorded `647483/1890461314`, rebuilt `137/400000`.
Those are different rationals whose values differ by 2.4e-14, which is far inside
the 0.0000001 tolerance of section 4, and section 4 requires prices expressed as
rationals to be compared **exactly**. The comparison is doing what the protocol says.
It changes no verdict here, because AQUA is already not comparable. It would matter
on the day comparability is fixed, and whether an aggregated book level price and an
offer price should be required to be the same rational is a methodology question.
The explanation column names only the first failure on a row, so this is one
observed instance and not a census.

**It says nothing about the recordings under `recordings/samples/`.** Those sixty
are the committed evidence and were recorded on 26 August. These two batches took
fresh recordings of the same sixty pairs.
