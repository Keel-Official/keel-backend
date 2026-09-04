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
