#!/usr/bin/env bash
#
# probe-hubble-contract-state.sh
#
# ONE QUESTION, AND NOTHING BEYOND IT. Is one Soroban contract data entry present
# in Hubble at one historical ledger, and was it live or archived at that point.
#
# THIS IS NOT THE START OF A DATA PATH. It creates no Go type, no package, and
# nothing another part of this repository can import. DEC-002 defers BigQuery as a
# source for the engine and this script does not reopen that. It answers a yes/no
# question that came out of docs/evidences/2026-08-31-ustry-reserve-config-history.md
# section 6, where the February 2026 ReserveConfig value was found to be
# unreachable from public unauthenticated sources and Hubble was named as the one
# route that would close it. If the answer here is no, the route is closed too and
# nobody has to build anything to find that out.
#
# WHY THE SCHEMA IS DISCOVERED AT RUNTIME RATHER THAN WRITTEN IN. The brief for
# this script says to confirm table and column names against the current schema and
# not to rely on names from memory or from documentation examples. Neither bq nor
# gcloud is installed on the machine where this file was written, so that
# confirmation could not be done by the author. Writing remembered names into the
# query and calling them confirmed would be the exact failure the brief forbids, so
# the confirmation is a STAGE OF THIS SCRIPT instead: it reads INFORMATION_SCHEMA,
# binds every column it needs to a column that actually exists, and REFUSES to
# build the probe if any of them is missing. A missing column prints the full
# column list and the override variable to set. It never guesses.
#
# WHAT RUNS AND WHAT DOES NOT.
#   Schema discovery queries DO run. They read INFORMATION_SCHEMA, which is
#   metadata and bills zero bytes, and the probe cannot be built without them.
#   Set NO_DISCOVERY=1 to print that SQL and stop without contacting BigQuery.
#   The PROBE query does NOT run. It is printed, dry run for a byte estimate, and
#   then the script exits. Only RUN=1 executes it.
#
# NO FLOATING POINT ANYWHERE, INCLUDING INTERMEDIATES. bash has no floats, which is
# one of the reasons this is bash and not Python. The cost estimate is integer
# arithmetic in micro-dollars and is truncated, never rounded, so it under-reports
# rather than over-reports. See estimate_cost_micro.
#
# THE RECORDER PRINCIPLE APPLIES. Raw bytes are stored unchanged. Nothing is
# decoded and nothing is computed from what is read. c_factor and l_factor are NOT
# turned into decimal values here. A reading that was arithmetic-ed at record time
# cannot later be checked against the arithmetic.
#
# Usage:
#   POOL_CONTRACT_ID=C... TARGET_LEDGER=61340408 MODE=asof LEDGER_LOOKBACK=... \
#     BILLING_PROJECT=my-project bash scripts/probe-hubble-contract-state.sh
#
# Exit codes: 0 the probe was built and dry run, or printed under NO_DISCOVERY.
#             1 a parameter, a tool, or a column is missing. Nothing was billed.

set -euo pipefail

# ---------------------------------------------------------------------------
# Parameters. NONE of the four values the brief calls Al's is defaulted here.
# A default for POOL_CONTRACT_ID or TARGET_LEDGER would be this script inventing
# the thing it was told not to invent, and a probe that runs with no arguments is
# a probe that answers a question nobody asked.
# ---------------------------------------------------------------------------

POOL_CONTRACT_ID="${POOL_CONTRACT_ID:-}"   # required. The Blend pool contract address
TARGET_LEDGER="${TARGET_LEDGER:-}"         # required. Ledger sequence for 22 February 2026
STORAGE_KEY="${STORAGE_KEY:-}"             # optional. Omit to probe every entry of the contract
MODE="${MODE:-}"                           # required. "at" or "asof", see below
LEDGER_LOOKBACK="${LEDGER_LOOKBACK:-}"     # required when MODE=asof
TARGET_DATE="${TARGET_DATE:-}"             # optional. Prunes the partition, see bound_cost note
BILLING_PROJECT="${BILLING_PROJECT:-}"     # required. Public datasets still bill somebody

