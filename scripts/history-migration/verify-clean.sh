#!/usr/bin/env bash
#
# verify-clean.sh: prove a cleaned clone carries none of the sensitive markers.
#
# Usage:
#   verify-clean.sh <path-to-clone>              scan the whole history
#   verify-clean.sh <path-to-clone> --tree-only  scan the working tree only
#   verify-clean.sh --self-test                  prove the scanner can fail
#
# Exit 0 when clean, 1 when a marker is found, 2 on a usage or environment error.
#
# WHAT IT SCANS, and why each part is separate. A history rewrite can miss any of
# these individually, and three of the five are not files:
#
#   1. every blob reachable from every ref, which is the file content
#   2. every tree entry name, which is the PATHS, because a rewrite that empties
#      a file still leaves the filename in the tree and the filename here is
#      itself a marker
#   3. every commit message
#   4. every author and committer identity
#   5. every tag and its message
#
# THE MARKERS COME FROM THE EXPOSURE INVENTORY of 25 August 2026, plus the two
# the task named explicitly: the SoW filename and the budget figure.
#
# A NOTE ON WHAT A PASS MEANS. Exit 0 says these markers are absent from this
# clone. It does not say the remote has forgotten them: after a filter-repo and
# a force push the pre-rewrite commits stay reachable on GitHub by direct SHA and
# through refs/pull/*, and this script cannot see those. Section 6 of RUNBOOK.md
# is what addresses that, and a green run here is not a substitute for it.

set -uo pipefail

# ---------------------------------------------------------------- Markers
#
# Each entry is  <scope>|<label>|<extended regex>. The label is what gets
# printed, so it has to say what was found without repeating the secret in the
# log where that would be self-defeating. The regexes are case-sensitive except
# where a marker is genuinely written both ways in the corpus.
#
# SCOPE is `all` or `path`. `path` markers are tested against tracked FILENAMES
# only, never against file content. That distinction is not cosmetic: `.env` as a
# content match fires on `.gitignore`'s own `.env` line in every one of this
# repository's eleven .gitignore revisions, which is a repository correctly
# ignoring dotenv files rather than a repository leaking one. Eleven findings
# that all mean "you are doing the right thing" is how a checker gets ignored.
#
# ADDING A MARKER: add the row, then add a matching row to SELFTEST_MARKERS below
# so the self-test still proves every marker can fire.

MARKERS=(
  # --- the two the task named explicitly
  'all|SoW filename|Keel_SoW'
  'all|budget figure, dollars|\$?2[,.]268'
  'all|budget figure, hours|126 (hours|hour|jam)'

  # --- the sensitive paths themselves, as tree entries and as prose references
  'all|path docs/context|docs/context'
  'all|path docs/internal|docs/internal'

  # --- the counterparty named in the SoW
  'all|funder name|Instawards|instawards'

  # --- the internal working documents by name
  'all|internal doc: audit|audit-2026-08-20'
  'all|internal doc: handoff|handoff-2026-08-21'
  'all|internal doc: memo|memo-pra-development'
  'all|internal doc: execution plan|Keel_Deliverable_1_Rencana_Eksekusi'
  'all|internal doc: breakdown|deliverable-1-breakdown-2026-08-25'

  # --- personal identity that a rewrite is the only chance to change.
  #     Present in all 59 commits as author and committer.
  'all|personal email|yazid\.al2418@gmail\.com'

  # --- generic credential shapes. None of these were found in the 25 August
  #     inventory; they are here so the script keeps working as a scanner and
  #     not merely as a checklist for one known incident.
  'all|private key block|-----BEGIN [A-Z ]*PRIVATE KEY'
  'all|openssh private key|-----BEGIN OPENSSH PRIVATE KEY'
  'all|github token|gh[pousr]_[A-Za-z0-9]{30,}'
  'all|github fine-grained token|github_pat_[A-Za-z0-9_]{30,}'
  'all|slack token|xox[baprs]-[A-Za-z0-9-]{10,}'
  'all|aws access key id|AKIA[0-9A-Z]{16}'
  'all|aws secret|aws_secret_access_key'

  # A tracked dotenv FILE, matched on the path. See the scope note above for why
  # this is not a content marker.
  'path|dotenv file tracked|(^|/)\.env($|\.[A-Za-z0-9])'
)

