// The holder_readings table: one trustline pull per asset per snapshot.
//
// It is a CACHE in the sense that `scan` reads it rather than pulling its own,
// and it is EVIDENCE in the sense that nothing in it is ever recomputed or
// overwritten. Both matter. migrations/0006_holder_readings.sql carries the
// arithmetic for why the pull cannot happen inside a scan round; this file only
// stores what that pull found.
//
// TWO DECISIONS HERE ARE WORTH STATING because they are the ones a reader would
// otherwise have to infer from the SQL.
//
//  1. A TRUNCATED READING IS STORED AND CARRIES NO FIGURES. The schema enforces
//     it with a CHECK, and this file refuses it before the database has to,
//     because an error naming the field is more useful than a constraint
//     violation naming the constraint. domain.ErrHolderSetTruncated is the same
//     statement in the domain: a trustline set that hit the page cap answers a
//     concentration question not at all, so absent is the only available value
//     and zero would be a measurement nobody made.
//
//  2. THIS PACKAGE STILL HAS NO CLOCK. LatestHolderReading returns the newest
//     row and its FetchedAt, and the caller decides whether that is too old.
//     StartRun's comment gives the reason and it applies unchanged: the moment a
//     thing happened belongs to whoever observed it. How stale a reading may be
//     before `scan` refuses it is DEC-018's question and is answered in Go.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// SnapshotBasis records WHICH definition produced a row's SnapshotLedger.
//
// It exists because the two are not the same number and the difference is not
// visible in the value. DEC-011 defines the snapshot ledger as the minimum
// latest_ledger across ALL pages of a pull; internal/horizon/holders.go does not
// implement that yet and keeps only the first and last page's header, so today's
// rows carry an approximation. Recording which one produced a row is what lets
// the two generations be told apart in a query rather than by the date on the
// row.
type SnapshotBasis string

// The two bases. BasisAllPagesMin is DEC-011's definition; BasisFirstAndLastPage
// is what is available until its implementation lands.
const (
	BasisFirstAndLastPage SnapshotBasis = "first-and-last-page"
	BasisAllPagesMin      SnapshotBasis = "all-pages-min"
)

// SnapshotLabel is DEC-011's atomic or mixed verdict, plus the honest third
// value for a pull that could not be judged.
//
// LabelUnknown is not a synonym for atomic. DEC-011 derives the label from a
// count of rows whose last_modified_ledger exceeds the snapshot ledger, and
// nothing counts them yet because horizon.Holder does not carry that field. A
// pull that was never checked for mid-pull mutation must not claim the value a
// reader trusts most.
type SnapshotLabel string

// The three labels.
const (
	LabelAtomic  SnapshotLabel = "atomic"
	LabelMixed   SnapshotLabel = "mixed"
	LabelUnknown SnapshotLabel = "unknown"
)

// HolderExclusionsJSON is the shape of the exclusions_applied column.
//
// A storage format, declared here for the reason jsonb.go gives about all of
// them: these names outlive any Go rename. It is stored per row because
// domain.HolderExclusions is asset-specific and can change, and a percentage
// taken over one population must never be compared with one taken over another
// without that being visible.
type HolderExclusionsJSON struct {
	Issuer    string   `json:"issuer"`
	Addresses []string `json:"addresses,omitempty"`
}

// HolderReading is one pull, its evidence, and the concentration computed from
// it. Every figure is a pointer because every one of them is absent for a
// truncated pull, and absent is a different answer from zero.
type HolderReading struct {
	ID int64

	// Asset, and NOT an assets.id. That table is one row per PAIR, and holder
	// concentration is a property of one asset regardless of what it is quoted
	// against. 0006's table comment argues it at length. The native asset cannot
	// appear here: it has no trustlines to enumerate.
	Asset domain.Asset

	// RunID is the holders run that produced it, nil for a reading taken by
	// hand. See the column comment in 0006.
	RunID *int64

	FetchedAt time.Time

	SnapshotLedger uint32
	SnapshotBasis  SnapshotBasis
	LedgerSpan     int
	SnapshotLabel  SnapshotLabel
	MutatedRows    *int
	MutatedBalance *decimal.Decimal

	// HoldersRead is len(Holders); HolderCountReported is Horizon's own figure.
	// Their disagreement is the truncation.
	HoldersRead         int
	HolderCountReported int
	Truncated           bool

	Population         *int
	CirculatingSupply  *decimal.Decimal
	Top1Pct            *decimal.Decimal
	Top10Pct           *decimal.Decimal
	HHI                *decimal.Decimal
	ZeroBalanceDropped *int
	ExcludedDropped    *int

	Exclusions HolderExclusionsJSON

	MethodologyVersion string
}

// Age is how old the reading is against the caller's clock. It takes `now`
// rather than reading one, which is decision 2 in this file's header.
func (r HolderReading) Age(now time.Time) time.Duration { return now.Sub(r.FetchedAt) }

