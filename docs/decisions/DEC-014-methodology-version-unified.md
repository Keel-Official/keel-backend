# DEC-014: One methodology version in force for the whole set

**Status:** Accepted
**Date:** 2026-09-05
**Kind:** Bookkeeping. No definition, no formula, no threshold and no contract schema
changes. It changes which version string those unchanged definitions are stamped with.
**Drafted by:** Claude
**Decided by:** Al, 5 September 2026, item 3 of that day's ratification sheet
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. The decision in section 1 is Al's; everything else is
drafting.
**Methodology version:** this record is what moves it. `1.0.3-draft` to `1.0.8-draft` as
the version in force for the whole set.

---

## 1. The decision

The methodology version in force is **one number for the whole document set**, and it is
`1.0.8-draft`. Every methodology file header reads it, `internal/domain.MethodologyVersion`
reads it, and the contract's examples follow the code.

## 2. The state it replaces

`07-supporting-metrics.md` reached `1.0.8-draft` through five increments of its own,
1.0.4 to 1.0.8, all of them work inside that one file. The other eleven methodology
files, the code constant at `internal/domain/types.go` line 29, and ten examples in
`docs/api/keel-openapi.yaml` all still read `1.0.3-draft`.

So the repository carried two answers to "what version is the methodology", and
`docs/methodology/README.md` line 3 claimed to state the one **in force** while naming
the older of the two. Ten files claiming two versions guarantees a reader cites the
wrong one.

`internal/domain/supporting.go` line 146 already referred to `1.0.8-draft` in a comment,
while the constant two files away said `1.0.3-draft`. That is the split arriving in the
code rather than staying in the documents.

## 3. What did NOT change, and this is what makes it bookkeeping

No definition, no formula, no threshold, no flag rule, no schema. Eleven file headers
moved and each of those eleven files gained one version-history row saying, in as many
words, that nothing in it changed. `07` is untouched: it was already there.

The consolidated version history in `README.md` section 4 gained the five rows for 1.0.4
to 1.0.8 that it never had, which is why the split was invisible from the one file whose
job is to state the version in force.

## 4. NFR-9 is untouched, and the reason is precise

NFR-9 promises that re-running at the same `ledgerSeq` with the same
`methodologyVersion` produces identical numbers. Rows already stored at `1.0.3-draft`
keep that label, and the computation those rows came from has not changed, so they stay
reproducible under their own version. New rows are stamped `1.0.8-draft`. Nothing
compares across the two, and `internal/store` selects by version string, so a history
query for one never picks up the other.

309 tests pass unchanged, and `make api-mocks-check` reports the mocks match.

## 5. The contract moved with it, to 1.4.6

Ten `methodologyVersion` examples and the `/methodology` example's `version` field now
read `1.0.8-draft`. `docs/api/mocks/` is generated from those examples, so leaving them
would have handed the frontend a version string this API no longer sends.

**That failure has already happened here once.** Contract 1.4.1 records it: the
`/methodology` example carried `1.0.2-draft` while the server returned `1.0.3-draft`, and
the generated mock served the wrong one. The rule that follows is worth stating: the
contract's version examples follow the code constant in the same commit that moves it.

Not breaking. `methodologyVersion` is documented as a value to read rather than to pin,
and a consumer that hardcoded `1.0.3-draft` was going to break at the next methodology
change regardless.

## 6. Rejected alternative

**Leave `07` alone and let each file version independently.** Defensible, and it is what
the repository was already doing by accident. Rejected because two things then have to
change to make it honest, and neither is wanted: `README.md`'s "version in force" line
has to go, since there would be no such thing, and `MethodologyVersion` in the code has
to mean something narrower than the whole methodology, which is not what any output
carrying it claims. Per-file versions also make a response's single
`methodologyVersion` field unanswerable: the depth number comes from `04`, the flags
from `09` and the supporting metrics from `07`, and one field cannot carry three
versions.

## 7. One thing this record does NOT resolve, and it is RED

`testdata/fixtures/ustry_pre_exploit.md` line 4 reads
`**MethodologyVersion:** 1.0.2-draft`. That is a **third** value, older than either of
the two this record unifies, and it sits in the golden fixture.

It is not touched here because `testdata/fixtures/` is RED, enforced in both the deny
list and the hook, and it is the one file whose independence from Claude is the reason
the red zone still exists after `compute.go` went YELLOW.

**Why it matters rather than being tidy-up.** The header is what tells a reader which
methodology the hand computation was worked under. With the code at `1.0.8-draft` and the
fixture at `1.0.2-draft`, nobody can tell from the file whether its expected values are
still the right ones, and the conformance suite compares numbers rather than versions so
it will not say. Two of the changes between 1.0.2 and 1.0.3 were substantive for exactly
these figures: `MaxReachablePrice` becomes null when a pool is active, and the sell-side
fee treatment was corrected.

**Handed to Al**, and the honest options are: confirm in the file that the expected
values are unchanged since 1.0.2-draft and label it, or rework the ones the 1.0.3 changes
touch. Claude may report the disagreement and may not write either.

## 8. Amendment history

This record is append-only from its first commit. An amendment adds a row here and text
below the section it concerns, and no earlier sentence is edited or deleted.

| Date | Amendment |
| --- | --- |
| 5 September 2026 | Record created. Al's decision in section 1, taken as item 3 of that day's ratification sheet. Section 7 opens the golden fixture question, which is RED and not resolved here |

