# What `-max-pages` buys, priced from the trustline counts already measured

**Derived 17 September 2026.** No request was made for this file. Every count comes
from `docs/evidences/2026-08-31-authorized-counts/authorized-and-supply.csv`, pulled
from Horizon on 31 August 2026, one request per asset to
`/assets?asset_code=&asset_issuer=`, which the manifest in that directory records.

**What it is for.** `keel holders` caps one reading at 25 pages of 200 accounts, and
an asset whose trustline set does not fit is stored with `truncated` set and **without
any figures**. DEC-018 point 3 proposes a staleness bound and says in its own section 7
that the number is proposed rather than derived. This file is the same exercise for the
page cap: it does not choose a number, it prices every number so the choice is a row in
a table rather than a guess.

---

## 1. The current cap explains the current coverage exactly

| | |
|---|---|
| Cap today | 25 pages, 5,000 accounts |
| Assets whose set fits | **35 of 63** |
| Assets carrying holder figures in production, 17 September | **35 of 61** |

The two numbers agree, and they agree for the right reason rather than by coincidence:
the 26 assets reporting no concentration are XLM, which is native and has no trustlines
at all, and 25 issued assets whose sets exceed 5,000. Nothing is failing. The cap is
doing exactly what it was set to do.

## 2. The cost is concentrated in three assets

| Asset | Authorized trustlines | Pages at 200 |
|---|---|---|
| ARST | 328,633 | 1,644 |
| AQUA | 191,828 | 960 |
| USDZ | 181,059 | 906 |
| SHX | 91,397 | 457 |
| XRP | 56,851 | 285 |
| yXLM | 53,887 | 270 |
| VELO | 41,224 | 207 |
| AFR | 38,169 | 191 |
| yUSDC | 34,866 | 175 |
| EURC | 30,072 | 151 |

Three assets carry 3,510 pages between them. Everything else in the set, 60 assets,
carries about 3,000 pages in total.

## 3. The table the decision is a row of

One request per page, NFR-6 caps this repository at 3,000 requests an hour. A pass is
one reading for every asset in the set.

| Cap, pages | Accounts | Assets fully covered | Pages per pass | Minutes at the cap |
|---|---|---|---|---|
| **25 (today)** | 5,000 | **35 / 63** | 842 | **16.8** |
| 50 | 10,000 | 43 / 63 | 1,457 | 29.1 |
| 100 | 20,000 | 51 / 63 | 2,252 | 45.0 |
| 250 | 50,000 | 57 / 63 | 3,486 | 69.7 |
| 500 | 100,000 | 60 / 63 | 4,498 | 90.0 |
| 1000 | 200,000 | 62 / 63 | 5,864 | 117.3 |
| **2000** | 400,000 | **63 / 63** | 6,508 | **130.2** |

Above 2000 nothing changes, because ARST at 1,644 pages is the largest set in the
demonstration set and every asset already fits.

## 4. What the shape of that table says

**The curve flattens where it matters, and that is the finding rather than the
numbers.** Complete coverage costs 130 minutes against 17 today. It is a factor of
eight on a job that runs ONCE A DAY, which leaves 21.8 hours of the day untouched, and
it is a quarter of what DEC-019 prices the trade-derived family at for a single refresh.

**The last three assets are cheap in relative terms.** Cap 500 buys 60 of 63 for 90
minutes; cap 2000 buys the remaining three for 40 minutes more. Whoever chooses 500 is
choosing to leave ARST, AQUA and USDZ permanently unevaluated to save 40 minutes a day,
and AQUA is a pair this repository has already written a decision record about.

**A cap is not the only lever and this file prices only that one.** A per-asset cap, or
pulling the three expensive assets on a slower cadence than the rest, would both change
this arithmetic. Neither is proposed here.

## 5. What this cannot tell you

**Whether the counts still hold.** They were read on 31 August 2026. A trustline set
grows, so every page figure here is a floor rather than a current value, and ARST at
1,644 pages is the number most likely to have moved. Re-running
`scripts/pull-authorized-counts.sh` is one request per asset and refreshes the whole
table.

**Whether a complete set is worth more than a fast one.** That is the judgement the
table exists to inform and it is not in the table. The measures behind it are
`HOLDER_CONCENTRATION_EXTREME` and `HOLDER_CONCENTRATION_HIGH`, both CRITICAL and HIGH
tier, so an asset without them cannot reach `bandConfidence: full` no matter what else
is filled in.

**Reproduce this file:**

```bash
python3 - <<'EOF'
import csv, math
rows = list(csv.DictReader(open(
    'docs/evidences/2026-08-31-authorized-counts/authorized-and-supply.csv')))
a = sorted(((r['asset_code'], int(r['accounts_authorized'] or 0)) for r in rows if r['asset_code']),
           key=lambda x: -x[1])
for cap in (25, 50, 100, 250, 500, 1000, 2000):
    covered = sum(1 for _, v in a if math.ceil(v / 200) <= cap)
    pages = sum(min(math.ceil(v / 200), cap) for _, v in a)
    print(cap, covered, len(a), pages, round(pages / 3000 * 60, 1))
EOF
```
