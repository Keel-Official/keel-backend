#!/usr/bin/env bash
#
# pull-holder-and-supply.sh
#
# Captures, as raw evidence, the two data sets that sections 2 and 3 of
# docs/methodology/07-supporting-metrics.md need and currently lack:
#
#   1. The full trustline holder set. Pages Horizon /accounts?asset=<code>:<issuer>
#      to the end. Every page's raw body is stored unchanged, and one combined CSV
#      carries one row per holder.
#   2. The /assets?asset_code=&asset_issuer= body in full. It carries the supply
#      figures and the account counts section 3 needs. Stored raw.
#
# WHAT THIS SCRIPT DOES NOT DO, AND THAT IS THE POINT.
#
# It computes nothing. No total, no share, no top-N, no HHI, no concentration
# measure of any kind. Those are section 2's definitions and section 2 has not
# written them yet. A number computed at record time is a number nobody agreed on,
# and it would sit in the evidence directory looking as authoritative as the bytes
# Horizon actually sent. The only counts printed are of the reading itself: how many
# pages were fetched and how many rows came back, which requirement 5 asks for.
#
# Every amount stays the exact string Horizon sent. Nothing is parsed into a
# number anywhere in this file. Stroops are not reconstructed and decimals are not
# normalised, because "10432382.3578289" is the evidence and any float that
# round-trips through it is not.
#
# READ ONLY. Keel is permanently read-only. This issues GET requests and writes
# only inside its own output directory.
#
# THE READING TIME IS PART OF THE EVIDENCE. Horizon /accounts is current state
# only. There is no cursor, no ledger parameter, and no archive that will give
# this same answer back tomorrow. A holder set with no timestamp cannot be
# checked by anybody, including the person who pulled it. So every response
# records its Latest-Ledger header, the wall clock time the request went out, the
# wall clock time the body came back, and Horizon's own Date header.
#
# Requires: curl, jq, and either shasum or sha256sum.
#
# Usage:
#   scripts/pull-holder-and-supply.sh <ASSET_CODE> <ASSET_ISSUER>
#
# Example (USTRY):
#   scripts/pull-holder-and-supply.sh USTRY \
#     GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC
#
# BOTH ARGUMENTS ARE REQUIRED AND THAT IS DELIBERATE. An asset is the pair
# (code, issuer) and is never matched on the ticker; see CLAUDE.md and the note at
# the top of configs/recorder-pairs.json. 97 distinct assets share the AQUA
# ticker. A script that accepted a bare code would be inviting the one mistake
# this repository has already written down twice.
#
# Environment:
#   HORIZON_URL     default https://horizon.stellar.org
#   PAGE_LIMIT      records per page, default 200 (Horizon's maximum)
#   MAX_PAGES       paging budget, default 60 pages = 12000 holders
#   OUT_ROOT        default docs/evidences
#   REQUEST_DELAY   seconds between requests, default 0.25
#   COMPRESS        1 (default) gzips each page body, 0 leaves it plain
#
# ON COMPRESSION. A page of 200 accounts is about 15 MB, because /accounts
# returns every balance of every account and not only the one asked for. The full
# USTRY set is roughly 65 MB, seventeen times the largest file currently tracked
# under docs/. gzip is a container and not an edit: the bytes decompress
# identically, and the sha256 recorded in requests.tsv is taken over the
# UNCOMPRESSED body so that identity is checkable either way. This follows the
# precedent already set by `keel record`, which writes one gzipped file per pair
# per day. Set COMPRESS=0 to keep the plain bodies.

set -euo pipefail

# --- arguments -------------------------------------------------------------

