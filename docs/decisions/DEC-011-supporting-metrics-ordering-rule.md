# DEC-011: A holder pull carries one snapshot ledger, and mid-pull mutation is detected rather than assumed away

**Status:** Accepted 2026-09-02 by Al. `MaxLedgerSpan` set to **24 ledgers** (see "The resolved value").
**Date drafted:** 2026-09-02
**Zone:** YELLOW (`docs/decisions`; implementation touches `internal/horizon/holders.go`, also YELLOW)
**Supersedes / reverses:** nothing.
**Referenced by:** `docs/methodology/07-supporting-metrics.md` 1.0.6-draft onward, section 2
Result ("Snapshot integrity") and section 3 ("Anchoring caveat").

---

## Decision

1. **A holder pull resolves to exactly one `snapshot_ledger`, defined as the minimum
   `latest_ledger` observed across all pages of that pull.** It is recorded as a column on
   every row and is the `LedgerSeq` the pull carries downstream.

2. **Mid-pull mutation is detected, counted, and recorded, not assumed absent.** Any row whose
   `last_modified_ledger` exceeds `snapshot_ledger` provably changed after the moment the
   snapshot is labelled with. The recorder counts these rows and writes the count and their
   combined balance alongside the pull.

3. **A pull is labelled `atomic` when the mutated-row count is zero and `mixed` when it is
   not.** The label travels with the data. Section 2 must state which label the figures it
   reports were computed from.

4. **The recorder refuses a pull whose ledger span exceeds `MaxLedgerSpan` = 24 ledgers.**
   Refusal is hard: no partial write, no "best effort" record. (See "The resolved value" for
   why 24.)

5. **Mutation does not by itself trigger refusal.** A detected mutation is recorded and
   labelled; it does not discard the pull.

---

## Reason

A holder pull is a sequence of page reads, not a read of one ledger, so no ledger describes it
exactly and any single `LedgerSeq` stamped on it is an approximation whose error must be
bounded rather than hidden. Taking the minimum makes that approximation conservative in the
direction that matters: staleness is over-reported rather than under-reported, so
`X-Keel-Staleness-Seconds` never claims the data is fresher than it is. Detecting mutation
through `last_modified_ledger` converts the problem from an unknowable one into a measured
one, because a row that changed mid-pull leaves evidence in the data already being recorded.

**Rejected alternative:** stamp the pull with `max(latest_ledger)`. It is the more natural
reading of "when was this pull finished", and it would make the mutated-row test come out
clean on the 31 August pull, since no row there has a `last_modified_ledger` above the maximum.
That is exactly why it is wrong: it makes the inconsistency undetectable by construction and
reports the data as fresher than its oldest page actually is. Choosing the label that hides
the defect is the same failure DEC-010 rejected for backtest windows that had not yet closed.

---

## Cause / Basis

### The pull is provably non-atomic

`docs/evidences/2026-08-31-USTRY.GCRYUGD5-holders-and-supply/holders.csv`, the only holder pull
recorded to date:

| Field | Value |
|---|---|
| Trustline rows | 875 |
| Pages | 5 (200 / 200 / 200 / 200 / 75) |
| `latest_ledger` per page | 64211133, 64211138, 64211142, 64211144, 64211152 |
| Ledger span | 19 |
| `read_at_utc` span | 2026-08-31T16:00:22Z to 16:01:53Z (91 seconds) |

Five distinct `latest_ledger` values. Non-negotiable rule 1 requires every output to carry one
`LedgerSeq`, and this pull as recorded carries five.

### Mutation is not hypothetical, and it is detectable

Testing every row's `last_modified_ledger` against `min(latest_ledger) = 64211133`:

| Test | Result |
|---|---|
| Rows with `last_modified_ledger` > 64211133 | **1** |
| Rows with `last_modified_ledger` > 64211152 (the maximum) | 0 |

The one row is `GCSO6DAFG52J…`, `last_modified_ledger` 64211140, read on page 4 at
`latest_ledger` 64211144, balance 0.0000001 USTRY. Its trustline changed after page 1 was read.
The pull therefore mixes pre-change and post-change state, and this is demonstrated from the
recorded columns alone, with no additional network call.

