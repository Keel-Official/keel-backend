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

psql_run() {
  docker compose exec -T postgres psql -U keel -d keel -v ON_ERROR_STOP=1 "$@"
}

if ! docker compose ps postgres >/dev/null 2>&1; then
  echo "migrate: docker compose is not reachable. Run: make up" >&2
  exit 1
fi

if ! psql_run -c 'SELECT 1' >/dev/null 2>&1; then
  echo "migrate: cannot reach Postgres in the postgres service. Run: make up" >&2
  exit 1
fi

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
