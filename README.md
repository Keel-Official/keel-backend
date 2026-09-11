# Keel

Liquidity risk engine for the Stellar ecosystem. It measures effective liquidity
depth from the SDEX orderbook and AMM pool reserves.

An oracle answers "what is the price". Keel answers "what volume can that price
actually support".

This file is how to RUN it. What is built and what is not, the layout, the
recording workflow and the one CI job that is red on purpose are in the git
history of this file, in `CLAUDE.md`, and in `docs/`.

---

# 1. Running it locally

## 1.1 What you need

Docker with the compose plugin, Go 1.23 or newer, `curl`, and `jq` for reading the
responses. Nothing else. No Stellar key, no API token: Keel is permanently read
only, it never signs and never submits a transaction.

## 1.2 The four commands, in this order

```bash
docker compose up -d                             # Postgres only
make migrate                                     # apply migrations/ in order
make assets PAIRS=configs/recorder-pairs.json    # declare the demonstration set
docker compose --profile app up -d --build       # the API and the scanner
```

Each one exists for a reason, and skipping one fails in a way that does not look
like the step you skipped.

**`docker compose up -d` does NOT start the API.** The `keel` and `keel-scan`
services sit behind `profiles: ["app"]`, so this command brings up Postgres and
nothing else. That is deliberate: `make up` has meant "a database on 5433" since
this repository existed, and a service that started with it would change what one
command does to every existing workflow, including the store integration suite.

**The database is published on host port 5433, not 5432.** A Postgres already
installed on the host takes 5432 first, and the symptom is
`role "keel" does not exist` rather than a refused connection. Inside the compose
network the port is 5432, because that is the container's own port and the
published mapping does not apply to it. `store.DefaultDSN` already points at 5433,
so nothing needs an argument.

**`make migrate` is the only mechanism that applies the schema.** The migrations
are deliberately not mounted into Postgres's `docker-entrypoint-initdb.d`, because
that directory runs only when the data directory is empty: it would apply the
first file on a fresh volume and silently ignore every file after it.

**`make assets` is not optional and its `PAIRS=` is not optional either.** The
scanner reads which pairs to measure from the `assets` table, never from a file,
so an empty table means the scanner measures nothing. The Makefile default points
at `scripts/record-pairs.example.json`, which holds ONE pair, so name the list you
want. `configs/recorder-pairs.json` is the eight provisional pairs;
`configs/demonstration-set.json` is the sixty.

## 1.3 Confirm it came up

```bash
docker compose --profile app ps
docker compose logs -f keel
```

The `keel` container has no healthcheck, on purpose. Its image is distroless: no
shell, no curl, no wget, so the usual `CMD-SHELL` probe cannot run inside it, and
adding a binary purely to test the process from inside would widen the image for a
check that can be made from outside. `GET /v1/health` is the healthcheck.

## 1.4 The five endpoints

Everything is under `/v1`. All five are read only, and no request can reach
Horizon: every figure served was computed by the scanner beforehand and stored. A
popular asset triggering a Horizon request per call would burn the rate limit
budget in minutes. The consequence is that metrics always lag, which NFR-1 accepts
explicitly and the `X-Keel-Staleness-Seconds` header reports.

Every response carries `X-Keel-Methodology-Version`. Every response holding a
figure also carries `X-Keel-Staleness-Seconds`.

### `GET /v1/health` - is the engine actually working

```bash
curl -s localhost:3000/v1/health | jq
```

```json
{
  "status": "ok",
  "latestScanAt": "2026-09-11T06:10:55.2804Z",
  "latestScanLedgerSeq": 64374091,
  "assetsMonitored": 64,
  "methodologyVersion": "1.0.8-draft",
  "historicalAvailable": false
}
```

This is not a ping. It answers whether the numbers you are about to read are worth
trusting. `status` is derived from the last scan and three separate states are
called `degraded` rather than merged: no scan at all, a scan that started and
never finished, and a scan that finished with failures. A crashed scan looks
exactly like a fast one from the outside, which is why the second is distinguished.

**`degraded` right after startup is correct and is not a failure.** The scanner
runs on a 15 minute interval, so the first round takes a few minutes to finish.
`latestScanLedgerSeq` comes from the newest metrics row and not from the `runs`
table, because `runs` records the job and not the ledger it reached.

`historicalAvailable: false` is deliberate. See the `?ledger=` note below.

### `GET /v1/methodology` - which parameters produced those numbers

```bash
curl -s localhost:3000/v1/methodology | jq
```

