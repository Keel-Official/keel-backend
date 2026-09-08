# Verifying the SOW's USDY figures, and the ticker does not resolve to an asset

**Read on:** 2026-09-08, from public Horizon mainnet with no account.
**Ledger bracket:** `64327429` at `2026-09-08T05:55:51Z` before the first read,
`64327504` at `2026-09-08T06:02:47Z` after the last. The `/assets` page reports
`Latest-Ledger` `64327439`.
**MethodologyVersion in force:** `1.1.0-draft`. **No methodology was run here.** Every
figure below is a Horizon reading or a sum of Horizon readings, and none of it passed
through `internal/domain`.

**What this closes.** The SOW's Week 1 planned work includes "Verify current stablecoin
data", and section 3 of the SOW says of its own USDY figures that "these figures will be
verified again during Week 1". That activity had no trace in this repository on
8 September 2026: `USDY` appeared zero times in `docs/` and zero times in `configs/`.
This document is that verification, nineteen days late, and it is the last unevidenced
line in Week 1.

**The figures the SOW states, quoted rather than paraphrased:**

> "For example, USDY was reported at approximately $109.58 million in supply, 684
> holders, and only $191 in 24-hour volume as of February 26, 2026."

---

## 1. The result, before the reasoning

| The SOW's figure, 26 February 2026 | Read today, 8 September 2026 | Direction |
|---|---|---|
| ~$109.58 million supply | `467,502,151.6966762` USDY, about 528 million USDC at the price in section 4 | up about 4.8x |
| 684 holders | `2,714` authorized trustlines, plus 44 contract holders | up about 4.0x |
| $191 in 24-hour volume | `58.5641351` USDY, about **66.21 USDC** | **down about 65 per cent** |

**The structural risk the SOW pointed at has got worse rather than better.** Supply and
holder count both roughly quadrupled while the executable market shrank. That is the
exact shape this engine exists to measure, and section 3 is the part a price feed cannot
see.

**But the figures above carry a caveat that is larger than the figures**, and it is
section 2.

---

## 2. `USDY` is not an asset. It is 37 assets

`GET /assets?asset_code=USDY` returns **37 issuers** at ledger `64327439`. Full survey:
`usdy-issuers.txt` and `usdy-issuers.json` in this directory, produced by
`go run ./cmd/keel universe -codes USDY`, which proposes and never selects.

The SOW names the ticker and no issuer. `CLAUDE.md` rule for this repository is that an
asset is the pair (code, issuer) and is never matched on the ticker, so **the SOW's
sentence does not by itself identify the asset it is about.** Everything in section 1 is
therefore conditional on the choice made in section 2.2, and that choice is stated rather
than assumed.

### 2.1 SEP-1 verification does not mean what a reader expects

`keel universe` performs the two-way check: the account named a home domain, and that
domain's `stellar.toml` names this exact (code, issuer) pair. Fifteen of the 37 pass it.
Here is what passing looks like:

| Issuer | Trustlines | Total supply | Home domain |
|---|---|---|---|
| `GD4QWEIY` | 2 | 840,000,000,000 | `treasury.dtcc.company` |
| `GDAEH2FU` | 8 | 740,000,000,000 | `ondo.dtcc.markets` |
| `GD4NLHSE` | 8 | 640,000,000,000 | `stellar.dtcc.network` |
| `GDNSYIT7` | 8 | 420,000,000,000 | `rwa.spacexai.money` |
| `GBHTCMYO` | 19 | 90,000,000,000 | `onxlm.com` |
| `GCCASO3P` | 14 | 1,110,000,000 | `mastercard.co.com` |
| `GBQONCEL` | 17 | 999,995,999 | `franklintempleton.co.com` |
| `GABKEFZM` | 9 | 1,000,000,000 | `JPmorganchase.co.com` |
| `GAW27CT2` | 14 | 1,000,000,000 | `Grayscale.co.com` |
| `GCWQQYWL` | 4 | 62,000,000 | `stellar.org.im` |
| `GC2ETXXA` | 4 | 30,000,000 | `kucoin.com.ro` |
| `GBUF7CHI` | 6 | 40,000,000 | `bybit.com.ro` |

Every row there is VERIFIED under SEP-1. Every row there also carries a domain that
reads as an imitation of an institution, and supplies in the hundreds of billions held
across single-digit trustline counts.

**This is not a defect in the check. It is the check's documented limit, and the header of
`cmd/keel/stellartoml.go` states it in advance:** a `stellar.toml` proves that whoever
controls the domain listed this pair, and `home_domain` proves that whoever controls the
account typed a domain in. Together they prove the two operators agree about each other.
**Neither proves the domain is who it says it is.** No amount of SEP-1 conformance
distinguishes `franklintempleton.co.com` from Franklin Templeton, and this survey is the
worked example of that gap on a live ticker.

### 2.2 The only issuer with a market does NOT pass the check

