# Keel: API Contract Changes, v1.1.0 then v1.2.0

**Decision:** The API contract moved to 1.1.0 to carry methodology v1.0.1, then to
**1.2.0** to carry v1.0.2-draft and to become handoff-ready for the frontend.
**Status:** NOT FROZEN, but no longer blocking a frontend. Three of the four freeze
conditions in section 7 are met; the fourth needs the frontend builder. The
`criticalDelta` question this document carried as open was SETTLED at 0.5, the value
this contract already used, so no contract change followed. See section 8.6 and its
dated note. See sections 7 and 8.
**Source of the changes:** the pre-development memo, sections 1 and 2, then the audit
findings P1-6 through P1-13 and P1-28 through P1-31. Neither source document is in
the repository; both lived in `docs/internal/`, which is gitignored under DEC-004 as
of 25 August 2026. The finding ids are reproducible from
`bash scripts/audit-verification.sh`; the memo is Al's local copy, and the citations
to it below are kept as provenance rather than as links.
**Affected files:** `docs/api/keel-openapi.yaml`, `docs/api/mocks/`,
`internal/domain/types.go`

> **Note on section 4.** It is left as written, including its claim that
> `MC(delta=1)` is `reachable: true`, which is wrong. Section 8 records the
> correction rather than editing the history of this document. Findings P1-26 and
> P1-27.

---

## 1. Why a minor version and not a patch

Three of these changes break compatibility for a consumer who has already written
code:

| Change | Why it breaks |
|---|---|
| `Asset.type` becomes required | A consumer validating the Asset object strictly will reject both old and new responses until their schema is updated |
| The `manipulationCost` ladder goes from 2 entries to 4 | Code reading `manipulationCost[1]` as delta 1.0 stays correct, but code assuming the array has length 2 becomes wrong |
| `cost` must now be read together with `reachable` | Old code displaying `cost` on its own now displays a misleading number in the `reachable: false` case. This is silent damage, not damage that raises an error |

The third is the most dangerous, because nothing fails. Old code keeps running and
keeps displaying a number. Only the meaning of the number is wrong. This change
therefore has to be communicated to the frontend builder explicitly; a changelog is
not enough.

There is no production consumer yet, so the cost of breaking is zero now and will
not be zero once it is frozen.

---

## 2. The changes and their reasons

### 2.1 `Asset.type` becomes required

Values: `native`, `credit_alphanum4`, `credit_alphanum12`.

The asset type is sent explicitly rather than inferred from the length of `code`. A
code of four characters or fewer may be issued as `credit_alphanum12`, and Horizon
reports it as issued. A consumer guessing from code length will construct an asset
identity different from the asset Keel actually measured.

**The rejected alternative:** letting consumers guess and simply documenting the
rule. Rejected because the rule is not always true, and the error only surfaces on
rare assets, which is precisely the class of asset Keel exists for.

### 2.2 The `manipulationCost` ladder becomes 0.5, 1, 10, 100

Each entry now carries `targetPrice` and `reachable` alongside `cost`.

The large rungs were added because an asset with a broken book is invisible to the
small rungs. On the USTRY fixture the only ask sits far above `P0 x 1.5`, so the 0.5
rung touches no liquidity at all while the rung at 1 absorbs the entire book.
Without the rungs at 10 and 100 it would not be visible that there is nothing above
that at all.

`targetPrice` is sent rather than left for the consumer to recompute, because
`midPrice` on a broken-book asset cannot be trusted as a multiplication base on the
client side. Sending it makes every rung of the ladder readable on its own.

### 2.3 `reachable` and `maxReachablePrice`

This is the core of the change. `cost: "0"` has two opposite meanings:

| cost | reachable | meaning |
|---|---|---|
| 0 | true | the target price can be reached at no cost |
| 0 | false | there is no liquidity at all in that range |

Without that distinction Keel's output is ambiguous on precisely the most dangerous
assets. `maxReachablePrice` completes it by stating the upper bound on price
movement through the book, so a consumer can check for themselves that every
`targetPrice` above that value really is `reachable: false`.

**The rejected alternative:** sending `cost: null` for the unreachable case.
Rejected because it discards information. On the delta 10 rung of the historical
example, the cost of 2210.4400000 is still meaningful: it is the cost of exhausting
every ask. What is lost is not the cost, it is the reaching of the target.

