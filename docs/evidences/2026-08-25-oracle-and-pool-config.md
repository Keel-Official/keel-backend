# Evidence: YieldBlox pool oracle path and reserve parameters

Status: IN PROGRESS
Collected by: Al
Collection date: 2026-08-25
Purpose: closes B-6 (oracle window) and B-7 (Blend V2 risk parameters) for
`docs/methodology/06-oracle.md` and `docs/methodology/08-collateral.md`.

## Rule for this file

Every number in this document carries a source contract address and the ledger
sequence at which it was read. A number without both is deleted, not kept with a
qualifier. Secondary sources (blogs, post-mortems, news) may appear in section 5
as context, and are never promoted into sections 2 through 4.

## 0. Anchor

| Field | Value | Source |
|---|---|---|
| Incident transaction | `3e81a3f7b6e17cc22d0a1f33e9dcf90e5664b125b9e61f108b8d2f082f2d4657` | blnd-huntr forensic workspace |
| Ledger | 61340408 | same, to be confirmed against Horizon |
| Timestamp | 2026-02-22 00:24:27 UTC | same |
| Contract call | `submit`, `request_type: 4` (Borrow) | same |
| Amount drawn | 61,249,278.3 XLM | same |

Confirmation against Horizon:

- [yes] `GET /transactions/{hash}` returns `ledger` = 61340408
- [yes] `GET /transactions/{hash}` returns `successful` = true
- [yes] `GET /transactions/{hash}/operations` contains an `invoke_host_function`
      operation naming the pool contract

Horizon ledger reported: 61340408
Discrepancy, if any: None

## 1. Pool contract identity

| Field | Value | How obtained | Ledger read |
|---|---|---|---|
| Pool contract address | `CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS` | incident transaction | 64118528 |
| Pool contract address | `CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS` | Blend mainnet UI | current |
| Addresses match | yes | | |

If the two routes disagree, stop. More than one pool is in view and the rest of
this document would describe the wrong one.

## 2. Pool configuration

Read via `get_market(e: Env) -> (PoolConfig, Vec<Reserve>)`, simulated, no
signature, no submission. Function signature confirmed by the Code4rena Blend V2
assessment quoting `blend-contracts-v2/pool/src/contract.rs` L412-L421, which
shows the function loading `storage::get_pool_config()` and iterating the
reserve list.

Read at ledger: `61340408`

| Reading | Value | Notes |
|---|---|---|
| Oracle address in `PoolConfig` | `CD74A3C54EKUVEGUC6WNTUPOTHB624WFKXN3IYTFJGX3EHXDXHCYMXXR` | Oracle Adapter; ia yang memanggil Reflector `CALI2BYU2JE6WVRUFYTS6MSBNEHGJ35P4AVCZYF3B6QOE3QKOB2PLE6M` |
| Backstop take rate | `2000000` (= 20%) | `bstop_rate` U32, scaled 7 desimal → 2,000,000 / 1e7 = 0.20 |
| Max positions | `6` | `max_positions` U32 |
| Other config fields present | `min_collateral` = 50000000 (i128; oracle 7-dec → $5.00); `status` = 0 (Active, deposit+borrow); `Name` = "YieldBlox"; plus instance keys `Admin`, `Backstop`, `BLNDTkn` | struct PoolConfig = {oracle, bstop_rate, max_positions, min_collateral, status} |


Applicability to February 2026: the Code4rena Blend V2 assessment (finding 18)
states that on owned pools the oracle contract address and the backstop take
rate cannot be modified by the pool owner, as a damage limit against a malicious
or compromised owner.

- [x] Confirm YieldBlox is an owned pool
      → PoolConfig instance storage has `Admin` = contract address
        (SCAddress type=contract, id 0x1B2C16AC…917CEDC0), i.e. the YieldBlox DAO
        governance contract. Non-null, non-burned admin ⇒ owned pool.
