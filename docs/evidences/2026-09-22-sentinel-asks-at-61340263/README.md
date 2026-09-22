# The sentinel asks at ledger 61340263 were resting, and the fixture does not hold them

**Read on:** 22 September 2026, from public Horizon mainnet, no account required.
**Pair:** `USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC` against
`USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN`.
**Ledger:** `61340263`, closed 2026-02-22T00:10:21Z, the ledger the manipulation executed in.
**MethodologyVersion:** none. Everything here is an offer identity, an account, a ledger
sequence or an operation, read from Horizon or from the replay's own fold.

**What this answers.** Report section 5.4 records that the repaired February series sees
an ask at price `2147483647` (`2^31 − 1`) at ledger 61340263 that the hand-computed fixture
does not hold. That one level moves four quantities: `maxReachablePrice` (106.7372828 by
hand, 2147483647 in the run), `costToMaxReachablePrice` (0 against 124.715139), and
`Reachable` at δ = 1, 10 and 100 (false against true). Section 10 names two readings and
leaves the choice open:

- **A.** The offer had left the book without emitting an event, which is the DEC-021 class,
  and belongs in `configs/known-removals.json`.
- **B.** The offer was genuinely resting and the hand computation did not include it.

**The answer is B, and the level is two offers rather than one.**

---

## 1. Which offers the level is

`keel replay -dump-offers` (added on branch `feat/replay-dump-offers`), run with the repaired
series' own parameters:

```
keel replay -pairs scripts/record-pairs.example.json -ledger 61340263 \
  -trades-from-ledger 60987032 -since-ledger 60987032 -lookahead 5000 \
  -max-pages-per-account 60 -max-pages-per-offering-account 400 \
  -known-removals configs/known-removals.json -dump-offers
```

The full output is `replay-61340263-dump-offers.txt` beside this file. The two asks at the
sentinel price:

| offer_id | seller | price_r | amount | last written at ledger | by operation |
|---|---|---|---|---|---|
| `1823051768` | `GBPFB6XNLDMXQKOFJAH6IRTOMTEUU4ZWFHNRMYWZNXCZEDNE6UU66WSG` | `2147483647/1` | 0.0000001 | 61158581 | `262674105265487873` |
| `1823841098` | same | `2147483647/1` | 0.0000001 | 61238659 | `263018037656375297` |

Together they are the 0.0000002 USTRY ask level at `2147483647` that the repaired series
shows at this ledger. The other two resting asks are the fixture's own, offer
`1824788980` at `266843207/2500000` by `GCNF5GNR`, partly consumed to 1.1684309.

The run is not complete, and its completeness line says so: 31 walks truncated, 153 at the
floor, 6 failed on Horizon 503, and 24 offers named by later trades never seen created. Each
of those can only REMOVE offers from the book. None of them can put these two on it, so the
gaps do not weaken the finding that they were there.

## 2. That both were resting at 61340263

Checked on Horizon for each offer:

| | `1823051768` | `1823841098` |
|---|---|---|
| `GET /offers/{id}` today | 404, gone | 404, gone |
| Operations by the seller naming it, at or before ledger 61340263 | **0** | **0** |
| First operation naming it after 61340263 | ledger 61344680, 2026-02-22T07:19:29Z | ledger 61344270, 2026-02-22T06:39:58Z |
| Operations naming it on 22 February | 55 | 245 |
| `GET /offers/{id}/trades` | fills from 07:19:29Z | fills from 06:39:58Z |

The operation scan read the seller's `/operations` ascending from ledger 61156000, 81 pages.

**An operation that names an offer by ID six hours after the exploit, as an update, proves
the offer existed then.** No operation touched either offer between its last write in
February and that update, and a resting offer leaves the book only through an operation, a
fill, or the protocol adjustment DEC-021 covers. There was no operation, the first fills are
at 06:39 and 07:19, and a protocol adjustment would have deleted the offer, which the later
update rules out. So both were on the book at 00:10:21Z.

The seller is a dust-posting account. Between ledger 60988098 (29 January) and 61238659 (15
February) it submitted 97 creates of 0.0000001 USTRY at `2147483647/1`, most of which do not
survive as offers, and it posts the same pattern today (offers `1835857810`, `1835894780`,
`1857685873` and `1858401538` rest on the live book as of this reading).

## 3. What follows, and whose each part is

1. **The fixture is incomplete by two dust asks.** `testdata/fixtures/ustry_pre_exploit.md`
   lists one ask. `testdata/fixtures/` is RED and hook-locked, so the correction is Al's by
   hand. What it does NOT change: best bid, best ask, `P0` 53.8971414, `spreadPct`, the
   whole depth ladder and the four flags, because 0.0000002 USTRY at `2^31 − 1` sits far
   outside every depth rung. What it DOES change is exactly report 5.4's four quantities, and
   the run's values are the right ones for the book that existed.
2. **Nothing belongs in `configs/known-removals.json`.** Reading A is ruled out.
3. **A methodology question the finding raises, which is Al's and is not answered here.**
   Under `05-manipulation-cost.md` as written, a target is `Reachable` when any ask at or
   above it exists, so two ten-millionths of a USTRY at `2^31 − 1` make every rung reachable
   and move `maxReachablePrice` to a price nobody could pay. The rule is applied correctly;
   whether dust should count toward reachability is a definition choice. `07` section 1
   already has a dust threshold of 0.01 USDC for trades, and this is the same question asked
   of resting offers.
4. **Report 5.4 and section 10** can now state the resolution: the four quantities differ
   because the fixture omits two resting dust asks, identified here by ID.