# The dataset named in the brief. Overridable, and verified rather than trusted:
# stage 2 fails loudly if INFORMATION_SCHEMA is not readable at this location.
HUBBLE_PROJECT="${HUBBLE_PROJECT:-crypto-stellar}"
HUBBLE_DATASET="${HUBBLE_DATASET:-crypto_stellar}"
HUBBLE_TABLE="${HUBBLE_TABLE:-}"           # optional. Skips table discovery when set

# On-demand analysis pricing, in micro-dollars per TiB scanned. THIS IS AN
# ASSUMPTION AND NOT A READING. It could not be verified from this machine, so it
# is a parameter and it is labelled as assumed everywhere it is printed. The byte
# count is the fact; the money is arithmetic over a rate you must confirm.
RATE_MICRO_USD_PER_TIB="${RATE_MICRO_USD_PER_TIB:-6250000}"

OUT_DIR="${OUT_DIR:-docs/evidences}"
RUN="${RUN:-0}"
NO_DISCOVERY="${NO_DISCOVERY:-0}"

readonly BYTES_PER_MIB=1048576
readonly MIB_PER_TIB=1048576

# ---------------------------------------------------------------------------
# Output helpers. Colour only on a terminal, the same rule
# scripts/audit-verification.sh carries and for the same reason: a report that is
# only correct on a terminal is not a report, because the reason to write one is
# that something else reads it.
# ---------------------------------------------------------------------------

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  bold=$'\033[1m'; dim=$'\033[90m'; red=$'\033[31m'; off=$'\033[0m'
else
  bold=''; dim=''; red=''; off=''
fi

section() { printf '\n%s%s%s\n' "$bold" "$1" "$off"; }
note()    { printf '%s       %s%s\n' "$dim" "$1" "$off"; }
die()     { printf '%sFATAL  %s%s\n' "$red" "$1" "$off" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Stage 0. Preflight.
# ---------------------------------------------------------------------------

require_param() {
  local name="$1" value="$2" why="$3"
  [ -n "$value" ] || die "$name is not set. $why"
}

is_integer()    { case "$1" in ''|*[!0-9]*) return 1 ;; *) return 0 ;; esac; }
is_identifier() { printf '%s' "$1" | grep -Eq '^[A-Za-z_][A-Za-z0-9_]*$'; }

preflight() {
  section "Stage 0. Preflight"

  command -v bq >/dev/null 2>&1 || die \
    "bq is not on PATH. Install the Google Cloud SDK. Nothing was billed."

  require_param POOL_CONTRACT_ID "$POOL_CONTRACT_ID" \
    "It is Al's value. docs/evidences/2026-08-31-ustry-reserve-config-history.md section 0 records the address; this script will not default to it."
  require_param TARGET_LEDGER "$TARGET_LEDGER" \
    "It is Al's value: the ledger sequence for 22 February 2026."
  require_param BILLING_PROJECT "$BILLING_PROJECT" \
    "A public dataset is free to read and still bills a project for bytes scanned."
  require_param MODE "$MODE" \
    "Set MODE=at or MODE=asof. The two answer different questions and the choice is methodological, so it is not defaulted."

  is_integer "$TARGET_LEDGER" || die "TARGET_LEDGER must be a positive integer, got: $TARGET_LEDGER"
  is_integer "$RATE_MICRO_USD_PER_TIB" || die "RATE_MICRO_USD_PER_TIB must be an integer number of micro-dollars"

  case "$MODE" in
    at)
      note 'MODE=at    asks: did this entry CHANGE at exactly TARGET_LEDGER.'
      note '           A "no" here does not mean the entry was absent, only that it was not written.'
      ;;
    asof)
      require_param LEDGER_LOOKBACK "$LEDGER_LOOKBACK" \
        "MODE=asof needs a window. It is a cost and correctness tradeoff, so it is Al's number and not a default."
      is_integer "$LEDGER_LOOKBACK" || die "LEDGER_LOOKBACK must be a positive integer"
      note 'MODE=asof  asks: was this entry LIVE as of TARGET_LEDGER, taking the newest'
      note "           row in [TARGET_LEDGER - $LEDGER_LOOKBACK, TARGET_LEDGER]."
      note '           A window too small reports absent for an entry last written before it.'
      ;;
    *) die "MODE must be exactly \"at\" or \"asof\", got: $MODE" ;;
  esac

  is_identifier "$HUBBLE_DATASET" || die "HUBBLE_DATASET is not a plain identifier: $HUBBLE_DATASET"

  if [ -z "$STORAGE_KEY" ]; then
    note 'STORAGE_KEY is unset, so the probe covers EVERY entry of the contract.'
    note 'That scans and returns more. The brief permits it when the key is not known.'
  fi
}

