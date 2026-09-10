# Putting the API on the internet

**Status: PREPARED, NOT APPLIED. Nothing in this directory runs by itself.**
The deploy job in `.github/workflows/deploy.yml` is still gated on the repository
variable `KEEL_DEPLOY_TARGET` being set, and it is not set. The image builds,
smoke tests and publishes today; nothing is deployed.

Prepared 11 September 2026 by Claude. Same division as `scripts/s3-archive/` and
`scripts/history-migration/`: the compose file, the Caddyfile, the workflow step
and this document are written here, and the host, its DNS, its passwords and the
repository secrets are Al's. Claude cannot rent a VPS, must not hold the key to
one, and must not be the party that provisions the infrastructure the
deliverable's evidence is served from.

Files this runbook drives, all of them in the repository already:

| File | What it is |
|---|---|
| `docker-compose.prod.yml` | the five services, at the repository root |
| `Caddyfile` | TLS and the reverse proxy, at the repository root |
| `.github/workflows/deploy.yml` | the deploy job, gated until section 7 is done |
| `scripts/migrate.sh` | the only mechanism that applies the schema, in production too |

---

## 1. What is being deployed, because it is more than "the API"

**A live Keel is three units and not one.** This is the sentence to keep, because
a deployment missing the third one looks exactly like a working deployment from
outside.

| Unit | Service | Without it |
|---|---|---|
| A Postgres that is not a throwaway | `postgres` | `keel serve` refuses to start when `schema_migrations` is empty |
| The read-only API | `keel-api` | no endpoints |
| The scanner | `keel-scan` | health reads `degraded` forever and every asset returns 404 "no metrics yet" |

Two more services carry the parts that are not Keel: `caddy` holds the
certificate and is the only thing with a port open, and `db-dump` writes a daily
`pg_dump` to `./backups` on the host.

**The shape and why.** One small VPS running `docker compose`, pulling the image
from `ghcr.io`. The reasoning is in `tugas-a.md` A3 and is worth disagreeing with
rather than inheriting: `scan` is a long-lived process with an interval, which is
awkward on anything that bills per request or sleeps idle services; Horizon's
rate limit is per IP, so one box is one budget you know, while a shared platform
IP can be throttled by somebody else's traffic; and the SOW puts a production
mainnet SLA explicitly out of scope, so one box with no redundancy is the right
size rather than a corner cut. About five dollars a month.

---

## 2. The hostname, and the two names this box must never touch

**The API is `api.keels.app`, and that is the whole of what this host serves.**

`keels.app` and `www.keels.app` are the landing page and the dashboard. They are
a different application, in a different repository, hosted somewhere else, and
their DNS points there. **This box must not serve them, must not redirect them,
and must not request a certificate for them.**

**That is a startup failure and not a matter of taste.** Caddy asks for a
certificate for every hostname in the Caddyfile as soon as it loads. An ACME
challenge for a name whose DNS answers with somebody else's address cannot
succeed: the CA connects to the address the world sees, which is not this host.
Caddy then retries with backoff, and the site that *does* resolve here is
degraded or down while it does. Adding `keels.app` to the Caddyfile "so the bare
domain redirects" does not add a redirect. It takes the API down.

The Caddyfile carries that reasoning in a comment at the top, so the next person
to reach for a `redir` block reads it there rather than here.

**The hostname is configuration.** `KEEL_DOMAIN`, set in the host's `.env`, read
by the `caddy` service, substituted into the Caddyfile as `{$KEEL_DOMAIN}`. The
documented value is:

```
KEEL_DOMAIN=api.keels.app
```

It is not hardcoded in the Caddyfile and it has no default there, so an unset
value stops Caddy at startup rather than serving the wrong name. The error it
prints is misleading; section 9 has it.

---

## 3. First-time setup

Placeholders to substitute: `<VPS_IPV4>`, `<VPS_IPV6>`, `<VPS_USER>`.

