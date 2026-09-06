# Layer 3 over seventy-nine sample ledgers, and what "passed" means on each one

**Run on:** 6 September 2026, from public Horizon mainnet, no account required.
**Protocol:** `docs/methodology/10-validation.md` section 3.
**Raw results:** `docs/evidences/layer3-fifty-ledgers-2026-09-06.csv`, one row per
comparison, 360 rows, every row carrying its own ledger sequence, its four comparison
verdicts, the reconstruction's completeness counters and the delay that produced it.
**MethodologyVersion:** `1.0.8-draft` on every row.

**What it is evidence for, and the wording matters.** The SOW's Week 1 expected output
reads "Cross-validation passed on at least 50 sample **ledgers**", and its Deliverable 1
evidence line reads "cross-validation results **against Horizon** on sample ledgers".
Both were already satisfied in kind by the runs of 26 and 31 August and neither was
satisfied in count: those three runs are 180 rows across **seven distinct ledgers each
and twenty-one in total**, 64129586 to 64129592, 64205568 to 64205574 and 64206210 to
64206216, because sixty pairs recorded in one sweep of thirty-five seconds all land
inside the same handful of ledgers. Sixty comparisons on seven ledgers is not fifty
ledgers, and reading it as though it were is
the same substitution of one word for another that DEC-002 section 8.2 records costing
three plans.

**Reproduce it:**

```bash
go run ./cmd/keel record \
  -pairs configs/recorder-pairs.json \
  -out recordings/ledgercount-2026-09-06 \
  -interval 2m \
  -crosscheck -crosscheck-after 5m \
  -crosscheck-out <path>
```

It will not reproduce these numbers, for the reason both earlier documents give about
themselves: the market moves. What is reproducible is the shape.

---

## 1. What the run was, and why it is shaped this way

| Property | Value |
|---|---|
| Pairs | the eight in `configs/recorder-pairs.json`, unchanged |
| Rounds | 45, one every 2 minutes |
| Recorded | 2026-09-06T14:37:28Z to 16:05:31Z |
| Rebuilt | 2026-09-06T14:42:28Z to 16:10:46Z |
| Delay | 300.001 s to 333.074 s, one arm, five minutes |
| Ledgers | **79 distinct**, 64302069 to 64303016 |
| Comparisons | 360 |
| Horizon requests | 2657 in the last hourly window, against a 3000 budget |

**The pair list was NOT chosen today.** `configs/recorder-pairs.json` was compiled on 25
August across four liquidity buckets, before any of these four runs existed, and it is
used here unedited. Choosing pairs today from the earlier runs' results would produce a
higher pass rate and no evidence: the selection would carry the answer.

**Distinct ledgers come from time spread, not from pair count**, and that is the whole
design change from the earlier runs. Eight pairs recorded back to back land on ONE
ledger, measured: a two pair round at 14:35 put both files on 64302050. So the ledger
count is the round count, and the round count is what was raised.

---

## 2. Results

| Verdict | Count | Meaning |
|---|---|---|
| MATCH | 96 | agreement at all four comparison depths, and the rebuild claimed no gap |
| MISMATCH | **0** | disagreement, with the rebuild claiming no gap |
| PARTIAL | 264 | the rebuild could not carry every offer back, so it says nothing either way |
| ERROR | 0 | |
| **Total** | **360** | over **79** distinct ledgers |

Against the SOW's count, read both ways:

| Reading | Result |
|---|---|
| Sample ledgers cross-validated | **79**, against the 50 required |
| Ledgers carrying at least one fully comparable pair that agreed at every depth | **51** |
| Ledgers carrying a disagreement the reconstruction did not predict | **0** |

Per comparison depth, over all 360 rows:

| Depth | Agreed | Disagreed |
|---|---|---|
| 1, level counts | 151 | 209 |
| 2, level prices | 97 | 263 |
| 3, level amounts | **358** | **2** |
| 4, `ComputeAssetRisk` | 187 | 173 |

---

## 3. THE PASSING LEDGERS ARE CARRIED BY TWO PAIRS, AND THAT IS THE LIMIT OF THIS RUN

