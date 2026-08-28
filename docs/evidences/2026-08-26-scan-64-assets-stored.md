# Running the engine over 64 assets and storing the results

**Run on:** 26 August 2026, from Horizon mainnet, no account required.
**Written up on:** 28 August 2026. The run is two days older than this document,
which is the finding that produced it: the evidence existed only in a local
Postgres and nowhere a reviewer could reach.
**What it is evidence for:** FR-17, "run the engine over at least 50 active
Stellar assets and store the results", and through it the Deliverable 1
acceptance line that names it.
**Run row:** `runs.id = 27`, kind `scan`.

**Reproduce it:**

```bash
make up && make migrate
go run ./cmd/keel assets -pairs configs/demonstration-set.json  # 60 pairs
go run ./cmd/keel assets -pairs configs/recorder-pairs.json     # 8 pairs, 4 new
go run ./cmd/keel scan -once
```

**The 64 is a union of two files and not the size of either.**
`configs/demonstration-set.json` holds 60 pairs, `configs/recorder-pairs.json`
holds 8, they overlap on 4, and the union is 64. The four the recorder file adds
are ARST, BRL, PYUSD and USTRY. Both files declare themselves PROVISIONAL in
their own `note` field and both name section 5 of `02-pair-selection.md` as what
supersedes them. So the number 64 is an artifact of which two files were loaded,
not a set anybody selected, and the moment those criteria are written the set
should be rebuilt rather than justified backwards.

It will NOT reproduce these numbers. Every asset is read at whatever ledger
Horizon is at when its request lands, so a run tomorrow stores a different
ledger, a different mid price and a different spread for all 64. What it will
reproduce is the shape: 64 assets in, 64 rows out, nothing failed.

---

## 1. The run

| Field | Value |
|---|---|
| Run id | 27 |
| Started | 2026-08-26T08:59:48.443648Z |
| Finished | 2026-08-26T09:00:41.220492Z |
| Wall clock | 53 seconds |
| `assets_ok` | 64 |
| `assets_failed` | 0 |
| Rows written to `metrics` | 64 |
| Distinct assets in those rows | 64 |
| Ledger range | 64130756 to 64130765 |
| `methodology_version` | `1.0.3-draft`, one value across all 64 rows |
| `data_source` | `horizon`, one value across all 64 rows |

The ledger range spans ten ledgers rather than one. That is not a defect and it
is not a snapshot: each asset costs three Horizon requests, the round takes 53
seconds, and mainnet closes a ledger about every 5 seconds. Every row carries
its own `ledger_seq`, which is rule 1 of the non-negotiables, so the spread is
recorded rather than hidden. A cross-asset comparison drawn from this run
compares states up to 50 seconds apart, and any document that draws one has to
say so.

Quote asset is USDC for all 64. The set is single-quote, so the absolute bands
in `09-flags-and-bands.md` are comparable across it. That is a property of this
particular set and not a decision: open question Q7 and section 5 of
`02-pair-selection.md` are what would make it one, and both are unanswered.

---

## 2. What the engine produced

Counted over the 64 rows of run 27.

| Quantity | Rows with a value | Rows null |
|---|---|---|
| `mid_price` | 64 | 0 |
| `spread_pct` | 44 | 20 |
| `depth` | 64 | 0 |
| `manipulation_cost_combined` | 64 | 0 |
| `max_safe_collateral` | 64 | 0 |
| `max_reachable_price` | 0 | **64** |
| `cost_to_max_reachable_price` | 0 | **64** |
| `oracle_resistance` | 0 | 64 |
| `holder_hhi`, `holder_top1_pct`, `holder_top10_pct` | 0 | 64 |
| `volume_to_supply` | 0 | 64 |

Split by `price_source`:

| `price_source` | Rows | of which have a two-sided book |
|---|---|---|
| `pool` | 36 | 16 |
| `book` | 28 | 28 |

`max_safe_collateral` is 0 for 17 of the 64 and above 0 for 47.

### 2.1 The three null columns that mean three different things

**`max_reachable_price` null on all 64 is consistent with the methodology and is
not a bug.** `05-manipulation-cost.md` nulls it when a pool is active, and the
20 rows with no `spread_pct` are exactly the rows with no two-sided book, which
means every one of the 64 pairs has a pool. The divergence measurement in
`measurements/divergence/summary.txt`, taken over an overlapping 60-pair set,
reports 0 pairs in case 2, "two-sided book, no pool", and 20 pairs across cases
3 and 4. The two counts agree. What this run therefore does NOT exercise is the
book-only path, and a demonstration set in which no asset lacks a pool cannot
show that path working.