Steps are in dependency order and the first one is DNS for a reason: Caddy asks
for a certificate within seconds of its first start, so the record has to exist
and have propagated **before** step 3.6, or the first ACME attempt fails and
backs off.

### 3.1 DNS: one record, or two if the host has IPv6

At the registrar or wherever `keels.app` is hosted:

| Type | Name | Value | TTL |
|---|---|---|---|
| `A` | `api` | `<VPS_IPV4>` | 300 |
| `AAAA` | `api` | `<VPS_IPV6>` | 300 |

The `AAAA` record is required **only if the box actually has a routable IPv6
address**, and then it is not optional: a published `AAAA` that does not answer
means every IPv6-first client, which is most mobile networks, fails or waits for
a fallback. If the host has no IPv6, publish no `AAAA`. Check with
`ip -6 addr show scope global` on the box.

Name it `api` and not `api.keels.app` if the DNS panel appends the zone itself;
getting `api.keels.app.keels.app` is the usual first attempt.

**The apex and www are not this box's concern.** Whatever `keels.app` and
`www.keels.app` point at, leave it alone. No record here points at
`<VPS_IPV4>` except `api`.

Verify before continuing, from anywhere but the box:

```bash
dig +short api.keels.app A       # must print <VPS_IPV4> and nothing else
dig +short api.keels.app AAAA    # must print <VPS_IPV6>, or nothing at all
```

### 3.2 The firewall: 80 and 443, both, inbound

| Port | Protocol | Why |
|---|---|---|
| 80 | TCP | **not optional** |
| 443 | TCP | the API |
| 443 | UDP | HTTP/3. This one is optional |

**Port 80 is the step that gets skipped, and the API is HTTPS only, so skipping
it looks defensible.** It is not, for two reasons. Caddy solves the ACME
HTTP-01 challenge on port 80, so with it closed the certificate is never issued
and there is no HTTPS to be "only" on. And port 80 is where the HTTP to HTTPS
redirect lives: with it closed, anybody who types the hostname without a scheme
gets a connection timeout instead of a redirect, which reads as "the API is
down".

```bash
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 443/udp
sudo ufw allow OpenSSH
sudo ufw enable
sudo ufw status verbose
```

If the provider has its own firewall in front of the box, a cloud security group
or similar, **open it in both places**. One of the two being closed produces
exactly the symptom of the other being closed.

Nothing else needs to be open. Postgres publishes no port at all in
`docker-compose.prod.yml`, and the API publishes none either: Caddy is the only
route in.

### 3.3 The box

Docker Engine with the compose plugin, and the repository. `git` is here because
the deploy job checks out a commit on the host and because `scripts/migrate.sh`
is the only thing that applies the schema.

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker <VPS_USER>   # log out and back in
sudo apt-get install -y git postgresql-client

sudo mkdir -p /opt/keel && sudo chown <VPS_USER> /opt/keel
git clone https://github.com/Keel-Official/keel-backend.git /opt/keel
cd /opt/keel
```

`/opt/keel` is the path the deploy job expects. It is overridable with the
repository variable `KEEL_DEPLOY_PATH`; see section 7.

### 3.4 The `.env` file

`.env` is gitignored, lives beside the compose file, and is the only place any
value on this box is configured. Nothing in it has a default in the compose
file: every one is `${VAR:?...}`, so a missing value stops `docker compose` with
a message naming the variable instead of starting something half configured.

```bash
cd /opt/keel
cat > .env <<'ENVFILE'
# The hostname Caddy serves and requests a certificate for. Section 2.
KEEL_DOMAIN=api.keels.app

# The dashboard's origins, exact, comma separated, no trailing slash and no
# wildcard. These are NOT this API's own origin. Section 6.
KEEL_CORS_ORIGINS=https://keels.app,https://www.keels.app

# The Postgres password. Generate it, do not choose it, and do not reuse
# keel_dev_only from docker-compose.yml.
KEEL_PG_PASSWORD=REPLACE_ME

