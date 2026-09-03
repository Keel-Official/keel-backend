#!/usr/bin/env bash
#
# check-manual-recomputation.sh
#
# Layer 1 of the validation protocol, docs/methodology/10-validation.md section 1:
# take a raw order book, transcribe it into a spreadsheet, compute depth BY HAND,
# and compare against engine output. The protocol requires five assets and names
# testdata/manual/ as where the evidence lives. Criterion 4 of the acceptance
# criteria scores that work 10 of 100.
#
# CORRECTED 3 SEPTEMBER 2026. This paragraph used to add "and PRD section 12 lists
# it among the four that are never cuttable", and that is false. Section 12's list
# is the Blend backtest, the methodology document, the cross-validation, and the
# limitations section. Layer 1 is none of those: the cross-validation is Layer 3,
# and the methodology document is the document rather than the evidence its
# protocol asks for.
#
# THE CORRECTION MAKES THE CASE FOR THIS SCRIPT STRONGER, NOT WEAKER, which is why
# it is written out instead of deleted. Section 12 is the list that governs
# decisions made under time pressure, and Layer 1 is NOT on it. What protects Layer
# 1 is an acceptance criterion, a box that has to be ticked for the deliverable to
# be accepted, and a criterion at 10 of 100 fourteen days in is not protected by
# much. So the absence this script reports is more fragile than the sentence it
# used to carry implied, not less.
#
# It is the second paraphrase of docs/api/Keel_PRD.md found wrong in this
# repository on one day. The other dropped the word "Hubble" from criterion 3 and
# three plans scored against the result; DEC-002 section 8 records it. Quote that
# file, do not summarise it.
#
# This script reports how many of the required recomputations are actually present
# and, for each one, whether it identifies WHAT was computed and AT WHICH LEDGER.
#
# Usage: bash scripts/check-manual-recomputation.sh
#
# Exit code, and it is a gate rather than a report:
#   0  every required recomputation is present and identified
#   1  fewer than required, or one of them identifies nothing
#   2  the protocol could not be read, so this script verified nothing
#
# EXIT 2 IS NOT A PASS AND NOT THE SAME FAILURE AS EXIT 1. A check that cannot
# find its own requirement must say so instead of falling back to a number
# compiled into itself, because a hardcoded five would keep reporting confidently
# after the protocol moved to six.
#
# NEITHER SIDE OF THIS CHECK CAN BE EDITED TO SATISFY IT, which is the property
# that four earlier checks in scripts/audit-verification.sh lacked and were bitten
# for. The required count is read out of the PROTOCOL. The actual count is read off
# DISK. Landing a recomputation closes this. Rewording the protocol does not, and
# editing this script does not, because it holds no expected value of its own.
#
# ---------------------------------------------------------------------------
# WHAT THIS SCRIPT MAY NOT DO, AND IT IS THE REASON IT IS A SEPARATE FILE
#
# testdata/manual/ is a RED ZONE. It holds the hand recomputations that are the
# independent oracle for internal/domain/compute.go, and compute.go has been
# YELLOW since 25 August 2026, so those numbers are the structural reason to
# believe the implementation is checked against figures derived independently of
# it. Numbers produced by Claude must never become the numbers that test Claude's
# code.
#
# So this script COUNTS and IDENTIFIES. It never computes a depth, never fills a
# blank, and never writes to testdata/manual/. It opens those files read only, and
# the one thing it asserts about their contents is that each one says which asset
# and which ledger it is about. Whether the figures inside are RIGHT is not a
# question a script written on this side of the wall is allowed to answer; that is
# what the comparison against engine output is for.
# ---------------------------------------------------------------------------

set -u
cd "$(dirname "$0")/.." || exit 2

PROTOCOL="docs/methodology/10-validation.md"
DIR="testdata/manual"

# Colour only when stdout is a terminal, and never when NO_COLOR is set. Same rule
# as scripts/audit-verification.sh, and for the reason recorded there: this script
# is meant to be read by CI and by the audit, and a report that is only correct on
# a terminal is not a report.
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  green=$'\033[32m'; red=$'\033[31m'; dim=$'\033[90m'; bold=$'\033[1m'; off=$'\033[0m'
else
  green=''; red=''; dim=''; bold=''; off=''
fi

# ---- the requirement, read out of the protocol ----------------------------
#
# "| Sample size | 5 assets, chosen to span the risk range |" in the Layer 1
# property table. Matched on the WORDS rather than on the row position, so
# reordering that table cannot make this read zero, and bounded by the start of
# section 2 so the Layer 2 sample size of ten can never be picked up by accident.
required_count() {
  awk '
    /^## 1\. Layer 1/            { f = 1 }
    /^## 2\./                    { if (f) exit }
    f && /Sample size/           { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+$/) { print $i; exit } }
  ' "$PROTOCOL" 2>/dev/null
}

required=$(required_count)
if ! printf '%s' "$required" | grep -Eq '^[0-9]+$' || [ "$required" -eq 0 ]; then
  printf "%sLayer 1 hand recomputation%s\n\n" "$bold" "$off"
  printf "  %sthe required sample size could not be read from %s%s\n" \
    "$red" "$PROTOCOL" "$off"
  printf "  %ssection 1 must carry a 'Sample size' row holding a number%s\n\n" \
    "$dim" "$off"
  echo "VERIFIED NOTHING. This check has no requirement to measure against."
  exit 2
