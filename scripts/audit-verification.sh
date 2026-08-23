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

green=$'\033[32m'; red=$'\033[31m'; dim=$'\033[90m'; bold=$'\033[1m'; off=$'\033[0m'
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
fixture_omits_pool(){
  [ -f docs/evidences/pool_ustry_usdc_2026-02.txt ] &&
    grep -qE 'Pools:[[:space:]]+nil' internal/conformance/fixture.go
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
empty_dirs_vanish(){
  for d in api internal/api internal/store; do
    [ -d "$d" ] && [ -z "$(ls -A "$d" 2>/dev/null)" ] && return 0
  done
  return 1
}
# THE RED ZONE HOOK, PROBED IN THREE DIRECTIONS. There are three ways a Bash
# command can reach the red zone: naming the file, naming the DIRECTORY that holds
# it, or sweeping a whole tree without naming either. P2-6, P2-6c and P2-6b below
# probe the first two for leaks and then probe the hook for over-refusal, which is
# a leak on a longer timescale, because a hook that refuses ordinary work gets
# switched off within a day.
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
# Leaking when the guard hook is absent, or present but does not refuse a mutation
# that names the red zone file.
#
# The probe path was internal/depth/x.go until 24 August 2026. That directory was
# removed on the 23rd, and the check would have stayed green for as long as the
# hook kept the dead path in its alternation, then flipped to PROVEN once it was
# tidied, reporting a leak that did not exist. Worse in the other direction: while
# it passed, it was proving that the hook defends a path nobody can write to. It
# probes the file that is actually the zone now.
red_zone_leaks(){
  hook_absent && return 0
  ! hook_refuses 'sed -i "" s/a/b/ internal/domain/compute.go'
}
# PROVEN while a command that never names the red zone file reaches it anyway,
# either by naming the directory or by sweeping a tree that contains it.
#
# This was the guarantee QUIETLY WEAKENING when the zone moved from a directory to
# a file, and it was open for one day. While the zone WAS internal/depth, any
# command broad enough to reach the code named the zone by definition, so both
# routes were closed for free, and the hook's header listed
# `gofmt -w internal/depth/` as an example of exactly that. The example survived
# the move and stopped being true, which is the seventh time this repository has
# found a check or a claim measuring the old shape of something.
#
# CLOSED 24 AUGUST 2026, both routes, in the order Al chose. The sweep first: an
# in-place formatter with no .go file named is refused, and `make fmt` is named
# explicitly because a hook cannot see inside a recipe. Then the directory: matched
# in directory form only, so `internal/domain/` is refused and
# `internal/domain/types.go` is not, which is what keeps P2-6b below at NOT.
#
# Five probes and not one, because each closed route has its own way of reopening.
red_zone_dir_bypass(){
  hook_absent && return 1
  ! hook_refuses 'gofmt -w internal/domain/'  && return 0
  ! hook_refuses 'rm -rf internal/domain'     && return 0
  ! hook_refuses 'gofmt -l -w .'              && return 0
  ! hook_refuses 'goimports -w ./...'         && return 0
  ! hook_refuses 'make fmt'                   && return 0
  return 1
}
# Over-refusing when the hook denies work it has no business denying. types.go and
# arch_test.go are Claude's to write and they sit in the same package as the zone,
# so the directory rule above is one careless character away from refusing them.
# Reading and running are allowed too: the red zone is not a secret zone.
red_zone_over_refuses(){
  hook_absent && return 1
  hook_refuses 'sed -i "" s/a/b/ internal/domain/types.go' && return 0
  hook_refuses 'gofmt -w internal/domain/types.go'         && return 0
  hook_refuses 'cat internal/domain/compute.go'            && return 0
  hook_refuses 'go test ./internal/domain/ -run TestArch'   && return 0
  hook_refuses 'go test ./internal/domain/ 2>&1 | tail -5' && return 0
  hook_refuses 'gofmt -l .'                                && return 0
  hook_refuses 'make ci'                                   && return 0
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
# THE FIX IS AL'S, and not because it is hard. Claude is blocked from loosening the
# guardrail Claude is subject to, which is the correct arrangement and the reason
# this line is a finding rather than a patch. Two parts, both in this hook:
#
#   1. A heredoc body is DATA. Scan `git commit -F - <<MSG` up to the `<<` and no
#      further. Keep scanning the full command when the thing being fed is an
#      interpreter or a writer, so `python3 - <<PY` writing to compute.go stays
#      refused, which is what the fifth probe in P2-6 guards.
#   2. Anchor every rule to COMMAND POSITION, the start of the line or just after a
#      shell separator, so a verb inside a quoted string is not a verb. This loses
#      `find . -exec rm {} + internal/domain`, which is the deliberate path rather
#      than the accidental one.
#
# Until then the workaround is `git commit -F <file>`, which keeps the prose off
# the command line. That works and it is not a fix: it depends on remembering.
red_zone_refuses_prose(){
  hook_absent && return 1
  hook_refuses 'git commit -m "rm -rf internal/domain is refused"' && return 0
  hook_refuses 'echo "make fmt is refused"'                        && return 0
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
check P2-6 "The red zone lock leaks through Bash, uncovered by the Edit and Write denials" red_zone_leaks
check P2-6b "or the same hook refuses a yellow file next door, which gets it switched off" red_zone_over_refuses
check P2-6c "or a directory-wide or tree-wide mutation reaches it without naming it at all" red_zone_dir_bypass
check P2-6d "The same hook reads prose as a command, so a commit message quoting the zone is refused" red_zone_refuses_prose
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
from decimal import Decimal, getcontext
getcontext().prec = 60
ask = Decimal(266843207) / Decimal(2500000)
bid = Decimal(1057) / Decimal(1000)
amt = Decimal("1.2185312")
p0 = (ask + bid) / 2
spread = (ask - bid) / p0 * 100
cost = ask * amt
def compare(name, computed, written, tol=Decimal("0.0000001")):
    ok = abs(computed - Decimal(written)) <= tol
    tag = "\033[32mMATCH \033[0m" if ok else "\033[31mDIFFER\033[0m"
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