# Which published image is running. The deploy job rewrites this line to the
# commit SHA it deployed, so this file is the record of what is live and
# rollback is putting an older SHA back. Any tag that exists in the registry
# works; `edge` is the newest build of main.
KEEL_IMAGE_TAG=edge
ENVFILE

# Generate the password into place rather than typing one
sed -i "s/^KEEL_PG_PASSWORD=.*/KEEL_PG_PASSWORD=$(openssl rand -base64 24 | tr -d '/+=')/" .env
chmod 600 .env
```

`chmod 600` because this file holds the database password and the compose file
reads it as the invoking user.

### 3.5 The schema, once

```bash
cd /opt/keel
docker compose -f docker-compose.prod.yml up -d postgres
COMPOSE_FILE=docker-compose.prod.yml bash scripts/migrate.sh
```

**`scripts/migrate.sh` and nothing else.** Its own header states the rule: a
migration applied from two places is a migration nobody can say ran. It holds
the ordering, the exactly-once bookkeeping in `schema_migrations`, and the
per-file transaction. `COMPOSE_FILE` is what points its compose transport at the
production project instead of `docker-compose.yml`; without it the script
addresses the development file, which on a fresh box is not running, and the
error says so.

It prints the transport it used, which is deliberate: the failure that line was
added after was a schema applied to one database while every client talked to
another. Expect `migrate: transport compose` and then
`migrate: 5 applied, 0 already present`. `keel serve` will now start.

**`.env` is read automatically and only from the project directory**, which is
why this step works with no `--env-file` and why every command in this runbook
starts with `cd /opt/keel`. Run it from anywhere else and `docker compose` stops
with "required variable KEEL_PG_PASSWORD is missing a value", which is the
`${VAR:?}` form reporting a missing file rather than a missing variable.

### 3.6 Declare the demonstration set

The scanner reads which pairs to measure from the `assets` table, not from a
file, so the table has to be populated once. The pair list is not in the image,
which is why it is bind mounted for this one command.

```bash
cd /opt/keel
docker compose -f docker-compose.prod.yml run --rm \
  -v "$PWD/configs:/configs:ro" \
  keel-api assets -pairs /configs/demonstration-set.json
```

Sixty pairs. `configs/recorder-pairs.json` is a different, provisional list and
is not the one to use here; `docs/methodology/02-pair-selection.md` section 5
supersedes it.

### 3.7 Everything up

```bash
cd /opt/keel
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps
```

Five services running. Watch the certificate being issued, which takes seconds
if section 3.1 and 3.2 are right and never happens if they are not:

```bash
docker compose -f docker-compose.prod.yml logs -f caddy
```

`certificate obtained successfully` is the line to wait for.

### 3.8 Verify it, from somewhere that is not the box

This is the step that says the deployment worked. Run it from a laptop, not over
SSH: from the box, `localhost` can answer in ways the internet cannot.

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
four fields one at a time, because each one is reporting a true thing:

- **`status: degraded`** means no scan has been recorded yet. The health handler
  derives the status from the last scan and calls three separate states
  degraded: no scan at all, a scan that started and never finished, and a scan
  that finished with failures. On a box that has been up for two minutes it is
  the first of those. It becomes `ok` after the first round of `keel-scan`
  finishes, which is within about fifteen minutes of `up -d`. A deployment that
  answered `ok` here would be answering for a scan that never ran.
- **`assetsMonitored: 60`** is step 3.6 having worked. **If this reads `0`, the
  deploy is fine and step 3.6 was skipped**: the API is up, the database is
  migrated, and nothing has been declared for the scanner to measure. `0` with
  `degraded` is also a correct response, it just means there is one step left.
- **`latestScanAt: null` and `latestScanLedgerSeq: null`** are the same fact as
  the status, and they are `null` rather than absent because the contract types
  them as nullable.
- **`methodologyVersion`** must match the constant in `internal/domain/types.go`
  at the commit that built the running image. This is the assertion the deploy
  job makes, and DEC-014 section 5 is why: the contract once advertised a
  version the server did not return, and the generated mock served it.
- **`historicalAvailable: false`** is FR-19 and is deliberate. See section 8.

Three more checks, and each one fails in its own distinct way if the section
above was rushed:

```bash
# TLS is real and the redirect works. -I follows nothing, -L follows it.
curl -sI http://api.keels.app/v1/health | head -1     # 308, from port 80
curl -s -o /dev/null -w '%{http_code} %{ssl_verify_result}\n' https://api.keels.app/v1/health

