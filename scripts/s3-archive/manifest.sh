#!/usr/bin/env bash
#
# manifest.sh
#
# Writes a sha256 manifest for a tree of recordings, in the format `sha256sum -c`
# and `shasum -a 256 -c` both read with no custom tooling.
#
# THIS IS THE PART THAT MAKES AN S3 ARCHIVE EVIDENCE RATHER THAN STORAGE. A git
# blob carries its own hash and sits in a chain anyone can walk, so a recording in
# git cannot be altered afterwards without the alteration being visible. An S3
# object carries neither. Committing the manifest to git gives the hashes back a
# chain: the bytes live in the bucket, the proof of what those bytes were lives in
# a public repository, and a reviewer needs no AWS account and no trust in anyone's
# IAM configuration to check one against the other.
#
# It is also, incidentally, the growth argument. One manifest line is about 128
# bytes against a recording of about 3 kilobytes, so the orphan branch grows at
# roughly a twenty-fourth of the rate.
#
# APPEND, NEVER REWRITE. The manifest for a day is built up one round at a time and
# an existing line is never touched. A rewritten manifest is indistinguishable from
# a tampered one, which would give away the property this file exists to establish.
# Lines are sorted so two runs over the same tree produce the same bytes, which is
# non-negotiable rule 2 applied to a text file.
#
# Usage:
#   bash scripts/s3-archive/manifest.sh <recordings-dir> [manifest-file]
#
# With no manifest file it prints to stdout, which is what the workflow does before
# appending.

set -euo pipefail

dir=${1:-}
out=${2:-}

if [ -z "$dir" ] || [ ! -d "$dir" ]; then
  echo "usage: bash scripts/s3-archive/manifest.sh <recordings-dir> [manifest-file]" >&2
  exit 1
fi

# sha256sum on Linux and GNU coreutils, shasum -a 256 on macOS. The workflow runs
# on ubuntu-latest and a person checking by hand is usually on the other one, so
# both have to work or the check stops being run.
# A variable and not a shell function, because xargs runs a COMMAND and cannot see
# a function defined in this shell. It is left unquoted at the call site so that
# "shasum -a 256" splits into its three words, which is the one place in this file
# where word splitting is wanted.
if command -v sha256sum >/dev/null 2>&1; then
  HASHER="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  HASHER="shasum -a 256"
else
  echo "manifest: neither sha256sum nor shasum is available" >&2
  exit 1
fi

# -print0 and -0, because a key is written by a machine but a directory name is
# written by a person and one of them will contain a space eventually.
#
# Paths are made relative to <dir> so the manifest names S3 KEYS and not whatever
# local path this happened to run in. That is what lets the same manifest verify a
# local tree and a bucket.
lines=$(cd "$dir" && find . -type f -name '*.json.gz' -print0 \
        | sort -z \
        | xargs -0 $HASHER \
        | sed 's|\./||')

if [ -z "$lines" ]; then
  echo "manifest: no .json.gz files under $dir" >&2
  exit 1
fi

if [ -n "$out" ]; then
  mkdir -p "$(dirname "$out")"
  printf '%s\n' "$lines" >> "$out"
  # Sorting the accumulated file keeps it deterministic across rounds without ever
  # dropping a line: sort -u would silently swallow a genuine duplicate hash, which
  # is a fact worth keeping rather than hiding.
  sort -o "$out" "$out"
  echo "manifest: $(wc -l < "$out" | tr -d ' ') lines in $out"
else
  printf '%s\n' "$lines"
fi