- [x] If confirmed, the oracle address read today also held in February 2026
      → oracle immutable to owner (finding 18); independently corroborated by
        contemporaneous incident reports naming the same Oracle Adapter → Reflector
        path at the time of the exploit.

Verdict: The pool's configured oracle at the incident ledger (61340408) was the
Oracle Adapter CD74A3C5…MXXR, which sources price from the Reflector oracle
CALI2BYU…LE6M. This is exactly the price path that was poisoned via SDEX
manipulation of USTRY. Backstop take rate 20%, max positions 6, status active.
YieldBlox is an owned pool, so per Code4rena finding 18 the oracle address and
backstop take rate could not have been changed by the owner between the incident
and the read; the current read therefore reflects the incident-time configuration.
Config confirmed consistent — the oracle dependency that enabled the exploit is
verified on-chain.

## 3. Oracle path

The pool may not read Reflector directly. Determine what the address from
section 2 actually is before calling anything on it.

Oracle address       : CD74A3C54EKUVEGUC6WNTUPOTHB624WFKXN3IYTFJGX3EHXDXHCYMXXR
Verified source repo : yieldblox/oracle-aggregator (fork of blend-capital/oracle-aggregator), commit 2307a9b  [R1]
Wrapped source oracle: CALI2BYU2JE6WVRUFYTS6MSBNEHGJ35P4AVCZYF3B6QOE3QKOB2PLE6M
                       Reflector / ReflectorPulse, SEP-40, decimals 14, resolution 300s  [R2][R3]

| Reading | Value | Notes |
|---|---|---|
| Contract at the oracle address is | **adapter** (oracle-aggregator) | Not Reflector core, not a bare SEP-40 feed. Wraps Reflector-like oracles via `lastprice`. [R1] |
| Functions exposed | `base()`, `decimals()`, `assets()`, `oracles()`, `asset_configs()`, `max_age()`, `lastprice(asset)`; admin: `set_admin`, `add_oracle`, `add_asset`, `add_base_asset`; `__constructor` | From src/contract.rs [R1]. NOT SEP-40 complete — missing `price()`, `prices()`, `resolution()`, `last_timestamp()`, `history_retention_period()`, `version()`, `admin()` present in the SEP-40 / Reflector interface [R2][R4] |
| `resolution()` present | **no** | Aggregator exposes no `resolution()` getter. Source oracle's resolution is only stored internally in `OracleConfig.resolution`, read once at `add_oracle` [R1]. → answer to B-6 |
| `resolution()` value | N/A on the pool's oracle (aggregator). Source Reflector = **300** (stored as `Oracles[0].resolution` in aggregator instance storage) | Not callable on the path the pool uses [R1][R5] |
| Unit of `resolution` | **seconds** | reflector-contract interface: "resolution … default tick period timeframe, in seconds — 5 minutes by default" [R2]. Confirmed by incident round timestamps 1771719600 vs 1771719300 = 300s [R6]. NOT inferred from magnitude |
| `decimals()` value | **7** (aggregator). Source Reflector = **14** | Aggregator normalizes Reflector's 14-dec price to 7 dec. From aggregator instance storage + Reflector `OracleConfig.decimals` [R1][R5] |
| Function the pool calls | **`lastprice()`** | Blend V2 pools invoke `oracle.lastprice(Asset::Stellar(addr))` + `decimals()` [R7]. Confirmed in incident execution trace [R6] |
| Records averaged, if `twap()` | **N/A — not a twap** | For USTRY (`max_dev` = 10%) the aggregator calls `prices(asset, 4)` ONLY to deviation-check the 2 most-recent rounds, then returns the single most-recent price — no averaging [R1]. Both recent rounds were poisoned (~$106.7371, ~0% deviation), so the check passed [R6] |

Effective window: **none (spot / single most-recent round).** The only time bound on the
path is the aggregator's `max_age` = **900s (15m)** staleness ceiling — a freshness cutoff,
not an averaging window [R1][R5]. Upstream, Reflector's 300s `resolution` is the feed tick
cadence, also not a consumer averaging window [R2].

