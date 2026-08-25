// Package api is Keel's read-only HTTP surface. Five endpoints, no writes, no
// authentication, and no path from a request to Horizon.
//
// GREEN ZONE, but four decisions in here are load bearing:
//
//  1. THE API NEVER CALLS AN ADAPTER. Every handler reads results that were
//     already computed, through the Reader interface below, which internal/store
//     satisfies. Rule 1 of this package's brief: one popular asset triggering a
//     Horizon request per call would burn the rate limit budget in minutes. The
//     consequence is that metrics always lag, which NFR-1 accepts explicitly and
//     the X-Keel-Staleness-Seconds header reports.
//
//  2. READS GO THROUGH AN INTERFACE, NOT THROUGH *store.Store. That is what lets
//     the handler tests run with no database, so they run in CI on every push
//     rather than only when somebody remembers to start Postgres. The store's own
//     integration tests are where the SQL is proven; these prove the HTTP.
//
//  3. ASSET IDENTITY IS RESOLVED FROM STORAGE, NEVER INFERRED. The contract's
//     assetId is CODE:ISSUER with no asset type in it, and inferring the type
//     from the length of the code is the trap recorded on domain.Asset, in this
//     package's brief, and in two decision records. See assetid.go.
//
//  4. NET/HTTP AND NOTHING ELSE. Go's ServeMux has had method and wildcard
//     patterns since 1.22, so five routes need no router dependency. Rejected
//     alternative: chi or gorilla/mux, which would be the third dependency in a
//     repository that has two.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// Reader is everything this package needs from storage. It is deliberately
// read-only: there is no method here that writes, so no handler can acquire the
// ability to write by accident.
type Reader interface {
	Assets(ctx context.Context, activeOnly bool) ([]store.Asset, error)
	PairsForAsset(ctx context.Context, code, issuer string) ([]store.Asset, error)
	LatestMetrics(ctx context.Context, assetID int, methodologyVersion string) (store.Metric, error)
	MetricsAtLedger(ctx context.Context, assetID int, ledgerSeq uint32, methodologyVersion string, source domain.DataSource) (store.Metric, error)
	MetricsHistory(ctx context.Context, assetID int, fromLedger, toLedger uint32, methodologyVersion string, source domain.DataSource, limit int) ([]store.Metric, error)
	LatestSummaries(ctx context.Context, f store.SummaryFilter) ([]store.Metric, int, error)
	LastRun(ctx context.Context, kind store.RunKind) (store.Run, error)
}

// Config is the whole of the server's configuration.
type Config struct {
	Reader Reader
	// Params supplies the thresholds GET /methodology reports. It is passed in
	// rather than read from domain.DefaultParams() here, so that a deployment
	// running non-default parameters cannot report the defaults.
	Params domain.Params
	// HistoricalAvailable is false while the Hubble path does not exist. Stated
	// as configuration rather than hardcoded, because DEC-002 defers that path
	// rather than canceling it.
	HistoricalAvailable bool
	Logf                func(format string, args ...any)
}

// Server is the read-only HTTP surface described by docs/api/keel-openapi.yaml.
// It computes nothing: every figure it serves was computed elsewhere and stored,
// so a request can never be the thing that triggers a methodology run.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// BasePath is the prefix every route carries. The contract's server URLs end in
// /v1, so the paths in it are relative to that.
const BasePath = "/v1"

// New builds a Server and refuses a Config that cannot serve: the reader is
// required, because a server that starts without one fails per request instead
// of at startup, and the second is far harder to notice.
func New(cfg Config) (*Server, error) {
	if cfg.Reader == nil {
		return nil, errors.New("api: no reader")
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}

	s.mux.HandleFunc("GET "+BasePath+"/health", s.handleHealth)
	s.mux.HandleFunc("GET "+BasePath+"/methodology", s.handleMethodology)
	s.mux.HandleFunc("GET "+BasePath+"/assets", s.handleAssets)
	s.mux.HandleFunc("GET "+BasePath+"/asset/{assetId}/depth", s.handleDepth)
	s.mux.HandleFunc("GET "+BasePath+"/asset/{assetId}/history", s.handleHistory)
	return s, nil
}

