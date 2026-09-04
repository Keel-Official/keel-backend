> **THIS FILE IS A RECONSTRUCTION, 2 September 2026.** The original was drafted on
> 1 September as `docs/decisions/DEC-011-supporting-metrics-ordering-rule.md`,
> amended twice on 2 September, and then overwritten on disk by a different record
> that took the same number and the same filename. It was never committed in this
> form, so git holds no copy. This text is rebuilt from the session transcript that
> read the file in full and from the two amendment scripts that produced the edits.
> It is faithful to the best of that record and it is NOT the original bytes. The
> number in the filename is deliberately left as `0XX`: DEC-011 is now an accepted
> record about holder pull atomicity, and renumbering this draft is Al's call.

# Annex to DEC-011: the reconstructed text of the ordering-rule record

**This file claims no decision number, by filename or by heading, and that is the
resolution rather than an omission.** It is the reconstructed text of the record that
became `DEC-011-supporting-metrics-ordering-rule.md`, kept because the original was
destroyed on disk before it was committed and this is the only copy of what it said.
It is history attached to DEC-011, not a second decision, so citing it as a decision
number was never right. Al settled this on 5 September 2026. Its original heading read
"DEC-011: The ordering rule extends to `internal/domain`, and `07` is the oracle for
the supporting metrics", and that line is preserved here rather than deleted, because
what this file is FOR is recording what the lost record said.

- **Status:** Proposed
- **Date:** 2026-09-01
- **Kind:** Extension of an existing rule to a wider path, plus the naming of an
oracle for work not yet written. No methodology change, no contract change, and
no change to `.claude/`.
- **Drafted by:** Claude, from three answers Al gave on 1 September 2026
- **Decided by:** Al
- **Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and
must not create or reverse a decision. The three answers in section 1 are Al's;
everything else in this file is drafting.
- **Methodology version:** `1.0.3-draft` in `internal/domain/types.go` line 29,
`1.0.5-draft` in `docs/methodology/07-supporting-metrics.md` line 3. This record
does not resolve that split. See section 10 item 4.
- **Number:** none. This is an annex to DEC-011 and claims no number of its own.
  The text below was drafted when DEC-010 was the highest number in use, and it
  noted that DEC-003 was then used twice, on the API contract record and on the
  USTRY/USDC pool ledger record. Both collisions were resolved on 5 September 2026:
  the pool record became DEC-013 and this file stopped claiming a number. DEC-014 is
  the next free number.

---

## 1. The decision

Three questions were put to Al on 1 September 2026 because the ordering rule that
replaced the `compute.go` lock does not, as written, reach the supporting metrics.
His answers, verbatim in substance:

| # | Question | Answer |
| --- | --- | --- |
| Q1 | Who computed the tables in `07` sections 1, 3 and 4, and with what? | "I computed them with my own query and a spreadsheet, then brainstormed the results with Claude Desktop to make the decisions" |
| Q2 | Does the ordering rule extend beyond `compute.go`? | "It extends to the whole of `internal/domain`" |
| Q3 | What is the oracle for the supporting metrics? | "Use `docs/methodology/07-supporting-metrics.md` and DEC-008" |

What follows is what those three answers mean when they are made specific enough
to act on, including one place where two of them contradict each other.

---

## 2. Ratified: the provenance of the figures in `07`, and why it is sufficient

The tables in `07` sections 1, 3 and 4 were produced by Al, from his own query
against the trade CSVs in `docs/evidences/` and his own spreadsheet. Claude
Desktop was used afterwards, on the results, to decide which of the seven
candidate exclusion criteria to accept, reject or defer. **The figures are Al's
arithmetic; the criteria selection was assisted.** Those are two different acts
and this record keeps them apart.

**The independence that the ordering rule protects is intact, and the argument is
structural rather than a matter of trust.** The rule exists to stop expected
values from being derived from the implementation they will judge. For the
supporting metrics there is no implementation to derive them from. Verified on
1 September 2026 by search across `internal/` and `cmd/`:

| Searched for | Found |
| --- | --- |
| a genuine-trade classifier | nothing. `internal/horizon/trades.go` line 10 states "There is no genuine-trade rule here, no dust" |
| holder concentration, top-1, top-10, HHI | field declarations in `types.go` lines 471 to 473 and thresholds at lines 392, 393, 457 and 458. No computation |
| volume to supply | field declarations at `types.go` lines 474 to 476. No computation |
| circulating supply, dust threshold | nothing outside comments |

Every touchpoint is a declared shape, a stored column or a configured threshold.
Nothing computes a supporting metric anywhere in this repository, so the figures
in `07` cannot have been derived from Keel's arithmetic. **A number cannot be
contaminated by code that does not exist.**

**What is NOT established, and it is named rather than left implied.** The query
that produced the figures is not recorded anywhere in the repository. Searching
`scripts/`, `docs/` and `measurements/` on 1 September for the distinctive values
(42,368 dust trades; 14,478 genuine; 5,723.2370064 USTRY) returns
`07-supporting-metrics.md` itself and the two raw CSVs, and nothing else.
`funding-graph-probe.sh` line 22 and `pull-holder-and-supply.sh` both state in
their own headers that they classify nothing.

So the figures are **independent but not reproducible.** Independence is what this
record needs and it has it. Reproducibility is what a reviewer of the paid
methodology will ask for, and `07` claims "Every figure below is measured, not
asserted" without saying by what. That gap is section 10 item 2 and it is Al's,
because the file is RED.

**AMENDMENT, 2 September 2026: the sentence in bold above stopped being true for
two of the four sections, and the argument that replaces it is weaker.** "A number
cannot be contaminated by code that does not exist" was checked on 1 September and
held for every supporting metric. On 2 September `scripts/holderstats/main.go`
appeared in the working tree, computing top-1, top-N, HHI and circulating supply
from the recorded pull, and the figures now in `07` sections 2 and 3 come from it.
This session did not write it.

The independence the ordering rule protects is still intact, because
`internal/domain` holds no implementation of these metrics and the rule exists to
stop an expected value from being a transcription of the code it will judge. But
the ground under it moved. For sections 1 and 4 the claim remains "no code
computes this". For sections 2 and 3 it is now "the code that computes this is not
the code under test", which is a different and weaker claim, and it carries a
condition the older one did not need:

**The `internal/domain` implementation must not be derived from
`scripts/holderstats/main.go`.** Two programs written from the same source are one
implementation with two entry points, and their agreement proves nothing. The
condition is sharper than it looks because `scripts/` is GREEN, so Claude may write
there: a Claude edit to that file is a Claude edit to the instrument that produced
the oracle. Whether that is acceptable, and whether `scripts/holderstats/` should
be reclassified now that its output feeds a RED file, is section 10 item 10.

---

## 3. The contradiction between Q2 and Q3, and how this record resolves it

The rule Q2 extends reads, in `CLAUDE.md` line 66 and again around line 134:

> a function may only be written after its expected values exist in
> `testdata/fixtures`

The oracle Q3 names is `docs/methodology/07-supporting-metrics.md`, which is not
in `testdata/fixtures`. **Taken literally, Q2 forbids exactly the work Q3
authorises.** Answering both was right; the two answers were given to different
questions and neither was wrong. Reconciling them is drafting work and is done
here rather than left for whoever hits it first.

**The resolution: the location generalises, the discipline does not.** What made
`testdata/fixtures` load-bearing was never the path. It was three properties:

1. It is RED, so Claude does not edit it to make a test pass.
2. The values in it are written before the code they judge, so they cannot be a
   transcription of that code.
3. It names its numbers publicly, so a disagreement between it and the code is a
   finding somebody can see rather than an argument between two private opinions.

`07` has all three, and property 1 in a materially weaker form. The weakening is
stated here rather than left to section 9, because this is the paragraph that
rests on it.

