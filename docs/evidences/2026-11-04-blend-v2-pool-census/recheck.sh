#!/usr/bin/env bash
# Submission-day re-check: Keel's monitored asset count, DefiLlama's Blend v2
# figures, and a fresh pool census. Run from the keel repo root.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
date -u +"read at %Y-%m-%dT%H:%M:%SZ"

echo "== Keel /v1/health"
curl -s https://api.keels.app/v1/health | python3 -c '
import json,sys; h=json.load(sys.stdin); h=h.get("health",h)
print("status", h.get("status"), "| assetsMonitored", h.get("assetsMonitored"),
      "| ledger", h.get("latestScanLedgerSeq"), "| methodology", h.get("methodologyVersion"))'

echo "== DefiLlama blend-pools-v2"
curl -s https://api.llama.fi/tvl/blend-pools-v2 | xargs printf "protocol TVL USD %.0f\n"
curl -s https://yields.llama.fi/pools | python3 -c '
import json,sys
rows=[p for p in json.load(sys.stdin)["data"] if p["project"]=="blend-pools-v2"]
for p in rows: print(" ", p["symbol"], p.get("poolMeta"), round(p["tvlUsd"]))
print(len(rows), "yield rows across", len({p.get("poolMeta") for p in rows}), "lending pools")'

echo "== Pool census"
(cd "$here/../../../../keel-mcp" && node "$here/census.mjs" >/dev/null)
python3 -c '
import json,collections
d=json.load(open("'"$here"'/pools.json"))
print(d["deploy_events"], "pools deployed;", dict(collections.Counter(p["status_label"] for p in d["pools"])))'
