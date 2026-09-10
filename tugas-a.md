# Tugas A: the live service

**You are holding one of two work tracks.** The other one is `tugas-b.md`. They were
split so that two people can work at the same time without ever editing the same file
and without waiting for each other. If you read only this file you will still know
exactly what to do.

**Both files are scored against the same source:** the Statement of Work, the document
the client is paying against. Every work item below quotes the SOW sentence it serves,
so you can always answer "why am I doing this".

---

## 1. Sixty seconds on what Keel is

An oracle answers **"what is the price"**. Keel answers **"what volume can that price
actually support"**.

In February 2026 an attacker pushed the price of a Stellar asset called USTRY up 100
times using a market so thin that moving it cost almost nothing, borrowed 61 million
dollars of XLM against the inflated collateral, and the ecosystem lost 10 million.
The oracle was not wrong about the price. Nobody had a number for **how much real
liquidity stood behind that price**. Keel produces that number.

Concretely, Keel reads the Stellar order book and AMM pools, and for each asset it
reports how much you could actually buy or sell before moving the price 2%, 5% or 10%,
how much it would cost an attacker to move it, how concentrated the holders are, and a
recommended maximum safe collateral size. It publishes that through a read-only API.

**Keel never signs or submits a transaction.** There is no signing code anywhere in the
repository and there never will be. It only reads.

---

## 2. Which half is yours

The two tracks are split by **what the work is for**, and that happens to also split
cleanly by file, which is what keeps you out of each other's way.

| | **Tugas A, yours** | Tugas B, the other file |
|---|---|---|
| One sentence | Make Keel something a stranger can open and use in five minutes | Make Keel's claim about February 2026 true and checkable |
| The question it answers | "Can I use this today?" | "Was this ever right?" |
| Its shape | a running service | a document with evidence under it |

**Yours in one line:** the API goes live on the internet, every asset it serves carries
its full set of metrics instead of six blank ones, the asset list is the real
demonstration set, and there is a video showing it working.

Your estimated load is **about 38 hours**, which is roughly the 8 days remaining at the
SOW's own pace of 4.6 hours per builder per day. Track B is about 36.

---

## 3. Getting running, ten minutes

You need Docker and Go 1.23. Nothing else.

```bash
git clone https://github.com/Keel-Official/keel-backend.git
cd keel-backend

make up                 # Postgres in Docker, on localhost:5433 (not 5432, see below)
make migrate            # apply the schema. This is the ONLY way migrations are applied
make ci                 # vet, architecture tests, 339 tests with -race, linter. Must be green
make serve              # the API on http://localhost:3000
```

Then, in another terminal:

```bash
curl -s localhost:3000/v1/health | python3 -m json.tool
```

You will get `"status": "degraded"` and `"assetsMonitored": 0`. **That is correct, not
broken.** The health endpoint reports degraded when no scan has ever run, because an API
serving nothing must not report ok. To give it something:

```bash
make scan               # reads Horizon, computes, stores. Ctrl-C to stop
```

**The one trap that has already cost this project a day.** Postgres is published on host
port **5433**, not 5432. If you already have a Postgres on your machine it owns 5432, and
a DSN pointing there will reach the wrong server and fail with `role "keel" does not
exist` rather than with a connection error. The long comment at the top of
`docker-compose.yml` is the full story.

Useful reading, in this order: `README.md`, then `CLAUDE.md` (the working rules), then
`docs/methodology/00-overview.md`.

---

## 4. Rules you cannot break

These are not style preferences. Each one exists because breaking it already caused a
real failure here.

1. **Never use `float64` for money.** Every monetary value uses
   `github.com/shopspring/decimal`. An architecture test scans the whole repository and
   fails the build if a float appears in the wrong place.
2. **Every output carries `LedgerSeq` and `MethodologyVersion`.** A number without the
   ledger it came from and the method that produced it is not a result, it is a rumour.
3. **Sort map keys before iterating.** Go randomises map order. Two runs of the same
   input must produce byte-identical output.
4. **Read prices from the `price_r` field**, the `n/d` fraction, never from the `price`
   string. The string is rounded.
