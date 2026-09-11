# DEC-016: The SOW names Horizon where the PRD names Hubble

**Status:** Accepted by Al, 12 September 2026. **OPTION A**, which section 6
recommends: the PRD's wording is amended to match the SOW, criterion 3 is scored as
cross-validation against Horizon, and FR-13 and FR-14 are re-priced from MUST to the
Phase 3 improvement DEC-002 section 4 already called them.
**Date drafted:** 2026-09-06
**Kind:** Scope. It changes no formula, no threshold and no stored value. What it
changes is which document the third Deliverable 1 acceptance criterion is scored
against, and whether two MUST-priority requirements are owed to the funder or were
added by the builders on top of what was promised.
**Drafted by:** Claude
**Decided by:** Al, 12 September 2026, Option A. Between 6 and 12 September this
line read "nobody yet" and the record proposed rather than decided; that is kept in the
version history rather than erased, because which of three options was taken is the
part a later reader needs and "Accepted" alone does not carry it.
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. Every option in section 6 is Al's.
**Settles:** the question DEC-002 section 8.3 left open and said it could not close.
**Depends on:** a document that is not in this repository. See section 8.

---

## 1. The evidence, read for the first time inside this repository on 6 September 2026

Al provided the SOW PDF on 6 September 2026 and it was read in full, seven pages.
`docs/context/Keel_SoW.pdf` is in the git history at `f499ab4` and was removed from the
working tree at `852ef27`, so no session before this one could open it.

**Measured, not summarised:**

| Term | Occurrences in the SOW |
|---|---|
| Hubble | **0** |
| BigQuery | **0** |
| big query | **0** |

The SOW names the other source three times, and once more by implication:

> **Week 1, planned work:** "Cross-check output **against Horizon** on sample ledgers."

> **Week 1, expected output:** "Cross-validation passed on at least 50 sample **ledgers**."

> **Section 6.1, Deliverable 1 evidence:** "Open-source repository containing the
> Liquidity Depth Engine, reproducible methodology, and **cross-validation results
> against Horizon on sample ledgers**."

> **Out-of-Scope:** "Captive-core ingestion. **The proof of concept uses public Horizon
> and RPC endpoints.**"

## 2. What the PRD says instead

> **Section 9, Deliverable 1:** "- [ ] Horizon versus **Hubble** cross-validation on at
> least 50 **pairs**, results tabulated"

> **Section 4.2:** "| FR-13 | Read a historical ledger snapshot from **Hubble** for a
> given `ledgerSeq` | **M** |" and "| FR-14 | Both sources return an identical
> `Snapshot` shape | **M** |"

The PRD defines **M** as "without it the deliverable fails". `internal/hubble/` holds
one `CLAUDE.md` and no Go, so two MUSTs are at zero.

**And the PRD says what it is** in its own opening paragraph: "This document
complements the SOW, it does not replace it. **The SOW is the commitment to the funder
about what gets delivered.** This PRD is the product definition for the builders."

## 3. Which of DEC-002 section 8.3's three readings this establishes

That section listed three and said only Al could close it, because `docs/context/` was
not on disk. The readings were:

1. The SOW names no source, the PRD narrowed it to Hubble on its own, and Layer 3
   satisfies the SOW.
2. The SOW names Hubble too.
3. The SOW is silent and the PRD is the operative contract.

**Reading 1 is established, and it is stronger than it was written.** The SOW does not
merely fail to name Hubble. It names Horizon, three times, in the two places that
define the work and the evidence. Reading 2 is refuted. Reading 3 does not arise,
because the SOW is not silent.

So the narrowing to Hubble was made in the PRD, by the builders, after the SOW was
agreed. It was not asked for by the funder.

## 4. The unit differs too, and that half cuts the other way

The SOW asks for 50 sample **ledgers**. The PRD asks for 50 **pairs**. They are not the
same count and this record will not quietly pick the convenient one.

Read as pairs, the runs of 26 and 31 August already cleared it: 60 pairs, 0 MISMATCH.
Read as ledgers, those three runs are 180 rows across **seven distinct ledgers each and
twenty-one in total**, which is not fifty. That gap was closed on 6 September 2026 and
the evidence is `docs/evidences/2026-09-06-layer3-fifty-ledgers.md`: 79 distinct
ledgers, 360 comparisons, 96 MATCH, **0 MISMATCH**, and 51 ledgers carrying at least one
fully comparable pair that agreed at all four depths.

**That document also states its own limit in its own heading**, and it belongs here
rather than in a footnote: the passing ledgers are carried by two quiet pairs, BRL and
ARST, while four of the eight pairs produced no comparable row in ninety minutes. The
honest claim is that the historical path agreed everywhere it claimed to be complete
and declined to answer on pairs whose offers move inside five minutes.

## 5. What is actually at stake

| | If the SOW governs | If the PRD governs as written |
|---|---|---|
| D1 criterion 3 | met, and evidenced | not met, and unmeetable without a Hubble adapter |
| FR-13, FR-14 | not owed to the funder | two MUSTs at zero |
| D1 against the SOW | about 93 per cent | unchanged, the SOW does not score itself this way |
| D1 against the PRD | unchanged at about 54 per cent until the wording moves | about 54 per cent |

## 6. The options, and one of them is recommended

**Option A. Amend the PRD's wording to match the SOW, and tell the Ambassador Chapter
Lead in writing.** Criterion 3 becomes cross-validation against Horizon on at least 50
sample ledgers, which is what was promised and what has been done. FR-13 and FR-14 are
re-priced from M to a later-phase improvement, which is what DEC-002 section 4 already
called Phase 3. **Recommended.**