Returns the methodology version and every threshold: `manipulationCheapAbsolute`,
`thinDepth5PctAbsolute`, `spreadExtremePct`, `holderTop1ExtremePct`,
`oracleWindowSeconds` and the rest. The point is that a consumer can apply its own
thresholds, which is also why every flag is reported separately rather than rolled
into one verdict.

Two things in the response are worth reading closely. `calibrated: false` states
plainly that the thresholds were chosen from the magnitude of the Blend incident of
February 2026 and conservative judgement, not calibrated against a set of
incidents. And the two unit keys carry the full `(code, issuer)` pair:

```json
"manipulationCheapUnit": "USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN"
```

never the bare ticker `USDC`. An asset is never matched on its code: 97 distinct
assets share the AQUA ticker, and a consumer that read `USDC` here and resolved it
itself could resolve it to a different asset than the one these thresholds are
counted in.

The values come from `Config.Params` rather than being read back from
`domain.DefaultParams()` inside the handler, so a deployment running non-default
parameters cannot report the defaults.

### `GET /v1/assets` - the monitored set, with a risk summary each

```bash
curl -s 'localhost:3000/v1/assets?limit=3' | jq
curl -s 'localhost:3000/v1/assets?band=CRITICAL&limit=5' | jq
curl -s 'localhost:3000/v1/assets?hasFlag=THIN_DEPTH_5PCT' | jq '.total'
```

This is the entry point, and where you get valid `assetId` values for the two
endpoints below. Four query parameters:

| Parameter | Values |
|---|---|
| `band` | `LOW`, `MEDIUM`, `HIGH`, `CRITICAL` |
| `hasFlag` | one of the enumerated flags, for example `THIN_DEPTH_5PCT`, `MANIPULATION_CHEAP`, `SPREAD_EXTREME`, `ZERO_DEPTH_2PCT`, `PRICE_SOURCE_CONFLICT` |
| `limit` | 1 to 200, default 50 |
| `offset` | paging, with `total` in the response |

A typo in `band` or `hasFlag` returns 400 `INVALID_RANGE` rather than an empty
list. That is the whole reason they are matched against an enumeration: an empty
list reads as "no asset has this problem", which is a different and much worse
answer than "you spelled the flag wrong".

Each item carries `band`, `bandConfidence`, the flag list, `midPrice`,
`priceSource`, `depth5PctBuySide`, `maxSafeCollateral` and `ledgerSeq`.

### `GET /v1/asset/{assetId}/depth` - the question Keel exists to answer

```bash
curl -s 'localhost:3000/v1/asset/XLM/depth' | jq
curl -s 'localhost:3000/v1/asset/USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC/depth' | jq
```

Not "what is the price" but "what volume can that price take". The response holds
depth at delta 0.02, 0.05 and 0.10 split into `fromSdex` and `fromAmm`,
manipulation cost at delta 0.5, 1, 10 and 100 both combined and order book only,
the collateral ceilings, the flags, the band, and `warnings` explaining any field
that is null for a structural reason rather than a missing one. Example, on a pair
with an active pool:

> maxReachablePrice and costToMaxReachablePrice are null because an active pool is
> present: under a constant product curve the price tends to infinity as the base
> reserve tends to zero, so every target is reachable and a highest price has no
> meaning

Six flags come back under `unevaluatedFlags` rather than as clear. The holder
concentration, volume-to-supply and last-genuine-trade metrics they depend on are
declared, stored and served, and none of them is computed yet. `unevaluated` is
not the same claim as clear, and the response says which it is.

**`assetId` is `CODE:ISSUER`, or `XLM` for the native asset.** A value that does
not match the contract's pattern is rejected with 400 `INVALID_ASSET_ID` before it
reaches a query. The asset TYPE is never inferred from the code length: it is read
from the `assets` table. USTRY and PYUSD are both five character codes and both
`credit_alphanum12`, and asking Horizon for either as `credit_alphanum4` returns an
empty order book with no error, so a length rule measures a different asset or
nothing at all.

Two query parameters:

- **`?quote=`** picks the quote asset. Omitted, you get the primary pair, which is
  USDC globally under DEC-015. An asset measured against several quotes with no
  USDC among them returns 400 with a `quoteCandidates` list, because calling one of
  them "primary" would assert a rule the methodology does not contain.
- **`?ledger=`** asks for a past ledger, and on this stack it always returns
  **503 `HISTORICAL_UNAVAILABLE`**. `serve` runs without `-historical`, the Hubble
  path is deferred by DEC-002, and returning a live figure wearing a historical
  label is the one genuinely dangerous alternative.

