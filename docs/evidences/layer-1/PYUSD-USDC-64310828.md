# Layer 1 raw input: PYUSD/USDC at ledger 64310828

**THIS FILE CONTAINS NO COMPUTED VALUE OF ANY KIND.** Every figure below is a byte
Horizon returned, reproduced verbatim. There is no mid price, no spread, no depth, no
unit conversion and no rounding applied by this repository. That is deliberate and it is
the only reason this file may exist: it is the INPUT to a Layer 1 hand recomputation, and
a file that arrived carrying the answers would defeat the layer it feeds.

**Zone:** `docs/evidences/` (YELLOW). The recomputation this feeds belongs in
`testdata/manual/`, which is RED and is Al's alone.

---

## 1. Provenance

| | |
|---|---|
| Base asset | `PYUSD` |
| Base issuer | `GDQE7IXJ4HUHV6RQHIUPRJSEZE4DRS5WY577O2FY6YQ5LVWZ7JZTU2V5` |
| Base type | `credit_alphanum12` |
| Quote asset | `USDC` |
| Quote issuer | `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` |
| Recorded at | `2026-09-07T04:11:19Z` |
| Ledger before the read | `64310828` |
| Ledger after the read | `64310828` |
| Ledger consistent | `True` |
| Order book URL | `https://horizon.stellar.org/order_book?buying_asset_code=USDC&buying_asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&buying_asset_type=credit_alphanum4&limit=200&selling_asset_code=PYUSD&selling_asset_issuer=GDQE7IXJ4HUHV6RQHIUPRJSEZE4DRS5WY577O2FY6YQ5LVWZ7JZTU2V5&selling_asset_type=credit_alphanum12` |
| Order book HTTP status | `200` |
| Order book body sha256 | `d1c565d50e9120765041b50b42667621286a89154bf2c6fff3c64e8c7925964e` |
| Pools URL | `https://horizon.stellar.org/liquidity_pools?limit=200&reserves=PYUSD%3AGDQE7IXJ4HUHV6RQHIUPRJSEZE4DRS5WY577O2FY6YQ5LVWZ7JZTU2V5%2CUSDC%3AGA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` |
| Pools HTTP status | `200` |
| Pools body sha256 | `2bef70f64f61764fe730a6e701998c48e85997f68fbaf2817c7d0d54260373bb` |

`ledger_consistent` is the recorder reading the ledger sequence before and after the
request and reporting whether it moved. A recomputation against a book that straddled a
ledger boundary has no single ledger to be true at, which is why the field is here rather
than assumed.

## 2. Two traps to read before transcribing anything

**Read the price from `price_r`, never from `price`.** `price` is Horizon's decimal
rendering and it is rounded. `price_r` is the exact fraction `n/d` the offer was submitted
with. This is rule 5 of `CLAUDE.md` and it is not a style preference: two offers at
different exact prices can render to the same string. The `price` column below is present
ONLY so the rounding is visible, and it must not enter the arithmetic.

**The `amount` on a BID is not in the same unit as the `amount` on an ASK.** Horizon
denominates a bid's amount in the counter asset, USDC here, while an ask's amount is in
the base asset. `domain.Level.Amount` is defined in BASE units, so one side needs
converting and the other does not. This repository calls that `BidAmountUnit`, and trap 5
of `internal/horizon/CLAUDE.md` is where it is documented. **Derive the conversion from
`docs/methodology/`, not from this file and not from the engine.** Which side needs it,
and in which direction, is exactly the class of error Layer 1 exists to catch.

## 3. Bids, verbatim, in the order Horizon returned them

Amounts on this side are denominated in **USDC, the counter asset. See section 2**.

