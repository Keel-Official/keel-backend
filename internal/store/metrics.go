// The metrics table: one computed result per asset per ledger per methodology
// version per source.
//
// ONE FIELD OF domain.AssetRisk IS NOT PERSISTED, and that is deliberate rather
// than an oversight. Supporting.GenuineVolumeInWindow has no column, and it has
// no field in the API contract either; the only place it exists is
// internal/domain/types.go. Its definition is an empty row in the table at the
// end of docs/methodology/07-supporting-metrics.md, which is a worksheet that
// says of itself that no definitions are recorded in it yet.
//
// Storing it now would put a number in the database whose definition nobody has
// written, and 0001_core.sql already argued that case in the other direction:
// "adding columns now for values nothing produces would be clutter that reads
// like a promise". The quantity is also already stored, inside
// oracle_resistance.genuineVolume, which is the same measurement over the same
// window, so a column would be a second home for one fact.
//
// TestGenuineVolumeInWindowIsNotPersisted asserts the gap rather than leaving it
// to be discovered, and handoff item 17 is where the decision sits: give the
// field a definition, a column and a contract field, or drop it from the type
// because the oracle object already carries it.

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// Metric is one metrics row: the result, plus the two things the row carries
// that the result type does not.
type Metric struct {
	ID         int64
	AssetID    int
	ComputedAt time.Time
	Risk       domain.AssetRisk
}

// SaveMetrics writes one result. inserted is false when a row for this
// (asset, ledger, methodology version, source) already existed, in which case
// NOTHING was written and the stored row is left exactly as it was.
//
// That is decision 2 in store.go and it is the reason this returns a bool rather
// than swallowing the case: a scan re-run over a ledger it has already seen is
// normal and must not be an error, while a result that DIFFERS from the stored
// one is a finding, and a caller cannot notice either if the write silently
// overwrites.
func (s *Store) SaveMetrics(ctx context.Context, assetID int, computedAt time.Time, risk domain.AssetRisk) (id int64, inserted bool, err error) {
	if err := validRisk(risk); err != nil {
		return 0, false, err
	}

	depth, err := encodeDepth(risk.Depth)
	if err != nil {
		return 0, false, err
	}
	combined, err := encodeManipulation(risk.ManipulationCostCombined)
	if err != nil {
		return 0, false, err
	}
	// Nullable, unlike the combined ladder: 0003 added this column without a NOT
	// NULL, so a row written before the venue split has no orderbook-only
	// figure. An empty array would claim it was computed and found empty.
	var orderbookOnly any
	if risk.ManipulationCostOrderbookOnly != nil {
		body, err := encodeManipulation(risk.ManipulationCostOrderbookOnly)
		if err != nil {
			return 0, false, err
		}
		orderbookOnly = string(body)
	}
	oracle, err := encodeOracleResistance(risk.OracleResistance)
	if err != nil {
		return 0, false, err
	}
	volume, err := encodeVolumeToSupply(risk.Supporting)
	if err != nil {
		return 0, false, err
	}
	trade, err := encodeLastGenuineTrade(risk.Supporting.LastGenuineTrade)
	if err != nil {
		return 0, false, err
	}

	err = s.db.QueryRowContext(ctx, `
		INSERT INTO metrics (
			asset_id, ledger_seq, ledger_closed_at, computed_at,
			methodology_version, data_source,
			mid_price, price_source, spread_pct,
			pool_spot_price, price_divergence_pct,
			depth, manipulation_cost_combined, manipulation_cost_orderbook_only,
			max_reachable_price, cost_to_max_reachable_price,
			oracle_resistance,
			max_safe_collateral, max_safe_collateral_liquidation, max_safe_collateral_manipulation,
			holder_top1_pct, holder_top10_pct, holder_hhi,
			volume_to_supply, last_genuine_trade, trades_excluded_pct,
			flags, unevaluated_flags, band, band_confidence, warnings
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7::numeric, $8, $9::numeric,
			$10::numeric, $11::numeric,
			$12::jsonb, $13::jsonb, $14::jsonb,
			$15::numeric, $16::numeric,
			$17::jsonb,
			$18::numeric, $19::numeric, $20::numeric,
			$21::numeric, $22::numeric, $23::numeric,
			$24::jsonb, $25::jsonb, $26::numeric,
			$27::text[], $28::text[], $29, $30, $31::text[]
		)
		ON CONFLICT (asset_id, ledger_seq, methodology_version, data_source) DO NOTHING
		RETURNING id`,
		assetID, int64(risk.LedgerSeq), risk.LedgerClosedAt.UTC(), computedAt.UTC(),
		risk.MethodologyVersion, string(risk.DataSource),
		numeric(risk.MidPrice), string(risk.PriceSource), numeric(risk.SpreadPct),
		numeric(risk.PoolSpotPrice), numeric(risk.PriceDivergencePct),
		string(depth), string(combined), orderbookOnly,
		numeric(risk.MaxReachablePrice), numeric(risk.CostToMaxReachablePrice),
		oracle,
		numeric(risk.MaxSafeCollateral), numeric(risk.MaxSafeCollateralLiquidation), numeric(risk.MaxSafeCollateralManipulation),
		numeric(risk.Supporting.HolderTop1Pct), numeric(risk.Supporting.HolderTop10Pct), numeric(risk.Supporting.HolderHHI),
		volume, trade, numeric(risk.Supporting.TradesExcludedPct),
		flagStrings(risk.Flags), flagStrings(risk.UnevaluatedFlags),
		string(risk.Band), string(risk.BandConfidence), stringsOrEmpty(risk.Warnings),
	).Scan(&id)

	// DO NOTHING returns no row, which arrives here as ErrNoRows. That is the
	// already-present case and not a failure.
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("store: save metrics asset %d ledger %d: %w", assetID, risk.LedgerSeq, err)
	}
	return id, true, nil
}