# ---------------------------------------------------------------------------
# Stage 1 and 2. Schema discovery. This is the part the brief insists on and it is
# why the query below is assembled rather than written out.
#
# INFORMATION_SCHEMA is metadata. It bills zero bytes, so it is queried for real.
# NO_DISCOVERY=1 prints the SQL and stops for anyone who wants this script to
# contact nothing at all.
# ---------------------------------------------------------------------------

bq_query() {
  # --format=csv with tail -n +2 drops the header. --quiet keeps the job chatter
  # out of the value. Every caller reads plain rows.
  #
  # stderr is deliberately NOT discarded. It does not reach the caller's variable,
  # because command substitution captures stdout only, so letting it through costs
  # nothing and a swallowed permission error is the hardest kind of failure to
  # diagnose from an empty result.
  bq --project_id="$BILLING_PROJECT" --quiet --headless \
     query --use_legacy_sql=false --format=csv "$1" | tail -n +2
}

sql_list_tables() {
  cat <<SQL
SELECT table_name
FROM \`${HUBBLE_PROJECT}.${HUBBLE_DATASET}.INFORMATION_SCHEMA.TABLES\`
WHERE LOWER(table_name) LIKE '%contract%'
ORDER BY table_name
SQL
}

sql_list_columns() {
  cat <<SQL
SELECT column_name, data_type, is_partitioning_column
FROM \`${HUBBLE_PROJECT}.${HUBBLE_DATASET}.INFORMATION_SCHEMA.COLUMNS\`
WHERE table_name = '${1}'
ORDER BY ordinal_position
SQL
}

discover_tables() {
  section "Stage 1. Which contract tables actually exist"

  if [ -n "$HUBBLE_TABLE" ]; then
    is_identifier "$HUBBLE_TABLE" || die "HUBBLE_TABLE is not a plain identifier: $HUBBLE_TABLE"
    note "HUBBLE_TABLE was supplied, so discovery is skipped: $HUBBLE_TABLE"
    return 0
  fi

  local rows
  rows="$(bq_query "$(sql_list_tables)")" || true
  [ -n "$rows" ] || die \
    "No table in ${HUBBLE_PROJECT}.${HUBBLE_DATASET} has \"contract\" in its name, or INFORMATION_SCHEMA is not readable with this project. Check BILLING_PROJECT and its permissions."

  # if/then rather than a trailing AND-list: on bash 3.2 a piped while whose last
  # iteration short-circuits returns 1, and under set -e that kills the script.
  # Verified on the machine this was written on before it was written this way.
  printf '%s\n' "$rows" | while IFS= read -r t; do
    if [ -n "$t" ]; then printf '       %s\n' "$t"; fi
  done

  local count
  count="$(printf '%s\n' "$rows" | grep -c . || true)"
  if [ "$count" -eq 1 ]; then
    HUBBLE_TABLE="$rows"
    note "exactly one candidate, using it: $HUBBLE_TABLE"
  else
    die "$count candidate tables. This script will not pick one for you: set HUBBLE_TABLE to the one that holds contract data entries."
  fi
}

# Column roles the probe needs, and the candidate names each role is matched
# against. A candidate list is NOT the same as trusting a remembered name: every
# role is bound only to a column INFORMATION_SCHEMA actually returned, and an
# unmatched role stops the script with the real column list printed. Override any
# role directly with COL_<ROLE>=<column_name>.
COL_CONTRACT_ID="${COL_CONTRACT_ID:-}"
COL_LEDGER_SEQ="${COL_LEDGER_SEQ:-}"
COL_DURABILITY="${COL_DURABILITY:-}"
COL_LIVE_UNTIL="${COL_LIVE_UNTIL:-}"
COL_RAW_VALUE="${COL_RAW_VALUE:-}"
COL_KEY="${COL_KEY:-}"
COL_DELETED="${COL_DELETED:-}"
COL_CLOSED_AT="${COL_CLOSED_AT:-}"
PARTITION_COLUMN=""
ALL_COLUMNS=""

bind_role() {
  # bind_role <role-label> <current-value> <candidate> [candidate...]
  # Echoes the bound column name, or nothing when no candidate exists.
  local label="$1"; shift
  local current="$1"; shift
  if [ -n "$current" ]; then
    if printf '%s\n' "$ALL_COLUMNS" | grep -Fxq "$current"; then
      printf '%s' "$current"; return 0
    fi
    die "$label was overridden to \"$current\", which is not a column of $HUBBLE_TABLE."
  fi
  local c
  for c in "$@"; do
    if printf '%s\n' "$ALL_COLUMNS" | grep -Fxq "$c"; then
      printf '%s' "$c"; return 0
    fi
  done
  printf ''
}

discover_columns() {
  section "Stage 2. Confirm the column names against the live schema"

  local rows
  rows="$(bq_query "$(sql_list_columns "$HUBBLE_TABLE")")" || true
  [ -n "$rows" ] || die "$HUBBLE_TABLE returned no columns. Wrong table name, or no access."

  ALL_COLUMNS="$(printf '%s\n' "$rows" | awk -F, 'NF{print $1}')"
  PARTITION_COLUMN="$(printf '%s\n' "$rows" | awk -F, '$3=="YES"{print $1; exit}')"

  printf '       %s columns in %s\n' "$(printf '%s\n' "$ALL_COLUMNS" | grep -c . || true)" "$HUBBLE_TABLE"
  if [ -n "$PARTITION_COLUMN" ]; then
    note "partitioning column: $PARTITION_COLUMN"
  else
    note 'no partitioning column reported. See the cost warning in stage 3.'
  fi

  COL_CONTRACT_ID="$(bind_role COL_CONTRACT_ID "$COL_CONTRACT_ID" contract_id contract_address)"
  COL_LEDGER_SEQ="$(bind_role COL_LEDGER_SEQ  "$COL_LEDGER_SEQ"  ledger_sequence last_modified_ledger closed_at_ledger)"
  COL_DURABILITY="$(bind_role COL_DURABILITY  "$COL_DURABILITY"  contract_durability durability)"
  COL_LIVE_UNTIL="$(bind_role COL_LIVE_UNTIL  "$COL_LIVE_UNTIL"  live_until_ledger_seq liveUntilLedgerSeq)"
  COL_RAW_VALUE="$(bind_role  COL_RAW_VALUE   "$COL_RAW_VALUE"   contract_data_xdr val_xdr val)"
  COL_KEY="$(bind_role        COL_KEY         "$COL_KEY"         key key_xdr contract_key_type)"
  COL_DELETED="$(bind_role    COL_DELETED     "$COL_DELETED"     deleted is_deleted)"
  COL_CLOSED_AT="$(bind_role  COL_CLOSED_AT   "$COL_CLOSED_AT"   closed_at batch_run_date)"

  local role missing=0
  for role in COL_CONTRACT_ID COL_LEDGER_SEQ COL_DURABILITY COL_LIVE_UNTIL COL_RAW_VALUE; do
    if [ -z "${!role}" ]; then
      printf '%s       UNBOUND  %s%s\n' "$red" "$role" "$off"
      missing=1
    else
      printf '       bound    %-18s -> %s\n' "$role" "${!role}"
    fi
  done
  [ -n "$COL_KEY" ]       && printf '       bound    %-18s -> %s\n' COL_KEY "$COL_KEY"
  [ -n "$COL_DELETED" ]   && printf '       bound    %-18s -> %s\n' COL_DELETED "$COL_DELETED"
  [ -n "$COL_CLOSED_AT" ] && printf '       bound    %-18s -> %s\n' COL_CLOSED_AT "$COL_CLOSED_AT"

  if [ "$missing" -eq 1 ]; then
    printf '\n       columns actually present in %s:\n' "$HUBBLE_TABLE"
    printf '%s\n' "$ALL_COLUMNS" | while IFS= read -r c; do
      if [ -n "$c" ]; then printf '         %s\n' "$c"; fi
    done
    die "A required role is unbound. Set it explicitly, for example COL_LIVE_UNTIL=<column>. This script will not substitute a column it was not told about."
  fi

  if [ -n "$STORAGE_KEY" ] && [ -z "$COL_KEY" ]; then
    die "STORAGE_KEY was supplied but no key column is bound. Set COL_KEY explicitly or unset STORAGE_KEY to probe the whole contract."
  fi
}

# ---------------------------------------------------------------------------
# Stage 3. Build the probe, bound its cost, dry run it, stop.
#
# BOUNDING THE COST. Published guidance is that LIMIT does not reduce bytes
# scanned, so the bound has to be a WHERE clause on a column BigQuery can prune
# on. Two clauses do that here and they are not interchangeable:
#   the ledger range   narrows rows, and prunes only if the table is clustered on it
#   TARGET_DATE        prunes the PARTITION, which is what actually cuts the bill
# TARGET_DATE is optional because inventing a date is inventing a parameter. When
# it is absent the dry run below is what tells you the truth, which is the whole
# reason the dry run is not skippable.
#
# Identifiers are interpolated because SQL cannot parameterise them, so every one
# passed through is_identifier before reaching this point. VALUES are passed as
# query parameters and never interpolated.
# ---------------------------------------------------------------------------

PROBE_SQL=""
declare -a BQ_PARAMS=()

build_probe() {
  section "Stage 3. The probe query"

  local ledger_clause
  if [ "$MODE" = "at" ]; then
    ledger_clause="t.${COL_LEDGER_SEQ} = @target_ledger"
  else
    ledger_clause="t.${COL_LEDGER_SEQ} BETWEEN @lookback_floor AND @target_ledger"
  fi

  local key_clause=""
  [ -n "$STORAGE_KEY" ] && key_clause="  AND t.${COL_KEY} = @storage_key"$'\n'

  local date_clause=""
  if [ -n "$TARGET_DATE" ] && [ -n "$PARTITION_COLUMN" ]; then
    date_clause="  AND t.${PARTITION_COLUMN} = @target_date"$'\n'
  fi

  local deleted_select="  CAST(NULL AS STRING) AS deleted_flag,"
  [ -n "$COL_DELETED" ] && deleted_select="  CAST(t.${COL_DELETED} AS STRING) AS deleted_flag,"

  # Every projected column is STRING or INT64. No FLOAT64 and no NUMERIC appears
  # anywhere, including as an intermediate, which is requirement 6.
  PROBE_SQL="$(cat <<SQL
SELECT
  t.${COL_CONTRACT_ID}                       AS contract_id,
  CAST(t.${COL_LEDGER_SEQ} AS INT64)         AS entry_ledger_seq,
  CAST(t.${COL_DURABILITY} AS STRING)        AS contract_durability,
  CAST(t.${COL_LIVE_UNTIL} AS INT64)         AS live_until_ledger_seq,
  CASE
    WHEN CAST(t.${COL_LIVE_UNTIL} AS INT64) IS NULL THEN 'unknown'
    WHEN CAST(t.${COL_LIVE_UNTIL} AS INT64) >= @target_ledger THEN 'live_at_target'
    ELSE 'archived_at_target'
  END                                        AS liveness_at_target,
${deleted_select}
  CAST(t.${COL_RAW_VALUE} AS STRING)         AS raw_entry_unchanged
FROM \`${HUBBLE_PROJECT}.${HUBBLE_DATASET}.${HUBBLE_TABLE}\` AS t
WHERE t.${COL_CONTRACT_ID} = @pool_contract_id
  AND ${ledger_clause}
${key_clause}${date_clause}ORDER BY CAST(t.${COL_LEDGER_SEQ} AS INT64) DESC
SQL
)"

  BQ_PARAMS=(
    "--parameter=pool_contract_id:STRING:${POOL_CONTRACT_ID}"
    "--parameter=target_ledger:INT64:${TARGET_LEDGER}"
  )
  [ "$MODE" = "asof" ] && BQ_PARAMS+=( "--parameter=lookback_floor:INT64:$((TARGET_LEDGER - LEDGER_LOOKBACK))" )
  [ -n "$STORAGE_KEY" ] && BQ_PARAMS+=( "--parameter=storage_key:STRING:${STORAGE_KEY}" )
  [ -n "$date_clause"  ] && BQ_PARAMS+=( "--parameter=target_date:DATE:${TARGET_DATE}" )

  printf '%s\n' "$PROBE_SQL"
  printf '\n'
  local p
  for p in "${BQ_PARAMS[@]}"; do printf '       %s\n' "$p"; done

  if [ -z "$date_clause" ]; then
    printf '\n%s       COST WARNING: the partition column is not constrained.%s\n' "$red" "$off"
    note 'Set TARGET_DATE to the partition value covering TARGET_LEDGER. Without it the'
    note 'scan may cover the whole table. The estimate below is the only honest answer.'
  fi
}

# Integer arithmetic only, in micro-dollars, truncating at both divisions so the
# figure under-reports. bytes -> MiB -> micro-dollars keeps every intermediate
# well inside a signed 64-bit integer, which bytes * rate would not.
estimate_cost_micro() {
  local bytes="$1"
  local mib=$(( bytes / BYTES_PER_MIB ))
  printf '%s' "$(( mib * RATE_MICRO_USD_PER_TIB / MIB_PER_TIB ))"
}

dry_run() {
  section "Stage 4. Dry run. Nothing is billed and nothing is read"

  local out bytes
  out="$(bq --project_id="$BILLING_PROJECT" --quiet --headless \
        query --use_legacy_sql=false --dry_run --format=json \
        "${BQ_PARAMS[@]}" "$PROBE_SQL" 2>&1)" || die "Dry run failed: $out"

  bytes="$(printf '%s' "$out" | tr -d ' "' | sed -n 's/.*totalBytesProcessed:\([0-9]*\).*/\1/p')"
  if [ -z "$bytes" ]; then
    bytes="$(printf '%s' "$out" | grep -oE '[0-9]+ bytes' | head -1 | grep -oE '[0-9]+' || true)"
  fi
  [ -n "$bytes" ] || die "Could not read a byte estimate from the dry run. Raw output: $out"

  local micro dollars frac
  micro="$(estimate_cost_micro "$bytes")"
  dollars=$(( micro / 1000000 ))
  frac=$(( micro % 1000000 ))

  printf '       estimated bytes processed : %s\n' "$bytes"
  printf '       assumed rate              : %s micro-USD per TiB\n' "$RATE_MICRO_USD_PER_TIB"
  printf '       estimated cost            : $%d.%06d  (truncated, and the rate is an ASSUMPTION)\n' \
    "$dollars" "$frac"

  if [ "$RUN" != "1" ]; then
    section "Stopped before the real query"
    note 'This is the designed end of the script. Re-run with RUN=1 to execute.'
    exit 0
  fi
}

# ---------------------------------------------------------------------------
# Stage 5. RUN=1 only. Report four things and nothing else, and store the raw
# bytes unchanged alongside the query text, the ledger sequence and the
# wall-clock reading time.
# ---------------------------------------------------------------------------

execute_and_record() {
  section "Stage 5. Executing, because RUN=1"

  local read_at out_file rows
  read_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  mkdir -p "$OUT_DIR"
  out_file="${OUT_DIR}/hubble-probe-${TARGET_LEDGER}-$(date -u '+%Y%m%dT%H%M%SZ').json"

  rows="$(bq --project_id="$BILLING_PROJECT" --quiet --headless \
         query --use_legacy_sql=false --format=prettyjson \
         "${BQ_PARAMS[@]}" "$PROBE_SQL")" || die "Query failed after billing. Check the job in BigQuery."

  {
    printf 'PROBE            hubble contract data entry existence\n'
    printf 'READ_AT_UTC      %s\n' "$read_at"
    printf 'TARGET_LEDGER    %s\n' "$TARGET_LEDGER"
    printf 'MODE             %s\n' "$MODE"
    printf 'CONTRACT_ID      %s\n' "$POOL_CONTRACT_ID"
    printf 'STORAGE_KEY      %s\n' "${STORAGE_KEY:-<whole contract>}"
    printf 'SOURCE_TABLE     %s.%s.%s\n' "$HUBBLE_PROJECT" "$HUBBLE_DATASET" "$HUBBLE_TABLE"
    printf '\n--- QUERY AS SENT ---\n%s\n' "$PROBE_SQL"
    printf '\n--- RAW RESULT, STORED UNCHANGED, NOTHING DECODED ---\n%s\n' "$rows"
  } > "$out_file"

  if [ -z "$rows" ] || [ "$rows" = "[]" ]; then
    printf '       a. present at TARGET_LEDGER : NO\n'
    printf '       b. contract_durability      : n/a, no row\n'
    printf '       c. live_until_ledger_seq    : n/a, no row\n'
    printf '       d. raw bytes                : %s\n' "$out_file"
    note 'A "NO" under MODE=at means only that no row was WRITTEN at that ledger.'
    exit 0
  fi

  printf '       a. present at TARGET_LEDGER : YES\n'
  printf '       b. contract_durability      : %s\n' \
    "$(printf '%s' "$rows" | sed -n 's/.*"contract_durability": *"\([^"]*\)".*/\1/p' | head -1)"
  printf '       c. live_until_ledger_seq    : %s  (%s)\n' \
    "$(printf '%s' "$rows" | sed -n 's/.*"live_until_ledger_seq": *"\{0,1\}\([0-9]*\).*/\1/p' | head -1)" \
    "$(printf '%s' "$rows" | sed -n 's/.*"liveness_at_target": *"\([^"]*\)".*/\1/p' | head -1)"
  printf '       d. raw bytes                : %s\n' "$out_file"
  note 'Nothing in that file was decoded. c_factor and l_factor are not computed here.'
}

# ---------------------------------------------------------------------------

main() {
  if [ "$NO_DISCOVERY" = "1" ]; then
    section "NO_DISCOVERY=1. Printing the discovery SQL and stopping"
    printf '%s\n\n' "$(sql_list_tables)"
    printf '%s\n' "$(sql_list_columns '<table-name-from-the-query-above>')"
    note 'Both read INFORMATION_SCHEMA and bill zero bytes. Nothing was contacted.'
    exit 0
  fi

  preflight
  discover_tables
  discover_columns
  build_probe
  dry_run
  execute_and_record
}

main "$@"
