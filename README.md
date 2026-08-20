# Keel

Liquidity risk engine for the Stellar ecosystem.

An oracle answers "what is the price". Keel answers "what volume can that price
actually support".

## Where this stands

This repository is under construction and **the core of the methodology is not
implemented yet.** What exists: the methodology definitions, the API contract, a
golden fixture computed by hand from real on-chain data, the shared types, and
architecture tests that enforce package purity. What does not exist yet: the
formulas in `internal/depth`, the data adapters, storage, and the API.

What that means for the commands you can run:

| Command | State |
|---|---|
| `make test` | works, and must be green |
| `make ci` | works, and must be green |
| `make arch` | works, enforces purity of `internal/domain` and `internal/depth` |
| `make up` | works, starts local Postgres |
| `make conformance` | **red on purpose.** The golden fixture is a specification waiting to be met, and `internal/depth` is still empty |
| `make record` | **no body yet**, exits with code 3 |
| `make scan` | **no body yet**, exits with code 3 |
| `make serve` | **no body yet**, exits with code 3 |

Exit code 3 is deliberately distinct from 1 so that a scheduler can tell "not
built yet" apart from "failed".

## Starting from nothing

```bash
git clone https://github.com/Keel-Official/keel-backend.git
cd keel-backend

make ci          # gofmt, build, vet, architecture tests, and tests. Must be green
go run ./cmd/keel version

make up          # start local Postgres, optional at this stage
make migrate     # apply migrations/ in order, tracked in schema_migrations
```

`make migrate` is the only way migrations are applied. They are deliberately not
mounted into Postgres's `docker-entrypoint-initdb.d`, because that directory runs
only when the data directory is empty: it applies the first file on a fresh volume
and silently ignores every file after it.

To see what is still unsettled in this repository before contributing:

```bash
bash scripts/verifikasi-audit.sh
```

That script re-runs every claim in `docs/internal/audit-2026-08-20.md` and prints
which ones still hold. It also recomputes the golden fixture arithmetic from the
raw `price_r` values, outside Go, as a cross-check.

## Layout

| Directory | Contents | State |
|---|---|---|
| `cmd/keel` | single entrypoint, several subcommands | skeleton |
| `internal/domain` | shared types, chiefly `Snapshot`, no computation | present |
| `internal/conformance` | golden fixture and conformance tests, black-box against `internal/depth` | present, waiting on `internal/depth` |
| `internal/depth` | the core methodology, written by hand | **empty** |
| `internal/horizon` | live data adapter | empty |
| `internal/hubble` | historical data adapter, deferred, see DEC-002 | empty |
| `internal/store` | persistence | empty |
| `internal/api` | read-only HTTP handlers | empty |
| `migrations` | Postgres schema, applied with `make migrate` | present, reconciled with TDD section 5 |
| `docs/methodology` | the methodology deliverable | present |
| `docs/decisions` | decision records | present |
| `docs/api` | OpenAPI contract | present |
| `docs/evidences` | raw on-chain evidence from Horizon | present |
| `docs/learning` | learning journal | present |
| `testdata/fixtures` | golden fixture, computed by hand | present |
| `scripts` | one-off tools and verification | present |

## Non-negotiable rules

They live in `CLAUDE.md`, and some are enforced mechanically by
`internal/domain/arch_test.go`: no I/O in a pure package, no `float64`, no
`time.Now`, no goroutines. A rule that lives only in a document gets broken within
two weeks.

## Language

English, everywhere. See `docs/decisions/DEC-005-english-as-repo-language.md`.

## License

MIT.