`maxReachablePrice` is null in two different situations: there is no ask at all, or
all the liquidity is in an AMM. A constant product curve has no upper price bound,
so there is no maximum to report. The two are deliberately not distinguished by
value, because `priceSource` already distinguishes them.

### 2.4 `spreadPct` and the `SPREAD_EXTREME` flag

`spreadPct = (best_ask - best_bid) / midPrice`, reported as a number rather than
only as a triggered status.

**A difference from the methodology, needs Al's approval.**
The pre-development memo, section 1.2, writes the `SpreadExtremePct`
default as 0.20, a fraction. The API contract uses a percent scale:
`spreadExtremePct: '20.0'`, and `spreadPct: '196.0777141'` for a spread of 196
percent.

The reason is internal consistency. Every existing field in this API whose name ends
in `Pct` is on a percent scale: `holderTop1Pct: '11.4200000'`, `tradesExcludedPct:
'2.1000000'`, `manipulationRatioLowPct: '1.0'`. One fractional field among percent
fields is a trap somebody will certainly walk into.

The consequence is that the internal variable name and the API value are on
different scales, and the conversion has to happen in one obvious place in the API
layer. If Al prefers fractions, then every other `Pct` field has to change too, not
only this one.

`spreadPct` is null when either side of the book is empty or when `priceSource` is
not `book`. Spread is undefined without two sides of a book.

### 2.5 `oracleResistance`

The methodology writes it as `MC(critical) + genuine volume in the oracle window`.
It is expressed here as an object with five required fields: `criticalDelta`,
`manipulationCost`, `reachable`, `genuineVolume`, `windowSeconds`, plus a `ratio`
that may be null.

**The rejected alternative:** a single scalar quotient. Rejected because a ratio
hides two states that have to be visible. First, a `genuineVolume` of zero makes the
ratio undefined, and an asset with no trading at all inside the oracle window is an
important finding, not missing data. Second, a ratio computed from a
`manipulationCost` whose `reachable` is false is a meaningless number. In object
form both states are readable and `ratio` is simply set to null.

`windowSeconds` is repeated inside the object even though it is also at
`/methodology`, so that an asset response can be read and archived without calling
another endpoint.

`criticalDelta` defaults to 0.5 and is always equal to one of the `delta` values in
`manipulationCost`.

### 2.6 `dataSource` accepts `trades-implied`

Used when the orderbook state at the requested ledger is unavailable and the price
and liquidity were reconstructed from trades that actually executed.

This value applies to `AssetRisk` and to `HistoryResponse` alike, through one shared
`DataSource` schema. They are unified because it is the historical path that most
often lacks a snapshot, and that is exactly the path the Blend case study uses.

A trade proves the liquidity that was used, not the liquidity that was available.
Depth from `trades-implied` is a lower bound and must carry a warning. The frontend
must not display it as equivalent to a `horizon` result.

The `assetBrokenBook` example uses this value, because the USTRY/USDC book at ledger
61340263 really was derived from on-chain operations rather than from an orderbook
snapshot. So this new value has one real example, not merely an enum entry.

### 2.7 `/methodology` gains two thresholds

`spreadExtremePct: '20.0'` and `oracleWindowSeconds: 300`.

`thresholds` remains `additionalProperties: true`. Consumers are required to read by
key name rather than by position, so adding the next threshold breaks nobody.

`oracleWindowSeconds` is Keel's assumption, not a reading from any oracle. Every
oracle has its own window. The value is reported so that a consumer can substitute
the window their oracle actually uses.

---

## 3. The `assetBrokenBook` example

Added as `components.examples.AssetBrokenBook`, wired to
`GET /asset/{assetId}/depth` under the key `bukuRusak`.

This is the third state the frontend has to distinguish, alongside a healthy asset
and an asset with no price. USTRY/USDC moments before ledger 61340263 held exactly
one ask at 106.7372828 and exactly one bid at 1.0570000. The midpoint is 53.8971414
for an asset worth about 1.06. A spread of 196 percent. HTTP 200, not an error, and
not a normal condition.

What the display has to do, already written into the example's `description`:

1. Do not show `midPrice` as a price without a marker.
2. Damp down the 2/5/10 percent depth ladder. All three are derived from a
   `midPrice` that no longer means anything, and they are reported only because the
   SOW promised them.
3. Promote the manipulation rungs at delta 10 and 100 along with
   `maxReachablePrice`.