// Handler returns the root handler. Every response is JSON, including the ones
// ServeMux produces for an unknown path, because a consumer parsing JSON should
// not have to handle an HTML body on a typo.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Keel-Methodology-Version", domain.MethodologyVersion)

		if _, pattern := s.mux.Handler(r); pattern == "" {
			s.writeError(w, http.StatusNotFound, codeAssetNotMonitored,
				"No such endpoint. See GET "+BasePath+"/health.", nil)
			return
		}
		s.mux.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------- meta

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	assets, err := s.cfg.Reader.Assets(ctx, true)
	if err != nil {
		s.fail(w, "health: assets", err)
		return
	}

	out := healthJSON{
		Status:              "ok",
		AssetsMonitored:     len(assets),
		MethodologyVersion:  domain.MethodologyVersion,
		HistoricalAvailable: s.cfg.HistoricalAvailable,
	}

	// The status is derived from the last scan, and the three degraded cases are
	// distinguished rather than merged. No scan at all is degraded: an API
	// serving nothing must not report ok. A scan that never finished is
	// degraded, because a crashed scan looks exactly like a fast one from the
	// outside. A scan with failures is degraded even when it finished.
	run, err := s.cfg.Reader.LastRun(ctx, store.RunScan)
	switch {
	case errors.Is(err, store.ErrNotFound):
		out.Status = "degraded"
	case err != nil:
		s.fail(w, "health: last run", err)
		return
	default:
		if run.FinishedAt != nil {
			at := run.FinishedAt.UTC()
			out.LatestScanAt = &at
		} else {
			out.Status = "degraded"
		}
		if run.AssetsFailed > 0 {
			out.Status = "degraded"
		}
	}

	// latestScanLedgerSeq comes from the newest metrics row rather than from the
	// run, because the runs table records the job and not the ledger it reached.
	if len(assets) > 0 {
		if m, err := s.cfg.Reader.LatestMetrics(ctx, assets[0].ID, ""); err == nil {
			seq := m.Risk.LedgerSeq
			out.LatestScanLedgerSeq = &seq
		} else if !errors.Is(err, store.ErrNotFound) {
			s.fail(w, "health: latest metrics", err)
			return
		}
	}

	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMethodology(w http.ResponseWriter, _ *http.Request) {
	t := s.cfg.Params.Thresholds

	// The threshold map is open ended by contract: a consumer reads it by key
	// name, so a new key needs no version bump. Every key ending in Pct is in
	// percent, matching the convention the whole API follows.
	//
	// TWO KEYS THE CONTRACT'S EXAMPLE CARRIES ARE DELIBERATELY ABSENT:
	// manipulationCheapUnit and thinDepth5PctUnit, both shown there as 'XLM'.
	// domain.Thresholds holds no unit, and these thresholds are compared against
	// notionals denominated in each asset's own QUOTE. Emitting 'XLM' would
	// assert a unit that is wrong for every pair not quoted in XLM. That is open
	// question Q7 in docs/methodology/02-pair-selection.md, and it is reported as
	// a gap rather than papered over with a literal.
	thresholds := map[string]any{
		"manipulationCheapAbsolute": t.ManipulationCheapAbsolute.String(),
		"manipulationRatioLowPct":   t.ManipulationRatioLowPct.String(),
		"thinDepth5PctAbsolute":     t.ThinDepth5PctAbsolute.String(),
		"spreadExtremePct":          t.SpreadExtremePct.String(),
		"priceDivergencePct":        t.PriceDivergencePct.String(),
		"holderTop1ExtremePct":      t.HolderTop1ExtremePct.String(),
		"holderTop10HighPct":        t.HolderTop10HighPct.String(),
		"washTradeSuspectedPct":     t.WashTradeSuspectedPct.String(),
		"genuineTradeStaleDays":     t.GenuineTradeStaleDays,
		"genuineTradeWarnDays":      t.GenuineTradeWarnDays,
		"oracleWindowSeconds":       int(s.cfg.Params.OracleWindow.Seconds()),
		"liquidationDelta":          num(s.cfg.Params.LiquidationDelta),
		"liquidationHaircut":        s.cfg.Params.LiquidationHaircut.String(),
		"manipulationCriticalDelta": num(s.cfg.Params.ManipulationCriticalDelta),
		"manipulationMargin":        s.cfg.Params.ManipulationMargin.String(),
	}

	s.writeJSON(w, http.StatusOK, methodologyJSON{
		Version:     domain.MethodologyVersion,
		DocumentURL: "https://github.com/Keel-Official/keel-backend/blob/main/docs/methodology/00-overview.md",
		Calibrated:  false,
		CalibrationNote: "The thresholds were chosen based on the magnitude of the Blend " +
			"incident of February 2026 and on conservative judgement, not calibrated against a " +
			"set of incidents. Every flag is reported separately so that consumers can apply " +
			"their own thresholds.",
		Thresholds: thresholds,
	})
}