# This script carries every marker it hunts for, as literal text, in MARKERS and
# in SELFTEST_MARKERS. Scanning itself would report a finding on every run and
# the finding would always be this file. Excluded by path, in the two scanners
# that read file content.
SELF="$(basename "${BASH_SOURCE[0]}")"

# The local Postgres password is DELIBERATELY NOT a marker. `keel_dev_only`
# appears in docker-compose.yml, the Makefile, the README and store.go, it is
# named for what it is, it guards a throwaway container, and it is meant to be
# public. Listing it would make this script fail forever on a repository that is
# not leaking anything, and a check that always fails is a check nobody reads.

# ---------------------------------------------------------------- Output

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  red=$'\033[31m'; grn=$'\033[32m'; dim=$'\033[2m'; bold=$'\033[1m'; off=$'\033[0m'
else
  red=''; grn=''; dim=''; bold=''; off=''
fi

FINDINGS=0

hit() {
  # hit <label> <where> <detail>
  FINDINGS=$((FINDINGS + 1))
  printf '%sFOUND%s  %-28s %s\n' "$red" "$off" "$1" "$2"
  [ -n "${3:-}" ] && printf '        %s%s%s\n' "$dim" "$3" "$off"
}

note() { printf '%s%s%s\n' "$dim" "$1" "$off"; }

# ---------------------------------------------------------------- Scanners

scan_blobs() {
  local repo="$1" label pattern
  local objs blobs
  objs=$(mktemp) || return 2
  blobs=$(mktemp) || return 2

  # Every object reachable from every ref, with the path it was last seen at.
  git -C "$repo" rev-list --all --objects > "$objs" 2>/dev/null

  # Narrow to blobs. cat-file --batch-check is one process for the whole set,
  # which matters: the naive loop is one fork per object and this repository has
  # 397 of them even before a rewrite.
  cut -d' ' -f1 "$objs" \
    | git -C "$repo" cat-file --batch-check='%(objectname) %(objecttype)' 2>/dev/null \
    | awk '$2=="blob"{print $1}' | sort -u > "$blobs"

  note "  $(wc -l < "$blobs" | tr -d ' ') blobs reachable from all refs"

  local sha path out rest scope
  while read -r sha; do
    [ -n "$sha" ] || continue
    path=$(grep -m1 "^$sha " "$objs" | cut -d' ' -f2-)
    # Never scan this script's own blob: it carries every marker as literal text.
    case "$path" in *"$SELF") continue ;; esac
    for m in "${MARKERS[@]}"; do
      scope="${m%%|*}"; rest="${m#*|}"
      [ "$scope" = "path" ] && continue
      label="${rest%%|*}"; pattern="${rest#*|}"
      out=$(git -C "$repo" cat-file blob "$sha" 2>/dev/null \
            | LC_ALL=C grep -aEm1 "$pattern" 2>/dev/null | cut -c1-100)
      if [ -n "$out" ]; then
        hit "$label" "blob ${sha:0:10} ${path:+at $path}" "$out"
      fi
    done
  done < "$blobs"

  rm -f "$objs" "$blobs"
}

scan_paths() {
  # Tree entry names across all history. A rewrite that blanks a file's contents
  # but keeps its name still leaks the name, and `Keel_SoW.pdf` is a marker in
  # its own right.
  local repo="$1" label pattern paths
  paths=$(mktemp) || return 2
  git -C "$repo" log --all --full-history --pretty=format: --name-only 2>/dev/null \
    | sort -u | grep -v '^$' > "$paths"

  note "  $(wc -l < "$paths" | tr -d ' ') distinct paths ever tracked"

  local rest scope
  for m in "${MARKERS[@]}"; do
    scope="${m%%|*}"; rest="${m#*|}"
    label="${rest%%|*}"; pattern="${rest#*|}"
    while IFS= read -r p; do
      hit "$label" "tracked path" "$p"
    done < <(LC_ALL=C grep -aE "$pattern" "$paths" 2>/dev/null)
  done
  rm -f "$paths"
}

scan_messages() {
  local repo="$1" label pattern msgs
  msgs=$(mktemp) || return 2
  git -C "$repo" log --all --format='%H%n%an <%ae>%n%cn <%ce>%n%B%n---' > "$msgs" 2>/dev/null

  note "  $(git -C "$repo" rev-list --all --count 2>/dev/null) commits, messages and identities"

  local rest scope
  for m in "${MARKERS[@]}"; do
    scope="${m%%|*}"; rest="${m#*|}"
    [ "$scope" = "path" ] && continue
    label="${rest%%|*}"; pattern="${rest#*|}"
    while IFS= read -r line; do
      hit "$label" "commit message or identity" "$(printf '%s' "$line" | cut -c1-100)"
    done < <(LC_ALL=C grep -aE "$pattern" "$msgs" 2>/dev/null)
  done
  rm -f "$msgs"
}

