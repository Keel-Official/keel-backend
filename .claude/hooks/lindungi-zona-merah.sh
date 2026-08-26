#!/usr/bin/env bash
#
# lindungi-zona-merah.sh
#
# A PreToolUse hook for Bash. It refuses commands that would MUTATE a red zone
# path, and it refuses one workflow that is Al's alone.
#
# WHAT CHANGED ON 25 AUGUST 2026, AND WHY THIS FILE SHRANK. internal/domain/
# compute.go was the original red zone and is no longer red: Al moved it to YELLOW
# so that Deliverable 1 could be finished by two hands instead of one. Every rule
# that existed to protect that one file came out with it. This header keeps the
# account of what was removed, because a guardrail that quietly loses coverage is
# the failure this file was built to prevent, and the same is true of one that
# quietly sheds it on purpose.
#
# Removed with compute.go: the file-name rule, and the DIRECTORY rule that matched
# internal/domain in directory form. Both are gone because internal/domain now
# holds no red file at all, and a lock on a package where every file is writable
# refuses ordinary work while protecting nothing. P2-6 and P2-6c in
# scripts/audit-verification.sh assert the old behaviour and must be INVERTED, not
# deleted: a probe that disappears leaves no trace that the guard ever existed.
# P2-6b, which asserts that internal/domain/types.go is NOT refused, still holds.
#
# WHAT REMAINS RED, and it is red all the way down:
#
#   testdata/fixtures/   the golden fixture
#   testdata/manual/     the Layer 1 hand recomputations, added 26 August 2026
#   docs/context/        the SoW and the execution plan
#
# These are matched in ANY form, a named file inside them included. None of them
# holds a single file Claude maintains, so the narrowness that once protected
# types.go has nothing to protect here. Finding P2-11.
#
# testdata/manual/ IS LOCKED BEFORE IT EXISTS, and that is the point rather than an
# oversight. It will hold the hand recomputation spreadsheets that are the
# independent oracle for compute.go, and the moment worth protecting is the one
# before the first file lands: a directory that spends even a day writable is a
# directory whose first spreadsheet may have been produced by the thing it exists
# to check, and nothing downstream can tell afterwards which numbers those were.
# Every other rule in this file was added after the fact, to a path that already
# had contents. This one is not, so the ordinary reading of an empty match here is
# that the work has not started, never that the guard is dead.
#
# The fixture is the one that matters most now. With compute.go yellow, the fixture
# is the ONLY remaining structural guarantee that the implementation is checked
# against numbers computed independently of it. internal/conformance/fixture.go
# states the rule in its own header: do not adjust these numbers to match the code,
# adjust the code to match these numbers. That rule was a convention while compute.go
# was red and one person wrote both sides. It is load-bearing now that two do.
#
# docs/methodology/ is red in the CLAUDE.md zone map and is deliberately NOT here.
# The map gives Claude a job inside it, restructuring and cross-referencing what Al
# defines, so a lock there would refuse the work the map assigns. A guardrail that
# refuses the work it protects gets switched off.
#
# THE SWEEP RULE IS NOT A ZONE RULE ANY MORE. `gofmt -l -w .` and `make fmt` are
# still refused, but no red zone holds a .go file, so nothing about a zone explains
# it. It survives as a WORKFLOW rule: CLAUDE.md assigns `make fmt` to Al so that
# formatting has one owner and CI's gofmt check has one fix. Keeping it is a
# deliberate choice not to loosen more than the zone change required. Removing it
# is a separate decision and should be recorded as one.
#
# Why a hook rather than just permissions. .claude/settings.json denies Edit and
# Write on the red paths, but Bash is untouched by those rules:
# `sed -i testdata/fixtures/ustry.md` walks straight past the lock. Finding P2-6 in
# docs/internal/audit-2026-08-20.md.
#
# What STAYS allowed, because a red zone is not a secret zone:
#   cat testdata/fixtures/ustry_pre_exploit.md
#   grep -rn ExpectedDepth testdata/fixtures/
#   go test ./internal/conformance/ -v
#   gofmt -l .                                  (lists, does not write)
#   gofmt -w internal/domain/compute.go         (yellow now, and named)
#
# HOW TO FORMAT. Name the files: `gofmt -w path/to/file.go`. To find out whether
# anything needs it, `gofmt -l .` is read only and still allowed. `make fmt` is
# refused for Claude and unaffected for Al, whose terminal this hook does not sit in.
#
# This is a guardrail, not a sandbox. It closes the accidental path, not the
# deliberate one. Its purpose is to remind, and a reminder that refuses is more
# useful than a reminder written in a document.
#
# It is biased toward refusing: a file-mutating command that merely mentions a red
# path is refused even when its real target is elsewhere. When that happens, use the
# Edit tool for the file outside the zone; permissions govern that path separately.

set -uo pipefail

input=$(cat)

# Pull out the command. This hook has to stay safe when jq is missing, so in that
# case we let the command through rather than guessing.
if ! command -v jq >/dev/null 2>&1; then
  exit 0
fi

command_line=$(printf '%s' "$input" | jq -r '.tool_input.command // empty')