// ---------------------------------------------------------------- assets

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	band, err := parseBand(q.Get("band"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange, err.Error(), nil)
		return
	}
	flag, err := parseFlag(q.Get("hasFlag"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange, err.Error(), nil)
		return
	}
	limit, err := parseBoundedInt(q.Get("limit"), 50, 1, 200)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange, "limit: "+err.Error(), nil)
		return
	}
	offset, err := parseBoundedInt(q.Get("offset"), 0, 0, 1<<30)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange, "offset: "+err.Error(), nil)
		return
	}

	rows, total, err := s.cfg.Reader.LatestSummaries(r.Context(), store.SummaryFilter{
		Band:   band,
		Flag:   flag,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		s.fail(w, "assets", err)
		return
	}

	out := assetListJSON{
		Items:              make([]assetSummaryJSON, 0, len(rows)),
		Total:              total,
		Limit:              limit,
		Offset:             offset,
		MethodologyVersion: domain.MethodologyVersion,
	}
	var newest store.Metric
	for _, m := range rows {
		out.Items = append(out.Items, summaryResponse(m))
		if m.Risk.LedgerSeq > newest.Risk.LedgerSeq {
			newest = m
		}
	}
	s.setStaleness(w, newest)
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDepth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pair, apiErr := s.resolvePair(ctx, r)
	if apiErr != nil {
		s.writeAPIError(w, apiErr)
		return
	}

	ledgerRaw := r.URL.Query().Get("ledger")
	if ledgerRaw == "" {
		m, err := s.cfg.Reader.LatestMetrics(ctx, pair.ID, "")
		if errors.Is(err, store.ErrNotFound) {
			// The pair IS monitored and simply has no result yet, which is a
			// different condition from an asset outside the demonstration set.
			// They share a code because the contract's error enum has no third
			// value, so the message is what separates them. That gap is handoff
			// item 18.
			s.writeError(w, http.StatusNotFound, codeAssetNotMonitored,
				"This pair is monitored, but no metrics have been computed for it yet. "+
					"See GET "+BasePath+"/health for the last scan.", nil)
			return
		}
		if err != nil {
			s.fail(w, "depth", err)
			return
		}
		s.setStaleness(w, m)
		s.writeJSON(w, http.StatusOK, riskResponse(m))
		return
	}

	// The historical path. DEC-002 defers Hubble, so there is no source that can
	// answer this yet, and a 503 with HISTORICAL_UNAVAILABLE is the contract's
	// own answer for that state. Returning a live figure and labeling it
	// historical would be the one genuinely dangerous alternative.
	if !s.cfg.HistoricalAvailable {
		s.writeError(w, http.StatusServiceUnavailable, codeHistoricalUnavailable,
			"Historical replay is not available. The Hubble path is deferred; see DEC-002.", nil)
		return
	}

	ledger, err := strconv.ParseUint(ledgerRaw, 10, 32)
	if err != nil || ledger == 0 {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"ledger must be a positive integer.", nil)
		return
	}
	m, err := s.cfg.Reader.MetricsAtLedger(ctx, pair.ID, uint32(ledger), "", domain.DataSourceHubble)
	if errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusNotFound, codeLedgerNotAvailable,
			"That ledger has not been replayed yet.", map[string]any{"ledger": ledger})
		return
	}
	if err != nil {
		s.fail(w, "depth at ledger", err)
		return
	}
	// Historical data does not go stale, which is why the header is zero rather
	// than absent here.
	w.Header().Set("X-Keel-Staleness-Seconds", "0")
	s.writeJSON(w, http.StatusOK, riskResponse(m))
}

