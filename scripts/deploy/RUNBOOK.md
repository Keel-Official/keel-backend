# Putting the API on the internet

**Status: PREPARED, NOT APPLIED. Nothing in this directory runs by itself.**
The deploy job in `.github/workflows/deploy.yml` is gated on the repository
variable `KEEL_DEPLOY_TARGET`, and until it is set the job writes a summary
saying what it is waiting for and deploys nothing.

Prepared 11 September 2026 by Claude. Same division as `scripts/s3-archive/` and
`scripts/history-migration/`: the compose file, the Caddyfile, the dump script,
this document and the workflow steps are written here, and the box, its DNS, its
password and the repository secrets are Al's. Claude cannot rent a VPS, must not
hold the key to one, and must not be the party that provisions the
infrastructure the deliverable's evidence is served from.

| File | What it is |
|---|---|
| `docker-compose.prod.yml` | the four services, at the repository root |
| `Caddyfile` | TLS, the reverse proxy and the access log, at the repository root |
| `scripts/deploy/dump-database.sh` | one dump, called by cron. Section 6 |
| `.github/workflows/deploy.yml` | the deploy job, gated until section 10 is done |
| `scripts/migrate.sh` | the only mechanism that applies the schema, in production too |

---

## 1. What is being deployed, because it is more than "the API"

**A live Keel is three units and not one.** Keep this sentence, because a
deployment missing the third looks exactly like a working one from outside.

| Unit | Service | Without it |
|---|---|---|
| A Postgres that is not a throwaway | `postgres` | `keel serve` refuses to start when it cannot connect, and again when `schema_migrations` is empty |
| The read-only API | `keel-serve` | no endpoints |
| The scanner | `keel-scan` | health reads `degraded` forever and every asset returns 404 "no metrics yet" |

**THE FIRST UNIT LEFT THIS STACK AND CAME BACK ON THE SAME DAY, 11 SEPTEMBER 2026,
AND BOTH MOVES ARE RECORDED HERE BECAUSE THE FIRST ONE'S REASONING IS STILL SOUND.**
It left because the box already ran a Postgres, so a second meant two databases on
one machine with two backup stories, and because a stack that owns no data cannot
destroy any. It came back because the operators of the box settled on one Postgres
container per application, and Keel is an application on it. No decision record
governs either move.

**What the return costs, and it is the sentence the first move was made for:**
`docker compose -f docker-compose.prod.yml down -v` destroys `keel_pgdata` and with
it every metric row the deliverable is built on. That was impossible for one day.
Section 6 is what makes it survivable and it is no longer optional.

**What it buys:** the database is inside the project, so `depends_on` gates
startup instead of `keel-serve` crash looping; `docker compose exec -T postgres` is
a transport again, which is what removes the client-version problem from section 6;
and the hostname is a service name in the compose file rather than a value somebody
types into `.env`, which deletes the three-spelling trap that used to live in
section 9.

`caddy` is the fourth service in this file and is not Keel: it holds the
certificate, is the only container with a port open to the internet, and writes the
access log. It is deliberately NOT on the same Docker network as `postgres`.

`keel-serve` and `keel-scan` run the **same image** at the **same tag**, with
different commands. That is deliberate rather than convenient: two tags would
mean the scanner computing rows under one methodology version while the API
reported another.

**Keel is permanently read only.** It never signs and never submits a
transaction, there is no signing code anywhere in the repository, and nothing in
this runbook asks for a key, a seed or a secret that could authorise one. The
only credentials here are a database password and an SSH key for the deploy.

---

## 2. The hostname, and the two names this box must never touch

**The API is `api.keels.app`, and that is the whole of what this host serves.**

`keels.app` and `www.keels.app` are the landing page and the dashboard. They are
a different application, in a different repository, **already deployed on
Vercel**, and their DNS points at Vercel. **This box must not serve them, must
not redirect them, and must not request a certificate for them.**

**That is a startup failure and not a matter of taste.** Caddy asks for a
certificate for every hostname in the Caddyfile as soon as it loads. An ACME
challenge for a name whose DNS answers with Vercel's address cannot succeed: the
CA connects to the address the world sees, which is not this host. Caddy then
retries with backoff, and the site that *does* resolve here is degraded or down
while it does. Adding `keels.app` to the Caddyfile "so the bare domain
redirects" does not add a redirect. It takes the API down.

The Caddyfile carries that reasoning in a comment at the top, so the next person
to reach for a `redir` block reads it there rather than here.

**The hostname is configuration.** `KEEL_DOMAIN` in the host's `.env`, read by
the `caddy` service, substituted into the Caddyfile as `{$KEEL_DOMAIN}`. The
documented value is:

```
KEEL_DOMAIN=api.keels.app
```

It is not hardcoded in the Caddyfile and has no default there, so an unset value
stops Caddy at startup rather than serving the wrong name. The error it prints
is misleading; section 9 has it.

---

## 3. First-time host setup

Placeholders to substitute: `<VPS_IPV4>`, `<VPS_IPV6>`, `<VPS_USER>`.

Steps are in dependency order and DNS is first for a reason: Caddy asks for a
certificate within seconds of its first start, so the record has to exist and
have propagated **before** step 3.7.

### 3.1 DNS: one record, or two if the host has IPv6

At the registrar or wherever the `keels.app` zone is hosted:

| Type | Name | Value | TTL |
|---|---|---|---|
| `A` | `api` | `<VPS_IPV4>` | 300 |
| `AAAA` | `api` | `<VPS_IPV6>` | 300 |

The `AAAA` record is required **only if the box has a routable IPv6 address**,
and then it is not optional: a published `AAAA` that does not answer means every
IPv6-first client, which is most mobile networks, fails or waits out a fallback.
Check with `ip -6 addr show scope global` on the box. If it prints nothing,
publish no `AAAA`.

Name it `api` and not `api.keels.app` if the DNS panel appends the zone itself.
Getting `api.keels.app.keels.app` is the usual first attempt.

**The apex and www are Vercel's and are not the VPS's concern.** Whatever
records `keels.app` and `www.keels.app` have, leave them exactly as they are.
They point at Vercel, that is correct, and no record in this zone points at
`<VPS_IPV4>` except `api`.

Verify before continuing, from anywhere but the box:

```bash
dig +short api.keels.app A       # must print <VPS_IPV4> and nothing else
dig +short api.keels.app AAAA    # must print <VPS_IPV6>, or nothing at all
dig +short keels.app A           # Vercel's address. NOT <VPS_IPV4>
```