# P2-6d part 1, REPAIRED THE SAME DAY IT LANDED. Finding P2-6e.
#
# A heredoc BODY is data. The line that OPENS one is not, and that distinction is
# the whole of P2-6e. The first form of this fix truncated the command at the `<<`
# marker, which is correct only while the marker is the last thing on the line. It
# is not: `cat <<'EOF' > testdata/fixtures/f.md` puts the redirect AFTER the marker,
# so truncating threw the target away and the write walked through. Only the BODY is
# removed, from the newline after the marker to its terminator, and the opening line
# is kept whole and scanned like any other command.
#
# Being generous about the terminator is the safe direction here and it is worth
# knowing why, because this is the one place in the file where "refuse more" and
# "match less" point the same way. Stopping the strip EARLY leaves more text to be
# scanned, so a marker that matches sooner than the real terminator can only cause
# a refusal, never miss one.
#
# The interpreter exception stays: for `python3 - <<PY` the body IS the program and
# it may write to a zone, so nothing is stripped and the whole command is scanned.
# That case is a probe in P2-6 and it must not regress.
#
# The interpreter has to be preceded by a separator rather than by any word
# boundary, and that is not fussiness. `\bsh\b` matches inside `probe.sh`, and a
# repository whose scripts all end in `.sh` would then skip the strip for nearly
# every command that touches one, which switches part 1 off exactly where it was
# needed. A dot is a word boundary; it is not a command boundary.
body=$command_line
if ! printf '%s' "$command_line" | grep -Eq '(^|[|;&([:space:]])(python3?|node|ruby|perl|awk|sh|bash|zsh)[[:space:]].*<<'; then
  body=$(printf '%s\n' "$command_line" | awk '
    skip { if ($0 ~ term) { skip = 0 }; next }
    { print }
    match($0, /<<-?[ \t]*[^ \t;|&<>]+/) {
      m = substr($0, RSTART, RLENGTH)
      gsub(/[^A-Za-z_0-9]/, "", m)
      if (m != "") { term = "^[ \t]*" m "[ \t]*$"; skip = 1 }
    }
  ')
fi

# Newlines become SEMICOLONS, not spaces, and one space is appended, so that every
# path in the command is followed by at least one character. That lets every pattern
# below end in a character class instead of a `$` anchor, and a `$` inside an
# alternation branch is not portable in POSIX ERE.
#
# The semicolon is the second half of P2-6e and it is the more important half. This
# line used to collapse newlines to SPACES, and its comment claimed that could only
# make the hook refuse more. That claim was true for exactly as long as there was no
# command-position anchor. Once part 2 below arrived, a newline collapsed to a space
# glued line two onto the end of line one, so the verb on line two was no longer at
# the start of anything and walked straight through:
#
#     cd /tmp
#     sed -i "" s/a/b/ testdata/fixtures/ustry_pre_exploit.md
#
# A multi-line command is the ORDINARY form, not a deliberate evasion, so this was
# the worst of the eight routes P2-6e found. A newline separates two commands, which
# is what a semicolon means, and the anchor reads it that way now.
line=$(printf '%s ' "$body" | tr '\n' ';')

# The zones that are red ALL THE WAY DOWN: matched in any form, a named file inside
# them included, because none of them holds a file Claude maintains. The trailing
# class costs nothing, since the line always ends in a space, and it keeps a sibling
# like testdata/fixtures_old out of the match. testdata/manual gets that same
# protection from the same trailing class, which is why the two testdata paths are
# listed in full rather than folded into testdata/(fixtures|manual): the folded form
# is one character from becoming a bare testdata/ prefix under a later edit, and a
# prefix that wide would swallow every sibling the class was added to exclude.
#
# This is kept as zone_any, a name wider than its current contents, because rule B
# below reads it to find a REDIRECT into a zone, and because a further family may be
# added here later. `echo x > testdata/fixtures/f.md` carries no mutating verb for
# rule C to catch; left out of this variable, that write walks straight through.
zone_any='(testdata/fixtures|testdata/manual|docs/context)[^[:alnum:]_]'

# P2-6d fix, part 2: command position anchor. A verb is only a verb at the start of
# the line or after a shell separator (|, ;, &, &&, ||, an opening paren, and a
# newline, which the semicolon above stands in for). A verb inside a quoted string
# is prose, not a command. This is approximate and loses `find . -exec rm {} +`,
# the deliberate path, not the accidental one.
#
# The second group is P2-6e again. A command position is not always the first WORD
# of it: `FOO=1 sed -i ...`, `sudo rm ...`, `env`, `time`, `nohup`, `xargs` and the
# `then`/`do` of a compound statement all put the verb one word later, and every one
# of them walked through the first form of this anchor. Zero or more such prefixes
# are skipped before the verb is read.
cmd='(^|[|;&(]+)[[:space:]]*(([A-Za-z_][A-Za-z_0-9]*=[^[:space:]]*|sudo|env|time|nohup|command|xargs|nice|then|do|else)[[:space:]]+)*'

# A formatter rewriting in place with no file named. This no longer reaches a red
# zone, since no red zone holds a .go file. It is refused as a workflow rule: see
# THE SWEEP RULE IS NOT A ZONE RULE ANY MORE in the header. A named .go file is the
# escape, and it is the intended way to format.
formatter_in_place="$cmd"'\b(gofmt|goimports)\b[^|;&]*-[a-zA-Z]*w'
names_go_file='\.go[^a-zA-Z0-9_]'

# `make fmt` is that same sweep wearing a different name: the recipe is
# `gofmt -l -w .`. A hook cannot see inside a recipe, so the target is named here.
# ADDING A FORMATTING TARGET TO THE MAKEFILE MEANS ADDING IT HERE TOO. That is a
# second home for one fact and it is the weakest line in this file.
make_fmt="$cmd"'\bmake\b[^|;&]*\bfmt\b'

touches_zone=0
if printf '%s' "$line" | grep -Eq "$zone_any"; then
  touches_zone=1
fi

sweeps_repo=0
if printf '%s' "$line" | grep -Eq "$formatter_in_place" &&
  ! printf '%s' "$line" | grep -Eq "$names_go_file"; then
  sweeps_repo=1
fi
if printf '%s' "$line" | grep -Eq "$make_fmt"; then
  sweeps_repo=1
fi

# Touches nothing this hook governs.
if [ "$touches_zone" -eq 0 ] && [ "$sweeps_repo" -eq 0 ]; then
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

blunt="If your real target is a file OUTSIDE the zone and this command only MENTIONS
the zone, that is this hook being deliberately blunt. Use the Edit or Write tool for
that file; permissions govern it separately."

# A. A tree-wide in-place format. Checked first, and it gets its own message rather
#    than the zone message, because it names no zone and mentions none. Sending a
#    reader refused over `make fmt` to a message about the golden fixture would
#    teach the wrong lesson twice: once about the refusal, once about where the rule
#    lives. That mismatch is exactly what the two-branch message in the previous
#    version of this file produced once compute.go left the zone.
if [ "$sweeps_repo" -eq 1 ]; then
  refuse "In-place formatting without naming a .go file is Al's to run.

This is a WORKFLOW rule, not a zone rule: no red zone holds a .go file any more, so
this command reaches nothing protected. CLAUDE.md assigns make fmt to Al so that
formatting has one owner and CI's gofmt check has one fix.

It covers a whole-tree sweep (gofmt -l -w .) and a directory target
(gofmt -w internal/domain/) alike, because neither names a file. That is broader
than the zone change required, and deliberately so: the zone move loosened what it
had to and nothing more.

Name the files instead: gofmt -w path/to/file.go. To see what needs formatting,
gofmt -l . is read only and allowed."
fi

# Everything below concerns a red path, so one message serves both remaining rules.
message="testdata/fixtures/, testdata/manual/ and docs/context/ are RED ZONES, red
all the way down.

The golden fixture's numbers are computed BY HAND before any implementation exists,
and internal/conformance/fixture.go says it in its own header: do not adjust these
numbers to match the code, adjust the code to match these numbers. This matters more
than it used to. compute.go is yellow as of 25 August 2026, so the fixture is now the
only structural guarantee that the implementation is checked against numbers derived
independently of it. A fixture Claude can edit is a fixture that confirms whatever
the code already does.

testdata/manual/ holds the Layer 1 hand recomputations, which are the independent
oracle for compute.go, and it is red for exactly the same reason one step further
back: numbers produced by Claude must never become the numbers that test Claude's
code. It is locked before it exists, so a refusal here on an empty directory is the
guard working early rather than a stale rule.

docs/context/ holds the SoW and the execution plan, which are inputs from outside
and are read, never written.

What you may do there: read them, quote them, grep them, and report where the code
and the fixture disagree. That disagreement IS the finding, and editing either side
of it destroys the finding rather than resolving it.

$blunt"

# B. A redirect whose target sits inside a red zone. Checked separately so that
#    `go test ./internal/conformance/ 2>&1 | tail` still passes: it contains a > but
#    its target is not a file in a zone.
if printf '%s' "$line" | grep -Eq '>>?[[:space:]]*"?'"'"'?[^|;&<>]*('"$zone_any"')'; then
  refuse "$message

Detected: output redirected into a red zone."
fi

# C. Commands whose job is to mutate files, naming a red zone.
mutating="$cmd"'(\bsed\b[^|;]*-i|\bperl\b[^|;]*-[a-zA-Z]*i|\btee\b|\bcp\b|\bmv\b|\brm\b|\bln\b|\binstall\b|\btruncate\b|\bdd\b|\bpatch\b|\btouch\b|\bmkdir\b|git[[:space:]]+(apply|checkout|restore|stash|rm|mv)|\b(gofmt|goimports)\b[^|;]*-w|\bpython3?\b|\bnode\b|\bruby\b|\bperl\b|\bawk\b[^|;]*-i)'
if printf '%s' "$line" | grep -Eq "$mutating"; then
  refuse "$message

Detected: a file-mutating command that names a red zone."
fi