// maxHistoryLedgers is 90 days of ledgers at one every five seconds, the limit
// the contract states and the number its own error example carries.
const maxHistoryLedgers = 1555200

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pair, apiErr := s.resolvePair(ctx, r)
	if apiErr != nil {
		s.writeAPIError(w, apiErr)
		return
	}

	q := r.URL.Query()
	from, err := strconv.ParseUint(q.Get("from"), 10, 32)
	if err != nil || from == 0 {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"from is required and must be a positive ledger sequence.", nil)
		return
	}
	to, err := strconv.ParseUint(q.Get("to"), 10, 32)
	if err != nil || to == 0 {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"to is required and must be a positive ledger sequence.", nil)
		return
	}
	if to < from {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"to must not be below from.", nil)
		return
	}
	if to-from > maxHistoryLedgers {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"The maximum range is 90 days per request.", map[string]any{
				"requestedLedgers": to - from,
				"maxLedgers":       maxHistoryLedgers,
			})
		return
	}

	resolution := q.Get("resolution")
	if resolution == "" {
		resolution = "day"
	}
	if resolution != "hour" && resolution != "day" {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"resolution must be hour or day.", nil)
		return
	}

	// ONE SERIES IS ONE DATA SOURCE. horizon is the default because it is the only
	// one of the four that is a direct reading; the other three are a warehouse
	// copy and two reconstructions, and trades-implied is a lower bound rather
	// than a measurement. Charting them together as one line would present the
	// weakest point in the range as if it were the same kind of number as the
	// strongest, which is what this endpoint did until 26 August 2026.
	source := domain.DataSource(q.Get("source"))
	if source == "" {
		source = domain.DataSourceHorizon
	}
	if !source.Valid() {
		s.writeError(w, http.StatusBadRequest, codeInvalidRange,
			"source must be one of horizon, hubble, offers-implied, trades-implied.",
			map[string]any{"source": q.Get("source"), "allowed": domain.DataSources()})
		return
	}

	rows, err := s.cfg.Reader.MetricsHistory(ctx, pair.ID, uint32(from), uint32(to), "", source, 0)
	if err != nil {
		s.fail(w, "history", err)
		return
	}

	points, gaps := downsample(rows, resolution)
	out := historyJSON{
		Asset:              asset(pair.Base),
		Quote:              asset(pair.Quote),
		From:               uint32(from),
		To:                 uint32(to),
		Resolution:         resolution,
		MethodologyVersion: domain.MethodologyVersion,
		// The source that was ASKED FOR, which is now the source of every row.
		// It used to be read off the last row, so an empty range reported a
		// default and a mixed range reported whichever source happened to sort
		// last. Both are labels the series could not support.
		DataSource: string(source),
		Gaps:       gaps,
		Points:     points,
	}
	s.writeJSON(w, http.StatusOK, out)
}

// downsample reduces one row per ledger to one point per bucket, and reports the
// buckets that hold nothing.
//
// IT SELECTS, IT DOES NOT AVERAGE, and that is the decision in this function.
// Averaging a band or a flag set is meaningless, and averaging a depth figure
// invents a number that no run of the methodology ever produced, which then
// appears on a chart with the same weight as a measured one. The LAST row in each
// bucket is taken, so every point on the chart is a real result that can be
// fetched again at its own ledger and checked.
//
// Rejected alternative: the worst band in each bucket, which is defensible for a
// risk chart and loses the property above.
func downsample(rows []store.Metric, resolution string) ([]historyPointJSON, []historyGapJSON) {
	points := make([]historyPointJSON, 0, len(rows))
	gaps := []historyGapJSON{}
	if len(rows) == 0 {
		return points, gaps
	}

	bucket := time.Hour
	if resolution == "day" {
		bucket = 24 * time.Hour
	}

	var (
		current time.Time
		held    store.Metric
		haveOne bool
	)
	flush := func() {
		if haveOne {
			points = append(points, historyPoint(held))
		}
	}
	for _, m := range rows {
		b := m.Risk.LedgerClosedAt.UTC().Truncate(bucket)
		if !haveOne {
			current, held, haveOne = b, m, true
			continue
		}
		if b.Equal(current) {
			held = m // the last row in the bucket wins
			continue
		}
		flush()
		// A bucket boundary crossed by more than one bucket is a gap. The
		// contract requires these to be reported so the dashboard draws a break
		// instead of interpolating across a period nothing was recorded.
		if b.Sub(current) > bucket {
			gaps = append(gaps, historyGapJSON{
				From:   held.Risk.LedgerSeq,
				To:     m.Risk.LedgerSeq,
				Reason: "no result was recorded between these ledgers",
			})
		}
		current, held = b, m
	}
	flush()
	return points, gaps
}

// ---------------------------------------------------------------- staleness