**`oracle_resistance` null on all 64 is an open input, not a missing formula.**
The window length is item B-6, unanswered by Reflector, and `06-oracle-resilience.md`
is `partial` for that reason.

**The supporting metric columns are null because the definitions do not exist.**
`holder_hhi`, `holder_top1_pct`, `holder_top10_pct` and `volume_to_supply` are
columns in `migrations/`, fields in `domain.Supporting`, and mapped in
`internal/api/wire.go`, and no code computes any of them. `07-supporting-metrics.md`
is a WORKSHEET with seven unfilled blanks, so FR-8 through FR-11 have no
definition to implement. This run is evidence for FR-17 and for nothing about
those four.

---

## 3. The 64 assets, as stored

Issuer truncated to the first 8 characters. `mid_price`, `spread_pct` and
`max_safe_collateral` are the values in the stored row, at the stored ledger, and
are not comparable across rows without the caveat in section 1.

**A trailing ellipsis means the decimal was CUT for column width, not rounded.**
The digits shown are the leading digits of the stored value and the value
continues. Nothing here is the authoritative figure; the row in `metrics` is.
Read the full precision with the query in section 5 rather than by trusting this
table, which exists to show coverage rather than to be computed against.

| Code | Issuer | Ledger | `price_source` | `mid_price` | `spread_pct` | `max_safe_collateral` |
|---|---|---|---|---|---|---|
| ACT | `GAHHULDP` | 64130756 | `pool` | 0.000784409274… | 870.40021330… | 0 |
| AFR | `GBX6YI45` | 64130756 | `book` | 0.000577003955… | 6.6852602084… | 0.030736800001… |
| AIus | `GDBYZGMI` | 64130756 | `pool` | 0.002494023922… | 103.07838312… | 0.000001425009… |
| AMM | `GARQ7O5J` | 64130756 | `pool` | 0.002516648895… | 109.93455112… | 13.853265125 |
| AQUA | `GBNZILST` | 64130757 | `book` | 0.000355656002… | 0.6376761934… | 7163.991389575… |
| ARST | `GCSAZVWX` | 64130757 | `pool` | 0.000546227763… | 454.38591976… | 0 |
| AUDD | `GDC7X2MX` | 64130757 | `book` | 0.757274669465… | 0.6243360726… | 56.73044822423… |
| BASH | `GBQ42A3K` | 64130757 | `pool` | 0.000009546098… | null | 64.73434663244… |
| BEER | `GBL2VH6M` | 64130757 | `pool` | 0.000002753451… | null | 0 |
| BRL | `GDVKY2GU` | 64130757 | `pool` | 0.155074117409… | 397.65920341… | 0 |
| BTC | `GDPJALI4` | 64130758 | `book` | 78697.64431911… | 0.1918578876… | 1329.251801625… |
| BTCLN | `GDPKQ2TS` | 64130758 | `pool` | 0.000791729239… | 79.484243935… | 10.00568256566… |
| DicInu | `GDMNBJWZ` | 64130758 | `pool` | 0.000005805651… | 15297.164183… | 0 |
| EMN | `GAKIOVTN` | 64130758 | `pool` | 105.4849580398… | null | 3.4200802 |
| ETH | `GBFXOHVA` | 64130758 | `pool` | 2462.277662457… | 38.622776565… | 29.82270074249… |
| EURC | `GAQRF3UG` | 64130758 | `book` | 0.59653605 | 22.044015613… | 6.957554934860… |
| EURC | `GDHU6WRG` | 64130758 | `book` | 1.159515535942… | 0.1390417275… | 53378.82127673… |
| FRED | `GCA73U2P` | 64130759 | `book` | 0.000119073781… | 17.837125621… | 0.001096475000… |
| FUNT | `GBUYUAI7` | 64130759 | `pool` | 1.216116473234… | null | 18.77515368409… |
| GOLD | `GBC5ZGK6` | 64130759 | `book` | 107.441039 | 4.0767122514… | 6.708622083966… |
| GOLD | `GBCB4WO6` | 64130759 | `pool` | 0.000010075025… | null | 15.92855094237… |
| GQX | `GD7TC72O` | 64130759 | `book` | 8.62 | 30.858468677… | 35.84814874378… |
| GROG | `GBE2P44L` | 64130759 | `book` | 0.00000015 | 66.666666666… | 42.72252774887… |
| GZC | `GCGZCF3I` | 64130759 | `pool` | 15.56601344584… | null | 74.15378661307… |
| HU | `GBGRBCUB` | 64130760 | `pool` | 0.003181949844… | null | 9.484042967391… |
| IDRT | `GDPKQ2TS` | 64130760 | `book` | 0.000047 | 4.2553191489… | 147.4940992000… |
| JDMC | `GDZ7MGCU` | 64130760 | `pool` | 0.000246697967… | null | 5.845881924239… |
| LIBRE | `GAYCCWKE` | 64130760 | `book` | 0.00108385 | 2.0021220648… | 0.419589775026… |
| LSP | `GAB7STHV` | 64130760 | `pool` | 0.000170775044… | 204.94661655… | 0 |
| PAYBO | `GDNUDY5L` | 64130760 | `pool` | 0.000113088896… | null | 22.48013480835… |
| PYUSD | `GDQE7IXJ` | 64130760 | `book` | 0.999254722601… | 0.0509805190… | 12601.46653677… |
| RBT | `GCMSCRWZ` | 64130761 | `pool` | 0.000048434848… | 20644226.818… | 0 |
| RCW | `GCEDS7HG` | 64130761 | `pool` | 2.218428746275… | null | 7.962482982895… |
| REAL8 | `GBVYYQ7X` | 64130761 | `pool` | 0.001254681055… | null | 0 |
| SCROOGE | `GD2TQV2V` | 64130761 | `book` | 0.0108 | 1.8518518518… | 70.93849370474… |
| SHX | `GDSTRSHX` | 64130761 | `book` | 0.003630946518… | 4.4587006896… | 447.6181093804… |
| SLVR | `GBZVELEQ` | 64130761 | `pool` | 1.495809038012… | 30.854878409… | 6.251437646251… |
| SOLS | `GAWTJMZI` | 64130761 | `pool` | 0.000001160776… | null | 0 |
| SSLX | `GBHFGY3Z` | 64130762 | `pool` | 0.000167193603… | 114.23989891… | 0.000011750000… |
| TFT | `GBOVQKJY` | 64130762 | `pool` | 0.004827603815… | 66.739797455… | 0 |
| TGM | `GBGRBCUB` | 64130762 | `pool` | 25529.55309674… | null | 71.65821289826… |
| THE1 | `GBGRBCUB` | 64130762 | `pool` | 5.439143052751… | null | 60.57513897753… |
| TRNPC | `GBOCPMDD` | 64130762 | `pool` | 0.000278587285… | null | 6.432717401142… |
| UAF | `GDT5EUZM` | 64130762 | `pool` | 0.000000730113… | 3780.2320343… | 0 |
| USDGLO | `GBBS25EG` | 64130762 | `book` | 0.9955005 | 1.1050722726… | 50.53270853099… |
| USDM | `GDHDC4GB` | 64130763 | `book` | 0.9721252 | 0.8486149726… | 9.415217895828… |
| USDT | `GCQTGZQQ` | 64130763 | `book` | 0.15482185 | 70.819138254… | 9.861904930036… |
| USDZ | `GAKTLPC4` | 64130763 | `book` | 0.999998599562… | 0.2194803151… | 1289.456957451… |
| USTRY | `GCRYUGD5` | 64130763 | `book` | 1.073021212249… | 0.0498532083… | 69496.42329403… |
| VAQM | `GC2BHMM5` | 64130763 | `book` | 0.28415 | 3.9767728312… | 6.865589812628… |
| VELO | `GDM4RQUQ` | 64130763 | `book` | 0.003800502524… | 7.3186659641… | 321.5147378511… |
| X | `GB3JS5OP` | 64130764 | `pool` | 0.004912477370… | null | 0 |
| XAG | `GB4I3O6E` | 64130764 | `pool` | 0.000128182410… | 78013706.596… | 0 |
| XAU | `GBCB4WO6` | 64130764 | `pool` | 0.000002039390… | null | 0 |
| XH5 | `GA6N7EVP` | 64130764 | `pool` | 0.040772848639… | 269.78737862… | 0 |
| XLM | `native` | 64130764 | `book` | 0.184033369085… | 0.0561449860… | 117936.0087564… |
| XLMFISHz | `GBLLGU7T` | 64130764 | `pool` | 0.453466864093… | null | 10.13298698852… |
| XRP | `GBXRPL45` | 64130764 | `book` | 1.428458586742… | 0.1999999999… | 550.9987045147… |
| XTAR | `GAORYJ3K` | 64130765 | `book` | 0.000004833333… | 75.862068965… | 0.333333325001… |
| eBTC | `GCEAHWRS` | 64130765 | `pool` | 0.001851096341… | null | 0 |
| neco | `GB5NWTBD` | 64130765 | `pool` | 0.018939749892… | null | 0 |
| sUSD | `GCHW7CWI` | 64130765 | `book` | 0.993871390717… | 0.3756323670… | 11776.94357143… |
| yUSDC | `GDGTVWSM` | 64130765 | `book` | 0.999206327886… | 0.0387611864… | 107218.1533956… |
| yXLM | `GARDNV3Q` | 64130765 | `book` | 0.183843948746… | 0.3000000000… | 808.4559634248… |

