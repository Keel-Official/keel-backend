# February USTRY/USDC pool coverage

Prepared 12 September 2026, before any real historical load. This is an inventory
of available evidence, not a certificate of complete venue coverage.

Base: USTRY `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`
(`credit_alphanum12`). Quote: USDC
`GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`
(`credit_alphanum4`). Prices use USDC/USTRY; reserves retain their asset units.

The one identified pool for this pair is
`27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb`.
Other pools appearing in a routed transaction are not automatically this pair.

| Period or observation | Available evidence | Supported use | Missing evidence |
|---|---|---|---|
| 1 February to 10 February 2026 before 16:59:35 UTC | Pair trade records and pool identity probe | Evidence of activity only | Ledger-aligned reserve history and complete pool enumeration |
| Ledger 61172481, 10 February 16:59:35 UTC | Archived command transcript of a pool trade effect: 15.4791416 USTRY, 16.3389179 USDC; fee 30 bp | A reported post-operation reserve observation for the named pool | Raw response chain with completeness and transaction ordering independently rechecked |
| After ledger 61172481 through incident ledger 61340263, 22 February 00:10:21 UTC | Transcript reports no intervening pool transaction; existing fixture carries the same reserves | Limited-confidence reconstructed reserve candidate, conditional on the transcript's continuity claim | Full retained pagination/ledger-state proof and reconciliation of intra-ledger book timing |
| After the incident through 28 February 2026 | Later trade activity and effect-walk transcript | Activity evidence; no continuous reserve series accepted here | Exact reserve states for each requested daily ledger |
| August 2026 collection observations | Current pool resource responses | Pool identification and collection-time observations | These reserves cannot be substituted for February reserves |

Sources: [pool investigation](../evidences/pool_ustry_usdc_2026-02.txt),
[filtered effect transcript](../evidences/fixed_result_reserves_null_pool.txt),
[later reserve observation](../evidences/reserves_pool.txt), and
[raw pool-ID probe](../evidences/2026-08-31-trade-pool-id-probe/pool-trades.json).
These pre-existing transcripts are weaker custody evidence than the new hashed
[operation captures](../evidences/track-b-2026-09-12/manifest.json).
Their assertions are identified here as assertions, not upgraded to fresh raw proof.

The filename containing `2026-02` does not establish coverage for every February
ledger. One discovered pool does not prove no other relevant pools existed.
The earlier trade export's empty pool-ID column is a documented decoding defect,
not evidence of pool absence; see the [investigation](../evidences/2026-08-31-trade-pool-id-defect.md).

**Load decision: no real daily combined-market result is accepted by this inventory.**
Do not convert unknown coverage into `Pools: []`, zero AMM depth, a negative flag,
or a confident band. A candidate reserve state alone cannot repair the incomplete
order book. Keep the historical request unavailable until its complete input can
be represented truthfully by the API. Limited-confidence evidence remains visible
in this report, outside the application's accepted result rows.

Before a future load, retain the exact pool list, ledger/time, reserve effects and
ordering, continuity evidence, source hashes, and acceptance decision alongside
the replay input. Align all venues to the requested ledger and methodology version.
The replay input validator checks structure and timestamp alignment; it does not
independently prove the operator's coverage assertion.

The inventory separates observations from continuity inferences so a reader can
see precisely what supports each candidate. Carrying August reserves backwards
was rejected because collection time does not establish historical state. Keeping
unsupported rows unavailable preserves uncertainty without inventing a new API field.
