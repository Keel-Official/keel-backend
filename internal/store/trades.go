// Storing and reading the trade half of the supporting metrics.
//
// This is holders.go's shape applied to the second expensive pull, and the
// duplication is deliberate rather than a missed abstraction: the two rows share
// a CADENCE argument and nothing else. A holder reading is keyed by asset because
// a trustline set belongs to one asset; a trade reading is keyed by the PAIR,
// because a trade is an event between two of them. A generic "reading" carrying
// both keys would have to permit the state where neither is set.
//
// See migrations/0009_trade_readings.sql for why the row holds figures rather
// than trades, and DEC-019 for why the pull cannot live inside a scan round.

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/shopspring/decimal"
)

// TradeScope says which half of DEC-019 the pull behind a row performed.
type TradeScope string

const (
	// ScopeLastGenuineOnly is the cheap half: walk back until a whole UTC day
	// holds a genuine trade, then stop. It answers FR-10 and the two
	// NO_GENUINE_TRADE flags and nothing else.
	ScopeLastGenuineOnly TradeScope = "last-genuine-only"

	// ScopeFullWindow is the cheap half plus the expensive one: thirty whole
	// days classified, which answers FR-9 and WASH_TRADE_SUSPECTED as well. Only
	// pairs under the DEC-019 threshold are walked this way.
	ScopeFullWindow TradeScope = "full-window"
)

// TradeReading is one backward walk over /trades for one pair, reduced to the
// figures it produced.
type TradeReading struct {
	ID int64

	// AssetID is the assets row, which names a PAIR. See the file header.
	AssetID int

	// RunID is the run that produced it, nil for a reading taken by hand.
	RunID *int64

	FetchedAt          time.Time
	MethodologyVersion string

	// Anchor is 00:00:00Z of the day the pull ran, and every window below is
	// measured back from it. It is not FetchedAt: DEC-019 section 8.1 refuses a
	// partial day, so the newest data any of these figures can describe is the
	// end of the last COMPLETE day.
	Anchor time.Time

	// LedgerSeq is the ledger of the newest trade in the walk, ZERO when the walk
	// saw no trades at all. It is not a Latest-Ledger header: public Horizon does
	// not send one on /trades. See the column comment in 0009 and the field of
	// the same name on horizon.TradeDayWalk.
	LedgerSeq uint32

	DaysWalked int
	Pages      int

	// Exhausted is the pair's whole history having been seen. BoundReached is the
	// walk having run out of days. They are stored separately because "this pair
	// has never had a genuine trade" and "we did not look far enough" are
	// opposite answers, and DEC-019 section 8.4 item 2 refuses to merge them.
	Exhausted    bool
	BoundReached bool

	Scope TradeScope

	// VolumeUnevaluatedReason is one phrase saying why the FR-9 figures are
	// absent, set whenever they are. DEC-019 section 5 asks for it by name.
	VolumeUnevaluatedReason string

	// LastGenuine is the newest genuine trade the walk found, nil when it found
	// none. Nil with Exhausted true is a measurement; nil with BoundReached true
	// is not.
	LastGenuine *domain.TradeRef

	// Genuine BASE volume per window, nil outside ScopeFullWindow. The ratio is
	// not stored; see the column comment in 0009.
	GenuineBaseD1  *decimal.Decimal
	GenuineBaseD7  *decimal.Decimal
	GenuineBaseD30 *decimal.Decimal

	// Genuine QUOTE volume inside the oracle window, and how many trades were
	// recorded there. A window with recorded trades and no genuine volume is an
	// active market full of fake prints; a window with no trades is a silent one.
	GenuineQuoteOracleWindow *decimal.Decimal
	OracleWindowRecorded     *int

	TradesExcludedPct *decimal.Decimal
}

// Age is how old the reading is against the caller's clock, measured from the
// anchor rather than from FetchedAt.
//
// THE ANCHOR IS THE HONEST CLOCK HERE and FetchedAt is not. A pull that ran at
// 23:50 and one that ran at 00:10 the next morning describe the same last
// complete day if they share an anchor, and a staleness bound built on FetchedAt
// would call one of them twenty minutes old and the other twenty minutes old
// while they differ by a day of coverage.
func (r TradeReading) Age(now time.Time) time.Duration { return now.Sub(r.Anchor) }

// Covered reports whether the figures describe the whole of each FR-9 window,
// which is what domain.SupportingInput.TradesCover asks and what separates a
// measured zero from an absence of data.
func (r TradeReading) Covered() bool { return r.Scope == ScopeFullWindow }

