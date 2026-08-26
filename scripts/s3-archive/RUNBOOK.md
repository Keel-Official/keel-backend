# Moving the recordings archive to S3

**Status: PREPARED, NOT APPLIED. Nothing in this directory runs by itself.**
`.github/workflows/record.yml` is untouched and still writes to the orphan
`recordings` branch every hour.

Prepared 26 August 2026 by Claude, at Al's request. Same division as
`scripts/history-migration/`: the patch is written here, the account is Al's, and
Al applies it. Claude cannot create an AWS account, cannot hold a role ARN, and
must not be the party that decides where the deliverable's evidence lives.

---

## 1. Read this before deciding, because the recommendation is "not yet"

Measured from real recordings on 26 August 2026, not estimated:

| | 8 pairs hourly, today |
|---|---|
| One round | 24.4 KB, 8 files |
| One month | ~17.6 MB, ~720 commits |
| One year | ~214 MB |
| S3 cost | storage rounds to a fraction of a cent; ~5,760 PUTs a month. **A few cents a month** |

**Cost is not the argument, in either direction.** 18 MB a month does not strain
git and a few cents does not strain a budget. What changes is the nature of the
evidence:

| | orphan git branch | plain S3 |
|---|---|---|
| Every recording has a hash anyone can recompute | yes, that is what a git blob is | no |
| A chain proves nothing was edited afterwards | yes | no, unless Object Lock, and the verifier must then trust AWS IAM instead of arithmetic |
| A third party verifies with no credentials | yes, the repo is public | only with a public bucket |
| The evidence outlives a paid account | yes | no |

The SOW names **a public repository link** as Deliverable 1 evidence. For an
archive whose whole job is to prove that a price existed at a ledger, a stranger
being able to recompute the hash themselves is the product, not a detail.

**So: do not move for cost or tidiness.** Move when volume actually bites:

- cadence rises to 60 pairs every 30 minutes, roughly 173 MB a month
- `-holders` is switched on, where file size grows with the asset's trustline count
  instead of staying flat
- retention is meant to outlast the sprint by a year or more

## 2. The design, and the one part that matters

**S3 holds the bytes. Git holds the hashes.**

Section 6 below writes a `sha256` manifest for every round and commits that to the
orphan branch instead of the gzipped blobs. This is not a nicety, it is the whole
reason the move is acceptable at all: without it, S3 objects are bytes a reviewer
has to take on trust, and the archive stops being evidence and becomes storage.

It also cuts git growth by roughly 24 times, from ~18 MB a month of blobs to
~740 KB a month of text, which is the practical benefit people usually reach for
S3 hoping to get.

A reviewer's path stays: clone the public repo, read the manifest, fetch the object
from the public bucket, hash it, compare. No AWS account needed on their side.

## 3. What Al decides, and it is only these

| Decision | Why it cannot be Claude's |
|---|---|
| Whether to move at all | Section 1. It is a judgement about the deliverable's evidence |
| AWS account and region | Al's account |
| Bucket name | Global namespace, must be chosen once and never changed |
| Public read, or private with a signed-URL story | Public read makes reviewer verification free and exposes the bucket to request costs from the internet. Private keeps costs bounded and makes a stranger's verification impossible without help. The SOW's "public repository" wording points at public read |
| Object Lock retention period | A compliance-mode lock cannot be shortened afterwards, not even by the root account. That is the point and it is also the risk |

## 4. Steps, in order

Placeholders to substitute: `<ACCOUNT_ID>`, `<BUCKET>`, `<REGION>`, `<ROLE_NAME>`.

**4.1 Create the bucket with versioning and Object Lock.** Object Lock can only be
enabled at creation time on a new bucket, so this is the one step with no second
chance.

```bash
aws s3api create-bucket --bucket <BUCKET> --region <REGION> \
  --create-bucket-configuration LocationConstraint=<REGION> \
  --object-lock-enabled-for-bucket
aws s3api put-bucket-versioning --bucket <BUCKET> \
  --versioning-configuration Status=Enabled
aws s3api put-object-lock-configuration --bucket <BUCKET> \
  --object-lock-configuration '{"ObjectLockEnabled":"Enabled","Rule":{"DefaultRetention":{"Mode":"COMPLIANCE","Days":365}}}'
```

**4.2 Register GitHub as an OIDC identity provider**, once per AWS account.

```bash
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com
```

**4.3 Create the role** with `github-oidc-trust-policy.json` in this directory as
its trust policy, and `recorder-iam-policy.json` as its only attached policy.

```bash
aws iam create-role --role-name <ROLE_NAME> \
  --assume-role-policy-document file://scripts/s3-archive/github-oidc-trust-policy.json
aws iam put-role-policy --role-name <ROLE_NAME> --policy-name recorder-put-only \
  --policy-document file://scripts/s3-archive/recorder-iam-policy.json
```

**4.4 Apply the workflow change.** `record-s3.yml.fragment` replaces three steps in
`.github/workflows/record.yml` and leaves the rest alone. The diff is roughly
minus 80 lines, plus 25.

**4.5 Probe it before trusting it, on a branch, with `workflow_dispatch`.** This is
the P2-6d lesson and it is the one step most likely to be skipped: the first
loosening of the permission layer was probed before it was committed and the probe
found eight reopened routes. Run one round, then check that the object exists, that
the manifest line matches it, and that a second run does NOT overwrite the first.

**4.6 Run both writers in parallel for a week.** Keep the orphan-branch commit AND
the S3 upload. If the two disagree about a single byte, the migration is wrong and
you still have the branch. Only then remove the branch writer.

## 5. Verifying it, which is step 4.5 spelled out

```bash
# The object landed
aws s3 ls s3://<BUCKET>/recordings/ --recursive | head

# It matches what git says it should be
bash scripts/s3-archive/verify-manifest.sh recordings/manifest/2026-08-26.sha256 <BUCKET>

# An overwrite is refused rather than silently accepted
aws s3 cp /dev/null s3://<BUCKET>/recordings/<an-existing-key>   # must fail
```

## 6. The manifest

`manifest.sh` writes one line per file in `sha256sum` format, so `sha256sum -c` and
`shasum -a 256 -c` both read it with no custom tooling. `verify-manifest.sh` fetches
each object and checks it.

The manifest is committed to the orphan `recordings` branch, one file per day,
appended per round. That branch already exists for exactly this purpose and shares
no history with `main`, so the review branch stays clean.

## 7. What this does NOT change

- `recordings/samples/` stays in git. Sixty files, 120 KB, the published sample and
  the thing a reviewer reads first. It is not worth moving and moving it would cost
  the deliverable its most legible piece of evidence.
- `keel record` is untouched. It writes files to a directory; where that directory
  ends up is the workflow's business and not the recorder's.
- `.gitignore` keeps excluding `/recordings/*` with the `samples/` negation. A
  manifest directory has to be re-included the same way when step 4.4 lands.
