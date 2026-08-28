#!/usr/bin/env bash
#
# verify_empty_pools.sh
#
# One-off verification supporting the R-7 Step 4 decision: does the
# "pool is present" predicate need to be reserve-based, and is
# `reserve > 0` a predicate that actually discriminates?
#
# Two questions this answers:
#
#   Q1. Do liquidity pools with zero reserves exist on the public network?
#       The protocol docs confirm the state is reachable (LiquidityPoolDeposit
#       has a dedicated "if the pool is empty" branch). This checks whether
#       such pools are actually present on mainnet right now.
#
#   Q2. Does Horizon surface them via GET /liquidity_pools?
#       If Horizon filters empty pools out of its listing, the predicate is
#       vacuous in practice for any Snapshot built from Horizon, even though
#       it is not vacuous at the protocol level.
#
# A zero count for Q1 does NOT prove empty pools are impossible. It only
# shows none exist at this ledger. Treat a zero count as inconclusive and
# re-run, or reconstruct from Hubble state tables instead.
#
# Requires: curl, jq
# Usage:    ./verify_empty_pools.sh [max_pages]

set -euo pipefail

HORIZON="${HORIZON_URL:-https://horizon.stellar.org}"
MAX_PAGES="${1:-40}"
PAGE_LIMIT=200

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

echo "Horizon:   $HORIZON"
echo "Max pages: $MAX_PAGES (x$PAGE_LIMIT pools per page)"
echo

url="${HORIZON}/liquidity_pools?limit=${PAGE_LIMIT}&order=asc"
page=0
total=0
empty_total=0
half_empty_total=0
examples_shown=0

while [[ -n "$url" && "$page" -lt "$MAX_PAGES" ]]; do
  page=$((page + 1))
  body="$(curl -sS --fail-with-body "$url")"

  count="$(jq '._embedded.records | length' <<<"$body")"
  [[ "$count" -eq 0 ]] && break
  total=$((total + count))

  # Both reserves zero: pool entry exists, holds nothing at all.
  empty="$(jq '[._embedded.records[]
                | select((.reserves | map(.amount | tonumber) | max) == 0)]
               | length' <<<"$body")"
  empty_total=$((empty_total + empty))

  # Exactly one reserve zero. Should be unreachable under constant product
  # invariants; if this is ever non-zero, the reserve-based predicate needs
  # to say which reserve it tests, not just "reserves".
  half="$(jq '[._embedded.records[]
               | select((.reserves | map(.amount | tonumber) | min) == 0)
               | select((.reserves | map(.amount | tonumber) | max) > 0)]
              | length' <<<"$body")"
  half_empty_total=$((half_empty_total + half))

  if [[ "$empty" -gt 0 && "$examples_shown" -lt 5 ]]; then
    jq -r '._embedded.records[]
           | select((.reserves | map(.amount | tonumber) | max) == 0)
           | "  pool=\(.id) type=\(.type) fee_bp=\(.fee_bp) shares=\(.total_shares) trustlines=\(.total_trustlines) reserves=\(.reserves | map(.amount) | join(","))"' \
      <<<"$body" | head -n 5
    examples_shown=$((examples_shown + empty))
  fi

  printf "page %-3d scanned=%-7d empty=%-5d half_empty=%d\n" \
    "$page" "$total" "$empty_total" "$half_empty_total"

  url="$(jq -r '._links.next.href // ""' <<<"$body")"
  # Horizon returns a next link even on the last page; stop on a short page.
  [[ "$count" -lt "$PAGE_LIMIT" ]] && url=""
done

echo
echo "=== Result ==="
echo "pools scanned:            $total"
echo "both reserves zero:       $empty_total"
echo "exactly one reserve zero: $half_empty_total"
echo

if [[ "$empty_total" -gt 0 ]]; then
  echo "CONCLUSION: Horizon surfaces pools with zero reserves."
  echo "  -> 'reserve > 0' discriminates. Reserve-based predicate is sound."
elif [[ "$total" -gt 0 ]]; then
  echo "CONCLUSION: inconclusive. No empty pool found in the scanned range."
  echo "  -> Either none exist at this ledger, or Horizon filters them."
  echo "  -> Raise max_pages, or check Hubble state tables before deciding."
else
  echo "CONCLUSION: no pools returned. Check connectivity before reading this."
fi

if [[ "$half_empty_total" -gt 0 ]]; then
  echo
  echo "WARNING: pools with exactly one zero reserve exist."
  echo "  -> The predicate must state which reserve it tests."
fi