The second row of that table is the argument against `max(latest_ledger)` stated as data: the
test returns zero under that choice for every pull, by definition, since Horizon cannot report
a modification later than the ledger it is currently on.

### Materiality of the one known mutation

At 0.0000001 USTRY against a circulating supply of 10,432,382.3504695, the row's share rounds
to 0.0000% at the four-decimal scale section 2 reports. No published figure in
`07-supporting-metrics.md` 1.0.7-draft changes: population 263, top 1 91.5406%, top 10
99.9475%, HHI 8,410.8452 all stand. The 31 August pull is labelled `mixed` and its figures
remain usable.

### Why refusal is bounded on span but not on mutation

The two conditions guard different failures. A large span means many pages were read far apart,
so undetected drift is likely across rows whose `last_modified_ledger` happens to fall below
the minimum; that risk grows with span and is not visible in the data, so it must be bounded
in advance. A detected mutation is the opposite case: it is visible, countable, and its
materiality can be computed, so discarding the pull over it would throw away a usable
measurement to avoid a quantified and disclosed error. Since `/accounts?asset=` is
current-state only and cannot be backfilled, a discarded pull is gone permanently, which makes
refusal the more expensive choice wherever the error can instead be measured and labelled.

---

## The resolved value

`MaxLedgerSpan` is set to **24 ledgers**. The 31 August pull spans 19 ledgers, so any value
below 19 makes that pull retroactively invalid, and any value at or above 19 accepts it.

| Value | Effect on the 31 Aug pull | Effect on future pulls |
|---|---|---|
| 12 (about 60s) | rejected | forces faster paging or a smaller page count; may fail on assets with many more trustlines |
| **24 (about 2 min)** | **accepted, with roughly 25% headroom** | **tolerant of transient Horizon latency; still well inside the 900s staleness target** |
| 180 (about 900s) | accepted | matches the staleness target but permits a snapshot spread so wide the label stops meaning much |

24 sits above the one observed real pull (19) with headroom for latency, and far below the
900-second staleness target, because the threshold's job is to catch a pull that went wrong,
not to define what "fresh" means.

---

## Implementation

`internal/horizon/holders.go` (YELLOW): compute `snapshot_ledger` as the minimum
`latest_ledger` across pages; compute `ledger_span` as maximum minus minimum; refuse when
`ledger_span > MaxLedgerSpan` (24); count rows whose `last_modified_ledger` exceeds
`snapshot_ledger` and sum their balances; derive the `atomic` / `mixed` label from that count.

Recorded columns added to the pull CSV: `snapshot_ledger`, `ledger_span`, `snapshot_label`,
`mutated_rows`, `mutated_balance`. Existing columns are unchanged, so previously recorded pulls
remain readable and can be re-labelled by recomputation without a refetch.

`scripts/holderstats` (GREEN): its `-max-ledger-spread` flag defaulted to 0 (report without
refusing) pending this record. On acceptance the default becomes 24, and the report prints the
`atomic` / `mixed` label rather than the free-text note it prints now.

Retroactive labelling of the 31 August pull is a recomputation over recorded columns, not a
refetch. The pull itself is not re-run and could not be.

---

## Reproduction

```
python3 - <<'PY'
import csv
rows = list(csv.DictReader(open('holders.csv')))
snapshot = min(int(r['latest_ledger']) for r in rows)
span = max(int(r['latest_ledger']) for r in rows) - snapshot
mutated = [r for r in rows if int(r['last_modified_ledger']) > snapshot]
print('snapshot_ledger', snapshot, 'ledger_span', span)
print('mutated_rows', len(mutated), 'label', 'atomic' if not mutated else 'mixed')
for r in mutated:
    print(' ', r['account_id'], r['last_modified_ledger'], r['balance'])
PY
```

---

## Open items this record does not settle

1. Whether a `mixed` label should propagate into the API response, for example as a header
   alongside `X-Keel-Staleness-Seconds`. That is an API contract change and falls under DEC-003
   freeze conditions, so it needs the frontend collaborator's sign-off and is out of scope here.
2. Whether section 3's backfilled volume-to-supply ratios should carry the snapshot label as
   well as the snapshot date. Section 3 is RED; this record raises the question and does not
   answer it.