Because the pool path resolves to `lastprice()` returning a single most-recent Reflector
round, there is no averaging window at all. `oracleWindowSeconds` in
`docs/api/keel-openapi.yaml` therefore does not merely lack a confirmed value — it assumes
a time-weighted-averaging mechanism the incident path never used. The parameters that DO
exist are `max_age` (900s, staleness bound) and Reflector `resolution` (300s, feed cadence);
neither is an averaging window.

This reinforces DEC-003 §3 (arbitrage asymmetry): arbitrage only corrects *executed*
mispricing, so an oracle reading *last-reported* trade prices reads exactly the unguarded
part of the market. A spot `lastprice()` with no averaging window is that exact read.

CONTRACT CHANGE (DEC-003): remove `oracleWindowSeconds` from `docs/api/keel-openapi.yaml`,
OR redefine it to reference `max_age` as a staleness bound, with an explicit note that the
incident path used spot pricing (single latest round), not averaging.

Also upgrades DEC-003 §4.2 honesty note: the claim "the oracle reads SDEX trade-derived
prices, not executable depth" is no longer only an inference. It is confirmed from source —
the pool's oracle is the aggregator, which returns Reflector (ReflectorPulse) `lastprice`,
and ReflectorPulse derives on-chain Stellar-asset prices from SDEX trades + ledger state [R2].

### References
[R1] yieldblox/oracle-aggregator, src/contract.rs + README (Last Price Method), stellar.expert-verified at commit 2307a9b — exposed fns, `max_dev` behavior, `max_age` 360–3600s bound.
[R2] reflector-network/reflector-contract, README (ReflectorPulse interface + "How It Works") — `resolution()` in seconds, 5-min default; on-chain Stellar prices sourced from SDEX trades/state.
[R3] stellar.expert contract CALI2BYU… — validation: verified vs reflector-contract, package reflector-pulse-contract, features [sep40].
[R4] SEP-40 oracle standard (stellar/stellar-protocol, ecosystem/sep-0040.md) — full interface, for the "not complete" comparison.
[R5] Aggregator instance storage (ledgerKeyContractInstance, key AAAAFA==): Decimals=7, MaxAge=900, Oracles[0]={address=CALI2BYU…, decimals=14, index=0, resolution=300}, USTRY AssetConfig max_dev=10 / oracle_index=0.
[R6] Incident execution trace TX2 3e81a3f7… (ledger 61340408) — OracleAdapter::decimals()→7, lastprice(USTRY)→1067372830, Reflector::prices([Stellar,USTRY],4)→[POISONED,POISONED,normal,normal].
[R7] Blend docs, "Selecting an Oracle" — pools require `lastprice` + `decimals`; always invoke `Asset::Stellar({contract_address})`. Oracles immutable after pool creation.

## 4. Reserve risk parameters (B-7)

Read via `get_reserve_list(e) -> Vec<Address>` then `get_reserve(e, asset) -> Reserve`
for each reserve, simulated (read-only), no signature, no submission.
[Correction: the deployed Blend V2 pool exposes NO `get_market`. The
`(PoolConfig, Vec<Reserve>)` shape is assembled from `get_config()` +
`get_reserve_list()` + `get_reserve(asset)`.]

Read at ledger: 64119285
Reserve list (8 total): idx0 XLM, idx1 USDC, idx2 CDTKPWPL…, idx3 CAUIKL3I…,
idx4 CB226ZOE…, idx5 USTRY, idx6 CAL6ER2T…, idx7 CCCRWH6Q…

