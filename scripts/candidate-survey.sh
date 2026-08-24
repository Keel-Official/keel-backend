#!/usr/bin/env bash
#
# candidate-survey.sh
#
# A TRIAGE INSTRUMENT, not a measurement. It reads a pair file, asks Horizon four
# cheap questions per pair, and prints one row each, so that the four liquidity
# buckets in docs/methodology/10-validation.md section 3 can be filled with assets
# chosen against numbers rather than against impressions.
#
# WHAT IT IS NOT. Nothing it prints is a Keel output, enters the methodology, or may
# be quoted as one. It computes in shell floating point, which the repository bans
# everywhere that matters and which is fine here for exactly that reason: these
# numbers pick a SAMPLE, they do not measure liquidity. Where a real figure is
# needed, `keel scan` produces it through internal/domain and decimal arithmetic.
#
# WHY IT REPORTS PRICES AND LEVEL COUNTS AND NOT AMOUNTS. An ask's amount is in the
# base asset and a bid's amount is in the QUOTE asset, which is trap 5 in
# internal/horizon/CLAUDE.md and was found by measuring rather than by reading. A
# triage script that got that conversion wrong would overstate sell-side liquidity by
# the price factor, and sell-side liquidity is one of the things the buckets are
# meant to separate. So it reports what cannot be misread: how many levels exist,
# what the best prices are, and how far apart they sit.
#
# USAGE
#   bash scripts/candidate-survey.sh scripts/record-pairs.example.json
#   bash scripts/candidate-survey.sh my-candidates.json | tee survey-$(date +%F).txt
#
# COST. Four requests per pair: /assets, /order_book, /liquidity_pools,
# /trade_aggregations. Twenty candidates is eighty requests against an hourly ceiling
# of about 3600, so this is affordable to run repeatedly while a list is being
# argued over.
#
# THE ASSET TYPE COMES FROM THE FILE AND IS NEVER INFERRED. Trap 4: Horizon answers a
# request naming the wrong type with an EMPTY book and no error, so a five character
# code guessed as credit_alphanum4 surveys as a dead asset. The pair file writes every
# type out for that reason and this script passes it through unchanged.

set -uo pipefail

HORIZON=${HORIZON:-https://horizon.stellar.org}
DAYS=${DAYS:-30}

pairs=${1:-}
if [ -z "$pairs" ] || [ ! -f "$pairs" ]; then
  echo "usage: bash scripts/candidate-survey.sh <pair-file.json>" >&2
  echo "  the same format keel record reads; copy scripts/record-pairs.example.json" >&2
  exit 1
fi
for tool in curl jq python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "candidate-survey: $tool is required" >&2; exit 1; }
done

# Horizon wants native written as "native" and everything else as CODE:ISSUER.
horizon_asset() {
  if [ "$3" = "native" ]; then printf 'native'; else printf '%s:%s' "$1" "$2"; fi
}

# Milliseconds is what /trade_aggregations wants, and the window is closed at both
# ends so the count means "in the last DAYS days" rather than "since the epoch".
now_ms=$(python3 -c 'import time; print(int(time.time()*1000))')
from_ms=$(python3 -c "print($now_ms - $DAYS*86400*1000)")

printf '%-14s %-8s %8s %6s %6s %14s %14s %9s %6s %14s %9s\n' \
  ASSET QUOTE HOLDERS BIDS ASKS BEST_BID BEST_ASK SPREAD% POOLS POOL_QUOTE "TRADES_${DAYS}D"

jq -r '.pairs[] | [.base.code, .base.issuer, .base.type, .quote.code, .quote.issuer, .quote.type] | @tsv' "$pairs" |
while IFS=$'\t' read -r bcode bissuer btype qcode qissuer qtype; do
  b=$(horizon_asset "$bcode" "$bissuer" "$btype")
  q=$(horizon_asset "$qcode" "$qissuer" "$qtype")

  # 1. /assets: holder count. The newer `accounts` object is preferred over the
  #    deprecated num_accounts for the reason decode.go gives: an adapter reading only
  #    the deprecated field goes silently to zero the day it is removed.
  if [ "$btype" = "native" ]; then
    holders="n/a"
  else
    holders=$(curl -sS --max-time 20 \
      "$HORIZON/assets?asset_code=$bcode&asset_issuer=$bissuer" |
      jq -r '(._embedded.records[0] // {}) | (.accounts.authorized // .num_accounts // "?")' 2>/dev/null)
  fi

  # 2. /order_book: level counts and the two best prices, read from price_r and not
  #    from the price string, which has already lost precision. Trap 2.
  book=$(curl -sS --max-time 20 \
    "$HORIZON/order_book?selling_asset_type=$btype&selling_asset_code=$bcode&selling_asset_issuer=$bissuer&buying_asset_type=$qtype&buying_asset_code=$qcode&buying_asset_issuer=$qissuer&limit=200")
  read -r nbids nasks bestbid bestask <<<"$(printf '%s' "$book" | jq -r '
    def px: if . == null then 0 else (.price_r.n / .price_r.d) end;
    [ (.bids | length), (.asks | length), (.bids[0] | px), (.asks[0] | px) ] | @tsv' 2>/dev/null |
    tr '\t' ' ')"
  nbids=${nbids:-0}; nasks=${nasks:-0}; bestbid=${bestbid:-0}; bestask=${bestask:-0}

  spread=$(python3 -c "
bid, ask = $bestbid, $bestask
print('n/a' if bid <= 0 or ask <= 0 else '%.2f' % ((ask - bid) / ((ask + bid) / 2) * 100))" 2>/dev/null || echo "n/a")

  # 3. /liquidity_pools: how many pools quote this pair, and how much quote asset
  #    sits in them. A pool is a venue the order book does not show.
  pools=$(curl -sS --max-time 20 "$HORIZON/liquidity_pools?reserves=$b,$q&limit=200")
  npools=$(printf '%s' "$pools" | jq -r '._embedded.records | length' 2>/dev/null || echo 0)
  poolq=$(printf '%s' "$pools" | jq -r --arg q "$q" '
    [ ._embedded.records[]?.reserves[]? | select(.asset == $q) | (.amount | tonumber) ] | add // 0' 2>/dev/null || echo 0)

  # 4. /trade_aggregations: how alive the pair is. Daily buckets, trade_count summed.
  trades=$(curl -sS --max-time 20 \
    "$HORIZON/trade_aggregations?base_asset_type=$btype&base_asset_code=$bcode&base_asset_issuer=$bissuer&counter_asset_type=$qtype&counter_asset_code=$qcode&counter_asset_issuer=$qissuer&resolution=86400000&start_time=$from_ms&end_time=$now_ms&limit=200" |
    jq -r '[ ._embedded.records[]?.trade_count | tonumber ] | add // 0' 2>/dev/null || echo 0)

  printf '%-14s %-8s %8s %6s %6s %14.7f %14.7f %9s %6s %14.7f %9s\n' \
    "$bcode" "$qcode" "$holders" "$nbids" "$nasks" "$bestbid" "$bestask" "$spread" "$npools" "$poolq" "$trades"
done

cat <<'NOTE'

Reading this table. A wide SPREAD% with few levels is thin. Zero on both sides with a
non-zero POOL_QUOTE is an asset whose only venue is the pool, which is a case the
methodology has to handle and which a healthy-assets-only sample never exercises.
TRADES_30D near zero is the dormant end. Nothing here is a threshold: the four
buckets and their boundaries are decision D-1 and belong in
docs/methodology/02-pair-selection.md section 5, written down before the list is cut.
NOTE