// metricColumns is the read side of the same list, in one place so the two
// cannot drift apart. The assets join supplies Base and Quote, which live on the
// result type but in a different table.
const metricColumns = `
	m.id, m.asset_id, m.computed_at,
	a.code, a.issuer, a.type, a.quote_code, a.quote_issuer, a.quote_type,
	m.ledger_seq, m.ledger_closed_at, m.methodology_version, m.data_source,
	m.mid_price::text, m.price_source, m.spread_pct::text,
	m.pool_spot_price::text, m.price_divergence_pct::text,
	m.depth, m.manipulation_cost_combined, m.manipulation_cost_orderbook_only,
	m.max_reachable_price::text, m.cost_to_max_reachable_price::text,
	m.oracle_resistance,
	m.max_safe_collateral::text, m.max_safe_collateral_liquidation::text,
	m.max_safe_collateral_manipulation::text,
	m.holder_top1_pct::text, m.holder_top10_pct::text, m.holder_hhi::text,
	m.volume_to_supply, m.last_genuine_trade, m.trades_excluded_pct::text,
	to_jsonb(m.flags), to_jsonb(m.unevaluated_flags), m.band, m.band_confidence,
	to_jsonb(m.warnings)`

// The three text[] columns are read as JSONB and not as arrays. Writing a
// []string into a text[] parameter works through database/sql, but scanning one
// back does not: the driver has no Scanner for a Postgres array, and
// database/sql offers no hook for one. to_jsonb converts server side, which
// costs nothing here and keeps the escaping Postgres's problem rather than this
// package's. The warnings column holds free text with commas, braces and quotes
// in it, and hand-rolled array parsing is exactly where that goes wrong.
//
// Rejected alternative: unnest into rows and a second query per metric, which
// avoids the conversion and turns one read into N+1.

// LatestMetrics returns the newest result for one asset, by ledger.
//
// It filters on methodology version because results from two versions are not
// comparable, and "latest" across a version change would silently mix them. An
// empty version means the current one, which is the only sensible default for a
// dashboard: showing a figure computed under a superseded definition without
// saying so is worse than showing nothing.
func (s *Store) LatestMetrics(ctx context.Context, assetID int, methodologyVersion string) (Metric, error) {
	if methodologyVersion == "" {
		methodologyVersion = domain.MethodologyVersion
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+metricColumns+`
		  FROM metrics m JOIN assets a ON a.id = m.asset_id
		 WHERE m.asset_id = $1 AND m.methodology_version = $2
		 ORDER BY m.ledger_seq DESC
		 LIMIT 1`, assetID, methodologyVersion)

	m, err := scanMetric(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Metric{}, fmt.Errorf("%w: metrics for asset %d at %s", ErrNotFound, assetID, methodologyVersion)
	}
	return m, err
}

