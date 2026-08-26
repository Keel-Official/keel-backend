# YELLOW ZONE: internal/horizon

Adapter for live data from the Horizon API.

The output MUST be a `domain.Snapshot`, identical to what `internal/hubble`
produces. If the two adapters produce different types the design has failed and
`computeDepth()` ends up changing with them.

## Endpoints

- `GET /order_book` with the selling_asset_* and buying_asset_* parameters
- `GET /liquidity_pools` with the `reserves=A,B` filter
- `GET /assets?asset_code=...` to verify asset identity, and to read the issued
  supply and Horizon's own holder count
- `GET /ledgers/{sequence}` for the ledger close time, which `/order_book` does
  not carry at all
- `GET /accounts?asset=CODE:ISSUER` for the trustline holders, paged. This is the
  only endpoint here that pages, and the only one whose request cost grows with
  the asset rather than being fixed. See `holders.go`

## Traps that keep happening

1. The old import path `github.com/stellar/go/...`. The correct one is
   `github.com/stellar/go-stellar-sdk/...`.
2. Reading the `price` string field. The correct one is `price_r`, shaped
   `{"n": 1, "d": 10}`. The string field has already lost precision.
3. Horizon does NOT serve historical data. If you need the past, that is
   internal/hubble's job. Never invent a historical endpoint. The trustline
   balance is the sharpest form of this: an order book can at least be
   reconstructed from the operations that posted it, which is what
   `offers-implied` means in the golden fixture, and a balance at a past ledger
   cannot be reconstructed from anything Horizon exposes. That is why
   `holders.go` exists as a recorder rather than as something `scan` calls.
4. Asset type must be passed explicitly and never inferred from code length.
   USTRY has a five character code and is `credit_alphanum12`; querying it as
   `credit_alphanum4` returns an empty result and no error. Two decision records
   in this repository contain exactly that mistake.

## After writing code here

Explain in three sentences: what design decision you took, one alternative you
rejected, and why.

## Two more traps, both found by running against live Horizon

5. **The two sides of `/order_book` are not denominated in the same asset.** An
   ask's `amount` is in the base asset. A bid's `amount` is in the QUOTE asset,
   because Horizon inverts the bid price into quote-per-base but leaves the
   amount as the underlying offer's selling amount, and a bid is an offer selling
   the quote asset. `domain.Level.Amount` is defined in base units, so a bid has
   to be converted. Reading it as base overstates sell-side depth by the price
   factor, and sell-side depth is the liquidation term of `C_max`. Measured, not
   assumed: `docs/evidences/order_book_amount_units_2026-08-24.txt`.
6. **`Latest-Ledger` is not on every response.** The collection endpoints send
   it, `/ledgers/{sequence}` does not, and it does not need to because it carries
   its own sequence in the body. The first version of this client required the
   header everywhere and failed on its first real request. `/order_book` carries
   no ledger sequence at all, so that header is the only honest stamp for a
   snapshot and a guess is not an acceptable substitute.

## A seventh trap, found while building the schema 2 recorder

7. **`/liquidity_pools` takes no asset type at all, and `/order_book` does.** The
   `reserves=` filter is ONE parameter holding two canonical asset strings
   separated by a comma, `native` for XLM and `CODE:ISSUER` otherwise, with no
   type anywhere in it. So the two endpoints fail DIFFERENTLY on the same bad
   configuration: `/order_book` with the wrong type returns an empty book and no
   error, which is trap 4, while `/liquidity_pools` never sees the type and
   answers correctly. A pair whose type is wrong therefore looks like a market
   with no orders and a working pool, which is a plausible reading of a real
   market rather than an obvious bug. The order of the two assets does not
   matter, the filter is an AND over both, and an empty result is HTTP 200 with
   `"records": []`. All measured, not assumed:
   `docs/evidences/liquidity_pools_reserves_2026-08-25.txt`.

## What exists here now

`client.go` is the read side: `GetSnapshot` assembles one `domain.Snapshot` from
`/order_book`, `/liquidity_pools`, and `/ledgers/{seq}`, and `VerifyAsset` closes
trap 4 once per asset rather than once per snapshot. `decode.go` holds the wire
shapes. `recorder.go` is the cross-validation recorder, one file per pair per
ledger, never overwritten. `tick.go` is recording schema 2, which is the same
recorder writing raw bytes and nothing else; it is the default since 25 August
2026 and `recorder.go` still writes schema 1 under `-schema 1`. Each file's own
header lists the design decisions and the alternative rejected, which is what
this zone asks for; the three that matter most across all of them are below.

## The three sentences this zone asks for

The decision: Horizon is read with `net/http` and hand-written response structs,
every snapshot keeps the raw bytes it was parsed from, and the recorder writes one
never-overwritten file per ledger. The alternative rejected: using
`horizonclient` from the Stellar SDK and recording only the parsed
`domain.Snapshot`, which is less code today. Why it was rejected: the SDK returns
its own structs that would need converting into `domain.Snapshot` anyway, so it
buys a dependency and saves no layer, and recording only the parsed form would
bake one reading of the order book into weeks of evidence, which is exactly the
reading that turned out to be wrong for bids until it was measured.

