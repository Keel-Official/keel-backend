# Keel: API Contract Changes for v1.1.0

**Decision:** The API contract moves to 1.1.0 in order to carry methodology
v1.0.1, namely `SPREAD_EXTREME`, the `reachable` distinction, the extended
manipulation ladder, and oracle resistance.
**Status:** DRAFT. Not frozen. See section 7 for the freeze conditions.
**Source of the changes:** `docs/internal/memo-pra-development.md` sections 1 and 2.
**Affected file:** `docs/api/keel-openapi.yaml`

> **Translation note.** Translated to English under DEC-005 with its content
> unchanged. Section 4 still carries an error the contract itself has already
> corrected: it lists `MC(delta=1)` as `reachable: true`, and the correct value is
> `false`. That is fixed under task T6, not here. See findings P1-26 and P1-27 in
> `docs/internal/audit-2026-08-20.md`.

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
`docs/internal/memo-pra-development.md` section 1.2 writes the `SpreadExtremePct`
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

`docs/internal/memo-pra-development.md` section 3 requires the golden fixture table
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

1. Does the list page need `spreadPct` on `AssetSummary`, or is the
   `SPREAD_EXTREME` flag enough to mark the row?
2. `criticalDelta` is pinned to 0.5 for every asset. Does the dashboard need to
   choose its own critical delta through a query parameter?
3. There is no example response with `oracleResistance.ratio` below 1, which is the
   most dangerous state, because no sample asset has both non-zero genuine volume
   inside a 300 second window and a low manipulation cost. Do we need a purpose
   built synthetic example to design that display, or do we wait for real data?
4. What kind of distinguishing display does `dataSource: trades-implied` need? Its
   depth is a lower bound, not a measurement.

---

## 7. Freeze conditions

The new contract may be frozen once all four are done:

- [ ] The golden fixture table in section 3 is filled in by hand and saved as
      `testdata/fixtures/ustry_pre_exploit.md` with a reason for every number
- [ ] Every `TODO-FIXTURE` marker and `reachable: null` in `AssetBrokenBook` is
      replaced with fixture numbers, and `flags` and `band` are completed
- [ ] The scale of `spreadPct` is agreed, see section 2.4. If fractions are chosen,
      the methodology stays and every other `Pct` field changes with it
- [ ] The four questions in section 6 are answered by the frontend builder

After that, `MethodologyVersion` rises to 1.0.1 in the implementation, not only in
the example responses.