4. Distinguish `cost: "0"` with `reachable: false` from `cost: "0"` with
   `reachable: true`.

**This example is incomplete and that is deliberate.** See section 4.

---

## 4. The numbers deliberately left blank

The pre-development memo, section 3, requires the golden fixture table
to be filled in by hand before a single line of implementation is written, and it
states the reason: a table filled in after the code exists merely confirms whatever
that code did.

The same reason applies to the API examples. Deriving `depth`, `cost`, `reachable`,
and `maxReachablePrice` here would be handing over the answers to the worksheet, and
that safeguard on the methodology would be gone.

What is filled in, because it is already written in the methodology document:

| Field | Value | Origin |
|---|---|---|
| `midPrice` | `53.8971414` | section 1.2 |
| `spreadPct` | `196.0777141` | (106.7372828 - 1.057) / 53.8971414 x 100, the 196 percent figure is named in section 2 |
| `targetPrice` for the whole ladder | 80.8457121, 107.7942828, 592.8685554, 5443.6112814 | `midPrice x (1 + delta)`, the formula and one worked example are given in section 3 |
| `MC(delta=1)` cost and reachable | `130.0627093`, `true` | the example already worked through in section 3 |
| `flags` | `[SPREAD_EXTREME]` | certain to trigger at a spread of 196 percent |
| `band` | `HIGH` | the consequence of SPREAD_EXTREME per section 1.2 |

Awaiting session 1, marked `TODO-FIXTURE` or `reachable: null`: all of `depth`,
`cost` at delta 0.5, 10, and 100, `reachable` on those three, `maxReachablePrice`,
the contents of `oracleResistance`, `maxSafeCollateral`, and the holder metrics.

The `flags` and `band` in that example are a lower bound. Other flags will most
likely also fire once the fixture is filled in, and the band may rise to CRITICAL.

The `TODO-FIXTURE` marker violates the `Decimal` schema pattern, and
`reachable: null` violates the boolean type. That is deliberate: an OpenAPI
validator will reject the file until those markers are replaced, so the contract
cannot be frozen by accident.

---

## 5. What is not changing, and why

| Not changed | Reason |
|---|---|
| `AssetSummary` does not gain `spreadPct` | Not requested in section 2. The list page already receives `SPREAD_EXTREME` through `flags`, which is enough to mark the row. See the open question in section 6 |
| `HistoryPoint.manipulationCost50Pct` stays a single rung | A four rung time series inflates the response payload with no concrete request from the dashboard |
| The depth ladder stays 2/5/10 percent | Promised in the SOW. Methodology section 1.4 already states this ladder is not an oracle safety metric, and that is now written into the API description |
| `Band` remains the worst triggered flag, not a weighting | Out of scope for this change |

---

## 6. Open questions for the frontend builder

**How an answer is recorded.** Each question below carries an `Answered:` line. An
answer is only complete when that line names the date and what changed, even when
the answer is "no change needed". An answer that leaves no trace is
indistinguishable from a question that was never asked, and
`scripts/audit-verification.sh` checks these lines mechanically rather than trusting
anyone's memory.

1. Does the list page need `spreadPct` on `AssetSummary`, or is the
   `SPREAD_EXTREME` flag enough to mark the row?
   **Answered:** not yet.
2. `criticalDelta` is pinned to one value for every asset. Does the dashboard need
   to choose its own critical delta through a query parameter?
   **Answered:** not yet. Note this question is now entangled with the open
   methodology question in section 8: the value itself is disputed, not only
   whether it is selectable.
3. There is no example response with `oracleResistance.ratio` below 1, which is the
   most dangerous state, because no sample asset has both non-zero genuine volume
   inside a 300 second window and a low manipulation cost. Do we need a purpose
   built synthetic example to design that display, or do we wait for real data?
   **Answered:** not yet.
4. What kind of distinguishing display does `dataSource: trades-implied` need? Its
   depth is a lower bound, not a measurement.
   **Answered:** not yet.
5. `asset-list-mixed.json` holds three rows and all three are
   `bandConfidence: full`. The `partial` case is real on that endpoint. Do you want
   a dedicated list example with a partial row, or is the detail example enough to
   design from?
   **Answered:** not yet.

---

## 7. Freeze conditions

- [x] The golden fixture table in section 3 is filled in by hand and saved as
      `testdata/fixtures/ustry_pre_exploit.md` with a reason for every number.
      Done, and independently recomputed in 60 digit decimal arithmetic.
