# Authorized trustline counts across the 64 assets in the metrics table

Pulled by `scripts/pull-authorized-counts.sh` on 2026-08-31 (UTC), from
`https://horizon.stellar.org`, no account required.

One request per asset, to `/assets?asset_code=&asset_issuer=`. No holder
paging: `/accounts` is not touched by this script at all.

## What this directory is for

Section 2 question 4 of `docs/methodology/07-supporting-metrics.md` asks
what is reported when a trustline set is too large to page within budget.
Section 6.4 of the technical design assumes the case is rare and does not
say how rare. The table below is that assumption measured.

## The set

| | |
| --- | --- |
| Assets in the set | 64 |
| Read from `/assets` | 63 |
| Not addressable by `/assets` | 1 |
| Pair files read | `configs/demonstration-set.json` `configs/recorder-pairs.json`  |

The assets that were not read, and why:

| Asset | Type | Status |
| --- | --- | --- |
| the native asset, no code and no issuer | `native` | `not_addressable` |

`/assets` is addressed by (code, issuer). The native asset has neither.
An empty code and issuer are read as NO FILTER and return the whole asset
collection; `asset_code=XLM` returns issued assets whose ticker is XLM
and not the native asset. Its trustline count is not zero and not
pending: holding XLM needs no trustline, so section 2 question 4 does not
apply to it. It keeps a row so that the set stays 64 and the
exclusion is visible rather than arithmetic.

## Distribution of `accounts.authorized`

Counted over the 63 assets that were read. Buckets are half open,
low inclusive.

| Authorized trustlines | Assets | Share of 63 |
| --- | --- | --- |
| 0 to 0 | 0 | 0.0% |
| 1 to 9 | 3 | 4.8% |
| 10 to 99 | 10 | 15.9% |
| 100 to 999 | 13 | 20.6% |
| 1000 to 9999 | 17 | 27.0% |
| 10000 to 99999 | 17 | 27.0% |
| 100000 and above | 3 | 4.8% |

Smallest: 4. 
Largest: 328633.

## The page budget, measured

| | |
| --- | --- |
| Page size (`PAGE_LIMIT`) | 200 |
| Pages allowed (`PAGE_BUDGET`) | 60 |
| Trustlines reachable | 60 x 200 = 12000 |
| **Assets above it** | **18 of 63** |

The assets above the budget, largest first:

| Asset | Issuer | `accounts.authorized` | Pages needed at 200 |
| --- | --- | --- | --- |
| `ARST` | `GCSAZVWX` | 328633 | 1644 |
| `AQUA` | `GBNZILST` | 191828 | 960 |
| `USDZ` | `GAKTLPC4` | 181059 | 906 |
| `SHX` | `GDSTRSHX` | 91397 | 457 |
| `XRP` | `GBXRPL45` | 56851 | 285 |
| `yXLM` | `GARDNV3Q` | 53887 | 270 |
| `VELO` | `GDM4RQUQ` | 41224 | 207 |
| `AFR` | `GBX6YI45` | 38169 | 191 |
| `yUSDC` | `GDGTVWSM` | 34866 | 175 |
| `EURC` | `GDHU6WRG` | 30072 | 151 |
| `SSLX` | `GBHFGY3Z` | 21665 | 109 |
| `TFT` | `GBOVQKJY` | 20145 | 101 |
| `BTC` | `GDPJALI4` | 19671 | 99 |
| `LSP` | `GAB7STHV` | 19590 | 98 |
| `ETH` | `GBFXOHVA` | 16087 | 81 |
| `GOLD` | `GBC5ZGK6` | 13731 | 69 |
| `SLVR` | `GBZVELEQ` | 13510 | 68 |
| `EURC` | `GAQRF3UG` | 12808 | 65 |

Pages needed is a ceiling division of a count Horizon stated by a page
size this script was given. It is arithmetic on the reading and not a
measurement: the real walk can need one more page, because /accounts is
current state and the set moves while it is being paged.

## Reading times

`/assets` is current state only. There is no ledger parameter and no
archive that returns the same answer later, so each row's reading time and
Latest-Ledger are part of the evidence.

| | Latest-Ledger | Read at (UTC) |
| --- | --- | --- |
| first request | 64211652 | 2026-08-31T16:48:59Z |
| last request | 64211669 | 2026-08-31T16:50:39Z |

The ledger advanced between them, so this is 63 readings taken
over that span and not a snapshot. Every CSV row carries its own
`latest_ledger` and `read_at_utc`, which is rule 1 of the non-negotiables
applied to a reading that has no LedgerSeq of its own.

## Files

| File | What it is |
| --- | --- |
| `bodies/<CODE>.<ISSUER8>.json` | each `/assets` body, unchanged |
| `bodies/<CODE>.<ISSUER8>.headers.txt` | its response headers, including Latest-Ledger |
| `authorized-and-supply.csv` | one row per asset, every amount the exact string Horizon sent |
| `requests.tsv` | every request: time out, time back, status, Latest-Ledger, Horizon Date, bytes, sha256 |
| `manifest.md` | this file. A VIEW over the stored bytes, recomputed from them |

The filename carries the issuer prefix because two codes in this set are
not unique: EURC and GOLD each appear twice under different issuers. An
asset is the pair (code, issuer) and is never matched on the ticker.

## What is NOT here

No total supply, no circulating figure, no concentration measure, no
top-N, no HHI, and no reconciliation of the four places an asset can sit
(trustlines, pools, contracts, claimable balances). Every one of those is
a definition sections 2 and 3 have not written. The CSV holds the fields
those definitions will need, as strings, and stops there.

`accounts.authorized` counts TRUSTLINES, not holders. A zero balance is
still a trustline and is counted here; whether it is a *holder* is section
2 question 1. The issuer holds no trustline to its own asset and is not in
this count either.

`num_accounts` and `amount` are the deprecated fields. Horizon now omits
them entirely rather than returning null, so the CSV records `ABSENT` for
both, which is a different fact from an empty value.
