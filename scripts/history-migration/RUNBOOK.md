# Clean-history migration runbook

**Status:** PREPARED, NOT EXECUTED. Every destructive step in this document is
written for Al to run. Nothing here has been run against the working repository
or against GitHub.

**Prepared:** 25 August 2026, against `main` at `852ef27` and
`recorder-seven-pairs-schema-2` at `e6ec1d1`.

**What this document is for.** `docs/context/Keel_SoW.pdf` and `docs/internal/`
were tracked while the repository was public. Untracking them stopped the
exposure growing and did nothing about what is already readable at earlier
commits. This runbook covers the two roads DEC-004 section 2 named, with their
real limits, so the choice is made against evidence rather than against a hope.

---

## 0. Read this before choosing a road

Four facts change the shape of the decision. Each is measured, and the command
that produced it is in the report accompanying this runbook.

**0.1 The repository was created public and has never been private.**

```
$ gh api repos/Keel-Official/keel-backend/events --jq '.[] | "\(.created_at) \(.type)"'
2026-08-18T15:47:52Z PublicEvent
...
$ gh repo view Keel-Official/keel-backend --json createdAt,visibility
{"createdAt":"2026-08-18T15:47:52Z","visibility":"PUBLIC"}
```

The `PublicEvent` timestamp is identical to `created_at` to the second. DEC-004
records the decision that the repository "stays **private** until the methodology
code passes the golden fixture" and marks it `IN FORCE from 20 August 2026, the
first commit`. That decision was never in force in fact. It was public two days
before the first commit landed. This is worth recording as its own finding, and
it is worth recording as a reversal in the DEC-004 line rather than by editing
DEC-004's text, per the repository's own rule on decision records.

**0.2 Nobody appears to have taken a copy.**

| Measure | Value |
|---|---|
| Forks | **0** (`gh api .../forks` returns `[]`) |
| Network count | 0 |
| Stars | 0 |
| Watchers / subscribers | 0 |
| Clones, 14 days | 68, **2 unique cloners** |
| Page views, 14 days | 3, 1 unique, referrer `github.com` |

The 68 clones are almost entirely GitHub Actions. CI runs three jobs, each doing
its own `actions/checkout`, so one run is three clones:

| Date | CI runs | Clones recorded | Ratio |
|---|---|---|---|
| 2026-08-20 | 11 | 37 | 3.4 |
| 2026-08-21 | 4 | 12 | 3.0 |
| 2026-08-23 | 4 | 13 | 3.3 |
| 2026-08-24 | 2 | 6 | 3.0 |

21 runs x 3 = 63 of the 68. The remainder is consistent with Al's own local
clones and fetches. The two "unique cloners" are best read as the Actions runner
fleet and Al's machine. **There is no evidence of a third-party copy.** That is
the number that decides how much this migration is worth, and it argues for the
cheaper road.

**0.3 The exposure window for the SoW PDF is closed. The one for `docs/internal/`
is still open right now.**

| Path | Entered history | Left `main` | Public window |
|---|---|---|---|
| `docs/context/Keel_SoW.pdf` | `f499ab4`, pushed 2026-08-20T13:53:36Z | `852ef27`, pushed 2026-08-25T09:36:40Z | ~4 days 20 hours, **closed** |
| `docs/internal/` (5 files) | `f499ab4`, pushed 2026-08-20T13:53:36Z | **still on `origin/main`** | **open, ongoing** |

```
$ git ls-tree -r --name-only origin/main -- docs/internal
docs/internal/Keel_Deliverable_1_Rencana_Eksekusi.md
docs/internal/audit-2026-08-20.md
docs/internal/deliverable-1-breakdown-2026-08-25.md
docs/internal/handoff-2026-08-21.md
docs/internal/memo-pra-development.md
```

The commit that untracks them, `7d31eb8`, is on the unmerged PR #2 branch. Until
PR #2 merges, or a separate commit lands on `main`, those five files are the
current contents of the public default branch. **This is the one thing in this
document that is worth doing today regardless of which road is chosen.** See
step 1.

**0.4 A history rewrite alone does NOT close this exposure.**

The budget figure and the funder's name live in files that no version of this
migration removes, because they are part of the deliverable:

| Marker | File that keeps it | Line |
|---|---|---|
| `126 hour, $2,268 budget` | `docs/decisions/DEC-004-visibilitas-repo.md` | 36 |
| `Keel_SoW.pdf` (filename) | `docs/decisions/DEC-004-visibilitas-repo.md` | 36 |
| `Keel_SoW.pdf` (filename) | `docs/decisions/DEC-005-english-as-repo-language.md` | 43 |
| `Keel_SoW.pdf` (filename) | `scripts/audit-verification.sh` | 415, 614 |
| `Instawards` | `docs/api/keel-openapi.yaml` | 31 |
| `Instawards` | `docs/api/Keel_PRD.md` | 5, 46 |

DEC-004 line 36 is the sharpest of these. It is a currently public file whose
own text discloses the exact commercial term it argues should not be public:

```
| `docs/context/Keel_SoW.pdf` | Contains the 126 hour, $2,268 budget and the terms
agreed with the funder. That is a document between Al and Instawards, not material
the public needs in order to assess the methodology |
```

**Treat this as a separate decision from the migration.** Redacting DEC-004 is an
amendment to a decision record, which the zone map permits Claude to draft; it is
not a history rewrite. If the figure is to stay public, then the migration's value
drops further, because the number the SoW protects is already out. If it is to be
redacted, that redaction must land BEFORE the rewrite, so that the rewrite carries
the redacted text and the old text goes with the old history. Doing it afterwards
means the figure is in the new history too and a second rewrite is needed.

---

## 1. Do this today, whichever road is chosen

Land the untracking of `docs/internal/` on `main`. It is the only open window.

```bash
# From a normal checkout. Nothing here rewrites history.
git switch main
git pull --ff-only

