# Funding graph probe: the yield of criterion 4

Produced by `scripts/funding-graph-probe.sh`. This measures how many trades a
shared-funder rule would identify. It does not label any trade genuine or
non-genuine and it does not recommend accepting or rejecting the criterion.
Section 1 of `docs/methodology/07-supporting-metrics.md` writes the rule; this
only says what the rule would find.

## 0. Read this before any number below

Horizon retains history from ledger **57903841** to **64211623**. Everything
earlier is not in this source. An account created before that ledger shows a
first operation that is not its `create_account`, and its funder cannot be
recovered from `/accounts/{id}/operations` at all.

So an unknown funder here does not mean the account had no funder. It means
the operation that created it is older than Horizon's window. A funder that is
unknown cannot be observed to be shared, therefore:

> **Every count in this document is a LOWER BOUND on what criterion 4 would
> catch against a complete history.**

`DEC-002` defers Hubble, which is the source that would lift this ceiling.

| Account classification | Count |
| --- | --- |
| `FUNDER_KNOWN` | 179 |
| `CREATION_BEFORE_RETAINED_HISTORY` | 247 |
| **total accounts probed** | **426** |

`UNCLASSIFIED` is any account whose first operation fits neither case. It is
listed separately rather than folded into either so it cannot hide.

## 1. Accounts sharing a funder with at least one other account

- Accounts with a known funder: **179**
- Distinct funders among them: **42**
- Funders that funded more than one of these accounts: **11**
- **Accounts sharing a funder with at least one other account: 148**

That is 148 of 179 accounts with a known funder, and
148 of 426 accounts probed.

## 2. Trades with both sides sharing a funder, per window

Volume is `counter_amount`, the notional in the quote asset, summed exactly
with `decimal.Decimal`. No float is used anywhere in this script.

### `USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv`

| | Trades | Volume (quote) | Share of window volume |
| --- | ---: | ---: | ---: |
| All rows | 13547 | 375320.8368055 | 1 |
| Both sides carry an account | 13489 | 375317.4722123 | 0.999991035 |
| One side is a pool, no account | 58 | 3.3645932 | 0.000008965 |
| Both sides an account, at least one funder unknown | 12099 | | |
| **Both sides share a funder** | **2** | **5.3475783** | **0.000014248** |

Share of the volume that could be evaluated at all, meaning rows where both
sides carry an account: **0.000014248**

### `USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-08-01_2026-09-01.csv`

| | Trades | Volume (quote) | Share of window volume |
| --- | ---: | ---: | ---: |
| All rows | 56615 | 6018.4588000 | 1 |
| Both sides carry an account | 56019 | 6017.3853691 | 0.999821644 |
| One side is a pool, no account | 596 | 1.0734309 | 0.000178356 |
| Both sides an account, at least one funder unknown | 53297 | | |
| **Both sides share a funder** | **0** | **0** | **0.000000000** |

Share of the volume that could be evaluated at all, meaning rows where both
sides carry an account: **0.000000000**

## 3. Funder cardinality distribution

How many funders funded exactly N of the probed accounts.

| Accounts funded by one funder | Number of such funders | Accounts covered |
| ---: | ---: | ---: |
| 1 | 31 | 31 |
| 2 | 5 | 10 |
| 3 | 3 | 9 |
| 4 | 1 | 4 |
| 6 | 1 | 6 |
| 119 | 1 | 119 |

Totals: 42 funders covering 179 accounts.

## 4. What was read

| | |
| --- | --- |
| Endpoint | `GET /accounts/{id}/operations?order=asc&limit=1` |
| Raw bodies | `accounts/<account id>.json`, one per account, unedited |
| Request log | `requests.tsv`, with the wall clock time of every request |
| Classification | `funders.csv`, one row per account |
| Retention ceiling | `horizon-root.json` |

`/accounts/{id}/operations` sends no `Latest-Ledger` header; only
`/order_book`, `/liquidity_pools` and `/assets` do, which
`internal/horizon/client.go` line 117 already records. The reading time of
every request is in `requests.tsv`, and the retention ceiling that bounds the
whole measurement is in `horizon-root.json`.

Re-runnable with `REPORT_ONLY=1 RUN_DIR=<this directory>`, which rebuilds this
report from the stored bodies without contacting Horizon.