fi

# ---- what is on disk -----------------------------------------------------
#
# Every regular file in the directory is a candidate EXCEPT README.md, which is
# the specification of the shape a recomputation must take rather than one of the
# recomputations, and dotfiles. Nothing is matched on extension: the protocol says
# "spreadsheet files" without fixing a format, and a check that demanded .md would
# report a perfectly good .csv as absent.
candidates() {
  [ -d "$DIR" ] || return 0
  find "$DIR" -maxdepth 1 -type f ! -name 'README.md' ! -name '.*' 2>/dev/null | sort
}

# An asset is the pair (code, issuer) and is NEVER matched on the ticker. That is
# a standing rule of this repository, stated for configs/ in the CLAUDE.md zone
# map, and it is what this matcher enforces: the issuer address is the half a
# ticker cannot fake. A file that says "USDC" and nothing else has named a string,
# not an asset, because several issuers use that code and one of them is the one
# that matters.
#
# A Stellar public key is G followed by 55 base32 characters.
names_asset() { grep -Eq '\bG[A-Z2-7]{55}\b' "$1" 2>/dev/null; }

# The ledger sequence. Every output in this repository carries LedgerSeq, rule 1
# of the non-negotiables, and a hand recomputation without one cannot be compared
# against anything: the book it describes is a book at a moment, and without the
# moment there is no engine output to put beside it.
#
# The label and the number, at least five digits apart from each other by no more
# than a short run of punctuation. This accepts "**Ledger:** 61340263" as the
# golden fixture writes it and "Ledger,61340263" as a CSV would.
names_ledger() { grep -Eiq 'ledger[_ ]?(seq(uence)?)?[^0-9]{0,20}[0-9]{5,}' "$1" 2>/dev/null; }

# The remaining two fields the README requires. These are REPORTED, not gated, and
# the distinction is deliberate rather than laziness: a file missing its asset or
# its ledger cannot be checked against anything at all, while a file missing its
# source line or its date is checkable and merely under-documented. The first is
# an absence of evidence, the second is evidence with a gap in its provenance, and
# collapsing the two would make the count mean less.
names_source() { grep -Eiq 'source|provenance' "$1" 2>/dev/null; }
names_date()   { grep -Eq '[0-9]{4}-[0-9]{2}-[0-9]{2}' "$1" 2>/dev/null; }

mark() { if "$1" "$2"; then printf '%syes%s' "$green" "$off"; else printf '%sno %s' "$red" "$off"; fi; }

printf "%sLayer 1 hand recomputation%s\n" "$bold" "$off"
printf "  %sprotocol: %s section 1, requires %s%s\n" "$dim" "$PROTOCOL" "$required" "$off"
printf "  %sevidence: %s/%s\n\n" "$dim" "$DIR" "$off"

present=0
total=0
underdocumented=0

if [ ! -d "$DIR" ]; then
  printf "  %s%s/ does not exist%s\n\n" "$red" "$DIR" "$off"
else
  files=$(candidates)
  if [ -z "$files" ]; then
    printf "  %s%s/ exists and holds no recomputation%s\n\n" "$red" "$DIR" "$off"
  else
    printf "  %-44s %-6s %-6s %-6s %-6s%s\n" "FILE" "asset" "ledger" "source" "date" ""
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      total=$((total + 1))
      printf "  %-44s %-6s %-6s %-6s %-6s\n" \
        "${f#"$DIR"/}" \
        "$(mark names_asset "$f")" \
        "$(mark names_ledger "$f")" \
        "$(mark names_source "$f")" \
        "$(mark names_date "$f")"
      if names_asset "$f" && names_ledger "$f"; then
        present=$((present + 1))
        if ! names_source "$f" || ! names_date "$f"; then
          underdocumented=$((underdocumented + 1))
        fi
      fi
    done <<EOF
$files
EOF
    printf "\n"
  fi
fi

# ---- the summary line ----------------------------------------------------
#
# Its shape is depended on elsewhere. P2-21 in scripts/audit-verification.sh reads
# the line matching '^[0-9]+ of [0-9]+ present' to print the count beside its own
# finding, so keep the first six words of it if this output is ever reworked.
missing=$((required - present))
[ "$missing" -lt 0 ] && missing=0

if [ "$present" -ge "$required" ]; then
  printf "%s%s of %s present. Layer 1 is complete.%s\n" "$green" "$present" "$required" "$off"
else
  printf "%s%s of %s present. Layer 1 is NOT done, %s missing.%s\n" \
    "$red" "$present" "$required" "$missing" "$off"
fi

if [ "$total" -gt "$present" ]; then
  printf "  %s%s file(s) in the directory identify no asset or no ledger and are not counted%s\n" \
    "$dim" "$((total - present))" "$off"
fi
if [ "$underdocumented" -gt 0 ]; then
  printf "  %s%s counted file(s) are missing a source or a date line, see %s/README.md%s\n" \
    "$dim" "$underdocumented" "$DIR" "$off"
fi

printf "  %s%s/ is RED. Al works these by hand; the figures in them must be derived%s\n" \
  "$dim" "$DIR" "$off"
printf "  %sindependently of internal/domain/compute.go, which is why nothing on this side%s\n" "$dim" "$off"
printf "  %sof the wall may fill them in%s\n" "$dim" "$off"

[ "$present" -ge "$required" ] && exit 0
exit 1