// SaveTradeReading writes one pull. It always appends, and inserted is always
// true.
//
// THIS DIVERGES FROM SaveMetrics AND SaveHolderReading, WHICH BOTH REFUSE A
// SECOND ROW FOR ONE KEY, and migration 0010 is where the reason is argued. A
// second metrics row for one ledger is the same measurement taken twice; a
// second trade reading for one day can be DEEPER, because this walk is bounded
// in pages as well as in days and a larger bound reaches further back. HU/USDC
// reached 11 days under a 400 page bound and 20 under 1,500 on 18 September
// 2026, and the refusal threw the better one away.
//
// The no-overwrite rule is untouched: nothing here replaces a row. Both
// readings survive with the bounds they ran under, and LatestTradeReading
// decides which one answers.
func (s *Store) SaveTradeReading(ctx context.Context, r TradeReading) (id int64, inserted bool, err error) {
	if err := validTradeReading(r); err != nil {
		return 0, false, err
	}

	var lastAt any
	var lastLedger any
	if r.LastGenuine != nil {
		lastAt = r.LastGenuine.At.UTC()
		lastLedger = int64(r.LastGenuine.LedgerSeq)
	}
	// Zero is written as NULL rather than as 0. The column's CHECK refuses a
	// non-positive sequence, and a stored 0 would in any case read as a ledger
	// that does not exist rather than as a walk with no trade to cite.
	var ledger any
	if r.LedgerSeq > 0 {
		ledger = int64(r.LedgerSeq)
	}

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO trade_readings (
			asset_id, run_id, fetched_at, methodology_version,
			anchor, ledger_seq, days_walked, pages, exhausted, bound_reached,
			scope, volume_unevaluated_reason,
			last_genuine_at, last_genuine_ledger,
			genuine_base_d1, genuine_base_d7, genuine_base_d30,
			genuine_quote_oracle_window, oracle_window_recorded,
			trades_excluded_pct)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
		        $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		RETURNING id`,
		r.AssetID, r.RunID, r.FetchedAt.UTC(), r.MethodologyVersion,
		r.Anchor.UTC(), ledger, r.DaysWalked, r.Pages, r.Exhausted, r.BoundReached,
		string(r.Scope), nullString(r.VolumeUnevaluatedReason),
		lastAt, lastLedger,
		numeric(r.GenuineBaseD1), numeric(r.GenuineBaseD7), numeric(r.GenuineBaseD30),
		numeric(r.GenuineQuoteOracleWindow), nullInt(r.OracleWindowRecorded),
		numeric(r.TradesExcludedPct),
	).Scan(&id)

	if err != nil {
		return 0, false, fmt.Errorf("store: save trade reading for asset %d: %w", r.AssetID, err)
	}
	return id, true, nil
}

// LatestTradeReading returns the DEEPEST reading for the newest day a pair has
// one, which is not always the newest row.
//
// Depth breaks the tie rather than insertion order, and migration 0010 argues
// why: a day can hold several readings once a walk may be re-run with a larger
// page bound, and depth is monotone for this walk, so a deeper reading is never
// a worse answer. Taking the newest row instead would let a shallower re-walk,
// a pass run with a smaller bound by mistake, quietly undo a deeper one.
//
// A reading whose volume half is unevaluated is NOT skipped, for the reason
// LatestHolderReading gives about truncated pulls: it is the answer to "what do
// we know now", and when the answer is "this pair is too busy to classify
// inside the budget", that is the answer.
func (s *Store) LatestTradeReading(ctx context.Context, assetID int, methodologyVersion string) (TradeReading, error) {
	if methodologyVersion == "" {
		return TradeReading{}, errors.New("store: latest trade reading: methodology version is empty")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+tradeColumns+`
		  FROM trade_readings
		 WHERE asset_id = $1 AND methodology_version = $2
		 ORDER BY anchor DESC, days_walked DESC, id DESC
		 LIMIT 1`, assetID, methodologyVersion)

	r, err := scanTradeReading(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TradeReading{}, fmt.Errorf("%w: trade reading for asset %d at %s", ErrNotFound, assetID, methodologyVersion)
	}
	if err != nil {
		return TradeReading{}, fmt.Errorf("store: latest trade reading for asset %d: %w", assetID, err)
	}
	return r, nil
}

// TradeReadingAt reads the deepest row stored for one pair on one day.
//
// Exported so a caller can ask whether a day has been read at all, which is what
// makes `keel trades` resumable after a spent request budget. Since 0010 a day
// may hold several readings, so this answers with the one LatestTradeReading
// would choose rather than with an arbitrary member of the set.
func (s *Store) TradeReadingAt(ctx context.Context, assetID int, anchor time.Time, methodologyVersion string) (TradeReading, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+tradeColumns+`
		  FROM trade_readings
		 WHERE asset_id = $1 AND anchor = $2 AND methodology_version = $3
		 ORDER BY days_walked DESC, id DESC
		 LIMIT 1`, assetID, anchor.UTC(), methodologyVersion)

	r, err := scanTradeReading(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TradeReading{}, fmt.Errorf("%w: trade reading for asset %d on %s", ErrNotFound, assetID, anchor.Format(time.RFC3339))
	}
	if err != nil {
		return TradeReading{}, fmt.Errorf("store: trade reading for asset %d on %s: %w", assetID, anchor.Format(time.RFC3339), err)
	}
	return r, nil
}

const tradeColumns = `
	id, asset_id, run_id, fetched_at, methodology_version,
	anchor, ledger_seq, days_walked, pages, exhausted, bound_reached,
	scope, volume_unevaluated_reason,
	last_genuine_at, last_genuine_ledger,
	genuine_base_d1::text, genuine_base_d7::text, genuine_base_d30::text,
	genuine_quote_oracle_window::text, oracle_window_recorded,
	trades_excluded_pct::text`

func scanTradeReading(sc scanner) (TradeReading, error) {
	var (
		r           TradeReading
		runID       sql.NullInt64
		ledger      sql.NullInt64
		scope       string
		reason      sql.NullString
		lastAt      sql.NullTime
		lastLedger  sql.NullInt64
		recorded    sql.NullInt64
		d1, d7, d30 sql.NullString
		quoteWin    sql.NullString
		excludedPct sql.NullString
		err         error
	)

	if err = sc.Scan(
		&r.ID, &r.AssetID, &runID, &r.FetchedAt, &r.MethodologyVersion,
		&r.Anchor, &ledger, &r.DaysWalked, &r.Pages, &r.Exhausted, &r.BoundReached,
		&scope, &reason,
		&lastAt, &lastLedger,
		&d1, &d7, &d30,
		&quoteWin, &recorded,
		&excludedPct,
	); err != nil {
		return TradeReading{}, err
	}

	if runID.Valid {
		v := runID.Int64
		r.RunID = &v
	}
	if ledger.Valid {
		r.LedgerSeq = uint32(ledger.Int64)
	}
	r.Scope = TradeScope(scope)
	r.VolumeUnevaluatedReason = reason.String
	r.OracleWindowRecorded = readInt(recorded)

	// The schema refuses half a reference, so testing one half is enough and
	// testing both would suggest the other state is reachable.
	if lastAt.Valid {
		r.LastGenuine = &domain.TradeRef{
			LedgerSeq: uint32(lastLedger.Int64),
			At:        lastAt.Time.UTC(),
		}
	}

	for _, f := range []struct {
		src sql.NullString
		dst **decimal.Decimal
		col string
	}{
		{d1, &r.GenuineBaseD1, "genuine_base_d1"},
		{d7, &r.GenuineBaseD7, "genuine_base_d7"},
		{d30, &r.GenuineBaseD30, "genuine_base_d30"},
		{quoteWin, &r.GenuineQuoteOracleWindow, "genuine_quote_oracle_window"},
		{excludedPct, &r.TradesExcludedPct, "trades_excluded_pct"},
	} {
		if *f.dst, err = readNumeric(f.src, f.col); err != nil {
			return TradeReading{}, err
		}
	}
	return r, nil
}

// validTradeReading refuses what the schema would refuse, and names the field
// while doing it.
func validTradeReading(r TradeReading) error {
	switch {
	case r.AssetID <= 0:
		return errors.New("store: trade reading: AssetID is not set")
	case r.MethodologyVersion == "":
		return errors.New("store: trade reading: MethodologyVersion is empty")
	case r.FetchedAt.IsZero():
		return errors.New("store: trade reading: FetchedAt is zero")
	case r.Anchor.IsZero():
		return errors.New("store: trade reading: Anchor is zero")
	case r.LedgerSeq == 0 && r.LastGenuine != nil:
		// A walk that met a genuine trade met a trade, so it can name a ledger.
		// A walk that met none may legitimately have none to name.
		return errors.New("store: trade reading: LedgerSeq is zero although a last genuine trade is cited")
	}
	switch r.Scope {
	case ScopeLastGenuineOnly, ScopeFullWindow:
	default:
		return fmt.Errorf("store: trade reading: scope %q is not one of %q or %q",
			r.Scope, ScopeLastGenuineOnly, ScopeFullWindow)
	}
	// The cheap half carries no volume figures and must say why, which is the
	// constraint in 0009 arriving one step earlier with a better message.
	if r.Scope == ScopeLastGenuineOnly {
		switch {
		case r.GenuineBaseD1 != nil || r.GenuineBaseD7 != nil || r.GenuineBaseD30 != nil:
			return errors.New("store: trade reading: scope is last-genuine-only and it carries volume figures")
		case r.TradesExcludedPct != nil:
			return errors.New("store: trade reading: scope is last-genuine-only and it carries an exclusion share")
		case r.VolumeUnevaluatedReason == "":
			return errors.New("store: trade reading: scope is last-genuine-only and no reason is given for the missing volume")
		}
	}
	if r.LastGenuine != nil && r.LastGenuine.At.IsZero() {
		return errors.New("store: trade reading: the last genuine trade has no close time")
	}
	if r.LastGenuine != nil && r.LastGenuine.LedgerSeq == 0 {
		return errors.New("store: trade reading: the last genuine trade has no ledger")
	}
	return nil
}