# The apex is NOT served by this box. Whatever answers, it must not be Caddy
# here. A 404 from this Caddy would mean the hostname reached it.
dig +short keels.app A                                # not <VPS_IPV4>

# The methodology header, on every response
curl -sI https://api.keels.app/v1/health | grep -i 'x-keel-'
```

Only `x-keel-methodology-version` comes back on a fresh deployment, and that is
correct: `X-Keel-Staleness-Seconds` reports how far behind the ledger a RESULT
was when it was computed, so there is no value to report until the first scan
has written one. It appears on `/v1/assets` and on a per-asset response once
metrics exist. Both headers are named in `Access-Control-Expose-Headers` from
the start, which is what lets the dashboard read them when they do arrive;
section 6 checks that.

CORS gets its own verification, in section 6.

---

## 4. What Al decides, and it is only these

| Decision | Why it cannot be Claude's |
|---|---|
| The provider and the box | Al's account, Al's card |
| DNS for `api.keels.app` | Al controls the zone |
| `KEEL_PG_PASSWORD` | a credential Claude must never generate into a file it can read back |
| The SSH key the deploy job uses | same |
| Repository variables and secrets in section 7 | GitHub settings are Al's |
| Whether `-historical` is turned on | FR-19, and it depends on whether Track B's rows exist. Section 8 |
| Whether the dumps go offsite | where the deliverable's data lives. Section 8 |

---

## 5. Day to day

```bash
cd /opt/keel

# what is running, and which image
docker compose -f docker-compose.prod.yml ps
grep KEEL_IMAGE_TAG .env

# logs
docker compose -f docker-compose.prod.yml logs -f keel-scan
docker compose -f docker-compose.prod.yml logs --since 1h caddy

# the database
docker compose -f docker-compose.prod.yml exec postgres psql -U keel -d keel

# force a scan round now instead of waiting for the interval
docker compose -f docker-compose.prod.yml run --rm keel-scan scan -once

# roll back to a previous image, which exists because every build is tagged
# with its commit SHA
sed -i 's/^KEEL_IMAGE_TAG=.*/KEEL_IMAGE_TAG=<previous-sha>/' .env
docker compose -f docker-compose.prod.yml up -d keel-api keel-scan
```

A new schema file in `migrations/` needs `scripts/migrate.sh` run again, by
hand, the same way as step 3.5. **The deploy job does not migrate**, and that is
deliberate: an automatic migration is a deploy that cannot be rolled back by
pointing at the previous image, because the schema has already moved.

---

## 6. CORS, and why the shared parent domain buys nothing

```
KEEL_CORS_ORIGINS=https://keels.app,https://www.keels.app
```

**`api.keels.app` is a different origin from `keels.app` as far as a browser is
concerned, and that is the whole reason this variable exists.** An origin is the
triple of scheme, host and port, compared exactly. `keels.app` and
`api.keels.app` are different hosts, so they are different origins, and sharing
the parent domain `keels.app` changes nothing about that: it is not a
same-origin relationship, it is not a same-site exemption that applies here, and
no browser will hand a response from one to a page on the other without the
header. `www.keels.app` is a third origin, which is why it is listed separately
rather than assumed.

So every call the dashboard makes to this API is cross-origin, and without
`Access-Control-Allow-Origin` naming the calling page's origin the browser
withholds the response from the JavaScript that asked for it. The request
succeeds, the server logs a 200, and the dashboard sees a failure with no
status code. That is the failure this variable prevents, and it reads as a
dashboard bug rather than a server configuration one.

Four things about the value:

- **Exact origins.** Scheme included, no trailing slash, no path, no wildcard.
  A `*` is refused at startup by `internal/api`, on purpose, because exact
  matching would otherwise fail closed on it and the symptom would be a
  dashboard whose every request fails with no header and no message.
- **These are the DASHBOARD's origins, not this API's.** `https://api.keels.app`
  does not belong in this list. Nothing is served from this host to a browser as
  a page, so nothing on this host is ever the origin making a call.