// MetricsAtLedger returns one exact row. All four key parts are required,
// because the key has four parts: asking for an asset at a ledger without
// naming the version and the source is asking for up to several different rows.
func (s *Store) MetricsAtLedger(ctx context.Context, assetID int, ledgerSeq uint32, methodologyVersion string, source domain.DataSource) (Metric, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+metricColumns+`
		  FROM metrics m JOIN assets a ON a.id = m.asset_id
		 WHERE m.asset_id = $1 AND m.ledger_seq = $2
		   AND m.methodology_version = $3 AND m.data_source = $4`,
		assetID, int64(ledgerSeq), methodologyVersion, string(source))

	m, err := scanMetric(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Metric{}, fmt.Errorf("%w: metrics for asset %d at ledger %d", ErrNotFound, assetID, ledgerSeq)
	}
	return m, err
}

// MetricsHistory returns results for one asset over a ledger range, oldest
// first, which is the order a time series is read in.
//
// The range is on LEDGER and not on time, deliberately. A ledger sequence is
// what every result is keyed by and what the historical replay path addresses;
// filtering by wall clock would make the same query return different rows
// depending on when a ledger happened to close.
//
// THE SOURCE IS PART OF THE FILTER, AND IT HAS TO BE. The key has four parts and
// this read constrains three of them plus a range on the fourth, so one ledger
// yields at most one row. Until 26 August 2026 it constrained only the asset and
// the version, and a ledger holding both a horizon row and a trades-implied row
// returned both. The caller downsamples by keeping the last row in each bucket
// and 'trades-implied' sorts last of the four alphabetically, so the series
// silently showed the LOWER BOUND wherever both existed and labeled it nothing.
// That is the posted-against-executed distinction the package brief calls not
// interchangeable, collapsed by an ORDER BY.
//
// An empty source means horizon rather than every source. A caller that wants a
// different one names it; there is deliberately no way to ask for all of them,
// because a series mixing derivations is not a series.
func (s *Store) MetricsHistory(ctx context.Context, assetID int, fromLedger, toLedger uint32, methodologyVersion string, source domain.DataSource, limit int) ([]Metric, error) {
	if methodologyVersion == "" {
		methodologyVersion = domain.MethodologyVersion
	}
	if source == "" {
		source = domain.DataSourceHorizon
	}
	if !source.Valid() {
		return nil, fmt.Errorf("store: data source %q is not one of the four", source)
	}
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+metricColumns+`
		  FROM metrics m JOIN assets a ON a.id = m.asset_id
		 WHERE m.asset_id = $1 AND m.methodology_version = $2
		   AND m.data_source = $3
		   AND m.ledger_seq >= $4 AND m.ledger_seq <= $5
		 ORDER BY m.ledger_seq ASC
		 LIMIT $6`,
		assetID, methodologyVersion, string(source), int64(fromLedger), int64(toLedger), limit)
	if err != nil {
		return nil, fmt.Errorf("store: metrics history asset %d: %w", assetID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Metric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// scanner is what *sql.Row and *sql.Rows have in common, so one scan function
// serves both the single-row and the many-row reads.
type scanner interface{ Scan(dest ...any) error }

func scanMetric(sc scanner) (Metric, error) {
	var (
		m                                     Metric
		issuer, quoteIssuer                   sql.NullString
		typ, quoteType                        string
		ledgerSeq                             int64
		dataSource, priceSource               string
		midPrice, spreadPct                   sql.NullString
		poolSpot, divergence                  sql.NullString
		depthBody, combinedBody               []byte
		orderbookOnlyBody, oracleBody         []byte
		maxReachable, costToMaxReachable      sql.NullString
		cmax, cmaxLiquidation, cmaxManipulate sql.NullString
		top1, top10, hhi                      sql.NullString
		volumeBody, tradeBody                 []byte
		excludedPct                           sql.NullString
		flagsBody, unevaluatedBody            []byte
		warningsBody                          []byte
		band, bandConfidence                  string
	)

	if err := sc.Scan(
		&m.ID, &m.AssetID, &m.ComputedAt,
		&m.Risk.Base.Code, &issuer, &typ, &m.Risk.Quote.Code, &quoteIssuer, &quoteType,
		&ledgerSeq, &m.Risk.LedgerClosedAt, &m.Risk.MethodologyVersion, &dataSource,
		&midPrice, &priceSource, &spreadPct,
		&poolSpot, &divergence,
		&depthBody, &combinedBody, &orderbookOnlyBody,
		&maxReachable, &costToMaxReachable,
		&oracleBody,
		&cmax, &cmaxLiquidation, &cmaxManipulate,
		&top1, &top10, &hhi,
		&volumeBody, &tradeBody, &excludedPct,
		&flagsBody, &unevaluatedBody, &band, &bandConfidence, &warningsBody,
	); err != nil {
		return Metric{}, err
	}

	m.Risk.Base.Issuer = issuer.String
	m.Risk.Base.Type = domain.AssetType(typ)
	m.Risk.Quote.Issuer = quoteIssuer.String
	m.Risk.Quote.Type = domain.AssetType(quoteType)
	m.Risk.LedgerSeq = uint32(ledgerSeq)
	m.Risk.DataSource = domain.DataSource(dataSource)
	m.Risk.PriceSource = domain.PriceSource(priceSource)
	m.Risk.Band = domain.Band(band)
	m.Risk.BandConfidence = domain.BandConfidence(bandConfidence)
	flags, err := decodeTextArray(flagsBody, "flags")
	if err != nil {
		return Metric{}, err
	}
	unevaluated, err := decodeTextArray(unevaluatedBody, "unevaluated_flags")
	if err != nil {
		return Metric{}, err
	}
	if m.Risk.Warnings, err = decodeTextArray(warningsBody, "warnings"); err != nil {
		return Metric{}, err
	}
	m.Risk.Flags = toFlags(flags)
	m.Risk.UnevaluatedFlags = toFlags(unevaluated)

	if m.Risk.MidPrice, err = readNumeric(midPrice, "mid_price"); err != nil {
		return Metric{}, err
	}
	if m.Risk.SpreadPct, err = readNumeric(spreadPct, "spread_pct"); err != nil {
		return Metric{}, err
	}
	if m.Risk.PoolSpotPrice, err = readNumeric(poolSpot, "pool_spot_price"); err != nil {
		return Metric{}, err
	}
	if m.Risk.PriceDivergencePct, err = readNumeric(divergence, "price_divergence_pct"); err != nil {
		return Metric{}, err
	}
	if m.Risk.MaxReachablePrice, err = readNumeric(maxReachable, "max_reachable_price"); err != nil {
		return Metric{}, err
	}
	if m.Risk.CostToMaxReachablePrice, err = readNumeric(costToMaxReachable, "cost_to_max_reachable_price"); err != nil {
		return Metric{}, err
	}
	if m.Risk.MaxSafeCollateral, err = readNumeric(cmax, "max_safe_collateral"); err != nil {
		return Metric{}, err
	}
	if m.Risk.MaxSafeCollateralLiquidation, err = readNumeric(cmaxLiquidation, "max_safe_collateral_liquidation"); err != nil {
		return Metric{}, err
	}
	if m.Risk.MaxSafeCollateralManipulation, err = readNumeric(cmaxManipulate, "max_safe_collateral_manipulation"); err != nil {
		return Metric{}, err
	}
	if m.Risk.Supporting.HolderTop1Pct, err = readNumeric(top1, "holder_top1_pct"); err != nil {
		return Metric{}, err
	}
	if m.Risk.Supporting.HolderTop10Pct, err = readNumeric(top10, "holder_top10_pct"); err != nil {
		return Metric{}, err
	}
	if m.Risk.Supporting.HolderHHI, err = readNumeric(hhi, "holder_hhi"); err != nil {
		return Metric{}, err
	}
	if m.Risk.Supporting.TradesExcludedPct, err = readNumeric(excludedPct, "trades_excluded_pct"); err != nil {
		return Metric{}, err
	}

	if m.Risk.Depth, err = decodeDepth(depthBody); err != nil {
		return Metric{}, err
	}
	if m.Risk.ManipulationCostCombined, err = decodeManipulation(combinedBody); err != nil {
		return Metric{}, err
	}
	if orderbookOnlyBody != nil {
		if m.Risk.ManipulationCostOrderbookOnly, err = decodeManipulation(orderbookOnlyBody); err != nil {
			return Metric{}, err
		}
	}
	if oracleBody != nil {
		if m.Risk.OracleResistance, err = decodeOracleResistance(oracleBody); err != nil {
			return Metric{}, err
		}
	}
	if volumeBody != nil {
		if err := decodeVolumeToSupply(volumeBody, &m.Risk.Supporting); err != nil {
			return Metric{}, err
		}
	}
	if tradeBody != nil {
		if m.Risk.Supporting.LastGenuineTrade, err = decodeLastGenuineTrade(tradeBody); err != nil {
			return Metric{}, err
		}
	}
	return m, nil
}

// ---------------------------------------------------------------- validation

// validRisk rejects what the CHECK constraints would reject anyway. The
// constraints are the guarantee; this exists so the error names the field
// instead of arriving as a Postgres constraint name.
func validRisk(r domain.AssetRisk) error {
	if r.MethodologyVersion == "" {
		return errors.New("store: methodology version is empty; every result carries one")
	}
	if r.LedgerSeq == 0 {
		return errors.New("store: ledger sequence is zero; every result carries one")
	}
	// domain.Valid rather than a switch repeated here. The set has drifted once
	// already, between this package's CHECK constraint and the const block, and
	// one enumeration is what stops it drifting again.
	if !r.DataSource.Valid() {
		return fmt.Errorf("store: data source %q is not one of the four", r.DataSource)
	}
	switch r.PriceSource {
	case domain.PriceSourceBook, domain.PriceSourcePool, domain.PriceSourceNone:
	default:
		return fmt.Errorf("store: price source %q is not one of the three", r.PriceSource)
	}
	switch r.Band {
	case domain.BandLow, domain.BandMedium, domain.BandHigh, domain.BandCritical:
	default:
		return fmt.Errorf("store: band %q is not one of the four", r.Band)
	}
	switch r.BandConfidence {
	case domain.BandConfidenceFull, domain.BandConfidencePartial:
	default:
		return fmt.Errorf("store: band confidence %q is not one of the two", r.BandConfidence)
	}
	// A none price source with a populated mid price is contradictory, and the
	// schema cannot express that. Letting it through would store a price for an
	// asset the methodology says has no executable price.
	if r.PriceSource == domain.PriceSourceNone && r.MidPrice != nil {
		return errors.New("store: price source is none but a mid price is set")
	}
	return nil
}

func flagStrings(flags []domain.Flag) []string {
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		out = append(out, string(f))
	}
	return out
}

func toFlags(in []string) []domain.Flag {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Flag, 0, len(in))
	for _, f := range in {
		out = append(out, domain.Flag(f))
	}
	return out
}