### 3.2 The firewall: 80 and 443, both, inbound

| Port | Protocol | Why |
|---|---|---|
| 80 | TCP | **not optional** |
| 443 | TCP | the API |

**Port 80 is the step that gets skipped, and because the API is HTTPS only,
skipping it looks defensible.** It is not, for two reasons. Caddy solves the
ACME HTTP-01 challenge on port 80, so with it closed the certificate is never
issued and there is no HTTPS to be "only" on. And port 80 is where the HTTP to
HTTPS redirect lives: with it closed, anybody who types the hostname without a
scheme gets a connection timeout instead of a 308, which reads as "the API is
down".

```bash
sudo ufw allow OpenSSH
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
sudo ufw status verbose
```

If the provider has its own firewall in front of the box, a cloud security group
or similar, **open it in both places**. One of the two being closed produces
exactly the symptom of the other being closed.

Nothing else needs to be open. `keel-serve` publishes no port: Caddy is the only
route in.

**AND NOTHING NEEDS TO BE OPENED FOR THE DATABASE, WHICH IS THE THIRD ANSWER THIS
PARAGRAPH HAS GIVEN IN A DAY.** The compose file publishes Postgres as
`127.0.0.1:5433:5432`, so the listener exists only on loopback and no firewall rule
can help or hurt it from outside. The containers do not use that port at all; they
reach the database by service name on the `data` network.

**The form matters more than the firewall here.** A bare `"5433:5432"` binds
`0.0.0.0`, and Docker writes its publish rules into `nat` PREROUTING, ahead of the
chains `ufw` manages: the database would be reachable from the internet while
`ufw status` looked correct. The `127.0.0.1:` prefix in the compose file is what
prevents that, and it is the part not to delete. Check it with `ss -ltnp | grep
5433`, which must show `127.0.0.1:5433` and never `0.0.0.0:5433`.

### 3.3 The box

**Either architecture is fine.** The image is published as a manifest list
covering `linux/amd64` and `linux/arm64`, so `docker pull` resolves the right
one from the same tag and x86_64 and arm64 instances are both supported. That
was not true before 11 September 2026, when `platforms:` was added to the
publish step; a box provisioned against the older advice is still correct.

```bash
uname -m     # x86_64 or aarch64, both supported
```

**One caveat worth knowing rather than acting on.** The smoke test in
`.github/workflows/deploy.yml` runs the amd64 image only, because the runner is
amd64 and cannot execute an arm64 one. The arm64 image is cross-built and
published without being started. Nothing in a Go binary built this way makes a
startup difference likely, and the deploy job's version check against the live
API would catch it, but on an arm64 host that check is the first thing that
proves the binary runs. If it fails there and the version is simply absent
rather than wrong, read `docker compose logs keel-serve` before assuming DNS.

Then Docker, the compose plugin, and the repository:

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker <VPS_USER>   # log out and back in
sudo apt-get install -y git

sudo mkdir -p /opt/keel && sudo chown <VPS_USER> /opt/keel
git clone https://github.com/Keel-Official/keel-backend.git /opt/keel
cd /opt/keel
```

`/opt/keel` is the path the deploy job expects, overridable with the repository
variable `KEEL_DEPLOY_PATH`. See section 10.

### 3.4 The `.env` file, and every variable in it

`.env` is gitignored, lives beside the compose file, and is the only place any
value on this box is configured. **No secret appears in any committed file.**
Every REQUIRED value is referenced as `${VAR:?...}` in the compose file, so a
missing one stops `docker compose` with a message naming the variable instead of
starting something half configured. The five optional `KEEL_DB_*` rows are the
exception and are referenced as `${VAR:-}`: unset means empty, the binary reads
empty as unset, and `store.DefaultConfig` answers. They are listed in the compose
file rather than omitted so that setting one here is enough, with no edit to a
committed file.

**`KEEL_DSN` IS NOT IN THIS TABLE AND SETTING IT DOES NOTHING.** It was here for
one day. The DSN is now composed inside `docker-compose.prod.yml` from
`POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB`, so the password is written
on this box exactly once and the hostname is a service name in a committed file
rather than something an operator types. Both services get the expression
character for character, which is what makes them drift only by an edit to that
file and never by a typo here.

| Variable | Required | What it is |
|---|---|---|
| `KEEL_IMAGE_TAG` | yes | the commit SHA to run. **Never `latest`.** The deploy job rewrites this line, and rollback is editing it |
| `POSTGRES_DB` | yes | **must be `keel`**, see the warning below |
| `POSTGRES_USER` | yes | **must be `keel`**, see the warning below |
| `POSTGRES_PASSWORD` | yes | the database password, and the only secret on this box. `openssl rand -base64 32` |
| `KEEL_DOMAIN` | yes | `api.keels.app`. Section 2 |
| `KEEL_CORS_ORIGINS` | yes | the dashboard's origins. Section 7 |
| `KEEL_IMAGE` | no | defaults to `ghcr.io/keel-official/keel-backend` |
| `KEEL_ACME_EMAIL` | no | Let's Encrypt expiry notices. Needs one line uncommented in the Caddyfile |
| `KEEL_DB_MAX_OPEN_CONNS` | no | connection pool ceiling. Default 8 |
| `KEEL_DB_MAX_IDLE_CONNS` | no | idle connections kept. Default 4, and capped at the ceiling above |
| `KEEL_DB_CONN_MAX_LIFETIME` | no | Go duration, e.g. `30m`. Default `30m` |
| `KEEL_DB_CONN_MAX_IDLE_TIME` | no | Go duration. Default `5m` |
| `KEEL_DB_PING_TIMEOUT` | no | Go duration. Default `5s`. See below |

**`KEEL_DB_PING_TIMEOUT` IS THE ONE OF THE FIVE THAT EARNS ITS ROW**, and it is
here because of the hostname trap immediately below. `keel serve` and `keel scan`
verify the connection before doing anything, and until 11 September 2026 that
check had no deadline. A `KEEL_DSN` naming a host that REFUSES a connection fails
at once; one naming a host that DROPS the packets, which is the ordinary
behaviour of a firewall, hung the container until the kernel gave up. Now it is
five seconds and a log line. Raise it only on a slow link; a value that does not
parse, or that is zero or negative, refuses to start rather than falling back,
because a pool silently the wrong size is worse than a container that will not
come up. The four pool rows are here for completeness and the defaults suit this
box: the API is read only and the scanner walks assets one at a time.

**THE PREFIX IS `KEEL_`, NOT `DATABASE_`.** Nothing in this repository reads a
`DATABASE_*` variable. One set in `.env` is accepted by `docker compose`, ignored
by the binary, and silent, which is the failure this note exists to prevent.

**`POSTGRES_PASSWORD` LEFT THIS TABLE AND CAME BACK THE SAME DAY.** For one day
this stack used a database it did not create, so it set no password and was handed
one that existed. It creates the database again, so it sets the password again, on
first boot and only on first boot.

**THE HOSTNAME TRAP IS GONE AND THE PASSWORD TRAP IS BACK. They are not the same
size and it is worth knowing which one you now have.**

The hostname trap was the worse of the two. `KEEL_DSN` had to name
`host.docker.internal`, and `localhost`, `postgres` and port 5433 were three
plausible wrong answers that all surfaced as an identical empty 502. That table is
deleted rather than quoted, because the compose file no longer takes a hostname
from anybody: it is the literal string `postgres`, resolved on the `data` network,
and there is nothing to get wrong.

What replaced it is the older and smaller trap, and it is a matter of TIMING
rather than of spelling:

> **Postgres reads `POSTGRES_PASSWORD` only when `initdb` runs, which is only when
> the volume is empty.** Changing this line after the first boot does not change
> the password in the database. It changes the password every client uses, so
> every client stops connecting and the database itself is untouched.

That failure has one visible form, `password authentication failed for user
"keel"`, and section 9 has the fix. There is no second place to keep this value in
step with, which is the part that improved: the DSN is composed from it.

**THE ROLE AND THE DATABASE MUST BOTH BE `keel`, AND THIS IS THE SHARPEST EDGE IN
THIS SECTION.** `scripts/migrate.sh` has `psql -U keel -d keel` written into its
compose transport. Set `POSTGRES_USER=app` here and everything starts, the API
connects, and only the migration fails, which is the hardest kind of break to find
because nothing else looks wrong.

```bash
cd /opt/keel