- **Setting it replaces the defaults, it does not extend them.** With the
  variable unset, `internal/api` allows `http://localhost:5173` and
  `http://127.0.0.1:5173` and nothing else, so a box that forgets it serves no
  browser rather than every browser. With it set to the two origins above,
  localhost is no longer allowed, which is correct for production. A developer
  pointing a local dashboard at the live API needs their origin added here, and
  that is a deliberate decision each time.
- **It is read once, at process start.** Changing `.env` needs
  `docker compose -f docker-compose.prod.yml up -d keel-api`. There is no
  reload.

Verify it from a laptop. The first command is the one that matters, the second
is the one that proves the allowlist is an allowlist:

```bash
# An allowed origin gets its own origin back
curl -sI -H 'Origin: https://keels.app' https://api.keels.app/v1/health \
  | grep -i 'access-control-allow-origin'
# access-control-allow-origin: https://keels.app

# Anything else gets no such header, and still gets its 200
curl -s -o /dev/null -w '%{http_code}\n' -H 'Origin: https://evil.example' \
  https://api.keels.app/v1/health          # 200
curl -sI -H 'Origin: https://evil.example' https://api.keels.app/v1/health \
  | grep -ci 'access-control-allow-origin' # 0

# The preflight
curl -sI -X OPTIONS -H 'Origin: https://keels.app' \
  -H 'Access-Control-Request-Method: GET' \
  https://api.keels.app/v1/health | head -1    # 204

# Vary: Origin, on all of the above, so no cache serves one origin's response
# to another
curl -sI -H 'Origin: https://keels.app' https://api.keels.app/v1/health | grep -i '^vary'
```

**A 200 for a disallowed origin is correct.** CORS is not authentication and
this API has none: it is public, unauthenticated, and answers `curl` from
anywhere. What the allowlist decides is whether a *browser* hands the body to a
page's JavaScript.

**Caddy adds no CORS header and must not be made to.** The allowlist lives in
one place, `internal/api`, and two `Access-Control-Allow-Origin` headers on one
response is not a lenient case a browser picks from, it is a hard failure. The
Caddyfile says so where somebody would otherwise add it.

---

## 7. Turning the deploy job on

Until the repository variable `KEEL_DEPLOY_TARGET` is set, the deploy job writes
a summary saying what it is waiting for and deploys nothing. That is the honest
state rather than a placeholder, and it is why the job does not run on every
push to `main`: a deploy workflow that fails for want of a secret teaches people
to ignore a red tick.

**Repository variables** (Settings, Secrets and variables, Actions, Variables):

| Variable | Value | What it does |
|---|---|---|
| `KEEL_DEPLOY_TARGET` | `<VPS_USER>@<VPS_IPV4>` | the SSH destination. Setting it is what turns the job on |
| `KEEL_PUBLIC_URL` | `https://api.keels.app` | what the post-deploy health check calls. No default and no hardcoded URL in the workflow: with `KEEL_DEPLOY_TARGET` set and this one missing, the job fails and says so |
| `KEEL_DEPLOY_PATH` | `/opt/keel` | optional, defaults to `/opt/keel` |

**Repository secrets:**