| Asset | Collateral factor (c_factor) | Liability factor (l_factor) | Other fields | Notes |
|---|---|---|---|---|
| USTRY (idx 5) | **0** (0.00 — collateral DISABLED) | 9000000 (0.90) | decimals 7; enabled true; util 0.50; max_util 0.80; r_base 0.01 / r_one 0.02 / r_two 0.10 / r_three 5.00; reactivity 20; supply_cap 250,000 | c_factor=0 today = post-exploit remediation. In Feb 2026 it was >0 (attacker used USTRY as collateral). MUST reconstruct Feb value from history |
| USDC (idx 1) | 9500000 (0.95) | 9500000 (0.95) | decimals 7; enabled true; util 0.80; max_util 0.95; r_base 0.03 / r_one 0.04 / r_two 0.12 / r_three 5.00; reactivity 20; supply_cap 200,000,000 | |
| XLM (idx 0) | 7500000 (0.75) | 7500000 (0.75) | decimals 7; enabled true; util 0.50; max_util 0.70; r_base 0.01 / r_one 0.04 / r_two 0.30 / r_three 5.00; reactivity 20; supply_cap 1,000,000,000 | |

(Factors scaled 7-decimal: value / 1e7 = fraction. c_factor multiplies collateral
value; l_factor is applied to liability value. supply_cap shown in whole tokens.)

Applicability to February 2026: these reserve parameters ARE owner-mutable — set via
the admin-only `queue_set_reserve()` + `set_reserve()`. Unlike the oracle in §2, there
is NO by-construction immutability here; the deployed contract explicitly exposes
`set_reserve`. Values read today are current-only until the config history is checked.

- [x] Search the pool contract's invocation history for reserve configuration
      changes after 2026-02-22
      → USTRY c_factor = 0 today is inconsistent with the attack (the attacker
        borrowed against USTRY collateral, so c_factor was > 0 in February 2026).
        A `set_reserve`/`queue_set_reserve` for USTRY therefore MUST exist after
        2026-02-22. Locate it and record the pre-change c_factor.
- [ ] If none found, today's values also held in February 2026
      → candidate for USDC and XLM pending confirmation; does NOT hold for USTRY.
- [x] If found, record the February values separately and note the change
      → USTRY: today c_factor=0 (collateral disabled, post-exploit remediation).
        February c_factor = <pre-remediation set_reserve param>. CHANGED post-incident.

Verdict: USDC (c_factor 0.95, l_factor 0.95) and XLM (c_factor 0.75, l_factor 0.75)
read live from the Blend V2 YieldBlox pool (CCCCIQSD…) are candidate February-2026
values, pending a clean `set_reserve` history check. USTRY's live c_factor=0 is a
post-exploit remediation (collateral disabled) and does NOT represent the February
2026 configuration; its incident-time c_factor must be reconstructed from `set_reserve`
transaction history. This is the concrete counterpart to §2: unlike the immutable
oracle, these reserve parameters were changed after the incident.

## 5. Secondary sources, for context only

These are not used as inputs to `06` or `08`. They are recorded because they
disagree with each other, which is itself a limitation worth citing in
`docs/methodology/11-limitations.md`.

| Claim | Source | Date |
|---|---|---|
| Reflector is a VWAP oracle sampling SDEX trades | dev.to write-up | 2026-03-28 |
| The protocol relied on latest price, without TWAP or multi-source checks | Cryip | 2026-02-24 |
| An Oracle Adapter between Reflector and Blend returned the latest price, took no median, flagged no deviation | Rekt News | 2026-02-27 |
| Four price entries returned for USTRY: two poisoned at 106.74, two normal at 1.06 | Rekt News | 2026-02-27 |
| Price pushed to roughly 106.74, about 100x | Rekt News | 2026-02-27 |
| Sell offer placed at roughly 501 USDC per USTRY, about 500x | dev.to write-up | 2026-03-28 |
| Reflector stated its product quoted correct prices and attributed the mispricing to market illiquidity | Reflector, via press coverage | 2026-02-23 |
| Root cause was pool-operator configuration, not a Blend V2 core-contract flaw | BlockSec | 2026-02-26 |

