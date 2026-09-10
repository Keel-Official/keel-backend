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

set -euo pipefail

cd "$(dirname "$0")/../.."

COMPOSE_FILE_PATH="docker-compose.prod.yml"
SERVICE="postgres"
OUT_DIR="${1:-backups}"
KEEP_DAYS="${KEEL_DUMP_KEEP_DAYS:-14}"

if [ ! -f "$COMPOSE_FILE_PATH" ]; then
  echo "dump: $COMPOSE_FILE_PATH not found in $(pwd)" >&2
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
# `exec -T` and not `run --rm`: this uses the running container, so it needs no
# second Postgres and no password on the command line. pg_dump inside the
# container is the same major version as the server by construction.
if docker compose -f "$COMPOSE_FILE_PATH" exec -T "$SERVICE" \
     pg_dump -U keel -d keel -Fc > "${out}.partial"; then
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