## The same three sentences for schema 2, added 25 August 2026

The decision: a tick stores the raw response bytes of `/order_book` and
`/liquidity_pools` as strings with their sha256, brackets them with the
`Latest-Ledger` of the first and last request, and parses, converts and selects
NOTHING, so a non-2xx and an empty pool list are both stored and kept. The
alternative rejected: extending `GetSnapshot` so one call returned the parsed
snapshot and the pool bytes together, which is far less code and reuses a path
that already works. Why it was rejected: the paragraph above is the reason in its
own words, because the parsed half of a schema 1 recording is precisely the half
that had to be revised once the bid amount unit was measured, and a recording
that claims nothing cannot be wrong in that way later.

The selection point is worth stating on its own, because it is the one that will
be under pressure. AQUA sits in 1308 pools and `reserves=AQUA,USDC` still returns
one record, since all 1308 are distinct asset pairs. If a pair ever returns more
than one, this package must still store all of them: CHOOSING AMONG CANDIDATE
POOLS IS A METHODOLOGY DECISION and methodology is the red zone. Summing their
reserves would be wrong anyway, because two pools at different prices are not one
deeper pool. Measured in `docs/evidences/aqua_identity_and_pools_2026-08-25.txt`.

## The same three sentences for the candidate universe, added 26 August 2026

`universe.go` reads `/assets` paged by ticker and `/accounts/{issuer}` for
`home_domain`. It gathers and never selects: there is no threshold in it, and the
inclusion criteria are `docs/methodology/02-pair-selection.md` section 5, which is
red and unwritten.

The decision: `AssetsByCode` asks for one `asset_code` and returns EVERY issuer
that answers, as separate rows sorted by issuer, walking `_links.next` until a page
comes back empty and reporting how many pages that took. The alternative rejected:
one request at `limit=200` returning the single best record per code, picked by
holder count, which is shorter and reuses the existing `findAssetRecord`. Why it
was rejected: both halves of that shortcut are the bug this repository has already
paid for, because 97 assets carry the code AQUA and a ranked pick promotes an
impostor with three pools and no `stellar.toml`, while a single page at `order=asc`
returns the OLDEST records and hides the real one on page two.

Three things about it that are decisions rather than gaps:

- **The asset type is read and never inferred.** `/assets` keyed on the code alone
  returns both widths and each record states its own `asset_type`, so trap 1 is
  closed by construction rather than by care. Querying per width would double the
  request count and put the guess back.
- **`Client.Throttled()` counts 429s including the ones a retry recovered from.**
  A rate limit absorbed silently is indistinguishable from one that never
  happened, and the two have opposite meanings for a short candidate list. It is
  incremented in both the parsed path and the raw tick path.
- **`HomeDomain` is deliberately half a proof and says so.** The other half is the
  domain's SEP-1 toml naming the same pair back, and that fetch lives in
  `cmd/keel/stellartoml.go` rather than here: a stellar.toml is served by a
  stranger's web server, not by Horizon, and giving this client a second transport
  with a second failure mode and a second timeout would blur what the package is.

## The same three sentences for the trade stream, added 26 August 2026

`trades.go` reads `/trades` for one pinned pair. It exists because trap 3 above is
about STATE and not about EVENTS: Horizon serves no order book at a past ledger and
serves the whole trade history of a pair, which is what DEC-002 section 1 records
and what makes its section 2 substitutes reachable at all. It gathers and never
selects: no genuine-trade rule, no dust threshold, no account filter, because those
are `docs/methodology/07-supporting-metrics.md` section 1 and that file is red and
unwritten.

The decision: the walk seeks with a LEDGER, follows Horizon's own `_links.next`
afterwards, and ends on a predicate the caller supplies over each decoded trade.
The alternative rejected: taking a start and an end TIME and converting both into
ledger sequences to build the cursors, which is the obvious shape for "give me
February". Why it was rejected: `00-overview.md` section 2 rule 4 forbids deriving
a time from a ledger sequence arithmetically, and the inverse is the same mistake
with the same error, roughly three weeks of drift over six months. A ledger is an
honest SEEK because arriving early only costs requests, while the window boundary
is decided on `ledger_close_time`, which every record states.

Three more things about it that are decisions rather than gaps:

- **A record whose base is not the asset that was asked for is REFUSED, not
  flipped.** Queried without a pinned pair, Horizon returns the 22 February exploit
  trade with USDC as the base and the fraction upside down, which is the case
  `orient()` in `price.go` was written for. Here the pair IS pinned, so a record
  that disagrees means the endpoint answered a different question, and silently
  inverting 106.74 into 0.0093 is a hundredfold error every downstream number would
  inherit. `ErrPairMismatch`, the same error `GetSnapshot` uses for the same reason.
- **The rational is the field called `price` on this endpoint and its members are
  STRINGS.** That is not trap 2. `/offers` spells the same value `price_r` with
  number members, and `flexInt64` accepting both is the whole reason `price.go`
  exists. The rounded string is never read.
