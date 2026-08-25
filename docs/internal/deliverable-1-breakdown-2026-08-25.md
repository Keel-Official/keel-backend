# Deliverable 1: what is missing, and who can do it

**Date:** 25 August 2026, day 6 of the 30 day sprint
**Scope:** Deliverable 1 only, the 126 hour Liquidity Depth Engine
**Companion to:** `handoff-2026-08-21.md`, which lists what is blocked across the whole
project. This document is narrower and answers one question: what stands between the
repository as it is today and the Deliverable 1 Definition of Done in
`Keel_Deliverable_1_Rencana_Eksekusi.md` section 6.

**Check every claim here with:**

```bash
bash scripts/audit-verification.sh          # 12 proven, 41 not, as of this document
go test ./...                               # 105 tests, 6 packages, green
make conformance                            # red on purpose, compute.go panics
grep -c '^check ' scripts/audit-verification.sh
for f in docs/methodology/*.md; do sed -n '1,8p' "$f" | grep -H '^\*\*Status'; done
```

---

## 1. How this document was produced

The SOW was read from `docs/context/Keel_SoW.pdf`, which required extracting the text
from the PDF, because no page renderer is installed. Its three deliverables and their
hour budgets are quoted rather than remembered.

Every status claim below was read out of the repository at commit `aece0ee` and is
reproducible with one of the commands above. Where a document and the repository
disagree, the disagreement is recorded as a finding rather than smoothed over.

---

## 2. What is missing from Deliverable 1

| Area | Promised | What exists | What is missing | Owner |
|---|---|---|---|---|
| D-1 pair selection | a written decision before coding | `02-pair-selection.md` exists | **WORKSHEET.** Five sections still read `_to be written_`: global versus per asset quote, the primary pair rule, the backtest pair, path payments, and the 50 asset criteria | Al, RED |
| D-4, D-5, D-6 | three written definitions | `07-supporting-metrics.md` exists, specimens A, B and C already collected | **WORKSHEET.** The genuine trade rule, the holder population, and the supply denominator are all unmade | Al, RED |
| D1.2 depth core | a pure function, 22 hours | types, fixture, arch tests, conformance harness | **five functions that `panic`** in `compute.go` | Al, RED |
| The with-pool fixture | hand computed numbers before any code | the Go fixture carries `PoolUSTRYUSDC` | the hand document still records `Pools: []` at line 30, and the with-pool depth and manipulation tables are not computed | Al, RED |
| D1.3 supporting metrics | 20 hours | `holders.go` reads trustlines, 15 tests | no formula at all. `HolderHHI`, `VolumeToSupplyD1/7/30` and `LastGenuineTrade` are empty, and their definitions do not exist yet | Al then Claude |
| D1.4 safe collateral | 12 hours | the formula in `08-collateral.md` is complete | `ComputeMaxSafeCollateral` panics, and the `h` and `m` defaults have no named source | Al |
| D1.5 historical replay | 20 hours | nothing. `keel replay` reports "not built yet" | all of it. DEC-002 defers Hubble | Al decides the scope |
| D1.6 Layer 1 | a spreadsheet for 5 assets | the protocol is defined | `testdata/manual/` does not exist. The results table is empty | Al |
| D1.6 Layer 2 | 10 testnet fixtures | 10 scenarios are defined | zero fixtures. The one that exists is USTRY, and that is Layer 1 | Al for the numbers, Claude for the harness |
| D1.6 Layer 3 | 50+ ledgers cross-validated | the recorder works. 4 files, 1 pair, all from 24 August | ground truth. And Layer 3 as defined needs Hubble, which DEC-002 defers, so the layer has no route at all right now | Al |
| D1.7 methodology | eleven complete files | 7 complete, and `11-limitations.md` is strong | `02` and `07` are worksheets, `06` is partial, `10` has empty result tables, and `09` points at `internal/domain/flags.go`, which does not exist | Al plus Claude |
| The demonstration set | at least 50 assets | `candidate-survey.sh` is built | one pair is declared. The inclusion criteria do not exist either; they are `02` section 5 | Al for criteria, Claude for the list |
| A public repository | the repo link is Deliverable 1 evidence | the README covers running from nothing | it is still PRIVATE. DEC-004 holds it until `make conformance` passes with no build tag | Al |
| Contract and mocks | clean artifacts | the contract is valid, the mocks match | `keel-openapi.yaml:123` and `mocks/methodology.json:5` still say "May 2026". DEC-001 corrected the incident to February | Claude |

### 2.1 Four findings that no document tracked before today

1. **"May 2026" is still live in the contract and the generated mocks.** DEC-001 section 1
   corrects the incident date to 22 February 2026 and its next action 3 asks for every
   reference to be changed. `docs/api/Keel_PRD.md` even documents, at lines 21 to 23,
   that this was not done, and then stays wrong at FR-26, line 322 and Q1.
