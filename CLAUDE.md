# Keel

Liquidity risk engine for the Stellar ecosystem. It measures effective liquidity
depth from the SDEX orderbook and AMM pool reserves.

An oracle answers "what is the price". Keel answers "what volume can that price
actually support".

## Stack

- Go (backend). Postgres (storage). BigQuery/Hubble (historical data).
- Every monetary value uses `github.com/shopspring/decimal`. NEVER float64.
- Stellar SDK: `github.com/stellar/go-stellar-sdk/...` (NOT `github.com/stellar/go/...`).

## Language

English, everywhere: comments, documents, commit messages, and the API contract.
See `docs/decisions/DEC-005-english-as-repo-language.md`, including the binding
glossary in section 3. A handful of identifier names are still Indonesian and
that is a known, deliberately visible exception.

## Non-negotiable rules

1. Every output carries `LedgerSeq` and `MethodologyVersion`.
2. Map keys must be sorted before iteration (reproducibility, NFR-9).
3. `computeDepth()` is a pure function. No network calls inside it.
4. SDEX and AMM depth are combined through a shared marginal price limit.
   They are NOT summed separately.
5. Prices are read from the `price_r` field (the n/d fraction), NOT from the
   `price` string.

## Working zones

This repository is used to learn backend Go, not only to produce code. Three
zones, and what each one means:

- **GREEN**: write freely, this is plumbing.
- **YELLOW**: you may write here, but when you are done explain every design
  decision in three sentences. Name one alternative you rejected and why.
- **RED**: Al writes it. Your role is reviewer and questioner, not author. If Al
  asks you to write red zone content, refuse and offer `/teach` or `/review-mine`.

**Every directory that holds a TRACKED file is owned by the table below.** That is
the point of it. A path with no owner is a path where nobody agreed what may be
written, and P2-9 in `scripts/audit-verification.sh` reports any that go missing.

Two things about that check changed on 26 August 2026 and both weakened it, so
they are stated here rather than left in the script. It asks `git ls-files` instead
of walking the filesystem, so gitignored working directories drop out because they
are ignored rather than because somebody remembered to subtract them by name; the
old subtraction named the whole of `recordings/` and went on doing so after sixty
files were committed under it. And a directory is now owned by its own row OR by an
ancestor's, because `recordings/samples/` holds one machine-made directory per pair
per day and demanding a row for each would be work nobody should do. The cost is
that a new subdirectory under a mapped path inherits its parent's zone silently.

