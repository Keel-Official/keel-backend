# B-7 continued: the USTRY reserve configuration, and the February 2026 value

**Date:** 31 August 2026
**Zone:** `docs/evidences/` (YELLOW). Every number below carries a contract address
and a ledger sequence, which is the rule this directory has.
**Continues:** `2026-08-25-oracle-and-pool-config.md` section 4, which closed B-7
for USDC and XLM and left USTRY open with the instruction "locate it and record
the pre-change c_factor".
**Outcome, stated first:** the remediation is located exactly. **February 2026's
`c_factor` for USTRY is NOT recoverable from public unauthenticated sources**, and
section 5 records the four routes tried and why each one fails. Section 6 names
what would close it.

---

## 0. Addresses used throughout

| Role | Address |
| --- | --- |
| Blend V2 YieldBlox pool | `CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS` |
| Pool admin, read this session | `CANSYFVMIP7JVYEZQ463Y2I2VLEVNLDJJ4QNZTDBGLOOGKURPTW4A6FQ` |
| USTRY SAC, reserve index 5 | `CBLV4ATSIWU67CFSQU2NVRKINQIKUZ2ODSZBUJTJ43VJVRSBTZYOPNUR` |
| XLM SAC, reserve index 0 | `CAS3J7GYLGXMF6TDJBBYYSE3HQ6BBSMLNUQ34T6TZMYMW2EVH34XOWMA` |
| USDC SAC, reserve index 1 | `CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75` |
| Oracle adapter | `CD74A3C54EKUVEGUC6WNTUPOTHB624WFKXN3IYTFJGX3EHXDXHCYMXXR` |
| Reflector, wrapped by the adapter | `CALI2BYU2JE6WVRUFYTS6MSBNEHGJ35P4AVCZYF3B6QOE3QKOB2PLE6M` |

**The pool admin is a CONTRACT, not an account.** That is new here and it shapes
everything below: reserve configuration changes arrive through governance, so the
account that submits a change is not the account that authorised it.

---

## 1. The live USTRY ReserveConfig, read independently

Read at latest ledger **64205116**, by `getLedgerEntries` on the ledger key
`ContractData(CCCCIQSD…, ResConfig(CBLV4ATS…), PERSISTENT)`. Entry
`lastModifiedLedgerSeq` **62117252**, `liveUntilLedgerSeq` 64937487.

| Field | Raw | Scaled |
| --- | --- | --- |
| `c_factor` | 0 | **0.00, collateral disabled** |
| `l_factor` | 9000000 | 0.90 |
| `util` | 5000000 | 0.50 |
| `max_util` | 8000000 | 0.80 |
| `r_base` | 100000 | 0.01 |
| `r_one` | 200000 | 0.02 |
| `r_two` | 1000000 | 0.10 |
| `r_three` | 50000000 | 5.00 |
| `reactivity` | 20 | 20 |
| `supply_cap` | 2500000000000 | 250,000 tokens |
| `decimals` | 7 | 7 |
| `enabled` | true | true |
| `index` | 5 | 5 |

**Every figure agrees with the 25 August reading**, which was taken by a different
method, at a different ledger, six days earlier. That is a free confirmation of
that document rather than a new fact, and it is recorded because a second reading
that agrees is worth writing down and a second reading that disagreed would have
been the finding.

`ResData` for USTRY has `lastModifiedLedgerSeq` 64142607, which is ordinary supply
and borrow activity and is not a configuration change.

---

## 2. The remediation is located exactly

`lastModifiedLedgerSeq` on the config entry is **62117252**, and that ledger closed
**2026-04-14T15:46:26Z**, 51 days after the incident.

| Item | Value |
| --- | --- |
| Transaction | `c1c8ff778000ed0b629423a50986f491e8bc2ea865ef7696194b8247411b049f` |
| Ledger | 62117252 |
| Closed at | 2026-04-14T15:46:26Z |
| Submitted by | `GDKXAKCXH6AXP4Y26R5GOJWOKW7BQHAP5KV6HIFVC2JUO7XSZ5L4YYBX` |
| Function | `set_reserve` |
| Successful | yes |

Found by scanning all 347 transactions in that ledger and matching the decoded
envelope against the byte pattern of the USTRY `ResConfig` ledger key, so the match
is on the Soroban footprint the transaction itself declares rather than on a label.

**A correction to the 25 August instruction, and it changes the method.** That
document said to locate the `set_reserve` call and read the pre-change `c_factor`
off it. The pool's interface makes that impossible:

```
fn queue_set_reserve(env, asset: Address, metadata: ReserveConfig);   // carries the config
fn set_reserve(env, asset: Address) -> u32;                          // carries no config
```

So the value lives in the `queue_set_reserve` that preceded it, and `set_reserve`
carries nothing but the asset. `set_reserve` was submitted here by an ordinary
account rather than by the admin contract, which means it takes no admin
authorisation: it applies whatever governance already queued.

---

## 3. New finding: the pool was frozen four hours after the incident

Not asked for by B-7, found while scanning the submitting account's history.

| Item | Value |
| --- | --- |
| Transaction | `5c3197b509450e6521bd5a44746ca22b45118dbf8df7d38caca8f1c796faf1dc` |
| Ledger | 61342737 |
| Closed at | **2026-02-22T04:11:13Z** |
| Function | `set_status` |
| Submitted by | `GDKXAKCXH6AXP4Y26R5GOJWOKW7BQHAP5KV6HIFVC2JUO7XSZ5L4YYBX` |

The manipulation trade closed at 2026-02-22T00:10:21Z. This is **4 hours and 1
minute later**, and it is a pool status change rather than a reserve change. The
sequence the pool operator actually ran was therefore: freeze within hours, disable
USTRY as collateral 51 days later.

