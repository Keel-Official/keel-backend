# Putting the API on the internet

**Status: PREPARED, NOT APPLIED. Nothing in this directory runs by itself.**
The deploy job in `.github/workflows/deploy.yml` is gated on the repository
variable `KEEL_DEPLOY_TARGET`, and until it is set the job writes a summary
saying what it is waiting for and deploys nothing.

Prepared 11 September 2026 by Claude. Same division as `scripts/s3-archive/` and
`scripts/history-migration/`: the compose file, the nginx server block, the dump
script, this document and the workflow steps are written here, and the box, its DNS,
its
password and the repository secrets are Al's. Claude cannot rent a VPS, must not
hold the key to one, and must not be the party that provisions the
infrastructure the deliverable's evidence is served from.

| File | What it is |
|---|---|
| `docker-compose.prod.yml` | the three services, at the repository root |
| `scripts/deploy/nginx-keel.conf` | TLS, the reverse proxy and the access log. Installed into `/etc/nginx` by hand. Section 3.8 |
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

**THERE WAS A FOURTH SERVICE AND IT IS GONE AS OF 11 SEPTEMBER 2026.** `caddy` held
the certificate, was the only container with a port open to the internet, wrote the
access log, and was deliberately kept off the same Docker network as `postgres`. It
was removed because this box already runs nginx for another application, and two
processes cannot both hold 80 and 443. The one that was already there keeps them.

**What moved, and where it went.** TLS, the HTTP redirect, the security headers and
the access log are now nginx's, on the host, from
`scripts/deploy/nginx-keel.conf`. Section 3.8 installs it. The `edge` network went
with Caddy, because it existed only to give Caddy a route to `keel-serve` without
giving it one to `postgres`, and nginx is not in this project at all.

**What it cost, and this is the part to carry:** the stack now has NO healthcheck on
the API. `keel-serve` is distroless, with no shell and no HTTP client, so it cannot
probe itself, and Caddy was the only container in the stack that could probe it.
`docker compose ps` will report `keel-serve` as running and tell you nothing about
whether it serves. The two checks that replace it are both outside the stack and
both in section 4.

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

**UNDER CADDY THIS WAS A STARTUP FAILURE. UNDER NGINX IT IS QUIETER, AND THAT IS
WORSE RATHER THAN BETTER.** The paragraph here read: Caddy asks for a certificate
for every hostname in the Caddyfile as soon as it loads, an ACME challenge for a
name whose DNS answers with Vercel's address cannot succeed because the CA connects
to the address the world sees, Caddy then retries with backoff, and the site that
*does* resolve here is degraded while it does. So adding `keels.app` "so the bare
domain redirects" did not add a redirect, it took the API down.

nginx does none of that. It will accept a `server_name keels.app;` block without
complaint and serve it, and the breakage moves to certbot, which still cannot solve
a challenge for a name pointed at Vercel. **The failure is then a renewal that
stops working weeks later rather than a site that will not start now.** A loud
failure at the moment of the mistake was the better of the two, and it is no longer
available.

`scripts/deploy/nginx-keel.conf` carries that reasoning in a comment at the top, so
the next person to reach for a second `server_name` reads it there rather than here.

**THE HOSTNAME IS NO LONGER CONFIGURATION, AND `KEEL_DOMAIN` IS NOW A DEAD
VARIABLE.** It used to live in the host's `.env`, read by the `caddy` service and
substituted into the Caddyfile as `{$KEEL_DOMAIN}`, with no default, so an unset
value stopped Caddy rather than serving the wrong name.

Nothing reads it now. Caddy was its only consumer: no Go file reads it, and with
that service removed no line of `docker-compose.prod.yml` does either. The name is
written into `scripts/deploy/nginx-keel.conf` as a literal, twice, because stock
nginx does no environment substitution in a config file and the envsubst templating
that the nginx *container* offers is not available to an nginx installed on the
host.

**So delete the `KEEL_DOMAIN` line from `.env`.** A variable that is set, looks
load bearing, and is read by nothing is the failure the deploy workflow's own
header names about a declared and unused Go version: somebody will later believe it
is being honoured, change it, and wonder why nothing moved. Section 3.4's table no
longer lists it.

If the hostname ever changes, it changes in `nginx-keel.conf` and in the
`KEEL_HEALTH_URL` repository variable, and those two are the whole of it.

---

## 3. First-time host setup

Placeholders to substitute: `<VPS_IPV4>`, `<VPS_IPV6>`, `<VPS_USER>`.