### `GET /v1/asset/{assetId}/history` - the same metrics over time

```bash
curl -s 'localhost:3000/v1/asset/XLM/history?from=64300000&to=64380000&resolution=day' | jq
```

`from` and `to` are required positive ledger sequences and the range is capped at
1,555,200 ledgers, which is 90 days at one ledger every five seconds.
`resolution` is `hour` or `day`, default `day`. `source` is one of `horizon`,
`hubble`, `offers-implied`, `trades-implied`, default `horizon`.

Two decisions inside this endpoint are worth knowing before you chart anything.

**One series is one data source.** `horizon` is the only one of the four that is a
direct reading; the others are a warehouse copy and two reconstructions, and
`trades-implied` is a lower bound rather than a measurement. Charting them together
as one line would present the weakest point in the range as the same kind of number
as the strongest.

**Downsampling selects, it does not average.** Averaging a band or a flag set is
meaningless. Buckets holding nothing are reported in a separate `gaps` array rather
than interpolated away.

### The error paths, which are worth testing too

```bash
curl -s 'localhost:3000/v1/assets?limit=999' | jq -c .
# {"error":{"code":"INVALID_RANGE","message":"limit: must be between 1 and 200"}}

curl -s 'localhost:3000/v1/asset/not-an-asset/depth' | jq -c .
# {"error":{"code":"INVALID_ASSET_ID", ...}}

curl -s 'localhost:3000/v1/asset/FAKE:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN/depth' | jq -c .
# {"error":{"code":"ASSET_NOT_MONITORED", ...}}

curl -s 'localhost:3000/v1/asset/XLM/depth?ledger=64000000' | jq -c .
# 503 {"error":{"code":"HISTORICAL_UNAVAILABLE", ...}}

curl -s 'localhost:3000/v1/asset/XLM/history?from=1&to=99999999' | jq -c .
# {"error":{"code":"INVALID_RANGE","detail":{"maxLedgers":1555200, ...}}}

curl -s localhost:3000/v1/nope | jq -c .
# 404, and it is JSON: a consumer parsing JSON should not get an HTML body on a typo
```

**Two different conditions share the code `ASSET_NOT_MONITORED`**: "not part of the
demonstration set" and "monitored, but no metrics computed yet". The contract's
error enum has no third value, so the message is what separates them.

### One pass over everything

```bash
BASE=localhost:3000/v1
for p in health methodology assets; do
  printf '%-14s ' "/$p"; curl -s -o /dev/null -w '%{http_code}\n' "$BASE/$p"
done
printf '%-14s ' /asset/XLM/depth
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/asset/XLM/depth"
printf '%-14s ' /asset/XLM/history
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/asset/XLM/history?from=64300000&to=64380000"
```

## 1.5 When something is wrong

| Symptom | Cause |
|---|---|
| Nothing answers on port 3000 | `--profile app` was omitted. `docker compose up -d` alone starts Postgres only |
| `role "keel" does not exist` | a client is talking to a Postgres that is not this container. The container is on 5433; `make store-test` runs a preflight that names which server answered |
| `/v1/assets` returns `"total": 0` | `make assets` has not run, or ran against the one-pair default list |
| `/v1/health` stays `degraded` | the first scan round has not finished. `docker compose logs -f keel-scan`. It is also `degraded` if a scan finished with failures |
| every asset returns 404 "no metrics yet" | the `assets` table is declared but the scanner has not stored a row for it yet |
| `serve: no migrations are applied` | `make migrate` has not run. The refusal is deliberate: a server that starts without a schema fails per request instead of at startup, and the second is far harder to notice |

## 1.6 Without Docker for the Go half

Postgres still comes from compose; `serve` and `scan` can run on the host against
it, which is faster to iterate on.

```bash
docker compose up -d && make migrate
make assets PAIRS=configs/recorder-pairs.json
go run ./cmd/keel scan -once     # one round and exit, instead of every 15 minutes
make serve                       # :3000
```

`-historical` is the flag that flips `?ledger=` from 503 to a real lookup, and it
should stay off until there are replayed rows to serve.

## 1.7 Stopping

```bash
docker compose --profile app down     # keeps the data
docker compose --profile app down -v  # destroys keel_pgdata as well
```

---

# 2. Running it in production

**`scripts/deploy/RUNBOOK.md` is the authority and this section is the map to it.**
Every long explanation lives there rather than in both places, because a second home
for a procedure drifts.

