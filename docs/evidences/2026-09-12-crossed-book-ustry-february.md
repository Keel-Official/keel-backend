# The crossed rows of the February series have a name, and it is not the one the 8 September reading proposed

**Read on:** 2026-09-12, from public Horizon mainnet, no account required.
**Pair:** `USTRY` `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`
against `USDC` `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`,
the identity fixed by DEC-001.
**MethodologyVersion:** none. **No methodology was run here.** Every figure below is a
Horizon reading, a row of a CSV already in this repository, or a comparison of the two.
Nothing passed through `internal/domain`.

**What this closes.** `tugas-b.md` B1 asks that the residual be characterised precisely
enough that a reader knows which rows to trust and why, and B2 asks that one of two
explanations be supported and the other ruled out. This document names the two offers
that produce every crossed row, and rules out the trade stream and the owner's own
operations as the cause. It does not name the mechanism that removed the ask, and
section 6 says exactly what is left to test.

**What it disputes.** `docs/evidences/2026-09-08-february-book-series.md` section 3 offers
two readings and declines to pick. Its section 6 prices a fix of about three hours built
on walking deeper. **Neither reading is right as written, and a deeper walk cannot fix
this**, for the reason section 5 gives.

---

## 1. The ghost has an offer id

The ask that crosses every crossed row is priced `1.0573892029461328` in both runs. That
is not a decimal the code chose. It is the exact value of the `price_r` ratio

    1981860307 / 1874295956