- [x] Every `TODO-FIXTURE` marker and `reachable: null` in `AssetBrokenBook` is
      replaced with fixture numbers, and `flags` and `band` are completed. Done.
- [x] The scale of `spreadPct` is agreed: PERCENT, matching every other field
      whose name ends in `Pct`. Recorded in `docs/methodology/README.md` section 2.
- [ ] The questions in section 6 are answered by the frontend builder. **This is
      the only remaining condition, and it is not something this repository can
      close on its own.**

Each of these is checked mechanically by `scripts/audit-verification.sh`, in the
section headed "DEC-003 freeze conditions". The checklist above is a summary of
those checks and not a substitute for them: a tick typed by hand is a claim, and a
passing check is evidence.

`MethodologyVersion` in the implementation is already `1.0.2-draft`, and every
example response now says the same. Before this pass the contract advertised
methodology 1.0.1 in most examples and 1.0.2-draft in one, while the code produced
1.0.2-draft, so it claimed a version nothing produced.

---

## 8. What v1.2.0 changed, 20 August 2026

Everything in this section closes a finding from the repository audit.

### 8.1 Three fields the code had and the contract did not

`costToMaxReachablePrice`, `unevaluatedFlags`, and `bandConfidence` are now in
`AssetRisk`, all three required, and present in all five example responses.
Findings P1-8 through P1-12.

`bandConfidence` also goes on `AssetSummary`, which section 5 of this document had
previously decided against for `spreadPct`. The reasoning differs: the list page is
where a band is read fastest and with the least context, and
`09-flags-and-bands.md` section 2 states the dashboard is *required* to display
confidence. A row reading `LOW` with no confidence marker is the exact display the
methodology forbids. `spreadPct` stays off the summary, because the
`SPREAD_EXTREME` flag already marks that row.

`costToMaxReachablePrice` carries one note the fixture forced: because the sum runs
over asks strictly below the maximum, a book whose only ask *is* the maximum yields
zero by construction, for any asset. A zero there is a statement about the shape of
the book, not a discovery about the asset. That is now written into the field
description so nobody reads the USTRY zero as unique to USTRY.

### 8.2 `oracleResistance` resolved in favor of the contract

`internal/domain/types.go` held it as a single `*decimal.Decimal` while this
document defined an object. The code now carries a `domain.OracleResistance`
struct matching the contract. Findings P1-6 and P1-7.

The contract won because it had the written reasoning and the scalar did not, and
that reasoning is still right: a ratio hides a `genuineVolume` of zero, which is a
finding rather than missing data, and it hides a `reachable: false`, which makes any
ratio meaningless.

### 8.3 Corrections to the examples

- `documentUrl` pointed at `github.com/ciganytry/keel` and at
  `docs/methodology/00-overview.md`. The org is `Keel-Official/keel-backend` and
  that file does not exist, so the link in the `/methodology` response was broken
  twice over. Findings P1-28 and P1-29.
- `assetBrokenBook.ledgerClosedAt` read `2026-02-21T23:39:00Z`. Ledger 61340263
  closed at `2026-02-22T00:10:21Z`, pinned from two directions: the golden fixture
  uses that time, and ledger 61340272 closed at 00:11:16, nine ledgers later.
  Findings P1-30 and P1-31.
- Two synthetic issuer addresses were **not valid Stellar addresses**. Both were 60
  characters instead of 56 and contained 0, 1, 8, and 9, which are not in the
  base32 alphabet, so they violated the `G[A-Z2-7]{55}` pattern this same file
  declares on `assetId`. A frontend copying one into a test would have built a path
  the API's own pattern rejects. They are replaced with addresses generated as
  proper StrKey ed25519 public keys, version byte 48 and a valid CRC16-XModem
  checksum, derived deterministically from a label so they are reproducible and
  belong to no real account.
- The methodology version in the examples was 1.0.1 in most places and 1.0.2-draft
  in one, while the implementation produces 1.0.2-draft. The contract advertised a
  version nothing produced. All examples now read 1.0.2-draft.

### 8.4 Findings from validating the file with a real tool

`npx @redocly/cli lint` had never been run against this contract. It reported five
errors and four warnings, none of them structural. Fixed:

- **`security` was absent entirely.** Keel has no authentication by design, but
  omitting the field says nothing; a client generator cannot tell "no auth" apart
  from "forgotten". The root now declares `security: []`, which is the explicit
  form.
