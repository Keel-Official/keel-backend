#!/usr/bin/env bash
#
# lindungi-zona-merah.sh
#
# A PreToolUse hook for Bash. It refuses commands that would MUTATE a red zone
# path, the code only Al may write.
#
# THE ZONE IS ONE FILE: internal/domain/compute.go. It was the internal/depth
# directory until methodology 1.0.3 moved the computations out, and this hook
# covered both paths for as long as the empty directory survived. Al removed the
# directory on 23 August 2026 and the second path came out of this file on the
# 24th, because a lock on a path that cannot exist is not distinguishable from a
# lock that works.
#
# THREE WAYS IN, AND THE THIRD WAS OPEN FOR A DAY. A command can reach the zone by
# naming the file, by naming the DIRECTORY that contains it, or by sweeping a whole
# tree without naming either. Only the first was closed when the zone became a
# file: while the zone WAS a directory, the second and third were closed for free,
# because any command broad enough to reach the file named the directory by
# definition. That coverage was lost silently in the move, and this file's own
# header carried `gofmt -w internal/depth/` through it as an example of what was
# refused. Finding P2-6c. All three are closed now:
#
#   1. the file      sed -i ... internal/domain/compute.go
#   2. the directory gofmt -w internal/domain/     rm -rf internal/domain
#   3. the sweep     gofmt -l -w .                 make fmt
#
# THE DIRECTORY RULE HAS TO STAY NARROW, and this is the constraint that shapes
# the whole file. internal/domain also holds types.go and arch_test.go, which
# Claude maintains, so a rule matching every mention of internal/domain would
# refuse most ordinary work in the package and the hook would be switched off
# within a day. So the directory is matched only in its DIRECTORY FORM: named as a
# target, not as the prefix of a named file inside it. `internal/domain/` is
# refused, `internal/domain/types.go` is not. Both directions are proven on every
# run by P2-6, P2-6b and P2-6c in scripts/audit-verification.sh.
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
#   gofmt -l .                                  (lists, does not write)
#   gofmt -w internal/domain/types.go           (yellow, and named)
#   sed -i "" s/a/b/ internal/domain/types.go   (same)
#
# HOW TO FORMAT, now that the sweep is closed. Name the files:
# `gofmt -w path/to/file.go`. To find out whether anything needs it, `gofmt -l .`
# is read only and still allowed. `make fmt` is refused for Claude and unaffected
# for Al, whose terminal this hook does not sit in, so CI's gofmt check still has
# an owner and a one command fix.
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

# Newlines collapsed to spaces and one space appended, so that every path in the
# command is followed by at least one character. That lets every pattern below end
# in a character class instead of a `$` anchor, and a `$` inside an alternation
# branch is not portable in POSIX ERE. Collapsing newlines can only make this hook
# refuse MORE, never less, which is the safe direction for a guardrail.
line=$(printf '%s ' "$command_line" | tr '\n' ' ')

# 1. The red zone FILE. Add here when the zone moves again, add the same path to
#    the deny list in .claude/settings.json, because neither one closes the other's
#    route, and REMOVE the old path in the same commit: an alternation branch that
#    can no longer match reports success forever.
zone='internal/domain/compute\.go'

# 2. The red zone's DIRECTORY, in directory form only. Two branches because there
#    is no lookahead here: either the path stops at `domain`, or it stops at the
#    slash after it. `internal/domain/types.go` matches neither, and must not.
zone_dir='(internal/domain[^/[:alnum:]_]|internal/domain/[^[:alnum:]_])'

zone_any="$zone|$zone_dir"

# 3. A formatter rewriting in place with no file named. `gofmt -l -w .` and
#    `goimports -w ./...` reach compute.go while mentioning neither it nor its
#    directory, which is how P2-6c stayed open. A named .go file is the escape.
formatter_in_place='\b(gofmt|goimports)\b[^|;&]*-[a-zA-Z]*w'
names_go_file='\.go[^a-zA-Z0-9_]'

# `make fmt` is that same sweep wearing a different name: the recipe is
# `gofmt -l -w .`. A hook cannot see inside a recipe, so the target is named here.
# ADDING A FORMATTING TARGET TO THE MAKEFILE MEANS ADDING IT HERE TOO. That is a
# second home for one fact and it is the weakest line in this file.
make_fmt='\bmake\b[^|;&]*\bfmt\b'

touches_zone=0
if printf '%s' "$line" | grep -Eq "$zone_any"; then
  touches_zone=1
fi

sweeps_zone=0
if printf '%s' "$line" | grep -Eq "$formatter_in_place" &&
  ! printf '%s' "$line" | grep -Eq "$names_go_file"; then
  sweeps_zone=1
fi
if printf '%s' "$line" | grep -Eq "$make_fmt"; then
  sweeps_zone=1
fi

# Reaches the red zone by none of the three routes, nothing to check.
if [ "$touches_zone" -eq 0 ] && [ "$sweeps_zone" -eq 0 ]; then
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

This command is refused because it would mutate that file, and Bash is the path
that the Edit and Write denials in .claude/settings.json do not close.

What you may do there: read, run tests, point out edge cases that are not handled
yet, and ask questions. Offer /teach for the concept or /review-mine once Al has
written it. See internal/domain/CLAUDE.md.

If your real target is a file OUTSIDE the zone and this command only MENTIONS the
zone, that is this hook being deliberately blunt. Use the Edit or Write tool for
that file; permissions govern it separately."

# A. A tree-wide in-place format. Checked first because it names neither the file
#    nor the directory, so no message about a mentioned path would make sense.
if [ "$sweeps_zone" -eq 1 ]; then
  refuse "$message

Detected: an in-place format over a whole tree, which reaches the red zone without
naming it. Name the files instead: gofmt -w path/to/file.go. To see what needs
formatting, gofmt -l . is read only and allowed. make fmt is Al's to run."
fi

# B. A redirect whose target sits inside the red zone. Checked separately so that
#    `go test ./internal/domain/ 2>&1 | tail` still passes: it contains a > but its
#    target is not a file in the zone.
if printf '%s' "$line" | grep -Eq '>>?[[:space:]]*"?'"'"'?[^|;&<>]*('"$zone_any"')'; then
  refuse "$message

Detected: output redirected into the red zone."
fi

# C. Commands whose job is to mutate files, naming the zone or its directory.
mutating='(\bsed\b[^|;]*-i|\bperl\b[^|;]*-[a-zA-Z]*i|\btee\b|\bcp\b|\bmv\b|\brm\b|\bln\b|\binstall\b|\btruncate\b|\bdd\b|\bpatch\b|\btouch\b|\bmkdir\b|git[[:space:]]+(apply|checkout|restore|stash|rm|mv)|\b(gofmt|goimports)\b[^|;]*-w|\bpython3?\b|\bnode\b|\bruby\b|\bperl\b|\bawk\b[^|;]*-i)'
if printf '%s' "$line" | grep -Eq "$mutating"; then
  refuse "$message

Detected: a file-mutating command that names the red zone or its directory."
fi
