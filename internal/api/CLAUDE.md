# GREEN ZONE: internal/api

Read-only HTTP handlers. Write freely, this is plumbing.

The contract already exists and changing it is not this package's business:
`docs/api/keel-openapi.yaml`. If a handler needs a different response shape, the
contract changes through a decision record, not the handler quietly.

## Rules

1. **The API never calls an adapter.** It only reads results already computed,
   out of `internal/store`. One popular asset triggering a Horizon call per
   request would burn the rate limit budget in minutes, and users would get
   unpredictable latency. The consequence is that metrics always lag slightly,
   and that is accepted explicitly in NFR-1.
2. **Every numeric value is sent as a string.** Stellar amounts are int64 stroops
   with 7 decimals, and a JSON number is an IEEE 754 double. The only exceptions
   are `delta` and integers such as `ledgerSeq`.
3. **Scale conversion happens here, in one obvious place.** Fields ending in
   `Pct` are on a PERCENT scale in the API. `spreadPct: '196.0777141'` means 196
   percent. If one fractional field hides among the percent fields, someone will
   eat it.
4. Every response carries `ledgerSeq` and `methodologyVersion`, plus the
   `X-Keel-Staleness-Seconds` and `X-Keel-Methodology-Version` headers.
5. **An asset with no price is not an error.** `priceSource: none` with band
   `CRITICAL` is an HTTP 200. A ledger that is not available yet is a 404 with
   code `LEDGER_NOT_AVAILABLE`, not a 500. A book with a spread of several hundred
   percent is also a 200. All three are findings, not failures.

## ~~Settle this before writing handlers~~ Settled, 24 August 2026

This section used to say the contract lagged `internal/domain/types.go` in four
places, and that writing handlers first meant writing them twice. All four are
closed: `costToMaxReachablePrice`, `unevaluatedFlags` and `bandConfidence` are in
contract 1.3.0, and `oracleResistance` is an object on both sides again after
handoff item 13. Findings P1-9, P1-10, P1-11 and P1-6 all read NOT.

It is kept rather than deleted because the reason it existed is the useful part:
the handlers were held back until the contract and the type agreed, and they were
written once.

## What exists here now

`api.go` holds the server, the five routes and the error mapping. `wire.go`
declares the response shapes, which ARE the contract rather than a derivation of
the domain types. `assetid.go` resolves the `assetId` parameter.

Three things to know before changing anything:

1. **Reads go through the `Reader` interface, not through `*store.Store`.** That
   is what lets `api_test.go` run with no database, so the handler tests run in CI
   on every push instead of only when somebody remembers to start Postgres. The
   SQL is proven by `internal/store`'s own integration tests; what is proven here
   is the HTTP.
2. **`assetId` carries no asset type, so the type is looked up and never
   inferred.** A five character code read as `credit_alphanum4` measures a
   different asset or nothing at all. The `assets` row is the authority.
3. **Decoding in the tests uses `json.Decoder.UseNumber`.** A JSON number
   otherwise lands in a `float64`, which the repository wide float ban forbids in
   test files too. It also makes the assertions stronger, because `json.Number`
   holds the digits that were actually sent.

## Two things the contract does not say, and one it says imprecisely

- ~~**The primary pair rule is not implemented.**~~ **IMPLEMENTED 11 September
  2026, and the struck-through text is kept because the reason it was written is
  the lesson.** It read: "With `quote` omitted the contract says to use the pair
  with the largest combined depth at 10 percent. That is decision D-1 and
  `docs/methodology/02-pair-selection.md` is still a worksheet, so a single pair
  resolves and several return 400 listing the candidates. Choosing by any other
  rule here would be this package quietly making a methodology decision."

  Every sentence of that was true when written and the refusal was right. Two
  things then changed and this brief did not notice either for six days. DEC-015
  closed D-1 on 5 September: section 2 of that document reads "The primary pair is
  USDC, always". And the contract's rule, largest combined depth at 10 percent,
  turned out to be one the methodology never held, so it was corrected in contract
  1.5.1 rather than implemented. An omitted `quote` now resolves to
  `domain.GlobalQuote()`, matched with `Asset.Equal` so another issuer's USDC does
  not qualify. The 400 survives for the one case that is still undecided: several
  pairs with no USDC pair among them.

  **The lesson for this package, and it is not about pairs.** The refusal was
  correct and it was also a lock that outlived its key. A comment saying "this is
  undecided" is a claim with a date on it, and nothing in the build re-checks it.
  When you write one, name the record or the document that would close it, the way
  the sentence above named D-1, because that name is what makes the staleness
  findable later.
- **The error enum has no code for two real states**: an ambiguous quote, and a
  monitored pair with no result yet. Both borrow a neighbouring code and are told
  apart by their message. Handoff item 18.
- **`X-Keel-Staleness-Seconds`** is implemented as the contract defines it,
  `computedAt` minus `ledgerClosedAt`, which is how far behind the ledger the data
  was when it was computed. That does not answer "how old is this now", which the
  name suggests and which the contract's own 900 second note reads as though it
  meant. Also handoff item 18.