**Why this matters beyond B-7.** Deliverable 2 has to state when the unsafe
threshold was crossed relative to the exploit date. This gives the response
timeline a second on-chain anchor that is not the attack itself, and the gap
between the two is 51 days.

The same account's complete history was scanned, 357 transactions, and it contains
exactly one other reserve-configuration action, a `queue_set_reserve` at ledger
59837030 on 2025-11-14 whose envelope does not name USTRY.

---

## 4. The incident borrow leg is XLM, and the amount is exact

Read from the envelope of the incident transaction, which Horizon still serves.

| Item | Value |
| --- | --- |
| Transaction | `3e81a3f7b6e17cc22d0a1f33e9dcf90e5664b125b9e61f108b8d2f082f2d4657` |
| Ledger | 61340408 |
| Contract, function | `CCCCIQSD…`, `submit` |
| Borrower, all three address arguments | `GBO7VUL2TOKPWFAWKATIW7K3QYA7WQ63VDY5CAE6AFUUX6BHZBOC2WXC` |
| Request | one, `request_type` **4** which is Borrow |
| Asset | `CAS3J7GY…`, the **XLM** SAC |
| Amount | `612492783064502` stroops, **61,249,278.3064502 XLM** |

**The borrowed asset is XLM, not USDC.** `scripts/audit-verification.sh` prints
"against roughly 10.97 million dollars borrowed" and DEC-001 carries a ratio built
on that figure. The dollar amount is consistent with this XLM quantity at an XLM
price near 0.179, but **no price reading is taken here**, so this document records
the quantity and does not restate the dollar figure. Whoever revisits that ratio
should quote 61,249,278.3064502 XLM at a named price and ledger.

**The footprint of that same transaction settles one thing and blocks another.**
Its `read_only` set contains `ResConfig` for USTRY, XLM and USDC, and its
`read_write` set does not. So USTRY's configuration entry existed and was consulted
at ledger 61340408 and was not written there, which means its February value is
not in this transaction's writes.

---

## 5. Why February's `c_factor` is not reachable, four routes

Recorded in full because a later reader must not repeat them.

| Route | Result |
| --- | --- |
| **Transaction meta at ledger 62117252.** A modified ledger entry appears in meta as pre-state and post-state, so the pre-change config is there | **Horizon no longer returns `result_meta_xdr`.** Verified: 200 of 200 transaction records at that ledger have no such field. An earlier scan of this session searched that absent field and reported no match, which was a false negative from my own query and not a fact about the chain |
| **RPC `getEvents` or `getTransactions`** | Retention is days. The target is 6 months back |
| **The governor's proposal storage.** `get_proposal(id)` returns `Calldata`, which would carry the queued `ReserveConfig` | Probed ids 0 to 79 on `CANSYFVM…`. Every one returns `null`. Proposal records are not retained after execution |
| **A public ledger-metadata archive.** Soroban meta is not in the standard history archives, so an exported datastore is the only form of it | Five candidate buckets probed. Four do not exist, `sdf-ledger-close-meta` returns 401. No `gcloud` or `bq` on this machine |

Two further dead ends worth naming. stellar.expert indexes contract invocations in
its interface but exposes no public endpoint for them: nine path shapes were probed
and only `/contract/{id}` and `/contract/{id}/value` answer, neither carrying
invocation history. And Horizon has no filter by contract, so the pre-February
`queue_set_reserve` cannot be found by scanning without knowing its ledger first.

---

## 6. What would close it

In cost order.

1. **Hubble.** One BigQuery query over transaction meta at ledger 62117252, or over
   `queue_set_reserve` invocations on the pool, answers this in minutes. Hubble is
   deferred by DEC-002 and this is not a reason to undefer it for the engine, but
   it is a concrete second use for it and belongs in the B-8 discussion.
2. **Any archive node or authenticated meta datastore.** The pre-state at 62117252
   is a single read once meta is available.
3. **The governance proposal record off-chain.** The YieldBlox proposal that queued
   `c_factor = 0` states the previous value in almost every governance process that
   exists. That is a secondary source and would go in a secondary-source section,
   never into `08-collateral.md` as a reading.

**What is already established without it, and it is not nothing.** USTRY's
`c_factor` was above zero in February 2026, because the incident transaction
borrowed against a USTRY position and a zero collateral factor makes that
impossible. The exact figure is unknown; the sign is not.

---

## 7. Reproducing the readings

The ledger key was built by hand rather than with an SDK, because none is installed
here. Correctness was checked before it was trusted: the constructed key was sent to
`getLedgerEntries` and the returned entry contained the same byte pattern, so a
wrong key would have returned nothing rather than something wrong.

```
LedgerKey.CONTRACT_DATA(6)
  contract   = SCAddress.CONTRACT(1) || sha256-id(pool)
  key        = SCVal.VEC(16) || present(1) || len(2)
                 || SCVal.SYMBOL(15) || len("ResConfig") || bytes || pad4
                 || SCVal.ADDRESS(18) || SCAddress.CONTRACT(1) || sha256-id(asset)
  durability = PERSISTENT(1)
```

Endpoints used: `https://mainnet.sorobanrpc.com` for `getLedgerEntries`,
`https://horizon.stellar.org` for ledgers, transactions and envelopes, and the
`stellar` CLI 27.1.0 for `contract info interface` and `xdr decode`. The RPC
rejects requests with no `User-Agent` header with HTTP 403.

---

## 8. Version history

| Date | Change |
| --- | --- |
| 31 August 2026 | Created. Continues B-7 from the 25 August document. USDC and XLM were closed there; USTRY is narrowed here to a located remediation and a named blocker |
