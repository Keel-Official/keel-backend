#!/usr/bin/env bash
#
# generate-api-mocks.sh
#
# Writes every example response in docs/api/keel-openapi.yaml out as a standalone
# JSON file under docs/api/mocks/, for the frontend to build against before the
# API exists.
#
# WHY GENERATED AND NOT HAND WRITTEN. A hand copied mock is a second home for the
# same data, and a second home drifts. This repository has been bitten by that
# repeatedly: the PRD still carries flag definitions the methodology superseded,
# and keel-bootstrap.sh still carries a CLAUDE.md from before the repository was
# in English. A generator plus a drift check is the only version of this that
# stays true.
#
# THE DRIFT CHECK. `make api-mocks-check` regenerates into a temporary directory
# and diffs. A non-empty diff means the contract moved and the mocks did not, and
# it fails loudly rather than serving stale data to a frontend.
#
# Requires node, for the YAML to JSON step only. Nothing else in this repository
# depends on node.
#
# Usage: bash scripts/generate-api-mocks.sh [output-directory]

set -euo pipefail

cd "$(dirname "$0")/.."

CONTRACT="docs/api/keel-openapi.yaml"
OUT="${1:-docs/api/mocks}"

if ! command -v npx >/dev/null 2>&1; then
  echo "generate-api-mocks: npx not found. It is needed only to read YAML." >&2
  exit 1
fi

mkdir -p "$OUT"

tmp_json=$(mktemp -t keel-contract-XXXXXX.json)
trap 'rm -f "$tmp_json"' EXIT

npx --yes js-yaml@4.1.0 "$CONTRACT" > "$tmp_json"

python3 - "$tmp_json" "$OUT" <<'PY'
import json, pathlib, sys

contract = json.load(open(sys.argv[1]))
out = pathlib.Path(sys.argv[2])

# The named examples under components.examples, keyed by the file name a frontend
# would expect rather than by the schema name.
named = {
    "AssetHealthy":    "asset-healthy.json",
    "AssetPoolOnly":   "asset-pool-only.json",
    "AssetNoPrice":    "asset-no-price.json",
    "AssetBrokenBook": "asset-broken-book.json",
    "AssetHistorical": "asset-historical.json",
    "AssetListMixed":  "asset-list-mixed.json",
    "HistoryUstry":    "history-ustry.json",
}

written = []
examples = contract.get("components", {}).get("examples", {})
for key, filename in named.items():
    if key not in examples:
        raise SystemExit(f"example {key} is missing from the contract")
    value = examples[key]["value"]
    (out / filename).write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n")
    written.append(filename)

# The two inline examples on the meta endpoints.
inline = {
    "/health":      "health.json",
    "/methodology": "methodology.json",
}
for path, filename in inline.items():
    node = contract["paths"][path]["get"]["responses"]["200"]["content"]["application/json"]
    (out / filename).write_text(json.dumps(node["example"], indent=2, ensure_ascii=False) + "\n")
    written.append(filename)

print(f"generate-api-mocks: wrote {len(written)} files to {out}")
for f in sorted(written):
    print(f"  {f}")
PY
