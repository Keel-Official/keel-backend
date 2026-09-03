#!/usr/bin/env bash
#
# audit-verification.sh
#
# Re-runs every claim in docs/internal/audit-2026-08-20.md.
# The point is to make that audit disputable rather than believed.
#
# A PROVEN line means the claim is still true of this repository as it stands.
# A NOT line means the claim is wrong, or it has been fixed.
# Both are useful. Once a finding is dealt with, its line MUST flip to NOT, and
# that is the signal the work is done.
#
# NOT EVERY PROVEN LINE IS AN OPEN FINDING. Some lines are the supporting half of
# a pair: the ones whose text starts with "although" state a fact that makes the
# line above it a defect, and they stay PROVEN forever because the fact stays
# true. P1-5 is the clearest case: "although the domain already has
# DataSourceTradesImplied" was what made the old CHECK constraint a bug, and it
# is still true now that the constraint is fixed. Read the pairs together.
#
# A claim must also never survive its own resolution. Where a finding is about a
# file or directory, the check is gated on that path still existing, because
# "imported by nobody" goes vacuously true the moment something is deleted. Where
# a finding is about file contents, the check matches the content rather than the
# filename, so a rename cannot make it disappear either.
#
# Usage: bash scripts/audit-verification.sh
# Exit code: always 0. This file reports, it does not judge.

cd "$(dirname "$0")/.." || exit 1

# Colour only when stdout is a terminal, and never when NO_COLOR is set.
#
# This was not cosmetic. The cross-check documented at the end of
# docs/internal/handoff-2026-08-21.md pipes this script into grep and reads the
# first field as a finding id, and while the escapes were unconditional that field
# arrived as `\033[32mP0-2`. The check therefore reported all eleven PROVEN ids as
# unaccounted for, which is the loudest possible false alarm from a check whose
# whole promise is that it prints nothing. A report that is only correct on a
# terminal is not a report, because the reason to write one is that something else
# reads it.
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  green=$'\033[32m'; red=$'\033[31m'; dim=$'\033[90m'; bold=$'\033[1m'; off=$'\033[0m'
else
  green=''; red=''; dim=''; bold=''; off=''
fi
export AUDIT_GREEN="$green" AUDIT_RED="$red" AUDIT_OFF="$off"
proven=0; not=0

check() {
  local id="$1" text="$2"; shift 2
  if "$@" >/dev/null 2>&1; then
    printf "%s%-7s PROVEN%s  %s\n" "$green" "$id" "$off" "$text"
    proven=$((proven + 1))
  else
    printf "%s%-7s NOT   %s  %s\n" "$red" "$id" "$off" "$text"
    not=$((not + 1))
  fi
}

section() { printf "\n%s%s%s\n" "$bold" "$1" "$off"; }

no_commit()        { ! git rev-parse HEAD; }

