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
| A Postgres that is not a throwaway | `postgres` | `keel serve` refuses to start when `schema_migrations` is empty |
| The read-only API | `keel-serve` | no endpoints |
| The scanner | `keel-scan` | health reads `degraded` forever and every asset returns 404 "no metrics yet" |

`caddy` is the fourth service and is not Keel: it holds the certificate, is the
only container with a port open, and writes the access log.

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

Nothing else needs to be open. `postgres` publishes no port at all and
`keel-serve` publishes none either: Caddy is the only route in.

### 3.3 The box, and check the architecture first

**The published image is `linux/amd64` only.** Neither build step in
`.github/workflows/deploy.yml` passes a `platforms:` input and the job runs on
`ubuntu-latest`, so there is no arm64 manifest. Many cheap instances are arm64,
and on one of those this stack does not start.

```bash
uname -m     # must print x86_64
```

If it prints `aarch64`, stop: either use an x86_64 instance, or the image job
needs `platforms: linux/amd64,linux/arm64`, which is a change to a file this
runbook does not own. Do not reach for qemu binfmt emulation for a service that
runs continuously.

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
Nothing in `.env` has a default in the compose file: every reference is
`${VAR:?...}`, so a missing value stops `docker compose` with a message naming
the variable instead of starting something half configured.

| Variable | Required | What it is |
|---|---|---|
| `KEEL_IMAGE_TAG` | yes | the commit SHA to run. **Never `latest`.** The deploy job rewrites this line, and rollback is editing it |
| `KEEL_DSN` | yes | the Postgres DSN `keel serve` and `keel scan` connect with |
| `POSTGRES_PASSWORD` | yes | the same password, for the database itself |
| `KEEL_DOMAIN` | yes | `api.keels.app`. Section 2 |
| `KEEL_CORS_ORIGINS` | yes | the dashboard's origins. Section 7 |
| `KEEL_IMAGE` | no | defaults to `ghcr.io/keel-official/keel-backend` |
| `KEEL_ACME_EMAIL` | no | Let's Encrypt expiry notices. Needs one line uncommented in the Caddyfile |

`POSTGRES_USER=keel` and `POSTGRES_DB=keel` are literals in the compose file
rather than entries here. Neither is a secret, and a value kept in two places is
a value that drifts.

**THE PASSWORD APPEARS TWICE AND THAT IS THE ONE TRAP IN THIS FILE.**
`POSTGRES_PASSWORD` and the password inside `KEEL_DSN` must be the same string.
If they disagree, Postgres starts perfectly, `keel serve` fails authentication,
and Caddy answers 502 with nothing in it that says why. So generate both from
one shell variable in one command and never type either by hand:

```bash
cd /opt/keel

PW="$(openssl rand -base64 24 | tr -d '/+=')"
cat > .env <<ENVFILE
# The commit SHA this box runs. The deploy job rewrites this line. Never latest.
KEEL_IMAGE_TAG=REPLACE_WITH_A_COMMIT_SHA

# The database. The host is the compose service name and the port is the
# CONTAINER port, 5432. See section 9 before changing either.
KEEL_DSN=postgres://keel:${PW}@postgres:5432/keel?sslmode=disable
POSTGRES_PASSWORD=${PW}

# The hostname Caddy serves and requests a certificate for. Section 2.
KEEL_DOMAIN=api.keels.app

# The dashboard's origins, exact, comma separated, no trailing slash, no
# wildcard. These are NOT this API's own origin. Section 7.
KEEL_CORS_ORIGINS=https://keels.app,https://www.keels.app
ENVFILE
unset PW

chmod 600 .env
grep -c '^KEEL_\|^POSTGRES_' .env     # 5
```

`chmod 600` because this file holds the database password and `docker compose`
reads it as the invoking user. `sslmode=disable` is correct in that DSN and only
there: the connection never leaves the box, it crosses a private bridge network
between two containers, and requiring TLS would mean issuing a certificate for a
hostname that exists only inside Docker.

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
docker compose -f docker-compose.prod.yml up -d postgres
COMPOSE_FILE=docker-compose.prod.yml bash scripts/migrate.sh
```

**`scripts/migrate.sh` and nothing else.** Its own header states the rule: a
migration applied from two places is a migration nobody can say ran. It holds
the ordering, the exactly-once bookkeeping in `schema_migrations`, and the
per-file transaction.

**WHY THE COMPOSE TRANSPORT AND NOT `KEEL_MIGRATE_DSN`.** That script has two
transports. `KEEL_MIGRATE_DSN=... bash scripts/migrate.sh` drives `psql` over
TCP and is the right form for a Postgres this compose file does not own, a
managed database for instance. It cannot work **here**, because `postgres`
publishes no host port at all: there is no address on the host for `psql` to
connect to, by design, and adding one to run a migration would open the database
to the internet for the sake of one command. `COMPOSE_FILE=...` points the
script's other transport, `docker compose exec -T postgres psql`, at this
project instead of at `docker-compose.yml`. It needs no port and no `psql` on
the host.

Expect `migrate: transport compose` and then
`migrate: 5 applied, 0 already present`. `keel serve` will now start; before
this it refuses to, and that refusal is the point.

**`.env` is read automatically and only from the project directory**, which is
why every command in this runbook starts with `cd /opt/keel`. Run one from
elsewhere and `docker compose` stops with "required variable POSTGRES_PASSWORD
is missing a value", which is the `${VAR:?}` form reporting a missing file
rather than a missing variable.

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
17 3 * * * cd /opt/keel && /usr/bin/env bash scripts/deploy/dump-database.sh >> /var/log/keel-dump.log 2>&1
```