**Property 1 is mechanical for `testdata/fixtures/` and conventional for `07`.**
The fixture directory is denied at `.claude/settings.json` lines 25 through 30 and
closed by `zone_any` at hook line 170, so "Claude cannot edit it" is a fact about
the tooling. `docs/methodology/` is RED in the zone map and deliberately not
enforced, which section 9 records, so for `07` the same sentence is a fact about
conduct. Al was shown the asymmetry on 1 September and chose to leave
`docs/methodology/` open rather than close it. The consequence belongs in this
record and not in a comment: the guarantee behind the supporting metrics oracle is
one rank weaker than the guarantee behind the golden fixture, and a reviewer
weighing the two should weigh them differently.

**Property 2 holds per table, not per file.** `07` is not complete. Section 2 of
it reads "Not yet run", which is exactly why three rows of section 5 below read
NO. What is established is narrower and is still enough: each table `07` already
carries was written before any code computing the quantity in it existed, which
section 2 of this record verifies. A table added to `07` later inherits nothing
from this record and has to satisfy property 2 on its own terms.

The rule therefore becomes: **a function in `internal/domain` may only be written
after its expected values exist in one of the RED artifacts enumerated below, and
that artifact and the specific values are named out loud before the function is
written.**

| Artifact | Covers | Property 1 |
| --- | --- | --- |
| `testdata/fixtures/` | depth, reference price, manipulation cost, collateral | mechanical, hook enforced |
| `docs/methodology/07-supporting-metrics.md` | the supporting metrics, FR-8 through FR-10 | conventional, enforced by nothing |

**The list is closed.** An artifact joins it by a new decision record and not by
being pointed at in a conversation. Without that clause the extension made here
decays into a rule that any RED file satisfies, which is the second alternative
section 7 rejects arriving by a different route.

**What deliberately does not generalise.** The obligation not to edit the oracle
to make a test pass is unchanged and now covers `07` as well. A disagreement
between the implementation and a table in `07` is reported as a finding and is
never resolved by editing either side, which is the rule
`internal/conformance/fixture.go` states in its own header and which
`internal/domain/CLAUDE.md` states as obligation 2.

**`internal/conformance/expected.go` is not absorbed by this extension.**
Obligation 2 names two places, `testdata/fixtures/` and the expected values in
`expected.go`, and only the first is RED. `expected.go` lives in
`internal/conformance`, which is GREEN, so the prohibition on editing it to make a
test pass rests on that obligation alone and on nothing in this record. Widening
the ordering rule must not be read as having covered it.

**What `testdata/fixtures/` holds today, since this section leans on it.** One
market. The golden fixture is Layer 1 applied to the incident state.
`testdata/fixtures/layer2/` does not exist, so Layer 2 stands at 0 of 10 with
every scenario skipping and the tally carried as finding P2-18. Section 7 below
says the directory "no longer holds the whole of the independent oracle", and that
is too gentle: it never held most of it.

**One disagreement inside the golden fixture is already open.**
`testdata/fixtures/ustry_pre_exploit.md` line 30 records `Pools: []` while
`GoldenSnapshot()` carries the pool that genuinely existed. That is handoff item
B-4 and it is Al's. It is named here because the paragraph above states the
discipline for a disagreement between an oracle and the code, and stating that
discipline while an instance of it sits open is the shape section 11 exists to
prevent.

---

## 4. The exact wording, and why the old sentence is kept verbatim

**`CLAUDE.md` line 66 is not edited.** P2-13 in `scripts/audit-verification.sh`
matches the literal string `expected values exist in .?testdata/fixtures` against
`CLAUDE.md`. Rewording that sentence flips P2-13 to PROVEN, which would report the
ordering rule as unwritten on the day it was widened. The existing sentence stays
exactly as it is and a second sentence is added beside it.

Proposed addition to the `internal/domain` row of the zone map, which is **Al's to
apply because the file is his brief**:

