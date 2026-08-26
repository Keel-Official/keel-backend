# Keel

Liquidity risk engine for the Stellar ecosystem.

An oracle answers "what is the price". Keel answers "what volume can that price
actually support".

## Where this stands

This repository is under construction. **The depth and manipulation engine computes
as of 26 August 2026**, and `make conformance` passes against a golden fixture whose
numbers were computed by hand before any implementation existed.

What exists: the methodology definitions, the API contract, that golden fixture, the
shared types, the depth, manipulation cost, reference price, collateral and flag
computations, architecture tests that enforce package purity, the live Horizon
adapter with the cross-validation recorder, the Postgres persistence layer, and the
read-only API.

What does not exist yet, and the list is specific on purpose:

- **The supporting metrics.** Holder concentration, the volume-to-supply ratios and
  the time since the last genuine trade are declared, stored and served, and none of
  them is computed. Their DEFINITIONS are not written either:
  `docs/methodology/07-supporting-metrics.md` is still a worksheet. Every result
  reports the six flags that depend on them as `unevaluated`, which is not the same
  claim as clear.
- **Layer 3 at the full sample size.** `keel crosscheck` executes it and the first
  run compared 37 of the 60 recordings with zero mismatches; the SOW asks for at
  least 50. Nothing is broken: seven hours passed between the recording and the
  comparison and 23 pairs had an offer move in the gap, four of them exactly one
  offer. `docs/evidences/2026-08-26-layer3-crosscheck.md` section 4 has the fix,
  and it is a step in the recorder workflow rather than code.
- **Historical POOL reserves.** `keel replay` rebuilds a past ORDER BOOK from the
  operations that posted it, and reconstructs no pool at all, so the snapshot it
  returns carries none. That is not a claim that no pool existed.
  `/liquidity_pools/{id}/operations` can answer it and DEC-002 section 2.3 calls
  that side the cleaner of the two, because it has no account discovery gap. Until
  it is written, any depth computed from a replayed snapshot is order book only.
- **Hubble.** Still deferred, DEC-002. What changed on 26 August 2026 is that it is
  no longer the only route to a past book: `keel replay` is DEC-002 section 2.3,
  and section 2.3's own precondition, "only attempt this if 2.1 and 2.2 prove
  insufficient", was met and measured in
  `docs/evidences/2026-08-26-ustry-february-trades-implied.md`.
- **A hand computed check on the AMM half.**
  `testdata/fixtures/ustry_pre_exploit.md` records `Pools: []` while the pool that
  genuinely existed at that ledger is in `GoldenSnapshot()`, so the with-pool depth
  and manipulation tables have no expected values yet. The AMM formulas are
  implemented from the methodology and checked only by invariants, and the header of
  `internal/domain/compute.go` says which functions are in that position.

What that means for the commands you can run:

| Command | State |
|---|---|
| `make test` | works, and must be green |
| `make ci` | works, and must be green |
| `make arch` | works, enforces purity of `internal/domain` |
| `make up` | works, starts local Postgres |
| `make conformance` | **green since 26 August 2026.** Fourteen tests against the golden fixture. The build tag came out the same day, so these also run inside `make test` and CI; this target runs the package alone and verbosely |
| `make record-once` | works, records one round of live Horizon snapshots and exits |
| `make record` | works, records every 30 minutes until stopped. Needs `PAIRS` |
| `make record-holders` | works, one round of pairs plus the trustline holder distribution of every base asset. `HOLDER_PAGES` raises the cap |
| `make survey` | works, prints one liquidity row per pair from Horizon. A triage instrument, not a measurement. Needs `PAIRS` |
| `make assets` | works, declares the demonstration set. Needs the database |
| `make serve` | works, serves every endpoint in the contract. Needs the database |
| `make store-test` | works, the `internal/store` integration tests. Needs the database |
| `make scan` | works, computes and stores one result per asset per ledger. The supporting metric fields are stored null, because they are not computed yet. Needs the database |
| `make backtest` | works, writes the trade-implied history of a pair as two CSV files. Needs `PAIRS`, `FROM`, `TO`. No database |
| `make replay` | works, rebuilds a pair's order book at a past ledger from the operations that posted it. Needs `PAIRS` and `LEDGER`. No database. **Read the completeness line it prints**: a book missing an offer reads as a thin book |
| `make crosscheck` | works, runs validation Layer 3 over the committed recordings. No database. First run, 26 August 2026: 60 recordings, 37 match, 0 mismatch, 23 partial |