cat > .env <<'ENVFILE'
# The commit SHA this box runs. The deploy job rewrites this line. Never latest.
KEEL_IMAGE_TAG=REPLACE_WITH_A_COMMIT_SHA

# The database this stack runs and owns. There is no KEEL_DSN line: the DSN is
# composed in docker-compose.prod.yml from these three. Both names must be keel.
POSTGRES_DB=keel
POSTGRES_USER=keel
POSTGRES_PASSWORD=REPLACE_WITH_A_GENERATED_PASSWORD

# The hostname Caddy serves and requests a certificate for. Section 2.
KEEL_DOMAIN=api.keels.app

# The dashboard's origins, exact, comma separated, no trailing slash, no
# wildcard. These are NOT this API's own origin. Section 7.
KEEL_CORS_ORIGINS=https://keels.app,https://www.keels.app
ENVFILE

chmod 600 .env
grep -c '^KEEL_\|^POSTGRES_' .env     # 6
```

Generate the password rather than inventing one:

```bash
openssl rand -base64 32
```

**THE HEREDOC IS QUOTED, `<<'ENVFILE'`, AND THAT MATTERS NOW THAT A PASSWORD IS
PASTED INTO IT.** Unquoted, the shell would expand anything in the password that
looks like `$foo`, and a password silently truncated at a dollar sign is a 502 with
no message. Quoted, every character lands verbatim. The password does not pass
through a shell variable at all any more, so there is nothing to `unset` and
nothing left in the environment.

`chmod 600` because this file holds the database password and `docker compose`
reads it as the invoking user.

`sslmode=disable` is in the composed DSN and is correct there. The connection
crosses a private Docker network between two containers on one host and never
touches a wire. Requiring TLS would mean issuing and rotating a certificate for the
name `postgres`, for a hop that cannot be observed without root on the box, which
already ends the argument. **If the database is ever moved off this machine that
reasoning is void**, and the value becomes `require` or `verify-full` with a root
certificate. It is written in `docker-compose.prod.yml` and not here, so that is
the one place to change.

Set `KEEL_IMAGE_TAG` to a real SHA before the first boot. Any commit whose
image the deploy workflow has published works:

```bash
# from a laptop, list what has been published
gh api /orgs/Keel-Official/packages/container/keel-backend/versions \
  --jq '.[].metadata.container.tags[]' | head
```

### 3.5 The schema, once, and it is Al's to run

```bash
cd /opt/keel
COMPOSE_FILE=docker-compose.prod.yml bash scripts/migrate.sh
```

**`scripts/migrate.sh` and nothing else.** Its own header states the rule: a
migration applied from two places is a migration nobody can say ran. It holds
the ordering, the exactly-once bookkeeping in `schema_migrations`, and the
per-file transaction.

**THE TRANSPORT HAS NOW INVERTED TWICE IN ONE DAY AND BOTH FORMS ARE IN SOMEBODY'S
SHELL HISTORY.** This section originally read exactly as it does above. It was then
rewritten to:

```
KEEL_MIGRATE_DSN='postgres://keel:PASSWORD@localhost:5432/keel?sslmode=disable' \
  bash scripts/migrate.sh
```

with the argument that the compose path "can only ever address a Postgres inside
the local compose project", which was disqualifying while the database was the
host's. The database is in the project again, so the original form works again and
is the better of the two for a reason worth stating rather than assuming:

**`psql` runs inside the server's own container, so the client and the server can
never disagree about their major version.** That is not a convenience. Postgres
18.4 is newer than the client Ubuntu ships, so the DSN path needs
`postgresql-client-18` from the PGDG repository installed on the host first, and
the failure when it is missing is `server version mismatch` from a command whose
job is to be the one reliable step.

**The DSN path still exists and is still supported**, because
`.github/workflows/deploy.yml` uses it and because a box where `docker compose exec`
is unavailable needs a route. On this host it reads:

```bash
cd /opt/keel
KEEL_MIGRATE_DSN='postgres://keel:REPLACE_WITH_THE_DB_PASSWORD@localhost:5433/keel?sslmode=disable' \
  bash scripts/migrate.sh
