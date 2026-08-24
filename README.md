# Keel

Liquidity risk engine for the Stellar ecosystem.

An oracle answers "what is the price". Keel answers "what volume can that price
actually support".

## Where this stands

This repository is under construction and **the core of the methodology is not
implemented yet.** What exists: the methodology definitions, the API contract, a
golden fixture computed by hand from real on-chain data, the shared types,
architecture tests that enforce package purity, the live Horizon adapter with the
cross-validation recorder, the Postgres persistence layer, and the read-only API.
What does not exist yet: the formulas in `internal/domain/compute.go`, which are
declared and panic, plus the historical adapter, which DEC-002 defers.

**Every layer around the engine is now built, and the engine is the gap.** `keel
serve` answers all five endpoints today; what it has no rows to return is metrics,
and `keel scan` cannot produce one, because every function in the red zone panics.
That makes the API worth running before the engine exists: the frontend can build
against real 404s, real 503s and real headers rather than against mocks.

What that means for the commands you can run:

| Command | State |
|---|---|
| `make test` | works, and must be green |
| `make ci` | works, and must be green |
| `make arch` | works, enforces purity of `internal/domain` |
| `make up` | works, starts local Postgres |
| `make conformance` | **red on purpose.** The golden fixture is a specification waiting to be met, and every function in `compute.go` panics |
| `make record-once` | works, records one round of live Horizon snapshots and exits |
| `make record` | works, records every 30 minutes until stopped. Needs `PAIRS` |
| `make record-holders` | works, one round of pairs plus the trustline holder distribution of every base asset. `HOLDER_PAGES` raises the cap |
| `make survey` | works, prints one liquidity row per pair from Horizon. A triage instrument, not a measurement. Needs `PAIRS` |
| `make assets` | works, declares the demonstration set. Needs the database |
| `make serve` | works, serves every endpoint in the contract. Needs the database |
| `make store-test` | works, the `internal/store` integration tests. Needs the database |
| `make scan` | **wired, and produces nothing.** It reads the book, verifies the assets and opens a run row, then every asset panics inside `compute.go` and it exits with code 3. Needs the database |

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

## Recording cross-validation evidence, and why it starts before anything else

```bash
make record-once                       # one round, into recordings/
cp scripts/record-pairs.example.json my-pairs.json   # then edit it
make record PAIRS=my-pairs.json        # every 30 minutes, Ctrl-C to stop
```

Layer 3 of `docs/methodology/10-validation.md` compares a live Horizon reading of
a ledger against a reconstruction of that same ledger, and that is what satisfies
the SOW promise of cross-validation over 50 or more sample ledgers. The live half
has to be taken while the ledger is current. **It is the only work in this
repository that cannot be caught up later**, so the recorder was written before
the storage layer and before the API.

Each file is `recordings/{pair}/{ledgerSeq}.json.gz` and holds both the parsed
conclusions and the raw response bodies. Existing files are never overwritten.
The raw stream is not tracked by git; `recordings/sample/` is the exception,
because the schema's own header promises 60 recordings as committed evidence. See
the reason in `.gitignore`.

Which assets to record is decision D-1 and
`docs/methodology/02-pair-selection.md` is still a worksheet, so no asset list is
compiled into the binary. The `-pairs` file is data, and the shipped one is an
example rather than a selection.

### The holder distribution, which is worse than the order book in one way

```bash
make record-holders PAIRS=my-pairs.json                  # one round of pairs AND holders
make record-holders PAIRS=my-pairs.json HOLDER_PAGES=60  # raise the 5000 account cap
```

**Check the asset's real holder count before switching this on.** A reading that hits
the page cap is written, flagged, and logged as TRUNCATED, and it answers a holder
count as a lower bound and a concentration question not at all, because the account it
did not reach may be the largest one. Horizon's own figure is one request away and
`HOLDER_PAGES` above is how the cap is raised; the comment on that variable in the
`Makefile` carries the command.

