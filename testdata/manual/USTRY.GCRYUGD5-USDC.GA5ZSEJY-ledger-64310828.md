# Layer 1 hand recomputation: USTRY/USDC at ledger 64310828

> Empty worksheet. Contains NO computed value of any kind.
> Every formula is owned by a file in `docs/methodology/` and is CITED here, never restated.
> Source input: `docs/evidences/layer-1/USTRY-USDC-64310828.md`.
> Note: the incident asset in a NORMAL state — the control on the golden fixture.

---

## Header (two GATED fields — a file missing either is not counted at all)

| Field | Gated | Value |
|---|---|---|
| Base asset | — | USTRY |
| Issuer address (G + 55 chars, INSIDE the file) | **YES** | `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` |
| Quote asset | — | USDC |
| Quote issuer | — | `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` |
| **Ledger:** (word `Ledger` + ≥5 digits) | **YES** | 64310828 |
| Source / provenance | reported | Horizon order_book + liquidity_pools, recorded 2026-09-07T04:11:22Z |
| Date | reported | 2026-09-__ (ISO `YYYY-MM-DD`) |

**Computed BEFORE any engine output for this ledger was read: yes / no** → **yes**
*(Answer honestly. If you ever saw the engine first, write no.)*

---

## Section 1 — Verbatim transcription (evidence, from `price_r`, NEVER from `price`)

### 1a. Bids — amount denominated in USDC (counter asset), see Section 2 (15 levels)

| # | price_r n | price_r d | price, DO NOT USE | amount |
|---|---|---|---|---|
| 0 | `1069382025` | `996115216` | 1.0735525 | `5387.1461901` |
| 1 | `1624634547` | `1513735037` | 1.0732622 | `55224.9265533` |
| 2 | `2145111` | `2000000` | 1.0725555 | `42.2442513` |
| 3 | `1330668590` | `1241014809` | 1.0722423 | `210980.6689045` |
| 4 | `10715311` | `10000000` | 1.0715311 | `450.0000000` |
| 5 | `53` | `50` | 1.0600000 | `30.4870374` |
| 6 | `102549237` | `100000000` | 1.0254924 | `87.5518336` |
| 7 | `2500000` | `2840909` | 0.8800000 | `37.3578000` |
| 8 | `59` | `2500` | 0.0236000 | `19.0000000` |
| 9 | `189` | `10000` | 0.0189000 | `5.0000000` |
| 10 | `53` | `10000` | 0.0053000 | `2.9999999` |
| 11 | `3899` | `10000000` | 0.0003899 | `1.9999999` |
| 12 | `1389` | `5000000` | 0.0002778 | `400.0000000` |
| 13 | `1` | `3600` | 0.0002778 | `3.0000000` |
| 14 | `1` | `2147483647` | 0.0000000 | `0.0000002` |

### 1b. Asks — amount denominated in the base asset (USTRY) (11 levels)

| # | price_r n | price_r d | price, DO NOT USE | amount |
|---|---|---|---|---|
| 0 | `21481769` | `20000000` | 1.0740885 | `17.4018079` |
| 1 | `1448484923` | `1348570099` | 1.0740895 | `4732.8636190` |
| 2 | `11105875` | `10337432` | 1.0743360 | `51654.4642287` |
| 3 | `449960972` | `418429649` | 1.0753563 | `195324.4235526` |
| 4 | `109` | `100` | 1.0900000 | `88.5084729` |
| 5 | `112213563` | `100000000` | 1.1221356 | `100.0000000` |
| 6 | `113` | `100` | 1.1300000 | `87.4930682` |
| 7 | `77058467` | `1000000` | 77.0584670 | `99.9999962` |
| 8 | `266843207` | `2500000` | 106.7372828 | `1.1684309` |
| 9 | `400058467` | `1000000` | 400.0584670 | `113.5163207` |
| 10 | `2147483647` | `1` | 2147483647.0000000 | `0.0000004` |

### 1c. Liquidity pool (one pool — exercises non-negotiable rule 4)

**Pool `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb`**, fee `30` bp, type `constant_product`, total shares `15.8497241`

| reserve asset | amount |
|---|---|
| `USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` | `16.5869899` |
| `USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` | `15.4481700` |

