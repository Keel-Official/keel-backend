# Historical replay validated against the control ledger, 5 September 2026

**What it is evidence for:** Deliverable 1, acceptance criterion 2, which reads
verbatim in `docs/context/Keel_PRD.md` section 9:

> - [ ] FR-12 through FR-17 work, historical replay validated against a control ledger

This document is the second clause. The first clause is a different question and
section 6 says where it stands.

**Read on:** 5 September 2026, from Horizon mainnet, no account required.
**Control ledger:** **61340262**, the state at the END of that ledger, which is the
state at the START of ledger 61340263, the ledger the manipulation executed in.
That is exactly what `testdata/fixtures/ustry_pre_exploit.md` describes: "the state
of the book immediately **before** the manipulation trade executed inside ledger
61340263".
**Second target:** **61340263**, the same book with the manipulation applied.
**Pair:** `USTRY` `GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC`
against `USDC` `GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`, the
identity fixed by DEC-001.
**Method:** `keel bookseries`, one operation walk folded at two targets. See
`internal/horizon/series.go` and `cmd/keel/bookseries.go`.
**Artefacts:** `USTRY.GCRYUGD5-USDC.GA5ZSEJY-control-ledger-61340262.csv` and its
`.meta.txt` sidecar, both in this directory.

**Reproduce it:**

```bash
go run ./cmd/keel bookseries \
  -pairs scripts/record-pairs.example.json \
  -also-ledger 61340262,61340263 \
  -trades-from-ledger 61300000 -since-ledger 61300000 -lookahead 5000 \
  -csv docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-control-ledger-61340262.csv
```

---

## 1. The result: every hand-computed value in the golden fixture, rebuilt

The fixture's numbers were **computed by hand in a spreadsheet before any
implementation existed** and its own header says so. `testdata/fixtures/` is RED and
enforced in both the deny list and the hook, so those numbers are not reachable from
the side of the wall that wrote the code being tested. Agreement with them is
therefore not this code agreeing with itself, which is the only reason this
document is worth anything.

| Quantity | Fixture, computed by hand | Rebuilt at ledger 61340262 | |
|---|---|---|---|
| best ask price | 106.7372828 | 106.7372828 | match |
| best bid price | 1.0570000 | 1.057 | match |
| `priceSource` | book | book | match |
| `P0` | 53.8971414 | 53.8971414 | match |
| `spreadPct` | 196.0777141 | 196.07771405850477999562329293 | match |
| depth buy, δ 0.02 | 0 | 0 | match |
| depth sell, δ 0.02 | 0 | 0 | match |
| depth buy, δ 0.05 | 0 | 0 | match |
| depth sell, δ 0.05 | 0 | 0 | match |
| depth buy, δ 0.10 | 0 | 0 | match |
| depth sell, δ 0.10 | 0 | 0 | match |
| manipulation target, δ 0.5 | 80.84571210 | 80.8457121 | match |
| manipulation cost, δ 0.5 | 0 | 0 | match |
| reachable, δ 0.5 | true | true | match |
| `maxReachablePrice` | 106.7372828 | 106.7372828 | match |
| `costToMaxReachablePrice` | 0 | 0 | match |
| bids on the book | 1 | 1 | match |
| asks on the book | 1 | 1 | match |
| band | CRITICAL, by `09-flags-and-bands.md` | CRITICAL | match |

The flags at that ledger are `MANIPULATION_CHEAP`, `SPREAD_EXTREME`,
`THIN_DEPTH_5PCT` and `ZERO_DEPTH_2PCT`, with `bandConfidence` **partial** because
six flags need supply data, trade history or trustline distribution that a book
snapshot cannot carry. `unevaluatedFlags` lists all six rather than reporting them
as clear, which is `09-flags-and-bands.md` section 2 working as written.

`spreadPct` is the one row where the two columns are not the same string.
196.07771405850477999562329293 is the exact quotient the engine carries as a
decimal; 196.0777141 is that number at the seven places the fixture writes its
figures to. They are the same value, and the difference is what rounding is.

**What was NOT compared.** The fixture is a book with no pool, `Pools: []`, and this
reconstruction rebuilds no pool either. So the agreement above says nothing about
the AMM half of the methodology. `10-validation.md` section 1 already records that
the golden fixture's with-pool tables are not computed, and DEC-006 is where the
pool in the fixture is discussed. Nothing here closes that.

## 2. What one walk cost, and what it did not reach

From the sidecar, and every figure below is in it rather than only here.

| | |
|---|---|
| accounts walked | 65, of which 58 came from the trade stream and 7 from offers resting today |
| trades read | 740 |
| operations read | 48,588 |
| offer operations applied | 229 |
| requests | 374 |
| elapsed | 2,841 seconds, 47 minutes and 21 seconds |
| operation floor | ledger 61300000 |
| earliest offer operation reached | ledger 61303621 |
| walks that stopped at the floor | 42 |
| **walks truncated at the page cap** | **7** |
| **walks that failed** | **10** |
| `walk_complete` | **false** |

