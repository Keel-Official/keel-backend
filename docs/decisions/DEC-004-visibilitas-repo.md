# DEC-004: Repository Visibility and the Conditions for Opening It

**Decision:** The repository `Keel-Official/keel-backend` stays **private** until
`internal/depth` passes the golden fixture. Before its visibility is changed to
public, two files must be taken out first.
**Status:** IN FORCE from 20 August 2026, the first commit.
**Related:** the Deliverable 1 DoD section 6 requires a public repository as
evidence.

---

## 1. Why not public immediately

The DoD promises a public repository, so visibility is not a question of whether
but of when. What weighed on the timing: at the first commit `internal/depth` is
still empty, the `conformance` job is red because of that, and the `golangci-lint`
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
| `docs/context/Keel_SoW.pdf` | Contains the 126 hour, $2,268 budget and the terms agreed with the funder. That is a document between Al and Instawards, not material the public needs in order to assess the methodology |
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
  before `internal/depth` is finished. If that happens, open it early and add a
  paragraph to the README explaining which red jobs are deliberate and why.
- A external collaborator appears who cannot be given private access.