| Field | Value | Source |
|---|---|---|
| Issuer | `GAJMPX5NBOG6TQFPQGRABJEEB2YE7RFRLUKJDZAZGAD5GFX4J7TADAZ6` | `/assets` |
| Asset type | `credit_alphanum4` | `/assets` |
| Authorized trustlines | `2,714` | `/assets`, `accounts.authorized` |
| In trustlines | `461,621,639.3628924` | `/assets`, `balances.authorized` |
| In liquidity pools | `262.0651156` across 42 pools | `/assets`, `liquidity_pools_amount` |
| In contracts | `5,880,250.2686682` across 44 contracts | `/assets`, `contracts_amount` |
| In claimable balances | `0` | `/assets` |
| **Total supply** | **`467,502,151.6966762`** | the four rows above, summed |
| `home_domain` | `ondo.finance` | `/accounts/{issuer}` |
| Verification | **`TOML_UNREACHABLE`** | see below |
| `auth_revocable` | `true` | `/assets`, flags |
| `auth_clawback_enabled` | `true` | `/assets`, flags |

`https://ondo.finance/.well-known/stellar.toml` **could not be read from this network,
twice**: once by `keel universe` and once by a direct `curl` with a 45 second timeout.
Both attempts ended in a connection timeout while awaiting headers, not in a DNS failure
and not in a 404.

**Unreachable is not absent.** A timeout from one network says nothing about whether the
document exists, and a reader elsewhere may well fetch it. That is why this is recorded
as the exact error rather than as a verdict, and why the row above says
`TOML_UNREACHABLE` and not "unverified issuer". What can be said is narrower and it is
enough for this document: **on 8 September 2026, from here, the identity of the only USDY
with a market could not be confirmed in the direction SEP-1 asks for.**

Second place is worth one line, because it shows what the ticker attracts:
`GCLFMBAFL7RTZAO5VYDAILKUVD7BDPCDIGVURKUDC5QLRCMXQN7EONDO`, 293 trustlines, home domain
`finance-ondo.com`, total supply `922,285,398,685`, and its TLS certificate is valid for
`*.web-hosting.com` and not for the domain serving it.

**Section 1 uses `GAJMPX5N` and nothing else.** The basis is stated so it can be
disputed: it is the only one of the 37 with a liquidity pool count above three, the only
one with contract holders, the only one whose trustline count is in four figures, and the
only one whose home domain is the issuer's real domain rather than a variant of it. It is
a judgement about which asset the SOW meant, not a fact the SOW supplies.

**What this cannot settle, and it is Al's to close, not Claude's.** Which issuer the
funder meant is a question for the Ambassador Chapter Lead. If it is a different one, the
figures in section 1 belong to a different asset and this document is the reason to ask
rather than the answer.

---

## 3. Where the liquidity actually is, and it is the finding

**Order book against USDC, the global quote asset under DEC-015: `0` bids and `0` asks.**
`bodies/orderbook-USDY-USDC.json`, read at the ledger bracket above. There is no order
book at all for this asset against the asset this repository prices everything in.

**Order book against XLM: 46 bids and 19 asks.** `bodies/orderbook-USDY-XLM.json`.

**Every one of the 42 liquidity pools, and the largest holds 70 USDY.**
`bodies/pools-USDY-GAJMPX5N.json`. The top of that list:

| USDY in pool | Against | Counter reserve | Fee |
|---|---|---|---|
| `70.3270806` | XLM native | `418.7744547` | 30 bp |
| `40.6880093` | `LGSr` | `119.4555448` | 30 bp |
| `30.7385942` | `Cleanshave` | `537,463,757.3448442` | 30 bp |
| `25.9155798` | `neco` | `1,689.8724346` | 30 bp |
| `18.0694091` | `DAWG` | `8,307.3421023` | 30 bp |
| `14.7342195` | `yUSDC` | `16.7063480` | 30 bp |
| `14.0145299` | `yXLM` | `83.6021893` | 30 bp |
| `3.1199699` | **`USDC`** | `3.5345377` | 30 bp |

**467.5 million USDY in supply. 262.07 USDY in every pool on the network combined. 3.12
USDY in the pool against USDC.** Two of the deepest pools pair it against assets called
`Cleanshave` and `DAWG`.

That contrast is the product thesis stated as a measurement rather than as a pitch, and
it is why the SOW's own framing of USDY as "comparable structural risk" reads correctly.
**No band is computed here.** `keel layer1` is the instrument for that and it refuses to
print until a hand recomputation exists to compare against, which is the ordering rule
working as designed. Section 6 says who can lift that.

---

## 4. Volume over 24 hours, and what Horizon will not tell you

**Horizon cannot answer "what did this asset trade in 24 hours".** `/trades`,
`/trade_aggregations` and `/order_book` all refuse a request naming only one side.
Verified again for this document rather than carried over:

```
GET /trades?base_asset_code=USDY&base_asset_issuer=GAJMPX5N...&order=desc&limit=1
400 Bad Request
"invalid_field": "base_asset_type,counter_asset_type"
"reason": "this endpoint supports asset pairs but only one asset supplied"
```

**So the counterparty set had to be enumerated first, and it is enumerated rather than
guessed:** the 42 pool counterparties above, plus USDC explicitly. 42 distinct pairs,
one `/trades` walk each, 43 requests logged with their status in `requests.tsv`.