> THE ORDERING RULE COVERS THE WHOLE PACKAGE since 1 September 2026, DEC-011: a
> function here may only be written after its expected values exist in one of the
> two RED artifacts DEC-011 section 3 enumerates, and that artifact and the values
> are named out loud before the function is written. The two are
> `testdata/fixtures/`, which the hook enforces, and
> `docs/methodology/07-supporting-metrics.md`, which it does not and which is the
> oracle for the supporting metrics. The list is closed and grows only by a new
> record. Neither artifact is ever adjusted to match code.

---

## 5. The oracle table, in the format DEC-008 section 6 uses

Q3 named DEC-008, and this is the part of DEC-008 that the naming asks for. Its
section 6 lists which functions have a hand computed oracle and which do not,
because the move is defensible only if the gap it opened is visible. The same
table for the work about to be written:

| To be written | Hand computed oracle | Where the expected values are |
| --- | --- | --- |
| genuine-trade classifier, five ordered conditions | **YES** | `07` section 1, "Result when run over the USTRY history". Two periods, per condition, count and volume. Plus three named specimen outcomes |
| genuine volume in the oracle window | **YES** | `07` section 4, the 15 / 30 / 60 minute table on the 22 February incident |
| volume-to-supply NUMERATOR, genuine volume in USTRY | **YES** | `07` section 3, the three window min and max table, and the month total 5,723.2370064 USTRY |
| volume-to-supply RATIO | **YES**, 2 September | `07` section 3 at `1.0.6-draft`, the seven window table, over a denominator of 10,432,382.3504695 USTRY. Carries that section's own anchoring caveat |
| holder concentration: top-1, top-10, HHI | **YES**, 2 September | `07` section 2 at `1.0.6-draft`: top-1 91.5406 %, top-10 99.9475 %, HHI 8,410.8452, over a population of 263 accounts |
| time since last genuine trade | **NO** | no expected value anywhere. The rule that identifies the trade has an oracle; the elapsed time computed from it does not |

**The oracle and the code are on different methodology versions.** `07` carries
`1.0.5-draft` and `internal/domain/types.go` line 29 carries `1.0.3-draft`, which
section 10 item 4 leaves unresolved. Every test written under this record
therefore judges a result stamped `1.0.3-draft` against figures measured under
`1.0.5-draft`, and it must say so where it does it. This blocks nothing and it is
not a formality: non-negotiable rule 1 puts `MethodologyVersion` on every output
so that a comparison across a version boundary is visible rather than silent, and
a conformance test is the one place that guarantee can be lost by omission.

**One row reads NO and that is the finding this table exists to produce.** Under
the rule as extended in section 3, that row may not be written yet.

**Two rows moved from NO to YES on 2 September and the table above is the amended
one.** As drafted on 1 September all three read NO, on the ground that the holder
pull had not run. It had, which is 6.1, and the hand computation over it landed the
next day, which is 6.2. Both rows carry the provenance caveat the amendment to
section 2 states: their figures came from a program rather than a spreadsheet, and
the condition attached to that is not optional.

---

## 6. What this unblocks, and what it does not

Stated plainly, because the point of the record is to be actionable.

| Work | Status under this record |
| --- | --- |
| genuine-trade classifier, `07` section 1 | **unblocked.** Oracle named, judged against the February and August tables |
| genuine volume in the oracle window, `07` section 4 | **unblocked** |
| volume-to-supply numerator, `07` section 3 | **unblocked** |
| holder concentration, `07` section 2 | **unblocked** on 2 September. See 6.2 |
| volume-to-supply ratio | **unblocked** on 2 September, with `07` section 3's anchoring caveat attached. See 6.2 |
| time since last genuine trade | **blocked** on a decision that is smaller than the others: whether a metric derived entirely from an oracled rule needs an oracle of its own |
| the first holder pull | **ALREADY DONE, 31 August 2026.** The draft of this record listed it as outstanding work. See 6.1 |

