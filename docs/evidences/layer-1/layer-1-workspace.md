# Layer 1 workspace: the procedure for the five hand recomputations

**THIS FILE CONTAINS NO COMPUTED VALUE OF ANY KIND.** It is the order of work, the
traps, and the definition of done for Layer 1. No mid price, no spread, no depth, no
target, no cost. The only numbers below are level counts read off Horizon at ledger
`64310828`, already published in `README.md` section 3 of this directory, and the
tolerance and sample size quoted from the protocol. That restraint is what lets this
file exist: the recomputations it describes are the independent oracle for
`internal/domain/compute.go`, and a procedure that arrived carrying answers would
defeat the layer it serves.

**Zone:** `docs/evidences/` (YELLOW).

**Written:** 2026-09-08, day 21 of 30. **Deadline:** 18 September 2026.

**Where this file sits, and which document wins.**

| Document | What it holds | Tracked |
|---|---|---|
| `docs/methodology/10-validation.md` section 1 | the protocol. **It wins over this file wherever they differ** | yes |
| `testdata/manual/README.md` | the required shape of one recomputation, and the red zone rule | see step 0 |
| `docs/evidences/layer-1/README.md` | the eight candidates measured, the five proposed, why two were excluded | yes |
| the six input files beside it | the raw books, verbatim, one per candidate | yes |
| `docs/internal/layer-1-worksheet-template.md` | the empty worksheet to fill in | **no**, gitignored under DEC-004 |
| **this file** | the long form of `README.md` section 5 | yes |

The worksheet template is local, so a clone cannot open it. That is why this file
carries the procedure in the repository rather than leaving it in a directory only one
machine has.

**Nothing here defines a quantity.** Every formula is owned by a file in
`docs/methodology/` and is cited rather than restated, because a second home for a
definition drifts.

---

## 0. Prerequisite: the README the red directory needs

**Half done as of 8 September 2026.** The draft preamble has been stripped, so
`testdata/manual/README.md` now opens on `# Layer 1: hand recomputation` rather than on
"This file is a DRAFT and is not the thing itself".

**What is left is the commit.** `git status` still reports `?? testdata/manual/`, so the
file is on one disk and in no clone. PRD section 9 asks for "the spreadsheet present in
the repository", and the Deliverable 1 evidence line in the SOW is a repository link.
Untracked is not present.

```bash
cd /Users/yazidalghozali/Development/Stellar-Funding/keel
head -3 testdata/manual/README.md      # must open on "# Layer 1: hand recomputation"
git add testdata/manual/README.md
git commit -m "docs(layer1): the README the red directory needs, not its draft"
```

Al runs this. The directory is RED in both the deny list and the hook.

---

## 1. The five assets, and three choices to make before starting

`README.md` section 3 proposed these from `configs/recorder-pairs.json`, which was
compiled on 25 August 2026 across four liquidity buckets and used here unedited.
Choosing pairs today, with four Layer 3 runs already visible, would let the selection
carry the answer.

| Order of work | Asset | Levels to transcribe | What it is for |
|---|---|---|---|
| 1 | AUDD `GDC7X2MX` | 16 | the thin end, 11 bids and 5 asks. Start here |
| 2 | USTRY `GCRYUGD5` | 26 | the incident asset in a NORMAL state, the control on the golden fixture |
| 3 | BRL `GDVKY2GU` | 40 | 35 bids against 5 asks, the most asymmetric book here, so it presses hardest on the buy versus sell split |
| 4 | PYUSD `GDQE7IXJ` | 42 | the middle of the range, and a `credit_alphanum12` code, so the asset type is not inferred from code length |
| 5 | EURC `GDHU6WRG` | 157 | the deep end. This is the heavy one |

All counts are at ledger `64310828`, read 2026-09-07. **The order is ascending by cost
on purpose:** a method error found on a 16 level book costs less than the same error
found on a 157 level one.

**Choice 1: EURC or ARST.** ARST `GCSAZVWX` is the spare, 29 levels. Swapping saves
perhaps three hours. The cost is already stated in `README.md` section 2: the deepest
end of the liquidity range goes unrepresented, because XLM and AQUA were excluded for
hitting the 200 level page cap and a capped side is a PREFIX of the real book rather
than a thin market. **If you swap, write in the file that the deep end went
unrepresented.** Recommended: work the four cheap ones first and decide EURC on the
days that are left.

**Choice 2: whether the golden fixture counts as one of the five.**
`testdata/fixtures/ustry_pre_exploit.md` is Layer 1 already applied to the incident
state, and it was computed before any implementation existed. Three things argue
against counting it: `scripts/check-manual-recomputation.sh` reads
`testdata/manual/` at `-maxdepth 1` only, `10-validation.md` section 6 records that its
with-pool tables are not computed, and finding P1-34 records that it carries `Pools`
nil although a pool held reserves at that ledger. So it exercises no part of the SDEX
and AMM combination, which is the most exposed rule in the methodology. **Recommended:
do not count it. Work five new ones.** Each of the five has exactly one liquidity pool,
so each exercises non-negotiable rule 4.