```

**THE PORT IS 5433 AND NOT 5432, AND THAT IS NOT A TYPO.** `docker-compose.prod.yml`
publishes the database as `127.0.0.1:5433:5432`. 5433 because this box runs one
Postgres container per application and 5432 belongs to the first one; loopback
because a bare mapping would put the database on the internet past `ufw`. Section
3.2. Inside the containers the port is 5432, because that is the container's own
port and the mapping does not apply to them.

It needs `psql` on the host, at least as new as the server:

```bash
sudo apt-get install -y postgresql-common
sudo /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh
sudo apt-get install -y postgresql-client-18
```

Expect `migrate: transport compose` and then
`migrate: 5 applied, 0 already present`. **The transport line is printed for a
reason and is worth reading**: the failure this script was written after was a
schema applied to one database while every client talked to another, and with two
Postgres containers on this box that failure is available again.

`keel serve` will now start; before this it refuses to, and that refusal is the
point.

**`.env` is read automatically and only from the project directory**, which is
why every `docker compose` command in this runbook starts with `cd /opt/keel`. Run
one from elsewhere and it stops with "required variable KEEL_IMAGE_TAG is missing
a value", which is the `${VAR:?}` form reporting a missing file rather than a
missing variable. The compose transport above reads `.env` for the same reason and
from the same place; the `KEEL_MIGRATE_DSN` form does not, which is why the
password appears on its command line and why that line is worth keeping out of
shell history.

### 3.6 Declare the demonstration set

The scanner reads which pairs to measure from the `assets` table, not from a
file, so the table has to be populated once. The pair list is not inside the
image, which is why it is bind mounted for this one command.

```bash
cd /opt/keel
docker compose -f docker-compose.prod.yml run --rm \
  -v "$PWD/configs:/configs:ro" \
  keel-serve assets -pairs /configs/demonstration-set.json
```

Sixty pairs. `configs/recorder-pairs.json` is a different, provisional list and
is not the one to use here: `docs/methodology/02-pair-selection.md` section 5
supersedes it.

### 3.7 First boot

```bash
cd /opt/keel
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps
```

Four services running. Watch the certificate being issued, which takes seconds
when 3.1 and 3.2 are right and never happens when they are not:

```bash
docker compose -f docker-compose.prod.yml logs -f caddy
```

`certificate obtained successfully` is the line to wait for.

---

## 4. Verification, and what a correct FIRST response looks like

Run this from a laptop, not over SSH: from the box, `localhost` can answer in
ways the internet cannot, and DNS and the certificate are half of what is being
checked.

```bash
curl -s https://api.keels.app/v1/health
```

A correct **first** response, before any scan has completed:

```json
{
  "status": "degraded",
  "latestScanAt": null,
  "latestScanLedgerSeq": null,
  "assetsMonitored": 60,
  "methodologyVersion": "1.0.8-draft",
  "historicalAvailable": false
}
```

**`degraded` IS THE CORRECT ANSWER HERE AND IS NOT A FAILED DEPLOY.** Read the
fields one at a time, because each reports a true thing:

- **`status: degraded`** means no scan has been recorded yet. The health handler
  derives the status from the last scan and calls three separate states
  degraded: no scan at all, a scan that started and never finished, and a scan
  that finished with failures. On a box that has been up two minutes it is the
  first. It becomes `ok` after the first round of `keel-scan` finishes, within
  about fifteen minutes of `up -d`. A deployment that answered `ok` here would
  be answering for a scan that never ran.
- **`assetsMonitored`** is step 3.6 having worked. **`0` with `degraded` is also
  a correct response**: the API is up, the schema is applied, and nothing has
  been declared for the scanner to measure, so there is one step left rather
  than a failure. `60` means 3.6 ran.
- **`latestScanAt` and `latestScanLedgerSeq` are `null`**, which is the same
  fact as the status. They are `null` rather than absent because the contract
  types them nullable.
- **`methodologyVersion`** must match the constant in `internal/domain/types.go`
  at the commit that built the running image. This is the one thing the deploy
  job asserts, and DEC-014 section 5 is why: the contract once advertised a
  version the server did not return, and the generated mock served it.
- **`historicalAvailable: false`** is FR-19 and is deliberate. `keel serve` ships
  without `-historical`, so a request for a past ledger returns
  `503 HISTORICAL_UNAVAILABLE`, which is the contract's honest answer until
  Track B's replayed rows exist. Turning it on with an empty table would make
  the same request a 404, saying "that ledger is missing" instead of "this
  deployment does not serve history". Flipping it later is one word in
  `docker-compose.prod.yml` and a restart of `keel-serve`.

Four more checks, each failing in its own distinct way:

```bash
# TLS, and the redirect that needs port 80
curl -sI http://api.keels.app/v1/health | head -1      # 308 Permanent Redirect
curl -s -o /dev/null -w '%{http_code} verify=%{ssl_verify_result}\n' \
  https://api.keels.app/v1/health                      # 200 verify=0

# The apex is NOT served by this box
dig +short keels.app A                                 # Vercel, not <VPS_IPV4>

# The methodology header
curl -sI https://api.keels.app/v1/health | grep -i 'x-keel-'
```

Only `x-keel-methodology-version` comes back on a fresh deployment, and that is
correct: `X-Keel-Staleness-Seconds` reports how far behind the ledger a RESULT
was when it was computed, so there is nothing to report until the first scan has
written one. Both are named in `Access-Control-Expose-Headers` from the start,
which is what lets the dashboard read them when they arrive. Section 7 checks
that.

The stack's own view, from the box:

```bash
cd /opt/keel
docker compose -f docker-compose.prod.yml ps
```

`caddy` should read `healthy`. **That healthcheck lives on `caddy` and probes
`http://keel-serve:3000/v1/health`, and the reason is worth knowing**: the API's
image is `gcr.io/distroless/static-debian12:nonroot`, which has no shell, no
curl and no wget, and `keel` has no health subcommand, so nothing inside that
container can make an HTTP request to it. `caddy` is the only container in the
stack with an HTTP client. **The probe asserts HTTP 200 and never reads the
`status` field**, because `degraded` is a correct 200 and treating it as a
failure would mark a working API unhealthy on every fresh deploy.

---

## 5. Rollback

Every image is published under its own commit SHA, so rollback is one line and
needs no rebuild:

```bash
cd /opt/keel
grep KEEL_IMAGE_TAG .env                                   # what is running now
sed -i 's/^KEEL_IMAGE_TAG=.*/KEEL_IMAGE_TAG=<previous-sha>/' .env
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d keel-serve keel-scan
curl -s https://api.keels.app/v1/health                    # from a laptop
```

`.env` is the record of what is live, which is why the tag lives there rather
than being passed on a command line. A tag passed only to `up` would leave the
file saying one thing while another ran, and the next person to type
`docker compose up -d` by hand would silently change the running version.