| Pair | MATCH | of |
|---|---|---|
| BRL `GDVKY2GU` | 45 | 45 |
| ARST `GCSAZVWX` | 42 | 45 |
| AUDD `GDC7X2MX` | 7 | 45 |
| USTRY `GCRYUGD5` | 2 | 45 |
| XLM native | 0 | 45 |
| EURC `GDHU6WRG` | 0 | 45 |
| PYUSD `GDQE7IXJ` | 0 | 45 |
| AQUA `GBNZILST` | 0 | 45 |

**So "51 ledgers passed" is true and it is not the whole sentence.** On most of those 51
the pair that passed was BRL or ARST, and four of the eight pairs did not produce a
single comparable row in ninety minutes. A reader who takes 51 as "the historical path
was validated on 51 ledgers across the liquidity range" is reading something this run
does not say.

What separates them is not size and section 5 of the 31 August document already measured
that: it is how often offers on the pair move. A pair whose offers change inside five
minutes cannot be walked back by a path that carries the CURRENT offer set to a past
ledger, so it is counted in `offers_changed_after_target` and the row is not comparable.
BRL and ARST are quiet. XLM/USDC, EURC, PYUSD and AQUA are not.

**The honest form of the claim is therefore this one:** the historical path was compared
against recorded Horizon on 79 sample ledgers and 360 pair-comparisons; it agreed
everywhere it claimed to be complete, on 51 of those ledgers; and on the pairs whose
offers move within five minutes it declines to answer rather than answering wrongly.

---

## 4. The self-report finding reproduces on a fourth run, and this is the strongest half

The 31 August document's section 3 found that the set of rows marked PARTIAL and the set
of rows that actually disagree are the same set, over 180 rows in three runs. Checked
again here, over 360 more:

| | Rows | Disagreeing at some depth |
|---|---|---|
| The rebuild claimed NO gap | 96 | **0** |
| The rebuild claimed a gap | 264 | 264, none agreed at every depth |

**Nothing on either side of the difference, again, and now over 540 rows in four runs.**
When the reconstruction says it carried every offer, it is right: four comparison depths,
prices compared as exact rationals, and not one disagreement in 96 rows. When it says it
could not, the two books do differ. The self-reported completeness counter is a correct
predictor of agreement in both directions, which is a stronger property than a pass rate.

---

## 5. One thing here has never happened before

**Amounts disagreed on two rows**, PYUSD at ledger 64302198 and AUDD at 64302327. Every
previous run reported amounts agreeing on every row, in all three runs, and the 31 August
document states that in those words: "Amounts never disagreed in any run."

Both rows are PARTIAL, so neither is a MISMATCH and neither contradicts section 4. It is
recorded here because it is the first occurrence and because a property that held for 180
rows and then stopped is worth a note before somebody quotes the old sentence. Two rows
in 360 is not a pattern; it is a thing to watch, and the raw CSV carries both rows with
their counters.

---

## 6. What this closes and what it does not

**Closes, against the SOW:** the count. Week 1's expected output asks for cross-validation
passed on at least 50 sample ledgers and the answer is 79 compared, 51 carrying a fully
comparable agreeing pair, 0 unpredicted disagreements. The Deliverable 1 evidence line
asks for cross-validation results against Horizon on sample ledgers, and the CSV beside
this file is that table.

**Does NOT close, against the PRD:** section 9's third criterion reads "Horizon versus
**Hubble** cross-validation on at least 50 pairs". This is Horizon against Horizon
rebuilt from Horizon, so it is not a second reader, and `internal/hubble/` still holds one
`CLAUDE.md` and no Go. FR-13 and FR-14 are still two MUSTs at zero. DEC-002 section 8.3
lists the three readings of that gap and the SOW, read on 6 September 2026, supports the
first of them: it never names Hubble or BigQuery, it names Horizon three times, and its
out-of-scope list says "the proof of concept uses public Horizon and RPC endpoints".
Changing the PRD's wording on that basis is a client-facing amendment and is Al's, not a
rescoring.

**Does NOT close Layer 1 or Layer 2.** `testdata/manual/` holds 0 of 5 hand
recomputations and `testdata/fixtures/layer2/` holds 0 of 10 testnet fixtures. Neither is
named in the SOW and both are the reason to believe `compute.go` is checked against
numbers that did not come from it.

---

## 7. Version history

| Date | Change |
|---|---|
| 6 September 2026 | Created. First Layer 3 run designed around the ledger count rather than the pair count |