### 6.1 The first holder pull had already run when this record was drafted

This subsection is a correction and is written as one, because the two rows above
that name the pull said, in the draft of 1 September, that it was still to come.

It was not. `scripts/pull-holder-and-supply.sh` ran on **31 August 2026** and its
output is committed at
`docs/evidences/2026-08-31-USTRY.GCRYUGD5-holders-and-supply/`, in `ff285c5` at
13:52 local on 1 September, which is the day this record was drafted. The manifest
there records `Pages fetched: 5`, `Paging completed: YES`, a stop reason of "short
page (75 < 200); the trustline set ended here", and 875 holder rows beside
`accounts.authorized = 875` from `/assets` at ledger 64211133. That is the full
trustline set `07` section 2 requires before the metric may be evaluated at all.

**How the error got in is the part worth keeping.** Section 2 of this record
searched `scripts/`, `docs/` and `measurements/` for the provenance of `07`'s
figures and named `pull-holder-and-supply.sh` while doing it. The directory that
script wrote was one level away and was not opened. The sentence "Not yet run" was
carried across from `07` section 2 on the assumption that a RED file is current. A
RED file is authoritative about definitions. It is not a status board, and the two
were confused here.

**What changes and what does not.**

| | In the draft | After this correction |
| --- | --- | --- |
| Section 5 row for top-1, top-10, HHI | NO | **still NO.** A population is not an expected value |
| What holder concentration waits on | a data pull, then Al's arithmetic | **Al's arithmetic alone** |
| Where that arithmetic gets its input | did not exist | 875 rows already in the repository |
| `07` section 2, "Result when run over the USTRY history" | "Not yet run" | untouched. `07` is RED. Section 10 item 7 hands it back |

The row that stayed NO is the one that matters. A pull produces a population; the
oracle is top-1, top-10 and HHI over it, and those three exist in no artifact, RED
or otherwise. Holder concentration remains closed to Claude under section 3. It is
closed for a shorter reason than the draft stated, and a shorter reason is a
different plan rather than a smaller one.

**A reading was taken from that file, and it answers an open question in `07`.**
Section 2 of `07` requires that its two excluded positions be confirmed "against
the first real holder pull before the exclusion is wired", and names them: the AMM
pool `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb` and the
Blend V2 position at Soroban contract
`CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS`. Read against
`holders.csv` on 2 September 2026, neither appears, the issuer account does not
appear either, and all 875 `account_id` values begin with `G`. So `/accounts?asset=`
surfaces neither position in any form. The exclusion needs no wiring on the holder
side at all, and both must still be subtracted from the section 3 denominator,
which is the point at which the two sections are required to agree. Writing that
into `07` is Al's, and it is section 10 item 8.

**Two counts are recorded here and they are NOT expected values.** Of the 875 rows,
612 carry a zero balance and 263 do not, so the population under `07` section 2's
definition is 263 accounts. This is stated so that the size of the remaining hand
computation is known before it is started. It is a reading taken by Claude from a
raw file, not an oracle: under section 3 an expected value comes from one of two
enumerated RED artifacts and this paragraph is neither. Nothing in
`internal/domain` may be judged against these two figures, and if Al's own
filtering yields a different count then his is the one that counts and this
paragraph is the finding.

### 6.2 What landed on 2 September, and the three questions it opens

`07` moved to `1.0.6-draft`. It was uncommitted in the working tree when this
amendment was written, so the figures cited here are read from that state and not
from a commit.

| Item | Where it stood | Now |
| --- | --- | --- |
| section 10 item 7, "Not yet run" is stale | open | **closed.** Section 2 Result carries the measurement |
| section 10 item 8, the form the two excluded positions take | open | **closed**, and resolved the same way 6.1 read it: neither appears, so the exclusion is a no-op against this endpoint and pool supply leaves the section 3 denominator by construction |
| section 5, holder concentration oracle | NO | **YES** |
| section 5, volume-to-supply ratio oracle | NO | **YES** |
| the zero-balance example in section 2's rationale | "the first record Horizon returns" | 612 of 875, which is the population fact 6.1 recorded and is now in the RED file where it belongs |