5. **SDEX and AMM depth are combined through a shared marginal price limit.** They are
   never computed separately and added.
6. **`testdata/fixtures/` and `testdata/manual/` are locked, and reading them is fine.**
   They hold numbers computed BY HAND before the code existed. If the code and those
   numbers disagree, the disagreement is the finding: report it, fix the code, and never
   edit the numbers. Neither directory is yours in this track anyway.
7. **You may make `.claude/` stricter, never looser.** Adding a permission rule is
   ordinary work; removing one is not yours. This is enforced by the harness, not by
   trust.
8. **English everywhere** in code, comments, documents and commit messages.
9. **Commit messages use Conventional Commits**: `feat(scope): imperative lowercase
   subject`, 72 characters max on the first line, body explaining the WHY.

**One more, and it is the most important one in this repository.** There is a written
ordering rule: a function may only be written after its expected values exist as hand
computed numbers in `testdata/`. No tool can enforce it, because no permission system can
tell whether a number was computed before or after the code that satisfies it. It is
still the rule. In practice, for your track: do not invent an expected value to make a
test pass.

---

## 5. What is already done, so you do not redo it

| Already working | Where |
|---|---|
| Order book and AMM pool readers, live against Horizon | `internal/horizon/` |
| Depth at 2%, 5%, 10%, manipulation cost, reference price, collateral sizing, flags and bands | `internal/domain/` |
| Postgres persistence and the read-only API, 5 endpoints | `internal/store/`, `internal/api/` |
| The API contract, 84 KB of OpenAPI, plus generated mocks the dashboard was built against | `docs/api/` |
| Methodology, twelve documents | `docs/methodology/` |
| A container image, smoke tested in CI and published to `ghcr.io` on every version tag | `Dockerfile`, `.github/workflows/deploy.yml` |
| 339 tests, race detector, zero linter findings | `make ci` |

Current score against the SOW: Deliverable 1 about 89%, Deliverable 2 about 59%,
Deliverable 3 about 38%. Weeks 1 and 2 are complete. Your track is most of what is left
of Deliverable 3 and the "live" half of Deliverable 2.

---

## 6. Your work items

Each item gives you the SOW sentence, why it matters, what done looks like, and how to
prove it. Do them in this order: A1 first because it lifts every other number, A2 and A3
next because they are what makes the work visible at all.

Items marked **AL ONLY** need something no engineer can supply: an account, a secret, or
a judgement the client is paying a person to make. Do not attempt those; prepare them and
hand them over.

---

### A1. Make every asset report all of its metrics

**SOW, Deliverable 1:** *"For each asset, it calculates effective depth at +/-2%, +/-5%,
and +/-10%, holder concentration, volume-to-supply ratio, times since the last genuine
trade, and a recommended maximum safe collateral size."*

**Why this is first.** Three of those seven things are computed nowhere in the running
system. Not missing, not broken: **written, tested, given database columns, and given API
fields, and then never called.** The scan loop calls the order-book-only variant of the
compute function.

```
cmd/keel/scan.go:269   risk, err = domain.ComputeAssetRisk(s, p)
                                          ^ the variant with no supporting metrics

internal/domain/compute.go:553
    func ComputeAssetRiskWith(s Snapshot, p Params, sup *SupportingMetrics)
                                                    ^ this argument is never supplied
                                                      in production
```

**The consequence a reviewer will see.** Six risk flags come back `unevaluated` on every
asset. `docs/methodology/09-flags-and-bands.md` section 2 says confidence drops to
`partial` whenever a high-tier flag is unevaluated, and it requires a dashboard to display
that word. So **every row of the demonstration set currently shows `partial` confidence**,
and four of those six flags are fixed by this one item.

**Done when:**

- `make scan` stores holder concentration (top 1%, top 10%, HHI), the volume-to-supply
  ratio, and the time since the last genuine trade, for every asset in the set.
- `curl localhost:3000/v1/asset/<id>/depth` returns those fields populated, not null.
- `bandConfidence` reads `full` for an asset with complete data, and still reads `partial`
  where data is genuinely missing, with the reason named. Zero and absent are different
  values and the engine already distinguishes them: keep that.
