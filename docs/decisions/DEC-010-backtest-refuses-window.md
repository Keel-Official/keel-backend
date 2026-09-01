# DEC-010: The backtest refuses a window whose end has not passed

**Status:** Proposed
**Date:** 2026-09-01
**Kind:** New constraint on a GREEN component. No methodology or contract changes.
**Drafted by:** Claude
**Decided by:** Al
**Zone:** `docs/decisions/` (YELLOW). Claude drafts and amends; Al decides.
**Methodology version:** `1.0.3-draft`, unchanged. This is a rule about when data
may be recorded, not about what is computed from it.

---

## 1. The decision

`cmd/keel/backtest.go` refuses to run when the requested window's end instant has
not yet passed at execution time, and every trade CSV it writes is accompanied by
a sidecar provenance file recording the window bounds, the row count, and the
observed minimum and maximum `closed_at`.
## 2. What happened

On 2026-08-31 at 16:09:55Z the backtest was run for the window 2026-08-01 to
2026-09-01. The window's end was more than seven hours in the future. It produced
a file named for that window holding 56,615 trades, which was every trade that
existed at 16:09Z and not every trade in the window.

Nothing failed. The file is well formed, its columns are correct, every row in it
is real, and it passed every check the repository had. The defect is entirely in
what is absent, and absence has no signature.

It was found only because a later re-read returned more rows and the difference
was mistaken for a regression in an unrelated patch.

**Three reads of the same nominal window, and only the third is complete:**

| Read | Taken at | Rows | `max_closed_at` | `stopped_past_window` |
| --- | --- | --- | --- | --- |
| 1 | 2026-08-31T16:09Z | 56,615 | 2026-08-31T15:57:10Z | not recorded, predates the sidecar |
| 2 | 2026-09-01T04:20Z | 56,759 | 2026-08-31T18:53:48Z | **false** |
| 3 | 2026-09-01T04:47Z | 56,863 | 2026-08-31T23:49:01Z | true |

**The first cause: recording a window that had not closed.** Read 1 was taken
inside its own window. This is what section 1's clock check refuses, and it would
have refused that run.

**The second cause is distinct and the clock check does not touch it. A window
that has closed is not necessarily a window that is complete.** Read 2 was taken
more than four hours after the window ended and still returned 104 fewer trades
than read 3, twenty-seven minutes later. Those 104 trades had closed by
2026-08-31T23:49:01Z and were absent from Horizon's `/trades` index at the earlier
time. No parameter of the run was different.

**The walk itself is sound, and this was tested rather than assumed.** The same
day, the window 2026-08-31 to 2026-09-01 was walked twice: once seeking to ledger
64201036, reading 1,169 records over 6 pages, and once with no seek, reading
353,901 records over 1,770 pages. Both returned the same 1,169 trades, so
`-from-ledger` is a seek optimisation and not a filter. The 2026-08-31 subset of
read 3 is identical by `trade_id` to the whole of that narrow window, so the long
walk and the short walk agree row for row. An earlier hypothesis that the walk
terminated arbitrarily and reported success is disproven by these three runs.

**The signal that detects the second cause already existed and already worked.**
`stopped_past_window` was `false` on read 2 and `true` on read 3 and on both
narrow-window runs. It reports whether the walk ran off the end of the pair's
history rather than seeing a trade at or past the boundary, which is precisely
what an incomplete read looks like. It was recorded correctly, read by two
reviewers in sequence, called harmless by both, and passed over. The failure was
not in the instrumentation.

**Three artifacts carried a number derived from a truncated file:**

| Artifact | Stale claim |
| --- | --- |
| `docs/evidences/2026-08-26-ustry-february-trades-implied.md` line 28 | separate defect, see the pool-id finding |
| `docs/evidences/2026-08-31-funding-graph-probe/report.md` line 66 | "All rows 56615"; the complete file holds 56,863. Its pool-trade count of 596 is correct and unaffected |
| the trade-pool-id regeneration task, step 9 | asserted `rows 56615` as an expected value. Authored by Claude from the truncated file without checking its provenance |

The daily CSV for 2026-08-31 read 921 trades against the complete figure of 1,169.
That row was wrong before and is right now. It is a correction, not a regression,
and must not be recorded as an effect of the pool-id patch.

