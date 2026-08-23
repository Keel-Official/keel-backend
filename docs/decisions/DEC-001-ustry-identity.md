# DEC-001: USTRY Asset Identity and the Incident Ledger Range

**Status:** The two items that were open in section 3 are now CONFIRMED from
primary sources. Items 3 to 5 remain open. Section 7 records what was closed, how,
and one thing that turned out worse than expected.
**Date:** August 2026, corrected 21 August 2026
**Impact:** all of Deliverable 2, the Hubble query range, and two corrections to the SOW

> **Corrected 21 August 2026.** Two claims this document made were wrong: the
> manipulation ratio in section 2, and the asset type in its own curl command in
> section 5.2. Both are fixed in place, and section 7 records what the ledger
> actually says. Findings P1-20 through P1-23 in
> `docs/internal/audit-2026-08-20.md`.

---

## 1. Corrections to the SOW (important, must be communicated to the Ambassador)

### Correction 1: the incident date

| | SOW | Correct |
|---|---|---|
| Date | 20 May 2026 | **22 February 2026, 00:25 UTC** |

Every other detail in the SOW matches the February incident: roughly 48 million XLM
frozen, losses of about $10 million, a 100x price manipulation, a 61 million XLM
loan. These are not two different incidents. The date was recorded wrongly.

**Impact:** the entire historical query range moves from May to February 2026. The
good news is that February data has had longer to settle and is therefore more
likely to be complete in Hubble.

### Correction 2: the unit of the loan amount

| | SOW | Correct |
|---|---|---|
| Loan | "$61 million in XLM" | **61,249,278.31 XLM**, plus 1,000,196.70 USDC |

61 million XLM is not $61 million. The two loans together are worth roughly $10 to
$11 million. That is a difference of about six times and an SCF reviewer will catch
it.

### Correction 3: what was exploited was not Blend core

What was exploited is the **YieldBlox DAO pool on Blend V2**, a community managed
pool whose parameters its own operator can set. BlockSec concluded this was a pool
operator configuration failure, not a vulnerability in Blend's core contracts.
Calling it "the Blend exploit" without qualification will read as careless to an
informed reader.

A more accurate phrasing: "the YieldBlox DAO pool incident on Blend V2".

---

## 2. Facts already confirmed

| Item | Value | Confidence |
|---|---|---|
| Date and time | 22 February 2026, 00:25 UTC (the two loan transactions) | High, many sources agree |
| The manipulated asset | USTRY, a US Treasury stablebond issued by **Etherfuse** | High |
| The manipulated market | **USTRY/USDC on SDEX** | High |
| Oracle | **Reflector**, VWAP based, reading prices from SDEX | High |
| Protocol | The YieldBlox DAO pool on Blend V2 (Script3) | High |
| Price before | about $1.05 to $1.058 | High |
| Price after | about $106 to $107 | High |
| Pre-incident volume | under $1 per hour | High |
| Loan 1 | 1,000,196.70 USDC | High |
| Loan 2 | 61,249,278.31 XLM | High |
| Collateral | reported as 13,003 USTRY then a further 140,000 USTRY. BlockSec cites a total of about 149,876 USTRY | **Medium, sources disagree. Verify on-chain** |
| Funds frozen | about 48 million XLM, worth roughly $7.2 million | High |

### The chronology as reported by Rekt

| Time (UTC) | Event |
|---|---|
| 14 Feb 2026 | The attacker's main account is created with 56.32 XLM |
| 14 to 20 Feb | Small test purchases of USTRY at the normal price of about $1.058 |
| 21 Feb 23:35 | A burner account is created with 15 XLM: `GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB` |
| 21 Feb 23:38 | A sell offer of 1.2185 USTRY at a price of 107 USDC. Transaction hash: `09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb` |
| 22 Feb around 00:10 | A third account executes a trade so that the oracle reads that price |
| 22 Feb 00:25 | Two loan transactions: 1,000,196 USDC then 61,249,278 XLM |

**The number that matters most to Keel**, corrected from the ledger on 21 August
2026: the manipulation offer was 1.2185312 USTRY, and the trade that executed
against it was **5.3475699 USDC** for 0.0501003 USTRY. It was not the $0.50 a
secondary source reported. Against roughly $10.87 million borrowed, the ratio of
what the attacker paid to what they took is about **1 to 2.05 million**.

That is still the single number that sells Keel's premise, and it is now a number
that survives being checked. The old 1 to 22 million came from a news figure and was
off by a factor of ten. See section 7, and note its caveat: 5.3475699 USDC is what
the attacker paid, which is not the same quantity as Keel's manipulation cost.

---

## 3. What is still unconfirmed