**A migration is not rolled back this way.** If the previous image predates a
migration that has been applied, pointing back at it runs old code against a
newer schema. That is why the deploy job does not migrate: section 3.5 is by
hand, deliberately, so the two are never coupled in a way that pretends to be
reversible.

---

## 6. The daily database dump

**A host cron entry calling `scripts/deploy/dump-database.sh`. There is no
always-on container for this.** An always-on container whose job is to sleep for
23 hours and 59 minutes is a process to supervise, a restart policy to reason
about and a log to read, in exchange for a schedule it cannot keep, because a
sleep loop drifts and cannot promise a time of day. One crontab line is visible
where an operator already looks for scheduled work.

```bash
crontab -e
```

```cron
# Keel: one database dump a day at 03:17 UTC, hashed and rotated.
# 03:17 rather than 03:00 so it does not land with every other cron on the host.
# KEEL_DUMP_DSN is the HOST's spelling: localhost, and port 5433, which is the
# loopback mapping docker-compose.prod.yml publishes. Inside the containers the
# same database is at postgres:5432. Keep this crontab at mode 600: it holds the
# password.
17 3 * * * cd /opt/keel && KEEL_DUMP_DSN='postgres://keel:REPLACE_WITH_THE_DB_PASSWORD@localhost:5433/keel?sslmode=disable' /usr/bin/env bash scripts/deploy/dump-database.sh >> /var/log/keel-dump.log 2>&1
```

**`KEEL_DUMP_DSN` IS REQUIRED and an older crontab line without it fails every
night.** The script reads the DSN from the environment rather than from an
argument because argv is world readable in `ps`, and it refuses to run with the
variable unset rather than producing an empty dump.

**IT NEEDS `pg_dump` ON THE HOST AT LEAST AS NEW AS THE SERVER, WHICH IS 18.4.**
Ubuntu ships 16, and 16 dumping 18 fails with `server version mismatch`. Install
the client as in section 3.5 before trusting this cron entry.

**THE VERSION MATCH IS AVAILABLE FREE AGAIN AND THIS SCRIPT DOES NOT TAKE IT, WHICH
IS A DELIBERATE CHOICE RATHER THAN AN OVERSIGHT.** The script dumped through
`docker compose exec -T postgres pg_dump` until 11 September 2026, needing no
credentials and no client, because it ran inside the database's own container. That
form was removed when the database briefly became the host's, and the database came
back the same day, so it could be restored. It has not been, for one reason: a
backup that only works while the database is a container in this project is a
backup that breaks the next time that decision moves, and it has moved twice in a
day. The DSN form works either way. **The cost is an apt repository on the host,
and it is written down here so the trade is visible rather than rediscovered.**

Run it once by hand first, because a cron entry that has never worked is a
backup nobody has:

```bash
cd /opt/keel
KEEL_DUMP_DSN='postgres://keel:REPLACE_WITH_THE_DB_PASSWORD@localhost:5433/keel?sslmode=disable' \
  bash scripts/deploy/dump-database.sh
ls -la backups/
```

It writes `backups/keel-<timestamp>.dump` in `pg_dump` custom format, a
`.sha256` beside it, and deletes dumps older than 14 days.
`KEEL_DUMP_KEEP_DAYS` overrides the retention. `backups/` is gitignored.

**Check a dump is real rather than assuming it:**

```bash
cd /opt/keel

# the hash first
( cd backups && sha256sum -c "$(ls -t *.dump.sha256 | head -1)" )
# keel-....dump: OK

# then that it is a readable archive and not 20 KB of nothing
latest=$(ls -t backups/*.dump | head -1)
docker run --rm -v "$PWD/backups:/dumps:ro" postgres:18.4 \
  pg_restore -l "/dumps/$(basename "$latest")" | head -8
# ;     Format: CUSTOM   <- this line is the one to look for
```

The `docker run` form above needs no server and no credentials, because it only
reads a file. **The image tag must be at least the server's major version**: an
archive written by `pg_dump` 18 is not readable by `pg_restore` 16, and the error
is indistinguishable from a corrupt dump. A host with `postgresql-client-18`
installed can run `pg_restore -l "$latest"` directly instead.

**Use a mounted directory and not a pipe for that second command.** The obvious
form, `pg_restore -l /dev/stdin < file`, fails
with `did not find magic string in file header`: `pg_restore -l` on a custom
format archive needs a seekable file, and stdin through a pipe is not one. That
error reads like a corrupt dump, which is the worst possible false alarm from a
command whose entire job is to tell you the dump is fine.

### 6.1 Offsite: BLOCKED ON AL, and written as blocked rather than as working

**The dumps are on the same disk as the database.** That survives a dropped
table, a bad migration and a bad deploy. It does not survive losing the box, and
nothing above pretends otherwise.

`scripts/s3-archive/` is the prepared and unapplied path to offsite, and
`dump-database.sh` reuses the part of it that has no AWS in it: the
`sha256sum`-format hash, which is what lets a dump be checked after a copy or a
restore. **The upload half cannot be written yet, and it is blocked on three
decisions rather than on code:**

1. **No bucket exists.** Every artifact in `scripts/s3-archive/` carries
   `<BUCKET>` as a placeholder, and that runbook's section 1 is titled "Read
   this before deciding, because the recommendation is *not yet*". It also needs
   `<ACCOUNT_ID>`, `<REGION>` and `<ROLE_NAME>`.
2. **Its credential path is GitHub Actions, not a host.**
   `github-oidc-trust-policy.json` trusts
   `token.actions.githubusercontent.com` with a `StringEquals` condition on
   `repo:Keel-Official/keel-backend:ref:refs/heads/main`. A cron job on this VPS
   has no OIDC token and cannot assume that role. Giving it one means a
   long-lived AWS key on the box, which is a new decision about a new
   credential.
3. **Its policy would refuse the dump and then keep it forever.**
   `recorder-iam-policy.json` allows `s3:PutObject` on `<BUCKET>/recordings/*`
   only, and a dump is not a recording. The bucket is also created with Object
   Lock in `COMPLIANCE` mode for 365 days plus an explicit `Deny` on
   `DeleteObject`, which is right for evidence and wrong for a backup that has
   to rotate. COMPLIANCE mode cannot be lifted, including by the account root.

