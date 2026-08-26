# Replaying the USTRY/USDC book at the incident ledger, from operations alone

**Read on:** 26 August 2026, from Horizon mainnet, no account required.
**Control ledger:** 61340262, which is the state at the START of ledger 61340263,
the ledger the manipulation executed in. That is exactly what
`testdata/fixtures/ustry_pre_exploit.md` describes: "the state of the book
immediately before the manipulation trade executed inside ledger 61340263".
**Method:** DEC-002 section 2.3, built. `keel replay`, and
`internal/horizon/replay.go` for how.

**Reproduce it:**

```bash
go run ./cmd/keel replay \
  -pairs scripts/record-pairs.example.json \
  -ledger 61340262 \
  -trades-from-ledger 61300000 -since-ledger 61300000 \
  -lookahead 5000 -compute
```

`-trades-from-ledger` and `-since-ledger` are the SAME value, and that is not a
coincidence. Section 4.1 is what happens when they are not.

---

## 1. The result: every expected value in the golden fixture, rebuilt

```
USTRY/USDC at ledger 61340262
  dataSource offers-implied, 1 bid level(s), 1 ask level(s)
    ask 0  price 106.7372828 (266843207/2500000)  amount 1.2185312
    bid 0  price 1.057 (1057/1000)  amount 0.0001
  read 740 trade(s), walked 64 account(s) (58 from trades, 6 only from the live offer book)
  228 offer operation(s) applied, 358 Horizon request(s)

  --- methodology 1.0.3-draft over the reconstructed book, ORDER BOOK ONLY, no pool ---
    P0 53.8971414 from book, spread 196.0777141 percent
    depth  delta 0.02  buy 0  sell 0
    depth  delta 0.05  buy 0  sell 0
    depth  delta 0.1   buy 0  sell 0
    cost   delta 0.5   target 80.8457121    cost 0                   reachable true
    cost   delta 1     target 107.7942828   cost 130.06270929502336  reachable false
    cost   delta 10    target 592.8685554   cost 130.06270929502336  reachable false
    cost   delta 100   target 5443.6112814  cost 130.06270929502336  reachable false
    maxReachablePrice 106.7372828  costToMaxReachablePrice 0
    band CRITICAL (partial), flags [ZERO_DEPTH_2PCT MANIPULATION_CHEAP SPREAD_EXTREME THIN_DEPTH_5PCT]
    unevaluated [MANIPULATION_RATIO_LOW NO_GENUINE_TRADE_30D HOLDER_CONCENTRATION_EXTREME
                 NO_GENUINE_TRADE_7D HOLDER_CONCENTRATION_HIGH WASH_TRADE_SUSPECTED]
```

Every one of those numbers is in the golden fixture, computed by hand before any
implementation existed. The two book levels match including the price as an exact
rational. `P0`, `spreadPct`, all six depth cells, all four manipulation rungs with
their reachability, both max-reachable figures, the band, the four triggered flags
and the six unevaluated ones all match.

Nothing on that path touched an order book endpoint, because Horizon serves none for
a past ledger. The book came out of the operations that posted it and the results
those operations returned, and then the ordinary engine ran over it.

That satisfies the Deliverable 1 criterion "historical replay validated against a
control ledger", against the strongest control this repository has.

**Section 3 is why that agreement is worth less than it looks.** The run is bounded
at ledger 61300000, and two offers that were resting at the target were created
before that bound. The reconstruction agreed partly because it was not looking far
enough back to disagree, and the completeness line is what says so.

---

## 2. What had to be solved to get there

Three things, each measured rather than assumed, each with the evidence in a test.

**Horizon never tells you the id of an offer it just created.** A create comes back
as `"offer_id": "0"` and its effects collection is EMPTY. Measured on operation
263453036239003649, the one that posted the 106.7372828 ask. The id exists in one
place Horizon serves: the transaction's `result_xdr`. So
`internal/horizon/offerxdr.go` decodes it, and
`TestTheAskOfTheFixtureDecodesFromItsOwnTransaction` recovers 1824788980 out of
that transaction's 272 bytes.

**A manage buy offer states its amount in the asset it is BUYING.** The operation
that posted the fixture's bid asked to buy 0.0001 USTRY at 1.0570000. The ledger
stored an offer SELLING 0.0001057 USDC at a price of 1000/1057. Neither number in
the request is the number on the book, and the price is upside down relative to how
the book reports it. Reading the request instead of the result puts a bid on the
book at the wrong size in the wrong unit.

**A trade names an offer on both sides even when one side never rested.** A taker
that crosses the book completely has no ledger offer, so Horizon synthesises one by
OR-ing the operation's TOID with bit 62:

```
operation TOID       263454423513071617  0x03a7fa6700008001
counter_offer_id    4875140441940459521  0x43a7fa6700008001
```

9,478 of the 10,077 distinct offer ids in USTRY/USDC's February 2026 trades are
synthetic. Before that was understood the completeness check in section 4 reported
367 missing offers. It reports 28.

