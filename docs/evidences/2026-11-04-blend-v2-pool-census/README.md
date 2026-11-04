# Blend v2 pool census, 4 November 2026

**Question.** How many Blend v2 lending pools are active on Stellar mainnet? DefiLlama
lists 5 Blend v2 yield pools; this record establishes whether that is the lending
pool count.

**Answer.** The Blend v2 pool factory has deployed **12 pools**. At ledgers
65299691 to 65299698 (2026-11-04T09:28Z) **none is in active status** (0 or 1).
Four are on-ice (new borrowing disabled, supply, withdraw and repay still open),
three are frozen, five never left setup. DefiLlama's 5 yield pools are **5 reserves
in 2 lending pools**, Fixed and YieldBlox, not 5 pools.

## Source

| | |
|---|---|
| Pool factory v2 | `CDSYOAVXFY7SM5S64IZPPPYB4GVGGLMQVFREPSQQEZVIWXX5R23G4QSU`, key `poolFactoryV2` in blend-capital/blend-utils `mainnet.contracts.json`; source verified by stellar.expert against blend-contracts-v2 commit `c19abee` |
| Pool list | the factory's `deploy` events, one per pool. The factory has no list function and Soroban RPC keeps events about 7 days, so the history is read from the stellar.expert index (12 events, all `deploy`). An independent check is Hubble `history_contract_events` filtered on the factory and topic `deploy` |
| Pool state | `get_config`, `get_reserve_list`, `get_reserve` simulated against `https://mainnet.sorobanrpc.com`. Read-only, nothing signed |
| Status codes | blend-contracts-v2 `pool/src/pool/status.rs`: 0 admin active, 1 active, 2 admin on-ice, 3 on-ice, 4 admin frozen, 5 frozen, 6 setup |
| Amounts | supplied = `b_supply * b_rate / 1e12`, borrowed = `d_supply * d_rate / 1e12`, in the reserve's own decimals (`pool/src/storage.rs`) |

## Pools

| Pool | Deployed | Status | Reserves with supply |
|---|---|---|---|
| `CAJJZSGM…` Fixed | 2025-04-14 | 3 on-ice | 3 of 3 |
| `CBNR7PYF…` | 2025-04-15 | 6 setup | 0 of 7 |
| `CCCCIQSD…` YieldBlox | 2025-04-17 | 3 on-ice | 8 of 8 |
| `CB4OFHAY…` | 2025-05-01 | 6 setup | 0 of 4 |
| `CAE7QVOM…` | 2025-05-01 | 4 admin frozen | 4 of 4 |
| `CBYOBT7Z…` | 2025-07-14 | 5 frozen | 4 of 4 |
| `CALRF5I2…` | 2025-11-21 | 6 setup | 0 of 5 |
| `CADR6Q2U…` | 2025-11-24 | 6 setup | 0 of 5 |
| `CDMAVJPF…` | 2025-11-24 | 4 admin frozen | 5 of 5 |
| `CDZVHCO7…` | 2026-06-10 | 2 admin on-ice | 1 of 1 |
| `CC4HHXPK…` | 2026-07-28 | 3 on-ice | 3 of 4 |
| `CAIYBZSB…` | 2026-08-26 | 6 setup | 0 of 3 |

Full addresses in `pools.csv`, per-reserve supplied and borrowed in `reserves.csv`.

Only two pools hold material size. Fixed: about 699 million XLM and 51.1 million USDC
supplied, 39.4 million USDC borrowed. YieldBlox: about 654 thousand XLM and 42
thousand USDC supplied. Every other pool holds under 200 thousand units of any
reserve. On-ice is a status, not an empty pool.

## DefiLlama, same morning

`blend-pools-v2` protocol TVL 126.7 million USD. Its 5 yield rows are XLM, USDC and
EURC in the Fixed Pool and XLM and USDC in the YieldBlox Pool.

## Sentence for Deliverable 1

> The Blend v2 pool factory (`CDSYOAVX…`) has deployed 12 pools. At ledger 65299691
> none is in active status; 4 are on-ice (new borrowing disabled, positions still
> open), 3 are frozen and 5 never left setup. Two pools, Fixed and YieldBlox, hold
> material balances; these are the 2 pools behind DefiLlama's 5 yield rows.

## Reproduce

```bash
# from the keel repo root; keel-mcp carries @stellar/stellar-sdk
(cd keel-mcp && node ../keel-backend/docs/evidences/2026-11-04-blend-v2-pool-census/census.mjs)
# submission-day re-check: Keel asset count, DefiLlama figures, fresh census
keel-backend/docs/evidences/2026-11-04-blend-v2-pool-census/recheck.sh
```

Re-running overwrites `pools.json`, `pools.csv` and `reserves.csv` with the current
state. Statuses change: on-ice is set by the backstop automatically and can lift
without an admin action.