The two independent readings agree. 6.1 counted 612 zero-balance trustlines and a
population of 263 from `holders.csv` on 2 September; `07` at `1.0.6-draft` states
the same two figures. That agreement is worth one line and no more: both readings
applied the same definition to the same file, so it checks the reading and not the
definition.

**Three questions are open and all three are Al's.** They are stated here because
`07` points at this record twice and because none of them is a tidy-up.

1. **The snapshot is not atomic and `07` asks this record what to do about it.**
   The pull's five pages span ledgers 64211133 to 64211152, 19 ledgers and 91
   seconds, so no single `LedgerSeq` describes the population and balances could
   have moved between pages. Non-negotiable rule 1 puts one `LedgerSeq` on every
   output. Section 10 item 9 sets out the roads and does not pick one.
2. **The instrument now exists and `07` still does not name it.** Item 2 of section
   10 asked for the query behind the figures. For sections 2 and 3 the answer is
   `scripts/holderstats/main.go`, and `docs/evidences/derived/holder-stats.md`
   names it while `07` does not. One line in `07` closes the older half of item 2.
3. **The provenance of these two rows is a program, not a spreadsheet**, which is
   the amendment to section 2 above and the condition it attaches.

---

## 7. Rejected alternatives

**Write fixtures for all four sections before any implementation.** This is the
stronger discipline and it was rejected on schedule grounds. `07` sections 1, 3
and 4 already carry measured figures over two full periods; recomputing them by
hand would spend one to two days of the only person who can also produce Layer 1
and Layer 2, both of which have been waiting since 28 August and both of which
score a criterion that this work does not. The cost of the rejection is that
`testdata/fixtures/` no longer holds the whole of the independent oracle, which is
why section 3 had to state what actually made that directory load-bearing.

**Leave the ordering rule at `compute.go` alone.** Rejected because the rule would
then be avoidable by naming a new file, which is precisely what the supporting
metrics were about to do legitimately. A rule that a well behaved writer steps
around without noticing is not a rule.

**Make this record the home of the ordering rule's wording.** Rejected for the
reason DEC-008 section 8 gives about itself. `CLAUDE.md` owns that sentence,
P2-13 reads `CLAUDE.md`, and a second home is the pattern this repository records
having lost to five times. Section 4 proposes wording for Al to apply there and
does not restate the rule as this file's own.

---

## 8. Design note, required by the YELLOW zone rule

Three sentences on the one design decision here, which is the generalisation in
section 3. The location clause of the ordering rule is widened from one directory
to a property that directory has, because the answers to Q2 and Q3 cannot both be
honoured under the literal wording and the property is what the rule was always
protecting. The rejected alternative was to keep the literal wording and require
Al to transcribe `07`'s tables into `testdata/fixtures/`, which would have been
mechanically tidier and would have bought nothing, since a transcription of a RED
document into a RED directory adds a copy rather than an independent check, and
the repository's own history is that a second home for one fact is how the two
begin to disagree.

---

## 9. Consequences for the harness

**P2-13 is unaffected**, on the condition in section 4 that the existing sentence
is kept verbatim. It stays NOT and continues to prove only that the sentence
exists.

**One new check is proposed, P2-25.** It asserts that the zone map carries the
package-wide extension and names `07` as an oracle. It carries the same self
stated limit as P2-13 and P2-22, in the same words those checks use: it proves the
sentence exists and never that a run honoured it. Numbering appends, so P2-25
follows P2-24 and the free low numbers stay free, per the note in P2-23.