**What to do when Al wants offsite dumps.** Not a variation of the recorder's
setup: a separate prefix, a separate policy that permits `PutObject` and
`DeleteObject` under it, lifecycle expiry instead of Object Lock, and its own
credential for the host. Then one `aws s3 cp` line at the end of
`dump-database.sh`. Until those exist, the honest state is a local dump with a
hash, and this section is the record of what is missing.

---

## 7. CORS, and why the shared parent domain buys nothing

```
KEEL_CORS_ORIGINS=https://keels.app,https://www.keels.app
```

**`api.keels.app` is a different origin from `keels.app` as far as a browser is
concerned, and that is the whole reason this variable exists.** An origin is the
triple of scheme, host and port, compared exactly. `keels.app` and
`api.keels.app` are different hosts, so they are different origins, and sharing
the parent domain changes nothing about that. `www.keels.app` is a third origin,
which is why it is listed separately rather than assumed.

So every call the dashboard makes to this API is cross-origin. Without
`Access-Control-Allow-Origin` naming the calling page's origin, the browser
withholds the response from the JavaScript that asked for it: the request
succeeds, the server logs a 200, and the dashboard sees a failure with no status
code. That reads as a dashboard bug rather than a server configuration one,
which is what this variable prevents.

Four things about the value:

- **Exact origins.** Scheme included, no trailing slash, no path, no wildcard.
  A `*` is refused at startup by `internal/api`, on purpose: exact matching
  would otherwise fail closed on it and the symptom would be a dashboard whose
  every request fails with no header and no message.
- **These are the DASHBOARD's origins, not this API's.**
  `https://api.keels.app` does not belong in the list. Nothing here is ever
  served to a browser as a page, so nothing here is ever the origin making a
  call.
- **Setting it replaces the defaults rather than extending them.** Unset,
  `internal/api` allows `http://localhost:5173` and `http://127.0.0.1:5173` and
  nothing else, so a box that forgets the variable serves no browser rather than
  every browser. With the two origins above set, localhost is no longer allowed,
  which is correct for production.
- **It is read once, at process start.** Changing `.env` needs
  `docker compose -f docker-compose.prod.yml up -d keel-serve`. There is no
  reload.

**Caddy adds no CORS header and must not be made to.** The allowlist lives in
one place, `internal/api`, and two `Access-Control-Allow-Origin` headers on one
response is not a lenient case a browser picks from, it is a hard failure. The
Caddyfile says so where somebody would otherwise add it.

Verify from a laptop:

```bash
# An allowed origin gets its own origin back
curl -sI -H 'Origin: https://keels.app' https://api.keels.app/v1/health \
  | grep -i 'access-control-allow-origin'
# access-control-allow-origin: https://keels.app

# Exactly ONE such header, which is what proves Caddy is not adding a second
curl -sI -H 'Origin: https://keels.app' https://api.keels.app/v1/health \
  | grep -ci 'access-control-allow-origin'          # 1

# Anything else gets no such header, and still gets its 200
curl -s -o /dev/null -w '%{http_code}\n' -H 'Origin: https://evil.example' \
  https://api.keels.app/v1/health                   # 200
curl -sI -H 'Origin: https://evil.example' https://api.keels.app/v1/health \
  | grep -ci 'access-control-allow-origin'          # 0

# The preflight
curl -sI -X OPTIONS -H 'Origin: https://keels.app' \
  -H 'Access-Control-Request-Method: GET' \
  https://api.keels.app/v1/health | head -1         # 204

# Vary: Origin, on all of the above
curl -sI -H 'Origin: https://keels.app' https://api.keels.app/v1/health \
  | grep -i '^vary'
```

**A 200 for a disallowed origin is correct.** CORS is not authentication and
this API has none: it is public, unauthenticated, and answers `curl` from
anywhere. What the allowlist decides is whether a *browser* hands the body to a
page's JavaScript.

---

## 8. Reading logs

```bash
cd /opt/keel

# the four services, live
docker compose -f docker-compose.prod.yml logs -f

# one of them, bounded
docker compose -f docker-compose.prod.yml logs --since 1h keel-scan
docker compose -f docker-compose.prod.yml logs --tail 100 keel-serve
docker compose -f docker-compose.prod.yml logs caddy | grep -i certificate
```

The **access log is a file**, on the `caddy_logs` volume, rolled by Caddy at
10 MiB with five kept, so it is capped near 50 MiB. That cap is a disk budget
rather than a retention policy: on a small VPS the same disk holds Postgres and
the dumps, and an access log that fills it takes down the database, which is far
worse than losing last week's requests.

```bash
# the raw file
docker compose -f docker-compose.prod.yml exec caddy \
  tail -n 50 /var/log/caddy/access.log

# it is JSON, so ask it questions rather than grepping
docker compose -f docker-compose.prod.yml exec caddy \
  sh -c 'tail -n 2000 /var/log/caddy/access.log' \
  | jq -r '[.status, .request.method, .request.uri] | @tsv' | sort | uniq -c | sort -rn | head

# what the rolled files look like
docker compose -f docker-compose.prod.yml exec caddy ls -la /var/log/caddy/
```

`jq` runs on the host in that second command, so install it there rather than in
the container.

---

## 9. Troubleshooting

**Caddy will not start and says `server block without any key is global
configuration, and if used, it must be first`.** `KEEL_DOMAIN` is empty. A site
block with no address in front of it is how a Caddyfile spells "global options",
and there is already one of those, so the error describes the shape the empty
variable left behind rather than the order of the blocks. Check
`grep KEEL_DOMAIN .env`.

**`docker compose` refuses to do anything and names a variable.** That is the
`${VAR:?...}` form working. Every REQUIRED value in section 3.4 uses it; the five
optional `KEEL_DB_*` rows do not and can never produce this message. Two causes:
the variable really is missing, or you are not in `/opt/keel`, because `.env` is
read only from the project directory.

**No certificate, and the Caddy log retries.** In this order: `dig +short
api.keels.app A` from off the box and check it is `<VPS_IPV4>`; check port 80 is
open from off the box, in the provider's firewall as well as `ufw`; check the
Caddyfile names only `{$KEEL_DOMAIN}` and that no apex or www block has crept
in. Let's Encrypt allows five duplicate certificates per name per week, so find
the cause before restarting repeatedly, and **do not delete the `caddy_data`
volume**, which holds the certificates and the ACME account key.