2. **`09-flags-and-bands.md:5` points at `internal/domain/flags.go`, which does not
   exist.** `internal/domain` holds `CLAUDE.md`, `arch_test.go`, `compute.go` and
   `types.go`. One reference, nothing else in the repository names that path. This is the
   `internal/depth` pattern that `CLAUDE.md` records, and this time the document doing
   the pointing is the paid deliverable.
3. **`10-validation.md:112` names `recordings/samples/` while `.gitignore:41` re-includes
   `recordings/sample/`.** Evidence placed at the path the methodology names would be
   ignored by git with no warning.
4. **`GoldenSnapshot()` has no numeric expectations.** `ExpectedDepth` and
   `ExpectedManipulation` pair with `BookOnlySnapshot()`, and the header of `expected.go`
   mislabels them as expectations for `GoldenSnapshot`. The only test that touches the
   with-pool path, `TestInvarianMonotonisitas`, checks ordering and not values. So the
   half of the fixture that represents the real market state is unverified by number.

### 2.2 BigQuery has never been touched

Recorded here because D1.5 depends on it and because the question is easy to answer
wrongly from the documents alone, which mention Hubble in many places.

| Evidence | Result |
|---|---|
| `go.mod` | two direct dependencies, `pgx/v5` and `shopspring/decimal`. No BigQuery client, and no Stellar SDK either |
| `internal/hubble` | the directory does not exist |
| `keel replay` | a stub that prints "not built yet" and names DEC-002 |
| `docs/evidences/` | every file is Horizon output. The DEC-002 spike of 21 August queried `/trades`, not BigQuery |
| `DataSourceHubble` | an enum value in `types.go` and nothing more |

Every number this repository has ever produced came from Horizon.

---

## 3. Work that can be executed

| ID | Work | Executor | Prerequisite | Output |
|---|---|---|---|---|
| C-1 | Start the recorder on 8 provisional assets today, replace the list later if Al's criteria differ | Claude, Al approves the provisional list | none | ground truth starts accumulating. The only item here whose cost rises every day it waits |
| C-2 | Build the candidate asset list from `make survey`, grouped into the four liquidity buckets, presented as a choice | Claude | none | Al picks rather than searches |
| C-3 | Change "May 2026" to "February 2026" in the contract, run `make api-mocks`, and fix the PRD | Claude | none | the contract and the mocks stop quoting the wrong incident |
| C-4 | Reconcile `recordings/samples/` in `10-validation.md` with `recordings/sample/` in `.gitignore` and the README | Claude | none | evidence stops being silently ignored by git |
| C-5 | Fix the `09-flags-and-bands.md:5` pointer to a path that exists | Claude drafts, Al confirms in one word | Al decides where the flag rules will live | the paid deliverable stops pointing at empty space |
| C-6 | Update the handoff: the counts are 12/41, item 14 is closed, item 3 still has an open remainder, and two new items are needed for findings 2 and 4 above | Claude | none | the document and the repository stop drifting |
| C-7 | Add four checks to `scripts/audit-verification.sh`: a with-pool ladder with no expected values, "May 2026" in the contract, the `recordings/sample` path, and every path named by an `Implemented in:` line must exist | Claude | none | today's four findings become tracked instead of rediscovered |
| C-8 | Pop `stash@{0}`, commit the one line `.gitignore` change, delete the merged local branch | Claude | none | housekeeping |
| C-9 | Prepare the Layer 1 spreadsheet skeleton and the Layer 2 harness for 10 scenarios, with no numbers in either | Claude | none | Al fills in numbers rather than building containers |
| A-1 | Fill in `02-pair-selection.md` | Al | none | makes C-2 final, and unblocks D1.3 |
| A-2 | Fill in `07-supporting-metrics.md` | Al | none | unblocks the half of `compute.go` that is locked today |
| A-3 | Put the pool into the fixture document, then hand compute the with-pool tables | Al | none | gives `compute.go` a complete opposing specification |
| A-4 | Write `compute.go`, the depth and manipulation half first | Al | A-3 | unblocks C-10 through C-13 |
| A-5 | Chase the Reflector VWAP window, then the Blend parameters | Al | another party answering | `06` moves from partial to complete, and `C_max` gains a named source |
| C-10 | Delete the `conformance` build tag and move the test into `make test` | Claude | A-4 | DEC-004 gets its trigger |
| C-11 | Run `scan` across 50 assets and fill in the results | Claude | A-4, A-1 | the DoD box for 50+ assets stored |
| C-12 | Compare recordings against engine output, fill in the Layer 3 table | Claude | A-4, C-1, the Hubble decision | the cross-validation evidence |
| C-13 | Write the supporting metric half outside the red zone, if Al places it there | Claude | A-2 and the zone decision | D1.3 complete |

---

## 4. Blocked for Claude, and what Al has to do

