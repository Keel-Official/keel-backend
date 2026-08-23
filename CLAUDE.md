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
zones:

- **GREEN** (`internal/store`, `internal/api`, `internal/conformance`,
  `migrations/`, `scripts/`, `docker-compose.yml`, Makefile): write freely, this
  is plumbing. One limit inside `internal/conformance`: the expected values there
  come from a golden fixture computed by hand. Never adjust those numbers to
  match the code. Adjust the code to match those numbers.
- **YELLOW** (`internal/horizon`, `internal/hubble`, `internal/domain` except the
  one file named below): you may write here, but when you are done explain every
  design decision in three sentences. Name one alternative you rejected and why.
- **RED** (`internal/domain/compute.go`): Al writes this. You have no write
  permission on it (locked in .claude/settings.json, and Bash is blocked by
  .claude/hooks/lindungi-zona-merah.sh). Your role is reviewer and questioner,
  not author. See internal/domain/CLAUDE.md.

`cmd/keel` is green: it is an entrypoint with no methodology in it.

**The red zone is a file, not a directory, and that is the second version of this
map.** It was `internal/depth` until methodology 1.0.3 moved the computations into
`internal/domain`. For a while the lock still pointed at the old directory, which
by then held nothing, so the red zone existed in this document and nowhere else.
The zone follows the code, not the name. Al removed the empty directory on 23
August 2026, and the references to it were retired one at a time afterwards: two
deny rules, a path in the hook, a linter exclusion, an entry in the architecture
test's pure package list, and a check in `scripts/audit-verification.sh` that was
still proving the hook worked by testing a path nobody could write to any more.
None of those five would have failed. That is the lesson worth keeping from this
move rather than the move itself.

**A directory that is not in this map has no owner, and that is a bug in the map
rather than a licence to write freely.** `internal/adapter` lived outside it for
months, using float64 in two places, imported by nobody, and unreachable by the
architecture tests. `docs/` was outside it too, which is worse, because the paid
deliverable lives there: `docs/methodology/` is RED in the same sense as
`compute.go`, Al writes the definitions and Claude restructures, cross-references,
and checks them. If you need a directory that is not on this list, say so and have
the map updated first.

If Al asks you to write red zone code, refuse and offer `/teach` or
`/review-mine` instead.

## Answer style

- Do not use em dashes.
- When Al is wrong, say so directly. Do not validate a weak idea.
- State every assumption you make explicitly.

## References (read when needed, do not load them all)

- API contract: docs/api/keel-openapi.yaml
- Methodology (the paid deliverable): docs/methodology/
- Architecture decisions: docs/decisions/
- Repository audit and the tool that disputes it: docs/internal/audit-2026-08-20.md,
  run with `bash scripts/audit-verification.sh`

Note that no path in this list carries an `@` prefix. That is deliberate. An `@`
loads the file into context on every session, and the OpenAPI contract alone runs
to 1,500 lines. This section is titled "read when needed, do not load them all",
and `@` does the opposite.