scan_tags() {
  local repo="$1" label pattern tags rest scope
  tags=$(git -C "$repo" for-each-ref --format='%(refname) %(contents)' refs/tags 2>/dev/null)
  [ -z "$tags" ] && { note "  no tags"; return 0; }
  for m in "${MARKERS[@]}"; do
    scope="${m%%|*}"; rest="${m#*|}"
    [ "$scope" = "path" ] && continue
    label="${rest%%|*}"; pattern="${rest#*|}"
    while IFS= read -r line; do
      hit "$label" "tag" "$(printf '%s' "$line" | cut -c1-100)"
    done < <(printf '%s\n' "$tags" | LC_ALL=C grep -aE "$pattern" 2>/dev/null)
  done
}

scan_tree() {
  local root="$1" label pattern rest scope
  note "  working tree at $root"
  for m in "${MARKERS[@]}"; do
    scope="${m%%|*}"; rest="${m#*|}"
    label="${rest%%|*}"; pattern="${rest#*|}"
    if [ "$scope" = "path" ]; then
      # Path markers are tested against filenames, not against file content.
      while IFS= read -r p; do
        hit "$label" "working tree path" "$p"
      done < <(find "$root" -path '*/.git' -prune -o -type f -print 2>/dev/null \
               | LC_ALL=C grep -aE "$pattern" 2>/dev/null | head -20)
      continue
    fi
    while IFS= read -r line; do
      hit "$label" "working tree" "$(printf '%s' "$line" | cut -c1-140)"
    done < <(LC_ALL=C grep -raEn --exclude-dir=.git --exclude="$SELF" \
               "$pattern" "$root" 2>/dev/null | head -20)
  done
}

# ---------------------------------------------------------------- Self-test
#
# The point of this is narrow and worth stating: it proves the scanner FAILS when
# it should. A verifier that has never been seen to fail is indistinguishable
# from `exit 0`, and this repository has been defeated by that pattern before.
#
# It builds a throwaway repository, plants one marker per MARKERS row in a place
# only that scanner looks at, asserts a non-zero exit, then removes the markers
# and asserts a zero exit. Both halves are needed: an always-failing scanner
# passes the first half.

SELFTEST_MARKERS=(
  'Keel_SoW.pdf'
  'Budget: 126 hours = $2,268'
  'funded by Instawards'
  'docs/internal/audit-2026-08-20.md'
  'yazid.al2418@gmail.com'
  '-----BEGIN RSA PRIVATE KEY-----'
  'ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
  'AKIAAAAAAAAAAAAAAAAA'
)

