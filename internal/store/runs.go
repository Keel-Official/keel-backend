// The runs table: one row per scan or replay job.
//
// It exists so that a partial failure is visible instead of silent. One asset
// failing must not fail a whole scan, which means the scan finishes reporting
// success, which means the only place the failure can be recorded is here.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RunKind is the CHECK on runs.kind. There are two, and `record` is deliberately
// not one of them: the cross-validation recorder writes files and touches no
// table, so giving it a run row would imply a database dependency it does not
// have.
type RunKind string

const (
	RunScan   RunKind = "scan"
	RunReplay RunKind = "replay"
)

type Run struct {
	ID           int64
	Kind         RunKind
	StartedAt    time.Time
	FinishedAt   *time.Time
	AssetsOK     int
	AssetsFailed int
	Notes        string
}

// StartRun opens a run and returns its id. startedAt is passed in rather than
// read from the clock here, so that the caller's notion of when the job began is
// the one recorded, and so this package needs no clock of its own.
func (s *Store) StartRun(ctx context.Context, kind RunKind, startedAt time.Time) (int64, error) {
	switch kind {
	case RunScan, RunReplay:
	default:
		return 0, fmt.Errorf("store: run kind %q is not scan or replay", kind)
	}
	var id int64
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO runs (kind, started_at) VALUES ($1, $2) RETURNING id`,
		string(kind), startedAt.UTC(),
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: start %s run: %w", kind, err)
	}
	return id, nil
}

// FinishRun closes a run with its counts.
//
// It refuses to close a run twice. A second close would overwrite the counts of
// the first, and a run that reported ten failures being quietly replaced by one
// reporting none is the exact failure this table exists to prevent.
func (s *Store) FinishRun(ctx context.Context, id int64, finishedAt time.Time, assetsOK, assetsFailed int, notes string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE runs
		   SET finished_at = $2, assets_ok = $3, assets_failed = $4, notes = NULLIF($5, '')
		 WHERE id = $1 AND finished_at IS NULL`,
		id, finishedAt.UTC(), assetsOK, assetsFailed, notes)
	if err != nil {
		return fmt.Errorf("store: finish run %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: open run with id %d", ErrNotFound, id)
	}
	return nil
}

// LastRun returns the most recently STARTED run of one kind, finished or not. An
// unfinished run is the interesting case: it means a job died without closing
// its row, and hiding it behind a finished_at filter would make a crashed scan
// look like a scan that never ran.
func (s *Store) LastRun(ctx context.Context, kind RunKind) (Run, error) {
	var (
		r        Run
		finished sql.NullTime
		notes    sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, kind, started_at, finished_at, assets_ok, assets_failed, notes
		  FROM runs WHERE kind = $1
		 ORDER BY started_at DESC, id DESC LIMIT 1`, string(kind),
	).Scan(&r.ID, &r.Kind, &r.StartedAt, &finished, &r.AssetsOK, &r.AssetsFailed, &notes)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, fmt.Errorf("%w: no %s run", ErrNotFound, kind)
	}
	if err != nil {
		return Run{}, fmt.Errorf("store: last %s run: %w", kind, err)
	}
	if finished.Valid {
		t := finished.Time
		r.FinishedAt = &t
	}
	r.Notes = notes.String
	return r, nil
}