- New tests cover it, and `make ci` is green.

**Files you will touch:** `cmd/keel/scan.go`, and likely `internal/store/metrics.go` plus a
new file in `migrations/`. Read `internal/store/metrics.go` line 5 first: it records that
`Supporting.GenuineVolumeInWindow` has no column yet, which tells you whether you need a
migration before you start.

**What to read before writing anything:** `docs/methodology/07-supporting-metrics.md`,
which defines all three measures, and `internal/domain/supporting.go`, which implements
them. The functions you need are `HolderConcentration`, `VolumeToSupply`, and
`TimeSinceLastGenuineTrade`.

**The part that needs care.** Holder data comes from Horizon's `/accounts?asset=`, which
returns the FULL account object for every holder, not just the balance. A page of 200
holders of a popular asset is tens of megabytes and takes 20 seconds. That endpoint has
already broken this repository once: `internal/horizon/client.go` now has a three minute
timeout and a 64 MB body cap for exactly this reason, and the comments there explain the
incident. Budget your Horizon requests: public Horizon allows about 3,600 per hour per IP
and the scanner defaults to 3,000.

**Estimate: 14 hours.** The largest single item in either track, and the highest value.

---

### A2. Let a browser call the API

**SOW, Deliverable 3:** *"A public web dashboard will show monitored Stellar assets, depth
curves, risk scores..."*

**Why.** The dashboard is a browser application served from a different origin. There are
**zero CORS headers** in `internal/api`. So the moment the API is live, the dashboard still
cannot call it, and the failure will look like a dashboard bug.

**Done when:** a cross-origin `GET` from a browser succeeds, a preflight `OPTIONS` is
answered, and the allowed origins come from a flag or environment variable rather than
being hardcoded.

**Do not use `Access-Control-Allow-Origin: *`.** The API is public and unauthenticated
today, but `*` removes the ability to narrow it later without breaking clients. An
allowlist that currently contains the dashboard's origin and localhost is the same amount
of work and keeps the choice open.

**Files:** `internal/api/api.go`, plus its test file.

**Estimate: 2 hours.**

---

### A3. Put the API on the internet

**SOW, section 6.1, evidence for Deliverable 2:** *"Live API URL, open backtest report,
raw data, and calculation code."*

**Why.** This is the item whose code is completely finished and whose score is near zero.
The image builds, passes a real smoke test in CI, and publishes to `ghcr.io`. Then it
stops, on purpose. Read the header of `.github/workflows/deploy.yml`: the deploy job
refuses to invent a host, because whoever provisions the infrastructure that holds the
evidence has no chain of custody over it.

**What already exists:** a distroless non-root image on port 3000, a compose file, and a
CI job that applies the real schema, starts the container, and asserts that the served
`methodologyVersion` matches the constant the same commit compiles.

**What is missing, and it is more than "a host".** A live Keel is **three** units, not
one:

| Unit | Why it is not optional |
|---|---|
| A Postgres that is not a throwaway | `keel serve` refuses to start if `schema_migrations` is empty |
| `keel serve` | the endpoints |
| `keel scan -interval 15m` | without it, health reads `degraded` and every asset returns 404 "no metrics yet" |

**Recommended shape, and the reasoning is in the file so you can disagree with it.** One
small VPS running `docker compose` with the image pulled from `ghcr.io`, and Caddy in
front for automatic TLS. Reasons: the compose file already exists and is correct; `scan` is
a long-lived process with an interval, which is awkward on platforms that bill per request
or sleep free services; Horizon's rate limit is per IP, so one box means one IP whose
budget you know, while a shared platform IP can be throttled because of someone else's
traffic; and the SOW explicitly puts "a production mainnet SLA" out of scope, so one box
without redundancy is the correct size rather than a compromise. About 5 dollars a month.

**Your half, prepare:**

- `docker-compose.prod.yml`: the `ghcr.io` image, the `scan` service, Caddy, a Postgres
  volume, and a daily database dump. There is already S3 tooling in `scripts/s3-archive/`
  you can reuse for the dump.
