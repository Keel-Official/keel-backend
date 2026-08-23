# DEC-005: English as the Repository Language

**Decision:** Every human-readable string in this repository is written in English:
code comments, configuration comments, documentation, decision records, the OpenAPI
contract, and the golden fixture.
**Status:** IN FORCE from 20 August 2026.
**Supersedes:** the previous unwritten convention of Indonesian prose throughout.

---

## 1. Why

The reviewers this repository is built for do not read Indonesian. Deliverable 1
is judged from a public repository, a methodology document, and cross-validation
results, and the people reading those are SCF reviewers and the Ambassador
Chapter Lead. A methodology nobody can read cannot be defended, and "reproducible
by a third party" is the central claim of this project.

**What did not drive this decision.** The `misspell` linter flags Indonesian words
against a US dictionary, and that is what surfaced the question. It is not the
reason for the answer. A linter default is not a basis for choosing the language of
a paid deliverable, and had the answer been Indonesian, the correct fix would have
been to drop the linter, not to bend the prose.

**The rejected alternative.** Translate code comments only and leave the documents
in Indonesian. Rejected because it inverts the priority: reviewers open
`docs/methodology/` and `docs/api/`, not `internal/domain/types.go`. A repository
half in each language is worse than either language applied consistently.

**The cost, stated honestly.** The frontend builder tracked in DEC-003 section 6
was given an Indonesian OpenAPI contract. That contract is now in English, and the
four open questions in DEC-003 section 6 must be re-sent in English rather than
assumed to have carried over.

---

## 2. Scope

| Included | Excluded |
|---|---|
| Comments in `.go`, `Makefile`, `*.yml`, `*.sql`, `*.sh` | File and directory names, which stay as they are |
| `docs/` in full, including the methodology, decisions, PRD, and TDD | Git history: existing commit messages are not rewritten |
| `docs/api/keel-openapi.yaml` descriptions and examples | `docs/context/Keel_SoW.pdf`, a third-party document |
| `README.md`, `CLAUDE.md`, and every zone `CLAUDE.md` | Identifier names already in English |
| `testdata/fixtures/`, `.claude/commands/`, `.claude/hooks/` | Identifier names, see below |
| `docs/evidences/` annotations | `keel-bootstrap.sh`, see below |

**`keel-bootstrap.sh` is deliberately not translated.** It is the scaffolding
generator that produced this repository's initial files, and it carries its own
embedded copies of `CLAUDE.md`, `README.md`, and the zone files. Translating those
copies would recreate the exact failure this repository keeps hitting: two homes for
one definition, guaranteed to drift. It has already drifted. An archive header in
English was added at the top of the file explaining this, and the recommendation is
to delete the file, which is Al's call rather than Claude's.

**Deleted on 23 August 2026.** Al took the recommendation, so this exemption no longer
has a subject. The row above is kept rather than removed, because the reasoning is the
part worth keeping: an embedded copy of a file is a second home for it, and a second
home drifts. The git history holds the file if it is ever needed again.

**Identifier names are left for a separate decision, and this is a deliberate
limit, not an oversight.** Several identifiers are Indonesian: `belumSiap`,
`paketMurni`, `cocokTerlarang`, and the test names `TestArchTanpaImportTerlarang`,
`TestInvarianDeterminisme`, and others. Renaming them is a code change rather than
a prose change, and it is mechanically coupled: `Makefile` and
`.github/workflows/ci.yml` both select tests with `-run TestArch`, so a rename that
misses one of them silently stops running the architecture tests, and a test that
silently stops running is worse than a test written in the wrong language. It also
buys a reader of the methodology nothing. Decide it on its own terms; until then
the repository has English prose and a handful of Indonesian identifiers, and that
inconsistency is visible on purpose rather than hidden.

**File names are deliberately not renamed.** `09-flag-dan-band.md` keeps its name
even though its contents are now English, because six documents cross-reference it
by path and renaming multiplies the blast radius of this change for no gain to a
reader. Renaming is a separate decision if it is ever worth taking.