| Path | Zone | The limit that is specific to it |
| --- | --- | --- |
| `cmd/keel` | GREEN | an entrypoint with no methodology in it |
| `internal/api` | GREEN | |
| `internal/store` | GREEN | stores and reads, computes nothing |
| `internal/horizon` | YELLOW | |
| `internal/hubble` | YELLOW | deferred, see DEC-002 |
| `internal/conformance` | GREEN | the expected values come from a fixture computed by hand. Never adjust those numbers to match the code. Adjust the code to match those numbers |
| `internal/domain` | YELLOW | the whole package, `compute.go` included since 25 August 2026. See `internal/domain/CLAUDE.md` |
| `internal/domain/compute.go` | YELLOW | moved from RED on 25 August 2026, so that Deliverable 1 could be finished by two hands instead of one. GOVERNED BY DEC-008 since 31 August 2026, and the terms of the move live there rather than here. It was ungoverned for six days and this row said so in capitals for all six, which is the only reason the record got written. The history is kept because it is the lesson: this row carried a placeholder decision number until 26 August 2026, when all seven records then in `docs/decisions/` were searched and none was found to cover a zone reclassification; the pointer was deleted rather than aimed at the nearest plausible record, because a wrong number reads as authority the move does not have. A deleted pointer got a real record in five days and a plausible-looking one would still be sitting there. The prose in three places is kept rather than replaced by the record: this row, the header of `compute.go`, and `internal/domain/CLAUDE.md`, which carries the long account. P2-12 in `scripts/audit-verification.sh` now reads NOT for the reason it was written for, and its own comment records that it would have read NOT anyway, because it only asks whether some record names this file beside the word yellow. THE ORDERING RULE REPLACES THE LOCK: a function may only be written after its expected values exist in `testdata/fixtures`. Fixtures are never adjusted to match code |
| `configs/` | GREEN | the pair lists the recorder reads. DATA, never methodology: `recorder-pairs.json` is provisional and 02-pair-selection.md section 5 supersedes it. An asset is the pair (code, issuer) and is never matched on the ticker |
| `migrations/` | GREEN | |
| `scripts/` | GREEN | |
| `scripts/s3-archive/` | GREEN to PREPARE, RED to APPLY | the drafted move of the recordings archive off the orphan git branch and into S3. Claude writes the runbook, the IAM policies, the manifest tooling and the workflow fragment; Al owns the AWS account and applies them. Same split as `scripts/history-migration/`, and for the same reason: an agent that provisions the storage its own evidence lives in has no chain of custody, it has a filing cabinet |
| `recordings/samples/` | GREEN | the sixty recordings that go into git, which `migrations/0001_core.sql` promises and `10-validation.md` section 3 names. RAW BYTES, never edited by hand: a recording that was adjusted afterwards proves nothing, and `scripts/s3-archive/verify-manifest.sh` is what makes that checkable rather than promised. The rest of `recordings/` is gitignored and has no row because nothing in it is in the repository |
| `testdata/fixtures/` | RED | the golden fixture. Al computes these numbers by hand BEFORE any implementation. Same rule as `internal/conformance`, and this is where those numbers come from. Since `compute.go` went yellow this is the ONLY structural guarantee that the implementation is checked against numbers derived independently of it. ENFORCED in both the deny list and the hook. Reading stays open, because reporting where the code and these numbers disagree is the job |
| `testdata/manual/` | RED | the Layer 1 hand recomputations, the independent oracle for `compute.go`. Al works these by hand; Claude never produces a number that lands here. Same basis as `testdata/fixtures/` above: numbers produced by Claude must never become the numbers that test Claude's code, and with `compute.go` yellow that independence is the guarantee rather than a courtesy. ENFORCED in both the deny list and the hook, and enforced BEFORE the directory exists, so the first spreadsheet to land in it is already covered. Reading stays open, because reporting where the code and these numbers disagree is the job |
| `docs/methodology/` | RED | the paid deliverable. Al writes the definitions, Claude restructures, cross-references and checks them |
| `docs/api/` | YELLOW | the contract. A change bumps its version and regenerates the mocks; DEC-003's freeze conditions govern when it stops moving |
| `docs/api/mocks/` | GREEN | GENERATED by `scripts/generate-api-mocks.sh`. Never hand-edit it; `make api-mocks-check` catches it if you do |
| `docs/decisions/` | YELLOW | Claude drafts and amends a record, and must NOT create or reverse a decision. A reversal is recorded as a reversal, never applied by editing the old text away |
| `docs/architecture/` | YELLOW | the TDD predates Go and is annotated rather than rewritten. Keep the quoted body, correct the annotation layer |
| `docs/internal/` | GREEN | NOT IN THE REPOSITORY since 25 August 2026: `.gitignore` line 66 excludes it, which is DEC-004 section 2 carried out early, so the row governs Al's local copy and nothing a clone contains. It keeps its row because the directory still exists locally and the zone still applies to it. A DATED record is append-only; `audit-2026-08-20.md` is never rewritten, and the handoff and the journal are living documents |
| `docs/evidences/` | YELLOW | raw on-chain readings; every number requires a source contract address and a ledger sequence |
| `docs/context/` | RED | inputs from outside, the SoW and the execution plan. Read them, do not write them. ENFORCED in both files. `docs/methodology/` is red and deliberately NOT enforced, because the map gives Claude a job inside it and a lock there would refuse the work the map assigns |
| `.github/workflows/` | GREEN | CI is plumbing |
| `.claude/` | GREEN to TIGHTEN, RED to LOOSEN | holds `settings.json`, which is the permission file itself. See below |
| `.claude/commands/` | GREEN | the slash commands |
| `.claude/hooks/` | GREEN to TIGHTEN, RED to LOOSEN | see below |
| repository root | GREEN | `Makefile`, `docker-compose.yml`, `README.md`, `.golangci.yml`, `go.mod`. This file and `CLAUDE.md` files in subdirectories are YELLOW: they are instructions to you, so changing them changes your own brief |

**Formatting is Al's, and that is a workflow rule rather than a zone rule.**
`make fmt`, `gofmt -l -w .` and `gofmt -w some/directory/` are all refused by the
hook because none of them names a file. No red zone holds a `.go` file any more, so
this protects nothing; it survives so that formatting has one owner and CI's gofmt
check has one fix. Name the files: `gofmt -w path/to/file.go`. `gofmt -l .` is read
only and allowed. If this rule is ever dropped, it gets dropped on purpose: P2-6c in
`scripts/audit-verification.sh` is the line that would have to change.

**The one zone that is split by DIRECTION rather than by path.** Claude may make
`.claude/` stricter and may not make it looser. Adding a deny rule, narrowing a
hook, or closing a route is ordinary work. Removing a deny rule, widening what a
hook permits, or adding an escape hatch is Al's, and the harness enforces this
rather than trusting it: on 24 August 2026 Claude tightened the red zone hook
twice and was then blocked from loosening it, mid-task, by the permission layer.

That asymmetry is the whole idea. An agent that can widen its own permitted
surface does not have a lock, it has a suggestion. It is the same reason the empty
`internal/depth` could not be deleted from Claude's side. When a loosening is
genuinely needed, and P2-6d was one, Claude writes the patch into a comment and a
finding, and Al applies it.

