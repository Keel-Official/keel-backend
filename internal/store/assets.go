// The assets table: the demonstration set, one row per scanned pair.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// Asset is one row of the assets table.
type Asset struct {
	ID            int
	Base          domain.Asset
	Quote         domain.Asset
	Active        bool
	SelectionNote string
}

// UpsertAsset inserts a pair if it is absent and returns its id either way.
//
// This is the ONE upsert in the package, and it is allowed for a reason that
// does not apply to metrics: an assets row is a membership statement about the
// demonstration set, not a measurement, so re-declaring it is not rewriting
// history. It updates only selection_note and active, and it will not overwrite
// a note that exists with an empty one, because losing the recorded reason an
// asset is in the set is a real loss and a re-run with no note is the most
// likely way it would happen.
func (s *Store) UpsertAsset(ctx context.Context, base, quote domain.Asset, note string) (int, error) {
	if err := validAsset(base, "base"); err != nil {
		return 0, err
	}
	if err := validAsset(quote, "quote"); err != nil {
		return 0, err
	}
	if base.Equal(quote) {
		return 0, fmt.Errorf("store: base and quote are the same asset: %s", base)
	}

	var id int
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO assets (code, issuer, type, quote_code, quote_issuer, quote_type, selection_note)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))
		ON CONFLICT (code, issuer, quote_code, quote_issuer) DO UPDATE
		   SET selection_note = COALESCE(NULLIF($7, ''), assets.selection_note),
		       active         = TRUE
		RETURNING id`,
		base.Code, nullIssuer(base), string(base.Type),
		quote.Code, nullIssuer(quote), string(quote.Type),
		note,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert asset %s/%s: %w", base, quote, err)
	}
	return id, nil
}

// AssetID finds a pair. It returns ErrNotFound rather than a zero id, because a
// zero id would be used as a foreign key by a caller that forgot to check.
func (s *Store) AssetID(ctx context.Context, base, quote domain.Asset) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM assets
		 WHERE code = $1 AND issuer IS NOT DISTINCT FROM $2
		   AND quote_code = $3 AND quote_issuer IS NOT DISTINCT FROM $4`,
		base.Code, nullIssuer(base), quote.Code, nullIssuer(quote),
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: asset %s/%s", ErrNotFound, base, quote)
	}
	if err != nil {
		return 0, fmt.Errorf("store: asset id %s/%s: %w", base, quote, err)
	}
	return id, nil
}

// Assets lists the demonstration set. activeOnly is what a scan passes; the API
// listing wants everything, so that a deactivated asset reads as deactivated
// rather than as absent.
//
// The ordering is explicit and total. Non-negotiable rule 2 is about sorting map
// keys before iteration, and the reason behind it is reproducibility, which an
// unordered query result breaks just as effectively.
func (s *Store) Assets(ctx context.Context, activeOnly bool) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, code, issuer, type, quote_code, quote_issuer, quote_type,
		       active, COALESCE(selection_note, '')
		  FROM assets
		 WHERE ($1 = FALSE OR active = TRUE)
		 ORDER BY code, issuer NULLS FIRST, quote_code, quote_issuer NULLS FIRST`, activeOnly)
	if err != nil {
		return nil, fmt.Errorf("store: list assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list assets: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAssetActive deactivates or reactivates a pair. A pair is never deleted:
// metrics reference it, and a delisted asset whose history vanishes takes the
// evidence that it was ever measured with it.
func (s *Store) SetAssetActive(ctx context.Context, id int, active bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE assets SET active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return fmt.Errorf("store: set asset %d active: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: asset id %d", ErrNotFound, id)
	}
	return nil
}

// nullIssuer maps the native asset's empty issuer to SQL NULL, which is what the
// assets_native_has_no_issuer constraint requires. Writing an empty string
// instead would violate it, and the constraint is what stops a native asset from
// being stored twice under two spellings.
func nullIssuer(a domain.Asset) any {
	if a.Issuer == "" {
		return nil
	}
	return a.Issuer
}

func validAsset(a domain.Asset, side string) error {
	switch a.Type {
	case domain.AssetTypeNative:
		if a.Issuer != "" {
			return fmt.Errorf("store: %s is native and carries an issuer", side)
		}
		// The assets table has code NOT NULL, so the native asset needs a code.
		// "XLM" is what domain.Asset.String() calls it and what the contract
		// uses, so it is what goes in the column.
		if a.Code == "" {
			return fmt.Errorf("store: %s is native and carries no code; use XLM", side)
		}
	case domain.AssetTypeAlphanum4, domain.AssetTypeAlphanum12:
		if a.Code == "" || a.Issuer == "" {
			return fmt.Errorf("store: %s needs both a code and an issuer", side)
		}
	default:
		// Never inferred from the length of the code. A five character code read
		// as alphanum4 returns an empty book from Horizon with no error, and the
		// CHECK on this column is the second line of defense.
		return fmt.Errorf("store: %s has asset type %q, which is not one of the three", side, a.Type)
	}
	return nil
}

// PairsForAsset returns every pair in the demonstration set whose BASE is this
// asset, ignoring the asset type.
//
// The type is ignored on purpose, and it is the one place in this repository that
// is right to do so. The API's `assetId` parameter is `CODE:ISSUER` with no type
// in it, so a request cannot state one. Resolving the identity by lookup is the
// alternative to inferring the type from the code length, which is the trap
// recorded on domain.Asset and in two decision records. The stored row is the
// authority on the type, and if two rows share a code and issuer with different
// types, both come back and the caller has to say which it meant.
func (s *Store) PairsForAsset(ctx context.Context, code, issuer string) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, code, issuer, type, quote_code, quote_issuer, quote_type,
		       active, COALESCE(selection_note, '')
		  FROM assets
		 WHERE code = $1 AND issuer IS NOT DISTINCT FROM $2
		 ORDER BY quote_code, quote_issuer NULLS FIRST`,
		code, nullString(issuer))
	if err != nil {
		return nil, fmt.Errorf("store: pairs for %s: %w", code, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func scanAsset(sc scanner) (Asset, error) {
	var (
		a                   Asset
		issuer, quoteIssuer sql.NullString
		typ, quoteType      string
	)
	if err := sc.Scan(&a.ID, &a.Base.Code, &issuer, &typ,
		&a.Quote.Code, &quoteIssuer, &quoteType, &a.Active, &a.SelectionNote); err != nil {
		return Asset{}, err
	}
	a.Base.Issuer = issuer.String
	a.Base.Type = domain.AssetType(typ)
	a.Quote.Issuer = quoteIssuer.String
	a.Quote.Type = domain.AssetType(quoteType)
	return a, nil
}