| Secret | What it is |
|---|---|
| `KEEL_DEPLOY_SSH_KEY` | the PRIVATE half of a key whose public half is in `<VPS_USER>`'s `authorized_keys`. Generate it for this purpose and use it for nothing else |
| `KEEL_DEPLOY_SSH_KNOWN_HOSTS` | output of `ssh-keyscan <VPS_IPV4>`, run from somewhere you trust. It is a variable's worth of secrecy and a secret's worth of importance: without it the job would have to accept any host key, which is the one thing that makes an SSH deploy worse than a manual one |

```bash
ssh-keygen -t ed25519 -f ./keel-deploy -C "github-actions deploy" -N ""
ssh-copy-id -i ./keel-deploy.pub <VPS_USER>@<VPS_IPV4>
ssh-keyscan <VPS_IPV4>          # into KEEL_DEPLOY_SSH_KNOWN_HOSTS
cat ./keel-deploy               # into KEEL_DEPLOY_SSH_KEY, then delete the file
```

What the job then does, in order: SSH to the host, fetch and check out the exact
commit being deployed, rewrite `KEEL_IMAGE_TAG` in `.env` to that commit's SHA,
`docker compose pull`, `docker compose up -d`, then call
`$KEEL_PUBLIC_URL/v1/health` **from the runner over the public internet** and
fail unless the served `methodologyVersion` is the constant that commit
compiles. It reports the `status` field without asserting on it, for the reason
section 3.8 gives at length: `degraded` is the correct answer within fifteen
minutes of a deploy, and asserting `ok` there would mean seeding a scan to
satisfy a check.

It does not run migrations. Section 5 says why.

---

## 8. What this does NOT do

- **No offsite backup.** `db-dump` writes to `./backups` on the same disk as the
  database. That survives a dropped table, a bad migration and a bad deploy; it
  does not survive losing the box. `scripts/s3-archive/` is the prepared and
  unapplied path to offsite, and pointing the dumps at a bucket is a decision
  about where the deliverable's data lives, so it is Al's. Nothing verifies a
  dump by restoring it either, and an untested backup is a hope.
- **No monitoring and no alerting.** If the box stops, nothing tells anybody.
  `curl https://api.keels.app/v1/health` from anywhere is the check, and an
  uptime pinger on that URL is the cheapest real improvement available here.
- **No redundancy, deliberately.** The SOW puts a production mainnet SLA out of
  scope. One box is the right size for this deliverable, not a compromise.
- **`-historical` is off.** FR-19. With it off, a request for a past ledger gets
  `503 HISTORICAL_UNAVAILABLE`, which is the contract's own honest answer for a
  deployment with no replayed rows. Turning it on while the table is empty makes
  the same request a 404, which says "that ledger is missing" instead of "this
  deployment does not serve history". Track B builds the thing that writes those
  rows; when they exist, add `-historical` to the `keel-api` command and
  restart. Neither track waits for the other.
- **No rate limiting.** Caddy does none without a plugin. The API is read-only
  and every response comes out of Postgres rather than Horizon, so the exposure
  is the box's own CPU rather than the Horizon budget, but it is worth knowing
  that nothing is bounding requests.

---

## 9. When it does not work

**Caddy will not start, and says `server block without any key is global
configuration, and if used, it must be first`.** `KEEL_DOMAIN` is empty. The
site block with no address in front of it is how a Caddyfile spells "global
options", and there is already one of those, so the error is about the shape the
empty variable left behind rather than about the order of the blocks. Check
`grep KEEL_DOMAIN .env`.

**`docker compose` refuses to do anything and names a variable.** That is the
`${VAR:?...}` form doing its job. Every value in section 3.4 is required and
none has a default. The message says which one.

**No certificate, and the Caddy log retries.** In this order: `dig +short
api.keels.app A` from off the box and check it is `<VPS_IPV4>`; check port 80 is
open from off the box, in the provider's firewall as well as `ufw`; check the
Caddyfile names only `{$KEEL_DOMAIN}` and no apex or www block crept in. Let's
Encrypt allows five duplicate certificates per name per week, so fix the cause
before restarting repeatedly, and do not delete the `caddy_data` volume, which
holds the certificates and the ACME account key.

