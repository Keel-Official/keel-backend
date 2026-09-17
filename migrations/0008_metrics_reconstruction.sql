-- A reconstructed row carries what its walk could detect about its own gaps.
--
-- DEC-022, accepted by Al on 17 September 2026, Option B: the gaps are stored in a
-- machine-readable field so a consumer can branch on them rather than only read
-- prose. The warnings array keeps carrying the same facts in sentences; this is
-- the half a dashboard can switch on.
--
-- WHY THE ROW EXISTS AT ALL, WHICH IS THE PART WORTH KEEPING HERE. Before this
-- decision `keel replay -persist` refused every reconstruction whose walk reported
-- any hole in itself, and DEC-022 section 3 showed that no configuration of the
-- walk can avoid one: a floor makes walks stop at the floor, and removing the
-- floor forces an unbounded trade walk that internal/horizon refuses to pair with
-- a bounded one. So the table could never receive an offers-implied row, and the
-- API's historical path could never answer anything but 503. The gate is now
-- asymmetric: gaps that lose offers are admissible because they make the book too
-- THIN, and the two that do not, an inflated book and a crossed one, stay refused
-- with no override and can therefore never appear in this column.
--
-- JSONB AND NOT SIX COLUMNS. The shape is one object produced by one walk and read
-- as a unit, never filtered or aggregated on, and the same argument the depth and
-- manipulation ladders were stored under applies unchanged. Six columns would also
-- have to be six nullable columns whose nullness has to agree row by row, which is
-- the invariant 0007 had to add a CHECK for. Here there is nothing to keep in
-- step: the object is present or it is not.
--
-- NULL MEANS "NOT A RECONSTRUCTION" AND NEVER "NO GAPS WERE FOUND", and the
-- constraint below is what makes that readable rather than promised. A live
-- Horizon scan reads the book as it stands and walks nothing, so it has no
-- counters to report; an empty object on such a row would assert a clean walk that
-- never happened. Every row in this table today is data_source 'horizon' and every
-- one of them keeps a NULL here, correctly.
--
-- THE CONSTRAINT IS `NOT VALID` FOR THE REASON 0007 GIVES AND NOT AS A WEAKENING.
-- Postgres applies it to every INSERT and UPDATE from now on and skips only the
-- scan of existing rows. Every existing row has NULL in a column that did not
-- exist a moment ago, so the scan it skips could not fail; `NOT VALID` here buys
-- the lock and nothing else. Validate it whenever, or leave it.

BEGIN;

ALTER TABLE metrics
    ADD COLUMN IF NOT EXISTS reconstruction JSONB;

COMMENT ON COLUMN metrics.reconstruction IS
    'What the acquisition walk could detect about its own completeness, for a row whose '
    'data_source is a reconstruction. NULL means this row is not a reconstruction; it NEVER '
    'means a walk ran and found no gaps. Every counter inside loses offers, so a row carrying '
    'them describes a book that is too THIN. See DEC-022 and migrations/0008.';

-- A live read has no walk, so it can have no walk diagnostics. This is the column
-- level statement of the sentence in the comment above: without it, a NULL would
-- be ambiguous between "read directly" and "reconstructed, gaps unrecorded", and
-- the second of those is the state DEC-022 exists to make impossible.
--
-- 'hubble' is permitted here although DEC-002 defers it and no such row can exist.
-- A replay of stored historical state is a reconstruction in the sense this column
-- means, so excluding it would encode an availability decision as a schema rule
-- and leave a trap for whoever undefers it. The data_source CHECK in 0003 is what
-- bounds the set of sources; this bounds only which of them may carry diagnostics.
ALTER TABLE metrics
    ADD CONSTRAINT metrics_reconstruction_only_on_reconstructed_rows CHECK (
        reconstruction IS NULL
        OR data_source IN ('hubble', 'offers-implied', 'trades-implied')
    ) NOT VALID;

COMMIT;