| ID | Blocked | Why Claude cannot | What to do |
|---|---|---|---|
| B-1 | `02-pair-selection.md` | RED, and this is a definition that is sold, not typing. Claude writing it means Claude inventing the methodology and then the code confirming Claude's invention | Answer the five `_to be written_` blocks. The decisive ones: **one**, a single global quote asset or one per asset, and if per asset then the rule, because the bands in `09` are absolute values in the quote asset and assets on different quotes are not comparable, which is open question Q7. **Two**, every pair with liquidity or only the primary, remembering that an attacker uses the cheapest route. **Three**, the backtest pair must be USDC because the oracle read USDC, and the reason has to be written rather than assumed. **Four**, path payments as a stated limitation or a partial approximation, and in which direction the bias runs. **Five**, the 50 asset criteria written BEFORE the list is built, which section 6's own checklist requires |
| B-2 | `07-supporting-metrics.md` | RED. And without it, half of `compute.go` cannot be written by anyone without inventing definitions | Three definitions. **Genuine trade:** the criteria table in section 1 has an empty Decision column; accept, reject or defer each of the five with a reason. Specimen B is the hard one, two different accounts matched against the seller's own offer, and the rule must either catch it or state what not catching it costs. **Holders:** which accounts leave the population, whether pool reserves count in the denominator, and this must agree with the supply definition in section 3. **Supply:** pick one of the three, and state whether volume is filtered by the genuine trade rule |
| B-3 | `internal/domain/compute.go` | denied in `.claude/settings.json` and closed in `lindungi-zona-merah.sh`, and it is the core of the product. Claude is reviewer here, not author | The depth and manipulation half is already unblocked: its definitions are complete in `03`, `04`, `05` and `08`. Start there and do not wait for B-1 or B-2. The supporting metric half can only follow B-2 |
| B-4 | The fixture document and the with-pool numbers | `testdata/fixtures/` has been RED and hook-locked since 25 August. Numbers Claude produces must not be the numbers that test Claude's code | Change `Pools: []` at line 30 to the pool that actually existed. Then hand compute the with-pool depth and manipulation tables. Compare your result against DEC-006 section 2: agreement is a free confirmation, disagreement is a second finding, and copying is neither |
| B-5 | Numeric expectations for `GoldenSnapshot()` | same as B-4 | This follows from B-4, and the size of the gap is worth knowing: the only test touching the with-pool path checks ordering, not values, so the half of the fixture representing the real market is unverified by number |
| B-6 | The Reflector VWAP window length | a statement by another party, not data on the ledger | Find Reflector's own documentation or statement. Note carefully that Script3's "no other trade in 15 minutes" is a different claim and is not evidence that the window is 15 minutes. Until it is confirmed, `oracleWindowSeconds` in the contract rests on an assumption |
| B-7 | Blend or YieldBlox risk parameters | same | Take them from the Blend V2 pool configuration. The DoD requires the source to be named, so "our defaults come from the Blend parameters in force in February 2026" is far stronger than a number chosen in house |
| B-8 | The Hubble and Layer 3 decision | this is a reversal. Claude may draft and amend a decision record and may not create or reverse one | Two options, pick one this week. **One**, redefine Layer 3 so it does not need Hubble, and correct `10-validation.md` to follow. **Two**, put Hubble back on the critical path and pay its 3 to 5 days by cutting Deliverable 3 scope. What cannot be chosen is leaving `10-validation.md` promising Layer 3 while DEC-002 removes the material it needs |
| B-9 | Repository visibility | DEC-004, and Deliverable 1 evidence asks for a public repository link | The trigger is `make conformance` passing with no build tag, so this follows B-3 mechanically, but the decision to open it stays yours |
| B-10 | Loosening the red zone hook | Claude may tighten `.claude/` and may not loosen it, and the harness enforces that rather than trusting it | Two small things. **One**, `\bpython3?\b` is in the mutating verb list, so a read-only reader of `docs/context` is refused even though the zone map grants Claude that read. **Two**, if you fix it, follow the P2-6d pattern: Claude drafts the patch, you apply it, Claude probes it BEFORE the commit, and the loosening gets a check of its own in `scripts/audit-verification.sh` |
| B-11 | Where the supporting metric formulas may live | only `compute.go` is RED, not all of `internal/domain`. Writing HHI into a new domain file would pass beside the lock without breaking a single rule | Decide in one sentence: the supporting metric formulas go into `compute.go` as well, or into a new file that is locked with it. This is a hole in the map rather than a permission, and left alone it is the pattern `CLAUDE.md` already records being defeated by five times |

---

## 5. Suggested order

1. **B-3, the depth half, today.** It waits on nothing. Its definitions are complete.
2. **C-1, today as well.** A ledger that has passed cannot be recorded afterwards. Every
   other item on this list can be done later at the same cost. This one cannot.
3. **B-4**, so the fixture and the code finally test each other.
4. **B-2 before the end of the week**, because it holds the remaining half of the engine.
5. C-3 through C-8 run in parallel throughout and touch nothing Al is working on.

## 6. Version history

| Date | Change |
|---|---|
| 25 August 2026 | Created. Deliverable 1 gap analysis at commit `aece0ee`, day 6 of 30 |
