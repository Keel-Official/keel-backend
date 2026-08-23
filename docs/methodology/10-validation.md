# Keel: Validation Protocol

**Methodology version:** 1.0.3-draft
**Status:** protocol defined. Results tables are empty until the runs are executed.

The SOW promises "cross-validation passed on at least 50 sample ledgers" without defining
what is validated against what. That definition is made here, because it determines how
convincing the evidence is.

Three layers, weakest to strongest. Each answers a different question, and none of them
substitutes for another.

---

## 1. Layer 1: hand recomputation

**Question answered:** is the formula correct?

Take a raw order book, transcribe it into a spreadsheet, compute depth by hand, compare
against engine output. The spreadsheet is committed to the repository as evidence.

| Property | Value |
|---|---|
| Sample size | 5 assets, chosen to span the risk range |
| Passes when | every figure matches within `Tolerance` |
| Evidence | spreadsheet files under `testdata/manual/` |
| Catches | wrong formula, wrong direction, wrong unit |
| Does not catch | correct formula applied to wrong data |

The golden fixture is Layer 1 applied to the incident state, and it was computed before
any implementation existed. That ordering is what gives it force: a fixture written after
the code merely confirms whatever the code does.

**Results**

| Asset | Ledger | Metrics compared | Result | Notes |
|---|---|---|---|---|
| | | | | |

---

## 2. Layer 2: synthetic testnet fixtures

**Question answered:** is the implementation correct?

Assets and books created on testnet with values chosen by us, so the correct answer is
known before the code runs.

| Property | Value |
|---|---|
| Sample size | 10 scenarios |
| Passes when | every scenario matches exactly |
| Evidence | fixture files plus the testnet transaction that created each |
| Catches | edge case handling, ordering bugs, fee treatment |
| Does not catch | a formula that is wrong in the same way in both places |

Scenarios that must be present, because each exercises a path no other scenario reaches:

1. Two-sided book, no pool
2. One-sided book, no pool
3. Empty book with an active pool
4. No book and no pool, giving `priceSource = none`
5. Pool priced **above** `P_target` at 2 percent, so `fromAmm` must be exactly zero
6. Two pools on the same pair
7. Book and pool diverging beyond `PriceDivergencePct`, triggering `PRICE_SOURCE_CONFLICT`
8. A target above every ask, giving `Reachable = false`
9. An active pool present, so `MaxReachablePrice` must be null
10. Monotonicity across the full delta ladder

Scenario 5 is the discriminating test for section 6 of `00-core.md`. An implementation
that sums SDEX and AMM independently returns a non-zero `fromAmm` and fails only here.

**Results**

| Scenario | Testnet tx | Expected | Actual | Result |
|---|---|---|---|---|
| | | | | |

---

## 3. Layer 3: recorded Horizon versus reconstructed history

**Question answered:** does the historical path agree with reality?

This is the layer that satisfies the SOW promise, and it requires ground truth that
cannot be created retroactively.

**Method.** A recorder captures raw Horizon snapshots for 8 selected assets every 30
minutes, storing each with its `ledgerSeq`. Later, the historical path is asked to
reproduce those same ledgers, and the two are compared.

The recorder must start on day 2 of the sprint. Ground truth for a ledger cannot be
obtained after that ledger has passed.

**Asset selection.** The 8 assets are chosen deliberately across the liquidity range: 2
highly liquid, 2 moderate, 2 thin, 2 near dormant. If every sample is a healthy asset,
the code paths that matter most, those handling assets with no executable price, are
never exercised.

**Comparison depth.** Four levels, reported separately. Aggregate agreement can hide a
discrepancy in a single large offer.

1. Number of bid and ask levels
2. Price of each level
3. Amount of each level
4. `ComputeAssetRisk` output from both sources

| Property | Value |
|---|---|
| Sample size | at least 50 pairs reported; the recorder produces thousands |
| Passes when | levels match exactly and computed figures match within `Tolerance` |
| Evidence | recordings under `recordings/samples/` plus the results table below |
| Catches | wrong reconstruction, missing events, ordering errors |
| Does not catch | a definition that is wrong in both paths |

**Mismatches are not failures.** A discrepancy that is explained correctly demonstrates
understanding of the data. A discrepancy that is never looked for demonstrates nothing.
Every mismatch is recorded with its explanation, and unexplained mismatches block the
deliverable.

**Results**

| Asset | Ledger | Levels | Prices | Amounts | Computed | Explanation |
|---|---|---|---|---|---|---|
| | | | | | | |

---

## 4. Tolerance

`Tolerance` is `0.0000001`.