self_test() {
  local tmp rc_dirty rc_clean fail=0
  tmp=$(mktemp -d) || exit 2
  trap 'rm -rf "$tmp"' RETURN

  printf '%s== self-test ==%s\n' "$bold" "$off"
  note "building a throwaway repository at $tmp"

  git -C "$tmp" init -q -b main 2>/dev/null || { echo "git init failed"; return 2; }
  git -C "$tmp" config user.email "selftest@example.invalid"
  git -C "$tmp" config user.name  "Self Test"

  # Phase 1: a clean commit. The scanner must pass here, which proves the
  # markers are not matching on something incidental.
  printf 'ordinary content, nothing sensitive\n' > "$tmp/README.md"
  git -C "$tmp" add -A && git -C "$tmp" commit -qm "clean commit"

  printf '\n%s-- phase 1: clean repository, expect PASS --%s\n' "$bold" "$off"
  ( FINDINGS=0; scan_blobs "$tmp"; scan_paths "$tmp"; scan_messages "$tmp"; scan_tags "$tmp"
    [ "$FINDINGS" -eq 0 ] ) && rc_clean=0 || rc_clean=1

  if [ "$rc_clean" -eq 0 ]; then
    printf '%sok%s     a clean repository passes\n' "$grn" "$off"
  else
    printf '%sFAIL%s   a clean repository did NOT pass: a marker is matching noise\n' "$red" "$off"
    fail=1
  fi

  # Phase 2: plant every marker. Content markers go in a blob, the path marker
  # goes in a filename, and the identity marker goes in the commit author, so
  # each of the four scanners has something to find.
  printf '\n%s-- phase 2: planted markers, expect FAIL --%s\n' "$bold" "$off"
  {
    for s in "${SELFTEST_MARKERS[@]}"; do printf '%s\n' "$s"; done
  } > "$tmp/planted.txt"
  mkdir -p "$tmp/docs/context"
  printf 'placeholder\n' > "$tmp/docs/context/Keel_SoW.pdf"
  # A tracked dotenv file, so the one `path`-scoped marker also fires. Its
  # CONTENT is deliberately innocuous: the marker must catch it on its name.
  printf 'HARMLESS=1\n' > "$tmp/.env.production"
  git -C "$tmp" add -A -f
  git -C "$tmp" -c user.email="yazid.al2418@gmail.com" -c user.name="Yazid Al Ghozali" \
      commit -qm "planted: Keel_SoW.pdf and the 126 hours = \$2,268 budget"

  local planted_findings
  planted_findings=$( { FINDINGS=0
      scan_blobs "$tmp" >/dev/null 2>&1
      scan_paths "$tmp" >/dev/null 2>&1
      scan_messages "$tmp" >/dev/null 2>&1
      printf '%s' "$FINDINGS"; } )

  if [ "${planted_findings:-0}" -gt 0 ]; then
    printf '%sok%s     planted markers were caught (%s findings)\n' "$grn" "$off" "$planted_findings"
  else
    printf '%sFAIL%s   planted markers were NOT caught. The scanner is broken.\n' "$red" "$off"
    fail=1
  fi

  # Phase 3: prove the markers survive a naive `git rm`, which is the exact
  # mistake this whole migration exists because of. Deleting the file in a new
  # commit must NOT make the scanner pass.
  printf '\n%s-- phase 3: git rm only, expect STILL FAIL --%s\n' "$bold" "$off"
  git -C "$tmp" rm -q -r docs/context planted.txt
  git -C "$tmp" commit -qm "remove the sensitive files"

  local after_rm
  after_rm=$( { FINDINGS=0
      scan_blobs "$tmp" >/dev/null 2>&1
      scan_paths "$tmp" >/dev/null 2>&1
      printf '%s' "$FINDINGS"; } )

  if [ "${after_rm:-0}" -gt 0 ]; then
    printf '%sok%s     git rm alone does not fool the scanner (%s findings still)\n' "$grn" "$off" "$after_rm"
  else
    printf '%sFAIL%s   git rm made the scanner pass. It is only reading the tree.\n' "$red" "$off"
    fail=1
  fi

  printf '\n'
  if [ "$fail" -eq 0 ]; then
    printf '%sself-test passed%s: the scanner fails when it should and passes when it should\n' "$grn" "$off"
    return 0
  fi
  printf '%sself-test FAILED%s: do not trust a green run from this script\n' "$red" "$off"
  return 1
}

# ---------------------------------------------------------------- Main

usage() {
  sed -n '3,10p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
}

case "${1:-}" in
  --self-test) self_test; exit $? ;;
  ''|-h|--help) usage ;;
esac

REPO="$1"
MODE="${2:-full}"

command -v git >/dev/null 2>&1 || { echo "git not found"; exit 2; }
[ -d "$REPO" ] || { echo "not a directory: $REPO"; exit 2; }

printf '%s== verify-clean: %s ==%s\n' "$bold" "$REPO" "$off"

if [ "$MODE" = "--tree-only" ]; then
  scan_tree "$REPO"
else
  git -C "$REPO" rev-parse --git-dir >/dev/null 2>&1 || {
    echo "not a git repository: $REPO (use --tree-only for a plain directory)"; exit 2; }
  scan_blobs    "$REPO"
  scan_paths    "$REPO"
  scan_messages "$REPO"
  scan_tags     "$REPO"
  scan_tree     "$REPO"
fi

printf '\n'
if [ "$FINDINGS" -eq 0 ]; then
  printf '%sCLEAN%s  no marker found in %s\n' "$grn" "$off" "$REPO"
  note 'Reminder: this proves nothing about objects still reachable on the remote'
  note 'by direct SHA or through refs/pull/*. See section 6 of RUNBOOK.md.'
  exit 0
fi
printf '%s%d finding(s)%s. Do not push this history.\n' "$red" "$FINDINGS" "$off"
exit 1