if [[ $# -lt 2 ]]; then
  cat >&2 <<'USAGE'
usage: pull-holder-and-supply.sh <ASSET_CODE> <ASSET_ISSUER>

Both are required. An asset is the pair (code, issuer), never the ticker alone.

example:
  scripts/pull-holder-and-supply.sh USTRY \
    GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC
USAGE
  exit 2
fi

ASSET_CODE="$1"
ASSET_ISSUER="$2"

if [[ ! "$ASSET_ISSUER" =~ ^G[A-Z2-7]{55}$ ]]; then
  echo "issuer does not look like a Stellar account id: $ASSET_ISSUER" >&2
  exit 2
fi

HORIZON="${HORIZON_URL:-https://horizon.stellar.org}"
PAGE_LIMIT="${PAGE_LIMIT:-200}"
MAX_PAGES="${MAX_PAGES:-60}"
OUT_ROOT="${OUT_ROOT:-docs/evidences}"
REQUEST_DELAY="${REQUEST_DELAY:-0.25}"
COMPRESS="${COMPRESS:-1}"

command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }
command -v jq   >/dev/null 2>&1 || { echo "jq is required"   >&2; exit 1; }

if command -v sha256sum >/dev/null 2>&1; then
  sha256_of() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256_of() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  echo "need sha256sum or shasum" >&2
  exit 1
fi

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# --- output layout ---------------------------------------------------------

RUN_DATE="$(date -u +%Y-%m-%d)"
ISSUER_PREFIX="${ASSET_ISSUER:0:8}"
OUT_DIR="${OUT_ROOT}/${RUN_DATE}-${ASSET_CODE}.${ISSUER_PREFIX}-holders-and-supply"
PAGE_DIR="${OUT_DIR}/pages"
REQ_LOG="${OUT_DIR}/requests.tsv"
CSV="${OUT_DIR}/holders.csv"
MANIFEST="${OUT_DIR}/manifest.md"

# --- manifest --------------------------------------------------------------
#
# The manifest is a VIEW over the stored bytes, not a second source. Every
# /assets figure below is pulled out of assets.json with jq at write time rather
# than from a shell variable, so the manifest cannot drift away from the evidence
# it indexes. That is also what makes MANIFEST_ONLY=1 safe: the manifest can be
# rebuilt from the stored files without re-reading Horizon, which matters because
# a holder set cannot be re-fetched for a past ledger.

write_manifest() {

a() { jq -r "._embedded.records[0].$1 | tostring" "$ASSETS_BODY"; }

{
  echo "# ${ASSET_CODE} holder set and supply, raw reading"
  echo
  echo "Pulled by \`scripts/pull-holder-and-supply.sh\` on ${RUN_DATE} (UTC)."
  echo
  echo "This directory is a reading, not a result. Nothing here is computed."
  echo "Sections 2 and 3 of \`docs/methodology/07-supporting-metrics.md\` have no"
  echo "definitions yet, so no total, share, top-N or HHI appears in any file"
  echo "below. When those definitions are written, they get run over these bytes."
  echo
  echo "## Asset"
  echo
  echo "| Field | Value |"
  echo "| --- | --- |"
  echo "| Code | \`${ASSET_CODE}\` |"
  echo "| Issuer | \`${ASSET_ISSUER}\` |"
  echo "| Type (from Horizon) | \`$(a asset_type)\` |"
  echo "| SAC contract id | \`$(a contract_id)\` |"
  echo "| SEP-1 toml | $(jq -r '._embedded.records[0]._links.toml.href // "none"' "$ASSETS_BODY") |"
  echo "| Horizon | ${HORIZON} |"
  echo
  echo "## The reading"
  echo
  echo "Horizon \`/accounts?asset=\` is current state only. There is no ledger"
  echo "parameter and no archive that returns this same answer later, so the"
  echo "reading time below is part of the evidence and not decoration."
  echo
  echo "| | Latest-Ledger | Read at (UTC) |"
  echo "| --- | --- | --- |"
  echo "| \`/assets\` | ${ASSETS_LEDGER} | ${ASSETS_READ_AT} |"
  echo "| first holder page | ${FIRST_PAGE_LEDGER:-none} | ${FIRST_PAGE_READ_AT:-none} |"
  echo "| last holder page | ${LAST_PAGE_LEDGER:-none} | ${LAST_PAGE_READ_AT:-none} |"
  echo
  echo "The ledger moved between the first and the last page if those two numbers"
  echo "differ. They usually will. Each CSV row carries the ledger of the page it"
  echo "came from, so the set is not pretending to be a single instant."
  echo
  echo "## Paging"
  echo
  echo "| | |"
  echo "| --- | --- |"
  echo "| Pages fetched | ${page} |"
  echo "| Page size requested | ${PAGE_LIMIT} |"
  echo "| Page budget | ${MAX_PAGES} |"
  echo "| Holder rows written | ${rows} |"
  echo "| Distinct account ids | ${DISTINCT} |"
  echo "| Paging completed | ${complete} |"
  echo "| Stop reason | ${stop_reason} |"
  echo
  echo "\`/assets\` reported \`accounts.authorized = $(a accounts.authorized)\` at ledger"
  echo "${ASSETS_LEDGER}. That figure and the row count above were produced by two"
  echo "different endpoints and are recorded side by side without being reconciled."
  echo
  echo "## Files"
  echo
  echo "| File | What it is |"
  echo "| --- | --- |"
  echo "| \`assets.json\` | the \`/assets\` body, unchanged. Carries the supply figures and account counts section 3 needs |"
  echo "| \`assets.headers.txt\` | its response headers, including Latest-Ledger |"
  echo "| \`pages/accounts-NNNN.json[.gz]\` | each holder page body, unchanged |"
  echo "| \`pages/accounts-NNNN.headers.txt\` | each page's response headers |"
  echo "| \`holders.csv\` | one row per holder, amounts as the strings Horizon sent |"
  echo "| \`requests.tsv\` | every request: time out, time back, status, Latest-Ledger, Horizon Date, bytes, sha256 |"
  echo
  if [[ -f "${PAGE_DIR}/accounts-0001.json.gz" ]]; then
    echo "Page bodies are gzipped. A page of ${PAGE_LIMIT} accounts runs to 12-21 MB,"
    echo "because \`/accounts\` returns every balance of every account and not only"
    echo "the one asked for. gzip is a container and not an edit: the sha256 in"
    echo "\`requests.tsv\` is taken over the UNCOMPRESSED body, so"
    echo "\`gzip -cd pages/accounts-0001.json.gz | shasum -a 256\` checks it."
    echo
  fi
  echo "## Where the supply figures are, and why they do not add up to one number"
  echo
  echo "Every value below is a field Horizon sent in \`assets.json\`, at ledger"
  echo "${ASSETS_LEDGER}. None is derived, and they are deliberately NOT summed."
  echo
  echo "| Field | Value |"
  echo "| --- | --- |"
  echo "| \`accounts.authorized\` | $(a accounts.authorized) |"
  echo "| \`accounts.authorized_to_maintain_liabilities\` | $(a accounts.authorized_to_maintain_liabilities) |"
  echo "| \`accounts.unauthorized\` | $(a accounts.unauthorized) |"
  echo "| \`balances.authorized\` | $(a balances.authorized) |"
  echo "| \`balances.authorized_to_maintain_liabilities\` | $(a balances.authorized_to_maintain_liabilities) |"
  echo "| \`balances.unauthorized\` | $(a balances.unauthorized) |"
  echo "| \`liquidity_pools_amount\` | $(a liquidity_pools_amount) |"
  echo "| \`num_liquidity_pools\` | $(a num_liquidity_pools) |"
  echo "| \`contracts_amount\` | $(a contracts_amount) |"
  echo "| \`num_contracts\` | $(a num_contracts) |"
  echo "| \`claimable_balances_amount\` | $(a claimable_balances_amount) |"
  echo "| \`num_claimable_balances\` | $(a num_claimable_balances) |"
  echo "| \`num_accounts\` (deprecated) | $(a num_accounts) |"
  echo "| \`amount\` (deprecated) | $(a amount) |"
  echo
  echo "**This is the whole of section 3 question 1, sitting in one table.** The"
  echo "asset exists in four places at once and Horizon counts them separately:"
  echo "trustlines (\`balances.authorized\`), liquidity pools"
  echo "(\`liquidity_pools_amount\`), Soroban contracts (\`contracts_amount\`, across"
  echo "$(a num_contracts) contracts including the SAC above), and claimable balances"
  echo "(\`claimable_balances_amount\`). \`holders.csv\` covers the FIRST of those four"
  echo "and nothing else, because \`/accounts?asset=\` returns accounts and a pool,"
  echo "a contract and a claimable balance are none of them."
  echo
  echo "So \"issued total\", \"total held in trustlines\" and \"circulating\" are three"
  echo "different numbers here, and the gap between them is large rather than"
  echo "rounding. Section 3 has to pick one and say so, and section 2 question 2"
  echo "has to make the same choice for the concentration denominator. Neither"
  echo "choice is made in this directory. \`num_accounts\` and \`amount\` are the"
  echo "deprecated fields and Horizon now returns null for both, which is why"
  echo "neither can be used as the answer."
  echo
  echo "The issuer flags are \`auth_required=$(a flags.auth_required)\`,"
  echo "\`auth_revocable=$(a flags.auth_revocable)\`,"
  echo "\`auth_immutable=$(a flags.auth_immutable)\`,"
  echo "\`auth_clawback_enabled=$(a flags.auth_clawback_enabled)\`. With"
  echo "\`auth_required\` false, every trustline is authorized on creation, which is"
  echo "why \`is_authorized\` is \`true\` on every row of \`holders.csv\` and carries no"
  echo "information for this asset. It is recorded anyway because an asset with the"
  echo "flag set would need it."
  echo
  echo "## Known limits of this reading"
  echo
  echo "1. Not an instant. The pages were read one after another and the ledger"
  echo "   advanced underneath them. A holder who moved between page 1 and page ${page}"
  echo "   is recorded at whichever page saw it."
  echo "2. Not re-fetchable. Nothing here can be reproduced for a past ledger."
  echo "   Re-running this script produces a different reading, not the same one."
  echo "3. Trustlines only, which is one of the four places named above. The issuer"
  echo "   holds no trustline to its own asset and does not appear. Pools, contracts"
  echo "   and claimable balances do not appear either. Those are properties of the"
  echo "   endpoint, not decisions this script made."
  echo "4. Custodial and exchange accounts are not marked, because they cannot be"
  echo "   detected reliably. Section 2 already says so."
  echo "5. A zero balance is still a trustline and is recorded as a row. Whether a"
  echo "   zero-balance trustline is a *holder* is section 2 question 1 and is not"
  echo "   answered here."
} >"$MANIFEST"

}

# --- rebuild the manifest from stored bytes, without touching Horizon --------
#
# MANIFEST_ONLY=1 RUN_DIR=<dir> regenerates manifest.md from the files already in
# <dir>. It exists because a holder set cannot be re-fetched for a past ledger:
# if the manifest is wrong or incomplete, re-running the pull would replace the
# evidence with a DIFFERENT reading rather than correct the description of this
# one. Every value it needs is recovered from requests.tsv, holders.csv and
# assets.json, so the rebuilt manifest describes the stored bytes and not the
# state of Horizon now.

if [[ "${MANIFEST_ONLY:-0}" == "1" ]]; then
  RUN_DIR="${RUN_DIR:?MANIFEST_ONLY=1 requires RUN_DIR=<existing output directory>}"
  OUT_DIR="$RUN_DIR"
  PAGE_DIR="${OUT_DIR}/pages"
  REQ_LOG="${OUT_DIR}/requests.tsv"
  CSV="${OUT_DIR}/holders.csv"
  MANIFEST="${OUT_DIR}/manifest.md"
  ASSETS_BODY="${OUT_DIR}/assets.json"

  for f in "$REQ_LOG" "$CSV" "$ASSETS_BODY"; do
    [[ -f "$f" ]] || { echo "missing $f" >&2; exit 1; }
  done

  RUN_DATE="$(basename "$OUT_DIR" | cut -d- -f1-3)"

  ASSETS_LEDGER="$(awk -F'\t' '$2=="assets"{print $6; exit}' "$REQ_LOG")"
  ASSETS_READ_AT="$(awk -F'\t' '$2=="assets"{print $4; exit}' "$REQ_LOG")"
  FIRST_PAGE_LEDGER="$(awk -F'\t' '$2=="accounts"{print $6; exit}' "$REQ_LOG")"
  FIRST_PAGE_READ_AT="$(awk -F'\t' '$2=="accounts"{print $4; exit}' "$REQ_LOG")"
  LAST_PAGE_LEDGER="$(awk -F'\t' '$2=="accounts"{v=$6} END{print v}' "$REQ_LOG")"
  LAST_PAGE_READ_AT="$(awk -F'\t' '$2=="accounts"{v=$4} END{print v}' "$REQ_LOG")"

  page="$(awk -F'\t' '$2=="accounts"' "$REQ_LOG" | wc -l | tr -d ' ')"
  rows="$(tail -n +2 "$CSV" | wc -l | tr -d ' ')"
  DISTINCT="$(tail -n +2 "$CSV" | cut -d, -f1 | sort -u | wc -l | tr -d ' ')"

  last_records="$(awk -F'\t' '$2=="accounts"{v=$9} END{print v}' "$REQ_LOG")"
  PAGE_LIMIT="$(awk -F'\t' '$2=="accounts"{print $9; exit}' "$REQ_LOG")"
  if [[ "$last_records" -lt "$PAGE_LIMIT" ]]; then
    complete="YES"
    stop_reason="short page (${last_records} < ${PAGE_LIMIT}); the trustline set ended here"
  else
    complete="NO"
    stop_reason="rebuilt from requests.tsv: the last page was full, so paging did not demonstrably finish"
  fi

  write_manifest
  echo "rebuilt ${MANIFEST} from stored bytes. Horizon was not contacted."
  exit 0
fi

mkdir -p "$PAGE_DIR"


printf 'seq\tkind\trequested_at_utc\treceived_at_utc\thttp_status\tlatest_ledger\thorizon_date\tbody_bytes\trecords\tsha256_uncompressed\tstored_file\turl\n' >"$REQ_LOG"

# One row per holder. The first six columns are what section 2 asked for. The
# last three are provenance: which page the row came from, the ledger that page
# was current at, and when it was read. A row that cannot say when it was true is
# not evidence.
printf 'account_id,balance,buying_liabilities,selling_liabilities,is_authorized,last_modified_ledger,source_page,latest_ledger,read_at_utc\n' >"$CSV"

# --- one GET ---------------------------------------------------------------
#
# Sets: R_STATUS R_LEDGER R_DATE R_BYTES R_SHA R_STORED R_REQUESTED R_RECEIVED
# Leaves the body at $2 uncompressed; the caller decides whether to gzip it.

do_get() {
  local url="$1" body_path="$2" head_path="$3"

  R_REQUESTED="$(now_utc)"
  # -X GET is redundant and stated anyway: this script never writes to Horizon.
  R_STATUS="$(curl -sS -X GET \
    --retry 3 --retry-delay 2 --retry-connrefused \
    --max-time 180 \
    -D "$head_path" -o "$body_path" \
    -w '%{http_code}' \
    "$url")"
  R_RECEIVED="$(now_utc)"

  # Horizon folds header case inconsistently across proxies, and a redirect or a
  # retry can leave more than one header block in the file, so the LAST match
  # wins. tolower() is used rather than awk's IGNORECASE, which is a gawk
  # extension: on macOS awk it is silently ignored and every ledger reads
  # MISSING. That failure is quiet and it defeats requirement 2 entirely, which
  # is why it is spelled out here.
  R_LEDGER="$(awk 'tolower($1) == "latest-ledger:" { v = $2 } END { gsub(/\r/, "", v); print v }' "$head_path")"
  R_DATE="$(awk 'tolower($1) == "date:" { sub(/^[^:]*:[ \t]*/, ""); v = $0 } END { gsub(/\r/, "", v); print v }' "$head_path")"
  [[ -n "${R_LEDGER:-}" ]] || R_LEDGER="MISSING"
  [[ -n "${R_DATE:-}"   ]] || R_DATE="MISSING"

  R_BYTES="$(wc -c <"$body_path" | tr -d ' ')"
  R_SHA="$(sha256_of "$body_path")"
}

log_request() {
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$1" "$2" "$R_REQUESTED" "$R_RECEIVED" "$R_STATUS" "$R_LEDGER" \
    "$R_DATE" "$R_BYTES" "$3" "$R_SHA" "$4" "$5" >>"$REQ_LOG"
}

pause() {
  # Only sleeps when asked to. A five page pull does not need throttling; a
  # thousand page one does, and the same script has to serve both.
  if [[ "$REQUEST_DELAY" != "0" ]]; then sleep "$REQUEST_DELAY"; fi
}

echo "Horizon:     $HORIZON"
echo "Asset:       ${ASSET_CODE}:${ASSET_ISSUER}"
echo "Output:      $OUT_DIR"
echo "Page limit:  $PAGE_LIMIT   Page budget: $MAX_PAGES"
echo

# --- 1. /assets ------------------------------------------------------------
#
# Fetched FIRST and on purpose. It reports the authorized account count, which
# is the only independent statement of how large the trustline set is. Pulling it
# before paging means the page count below can be read against a number that was
# not produced by the paging loop itself.

ASSETS_BODY="${OUT_DIR}/assets.json"
ASSETS_HEAD="${OUT_DIR}/assets.headers.txt"
ASSETS_URL="${HORIZON}/assets?asset_code=${ASSET_CODE}&asset_issuer=${ASSET_ISSUER}"

echo "GET /assets ..."
do_get "$ASSETS_URL" "$ASSETS_BODY" "$ASSETS_HEAD"

if [[ "$R_STATUS" != "200" ]]; then
  echo "  /assets returned HTTP $R_STATUS. Body kept at $ASSETS_BODY" >&2
  log_request 0 assets "" "assets.json" "$ASSETS_URL"
  exit 1
fi

ASSETS_RECORDS="$(jq '._embedded.records | length' "$ASSETS_BODY")"
ASSETS_LEDGER="$R_LEDGER"
ASSETS_READ_AT="$R_RECEIVED"
log_request 0 assets "$ASSETS_RECORDS" "assets.json" "$ASSETS_URL"

echo "  records=$ASSETS_RECORDS  Latest-Ledger=$ASSETS_LEDGER  read_at=$ASSETS_READ_AT"

if [[ "$ASSETS_RECORDS" == "0" ]]; then
  echo "  /assets knows no such (code, issuer) pair. Check the issuer." >&2
  exit 1
fi

# Reported, not computed. These are fields Horizon sent, quoted back verbatim so
# the run is legible from the terminal without opening the JSON.
ASSET_TYPE="$(jq -r '._embedded.records[0].asset_type' "$ASSETS_BODY")"
ACC_AUTH="$(jq -r '._embedded.records[0].accounts.authorized'  "$ASSETS_BODY")"
ACC_AUTH_MAINT="$(jq -r '._embedded.records[0].accounts.authorized_to_maintain_liabilities' "$ASSETS_BODY")"
ACC_UNAUTH="$(jq -r '._embedded.records[0].accounts.unauthorized' "$ASSETS_BODY")"
NUM_POOLS="$(jq -r '._embedded.records[0].num_liquidity_pools' "$ASSETS_BODY")"
NUM_CB="$(jq -r '._embedded.records[0].num_claimable_balances' "$ASSETS_BODY")"

echo "  asset_type=$ASSET_TYPE  accounts.authorized=$ACC_AUTH  liquidity_pools=$NUM_POOLS"
echo

pause

# --- 2. /accounts?asset= ---------------------------------------------------

url="${HORIZON}/accounts?asset=${ASSET_CODE}%3A${ASSET_ISSUER}&limit=${PAGE_LIMIT}&order=asc"
page=0
rows=0
complete="NO"
http_error="NO"
stop_reason="budget exhausted at ${MAX_PAGES} pages"
FIRST_PAGE_LEDGER=""
LAST_PAGE_LEDGER=""
FIRST_PAGE_READ_AT=""
LAST_PAGE_READ_AT=""

echo "GET /accounts?asset= ..."

while [[ -n "$url" && "$page" -lt "$MAX_PAGES" ]]; do
  page=$((page + 1))
  page_name="$(printf 'accounts-%04d.json' "$page")"
  body="${PAGE_DIR}/${page_name}"
  head="${PAGE_DIR}/$(printf 'accounts-%04d.headers.txt' "$page")"

  do_get "$url" "$body" "$head"

  if [[ "$R_STATUS" != "200" ]]; then
    log_request "$page" accounts "" "pages/${page_name}" "$url"
    http_error="YES"
    stop_reason="HTTP ${R_STATUS} on page ${page}; body kept at ${body}"
    echo "  page $page returned HTTP $R_STATUS. Stopping." >&2
    break
  fi

  count="$(jq '._embedded.records | length' "$body")"

  [[ -z "$FIRST_PAGE_LEDGER" ]] && { FIRST_PAGE_LEDGER="$R_LEDGER"; FIRST_PAGE_READ_AT="$R_RECEIVED"; }
  LAST_PAGE_LEDGER="$R_LEDGER"
  LAST_PAGE_READ_AT="$R_RECEIVED"

  # One row per holder. Amounts are copied across as the strings Horizon sent;
  # no tonumber appears anywhere in this program. A balance entry carrying a
  # liquidity_pool_id has no asset_code and is filtered out by the same select,
  # which is correct: a pool share is not a holding of this asset.
  added=0
  if [[ "$count" != "0" ]]; then
    jq -r \
      --arg code "$ASSET_CODE" \
      --arg iss  "$ASSET_ISSUER" \
      --arg page "$page_name" \
      --arg led  "$R_LEDGER" \
      --arg at   "$R_RECEIVED" '
      ._embedded.records[] as $a
      | $a.balances[]
      | select(.asset_code == $code and .asset_issuer == $iss)
      | [ $a.id,
          .balance,
          .buying_liabilities,
          .selling_liabilities,
          (.is_authorized | tostring),
          ($a.last_modified_ledger | tostring),
          $page, $led, $at ]
      | @csv' "$body" >>"$CSV"
    added="$(jq -r --arg code "$ASSET_CODE" --arg iss "$ASSET_ISSUER" '
      [ ._embedded.records[].balances[]
        | select(.asset_code == $code and .asset_issuer == $iss) ] | length' "$body")"
  fi
  rows=$((rows + added))

  stored="pages/${page_name}"
  if [[ "$COMPRESS" == "1" ]]; then
    gzip -n -9 "$body"
    stored="pages/${page_name}.gz"
  fi

  log_request "$page" accounts "$count" "$stored" "$url"

  printf '  page %-3d records=%-4s holders=%-4s bytes=%-9s Latest-Ledger=%s  %s\n' \
    "$page" "$count" "$added" "$R_BYTES" "$R_LEDGER" "$R_RECEIVED"

  # Horizon returns a next link even on the final page, so a short page is the
  # real terminator. An exactly-full final page costs one extra request that
  # comes back empty, and that empty page is kept: it is the proof of the end.
  if [[ "$count" -lt "$PAGE_LIMIT" ]]; then
    complete="YES"
    stop_reason="short page (${count} < ${PAGE_LIMIT}); the trustline set ended here"
    url=""
  else
    if [[ "$COMPRESS" == "1" ]]; then
      url="$(gzip -cd "${body}.gz" | jq -r '._links.next.href // ""')"
    else
      url="$(jq -r '._links.next.href // ""' "$body")"
    fi
    if [[ -z "$url" ]]; then
      complete="YES"
      stop_reason="Horizon offered no next link"
    fi
    pause
  fi
done

if [[ "$complete" != "YES" && "$http_error" == "NO" && "$page" -ge "$MAX_PAGES" ]]; then
  stop_reason="PAGING BUDGET HIT: stopped after ${MAX_PAGES} pages with a next link still outstanding"
fi

# Distinct account ids, reported beside the row count rather than folded into
# it. Cursor paging on /accounts is by account id and should never repeat one;
# if these two numbers ever differ, that difference is itself the finding.
DISTINCT="$(tail -n +2 "$CSV" | cut -d, -f1 | sort -u | wc -l | tr -d ' ')"


write_manifest

# --- report ----------------------------------------------------------------

echo
echo "=== Result ==="
echo "pages fetched:         $page"
echo "holder rows written:   $rows"
echo "distinct account ids:  $DISTINCT"
echo "paging completed:      $complete"
echo "stop reason:           $stop_reason"
echo "/assets accounts.authorized: $ACC_AUTH  (at ledger $ASSETS_LEDGER)"
echo
echo "output: $OUT_DIR"

if [[ "$complete" != "YES" ]]; then
  echo
  echo "PAGING DID NOT COMPLETE. The holder set in holders.csv is PARTIAL." >&2
  echo "This is the measurement 07 section 2 question 4 asks for: the trustline" >&2
  echo "set is larger than a ${MAX_PAGES} page budget. Raise MAX_PAGES and re-run," >&2
  echo "or record this limit as the answer." >&2
  exit 3
fi