For the long-running recorder, give holders their own cadence with
`-holder-interval`. Without one they are read on every `-interval` round, which sets
the pace of a balance that moves over days by the pace of a book that moves in
seconds, and this is the one recording here whose file size grows with the asset:

```bash
go run ./cmd/keel record -pairs my-pairs.json -interval 30m -holders -holder-interval 6h
```

`-holders` adds `recordings/holders/{asset}/{ledgerSeq}.json.gz`, one file per
ledger per BASE asset, holding every trustline holder, the issuer among them and
flagged, and the issued supply. `07-supporting-metrics.md` promises holder
concentration and a volume-to-supply ratio, and both are computed from exactly
this.

**The order book can be reconstructed from history and a trustline balance
cannot.** That is why the golden fixture's book is labelled `offers-implied`: it
was rebuilt from the operations that posted it. Horizon serves no historical
balance at all, by any route, so a holder concentration figure for a past ledger
is not recoverable from Horizon once the moment has passed. Hubble can answer it
and DEC-002 defers Hubble.

It is off by default, and that is a budget decision. A pair snapshot is three
requests whatever the market looks like; a holder reading is one request per two
hundred accounts, so a large asset can spend an hourly budget on its own. The cap
is `-holder-pages`, 25 pages or 5000 accounts by default, and a reading that hits
the cap says so in the file and in the log rather than quietly returning a
subset. A truncated reading is a lower bound on the holder count and answers no
concentration question at all, because the holder it is missing may be the
largest one.

Nothing computes these numbers yet. What counts as a holder is decision D-5 and
which supply the ratio uses is D-6, both in a worksheet that says of itself that
no definitions are recorded in it. The adapter therefore excludes nothing and
ranks nothing: it records what is there, so the decision can be applied later,
and applied both ways, against the same evidence.

## The database

```bash
make up && make migrate                  # start Postgres, apply migrations/
make assets PAIRS=my-pairs.json          # declare the demonstration set
make store-test                          # the integration tests, needs the above
```

`keel assets` is the only command that writes a result to the database today.
`keel scan` writes its run row and gets no further, because every asset panics
inside the red zone before there is anything to store. That row is the point of
running it anyway: `runs` records what a job attempted, so a scan that stored
nothing is visible rather than silent.

**If a connection fails with `role "keel" does not exist`**, port 5432 is being
answered by a Postgres that is not this project's container. A server already
running on the host takes the port first, and `make migrate` will not notice
because it goes through `docker compose exec`. Point the client at the container
explicitly:

```bash
make store-test KEEL_TEST_DSN="postgres://keel:keel_dev_only@<container-address>:5432/keel?sslmode=disable"
KEEL_DSN="..." make assets
```

To see what is still unsettled in this repository before contributing:

```bash
bash scripts/audit-verification.sh
```

That script re-runs every claim in `docs/internal/audit-2026-08-20.md` and prints
which ones still hold. It also recomputes the golden fixture arithmetic from the
raw `price_r` values, outside Go, as a cross-check.

## Layout

| Directory | Contents | State |
|---|---|---|
| `cmd/keel` | single entrypoint, several subcommands | skeleton |
| `internal/domain` | shared types in `types.go`, the methodology in `compute.go` | types present, `compute.go` declared and panicking |
| `internal/conformance` | golden fixture and conformance tests, black-box against `internal/domain` | present, waiting on `compute.go` |
| `internal/horizon` | live data adapter and the cross-validation recorder | present |
| `internal/hubble` | historical data adapter, deferred, see DEC-002 | empty |
| `internal/store` | Postgres persistence for assets, metrics and runs | present |
| `internal/api` | read-only HTTP handlers, five endpoints | present |
| `migrations` | Postgres schema, applied with `make migrate` | present, reconciled with TDD section 5 |
| `docs/methodology` | the methodology deliverable | present |
| `docs/decisions` | decision records | present |
| `docs/api` | OpenAPI contract | present |
| `docs/evidences` | raw on-chain evidence from Horizon | present |
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