- **`license` had a name and no identifier.** Now `identifier: MIT`.
- **`429` was declared on 2 of 5 endpoints**, although NFR-5 applies a limit of 60
  requests per minute per IP globally. A frontend polling `/health` would have met
  an undocumented 429, and `/asset/{assetId}/history` is the heaviest endpoint of
  the five. All five now declare it.

The one remaining warning is that the server URLs point at `example.com` and
`localhost`. That is accurate: there is no production host yet.

### 8.5 Generated mocks, and why they are generated

`docs/api/mocks/` now holds every example response as standalone JSON, plus a
README for the frontend. They are produced by `make api-mocks` and their freshness
is provable with `make api-mocks-check`.

Hand copied mocks were rejected as an option. A hand copy is a second home for the
same data, and every second home in this repository has drifted: section 5 of the
PRD still carries flag definitions this methodology superseded, and
`keel-bootstrap.sh` still carries a `CLAUDE.md` from before the repository was in
English. A generator plus a drift check is the only version that stays true.

### 8.6 The one methodology question still open, and it is Al's

`criticalDelta` has two values in this repository and they disagree.

| Source | Value |
|---|---|
| This contract, and every example | 0.5 |
| `08-collateral.md`, section 9 of the pre-split core file: `P_critical = 2 x P0` | 1.0 |
| `DefaultParams()` in `internal/conformance` | 1.0 |

It is not cosmetic. On the golden fixture, delta 0.5 gives `Cost 0` with
`Reachable true`, and delta 1 gives `Cost 130.06` with `Reachable false`. Since
`C_max` takes `MC(P_critical) x m` as one of its two terms, the choice changes
`C_max` itself.

**The argument for 0.5**, which is also what the contract already says: at delta 1
on this fixture the target is unreachable, so `MC` there is not a cost-to-reach at
all and using it as a term in `C_max` multiplies a meaningless number. At delta 0.5
the target is reachable at zero cost, `C_max` comes out zero, and zero is the
correct conservative answer for an asset in that state.

**Why this is not settled here.** Changing it means changing
`08-collateral.md` and raising `MethodologyVersion`. The
methodology is the paid deliverable and `internal/domain/compute.go` is the red
zone, so the value is Al's to set, not Claude's. The contract is deliberately left at 0.5 and
the disagreement is recorded rather than papered over.

Until it is settled, `docs/api/mocks/README.md` tells the frontend to read
`criticalDelta` from the response and hardcode neither value.

#### Settled 25 August 2026: 0.5, and the contract does not move

Everything above this line is left as written, because it records a disagreement that
was real when it was written. What follows is what happened to it.

Al confirmed 0.5 on 25 August, and no contract change follows, because 0.5 is what
this contract and every example already carried. The disagreement had in fact ended on
23 August: methodology 1.0.3 moved `08-collateral.md` and `DefaultParams()` to 0.5 and
said so in the document, so two of the three rows in the table above were already out
of date when they were read again. The table describes 21 August.

**The argument reproduced above does not survive, though the conclusion does.** It
says that at delta 1 the manipulation term multiplies a meaningless number. The
`Reachable` guard prevents that by construction: when the target is unreachable the
term is not evaluated at all, and `C_max` falls back to the liquidation limit with a
warning. So the cost of delta 1 is a constraint that goes missing, not a wrong number.
`08-collateral.md` was corrected on 25 August and `internal/domain/types.go`, which
carried the same claim word for word, was corrected with it.

**What this does NOT do to the freeze.** Condition 4 in section 7 is unchanged and
still open: it needs the frontend builder to answer section 6, and it never depended
on this question. The line above about `docs/api/mocks/README.md` is superseded; that
file now tells the frontend the value is settled at 0.5 and still says to read it from
the response, because a caller-configurable parameter is reported for a reason.

---

## 9. What v1.3.0 changed, 23 August 2026

Methodology 1.0.3 changed what is computed, so the contract had to follow. One
minor version, one pass, mocks regenerated.

### 9.1 Additive, so nothing breaks