---

## 3. A finding about the fixture: two offers were resting that it does not have

At ledger 61340262 the account
`GBPFB6XNLDMXQKOFJAH6IRTOMTEUU4ZWFHNRMYWZNXCZEDNE6UU66WSG`, a market maker, held
two USTRY/USDC offers, both PARKED:

| Offer | Last touched | Selling | Amount | Price (n/d) | As quote per base |
|---|---|---|---|---|---|
| 1823025211 | ledger 61156591, 2026-02-09T15:23:32Z | USDC | 0.0000001 | 2147483647/1 | a bid at about 4.66e-10 |
| 1823841098 | ledger 61238659, 2026-02-15T04:12:29Z | USTRY | 0.0000001 | 2147483647/1 | **an ask at 2,147,483,647** |

`2147483647` is `2^31 - 1`, the largest value an XDR price numerator holds. Parking
an offer at the maximum price with a one stroop amount is how a market making bot
takes a quote off the market without giving up the offer.

**This was established by direct Horizon queries and does NOT rest on the replay.**
Each link is a reading anybody can repeat:

1. Both offers were last modified before the control ledger, at the ledgers above,
   and the state recorded there is the state in the table. Read out of each
   operation's `result_xdr`, because both were creates and a create names no id in
   JSON.
2. The account performed NO operation at all between ledger 61246996
   (2026-02-15T17:45:41Z) and ledger 61344270 (2026-02-22T06:39:58Z). Walked in both
   directions from the control ledger.
3. Its first operation after the control ledger, at 61344270, is a
   `manage_sell_offer` naming `offer_id 1823841098`. That is an UPDATE, so the offer
   still existed, and an offer id is never reused, so it had existed continuously.
4. Neither offer traded before the control ledger. First fills are at ledgers
   61344270 and 61344278, five and a half hours after it.

So both were resting at ledger 61340262, and the fixture's book has neither.

### 3.1 What it does NOT change, which is most of the fixture

The parked ask sits far above the 106.7372828 ask and the parked bid far below the
1.057 bid, so neither is the best on its side:

| Quantity | Fixture | With the parked offers |
|---|---|---|
| `P0` | 53.8971414 | unchanged |
| `priceSource` | book | unchanged |
| `spreadPct` | 196.0777141 | unchanged |
| Depth at ±2, ±5, ±10 percent | zero on every rung | unchanged: both levels are outside every window |
| Flags and band | CRITICAL | unchanged |

**It reconciles with the incident rather than contradicting it.**
`10-validation.md` section 7 argues that order book manipulation cost was zero
because the manipulating operation produced exactly one trade record, which proves
no third party ask sat between 1.057 and 106.74. A parked ask at 2.1 billion is not
between them. That argument survives intact, and the parking is the reason this
market maker's real quotes, around 1.0574 bid and 1.0584 ask when it resumed on the
morning of the 22nd, were not in the attack's way.

### 3.2 What it does change, and it is the line the fixture calls its most important

`maxReachablePrice` is defined as the highest ask price in the book. That is the
fixture's own invariant 3, and `TestInvarianMaxReachableAdalahAskTertinggi` asserts
it.

| Quantity | Fixture | With the parked ask |
|---|---|---|
| `maxReachablePrice` | 106.7372828 | **2,147,483,647** |
| `costToMaxReachablePrice` | 0 | **130.0627093**, the cost of clearing the 106.74 ask, which is the only ask strictly below the target |

The fixture says of that pair: "This is the most important line in the whole
fixture. The highest price an attacker can reach is 106.74 and reaching it is free."

Under the same definition applied to the fuller book, the sentence becomes: the
highest price an attacker can reach is 2.1 billion, and reaching it costs 130.06
USDC. That is not a weaker claim than the fixture's. It is a considerably stronger
one.

### 3.3 Reported, not resolved

`testdata/fixtures/` is RED. `internal/conformance/fixture.go` says it in its own
header: adjust the code to match those numbers, never the reverse, and the same
protects the inputs. Nothing here has been edited. Two questions come out of it and
both are Al's:

1. **Is the fixture's book input incomplete?** The fixture states where its two
   levels came from: two operations by one account. It never claimed to have
   enumerated every account, and this is that gap. Closing it means recomputing
   `maxReachablePrice` and `costToMaxReachablePrice` by hand.
2. **Is "the highest ask in the book" the right definition?** The larger question,
   and it is not about USTRY. Parked dust at extreme prices is a normal market
   making pattern, so on a real book `maxReachablePrice` will usually be whatever
   absurd number somebody parked at, and its cost the cost of clearing everything
   genuine below it. Whether that is the intended reading, or whether the definition
   needs a notional floor, belongs to `docs/methodology/05-manipulation-cost.md`.

### 3.4 One existing audit note is refined by this

`scripts/audit-verification.sh` prints:

> PROVENANCE bid 1.057 is not a Horizon reading. Nearest on-ledger figure:
> 1.0574630, offer 1823025211

Offer 1823025211 is one of the two parked offers. At the control ledger it was not
at 1.0574630 at all; it was parked at 2147483647/1 with a one stroop amount, and it
was re-priced to 1.0574630 on 22 February at 06:40:42, five and a half hours after
the incident. The note's number is real and its ledger is the wrong one.

---

## 4. A finding about the tool, and it is the more important one

### 4.1 The two windows have to line up, and the failure runs the wrong way

Offers come from the operation walk. Consumption comes from the trade walk. An
offer created inside the operation window and eaten BEFORE the trade window starts
is never decremented, and it rests on the reconstructed book for ever.

**Measured, on the run that found it.** With no operation floor and a ten thousand
ledger trade window, the reconstruction applied **8,253 offer operations against 489
trades** and produced **334 asks starting at 1.0527** for a ledger the chain proves
had one ask at 106.7372828. Every completeness counter read clean: 0 unsizable, 24
missing, no warning of any kind.

That direction is what makes it the serious one. Every other gap in this method
loses offers and makes a book look THINNER, which is conservative for a product
whose job is to warn. This one keeps offers that were already eaten and makes the
market look DEEPER than it was, which is the failure this product exists to prevent
and the one it must never commit itself.

Three things changed because of it:

- `ReconstructBook` now REFUSES the configurations that cause it. An unbounded
  operation walk requires an unbounded trade walk, and a trade window may not start
  after the operation floor. Refusing beats reporting here, because the output of
  the bad configuration is a plausible looking deep book.
- The result carries `EarliestOfferOp` and `TradeWindowFrom`, so the residue is
  measurable: a page of operations can straddle the floor and reach one record below
  it. `MayBeInflated()` is separate from the other counters and `Complete()` fails on
  it.
- `keel replay` prints both windows on every run and shouts on inflation.

`TestAWindowPairingThatWouldInflateTheBookIsRefused` and
`TestTheInflationFlagIsSeparateFromTheOtherGaps` hold it.

### 4.2 The gaps that remain, counted on every run

```
completeness: 10 account walk(s) truncated, 49 stopped at the ledger floor, 0 failed,
              0 result(s) unsizable, 28 offer(s) named by trades but never seen
windows: offers applied back to ledger 61303621, trades read from ledger 61300000
```

The windows line reads correctly here: offers were applied from 61303621 and trades
from 61300000, so the trade walk covers every offer the operation walk produced, and
nothing on this book had already been eaten.

| Gap | What it is | Direction |
|---|---|---|
| A failed walk | public Horizon answers 503 under a deep sustained walk. Counted, not fatal | thinner |
| Account discovery | an account that posted an offer, never traded it, and does not rest today is invisible. Trades are read PAST the target to shrink it, and the live offer book adds six accounts here | thinner |
| Walk depth | each account is walked backwards to a page cap and to a ledger floor, both parameters | thinner |
| Result decoding | an operation whose transaction holds an EARLIER operation of unknown width is dropped rather than guessed | thinner |
| Window misalignment | section 4.1 | **deeper**, and now refused |

**`MissingOfferIDs` is the self-check for the first three** and it catches all of
them at once: an offer a trade names that this replay never saw cannot be on the
book. It does NOT catch the fourth, which is why the fourth needed its own
measurement. An empty list is the strongest statement the method can make about
itself, and it is still not a proof of correctness.

**Pools are not reconstructed at all.** The snapshot carries none, which is not a
claim that no pool existed; DEC-006 establishes that one did, holding 16.3389179
USDC and 15.4791416 USTRY at that ledger. `/liquidity_pools/{id}/operations` can
answer the pool side and DEC-002 section 2.3 calls it the cleaner half, because it
has no discovery gap. Until it is written, any depth computed from a replayed
snapshot is ORDER BOOK ONLY, and presenting it as combined depth is the error
DEC-006 section 4 is about.

---

## 5. Cost

| | Bounded run of section 1 | Unbounded run of section 4.1 |
|---|---|---|
| Accounts walked | 64 | 38 |
| Horizon requests | 358 | 1,136 |
| Trades read | 740 | 489 |
| Offer operations applied | 228 | 8,253 |

**Public Horizon refuses a walk much deeper than this.** A sixty page cap with the
floor at ledger 61150000 drew a 503 that survived five retries partway through, on
26 August 2026. One account failing no longer aborts the reconstruction: the walk is
recorded as failed and counted, and the run continues, because losing one account
makes the book thinner and throwing away the other sixty-three does not make that
better.

The dominant cost is accounts with no offers on the pair at all. Horizon's
`/accounts/{id}/operations` takes no asset filter, so a path payment bot with
thousands of operations and no offers still costs its full page allowance. Two
things already narrow it: only accounts a trade names as an OFFER OWNER are walked,
which drops pure takers entirely, and operations are matched against the pair before
their result XDR is decoded.