**Status: PREPARED, NOT APPLIED.** The compose file, the Caddyfile, the dump script
and the deploy workflow are written. The box, its DNS, its database password and the
repository secrets are Al's, and Al applies them. The deploy job is gated on the
repository variable `KEEL_DEPLOY_TARGET`: until it is set the job writes a summary
saying what it is waiting for and deploys nothing.

That split is not a courtesy. An agent that provisions the storage its own evidence
lives in has no chain of custody, it has a filing cabinet. It is the same division as
`scripts/s3-archive/`, and `CLAUDE.md` carries it as a zone rule: `scripts/deploy/` is
GREEN to prepare and RED to apply.

## 2.1 What is deployed, because it is more than "the API"

A live Keel is three units, and a deployment missing the third looks exactly like a
working one from outside.

| Unit | Service | Without it |
|---|---|---|
| Postgres that is not a throwaway | `postgres` | `serve` refuses to start, twice: no connection, then empty `schema_migrations` |
| The read-only API | `keel-serve` | no endpoints |
| The scanner | `keel-scan` | health reads `degraded` forever and every asset returns 404 "no metrics yet" |

`caddy` is a fourth service and is not Keel: it holds the certificate, is the only
container with a port open to the internet, and writes the access log. It is
deliberately not on the same Docker network as `postgres`.

`keel-serve` and `keel-scan` run the **same image at the same tag** with different
commands. Two tags would mean the scanner computing rows under one methodology
version while the API reported another.

## 2.2 The compose file must be named explicitly

```bash
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

`docker compose` with no `-f` reads `docker-compose.yml`, which is the developer's
Postgres and not this. The prod file sets `name: keel-prod` for the same class of
reason: with no name, compose derives the project from the directory, so both files
in one clone would share every named volume, including `keel_pgdata`, which is the
throwaway database and the real dataset under the same name.

## 2.3 First-time setup, in dependency order

Sections in the runbook, and DNS is first because Caddy asks for a certificate as
soon as it loads.

| Step | Runbook | The thing that bites |
|---|---|---|
| DNS | 3.1 | `api.keels.app` only. `keels.app` and `www` are on Vercel |
| Firewall | 3.2 | 80 and 443 both inbound. 80 is not optional, the ACME challenge needs it |
| The box | 3.3 | amd64 and arm64 are both published from the same tag |
| `.env` | 3.4 | six variables, one secret, `chmod 600` |
| Schema | 3.5 | `scripts/migrate.sh`, by hand, and it is Al's to run |
| Demonstration set | 3.6 | sixty pairs, from `configs/demonstration-set.json` |
| First boot | 3.7 | wait for `certificate obtained successfully` |

**The hostname is configuration, and adding a second one takes the API down.** Caddy
requests a certificate for every hostname in the Caddyfile at load. An ACME challenge
for a name whose DNS points at Vercel cannot succeed, Caddy retries with backoff, and
the site that does resolve here is degraded while it does. Adding `keels.app` "so the
bare domain redirects" does not add a redirect, it takes the API down.

**`KEEL_DSN` is not a variable on this box and setting it does nothing.** The DSN is
composed inside `docker-compose.prod.yml` from `POSTGRES_USER`, `POSTGRES_PASSWORD`
and `POSTGRES_DB`, so the password is written on the box exactly once and the
hostname is a service name in a committed file rather than something an operator
types. The prefix is `KEEL_`, never `DATABASE_`: a `DATABASE_*` variable is accepted
by compose, ignored by the binary, and silent.

**The role and the database must both be `keel`.** `scripts/migrate.sh` has
`psql -U keel -d keel` written into its compose transport. Set `POSTGRES_USER=app`
and everything starts, the API connects, and only the migration fails, which is the
hardest kind of break to find because nothing else looks wrong.

**`POSTGRES_PASSWORD` is read only when `initdb` runs, which is only when the volume
is empty.** Changing it after first boot does not change the password in the
database. It changes the password every client uses, so every client stops connecting
and the database itself is untouched.

## 2.4 Verifying a deploy, and what a correct first response looks like

Run this from a laptop and not over SSH. From the box, `localhost` can answer in ways
the internet cannot, and DNS and the certificate are half of what is being checked.

```bash
curl -s https://api.keels.app/v1/health
curl -sI http://api.keels.app/v1/health | head -1        # 308 Permanent Redirect
curl -s -o /dev/null -w '%{http_code} verify=%{ssl_verify_result}\n' \
  https://api.keels.app/v1/health                        # 200 verify=0