---

## Section 2 — The unit question (write it BEFORE using it) — the most productive trap

Which side needs converting to BASE units, and in which direction?
Derive it from `docs/methodology/`, NOT from the input file, NOT from `internal/horizon/CLAUDE.md`, NOT from the engine.

- Side that is converted: side **bid** only.
- Direction of conversion (formula, cited): **amount_base = amount_quote ÷ price, where price is quote-per-base taken from price_r = n/d
- Where the methodology establishes this (file + section): 00-overview.md, 03-reference-price.md and 04-depth.md.

---

## Section 3 — Reference price — `docs/methodology/03-reference-price.md`

*(Remember: a name ending in `Pct` is a percentage; an input written δ is a fraction. `docs/methodology/README.md` section 2.)*

| Quantity | Value | Working | Methodology ref |
|---|---|---|---|
| Best bid | | | |
| Best ask | | | |
| P0 | | | |
| priceSource | | | |
| spreadPct | | | |

---

## Section 4 — Depth, both sides SEPARATELY, at each δ — `docs/methodology/04-depth.md`

*(FR-4: report the two sides separately. Only levels whose price falls inside the target band contribute, plus the one boundary level that fills partially. Derive the band edges from the methodology.)*

### 4a. Per side

| δ | Depth bid | Depth ask | Working | Ref |
|---|---|---|---|---|
| | | | | |
| | | | | |

### 4b. The table that matters most: the combination — SDEX and AMM are combined through a shared marginal price limit and are NOT summed separately (non-negotiable rule 4)

Rule in your own words: `____`

| δ | from SDEX | from AMM | combined | Reason they are not added independently |
|---|---|---|---|---|
| | | | | |
| | | | | |

---

## Section 5 — Manipulation cost, collateral, flags, band — `05-`, `08-`, `09-`

*(Three things that slip: (1) cost is the notional paid to OTHER parties; `Reachable` is a separate claim. (2) `maxReachablePrice` is **null** when an active pool is present — this asset has one; if you compute a value, one of the two is wrong = a finding. (3) Zero ≠ absent: write "zero + reason" or "unevaluated + what is missing".)*

| Quantity | Value | Working | Ref |
|---|---|---|---|
| Manipulation cost | | | |
| Reachable | | | |
| maxReachablePrice | | | |
| Collateral | | | |
| Flags | | | |
| Band | | | |

---

## Section 6 — Prove the file is readable (BEFORE looking at the engine)

```bash
make manual-check
```
Read the row for this file: yes/no under each of the four fields. Exit 0 = pass. Exit 2 is NOT a pass.

---

## Section 7 — Only now ask the engine

```bash
go run ./cmd/keel layer1 \
  -recording docs/evidences/layer-1/raw/USTRY-USDC-64310828.json.gz \
  -after-writing testdata/manual/USTRY.GCRYUGD5-USDC.GA5ZSEJY-ledger-64310828.md
```

---

## Section 8 — Comparison against the engine (Tolerance `0.0000001`, from `10-validation.md` section 4)

*(Level counts, prices expressed as rationals, and flag sets are compared EXACTLY; only derived decimal quantities use the tolerance.)*

| # | Quantity | My figure | Engine figure | Agree within Tolerance? |
|---|---|---|---|---|
| 1 | Bid level count | | | |
| 2 | Ask level count | | | |
| 3 | Best bid | | | |
| 4 | Best ask | | | |
| 5 | P0 | | | |
| 6 | spreadPct | | | |
| 7 | Depth bid @δ | | | |
| 8 | Depth ask @δ | | | |
| 9 | Combined depth @δ | | | |
| 10 | Manipulation cost | | | |
| 11 | maxReachablePrice | | | |
| 12 | Flags | | | |

---

## Section 9 — When a figure disagrees

The disagreement IS the finding — it is not resolved by editing either side.

1. Record it: your figure and the engine's, side by side.
2. Commit the file with the disagreement still in it. That is evidence, not a failure.
3. Hand it over. The code is adjusted to match your numbers, NEVER your numbers to the code.
4. If the transcription turns out to be what was wrong, that is also a finding and is still recorded.

Finding: `____`
