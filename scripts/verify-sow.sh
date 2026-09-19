#!/usr/bin/env bash
#
# verify-sow.sh
#
# Scores this project against `Keel_SoW (1).pdf`, and nothing else.
#
# WHY THIS EXISTS. `docs/internal/deliverable-and-week-trackers.md` is a document,
# and a document is a claim about a moment. On 19 September 2026 three rows of it
# were wrong in the same direction: the historical path, the dashboard and the live
# API had all been scored from the repository while they were in fact ANSWERING in
# production. Two of those errors were found by curl and one was found by using the
# wrong USTRY issuer and believing the 404. This script is the difference between
# "the tracker says" and "the deployment says".
#
# WHAT IT CHECKS. Only the SOW clauses a machine can settle. A clause about whether
# a sentence has been written, or whether the Ambassador was told something, is not
# in here and must not be added: a script that reports PASS for a judgement nobody
# made is worse than no script.
#
# WHAT A FAIL MEANS. The clause is not true of the deployment right now. It does not
# say whose fault that is and it does not say the code is missing; the commonest
# cause in this project's history is a production image older than main.
#
# Usage:
#   scripts/verify-sow.sh                 # probe production
#   KEEL_API=http://localhost:3000 scripts/verify-sow.sh
#   scripts/verify-sow.sh --no-remote     # repository-only checks
#
set -uo pipefail

API="${KEEL_API:-https://api.keels.app}"
WEB="${KEEL_WEB:-https://keels.app}"
REMOTE=1
[ "${1:-}" = "--no-remote" ] && REMOTE=0

# The three ledgers DEC-023 authorises, and no others. Adding one here would be
# adding a decision, which section 3.9 of the runbook says is not a script's to make.
USTRY="USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC"
AUTHORISED_LEDGERS="61340172 61340262 61340263"

# The SOW's own numbers. Named as constants so a reader can see that 50 is the
# SOW's figure and not this script's opinion.
SOW_MIN_ASSETS=50
SOW_MIN_LEDGERS=50

green=""; red=""; yellow=""; off=""
if [ -t 1 ]; then green=$'\033[32m'; red=$'\033[31m'; yellow=$'\033[33m'; off=$'\033[0m'; fi

pass=0; fail=0; skip=0

ok()   { printf '  %sPASS%s  %s\n' "$green" "$off" "$1"; pass=$((pass+1)); }
no()   { printf '  %sFAIL%s  %s\n' "$red" "$off" "$1"; fail=$((fail+1)); }
na()   { printf '  %sSKIP%s  %s\n' "$yellow" "$off" "$1"; skip=$((skip+1)); }
head_() { printf '\n%s\n' "$1"; }

get() { curl -sS -m 30 "$@" 2>/dev/null; }
code() { curl -sS -o /dev/null -w '%{http_code}' -m 30 "$@" 2>/dev/null; }

# jq is not assumed. Every field this script reads is a flat scalar in a small
# object, and grep over the raw body is enough for that and adds no dependency to
# a box whose whole point is that it runs the distroless image and nothing else.
field() { printf '%s' "$1" | tr -d ' \n' | grep -o "\"$2\":[^,}]*" | head -1 | cut -d: -f2- | tr -d '"'; }

head_ "Keel against the SOW  --  $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
printf '  api %s\n  web %s\n' "$API" "$WEB"

# ---------------------------------------------------------------------------
head_ "Deliverable 1: Liquidity Depth Engine"

# "The methodology will be documented and reproducible." Reproducibility is NFR-9
# and is tested in Go; what is checked here is the weaker, and separately broken,
# claim that the set states ONE version. DEC-014 decided that and the set has
# drifted off it twice.
versions=$(grep -h -m1 '^\*\*Methodology version:\*\*' docs/methodology/*.md 2>/dev/null \
  | sed 's/.*:\*\* *//' | sort -u | tr '\n' ' ')
nver=$(printf '%s' "$versions" | wc -w | tr -d ' ')
codever=$(grep -o '"[0-9.]*-draft"' internal/domain/types.go | head -1 | tr -d '"')
if [ "$nver" = "1" ] && [ "$versions" = "$codever " ]; then
  ok "methodology states one version and the code agrees ($codever)"
else
  no "methodology version split: documents say [ $versions], code says $codever (DEC-014 decided ONE)"
fi

if grep -q '^\*\*Status:\*\* partial' docs/methodology/06-oracle-resilience.md 2>/dev/null; then
  no "06-oracle-resilience.md still reads 'Status: partial'"
else
  ok "no methodology document reports itself partial"
fi

# Layer 1 is the only structural guarantee that compute.go is checked against
# numbers derived independently of it, which is why it is scored here at all.
if [ -x scripts/check-manual-recomputation.sh ]; then
  # CAPTURED RATHER THAN PIPED, and the reason is worth the line. Under
  # `set -o pipefail`, `producer | grep -q` reports FAILURE whenever grep matches
  # early enough to close the pipe and SIGPIPE the producer, so the check inverts
  # exactly when it succeeds. This script reported "Layer 1 incomplete" against a
  # complete Layer 1 on its first run for that reason.
  manual_out=$(bash scripts/check-manual-recomputation.sh 2>&1)
  case "$manual_out" in
    *'Layer 1 is complete'*) ok "Layer 1 hand recomputations complete (>= 5 assets)" ;;
    *)                       no "Layer 1 hand recomputations incomplete" ;;
  esac