**Choice 3: format, and the trap in it.** Markdown, CSV or plain text. **Never
`.xlsx`.** `.xlsx`, `.ods` and `.numbers` are zip archives, their text is deflated, and
the checker reads with `grep`. Five correct recomputations in one of those formats
leave Layer 1 reading 0 of 5, and the failure looks like a broken checker rather than a
wrong format. This was verified on 3 September 2026 rather than assumed. If the work
happens in a spreadsheet application, keep the working file outside the repository and
commit the export.

---

## 2. Nine steps, per asset

Run all nine for one asset before starting the next. Step 8 is where a finding
surfaces, and a finding is worth more after one asset than after five.

### 2.1 Copy the template

Everything below the rule in `docs/internal/layer-1-worksheet-template.md` into a new
file:

```
testdata/manual/AUDD.GDC7X2MX-USDC.GA5ZSEJY-ledger-64310828.md
```

Nothing gates the filename. That shape makes five files sort into a readable order and
puts the asset in front of a reader without opening anything.

### 2.2 Fill the header, and note which two fields are gated

`scripts/check-manual-recomputation.sh` is the authority on this, not the table below.
Two fields are GATED, and a file missing either is not counted at all, because a
recomputation that does not say what it is about cannot be compared with anything.

| Field | Gated | Form |
|---|---|---|
| Issuer address | **yes** | `G` and 55 base32 characters, **inside the file**. The eight characters in a filename are not read |
| Ledger sequence | **yes** | the word `Ledger` and at least five digits, close together: `**Ledger:** 64310828` or `Ledger,64310828` |
| Source or provenance | reported | the word `source` or `provenance` |
| Date | reported | an ISO date, `YYYY-MM-DD` |

An asset is the pair (code, issuer) and is never matched on the ticker. Several issuers
use the code `USDC` and only one of them is the one that matters.

The date is what makes the ordering rule inspectable by a reader, which is the closest
thing to enforcement that rule can have.

**Answer this line honestly:** "Computed BEFORE any engine output for this ledger was
read: yes / no". If the answer is ever no, write no. A recomputation that saw the
answer first is still worth keeping as a check on transcription, and calling it
independent when it was not is the one thing that would make the directory worthless.

### 2.3 Transcribe from `price_r`, never from `price`

The source is the input file beside this one, sections 3 and 4. The `price` column
exists there ONLY so the rounding is visible and must not enter the arithmetic. Rule 5
of `CLAUDE.md`.

The live example is in `AUDD-USDC-64310828.md`, its first two bids: `7439209/10000000`
and `20000000/26884579` both render as `0.7439209`, and only the first is exactly that.
A transcription of the `price` column produces a book with two levels at one price, and
its ordering and its depth boundaries are then both wrong.

### 2.4 Answer the unit question in writing before using it

This is section 2 of the worksheet and it is the most productive trap in Layer 1.
Horizon denominates a bid's amount in the counter asset and an ask's amount in the base
asset. `domain.Level.Amount` is defined in BASE units. So one side needs converting and
the other does not.

**Derive which side, and in which direction, from `docs/methodology/`.** Not from the
input file, not from `internal/horizon/CLAUDE.md` trap 5, and not from the engine.
Write down where in the methodology that is established.

Getting it backwards produces depth figures wrong by roughly the price, on one side
only, and that shape looks plausible. Section 1 of the protocol names three things this
layer catches and the third is wrong unit.

### 2.5 Reference price

`docs/methodology/03-reference-price.md`. Best bid, best ask, `P0`, `priceSource`,
`spreadPct`, each with its working and its methodology reference.

Unit convention: a name ending in `Pct` is a percentage, an input written δ is a
fraction. `docs/methodology/README.md` section 2.

### 2.6 Depth, both sides separately, at every delta

`docs/methodology/04-depth.md`. Reporting the two sides separately is FR-4 and it is
half of what the oral test asks about.

One labour saver you can derive yourself: only the levels whose price falls inside the
target band contribute, plus the one boundary level that fills partially. Transcribe
every level as evidence, then compute over far fewer. Derive the band edges from the
methodology rather than from intuition.

**Then the table that matters most in the whole worksheet: the combination.** SDEX and
AMM are combined through a shared marginal price limit and are NOT summed separately,
which is non-negotiable rule 4. The worksheet asks for the rule in your own words, then
`from SDEX`, `from AMM` and `combined` at each δ with the reason they are not added
independently. All five assets carry a pool, so all five test this. If Layer 1 finds a
real defect anywhere, this is the likeliest place.

### 2.7 Manipulation cost, collateral, flags, band

`05-manipulation-cost.md`, `08-collateral.md`, `09-flags-and-bands.md`. Three things
that slip:

- The cost is the notional paid to **other parties**, and `Reachable` is a separate
  claim from the cost.
- `maxReachablePrice` is **null** when an active pool is present. All five assets have
  one. If you compute a value for any of them, one of the two is wrong, and that is a
  finding.
- **Zero is a value and is not the same as absent.** Where a figure is genuinely zero,
  write zero and write the reason. Where it cannot be computed at all, write
  unevaluated and write what is missing. The engine makes that same distinction in its
  flag output, and a recomputation that collapses the two cannot check it.

### 2.8 Prove the file is readable, before looking at the engine

```bash
make manual-check
```

Read the row for your file: yes or no under each of the four fields. Exit 0 means every
required recomputation is present and identified, exit 1 means fewer than required or
one identifies nothing, exit 2 means the protocol could not be read. **Exit 2 is not a
pass.**

### 2.9 Only now ask the engine

```bash
go run ./cmd/keel layer1 \
  -recording docs/evidences/layer-1/raw/AUDD-USDC-64310828.json.gz \
  -after-writing testdata/manual/AUDD.GDC7X2MX-USDC.GA5ZSEJY-ledger-64310828.md
```

It refuses to print until the file named by `-after-writing` exists and is not empty. It
reads the recorded bytes and computes over exactly them, with no network request and no
reconstruction, so a disagreement is about the formula rather than about a rebuild. The
rebuild is what Layer 3 tests. The command cannot prove a figure was worked by hand, and
its own help text says so; what it makes deliberate is that the answer was not visible
while the figure was being derived.

Fill in section 8 of the worksheet, twelve rows, including the "agree within
`Tolerance`" column. `Tolerance` is `0.0000001`, from `10-validation.md` section 4.
Level counts, prices expressed as rationals and flag sets are compared **exactly**;
only derived decimal quantities use the tolerance.

---

## 3. When a figure disagrees

**The disagreement IS the finding.** It is not resolved by editing either side.

1. Record it in section 8 of your file, your figure and the engine's side by side.
2. Commit the file with the disagreement still in it. That is evidence, not a failure.
3. Hand it over. `internal/domain/compute.go` is YELLOW, so the code is Claude's to
   correct, and the direction runs one way only: **adjust the code to match your
   numbers, never your numbers to match the code.** That rule is written in
   `internal/conformance/fixture.go`, in the `CLAUDE.md` zone map, in
   `testdata/manual/README.md` and in `10-validation.md` section 1.
4. If the transcription turns out to be the thing that was wrong, that is also a finding
   and it is still recorded. What must never happen is a number quietly brought into
   line.

---

## 4. When Layer 1 is done

| Evidence | Now | Done |
|---|---|---|
| `make manual-check` | exit 1, `0 of 5 present` | **exit 0, 5 of 5** |
| P2-23 in `scripts/audit-verification.sh` | PROVEN | **NOT** |
| `10-validation.md` section 1, Results table | empty | one row per asset |
| `10-validation.md` section 6, first box | unticked, "0 of 5" | ticked |
| PRD section 9, Deliverable 1 criterion 4 | prerequisite only | met |

The last three rows are Claude's once the five files exist: the Results table filled by
COPYING Al's figures rather than deriving any, the definition of done box ticked, the
tracker and the criterion score updated. The boundary is stated in advance because it
is the whole basis of this layer: Claude transcribes, Claude does not compute.

---

## 5. Time estimate

| Asset | Levels | Estimate |
|---|---|---|
| AUDD | 16 | 1.5 to 2 hours, including the learning time for the unit question and the combination rule |
| USTRY | 26 | 1 to 1.5 hours |
| BRL | 40 | 1.5 hours |
| PYUSD | 42 | 1.5 hours |
| EURC | 157 | 3 to 4 hours, or about 1.5 if swapped for ARST |
| **Total** | | **8.5 to 10.5 hours, or about 7 with ARST** |

Ten days remain. This is two sittings. It runs in parallel with the Claude side lanes,
because Layer 1 is Al's hands throughout and none of it waits on code.

---

## 6. What this file does not claim

It does not claim the procedure is sufficient. `10-validation.md` section 1 states what
Layer 1 does not catch, and it is a short sentence with a long reach: a correct formula
applied to the wrong data. Section 5 of that file goes further, and it is the honest
frame for all of this: if the definition itself is wrong, all three layers pass and the
numbers are still wrong.

It also does not claim the ordering rule is enforced. No permission layer can tell
whether a number was computed before or after the code it agrees with. The `-after-writing`
flag makes the ordering deliberate, the date field makes it inspectable, and neither
makes it provable.

---

## 7. Version history

| Date | Change |
|---|---|
| 8 September 2026 | Created. The long form of `README.md` section 5, written because the worksheet template it pointed at is gitignored under DEC-004 and therefore invisible to a clone. Records the three choices to make before starting, the nine steps, and the five rows that mark Layer 1 done |
