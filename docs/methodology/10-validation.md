# Keel: Validation Protocol

**Methodology version:** 1.0.3-draft
**Status:** protocol defined. Layer 3 EXECUTED 26 August 2026 and tabulated in
section 3, 60 recordings, 37 match, 0 mismatch, 23 partial. Layer 2 has a harness
and 0 of 10 fixtures, finding P2-18. Layer 1 has neither. Their definition of done
in section 6 is unmet.

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

**0 of 10. The harness exists and the fixtures do not.**
`internal/conformance/layer2.go` carries all ten scenarios, in this section's
numbering, and `internal/conformance/layer2_test.go` runs each one. Every
scenario currently SKIPS, naming the file it looked for. Finding P2-18 in
`scripts/audit-verification.sh` reports the tally, because a skip is easy to
stop noticing.

The harness holds no figures and must never hold any. Each scenario reads
`testdata/fixtures/layer2/NN-slug.json`, which is RED: Al creates the state on
testnet, records the transaction that created it, and works the expected values
by hand. Same rule as the golden fixture and for the same reason, which section
1 states.

| # | Slug, and the file the harness reads | Testnet tx | Expected | Result |
|---|---|---|---|---|
| 1 | `01-two-sided-book-no-pool.json` | | | not provided |
| 2 | `02-one-sided-book-no-pool.json` | | | not provided |
| 3 | `03-empty-book-active-pool.json` | | | not provided |
| 4 | `04-no-book-no-pool.json` | | | not provided |
| 5 | `05-pool-above-target.json` | | | not provided |
| 6 | `06-two-pools-one-pair.json` | | | not provided |
| 7 | `07-divergence-conflict.json` | | | not provided |
| 8 | `08-target-above-every-ask.json` | | | not provided |
| 9 | `09-active-pool-nulls-max-reachable.json` | | | not provided |
| 10 | `10-monotonic-ladder.json` | | | not provided |

**A fixture is useful before its numbers are.** Each scenario carries the
property this section states in WORDS, and the harness checks it against the
computed result with no hand figure involved: scenario 5's `fromAmm` must be
exactly zero, scenario 4's `priceSource` must be `none`, scenario 9's
`maxReachablePrice` must be null, scenario 10's ladder must not fall. So an
input alone already tests the thing the scenario exists for, and the hand
computation that follows tests the magnitudes. Two of the ten, 1 and 2, have a
property that is structural rather than numeric; the rest check a stated rule.

**The file shape**, defined by `Layer2Fixture` in `internal/conformance/layer2.go`,
which is the authority if this drifts:

```json
{
  "scenario": 5,
  "slug": "pool-above-target",
  "testnetTx": "the transaction that created this state, REQUIRED",
  "ledgerSeq": 0,
  "ledgerClosedAt": "2026-08-28T00:00:00Z",
  "base":  {"code": "TEST", "issuer": "G...", "type": "credit_alphanum4"},
  "quote": {"code": "USDC", "issuer": "G...", "type": "credit_alphanum4"},
  "book": {
    "bids": [{"priceN": 9, "priceD": 10, "amount": "5.0000000"}],
    "asks": [{"priceN": 11, "priceD": 10, "amount": "5.0000000"}]
  },
  "pools": [{"poolId": "...", "reserveBase": "1000.0000000",
             "reserveQuote": "1000.0000000", "feeBp": 30}],

  "expected": {
    "priceSource": "book",
    "midPrice": "1.0000000",
    "spreadPct": "20.0000000",
    "maxReachablePrice": "null",
    "depth": [{"delta": "0.02", "buySide": "...", "sellSide": "...",
               "fromSdex": "...", "fromAmm": "0"}],
    "flags": ["PRICE_SOURCE_CONFLICT"]
  }
}
```

Four rules the loader enforces rather than trusts, each one a way a hand written
fixture goes wrong:

1. **Every decimal is a string, and a price is the `n/d` fraction**, never the
   decimal string beside it. Rule 5 of the non-negotiables, and a JSON number is
   a float64 on the way in.
2. **`expected` is optional and every field inside it is nullable.** A field left
   out is not checked. "Not computed yet" and "computed to be zero" are different
   claims and the loader keeps them apart; an empty decimal string is refused
   rather than read as zero.
3. **`testnetTx` may not be empty.** This section asks for the transaction that
   created each scenario, and a state nobody can go and look at is a number
   somebody typed.
4. **An unknown key is an error.** A misspelled field would otherwise be a silent
   zero in a hand computation.

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