It exists because some expected values do not terminate as decimals. The precision of the
computation itself is a methodology constant and is stated in `00-core.md` section 5;
until that value is finalised, this tolerance applies.

Level counts, prices expressed as rationals, and flag sets are compared **exactly**. Only
derived decimal quantities use the tolerance.

---

## 5. What none of these layers catches

If the definition itself is wrong, all three layers pass and the numbers are still wrong.
Layer 1 checks the formula against a definition. Layers 2 and 3 check the implementation
against the formula. Nothing here checks the definition against reality.

That gap is closed only by the incident backtest, where a documented methodology meets a
known outcome, and by publishing the methodology openly so others can dispute it.

---

## 6. Definition of done

- [ ] Layer 1 complete for 5 assets, spreadsheets in the repository
- [ ] Layer 2 complete for all 10 scenarios
- [ ] Layer 3 complete for at least 50 pairs
- [ ] Every mismatch recorded and explained
- [ ] The reproducibility test passes: identical input yields byte-identical JSON
- [ ] An outsider reproduces one number from the documents alone, without asking us

The final item is a promise the DoD makes and that nothing else here tests. It must be
run as an actual exercise with a real person, not assumed.

---

## 7. The case this was validated against: 22 February 2026

**Moved here in the road 1 split**, from `keel-methodology-core.md` section 11. Layer 1
of this protocol is hand recomputation, and this is the recomputation that was done.

Every figure below is derived from Horizon mainnet and reproducible without an account.

```
USTRY : GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC  credit_alphanum12
USDC  : GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN  credit_alphanum4
Pool  : 27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb  fee 30 bps
```

### Verified timeline

| Time UTC        | Event                                                                                            | Evidence                           |
| --------------- | ------------------------------------------------------------------------------------------------ | ---------------------------------- |
| 10 Feb 16:59:35 | Last pool effect before the attack. Reserves 16.3389179 USDC and 15.4791416 USTRY                | pool effect                        |
| 21 Feb 23:36:28 | Burner swaps 1 XLM for 0.1612003 USDC                                                            | op 263452928864530433              |
| 21 Feb 23:38:51 | Manipulation offer: sell 1.2185312 USTRY @ 106.7372828                                           | tx `09e1a9d1...`, offer 1824788980 |
| 21 Feb 23:39:31 | Buy order for 0.0001 USTRY @ 1.0570000                                                           | op 263453066303434753              |
| 22 Feb 00:10:21 | Manipulating trade: 5.3475699 USDC for 0.0501003 USTRY, matched against the attacker's own offer | ledger 61340263                    |
| 22 Feb 00:10:57 | Dust trade of 0.0000080 USTRY @ 1.057 between attacker accounts                                  |                                    |
| 22 Feb ~00:25   | Borrows: 1,000,196.70 USDC then 61,249,278.31 XLM                                                | secondary source                   |
| 22 Feb 22:08:33 | Next pool effect, nearly 22 hours after the attack                                               | pool effect                        |

### Arithmetic consistency

```
0.0501003 × 106.7372828  = 5.3475699 USDC     matches the amount paid
1.2185312 − 0.0501003    = 1.1684309 USTRY    matches the offer remainder today
106.7372828 / 1.057      = 100.98×            matches the reported "100x"
16.3389179 / 15.4791416  = 1.0555442          the pool's honest price
61,249,278.31 × 0.1612003 + 1,000,196.70 = 10,873,599 USDC
149,876.10 × 1.057 = 158,419 USDC real,  × 106.7372828 = 15,997,368 USDC manipulated
```

### Key findings

**Order book manipulation cost was zero, verified.** The trade list for account
`GDHRCQNC...` on 22 Feb 00:10:21 contains exactly one record, against an offer owned by
the attacker. Because the matching engine fills from the best price and every match
produces a trade record, the absence of any other record proves there were no third-party
asks anywhere between 1.057 and 106.74. This is a direct observation, not an inference.

**An honest pool was present throughout and prevented nothing.** Reserves went unchanged
for 12 days spanning the entire attack. Moving the actual market price to 106.74 would
have cost roughly 147.96 USDC. The attacker did not do that.

**What Keel would report.** Band `CRITICAL` with `bandConfidence` of `partial`. The flag
breakdown is in `testdata/fixtures/ustry_pre_exploit.md`.

### Outstanding verification

1. Both borrow transactions on the YieldBlox pool, still from secondary sources
2. The YieldBlox pool risk parameters in force at the time
3. Reflector's actual VWAP window length
4. Whether Reflector considers AMM reserves at all

---

## 8. Version history

| Version | Change |
|---|---|
| 1.0.3-draft | Initial document. Defines the three layers, the sample sizes, and what each layer does not catch |