Steps are in dependency order and DNS is still first, though the reason changed
with Caddy. It read "Caddy asks for a certificate within seconds of its first
start". Nothing asks automatically now: certbot is run by hand in step 3.8, and it
fails if the record does not resolve to this box. So the record has to exist and
have propagated **before** step 3.8 rather than before 3.7, and the stack in 3.7
comes up perfectly well without it.

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
skipping it looks defensible.** It is not, for two reasons, and neither changed
when Caddy did. The ACME HTTP-01 challenge is solved on port 80, so with it closed
the certificate is never issued and there is no HTTPS to be "only" on. And port 80
is where the HTTP to HTTPS redirect lives: with it closed, anybody who types the
hostname without a scheme gets a connection timeout instead of a 301, which reads
as "the API is down".

**BOTH PORTS ARE PROBABLY ALREADY OPEN, because nginx was already serving another
application on this box before Keel arrived.** Check rather than assume, and check
that nginx and not something else holds them: `sudo ss -ltnp | grep -E ':(80|443)'`
should name `nginx`. If a `docker-proxy` process appears there, something published
a container port on a public address and section 3.7's warning applies to it.

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

Nothing else needs to be open, and in particular **the API's own port must not
be**. `keel-serve` used to publish nothing at all, because Caddy reached it over a
Docker network. It now publishes `127.0.0.1:3000` so that nginx on the host can
reach it, and loopback is the whole of what keeps that off the internet. The same
`ss` check applies: `ss -ltnp | grep 3000` must show `127.0.0.1:3000` and never
`0.0.0.0:3000`. A bare `"3000:3000"` in the compose file would serve the API over
plain HTTP on the public address, past nginx and past `ufw`, while `ufw status`
looked correct.

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
| `KEEL_CORS_ORIGINS` | yes | the dashboard's origins. Section 7 |
| `KEEL_IMAGE` | yes | `ghcr.io/keel-official/keel-backend`. It had a default until 11 September 2026 and `keel-serve` now requires it, while `keel-scan` still defaults. Setting it satisfies both |
| `KEEL_BIND_ADDR` | no | where the API listens on this box. Defaults to `127.0.0.1:3000`. Section 3.8 |
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

# The image, which stopped having a default in the compose file on 11 September
# 2026. Without this line compose stops and names the variable.
KEEL_IMAGE=ghcr.io/keel-official/keel-backend

# WHERE THE API LISTENS ON THIS BOX, for nginx to proxy to. Optional: leave it out
# and the compose file uses 127.0.0.1:3000. Set it if another application on this
# host already holds 3000, and change the proxy_pass port in
# scripts/deploy/nginx-keel.conf to match, because the two are one setting written
# in two files. Check first with `ss -ltnp | grep 3000`.
#
# 127.0.0.1 IS NOT DECORATION. Drop it and the API is served over plain HTTP on the
# public address, past nginx and past ufw. Section 3.2.
#KEEL_BIND_ADDR=127.0.0.1:3000

# THERE IS NO KEEL_DOMAIN LINE and one here would be read by nothing. Caddy was its
# only consumer and Caddy is gone; the hostname is a literal in
# scripts/deploy/nginx-keel.conf now. Section 2.

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

Three services running. **Nothing here reports whether the API actually serves**,
because the stack no longer holds a healthcheck for it: `keel-serve` is distroless
and Caddy, which used to probe it, is gone. `ps` says `running` for a container
that is crash looping between polls. Ask the API instead:

```bash
curl -s http://127.0.0.1:3000/v1/health          # or KEEL_BIND_ADDR, if set
docker compose -f docker-compose.prod.yml logs --tail 40 keel-serve
```

A JSON body with `"status": "degraded"` is the correct answer here. Section 4 reads
it field by field. **Nothing is reachable from the internet yet**: that is section
3.8, and this is the last step that works without DNS.

### 3.8 nginx and the certificate, and both are Al's

**This box already runs nginx, which is why Caddy was removed from the stack on 11
September 2026.** So this section is an nginx site added beside the one already
there, not an nginx installed. Nothing under `scripts/deploy/` is loaded from the
checkout: nginx reads `/etc/nginx/sites-enabled/`, so installing the server block
is a copy and a symlink.

```bash
cd /opt/keel
sudo cp scripts/deploy/nginx-keel.conf /etc/nginx/sites-available/keel
sudo ln -sfn /etc/nginx/sites-available/keel /etc/nginx/sites-enabled/keel
sudo nginx -t
```

