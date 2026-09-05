# DEC-004: Repository Visibility and the Conditions for Opening It

**Decision:** The repository `Keel-Official/keel-backend` stays **private** until
the methodology code passes the golden fixture. Before its visibility is changed to
public, two files must be taken out first.
**Status:** IN FORCE from 20 August 2026, the first commit.
**Related:** the Deliverable 1 DoD section 6 requires a public repository as
evidence.

---

## 1. Why not public immediately

The DoD promises a public repository, so visibility is not a question of whether
but of when. What weighed on the timing: at the first commit the methodology code
is still empty, the `conformance` job is red because of that, and the `golangci-lint`
job was red because of an action version. A first-time reader arriving from an SCF
link would see two red jobs and an empty core package, with no context for knowing
that both are deliberate.

Waiting until the golden fixture passes buys a different first impression at zero
cost, because the commit history stays intact and still shows work starting on 20
August. What is lost by waiting: nothing. A private repository already serves as a
backup of the work.

**The trigger for opening:** `make conformance` passes without a build tag, meaning
`internal/conformance/golden_test.go` no longer needs `//go:build conformance` and
the `conformance` job in CI is green without `continue-on-error`.

---

## 2. The two files that must come out before going public

| File | Reason |
|---|---|
| `docs/context/Keel_SoW.pdf` | Carries the commercial terms agreed with the funder: the hour allocation, the fee, and the conditions attached to them. That is a document between Al and the funder, not material the public needs in order to assess the methodology. The specific terms are deliberately not restated here; see the amendment in section 6 |
| `docs/internal/` | The pre-development memo, the execution plan, and the repository audit. They contain criticism of the repository itself, corrections to the SOW, and an allocation of working hours |

Note that **`git rm` alone is not sufficient.** Both are already in the history as
of commit `f499ab4`, so deleting them in a later commit still leaves them in the
history and still readable by anyone once the repository is public. One of two
roads applies:

1. Rewrite the history with `git filter-repo` before the repository is opened. This
   is possible while it is still private and nobody else has forked or cloned it.
2. Move the contents into a fresh public repository with one clean initial commit,
   and keep this one as a private archive.

Road 1 is cheaper as long as nobody else has cloned. Road 2 is safer and loses the
commit history, which is itself one of the pieces of evidence worth showing. Choose
when visibility is actually changed, not now, because the number of third party
clones is not knowable until then.

**What stays and should be public:** `docs/methodology/`, `docs/decisions/`,
`docs/architecture/`, `docs/api/`, `docs/evidences/`, and `testdata/fixtures/`.
Those are the deliverable. `docs/evidences/` holds on-chain data anyone can fetch
from Horizon themselves, so there is nothing there to hide.

---

## 3. Why this decision is written down

A decision that lives only in someone's head disappears. The most likely failure
here is not forgetting to open the repository; it is opening it next month in a
rush before a deadline, with nobody remembering that the SoW went along with it.

So the condition is also enforced mechanically rather than only written: the
"Repository visibility" section of `scripts/audit-verification.sh` checks visibility
through `gh` and shouts if the repository is already public while either file is
still present. A rule that lives only in a document gets broken within two weeks.

---

## 4. What would change this decision

- The Ambassador Chapter Lead or an SCF reviewer asks for a public repository link
  before the methodology code is finished. If that happens, open it early and add a
  paragraph to the README explaining which red jobs are deliberate and why.
- A external collaborator appears who cannot be given private access.

---

## 5. One amendment, 24 August 2026

Where this record said `internal/depth`, it now says "the methodology code". The
package it named was emptied by methodology 1.0.3, which moved the computations to
`internal/domain/compute.go`, and removed on 23 August 2026.

The decision itself does not change, and the reason it does not is worth stating:
the trigger in section 1 was already written as an observable event, `make
conformance` passing without a build tag, and not as a property of a directory.
That is why a package being renamed and then deleted underneath this record left
the condition intact and only made the prose wrong. A condition phrased as "when
package X is done" would have had to be renegotiated here.

---

## 6. Second amendment, 28 August 2026

