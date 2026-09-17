# DEC-022: A reconstruction may be stored when it names its own gaps, or the historical path stays closed for ever

**Status:** **ACCEPTED use Option B.** Decided by Al on 2026-09-17. In force for the mechanism only; the scope limits in section 6 and the follow-ups in section 10 hold.
**Date drafted:** 2026-09-17
**Kind:** Mechanism. It changes what `keel replay -persist` is allowed to accept and
what a stored `offers-implied` row must say about itself. It changes no formula, no
threshold and no flag. Whether it changes the contract schema is the choice in
section 5.
**Drafted by:** Claude
**Decided by:** Al
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. Sections 2 and 3 are code readings and measurements
and are evidence, not decision. Section 5 is a proposal and section 6 is a refusal to
decide something that is not this record's to decide.
**Rests on:**
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-control-ledger-61340262.meta.txt`, the run
of 5 September 2026 over exactly the two ledgers this would first be used for, and
`docs/evidences/USTRY.GCRYUGD5-USDC.GA5ZSEJY-bookseries-2026-02-01_2026-03-01-cap400-repaired.meta.txt`,
the repaired February series.
**Relates to:** DEC-002, which deferred the second data source and left this path as
the only route to a past book; DEC-021, which established that the fold may be told
about a proven removal, and whose shape this record copies deliberately; DEC-013,
which supplies the only audited pool coverage this path has; DEC-010, which requires a
provenance sidecar beside every backtest artefact.

---

## 1. The question this record puts to Al

`GET /v1/asset/{id}/depth?ledger=` has answered 503 on every request since the API was
deployed. The cause is not the one the error message gave until today, and it is not
BigQuery. It is that the production database holds no reconstructed row, and the one
writer that could put one there refuses every reconstruction this repository has ever
produced.

**The question is not "should the gate be loosened".** It is narrower and it has a
right answer either way:

> When a reconstruction can detect a hole in itself, is the honest response to refuse
> to store it, or to store it with the hole named in the row?

Section 3 shows that under the present rule the first answer is permanent. There is no
configuration of the walk that passes, so "refuse until it is complete" is in practice
"refuse for ever", and that sentence has never been written down where the decision
could be seen.

---

## 2. What the gate is, read from the code

`cmd/keel/replay.go:251` is the whole of it:

```go
_, _, crossed := res.Snapshot.Book.Crossed()
if !res.Complete() || res.StoppedAtFloor != 0 || crossed {
    return 0, false, fmt.Errorf("refusing to persist incomplete reconstruction: ...")
}
```

and `internal/horizon/replay.go:309`:

```go
func (r ReplayResult) Complete() bool {
    return len(r.MissingOfferIDs) == 0 && r.Truncated == 0 && r.Unsizable == 0 &&
        r.Failed == 0 && !r.MayBeInflated() && !r.Crossed
}
```

So seven conditions, every one of which must be zero or false:

| Condition | What a nonzero value means | Direction of its error |
| --- | --- | --- |
| `MissingOfferIDs` | a trade named an offer the walk never saw | book too THIN |
| `Truncated` | an account walk hit its page cap | book too THIN |
| `Failed` | Horizon refused or timed out on an account walk | book too THIN |
| `Unsizable` | an operation result could not be sized | book too THIN |
| `StoppedAtFloor` | a walk ended at `-since-ledger` rather than at the account's first operation | book too THIN |
| `MayBeInflated` | offers were applied from before the trade window, so something already eaten is still on the book | book too DEEP |
| `Crossed` | a bid sits at or above an ask, which the matching engine would have executed | PROOF an offer is missing |

**Six of the seven fail in the pessimistic direction and one does not.** That
asymmetry is already the repository's own reading: `reportReplay` gives
`MayBeInflated` a line of its own and says why, "the direction a warning product must
never fail in". The gate does not use that distinction. It treats all seven the same.

---

## 3. Why no bounded walk can pass, which is the part that was never measured

This is not a claim that the caps are too low. It is that the two remaining
configurations exclude each other.

**With a floor.** `-since-ledger N` bounds each account's backwards walk. Any walk that
reaches the floor before reaching the account's own first operation increments
`StoppedAtFloor`, and any nonzero value is refused. The run of 5 September 2026 over
exactly the two ledgers this path would first serve, in
`USTRY.GCRYUGD5-USDC.GA5ZSEJY-control-ledger-61340262.meta.txt`:

```
operation_floor_ledger: 61300000
accounts_walked: 65
walks_truncated: 7
walks_stopped_at_floor: 42
walks_failed: 10
walk_complete: false
requests: 374
elapsed_seconds: 2841
```

Forty-two of sixty-five. The repaired February series, a much larger walk with
`max_pages_per_offering_account: 400`, reports `walks_stopped_at_floor: 167` and
`walk_complete: false` as well.

**Without a floor.** `internal/horizon/replay.go:365` refuses the configuration
outright:

```go
if q.SinceLedger == 0 && q.TradesFromLedger != 0 { ... }
```

Removing the floor therefore forces `TradesFromLedger` to 0 as well, which means the
trade walk used for account discovery is unbounded too. DEC-002 section 7 measured what
that costs on this pair: at least twelve thousand trades inside four days of 2025, out
of a history spanning at least fourteen months, and section 7.2 of that record
recommends the exact opposite of this, bounding the pull by ledger range. The bounded
run above still took 2841 seconds and 374 requests against a 3000 requests per hour
budget.

**And `Failed` is not under our control at all.** Ten of sixty-five account walks failed
on the bounded run. One Horizon timeout anywhere in an unbounded walk fails the gate
after the hours it took to get there.

**Conclusion, and it is a reading rather than an opinion.** There is no setting of the
flags under which a real historical ledger on this pair can be persisted. The rule as
written is not a high bar. It is a closed door with a handle painted on it.

---

## 4. What is NOT wrong with the current rule, stated first

The rule is right about the thing it was written for, and a decision that forgets this
will produce a worse one.

1. **A thinner book is this product's most interesting finding.** An understated book
   does not look like an error, it looks like a discovery, and `keel replay`'s own usage
   text says so. Storing a gap-ridden book unlabelled would manufacture exactly the
   finding Keel sells.
2. **The API could not say so.** `persistReplay`'s comment is precise: "Unknown pool
   coverage is refused: the API cannot label a stored result as order-book-only". A
   store that can hold a qualified number, served by an API that cannot express the
   qualification, is worse than a refusal.
3. **A crossed book is proof, not risk.** Every other counter says an offer MIGHT be
   missing. `Crossed` says one IS.

Any proposal has to keep all three. Section 5 keeps all three.

---

## 5. The proposal

### 5.1 The rule

> A reconstruction whose detectable gaps all run in the pessimistic direction MAY be
> persisted, if and only if every nonzero counter is written into the stored row as a
> warning, and the run was asked for it explicitly on the command line.

Three parts, and each one is doing work:

**Opt in on the command line.** A new flag, `-accept-incomplete`, on `keel replay`
beside `-persist`. Without it the behaviour is byte for byte what it is today. This is
DEC-021 section 1 item 2 applied again and for the same reason: a reconstruction that
relaxes its own standard without being asked stops being evidence.

**The asymmetry is the rule, not a tolerance.** `MayBeInflated` and `Crossed` stay
refused with no override, because one fails optimistically and the other is proof of a
missing offer. `MissingOfferIDs`, `Truncated`, `Failed`, `Unsizable` and
`StoppedAtFloor` become admissible. Pool coverage stays required exactly as it is:
`-pool-snapshots` is untouched by this record.

**The row must carry its own gaps.** One warning per nonzero counter, naming the
counter and its value, plus one naming the floor ledger when there was one. A row whose
warnings do not account for its counters is a bug, and section 7 tests for it.

### 5.2 Where the qualification is carried, and this is the choice

| | Option A: warnings only | Option B: a machine-readable field |
| --- | --- | --- |
| Storage | none. `metrics.warnings` is already `TEXT[] NOT NULL` in `0001_core.sql` | a new JSONB column, a migration in `migrations/` (GREEN) |
| Domain | none. `AssetRisk.Warnings` already exists | a new field on `AssetRisk` (`internal/domain`, YELLOW) |
| API | none. `wire.go:170` already serves `warnings` | a new schema object, a contract minor bump, `make api-mocks` regenerated (`docs/api`, YELLOW) |
| A consumer can | read the gaps | branch on the gaps |
| Risk | prose a dashboard may not render | a schema change made before anyone has asked to branch on it |

**Recommended: Option A.** Not because it is cheaper, but because the contract already
promised it. `docs/api/keel-openapi.yaml:655` describes `offers-implied` to consumers
as a source that "is a reconstruction and it carries `warnings`". The mechanism this
record needs is the one a consumer was already told to expect, and every asset in
production already returns a non-empty `warnings` array, so the dashboard has to render
it regardless of this decision. Option B remains available later without rework: a row
stored under A keeps its counters in text, and a field added afterwards would be
populated by new runs, which is what the `data_source` key in the UNIQUE constraint
exists to keep separable.

### 5.3 One consequence worth naming before it is chosen

Under Option A the warning text is appended in `cmd/keel` after
`domain.ComputeAssetRisk` returns, so a stored row will not be byte-identical to a
recompute of the same snapshot. That touches NFR-9 reproducibility in a narrow way: the
numbers are identical and reproducible, the provenance prose is added at the boundary.
The alternative is to teach `internal/domain` about walk diagnostics, which puts
acquisition facts inside a pure function that is not allowed to know about acquisition.
Between a provenance string added at the edge and a pure function that learns what
Horizon did, the edge is the lesser harm, and Al may disagree.

**Sub-decision on who populates the field, 2026-09-17.** Resolved as B3, not the binary in section 5.3. `domain.ComputeAssetRisk` stays snapshot-pure and never learns an acquisition fact. The new machine-readable field is populated by a distinct, deterministic domain step — a constructor over (`AssetRisk`, `WalkDiagnostics`) — added in `internal/domain` (YELLOW). NFR-9 reproducibility is redefined for the persisted `offers-implied` row: the reproducible unit is the assembled record given (snapshot, diagnostics), not `ComputeAssetRisk` alone. This preserves byte-identity of a recompute that is given the stored diagnostics, and keeps the risk computation free of acquisition. Section 8's blast radius for Option B gains one domain type and one assembly function; the risk math is untouched.

---

## 6. What this record does NOT authorise, and one of these is the reason it is separate

**It does not authorise the two control ledgers.** 61340262 and 61340263 are the
obvious first use and this record deliberately does not reach them.
`docs/report/blend-february-2026.md` section 5.4 records that the repaired run
disagrees with the hand-computed golden fixture on four quantities at 61340263:

| Quantity at 61340263 | Fixture, by hand | The run |
| --- | --- | --- |
| `maxReachablePrice` | 106.7372828 | 2147483647 |
| `costToMaxReachablePrice` | 0 | 124.715139 |
| `Reachable` at δ = 1, 10, 100 | false | true |
| Asks on the book | 1 | 2 |

and the report closes with the instruction that those columns "must not be quoted for
the two control rows until this is settled". Persisting those two ledgers publishes
precisely those quantities through the API, to anyone, with no report around them. That
question is open, it belongs to the fixture, and the fixture is RED. **A decision on the
mechanism must not smuggle in a decision on the disputed rows.** If this record is
accepted, the first ledgers to be stored are named in a second, smaller decision.

**It does not give `keel bookseries` a persist path.** That command is the cheap route
to many ledgers, one walk behind all of them, and `bookseries.go:101` states that pools
are not reconstructed and every row is order book only. Storing an order-book-only row
as a combined-depth result understates depth in exactly the direction section 4 warns
about, and no `-pool-snapshots` equivalent exists there. A series in the database is a
separate piece of work with its own cost.

**It does not touch DEC-002.** No source is undeferred, no adapter is proposed, and
`internal/hubble/` still holds no Go. If Hubble is ever taken, `internal/api` reads one
source at that path today and ordering two is a decision, which the code comment at
`internal/api/api.go` already assigns to DEC-002 rather than to a patch.

**It does not flip `-historical` in production.** That is one word in
`docker-compose.prod.yml` and a restart, it is Al's under `scripts/deploy/`, and the
order is not optional: rows first, flag second. RUNBOOK section 4 already records why,
that flipping the flag over an empty table turns a truthful 503 into a 404 that says
the ledger is missing rather than that this deployment serves no history.

---

## 7. The acceptance test, pre-registered

Registered before any code is written, on DEC-021's precedent, so that passing cannot
be defined after the fact.

1. **The default is unchanged.** `keel replay -persist -pool-snapshots F` over a
   reconstruction with any nonzero counter still fails, with today's message. A test
   asserts the refusal, not the absence of a flag.
2. **A crossed book is still refused, with `-accept-incomplete` set.** No override
   exists for it at any flag combination.
3. **An inflated book is still refused, with `-accept-incomplete` set.**
4. **Every nonzero counter appears in the stored row.** Given a fixture with
   `Truncated: 3, Failed: 1, StoppedAtFloor: 42`, the row read back from the store
   carries three warnings naming those three counters and their values.
5. **The API serves them.** `GET /v1/asset/{id}/depth?ledger=` on that row answers 200
   with `dataSource: "offers-implied"` and those warnings present in the array. The
   integration test at `cmd/keel/replay_integration_test.go` already runs this shape
   end to end against a real Postgres and is extended rather than duplicated.
6. **No contract change is needed for Option A.** `make api-mocks-check` passes
   untouched. If it does not, Option A was misjudged and the record returns to Al.

---

## 8. Cost and blast radius, by zone

| File | Zone | Change |
| --- | --- | --- |
| `cmd/keel/replay.go` | GREEN | one flag, the gate split into two, warnings composed from the counters |
| `cmd/keel/replay_test.go`, `replay_integration_test.go` | GREEN | the six tests in section 7 |
| `internal/horizon/replay.go` | YELLOW | none expected. `Complete()` keeps its meaning; the caller stops treating it as one boolean |
| `docs/methodology/11-limitations.md` | RED | one paragraph, Al's, saying a stored historical row may carry named gaps |
| `docs/api/keel-openapi.yaml` | YELLOW | none under Option A |
| `migrations/` | GREEN | none under Option A |

Under Option B, add a migration, a domain field, a contract minor bump and regenerated
mocks.

---

## 9. What would make this record wrong

1. **If a bounded walk can pass after all.** Section 3 is a reading of two flags and two
   meta files, not an exhaustive search. A single run at a real February ledger with no
   floor, reporting `walk_complete: true`, falsifies section 3 and this record should be
   rejected on the spot.
2. **If the thin-book bias is circular here.** The February claim is that the book was
   thin. Storing a book that is thin partly because the walk lost offers is evidence
   that argues for its own conclusion. The warnings are the mitigation and they may not
   be enough for the one asset the whole deliverable is about, which is a second reason
   section 6 keeps the control ledgers out.
3. **If nobody reads warnings.** The mechanism rests entirely on a consumer rendering an
   array of prose. If the dashboard shows them in a tooltip or not at all, Option A has
   labelled the row for a reader who never arrives, and Option B was the right choice.

---

## 10. Decision

**Decided by Al, 2026-09-17.** Recorded by Claude; the choice is Al's, the transcription is not.

**The mechanism in section 5.1 is accepted.** The asymmetric gate stands: a reconstruction whose detectable gaps all run in the pessimistic direction MAY be persisted behind `-accept-incomplete`, with every nonzero counter written into the stored row. `MayBeInflated` and `Crossed` keep their refusal with no override. Pool coverage stays required, `-pool-snapshots` untouched.

**The qualification is carried by Option B, not Option A.** This overrides the record's own recommendation. The gaps are stored in a machine-readable field so a consumer can branch on them, not only read them. That accepts, deliberately, the four costs sections 5.2 and 8 attach to B: a JSONB column and a migration in `migrations/` (GREEN), a new field on `AssetRisk` in `internal/domain` (YELLOW), and a new schema object with a contract minor bump and regenerated mocks in `docs/api` (YELLOW).

**This decides against section 5.3's recommendation, and does so knowingly.** Option B is the "alternative" 5.3 named: it puts walk diagnostics into `internal/domain`. Al accepts that `AssetRisk` carries what the acquisition produced. One sub-question stays open and is Claude's to draft, not to settle: whether the new field is populated inside `domain.ComputeAssetRisk` or set by the caller at the edge. Populated inside, a recompute is byte-identical and NFR-9 is honoured, at the cost of a pure function that knows an acquisition fact; set at the edge, domain purity holds but the row is no more reproducible than under Option A. Al decides that before code is written.

### 10.1 The sub-question, answered

**Decided by Al, 2026-09-17, the same day.** Recorded by Claude; the choice is Al's,
the transcription is not. **The field is populated INSIDE the domain, through a new
entry point that takes it as an argument.** `Snapshot` is not widened.

**The question had three answers, not the two named above, and the third is the one
taken.** The paragraph above offered "inside `ComputeAssetRisk`" against "at the edge",
and "inside" was read as putting the diagnostics into `Snapshot` so the function could
copy them through. That reading was already refused in this repository, by the header of
`ComputeAssetRiskWith` at `internal/domain/compute.go:553`, about the same class of
data:

> The alternative rejected was adding Trades and Holders to Snapshot: a Snapshot is the
> book and the pools at one ledger, a trade history is a range of ledgers and a
> trustline pull is current-state-only and has no ledger at all, so putting all three in
> one struct would have made LedgerSeq mean three different things.

A walk diagnostic is a fact about how a snapshot was ACQUIRED, not about the market at
that ledger, so it fails that test the same way. The pattern that header chose instead
was a second entry point, and this decision takes a third one rather than inventing a
mechanism beside it.

**What that buys and what it costs.**

- `Snapshot` keeps meaning one thing, which is the whole of the objection above.
- The stored row is reproducible from its inputs, because the provenance IS an input.
  That is the NFR-9 property the edge option could not have, and it is why Option B was
  chosen over A in the first place.
- `compute.go` is touched, so obligation 1 of `internal/domain/CLAUDE.md` applies and is
  answered here rather than discovered later: **no fixture value judges this field.** It
  carries no methodology number. It reports counters produced by acquisition, and the
  ordering rule's subject is a computed quantity. The obligation is to say so out loud,
  in the code and in the report, and that is what this paragraph and the code comment
  do. The golden fixture is RED and nothing here asks it to move.

**The gate in section 10 is therefore closed and the work may start.** Section 6 is
untouched by this: no ledger is authorised for storage by it, and `-historical` stays
off in production.

**Two consequences section 7 must absorb.** Item 6 of the pre-registered test ("No contract change is needed for Option A", `make api-mocks-check` passes untouched) is now inverted: under Option B the contract changes on purpose, so the check is regenerated and re-registered, not asserted stable. Items 1 through 5 stand; item 4's read-back now asserts the field as well as the warnings array.

**Scope is unchanged.** Everything section 6 refuses stays refused: the control ledgers 61340262/61340263, a `keel bookseries` persist path, DEC-002, and the `-historical` flag in production. The first ledgers to be stored are named in a second, smaller decision. Rows first, flag second.

**Still owed by Al, RED.** The one paragraph in `docs/methodology/11-limitations.md` recording that a stored historical row may carry named gaps. This decision does not write it.