else
  na "scripts/check-manual-recomputation.sh absent"
fi

# The supporting metrics reached the API through one call site, and the whole of
# D1 rows 5 to 7 turned on whether scan supplied them. It is asserted here because
# it silently regressed once already.
if grep -q 'ComputeAssetRiskWith' cmd/keel/scan.go; then
  ok "scan supplies supporting metrics (ComputeAssetRiskWith)"
else
  no "scan calls the order-book-only variant: holder concentration, volume-to-supply and last-genuine-trade cannot be stored"
fi

if [ "$REMOTE" = 1 ]; then
  health=$(get "$API/v1/health")
  if [ -n "$health" ]; then
    monitored=$(field "$health" assetsMonitored)
    rev=$(field "$health" buildRevision)
    if [ "${monitored:-0}" -ge "$SOW_MIN_ASSETS" ] 2>/dev/null; then
      ok "engine runs across $monitored assets (SOW asks $SOW_MIN_ASSETS)"
    else
      no "only ${monitored:-0} assets monitored (SOW asks $SOW_MIN_ASSETS)"
    fi
    # An image older than main is this project's most repeated failure and it
    # always presents as a missing feature rather than as a stale deploy.
    if [ -n "$rev" ] && git rev-parse --verify --quiet "$rev" >/dev/null 2>&1; then
      behind=$(git rev-list --count "$rev..HEAD" 2>/dev/null || echo '?')
      if [ "$behind" = "0" ]; then
        ok "production runs HEAD ($rev)"
      else
        no "production is $behind commit(s) behind HEAD (running ${rev:0:7}); a feature merged after it will read as missing"
      fi
    else
      na "production revision ${rev:-unknown} not in this checkout"
    fi
  else
    no "GET /v1/health did not answer"
  fi

  # FR-8 to FR-10. Reported as a count rather than pass/fail per asset, because a
  # truncated trustline set is a REFUSAL by design (supporting.go ErrHolderSetTruncated)
  # and counting it as a failure would ask the engine to guess.
  first=$(get "$API/v1/assets?limit=1")
  aid_code=$(field "$first" code); aid_issuer=$(field "$first" issuer)
  if [ -n "$aid_code" ] && [ -n "$aid_issuer" ]; then
    d=$(get "$API/v1/asset/$aid_code:$aid_issuer/depth")
    unev=$(printf '%s' "$d" | tr -d ' \n' | grep -o '"unevaluatedFlags":\[[^]]*\]')
    case "$unev" in
      *NO_GENUINE_TRADE*) no "trade-derived metrics absent in production (run \`keel trades\`, runbook 3.6b)" ;;
      *)                  ok "trade-derived metrics present (FR-9, FR-10)" ;;
    esac
    case "$unev" in
      *HOLDER_CONCENTRATION*) no "holder concentration absent on $aid_code (run \`keel holders\`, runbook 3.6a)" ;;
      *)                      ok "holder concentration present on $aid_code (FR-8)" ;;
    esac
  else
    na "could not read an asset id from /v1/assets"
  fi
else
  na "remote checks disabled"
fi

# ---------------------------------------------------------------------------
head_ "Deliverable 2: Public Risk API and Blend Backtest Report"