**No change to `.claude/settings.json` and no change to
`.claude/hooks/lindungi-zona-merah.sh`.** This record loosens nothing.
`testdata/fixtures/**`, `testdata/manual/**` and `docs/context/**` stay denied at
settings lines 25 through 30 and closed by `zone_any` at hook line 170.
`docs/methodology/` is RED in the map and deliberately not enforced, which is the
existing arrangement and is why Claude can read `07` and, by conduct rather than
by tooling, does not write it. Closing that gap was proposed on 1 September and
Al chose to leave it open. What that costs is stated in section 3 and not here,
because section 3 is what depends on it.

---

## 10. Open items handed back

1. **Accept or reject.** If sections 2 through 6 are not what the three answers
   meant, the draft is wrong and the correction is Al's.
2. **The figures in `07` are independent but not reproducible.** The query that
   produced them is recorded nowhere. `07` claims "Every figure below is measured,
   not asserted" without naming the instrument. Al's, because the file is RED. The
   cheapest fix is a few lines in `07` naming the query and the spreadsheet, not a
   new document.
3. **Holder concentration has no oracle, and the only thing standing between it
   and one is Al's arithmetic.** The pull this item originally waited on ran on
   31 August 2026 and section 6.1 corrects the draft that said otherwise. Al works
   top-1, top-10 and HHI by hand from the population already in the repository, and
   only then may Claude write the holder concentration implementation in
   `internal/domain`. Claude writes no part of `07` itself, which is RED, and the
   words "section 2" in this item mean the metrics that section of `07` defines and
   never the text of it. That reading was made once already on the day this draft
   was written, which is why the sentence is now explicit. This is the single item
   that decides when the rest of the supporting metrics can be implemented.
4. **The methodology version is split.** `07` is `1.0.5-draft`, the code constant
   and nine other methodology files are `1.0.3-draft`. Bumping changes every stored
   row and every API response, so it is a decision and not a chore. No record asks
   it yet.
5. **`10-validation.md` says nothing about the supporting metrics.** Its three
   layers are all about depth, reference price and manipulation cost; searching it
   for genuine, holder or supporting returns nothing. So the validation protocol
   has no opinion on how FR-8 through FR-10 are verified, and this record names
   `07` as their oracle without a layer to sit in. Al's, and it is a real gap
   rather than a formality.
6. **The pointer edits.** The zone map addition in section 4 and the pointer in
   `internal/domain/CLAUDE.md`, both only once the status changes, and
   deliberately not in the same commit as this draft. A record that is still
   Proposed must not already be cited as authority by the files it governs, which
   is the discipline DEC-008 section 10 states and its amendment history shows
   being followed.
7. **CLOSED 2 September.** `07` section 2 read "Not yet run" while the pull it
   waited on had run on 31 August. `1.0.6-draft` replaces it with the measurement.
8. **CLOSED 2 September.** The open question in `07` section 2 about the FORM the
   two excluded positions take is answered, and `1.0.6-draft` answers it the way
   6.1 read it: neither the pool ID nor the Blend contract appears in
   `/accounts?asset=` in any form.
9. **The holder pull is not an instant, and `07` hands the question here.** Five
   pages spanning ledgers 64211133 to 64211152 produce five `latest_ledger` values,
   and non-negotiable rule 1 requires one. Four roads, none of them picked here
   because picking one is creating a decision and this record may not:
   (a) stamp the output with the LAST ledger of the pull and carry the span as its
   own field, so the figure is honest about being a 91 second read;
   (b) stamp it with the FIRST, which makes the figure a claim about a ledger at
   which four fifths of it had not been read;
   (c) keep either stamp but REJECT a pull whose span exceeds a threshold, which
   bounds the error instead of reporting it, and needs a number nobody has chosen;
   (d) give this metric a ledger RANGE rather than a `LedgerSeq`, which is not a
   small choice, because rule 1 is a non-negotiable and the API contract would
   carry a differently shaped field for one metric.
   Whichever road is taken, it reaches `07` section 3 as well: that section's
   anchoring caveat says the ratio inherits this pull's span. **This probably wants
   its own record rather than a section of this one**, because the subject of this
   record is the ordering rule. DEC-012 is taken, so the next free number is
   DEC-013.