---

## 4. One discrepancy, recorded rather than smoothed over

**Run 25 reports `assets_ok = 65` and stored 65 rows for 64 distinct assets.**
Run 25 is the earlier scan the same morning, 2026-08-26T07:02:02Z. One asset has
two rows in it:

| Code | Issuer | Rows | Ledgers |
|---|---|---|---|
| XLM | native | 2 | 64129517, 64129518 |

The `assets` table cannot hold XLM/USDC twice: its unique constraint covers
`(code, issuer, quote_code, quote_issuer)` with `NULLS NOT DISTINCT`, and it
holds 64 rows. So the duplicate is in the round, not in the set. Two
explanations fit the evidence and this document does not choose between them:
the round processed one asset twice, or a transient failure was retried and both
attempts stored, one ledger apart.

The write path does not prevent it. `scan` deduplicates on
`(asset, ledger, methodology version, source)`, which is stated in its own usage
text, and 64129517 and 64129518 are different ledgers, so both rows are legal
writes rather than a broken constraint.

**Run 27 does not have it: 64 rows, 64 distinct assets, and it is the run this
document is evidence for.** The discrepancy is recorded because
`10-validation.md` section 3 holds that a discrepancy explained is worth
something and a discrepancy never looked for is worth nothing, and because
`assets_ok` is the counter a reader would otherwise quote as the asset count. It
is not one. The distinct count is.

