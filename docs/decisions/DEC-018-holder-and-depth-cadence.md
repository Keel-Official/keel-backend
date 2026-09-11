# DEC-018: A metrics row mixes two cadences, and the older half carries its own ledger

**Status:** Accepted by Al
**Date drafted:** 2026-09-12
**Kind:** Data provenance. It does not change any formula, any threshold or any
computation. It decides what a stored row MEANS once its two halves stop coming from
one ledger, and what the contract says about that.
**Drafted by:** Claude
**Decided by:** pending
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends a record here and must
not create or reverse a decision. Section 1 is Al's. Sections 2 and 3 are arithmetic
and code readings and are evidence, not decision.
**Supersedes / reverses:** nothing. It EXTENDS DEC-011 to a case DEC-011 does not
reach, and section 4 is careful about the difference.
**Referenced by:** nothing yet. On acceptance: `docs/methodology/07-supporting-metrics.md`
section 2, and the A1 work in `cmd/keel/scan.go` and `internal/store/`.

---

## 1. The proposed decision

1. **Holder concentration and order book depth are read on DIFFERENT cadences, and
   that is permanent rather than an interim measure.** Depth is read every scan round.
   The holder pull runs on its own schedule, once a day, and `scan` reads the newest
   cached reading rather than pulling one of its own.

2. **A metrics row therefore carries two ledgers, and BOTH are stored.** `LedgerSeq`
   stays what it is today: the ledger the book and the pools were read at. The holder
   half carries the `snapshot_ledger` of the pull it came from, under a separate
   column, and that column is never null when a holder figure is non-null.

3. **A holder reading older than `MaxHolderAge` makes holder concentration
   `unevaluated`, not stale.** The proposed value is **48 hours**. Past it,
   `HolderTop1Pct`, `HolderTop10Pct` and `HolderHHI` are nil and the two holder flags
   read `unevaluated` exactly as they do when a pull was truncated.

4. **The API exposes the holder half's own ledger and its age.** A consumer that
   cannot see the gap cannot reason about it, and the gap is a property of the answer
   rather than of the implementation.

5. **This record does not permit the gap to grow silently.** If the holder cadence
   ever moves from daily, `MaxHolderAge` is reconsidered in the same change, because
   the two numbers only mean something as a pair.

---

## 2. Why the cadences cannot be one cadence, and this half is arithmetic

`scan` runs every 15 minutes, so four rounds an hour.

One holder reading costs 1 summary request plus 1 request per page of 200 accounts,
capped at 25 pages by `defaultMaxHolderPages` in `internal/horizon/holders.go`. So the
worst case is 26 requests for one asset.

Sixty assets is up to 1,560 requests in one round. Four rounds an hour is up to
**6,240 requests per hour**, against the scanner's own budget of **3,000** and public
Horizon's roughly 3,600 per IP per hour.

So a holder pull inside the scan round is not merely expensive, it exceeds the budget
by itself and would starve the depth reading that is the deliverable. No tuning of the
page cap rescues it: the cap would have to fall to about 11 pages, which truncates
every popular asset, and a truncated pull answers a concentration question not at all
rather than approximately.

A second, independent reason: the two quantities are different SHAPES. Depth is a
property of one ledger and is meaningless smeared across a range. Holder concentration
is a snapshot of current state that no ledger describes exactly, which is the whole
subject of DEC-011. Forcing them onto one cadence would not make the row more coherent,
it would make the holder half wrong more often.

---

## 3. What a row means once this lands, stated plainly

Today every field in a `metrics` row is derived from one ledger, and `LedgerSeq`
answers "as of when" for the whole row. After A1 that stops being true. A row will
read, in effect:

> at ledger 64,381,285 the book supported this much depth, and as of a holder pull
> taken at ledger 64,362,110 yesterday, the top holder held this share.

That is a different kind of statement, and the risk is not that it is wrong. It is
that it looks exactly like the old one. A reader who takes `LedgerSeq` to cover the
whole row will read a holder figure as current when it can be a day old, and nothing
in the response contradicts them. Non-negotiable rule 1 in `CLAUDE.md` says every
output carries `LedgerSeq` and `MethodologyVersion` because a number without the
ledger it came from is a rumour. The holder half would have no ledger of its own,
which is the same failure wearing the row's ledger as a disguise.