// setStaleness reports how far behind the ledger the data was WHEN IT WAS
// COMPUTED, which is what the contract's header description defines.
//
// It is computedAt minus ledgerClosedAt, and both are stored columns, so it is a
// property of the record rather than of the moment it is served. Note that this
// does NOT answer "how old is this data now", which a consumer might reasonably
// expect from the name; the contract's own note about a 900 second target reads
// as though it did. Reported to Al as handoff item 18 rather than resolved here,
// because changing the field's meaning is a contract change.
//
// Zero for anything not read live: historical data does not go stale.
func (s *Server) setStaleness(w http.ResponseWriter, m store.Metric) {
	if m.Risk.LedgerSeq == 0 {
		return
	}
	if m.Risk.DataSource != domain.DataSourceHorizon {
		w.Header().Set("X-Keel-Staleness-Seconds", "0")
		return
	}
	lag := int(m.ComputedAt.Sub(m.Risk.LedgerClosedAt).Seconds())
	if lag < 0 {
		lag = 0
	}
	w.Header().Set("X-Keel-Staleness-Seconds", strconv.Itoa(lag))
}

// ---------------------------------------------------------------- plumbing

const (
	codeInvalidAssetID        = "INVALID_ASSET_ID"
	codeInvalidRange          = "INVALID_RANGE"
	codeAssetNotMonitored     = "ASSET_NOT_MONITORED"
	codeLedgerNotAvailable    = "LEDGER_NOT_AVAILABLE"
	codeHistoricalUnavailable = "HISTORICAL_UNAVAILABLE"
)

func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		// The status line is already sent, so this cannot become an error
		// response. It is logged instead of being swallowed.
		s.cfg.Logf("api: writing response: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string, detail map[string]any) {
	s.writeJSON(w, status, errorBodyJSON{Error: errorDetailJSON{
		Code: code, Message: message, Detail: detail,
	}})
}

// fail is the 500 path. The internal error is logged and never sent: a database
// error message can name tables and columns, and a read-only public API has no
// reason to describe its own schema to a stranger.
func (s *Server) fail(w http.ResponseWriter, where string, err error) {
	s.cfg.Logf("api: %s: %v", where, err)
	s.writeJSON(w, http.StatusInternalServerError, errorBodyJSON{Error: errorDetailJSON{
		Code:    "INTERNAL",
		Message: "The request could not be served. The failure has been logged.",
	}})
}

func parseBand(raw string) (domain.Band, error) {
	if raw == "" {
		return "", nil
	}
	switch b := domain.Band(raw); b {
	case domain.BandLow, domain.BandMedium, domain.BandHigh, domain.BandCritical:
		return b, nil
	}
	return "", fmt.Errorf("band must be one of LOW, MEDIUM, HIGH, CRITICAL")
}

func parseFlag(raw string) (domain.Flag, error) {
	if raw == "" {
		return "", nil
	}
	// Matched against the enumerated flags rather than passed through, so that a
	// typo returns 400 instead of an empty list that reads as "no asset has this
	// problem".
	for _, f := range allFlags {
		if domain.Flag(raw) == f {
			return f, nil
		}
	}
	return "", fmt.Errorf("hasFlag %q is not a known flag", raw)
}

var allFlags = []domain.Flag{
	domain.FlagNoExecutablePrice, domain.FlagZeroDepth2Pct, domain.FlagManipulationCheap,
	domain.FlagManipulationRatioLow, domain.FlagPriceSourceConflict, domain.FlagSpreadExtreme,
	domain.FlagNoGenuineTrade30D, domain.FlagNoGenuineTrade7D,
	domain.FlagHolderConcentrationExtreme, domain.FlagHolderConcentrationHigh,
	domain.FlagThinDepth5Pct, domain.FlagWashTradeSuspected,
}

func parseBoundedInt(raw string, def, lo, hi int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("must be an integer")
	}
	if v < lo || v > hi {
		return 0, fmt.Errorf("must be between %d and %d", lo, hi)
	}
	return v, nil
}

// Serve runs the HTTP server until the context is canceled, then shuts down
// gracefully so an in-flight response is not cut off mid-body.
func (s *Server) Serve(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          log.New(discard{}, "", 0),
	}

	done := make(chan error, 1)
	go func() { done <- srv.ListenAndServe() }()

	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// discard silences net/http's own logger, which writes TLS handshake noise from
// port scanners at a level nothing can act on. Handler errors are logged through
// Config.Logf instead.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