| # | price_r n | price_r d | price, DO NOT USE | amount |
|---|---|---|---|---|
| 0 | `10000000` | `10003879` | 0.9996123 | `1012.3410135` |
| 1 | `4998059` | `5000000` | 0.9996118 | `2563.0928207` |
| 2 | `161180593` | `161261222` | 0.9995000 | `611.1695856` |
| 3 | `1999` | `2000` | 0.9995000 | `3970.6519343` |
| 4 | `10000` | `10009` | 0.9991008 | `525.0000000` |
| 5 | `50000` | `50047` | 0.9990609 | `700.0000000` |
| 6 | `3125` | `3128` | 0.9990409 | `262.4657640` |
| 7 | `1000` | `1001` | 0.9990010 | `265.1515104` |
| 8 | `999` | `1000` | 0.9990000 | `17809.0559742` |
| 9 | `10000000` | `10013333` | 0.9986685 | `44730.0000000` |
| 10 | `9980107` | `10000000` | 0.9980107 | `8000.0001715` |
| 11 | `4967553` | `5000000` | 0.9935106 | `7999.9997548` |
| 12 | `4962553` | `5000000` | 0.9925106 | `7999.9998408` |
| 13 | `9600003` | `10000000` | 0.9600003 | `492.4787323` |
| 14 | `24` | `25` | 0.9600000 | `100.0000000` |
| 15 | `9000003` | `10000000` | 0.9000003 | `42.1029199` |
| 16 | `9` | `10` | 0.9000000 | `4.9999999` |
| 17 | `5000003` | `10000000` | 0.5000003 | `17.2516479` |
| 18 | `1` | `2` | 0.5000000 | `2.0000000` |
| 19 | `1` | `500` | 0.0020000 | `1.0000000` |
| 20 | `1` | `5000` | 0.0002000 | `1.0000000` |
| 21 | `23` | `1000000` | 0.0000230 | `1.1500000` |
| 22 | `11` | `500000` | 0.0000220 | `1.4999999` |
| 23 | `159` | `10000000` | 0.0000159 | `1.9999999` |
| 24 | `1` | `100000` | 0.0000100 | `10.0000000` |
| 25 | `3` | `2000000` | 0.0000015 | `1.0000000` |
| 26 | `1` | `2147483647` | 0.0000000 | `0.0000002` |


## 4. Asks, verbatim, in the order Horizon returned them

Amounts on this side are denominated in **the base asset**.

| # | price_r n | price_r d | price, DO NOT USE | amount |
|---|---|---|---|---|
| 0 | `10000000` | `10000341` | 0.9999659 | `187.6653856` |
| 1 | `1249999` | `1250041` | 0.9999664 | `335.7977204` |
| 2 | `9999673` | `10000000` | 0.9999673 | `1010.3110211` |
| 3 | `5000000` | `5000097` | 0.9999806 | `9004.0000001` |
| 4 | `100000000` | `100000001` | 1.0000000 | `282.9924524` |
| 5 | `1` | `1` | 1.0000000 | `4510.5762077` |
| 6 | `2501313` | `2500000` | 1.0005252 | `1309.0225001` |
| 7 | `20011` | `20000` | 1.0005500 | `2903.7366947` |
| 8 | `25017` | `25000` | 1.0006800 | `2621.1089985` |
| 9 | `501` | `500` | 1.0020000 | `20000.0000000` |
| 10 | `11904757` | `10000000` | 1.1904757 | `0.0002849` |
| 11 | `25` | `21` | 1.1904762 | `0.2199999` |
| 12 | `50` | `37` | 1.3513514 | `2.2999999` |
| 13 | `99` | `1` | 99.0000000 | `2.0000000` |
| 14 | `2147483647` | `1` | 2147483647.0000000 | `0.0000002` |


## 5. Liquidity pools for this pair, verbatim

**Pool `6493c1cdc13f71f180b05e4a1d4cb3873398e5e781dd26a8607ba5fdad7ce19e`**, fee `30` bp, type `constant_product`, total shares `3.9982978`

| reserve asset | amount |
|---|---|
| `USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` | `4.1981747` |
| `PYUSD:GDQE7IXJ4HUHV6RQHIUPRJSEZE4DRS5WY577O2FY6YQ5LVWZ7JZTU2V5` | `4.1999386` |



## 6. What to do with this file

1. Transcribe the tables above into the worksheet. The template is
   `docs/internal/layer-1-worksheet-template.md`.
2. Compute every quantity by hand from `docs/methodology/`, before looking at any engine
   output.
3. Write the result into `testdata/manual/`, naming this asset's issuer and this ledger
   sequence. `scripts/check-manual-recomputation.sh` requires both and gates on them.
4. Then ask for the engine's figures at this same ledger and compare. Where they disagree,
   the disagreement is the finding: adjust the code, never these numbers.