- A `Caddyfile`.
- A runbook. Follow the shape of `scripts/s3-archive/RUNBOOK.md`: numbered steps somebody
  can execute without guessing.
- The deploy step in `.github/workflows/deploy.yml`, replacing the "Refuse to guess" step:
  SSH to the host, `docker compose pull`, `docker compose up -d`, then `curl /v1/health`
  and fail the job if the methodology version is not the one this commit compiles. Rollback
  is pointing at the previous image tag, which exists because every image is tagged with
  its commit SHA.

**AL ONLY, apply:** the host and its DNS, the repository variable `KEEL_DEPLOY_TARGET`, the
secret `KEEL_DSN`, and running `KEEL_MIGRATE_DSN=... bash scripts/migrate.sh` once against
the real database.

**One flag, and it is the only coordination point with Track B.** `keel serve -historical`
declares that the historical replay path has data behind it. Ship with it **off**. With it
off, a request for a past ledger returns `503 HISTORICAL_UNAVAILABLE`, which is the
contract's own honest answer. Track B is building the thing that writes those rows. When
B's rows exist, flipping this flag is a one-line change to the compose command. **Neither
track waits for the other.**

**Estimate: 10 hours to prepare, plus about 2 of Al's to apply.**

---

### A4. Make the asset list the real demonstration set

**SOW, Deliverable 3:** *"The demonstration set will contain at least 50 active Stellar
assets."*

**Why.** `configs/demonstration-set.json` holds 60 pairs, so the count passes. But its own
note says **PROVISIONAL**, and it was generated as the union of two draft files.
`docs/methodology/02-pair-selection.md` section 5 now defines the real selection criteria
and supersedes it. Shipping a client demo built on a file that calls itself provisional is
an avoidable question at review time.

**Done when:** the set is regenerated from the section 5 criteria, still holds at least 50
active assets, the note no longer says provisional, and a short evidence file in
`docs/evidences/` records how it was selected and when.

**The one rule you must not get wrong here:** an asset is the pair **(code, issuer)** and
is never matched on the ticker alone. This is not pedantry. `/assets?asset_code=USDY`
returns 37 different issuers. Several issuers use the code `USDC` and only one of them is
the real one. Match on the 56-character issuer address.

**Files:** `configs/demonstration-set.json`, one new file under `docs/evidences/`, and
`scripts/candidate-survey.sh` is the existing tool for the survey.

**Estimate: 6 hours.**

---

### A5. Make `README.md` true again

**Why.** The README's "What does not exist yet" section says the supporting metrics are
"declared, stored and served, and none of them is computed" and that
`07-supporting-metrics.md` "is still a worksheet". Both were true in August. Neither is
true now: `internal/domain/supporting.go` is 994 lines with tests, and document `07` is
defined and reviewed. After A1 lands, that section is wrong in a way that undersells the
project to the first person who reads it, and the README is the SOW's own evidence for
Deliverable 1.

**Done when:** the "what exists" and "what does not exist yet" lists match reality at your
commit, the specific-on-purpose tone is kept, and anything still missing is still named
specifically. Do not turn honest gaps into marketing.

**Estimate: 2 hours.** Do it last, after A1 and A3, so you describe the real end state.

---

### A6. Record the demo video

**SOW, Deliverable 3:** *"Final materials include methodology documentation and a 3 to 5
minute demo video."* **Evidence:** *"Live dashboard URL and demo recording."*

**Why.** It is a required deliverable and it currently does not exist. It also has a
prerequisite chain: it needs A3 done, and the dashboard, which lives in a different
repository.

**Done when:** a 3 to 5 minute recording shows the workflow end to end. Open the
dashboard, pick an asset, read its depth curve, read its flags and its band, read the
recommended maximum safe collateral, then open one asset's detail view. Say out loud what
each number means. The audience is an Ambassador Chapter Lead with minimal technical
expertise, which the SOW states in those words, so no jargon without a definition.