| Added | Why |
|---|---|
| `poolSpotPrice`, `priceDivergencePct` | the `P0` rule now compares two sources; a consumer needs both numbers, not only the one that won |
| `PRICE_SOURCE_CONFLICT` in the flag enum | the disagreement is reported rather than hidden |
| `maxSafeCollateralLiquidation`, `maxSafeCollateralManipulation` | methodology section 9 requires both terms, not only their minimum. Finding P1-15 |
| `offers-implied` in the data source enum | handoff item 5b. An offer proves POSTED liquidity, a trade proves only CONSUMED liquidity |
| `oracleResistance.totalAttackCost` | the 1.0.3 sum, kept alongside the ratio because they answer different questions |
| `priceDivergencePct` in `/methodology` thresholds | every threshold is readable from the API |

### 9.2 One breaking change, and why it is not kept as an alias

`manipulationCost` is renamed to `manipulationCostCombined`, and
`manipulationCostOrderbookOnly` is added beside it.

A deprecated alias was considered and rejected. This repository's recurring defect
is two homes for one thing, and an alias is exactly that: two names for one field,
one of which is a lie about what it contains once the split exists. The API is not
implemented yet and the only consumer is reading mocks, so the rename costs one
message today and nothing later.

**That message is now part of freeze condition 4.** Whoever is told the five
questions in section 6 has to be told this rename in the same message, because a
frontend built against the mocks before 23 August has the old field name in it.

### 9.3 Section 8.2 reversed, then reversed back

Section 8.2 resolved `oracleResistance` in favour of the contract's object form
over the code's scalar. Methodology 1.0.3 reintroduced the scalar, which reversed
that decision without recording it, and the audit check that should have caught it
matched the exact whitespace of the old struct and went quietly green.

The object is restored, and the 1.0.3 sum lives inside it as `totalAttackCost`, so
both quantities survive and the contract does not move. The full reasoning is
handoff item 13. The short version is that a ratio has two undefined states, a
`genuineVolume` of zero and an unreachable target, and a scalar has nowhere to put
either one.

---

## 10. What v1.4.0 changed, 26 August 2026

One additive query parameter on `GET /asset/{assetId}/history`, and it exists to
repair a defect rather than to add a feature. Mocks regenerated, and their content
did not move, because the change is to the parameter list and not to any example.

### 10.1 The defect

A result is keyed by `(asset_id, ledger_seq, methodology_version, data_source)`.
The history read constrained the asset and the methodology version and ranged over
the ledger, and left `data_source` unconstrained. One ledger could therefore return
up to four rows.

The handler downsamples by keeping the last row in each time bucket. Of the four
source values, `trades-implied` sorts last alphabetically, and the query ordered by
`data_source ASC`. So for any ledger holding both a live `horizon` reading and a
`trades-implied` reconstruction, the series charted the reconstruction.

That is not a cosmetic mix. `trades-implied` is rebuilt from trades that executed,
so it proves only liquidity that was CONSUMED and is a **lower bound**, while
`horizon` is a direct reading. The endpoint was presenting the weakest number in
the range as though it were the same kind of number as the strongest. The response
even carried `dataSource` read off the last row, so the label moved with the data
instead of describing the series.

This is the same posted-against-executed distinction that
`docs/methodology/06-oracle-resilience.md` section 2 builds its argument on, and
`internal/store/CLAUDE.md` rule 4 already said the two "are not interchangeable".
Both were correct; the query did not implement them.

### 10.2 The change

| Added | Why |
|---|---|
| `source` query parameter, enum of the four data sources, default `horizon` | one series is one source. The default is the only value of the four that is a direct live reading |
| A value outside the enum is `400 INVALID_RANGE` | "no data" and "no such source" are different answers, and returning an empty series for a misspelled source hides the typo |

`dataSource` in the response body already existed and is unchanged in shape. It now
reports the source that was **asked for**, which is the source of every row,
instead of whichever row happened to sort last.

### 10.3 Why not "return every source and let the client split them"

Considered and rejected. It moves the same decision to every consumer and makes the
default behavior the wrong one, since a client that does not know to split gets the
old bug back. The dashboard would have to learn the confidence ordering of the four
values to draw one line, and that ordering is methodology, not presentation.

The narrower alternative, requiring `source` with no default, was also rejected: it
is the most internally consistent option and it breaks every existing caller of an
endpoint whose contract is already in a frontend's hands.

### 10.4 What this does not close

`internal/store/MetricsHistory` still has no way to ask for several sources at
once, deliberately. If a cross-validation view ever needs to compare two sources on
one chart, that is a new endpoint or a new parameter that names a comparison, not a
relaxation of this filter.