**One file cannot be translated by Claude.** `internal/depth/CLAUDE.md` sits in the
red zone, where Claude has no write permission by design. Al translates that one
by hand, or it stays in Indonesian as the single exception.

---

## 3. Glossary, binding

Translation without a fixed glossary drifts, and in a methodology document drift
changes meaning. These renderings are binding. Anything not listed here follows
the closest match in this table.

| Indonesian | English | Note |
|---|---|---|
| harga acuan | reference price | the quantity written `P0` |
| kedalaman efektif | effective depth | |
| biaya manipulasi | manipulation cost | `MC` in formulas |
| tercapai | reachable | already the field name `reachable` |
| batas atas / batas bawah | upper bound / lower bound | never "maximum" for a bound |
| aset dasar / aset quote | base asset / quote asset | |
| tangga delta | delta ladder | |
| pita di sekitar `P0` | price window | **not** "band", which is reserved |
| band risiko | risk band | `LOW`, `MEDIUM`, `HIGH`, `CRITICAL` |
| buku rusak | broken book | the `assetBrokenBook` case |
| trade asli | genuine trade | |
| jendela oracle | oracle window | |
| ketahanan oracle | oracle resistance | |
| ukuran collateral maksimum aman | maximum safe collateral | `C_max` |
| haircut likuidasi | liquidation haircut | |
| ambang | threshold | |
| dipilih, bukan dikalibrasi | chosen, not calibrated | appears verbatim in many places |
| masukan / keluaran | input / output | |
| kemurnian paket | package purity | the rule `arch_test.go` enforces |
| zona hijau / kuning / merah | green / yellow / red zone | |
| terpicu / bersih / tidak dapat dinilai | triggered / clear / unevaluated | the three flag states |
| likuiditas tercatat / eksekutabel | posted / executable liquidity | |
| sentuhan terakhir | final touch | the attacker's last, cheapest trade |
| melalap seluruh ask | sweep the entire ask side | |
| penyerang | attacker | |
| bukti | evidence | never "proof" unless it is one |
| keterbatasan yang diketahui | known limitations | |
| prinsip konservatif | conservative principle | |
| nol yang benar | a correct zero | the distinction the fixture turns on |

Two words to avoid entirely. **"Band"** never means a price range, only a risk
level. **"Proof"** is reserved for things actually proven on-chain; a reading from
a secondary source is "evidence" or "a report".

---

## 4. What this breaks, and how it is caught

`scripts/audit-verification.sh` verifies audit claims by grepping for Indonesian
sentences in the documents. Translating those sentences makes every such check
fail, and a failing check prints `NOT`, which reads as "this finding was fixed".
It would not be fixed. It would be invisible.

The script's own vocabulary changed with this pass: `TERBUKTI` became `PROVEN` and
`TIDAK` became `NOT`.

This is exactly the silent-failure class this project exists to catch, so it is
handled explicitly: every grep anchor in that script is updated in the same pass as
the document it points at, and the script's totals before and after the translation
must match. Before: 36 proven, 7 not. Any other number after translation means an
anchor was missed, not that a finding was resolved.

---

## 5. Order of work

Translation runs in this order, so that the terms are fixed before the prose that
depends on them:

1. This decision record and its glossary
2. Code and configuration comments
3. `README.md`, `CLAUDE.md`, and the zone files
4. `docs/methodology/`, the paid deliverable and the most delicate prose
5. `docs/decisions/` and `docs/api/keel-openapi.yaml`
6. `docs/internal/`, `docs/learning/`, `testdata/fixtures/`
7. `scripts/audit-verification.sh` anchors, then confirm the totals still read 36 and 7

---

## 6. What would reverse this

Only one thing: the frontend builder, or the Ambassador Chapter Lead, stating they
need Indonesian. If that happens, the answer is not to revert but to keep English
as the repository language and produce an Indonesian summary of the methodology as
a separate document, because the reviewer audience does not shrink.