The last two rows are the thesis of this project stated by the parties
themselves: the oracle reported what the market said, and the market could not
support the price it reported. Nobody measures that gap. That is what Keel
measures.

The 100x and 500x rows cannot both be correct. Al's own on-chain reading of
offer ID 1824788980 supersedes both.

## 6. Open items

- [ ] USTRY c_factor at ledger 61340408. Live value 0 (post-exploit remediation).
      NOT in TX2 resultMetaXdr — ResConfig is read-only during borrow, so it is not
      in the write-meta (confirmed: PoC's LEDGER_ENTRY_STATE extract shows only
      ResData/Positions, no c_factor). Authoritative source: the remediation
      `set_reserve` tx for USTRY after 2026-02-22 (LEDGER_ENTRY_STATE = Feb value),
      located via the stellar.expert Invocations tab (browser; JSON ops API 404s).
      ESTIMATE ONLY (do not finalize): inverting Blockaid's reported on-chain
      HF = 1.0985 with PoC collateral/debt and today's l_factors gives c_factor_USTRY
      ≈ 0.95–0.98 — an aggressive collateral factor for a thin-liquidity RWA. Exact
      value pending the set_reserve tx. STATUS: open.

- [ ] Confirm USDC and XLM reserve params unchanged since 2026-02-22. Same
      constraint as above: no `set_reserve` for USDC (idx1) / XLM (idx0) after
      2026-02-22 → today's values (USDC 0.95/0.95, XLM 0.75/0.75) held at the
      incident. Verify via the same Invocations-tab scan. STATUS: open, candidate
      values only.

- [x] Aggregator base() asset. RESOLVED: base = USDC
      (CCW67TSZV3SSS2HXMBQ5JFGCKJNXKZM7UQUWUZPUTHXSTZLEO7SJMI75), decimals 7.
      Prices are USDC-denominated (≈ USD, USDC being a USD stablecoin).
      Confirmed two ways: decoded from the aggregator instance storage `Base` entry,
      and the incident trace shows `lastprice(USDC) → 10000000` (= 1.0 in 7 dec),
      i.e. the base asset returns a fixed 1.0. So USTRY/XLM prices from this oracle
      are expressed in USDC units.

- [ ] DEC-003: oracleWindowSeconds removed or redefined against max_age.
      RECOMMENDATION (pending RED decision): REMOVE it. The incident path is spot
      `lastprice()` with no averaging window (§3); `oracleWindowSeconds` models a
      mechanism that does not exist. Redefining it against `max_age` (900s) conflates
      staleness with averaging — two different concepts. If a time parameter is still
      wanted, add a distinct `maxPriceAgeSeconds` mapped to the aggregator `max_age`,
      and document it as a staleness bound, NOT a window. STATUS: open — contract
      change → version bump + mock regeneration; frontend collaborator affected.

- [ ] Two-round deviation check vs manipulationCost (RED / Al only). EXTERNALLY
      CONFIRMED by Blockaid: max_dev=10% compared each new 300s window only to the
      immediately preceding one; both windows carried the poisoned price, so measured
      deviation was 0% and validation passed. Implication for manipulationCost:
      measure cost to hold the poisoned price across ≥2 consecutive 300s rounds
      (not a single trade). In-incident that cost was still ~0. STATUS: open (RED/Al).

## 7. Zone note

`docs/evidences` is YELLOW. Claude may structure and format this file and may
draft the tables, but may not supply any value in sections 1 through 4. Those
come from Al's readings only. The three-sentence rationale for that placement:
this file holds raw on-chain readings rather than derived methodology, so
formatting help carries no risk of the methodology being written by the wrong
party. Keeping it separate from `docs/context` matters because that directory
holds the SoW and may leave the repository before it goes public, and technical
evidence should not leave with it. The rejected alternative was placing this
file under `docs/context` as RED, which is safer but couples the evidence to a
directory whose future is a commercial decision rather than a technical one.