**`502` from Caddy.** `keel-serve` is not running or not listening.
`docker compose -f docker-compose.prod.yml logs keel-serve`. The `depends_on`
gate holds it until `pg_isready` passes, so it should no longer crash loop waiting
for a database that is merely slow to start. **The gate does not cover the schema**,
which is deliberate: it waits for a database that accepts connections, not for one
that has tables. Three usual causes, in the order they happen:

1. `serve` connects and refuses to start because `schema_migrations` is empty,
   which is section 3.5 not having been run. This is now the FIRST thing to
   suspect rather than the second, because the gate has already ruled out the
   database being absent.
2. Authentication fails, below.
3. `serve` cannot reach the database at all, which after the gate passes means
   something is wrong with the `data` network rather than with Postgres. The log
   says `store: ping: no answer within 5s` if the packets were dropped rather than
   refused; `KEEL_DB_PING_TIMEOUT` in section 3.4 sets that figure. Check with
   `docker compose -f docker-compose.prod.yml exec postgres pg_isready -U keel`.

**`502`, and `keel-serve`'s log says `password authentication failed for user
"keel"`.** `POSTGRES_PASSWORD` in `.env` is not the password the database actually
holds, and there is only one way that happens now that the DSN is composed from
that same variable: **the value was changed after the first boot.**

`initdb` runs once, when `keel_pgdata` is empty, and that is the only moment
Postgres reads `POSTGRES_PASSWORD`. Editing the line afterwards changes what every
client sends and changes nothing in the database. Section 3.4 states this; this is
what it looks like when it bites.

Two fixes, and pick on purpose:

```bash
# 1. put the ORIGINAL password back in .env, if you still have it
#    nothing else is needed: the database was never wrong
docker compose -f docker-compose.prod.yml up -d keel-serve keel-scan

# 2. or make the database agree with the new value
docker compose -f docker-compose.prod.yml exec postgres \
  psql -U keel -d keel -c "ALTER USER keel PASSWORD 'the-one-now-in-.env';"
docker compose -f docker-compose.prod.yml up -d keel-serve keel-scan
```

Fix 2 needs a working connection to run, so it only helps while some client can
still authenticate. If none can, the password is lost and the volume has to be
restored from section 6.

**Restart both services or neither:** they read the same variable, so a restart of
one leaves the other on the old value, and the visible symptom is an API that works
while the scanner writes nothing.

**THE OLDER FORM OF THIS ENTRY POINTED AT THE HOST'S POSTGRES** and said to fix it
with `psql 'postgres://postgres@localhost:5432/postgres' -c "ALTER USER keel ...`.
That command now reaches the OTHER application's Postgres on this box, if it is
reachable at all, and altering a role there does nothing for Keel. It is quoted so
it is recognised and not pasted.

**`connection refused` to Postgres, and the port number is the reason.** This is
the 5433 story, and it now has a development form and a production form that mean
different things.

In development, `docker-compose.yml` publishes Postgres as `"5433:5432"`. The
host side is 5433 and the container side is 5432, and the comment on that line
records what the old `5432:5432` cost: a Postgres already installed on the host
takes that port first, the host server binds `127.0.0.1` while Docker binds the
wildcard, so `localhost:5432` reaches the *host's* server and the symptom is
`role "keel" does not exist` rather than a refused connection. A whole day went
into that, and `make migrate` never noticed because it goes through
`docker compose exec` and touches no published port at all.

**THIS SECTION HAS NOW SAID THREE DIFFERENT THINGS IN ONE DAY. Both retired
versions are quoted, because the wrong one being remembered is the whole risk.**

The first version said:

> **In production neither number is published.** `postgres` in
> `docker-compose.prod.yml` has no `ports:` key. So: inside `KEEL_DSN`, the port
> is 5432 and the host is `postgres`. From the host shell, no port works, and that
> is correct. So `KEEL_MIGRATE_DSN` cannot be used here.

The second said the database was the host's, reached at `host.docker.internal`
from the containers and `localhost:5432` from the host, with `KEEL_MIGRATE_DSN` as
the only transport.

**What holds now is close to the first version but not identical to it, and the
difference is the one sentence that matters:**

- **Inside the containers the host is `postgres` and the port is 5432.** That is a
  service name on the `data` network in `docker-compose.prod.yml`, not a value
  anybody types. `keel-serve` and `keel-scan` are on that network; `caddy`
  deliberately is not, so a shell in the Caddy container cannot reach the database
  and that is not a fault.
- **From the host shell the database is at `localhost:5433`**, which is the
  difference from the first version: there IS a published port now, bound to
  loopback only. That is the spelling for `KEEL_MIGRATE_DSN` in section 3.5, for
  `KEEL_DUMP_DSN` in section 6, and for a `psql` shell:

```bash
psql 'postgres://keel:REPLACE_WITH_THE_DB_PASSWORD@localhost:5433/keel'
```

- **THE TWO SPELLINGS ARE NOT INTERCHANGEABLE AND SWAPPING THEM IS THE LIKELY
  MISTAKE.** `postgres:5432` from the host gives "could not translate host name".
  `localhost:5433` from inside a container gives a refused connection, because
  `localhost` there is the container itself.
- **`localhost:5432` on this host is the OTHER application's Postgres**, or
  nothing, depending on whether they publish a port. It is never Keel's. If a
  command against 5432 succeeds and shows an unfamiliar database, that is what
  happened, and it is exactly the `role "keel" does not exist` failure from the
  development story wearing production clothes.
- **The published port must stay on loopback.** `ss -ltnp | grep 5433` must show
  `127.0.0.1:5433`. If it ever shows `0.0.0.0:5433`, the `127.0.0.1:` prefix was
  dropped from the compose file and the database is on the internet regardless of
  what `ufw status` says. Section 3.2.

**`health` reads `degraded` forever with `assetsMonitored` above 0.** Look at
the scanner: `docker compose -f docker-compose.prod.yml logs keel-scan`. A scan
that cannot reach Horizon, or that fails on every asset, is a degraded status
reported correctly.

**`assetsMonitored: 0`.** Section 3.6 was not run, or was run against a
different database.

**`exec format error` on `up -d`.** This was the arm64 symptom until
11 September 2026 and should no longer happen: the image is published for both
architectures. If it does, the tag in `KEEL_IMAGE_TAG` predates that change.
Check with `docker buildx imagetools inspect
ghcr.io/keel-official/keel-backend:$(grep '^KEEL_IMAGE_TAG=' .env | cut -d= -f2)`,
which lists the platforms in the manifest, and move to a newer tag. Section 3.3.

