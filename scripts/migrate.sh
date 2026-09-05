#!/usr/bin/env bash
#
# migrate.sh
#
# Applies every file in migrations/ in filename order, exactly once each, and
# records what was applied in a schema_migrations table.
#
# WHY THIS EXISTS. docker-compose used to mount migrations/ into
# /docker-entrypoint-initdb.d, which Postgres runs ONLY when the data directory
# is empty. That works for the first file on a fresh volume and silently ignores
# every file after it. A migration that never runs, with nothing reporting that
# it never ran, is the failure mode this repository keeps finding. One mechanism
# is better than two, so the initdb mount is gone and this is the only path.
#
# Usage: bash scripts/migrate.sh        (or: make migrate)
#
# Requires the local Postgres from docker-compose to be up: make up
#
# Each file is applied inside ONE transaction together with its bookkeeping
# insert, so a file that fails leaves no trace and can be fixed and re-run.
# Postgres runs DDL transactionally, which is what makes that possible.

set -euo pipefail

cd "$(dirname "$0")/.."

# ONE MECHANISM, TWO TRANSPORTS, ADDED 5 SEPTEMBER 2026.
#
# Everything below this block is unchanged: the same files in the same order, the
# same schema_migrations bookkeeping, the same refusal to apply a file twice. What
# is new is that the psql it drives can be reached in two ways.
#
# WHY IT HAD TO GAIN ONE. The compose path can only ever address a Postgres inside
# the local compose project. `keel serve` refuses to start unless
# schema_migrations has rows, so a deployment with no way to apply the schema is a
# deployment that cannot start, and the smoke test in
# .github/workflows/deploy.yml is the first caller that is not a laptop.
#
# WHY IT IS NOT A SECOND MECHANISM, which is the rule this file's header sets. The
# ordering, the bookkeeping and the exactly-once guarantee live here and are
# shared. A copy of that logic written into a workflow, which was the alternative,
# is what the header forbids and for the reason it gives: a migration applied from
# two places is a migration nobody can say ran.
#
# Usage stays `bash scripts/migrate.sh` for the compose path. Set KEEL_MIGRATE_DSN
# to address anything else, and psql must be on PATH for that route.
if [ -n "${KEEL_MIGRATE_DSN:-}" ]; then
  transport="dsn"
  psql_run() {
    psql "$KEEL_MIGRATE_DSN" -v ON_ERROR_STOP=1 "$@"
  }
  if ! command -v psql >/dev/null 2>&1; then
    echo "migrate: KEEL_MIGRATE_DSN is set and psql is not on PATH" >&2
    exit 1
  fi
  if ! psql_run -c 'SELECT 1' >/dev/null 2>&1; then
    echo "migrate: cannot reach Postgres at KEEL_MIGRATE_DSN" >&2
    exit 1
  fi
else
  transport="compose"
  psql_run() {
    docker compose exec -T postgres psql -U keel -d keel -v ON_ERROR_STOP=1 "$@"
  }
  if ! docker compose ps postgres >/dev/null 2>&1; then
    echo "migrate: docker compose is not reachable. Run: make up" >&2
    echo "         or set KEEL_MIGRATE_DSN to address a database directly" >&2
    exit 1
  fi
  if ! psql_run -c 'SELECT 1' >/dev/null 2>&1; then
    echo "migrate: cannot reach Postgres in the postgres service. Run: make up" >&2
    exit 1
  fi
fi

# THE TRANSPORT IS PRINTED, because the failure this file was written after was a
# schema applied to one database while every client talked to another. A run that
# does not say which database it reached cannot be told apart from that.
printf "migrate: transport %s\n" "$transport"

psql_run -q <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

applied=$(psql_run -t -A -c 'SELECT version FROM schema_migrations ORDER BY version')

shopt -s nullglob
files=(migrations/*.sql)
if [ ${#files[@]} -eq 0 ]; then
  echo "migrate: no files in migrations/"
  exit 0
fi

count=0
for path in "${files[@]}"; do
  version=$(basename "$path")

  if printf '%s\n' "$applied" | grep -Fxq "$version"; then
    printf "  skip   %s\n" "$version"
    continue
  fi

  printf "  apply  %s\n" "$version"
  {
    cat "$path"
    printf "\nINSERT INTO schema_migrations (version) VALUES ('%s');\n" "$version"
  } | psql_run -q -1
  count=$((count + 1))
done

printf "migrate: %d applied, %d already present\n" "$count" $((${#files[@]} - count))