**Section 2 disclosed the commercial terms it argues should not be public.** The
row for `docs/context/Keel_SoW.pdf` quoted the hour allocation and the fee as its
reason for excluding the file that carries them. Anyone reading this record
learned the number the record exists to keep out of it. The row now names the
CLASS of term rather than its value, and the reasoning is unchanged because the
reasoning never depended on the figure.

**This amendment deliberately does not restate what was removed, and that
restraint is the whole technique.** An amendment reading "the figures X and Y were
taken out" reintroduces X and Y one paragraph below where they were deleted, in
the same public file, and leaves the record worse than before it was touched. So
the change is described and the values are not. Whoever needs them reads the SoW,
which is where they belong.

**A history rewrite would not have fixed this.** The runbook in
`scripts/history-migration/` removes `docs/context/` and `docs/internal/` from the
history. It does not touch a current, tracked, deliberately public file, and this
row was one. That is the general point worth keeping: the migration addresses what
is readable at old commits, and a marker sitting in today's working tree is a
separate problem with a separate fix, which is an ordinary edit.

**What this amendment does NOT do.** It does not reverse or soften the decision in
section 2. Both files still have to come out before the repository is public, for
the same reasons, and `git rm` alone is still not sufficient.

**Two things next to this are Al's and are left open rather than decided here.**

1. **The funder is still named in two tracked public files**, `docs/context/Keel_PRD.md`
   at two lines, which was `docs/api/Keel_PRD.md` when this was written, and
   `docs/api/keel-openapi.yaml` at one. It is no longer named in
   this record, which is a side effect of rewording the row above and not a
   decision that it should go: naming who funded the work is
   ordinary attribution and is a different thing from disclosing the terms, which
   is what section 2 protects. This record does not decide it, and removing the
   name from one of those files while the other carries it would achieve nothing.
   `scripts/history-migration/verify-clean.sh` treats the name as a marker, which
   is the scanner being blunt on purpose rather than a finding against these files.
2. **The Decision line and the Status line of this record are contradicted by
   measurement.** They say the repository stays private and that this was in force
   from 20 August 2026, the first commit. `gh api` reports a `PublicEvent` at
   2026-08-18T15:47:52Z, identical to the repository's `created_at`, two days
   before the first commit landed. The decision was never in force in fact.
   Recording that is a reversal, and a reversal is Al's to make and is recorded as
   a reversal rather than applied by editing the old text away. Section 0.1 of the
   local migration runbook holds the evidence.

---

## 7. Third amendment, 5 September 2026: `docs/context/` is no longer all one thing

**This amends and does not reverse.** Section 2 stands as written. What changed is what
is in the directory it talks about.

Al moved `Keel_PRD.md` into `docs/context/` on 5 September 2026, because it is an input
from outside in the same class as the SoW: it is the document the work is scored
against. The zone map row records it.

**The directory is now split, and a reader of section 2 alone would not know it.**

| In `docs/context/` | Tracked | What section 2 says about it |
|---|---|---|
| `Keel_SoW.pdf` | no, and it stays out | it must come out before going public |
| `Keel_PRD.md` | **yes, on purpose** | nothing. It was in `docs/api/` when section 2 was written, and that directory is on section 2's "what stays and should be public" list |

The PRD is kept tracked by one negation line in `.gitignore`, which landed in its own
commit before the move. Without it the move would not have reclassified the file, it
would have removed the acceptance criteria from the repository.

**THE TRAP THIS OPENS, AND IT IS WHY THIS SECTION EXISTS.** Section 2 gives two roads
out of the history, a rewrite or a fresh repository. Anyone reading section 2 and
reaching for `git filter-repo --path docs/context/ --invert-paths` would now delete the
PRD from the history along with the SoW, and the PRD is one of the documents a reviewer
needs in order to check the work against its own acceptance criteria. **The purge is by
FILE, not by directory.** When the road in section 2 is taken, the paths to strip are
`docs/context/Keel_SoW.pdf` and `docs/internal/`, named one at a time.

Nothing here decides item 1 of the 28 August amendment. The funder is named in the PRD
at lines 5 and 51 and the fee at line 364, exactly as it was while the file sat in
`docs/api/`. The move changes neither the exposure nor the decision, and that decision
is still Al's.