**The dashboard sees a network error with no status code, and `curl` works.**
That is CORS. Section 7, and read the `.env` value first: exact origins, no
trailing slash, and `keel-serve` restarted since it changed.

---

## 10. Turning the deploy job on

Until the repository variable `KEEL_DEPLOY_TARGET` is set, the deploy job writes
a summary saying what it is waiting for and deploys nothing. That is the honest
state rather than a placeholder, and it is why the job does not fire on every
push: a deploy workflow that fails for want of a secret teaches people to ignore
a red tick.

**It runs on version tags only, and the tag name has to carry a suffix.** The
trigger is two patterns, `v*-development` and `v*-production`, so the way to deploy
is:

```bash
git tag -a v0.3.0-production -m "..." && git push origin v0.3.0-production
```

**`git tag v0.3.0` TRIGGERS NOTHING**, and this block gave exactly that command
until 11 September 2026, when the trigger was narrowed from `v*` to the two
suffixed patterns. A bare name matched before and does not now. That failure is
silent: a tag matching no pattern produces no red tick, no summary, and nothing in
the Actions tab to notice, so the symptom is a release that appears to have been
cut and a host still running the previous image.

**THE TWO SUFFIXES DO THE SAME THING TODAY.** Both patterns run the same two jobs
against the same single `KEEL_DEPLOY_TARGET`, so `-development` and `-production`
are two names for one path and one host. If they are meant to reach different
boxes, that is a second target and a job-level environment, and it is a change to
make deliberately rather than a meaning to read into the names.

**THERE IS NO LONGER A WAY TO PUBLISH AN IMAGE WITHOUT DEPLOYING.** The same edit
removed `workflow_dispatch`, which was the route that built, smoke tested and
published while deploying nothing. Every tag that builds an image now also attempts
a deploy, gated only by `KEEL_DEPLOY_TARGET` being set. The paragraph that used to
sit here explained why the two jobs did NOT share a trigger, and the reasoning was
that publishing is cheap and reversible while putting an image in front of the
public API is a decision. That reasoning has not been withdrawn; the mechanism that
carried it has.

**Repository variables** (Settings, Secrets and variables, Actions, Variables):

| Variable | Value | What it does |
|---|---|---|
| `KEEL_DEPLOY_TARGET` | `<VPS_USER>@<VPS_IPV4>` | the SSH destination. Setting it is what turns the job on |
| `KEEL_HEALTH_URL` | `https://api.keels.app/v1/health` | what the job curls after deploying. No default and no URL in the workflow: with a target set and this missing, the job fails and says so |
| `KEEL_DEPLOY_PATH` | `/opt/keel` | optional, defaults to `/opt/keel` |

**Repository secrets:**

| Secret | What it is |
|---|---|
| `KEEL_DEPLOY_SSH_KEY` | the private half of a key whose public half is in `<VPS_USER>`'s `authorized_keys`. Generate it for this and nothing else |
| `KEEL_DEPLOY_KNOWN_HOSTS` | output of `ssh-keyscan <VPS_IPV4>`, run from somewhere you trust. Without it the job would have to accept any host key, which is the one thing that makes an SSH deploy worse than a manual one |

Neither of those is a Stellar key and neither can authorise anything on the
network. Keel signs nothing and submits nothing; there is no signing code in the
repository to hold a key for.

```bash
ssh-keygen -t ed25519 -f ./keel-deploy -C "github actions deploy" -N ""
ssh-copy-id -i ./keel-deploy.pub <VPS_USER>@<VPS_IPV4>
ssh-keyscan <VPS_IPV4>          # into KEEL_DEPLOY_KNOWN_HOSTS
cat ./keel-deploy               # into KEEL_DEPLOY_SSH_KEY, then delete both files
```

What the job does, in order: SSH with that key and a pinned host key, rewrite
`KEEL_IMAGE_TAG` in `.env` to this commit's SHA, `docker compose pull`,
`docker compose up -d`, wait for the `caddy` healthcheck to report healthy, then
`curl $KEEL_HEALTH_URL` **from the runner over the public internet** and fail
unless the served `methodologyVersion` equals the constant this commit compiles.

**That version check is the entire point of the job.** It reports the `status`
field and never asserts on it, because `degraded` is the correct answer for up
to fifteen minutes after a deploy, and asserting `ok` would mean either waiting
a quarter of an hour in CI or seeding a scan to satisfy a check.

The job does not migrate. Section 3.5 and section 5 say why.

---

## 11. What was actually run before this was written, and what was not

"Prepared" is a claim, and the difference between a runbook that was reasoned
out and one that was executed is the question a reader should be asking.

**Run for real, 11 September 2026, on a laptop, against this repository's
`docker-compose.prod.yml` and `Caddyfile`:** the full stack with
`KEEL_DOMAIN=localhost`, which is the one value that makes Caddy use its own
internal CA and skip ACME entirely. Sections 3.5, 3.6 and 3.7 verbatim, then the
checks in section 4 and section 7.

What that established, each of them a live response rather than a reading of the
code: `migrate: 5 applied` through the compose transport, which is why 3.5
quotes that number; the section 4 body exactly as printed, `assetsMonitored: 60`
included; port 80 answering `308`; `curl localhost:3000` not answering, so the
API really is reachable only through the proxy; and the whole of section 7,
including **exactly one** `Access-Control-Allow-Origin` header on an allowed
response, which is what proves Caddy adds no second one.

**Two collisions were found that way and fixed, and they are why this section
exists.** Both appear only in a checkout that already has the development stack
running, which is every developer's checkout and no VPS: the two compose files
both resolved to the project name `keel` and therefore shared the `keel_pgdata`
volume, and every `container_name` was identical in both. The first meant a
production stack started in a developer's checkout would attach the developer's
database; the second meant it would not start at all. Reasoning about the file
would not have found either.

**NOT run, and each needs the host that does not exist yet:**

- **The ACME certificate.** `KEEL_DOMAIN=localhost` deliberately avoids it, so
  nothing has tested a real issuance, the port 80 challenge, or the
  apex-and-www reasoning in section 2. That reasoning is argued, not measured.
- **The SSH deploy.** The workflow's deploy steps have never run. The `git
  fetch`, the `compose pull` from `ghcr.io` and the SSH itself have run nowhere.
- **The cron dump on a schedule.** `dump-database.sh` has not been run against
  this stack, and the 14 day rotation has not been waited out.
- **A restore.** No dump has been restored into an empty database. Section 6.1
  says this, and it is the gap most worth closing first.