if [ "$REMOTE" = 1 ]; then
  [ "$(code "$API/v1/health")" = "200" ] \
    && ok "live API URL answers (SOW 6.1 'Live API URL')" \
    || no "live API URL does not answer"

  # The SOW writes this path. The served contract mounts it under /v1, which
  # DEC-003 records; the clause is the SHAPE and it is satisfied under the prefix.
  if [ -n "${aid_code:-}" ]; then
    [ "$(code "$API/v1/asset/$aid_code:$aid_issuer/depth")" = "200" ] \
      && ok "GET /asset/{code}:{issuer}/depth serves current depth" \
      || no "current depth endpoint does not answer 200"
  fi

  # "historical metrics through the ledger query parameter". Three ledgers is the
  # designed end state, not a shortfall: each one costs about 45 minutes of walk
  # and DEC-023 authorises exactly these three.
  served=0
  for L in $AUTHORISED_LEDGERS; do
    [ "$(code "$API/v1/asset/$USTRY/depth?ledger=$L")" = "200" ] && served=$((served+1))
  done
  if [ "$served" = 3 ]; then
    ok "ledger query parameter serves all 3 ledgers DEC-023 authorises"
  elif [ "$served" -gt 0 ]; then
    no "ledger query parameter serves $served of 3 authorised ledgers"
  else
    no "ledger query parameter serves no authorised ledger (runbook 3.9 step A not run)"
  fi

  # A reconstruction that does not declare its gaps is the one failure mode this
  # product cannot survive, because a thinner book is its most interesting finding.
  body=$(get "$API/v1/asset/$USTRY/depth?ledger=61340262")
  case "$body" in
    *'"dataSource"'*'offers-implied'*) ok "historical row is labelled offers-implied, not passed off as a live read" ;;
    *) no "historical row does not declare itself a reconstruction" ;;
  esac
  case "$body" in
    *'"reconstruction"'*) ok "historical row carries its reconstruction counters" ;;
    *) no "historical row carries no reconstruction counters (DEC-022)" ;;
  esac
fi

# "An open, reproducible report". Open is checkable; reproducible is section 9's
# command list; whether the findings MEAN anything is Al's and is not scored here.
R=docs/report/blend-february-2026.md
if [ -f "$R" ]; then
  for s in "## 5." "## 6." "## 9."; do
    grep -q "^$s" "$R" && ok "report has section ${s#\#\# }" || no "report missing section ${s#\#\# }"
  done
  # Section 6.5 is the sentence about MEANING and the zone map gives it to Al. It
  # is checked for the marker rather than for prose, because only Al can clear it.
  if grep -q 'AL WRITES WHAT FOLLOWS' "$R"; then
    no "report 6.5 still holds the placeholder: the sentence about what the facts mean is unwritten (Al)"
  else
    ok "report 6.5 carries a written reading"
  fi
else
  no "backtest report absent"
fi

# ---------------------------------------------------------------------------
head_ "Deliverable 3: Asset Risk Dashboard and supporting materials"

if [ "$REMOTE" = 1 ]; then
  for p in / /dashboard /dashboard/methodology; do
    [ "$(code "$WEB$p")" = "200" ] && ok "dashboard $p answers" || no "dashboard $p does not answer"
  done
  if [ -n "${aid_code:-}" ]; then
    [ "$(code "$WEB/dashboard/asset/$aid_code:$aid_issuer")" = "200" ] \
      && ok "per-asset detail view answers" \
      || no "per-asset detail view does not answer"
  fi
fi

# "The demonstration set will contain at least 50 active Stellar assets."
pairs=$(grep -c '"base"' configs/demonstration-set.json 2>/dev/null || echo 0)
[ "$pairs" -ge "$SOW_MIN_ASSETS" ] \
  && ok "demonstration set holds $pairs pairs (SOW asks $SOW_MIN_ASSETS)" \
  || no "demonstration set holds $pairs pairs (SOW asks $SOW_MIN_ASSETS)"

# The only SOW row that has never been anything but zero. Captured rather than
# piped into `grep -q`, for the pipefail reason the Layer 1 check above records.
recording=$(find . -path ./.git -prune -o \( -iname '*.mp4' -o -iname '*.mov' -o -iname '*.webm' \) -print 2>/dev/null | head -1)
if [ -n "$recording" ]; then
  ok "a demo recording exists in the checkout ($recording)"
else
  no "no 3 to 5 minute demo recording exists (SOW D3, evidence 6.1)"
fi

# ---------------------------------------------------------------------------
head_ "Cross-validation, Week 1's numbered output"
ev=docs/evidences/2026-09-06-layer3-fifty-ledgers.md
if [ -f "$ev" ]; then
  n=$(grep -o '[0-9]\+ distinct ledgers\|seventy-nine\|79 ' "$ev" | head -1)
  ok "Layer 3 evidence present (${n:-see file}); SOW asks $SOW_MIN_LEDGERS sample ledgers"
else
  no "Layer 3 fifty-ledger evidence absent"
fi

# ---------------------------------------------------------------------------
head_ "Out-of-scope compliance (building an excluded thing costs the same as missing an included one)"
viol=0
grep -rqs 'txnbuild\|SubmitTransaction' cmd internal 2>/dev/null && { no "transaction signing or submission present"; viol=1; }
grep -rqs 'hooks.slack.com\|pagerduty\|webhook' cmd internal 2>/dev/null && { no "alert delivery present"; viol=1; }
[ "$viol" = 0 ] && ok "no signing, no submission, no alert delivery"

# ---------------------------------------------------------------------------
printf '\n  %d pass, %d fail, %d skipped\n\n' "$pass" "$fail" "$skip"
[ "$fail" = 0 ] || exit 1
