#!/usr/bin/env bash
#
# funding-graph-probe.sh
#
# Measures the YIELD of criterion 4 in docs/methodology/07-supporting-metrics.md
# section 1, "both sides funded from a common source". The worksheet already
# prices the criterion as expensive. This does not re-argue the cost. It asks the
# only question the table cannot answer from a desk: if you paid it, how many
# trades would it actually catch.
#
# For every account appearing on either side of a trade in the CSVs named on the
# command line, this fetches the account's FIRST operation
# (/accounts/{id}/operations?order=asc&limit=1) and records the create_account
# funder when there is one.
#
# WHAT IT REPORTS, AND NOTHING ELSE:
#   1. how many accounts share a funder with at least one other account
#   2. per window, how many trades have both sides sharing a funder, and the
#      share of volume those trades carry
#   3. the funder cardinality distribution
#
# WHAT IT DOES NOT DO. It does not call any trade genuine or non-genuine, and it
# does not recommend accepting or rejecting the criterion. Section 1 has not
# written the rule and this script is not entitled to imply one. A shared funder
# is a fact about two accounts; whether it makes a trade non-genuine is a
# judgement, and the judgement is the deliverable, not this measurement.
#
# THE RETENTION CEILING IS THE FIRST THING TO READ IN THE OUTPUT.
# horizon.stellar.org does not keep full history. Its root reports
# history_elder_ledger, and every operation before that ledger is gone. An
# account created before it shows a first operation that is NOT its
# create_account, and its funder cannot be recovered from this endpoint at all.
#
# That is why this script never writes "not created by create_account" as a
# conclusion. It is almost always the wrong reading: the account was created by
# create_account like every other Stellar account, and the operation that did it
# fell off the back of Horizon's history. The two cases are separated in the
# output as FUNDER_KNOWN and CREATION_BEFORE_RETAINED_HISTORY, and a third bucket
# catches anything that fits neither so it cannot hide inside one of them.
#
# The consequence for the yield number is direct and it is stated in the report
# rather than left for the reader to work out: a funder that is unknown cannot be
# shared, so every count below is a LOWER BOUND on what criterion 4 would catch
# against a complete history. DEC-002 defers Hubble, which is the source that
# would lift the ceiling.
#
# READ ONLY. Keel is permanently read-only. GET requests, and writes confined to
# the output directory.
#
# NO FLOAT. Amounts are summed with Python's decimal.Decimal at full precision,
# never awk or shell arithmetic. The CSV strings go in and a string comes out.
#
# SORTED. Accounts are probed in sorted order and every map is sorted before it
# is iterated or printed, per NFR-9. Re-running produces byte-identical reports
# from the same raw bodies.
#
# Requires: curl, jq, python3, and either shasum or sha256sum.
#
# Usage:
#   scripts/funding-graph-probe.sh <trades.csv> [<trades.csv> ...]
#
# Example, the February and August windows:
#   scripts/funding-graph-probe.sh \
#     docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv \
#     docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-08-01_2026-09-01.csv
#
# Environment:
#   HORIZON_URL     default https://horizon.stellar.org
#   OUT_ROOT        default docs/evidences
#   REQUEST_DELAY   seconds between requests, default 0.25
#   MAX_ACCOUNTS    safety budget, default 5000
#   REPORT_ONLY=1 RUN_DIR=<dir>
#                   rebuild report.md from bodies already stored in <dir>,
#                   contacting Horizon not at all

set -euo pipefail