| 19 September 2026 | Section 9 added. The set has drifted off the decision in section 1 and now carries THREE versions, two of them without the `-draft` suffix. Drafted by Claude; it measures and proposes and decides nothing |
| 25 September 2026 | Section 10 added. **Road B taken** and `1.2.0-draft` chosen, which REVERSES section 1 for the code constant only. Decided by Al in a working session; drafted and applied by Claude. Section 1 is left as written, as the zone rule requires |

---

## 9. The decision in section 1 is no longer true of the repository, 19 September 2026

**Section 1 decided one number for the whole set. There are three.** Measured at
`ddab9ac` by `scripts/verify-sow.sh`, which fails on this and names it:

| What | Header says | `-draft`? |
| --- | --- | --- |
| `01-data-sources.md` | **1.2.0** | **no** |
| `11-limitations.md` | **1.2.0** | **no** |
| `00`, `02` to `10`, and `README.md` | **1.1.0-draft** | yes |
| `internal/domain.MethodologyVersion`, `types.go:29` | **1.0.8-draft** | yes |
| the contract's examples, `docs/api/keel-openapi.yaml` | **1.0.8-draft** | yes |

**This is the state section 2 describes, one increment worse.** Section 2 records
two answers and calls ten files claiming two versions a guarantee that a reader
cites the wrong one. Three answers across thirteen files is the same defect with
more places to land on.

**THE SERVED VERSION IS NOT WRONG, and confusing the two would send the repair in
the wrong direction.** `GET /v1/health` answers `methodologyVersion: 1.0.8-draft`
and that string is correct: it is what the code that computed the row was stamped
with, and non-negotiable rule 1 requires every output to carry the version it was
computed under, not the newest version anybody has written down. Bumping the
constant to make the numbers match would make every stored row claim a methodology
it was not computed under. The contract's own comment at line 47 already says this
in as many words.

**What the `-draft` suffix drop asserts, which is the part nobody has decided.**
`1.2.0` without `-draft` is a stronger claim than any other header in the set makes:
it says the definitions in those two files are settled. Whether that is intended is
not readable from the files, and neither file's version history says a decision was
taken. This record cannot settle it because `docs/methodology/` is RED.

### 9.1 The two roads, and neither is taken here

**Road A, one number again, the way section 1 meant it.** Pick the version in force,
stamp all thirteen headers and the code constant with it, regenerate the mocks, and
add a version-history row to each file saying nothing in it changed. This is what the
5 September pass did and it is about an hour of bookkeeping. Its cost is that it
flattens whatever the `1.2.0` headers were trying to say.

**Road B, say that the documents and the code version SEPARATELY, and write the rule
down.** The methodology set moves as documents are written; the code constant moves
when the behaviour it stamps changes. They are then allowed to differ and the README
states which is which. Its cost is that section 1 of this record is reversed, and a
reversal is recorded as a reversal rather than applied by editing section 1 away.

**Road B is the one this drafting would pick if picking were Claude's**, because the
served-version paragraph above is an argument that the two things are genuinely
different clocks, and Road A has now been run once and drifted within a fortnight. A
rule that has to be re-applied every two weeks is usually the wrong rule rather than
an under-enforced one. **It is recorded as a preference and not as a decision**, and
the drift is reported either way.

### 9.2 What is Claude's once Al picks

All of the bookkeeping and none of the choosing: the header edits under Al's
direction, the version-history rows, `make api-mocks-check`, and the line in
`docs/methodology/README.md` section 4. What stays Al's is which number, whether
`1.2.0` keeps its dropped suffix, and, on Road B, the reversal of section 1.

---

## 10. Road B, decided 25 September 2026, and what it reverses

**The decision, taken by Al, not by this drafting.** Road B of section
9.1, with the document set at `1.2.0-draft`.

**What it reverses, stated as a reversal.** Section 1 said one number is read by every
methodology header AND by `internal/domain.MethodologyVersion`. The second half no longer
holds. From 25 September 2026:

| Clock | Value | Moves when | Who follows it |
|---|---|---|---|
| Document version | `1.2.0-draft` | a definition in any methodology file is written or changed; every header moves together | the thirteen files in `docs/methodology/` |
| Engine version | `1.0.8-draft` | the behaviour of the code that computes a stored row changes | stored rows, `GET /v1/health`, every API output, and the examples in `docs/api/keel-openapi.yaml` |

The first half of section 1, one number for the whole document set, still stands, and
the set satisfies it again: all thirteen headers read `1.2.0-draft`.

**Why the suffix stays.** `1.2.0` without `-draft` would say the definitions are
settled. The thresholds in `09-flags-and-bands.md` are chosen rather than calibrated, and
`01` and `11` recorded no decision to drop the suffix, so their headers were corrected
back to `1.2.0-draft` with a version-history row each.

**Why not Road A, measured rather than asserted.** The API reads
`metrics WHERE methodology_version = <constant>`, and the holder and trade caches are
keyed by the same constant. Raising it would drop every stored row, the whole
`/history` series and the three `?ledger=` reconstructions, out of the API until each
was recomputed, and every asset would read null until `keel holders` and `keel trades`
refilled. It would also stamp single-pair output with a version whose section 2 rule,
the worst band across pairs, is not implemented, which is the NFR-9 problem DEC-015
section 5 records.

**What keeps it from drifting a fourth time.** `docs/methodology/README.md` states both
clocks in its header, and `scripts/verify-sow.sh` now checks three things instead of
one: the thirteen headers state one version, the README names that version as the one
in force, and the README names the engine version the code actually carries.

**What this does not touch.** No definition, formula, threshold or contract schema. The
golden fixture's `1.0.2-draft` header, section 7, is still open and still RED.

