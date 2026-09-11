#!/usr/bin/env bash
#
# dump-database.sh
#
# One database dump, one hash beside it, and old dumps deleted. Called from a
# host cron entry once a day. It is NOT a service: nothing on this box runs it
# except cron, and it exits.
#
# WHY CRON AND NOT A CONTAINER. An always-on container whose whole job is to
# sleep for 23 hours and 59 minutes is a process to supervise, a restart policy
# to reason about, and a log to read, in exchange for a schedule it cannot
# actually keep: a sleep loop drifts and cannot promise a time of day. One crontab
# line is visible in the one place an operator already looks for scheduled work.
# RUNBOOK.md section 6 has the entry.
#
# Usage, from the directory holding docker-compose.prod.yml:
#   bash scripts/deploy/dump-database.sh [output-dir]
#
# WHAT IT REUSES FROM scripts/s3-archive/, AND WHAT IT CANNOT.
#
# It reuses manifest.sh, which is pure sha256 in `sha256sum -c` format with no
# AWS in it at all. A dump with a hash beside it can be checked after a copy or a
# restore; a dump on its own cannot.
#
# IT DOES NOT UPLOAD, AND THAT IS BLOCKED ON AL RATHER THAN UNFINISHED. Three
# separate things in scripts/s3-archive/ stop a host cron job from using it, and
# all three are decisions rather than code:
#
#   1. NO BUCKET EXISTS. Every artifact in that directory carries <BUCKET> as a
#      placeholder, and its own RUNBOOK's section 1 is titled "the recommendation
#      is not yet".
#   2. ITS CREDENTIAL PATH IS GITHUB, NOT A HOST. github-oidc-trust-policy.json
#      trusts token.actions.githubusercontent.com with a StringEquals condition on
#      repo:Keel-Official/keel-backend:ref:refs/heads/main. A cron job on a VPS
#      has no OIDC token and cannot assume that role. It would need a long-lived
#      key on the box, which is a new decision about a new credential.
#   3. ITS POLICY WOULD REFUSE THE DUMP AND THEN KEEP IT FOREVER.
#      recorder-iam-policy.json allows s3:PutObject on <BUCKET>/recordings/*
#      only, and a dump is not a recording. The bucket is also created with
#      Object Lock in COMPLIANCE mode for 365 days, with an explicit Deny on
#      DeleteObject, which is right for evidence and wrong for a backup that has
#      to rotate.
#
# So this writes locally and RUNBOOK.md section 6 carries the offsite step marked
# BLOCKED, with those three items as what has to be settled. A dump on the same
# disk as the database survives a dropped table, a bad migration and a bad
# deploy. It does not survive losing the box, and nothing here pretends otherwise.

# THE TRANSPORT CHANGED ON 11 SEPTEMBER 2026 AND THE OLD ONE IS RECORDED HERE
# BECAUSE IT IS IN A CRONTAB SOMEWHERE. This script used to dump through
# `docker compose exec -T postgres pg_dump`, which needed no password on any
# command line and guaranteed the client matched the server's major version,
# because both were the same container. That container went away when the host's
# own Postgres briefly became the database, so `exec -T postgres` failed with "no
# such service" and a cron entry calling this would have reported a failed dump
# every day.
#
# It now runs `pg_dump` against a DSN, and the DSN comes from KEEL_DUMP_DSN in the
# ENVIRONMENT and never from an argument. That is not decoration: a DSN on the
# command line is visible to every user on the box in `ps`, and this one carries
# the database password. In the environment of a running process it is readable by
# its owner and by root, which is the same set that can read the dump itself.
#
# THE `postgres` SERVICE CAME BACK THE SAME DAY AND THIS SCRIPT DID NOT FOLLOW IT.
# THAT IS A DECISION, NOT AN OVERSIGHT, AND IT IS THE PART OF THIS HEADER WORTH
# READING. The compose transport is available again and it is genuinely better on
# two axes: no password anywhere, and the client version matched to the server for
# free. It is not taken because a backup that only works while the database happens
# to be a container in this compose project is a backup that breaks the next time
# somebody moves that decision, and that decision moved twice in one day. The DSN
# form works against a container, against a host install, and against a managed
# database. A backup is the wrong place to optimise for the current arrangement.
#
# NOTE THAT KEEL_DUMP_DSN IS NOT THE DSN THE CONTAINERS USE. This script runs on
# the HOST, so it reaches the database through the loopback port the compose file
# publishes: localhost:5433. The containers reach the same database as
# postgres:5432, a service name on the `data` network. Using the container's
# spelling here gives "could not translate host name"; using this one from inside a
# container gives a refused connection. The same split as the migration step;
# RUNBOOK.md section 9.
#
# THE PORT IS 5433 AND NOT 5432. This box runs one Postgres container per
# application and 5432 belongs to another one. A dump taken against 5432 either
# fails or, worse, succeeds against somebody else's database.
#
# THE CLIENT VERSION IS THE OPERATOR'S PROBLEM AND IT IS THE PRICE OF THE CHOICE
# ABOVE. pg_dump refuses to dump a server newer than itself, so the host needs a
# client at least as new as the server, which is 18.4. Ubuntu ships 16 and fails
# with "server version mismatch". RUNBOOK.md section 3.5 has the PGDG repository
# lines that install postgresql-client-18.