**Window:** `2026-09-07T06:00:05Z` to `2026-09-08T06:00:05Z`, 24 hours ending at the
read. Every trade in it: `usdy-trades-24h.csv`, 795 rows, one per trade, each with its
`ledger_close_time`, both amounts, the price as the `n/d` fraction, and its `trade_type`.

| Measure | Value |
|---|---|
| Trades in the window | **795** |
| USDY volume in the window | **`58.5641351`** |
| Of which against USDC | `43.6758435` over 208 trades |
| Of which against XLM | `11.2464255` over 198 trades |
| Implied price, from the USDC trades | `49.3750026 / 43.6758435` = **`1.1304877`** USDC per USDY |
| **Volume valued at that price** | **about `66.21` USDC** |
| Volume to supply ratio | about `1.25e-7` |

**Two things in the distribution matter more than the total.**

**596 of the 795 trades are `liquidity_pool` and 199 are `orderbook`.** Three quarters of
this asset's trading is against pools that hold 262 USDY between them.

**499 of the 795 trades are smaller than 0.01 USDY, which is 62 per cent of them.**
Median trade size is `0.0025895` USDY, about a third of a US cent. The largest single
trade in the window is `16.0631240` USDY. A 795 trade day that moves 58 USDY is not an
active market, and FR-10 exists to say so: the wash trade exclusion rules in
`docs/methodology/07-supporting-metrics.md` are what turn that observation into a metric,
and they are not applied here because nothing here ran the methodology.

**What this figure is NOT.** It is volume across the 42 pairs that could be discovered
from pool reserves plus USDC. An order book pair with no pool behind it cannot be reached
this way, for the reason `configs/demonstration-set.json` records: Horizon offers no way
to enumerate the counterparties of an asset. So `66.21` USDC is a **lower bound** across a
**named, reproducible** set of pairs, and the SOW's `$191` came from a source that is not
named in the SOW. The two numbers are close enough in magnitude to say the SOW's claim
still holds in kind, and they are not the same measurement.

---

## 5. Reproducing this

Nothing here needs an account, a key, or a registration. NFR-10.

```bash
# The 37 issuers, with the two-way SEP-1 check
go run ./cmd/keel universe -codes USDY -out <a directory> -name usdy-issuers

# Supply and holders
curl "https://horizon.stellar.org/assets?asset_code=USDY&limit=200"

# The 42 pools
curl "https://horizon.stellar.org/liquidity_pools?reserves=USDY%3AGAJMPX5NBOG6TQFPQGRABJEEB2YE7RFRLUKJDZAZGAD5GFX4J7TADAZ6&limit=200"

# The order book against the global quote asset
curl "https://horizon.stellar.org/order_book?selling_asset_type=credit_alphanum4&selling_asset_code=USDY&selling_asset_issuer=GAJMPX5NBOG6TQFPQGRABJEEB2YE7RFRLUKJDZAZGAD5GFX4J7TADAZ6&buying_asset_type=credit_alphanum4&buying_asset_code=USDC&buying_asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&limit=200"
```

It will not reproduce these numbers, for the reason every document in this directory
gives about itself: the market moves, and a 24 hour window moves with the clock. What is
reproducible is the shape, and `requests.tsv` carries every URL that produced a figure
above.

**The raw bodies are in `bodies/`** with their response headers beside them, so a reader
can check the readings without repeating the requests.

---

## 6. What this leaves open

1. **Which issuer the funder meant.** Section 2.2. This is a question for the Ambassador
   Chapter Lead and it travels naturally with the message DEC-001 section 8.3 already
   says is owed about the February date. Two corrections in one note.
2. **Holder concentration for this asset.** FR-8 is implemented as of `00ffe15` and was
   not run here. `holderstats` would page the 2,714 trustlines and give top-1, top-10 and
   HHI. It is a request cost of about fourteen pages and it is not in the SOW's Week 1
   line, so it was left out rather than bundled in.
3. **The engine's own reading of USDY.** No depth, no manipulation cost, no band appears
   in this document. `keel layer1` requires a hand recomputation to exist first, and
   `testdata/manual/` is RED. So the figure that would matter most here is Al's to unlock,
   and `docs/evidences/layer-1/layer-1-workspace.md` is the procedure. **USDY is a
   candidate worth adding to that batch**, because an asset with an empty book against
   its quote asset and a 3 USDY pool exercises a path none of the five proposed assets
   reaches.
4. **The 50 asset dataset does not carry this class of reading.** Run 27 of 26 August
   stored `holder_hhi`, `holder_top1_pct`, `holder_top10_pct` and `volume_to_supply` as
   null across all 64 rows, because FR-8 to FR-10 landed on 5 September. The refresh that
   fixes it is Claude's and is scheduled.

---

## 7. Version history

| Date | Change |
|---|---|
| 8 September 2026 | Created. Closes the SOW's Week 1 "verify current stablecoin data" line, which had no evidence in the repository nineteen days after the record that asked for it. Records that the ticker resolves to 37 assets, that SEP-1 verification passes for a dozen imitation domains, and that the only USDY with a market holds 262 of its 467 million units in pools |