git rm -r --cached docs/internal
# Copy the /docs/internal/ block from the PR #2 branch's .gitignore, or:
printf '\n# Internal working documents. DEC-004 section 2.\n/docs/internal/\n' >> .gitignore

git add .gitignore
git commit -m "Untrack the internal working documents

DEC-004 section 2 requires these out before the repository is public. The
repository has been public since 2026-08-18, so this is late rather than
preventive, and git rm alone does not undo the exposure. The history rewrite
is tracked separately in scripts/history-migration/RUNBOOK.md."
git push origin main
```

Merging PR #2 achieves the same thing, since `7d31eb8` is on that branch. Either
is fine. The point is that it stops being tomorrow's problem.

---

## 2. Back up the things that are not recoverable

**Run this before either road. It is not optional.**

Ledger readings cannot be re-taken. A live Horizon reading of a ledger is only
available while that ledger is current, and nothing in this migration may drop
one.

```bash
# 2.1 The recordings on disk. These are gitignored and untracked, so they exist
#     ONLY on this machine. No clone, no fork, no branch has them.
mkdir -p ~/keel-migration-backup
cp -a recordings ~/keel-migration-backup/recordings
ls -la ~/keel-migration-backup/recordings/*/

# 2.2 A complete mirror of the repository as it stands, including every ref,
#     every PR ref, and every blob. This is the rollback for the whole migration.
git clone --mirror https://github.com/Keel-Official/keel-backend.git \
  ~/keel-migration-backup/keel-backend-preimage.git

# 2.3 The PR bodies and review threads, which no git operation preserves.
gh pr view 1 --json number,title,body,comments,reviews,mergedAt > ~/keel-migration-backup/pr-1.json
gh pr view 2 --json number,title,body,comments,reviews,state    > ~/keel-migration-backup/pr-2.json

# 2.4 The CI history, which is not in git at all.
gh run list --limit 200 --json databaseId,name,conclusion,createdAt,headSha \
  > ~/keel-migration-backup/ci-runs.json
```

**What is at stake in 2.1.** Four schema 1 recordings from 23 August:

```
recordings/USTRY.GCRYUGD5-USDC.GA5ZSEJY/64091884.json.gz
recordings/USTRY.GCRYUGD5-USDC.GA5ZSEJY/64091886.json.gz
recordings/USTRY.GCRYUGD5-USDC.GA5ZSEJY/64091961.json.gz
recordings/USTRY.GCRYUGD5-USDC.GA5ZSEJY/64093650.json.gz
```

Ledgers 64091884, 64091886, 64091961 and 64093650 closed on 23 August 2026. They
are gone from Horizon's live endpoints. These four files are the only copy. They
are not in git history (`recordings/` has never been tracked, confirmed against
the full-history path list), so no rewrite can touch them, but a `git clean -xdf`
or a switch to a fresh clone would.

---

## 3. The two roads, side by side

| | **Road A: `git filter-repo` + force push** | **Road B: fresh repository** |
|---|---|---|
| Removes blobs from the new history | Yes | Yes |
| Removes blobs from GitHub's storage | **No, not by itself** | Yes, if the old repo is deleted |
| Repository URL | Unchanged | New, or reused after deleting the old |
| Commit history (59 commits) | Preserved, every SHA rewritten | **Lost.** One clean initial commit |
| Stars (0) / watchers (0) | Kept | Lost, and both are zero |
| Issues (0) | Kept | Lost, and there are none |
| PR #1 (merged) and #2 (open) | Records survive; **diffs break**, SHAs orphan | **Lost entirely** |
| CI run history (27 runs) | Kept, pointing at SHAs that no longer exist | Lost |
| `recordings` branch | Does not exist yet on the remote either way | Same |
| Repository creation date | Preserved (2026-08-18) | Resets |
| Requires GitHub Support | **Yes, to purge cached views** | No |
| Requires Al's GitHub account | Force push | Create, delete, transfer |
| Effort | Half a day including the Support round trip | Two hours |

### 3.1 Road A's real limits, stated plainly

`git filter-repo` rewrites your local history. Force pushing replaces the branch
pointers on GitHub. **Neither deletes the old objects from GitHub.** The specific
ways the old content stays reachable:

1. **Orphaned commits remain reachable by direct SHA.** After the force push,
   `https://github.com/Keel-Official/keel-backend/commit/f499ab4` still resolves,
   and so does the blob view of `docs/context/Keel_SoW.pdf` at that commit.
   GitHub does not garbage-collect unreachable objects in a public repository on
   any schedule you control. Anyone who noted a SHA, and any tool that logged
   one, keeps working access.

2. **`refs/pull/*/head` and `refs/pull/*/merge` are not yours to rewrite.**
   GitHub creates these refs for every pull request and a force push to a branch
   does not touch them. PR #1's merge commit `aece0ee` and PR #2's head `e6ec1d1`
   stay reachable through those refs. `git ls-remote origin 'refs/pull/*'` shows
   them. Only GitHub can remove them.

3. **Cached views persist.** The commit list, the file blob view, the diff view
   and the raw endpoint on `raw.githubusercontent.com` are cached independently.
   A force push invalidates none of them reliably.

4. **Any fork keeps a complete copy.** Forks share GitHub's object store with the
   upstream, so a fork can serve objects the upstream no longer references. There
   are **zero forks here**, which is the single biggest reason Road A is viable
   at all. Re-verify immediately before executing (step 4.1).

5. **Third-party mirrors are out of reach entirely.** Nothing GitHub does affects
   an archiver, a package proxy, or a code-search index that already crawled the
   repository. The 3 recorded page views with referrer `github.com` and one
   unique visitor is the whole evidence base here; it suggests nothing crawled
   it, but it does not prove it.

**Do not treat Road A as complete without step 6 (GitHub Support).** A rewrite
plus force push, on its own, is a rewrite of what `main` points at and nothing
more.

### 3.2 Road B's real limits

1. **The commit history is itself deliverable evidence.** DEC-004 says so in as
   many words: "Road 2 is safer and loses the commit history, which is itself one
   of the pieces of evidence worth showing." 59 commits over five days, with the
   audit-and-repair pattern visible in the messages, is a substantial part of what
   a reviewer would look at. A single squashed initial commit shows none of it.

2. **The old repository still exists until Al deletes it.** Making it private is
   not the same as deleting it, and deleting it is irreversible and takes the PRs
   and CI history with it. Until it is deleted, Road B has removed nothing; it has
   only moved where the clean copy lives.

3. **PR #2 is open and would have to be re-created by hand** in the new
   repository, along with its description, which is substantial.

4. **Zero stars and zero forks means Road B's advantage is theoretical here.**
   Its advantage is that it leaves nothing orphaned. With nobody watching, that
   advantage buys less than the history it costs.

### 3.3 The recommendation, and the reasoning

**Road A, plus step 6, plus the DEC-004 redaction in step 0.4.** The evidence for
it: zero forks, zero stars, two unique cloners both explicable as machines Al
controls, three page views. The content at risk is a fee schedule and internal
working notes, not a credential; nothing needs rotating, and no third party is
harmed by a slow fix. Against that, Road B destroys 59 commits that DEC-004 itself
identifies as evidence for the paid deliverable.

If a fork appears before you execute, or if `gh api .../traffic/clones` shows
unique cloners rising without a matching CI run, that reasoning inverts and Road B
becomes correct. Re-run step 4.1 immediately before executing.

---

## 4. Road A, step by step

### 4.1 Re-verify the assumption Road A rests on

```bash
gh api repos/Keel-Official/keel-backend/forks --jq 'length'          # must be 0
gh api repos/Keel-Official/keel-backend --jq '.network_count'        # must be 0
gh api repos/Keel-Official/keel-backend/traffic/clones --jq '.uniques'
gh run list --limit 100 --json createdAt --jq '.[] | .createdAt[0:10]' | sort | uniq -c
```

If forks is not 0, **stop and switch to Road B.** If unique cloners exceeds what
the CI run count explains at three clones per run, stop and reconsider.

### 4.2 Install filter-repo and take a fresh clone

`git filter-repo` refuses to run on a repository that is not a fresh clone. That
refusal is a safety feature; do not override it with `--force` on your working
copy. Work on a throwaway clone so your working repository, and the untracked
`recordings/` inside it, are never touched.

```bash
brew install git-filter-repo      # or: pipx install git-filter-repo
git filter-repo --version

cd ~/keel-migration-backup
git clone https://github.com/Keel-Official/keel-backend.git keel-rewrite
cd keel-rewrite
git fetch origin '+refs/heads/*:refs/heads/*' --update-head-ok
git branch -a
```

### 4.3 Land the redactions from step 0.4 first

If DEC-004's budget line and the `Instawards` mentions are to be redacted, commit
that on `main` and push it **before** rewriting, so the rewrite carries the
redacted text. Skipping this means a second rewrite later.

### 4.4 Run the rewrite

```bash
cd ~/keel-migration-backup/keel-rewrite

git filter-repo \
  --path docs/context/ \
  --path docs/internal/ \
  --invert-paths
```

Optionally normalise the commit identity at the same time. Every one of the 59
commits carries `yazid.al2418@gmail.com`, which differs from the account address
and is a personal address baked into metadata that a rewrite is the only chance
to change:

```bash
# Only if you want this. Write the mapping first.
cat > ../mailmap <<'MAP'
Yazid Al Ghozali <54970485+yazidalg@users.noreply.github.com> <yazid.al2418@gmail.com>
MAP
git filter-repo --mailmap ../mailmap
```

### 4.5 Verify before pushing

```bash
bash /path/to/keel/scripts/history-migration/verify-clean.sh ~/keel-migration-backup/keel-rewrite
```

It must exit 0. If it exits non-zero it prints every marker it found and where.
Do not push until it is clean. Run its self-test first, so you know it can fail:

```bash
bash /path/to/keel/scripts/history-migration/verify-clean.sh --self-test
```

### 4.6 Restore the remote and force push

`git filter-repo` deletes the `origin` remote on purpose, so that a push is a
deliberate act rather than an accident.

```bash
cd ~/keel-migration-backup/keel-rewrite
git remote add origin https://github.com/Keel-Official/keel-backend.git

# Dry run first. Read the output.
git push --force --all --dry-run origin
git push --force --tags --dry-run origin
```

**Al executes the real push.** It is not in this document as a runnable line,
deliberately:

> Remove `--dry-run` from the two commands above and run them again.

Branch protection on `main`, if enabled, will reject the force push. Disable it,
push, re-enable it. That is an account action.

### 4.7 Confirm the rewrite landed and the orphans are still there

```bash
git ls-remote origin
gh api repos/Keel-Official/keel-backend/commits/f499ab4 --jq '.sha' 2>&1 || echo "gone"
git ls-remote origin 'refs/pull/*'
```

Expect `f499ab4` to **still resolve**. That is the point of step 6, not a failure
of step 4.6.

### 4.8 Everyone with a clone must re-clone

There is one other clone: Al's working repository. After the force push it holds
the old history and will try to reconcile. Do not merge or rebase it onto the new
history; re-clone, and copy `recordings/` back in from step 2.1.

```bash
mv ~/Development/Stellar-Funding/keel ~/Development/Stellar-Funding/keel-preimage
git clone https://github.com/Keel-Official/keel-backend.git ~/Development/Stellar-Funding/keel
cp -a ~/keel-migration-backup/recordings ~/Development/Stellar-Funding/keel/recordings
cp ~/Development/Stellar-Funding/keel-preimage/.claude/settings.local.json \
   ~/Development/Stellar-Funding/keel/.claude/ 2>/dev/null || true
```

---

## 5. Road B, step by step

Only if step 4.1 fails, or if you decide the orphan-by-SHA residue is
unacceptable.

```bash
# 5.1 Build a clean tree from the current main, with nothing sensitive in it.
mkdir -p ~/keel-migration-backup/keel-fresh
cd ~/keel-migration-backup/keel-fresh
git init -b main

# Copy the working tree from a clean checkout, excluding .git and the sensitive dirs.
rsync -a --exclude '.git' --exclude 'docs/context' --exclude 'docs/internal' \
      --exclude 'recordings' \
      ~/Development/Stellar-Funding/keel/ .

bash /path/to/keel/scripts/history-migration/verify-clean.sh . --tree-only
git add -A
git commit -m "Keel: liquidity risk engine for the Stellar ecosystem

Initial commit of a repository whose history was restarted. The previous
repository's history contained a third-party document and internal working
notes that could not be published; see DEC-004. The prior repository is
retained privately as an archive."
```

Then, as account actions Al performs in the GitHub UI or CLI:

> 1. Create the new repository under `Keel-Official`.
> 2. Add it as a remote and push `main`.
> 3. Re-create PR #2 by pushing its branch and opening a new pull request; its
>    description is saved at `~/keel-migration-backup/pr-2.json`.
> 4. Make the OLD repository private, verify the new one is complete, and only
>    then decide whether to delete the old one. Deleting is irreversible.

`docs/decisions/` must gain a record of this as a reversal of DEC-004's Road 1
preference, written as a reversal and not by editing DEC-004's text.

---

## 6. GitHub Support: the step that makes Road A complete

Road A is not finished at step 4.6. Open a Support ticket and ask for the
unreachable objects to be purged and the cached views invalidated.

> **Where:** https://support.github.com/contact
> **Category:** Account or repository, then "Sensitive data removal".

Suggested text, with the specifics filled in:

```
Repository: Keel-Official/keel-backend

I have rewritten this repository's history with git filter-repo and force
pushed, to remove a third-party document (docs/context/Keel_SoW.pdf) and a
directory of internal working notes (docs/internal/) that were committed while
the repository was public.

Please:

1. Garbage-collect the now-unreachable objects so that the pre-rewrite commits
   are no longer retrievable by direct SHA. The commits that introduced the
   content are f499ab4 and its descendants up to 852ef27.
2. Remove or refresh the cached views for those commits and for the blob
   docs/context/Keel_SoW.pdf.
3. Remove the pull request refs refs/pull/1/* and refs/pull/2/*, which a force
   push does not rewrite and which still reference the pre-rewrite commits.

There are no forks of this repository (network_count is 0), so no fork needs to
be considered.
```

Do not close the migration until Support confirms. Re-check afterwards:

```bash
gh api repos/Keel-Official/keel-backend/commits/f499ab4 2>&1 | head -3   # want a 404
```

---

## 7. Sequencing, with the recorder

The recorder is the one piece with a hard ordering constraint, because ledger
data is not recoverable retroactively.

**Current state, measured:**

| Fact | Value |
|---|---|
| `recordings` branch on the remote | **Does not exist.** `git ls-remote --heads origin` lists only `main`, `layers-around-the-engine`, `recorder-seven-pairs-schema-2` |
| `recordings/` ever tracked in git | **Never.** Zero hits across the full-history path list |
| Recordings on disk | 4 files, schema 1, 23 August, gitignored |
| `record.yml` | On the PR #2 branch only. Not on `main`, so GitHub has never registered it |
| Schedule | Commented out. No run has ever fired |

**What follows from that.** There is nothing on the remote for the migration to
drop. The `recordings` branch will be created by the first workflow run, and that
run has not happened. The four files on disk are outside git entirely. So the
recorder does not constrain the migration; the migration constrains the recorder.

**The order:**

| Step | Action | Why here |
|---|---|---|
| 1 | Land the `docs/internal/` untracking on `main` (section 1) | The only open window. Independent of everything else |
| 2 | Back up `recordings/` and mirror the repo (section 2) | Before anything destructive. The four files exist nowhere else |
| 3 | Redact DEC-004 and the `Instawards` mentions, if that is the decision (0.4) | Must precede the rewrite or it needs a second one |
| 4 | Merge or close PR #2 | A force push orphans its head. Deciding its fate before the rewrite is cheaper than reconstructing it after |
| 5 | Rewrite and force push (section 4), or migrate (section 5) | |
| 6 | Re-clone the working repository, restore `recordings/` (4.8) | |
| 7 | **Only now** enable the recorder | |
| 8 | GitHub Support (section 6) | Can run in parallel with 7 |

**Step 7 in detail.** After the migration, `record.yml` must be present on the
default branch before GitHub will register it at all; it currently is not, which
is why `gh run list --workflow=record.yml` returns `404: workflow record.yml not
found on the default branch`. Then:

> 1. Confirm `.github/workflows/record.yml` is on `main` in the migrated repo.
> 2. Trigger one `workflow_dispatch` run by hand and read the summary table.
>    That run creates the orphan `recordings` branch.
> 3. Confirm the branch exists and holds one round: `git ls-remote --heads origin`.
> 4. Only then uncomment the two `schedule:` lines and push.

**Do not enable the schedule before the rewrite.** A scheduled run during the
window would push commits to a branch that the rewrite then has to account for,
and worse, an hourly job writing to a repository mid-force-push is a good way to
lose a round. The recorder loses evidence permanently when a round fails, so the
correct order is: finish the migration, verify, then start recording.

**What happens to the four existing recordings.** They are schema 1, they are
gitignored, and they stay on disk through every step above provided section 2 is
done. They were never destined for git under the current `.gitignore`, which
re-includes only `recordings/samples/`. If they are meant to become the
`recordings/samples/` evidence that `migrations/0001_core.sql` and
`10-validation.md:112` promise, that is a separate decision about which readings
are representative, and it is a methodology question, not a migration step.

---

## 8. Zone map

`scripts/history-migration/` is a new directory holding files, and the coverage
check in `scripts/audit-verification.sh` (P2-9, `zones_incomplete`) reports any
directory with no row in the `CLAUDE.md` zone map. It will fail until a row is
added. `CLAUDE.md` is Claude's own brief, so the row is Al's to add. Suggested:

```
| `scripts/history-migration/` | GREEN | the clean-history migration runbook and its verifier. The runbook DESCRIBES destructive account actions and never performs one; every force push, repository deletion and Support request in it is Al's to execute |
```