**`nginx -t` FAILS AT THIS POINT AND THAT IS EXPECTED.** The file names certificate
paths under `/etc/letsencrypt/live/api.keels.app/` that do not exist until certbot
has run, and nginx refuses to load a TLS server block whose certificate is missing.
Two ways round it, and the first is less fiddly:

```bash
# Let certbot write the TLS half itself, from an HTTP-only server block.
sudo certbot --nginx -d api.keels.app
```

`certbot --nginx` edits the site in place, adds the `listen 443 ssl` half and the
certificate paths, and installs the renewal timer. **It will rewrite parts of this
file**, which is the cost of the easy road: the installed copy and
`scripts/deploy/nginx-keel.conf` then differ, and the one in git stops being the
record of what is serving. Diff them afterwards and carry anything certbot dropped
back by hand, in particular the four security headers and the proxy timeouts.

```bash
# Or issue the certificate first with the webroot plugin, then enable the site
# exactly as written, with no rewriting.
sudo certbot certonly --webroot -w /var/www/html -d api.keels.app
sudo nginx -t && sudo systemctl reload nginx
```

The second form is what `nginx-keel.conf` is written for: its port 80 block already
serves `/.well-known/acme-challenge/` from `/var/www/html` and redirects everything
else, which is both what certbot needs now and what renewal needs in ninety days.

**The renewal is the part that fails silently three months later.** Check that the
timer exists and that a dry run completes:

```bash
systemctl list-timers | grep certbot
sudo certbot renew --dry-run
```

**THE PROXY PORT IS ONE SETTING IN TWO FILES.** `proxy_pass` in the nginx file and
`KEEL_BIND_ADDR` in `.env` have to name the same port. They both default to
`127.0.0.1:3000`. If 3000 was already taken on this box, both move together, and
the symptom of moving only one is a 502 whose `/var/log/nginx/keel-error.log` line
reads `connect() failed (111: Connection refused)` and names the port nginx tried.

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
curl -s http://127.0.0.1:3000/v1/health          # or KEEL_BIND_ADDR, if set
sudo tail -5 /var/log/nginx/keel-access.log
```

**`ps` NO LONGER TELLS YOU WHETHER THE API SERVES, AND THIS IS THE ONE REGRESSION
THE NGINX MOVE CAUSED.** It used to: `caddy` carried a healthcheck that probed
`http://keel-serve:3000/v1/health`, so `healthy` in that column meant the API had
answered within the last thirty seconds and the proxy hop worked. Caddy was removed
on 11 September 2026 and there is nothing left in the stack to carry that probe.
`keel-serve`'s image is `gcr.io/distroless/static-debian12:nonroot`, which has no
shell, no curl and no wget, and `keel` has no health subcommand, so nothing inside
that container can make an HTTP request to itself.

**So `running` now means the process has not exited, and nothing more.** A
container crash looping between two polls reads as running. The `curl` above is
what replaced it, and the deploy job runs the same request over SSH before it
trusts the public one.

**Whatever probes this, it must assert HTTP 200 and never read the `status`
field.** `degraded` is a correct 200: the handler reports it whenever no scan has
been recorded, which is every deployment for up to fifteen minutes. That rule
outlived the healthcheck that first needed it.

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

**nginx adds no CORS header and must not be made to.** The allowlist lives in one
place, `internal/api`, and two `Access-Control-Allow-Origin` headers on one response
is not a lenient case a browser picks from, it is a hard failure.
`scripts/deploy/nginx-keel.conf` says so where somebody would otherwise add it, the
same way the Caddyfile did before 11 September 2026.

**NGINX HAS A SECOND WAY TO BREAK THIS THAT CADDY DID NOT.** An `add_header` inside
a `location` block replaces every header inherited from the `server` block rather
than adding to them. So a single `add_header` dropped into the `location /` block,
for CORS or for anything else, silently removes the four security headers above it.
The installed file carries that warning at the point where somebody would type it.

Verify from a laptop:

```bash
# An allowed origin gets its own origin back
curl -sI -H 'Origin: https://keels.app' https://api.keels.app/v1/health \
  | grep -i 'access-control-allow-origin'
# access-control-allow-origin: https://keels.app

# Exactly ONE such header, which is what proves nginx is not adding a second
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

# the three services, live
docker compose -f docker-compose.prod.yml logs -f

# one of them, bounded
docker compose -f docker-compose.prod.yml logs --since 1h keel-scan
docker compose -f docker-compose.prod.yml logs --tail 100 keel-serve
```

