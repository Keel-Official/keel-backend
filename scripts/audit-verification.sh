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
# recordings/ and scripts/history-migration/ are excluded for the same reason:
# both are gitignored, both exist only on the machine that produced them, and a
# map row for a directory no clone has is a row nobody can check. The second one
# is also the directory the map must NOT name for a different reason, recorded in
# .gitignore: its two files carry the exposure markers as literal text.
mapped_dirs(){
  find . -type f \
    -not -path './.git/*' -not -path './recordings/*' \
    -not -path './scripts/history-migration/*' -not -name '.DS_Store' \
    -exec dirname {} \; 2>/dev/null | sort -u | sed 's|^\./||' | grep -v '^\.$'
}
unmapped_dirs(){
  local d
  mapped_dirs | while IFS= read -r d; do
    grep -qE "\`$d/?\`.*(GREEN|YELLOW|RED)" CLAUDE.md || printf '%s\n' "$d"
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
# It reads the zone map rather than docs/decisions/, because the map is what the
# next reader consults and a dangling pointer there is the defect. The number
# collision between the two DEC-003 documents is a separate question and not this
# one.
loosening_unnumbered(){
  grep -qE 'DEC-00X|DEC-0\?\?|DEC-TBD' CLAUDE.md
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
check P2-12 "The compute.go loosening points at a DEC number that does not exist" loosening_unnumbered
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
# This section needs the network and gh. It is skipped when either is missing,
# because this script has to stay useful offline.
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
      printf "%s  DEC-004 VIOLATION  %s is still present although the repository is PUBLIC%s\n" "$red" "$f" "$off"
      remaining=$((remaining + 1))
    fi
  done
  if [ "$remaining" = 0 ]; then
    printf "%s       public, and both files are out. The DEC-004 condition is met%s\n" "$green" "$off"
  else
    echo "       See DEC-004 section 2. git rm alone is not enough; both are already in the history"
  fi
fi

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

section "Summary"
printf "  %s%d claims proven%s, %s%d not%s\n" "$green" "$proven" "$off" "$red" "$not" "$off"
echo "  The full audit: docs/internal/audit-2026-08-20.md"