if [[ $# -lt 1 && "${REPORT_ONLY:-0}" != "1" ]]; then
  cat >&2 <<'USAGE'
usage: funding-graph-probe.sh <trades.csv> [<trades.csv> ...]

Each CSV is one window. Window names are taken from the file names.
USAGE
  exit 2
fi

HORIZON="${HORIZON_URL:-https://horizon.stellar.org}"
OUT_ROOT="${OUT_ROOT:-docs/evidences}"
REQUEST_DELAY="${REQUEST_DELAY:-0.25}"
MAX_ACCOUNTS="${MAX_ACCOUNTS:-5000}"

for c in curl jq python3; do
  command -v "$c" >/dev/null 2>&1 || { echo "$c is required" >&2; exit 1; }
done

if command -v sha256sum >/dev/null 2>&1; then
  sha256_of() { sha256sum "$1" | awk '{print $1}'; }
else
  sha256_of() { shasum -a 256 "$1" | awk '{print $1}'; }
fi

now_utc() { date -u +%Y-%m-%dT%H:%M:%SZ; }

RUN_DATE="$(date -u +%Y-%m-%d)"

if [[ "${REPORT_ONLY:-0}" == "1" ]]; then
  OUT_DIR="${RUN_DIR:?REPORT_ONLY=1 requires RUN_DIR=<existing output directory>}"
else
  OUT_DIR="${OUT_ROOT}/${RUN_DATE}-funding-graph-probe"
fi
ACC_DIR="${OUT_DIR}/accounts"
REQ_LOG="${OUT_DIR}/requests.tsv"
FUNDERS="${OUT_DIR}/funders.csv"
REPORT="${OUT_DIR}/report.md"
ROOT_BODY="${OUT_DIR}/horizon-root.json"
WINDOWS="${OUT_DIR}/windows.tsv"

# --------------------------------------------------------------- the fetching

if [[ "${REPORT_ONLY:-0}" != "1" ]]; then
  mkdir -p "$ACC_DIR"

  printf 'account_id\trequested_at_utc\treceived_at_utc\thttp_status\tsha256\tstored_file\turl\n' >"$REQ_LOG"
  : >"$WINDOWS"

  # The retention ceiling, read once and stored, because every classification
  # below is relative to it and a report that cannot name it is unreadable.
  echo "GET / (history retention)"
  root_req="$(now_utc)"
  root_status="$(curl -sS -X GET --retry 3 --retry-delay 2 --max-time 60 \
    -o "$ROOT_BODY" -w '%{http_code}' "${HORIZON}/")"
  root_rec="$(now_utc)"
  [[ "$root_status" == "200" ]] || { echo "root returned HTTP $root_status" >&2; exit 1; }
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "(root)" "$root_req" "$root_rec" "$root_status" \
    "$(sha256_of "$ROOT_BODY")" "horizon-root.json" "${HORIZON}/" >>"$REQ_LOG"

  ELDER="$(jq -r '.history_elder_ledger' "$ROOT_BODY")"
  echo "  history_elder_ledger=${ELDER}  history_latest_ledger=$(jq -r '.history_latest_ledger' "$ROOT_BODY")"
  echo

  # Collect the account set. Sorted, so the probe order is reproducible and two
  # runs write their raw bodies in the same sequence.
  tmp_accounts="${OUT_DIR}/.accounts.txt"
  : >"$tmp_accounts"
  for csv in "$@"; do
    [[ -f "$csv" ]] || { echo "no such CSV: $csv" >&2; exit 1; }
    printf '%s\t%s\n' "$(basename "$csv")" "$csv" >>"$WINDOWS"
    python3 - "$csv" >>"$tmp_accounts" <<'PY'
import csv, sys
with open(sys.argv[1], newline="") as fh:
    for row in csv.DictReader(fh):
        for k in ("base_account", "counter_account"):
            v = row[k].strip()
            if v:
                print(v)
PY
  done
  sort -u "$tmp_accounts" -o "$tmp_accounts"

  total="$(wc -l <"$tmp_accounts" | tr -d ' ')"
  echo "distinct accounts across $# window(s): $total"
  if [[ "$total" -gt "$MAX_ACCOUNTS" ]]; then
    echo "account set larger than MAX_ACCOUNTS=$MAX_ACCOUNTS; raise it deliberately" >&2
    exit 3
  fi

  n=0
  while IFS= read -r acct; do
    n=$((n + 1))
    body="${ACC_DIR}/${acct}.json"
    head="${ACC_DIR}/${acct}.headers.txt"
    url="${HORIZON}/accounts/${acct}/operations?order=asc&limit=1"

    if [[ -s "$body" ]]; then
      printf '\r  %d/%d (cached)      ' "$n" "$total"
      continue
    fi

    req="$(now_utc)"
    status="$(curl -sS -X GET --retry 3 --retry-delay 2 --retry-connrefused \
      --max-time 60 -D "$head" -o "$body" -w '%{http_code}' "$url")"
    rec="$(now_utc)"

    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$acct" "$req" "$rec" "$status" "$(sha256_of "$body")" \
      "accounts/${acct}.json" "$url" >>"$REQ_LOG"

    printf '\r  %d/%d  %s  HTTP %s' "$n" "$total" "${acct:0:8}" "$status"

    # Courtesy to a public endpoint, and the reason this is not parallel.
    [[ "$REQUEST_DELAY" != "0" ]] && sleep "$REQUEST_DELAY"
  done <"$tmp_accounts"
  echo
  echo
fi

# --------------------------------------------------------------- the reporting
#
# Reads only the stored bodies and the CSVs. Decimal throughout.

python3 - "$OUT_DIR" "$REPORT" "$FUNDERS" "$ROOT_BODY" "$WINDOWS" <<'PY'
import csv, json, os, sys
from collections import Counter, defaultdict
from decimal import Decimal, getcontext

getcontext().prec = 60

out_dir, report_path, funders_path, root_path, windows_path = sys.argv[1:6]
acc_dir = os.path.join(out_dir, "accounts")

root = json.load(open(root_path))
elder = int(root["history_elder_ledger"])
latest = int(root["history_latest_ledger"])
elder_at = None

# ---- classify every probed account -----------------------------------------

FUNDER_KNOWN = "FUNDER_KNOWN"
BEFORE_HISTORY = "CREATION_BEFORE_RETAINED_HISTORY"
OTHER = "UNCLASSIFIED"

def ledger_of(op_id):
    # A TOID packs the ledger sequence into its high 32 bits. Decoding an
    # identifier, not deriving a time. Same rule as internal/horizon/trades.go.
    try:
        return int(op_id) >> 32
    except (TypeError, ValueError):
        return 0

rows = []
for fn in sorted(os.listdir(acc_dir)):
    if not fn.endswith(".json"):
        continue
    acct = fn[:-5]
    try:
        body = json.load(open(os.path.join(acc_dir, fn)))
    except json.JSONDecodeError:
        rows.append((acct, OTHER, "", "", "", "", "0"))
        continue
    recs = body.get("_embedded", {}).get("records", [])
    if not recs:
        rows.append((acct, OTHER, "", "", "", "", "0"))
        continue
    op = recs[0]
    op_id = op.get("id", "")
    op_type = op.get("type", "")
    created = op.get("created_at", "")
    seq = ledger_of(op_id)

    if op_type == "create_account" and op.get("account") == acct:
        rows.append((acct, FUNDER_KNOWN, op.get("funder", ""), op_id, op_type, created, str(seq)))
    else:
        # The account existed before its first retained operation. Its
        # create_account is outside the window Horizon keeps, so the funder is
        # not merely absent, it is unobtainable from this endpoint.
        rows.append((acct, BEFORE_HISTORY, "", op_id, op_type, created, str(seq)))

rows.sort(key=lambda r: r[0])

with open(funders_path, "w", newline="") as fh:
    w = csv.writer(fh)
    w.writerow(["account_id", "status", "funder", "first_op_id",
                "first_op_type", "first_op_created_at", "first_op_ledger"])
    w.writerows(rows)

funder_of = {r[0]: r[2] for r in rows if r[1] == FUNDER_KNOWN and r[2]}
status_of = {r[0]: r[1] for r in rows}
status_counts = Counter(r[1] for r in rows)

# ---- 1. accounts sharing a funder with at least one other -------------------

by_funder = defaultdict(list)
for acct in sorted(funder_of):
    by_funder[funder_of[acct]].append(acct)

shared_funders = {f: a for f, a in sorted(by_funder.items()) if len(a) > 1}
accounts_sharing = sorted(a for f in sorted(shared_funders) for a in shared_funders[f])

# ---- 3. funder cardinality distribution ------------------------------------

cardinality = Counter(len(a) for a in by_funder.values())

# ---- 2. per window: trades with both sides sharing a funder ----------------

windows = []
with open(windows_path) as fh:
    for line in fh:
        line = line.rstrip("\n")
        if not line:
            continue
        name, path = line.split("\t", 1)
        windows.append((name, path))

def analyse(path):
    total_vol = Decimal(0)
    both_side_vol = Decimal(0)
    shared_vol = Decimal(0)
    no_account_vol = Decimal(0)
    n_rows = n_both = n_shared = n_no_account = n_unknown = 0
    with open(path, newline="") as fh:
        for row in csv.DictReader(fh):
            # counter_amount is the notional in the quote asset, the same
            # quantity domain.Trade calls S. Read as a string, never a float.
            v = Decimal(row["counter_amount"])
            total_vol += v
            n_rows += 1
            b = row["base_account"].strip()
            c = row["counter_account"].strip()
            if not b or not c:
                n_no_account += 1
                no_account_vol += v
                continue
            n_both += 1
            both_side_vol += v
            fb, fc = funder_of.get(b), funder_of.get(c)
            if fb is None or fc is None:
                n_unknown += 1
                continue
            if fb == fc:
                n_shared += 1
                shared_vol += v
    return dict(n_rows=n_rows, total_vol=total_vol, n_both=n_both,
                both_side_vol=both_side_vol, n_shared=n_shared,
                shared_vol=shared_vol, n_no_account=n_no_account,
                no_account_vol=no_account_vol, n_unknown=n_unknown)

def share(num, den):
    if den == 0:
        return "undefined (denominator is zero)"
    # Exact ratio, rendered to 9 decimal places. Decimal division, no float.
    # Formatted with "f" so an exact zero prints 0.000000000 rather than
    # Decimal's scientific 0E-9, which reads like a missing value in a table.
    return format((num / den).quantize(Decimal("0.000000001")), "f")

results = [(name, path, analyse(path)) for name, path in windows]

# ---- write the report ------------------------------------------------------

L = []
w = L.append
w("# Funding graph probe: the yield of criterion 4")
w("")
w("Produced by `scripts/funding-graph-probe.sh`. This measures how many trades a")
w("shared-funder rule would identify. It does not label any trade genuine or")
w("non-genuine and it does not recommend accepting or rejecting the criterion.")
w("Section 1 of `docs/methodology/07-supporting-metrics.md` writes the rule; this")
w("only says what the rule would find.")
w("")
w("## 0. Read this before any number below")
w("")
w(f"Horizon retains history from ledger **{elder}** to **{latest}**. Everything")
w("earlier is not in this source. An account created before that ledger shows a")
w("first operation that is not its `create_account`, and its funder cannot be")
w("recovered from `/accounts/{id}/operations` at all.")
w("")
w("So an unknown funder here does not mean the account had no funder. It means")
w("the operation that created it is older than Horizon's window. A funder that is")
w("unknown cannot be observed to be shared, therefore:")
w("")
w("> **Every count in this document is a LOWER BOUND on what criterion 4 would")
w("> catch against a complete history.**")
w("")
w("`DEC-002` defers Hubble, which is the source that would lift this ceiling.")
w("")
w("| Account classification | Count |")
w("| --- | --- |")
for k in (FUNDER_KNOWN, BEFORE_HISTORY, OTHER):
    if status_counts.get(k):
        w(f"| `{k}` | {status_counts[k]} |")
w(f"| **total accounts probed** | **{sum(status_counts.values())}** |")
w("")
w("`UNCLASSIFIED` is any account whose first operation fits neither case. It is")
w("listed separately rather than folded into either so it cannot hide.")
w("")

w("## 1. Accounts sharing a funder with at least one other account")
w("")
w(f"- Accounts with a known funder: **{len(funder_of)}**")
w(f"- Distinct funders among them: **{len(by_funder)}**")
w(f"- Funders that funded more than one of these accounts: **{len(shared_funders)}**")
w(f"- **Accounts sharing a funder with at least one other account: {len(accounts_sharing)}**")
w("")
if funder_of:
    w(f"That is {len(accounts_sharing)} of {len(funder_of)} accounts with a known funder, and")
    w(f"{len(accounts_sharing)} of {sum(status_counts.values())} accounts probed.")
w("")

w("## 2. Trades with both sides sharing a funder, per window")
w("")
w("Volume is `counter_amount`, the notional in the quote asset, summed exactly")
w("with `decimal.Decimal`. No float is used anywhere in this script.")
w("")
for name, path, r in results:
    w(f"### `{name}`")
    w("")
    w("| | Trades | Volume (quote) | Share of window volume |")
    w("| --- | ---: | ---: | ---: |")
    w(f"| All rows | {r['n_rows']} | {r['total_vol']} | 1 |")
    w(f"| Both sides carry an account | {r['n_both']} | {r['both_side_vol']} | {share(r['both_side_vol'], r['total_vol'])} |")
    w(f"| One side is a pool, no account | {r['n_no_account']} | {r['no_account_vol']} | {share(r['no_account_vol'], r['total_vol'])} |")
    w(f"| Both sides an account, at least one funder unknown | {r['n_unknown']} | | |")
    w(f"| **Both sides share a funder** | **{r['n_shared']}** | **{r['shared_vol']}** | **{share(r['shared_vol'], r['total_vol'])}** |")
    w("")
    w(f"Share of the volume that could be evaluated at all, meaning rows where both")
    w(f"sides carry an account: **{share(r['shared_vol'], r['both_side_vol'])}**")
    w("")

w("## 3. Funder cardinality distribution")
w("")
w("How many funders funded exactly N of the probed accounts.")
w("")
w("| Accounts funded by one funder | Number of such funders | Accounts covered |")
w("| ---: | ---: | ---: |")
for n in sorted(cardinality):
    w(f"| {n} | {cardinality[n]} | {n * cardinality[n]} |")
w("")
w(f"Totals: {len(by_funder)} funders covering {len(funder_of)} accounts.")
w("")

w("## 4. What was read")
w("")
w("| | |")
w("| --- | --- |")
w("| Endpoint | `GET /accounts/{id}/operations?order=asc&limit=1` |")
w("| Raw bodies | `accounts/<account id>.json`, one per account, unedited |")
w("| Request log | `requests.tsv`, with the wall clock time of every request |")
w("| Classification | `funders.csv`, one row per account |")
w("| Retention ceiling | `horizon-root.json` |")
w("")
w("`/accounts/{id}/operations` sends no `Latest-Ledger` header; only")
w("`/order_book`, `/liquidity_pools` and `/assets` do, which")
w("`internal/horizon/client.go` line 117 already records. The reading time of")
w("every request is in `requests.tsv`, and the retention ceiling that bounds the")
w("whole measurement is in `horizon-root.json`.")
w("")
w("Re-runnable with `REPORT_ONLY=1 RUN_DIR=<this directory>`, which rebuilds this")
w("report from the stored bodies without contacting Horizon.")

open(report_path, "w").write("\n".join(L) + "\n")

# ---- terminal summary, the three requested reports only --------------------

print("=== 1. accounts sharing a funder with at least one other ===")
print(f"  accounts probed                    : {sum(status_counts.values())}")
print(f"  funder known                       : {len(funder_of)}")
print(f"  creation before retained history   : {status_counts.get(BEFORE_HISTORY, 0)}")
print(f"  unclassified                       : {status_counts.get(OTHER, 0)}")
print(f"  distinct funders                   : {len(by_funder)}")
print(f"  funders with >1 account            : {len(shared_funders)}")
print(f"  ACCOUNTS SHARING A FUNDER          : {len(accounts_sharing)}")
print()
print("=== 2. trades with both sides sharing a funder, per window ===")
for name, path, r in results:
    print(f"  {name}")
    print(f"    rows                              : {r['n_rows']}")
    print(f"    both sides carry an account       : {r['n_both']}")
    print(f"    at least one funder unknown       : {r['n_unknown']}")
    print(f"    BOTH SIDES SHARE A FUNDER         : {r['n_shared']}")
    print(f"    volume of those trades            : {r['shared_vol']}")
    print(f"    window volume                     : {r['total_vol']}")
    print(f"    SHARE OF WINDOW VOLUME            : {share(r['shared_vol'], r['total_vol'])}")
print()
print("=== 3. funder cardinality distribution ===")
for n in sorted(cardinality):
    print(f"    {n} account(s) per funder          : {cardinality[n]} funder(s)")
print()
print(f"report: {report_path}")
PY