which Horizon reports on the trades that hit it. Searching
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv` for that
ratio returns seven trades, on two offers, both selling USTRY for USDC, both owned by
`GBPFB6XNLDMXQKOFJAH6IRTOMTEUU4ZWFHNRMYWZNXCZEDNE6UU66WSG`:

| Offer | Trades | First | Last |
|---|---|---|---|
| `1822775941` | 2 | ledger 61143618, 2026-02-08T18:34:01Z | ledger 61143619, 2026-02-08T18:34:08Z |
| `1822881343` | 6 | ledger 61144107, 2026-02-08T19:21:15Z | ledger 61144462, 2026-02-08T19:55:21Z |

`1822881343` is not the one: its last fill at ledger 61144462 carries a different ratio,
`105905209/100000000`, so the offer had been re-priced off `1.0573892` before it left.

**`1822775941` is the one, and the arithmetic that makes it one stroop is exact.** The
account's own operation at ledger 61143618 sets the offer to amount `0.0982607`. The trade
one ledger later, 61143619, fills `0.0982606` of it. The difference is

    0.0982607 − 0.0982606 = 0.0000001

which is the `best_ask_amount` the series reports on every row from 9 February onward, and
9 February is the first sampled ledger after 61143619.

---

## 2. Nothing removed it, by either route the fold can see

Two independent readings, both from Horizon on 12 September 2026.

**The trade route.** `GET /offers/1822775941/trades` returns **exactly two trades**, the
two in the table above, both on 8 February. Horizon's own per-offer index therefore agrees
with the repository's CSV, which also closes the P2-24 worry for this one offer: the CSV
is not missing a fill against it.

**The operation route.** A forward walk of the owner's operations from ledger 61143617,
two pages of 200 records, reaches ledger 61344294, which is past the control ledger
61340263. In that whole range the offer id `1822775941` appears in **five operations, all
at ledger 61143618**, all `manage_sell_offer`, all with `transaction_successful: true`, and
never again afterwards.

    A=GBPFB6XNLDMXQKOFJAH6IRTOMTEUU4ZWFHNRMYWZNXCZEDNE6UU66WSG
    curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61143617*4294967296))&order=asc&limit=200&join=transactions"

So between 8 February and ledger 61344294 the offer was neither traded nor modified. A
walk that went deeper, or a cap that went higher, would find exactly the same nothing.
**That is why section 6 of the 8 September document prices the wrong fix.** Its three
hours buy more pages, and there is no page to find.

**The third route, added after this document was first written, and it is closed too.**
USTRY is an authorized asset, so the issuer can delete a holder's offers without the
holder acting, by revoking authorization or by clawing the asset back. It did not. A walk
of the issuer `GCRYUGD5` across the same range returns **59 operations in total**, of which
46 are `invoke_host_function`, 10 are `payment` and 3 are `create_claimable_balance`.
There is no `allow_trust`, no `set_trust_line_flags` and no `clawback`, and **no operation
of any kind naming `GBPFB6XN` other than payments**.

    curl -s "https://horizon.stellar.org/accounts/GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC/operations?cursor=$((61143618*4294967296))&order=asc&limit=200&join=transactions"

---

## 3. The bid that crosses it is real, and that is the half that decides

The crossed row this repository can check most cheaply is 22 February, ledger `61340172`,
the daily sample. Its `best_bid` is `1.057426943`, the ratio `2125646195/2010206197`.

That ratio resolves to offer **`1824767559`**, owned by
`GABFRFPYM2BXM4OM2ZA4YDBWY4CMPVESHQMKXSM47MWWJD4TW2KQDWWN`, which is the other of the two
accounts the 8 September document names as holding the book. It appears in 19 trades:

| | Ledger | Time |
|---|---|---|
| first fill | 61337700 | 2026-02-21T20:02:34Z |
| last fill | 61340224 | 2026-02-22T00:06:31Z |

**The sample ledger 61340172 sits between those two.** An offer that trades before a ledger
and trades again after it was resting at it. No walk depth, no floor and no cap changes
that, so the bid is not in question.

**Both sides of the 22 February row cannot be right.** The bid is proven resting. The book
is crossed. Therefore the ask is the phantom, and the direction of the defect is settled
even though its mechanism is not.

---

## 4. Which rows to trust, and it is fewer than the last reading said

`crossed` is now a column in the CSV and a line in the sidecar, so this table is
reproducible rather than hand-made. On the cap400 run:

| Rows | Verdict |
|---|---|
| 1 to 8 February | the phantom does not exist yet. Not implicated by this document |
| 9 to 21 February | the phantom is on the book, priced below the real ask, and no bid reaches it. **`best_ask` is wrong on every one of these rows**, and so is every figure derived from it: `p0`, `spread_pct`, both depth ladders and the band |
| 22 February, ledger 61340172 | **crossed and proven so.** The cap400 run crosses here, one day earlier than the 8 September reading found, because that reading used the shallower run whose bids were lower |
| the two control ledgers | `best_bid` and `ask_amount_total` match the fixture. `best_ask` does not, and section 5 proves the fixture is the one that is right |
| 23 to 28 February | crossed. Beyond ledger 61344294 this document did not walk, so these are crossed by the CSV's own arithmetic rather than by a reading taken here |

**The correction to the earlier reading is that the damage starts on 9 February and not on
23 February.** A crossed book announces itself; a phantom ask that is merely cheaper than
the real one does not, and it is wrong in exactly the same way on thirteen quiet rows
where nothing looks amiss. The band flip to `LOW` on 25 February is still not citable, and
now neither is any `best_ask` after 8 February.

---

## 5. The incident ledger settles it, and the fixture stands

There is a test sharper than any walk, and its data was already committed to this
repository. **The manipulation at ledger 61340263 was a buy.** A buy fills from the
cheapest ask upward, so if a one stroop ask at `1.0573892` had been resting, the
manipulation would have consumed it *before* reaching the ask at `106.7372828`.

Ledger 61340263 contains **exactly one trade**:

| fill | price ratio | price | base amount | counter amount | resting offer |
|---|---|---|---|---|---|
| 0 | `266843207/2500000` | `106.7372828` | `0.0501003` USTRY | `5.3475699` USDC | `1824788980` |

No fill at `1.0573892`. **So offer `1822775941` was not on the book at ledger 61340263.**

**The golden fixture is right and the reconstruction is wrong.** That closes the question
`docs/evidences/2026-09-08-february-book-series.md` section 3 declined to pick, and it
picks against that document's reading 1: the fixture is not incomplete, and its zero depth
argument is not wrong.

### 5.1 The removal is dated to six minutes on 8 February

The same logic dates it. Any trade after the ghost's last fill, priced **above**
`1.057389202946`, had to clear that offer first. The first one is at ledger **61143682**,
`2026-02-08T18:40:14Z`, three fills against offer `1822763294` at `1.05738925629949`.

And between the ghost's last fill and that ledger there were **no trades at all**:

| Boundary | Ledger | Time |
|---|---|---|
| the ghost is left at one stroop | 61143619 | 2026-02-08T18:34:08Z |
| a higher-priced ask trades, so the ghost is gone | 61143682 | 2026-02-08T18:40:14Z |

**Six minutes and six seconds.** The offer was removed inside that window, with no trade
against it, no operation naming it, and no action by the issuer.

**So the reconstruction carries a phantom for TWENTY DAYS**, 9 to 28 February, after the
offer it represents had already left the book. That is the measure of the defect, and it
is far larger than the six crossed rows that announced it.

### 5.2 What removed it, stated as a candidate and not as a finding

Everything a trade-and-operation fold can see has been eliminated. What remains is the
ledger acting on its own, and the candidate that fits every observed detail is the
matching engine's own dust handling: an offer whose remainder cannot produce a non-zero
exchange is dropped during crossing, and no claim atom, and therefore no Horizon trade, is
emitted for it. The remainder here is **one stroop**, the smallest amount that case can
apply to.

**This document does not claim that mechanism is proven.** It claims the elimination,
which is proven, and it names the candidate so the next reader does not start over.

**What follows regardless of the mechanism, and this is the part that matters:** an offer
left the book without emitting either of the two event types this reconstruction is built
from. **A deeper walk cannot fix that, and neither can a wider trade window.** The fix has
to reconcile against something that reports state rather than events. That is a design
question about whether the historical path in `docs/methodology/` may claim event-sourced
reconstruction at all, and it is Al's.

---

## 6. What changed in the code, and what it does not claim

`domain.OrderBook.Crossed()` reports whether the best bid is at or above the best ask, and
returns the two levels when it is. `SeriesPoint` and `ReplayResult` carry it, both
`Complete()` methods now refuse a crossed book, `bookseries` writes a `crossed` column and
a `crossed_points` line in the sidecar and names each crossed row with both prices, and
`keel replay` prints the two ratios.

**It is a detector and not a repair.** Dropping the crossing level would make the book
possible and leave it wrong, and this document is the demonstration that deciding which of
the two offers is the phantom took a walk of each side. Nothing in the book alone can do it.

**It would not have caught the 9 to 21 February rows**, and that is the honest limit of it.
Those rows are wrong and not crossed. What the detector guarantees is narrower than it
looks: no run can now publish a provably impossible row in silence, which is the one thing
`tugas-b.md` B1 calls unacceptable.

---

## 7. Reproducing this

Everything in sections 1 and 3 is offline, against a file already committed:

```bash
python3 - <<'EOF'
import csv
rows=list(csv.DictReader(open(
  "docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv")))