set -euo pipefail

cd "$(dirname "$0")/../.."

OUT_DIR="${1:-backups}"
KEEP_DAYS="${KEEL_DUMP_KEEP_DAYS:-14}"
DSN="${KEEL_DUMP_DSN:-}"

if [ -z "$DSN" ]; then
  echo "dump: KEEL_DUMP_DSN is not set." >&2
  echo "      This runs on the HOST, so the database is at localhost:5433, which is" >&2
  echo "      the loopback port docker-compose.prod.yml publishes. NOT postgres:5432," >&2
  echo "      which is the containers' spelling, and NOT 5432, which on this box is" >&2
  echo "      another application's Postgres." >&2
  echo "      See scripts/deploy/RUNBOOK.md section 6." >&2
  exit 1
fi

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "dump: pg_dump is not on PATH." >&2
  echo "      This runs on the host, not in the server's container, so it needs a" >&2
  echo "      client at least as new as the server, which is 18.4. Ubuntu ships 16" >&2
  echo "      and fails with \"server version mismatch\": install postgresql-client-18" >&2
  echo "      from the PGDG repository. RUNBOOK.md section 3.5 has the three lines." >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

ts=$(date -u +%Y-%m-%dT%H%M%SZ)
out="$OUT_DIR/keel-${ts}.dump"

# -Fc IS THE CUSTOM FORMAT, which is what pg_restore reads and what allows a
# single table to be pulled out of it. A plain SQL dump would restore only in
# full.
#
# THE DUMP IS WRITTEN TO A .partial NAME AND MOVED ONLY ON SUCCESS. An
# interrupted dump must never be left looking like a good one, because the moment
# it is needed is the moment nobody has time to check.
#
# THE DSN IS PASSED THROUGH THE ENVIRONMENT AND NOT AS AN ARGUMENT, for the reason
# in the header: argv is world readable in `ps` and this string holds a password.
# `pg_dump -d "$DSN"` would put it in argv, so PGDATABASE carries it instead;
# pg_dump accepts a full connection URI in that variable.
if PGDATABASE="$DSN" pg_dump -Fc > "${out}.partial"; then
  mv "${out}.partial" "$out"
else
  rm -f "${out}.partial"
  echo "dump: FAILED at ${ts}" >&2
  exit 1
fi

# The hash, in the format `sha256sum -c` and `shasum -a 256 -c` both read.
# scripts/s3-archive/manifest.sh works on a directory of recordings, so the
# single-file case is done here with the same tool it uses; see the header for
# why the upload half is not.
if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$OUT_DIR" && sha256sum "$(basename "$out")" > "$(basename "$out").sha256" )
elif command -v shasum >/dev/null 2>&1; then
  ( cd "$OUT_DIR" && shasum -a 256 "$(basename "$out")" > "$(basename "$out").sha256" )
else
  echo "dump: no sha256sum or shasum on PATH, wrote the dump without a hash" >&2
fi

size=$(du -h "$out" | cut -f1)
echo "dump: wrote $out ($size)"

# ROTATION IS BY AGE AND NOT BY COUNT, so a week with no dumps does not silently
# extend retention. -mtime +N is "older than N days".
find "$OUT_DIR" -maxdepth 1 -name 'keel-*.dump' -type f -mtime "+${KEEP_DAYS}" -delete
find "$OUT_DIR" -maxdepth 1 -name 'keel-*.dump.sha256' -type f -mtime "+${KEEP_DAYS}" -delete

remaining=$(find "$OUT_DIR" -maxdepth 1 -name 'keel-*.dump' -type f | wc -l | tr -d ' ')
echo "dump: ${remaining} dump(s) held, keeping ${KEEP_DAYS} days"