// SaveHolderReading writes one pull. inserted is false when a row for this
// (asset, snapshot ledger, methodology version) already existed, in which case
// NOTHING was written.
//
// Same contract as SaveMetrics and for the same reason: re-running a pull over a
// snapshot already recorded is normal and must not be an error, while a result
// that DIFFERS from the stored one is a finding that a silent overwrite would
// destroy.
func (s *Store) SaveHolderReading(ctx context.Context, r HolderReading) (id int64, inserted bool, err error) {
	if err := validHolderReading(r); err != nil {
		return 0, false, err
	}

	excl, err := json.Marshal(r.Exclusions)
	if err != nil {
		return 0, false, fmt.Errorf("store: encoding exclusions_applied: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO holder_readings (
			code, issuer, type, run_id, fetched_at,
			snapshot_ledger, snapshot_ledger_basis, ledger_span,
			snapshot_label, mutated_rows, mutated_balance,
			holders_read, holder_count_reported, truncated,
			population, circulating_supply, top1_pct, top10_pct, hhi,
			zero_balance_dropped, excluded_dropped,
			exclusions_applied, methodology_version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
		        $14, $15, $16, $17, $18, $19, $20, $21, $22, $23)
		ON CONFLICT (code, issuer, snapshot_ledger, methodology_version) DO NOTHING
		RETURNING id`,
		r.Asset.Code, r.Asset.Issuer, string(r.Asset.Type), r.RunID, r.FetchedAt.UTC(),
		int64(r.SnapshotLedger), string(r.SnapshotBasis), r.LedgerSpan,
		string(r.SnapshotLabel), nullInt(r.MutatedRows), numeric(r.MutatedBalance),
		r.HoldersRead, r.HolderCountReported, r.Truncated,
		nullInt(r.Population), numeric(r.CirculatingSupply),
		numeric(r.Top1Pct), numeric(r.Top10Pct), numeric(r.HHI),
		nullInt(r.ZeroBalanceDropped), nullInt(r.ExcludedDropped),
		excl, r.MethodologyVersion,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		// DO NOTHING fired. The row is already there and was not touched.
		existing, findErr := s.HolderReadingAt(ctx, r.Asset, r.SnapshotLedger, r.MethodologyVersion)
		if findErr != nil {
			return 0, false, findErr
		}
		return existing.ID, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: save holder reading for %s: %w", r.Asset, err)
	}
	return id, true, nil
}

// LatestHolderReading returns the newest reading for one asset, truncated or
// not.
//
// A TRUNCATED READING IS NOT SKIPPED HERE, and that is deliberate. Returning the
// most recent usable row instead would hand the caller a figure from three days
// ago while a fresh pull sits unread above it, and the caller would have no way
// to tell. The newest reading is the answer to "what do we know about this
// asset now", and when the answer is "the set was truncated, so nothing", that
// is the answer.
func (s *Store) LatestHolderReading(ctx context.Context, a domain.Asset, methodologyVersion string) (HolderReading, error) {
	if methodologyVersion == "" {
		return HolderReading{}, errors.New("store: latest holder reading: methodology version is empty")
	}
	if a.IsNative() {
		return HolderReading{}, fmt.Errorf("%w: the native asset has no trustlines, so no holder reading exists", ErrNotFound)
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+holderColumns+`
		  FROM holder_readings
		 WHERE code = $1 AND issuer = $2 AND methodology_version = $3
		 ORDER BY fetched_at DESC, id DESC
		 LIMIT 1`, a.Code, a.Issuer, methodologyVersion)

	r, err := scanHolderReading(row)
	if errors.Is(err, sql.ErrNoRows) {
		return HolderReading{}, fmt.Errorf("%w: holder reading for %s at %s", ErrNotFound, a, methodologyVersion)
	}
	if err != nil {
		return HolderReading{}, fmt.Errorf("store: latest holder reading for %s: %w", a, err)
	}
	return r, nil
}

// HolderReadingAt reads the reading stored for one asset at one snapshot.
//
// Exported because a caller whose write was refused by ON CONFLICT DO NOTHING
// needs to see WHAT it collided with. Decision 2 in store.go says a result that
// differs from the stored one is a finding rather than an overwrite, and a
// finding nobody can read is not one.
func (s *Store) HolderReadingAt(ctx context.Context, a domain.Asset, ledger uint32, methodologyVersion string) (HolderReading, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+holderColumns+`
		  FROM holder_readings
		 WHERE code = $1 AND issuer = $2 AND snapshot_ledger = $3 AND methodology_version = $4`,
		a.Code, a.Issuer, int64(ledger), methodologyVersion)

	r, err := scanHolderReading(row)
	if errors.Is(err, sql.ErrNoRows) {
		return HolderReading{}, fmt.Errorf("%w: holder reading for %s at ledger %d", ErrNotFound, a, ledger)
	}
	if err != nil {
		return HolderReading{}, fmt.Errorf("store: holder reading for %s at ledger %d: %w", a, ledger, err)
	}
	return r, nil
}

const holderColumns = `
	id, code, issuer, type, run_id, fetched_at,
	snapshot_ledger, snapshot_ledger_basis, ledger_span,
	snapshot_label, mutated_rows, mutated_balance::text,
	holders_read, holder_count_reported, truncated,
	population, circulating_supply::text, top1_pct::text, top10_pct::text, hhi::text,
	zero_balance_dropped, excluded_dropped,
	exclusions_applied, methodology_version`

func scanHolderReading(sc scanner) (HolderReading, error) {
	var (
		r                                        HolderReading
		runID                                    sql.NullInt64
		ledger                                   int64
		basis, label, assetType                  string
		mutatedRows                              sql.NullInt64
		mutatedBalance                           sql.NullString
		population, zeroDropped, excludedDropped sql.NullInt64
		supply, top1, top10, hhi                 sql.NullString
		exclusions                               []byte
		err                                      error
	)

	if err = sc.Scan(
		&r.ID, &r.Asset.Code, &r.Asset.Issuer, &assetType, &runID, &r.FetchedAt,
		&ledger, &basis, &r.LedgerSpan,
		&label, &mutatedRows, &mutatedBalance,
		&r.HoldersRead, &r.HolderCountReported, &r.Truncated,
		&population, &supply, &top1, &top10, &hhi,
		&zeroDropped, &excludedDropped,
		&exclusions, &r.MethodologyVersion,
	); err != nil {
		return HolderReading{}, err
	}

	r.Asset.Type = domain.AssetType(assetType)
	if runID.Valid {
		v := runID.Int64
		r.RunID = &v
	}
	r.SnapshotLedger = uint32(ledger)
	r.SnapshotBasis = SnapshotBasis(basis)
	r.SnapshotLabel = SnapshotLabel(label)
	r.MutatedRows = readInt(mutatedRows)
	r.Population = readInt(population)
	r.ZeroBalanceDropped = readInt(zeroDropped)
	r.ExcludedDropped = readInt(excludedDropped)

	for _, f := range []struct {
		src sql.NullString
		dst **decimal.Decimal
		col string
	}{
		{mutatedBalance, &r.MutatedBalance, "mutated_balance"},
		{supply, &r.CirculatingSupply, "circulating_supply"},
		{top1, &r.Top1Pct, "top1_pct"},
		{top10, &r.Top10Pct, "top10_pct"},
		{hhi, &r.HHI, "hhi"},
	} {
		if *f.dst, err = readNumeric(f.src, f.col); err != nil {
			return HolderReading{}, err
		}
	}

	if len(exclusions) > 0 {
		if err := json.Unmarshal(exclusions, &r.Exclusions); err != nil {
			return HolderReading{}, fmt.Errorf("store: column exclusions_applied: %w", err)
		}
	}
	return r, nil
}

// validHolderReading refuses what the schema would refuse, and names the field
// while doing it.
func validHolderReading(r HolderReading) error {
	switch {
	case r.Asset.Code == "":
		return errors.New("store: holder reading: the asset has no code")
	case r.Asset.IsNative():
		return errors.New("store: holder reading: the native asset has no trustlines to enumerate")
	case r.Asset.Issuer == "":
		return fmt.Errorf("store: holder reading: %s has no issuer", r.Asset.Code)
	}
	if r.FetchedAt.IsZero() {
		return errors.New("store: holder reading: FetchedAt is zero")
	}
	if r.MethodologyVersion == "" {
		return errors.New("store: holder reading: MethodologyVersion is empty")
	}
	switch r.SnapshotBasis {
	case BasisFirstAndLastPage, BasisAllPagesMin:
	default:
		return fmt.Errorf("store: holder reading: snapshot basis %q is not one of %q or %q",
			r.SnapshotBasis, BasisFirstAndLastPage, BasisAllPagesMin)
	}
	switch r.SnapshotLabel {
	case LabelAtomic, LabelMixed, LabelUnknown:
	default:
		return fmt.Errorf("store: holder reading: snapshot label %q is not atomic, mixed or unknown", r.SnapshotLabel)
	}
	if r.LedgerSpan < 0 {
		return fmt.Errorf("store: holder reading: ledger span %d is negative", r.LedgerSpan)
	}
	// The rule the whole table exists to keep. See decision 1 in the header.
	if r.Truncated {
		switch {
		case r.Top1Pct != nil, r.Top10Pct != nil, r.HHI != nil,
			r.Population != nil, r.CirculatingSupply != nil:
			return errors.New("store: holder reading: the pull was truncated, so it carries no concentration figures; " +
				"zero and absent are different values and only absent is available here")
		}
	}
	return nil
}

// nullInt renders an optional int for a nullable INT parameter, nil as SQL NULL.
func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// readInt parses a nullable INT. NULL comes back as a nil pointer.
func readInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}