---

## 5. What a reader has to run to check this

The store is a local Postgres. It is not in the repository, and no clone
contains these rows. That is the honest limit of this document: it reports what
a run produced, and a third party reproducing it gets their own numbers at their
own ledgers, per the note at the top.

```sql
-- the run row
select * from runs where id = 27;

-- the counts in section 1
select count(*), count(distinct asset_id),
       min(ledger_seq), max(ledger_seq),
       string_agg(distinct methodology_version, ','),
       string_agg(distinct data_source, ',')
from metrics
where computed_at between '2026-08-26 08:59:48+00' and '2026-08-26 09:00:42+00';

-- the duplicate in section 4
select asset_id, count(*), string_agg(ledger_seq::text, ',')
from metrics
where computed_at between '2026-08-26 07:02:02+00' and '2026-08-26 07:03:00+00'
group by asset_id having count(*) > 1;
```

Connect with the DSN the container publishes, which is port **5433** on the
host and not 5432:

```bash
PGPASSWORD=keel_dev_only psql -h localhost -p 5433 -U keel -d keel
```

---

## 6. What this closes and what it does not

| | |
|---|---|
| **Closes** | FR-17. 64 active assets, above the 50 the requirement names, engine run over all of them, 0 failed, results stored with `ledgerSeq` and `methodologyVersion` on every row |
| **Does not close** | FR-8 through FR-11. Four supporting metric columns are null in all 64 rows because `07-supporting-metrics.md` has no definitions in it |
| **Does not close** | the demonstration set itself. `02-pair-selection.md` section 5 promises inclusion criteria written BEFORE the list is built, and the list was built first. These 64 are provisional, the same status `configs/recorder-pairs.json` carries |
| **Does not close** | the book-only code path. Every one of the 64 pairs has a pool, so no row in this run exercises an asset without one |

## 7. Version history

| Date | Change |
|---|---|
| 28 August 2026 | Created. Documents scan run 27 of 26 August 2026, and the run 25 duplicate |
