#!/usr/bin/env bash
#
# pull-authorized-counts.sh
#
# Answers ONE question with one request per asset: how large is each asset's
# trustline set, as Horizon itself reports it, and how many of the assets Keel
# already measures are too large to page within a fixed budget.
#
# That question is section 2 question 4 of docs/methodology/07-supporting-metrics.md:
# "What is reported when the trustline set is too large to page within budget?"
# The question is currently hypothetical. Section 6.4 of the technical design
# assumes the case is rare and does not say how rare. This script replaces the
# assumption with a count.
#
# ONE REQUEST PER ASSET, AND NO HOLDER PAGING AT ALL.
#
# `/assets?asset_code=&asset_issuer=` states `accounts.authorized` directly. That
# is a statement about the size of the trustline set obtained WITHOUT walking it,
# which is the whole point: paging 64 holder sets to find out which ones are too
# big to page is the cost this script exists to avoid. Nothing here touches
# `/accounts`. scripts/pull-holder-and-supply.sh is the script that pages holders,
# for one asset at a time, and this is deliberately not that script.
#
# WHAT IT COMPUTES, WHICH IS ALMOST NOTHING.
#
# Every field lands in the CSV as the exact string Horizon sent. No total, no
# share, no top-N, no HHI, no supply definition, no reconciliation between the
# four places an asset can sit. Those are sections 2 and 3 and both are unwritten;
# a number computed here would sit in the evidence directory looking like a
# decision nobody made.
#
# The manifest carries exactly TWO derived figures, and both were asked for by
# name: the distribution of `accounts.authorized` across the set, and the count of
# assets above PAGE_BUDGET x PAGE_LIMIT trustlines. Both are counts OF the
# readings rather than statistics DRAWN FROM them, both are recomputed from the
# stored CSV rather than from a shell variable, and the threshold appears in the
# output as its two factors so a reader can see it move.
#
# THE NATIVE ASSET CANNOT BE READ HERE, AND IT IS RECORDED RATHER THAN DROPPED.
#
# One of the 64 is XLM, whose base has type `native` and no code and no issuer.
# `/assets` is addressed by (code, issuer) and has no address for it: an empty
# code and issuer are read as NO FILTER and return the whole asset collection,
# and `asset_code=XLM` returns ten issued assets whose ticker is XLM and not the
# native asset, which is the ticker trap configs/ is annotated against. So the
# native asset gets a row with status `not_addressable` and no request is sent.
# Its trustline count is not zero and not unknown-for-now: XLM has no trustlines
# because holding it needs no trustline, and section 2 question 4 does not apply
# to it. Silently making the set 63 would hide that.
#
# READ ONLY. Keel is permanently read-only. This issues GET requests and writes
# only inside its own output directory.

set -euo pipefail

# --- arguments -------------------------------------------------------------

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  cat >&2 <<'USAGE'
usage: pull-authorized-counts.sh [PAIR_FILE ...]

Reads the BASE asset of every pair in each PAIR_FILE, takes the union by
(type, code, issuer), and fetches /assets once per asset.

With no arguments it reads the two files whose union is the 64 assets already in
the metrics table, per docs/evidences/2026-08-26-scan-64-assets-stored.md:

  configs/demonstration-set.json   60 pairs
  configs/recorder-pairs.json       8 pairs, 4 of them new

environment:
  HORIZON_URL     default https://horizon.stellar.org
  OUT_ROOT        default docs/evidences
  REQUEST_DELAY   seconds between requests, default 0.25
  PAGE_LIMIT      holder page size the budget is expressed in, default 200
  PAGE_BUDGET     pages allowed per asset, default 60
  MANIFEST_ONLY   1 with RUN_DIR=<dir> rebuilds the manifest from stored bytes
USAGE
  exit 2
fi

HORIZON="${HORIZON_URL:-https://horizon.stellar.org}"
OUT_ROOT="${OUT_ROOT:-docs/evidences}"
REQUEST_DELAY="${REQUEST_DELAY:-0.25}"
PAGE_LIMIT="${PAGE_LIMIT:-200}"
PAGE_BUDGET="${PAGE_BUDGET:-60}"

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
OUT_DIR="${OUT_ROOT}/${RUN_DATE}-authorized-counts"
BODY_DIR="${OUT_DIR}/bodies"
REQ_LOG="${OUT_DIR}/requests.tsv"
CSV="${OUT_DIR}/authorized-and-supply.csv"
MANIFEST="${OUT_DIR}/manifest.md"