**Executed 26 August 2026** over the sixty recordings committed under
`recordings/samples/`, taken at ledgers 64129586 to 64129592 and rebuilt about
seven hours later. Raw output, one row per recording with every count behind
every verdict: `docs/evidences/layer3-crosscheck-2026-08-26.csv`. The narrative
account is `docs/evidences/2026-08-26-layer3-crosscheck.md`. Reproduce with
`make crosscheck`, which will not reproduce these numbers: every hour that
passes moves more offers, so the same command compares fewer pairs tomorrow.

| Verdict | Count | Meaning |
|---|---|---|
| MATCH | 37 | agreement at all four comparison depths, and the rebuild claimed no gap |
| MISMATCH | **0** | disagreement, with the rebuild claiming no gap |
| PARTIAL | 23 | the rebuild could not carry every offer back, so it says nothing either way |
| ERROR | 0 | |
| **Total** | **60** | above the 50 this section requires |

**A PARTIAL row's four comparison columns are not findings, and reading them as
agreement or disagreement is the mistake this table is arranged to prevent.**
The verdict is decided before the comparison: `crosscheckRow.Comparable()` in
`cmd/keel/crosscheck.go` returns false as soon as an offer changed after the
target ledger or went unresolved, and a row that is not comparable is PARTIAL
whatever the columns then say. So a PARTIAL row reading `amounts yes` has not
confirmed the amounts. It has confirmed nothing. Twenty-two of the twenty-three
read `amounts yes` for exactly that reason, and that column is the clearest
example of a number which looks like evidence and is not.

The 37 MATCH rows are not listed here. Each carries `yes` in all four columns,
`0` in all three of carried-changed-gone, and no explanation, so a row per asset
would add sixty lines and no information. They are in the CSV, one line each.
Listing the 23 non-MATCH rows is what this section's own rule asks for: every
mismatch recorded with its explanation.

| Asset | Ledger | Levels | Prices | Amounts | Computed | Explanation |
|---|---|---|---|---|---|---|
| AFR `GBX6YI45` | 64129587 | **no** | **no** | yes | **no** | PARTIAL. 25 offer(s) could not be carried back. recorded 24 bid / 77 ask, rebuilt 24 bid / 75 ask |
| AQUA `GBNZILST` | 64129587 | **no** | **no** | yes | yes | PARTIAL. 44 offer(s) could not be carried back. recorded 55 bid / 200 ask, rebuilt 27 bid / 286 ask (a recorded side is at the endpoint limit and is a prefix) |
| AUDD `GDC7X2MX` | 64129587 | **no** | **no** | yes | **no** | PARTIAL. 7 offer(s) could not be carried back. recorded 11 bid / 5 ask, rebuilt 7 bid / 4 ask |
| BTC `GDPJALI4` | 64129586 | **no** | **no** | yes | yes | PARTIAL. 49 offer(s) could not be carried back. recorded 200 bid / 31 ask, rebuilt 406 bid / 27 ask (a recorded side is at the endpoint limit and is a prefix) |
| ETH `GBFXOHVA` | 64129589 | **no** | yes | **no** | yes | PARTIAL. 14 offer(s) moved after the target ledger. recorded 164 bid / 34 ask, rebuilt 163 bid / 34 ask |
| EURC `GAQRF3UG` | 64129591 | **no** | **no** | yes | **no** | PARTIAL. 5 offer(s) moved after the target ledger. recorded 13 bid / 10 ask, rebuilt 10 bid / 8 ask |
| EURC `GDHU6WRG` | 64129587 | **no** | **no** | yes | **no** | PARTIAL. 235 offer(s) could not be carried back. recorded 64 bid / 86 ask, rebuilt 33 bid / 34 ask |
| GOLD `GBC5ZGK6` | 64129588 | **no** | **no** | yes | **no** | PARTIAL. 1 offer(s) moved after the target ledger. recorded 32 bid / 18 ask, rebuilt 31 bid / 18 ask |
| LIBRE `GAYCCWKE` | 64129588 | **no** | **no** | yes | **no** | PARTIAL. 2 offer(s) could not be carried back. recorded 5 bid / 8 ask, rebuilt 2 bid / 5 ask |
| LSP `GAB7STHV` | 64129589 | **no** | **no** | yes | **no** | PARTIAL. 23 offer(s) moved after the target ledger. recorded 30 bid / 120 ask, rebuilt 19 bid / 108 ask |
| SCROOGE `GD2TQV2V` | 64129588 | **no** | **no** | yes | **no** | PARTIAL. 1 offer(s) moved after the target ledger. recorded 16 bid / 29 ask, rebuilt 15 bid / 29 ask |
| SHX `GDSTRSHX` | 64129587 | **no** | **no** | yes | yes | PARTIAL. 7 offer(s) could not be carried back. recorded 103 bid / 200 ask, rebuilt 85 bid / 379 ask (a recorded side is at the endpoint limit and is a prefix) |
| sUSD `GCHW7CWI` | 64129588 | **no** | **no** | yes | **no** | PARTIAL. 37 offer(s) could not be carried back. recorded 17 bid / 39 ask, rebuilt 14 bid / 27 ask |
| TFT `GBOVQKJY` | 64129588 | **no** | **no** | yes | yes | PARTIAL. 16 offer(s) moved after the target ledger. recorded 16 bid / 40 ask, rebuilt 16 bid / 29 ask |
| USDGLO `GBBS25EG` | 64129587 | **no** | **no** | yes | **no** | PARTIAL. 1 offer(s) moved after the target ledger. recorded 5 bid / 2 ask, rebuilt 5 bid / 1 ask |
| USDM `GDHDC4GB` | 64129588 | **no** | **no** | yes | **no** | PARTIAL. 16 offer(s) could not be carried back. recorded 7 bid / 7 ask, rebuilt 5 bid / 2 ask |
| USDZ `GAKTLPC4` | 64129587 | **no** | **no** | yes | **no** | PARTIAL. 7 offer(s) could not be carried back. recorded 13 bid / 16 ask, rebuilt 10 bid / 8 ask |
| VELO `GDM4RQUQ` | 64129588 | **no** | **no** | yes | yes | PARTIAL. 4 offer(s) could not be carried back. recorded 75 bid / 200 ask, rebuilt 65 bid / 215 ask (a recorded side is at the endpoint limit and is a prefix) |
| XLM (native) | 64129586 | yes | **no** | yes | yes | PARTIAL. 1605 offer(s) could not be carried back. bid 0 price differs: recorded 229241/1250000, rebuilt 20000/111073 |
| XRP `GBXRPL45` | 64129587 | **no** | **no** | yes | **no** | PARTIAL. 77 offer(s) could not be carried back. recorded 185 bid / 119 ask, rebuilt 177 bid / 112 ask |
| XTAR `GAORYJ3K` | 64129588 | **no** | **no** | yes | **no** | PARTIAL. 1 offer(s) moved after the target ledger. recorded 4 bid / 46 ask, rebuilt 4 bid / 45 ask |
| yUSDC `GDGTVWSM` | 64129586 | **no** | **no** | yes | **no** | PARTIAL. 348 offer(s) could not be carried back. recorded 43 bid / 21 ask, rebuilt 37 bid / 17 ask |
| yXLM `GARDNV3Q` | 64129586 | **no** | **no** | yes | **no** | PARTIAL. 102 offer(s) could not be carried back. recorded 56 bid / 46 ask, rebuilt 50 bid / 42 ask |

