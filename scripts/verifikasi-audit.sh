#!/usr/bin/env bash
#
# verifikasi-audit.sh
#
# Re-runs every claim in docs/internal/audit-2026-08-20.md.
# The point is to make that audit disputable rather than believed.
#
# A PROVEN line means the claim is still true of this repository as it stands.
# A NOT line means the claim is wrong, or it has been fixed.
# Both are useful. Once a finding is dealt with, its line MUST flip to NOT, and
# that is the signal the work is done.
#
# Usage: bash scripts/verifikasi-audit.sh
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
depth_empty()      { [ -z "$(ls internal/depth/*.go 2>/dev/null)" ]; }
conformance_dead() { ! go vet -tags conformance ./internal/conformance/; }
adapter_unused()   { ! grep -rq "internal/adapter" --include='*.go' .; }
metrics_missing()  { ! grep -riq "create table.*metrics" migrations/; }
contract_lacks()   { ! grep -qE "^ +$1:" docs/api/keel-openapi.yaml; }
ikhtisar_missing() { [ ! -f docs/methodology/00-ikhtisar.md ]; }
spike_one_page()   { [ "$(grep -c ledger_close_time docs/evidences/spike_results_1.txt)" = 200 ]; }

learning_pointed_but_missing(){ grep -q "docs/learning" README.md && [ ! -d docs/learning ]; }
readme_promises_record(){ grep -qF "make record      # jalankan snapshot recorder" README.md; }
empty_dirs_vanish(){
  for d in api internal/api internal/store; do
    [ -d "$d" ] && [ -z "$(ls -A "$d" 2>/dev/null)" ] && return 0
  done
  return 1
}
# Leaking when the guard hook is absent, or present but does not refuse a mutation.
# Needs jq, exactly like the hook itself.
red_zone_leaks(){
  [ -f .claude/hooks/lindungi-zona-merah.sh ] || return 0
  command -v jq >/dev/null 2>&1 || return 0
  ! printf '%s' 'sed -i "" s/a/b/ internal/depth/x.go' \
    | jq -Rs '{tool_name:"Bash", tool_input:{command:.}}' \
    | bash .claude/hooks/lindungi-zona-merah.sh 2>/dev/null \
    | grep -q '"deny"'
}

section "P0  What blocks everything else"
check P0-1 "There is not a single commit, though the origin remote is configured" no_commit
check P0-2 "internal/depth holds no .go file" depth_empty
check P0-3 "The conformance test fails to build, so the golden fixture tests nothing" conformance_dead

section "P1  Forked specification"
check P1-1 "The TDD states raw snapshots are not stored in the DB" \
  grep -qF 'Raw snapshots are not stored in the database' docs/architecture/Keel_Technical_Design_Document.md
check P1-2 "yet 0001_snapshots.sql stores a raw JSONB column" \
  grep -q "raw                 JSONB" migrations/0001_snapshots.sql
check P1-3 "and the metrics table the API reads is absent from migrations" metrics_missing
check P1-4 "The CHECK on source accepts only horizon and hubble" \
  grep -qF "source IN ('horizon', 'hubble')" migrations/0001_snapshots.sql
check P1-5 "although the domain already has DataSourceTradesImplied" \
  grep -q "DataSourceTradesImplied" internal/domain/types.go
check P1-6 "types.go holds OracleResistance as a scalar" \
  grep -qF "OracleResistance        *decimal.Decimal" internal/domain/types.go
check P1-7 "although DEC-003 rejects the scalar form explicitly" \
  grep -qF "a single scalar quotient" docs/decisions/DEC-003-api-contract-v1-1.md
check P1-8 "CostToMaxReachablePrice exists in the code" \
  grep -q "CostToMaxReachablePrice" internal/domain/types.go
check P1-9 "but is absent from the API contract" contract_lacks costToMaxReachablePrice
check P1-10 "unevaluatedFlags is absent as a contract field" contract_lacks unevaluatedFlags
check P1-11 "bandConfidence is absent as a contract field" contract_lacks bandConfidence
check P1-12 "although 09-flag-dan-band requires both in the openapi file" \
  grep -qF 'add `unevaluatedFlags`, `bandConfidence`' docs/methodology/09-flag-dan-band.md
