# DEC-007: Pool presence is reserve-based, and `reachable` stays a plain boolean

- **Status:** Approved by AL
- **Date:** 2026-08-28
- **Kind:** Ratification of existing behaviour, plus one reversal (section 5)
- **Related:** DEC-003 (contract freeze conditions), DEC-006, R-7 Steps 3 and 4
- **Zone:** `docs/decisions/` (YELLOW). Drafted by Claude. The normative
  definitions live in `docs/methodology/` (RED) and are authored by Al.

## 1. Why this record exists

Two reported fields depend on whether a liquidity pool is present for an
asset: `maxReachablePrice` (null when a pool is present) and
`ManipulationCost.reachable` on the combined ladder (unconditionally true
when a pool is present).

Both behaviours were already implemented and tested. Neither had a decision
record, and the word used for the condition was "active", which was ambiguous
between two readings:

1. **Activity-based** — the pool has traded within some recent window.
2. **Reserve-based** — the pool holds non-zero reserves.

The ambiguity surfaced on the USTRY/USDC pool
(`27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb`), which
was dormant for twelve days spanning the February 2026 incident while still
holding reserves of 15.4791416 USTRY and 16.3389179 USDC (DEC-006 section 1).
The two readings produce different output for that snapshot, and that
snapshot is the project's primary validation case.

This record does not change the code. It states which reading is normative,
records the evidence, and closes review finding #2.

## 2. Ratified: pool presence is reserve-based

A liquidity pool counts as present when **neither reserve is zero**. There is
no threshold and no reference to trading history.

This is what `internal/domain/types.go` already implements:

```go
func (p PoolReserves) IsEmpty() bool {
    return p.ReserveBase.IsZero() || p.ReserveQuote.IsZero()
}
```

`Snapshot.ActivePools()` filters on it, `sortedActivePools` consumes that,
and `TestEmptyPoolIsNotAPool` locks it. This record names that predicate as
normative and requires both dependent fields to reference it rather than
restate it.

## 3. Rationale

**1. An activity-based predicate is not computable from `Snapshot`.**
Dormancy is a property of trade history; `Snapshot` carries state. Supplying
it would require either a network call inside a pure function (violating
non-negotiable rule 3 and the `internal/domain` purity constraint enforced by
`arch_test.go`) or a recorder-side derivation (violating the recorder's
raw-bytes constraint, which is what makes hand-authored fixtures the sole
verification oracle). The reserve-based predicate reads fields already in the
snapshot.

**2. Manipulation cost is counterfactual, not descriptive.** Keel measures
what it would cost an attacker to move the price, not what traders did last
week. A dormant pool holding real reserves remains tradeable at any time.
DEC-006 establishes that arbitrage correction on this pool was unprofitable
after the 30 bps fee, which explains why the pool was quiet; it does not make
the pool absent as a manipulation venue.

**3. The February 2026 snapshot would be misread otherwise.** Under an
activity-based predicate the incident snapshot would report a non-null
`maxReachablePrice` for a ledger state in which the pool was demonstrably
available as a manipulation path.

**4. No new parameter.** An activity-based predicate needs a time window, and
any window must be justified against a fixture. That is additional R-7 work.

## 4. Evidence

**Protocol level.** The `LiquidityPoolDeposit` operation reference documents
a dedicated branch for the case where the pool is empty, confirming that a
pool ledger entry with zero reserves is a reachable protocol state. The
mechanism follows from pool creation: `ChangeTrustOp` on the pool share asset
creates the entry, and reserves become non-zero only on a subsequent
`LiquidityPoolDepositOp`.

**Network level.** `scripts/verify_empty_pools.sh` scanned the first 8,000
pools returned by Horizon `GET /liquidity_pools` on 2026-08-28.

| Measure | Count |
|---|---|
| Pools scanned | 8,000 |
| Both reserves zero | 82 |
| Exactly one reserve zero | 0 |

