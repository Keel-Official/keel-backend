#!/usr/bin/env bash
#
# verify-manifest.sh
#
# Checks that what is in the bucket is byte for byte what git says was recorded.
#
# THIS IS THE REVIEWER'S PATH AND IT IS THE POINT OF THE WHOLE ARRANGEMENT. Clone
# the public repository, read a manifest committed to it, fetch the objects, hash
# them, compare. No AWS account is needed on the reviewer's side when the bucket is
# public read, and no trust in anybody's IAM configuration is needed either way:
# the answer comes out of arithmetic the reviewer performs themselves.
#
# A FAILURE HERE IS NOT A SCRIPT PROBLEM. It means the archive and the record of
# the archive disagree, which is the one thing this design exists to make
# impossible to do quietly. Read the failing key before assuming a transfer glitch.
#
# Two modes:
#   bash verify-manifest.sh <manifest-file> <bucket>       fetch from S3 and check
#   bash verify-manifest.sh <manifest-file> --local <dir>  check a local tree
#
# The second mode needs no AWS anything and is how a reviewer checks the
# recordings/samples/ tree that is already in git.

set -euo pipefail

manifest=${1:-}
target=${2:-}
localdir=${3:-}

if [ -z "$manifest" ] || [ ! -f "$manifest" ] || [ -z "$target" ]; then
  echo "usage: bash scripts/s3-archive/verify-manifest.sh <manifest-file> <bucket>" >&2
  echo "       bash scripts/s3-archive/verify-manifest.sh <manifest-file> --local <dir>" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  HASHER="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  HASHER="shasum -a 256"
else
  echo "verify-manifest: neither sha256sum nor shasum is available" >&2
  exit 1
fi

if [ "$target" = "--local" ]; then
  if [ -z "$localdir" ] || [ ! -d "$localdir" ]; then
    echo "verify-manifest: --local needs a directory" >&2
    exit 1
  fi
  # The manifest names keys relative to the recordings root, so the check runs
  # from that root. Exit status comes straight from the checker: a mismatch is a
  # non-zero exit and not a printed warning somebody scrolls past.
  cd "$localdir"
  $HASHER -c "$(cd - >/dev/null && cd "$(dirname "$manifest")" && pwd)/$(basename "$manifest")"
  exit $?
fi

command -v aws >/dev/null 2>&1 || { echo "verify-manifest: the aws cli is required" >&2; exit 1; }

bucket=$target
work=$(mktemp -d)
# Cleaned up on every exit path including a failure, because a script that leaves a
# few hundred megabytes behind when it fails is a script people stop running.
trap 'rm -rf "$work"' EXIT

ok=0; bad=0; missing=0

# read without -r would eat backslashes in a key. Keys here are machine written and
# contain none, which is exactly the kind of assumption that stops being true.
while read -r want key; do
  [ -z "${key:-}" ] && continue
  dest="$work/$key"
  mkdir -p "$(dirname "$dest")"
  if ! aws s3 cp "s3://$bucket/recordings/$key" "$dest" --quiet 2>/dev/null; then
    printf 'MISSING  %s\n' "$key"
    missing=$((missing + 1))
    continue
  fi
  got=$($HASHER "$dest" | awk '{print $1}')
  if [ "$got" = "$want" ]; then
    ok=$((ok + 1))
  else
    printf 'MISMATCH %s\n  git says %s\n  s3  has %s\n' "$key" "$want" "$got"
    bad=$((bad + 1))
  fi
done < "$manifest"

printf '\n%d verified, %d mismatched, %d missing\n' "$ok" "$bad" "$missing"
[ "$bad" -eq 0 ] && [ "$missing" -eq 0 ]