check P1-13 "The contract uses criticalDelta 0.5" \
  grep -q "criticalDelta: 0.5" docs/api/keel-openapi.yaml
check P1-14 "while DefaultParams uses a critical delta of 1.0" \
  grep -qF 'ManipulationCriticalDelta: dec("1.0")' internal/conformance/fixture.go
check P1-15 "The methodology requires both C_max terms reported, not only the minimum" \
  grep -qF 'both have to be reported' docs/methodology/keel-methodology-core.md
check P1-16 "internal/adapter uses float64" grep -q "float64" internal/adapter/horizon.go
check P1-17 "arch_test scans only internal/domain and internal/depth" \
  grep -qF 'paketMurni = []string{".", "../depth"}' internal/domain/arch_test.go
check P1-18 "internal/adapter is imported by no package" adapter_unused
check P1-19 "internal/adapter does not appear in the CLAUDE.md zone map" \
  bash -c '! grep -q "internal/adapter" CLAUDE.md'
check P1-20 "DEC-001 still uses the 0.50 dollar report and a ratio of 1 to 22 million" \
  grep -qF "1 to 22 million" docs/decisions/DEC-001-ustry-identity.md
check P1-21 "although evidence in the repo shows the executing trade was 5.3475699 USDC" \
  grep -qF '"base_amount": "5.3475699"' docs/evidences/spike_result_2.txt
check P1-22 "The curl in DEC-002 types USTRY as credit_alphanum4" \
  grep -q "credit_alphanum4&base_asset_code=USTRY" docs/decisions/DEC-002-hold-bigquery.md
check P1-23 "The curl in DEC-001 makes the same mistake" \
  grep -q "credit_alphanum4&base_asset_code=USTRY" docs/decisions/DEC-001-ustry-identity.md
check P1-24 "although the evidence states USTRY is credit_alphanum12" \
  grep -qF '"counter_asset_type": "credit_alphanum12"' docs/evidences/spike_result_2.txt
check P1-25 "The spike evidence stops at exactly 200 records, one Horizon page" spike_one_page
check P1-26 "DEC-003 still lists MC delta 1 as reachable true" \
  grep -qF '`130.0627093`, `true`' docs/decisions/DEC-003-api-contract-v1-1.md
check P1-27 "although the fixture and the contract already corrected it to false" \
  grep -qF 'The delta 1.0 entry previously' docs/api/keel-openapi.yaml
check P1-28 "documentUrl points at the ciganytry org, not Keel-Official" \
  grep -q "github.com/ciganytry/keel" docs/api/keel-openapi.yaml
check P1-29 "and points at 00-ikhtisar.md, which does not exist" ikhtisar_missing
check P1-30 "assetBrokenBook uses ledgerClosedAt 2026-02-21T23:39:00Z" \
  grep -q "2026-02-21T23:39:00Z" docs/api/keel-openapi.yaml
check P1-31 "although the fixture and the evidence state 2026-02-22T00:10:21Z" \
  grep -q "2026-02-22T00:10:21Z" testdata/fixtures/ustry_pre_exploit.md
check P1-32 "GoldenSnapshot labels itself horizon" \
  grep -q "Source: domain.DataSourceHorizon" internal/conformance/fixture.go
check P1-33 "although the contract labels the same book trades-implied" \
  grep -q "dataSource: trades-implied" docs/api/keel-openapi.yaml

section "P2  Cheap hygiene"
check P2-1 "CLAUDE.md force-loads keel-openapi.yaml into every session" \
  grep -qF "@docs/api/keel-openapi.yaml" CLAUDE.md
check P2-2 "README points at docs/learning, and that directory does not exist" learning_pointed_but_missing
check P2-4 "README promises make record works, though it exits with code 3" readme_promises_record
check P2-5 "An empty directory exists that will vanish when somebody else clones" empty_dirs_vanish
check P2-6 "The red zone lock leaks through Bash, uncovered by the Edit and Write denials" red_zone_leaks
check P2-7 "The methodology file structure decision is still open in the methodology README" \
  grep -qF "The decision that has to be made" docs/methodology/README.md
check P2-8 "The fixture writes 'All four' for a list holding six flags" \
  grep -qF "All four must be reported" testdata/fixtures/ustry_pre_exploit.md

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