10. **`scripts/holderstats/` produces the figures in a RED file while sitting in a
   GREEN directory, and the zone map has no row for it.** Two things follow and
   both are Al's. First, the map: a directory holding a tracked file is owned by a
   row or by an ancestor's row, and `scripts/` is GREEN, so this inherits GREEN
   silently, which is the cost the map's own preamble names. Second, the substance:
   a GREEN directory is one Claude may write in, and this one is now the instrument
   behind two rows of the section 5 oracle table. Whether that is acceptable, and
   whether the `internal/domain` implementation may be written by anyone who has
   read `main.go`, is the condition the amendment to section 2 attaches and it is
   not answered here.
11. **The methodology version split widened rather than closed.** Item 4 recorded
   `07` at `1.0.5-draft` against `1.0.3-draft` in the code and nine other
   methodology files. `07` is now `1.0.6-draft`. The gap is two minor versions and
   still no record asks for the bump.

---

## 11. Amendment history

This record is append-only from its first commit, the treatment DEC-006 section 9,
DEC-008 section 12 and DEC-009 section 10 give themselves. An amendment adds a row
here and text below the section it concerns, and no earlier sentence is edited or
deleted. A document that quietly grows correct hides the fact that it was wrong,
and the fact that it was wrong is the finding.

**Append-only begins at the first commit and not before.** The row below records
what was corrected between drafting and that commit, so the corrections are
visible without a document that argues with itself over text nobody ever saw.

| Date | Amendment |
| --- | --- |
| 2026-09-01 | Created as Proposed, the same day the three answers were given |
| 2026-09-01 | Corrected before first commit, on Al's review. Section 3: property 1 downgraded to conventional for `07` and the asymmetry stated, property 2 scoped per table rather than per file, the rule clause turned into a closed enumeration, and three omissions added, namely that `expected.go` is not absorbed, that `testdata/fixtures/` holds one market and no Layer 2, and that B-4 is an open instance of the discipline the section states. Section 4: wording follows the closed enumeration. Section 5: the methodology version split recorded where it touches the oracle. Section 9: pointed at section 3 for the cost of leaving `docs/methodology/` unenforced. Section 10 item 3: "section 2" disambiguated after it was misread once |
| 2026-09-02 | Corrected before first commit. Section 6 gains 6.1: the first holder pull had already run on 31 August 2026 and was committed in `ff285c5` on the day this record was drafted, so both section 6 rows that treated it as outstanding were wrong, and so was the section 5 paragraph that made holder concentration wait on it. The section 5 table row stays NO, because a population is not an expected value, and 6.1 says which of the two the correction moves. Section 10 item 3 corrected to match. Items 7 and 8 added: `07` section 2's "Not yet run" is stale, and its open question about the form of the two excluded positions is answered by a reading of `holders.csv`. Section 3's quotation of "Not yet run" was checked and left standing, because it quotes `07` accurately and `07` has not changed |
| 2026-09-02 | Amended again the same day, after `07` moved to `1.0.6-draft` in the working tree. Section 2: the argument "a number cannot be contaminated by code that does not exist" no longer covers sections 2 and 3, because `scripts/holderstats/main.go` now computes those figures; the weaker replacement claim is stated with the condition it needs. Section 5: the holder concentration and volume-to-supply ratio rows move from NO to YES and the paragraph under the table is rewritten from three rows to one. Section 6: both rows move to unblocked, and 6.2 records what landed and what it opened. Section 10: items 7 and 8 closed, items 9, 10 and 11 added, being the non-atomic snapshot question `07` hands here, the zone of `scripts/holderstats/`, and the widened version split |
| 2026-09-02 | **RECONSTRUCTED.** The file was overwritten on disk by a different record taking the same number and filename before this one was ever committed, so git holds no copy of the original. This text is rebuilt from the session transcript that read it in full and from the two amendment scripts that produced the edits above. Faithful, not the original bytes |
