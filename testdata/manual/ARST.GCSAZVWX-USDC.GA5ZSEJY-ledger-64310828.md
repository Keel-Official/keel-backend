# Layer 1 hand recomputation: ARST/USDC at ledger 64310828

> Empty worksheet. Contains NO computed value of any kind.
> Every formula is owned by a file in `docs/methodology/` and is CITED here, never restated.
> Source input: `docs/evidences/layer-1/ARST-USDC-64310828.md`.
> Note: this is the SPARE asset (29 levels). If used in place of EURC, write in the file that the deep end went unrepresented.

---

## Header (two GATED fields — a file missing either is not counted at all)

| Field | Gated | Value |
|---|---|---|
| Base asset | — | ARST |
| Issuer address (G + 55 chars, INSIDE the file) | **YES** | `GCSAZVWXZKWS4XS223M5F54H2B6XPIIXZZGP7KEAIU6YSL5HDRGCI3DG` |
| Quote asset | — | USDC |
| Quote issuer | — | `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` |
| **Ledger:** (word `Ledger` + ≥5 digits) | **YES** | 64310828 |
| Source / provenance | reported | Horizon order_book + liquidity_pools, recorded 2026-09-07T04:11:20Z |
| Date | reported | 2026-09-__ (ISO `YYYY-MM-DD`) |

**Computed BEFORE any engine output for this ledger was read: yes / no** → `____`
*(Answer honestly. If you ever saw the engine first, write no.)*

---

## Section 1 — Verbatim transcription (evidence, from `price_r`, NEVER from `price`)

### 1a. Bids — amount denominated in USDC (counter asset), see Section 2 (6 levels)

| # | price_r n | price_r d | amount (USDC) |
|---|---|---|---|
| 0 | | | |
| 1 | | | |
| 2 | | | |
| 3 | | | |
| 4 | | | |
| 5 | | | |

### 1b. Asks — amount denominated in the base asset (ARST) (23 levels)

| # | price_r n | price_r d | amount (ARST) |
|---|---|---|---|
| 0 | | | |
| 1 | | | |
| 2 | | | |
| 3 | | | |
| 4 | | | |
| 5 | | | |
| 6 | | | |
| 7 | | | |
| 8 | | | |
| 9 | | | |
| 10 | | | |
| 11 | | | |
| 12 | | | |
| 13 | | | |
| 14 | | | |
| 15 | | | |
| 16 | | | |
| 17 | | | |
| 18 | | | |
| 19 | | | |
| 20 | | | |
| 21 | | | |
| 22 | | | |

### 1c. Liquidity pool (one pool — exercises non-negotiable rule 4)

| Field | Value |
|---|---|
| Pool id | |
| Fee (bp) | |
| Type | |
| Total shares | |
| Reserve ARST | |
| Reserve USDC | |

---

## Section 2 — The unit question (write it BEFORE using it) — the most productive trap

Which side needs converting to BASE units, and in which direction?
Derive it from `docs/methodology/`, NOT from the input file, NOT from `internal/horizon/CLAUDE.md`, NOT from the engine.

- Side that is converted: `____`
- Direction of conversion (formula, cited): `____`
- Where the methodology establishes this (file + section): `____`

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
  -recording docs/evidences/layer-1/raw/ARST-USDC-64310828.json.gz \
  -after-writing testdata/manual/ARST.GCSAZVWX-USDC.GA5ZSEJY-ledger-64310828.md
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