1. **The USTRY issuer address (`G...`).** Not found in any secondary source. It has
   to come from the ledger.
2. **Ledger sequence numbers** for each point in the chronology above.
3. **The exact collateral amount.** Different sources cite 153,003 and 149,876 USTRY.
4. **Reflector's VWAP window length.** Script3 states there was no other trade
   within 15 minutes. It needs confirming whether 15 minutes really is the oracle
   window or a coincidence.
5. **The YieldBlox pool risk parameters** at the time: the USTRY collateral factor,
   and the XLM and USDC liability factors.

---

## 4. Consequences for Keel's design

**USTRY's primary pair is USDC, not XLM.** This settles part of open questions T3
and D-1. The oracle reads the USTRY/USDC market, so the backtest is required to
measure that market. Measuring USTRY/XLM would answer the wrong question.

**Manipulation cost has to be computed relative to the oracle window, not only
relative to a price shift.** This is an important refinement to metric K5. The
Reflector oracle is VWAP based, so what an attacker needs to move is not the
instantaneous price but a volume weighted average over some window. In a market
with no other trades, one trade dominates that average entirely, and that is what
made this attack cheap. Record it in
`docs/methodology/07-supporting-metrics.md` either as a limitation or as an extension
of the metric.

**The metric "time since the last genuine trade" is proven relevant.** Volume under
$1 per hour and no trade inside the oracle window are exactly the conditions that
should trigger `NO_GENUINE_TRADE_7D` and `THIN_DEPTH_5PCT`. Keel would have marked
this asset red long before 22 February.

**There is a factor Keel will not catch, and this has to go in the limitations
section.** Reflector states that the market maker for that market withdrew all of
its liquidity at some point before the exploit. That means the dangerous condition
appeared relatively suddenly. A Keel that scans every 15 minutes catches it; a Keel
that scans daily may be too late. Scan frequency therefore becomes a parameter that
has to be discussed honestly in the report rather than hidden.

---

## 5. Procedure for closing the two open items

Everything below uses public Horizon: free, no account.

### 5.1 Finding the USTRY issuer

```bash
curl -s "https://horizon.stellar.org/assets?asset_code=USTRY&limit=20" | jq '._embedded.records[] | {code:.asset_code, issuer:.asset_issuer, amount, num_accounts:.num_accounts}'
```

If more than one issuer appears, do not guess. Disambiguate by matching against the
known burner account:

```bash
curl -s "https://horizon.stellar.org/accounts/GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB" | jq '.balances'
```

That account's balances will contain `asset_code: "USTRY"` together with the
`asset_issuer` actually used in the attack. That is the definitive answer, because
it comes from the ledger rather than from an article.

### 5.2 Finding the ledger sequence

The transaction hash of the manipulation offer is already known:

```bash
curl -s "https://horizon.stellar.org/transactions/09e1a9d1197c9bf0af4e87da328c4f2d5eb49b487630aa61991fb5c1c4637cdb" | jq '{ledger, created_at, source_account, successful}'
```

The `ledger` field is the main anchor. From there, pull the burner account's full
history:

```bash
curl -s "https://horizon.stellar.org/accounts/GCNF5GNRIT6VWYZ7LXUZ33Q3SR2NUGO32F5X65VVKAEWWIQCKGYN75HB/operations?limit=200&order=asc" | jq '._embedded.records[] | {id, type, created_at, transaction_hash}'
```

Every operation carries its time and can be traced to its ledger. From this account
you will find the counterparty of the trade, and from there the borrowing account.

For the backtest range, a sensible target is **the ledgers from 1 February 2026
00:00 UTC through 28 February 2026 23:59 UTC.** Ledgers close about every 5
seconds, so roughly 17,280 ledgers per day and about 480 thousand ledgers for the
month. Do not compute this from an estimate; take the bounds from the data:

```bash
# USTRY is credit_alphanum12, not credit_alphanum4: its code is five characters.
# The wrong type returns an empty result and NO error, the silent failure that
# internal/domain/types.go warns about. This command used to carry that mistake.
curl -s "https://horizon.stellar.org/trades?base_asset_type=credit_alphanum12&base_asset_code=USTRY&base_asset_issuer=GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC&counter_asset_type=credit_alphanum4&counter_asset_code=USDC&counter_asset_issuer=GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN&order=asc&limit=200" | jq '._embedded.records[] | {ledger_close_time, base_amount, counter_amount, price}'
```

That endpoint is historical and free. It hands you the full trade history of that
market directly, including the manipulation trade. For an asset as thin as USTRY the
trade count is probably small, and the whole market history fits in a few pages.

Verify the USDC issuer above before using it.

