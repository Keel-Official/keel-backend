# Layer 1: hand recomputation

**ZONE: RED.** Al works every file in this directory by hand. Claude never produces
a number that lands here, and the deny list and the hook both refuse it.

**Why that matters more than it reads.** These files are the independent oracle for
`internal/domain/compute.go`, which has been YELLOW since 25 August 2026. Since that
move, the numbers in this directory and in `testdata/fixtures/` are the only
structural reason to believe the implementation is checked against figures derived
independently of it. A recomputation produced by the thing it is meant to test is not
evidence, it is a restatement.

**The direction of correction, which is stated three times in three files and is
stated again here.** Adjust the code to match these numbers. Never adjust these
numbers to match the code. Where a file here and the engine disagree, the
disagreement IS the finding: report it, do not resolve it by editing either side.

**The ordering rule.** A figure in this directory must exist before the code that
satisfies it. No mechanism can enforce that, because no permission layer can tell
whether a number was computed before or after the code it agrees with. It is
honest about that and it is still the rule.

---

## 1. What the protocol asks for

From `docs/methodology/10-validation.md` section 1:

| Property | Value |
|---|---|
| Question answered | is the formula correct? |
| Sample size | 5 assets, chosen to span the risk range |
| Passes when | every figure matches within `Tolerance` |
| Catches | wrong formula, wrong direction, wrong unit |
| Does not catch | a correct formula applied to the wrong data |

`Tolerance` is `0.0000001`, from section 4 of the same file. Level counts, prices
expressed as rationals, and flag sets are compared **exactly**; only derived decimal
quantities use the tolerance.

**"Spanning the risk range" is the part that is easy to lose.** Five healthy assets
would exercise one code path five times. The recomputations are worth what they are
because they disagree with each other: an asset with no bid, an asset with no
executable price at all, an asset whose only depth is in a pool, an asset where the
book and the pool both matter, and one ordinary liquid pair. The protocol's own
Layer 2 list is arranged the same way and for the same reason.

**The sample size is READ, not hardcoded.** `scripts/check-manual-recomputation.sh`
takes the number 5 out of the Sample size row above rather than carrying its own
copy, so if the protocol ever moves to six the checker follows it. Neither side of
that check can be edited to satisfy it: the requirement comes from the protocol, the
count comes from disk, and the script holds no expected value of its own.

---

## 2. What one file must contain

Two fields are GATED. A file missing either is not counted at all, because a
recomputation that does not say what it is about cannot be compared with anything.

| Field | Rule | Why it is gated |
|---|---|---|
| **The asset** | the issuer address, `G` followed by 55 base32 characters | an asset is the pair (code, issuer) and is never matched on the ticker. Several issuers use the code `USDC` and only one of them is the one that matters. The issuer address is the half a ticker cannot fake |
| **The ledger sequence** | the word `Ledger` and a number of at least five digits, within a short run of punctuation of each other | rule 1 of the non-negotiables: every output carries `LedgerSeq`. A book is a book at a moment, and without the moment there is no engine output to put beside it |

Both of these forms are accepted, and they are the two the checker was written
against:

```
**Ledger:** 61340263
Ledger,61340263
```

Two more fields are REPORTED but not gated, and the difference is deliberate. A file
missing its asset or its ledger is an absence of evidence. A file missing its source
or its date is evidence with a gap in its provenance. Both are worth knowing and they
are not the same thing.

| Field | Rule | Why |
|---|---|---|
| **Source** | the word `source` or `provenance` somewhere in the file | where the raw book came from. A recomputation of data whose origin is unrecorded catches a wrong formula and proves nothing about the market |
| **Date** | an ISO date, `YYYY-MM-DD` | when the recomputation was worked. It is what makes the ordering rule inspectable by a reader, which is the closest thing to enforcement that rule can have |

---

## 3. Format, and the trap in it

**Commit these as PLAIN TEXT. Markdown or CSV.**

`scripts/check-manual-recomputation.sh` reads these files with `grep`. It does not
care about the extension, and that is deliberate: the protocol says "spreadsheet
files" without fixing a format, and a check that demanded `.md` would report a
perfectly good `.csv` as absent.

