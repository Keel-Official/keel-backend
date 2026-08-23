#!/usr/bin/env bash
#
# lindungi-zona-merah.sh
#
# A PreToolUse hook for Bash. It refuses commands that would MUTATE a red zone
# path, the code only Al may write.
#
# THE ZONE IS ONE FILE: internal/domain/compute.go. It was the internal/depth
# directory until methodology 1.0.3 moved the computations out, and this pattern
# covered both paths for as long as the empty directory survived. Al removed the
# directory on 23 August 2026 and the second path came out of this pattern on the
# 24th, because a lock on a path that cannot exist is not distinguishable from a
# lock that works.
#
# The pattern has to stay this narrow. internal/domain also holds types.go and
# arch_test.go, which Claude maintains, so matching on the directory would refuse
# most ordinary work in the package and the hook would be turned off within a day.
# Both directions are checked by scripts/audit-verification.sh: P2-6 proves a
# mutation of compute.go is refused, P2-6b proves the same command against
# types.go is not.
#
# Why a hook rather than just permissions. .claude/settings.json already denies
# Edit and Write on this path, but Bash is untouched by those rules.
# `sed -i internal/domain/compute.go` walks straight past the lock. Finding P2-6
# in docs/internal/audit-2026-08-20.md.
#
# What STAYS allowed, because the red zone is not a secret zone:
#   cat internal/domain/compute.go
#   go test ./internal/domain/ -run TestX -v
#   grep -rn ComputeDepth internal/domain/
#   go test ./internal/domain/ 2>&1 | tail -5   (the redirect is NOT into the zone)
#
# What gets refused:
#   sed -i ... internal/domain/compute.go
#   echo x > internal/domain/compute.go
#   cp /tmp/a.go internal/domain/compute.go
#   gofmt -w internal/domain/compute.go
#   python3 - <<PY ... internal/domain/compute.go ... PY
#
# WHAT IS NOT REFUSED, AND IT IS A HOLE, NOT A DESIGN CHOICE. A command that
# mutates the DIRECTORY without naming the file walks straight through, because
# the pattern below is the file:
#   gofmt -w internal/domain/
#   gofmt -l -w .              which is what `make fmt` runs, and make is allowed
# This coverage existed while the zone WAS a directory and was lost when the zone
# became a file. Finding P2-6c in scripts/audit-verification.sh, which probes it
# on every run and states the two candidate fixes and what each one costs. Al
# decides which; both of them refuse something that is currently allowed.
#
# This is a guardrail, not a sandbox. It closes the accidental path, not the
# deliberate one. Its purpose is to remind, and a reminder that refuses is more
# useful than a reminder written in a document.
#
# It is biased toward refusing: a file-mutating command that merely mentions the
# zone is refused even when its real target is elsewhere. When that happens, use
# the Edit tool for the file outside the zone; permissions already govern that
# path separately.

set -uo pipefail

input=$(cat)

# Pull out the command. This hook has to stay safe when jq is missing, so in that
# case we let the command through rather than guessing.
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

command_line=$(printf '%s' "$input" | jq -r '.tool_input.command // empty')

# Every red zone path, as one alternation. It holds one entry today. Add here when
# the zone moves again, add the same path to the deny list in
# .claude/settings.json, because neither one closes the other's route, and REMOVE
# the old path in the same commit: an alternation branch that can no longer match
# reports success forever.
zone='internal/domain/compute\.go'

# Does not touch the red zone at all, nothing to check.
if ! printf '%s' "$command_line" | grep -Eq "$zone"; then
  exit 0
fi

refuse() {
  jq -nc --arg reason "$1" '{
    hookSpecificOutput: {
      hookEventName: "PreToolUse",
      permissionDecision: "deny",
      permissionDecisionReason: $reason
    }
  }'
  exit 0
}

message="internal/domain/compute.go is the RED ZONE. Al writes it, not you.

This command is refused because it would mutate a file in that zone, and Bash is
the path that the Edit and Write denials in .claude/settings.json do not close.

What you may do there: read, run tests, point out edge cases that are not handled
yet, and ask questions. Offer /teach for the concept or /review-mine once Al has
written it. See internal/domain/CLAUDE.md.

If your real target is a file OUTSIDE the zone and this command only MENTIONS the
zone, that is this hook being deliberately blunt. Use the Edit or Write tool for
that file; permissions govern it separately."

# 1. A redirect whose target sits inside the red zone. Checked separately so that
#    `go test ./internal/domain/ 2>&1 | tail` still passes: it contains a > but its
#    target is not a file in the zone.
if printf '%s' "$command_line" | grep -Eq '>>?[[:space:]]*"?'"'"'?[^|;&<>]*('"$zone"')'; then
  refuse "$message

Detected: output redirected into the red zone."
fi

# 2. Commands whose job is to mutate files, mentioning the red zone.
mutating='(\bsed\b[^|;]*-i|\bperl\b[^|;]*-[a-zA-Z]*i|\btee\b|\bcp\b|\bmv\b|\brm\b|\bln\b|\binstall\b|\btruncate\b|\bdd\b|\bpatch\b|\btouch\b|\bmkdir\b|git[[:space:]]+(apply|checkout|restore|stash|rm|mv)|\b(gofmt|goimports)\b[^|;]*-w|\bpython3?\b|\bnode\b|\bruby\b|\bperl\b|\bawk\b[^|;]*-i)'
if printf '%s' "$command_line" | grep -Eq "$mutating"; then
  refuse "$message

Detected: a file-mutating command that mentions the red zone."
fi

# Mentions the red zone but appears to only read. Allowed.
exit 0