# --- the two derived figures, both recomputed from the stored CSV -----------
#
# THRESHOLD is shown as its two factors everywhere it appears. A budget written
# as one number invites the reader to treat 12000 as a property of the network,
# and it is a property of a page size and a page count that section 6.4 chose.

THRESHOLD=$(( PAGE_BUDGET * PAGE_LIMIT ))

# distribution buckets. Fixed, ascending, and the top bucket is open. The
# boundaries are round numbers rather than quantiles of this particular set: a
# quantile boundary would move when the set is rebuilt under section 5 of
# 02-pair-selection.md, and the whole point of the count is to be comparable
# across that rebuild.
BUCKETS=(0 1 10 100 1000 10000 100000)

write_manifest() {

  # $1 field name, from the CSV header. Prints the column of values for
  # assets that were actually read.
  col() {
    awk -F, -v want="$1" '
      NR==1 { for (i = 1; i <= NF; i++) if ($i == want) c = i; next }
      $2 == "ok" { print $c }
    ' "$CSV"
  }

  local read_ok read_absent total
  total="$(tail -n +2 "$CSV" | wc -l | tr -d ' ')"
  read_ok="$(awk -F, 'NR>1 && $2=="ok"' "$CSV" | wc -l | tr -d ' ')"
  read_absent="$(awk -F, 'NR>1 && $2!="ok"' "$CSV" | wc -l | tr -d ' ')"

  local over
  over="$(col accounts_authorized | awk -v t="$THRESHOLD" '$1+0 > t {n++} END {print n+0}')"

{
  echo "# Authorized trustline counts across the ${total} assets in the metrics table"
  echo
  echo "Pulled by \`scripts/pull-authorized-counts.sh\` on ${RUN_DATE} (UTC), from"
  echo "\`${HORIZON}\`, no account required."
  echo
  echo "One request per asset, to \`/assets?asset_code=&asset_issuer=\`. No holder"
  echo "paging: \`/accounts\` is not touched by this script at all."
  echo
  echo "## What this directory is for"
  echo
  echo "Section 2 question 4 of \`docs/methodology/07-supporting-metrics.md\` asks"
  echo "what is reported when a trustline set is too large to page within budget."
  echo "Section 6.4 of the technical design assumes the case is rare and does not"
  echo "say how rare. The table below is that assumption measured."
  echo
  echo "## The set"
  echo
  echo "| | |"
  echo "| --- | --- |"
  echo "| Assets in the set | ${total} |"
  echo "| Read from \`/assets\` | ${read_ok} |"
  echo "| Not addressable by \`/assets\` | ${read_absent} |"
  echo "| Pair files read | ${PAIR_FILES_LABEL} |"
  echo
  if [[ "$read_absent" != "0" ]]; then
    echo "The assets that were not read, and why:"
    echo
    echo "| Asset | Type | Status |"
    echo "| --- | --- | --- |"
    # An empty code renders as an empty cell, which reads as a missing value
    # rather than as the fact that there is no code to print. Name it instead.
    awk -F, 'NR>1 && $2!="ok" {
      id = ($3 == "" ? "the native asset, no code and no issuer" : "`" $3 "` / `" substr($4,1,8) "`")
      print "| " id " | `" $5 "` | `" $2 "` |"
    }' "$CSV"
    echo
    echo "\`/assets\` is addressed by (code, issuer). The native asset has neither."
    echo "An empty code and issuer are read as NO FILTER and return the whole asset"
    echo "collection; \`asset_code=XLM\` returns issued assets whose ticker is XLM"
    echo "and not the native asset. Its trustline count is not zero and not"
    echo "pending: holding XLM needs no trustline, so section 2 question 4 does not"
    echo "apply to it. It keeps a row so that the set stays ${total} and the"
    echo "exclusion is visible rather than arithmetic."
    echo
  fi
  echo "## Distribution of \`accounts.authorized\`"
  echo
  echo "Counted over the ${read_ok} assets that were read. Buckets are half open,"
  echo "low inclusive."
  echo
  echo "| Authorized trustlines | Assets | Share of ${read_ok} |"
  echo "| --- | --- | --- |"
  local i lo hi n
  for (( i = 0; i < ${#BUCKETS[@]}; i++ )); do
    lo="${BUCKETS[$i]}"
    if (( i + 1 < ${#BUCKETS[@]} )); then
      hi="${BUCKETS[$((i+1))]}"
      n="$(col accounts_authorized | awk -v lo="$lo" -v hi="$hi" '$1+0 >= lo && $1+0 < hi {n++} END {print n+0}')"
      printf '| %s to %s | %s | %s |\n' "$lo" "$((hi-1))" "$n" \
        "$(awk -v n="$n" -v d="$read_ok" 'BEGIN{printf "%.1f%%", d ? 100*n/d : 0}')"
    else
      n="$(col accounts_authorized | awk -v lo="$lo" '$1+0 >= lo {n++} END {print n+0}')"
      printf '| %s and above | %s | %s |\n' "$lo" "$n" \
        "$(awk -v n="$n" -v d="$read_ok" 'BEGIN{printf "%.1f%%", d ? 100*n/d : 0}')"
    fi
  done
  echo
  echo "Smallest: $(col accounts_authorized | sort -n | head -1). "
  echo "Largest: $(col accounts_authorized | sort -n | tail -1)."
  echo
  echo "## The page budget, measured"
  echo
  echo "| | |"
  echo "| --- | --- |"
  echo "| Page size (\`PAGE_LIMIT\`) | ${PAGE_LIMIT} |"
  echo "| Pages allowed (\`PAGE_BUDGET\`) | ${PAGE_BUDGET} |"
  echo "| Trustlines reachable | ${PAGE_BUDGET} x ${PAGE_LIMIT} = ${THRESHOLD} |"
  echo "| **Assets above it** | **${over} of ${read_ok}** |"
  echo
  echo "The assets above the budget, largest first:"
  echo
  echo "| Asset | Issuer | \`accounts.authorized\` | Pages needed at ${PAGE_LIMIT} |"
  echo "| --- | --- | --- | --- |"
  awk -F, -v t="$THRESHOLD" -v lim="$PAGE_LIMIT" '
    NR==1 { for (i=1;i<=NF;i++) h[$i]=i; next }
    $2=="ok" && $(h["accounts_authorized"])+0 > t {
      printf "%d\t| `%s` | `%s` | %d | %d |\n", $(h["accounts_authorized"]),
        $3, substr($4,1,8), $(h["accounts_authorized"]),
        int(($(h["accounts_authorized"]) + lim - 1) / lim)
    }' "$CSV" | sort -rn | cut -f2-
  echo
  echo "Pages needed is a ceiling division of a count Horizon stated by a page"
  echo "size this script was given. It is arithmetic on the reading and not a"
  echo "measurement: the real walk can need one more page, because /accounts is"
  echo "current state and the set moves while it is being paged."
  echo
  echo "## Reading times"
  echo
  echo "\`/assets\` is current state only. There is no ledger parameter and no"
  echo "archive that returns the same answer later, so each row's reading time and"
  echo "Latest-Ledger are part of the evidence."
  echo
  echo "| | Latest-Ledger | Read at (UTC) |"
  echo "| --- | --- | --- |"
  echo "| first request | ${FIRST_LEDGER:-none} | ${FIRST_READ_AT:-none} |"
  echo "| last request | ${LAST_LEDGER:-none} | ${LAST_READ_AT:-none} |"
  echo
  echo "The ledger advanced between them, so this is ${read_ok} readings taken"
  echo "over that span and not a snapshot. Every CSV row carries its own"
  echo "\`latest_ledger\` and \`read_at_utc\`, which is rule 1 of the non-negotiables"
  echo "applied to a reading that has no LedgerSeq of its own."
  echo
  echo "## Files"
  echo
  echo "| File | What it is |"
  echo "| --- | --- |"
  echo "| \`bodies/<CODE>.<ISSUER8>.json\` | each \`/assets\` body, unchanged |"
  echo "| \`bodies/<CODE>.<ISSUER8>.headers.txt\` | its response headers, including Latest-Ledger |"
  echo "| \`authorized-and-supply.csv\` | one row per asset, every amount the exact string Horizon sent |"
  echo "| \`requests.tsv\` | every request: time out, time back, status, Latest-Ledger, Horizon Date, bytes, sha256 |"
  echo "| \`manifest.md\` | this file. A VIEW over the stored bytes, recomputed from them |"
  echo
  echo "The filename carries the issuer prefix because two codes in this set are"
  echo "not unique: EURC and GOLD each appear twice under different issuers. An"
  echo "asset is the pair (code, issuer) and is never matched on the ticker."
  echo
  echo "## What is NOT here"
  echo
  echo "No total supply, no circulating figure, no concentration measure, no"
  echo "top-N, no HHI, and no reconciliation of the four places an asset can sit"
  echo "(trustlines, pools, contracts, claimable balances). Every one of those is"
  echo "a definition sections 2 and 3 have not written. The CSV holds the fields"
  echo "those definitions will need, as strings, and stops there."
  echo
  echo "\`accounts.authorized\` counts TRUSTLINES, not holders. A zero balance is"
  echo "still a trustline and is counted here; whether it is a *holder* is section"
  echo "2 question 1. The issuer holds no trustline to its own asset and is not in"
  echo "this count either."
  echo
  echo "\`num_accounts\` and \`amount\` are the deprecated fields. Horizon now omits"
  echo "them entirely rather than returning null, so the CSV records \`ABSENT\` for"
  echo "both, which is a different fact from an empty value."
} >"$MANIFEST"

}

# --- rebuild the manifest from stored bytes --------------------------------
#
# Same reason as pull-holder-and-supply.sh: a reading of current state cannot be
# re-fetched. If the manifest is wrong, re-running the pull would replace the
# evidence with a DIFFERENT reading rather than correct the description of this
# one.

if [[ "${MANIFEST_ONLY:-0}" == "1" ]]; then
  RUN_DIR="${RUN_DIR:?MANIFEST_ONLY=1 requires RUN_DIR=<existing output directory>}"
  OUT_DIR="$RUN_DIR"
  BODY_DIR="${OUT_DIR}/bodies"
  REQ_LOG="${OUT_DIR}/requests.tsv"
  CSV="${OUT_DIR}/authorized-and-supply.csv"
  MANIFEST="${OUT_DIR}/manifest.md"

  for f in "$REQ_LOG" "$CSV"; do
    [[ -f "$f" ]] || { echo "missing $f" >&2; exit 1; }
  done

  RUN_DATE="$(basename "$OUT_DIR" | cut -d- -f1-3)"
  FIRST_LEDGER="$(awk -F'\t' 'NR>1{print $6; exit}' "$REQ_LOG")"
  FIRST_READ_AT="$(awk -F'\t' 'NR>1{print $4; exit}' "$REQ_LOG")"
  LAST_LEDGER="$(awk -F'\t' 'NR>1{v=$6} END{print v}' "$REQ_LOG")"
  LAST_READ_AT="$(awk -F'\t' 'NR>1{v=$4} END{print v}' "$REQ_LOG")"
  if [[ -f "${OUT_DIR}/pair-files.txt" ]]; then
    PAIR_FILES_LABEL="$(awk '{printf "`%s` ", $0}' "${OUT_DIR}/pair-files.txt")"
  else
    PAIR_FILES_LABEL="not recorded by the run that wrote this directory"
  fi

  write_manifest
  echo "rebuilt ${MANIFEST} from stored bytes. Horizon was not contacted."
  exit 0
fi

# --- the asset set ---------------------------------------------------------

if [[ $# -gt 0 ]]; then
  PAIR_FILES=("$@")
else
  PAIR_FILES=(configs/demonstration-set.json configs/recorder-pairs.json)
fi

for f in "${PAIR_FILES[@]}"; do
  [[ -f "$f" ]] || { echo "no such pair file: $f" >&2; exit 1; }
done

PAIR_FILES_LABEL="$(printf '`%s` ' "${PAIR_FILES[@]}")"

mkdir -p "$BODY_DIR"

# Recorded so MANIFEST_ONLY can say which files the set came from rather than
# inventing a label. The set is an artifact of which files were loaded, per
# docs/evidences/2026-08-26-scan-64-assets-stored.md, so the files are evidence.
printf '%s\n' "${PAIR_FILES[@]}" >"${OUT_DIR}/pair-files.txt"

# The union of BASE assets, keyed on (type, code, issuer) and SORTED before
# iteration. Sorting is rule 2 of the non-negotiables: two runs over the same
# files must walk the set in the same order, or the reading times and the request
# log stop being comparable between runs. jq's own object key order is not a
# promise this script relies on.
ASSET_LIST="${OUT_DIR}/.assets.tsv"
jq -r '.pairs[].base | [(.type // ""), (.code // ""), (.issuer // "")] | @tsv' \
  "${PAIR_FILES[@]}" | sort -u >"$ASSET_LIST"

TOTAL="$(wc -l <"$ASSET_LIST" | tr -d ' ')"

printf 'seq\tkind\trequested_at_utc\treceived_at_utc\thttp_status\tlatest_ledger\thorizon_date\tbody_bytes\trecords\tsha256\tstored_file\turl\n' >"$REQ_LOG"

# One row per asset. Columns 1-5 identify the reading; the rest are the fields
# Horizon sent, as strings. accounts_authorized is column 6 and is the one figure
# this whole script exists to obtain.
printf 'seq,status,asset_code,asset_issuer,asset_type,accounts_authorized,accounts_authorized_to_maintain_liabilities,accounts_unauthorized,balances_authorized,balances_authorized_to_maintain_liabilities,balances_unauthorized,liquidity_pools_amount,num_liquidity_pools,contracts_amount,num_contracts,claimable_balances_amount,num_claimable_balances,num_accounts_deprecated,amount_deprecated,contract_id,flags_auth_required,flags_auth_revocable,flags_auth_immutable,flags_auth_clawback_enabled,latest_ledger,read_at_utc,body_sha256,stored_file\n' >"$CSV"

# --- one GET ---------------------------------------------------------------

do_get() {
  local url="$1" body_path="$2" head_path="$3"

  R_REQUESTED="$(now_utc)"
  # -X GET is redundant and stated anyway: this script never writes to Horizon.
  R_STATUS="$(curl -sS -X GET \
    --retry 3 --retry-delay 2 --retry-connrefused \
    --max-time 60 \
    -D "$head_path" -o "$body_path" \
    -w '%{http_code}' \
    "$url")"
  R_RECEIVED="$(now_utc)"

  # tolower() rather than awk's IGNORECASE, which is a gawk extension that macOS
  # awk silently ignores, and the LAST match rather than the first, because a
  # retry or redirect leaves more than one header block in the file. Both traps
  # are already documented in pull-holder-and-supply.sh and both are quiet.
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

pause_if_needed() {
  # Only sleeps when asked to. 63 requests against a 3600 per hour budget needs
  # no throttling at all; the delay exists so the same script can be pointed at a
  # far larger pair file without being rewritten.
  if [[ "$REQUEST_DELAY" != "0" ]]; then sleep "$REQUEST_DELAY"; fi
}

# CSV_COLUMNS is derived from the header rather than written twice. A placeholder
# row for an asset that was not read still has to have the same shape as a row
# that was, and counting commas by hand is how that goes wrong silently.
CSV_COLUMNS="$(head -1 "$CSV" | awk -F, '{print NF}')"

# pad_row <col1> <col2> ... emits the fields given, then empty cells up to
# CSV_COLUMNS, then a newline.
pad_row() {
  local n=$# i
  printf '%s' "$1"; shift
  for f in "$@"; do printf ',%s' "$f"; done
  for (( i = n; i < CSV_COLUMNS; i++ )); do printf ','; done
  printf '\n'
}

# ABSENT and null are different facts and the CSV keeps them apart. Horizon omits
# the deprecated fields entirely now; an older Horizon returned them as null.
# Collapsing both to an empty cell would lose which one this reading saw.
read -r -d '' JQ_ROW <<'JQ' || true
def f($k): if has($k) then (.[$k] // "null" | tostring) else "ABSENT" end;
def g($o; $k): if (.[$o] // {} | has($k)) then (.[$o][$k] // "null" | tostring) else "ABSENT" end;
._embedded.records[0]
| [ g("accounts";"authorized"), g("accounts";"authorized_to_maintain_liabilities"), g("accounts";"unauthorized"),
    g("balances";"authorized"), g("balances";"authorized_to_maintain_liabilities"), g("balances";"unauthorized"),
    f("liquidity_pools_amount"), f("num_liquidity_pools"),
    f("contracts_amount"), f("num_contracts"),
    f("claimable_balances_amount"), f("num_claimable_balances"),
    f("num_accounts"), f("amount"),
    f("contract_id"),
    g("flags";"auth_required"), g("flags";"auth_revocable"), g("flags";"auth_immutable"), g("flags";"auth_clawback_enabled")
  ] | @csv
JQ

echo "Horizon:     $HORIZON"
echo "Pair files:  ${PAIR_FILES[*]}"
echo "Assets:      $TOTAL"
echo "Output:      $OUT_DIR"
echo "Budget:      ${PAGE_BUDGET} pages x ${PAGE_LIMIT} = ${THRESHOLD} trustlines"
echo

seq_n=0
requested=0
failed=0

while IFS=$'\t' read -r a_type a_code a_issuer; do
  seq_n=$((seq_n + 1))

  if [[ "$a_type" == "native" || -z "$a_code" || -z "$a_issuer" ]]; then
    # No request. See the header: /assets has no address for the native asset,
    # and the two spellings that look like one are both wrong.
    pad_row "$seq_n" not_addressable "${a_code:-}" "${a_issuer:-}" "$a_type" >>"$CSV"
    printf '%4d/%d  %-10s %s\n' "$seq_n" "$TOTAL" "${a_code:-(native)}" "not addressable by /assets, no request sent"
    continue
  fi

  slug="${a_code}.${a_issuer:0:8}"
  body="${BODY_DIR}/${slug}.json"
  head="${BODY_DIR}/${slug}.headers.txt"
  url="${HORIZON}/assets?asset_code=${a_code}&asset_issuer=${a_issuer}"

  do_get "$url" "$body" "$head"
  requested=$((requested + 1))

  if [[ "$R_STATUS" != "200" ]]; then
    log_request "$seq_n" assets 0 "bodies/${slug}.json" "$url"
    pad_row "$seq_n" "http_$R_STATUS" "$a_code" "$a_issuer" "$a_type" >>"$CSV"
    printf '%4d/%d  %-10s HTTP %s, body kept\n' "$seq_n" "$TOTAL" "$a_code" "$R_STATUS"
    failed=$((failed + 1))
    pause_if_needed
    continue
  fi

  records="$(jq '._embedded.records | length' "$body")"
  log_request "$seq_n" assets "$records" "bodies/${slug}.json" "$url"

  if [[ "$records" == "0" ]]; then
    # A 200 with an empty collection. The (code, issuer) pair is not one Horizon
    # knows, which for a set built from a provisional pair file is a finding
    # about the file and not a transport error.
    pad_row "$seq_n" no_such_asset "$a_code" "$a_issuer" "$a_type" >>"$CSV"
    printf '%4d/%d  %-10s 200 with zero records: /assets knows no such (code, issuer)\n' "$seq_n" "$TOTAL" "$a_code"
    pause_if_needed
    continue
  fi

  row="$(jq -r "$JQ_ROW" "$body")"
  # The row is quoted CSV from jq; unquote the plain scalars so the file is one
  # consistent dialect. jq quotes every string, and every field here is a string.
  row="$(printf '%s' "$row" | sed 's/"//g')"
  printf '%s,ok,%s,%s,%s,%s,%s,%s,%s,bodies/%s.json\n' \
    "$seq_n" "$a_code" "$a_issuer" "$a_type" "$row" \
    "$R_LEDGER" "$R_RECEIVED" "$R_SHA" "$slug" >>"$CSV"

  authorized="$(printf '%s' "$row" | cut -d, -f1)"
  printf '%4d/%d  %-10s authorized=%-8s ledger=%s\n' \
    "$seq_n" "$TOTAL" "$a_code" "$authorized" "$R_LEDGER"

  pause_if_needed
done <"$ASSET_LIST"

FIRST_LEDGER="$(awk -F'\t' 'NR>1{print $6; exit}' "$REQ_LOG")"
FIRST_READ_AT="$(awk -F'\t' 'NR>1{print $4; exit}' "$REQ_LOG")"
LAST_LEDGER="$(awk -F'\t' 'NR>1{v=$6} END{print v}' "$REQ_LOG")"
LAST_READ_AT="$(awk -F'\t' 'NR>1{v=$4} END{print v}' "$REQ_LOG")"

rm -f "$ASSET_LIST"

write_manifest

echo
echo "assets in set: $TOTAL   requests sent: $requested   failed: $failed"
echo "wrote $CSV"
echo "wrote $MANIFEST"