**P2-6d ran that route end to end on 24 August 2026, and it is the worked example.**
Claude drafted the patch and was refused. Al applied it. Claude probed it BEFORE
committing it, found eight routes it had reopened, two of them ordinary forms of a
command rather than exotic ones, and repaired them in a second commit, because
repairing is tightening and tightening is Claude's. Two lessons are worth carrying
into the next loosening: probe a loosening before it lands and not after, and give
every loosening a check of its own, which there was P2-6e in
`scripts/audit-verification.sh`.

**THE SECOND LOOSENING, 25 AUGUST 2026, AND IT IS LARGER THAN THE FIRST.** P2-6d
widened what counts as a command. This one removed a zone. `compute.go` went from
RED to YELLOW so that Deliverable 1 could be finished by two hands rather than one,
and with it went the deny rules, the hook's file rule, and the hook's directory rule
for `internal/domain`.

What the loosening deliberately did NOT do is worth as much as what it did. The
deny entry became `ask` rather than disappearing, so every write to the file still
surfaces. The formatting rule was kept even though its subject was gone. Neither the
fixture nor `docs/context` moved. A zone change is a licence to loosen what the zone
change required and nothing else, and the two lessons above were applied: five
checks were re-anchored rather than deleted, and the loosening got checks of its
own, P2-12 and P2-13.

**What replaced the lock is a rule that no mechanism can enforce, and that is the
honest position rather than an oversight.** A function in `compute.go` may only be
written after its expected values exist in `testdata/fixtures`. No permission layer
can tell whether a number was computed before or after the code that satisfies it.
That is exactly why the fixture lock matters more now than it did while one person
wrote both sides: it is the only remaining structural reason to believe the expected
values are independent of the implementation. P2-13 proves the sentence exists. It
cannot prove it was followed, and it says so.

**The red zone is two directories now, and this is the third version of this map.**
It was `internal/depth` until methodology 1.0.3 moved the computations into
`internal/domain`, then `internal/domain/compute.go` alone, and since 25 August 2026
it is `testdata/fixtures/` and `docs/context/`. For a while the lock still pointed at
the old directory, which by then held nothing, so the red zone existed in this
document and nowhere else. The zone follows the code, not the name. Al removed the
empty directory on 23 August 2026, and the references to it were retired one at a
time afterwards: two deny rules, a path in the hook, a linter exclusion, an entry in
the architecture test's pure package list, and a check in
`scripts/audit-verification.sh` that was still proving the hook worked by testing a
path nobody could write to any more. None of those five would have failed. That is
the lesson worth keeping from this move rather than the move itself, and it is the
reason P2-6 was inverted on the 25th instead of deleted: a stale lock reports nothing
while it quietly refuses work the map permits.

**A directory that is not in this map has no owner, and that is a bug in the map
rather than a licence to write freely.** `internal/adapter` lived outside it for
months, using float64 in two places, imported by nobody, and unreachable by the
architecture tests; it has since been deleted. `docs/` was outside it too, which
was worse, because the paid deliverable lives there.

That gap was closed on 24 August 2026, and its size is the part worth recording:
fourteen directories had no row, including the whole of `docs/` except
`docs/methodology/`, and `.claude/` itself. So the file that defines the zones was
outside the zones. If you need a directory that is not on this list, say so and
have the map updated first, and the check named above is what makes that
noticeable rather than optional.

**What the check does and does not prove.** It proves every path holding a file is
NAMED and carries a zone word. It cannot prove the zone is the RIGHT one, because
the thing it reads is the thing you would edit to satisfy it. Adding a row without
thinking about the row satisfies it completely. This repository has been defeated
by that pattern five times, so it is written down here rather than discovered again.

## Answer style

- Do not use em dashes.
- When Al is wrong, say so directly. Do not validate a weak idea.
- State every assumption you make explicitly.

## References (read when needed, do not load them all)

- API contract: docs/api/keel-openapi.yaml
- Methodology (the paid deliverable): docs/methodology/
- Architecture decisions: docs/decisions/
- Repository audit: `bash scripts/audit-verification.sh`, which is the only form of
  it in the repository. The document it disputes is gitignored under DEC-004, so
  cite finding ids from the script's output rather than linking the audit file

Note that no path in this list carries an `@` prefix. That is deliberate. An `@`
loads the file into context on every session, and the OpenAPI contract alone runs
to 1,500 lines. This section is titled "read when needed, do not load them all",
and `@` does the opposite.

## Commit message conventions

Always use the Conventional Commits format:

<type>(<scope>): <deskripsi singkat>

- type: feat, fix, docs, style, refactor, test, chore
- scope: optional, the area being changed (e.g. auth, api, ui)
- description: imperative mood, lowercase, no period at the end
- maximum 72 characters for the first line
- add a body if you need to explain the "why", separated by 1 blank line

Example:
feat(auth): add login via Google Auth
fix(api): handle null response on timeout