**`502` from Caddy.** `keel-api` is not running or not listening.
`docker compose -f docker-compose.prod.yml logs keel-api`. The usual cause is
`serve` refusing to start because `schema_migrations` is empty, which is step
3.5 not having been run against this database.

**`health` says `degraded` forever, with `assetsMonitored` above 0.** The
scanner is the unit to look at: `docker compose -f docker-compose.prod.yml logs
keel-scan`. A scan that cannot reach Horizon, or that fails on every asset, is a
degraded status reported correctly.

**`assetsMonitored: 0`.** Step 3.6 was not run, or was run against a different
database.

**The dashboard sees a network error with no status code, and `curl` works.**
That is CORS. Section 6, and the value in `.env` is the thing to read first:
exact origins, no trailing slash, and the container restarted since it changed.

---

## 10. What was actually run before this was written, and what was not

This section exists because "prepared" is a claim, and the difference between a
runbook that was reasoned out and one that was executed is the whole question a
reader should be asking.

**Run for real, 11 September 2026, on a laptop, against
`docker-compose.prod.yml` and the `Caddyfile` in this repository:** the full
five-service stack with `KEEL_DOMAIN=localhost`, which is the one value that
makes Caddy use its own internal CA and skip ACME entirely. In order: sections
3.5, 3.6 and 3.7 verbatim, then every check in 3.8 and every check in section 6.

What that established, and each of these was a live response rather than a
reading of the code:

- `migrate: 5 applied` through the compose transport with `COMPOSE_FILE` set,
  which is why 3.5 quotes that number.
- `https://.../v1/health` through Caddy returning exactly the body printed in
  section 3.8, `assetsMonitored: 60` included.
- Port 80 answering `308 Permanent Redirect`, so section 3.2's claim about the
  redirect is measured.
- `curl http://localhost:3000` NOT answering, so the API really is reachable
  only through the proxy.
- CORS: both dashboard origins echoed back, a disallowed origin getting `200`
  and zero `Access-Control-Allow-Origin` headers, `Vary: Origin` on all of them,
  the preflight `204` with all four headers, and **exactly one**
  `Access-Control-Allow-Origin` header on an allowed response, which is what
  proves Caddy is not adding a second one.
- The `db-dump` sidecar writing a valid `pg_dump` CUSTOM-format archive on
  start, checked with `pg_restore -l`.

**Two collisions were found that way and fixed, and they are the reason this
section is worth reading.** Both only appear in a checkout that already has the
development stack running, which is every developer's checkout and no VPS:
`docker-compose.prod.yml` and `docker-compose.yml` both resolved to the project
name `keel` and therefore shared the `keel_pgdata` volume, and every
`container_name` was identical in both files. The first meant a production stack
started in a developer's checkout would attach the developer's database; the
second meant it would not start at all. Reasoning about the file would not have
found either.

**NOT run, and each one needs the host that does not exist yet:**

- **The ACME certificate.** `KEEL_DOMAIN=localhost` deliberately avoids it, so
  nothing here has tested a real Let's Encrypt issuance, the port 80 challenge,
  or the apex-and-www reasoning in section 2. That reasoning is why the
  Caddyfile names one hostname; it is argued, not measured.
- **The SSH deploy.** `deploy-remote.sh` and the workflow's deploy step have
  never run end to end. The `.env` rewrite inside the script was checked as a
  text transformation on its own; the `git fetch`, the `compose pull` from
  `ghcr.io` and the SSH itself have not run anywhere.
- **The dump cadence.** The first dump was observed. The 24 hour sleep and the
  `KEEP_DAYS` retention delete were not waited out.
- **A restore.** No dump has been restored into an empty database. Section 8
  says this and it is the gap in section 8 most worth closing first.