**THE ACCESS LOG IS NGINX'S NOW AND IT IS NOT IN THIS PROJECT.** It used to be a
JSON file on the `caddy_logs` volume, rolled by Caddy at 10 MiB with five kept, so
`docker compose exec caddy` reached it and `jq` could ask it questions. Since 11
September 2026 it is an ordinary nginx access log on the host, in the combined
format:

```bash
sudo tail -n 50 /var/log/nginx/keel-access.log
sudo tail -n 50 /var/log/nginx/keel-error.log

# status codes over the last few thousand requests
sudo tail -n 2000 /var/log/nginx/keel-access.log | awk '{print $9}' | sort | uniq -c | sort -rn
```

**WHAT THE MOVE COST, AND IT IS WORTH KNOWING BEFORE YOU NEED THE LOG.** Caddy wrote
JSON, so a question could be asked with `jq` and no regex. nginx writes the combined
format, so the commands above are `awk` over positional fields and they break on a
user agent containing a quote. Switching nginx to a JSON `log_format` is a few lines
and is deliberately not done here, because this box's other application already
reads its logs in the combined format and one host with two log formats is worse
than one awkward format.

**THE ROTATION IS LOGROTATE'S AND IT IS NOT AUTOMATIC FOR A NEW FILE NAME.** Caddy's
10 MiB times five was a disk budget rather than a retention policy: on a small VPS
the same disk holds Postgres and the dumps, and an access log that fills it takes
down the database, which is far worse than losing last week's requests. That budget
is now whatever `/etc/logrotate.d/nginx` says. Check that it covers these two files,
because a file nginx writes and logrotate does not know about grows without limit:

```bash
grep -r 'log/nginx' /etc/logrotate.d/ | head
sudo logrotate -d /etc/logrotate.d/nginx 2>&1 | grep -i keel
```

---

## 9. Troubleshooting

**`nginx -t` says `cannot load certificate ... No such file or directory`.** The
site is enabled and certbot has not run for this name yet. Section 3.8, and it is
expected rather than wrong: nginx refuses to load a TLS block whose certificate is
missing, which is the correct behaviour and the reason certbot comes after the copy
rather than before it.

**A paragraph that used to be here is gone with Caddy, and it is recorded because
the symptom was so misleading.** An empty `KEEL_DOMAIN` made Caddy say `server block
without any key is global configuration, and if used, it must be first`, which
describes the shape an empty variable left behind rather than the order of the
blocks. `KEEL_DOMAIN` is read by nothing now, so the error cannot occur. Section 2
says why the variable should be deleted from `.env` rather than left set.

**`docker compose` refuses to do anything and names a variable.** That is the
`${VAR:?...}` form working. Every REQUIRED value in section 3.4 uses it; the five
optional `KEEL_DB_*` rows do not and can never produce this message. Two causes:
the variable really is missing, or you are not in `/opt/keel`, because `.env` is
read only from the project directory.

**certbot cannot issue.** In this order: `dig +short api.keels.app A` from off the
box and check it is `<VPS_IPV4>`; check port 80 is open from off the box, in the
provider's firewall as well as `ufw`; check the challenge reaches the webroot, with
`sudo tail /var/log/nginx/keel-access.log` while certbot runs, looking for a request
to `/.well-known/acme-challenge/`; check no apex or www `server_name` has crept into
any enabled site, with `grep -r server_name /etc/nginx/sites-enabled/`. Let's Encrypt
allows five duplicate certificates per name per week, so find the cause before
retrying repeatedly, and **do not delete `/etc/letsencrypt/`**, which holds the
certificates and the ACME account key. The rate limit is per name and is not reset
by removing files.

**`502` from nginx, and the error log names the port.** Read
`sudo tail /var/log/nginx/keel-error.log` first, because it separates two causes
that look identical from outside. `connect() failed (111: Connection refused)` means
nothing is listening on the port nginx tried, which is either `keel-serve` being
down or **`proxy_pass` and `KEEL_BIND_ADDR` naming different ports**, section 3.8.
`upstream timed out` means it is listening and slow, which is the API's own problem
and not the proxy's.