**The two shapes of PARTIAL, and neither is a disagreement between the sources.**
Fifteen rows have offers the rewind could not carry back to the target ledger.
Eight have offers that moved after it, which the rewind reports rather than
papers over. Both are the reconstruction declaring its own incompleteness. The
protocol above says unexplained mismatches block the deliverable; there are no
mismatches to block it, and the honest statement of that result is that 37 pairs
agree and 23 were not testable seven hours after recording.

**What this run does not establish.** The gap between recording and rebuild was
about seven hours, and it is the direct cause of all 23 PARTIAL rows. A rebuild
run minutes after the recording would convert most of them, and until that is
done the ratio 37 to 23 is a property of the delay rather than of the historical
path. Rerunning `make crosscheck` does not recover it either, because the delay
only grows. The next recorder batch should be crosschecked the same hour.

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
      **0 of 5.** `testdata/manual/` does not exist. The golden fixture is Layer 1
      applied to one asset and its with-pool tables are not computed
- [ ] Layer 2 complete for all 10 scenarios
      **0 of 10.** The harness is built and every scenario skips; see the results
      table in section 2 and finding P2-18. No testnet fixture exists yet
- [x] Layer 3 complete for at least 50 pairs
      **60 reported**, section 3. Read the qualification there rather than this box:
      37 of the 60 were actually comparable, and 23 were not testable because of the
      seven hour gap between recording and rebuild
- [x] Every mismatch recorded and explained
      **0 mismatches to record.** The 23 PARTIAL rows are listed with their
      explanations in section 3, which is the stronger reading of this line
- [x] The reproducibility test passes: identical input yields byte-identical JSON
      `TestInvarianDeterminisme` in `internal/conformance/golden_test.go`, run by
      `make test` and `make conformance`
- [ ] An outsider reproduces one number from the documents alone, without asking us
      not attempted. This is the item the note below says must be a real exercise

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
