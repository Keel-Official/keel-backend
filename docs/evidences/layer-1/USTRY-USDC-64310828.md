# Layer 1 raw input: USTRY/USDC at ledger 64310828

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
| Base asset | `USTRY` |
| Base issuer | `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` |
| Base type | `credit_alphanum12` |
| Quote asset | `USDC` |
| Quote issuer | `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` |
| Recorded at | `2026-09-07T04:11:22Z` |
| Ledger before the read | `64310828` |
| Ledger after the read | `64310828` |
| Ledger consistent | `True` |
| Order book URL | `https://horizon.stellar.org/order_book?buying_asset_code=USDC&buying_asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&buying_asset_type=credit_alphanum4&limit=200&selling_asset_code=USTRY&selling_asset_issuer=GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC&selling_asset_type=credit_alphanum12` |
| Order book HTTP status | `200` |
| Order book body sha256 | `e09a9537fbe8ea97973cb87d32498b1cadf23a77f1f44fbb3df16c6f657ca157` |
| Pools URL | `https://horizon.stellar.org/liquidity_pools?limit=200&reserves=USTRY%3AGCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC%2CUSDC%3AGA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` |
| Pools HTTP status | `200` |
| Pools body sha256 | `2a0f4298846446946faba2fad3991b19a7743a5042dcbc63238475903130a6c2` |

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


## 4. Asks, verbatim, in the order Horizon returned them

Amounts on this side are denominated in **the base asset**.

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


## 5. Liquidity pools for this pair, verbatim

**Pool `27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb`**, fee `30` bp, type `constant_product`, total shares `15.8497241`

| reserve asset | amount |
|---|---|
| `USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN` | `16.5869899` |
| `USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` | `15.4481700` |



## 6. What to do with this file

1. Transcribe the tables above into the worksheet. The template is
   `docs/internal/layer-1-worksheet-template.md`.
2. Compute every quantity by hand from `docs/methodology/`, before looking at any engine
   output.
3. Write the result into `testdata/manual/`, naming this asset's issuer and this ledger
   sequence. `scripts/check-manual-recomputation.sh` requires both and gates on them.
4. Then ask for the engine's figures at this same ledger and compare. Where they disagree,
   the disagreement is the finding: adjust the code, never these numbers.