### 5.3 Primary sources that must be read directly

Every source used so far is secondary. For a report that can be defended, read
directly:

- Script3's statements (@script3official) of 22 and 23 February 2026
- Reflector's statement on the cause of the mispricing
- BlockSec's analysis: https://blocksec.com/blog/yieldblox-dao-incident-on-stellar-oracle-misconfiguration-enabled-a-10m-drain
- QuillAudits' analysis: https://www.quillaudits.com/blog/hack-analysis/yeildblox-10m-hack-explained
- Rekt's forensic reconstruction: https://rekt.news/yieldblox-rekt
- The YieldBlox pool configuration on Blend V2, for the risk parameters

The rule for the Deliverable 2 report: every number claimed must have one of two
sources, either reproducible on-chain data or an official statement by a party
involved. A number from a news article is only a hint to go looking for in the
ledger, never a fact to cite.

---

## 6. Next actions

1. Run the commands in 5.1 and 5.2, fill in the two open items, update this document
2. Tell the Ambassador Chapter Lead about the date and unit corrections. This is not
   bad news, it is evidence that you verified
3. Change every reference to "May 2026" to "February 2026" in the PRD, the build
   plan, and the checklist
4. Set the Hubble spike range to February 2026
5. Fix USDC as USTRY's primary quote pair for backtest purposes

---

## 7. What was closed on 21 August 2026, and how

Section 3 listed five unconfirmed items. Two are now closed from primary sources,
using the procedures section 5 prescribes rather than a shortcut.

### 7.1 Item 1, the USTRY issuer: CONFIRMED

```
GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC   credit_alphanum12
```

Section 5.1 said: if more than one issuer appears, do not guess, disambiguate
against the burner account. That turned out to matter more than expected.
`/assets?asset_code=USTRY` returns **seven** distinct issuers, every one of them
`credit_alphanum12`. The name "USTRY" alone is ambiguous seven ways.

The burner account `GCNF5GNRIT...` holds `1.1869077 USTRY` issued by
`GCRYUGD5...`, which settles it from the ledger rather than from an article. That is
also the issuer the golden fixture already used, so the fixture was right, and it is
now right for a reason that can be shown.

**Consequence for the demonstration set:** an asset row keyed on code alone is
wrong seven times over for this asset. `assets` in `migrations/0001_core.sql` keys
on `(code, issuer, quote_code, quote_issuer)`, which is correct, and this is the
concrete reason it has to stay that way.

### 7.2 Item 2, the ledger sequences: CONFIRMED

| Event | Ledger | Time |
|---|---|---|
| Manipulation offer placed, op 263453036239003649 | derived from tx `09e1a9d1...` | 2026-02-21T23:38:51Z |
| Manipulation trade, trade 263454423513071617-0 | **61340263** | 2026-02-22T00:10:21Z |
| The 1.057 bid cancelled, op 263454462168100865 | **61340272** | 2026-02-22T00:11:16Z |

### 7.3 A methodology claim checked against the live ledger

Methodology section 10.3 asserts `1.2185312 - 0.0501003 = 1.1684309 USTRY` and says
it matches the offer remainder today. It does, exactly:

```
$ curl -s ".../accounts/GCNF5GNRIT.../offers"
  id     = 1824788980
  amount = 1.1684309 USTRY
  price_r = {n: 266843207, d: 2500000}   = 106.7372828
```

Difference from the claim: zero. Six months after the incident that offer is still
open, at the same price, and the fixture's `price_r` is confirmed byte for byte
against the live ledger. The arithmetic consistency argument in section 10.3 holds.

It also means the broken book state has never been repaired.

### 7.4 The ratio, corrected

The old text derived 1 to 22 million from a news report of "$0.50". The ledger says
5.3475699 USDC, so the ratio against roughly $10.87 million borrowed is about
**1 to 2.05 million**.

**The caveat that has to travel with that number.** 5.3475699 USDC is what the
attacker paid, and methodology section 7.2 is explicit that the manipulation cost is
the notional paid to **other parties**. This payment went to an offer the attacker
owned, so it came back. What the number measures is the trade size needed to move
the oracle's reading, which is capital an attacker must be able to **move**, not
capital they must **spend**. Both quantities belong in the report under different
names, and DEC-006 adds a third: what it would have cost to move the combined
orderbook and pool price, which is 127.03 USDC at the critical delta.

### 7.5 Still open

Items 3, 4, and 5 of section 3 are untouched: the exact collateral amount, the
Reflector VWAP window length, and the YieldBlox pool risk parameters. All three need
sources this repository does not have, and two of them are what DEC-003 section 8.6
and methodology section 9 are waiting on.