dig +short keels.app A                                   # Vercel, not the VPS
curl -sI https://api.keels.app/v1/health | grep -i 'x-keel-'
```

**`"status": "degraded"` with `latestScanAt: null` is the correct FIRST response and
is not a failed deploy.** It becomes `ok` within about fifteen minutes, when the first
scan round finishes. A deployment that answered `ok` here would be answering for a
scan that never ran.

Only `x-keel-methodology-version` comes back on a fresh deployment.
`X-Keel-Staleness-Seconds` reports how far behind the ledger a RESULT was when it was
computed, so there is nothing to report until the first scan has written one.

`assetsMonitored: 0` alongside `degraded` is also correct: the API is up, the schema
is applied, and step 3.6 has not run. One step left rather than a failure.

In `docker compose ps`, the healthcheck lives on `caddy` and probes
`http://keel-serve:3000/v1/health`, because the API's distroless image has no HTTP
client inside it. **That probe asserts HTTP 200 and never reads the `status` field**,
since `degraded` is a correct 200 and treating it as a failure would mark a working
API unhealthy on every fresh deploy.

## 2.5 Deploying a new version

The deploy job runs on version tags only, and the name has to end in
`-development` or `-production`. Those are the two patterns in the trigger.

```bash
git tag -a v0.3.0-production -m "..." && git push origin v0.3.0-production
```

**A bare `v0.3.0` fires nothing**, and a tag that fires no workflow is silent: no
red tick, no summary, nothing in the Actions tab. The two suffixes currently do the
same thing, because both patterns run the same jobs against the same single
`KEEL_DEPLOY_TARGET`. There is also no longer a route that publishes an image
without deploying, since the same edit removed `workflow_dispatch`. Runbook section
10 carries both points.

The job SSHes with a pinned host key, rewrites `KEEL_IMAGE_TAG` in `.env` to the
commit SHA, pulls, brings the stack up, waits for the `caddy` healthcheck, then curls
the public health URL from the runner and fails unless the served
`methodologyVersion` equals the constant that commit compiles. That version check is
the entire point of the job: the contract once advertised a version the server did
not return, and the generated mock served it.

**The job does not migrate.** Section 3.5 is by hand, deliberately, so a schema
change and an image change are never coupled in a way that pretends to be reversible.

## 2.6 Rollback

Every image is published under its own commit SHA, so rollback needs no rebuild.

```bash
cd /opt/keel
grep KEEL_IMAGE_TAG .env
sed -i 's/^KEEL_IMAGE_TAG=.*/KEEL_IMAGE_TAG=<previous-sha>/' .env
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d keel-serve keel-scan
```

`.env` is the record of what is live, which is why the tag lives there rather than
being passed on a command line. A tag passed only to `up` would leave the file saying
one thing while another ran.

**A migration is not rolled back this way.** Pointing back at an image that predates
an applied migration runs old code against a newer schema.

## 2.7 Backups, and they are not optional

**`docker compose -f docker-compose.prod.yml down -v` destroys `keel_pgdata` and with
it every metric row the deliverable is built on.** The stack owns its database again
as of 11 September 2026, so this is possible.

One host cron line calls `scripts/deploy/dump-database.sh`, which writes
`backups/keel-<timestamp>.dump` in custom format with a `.sha256` beside it and
rotates at 14 days. `KEEL_DUMP_DSN` is required and the script refuses to run without
it rather than producing an empty dump. Runbook section 6 has the crontab line, the
`postgresql-client-18` prerequisite, and how to verify a dump is a readable archive
rather than 20 KB of nothing.

**The dumps are on the same disk as the database.** That survives a dropped table, a
bad migration and a bad deploy. It does not survive losing the box. Offsite is
`scripts/s3-archive/`, prepared and blocked on Al, and section 6.1 is written as
blocked rather than as working.

## 2.8 CORS

An exact-match allowlist, never a wildcard, set through `KEEL_CORS_ORIGINS` in the
host's `.env`, comma separated, no trailing slash. Those are the dashboard's origins
and not this API's own. Runbook section 7 explains why a shared parent domain buys
nothing here.

---

## References

- API contract: `docs/api/keel-openapi.yaml`, with generated examples in `docs/api/mocks/`
- Methodology, the paid deliverable: `docs/methodology/`
- Architecture decisions: `docs/decisions/`
- Deployment: `scripts/deploy/RUNBOOK.md`
- Repository audit: `bash scripts/audit-verification.sh`
- Working zones and the non-negotiable rules: `CLAUDE.md`

## Language

English, everywhere. See `docs/decisions/DEC-005-english-as-repo-language.md`.

## License

MIT.