# P0-2 and P0-3 are the same defect seen from two sides: the methodology code has
# no body, so the golden fixture proves nothing. Both checks were re-anchored on
# 23 August 2026 when methodology 1.0.3 moved the computations out of the
# internal/depth directory and into internal/domain/compute.go.
#
# The old anchors would both have lied. "internal/depth holds no .go file" stayed
# true and stopped being relevant, a PROVEN line pointing at the wrong place. And
# `go vet -tags conformance` now succeeds, because the functions exist and panic
# instead of being absent, so P0-3 would have flipped to NOT while the fixture
# still tested nothing. A check that measures the old shape of a defect reports the
# defect gone the moment it changes shape.
methodology_unwritten() { grep -q 'panic("not implemented")' internal/domain/compute.go; }
conformance_dead()      { ! go test -tags conformance ./internal/conformance/ >/dev/null 2>&1; }
# The four internal/adapter findings are all gated on the directory still
# existing. Without the gate they would read as PROVEN once it was deleted,
# because "no package imports it" and "it is not in the zone map" are both
# vacuously true of a directory that is gone. A finding must not survive its own
# resolution.
adapter_exists()   { [ -d internal/adapter ]; }
adapter_uses_float(){ adapter_exists && grep -rq "float64" internal/adapter/; }
adapter_unused()   { adapter_exists && ! grep -rq "internal/adapter" --include='*.go' .; }
adapter_unzoned()  { adapter_exists && ! grep -q "internal/adapter" CLAUDE.md; }
# GENERALISED FROM adapter_unzoned, which is P1-19 above. That check asked whether
# ONE directory was in the zone map, and it retired itself when the directory was
# deleted, taking the question with it. The question outlived the directory: on 24
# August 2026 fourteen paths had no row in the map, including the whole of docs/
# except docs/methodology/, and .claude/ itself, so the file defining the zones sat
# outside the zones.
#
# A path counts as MAPPED when CLAUDE.md holds a line naming it in backticks
# alongside a zone word. Requiring the zone word is what stops a passing mention in
# prose from satisfying it.
#
# WHAT THIS CANNOT PROVE, stated because the limit is structural and not an
# oversight: it reads the document that the fix edits, so adding a row without
# thinking about the row satisfies it completely. It proves a path is named and
# zoned, never that the zone is right. Fifth time this class of limit has come up
# here, and the honest response is to write it down rather than pretend the check
# is stronger than it is.
# IT ASKS GIT, AND IT USED TO ASK find. That changed on 26 August 2026 and the
# reason is the finding rather than the fix.
#
# The old version walked the filesystem and then subtracted two directories by
# name, recordings/ and scripts/history-migration/, on the stated grounds that both
# were gitignored and a map row for a directory no clone has is a row nobody can
# check. The grounds were sound and the implementation restated them as a guess.
# The day recordings/samples/ was committed through the .gitignore negation, sixty
# files entered the repository and walked straight past the one check that exists
# to give every committed directory an owner, because the subtraction still named
# the whole tree.
#
# `git ls-files` states the actual rule instead of approximating it: a directory
# that holds a TRACKED file needs a row. Everything gitignored drops out because it
# is gitignored, not because somebody remembered to list it, so re-including a new
# path under recordings/ now makes this check speak up on its own. Untracked working
# files are not in the repository and do not need an owner yet.
mapped_dirs(){
  git ls-files 2>/dev/null \
    | grep -v '/\.DS_Store$' | grep -v '^\.DS_Store$' \
    | xargs -n1 dirname 2>/dev/null | sort -u | grep -v '^\.$'
}
# A directory is covered by its own row OR by an ancestor's row, and that is a
# DELIBERATE WEAKENING made on 26 August 2026, so it is written down rather than
# slipped in.
#
# Exact matching was right while every tracked directory was hand made. It stopped
# being right when recordings/samples/ landed: its contents are one directory per
# pair per day, created by keel record, so exact matching demanded a map row for
# sixty machine generated paths and one more for every pair on every future day. A
# check that can only be satisfied by work nobody should do is a check that gets
# switched off, and this repository has already lost coverage that way once.
#
# What is given up: a NEW subdirectory under a mapped path no longer announces
# itself. It silently inherits its parent's zone. That is the correct answer for
# recordings/samples/ and for docs/methodology/, and it would be the wrong answer
# for a subdirectory that genuinely deserves a different owner. The map is still
# the only place that decides, and this check still cannot tell whether a zone is
# the RIGHT one, which the note above already says.
unmapped_dirs(){
  local d p
  mapped_dirs | while IFS= read -r d; do
    p=$d
    while [ -n "$p" ] && [ "$p" != "." ]; do
      if grep -qE "\`$p/?\`.*(GREEN|YELLOW|RED)" CLAUDE.md; then
        continue 2
      fi
      p=$(dirname "$p")
    done
    printf '%s\n' "$d"
  done
}
zones_incomplete(){ [ -n "$(unmapped_dirs)" ]; }
# Proven while the float ban still stops at the pure packages.
float_ban_partial(){ ! grep -q "TestArchTanpaFloatDiSeluruhRepo" internal/domain/arch_test.go; }
metrics_missing()  { ! grep -riq "create table.*metrics" migrations/; }
# Any migration that puts a raw snapshot blob in the database, whatever the file
# is called. Matched on the column rather than on a filename, so renaming the
# file cannot make the finding disappear.
migration_stores_raw(){ grep -rqE '^\s*raw\s+JSONB' migrations/; }
tdd_says_no_raw(){ grep -qF 'Raw snapshots are not stored in the database' docs/architecture/Keel_Technical_Design_Document.md; }
tdd_versus_migrations(){ tdd_says_no_raw && migration_stores_raw; }
# A source or data_source CHECK that lists horizon and hubble but not
# trades-implied. Matched on the absent value, not on an exact string.
check_rejects_trades_implied(){
  grep -rqE "(data_)?source[[:space:]]+(TEXT[[:space:]]+)?(NOT NULL[[:space:]]+)?CHECK" migrations/ || return 1
  grep -rq "trades-implied" migrations/ && return 1
  return 0
}
contract_lacks()   { ! grep -qE "^ +$1:" docs/api/keel-openapi.yaml; }
# documentUrl pointing at a file that is not in the repository. Resolved from the
# URL itself rather than hardcoding 00-ikhtisar.md, so fixing the URL retires the
# finding and pointing it at some other missing file does not.
documenturl_dangling() {
  local url path
  url=$(grep -oE 'documentUrl: https://[^ ]+' docs/api/keel-openapi.yaml | head -1 | sed 's/documentUrl: //')
  [ -n "$url" ] || return 1
  path=${url#*/blob/main/}
  [ "$path" != "$url" ] || return 1
  [ ! -f "$path" ]
}
# Proven while the DEC-002 spike DoD has no recorded answer. Matched on the answer
# existing, not on the raw evidence file, because the evidence file legitimately
# stays one page long forever; what was missing was the table it was supposed to
# produce.
spike_dod_unanswered(){ ! grep -qF 'The spike result, 21 August 2026' docs/decisions/DEC-002-hold-bigquery.md; }
# The golden fixture omits a pool that demonstrably held reserves at its ledger.
# Gated on the evidence existing, so the finding cannot be raised without it.
#
# IT PROBED THE WRONG FILE UNTIL 25 AUGUST 2026, and the way it went wrong is the
# one this repository keeps meeting. The fixture has two halves: the hand document
# in testdata/fixtures/, which DEC-006 section 1 names, and the Go fixture that
# feeds the conformance test. This checked the Go half for `Pools: nil`. The pool
# went into GoldenSnapshot on 23 August, so the check flipped to NOT and reported
# the finding closed while the document DEC-006 is about had not moved, and while
# DEC-006 itself still said OPEN.
#
# Both halves are checked now. Either one omitting the pool is the finding, because
# a fixture whose two halves disagree about their own INPUT is worse than one that
# is wrong in both: the hand numbers stop describing the snapshot they are compared
# against, and nothing in a test run says so.
fixture_omits_pool(){
  [ -f docs/evidences/pool_ustry_usdc_2026-02.txt ] || return 1
  grep -qE 'Pools:[[:space:]]+nil' internal/conformance/fixture.go && return 0
  grep -qE 'Pools:[[:space:]]*\[\]' testdata/fixtures/ustry_pre_exploit.md && return 0
  return 1
}
# The methodology still states the manipulation cost was zero without saying which
# venue it was zero through, which is the half of the claim that was wrong. Zero
# through the order book is TRUE and directly observed. Zero through the combined
# market is false, because the pool held an honest price the whole time. So the
# check is on the distinction being drawn, not on a phrase: the finding is fixed
# once the document separates the two venues.
#
# The phrase this used to grep, 'Not thin. Zero.', was rewritten out of the
# methodology in 1.0.3, and the check flipped to NOT on the rewrite rather than on
# the correction. Anchoring on prose that the fix itself rewrites cannot work.
methodology_claims_zero(){
  grep -rqiE 'cost (was|is) zero' docs/methodology/ \
    && ! grep -rqF 'orderbookOnly' docs/methodology/
}

# P1-15 is a pair of conditions, and only the methodology half was ever checked.
# The finding is that the methodology demands both C_max terms while the output
# type carries only their minimum, so it is fixed when the type carries both. The
# sentence in the methodology moved from 'both have to be reported' to 'Both terms
# must be reported separately' in 1.0.3, which is the same requirement worded
# differently, so the grep is loose on wording and strict on the type.
cmax_terms_missing(){
  grep -rqiE 'both (terms )?(must|have to) be reported' docs/methodology/ \
    && ! grep -q 'MaxSafeCollateralLiquidation' internal/domain/types.go
}

learning_pointed_but_missing(){ grep -q "docs/learning" README.md && [ ! -d docs/learning ]; }
readme_promises_record(){ grep -qF "make record      # jalankan snapshot recorder" README.md; }

# P2-10 generalises P2-4 the way P2-9 generalised P1-19. P2-4 is anchored on one
# pre-translation Indonesian line that cannot come back, so it can no longer
# fail, and a check that cannot fail is not a check. It is left exactly as it is,
# because the audit is a DATED document and its claim P2-4 has to keep matching
# the line that carries its id.
#
# This asks the question P2-4 was really asking, in a form that can fail in both
# directions: does the README's command table agree with the entrypoint about
# which subcommands have a body. `belum` is the stub helper in cmd/keel/main.go
# and "no body yet" is the README's phrase for the same state, so a subcommand
# that gains an implementation without the README noticing is PROVEN, and so is a
# README claiming an implementation that does not exist.
#
# WHAT IT CANNOT PROVE: that the body works. It reads a dispatch arm and a table
# row. `make record-once` is what proves the body.
subcommand_stubbed(){
  awk -v c="$1" '$0 ~ "case \""c"\":" {f=1; next} /^\tcase / {f=0} f' cmd/keel/main.go | grep -q 'belum('
}
readme_calls_stubbed(){ grep -qE '`make '"$1"'`[^|]*\|[^|]*no body yet' README.md; }
readme_disagrees_with_code(){
  local c
  for c in record scan serve; do
    if subcommand_stubbed "$c"; then
      readme_calls_stubbed "$c" || return 0
    else
      readme_calls_stubbed "$c" && return 0
    fi
  done
  return 1
}
empty_dirs_vanish(){
  for d in api internal/api internal/store; do
    [ -d "$d" ] && [ -z "$(ls -A "$d" 2>/dev/null)" ] && return 0
  done
  return 1
}
# THE ZONE HOOK, PROBED IN BOTH DIRECTIONS, AND RE-ANCHORED ON 25 AUGUST 2026.
# internal/domain/compute.go was the red zone until Al moved it to YELLOW so that
# Deliverable 1 could be finished by two hands instead of one. Every check below
# used to probe that file. Not one of them was deleted, because a check that
# disappears leaves no trace that the guard ever existed, and this repository has
# been defeated five times by exactly that. They were RE-ANCHORED instead, onto
# what is still red, and P2-6 was INVERTED: it used to ask whether the lock worked,
# and it now asks whether the lock outlived the zone it was built for.
#
# A stale lock is not a harmless leftover. It refuses work that the map now permits,
# and the map is what people read, so the disagreement surfaces as a guardrail
# misfiring rather than as a document being wrong. That gets the guardrail switched
# off, which is the failure the hook's own header names as the worst one.
#
# Runs the hook the way Claude Code does, and needs jq exactly like the hook does.
# Returns 0 when the hook REFUSES the command.
hook_refuses(){
  printf '%s' "$1" \
    | jq -Rs '{tool_name:"Bash", tool_input:{command:.}}' \
    | bash .claude/hooks/lindungi-zona-merah.sh 2>/dev/null \
    | grep -q '"deny"'
}
hook_absent(){
  [ ! -f .claude/hooks/lindungi-zona-merah.sh ] || ! command -v jq >/dev/null 2>&1
}
# INVERTED 25 AUGUST 2026. PROVEN while a lock on compute.go SURVIVES the zone
# change that retired it, in either of the two files that can carry one.
#
# The original claim, that the red zone lock leaked through Bash while the Edit and
# Write denials did not see Bash, was true, was closed on 24 August, and stopped
# being about anything the following day. Read the id in the dated audit as the
# question it was asking, which was whether the harness and the map agree about
# compute.go. That question survives the answer changing sides.
#
# Both files are checked because neither closes the other's route: the deny list
# does not see Bash, and the hook does not see Edit.
# Reads the deny array only. `compute.go` appearing anywhere in the file is not the
# finding: it is expected to appear under "ask", which is where the loosening put
# it, and matching the whole file would report the intended state as a defect.
compute_in_deny(){
  awk '/"deny"[[:space:]]*:/{f=1} f{print} f && /\]/{exit}' .claude/settings.json \
    | grep -q 'internal/domain/compute\.go'
}
stale_lock_on_compute(){
  compute_in_deny && return 0
  hook_absent && return 1
  hook_refuses 'sed -i "" s/a/b/ internal/domain/compute.go' && return 0
  hook_refuses 'rm -f internal/domain/compute.go'            && return 0
  return 1
}
# RE-ANCHORED 25 AUGUST 2026, FROM A ZONE RULE TO A WORKFLOW RULE. PROVEN while an
# in-place format that names no .go file is not refused.
#
# This used to ask whether a command could reach the red zone without naming it,
# and while the zone was internal/depth any command broad enough to reach the code
# named the zone by definition, so the route was closed for free. It reopened
# silently when the zone became a file, was closed on 24 August, and lost its
# subject entirely on the 25th: no red zone holds a .go file any more, so a sweep
# reaches nothing protected.
#
# The rule survives anyway, and deliberately. CLAUDE.md assigns `make fmt` to Al so
# that formatting has one owner and CI's gofmt check has one fix, and a zone change
# is a licence to loosen what the zone change required and nothing else. Keeping it
# is therefore a choice, and this check is where that choice is visible: if it is
# ever dropped, it gets dropped on purpose and with a line to change.
#
# `rm -rf internal/domain` came out of the probe list, because the directory is
# yellow now and refusing it would be over-refusal, which is P2-6b's question.
# `gofmt -w internal/domain/` stays, as a DIRECTORY target naming no file.
sweep_not_refused(){
  hook_absent && return 1
  ! hook_refuses 'gofmt -l -w .'              && return 0
  ! hook_refuses 'goimports -w ./...'         && return 0
  ! hook_refuses 'make fmt'                   && return 0
  ! hook_refuses 'gofmt -w internal/domain/'  && return 0
  return 1
}
# Over-refusing when the hook denies work it has no business denying. types.go and
# arch_test.go are Claude's to write and they sit in the same package as the zone,
# so the directory rule above is one careless character away from refusing them.
# Reading and running are allowed too: a red zone is not a secret zone.
#
# WIDENED 25 AUGUST 2026. compute.go is yellow, so writing to it by a NAMED path is
# ordinary work and refusing it is over-refusal. The named-file probes sit here
# rather than in P2-6, because P2-6 asks about a lock surviving in a settings file
# and this asks what the hook does to a command. A yellow file that the hook still
# refuses would be discovered as a misfire in the middle of Deliverable 1.
red_zone_over_refuses(){
  hook_absent && return 1
  hook_refuses 'sed -i "" s/a/b/ internal/domain/types.go' && return 0
  hook_refuses 'gofmt -w internal/domain/types.go'         && return 0
  hook_refuses 'gofmt -w internal/domain/compute.go'       && return 0
  hook_refuses 'sed -i "" s/a/b/ internal/domain/compute.go' && return 0
  hook_refuses 'cat internal/domain/compute.go'            && return 0
  hook_refuses 'go test ./internal/domain/ -run TestArch'   && return 0
  hook_refuses 'go test ./internal/domain/ 2>&1 | tail -5' && return 0
  hook_refuses 'gofmt -l .'                                && return 0
  hook_refuses 'make ci'                                   && return 0
  # Two multi-line probes, added 24 August 2026 with P2-6e. Once a newline counts
  # as a command separator, every ordinary two line command is a new chance to
  # over-refuse, and a two line command is what most real work looks like.
  hook_refuses 'cd /tmp
go test ./internal/domain/ -run TestArch'                  && return 0
  hook_refuses 'echo first
sed -i "" s/a/b/ internal/domain/types.go'                 && return 0
  return 1
}
# PROVEN while the hook reads PROSE as a command. Closing routes two and three
# bought this: the hook now refuses `git commit` when the message merely QUOTES a
# zone path beside a mutating word, or mentions `make fmt`. In this repository that
# is most commit messages, including the one that closed P2-6c, which had to be
# passed through a file to land.
#
# It is the failure the hook's own header names as the worst one. A guardrail that
# refuses the work it protects gets switched off, and then the zone has no lock at
# all. It is P2-6d and not part of P2-6b because the fix is different: P2-6b is
# about which PATHS are matched, this is about what counts as a COMMAND.
#
# THE FIX WAS AL'S, and not because it was hard. Claude is blocked from loosening
# the guardrail Claude is subject to, which is the correct arrangement and the
# reason this line was a finding rather than a patch. Two parts, both in the hook:
#
#   1. A heredoc body is DATA, so the body is not scanned. The full command is still
#      scanned when the thing being fed is an interpreter, so `python3 - <<PY`
#      writing to compute.go stays refused, which is what the seventh probe in P2-6
#      guards.
#   2. Anchor every rule to COMMAND POSITION, the start of the line or just after a
#      shell separator, so a verb inside a quoted string is not a verb. This loses
#      `find . -exec rm {} + internal/domain`, which is the deliberate path rather
#      than the accidental one.
#
# CLOSED 24 AUGUST 2026. Al applied the patch, Claude committed it on Al's
# instruction and then repaired it, which is P2-6e directly below. The workaround
# this replaces was `git commit -F <file>`, which worked and was not a fix, because
# it depended on remembering.
red_zone_refuses_prose(){
  hook_absent && return 1
  hook_refuses 'git commit -m "rm -rf testdata/fixtures is refused"' && return 0
  hook_refuses 'echo "make fmt is refused"'                          && return 0
  return 1
}
# PROVEN while the fix for P2-6d has reopened a route that P2-6 or P2-6c closed.
#
# EVERY LOOSENING NEEDS ONE OF THESE, and that is the general lesson rather than
# anything about heredocs. P2-6d was a real finding and its patch was correct in
# intent, and it still opened eight routes, because it changed WHAT COUNTS AS A
# COMMAND in a file where three separate rules all read that answer. It was probed
# before being committed and the probes are kept here so the eight cannot come back
# one at a time.
#
# The two that matter are not the exotic ones:
#
#   1. A MULTI-LINE COMMAND IS THE ORDINARY FORM. The hook collapsed newlines to
#      spaces, under a comment claiming that could only make it refuse MORE. That
#      was true until the command-position anchor existed, and then it was exactly
#      backwards: line two got glued to the end of line one, so its verb was no
#      longer at the start of anything. `cd /tmp` on line one was enough to walk a
#      mutation of compute.go straight through. Newlines are semicolons now.
#   2. TRUNCATING AT `<<` THREW AWAY THE REDIRECT. `cat <<'EOF' > compute.go` puts
#      the target AFTER the marker, so cutting the line there removed the very thing
#      the redirect rule reads. Only the heredoc BODY is dropped now, and the line
#      that opens it is scanned like any other command.
#
# The other six are the same mistake in smaller shapes: a command position is not
# always the first word of a line. `FOO=1 sed -i`, `sudo`, `env`, `time`, `nohup`,
# `xargs` and the `then` of a compound statement each put the verb one word later.
# RE-ANCHORED 25 AUGUST 2026, ONTO testdata/fixtures. The eight routes were found
# against compute.go, and compute.go is yellow now, so every probe here would have
# passed for the wrong reason and reported all eight reopened. The routes are
# properties of the hook's command parser, not of any one path, so they transfer
# whole. Losing them would mean losing the only record of what a loosening cost.
red_zone_reopened_by_the_fix(){
  hook_absent && return 1
  ! hook_refuses 'cat <<'"'"'EOF'"'"' > testdata/fixtures/ustry_pre_exploit.md
hello
EOF'                                                        && return 0
  ! hook_refuses 'cd /tmp
sed -i "" s/a/b/ testdata/fixtures/ustry_pre_exploit.md'    && return 0
  ! hook_refuses 'echo starting
rm -f testdata/fixtures/ustry_pre_exploit.md'               && return 0
  ! hook_refuses 'cd /tmp
make fmt'                                                   && return 0
  ! hook_refuses 'FOO=1 sed -i "" s/a/b/ testdata/fixtures/x.md' && return 0
  ! hook_refuses 'sudo rm -f docs/context/Keel_SoW.pdf'     && return 0
  ! hook_refuses 'env sed -i "" s/a/b/ testdata/fixtures/x.md'   && return 0
  return 1
}
# ADDED 25 AUGUST 2026. PROVEN while the loosening that moved compute.go out of the
# red zone is recorded as a placeholder rather than as a decision.
#
# EVERY LOOSENING NEEDS A CHECK OF ITS OWN. That is the lesson P2-6e was written to
# carry, and this is its second application. P2-6e asked whether the loosening
# reopened a route; this asks whether the loosening was written down at all. The row
# in the zone map went in on the day of the move carrying `DEC-00X`, a number that
# does not exist, so the largest loosening in this repository so far points at
# nothing. A decision record numbered later is a decision record that is numbered
# when somebody happens to remember.
#
# RE-ANCHORED 26 AUGUST 2026, ONTO docs/decisions/, AND THE OLD ANCHOR IS THE REASON.
# It read the zone map, on the stated grounds that the map is what the next reader
# consults and a dangling pointer there is the defect. That was half true and the
# wrong half was load-bearing: it matched the literal placeholder `DEC-00X`, so
# DELETING the placeholder closed the finding whether or not a decision record had
# been written. The pointer was the SYMPTOM. The missing decision is the defect, and
# on 26 August 2026 the symptom was tidied away — correctly, since no record governs
# the move and aiming the row at the nearest plausible one would have been worse —
# which would have flipped this line to NOT and reported the largest loosening in the
# repository as recorded. Fourth time this class of bug has been found in this file:
# a check anchored on the shape of a defect reports it gone the moment the shape
# changes.
#
# It now asks docs/decisions/ for a record that governs the move: one naming
# compute.go alongside the zone it moved INTO. No edit to CLAUDE.md can satisfy it,
# which is the point, and the number that record will carry is deliberately not
# written here, because a check naming DEC-007 in advance is a check that whoever
# writes DEC-007 has to come and edit.
#
# WHAT THIS CANNOT PROVE, stated because the limit is structural and is the same one
# P2-9 carries: it proves a record names the file and the zone, never that the record
# decides anything. A future document mentioning compute.go and the word yellow in
# passing satisfies it. Matching the phrasing of a decision nobody has written yet
# would be guessing at prose, so the weaker check is the honest one and this note is
# what stands in for the strength it does not have.
#
# The number collision between the two DEC-003 documents is a separate question and
# not this one.
loosening_unnumbered(){
  local f
  for f in docs/decisions/*.md; do
    grep -qF 'compute.go' "$f" 2>/dev/null || continue
    grep -qiE '\byellow\b' "$f" 2>/dev/null && return 1
  done
  return 0
}
# ADDED 25 AUGUST 2026. PROVEN while the rule that REPLACED the compute.go lock is
# not written where the harness can see it.
#
# A lock was removed and nothing mechanical took its place, which is correct: the
# rule that replaces it cannot be enforced by any hook. A function may only be
# written after its expected values exist in testdata/fixtures, and no permission
# layer can tell whether a number was computed before or after the code that meets
# it. So the rule lives in prose, and prose that nothing checks is the arrangement
# the zone map itself calls a suggestion rather than a lock.
#
# This is the weakest check in this file and it says so: it proves the sentence
# exists, never that it was followed. That limit is structural, the same one P2-9
# carries, and writing it down is the honest response.
ordering_rule_unwritten(){
  ! grep -qiE 'expected values exist in .?testdata/fixtures' CLAUDE.md
}
# PROVEN while the golden fixture and the inputs from outside have no lock, in
# either of the two files that would carry one.
#
# This is the OLDEST hole in the repository rather than a new one, and its age is
# the finding. testdata/fixtures/ and docs/context/ have been red in the zone map
# since the map existed, and until 25 August 2026 nothing anywhere refused a write
# to them: the deny list named one path, compute.go, and so did the hook. The map
# said red, the harness said nothing, and the gap survived the audit that produced
# P2-6 because that audit asked whether the lock it knew about worked.
#
# The fixture is the one artefact whose entire value is that Claude did not produce
# its numbers. internal/conformance/fixture.go says so in its own header, and
# DEC-006 section 5 repeats it: having Claude recompute the expected values destroys
# the safeguard the fixture exists to be. A document saying that, with no lock under
# it, is the arrangement the zone map calls a suggestion rather than a lock.
#
# Both halves are checked because neither closes the other's route, which is the
# same sentence the hook's own header carries about compute.go: the deny list does
# not see Bash, and the hook does not see Edit.
red_tree_leaks(){
  grep -qF 'Edit(testdata/fixtures/**)' .claude/settings.json || return 0
  grep -qF 'Edit(docs/context/**)'      .claude/settings.json || return 0
  hook_absent && return 0
  ! hook_refuses 'sed -i "" s/a/b/ testdata/fixtures/ustry_pre_exploit.md' && return 0
  ! hook_refuses 'echo x > testdata/fixtures/ustry_pre_exploit.md'         && return 0
  ! hook_refuses 'rm -rf testdata/fixtures'                                && return 0
  ! hook_refuses 'cp /tmp/x docs/context/Keel_SoW.pdf'                     && return 0
  ! hook_refuses 'cd /tmp
sed -i "" s/a/b/ testdata/fixtures/x.md'                                   && return 0
  return 1
}
# Over-refusing when that rule reaches work it has no business reaching, which for
# these two zones is a wider risk than it was for compute.go.
#
# Reading the fixture is not a courtesy, it IS Claude's job in there: reporting where
# the code and the hand-computed numbers disagree is the finding the fixture exists
# to produce, and a hook that refuses `cat` on it would refuse the finding. The
# sibling probe is the other direction, because `testdata/fixtures` as a bare prefix
# would swallow any path that merely starts with those letters.
red_tree_over_refuses(){
  hook_absent && return 1
  hook_refuses 'cat testdata/fixtures/ustry_pre_exploit.md'      && return 0
  hook_refuses 'grep -rn USTRY testdata/fixtures/'               && return 0
  hook_refuses 'go test ./internal/conformance/ -count=1'        && return 0
  hook_refuses 'sed -i "" s/a/b/ testdata/fixtures_old/x.md'     && return 0
  hook_refuses 'git commit -m "testdata/fixtures is locked now"' && return 0
  return 1
}

section "P0  What blocks everything else"
check P0-1 "There is not a single commit, though the origin remote is configured" no_commit
check P0-2 "The methodology computations are declared and panic, so nothing is implemented" methodology_unwritten
check P0-3 "The conformance test cannot pass, so the golden fixture tests nothing" conformance_dead

section "P1  Forked specification"
check P1-1 "The TDD and the migrations contradict each other on storing raw snapshots" tdd_versus_migrations
check P1-2 "A migration stores raw snapshots in a JSONB column" migration_stores_raw
check P1-3 "and the metrics table the API reads is absent from migrations" metrics_missing
check P1-4 "A data source CHECK rejects trades-implied" check_rejects_trades_implied
check P1-5 "although the domain already has DataSourceTradesImplied" \
  grep -q "DataSourceTradesImplied" internal/domain/types.go
# WHITESPACE, not content. The first version of this check matched the exact
# column alignment of the old struct field. When the scalar form came back with a
# different number of spaces the check went quietly green, which is the worst
# failure mode a check has: it reported the defect gone while the defect was
# there. Match the declaration, not its layout.
check P1-6 "types.go holds OracleResistance as a scalar" \
  grep -qE '^[[:space:]]*OracleResistance[[:space:]]+\*decimal\.Decimal' internal/domain/types.go
check P1-7 "although DEC-003 rejects the scalar form explicitly" \
  grep -qF "a single scalar quotient" docs/decisions/DEC-003-api-contract-v1-1.md
check P1-8 "CostToMaxReachablePrice exists in the code" \
  grep -q "CostToMaxReachablePrice" internal/domain/types.go
check P1-9 "but is absent from the API contract" contract_lacks costToMaxReachablePrice
check P1-10 "unevaluatedFlags is absent as a contract field" contract_lacks unevaluatedFlags
check P1-11 "bandConfidence is absent as a contract field" contract_lacks bandConfidence
check P1-12 "although 09-flags-and-bands requires both in the openapi file" \
  grep -qF 'add `unevaluatedFlags`, `bandConfidence`' docs/methodology/09-flags-and-bands.md
check P1-13 "The contract uses criticalDelta 0.5" \
  grep -q "criticalDelta: 0.5" docs/api/keel-openapi.yaml
check P1-14 "while DefaultParams uses a critical delta of 1.0" \
  grep -qF 'ManipulationCriticalDelta: dec("1.0")' internal/conformance/fixture.go
check P1-15 "The methodology requires both C_max terms reported, not only the minimum" \
  cmax_terms_missing
check P1-16 "internal/adapter uses float64" adapter_uses_float
check P1-17 "The float ban stops at the pure packages instead of covering the repository" float_ban_partial
check P1-18 "internal/adapter is imported by no package" adapter_unused
check P1-19 "internal/adapter does not appear in the CLAUDE.md zone map" adapter_unzoned
# Anchored on the CORRECTED figure being absent, not on the wrong one being
# present. The first version matched the sentence that explains the old error, so it
# stayed PROVEN after the correction was made. Third time this class of bug appeared
# in this file: a check that reads prose about the data instead of the data.
check P1-20 "DEC-001 has not adopted the on-chain ratio of about 1 to 2.05 million" \
  bash -c '! grep -qF "1 to 2.05 million" docs/decisions/DEC-001-ustry-identity.md'
check P1-21 "although evidence in the repo shows the executing trade was 5.3475699 USDC" \
  grep -qF '"base_amount": "5.3475699"' docs/evidences/spike_result_2.txt
check P1-22 "The curl in DEC-002 types USTRY as credit_alphanum4" \
  grep -q "credit_alphanum4&base_asset_code=USTRY" docs/decisions/DEC-002-hold-bigquery.md
check P1-23 "The curl in DEC-001 makes the same mistake" \
  grep -q "credit_alphanum4&base_asset_code=USTRY" docs/decisions/DEC-001-ustry-identity.md
check P1-24 "although the evidence states USTRY is credit_alphanum12" \
  grep -qF '"counter_asset_type": "credit_alphanum12"' docs/evidences/spike_result_2.txt
check P1-25 "The DEC-002 spike DoD has no recorded answer" spike_dod_unanswered
# Anchored on the correction being recorded, not on the wrong table being gone.
# Section 4 keeps its original wrong claim on purpose, as a record of what the
# document said at the time; what matters is that a reader is told it is wrong.
check P1-26 "DEC-003 carries the wrong reachable value with no correction recorded" \
  bash -c 'grep -qF "\`130.0627093\`, \`true\`" docs/decisions/DEC-003-api-contract-v1-1.md && ! grep -qF "Note on section 4" docs/decisions/DEC-003-api-contract-v1-1.md'
check P1-27 "although the fixture and the contract already corrected it to false" \
  grep -qF 'The delta 1.0 entry previously' docs/api/keel-openapi.yaml
check P1-28 "documentUrl points at the ciganytry org, not Keel-Official" \
  grep -q "github.com/ciganytry/keel" docs/api/keel-openapi.yaml
check P1-29 "and documentUrl points at a file that is not in the repository" documenturl_dangling
check P1-30 "assetBrokenBook uses ledgerClosedAt 2026-02-21T23:39:00Z" \
  grep -q "2026-02-21T23:39:00Z" docs/api/keel-openapi.yaml
check P1-31 "although the fixture and the evidence state 2026-02-22T00:10:21Z" \
  grep -q "2026-02-22T00:10:21Z" testdata/fixtures/ustry_pre_exploit.md
check P1-32 "GoldenSnapshot labels itself horizon" \
  grep -q "Source: domain.DataSourceHorizon" internal/conformance/fixture.go
check P1-33 "although the contract labels the same book trades-implied" \
  grep -q "dataSource: trades-implied" docs/api/keel-openapi.yaml
check P1-34 "The golden fixture records Pools nil although a pool held reserves at that ledger" fixture_omits_pool
check P1-35 "and the methodology still states the manipulation cost was zero" methodology_claims_zero

section "P2  Cheap hygiene"
check P2-1 "CLAUDE.md force-loads keel-openapi.yaml into every session" \
  grep -qF "@docs/api/keel-openapi.yaml" CLAUDE.md
check P2-2 "README points at docs/learning, and that directory does not exist" learning_pointed_but_missing
check P2-4 "README promises make record works, though it exits with code 3" readme_promises_record
check P2-5 "An empty directory exists that will vanish when somebody else clones" empty_dirs_vanish
check P2-6 "A lock on compute.go outlived the zone it was built for, in the deny list or the hook" stale_lock_on_compute
check P2-6b "or the same hook refuses a yellow file next door, which gets it switched off" red_zone_over_refuses
check P2-6c "or in-place formatting that names no .go file stops being Al's alone" sweep_not_refused
check P2-6d "The same hook reads prose as a command, so a commit message quoting a zone is refused" red_zone_refuses_prose
check P2-6e "or the fix for that reopens a route, because a newline or a heredoc hid the verb" red_zone_reopened_by_the_fix
check P2-12 "The compute.go move from RED to YELLOW is governed by no decision record" loosening_unnumbered
check P2-13 "and the fixture-first rule that replaces its lock is not written in the zone map" ordering_rule_unwritten
check P2-11 "The golden fixture and docs/context are red in the map and locked by nothing" red_tree_leaks
check P2-11b "or that lock reaches reading, which is the whole job Claude has in the fixture" red_tree_over_refuses
check P2-9 "A directory holding files has no row in the CLAUDE.md zone map, so it has no owner" zones_incomplete
# A PROVEN line that does not say WHICH path is missing is a chore rather than a
# finding, so the paths are printed here. Nothing is printed when the map is
# complete, which is the normal state and does not need announcing.
if zones_incomplete; then
  unmapped_dirs | while IFS= read -r d; do
    printf "        %sno row in the zone map:%s %s\n" "$dim" "$off" "$d"
  done
fi
check P2-10 "The README's command table and cmd/keel disagree about which subcommands have a body" \
  readme_disagrees_with_code
check P2-7 "The methodology file structure decision is still open in the methodology README" \
  grep -qF "The decision that has to be made" docs/methodology/README.md
check P2-8 "The fixture writes 'All four' for a list holding six flags" \
  grep -qF "All four must be reported" testdata/fixtures/ustry_pre_exploit.md

section "DEC-003 freeze conditions"
# The checklist in DEC-003 section 7 is a summary of these checks, not a
# substitute for them. A tick typed by hand is a claim; a passing check is
# evidence. Each line prints MET or OPEN.
syarat() {
  local nama="$1"; shift
  if "$@" >/dev/null 2>&1; then
    printf "%s       MET   %s%s\n" "$green" "$nama" "$off"
  else
    printf "%s       OPEN  %s%s\n" "$red" "$nama" "$off"
  fi
}

fixture_filled(){
  [ -f testdata/fixtures/ustry_pre_exploit.md ] &&
    ! grep -qE 'TODO|TBD|\?\?\?' testdata/fixtures/ustry_pre_exploit.md
}
# Matched as YAML VALUES, anchored to end of line. The first version of this
# check matched the prose in the assetBrokenBook description that says the
# placeholders are gone, so it reported OPEN precisely because the work was done.
# A check that reads prose instead of data is worse than no check.
contract_no_placeholders(){
  ! grep -qE '^[[:space:]]+([a-zA-Z]+: TODO-FIXTURE|reachable: null)[[:space:]]*$' docs/api/keel-openapi.yaml
}
spread_scale_agreed(){
  grep -qF "spreadExtremePct: '20.0'" docs/api/keel-openapi.yaml &&
    grep -qF 'ending in `Pct`, a reported quantity' docs/methodology/README.md
}
# Every question in DEC-003 section 6 must carry an Answered line that is not
# "not yet". Counting rather than eyeballing, because a question quietly dropped
# from the list would otherwise look like a question answered.
questions_answered(){
  local total unanswered
  total=$(grep -c '^   \*\*Answered:\*\*' docs/decisions/DEC-003-api-contract-v1-1.md || true)
  unanswered=$(grep -c '^   \*\*Answered:\*\* not yet' docs/decisions/DEC-003-api-contract-v1-1.md || true)
  [ "$total" -gt 0 ] && [ "$unanswered" -eq 0 ]
}
contract_validates(){
  command -v npx >/dev/null 2>&1 || return 1
  npx --yes @redocly/cli@1.34.2 lint docs/api/keel-openapi.yaml 2>&1 | grep -q 'is valid'
}
mocks_fresh(){
  local tmp
  tmp=$(mktemp -d) || return 1
  bash scripts/generate-api-mocks.sh "$tmp" >/dev/null 2>&1 || { rm -rf "$tmp"; return 1; }
  # README.md is hand written and not generated, so it is excluded rather than
  # counted as drift.
  diff -r -q -x README.md docs/api/mocks "$tmp" >/dev/null 2>&1
  local rc=$?
  rm -rf "$tmp"
  return $rc
}

syarat "1. golden fixture filled in by hand, no placeholders left" fixture_filled
syarat "2. no TODO-FIXTURE or reachable: null left in the contract" contract_no_placeholders
syarat "3. spreadPct scale agreed as percent, in contract and methodology" spread_scale_agreed
syarat "4. every question in DEC-003 section 6 carries a real Answered line" questions_answered
echo "       ---- beyond the four, what makes the contract usable at all ----"
syarat "the contract is structurally valid OpenAPI 3.1" contract_validates
syarat "docs/api/mocks matches the contract, so the frontend is not served stale data" mocks_fresh

section "Repository visibility, see DEC-004"
# ONLY THE BLOCK BELOW needs the network and gh, and it is skipped when either is
# missing, because this script has to stay useful offline. The two checks that
# follow it read local git alone and therefore always run: whether the history is
# clean is exactly the work that has to happen WHILE the repository is still
# private, so gating it on the repository already being public would report it
# only once it was too late to act on.
if ! command -v gh >/dev/null 2>&1; then
  echo "       gh is not installed, the visibility check is skipped"
elif ! private=$(gh repo view Keel-Official/keel-backend --json isPrivate --jq '.isPrivate' 2>/dev/null); then
  echo "       cannot reach GitHub, the visibility check is skipped"
elif [ "$private" = "true" ]; then
  printf "       the repository is still PRIVATE. The DEC-004 condition does not apply yet.\n"
  echo "       Trigger for opening: make conformance passes without a build tag"
else
  remaining=0
  for f in docs/context/Keel_SoW.pdf docs/internal; do
    if git ls-files --error-unmatch "$f" >/dev/null 2>&1 || [ -d "$f" ]; then
      printf "%s  DEC-004 VIOLATION  %s is still in the working tree although the repository is PUBLIC%s\n" "$red" "$f" "$off"
      remaining=$((remaining + 1))
    fi
  done
  if [ "$remaining" = 0 ]; then
    printf "%s       working tree clean: public, and both paths are out of the working tree%s\n" "$green" "$off"
  else
    echo "       See DEC-004 section 2. git rm alone is not enough; both are already in the history"
  fi
fi

# THE WORKING TREE IS NOT THE REPOSITORY, and until 26 August 2026 this section
# behaved as though it were. The block above asks `git ls-files` and `[ -d ]`,
# which is a question about the current commit and the current disk, and it
# answered "the DEC-004 condition is met" while every byte of docs/internal/ and
# docs/context/ was still reachable from `--all` and would land in the working
# directory of anyone who cloned. DEC-004 section 2 says this in words -- "git rm
# alone is not sufficient" -- and the check that was supposed to enforce section 2
# could not see the thing section 2 is about. That is the worst shape a report can
# take: not silence, but a green line that stops the reader.
#
# So the working-tree check keeps its logic untouched and loses its overbroad
# label, and history gets its own line. The two can disagree, and the point is
# that they can: clean tree plus dirty history is the state this repository is
# actually in, and it is the one state the old single check could not express.
#
# THE COUNT IS THE FINDING, not the boolean. "history is dirty" is a fact to file
# away; "35 objects are reachable" is a number that goes down as filter-repo runs
# and reaches zero when the work is done, so it is what gets printed. `grep -c`
# exits 1 on a count of zero, hence the `|| true`, which is the same guard
# questions_answered() uses above.
leaked_objects=$(git rev-list --objects --all 2>/dev/null | grep -cE 'docs/(internal|context)/' || true)
leaked_objects=${leaked_objects:-0}
check DEC-004 "docs/context/ and docs/internal/ are still reachable in the git history" \
  test "$leaked_objects" -gt 0
if [ "$leaked_objects" -gt 0 ]; then
  printf "%s       %s objects under docs/internal/ or docs/context/ are reachable from --all, expected 0%s\n" \
    "$red" "$leaked_objects" "$off"
  echo "       Anyone who clones gets them. DEC-004 section 2 gives the two roads: filter-repo now,"
  echo "       while nobody has forked, or a fresh public repository with one clean initial commit"
else
  printf "%s       history clean: no object path under docs/internal/ or docs/context/ is reachable from --all%s\n" "$green" "$off"
fi

# INFO, NOT PASS/FAIL, and deliberately so. A public history exposes the author
# and committer identity of every commit, which is a real consequence of DEC-004
# and belongs in this section. But WHICH identities are acceptable is Al's
# decision, and DEC-004 has not made it: there is no allowed set to compare
# against, so a PASS or FAIL here would be this script inventing the policy it is
# supposed to be checking. It reports and does not judge.
#
# NO ADDRESS IS WRITTEN INTO THIS FILE. The set is read out of the history, and
# only the DOMAINS are printed, never the addresses. This report is piped, pasted
# and read by people, and a check about not publishing a personal address should
# not be the thing that publishes it. The address COUNT is safe to print and is
# what tells Al whether the domain list is hiding several identities.
ident_domains(){ git log --all --format='%ae%n%ce' 2>/dev/null | sort -u | sed 's/.*@//' | sort -u; }
ident_addr_count=$(git log --all --format='%ae%n%ce' 2>/dev/null | sort -u | grep -c . || true)
ident_domain_list=$(ident_domains | tr '\n' ' ' | sed 's/ *$//')
ident_domain_count=$(ident_domains | grep -c . || true)
printf "       INFO  %s distinct author/committer email domains in history, across %s distinct addresses\n" \
  "${ident_domain_count:-0}" "${ident_addr_count:-0}"
printf "       INFO  domains: %s\n" "${ident_domain_list:-none}"
echo "       Not scored. DEC-004 names no acceptable set, and until it does this is Al's call to make"

section "Methodology completeness"
# ADDED 26 AUGUST 2026. Nothing in this repository fails when a methodology file
# ships with a blank in it, and docs/methodology/ is the PAID DELIVERABLE. Every
# other check here guards code or contracts against the methodology; none of them
# asks whether the methodology says anything yet. Two files currently announce in
# their own status line "Do not ship this file with blanks", which is an instruction
# with no enforcement behind it, and an instruction nothing enforces is the
# arrangement the zone map calls a suggestion rather than a lock.
#
# THIS SECTION ONLY READS. docs/methodology/ is RED and deliberately not hook-locked,
# because the map gives Claude a job inside it. That job is reporting, and this is the
# reporting.

# THE PER-FILE BREAKDOWN IS THE OUTPUT, not the total. Thirteen blanks across the
# tree is a number nobody can act on; six in 02-pair-selection.md and seven in
# 07-supporting-metrics.md tells Al which two documents to sit down with, and the
# per-file counts go down independently as each is filled. Matched on the literal
# marker the documents actually use.
blank_marker='_to be written_'
blanks_total(){ cat docs/methodology/*.md 2>/dev/null | grep -c "$blank_marker" || true; }
blanks_by_file(){
  local f n
  for f in docs/methodology/*.md; do
    n=$(grep -c "$blank_marker" "$f" 2>/dev/null || true)
    [ "${n:-0}" -gt 0 ] && printf '%s\t%s\n' "${n}" "$(basename "$f")"
  done
}
methodology_has_blanks(){ [ "$(blanks_total)" -gt 0 ]; }
check P2-14 "A methodology file still carries an unfilled _to be written_ blank" \
  methodology_has_blanks
if methodology_has_blanks; then
  printf "%s       %s blanks total, expected 0. The per-file counts are the finding:%s\n" \
    "$red" "$(blanks_total)" "$off"
  blanks_by_file | while IFS=$'\t' read -r n f; do
    printf "        %s%s blanks%s in %s\n" "$dim" "$n" "$off" "$f"
  done
else
  printf "%s       no _to be written_ blank left in docs/methodology/%s\n" "$green" "$off"
fi

# THE STATUS LINE IS LINE 4 IN ELEVEN OF THE THIRTEEN FILES AND IS NOT A STATUS LINE
# IN THE OTHER TWO, so this reads line 4 and then checks what it actually got.
# 09-flags-and-bands.md carries `**Supersedes:**` there and docs/methodology/README.md
# carries `**In sync with:**`; neither file has a `**Status:**` line anywhere. Printing
# line 4 verbatim for all thirteen would have reported "Supersedes: PRD sections 5.1
# and 5.2" as that file's status, which is the failure mode this whole script exists
# to catch, so a file with no status line is reported as having none rather than
# having whatever line 4 happens to hold.
#
# A MISSING STATUS LINE IS INFO AND NOT A FAILURE. Only WORKSHEET fails, which is the
# instruction, and inventing a second failing condition would be this check deciding a
# documentation convention that nobody has written down. It is surfaced because Al may
# want to decide it, not because this script has.
#
# `partial` IS DELIBERATELY NOT FLAGGED. 06-oracle-resilience.md is partial because the
# VWAP window length is an open question against a third party, which is handoff item 6
# and is honest rather than unfinished. A check that failed on it would be demanding a
# certainty the document is right not to claim.
status_line(){ sed -n '4p' "$1" 2>/dev/null; }
status_text(){
  local l; l=$(status_line "$1")
  case $l in
    '**Status:**'*) printf '%s' "${l#'**Status:** '}" ;;
    *)              printf 'NO STATUS LINE, line 4 reads: %s' "$l" ;;
  esac
}
worksheet_files(){
  local f
  for f in docs/methodology/*.md; do
    status_line "$f" | grep -qiE '\*\*Status:\*\*.*WORKSHEET' && printf '%s\n' "$(basename "$f")"
  done
  return 0
}
methodology_has_worksheet(){ [ -n "$(worksheet_files)" ]; }
check P2-15 "A methodology file is still a WORKSHEET, so its definitions are unmade" \
  methodology_has_worksheet
for mf in docs/methodology/*.md; do
  mtext=$(status_text "$mf")
  case $(status_line "$mf") in
    *[Ww][Oo][Rr][Kk][Ss][Hh][Ee][Ee][Tt]*)
      printf "%s        FAIL  %-34s %s%s\n" "$red" "$(basename "$mf")" "$mtext" "$off" ;;
    *)
      printf "        INFO  %-34s %s\n" "$(basename "$mf")" "$mtext" ;;
  esac
done
echo "       partial is not a failure: 06-oracle-resilience.md is open against Reflector, handoff item 6"

# ---- P2-18: Layer 2 of the validation protocol has no fixtures ----
#
# ADDED 28 AUGUST 2026, alongside the harness in internal/conformance/layer2.go. The
# harness reports 0 of 10 as ten SKIPPED subtests, and a skip is easy to stop noticing,
# which is the whole reason this check exists beside it rather than instead of it.
#
# NEITHER SIDE OF THIS CHECK CAN BE EDITED TO SATISFY IT, which is the property four
# earlier checks in this file lacked and were bitten for. The required count is read out
# of the PROTOCOL, section 2 of 10-validation.md, and the actual count is read off DISK.
# Writing a fixture closes it. Rewording either document does not, and deleting the
# harness does not either, because the protocol still asks for ten.
layer2_required(){
  # "Sample size | 10 scenarios" in the Layer 2 property table. Matched on the words
  # rather than on the row position, so reordering that table cannot make this read 0.
  awk '/^## 2\. Layer 2/{f=1} f && /Sample size/{ for(i=1;i<=NF;i++) if($i ~ /^[0-9]+$/){print $i; exit} } /^## 3\./{if(f) exit}' \
    docs/methodology/10-validation.md
}
layer2_present(){ ls docs/../testdata/fixtures/layer2/*.json 2>/dev/null | wc -l | tr -d ' '; }
layer2_with_numbers(){ grep -l '"expected"' testdata/fixtures/layer2/*.json 2>/dev/null | wc -l | tr -d ' '; }
layer2_incomplete(){
  local req act
  req=$(layer2_required); act=$(layer2_present)
  [ -n "$req" ] || return 0          # protocol unreadable is reported, not passed
  [ "$act" -lt "$req" ]
}
check P2-18 "Layer 2 of the validation protocol has fewer fixtures than it requires" \
  layer2_incomplete
l2req=$(layer2_required); l2act=$(layer2_present); l2num=$(layer2_with_numbers)
printf "        %s%s of %s scenario(s) present, %s of those carry hand computed expectations%s\n" \
  "$dim" "${l2act:-0}" "${l2req:-?}" "${l2num:-0}" "$off"
if [ "${l2act:-0}" -lt "${l2req:-0}" ]; then
  echo "       missing, by the slug the harness looks for:"
  # The slugs come from the harness, so a scenario renamed there is renamed here too.
  grep -o 'Slug: "[a-z0-9-]*"' internal/conformance/layer2.go | sed 's/.*"\(.*\)"/\1/' \
  | while read -r slug; do
      [ -n "$slug" ] || continue
      ls testdata/fixtures/layer2/*-"$slug".json >/dev/null 2>&1 || printf "        %s\n" "$slug"
    done
fi
echo "       testdata/fixtures/layer2/ is RED. Al creates each state on testnet, records the"
echo "       transaction that created it, and works the expected values by hand"

section "Methodology and contract agreement"
# ADDED 27 AUGUST 2026, extending the methodology section above. That section asks
# whether the methodology says anything yet. This one asks whether it and the contract
# say the SAME thing, which is a different question and the one that bit twice on
# 26 August: two documents each internally consistent, disagreeing with each other, and
# nothing in this repository failing.
#
# WHY THIS IS NOT ONE MORE `grep -qF` PAIR. Every earlier check of this shape anchors on
# the wrong side existing, and this file has been bitten four times by anchoring on prose
# that the fix itself rewrites. Both checks below read the RULE out of each document,
# normalise it, and fail while the two normalised rules differ. Neither one knows which
# document is right, and neither one can be satisfied by editing only the side the check
# was written against. Whichever side moves, the check closes.
#
# THIS SECTION ONLY READS. docs/methodology/ is RED. docs/api/ is YELLOW and neither
# check writes to it.

# ---- P2-16: when is maxReachablePrice null ----
#
# 05-manipulation-cost.md section 5 keys the null on a pool being PRESENT. The contract
# keys it on the pool being the ONLY venue. On a market with a book and a pool the two
# give opposite answers, and the USTRY fixture is that market, which is why this sits in
# DEC-006 rather than on its own.
#
# The heading is matched on its subject rather than on "## 5.", so renumbering the
# methodology file cannot make the finding vanish. The contract block is matched on the
# property key at its own indent and ends at the next sibling key, so a reworded
# description is still read rather than missed.
mrp_methodology_block(){
  awk '/^#+ /{ if (f) exit; if (tolower($0) ~ /maximum reachable price/) f=1; next } f' \
    docs/methodology/05-manipulation-cost.md
}
mrp_contract_block(){
  awk '/^        maxReachablePrice:[[:space:]]*$/{f=1; next} f && /^        [A-Za-z]/{exit} f' \
    docs/api/keel-openapi.yaml
}
# Normalises a null rule to one word. `presence` means the rule fires because a pool
# exists at all; `exclusivity` means it fires only when the pool is the whole market.
# Anything else is reported as it is rather than guessed at, because a rule this check
# cannot read is not a rule this check may declare agreed.
#
# THE TEXT IS JOINED INTO ONE LINE FIRST, and that was not a tidy-up. The contract wraps
# "or all the / liquidity comes from an AMM" across two lines, so a per-line grep read the
# contract rule as unreadable and the check would have reported a disagreement it could
# only see one side of. A rule that spans a line break is still the rule.
mrp_rule(){
  local t p=0 x=0
  t=$(tr '\n' ' ' | tr -s ' ')
  printf '%s\n' "$t" | grep -qiE 'pool is present|a pool exists|pure order ?book' && p=1
  printf '%s\n' "$t" | grep -qiE 'all (of )?(the )?liquidity comes from an amm|liquidity comes only from an amm|only venue' && x=1
  if [ "$p" = 1 ] && [ "$x" = 1 ]; then printf 'ambiguous'
  elif [ "$p" = 1 ]; then printf 'presence'
  elif [ "$x" = 1 ]; then printf 'exclusivity'
  else printf 'unreadable'
  fi
}
mrp_rules_disagree(){
  local m c
  m=$(mrp_methodology_block | mrp_rule)
  c=$(mrp_contract_block | mrp_rule)
  case $m in presence|exclusivity) ;; *) return 0 ;; esac
  case $c in presence|exclusivity) ;; *) return 0 ;; esac
  [ "$m" != "$c" ]
}
check P2-16 "The methodology and the contract disagree about when maxReachablePrice is null" \
  mrp_rules_disagree
printf "        %smethodology%s 05-manipulation-cost.md keys the null on %s%s%s\n" \
  "$dim" "$off" "$bold" "$(mrp_methodology_block | mrp_rule)" "$off"
printf "        %scontract%s    keel-openapi.yaml keys the null on %s%s%s\n" \
  "$dim" "$off" "$bold" "$(mrp_contract_block | mrp_rule)" "$off"
# The same disagreement expressed in DATA rather than in prose, printed as supporting
# evidence and not as a second condition. An example that carries a pool spot price and
# a non-null maxReachablePrice has taken the contract's side of the conflict, whatever
# either description says.
mrp_examples_with_pool(){
  awk '
    /^    [A-Za-z][A-Za-z0-9]*:[[:space:]]*$/ { name=$1; sub(":","",name); pool=""; mrp="" }
    /^        poolSpotPrice:/ { pool=$2 }
    /^        maxReachablePrice:/ { mrp=$2
      if (pool != "" && pool != "null" && mrp != "null")
        printf "%s  poolSpotPrice %s  maxReachablePrice %s\n", name, pool, mrp
    }
  ' docs/api/keel-openapi.yaml
}
mrp_examples_with_pool | while read -r line; do
  printf "        %sboth venues, non-null:%s %s\n" "$dim" "$off" "$line"
done

# ---- P2-17: how long is the oracle window ----
#
# The /methodology example is what scripts/generate-api-mocks.sh writes into
# docs/api/mocks/methodology.json, so a wrong number there is the number a frontend
# builds against. The methodology states the window in MINUTES and the contract in
# SECONDS, so both sides are read and converted rather than compared as strings.
#
# THIS CHECK DOES NOT SAY WHICH IS RIGHT, and it must not. The window length is open
# against Reflector, handoff item 6, and 06-oracle-resilience.md is honest to call it an
# assumption. The finding is the DISAGREEMENT, so the check fails in either direction and
# closes whichever way that question lands.
contract_window_seconds(){
  grep -oE '^[[:space:]]+oracleWindowSeconds:[[:space:]]+[0-9]+' docs/api/keel-openapi.yaml \
    | grep -oE '[0-9]+$' | sort -u
}
methodology_window_seconds(){
  grep -rhoiE '[0-9]+[ -]minute default' docs/methodology/ \
    | grep -oE '^[0-9]+' | sort -un | while read -r m; do echo $((m * 60)); done
}
one_value(){ [ "$(printf '%s\n' "$1" | grep -c .)" = 1 ] && [ -n "$1" ]; }
oracle_window_disagrees(){
  local c m
  c=$(contract_window_seconds); m=$(methodology_window_seconds)
  one_value "$c" || return 0
  one_value "$m" || return 0
  [ "$c" != "$m" ]
}
check P2-17 "The /methodology example and the methodology state different oracle window lengths" \
  oracle_window_disagrees
printf "        %scontract%s    oracleWindowSeconds in the example: %s%s%s seconds\n" \
  "$dim" "$off" "$bold" "$(contract_window_seconds | paste -sd, -)" "$off"
printf "        %smethodology%s the stated default window:         %s%s%s seconds\n" \
  "$dim" "$off" "$bold" "$(methodology_window_seconds | paste -sd, -)" "$off"
printf "        %sthe example is what docs/api/mocks/methodology.json is generated from, so it is what a frontend builds against%s\n" \
  "$dim" "$off"
# The same number is repeated in the asset examples, and those are generated too. Counted
# rather than listed, because the count is what says whether this is one edit or several.
win_repeats=$(grep -cE '^[[:space:]]+windowSeconds:[[:space:]]+[0-9]+' docs/api/keel-openapi.yaml || true)
win_prose=$(grep -coE '[0-9]+ second oracle window' docs/api/keel-openapi.yaml || true)
printf "        %s%s oracleResistance.windowSeconds example values and %s prose mentions carry the same figure%s\n" \
  "$dim" "$win_repeats" "$win_prose" "$off"

section "Golden fixture arithmetic, recomputed from scratch"
python3 - <<'PY'
import os
from decimal import Decimal, getcontext
getcontext().prec = 60
GREEN = os.environ.get("AUDIT_GREEN", "")
RED = os.environ.get("AUDIT_RED", "")
OFF = os.environ.get("AUDIT_OFF", "")
ask = Decimal(266843207) / Decimal(2500000)
bid = Decimal(1057) / Decimal(1000)
amt = Decimal("1.2185312")
p0 = (ask + bid) / 2
spread = (ask - bid) / p0 * 100
cost = ask * amt
def compare(name, computed, written, tol=Decimal("0.0000001")):
    ok = abs(computed - Decimal(written)) <= tol
    tag = f"{GREEN}MATCH {OFF}" if ok else f"{RED}DIFFER{OFF}"
    print(f"{tag} {name:<26} computed {computed:<24} fixture {written}")
compare("P0", p0, "53.8971414")
compare("spreadPct", spread, "196.0777140585048")
compare("cost of the single ask", cost, "130.06270929502336")
compare("target delta 0.5", p0 * Decimal("1.5"), "80.8457121")
compare("target delta 1", p0 * 2, "107.7942828")
compare("target delta 10", p0 * 11, "592.8685554")
compare("target delta 100", p0 * 101, "5443.6112814")
compare("maxReachablePrice", ask, "106.7372828")
print(f"       buy targets at 2/5/10 percent   {p0*Decimal('1.02')} {p0*Decimal('1.05')} {p0*Decimal('1.10')}")
print(f"       sell targets at 2/5/10 percent  {p0*Decimal('0.98')} {p0*Decimal('0.95')} {p0*Decimal('0.90')}")
print("       the only ask at 106.7372828 sits outside every buy target, the only bid at 1.057 outside every sell target")
print("       which is why depth is zero in six cells, and that zero is correct")

# TWO INPUTS ABOVE HAVE NO LEDGER PROVENANCE, established 25 August 2026, and they
# are printed rather than silently corrected because both are red zone numbers.
#
# The bid of 1.057 appears in no Horizon response. The offers endpoint returns four
# surviving bids on this pair, at 0.0002778, 0.0002778, 0.0053 and 0.0189 USDC per
# USTRY, none of them near it. The nearest real figure is offer 1823025211 at about
# 1.0574630, recovered from a trade AFTER the fixture ledger: its id is lower than
# the manipulation offer's and offer ids are globally monotonic and never reused, so
# it was resting in the book at the fixture ledger. 1.057 looks like a rounding of
# that, and P0 = 53.8971414 is derived from it.
#
# The amount of 1.2185312 is the offer BEFORE the manipulation trade. The offer
# holds 1.1684309 today, unchanged since ledger 61340263, and the difference is
# exactly 0.0501003, the USTRY that changed hands in the manipulation. Al pinned the
# fixture to ledger 61340408, which is after that trade, so the fixture ledger and
# this amount disagree by one trade.
delta = Decimal("1.2185312") - Decimal("1.1684309")
print()
print(f"{RED}       PROVENANCE   bid 1.057 is not a Horizon reading. Nearest on-ledger figure: 1.0574630, offer 1823025211{OFF}")
print(f"{RED}       PROVENANCE   amt 1.2185312 is the pre-manipulation size; the offer holds 1.1684309 at ledger 61340408{OFF}")
print(f"       the difference is {delta}, the exact USTRY volume of the manipulation trade")
print("       both are red zone numbers. This block reports the disagreement and does not resolve it")
PY

section "What the attacker actually paid to manipulate the price"
printf "%s" "$dim"
grep -A3 '"ledger_close_time": "2026-02-22T00:10:21Z"' docs/evidences/spike_result_final.txt \
  | grep -E 'base_amount|counter_amount' || true
printf "%s" "$off"
echo "       5.3475699 USDC exchanged for 0.0501003 USTRY at 106.7372828. This is the headline number of Deliverable 2."
echo "       Against roughly 10.97 million dollars borrowed, the ratio is about 1 to 2.05 million."
echo "       Not 1 to 22 million as DEC-001 states."

section "Manipulation cost stated per venue, DEC-009"

# ---- P2-19: section 1 of the owning file still defines MC over asks alone ----
#
# ADDED 31 AUGUST 2026 alongside DEC-009.
#
# WHAT THIS CHECKS, AND WHAT IT CANNOT. It cannot verify the rule is correct. Only
# a human can, and that is the whole reason section 1 of that file is RED. What it
# reports is whether the rule was generalised AT ALL, because the gap is silent:
# 05-manipulation-cost.md section 1 defines MC and Reachable over asks, section 3
# of the same file introduces two venue forms, compute.go implements both, and
# nothing in the build fails while the definition covers one of them. A gap that
# fails nothing is the gap this repository keeps rediscovering.
#
# This is a TRIPWIRE. It is expected to read PROVEN on the commit that adds it and
# to flip to NOT once Al has written section 1. Do not reword the section to
# satisfy it; write the rule.
#
# Assertion (a) is deliberately weak. It cannot be strengthened without this
# script prescribing the wording of a RED section, which is the zone model
# defeated by a grep.
mc_section_one(){
  awk '/^## 1\./{f=1;next} /^## /{f=0} f' docs/methodology/05-manipulation-cost.md
}
mc_section_one_asks_only(){
  [ -f docs/methodology/05-manipulation-cost.md ] || return 0
  local s; s="$(mc_section_one)"
  [ -n "$s" ] || return 0            # heading renamed is reported, not passed
  # (a) no venue other than the order book named, so the combined form has no
  # written definition. (b) Reachable's active-pool consequence is not stated
  # here, and it currently lives only in DEC-003 (API contract v1.1) section 4.3,
  # in compute.go and in a test, none of which is the owning file.
  ! printf '%s' "$s" | grep -Eqi '(liquidity pool|\bpool\b|\bAMM\b|\bvenue\b)'
}
check P2-19 "section 1 of 05-manipulation-cost.md defines manipulation cost over asks alone, while section 3 and compute.go carry two venue forms" \
  mc_section_one_asks_only
if [ -f docs/methodology/05-manipulation-cost.md ]; then
  s1="$(mc_section_one)"
  printf "        %ssection 1 names a second venue: %s. states Reachable under an active pool: %s%s\n" \
    "$dim" \
    "$(printf '%s' "$s1" | grep -Eqi '(liquidity pool|\bpool\b|\bAMM\b|\bvenue\b)' && echo yes || echo no)" \
    "$(printf '%s' "$s1" | grep -Eqi 'reachable' && printf '%s' "$s1" | grep -Eqi '(liquidity pool|\bpool\b|\bAMM\b)' && echo yes || echo no)" \
    "$off"
  echo "       DEC-009 section 2 records the reading Al chose, reading A. The wording of the rule is his"
fi

# ---- P2-20: a pool-only manipulation ladder has appeared in a stored position ----
#
# WHY THIS REPORTS RATHER THAN PROHIBITS. Whether combined minus orderbookOnly IS
# the pool term by definition is OPEN. DEC-006 section 8 asks it as handoff item 17
# and DEC-009 section 9 item 1 hands it back to Al explicitly, saying the identity
# would be definitional under reading A and that this record does not decide it.
# So this line does not forbid a third ladder. It reports one arriving, because if
# the identity is definitional then a stored pool-only figure is a third home for a
# fact already stored twice, and this repository has lost to the second-home
# pattern more often than to any other. Al reads the line and decides.
#
# WHAT IT MATCHES. Field names, column names and schema properties only, in the
# three places a third ladder would actually have to appear. Prose in docs/ is NOT
# matched: discussing the identity is the opposite of the thing being watched for,
# and a check that fired on its own decision record would be silenced within a week
# and then trusted anyway.
#
# WHAT IT CANNOT DO. It cannot see the same quantity arriving under a name nobody
# thought to grep for. It watches the obvious door, not every door.
pool_only_ladder_targets(){
  local f
  for f in internal/domain/types.go docs/api/keel-openapi.yaml; do
    [ -f "$f" ] && printf '%s\n' "$f"
  done
  [ -d migrations ] && find migrations -name '*.sql' -type f | sort
}
pool_only_ladder_stored(){
  local -a files=()
  while IFS= read -r f; do [ -n "$f" ] && files+=("$f"); done < <(pool_only_ladder_targets)
  # No readable target means this line is verifying nothing, which is reported as
  # the finding standing rather than quietly as a pass.
  [ "${#files[@]}" -gt 0 ] || return 0
  grep -REqn \
    -e 'manipulation[_-]?[Cc]ost[_-]?[Pp]ool[_-]?[Oo]nly' \
    -e '[Pp]ool[_-]?[Oo]nly[_-]?([Cc]ost|[Ll]adder)' \
    "${files[@]}"
}
check P2-20 "a pool-only manipulation cost ladder is stored in types, contract or migrations, which is a third home for the pool term" \
  pool_only_ladder_stored
printf "        %sread: %s%s\n" "$dim" "$(pool_only_ladder_targets | tr '\n' ' ')" "$off"
echo "       the identity that decides whether this is a defect is DEC-006 section 8, handed back to Al in DEC-009 section 9 item 1"

# ---- P2-21: the delay behind the 23 PARTIAL rows is not recorded on any row ----
#
# WHAT THIS IS ABOUT. The first Layer 3 run returned 37 MATCH, 0 MISMATCH and 23
# PARTIAL, and all 23 are attributed to the seven hours between the recording and
# the rebuild. That attribution is written down as settled in three places:
# 10-validation.md section 3, docs/evidences/2026-08-26-layer3-crosscheck.md
# section 4, and the README. It is a HYPOTHESIS. Every row in that run had the same
# delay, so the run contains no contrast that could test it, and no row recorded
# what its delay was, so a second run at a different delay could not have been
# compared against it either.
#
# The finding is therefore not "the attribution is wrong". It is that the variable
# it names was not stored, which is what makes it untestable. This line flips to
# NOT once a comparison row carries its own elapsed time.
#
# IT MATCHES THE COLUMN AND NOT A FILE NAME, so moving the comparison to another
# file in cmd/keel does not make the finding disappear.
crosscheck_row_has_no_elapsed(){ ! grep -rq 'elapsed_seconds' cmd/keel; }
check P2-21 "no Layer 3 comparison row records the gap between the recording and the rebuild, so the cause the 23 PARTIAL rows are attributed to is not stored anywhere in the output" \
  crosscheck_row_has_no_elapsed
echo "       what it cannot prove: that the number in that column was measured from the right instant. It proves the column exists"

# ---- P2-22: the delay under test is a magic number ----
#
# The same-hour pairing has exactly one independent variable and this is it. A
# delay written as a literal at the flag site is an experiment whose variable is not
# named anywhere, which is how a run gets reported without the setting it was run
# at; the whole defect above is one sentence quoting a rate with no delay beside it.
#
# Two constants, because a default alone does not make "the same hour" checkable.
# maxCrosscheckDelay is what refuses a delay that would put the comparison outside
# the hour the recording was taken in, and without it the phrase is a promise.
samehour_delay_unnamed(){
  ! { grep -rq 'defaultCrosscheckDelay' cmd/keel && grep -rq 'maxCrosscheckDelay' cmd/keel; }
}
check P2-22 "the same-hour crosscheck delay is a literal rather than a named default with a ceiling, so the one variable the experiment moves is not named in the code" \
  samehour_delay_unnamed
if [ -d cmd/keel ]; then
  printf "        %s%s%s\n" "$dim" \
    "$(grep -rhE '^const (default|max)CrosscheckDelay' cmd/keel | tr '\n' ' ' | sed 's/  */ /g')" "$off"
fi
echo "       same limitation as P2-13: it proves the constants exist, never that a run used them"

section "Layer 1 hand recomputation, acceptance criterion 4"

# ---- P2-23: Layer 1 has no hand recomputations, and the directory is absent ----
#
# ADDED 31 AUGUST 2026. This is the check C-20 promised and did not deliver: that
# item added P2-18 for Layer 2, said in its own status line that the
# testdata/manual/ check was not written, and left it there.
#
# WHY IT MATTERS MORE THAN ITS TEN POINTS, and the reason is NOT the one this
# comment used to give. Layer 1 is the only layer that asks whether the FORMULA is
# correct; Layers 2 and 3 both ask whether the implementation matches the formula,
# so if the formula is wrong they agree with each other and are both wrong.
# 10-validation.md section 5 says this in its own words. With compute.go yellow
# since 25 August 2026, these five recomputations and the golden fixture are the
# whole of the independent oracle.
#
# CORRECTED 3 SEPTEMBER 2026. This paragraph also said "PRD section 12 lists it
# among the four that are never cuttable". It does not. That list is the Blend
# backtest, the methodology document, the cross-validation, and the limitations
# section; the cross-validation is Layer 3 and Layer 1 is on no such list.
#
# The argument above never needed it and stands unchanged, which is the point of
# separating them: a claim about a document was propping up a claim about the
# validation protocol, and only one of the two was true. Section 12 is what governs
# a decision made under time pressure, so the honest reading is that Layer 1 is
# MORE exposed than this comment implied, not less. See also the same wrong sentence
# in scripts/check-manual-recomputation.sh and in the Makefile, corrected the same
# day, and DEC-002 section 8 for the other PRD paraphrase found wrong that day.
#
# THE ID IS P2-23 AND P2-3 IS STILL FREE, deliberately. Other documents cite these
# ids by number, so a gap that is backfilled makes every earlier citation ambiguous
# about which check it meant. The numbering appends.
#
# WHY THIS DELEGATES INSTEAD OF GREPPING. The count lives in
# scripts/check-manual-recomputation.sh, which is also what CI runs and what
# `make manual-check` runs. One fact, one home. A second copy of the matcher here
# is how P2-18's shape would have drifted from the harness beside it, and this
# repository has lost to the second-home pattern more often than to any other.
#
# NEITHER SIDE CAN BE EDITED TO SATISFY IT. That script reads the required sample
# size out of the protocol and the actual count off disk, so it holds no expected
# value that could be lowered. See its header.
manual_layer1_report=$(bash scripts/check-manual-recomputation.sh 2>&1)
manual_layer1_rc=$?
manual_layer1_incomplete(){ [ "$manual_layer1_rc" -ne 0 ]; }
check P2-23 "Layer 1 of the validation protocol has fewer hand recomputations than it requires, or one of them names no asset and no ledger sequence" \
  manual_layer1_incomplete
# Exit 2 means the script could not read its own requirement. That is reported as
# the finding STANDING rather than as a pass, the same way P2-20 treats an
# unreadable target: a check that verified nothing must not read as reassurance.
if [ "$manual_layer1_rc" -eq 2 ]; then
  printf "        %sthe protocol could not be read, so this line reports the finding standing rather than passing%s\n" "$red" "$off"
else
  printf "        %s%s%s\n" "$dim" \
    "$(printf '%s\n' "$manual_layer1_report" | grep -E '^[0-9]+ of [0-9]+ present' | head -1)" "$off"
  printf '%s\n' "$manual_layer1_report" | grep -E '^  [a-zA-Z0-9._-]+ +(yes|no)' \
    | while IFS= read -r row; do printf "      %s%s%s\n" "$dim" "$row" "$off"; done
fi
echo "       testdata/manual/ is RED. Al works each recomputation by hand from a raw book, and the"
echo "       required shape of a file is specified in testdata/manual/README.md"
echo "       what it cannot prove: that a figure inside one of those files is correct, or that it was"
echo "       computed independently of compute.go. It proves the evidence exists and says what it is about"

# ---- P2-24: a committed trades CSV carries no proof that its window was whole ----
#
# ADDED 1 SEPTEMBER 2026, and it is the third mechanism aimed at one defect class
# because the first two both act at a moment that leaves no trace in the
# repository. DEC-010 section 1 refuses a window that has not closed, which
# catches the read taken at 2026-08-31T16:09Z. cmd/keel/backtest.go warns on
# stderr when the walk never saw past the window end, which is the read taken at
# 2026-09-01T04:20Z: closed for four hours, 104 trades short, because Horizon's
# index had not caught up. Neither one asks the question this check asks, which is
# whether the file that ENDED UP IN GIT is the complete one.
#
# THE CASE IT EXISTS FOR IS stopped_past_window FALSE ON A CLOSED WINDOW. That is
# the hardest shape to dismiss a file over, because every other property of it is
# right: the window is legitimate, the rows are real, the columns are correct, the
# clock check passes. What is wrong is only what is absent, and absence has no
# signature. The field reported it correctly on 2026-09-01 and two reviewers in
# sequence called it harmless.
#
# WHY THE AUDIT AND NOT A THIRD WARNING. The two existing mechanisms fire once,
# into a terminal, at the moment of the run. This repository's record of that
# mechanism is one dismissal per reader. The audit is read against the repository
# as it stands, at any later time, by anyone, and a file cannot scroll off the top
# of it. It also covers files that PREDATE both mechanisms, which is the whole of
# docs/evidences/ today and is exactly where the incomplete August read is.
#
# IT ASKS git ls-files AND NOT THE FILESYSTEM. A sidecar that is on disk and not
# in the repository proves nothing to a clone, and a clone is what a reader of the
# paid deliverable has. Same reasoning as P2-9, and the opposite failure: there it
# cost the check a directory, here it would credit a file with provenance nobody
# else can see.
evidence_report=""
evidence_unproven=0
evidence_unproven_count=0
evidence_csv_count=0
while IFS= read -r evidence_csv; do
  [ -n "$evidence_csv" ] || continue
  evidence_csv_count=$((evidence_csv_count + 1))
  evidence_meta="${evidence_csv%.csv}.meta.txt"
  if ! git ls-files --error-unmatch "$evidence_meta" >/dev/null 2>&1; then
    evidence_verdict="NO SIDECAR in the repository, so the file makes no coverage claim at all"
    evidence_unproven=1; evidence_unproven_count=$((evidence_unproven_count + 1))
  elif grep -q '^stopped_past_window: true$' "$evidence_meta"; then
    evidence_verdict="stopped_past_window: true, the walk saw past the window end"
  else
    evidence_verdict="stopped_past_window is NOT true, so this row count is a floor and not a total"
    evidence_unproven=1; evidence_unproven_count=$((evidence_unproven_count + 1))
  fi
  evidence_report="${evidence_report}  $(basename "$evidence_csv")
    ${evidence_verdict}
"
done < <(git ls-files 'docs/evidences/*-trades-*.csv')
evidence_window_unproven() { [ "$evidence_unproven" -eq 1 ]; }
check P2-24 "a trades CSV in the repository carries no sidecar, or one that does not prove the walk reached its window end, so a file named for a whole window may hold part of one" \
  evidence_window_unproven
# Zero matches is reported rather than passed. A check that examined nothing must
# not read as reassurance, the same way P2-23 treats an unreadable protocol.
if [ "$evidence_csv_count" -eq 0 ]; then
  printf "        %sno trades CSV is tracked under docs/evidences, so this line examined nothing%s\n" "$red" "$off"
else
  printf "        %s%d tracked trades CSV, %d without proof of a whole window%s\n" "$dim" \
    "$evidence_csv_count" "$evidence_unproven_count" "$off"
  printf '%s' "$evidence_report" | while IFS= read -r evidence_row; do
    printf "      %s%s%s\n" "$dim" "$evidence_row" "$off"
  done
fi
echo "       the sidecar is written by cmd/keel/backtest.go beside every CSV it produces, and every field"
echo "       in it is derived from the records rather than the clock, so a re-read of a closed window"
echo "       produces identical bytes. See DEC-010 section 5"
echo "       what it cannot prove: that a CSV whose sidecar says true is the file that sidecar describes."
echo "       Nothing binds the two beyond a shared basename, and a hand-edited recording is out of scope"
echo "       here rather than covered. scripts/s3-archive/verify-manifest.sh is the check that binds bytes"

# ---------------------------------------------------------------------------
# P2-25 and P2-26. Decision record numbering.
#
# This pair was written after the defect it describes had already fired twice and
# been caught by neither the harness nor a reader. DEC-003 is used by two records,
# which DEC-009 section 9 item 3 records. DEC-011 was then used by two more on the
# same day, 1 September 2026, and the second one overwrote the first ON DISK before
# it was ever committed, so git holds no copy of it. It exists today only because a
# session transcript had read it in full, and it is in the repository as
# DEC-0XX-...RECONSTRUCTED.md with its number deliberately left unassigned.
#
# THE COST OF THE MISSING CHECK IS THE WHOLE POINT. A collision is not a filing
# annoyance. The second one destroyed a record. Writing the check is under an hour
# and it was not written after the first collision, which is why there was a second.
#
# WHY IT READS THE HEADING AND NOT ONLY THE FILENAME, and this is what makes it
# catch the second collision rather than only the first. Renaming a file moves the
# number in the filename and leaves the number in the `# DEC-NNN:` heading where it
# was. That is exactly the state the repository is in now: the reconstructed record
# is FILED as 0XX and still CLAIMS DEC-011 in its own first line, while an accepted
# record of a different subject holds DEC-011 too. A check that read filenames
# alone would report the second collision resolved. It is not resolved, it is
# renamed, and the two are different things.
#
# WHAT NEITHER LINE CAN PROVE. That the number a record claims is the RIGHT one.
# Both are satisfied by renumbering, and renumbering without deciding which record
# owns the number is the failure this file has been defeated by five times in other
# forms. The fix is Al's: DEC-013 is free, and section 6 of the 2 September
# breakdown asks for that decision rather than for a rename.
dec_claims=""
dec_mismatch_report=""
dec_placeholder_report=""
dec_file_count=0
dec_mismatch_count=0
dec_placeholder_count=0
while IFS= read -r dec_file; do
  [ -n "$dec_file" ] || continue
  dec_file_count=$((dec_file_count + 1))
  dec_base=$(basename "$dec_file")
  dec_fname_num=$(printf '%s' "$dec_base" | sed -n 's/^DEC-\([^-]*\)-.*/\1/p')
  dec_head_num=$(sed -n 's/^# DEC-\([0-9A-Za-z]*\)[:[:space:]].*/\1/p' "$dec_file" | head -1)

  [ -n "$dec_fname_num" ] && dec_claims="${dec_claims}${dec_fname_num}	${dec_base}
"
  if [ -n "$dec_head_num" ] && [ "$dec_head_num" != "$dec_fname_num" ]; then
    dec_claims="${dec_claims}${dec_head_num}	${dec_base}
"
    dec_mismatch_count=$((dec_mismatch_count + 1))
    dec_mismatch_report="${dec_mismatch_report}  ${dec_base}
    filed as DEC-${dec_fname_num}, claims DEC-${dec_head_num} in its own heading
"
  fi
  case "$dec_fname_num" in
    ''|*[!0-9]*)
      dec_placeholder_count=$((dec_placeholder_count + 1))
      dec_placeholder_report="${dec_placeholder_report}  ${dec_base}
    filed under the placeholder DEC-${dec_fname_num}, so it is a record no decision can be cited as
"
      ;;
  esac
done < <(git ls-files 'docs/decisions/DEC-*.md')

# A number is collided when two DIFFERENT files claim it. The sort -u first is
# what stops a file whose filename and heading agree from colliding with itself.
dec_collisions=$(printf '%s' "$dec_claims" | sort -u | cut -f1 | sort | uniq -d)
dec_collision_count=$(printf '%s' "$dec_collisions" | grep -c '[^[:space:]]')

dec_number_collision() { [ "$dec_collision_count" -gt 0 ]; }
dec_number_unfiled()   { [ "$dec_mismatch_count" -gt 0 ] || [ "$dec_placeholder_count" -gt 0 ]; }

check P2-25 "two decision records claim the same number, counting the filename and the record's own heading, so citing that number names two documents" \
  dec_number_collision
if [ "$dec_file_count" -eq 0 ]; then
  printf "        %sno decision record is tracked under docs/decisions, so this line examined nothing%s\n" "$red" "$off"
else
  printf "        %s%d tracked records, %d number(s) claimed by more than one%s\n" "$dim" \
    "$dec_file_count" "$dec_collision_count" "$off"
  printf '%s\n' "$dec_collisions" | while IFS= read -r dec_num; do
    [ -n "$dec_num" ] || continue
    printf "      %sDEC-%s is claimed by:%s\n" "$dim" "$dec_num" "$off"
    printf '%s' "$dec_claims" | sort -u | awk -F'\t' -v n="$dec_num" '$1 == n { print "        " $2 }'
  done
fi
echo "       DEC-009 section 9 item 3 records the first collision. The second destroyed a record on"
echo "       disk before it was committed, and it survives only as a reconstruction from a transcript"
echo "       what it cannot prove: that the number a record claims is the right one. Renumbering"
echo "       satisfies this line, and renumbering without deciding which record owns the number is"
echo "       the pattern this file has been defeated by before. DEC-013 is free and the call is Al's"

check P2-26 "a decision record is filed under a number its own heading contradicts, or under a placeholder, so the filename and the record disagree about what it is" \
  dec_number_unfiled
if [ "$dec_mismatch_count" -eq 0 ] && [ "$dec_placeholder_count" -eq 0 ]; then
  printf "        %severy tracked record's filename number matches the number in its heading%s\n" "$dim" "$off"
else
  printf "        %s%d filename/heading disagreement(s), %d placeholder(s)%s\n" "$dim" \
    "$dec_mismatch_count" "$dec_placeholder_count" "$off"
  printf '%s%s' "$dec_mismatch_report" "$dec_placeholder_report" | while IFS= read -r dec_row; do
    [ -n "$dec_row" ] || continue
    printf "      %s%s%s\n" "$dim" "$dec_row" "$off"
  done
fi
echo "       this is the line that separates renamed from resolved. A collision cleared by moving the"
echo "       filename and leaving the heading is still a collision, and P2-25 alone would call it fixed"

# ---------------------------------------------------------------------------
# P2-27 and P2-28. The PRD, and the directory it is filed in.
#
# docs/api/Keel_PRD.md holds the acceptance criteria in section 9, the functional
# requirements in section 4, and the scope cutting order in section 12. It is an
# INPUT FROM OUTSIDE in exactly the sense the zone map means when it makes
# docs/context/ red and enforced: it is the document the work is scored against,
# and Claude is one of the two things it scores.
#
# It is filed in docs/api/, which the zone map makes YELLOW and describes as "the
# contract". The PRD is not the contract. So until 3 September 2026 the criteria
# that measure Claude's work sat in a directory Claude may write, and nothing in
# either permission file said otherwise.
#
# THIS IS THE SAME DEFECT CLASS THE MAP CLOSED ON 24 AUGUST 2026, when fourteen
# directories including .claude/ itself had no row, so the file that defined the
# zones was outside the zones. Here the file that defines DONE was inside a
# writable one. Neither was exploited and neither needed to be: the map's own note
# says a path with no owner is a path where nobody agreed what may be written.
#
# WHAT THE FIX IS AND IS NOT. Adding a deny rule and a hook path is TIGHTENING, so
# it is Claude's to do, and it was done on 3 September 2026. Moving the file into
# docs/context/ where its class belongs, or giving docs/api/ a second row, is a
# zone decision and is Al's. The pair below is arranged so the second half stays
# PROVEN while only the first half has been done, because a lock the map does not
# mention is the failure this repository has already paid for once in the other
# direction: five references to a retired red zone that would each have gone on
# refusing work the map permitted, and none of which would have failed.
prd_in_deny(){
  awk '/"deny"[[:space:]]*:/{f=1} f{print} f && /\]/{exit}' .claude/settings.json \
    | grep -q 'Keel_PRD\.md'
}
prd_hook_locked(){
  hook_absent && return 1
  hook_refuses 'sed -i "" s/a/b/ docs/api/Keel_PRD.md'
}
# The probe list is the one P2-6e paid for: the ordinary forms, not the exotic
# ones. A redirect carries no mutating verb, and `cat > file` is how a document
# gets rewritten wholesale rather than edited.
prd_writable(){
  prd_in_deny && prd_hook_locked && return 1
  return 0
}
prd_lock_unmapped(){
  prd_hook_locked || return 1
  grep -q 'Keel_PRD\.md' CLAUDE.md && return 1
  return 0
}
check P2-27 "the PRD, which holds the acceptance criteria the work is scored against, is writable by Claude in at least one of the two permission files, so the definition of done sits inside the writable surface" \
  prd_writable
if prd_in_deny; then
  printf "        %sdeny list: locked%s\n" "$dim" "$off"
else
  printf "        %sdeny list: NOT locked, so the Edit and Write tools reach it%s\n" "$red" "$off"
fi
if prd_hook_locked; then
  printf "        %shook: refuses a mutating command that names it%s\n" "$dim" "$off"
else
  printf "        %shook: does NOT refuse it, so Bash reaches it%s\n" "$red" "$off"
fi
echo "       neither file closes the other's route: the deny list does not see Bash, and the hook"
echo "       does not see Edit. Same reasoning as P2-6, and it is why both halves are probed"
echo "       what it cannot prove: that the criteria inside it are the right ones, or that they"
echo "       match the SOW. docs/context/ is not on disk, so no check here can compare the two"

check P2-28 "the harness refuses the PRD while no row in the CLAUDE.md zone map names it, so the lock is invisible to the document people actually read" \
  prd_lock_unmapped
if prd_hook_locked && grep -q 'Keel_PRD\.md' CLAUDE.md; then
  printf "        %sthe map names the file, so the harness and the map agree%s\n" "$dim" "$off"
elif ! prd_hook_locked; then
  printf "        %snothing to map: the harness does not lock it yet, which is P2-27%s\n" "$dim" "$off"
else
  printf "        %sthe harness locks it and the map's docs/api row does not mention it%s\n" "$red" "$off"
fi
echo "       a lock the map does not mention surfaces later as a guardrail misfiring rather than as"
echo "       a document being wrong, and that is what gets guardrails switched off. The hook's own"
echo "       header names that as the worst outcome"
echo "       what it cannot prove: that the row says the right thing. Reclassifying the file, or"
echo "       moving it to where its class belongs, is a zone decision and is Al's"

section "Summary"
printf "  %s%d claims proven%s, %s%d not%s\n" "$green" "$proven" "$off" "$red" "$not" "$off"
echo "  The full audit: docs/internal/audit-2026-08-20.md"