**AL ONLY** for the narration and the claims about what the numbers mean. The zone rules
put every claim about MEANING with Al. Preparing the script, the click path and the
recording setup is not Al-only.

**Estimate: 4 hours.**

---

## 7. The boundary with Track B

**Files Track B owns. Do not edit them.** If you need a change in one, write it down and
hand it over rather than reaching across.

```
internal/horizon/series.go, rewind.go, replay.go, offerxdr.go, trades.go
cmd/keel/bookseries.go, replay.go, crosscheck.go, divergence.go
docs/report/          docs/evidences/       docs/methodology/
docs/decisions/       testdata/
```

**Files you own. Track B will not edit them.**

```
cmd/keel/scan.go, serve.go       internal/api/         internal/store/
internal/domain/                 migrations/           configs/
Dockerfile, docker-compose*.yml, Caddyfile
.github/workflows/deploy.yml     scripts/deploy/       docs/api/    README.md
```

**Three places the two tracks touch, and how each is handled so neither blocks:**

1. **`store.SaveMetrics`.** Both tracks call it. Only you may change it. If you change its
   signature, say so before you do; keep it backward compatible.
2. **`internal/domain` compute functions.** Both tracks call them. Only you may edit that
   package.
3. **The `-historical` flag.** Yours, in the deploy config. It flips from off to on after
   Track B's rows exist. Until then a 503 is the correct answer, so shipping without it is
   not shipping something broken.

Nothing else. No item in your list waits on any item in theirs.

---

## 8. Definition of done for Track A

```
[ ] A1  make scan stores holder concentration, volume-to-supply and last-genuine-trade
[ ] A1  the API returns those fields populated, and bandConfidence reads full
[ ] A2  a browser on another origin can call the API
[ ] A3  docker-compose.prod.yml, Caddyfile, runbook and deploy step written
[ ] A3  AL: host, DNS, KEEL_DEPLOY_TARGET, KEEL_DSN, migrate run once
[ ] A3  a public URL where GET /v1/health answers, and scan runs on a schedule
[ ] A4  demonstration set regenerated from 02-pair-selection.md section 5, 50+ assets
[ ] A5  README.md describes the real state
[ ] A6  a 3 to 5 minute demo recording exists
[ ] make ci green on every commit
```

When all of these are true, the SOW's Deliverable 3 evidence line, "Live dashboard URL and
demo recording", is satisfiable, and Deliverable 2's "Live API URL" is satisfied.

---

## 9. Glossary

| Term | What it means here |
|---|---|
| **SDEX** | Stellar Decentralized Exchange. The on-chain order book |
| **AMM** | Automated Market Maker. Liquidity pools, an alternative venue to the order book |
| **Depth at 2%** | how much you can trade before the price moves 2% away from the reference price |
| **Manipulation cost** | what an attacker would have to spend to move the price by a given amount |
| **Band** | the overall risk verdict for an asset: LOW, MEDIUM, HIGH, CRITICAL |
| **Flag** | a specific named condition, for example `MANIPULATION_CHEAP`. Each has a tier |
| **`bandConfidence`** | `full` or `partial`. `partial` means at least one important flag could not be evaluated. A LOW band with partial confidence is not a clean bill of health |
| **`unevaluated`** | a flag that could not be computed. Deliberately different from "did not fire" |
| **Horizon** | Stellar's public HTTP API. Keel's only data source |
| **Hubble** | Stellar's BigQuery dataset. Deliberately deferred, see DEC-002 |
| **Layer 1 / 2 / 3** | the three validation layers in `docs/methodology/10-validation.md`. Layer 1 is hand recomputation, Layer 3 is Horizon against rebuilt history |
| **Zone** | GREEN, YELLOW or RED. Who may write in a directory. The table is in `CLAUDE.md` |
| **DEC-nnn** | an architecture decision record in `docs/decisions/`. They are amended, never quietly reversed |
| **`offers-implied`** | a book reconstructed by replaying past offer operations, rather than read live |
| **Golden fixture** | `testdata/fixtures/ustry_pre_exploit.md`. Numbers computed by hand before any code existed. The code is corrected to match it, never the reverse |