Run it once by hand first, because a cron entry that has never worked is a
backup nobody has:

```bash
cd /opt/keel
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
docker run --rm -v "$PWD/backups:/dumps:ro" postgres:16-alpine \
  pg_restore -l "/dumps/$(basename "$latest")" | head -8
# ;     Format: CUSTOM   <- this line is the one to look for
```

**Use a mounted directory and not a pipe for that second command.** The obvious
form, `docker compose exec -T postgres pg_restore -l /dev/stdin < file`, fails
with `did not find magic string in file header`: `pg_restore -l` on a custom
format archive needs a seekable file, and stdin through `exec` is not one. That
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
`${VAR:?...}` form working. Every value in section 3.4 is required and none has
a default. Two causes: the variable really is missing, or you are not in
`/opt/keel`, because `.env` is read only from the project directory.

**No certificate, and the Caddy log retries.** In this order: `dig +short
api.keels.app A` from off the box and check it is `<VPS_IPV4>`; check port 80 is
open from off the box, in the provider's firewall as well as `ufw`; check the
Caddyfile names only `{$KEEL_DOMAIN}` and that no apex or www block has crept
in. Let's Encrypt allows five duplicate certificates per name per week, so find
the cause before restarting repeatedly, and **do not delete the `caddy_data`
volume**, which holds the certificates and the ACME account key.

**`502` from Caddy.** `keel-serve` is not running or not listening.
`docker compose -f docker-compose.prod.yml logs keel-serve`. Two usual causes:
`serve` refusing to start because `schema_migrations` is empty, which is section
3.5 not having been run against this database, or the password mismatch below.

**`502`, and `keel-serve`'s log says `password authentication failed for user
"keel"`.** `POSTGRES_PASSWORD` and the password inside `KEEL_DSN` disagree.
Section 3.4 generates both from one variable for exactly this reason. Note that
changing `POSTGRES_PASSWORD` in `.env` after the volume exists does **not**
change the password in the database: Postgres set it on first initialisation and
ignores the variable afterwards. Either put the original password back into
`.env`, or change it in the database:

```bash
docker compose -f docker-compose.prod.yml exec postgres \
  psql -U keel -d keel -c "ALTER USER keel PASSWORD 'the-one-in-KEEL_DSN';"
```

**`connection refused` to Postgres, and the port number is the reason.** This is
the 5433 story from `docker-compose.yml`, in its production form, and the
production form is different enough to be worth stating separately.

In development, `docker-compose.yml` publishes Postgres as `"5433:5432"`. The
host side is 5433 and the container side is 5432, and the comment on that line
records what the old `5432:5432` cost: a Postgres already installed on the host
takes that port first, the host server binds `127.0.0.1` while Docker binds the
wildcard, so `localhost:5432` reaches the *host's* server and the symptom is
`role "keel" does not exist` rather than a refused connection. A whole day went
into that, and `make migrate` never noticed because it goes through
`docker compose exec` and touches no published port at all.

**In production neither number is published.** `postgres` in
`docker-compose.prod.yml` has no `ports:` key. So:

- **Inside `KEEL_DSN`, the port is 5432 and the host is `postgres`.** That is
  the container port on the compose network. Writing 5433 there fails as a
  refused connection, because 5433 is a host-side number that this file never
  creates.
- **From the host shell, no port works, and that is correct.** `psql -h
  localhost -p 5432` and `-p 5433` both fail. There is no address to connect to
  from outside the compose network, which is the single most valuable line in
  that file: a Postgres reachable from the internet on a box with one password
  is the worst thing this stack could do.
- **So `KEEL_MIGRATE_DSN` cannot be used here**, and section 3.5 explains the
  substitute. To get a shell on the database, go through the container:

```bash
docker compose -f docker-compose.prod.yml exec postgres psql -U keel -d keel
```

  If a port must be published temporarily, bind it to loopback only,
  `"127.0.0.1:5433:5432"`, and remove it afterwards. Do not leave it.

**`health` reads `degraded` forever with `assetsMonitored` above 0.** Look at
the scanner: `docker compose -f docker-compose.prod.yml logs keel-scan`. A scan
that cannot reach Horizon, or that fails on every asset, is a degraded status
reported correctly.

**`assetsMonitored: 0`.** Section 3.6 was not run, or was run against a
different database.

**`exec format error` on `up -d`.** The box is arm64 and the image is amd64.
Section 3.3.

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

**It runs on version tags only.** The image build also runs on
`workflow_dispatch`, and the deploy job deliberately does not: a manual dispatch
builds and publishes, and deploying is a decision that gets a tag. So the way to
deploy is:

```bash
git tag -a v0.3.0 -m "..." && git push origin v0.3.0
```

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
