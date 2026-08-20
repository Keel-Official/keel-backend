#!/usr/bin/env bash
#
# lindungi-zona-merah.sh
#
# A PreToolUse hook for Bash. It refuses commands that would MUTATE files under
# internal/depth, the red zone that only Al may write in.
#
# Why a hook rather than just permissions. .claude/settings.json already denies
# Edit and Write on internal/depth/**, but Bash is untouched by those rules.
# `sed -i internal/depth/x.go` walks straight past the lock. Finding P2-6 in
# docs/internal/audit-2026-08-20.md.
#
# What STAYS allowed, because the red zone is not a secret zone:
#   cat internal/depth/CLAUDE.md
#   ls internal/depth/
#   go test ./internal/depth/ -run TestX -v
#   grep -rn ComputeDepth internal/depth/
#   go test ./internal/depth/ 2>&1 | tail -5      (the redirect is NOT into the zone)
#
# What gets refused:
#   sed -i ... internal/depth/depth.go
#   echo x > internal/depth/depth.go
#   cp /tmp/a.go internal/depth/
#   gofmt -w internal/depth/
#   python3 - <<PY ... internal/depth ... PY
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

# Does not touch the red zone at all, nothing to check.
if ! printf '%s' "$command_line" | grep -q 'internal/depth'; then
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

message="internal/depth is the RED ZONE. Al writes there, not you.
This command is refused because it would mutate a file in that zone, and Bash is
the path that the Edit and Write denials in .claude/settings.json do not close.

What you may do there: read, run tests, point out edge cases that are not handled
yet, and ask questions. Offer /teach for the concept or /review-mine once Al has
written it. See internal/depth/CLAUDE.md."

# 1. A redirect whose target sits inside the red zone. Checked separately so that
#    `go test ./internal/depth/ 2>&1 | tail` still passes: it contains a > but its
#    target is not a file in the zone.
if printf '%s' "$command_line" | grep -Eq '>>?[[:space:]]*"?'"'"'?[^|;&<>]*internal/depth'; then
  refuse "$message

Detected: output redirected into internal/depth."
fi

# 2. Commands whose job is to mutate files, mentioning the red zone.
mutating='(\bsed\b[^|;]*-i|\bperl\b[^|;]*-[a-zA-Z]*i|\btee\b|\bcp\b|\bmv\b|\brm\b|\bln\b|\binstall\b|\btruncate\b|\bdd\b|\bpatch\b|\btouch\b|\bmkdir\b|git[[:space:]]+(apply|checkout|restore|stash|rm|mv)|\b(gofmt|goimports)\b[^|;]*-w|\bpython3?\b|\bnode\b|\bruby\b|\bperl\b|\bawk\b[^|;]*-i)'
if printf '%s' "$command_line" | grep -Eq "$mutating"; then
  refuse "$message

Detected: a file-mutating command that mentions internal/depth."
fi

# Mentions the red zone but appears to only read. Allowed.
exit 0
