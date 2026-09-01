# USTRY holder set and supply, raw reading

Pulled by `scripts/pull-holder-and-supply.sh` on 2026-08-31 (UTC).

This directory is a reading, not a result. Nothing here is computed.
Sections 2 and 3 of `docs/methodology/07-supporting-metrics.md` have no
definitions yet, so no total, share, top-N or HHI appears in any file
below. When those definitions are written, they get run over these bytes.

## Asset

| Field | Value |
| --- | --- |
| Code | `USTRY` |
| Issuer | `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` |
| Type (from Horizon) | `credit_alphanum12` |
| SAC contract id | `CBLV4ATSIWU67CFSQU2NVRKINQIKUZ2ODSZBUJTJ43VJVRSBTZYOPNUR` |
| SEP-1 toml | https://etherfuse.com/.well-known/stellar.toml |
| Horizon | https://horizon.stellar.org |

## The reading

Horizon `/accounts?asset=` is current state only. There is no ledger
parameter and no archive that returns this same answer later, so the
reading time below is part of the evidence and not decoration.

| | Latest-Ledger | Read at (UTC) |
| --- | --- | --- |
| `/assets` | 64211133 | 2026-08-31T15:59:53Z |
| first holder page | 64211133 | 2026-08-31T16:00:22Z |
| last holder page | 64211152 | 2026-08-31T16:01:53Z |

The ledger moved between the first and the last page if those two numbers
differ. They usually will. Each CSV row carries the ledger of the page it
came from, so the set is not pretending to be a single instant.

## Paging

| | |
| --- | --- |
| Pages fetched | 5 |
| Page size requested | 200 |
| Page budget | 60 |
| Holder rows written | 875 |
| Distinct account ids | 875 |
| Paging completed | YES |
| Stop reason | short page (75 < 200); the trustline set ended here |

`/assets` reported `accounts.authorized = 875` at ledger
64211133. That figure and the row count above were produced by two
different endpoints and are recorded side by side without being reconciled.

## Files

| File | What it is |
| --- | --- |
| `assets.json` | the `/assets` body, unchanged. Carries the supply figures and account counts section 3 needs |
| `assets.headers.txt` | its response headers, including Latest-Ledger |
| `pages/accounts-NNNN.json[.gz]` | each holder page body, unchanged |
| `pages/accounts-NNNN.headers.txt` | each page's response headers |
| `holders.csv` | one row per holder, amounts as the strings Horizon sent |
| `requests.tsv` | every request: time out, time back, status, Latest-Ledger, Horizon Date, bytes, sha256 |

Page bodies are gzipped. A page of 200 accounts runs to 12-21 MB,
because `/accounts` returns every balance of every account and not only
the one asked for. gzip is a container and not an edit: the sha256 in
`requests.tsv` is taken over the UNCOMPRESSED body, so
`gzip -cd pages/accounts-0001.json.gz | shasum -a 256` checks it.

## Where the supply figures are, and why they do not add up to one number

Every value below is a field Horizon sent in `assets.json`, at ledger
64211133. None is derived, and they are deliberately NOT summed.

| Field | Value |
| --- | --- |
| `accounts.authorized` | 875 |
| `accounts.authorized_to_maintain_liabilities` | 0 |
| `accounts.unauthorized` | 0 |
| `balances.authorized` | 10432382.3504695 |
| `balances.authorized_to_maintain_liabilities` | 0.0000000 |
| `balances.unauthorized` | 0.0000000 |
| `liquidity_pools_amount` | 732.9256906 |
| `num_liquidity_pools` | 40 |
| `contracts_amount` | 1080866.4532456 |
| `num_contracts` | 37 |
| `claimable_balances_amount` | 0.0000000 |
| `num_claimable_balances` | 0 |
| `num_accounts` (deprecated) | null |
| `amount` (deprecated) | null |

**This is the whole of section 3 question 1, sitting in one table.** The
asset exists in four places at once and Horizon counts them separately:
trustlines (`balances.authorized`), liquidity pools
(`liquidity_pools_amount`), Soroban contracts (`contracts_amount`, across
37 contracts including the SAC above), and claimable balances
(`claimable_balances_amount`). `holders.csv` covers the FIRST of those four
and nothing else, because `/accounts?asset=` returns accounts and a pool,
a contract and a claimable balance are none of them.

So "issued total", "total held in trustlines" and "circulating" are three
different numbers here, and the gap between them is large rather than
rounding. Section 3 has to pick one and say so, and section 2 question 2
has to make the same choice for the concentration denominator. Neither
choice is made in this directory. `num_accounts` and `amount` are the
deprecated fields and Horizon now returns null for both, which is why
neither can be used as the answer.

The issuer flags are `auth_required=false`,
`auth_revocable=false`,
`auth_immutable=false`,
`auth_clawback_enabled=false`. With
`auth_required` false, every trustline is authorized on creation, which is
why `is_authorized` is `true` on every row of `holders.csv` and carries no
information for this asset. It is recorded anyway because an asset with the
flag set would need it.

## Known limits of this reading

1. Not an instant. The pages were read one after another and the ledger
   advanced underneath them. A holder who moved between page 1 and page 5
   is recorded at whichever page saw it.
2. Not re-fetchable. Nothing here can be reproduced for a past ledger.
   Re-running this script produces a different reading, not the same one.
3. Trustlines only, which is one of the four places named above. The issuer
   holds no trustline to its own asset and does not appear. Pools, contracts
   and claimable balances do not appear either. Those are properties of the
   endpoint, not decisions this script made.
4. Custodial and exchange accounts are not marked, because they cannot be
   detected reliably. Section 2 already says so.
5. A zero balance is still a trustline and is recorded as a row. Whether a
   zero-balance trustline is a *holder* is section 2 question 1 and is not
   answered here.