**What it cannot read is a compressed one.** `.xlsx`, `.ods` and `.numbers` are all
zip archives. The text inside them is deflated, so the asset address and the ledger
line are not present as bytes and the grep finds neither. A file in one of those
formats fails BOTH gated fields and is reported as identifying no asset and no
ledger, which reads exactly like an empty file.

This was verified rather than assumed, on 3 September 2026: the same two lines were
put in a text file and in a zip, and grepped with the checker's own two patterns. The
text file matched both. The zip matched neither.

**So if the work is done in a spreadsheet application, export it.** Save the working
file wherever you like outside the repository, and commit the export. A CSV export
plus a short Markdown file that carries the asset, the ledger, the source and the
date is the shape that costs least and passes.

**One file per recomputation, at the top level of this directory.** The checker looks
at `-maxdepth 1` only, so a recomputation split across a subdirectory is invisible to
it. A spreadsheet exported as a folder of CSVs does not count as one recomputation; it
counts as zero.

**Do not leave scratch files here.** Every regular file in this directory except
`README.md` and dotfiles is treated as a candidate recomputation. A leftover
`notes.txt` is reported as a file identifying no asset, which makes the count read
worse than the work is.

---

## 4. Suggested filename

Not enforced by anything. It is here so five files sort into an order a reader can
follow, and so the asset is legible without opening them.

```
<CODE>.<first 8 of issuer>-<QUOTE CODE>-ledger-<seq>.md
```

For example, the shape the golden fixture would take if it lived here:

```
USTRY.GCRYUGD5-USDC.GA5ZSEJY-ledger-61340263.md
```

The full 56-character issuer address still has to appear INSIDE the file. The eight
characters in the filename are for reading, and the checker does not look at
filenames for the asset.

---

## 5. What to put in the body

The worked example is `testdata/fixtures/ustry_pre_exploit.md`. It is Layer 1 applied
to the incident state, and it was computed before any implementation existed, which
is what gives it force. Its shape is the one to follow:

1. **A header** carrying the ledger and its close time, the methodology version, the
   source line, and the date the recomputation was worked.
2. **The input, transcribed.** Base and quote as (code, issuer) with the asset type.
   Every bid and ask level as `price_r` — the `n/d` fraction, never the `price`
   string, which is rule 5 of the non-negotiables — with its amount. Pool reserves if
   there are any, and an explicit empty list if there are none.
3. **The arithmetic, shown.** Not only the results. The notional of each level
   written out is what makes a wrong unit visible to a reader, and a wrong unit is
   one of the three things this layer exists to catch.
4. **Where the levels came from.** The golden fixture carries a table of on-chain
   operations with times and operation ids. That is what turns a transcription into
   evidence.
5. **The expected output**, one row per quantity, each with the reason it takes that
   value. `P0` and its source, the spread, the depth ladder at each delta on both
   sides, the manipulation cost ladder, `C_max` and its two terms, the flag set and
   the band.
6. **What is deliberately absent, and why.** The golden fixture states that the book
   it holds is the state immediately before the manipulation trade, that Horizon
   would never serve that as a snapshot, and that a contradiction about its
   `dataSource` label remains open as finding P1-32. A recomputation that records its
   own soft spots is worth more than one that reads clean.

**Zero is a value and is not the same as absent.** Where a figure is genuinely zero,
write zero and write the reason it is zero. Where a figure cannot be computed at all,
say unevaluated and say what is missing. The engine makes that same distinction in
its flag output and a recomputation that collapses the two cannot check it.

---

## 6. How to know where you stand

```
bash scripts/check-manual-recomputation.sh
```

or `make manual-check`. It reports one row per file with a yes or no under each of
the four fields, then a summary line. Exit codes are a gate rather than a report:

| Code | Meaning |
|---|---|
| 0 | every required recomputation is present and identified |
| 1 | fewer than required, or one of them identifies nothing |
| 2 | the protocol could not be read, so the check verified nothing |

**Exit 2 is not a pass.** It means the Sample size row could not be found in
`10-validation.md` section 1, and a check that cannot find its own requirement must
say so rather than fall back to a number compiled into itself.

**What the checker does NOT do, and it is the reason this directory is red.** It
counts and it identifies. It never computes a depth, never fills a blank, and never
writes here. Whether the figures inside a file are RIGHT is not a question anything
on the Claude side of the wall is allowed to answer. That is what the comparison
against engine output is for, and that comparison is the deliverable.