for oid in ("1822775941","1824767559"):
    hits=[r for r in rows if oid in (r['base_offer_id'], r['counter_offer_id'])]
    print(oid, len(hits), hits[0]['ledger_seq'], hits[-1]['ledger_seq'],
          {f"{r['price_n']}/{r['price_d']}" for r in hits})
EOF
```

Section 5 is offline too, and it is the one that settles the question:

```bash
python3 - <<'EOF'
import csv
rows=list(csv.DictReader(open(
  "docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-trades-2026-02-01_2026-03-01.csv")))
print("fills in the incident ledger 61340263:")
for r in rows:
    if r['ledger_seq']=='61340263':
        print("  ", r['price_n']+"/"+r['price_d'], r['base_amount'], r['base_offer_id'])
print("trades in ledgers 61143620..61143682:")
for r in rows:
    if 61143620 <= int(r['ledger_seq']) <= 61143682:
        print("  ", r['ledger_seq'], r['price_quote_per_base'], r['base_offer_id'])
EOF
```

Section 2 is three Horizon reads:

```bash
curl -s "https://horizon.stellar.org/offers/1822775941/trades?limit=200&order=asc"
A=GBPFB6XNLDMXQKOFJAH6IRTOMTEUU4ZWFHNRMYWZNXCZEDNE6UU66WSG
curl -s "https://horizon.stellar.org/accounts/$A/operations?cursor=$((61143617*4294967296))&order=asc&limit=200&join=transactions"
I=GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC
curl -s "https://horizon.stellar.org/accounts/$I/operations?cursor=$((61143618*4294967296))&order=asc&limit=200&join=transactions"
```

The detector:

```bash
go test ./internal/domain/ -run TestCrossedBook -v
```

---

## 8. Version history

| Date | Change |
|---|---|
| 12 September 2026 | Written. The phantom ask resolved to offer `1822775941`, the crossing bid to offer `1824767559`, the deeper-walk fix ruled out, and the crossed-book detector added to `internal/domain`, `internal/horizon` and `cmd/keel` |
| 12 September 2026, later | The issuer route closed: 59 operations in the window and none touches `GBPFB6XN`'s trustline, so authorization and clawback are out. Then section 5 settled the whole question from a file already in the repository: the incident ledger 61340263 holds exactly one fill, at `106.7372828`, so the dust ask was not on the book and **the golden fixture stands**. Removal dated to six minutes on 8 February, ledgers 61143619 to 61143682. The phantom is carried for twenty days, not six rows |