**The walk was incomplete and the answer was still right.** Ten of sixty-five
account walks failed on 503s from public Horizon and seven more truncated at the
page cap, and `keel bookseries` prints `WALK INCOMPLETE` on every run that ends this
way. That is the conservative direction and it is why the reconstruction is trusted
here rather than in spite of it: a walk that fails loses offers, a lost offer is a
level that does not appear, and a book missing a level reads as **thinner** than the
market was. The failure mode of this method is to understate liquidity, which
overstates risk, which is principle P-2.

It also means this run is a **lower bound** on the book at that ledger, and it
happens to coincide with the hand-computed answer. A run that had lost the ask at
106.7372828 would have produced a different book and this document would say so.

**`missing_offer_ids` is 28 on both rows and that number needs reading carefully.**
It counts offers that a trade AFTER the target names as resting and that the replay
never saw created. The lookahead is 5,000 ledgers past the target, and the target is
the ledger the manipulation executed in: the pair went from 93 trades on 21 February
to 2,715 on the 22nd. So the window this check looks into is the busiest in the
pair's history, and most of those 28 are offers created after the target rather than
holes before it. The check is loud in exactly this window by construction, and
`fold_complete` reads false because of it.

## 3. The two ledgers came out identical, and that was a defect in the reporting

The two rows in the artefact CSV are **byte-identical**, and they should not be. The
manipulation trade sits between them.

The cause is not the reconstruction. It is that the trade changed the ask's
**amount** from 1.2185312 to 1.1684309 and did not touch its price, and the CSV
carried no amount column. Every depth column is zero on both rows and correctly so:
a 196 per cent spread puts every δ target outside the book, which is the fixture's
own explanation for those zeros. So on the one book in this repository that matters
most, the depth ladder cannot carry size, and nothing else did.

Three defects in the reporting layer were found by this validation and all three are
fixed, in `04cc735` and the commit that carries this document:

1. **No size anywhere.** Four columns added: `best_bid_amount`, `best_ask_amount`,
   `bid_amount_total`, `ask_amount_total`. The side totals are the size posted
   independently of the midpoint, which is the figure that stays meaningful when the
   midpoint does not.
2. **The manipulation ladder was the wrong ladder.** The columns were built from
   `rungs()`, which is the backtest's ladder of market deltas plus the critical
   delta. Three of the four manipulation columns were therefore named after deltas
   that are not on the manipulation ladder and were permanently empty, and the rungs
   the methodology defines at 1, 10 and 100 had no column at all. The fixture's
   manipulation table is what that hid: cost 130.0627093 with `reachable` false at
   δ 1, 10 and 100, against cost 0 with `reachable` true at δ 0.5. The fixture calls
   the difference between those two kinds of zero the point of the table, and only
   one of them was in the file.
3. **A missing regression.** Two books differing only in the size at the top of the
   ask side must not write identical rows. That is now a test.

**The artefact in this directory is the run as it happened and is not regenerated.**
It carries the column set of the run that produced it, so `mc_cost_1`, `mc_cost_10`
and `mc_cost_100` are absent from it and the amount columns are too. Rewriting it
would be reporting a run that never took place. The comparison in section 1 is
unaffected: every value in it was read from that file or from the run's own summary.
The complete column set lands with the February series, which carries this same
control ledger as one of its targets.

## 4. What this closes and what it does not

**Closes:** the second clause of acceptance criterion 2, "historical replay
validated against a control ledger". A historical book was rebuilt at a named
control ledger from a source that serves no historical books, and every quantity the
methodology derives from it matched a set of numbers worked by hand before the code
existed.

**Does not close:** the first clause, "FR-12 through FR-17 work". FR-13 is reading a
historical ledger snapshot from **Hubble**, it is priority M, `internal/hubble/`
holds one `CLAUDE.md` and no Go, and Al held that road on 5 September 2026. FR-14,
both sources returning an identical `Snapshot` shape, cannot be tested while one
source exists. DEC-002 section 8 is where that stands.

**Does not close criterion 3 either**, and the distinction is the one DEC-002
section 8.2 records. This is Horizon rebuilt from Horizon operations. It is not a
second reader, and criterion 3 names Hubble.

## 5. Why this is not the same thing as the 26 August reading

`2026-08-26-ustry-book-replayed-from-operations.md` rebuilt the same control ledger
and reported the same book. This run differs in three ways that matter:

1. **One walk, two targets.** The reconstruction was folded at 61340262 and at
   61340263 from a single set of operations, which is the property the February
   series depends on. A fold that only worked for the target it fetched at would
   have shown up here as a wrong second row.
2. **The methodology ran over it**, at version 1.0.8-draft, producing the flags, the
   band and the whole depth and manipulation ladder, rather than the book alone.
3. **It produced a machine-readable artefact with a provenance sidecar**, in the
   shape DEC-010 requires, rather than a reading transcribed into prose.

## 6. Version history

| Date | Change |
|---|---|
| 5 September 2026 | Created. Records the control ledger validation, the three reporting defects it found, and what it does and does not close |