If the port is right and nothing is listening, `keel-serve` is not running or not
listening.
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
  anybody types. `keel-serve` and `keel-scan` are both on it, and since 11 September
  2026 they are the only two containers in the project, so it is now the only
  network. It used to be the point of the split that `caddy` was deliberately NOT on
  it and a shell in the Caddy container could not reach the database. nginx is on
  the host, outside Docker entirely, so it has no route to the `data` network at
  all: the separation is stronger than the one it replaced, and it is a property of
  where nginx runs rather than of anything configured here.
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
`docker compose up -d`, curl the API on its loopback port over the same SSH
connection until it answers, then `curl $KEEL_HEALTH_URL` **from the runner over the
public internet** and fail unless the served `methodologyVersion` equals the
constant this commit compiles.

**The loopback step reads `KEEL_BIND_ADDR` out of `.env`** rather than assuming
3000, by grep and not by sourcing the file, because `.env` holds the database
password. It replaced a `docker inspect keel-prod-caddy` poll when Caddy was
removed: with no container healthcheck left in the stack, the probe had to move
onto the host, which has curl.

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
`docker-compose.prod.yml` and the `Caddyfile` AS THEY THEN WERE:** the full stack
with `KEEL_DOMAIN=localhost`, which was the one value that made Caddy use its own
internal CA and skip ACME entirely.

**THAT RUN NO LONGER DESCRIBES THIS STACK, AND SAYING SO IS THE POINT OF THIS
SECTION.** Caddy was removed hours later, on the same day, when the box turned out
to already run nginx. So the TLS half, the redirect, the header assertions and the
CORS header count in section 7 were all measured against a proxy that is not the one
serving now. What still holds from it is everything below Caddy: 3.5, 3.6 and the
compose file's own behaviour. **What has been measured nowhere is the nginx server
block in `scripts/deploy/nginx-keel.conf`**, which has never been loaded by an nginx
anywhere, and section 3.8 is written as instructions rather than as a transcript for
that reason. Sections 3.5, 3.6 and 3.7 verbatim, then the
checks in section 4 and section 7.

What that established, each of them a live response rather than a reading of the
code: `migrate: 5 applied` through the compose transport, which is why 3.5
quotes that number; the section 4 body exactly as printed, `assetsMonitored: 60`
included; port 80 answering `308`; `curl localhost:3000` not answering, so the
API really is reachable only through the proxy; and the whole of section 7,
including **exactly one** `Access-Control-Allow-Origin` header on an allowed
response, which is what proved Caddy added no second one. That assertion has to be
re-run against nginx, and section 7 is where it lives.

**Two collisions were found that way and fixed, and they are why this section
exists.** Both appear only in a checkout that already has the development stack
running, which is every developer's checkout and no VPS: the two compose files
both resolved to the project name `keel` and therefore shared the `keel_pgdata`
volume, and every `container_name` was identical in both. The first meant a
production stack started in a developer's checkout would attach the developer's
database; the second meant it would not start at all. Reasoning about the file
would not have found either.

**NOT run, and the host now exists, so this list is a work queue rather than a
statement about an absent box:**

- **The whole of nginx.** `scripts/deploy/nginx-keel.conf` has never been loaded by
  an nginx anywhere. Not `nginx -t`, not the proxy, not the header block, not the
  port 80 challenge location. It is a translation of a Caddyfile that did work, and
  a translation is not a measurement.
- **The certificate.** `KEEL_DOMAIN=localhost` let the one real run skip ACME
  entirely, so nothing has tested an issuance, the port 80 challenge, the renewal
  timer, or the apex-and-www reasoning in section 2. That reasoning is argued, not
  measured, and under nginx it fails quietly rather than loudly, which section 2
  says is the worse of the two.
- **The loopback publish.** `keel-serve` has never been started with a published
  port. The `127.0.0.1` binding is the whole of what keeps the API off the public
  internet and `ss -ltnp | grep 3000` is the check that has not been run.
- **The SSH deploy.** The workflow's deploy steps have never completed. As of 11
  September 2026 the SSH step itself has been reached and refused with
  `Permission denied (publickey,password)`, which at least proves
  `KEEL_DEPLOY_KNOWN_HOSTS` is right, because a wrong one fails earlier and
  differently. The `git fetch`, the `compose pull` from `ghcr.io` and the new
  loopback health probe have all run nowhere.
- **The cron dump on a schedule.** `dump-database.sh` has not been run against
  this stack, and the 14 day rotation has not been waited out.
- **A restore.** No dump has been restored into an empty database. Section 6.1
  says this, and it is the gap most worth closing first.