- **`base_is_seller` is carried, and it is not decoration.** It is what separates a
  book walk from a `path_payment_strict_send` that buys the base asset on one venue
  and sells it on another inside ONE operation.
  `internal/domain/TradeImpliedDepthBounds` groups on it, and
  `docs/evidences/2026-08-26-ustry-february-trades-implied.md` section 2 records
  the wrong number that grouping by operation alone produced.

## An eighth trap, and it is the one that made `replay.go` possible at all

8. **Horizon never tells you the id of an offer that was just created.** A
   `manage_sell_offer` that creates comes back with `"offer_id": "0"`, and its
   effects collection is EMPTY. Measured on operation 263453036239003649, the one
   that posted the 106.7372828 ask of the incident. The id exists in exactly one
   place Horizon serves: the transaction's `result_xdr`. So a reconstruction that
   reads only the JSON can apply cancels and updates, which name their offer, and
   cannot apply a single create.

   There is a ninth thing in the same area, and it is not a trap so much as a
   convention nobody writes down. **A trade names an offer id on both sides even
   when one side never rested.** A taker that crosses the book completely has no
   ledger offer, so Horizon synthesises one by OR-ing the operation's TOID with
   bit 62:

   ```
   operation TOID       263454423513071617  0x03a7fa6700008001
   counter_offer_id    4875140441940459521  0x43a7fa6700008001
   ```

   9,478 of the 10,077 distinct offer ids in USTRY/USDC's February 2026 trades are
   synthetic. Treating them as resting offers makes every completeness check noise.

## The same three sentences for the offer decoder, added 26 August 2026

`offerxdr.go` reads one operation's `ManageOfferSuccessResult` out of a base64
transaction result. It exists because of trap 8.

The decision: a hand written XDR reader over the exact subset needed, skipping
every earlier operation result by computing its width from an ALLOW LIST of types
whose success body is void, and refusing anything it has not seen. The alternative
rejected: importing `github.com/stellar/go-stellar-sdk/xdr`, which decodes all of
it correctly. Why it was rejected: that module pulls a large dependency tree into a
`go.mod` with two direct requirements, for one struct on one path, and the surface
actually needed is under two hundred lines asserted against three transactions
anybody can refetch; if a second XDR need appears the trade flips and this file
should be deleted rather than extended.

Two things about it that are decisions rather than gaps:

- **The allow list runs the safe way round.** The first version treated "not a
  type with a body" as void, which is wrong in the dangerous direction: a protocol
  version adding an operation with a body would be skipped three int32s short, and
  the bytes after it decode into a VALID LOOKING offer rather than into an error.
  Unknown types now reach `ErrUnsizableResult` and the caller counts them.
- **An AccountID inside an Asset is 36 bytes and not 32.** Reading it as 32 puts
  every later field four bytes early, and it does not fail loudly: the second
  asset's code decodes as printable text with the type integer glued to its front,
  and the amount arrives as a plausible negative int64. It cost a debugging pass
  and the file's header carries the note.

## The same three sentences for the replay, added 26 August 2026

`replay.go` rebuilds a pair's order book at a past ledger from the operations that
posted it. Trap 3 still stands and this does not contradict it: Horizon serves no
past STATE and serves every past EVENT, and this replays the events. It is DEC-002
section 2.3, built once its own precondition was met, which
`docs/evidences/2026-08-26-ustry-february-trades-implied.md` measured.

The decision: accounts are discovered from the trade stream and the live offer
book, their operations are walked BACKWARDS from the target, and every
approximation is counted into the result rather than logged. The alternative
rejected: walking forward from a fixed start ledger over every account that ever
touched the asset, which is the shape DEC-002 section 2.3 describes. Why it was
rejected: forward from a fixed start silently loses every offer created before that
start and still resting, and the loss looks exactly like a thin book, which is this
product's most interesting finding and therefore the worst thing to produce by
accident; backwards puts the operations that decide the target state on the first
page and turns the same limitation into a depth the file can report.

- **Consumption comes from the trade stream and never from `offersClaimed`**, even
  though the claims are decoded and carry exact stroop amounts. A resting offer can
  be taken by a manage offer, by a path payment either way, or by one that also
  crosses a pool, and only the first is a result this decoder reads. All of them
  produce a trade on the pair. One mechanism for all consumption is what stops an
  offer being decremented twice or not at all.
- **Inside one operation its trades are applied FIRST and its own result LAST.**
  The result is the ledger's statement of what the submitting offer looked like when
  the operation finished, so writing it last is correct even though that
  operation's trades appear to touch it.
- **The bid side is not the ask side with a sign flipped.** An ask sells the base,
  so its price is already quote per base and its amount already in base units. A
  bid sells the QUOTE: its price reads base per quote and inverts, and its amount is
  in quote units and converts. That is trap 5 read from the other direction, and
  `TestTheTwoCreatesRebuildTheFixtureBookExactly` is the assertion that holds it.
- **POOLS ARE NOT RECONSTRUCTED.** The snapshot carries none, and that is not a
  claim that no pool existed. `/liquidity_pools/{id}/operations` can answer it and
  DEC-002 section 2.3 calls that side cleaner, because it has no discovery gap. Any
  depth computed from a replayed snapshot today is ORDER BOOK ONLY.
