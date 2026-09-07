# Layer 1 raw inputs, read 7 September 2026

**What this directory is.** The order books that five hand recomputations will be worked
out from, reproduced verbatim from Horizon. `docs/methodology/10-validation.md` section 1
defines Layer 1 as "take a raw order book, transcribe it into a spreadsheet, compute depth
by hand, compare against engine output", and the first word of that sentence is what this
directory supplies.

**What this directory is NOT.** It holds no computed value. No mid price, no spread, no
depth, no unit conversion, no rounding. Every file says so in its own first paragraph.
That restraint is the only reason these files may exist at all: `testdata/manual/` is RED
because numbers produced by the party being tested must never become the numbers that test
it, and a raw input that arrived carrying the answers would move the same problem one
directory sideways.

**Read on:** 2026-09-07, ledgers 64310827 and 64310828, from public Horizon mainnet with
no account. One round of `keel record` over `configs/recorder-pairs.json`, 24 requests,
every read ledger-bracketed and reported consistent.

**`raw/` holds the recorded bytes**, one schema 2 recording per proposed asset, which is
what `keel layer1` reads. The markdown files beside them are those same bytes rendered as
tables for transcription, and the sha256 in each file's provenance section is the hash of
the body inside the corresponding `raw/` file. Neither is derived from the other by hand:
both come out of the same recording, which is the point of keeping both.

---

## 1. The eight candidates, measured rather than assumed

| Asset | Bids | Asks | Pools | Ledger | Proposed |
|---|---|---|---|---|---|
| XLM native | **200** | **200** | 1 | 64310827 | no, see section 2 |
| AQUA `GBNZILST` | 49 | **200** | 1 | 64310828 | no, see section 2 |
| EURC `GDHU6WRG` | 65 | 92 | 1 | 64310828 | **yes** |
| BRL `GDVKY2GU` | 35 | 5 | 1 | 64310828 | **yes** |
| PYUSD `GDQE7IXJ` | 27 | 15 | 1 | 64310828 | **yes** |
| ARST `GCSAZVWX` | 6 | 23 | 1 | 64310828 | spare |
| USTRY `GCRYUGD5` | 15 | 11 | 1 | 64310828 | **yes** |
| AUDD `GDC7X2MX` | 11 | 5 | 1 | 64310828 | **yes** |

**The candidate list was not chosen today.** `configs/recorder-pairs.json` was compiled on
25 August 2026 across four liquidity buckets, before any of this existed, and it is used
here unedited. Choosing pairs today, with four Layer 3 runs already visible, would let the
selection carry the answer.

## 2. Why XLM and AQUA are excluded, and it is not convenience

`/order_book` returns at most 200 levels a side and does not say when it truncated. Both
of those books hit that cap: XLM on both sides, AQUA on the ask.

**A capped side is a PREFIX of the real book.** Depth at ±10 per cent asks how much
liquidity sits inside a price band, and if the band extends past level 200 the recorded
file does not contain the answer. So a hand recomputation over either of these has no
determinable correct value at the wider deltas, and a figure computed from a prefix would
look like a thin market rather than like a truncated read. `cmd/keel/crosscheck.go` already
carries this rule for Layer 3, where `countAgrees` accepts `rebuilt >= recorded` on a
capped side and skips the risk comparison entirely.

That leaves a real cost and it is stated rather than hidden: **the deepest end of the
liquidity range is not represented in this batch.** EURC at 157 levels is the deepest book
here that is complete.

## 3. The five proposed, and what each one is for

| # | Asset | Levels to transcribe | What it exercises |
|---|---|---|---|
| 1 | EURC | 157 | the deep end, and the only one where the level count itself is work |
| 2 | BRL | 40 | 35 bids against 5 asks. The most asymmetric book here, so it presses hardest on the buy versus sell split, which is half of what the oral test asks about |
| 3 | PYUSD | 42 | the middle of the range, and a `credit_alphanum12` code, so the asset type is not inferred from the code length |
| 4 | USTRY | 26 | the incident asset in a NORMAL state. The golden fixture is this same asset at its worst; this is the control |
| 5 | AUDD | 16 | the thin end, 11 bids and 5 asks |

ARST is the spare. Swap it for EURC if 157 levels is more transcription than the day
allows, and record that the deep end went unrepresented when you do.

**Every one of the five has exactly one liquidity pool**, so all five exercise the SDEX and
AMM combination rather than the order book alone. That was not arranged; it is what
Horizon returned.

## 4. One thing worth looking at before you start, in `AUDD-USDC-64310828.md`

Its first two bids:

| # | price_r n | price_r d | price |
|---|---|---|---|
| 0 | 7439209 | 10000000 | 0.7439209 |
| 1 | 20000000 | 26884579 | 0.7439209 |

**Two different exact prices rendering to the same string.** `7439209/10000000` is exactly
0.7439209; `20000000/26884579` is not. This is rule 5 of `CLAUDE.md`, "prices are read from
the `price_r` field, NOT from the `price` string", and it is not hypothetical: a hand
computation that transcribes the `price` column produces a book with two levels at one
price, and its ordering and its depth boundaries are both wrong.

## 5. The order of work

1. Transcribe from these files into the worksheet,
   `docs/internal/layer-1-worksheet-template.md`.
2. Compute by hand from `docs/methodology/`. **Before looking at any engine output.**
3. Write each result into `testdata/manual/`, naming the issuer and the ledger. Verify with
   `make manual-check`, which requires both and gates on them, and which reads the required
   count out of the protocol rather than out of a constant.
4. **Then** run the engine over the same recorded bytes and compare:

   ```bash
   keel layer1 -recording docs/evidences/layer-1/raw/<ASSET>-USDC-64310828.json.gz \
     -after-writing testdata/manual/<your file>.md
   ```

   That command refuses to print until the file named by `-after-writing` exists and is not
   empty. It cannot prove a figure was worked out by hand, and it says so in its own help
   text; what it makes deliberate is that the answer was not visible while the figure was
   being derived.

Where the engine and your file disagree, **the disagreement is the finding**. Adjust the
code to match your numbers. Never adjust your numbers to match the code.

## 6. Version history

| Date | Change |
|---|---|
| 7 September 2026 | Created. Eight candidates read at one ledger, five proposed, two excluded for hitting the page cap |