Two conclusions. First, Horizon surfaces empty pools rather than filtering
them, so `IsEmpty` discriminates in practice over any snapshot built from
Horizon; it is not a vacuous predicate. Second, no single-sided empty pool
was observed, consistent with the constant product invariant, which supports
the separate methodology claim that marginal price on a present pool is
unbounded above rather than undefined.

The scan terminated at its page limit, not at the end of the data set. 82 is
a count within the first 8,000 pools, not a network total.

## 5. Reversal: `reachable` stays `bool`

On 2026-08-28, earlier in the same session that produced this record, Al
provisionally chose to make `reachable` not-applicable (null) when a pool is
present, aligning it with `maxReachablePrice`. **That choice is reversed.**
It is recorded here rather than deleted, per the amendment rule.

The choice was made without either party having read `internal/domain`. On
reading it, three facts changed the picture:

1. The behaviour was already implemented as an unconditional `true`
   (`ComputeManipulationCost`), documented in `types.go` and `compute.go`,
   and locked by `TestAnActivePoolMakesEveryTargetReachable`.
2. `ComputeMaxSafeCollateral` reads `Reachable` from the
   **orderbookOnly** ladder, never the combined one. The order-book-only
   ladder is unaffected by pool presence, so the binding path was never at
   risk of inheriting an uninformative `true`.
3. A nullable representation would touch seven places (`types.go`,
   `compute.go`, `compute_test.go`, `internal/store/jsonb.go`,
   `internal/api/api_test.go`, `keel-openapi.yaml`, generated mocks) plus
   affected fixture rows.

The original objection stands and is legitimate: a reader of the contract
will assume a field that is always true is checking something. But that is a
documentation defect, and the remedy is the contract description in section
6, not a type change across seven files.

**Rejected alternatives.**

*Nullable `reachable` (tri-state).* Rejected on cost as above, and because
the failure mode it guards against does not exist: the binding consumer reads
the order-book-only ladder.

*Removing `reachable` from the combined ladder.* Rejected because it produces
two different shapes for the same schema, forcing every consumer to branch on
which ladder it is reading, to fix a description problem.

*Dust threshold (`reserve > N`) on the presence predicate.* Deferred, not
dismissed. A pool holding one stroop makes `maxReachablePrice` null while
contributing no meaningful liquidity. Any threshold requires justification
from a fixture, which is new R-7 work, and the residual cost is lost
information rather than incorrect figures. A future threshold amends this
record rather than replacing it.

*Shares-based predicate (`total_shares > 0`).* Rejected despite correlating
perfectly with zero reserves in the scan. The depth curve consumes reserves,
not shares; testing a proxy would fail silently if the two ever diverge.

## 6. Consequences

- No code change. `Reachable bool` stays as it is in `ManipulationPoint`,
  `manipulationJSON`, and the `ManipulationCost` schema.
- `keel-openapi.yaml` gains a description-only change on
  `ManipulationCost.reachable` and a correction on `maxReachablePrice`. See
  the companion patch.
- Review finding #2 is resolved: the contract's null condition for
  `maxReachablePrice` is corrected to match `maxReachable()` in `compute.go`.
  The two texts previously disagreed; see section 7.
- Naming: "active pool" is retired in favour of a term denoting presence of
  reserves. The term is chosen in `docs/methodology/` (RED). `ActivePools()`
  and `sortedActivePools` should be renamed to match once the term is fixed;
  that rename is mechanical and belongs in a follow-up.

## 7. Finding #2, stated precisely

`keel-openapi.yaml` currently says `maxReachablePrice` is null in two
situations: there is no ask at all on this pair, or **all the liquidity comes
from an AMM**.

`maxReachable()` in `compute.go` returns nil when
`len(s.ActivePools()) > 0 || len(s.Book.Asks) == 0`.

These are not the same condition. A pair with a populated ask side **and** a
pool with reserves returns nil from the code, while the contract's wording
promises a value, because not all of the liquidity comes from the AMM. Under
this record the code is correct and the contract text is wrong: presence of a
pool with reserves is sufficient, regardless of how much order book sits
beside it. The patch in section 6 corrects the contract.