Exit code 3 is deliberately distinct from 1 so that a scheduler can tell "not
built yet" apart from "failed". **No subcommand means "not built yet" any more**, as
of 26 August 2026, when `keel replay` got a body and the helper that printed that
line went with it. `keel scan` still uses the code, for a case that is not unbuilt
and reads the same way to a scheduler: a round where every asset panicked has
nothing to store.

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
make record-once                                     # one round, into recordings/
make record-once PAIRS=configs/recorder-pairs.json   # the eight provisional pairs
cp scripts/record-pairs.example.json my-pairs.json   # then edit it
make record PAIRS=my-pairs.json                      # every 30 minutes, Ctrl-C to stop
```

`configs/recorder-pairs.json` holds eight pairs, all quoted in USDC, and it is
PROVISIONAL: `docs/methodology/02-pair-selection.md` section 5 supersedes it once
written, and nothing in it is a methodology claim. It is what
`.github/workflows/record.yml` records hourly.

Layer 3 of `docs/methodology/10-validation.md` compares a live Horizon reading of
a ledger against a reconstruction of that same ledger, and that is what satisfies
the SOW promise of cross-validation over 50 or more sample ledgers. The live half
has to be taken while the ledger is current. **It is the only work in this
repository that cannot be caught up later**, so the recorder was written before
the storage layer and before the API.

Each file is `recordings/{pair}/{date}/{ledgerBefore}.json.gz` and holds ONLY the
raw response bodies: the order book and the liquidity pools, each with the exact
URL requested, the HTTP status, the body verbatim as a string, and that body's
sha256. It parses nothing and converts nothing. Nothing is ever overwritten; a
name already taken gets a monotonic suffix.

That is recording schema 2, and it is the default. Schema 1 wrote
`recordings/{pair}/{ledgerSeq}.json.gz` and held the parsed conclusions beside
the bytes; it is still reachable with `-schema 1` and every file it wrote stays
readable, but the parsed half is the half that had to be revised once already,
when the bid amount unit turned out to be quote-denominated. A recording that
claims nothing cannot go stale that way.

An empty pool list and a non-2xx are both recorded and kept. The recorder makes
no judgement about data quality, and `ledger_consistent` says whether the two
requests were served from the same ledger rather than hiding it.

The raw stream is not tracked by git; `recordings/samples/` is the exception,
because the schema's own header promises 60 recordings as committed evidence. See
the reason in `.gitignore`. That directory is PLURAL because
`docs/methodology/10-validation.md` names it that way, and it is the deliverable.

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

`keel assets` declares the demonstration set and `keel scan` fills it in: one metrics
row per asset per ledger, plus a `runs` row recording what the job attempted. `runs`
exists so a scan that stored nothing is visible rather than silent, which is worth as
much now that scans do store something as it was while they could not.

**The container is published on 5433, not 5432.** A Postgres already installed on
the host takes 5432 before the container can, `make migrate` does not notice
because it goes through `docker compose exec`, and the symptom is `role "keel"
does not exist` rather than a refused connection. The published port moved on 26
August 2026 and `DefaultDSN` moved with it, so the defaults need no argument.

**If a connection still fails with `role "keel" does not exist`**, something on
this machine is answering 5433 too. `make store-test` runs a preflight before the
suite and says which it is, rather than failing 31 tests identically. Point the
client at the container explicitly:

```bash
make store-test KEEL_TEST_DSN="postgres://keel:keel_dev_only@<container-address>:5433/keel?sslmode=disable"
KEEL_DSN="..." make assets
```

To see what is still unsettled in this repository before contributing:

```bash
bash scripts/audit-verification.sh
```

That script re-runs every claim of the repository audit and prints which ones still
hold. The audit document itself is not in the repository and there is no point
looking for it: `docs/internal/` is gitignored, because DEC-004 requires it out
before the repository goes public. So the script IS the public form of the audit.
Every line it prints carries its own finding id, and those ids are what the
decision records in `docs/decisions/` cite. It also recomputes the golden fixture
arithmetic from the raw `price_r` values, outside Go, as a cross-check.

## Layout

| Directory | Contents | State |
|---|---|---|
| `cmd/keel` | single entrypoint, several subcommands | skeleton |
| `internal/domain` | shared types in `types.go`, the methodology in `compute.go`, the flag and band rules in `flags.go` | present. The supporting metric formulas are not written, and neither are their definitions |
| `internal/conformance` | golden fixture and conformance tests, black-box against `internal/domain` | present and green |
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