**Option B. Keep the PRD as written and ship with two MUSTs at zero.** Legitimate, and
it then belongs in the handoff in writing rather than in nobody's notes. It costs D1
criterion 3 for the rest of the engagement.

**Option C. Build the Hubble adapter.** DEC-002 section 8.4 prices it at about +12 D1
points against the PRD, and `internal/hubble/` is YELLOW so the writing is Claude's.
The block is not technical: `gcloud` is `deny` in `.claude/settings.json`, `bq` is
`ask`, and `~/.config/gcloud` does not exist. The interactive login is Al's, for the
reason `scripts/s3-archive/` splits PREPARE from APPLY. It also remains the only road
that satisfies the PRD as written, so A and C are not mutually exclusive: A settles what
is owed, C is an improvement either way.

## 7. What this record is NOT, and the distinction is the whole of it

**It is not a rescoring.** `internal/conformance` carries a written rule against
adjusting the expected numbers to match the code, and lowering a bar because the work
did not clear it is the same move wearing different clothes. This repository has stated
that rule in four places and it applies to its own acceptance criteria first.

The distinction that makes Option A legitimate is narrow and it has to survive being
read by a sceptic: **the bar being removed is one the builders added to the funder's
bar without telling the funder.** Restoring the SOW's own wording is not lowering the
commitment, it is ending a unilateral raise. Two things keep that honest and both are
required:

1. The amendment is disclosed to the Ambassador Chapter Lead, in writing, before the
   deliverable is assessed. An amendment the client is told about and a quiet rescore
   differ only by that disclosure.
2. The Hubble gap is stated in the handoff whichever option is chosen, because a
   reader who compares the PRD's history against its present must find the reason
   rather than a silent edit.

## 8. What this record cannot settle, and one consequence nobody has named

**It cannot settle whether the funder read the SOW's Week 1 line as narrowly as this
record reads it.** Only the Ambassador Chapter Lead can say that, which is why every
option above routes through them.

**The consequence:** the SOW is not in the working tree, and
`scripts/history-migration/RUNBOOK.md` section 10.1 is about to remove it from the git
history as well, correctly, because it carries the commercial terms. After that runs,
**the document Deliverable 1 is measured against will exist only on Al's disk**, and a
future session will be in exactly the position every session before 6 September 2026
was in: unable to read the thing it is scored by, and reduced to paraphrase. DEC-002
section 8.2 records what that already cost, three plans scored against a rendering with
two words missing.

The PRD escaped this by one negation line in `.gitignore` that landed in its own commit
before it moved. The SOW cannot take the same route, because unlike the PRD it does
carry the fee and the hour allocation. So the mitigation is different and it is worth
deciding at the same time as section 6: extract the SOW's SCOPE and ACCEPTANCE language
into a tracked file that carries no commercial term, and keep the PDF out. That is a
proposal, not a decision, and the quotations in section 1 of this record are the first
four lines of it.

## 9. What Claude does once Al chooses

| Option | Claude's part | Al's part |
|---|---|---|
| A | amend this record to Accepted, update `docs/internal/delivery-tracker.md` and every brief that scores criterion 3, and re-anchor DEC-002 section 8 to the settled reading | edit `docs/context/Keel_PRD.md`, which is RED, and send the Ambassador the note |
| B | write the Hubble gap into the handoff, with FR-13 and FR-14 named and priced | nothing further |
| C | write `internal/hubble/`, the adapter, its tests and the 50-pair tabulation | `gcloud auth login` and a project with the BigQuery sandbox |

**ROW A IS THE ONE IN FORCE SINCE 12 SEPTEMBER 2026.** What it has produced so far,
and what it has not:

| Item | Owner | State |
|---|---|---|
| amend this record to Accepted | Claude | done, 12 September 2026 |
| `docs/internal/delivery-tracker.md` | Claude | done. NOT in any clone: `.gitignore` line 145 excludes that directory, so this is Al's local copy and a reader of the repository cannot check it |
| re-anchor DEC-002 section 8 to the settled reading | Claude | NOT DONE. `docs/decisions/` belongs to Track B under `tugas-a.md` section 7, and re-anchoring an open question inside a record Track B may be reading is where two tracks collide. Named here so it is not mistaken for finished |
| every other brief that scores criterion 3 | Claude | the tracked ones are four files under `docs/evidences/`, which section 7 also gives to Track B. NOT DONE, and for the same reason |
| the PRD's own wording | **Al** | not done. That file is RED and it is the one that defines what finished means |
| the written note to the Ambassador Chapter Lead | **Al** | not done, and section 7 condition 1 makes it binding rather than courteous |

**THE TWO CONDITIONS IN SECTION 7 ARE NOT SATISFIED BY THIS ROW BEING TAKEN.** Until
the Ambassador is told in writing, Option A and a quiet rescore are indistinguishable
from outside, which is the sceptic's reading section 7 says the distinction has to
survive.

## 10. Version history

| Date | Change |
|---|---|
| 6 September 2026 | Drafted. Records the first reading of the SOW inside this repository, establishes reading 1 of DEC-002 section 8.3, and hands three options to Al |
| 12 September 2026 | **Accepted by Al, Option A.** The header said `Accepted by Al` and `Decided by: nobody yet` at the same time for part of that day, which is why this row names the option: a record that says it was accepted without saying to what settles nothing, and section 9 assigns different work under each of the three. Section 7's two conditions bind with it and NEITHER is done yet: the amendment is disclosed to the Ambassador Chapter Lead in writing before the deliverable is assessed, and the Hubble gap goes into the handoff whichever option was chosen. Without the first, Option A and a quiet rescore differ by nothing a reader can see |