// decodeTextArray reads a text[] column that the query turned into JSONB. An
// empty array comes back as an empty slice and not as nil, because the three
// columns it serves are all NOT NULL: no flags is a measurement, and it is not
// the same statement as no data.
func decodeTextArray(body []byte, column string) ([]string, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("store: column %s: %w", column, err)
	}
	return out, nil
}

// stringsOrEmpty keeps a nil slice out of a NOT NULL text[] column.
func stringsOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// SummaryFilter narrows the asset list. An empty Band or Flag means no filter on
// that field.
type SummaryFilter struct {
	MethodologyVersion string
	Band               domain.Band
	Flag               domain.Flag
	Limit              int
	Offset             int
}

// LatestSummaries returns the newest result for every ACTIVE asset, filtered and
// paginated, together with the total before pagination.
//
// The latest row per asset is chosen FIRST and the band and flag filters are
// applied to that row afterwards. The other order looks equivalent and is not: it
// would return the most recent row that happens to carry the requested band,
// which for an asset that has since recovered means reporting a stale CRITICAL as
// if it were current.
func (s *Store) LatestSummaries(ctx context.Context, f SummaryFilter) ([]Metric, int, error) {
	if f.MethodologyVersion == "" {
		f.MethodologyVersion = domain.MethodologyVersion
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	const where = `
		 WHERE m.id IN (
		         SELECT DISTINCT ON (asset_id) id
		           FROM metrics WHERE methodology_version = $1
		          ORDER BY asset_id, ledger_seq DESC)
		   AND a.active
		   AND ($2 = '' OR m.band = $2)
		   AND ($3 = '' OR $3 = ANY(m.flags))`

	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM metrics m JOIN assets a ON a.id = m.asset_id`+where,
		f.MethodologyVersion, string(f.Band), string(f.Flag),
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count summaries: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+metricColumns+`
		   FROM metrics m JOIN assets a ON a.id = m.asset_id`+where+`
		  ORDER BY a.code, a.issuer NULLS FIRST, a.quote_code
		  LIMIT $4 OFFSET $5`,
		f.MethodologyVersion, string(f.Band), string(f.Flag), f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("store: list summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Metric
	for rows.Next() {
		m, err := scanMetric(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}
