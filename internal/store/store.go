// Package store is Keel's Postgres persistence. It stores and it reads, and it
// computes nothing.
//
// GREEN ZONE, but three decisions in here are load bearing enough to state:
//
//  1. EVERY MONETARY VALUE CROSSES THIS BOUNDARY AS A STRING. A decimal is
//     rendered with String() on the way in and parsed with
//     decimal.NewFromString on the way out, and NUMERIC columns are read into
//     sql.NullString and never into a driver's numeric type. No float64 exists
//     anywhere on either path, which is non-negotiable rule 1, and the arch test
//     enforces it across the whole repository. Rejected alternative: pgx's
//     native numeric type plus github.com/jackc/pgx-shopspring-decimal, which
//     reads better and adds a second dependency whose conversion is somebody
//     else's code on the one axis this product cannot afford to be wrong about.
//
//  2. A RESULT IS NEVER OVERWRITTEN. Insert is ON CONFLICT DO NOTHING against
//     the (asset_id, ledger_seq, methodology_version, data_source) key, and
//     SaveMetrics reports whether the row was new. Rule 3 of this package's
//     brief says a result from a different methodology version is a different
//     row rather than an overwrite, and the same argument forbids overwriting a
//     row from the SAME version: a re-run that silently changed a stored number
//     would make the time series unusable as evidence. Rejected alternative: ON
//     CONFLICT DO UPDATE, which is the usual idiom and is wrong here.
//
//  3. THE JSONB COLUMNS HAVE THEIR OWN SHAPES, DECLARED IN jsonb.go. The domain
//     types are not marshaled directly. A JSON field name inside a database
//     column is a wire format that outlives any Go rename, so it is written down
//     once, explicitly, matching the names the API contract uses. Every decimal
//     inside them is a STRING even where the contract sends a number, because a
//     JSON number is an IEEE 754 double; converting to the contract's scale and
//     type is internal/api's job, and rule 3 of its brief already puts it there.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	// The driver, registered as "pgx" for database/sql. Imported for its side
	// effect only: this package holds no pgx type in any signature, so the
	// driver can be swapped without touching a caller.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// DefaultDSN points at the local docker-compose Postgres. The password is the
// development-only one already written into docker-compose.yml in this
// repository, so nothing is being leaked by naming it here; anything real reads
// the DSN from its environment.
const DefaultDSN = "postgres://keel:keel_dev_only@localhost:5432/keel?sslmode=disable"

// ErrNotFound is returned by every read that asks for one row and finds none. It
// is distinct from a zero value on purpose: a caller has to be able to tell "no
// metrics for this asset yet" from "metrics whose every field is zero", and the
// second one is a legitimate and very interesting state for a dead asset.
var ErrNotFound = errors.New("store: not found")

// dbtx is the subset of *sql.DB and *sql.Tx this package uses.
//
// It exists so a test can hand the Store a transaction and roll it back, which
// is what keeps the integration tests from leaving rows behind in a database
// somebody else is also using. It is unexported: a caller gets a Store from
// Open and nothing else.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is Keel's Postgres persistence. It stores and reads and computes
// nothing, which is the rule for this package: a figure that arrives here has
// already been decided somewhere a reviewer can find.
type Store struct {
	db      dbtx
	closeFn func() error
}

// Open connects and verifies the connection before returning. A Store that
// cannot reach its database is not a Store, and finding that out on the first
// query instead of here is how a scan gets halfway through before failing.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		dsn = DefaultDSN
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// A scan walks assets one at a time and the API is read-only, so a large
	// pool buys nothing and a bounded one keeps a runaway loop from exhausting
	// Postgres's connection slots.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{db: db, closeFn: db.Close}, nil
}

// Close releases the connection pool. It is safe on a Store built around an
// existing transaction, where there is no pool to release and closeFn is nil.
func (s *Store) Close() error {
	if s.closeFn == nil {
		return nil
	}
	return s.closeFn()
}

// SchemaVersion returns the migrations that have been applied, newest first. It
// is how a caller can refuse to write against a schema older than the code
// expects, instead of failing on a missing column halfway through a scan.
//
// The column is `version` and it holds a FILENAME, which scripts/migrate.sh
// creates and is the authority on. This function was written against a guessed
// column name first and the test caught it, which is the argument for the
// integration tests needing a real Postgres rather than a fake.
func (s *Store) SchemaVersion(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT version FROM schema_migrations ORDER BY version DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: schema version: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- decimals

// numeric renders an optional decimal for a NUMERIC parameter. A nil decimal
// becomes SQL NULL and NOT zero. Every nullable column in this schema documents
// that distinction, because nil means unknown or not applicable while zero is a
// measurement, and for this product the difference is the whole point: a
// manipulation cost of zero says the attack is free.
func numeric(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return d.String()
}

// readNumeric parses a nullable NUMERIC. NULL comes back as a nil pointer.
func readNumeric(v sql.NullString, column string) (*decimal.Decimal, error) {
	if !v.Valid {
		return nil, nil
	}
	d, err := decimal.NewFromString(v.String)
	if err != nil {
		return nil, fmt.Errorf("store: column %s: %q: %w", column, v.String, err)
	}
	return &d, nil
}