---

## 4. What DEC-011 already settles, and where it stops

DEC-011 settles non-atomicity **inside one pull**: a pull resolves to one
`snapshot_ledger` taken as the minimum `latest_ledger` across its pages, rows modified
after it are counted, the pull is labelled `atomic` or `mixed`, and a ledger span above
24 is refused outright.

Every one of those is about the pages of a single reading disagreeing with each other
by a few ledgers.

This record is about a different distance: **between a holder reading and a depth
reading that were never intended to be simultaneous**, measured in hours rather than
in ledgers. DEC-011's `MaxLedgerSpan` of 24 ledgers is about two minutes. The gap here
is up to a day, three orders of magnitude larger, and bounding it with the same number
would refuse every row.

So the two records compose rather than conflict: DEC-011 says which single ledger a
holder pull is stamped with, and this record says what happens when that stamp is
carried into a row stamped with a different one.

---

## 5. The alternatives, and why each is worse

**Pull holders inside the scan round.** Rejected on section 2's arithmetic. It does
not fit in the request budget, and the version that fits truncates every asset worth
measuring.

**Store the holder half with no ledger of its own and let `LedgerSeq` cover the row.**
Rejected because it is the failure in section 3 exactly. It is also the cheapest
option, which is why it needs writing down rather than leaving to be defaulted into.

**Let a stale reading through with a staleness field and no bound.** Rejected, but
less firmly, and this is the alternative most worth arguing about. It is honest as far
as it goes. What it lacks is a point at which the answer stops being an answer: a
holder reading from three weeks ago, correctly labelled as three weeks old, is still
reported as a concentration figure, and a consumer filtering on `bandConfidence` will
see `full`. The proposed 48 hours makes the engine say "I do not know" instead, which
is the same distinction `ErrHolderSetTruncated` already draws between zero and absent.

**Emit two separate result objects, one per cadence.** Rejected for now. It is the
cleanest model and it breaks the contract, every stored row and the dashboard. Worth
revisiting if a third cadence ever appears; see section 7.

---

## 6. What changes on acceptance

Nothing computational. `internal/domain` is untouched: `ComputeSupporting` already
takes holders as an input and already returns nil figures when they are absent or
truncated, so a stale reading is expressed with the machinery that exists.

- `migrations/`: the holder cache table carries `snapshot_ledger`, and the `metrics`
  table gains a nullable column for the holder half's ledger.
- `internal/store/`: `LatestHolderReading` returns the reading with its
  `snapshot_ledger` and its age.
- `cmd/keel/scan.go`: reads the newest reading, applies `MaxHolderAge`, and passes
  `HoldersKnown: false` when it is exceeded.
- `docs/api/`: the holder ledger and age become response fields, which is a contract
  change and therefore a version bump under DEC-003.
- `docs/methodology/07-supporting-metrics.md` section 2: the anchoring caveat gains
  the second distance. **This is the one edit outside Track A's files**, and under the
  carve-out in `tugas-a.md` section 7 the methodology directory is Track B's except
  where a record assigns it. Name the owner before editing it.

---

## 7. What this record does not settle

1. **Whether `MaxHolderAge` is 48 hours.** It is proposed, not derived. Nothing has
   been measured about how fast the top 1 percent share of a thin Stellar asset moves,
   and that measurement would settle it properly. 48 hours is two daily pulls plus
   room for one to fail, which is an operational argument rather than a statistical
   one, and it should be replaced by a measured value when one exists.
2. **Whether the `mixed` label from DEC-011 propagates into the API response.**
   DEC-011 left this open and this record does not close it.
3. **Whether a row whose holder half is unevaluated should still be stored.** The
   proposal assumes yes, because the depth half is the deliverable and is complete.
4. **The trade half of the supporting metrics.** Trades are an append-only event log
   with a different cost profile again, and they are deferred under the split in
   `tugas-a.md` section 6b. A third cadence would reopen the "two separate result
   objects" alternative in section 5.