## 3. Rationale

**The defect class is an artifact that is correct in every respect except
completeness.** Not one truncated file. A recording named for a window it does
not cover is well formed, every row in it is true, and it satisfies every schema
and every count check that can be written against it. What is wrong about it is
what is not in it.

**This class defeats every defence this repository has, and it is the only one
that does.** The pool-id defect announced itself as an empty column once someone
looked. A wrong fixture value announces itself as a failing test. A contract
change announces itself as a regenerated mock. Absence announces nothing. The
truncated August file passed `make ci`, `make test` and `make arch`, was cited
by two evidence documents, and supplied the expected value for an assertion in a
later task, where its failure was briefly read as a regression in an unrelated
patch. Four artifacts were touched before anything registered, and what
registered was the wrong thing.

**The data was always recoverable; the beliefs built on it were not.** `/trades`
over a closed window is addressed by cursor and can be re-read at any time, so
the file itself was never at risk. What could not be re-read were the two evidence
documents and the assertion that had already quoted it. This is the inverse of the
order book recordings, where the data is unrecoverable and the citations are
cheap. Both cases point the same way: the moment to be strict is the moment of
recording.

**The capability being given up is one Keel does not want.** The rule forbids
recording a window that has not ended. A window that has ended is available
immediately after it ends. The only thing lost is the ability to record a
partial period under a whole period's name, which is the defect itself rather
than a use case.

**The asymmetry decides it.** A refused run costs the wait until the window ends,
which is bounded and known in advance. An accepted one cost four artifacts, one
false assertion, and an hour spent suspecting a patch that was innocent. NFR-9
requires that two runs of the same nominal window produce the same data; under
the old behaviour they did not, and nothing in the output distinguished the two
results.

## 4. What it does not cover

This record does not address the two stale citations above. Each is a prose
correction in `docs/evidences/` with its own owner and its own date, and folding
them in here would give one record two subjects.

It does not change `.claude/`, the API contract, any methodology document, or any
fixture.

It does not decide what happens to the truncated file's name in git history.

## 5. Design note, required by the YELLOW zone rule

**On where the refusal sits.** It is placed at the point the file is created
rather than at the point it is read, because a file that never existed cannot be
cited while a warning attached to one that does exist can be, and was. The
refusal is a hard error with no flag, no environment variable and no force mode,
so there is no path by which a partial window acquires a whole window's name. The
rejected alternative was to keep producing the file and emit a warning when the
window is open: rejected because that warning already existed in the form of
`requested_at_utc` in the probe metadata, was recorded correctly, and was read by
nobody, so adding a second one repeats a mechanism with a demonstrated failure.

**On where the provenance sits.** It is written to a sidecar file beside the CSV,
under the same basename, rather than into the CSV as a leading comment line. The
sole live consumer of these files parses them with Python's `csv.DictReader`,
which has no comment syntax and reads a `#` line as the header row, so an inline
comment breaks every field lookup in the script that exists to record provenance;
this was tested rather than assumed. Keeping the CSV a pure RFC 4180 table also
leaves it readable positionally or by name by any reader, while the shared
basename keeps the coverage claim from being separated from the data by accident.

The rejected alternative was the leading comment line, which was this record's
first preference and lost on evidence rather than on taste. Its failure mode is a
`KeyError` on the first row, which is loud; the version worth fearing is a reader
that silently absorbs the shifted header and produces quietly wrong counts, and
the sidecar avoids both.

Every field in the sidecar is derived from the records and none from the wall
clock, so a re-read of a closed window produces identical bytes and any diff means
the data moved rather than the clock did. A `generated_at_utc` stamp was rejected
on that ground and a test guards against its return.

## 6. Consequences

The refusal is a hard error, not a flag. A partial window is available by asking
for a window that has ended.

Any future artifact naming a window that has not ended is, by this record, either
predating it or a bug.

## 7. Open items

1. Ratify or reject.
2. Re-run any recording made for a window that had not closed at execution time.
   Whether any exist besides the August one is unknown and unowned.
3. `report.md` line 66 and the line-28 correction remain open under their own
   owners.

## 8. Amendment history

Append-only from first commit.

| Date | Amendment |
| --- | --- |
| 2026-09-01 | Created as Proposed |