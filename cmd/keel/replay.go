// The `replay` subcommand: the order book of a pair at a past ledger.
//
// IT OWNS NO METHODOLOGY AND NO RECONSTRUCTION. internal/horizon does the work;
// this file parses flags, prints what came back, and optionally writes the
// snapshot as JSON. If a formula or an offer rule ever appears here it is in the
// wrong file.
//
// WHAT REPLACED THE STUB. Until 26 August 2026 this subcommand printed "not
// implemented yet (needs internal/hubble)" and exited 3, because DEC-002 deferred
// BigQuery and BigQuery was assumed to be the only way to a past book. It is not.
// Horizon serves every operation and every operation RESULT for ever, and a book
// is the sum of the operations that posted it. See the header of
// internal/horizon/replay.go, and DEC-002 section 2.3, which specified this and
// gated it behind "only attempt this if 2.1 and 2.2 prove insufficient". They did.
//
// THIS IS NOT HUBBLE AND DOES NOT CLOSE DEC-002. Two things this path cannot do
// that a full historical dataset can: it cannot see an offer whose owner never
// traded and is not resting today, and it does not reconstruct pool reserves at
// all, so the snapshot it produces carries no pools. Both are printed on every
// run rather than left in a document.
package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
	"github.com/shopspring/decimal"
)

func runReplay(args []string) error {
	fs := flag.NewFlagSet("replay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	pairsPath := fs.String("pairs", "", "path to a pair list, required. scripts/record-pairs.example.json holds USTRY/USDC")
	ledger := fs.Uint("ledger", 0, "the ledger sequence to rebuild the book at, required")
	tradesFrom := fs.Uint("trades-from-ledger", 0,
		"ledger to seek the trade walk to. 0 walks the pair's whole history. Being early costs requests; being late loses offers")
	lookahead := fs.Uint("lookahead", 20000,
		"how many ledgers PAST the target the trade walk continues, to discover accounts that were resting and had not traded yet. Trades after the target are never applied")
	since := fs.Uint("since-ledger", 0,
		"floor on each account's backwards walk: operations older than this are not read. 0 walks to the account's first operation. A floor makes the cost predictable and makes every offer created below it invisible, which reads as a THINNER book, so the depth each walk reached is reported")
	maxPages := fs.Int("max-pages-per-account", 0, "cap on each account's backwards operation walk, in pages of 200. 0 uses the built-in default")
	maxPagesOffering := fs.Int("max-pages-per-offering-account", 0,
		"the deeper cap that applies from an account's first offer operation on this pair. 0 uses the built-in default")
	quiet := fs.Bool("quiet", false, "do not print one progress line per account walked")
	compute := fs.Bool("compute", false,
		"run the methodology over the reconstructed book and print the result. ORDER BOOK ONLY, because no pool is reconstructed, so a combined depth figure from it would be wrong")
	removalsPath := fs.String("known-removals", "",
		"path to a known-removals list, e.g. configs/known-removals.json. OFF by default. Each entry takes one "+
			"offer off the book at targets at or above its gone_by_ledger, and the applied ids are printed with the result")
	persist := fs.Bool("persist", false, "store computed offers-implied metrics; requires -pool-snapshots, a declared pair and no detected reconstruction gaps")
	acceptIncomplete := fs.Bool("accept-incomplete", false,
		"store a reconstruction whose walk detected gaps, with every gap recorded in the row. DEC-022. A crossed or inflated book is still refused")
	tradeMetrics := fs.Bool("trade-metrics", false,
		"compute the trade-derived metrics at the target from the trades this walk already read: last genuine trade, excluded share, and the genuine volume in the oracle window. See cmd/keel/historicaltrades.go")
	poolsFromEffects := fs.Bool("pools-from-effects", false,
		"reconstruct pool reserves at the target from each pool's own effects instead of reading a -pool-snapshots file. See internal/horizon/poolhistory.go")
	poolEffectPages := fs.Int("pool-effect-pages", 5,
		"requests allowed per pool when reading back to its last effect at or before the target")
	poolSnapshots := fs.String("pool-snapshots", "", "JSON snapshot array with audited pool coverage at this pair and ledger, required for -persist; null Pools means unknown and is refused")
	dsn := fs.String("dsn", envOr(envDSN, store.DefaultDSN), "Postgres DSN for -persist only, or set KEEL_DSN")
	out := fs.String("out", "", "write the reconstructed snapshot to this file as JSON. Optional")
	dumpOffers := fs.Bool("dump-offers", false,
		"print every offer resting at the target, one row per offer, with its ID, seller and the operation that last wrote it")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel replay - rebuild a pair's order book at a past ledger

Horizon serves no order book at a past ledger. It serves every operation and the
RESULT of every operation, and a book is what those operations left behind. This
replays them. The output is dataSource "offers-implied", the same label the golden
fixture carries, and it is NOT a measurement.

READ THE DIAGNOSTICS. The reconstruction has three ways to be incomplete and all
three are counted on every run. A book with missing offers reads as a THIN book,
which is this product's most interesting finding and therefore the worst thing to
produce by accident.

Pools are not reconstructed. The snapshot carries none, and that is not a claim
that no pool existed.

Persistence requires independently established pool coverage in -pool-snapshots.
Each entry uses domain.Snapshot JSON: Base, Quote, LedgerSeq, LedgerClosedAt,
Source (offers-implied), and Pools. An explicit empty Pools array asserts that no
pool existed; null or an omitted field asserts nothing and cannot be persisted.
Keep the source evidence with that file. Supplying it does not certify the book.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pairsPath == "" {
		return errors.New("replay: -pairs is required")
	}
	if *ledger == 0 || uint64(*ledger) > uint64(^uint32(0)) {
		return errors.New("replay: -ledger must be a positive uint32")
	}

	pairs, err := horizon.LoadPairs(*pairsPath)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}
	if *poolSnapshots != "" && !*persist {
		return errors.New("replay: -pool-snapshots requires -persist")
	}
	if *poolsFromEffects && *poolSnapshots != "" {
		return errors.New("replay: -pools-from-effects and -pool-snapshots are two answers to one question; pass one")
	}
	var poolEvidence []domain.Snapshot
	if *persist {
		// EITHER ROUTE SUPPLIES POOL COVERAGE, AND NEITHER IS A DEFAULT. A stored
		// row must never report an unobserved pool as absent, so the operator says
		// where the reserves come from: a hand-audited evidence file, or the pools'
		// own effects at the target. The second refuses to answer when its walk did
		// not reach a pool, which leaves Pools nil and makes this same check fail
		// later, at persist time.
		if *poolSnapshots == "" && !*poolsFromEffects {
			return errors.New("replay: -persist requires -pool-snapshots or -pools-from-effects; unobserved pools must not be stored as absent")
		}
		if *poolSnapshots == "" {
			poolEvidence = nil
		}
		if *poolSnapshots != "" {
			poolEvidence, err = readReplayPoolSnapshots(*poolSnapshots)
			if err != nil {
				return err
			}
		}
		for _, p := range pairs {
			if _, err := replayPoolSnapshot(poolEvidence, p.Base, p.Quote, uint32(*ledger)); err != nil {
				return err
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var removals []horizon.KnownRemoval
	if *removalsPath != "" {
		var err error
		removals, err = horizon.LoadKnownRemovals(*removalsPath)
		if err != nil {
			return fmt.Errorf("replay: %w", err)
		}
		fmt.Fprintf(os.Stdout, "  %d known removal(s) from %s\n", len(removals), *removalsPath)
	}

	var db *store.Store
	if *persist {
		db, err = openStore(ctx, *dsn)
		if err != nil {
			return err
		}
		defer func() { _ = db.Close() }()
		applied, err := db.SchemaVersion(ctx)
		if err != nil {
			return fmt.Errorf("replay: reading schema_migrations: %w; run make migrate", err)
		}
		if len(applied) == 0 {
			return errors.New("replay: no migrations are applied; run make migrate")
		}
		for _, p := range pairs {
			if _, err := db.AssetID(ctx, p.Base, p.Quote); err != nil {
				return fmt.Errorf("replay: declare %s with keel assets before persisting: %w", p, err)
			}
		}
	}

	client := horizon.NewClient(horizon.Config{BaseURL: *baseURL, Budget: *budget})

	snapshots := make([]domain.Snapshot, 0, len(pairs))
	for _, p := range pairs {
		walked := 0
		res, err := client.ReconstructBook(ctx, p.Base, p.Quote, horizon.ReplayQuery{
			TargetLedger:       uint32(*ledger),
			TradesFromLedger:   uint32(*tradesFrom),
			TradeLookahead:     uint32(*lookahead),
			SinceLedger:        uint32(*since),
			KnownRemovals:      removals,
			MaxPagesPerAccount: *maxPages,

			MaxPagesPerOfferingAccount: *maxPagesOffering,
			Progress: func(w horizon.AccountWalk) {
				walked++
				if *quiet {
					return
				}
				fmt.Fprintf(os.Stderr, "  [%3d] %s  %d page(s), back to ledger %d, %d offer op(s)%s%s%s\n",
					walked, w.Account[:8], w.Pages, w.EarliestLedger, w.OfferOperations,
					flagIf(w.Truncated, " TRUNCATED"), flagIf(w.StoppedAtFloor, " floor"),
					flagIf(w.Err != "", " FAILED"))
			},
		})
		if err != nil {
			return fmt.Errorf("replay %s: %w", p, err)
		}
		reportReplay(os.Stdout, p, res)
		if *dumpOffers {
			reportResting(os.Stdout, res.Resting)
		}
		if *poolsFromEffects {
			pools, walk, err := client.PoolReservesAt(ctx, p.Base, p.Quote, uint32(*ledger), *poolEffectPages)
			if err != nil {
				return fmt.Errorf("replay %s: pools at ledger %d: %w", p, *ledger, err)
			}
			reportPoolsAt(os.Stdout, pools, walk)
			// A pool the walk could not reach is not a pool that was absent, and a
			// book that quietly drops one reports less AMM liquidity than the ledger
			// held. Leaving Pools nil is what makes -persist refuse.
			if walk.Complete() {
				res.Snapshot.Pools = make([]domain.PoolReserves, 0, len(pools))
				for _, at := range pools {
					res.Snapshot.Pools = append(res.Snapshot.Pools, at.Pool)
				}
			}
		}
		// The ledger resource supplies the actual close time. A wall-clock time or a
		// ledger-to-time estimate would corrupt historical downsampling, and the
		// trade metrics anchor their windows on it.
		if (*persist || *tradeMetrics) && res.Snapshot.LedgerClosedAt.IsZero() {
			res.Snapshot.LedgerClosedAt, err = client.ReplayLedgerCloseTime(ctx, res.Snapshot.LedgerSeq)
			if err != nil {
				return fmt.Errorf("replay: %w", err)
			}
		}

		var sup *domain.SupportingMetrics
		if *tradeMetrics {
			var notes []string
			sup, notes = historicalSupporting(res.Trades, p.Base, res.Snapshot.LedgerClosedAt,
				domain.DefaultParams().OracleWindow, domain.DefaultGenuineRules())
			fmt.Fprintf(os.Stdout, "  trade metrics at the target: %s\n",
				map[bool]string{true: "computed", false: "UNEVALUATED"}[sup != nil])
			for _, n := range notes {
				fmt.Fprintf(os.Stdout, "    %s\n", n)
			}
			if sup != nil && sup.LastGenuineTrade != nil {
				fmt.Fprintf(os.Stdout, "    last genuine trade at %s, ledger %d\n",
					sup.LastGenuineTrade.At.UTC().Format(time.RFC3339), sup.LastGenuineTrade.LedgerSeq)
			}
		}

		if *persist {
			// The evidence file is checked against Horizon's own close time. The
			// effects route needs no such check: its reserves are read AT the
			// target rather than declared for it, so there is no second reading
			// to disagree with.
			if poolEvidence != nil {
				pool, err := replayPoolSnapshot(poolEvidence, p.Base, p.Quote, res.Snapshot.LedgerSeq)
				if err != nil {
					return err
				}
				if !pool.LedgerClosedAt.Equal(res.Snapshot.LedgerClosedAt) {
					return fmt.Errorf("replay: pool evidence close time differs from Horizon at ledger %d", res.Snapshot.LedgerSeq)
				}
				res.Snapshot.Pools = pool.Pools
			}
			id, inserted, err := persistReplay(ctx, db, res, sup, *acceptIncomplete, uint32(*since))
			if err != nil {
				return fmt.Errorf("replay %s: %w", p, err)
			}
			poolSource := *poolSnapshots
			if poolSource == "" {
				poolSource = "pool effects at the target"
			}
			fmt.Fprintf(os.Stdout, "  metrics row %d: inserted=%t, ledger=%d, methodology=%s, source=offers-implied, pool coverage=%s (%d pools)\n",
				id, inserted, res.Snapshot.LedgerSeq, domain.MethodologyVersion, poolSource, len(res.Snapshot.Pools))
			if !inserted {
				fmt.Fprintln(os.Stdout, "  existing row retained unchanged; a different reconstruction requires investigation, not an overwrite")
			}
		}
		if *compute {
			switch {
			case res.Snapshot.Pools != nil:
				reportRiskUnder(os.Stdout, res.Snapshot, sup, "risk with the pool coverage supplied for this ledger")
			default:
				reportRisk(os.Stdout, res.Snapshot, sup)
			}
		}
		snapshots = append(snapshots, res.Snapshot)
	}

	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return fmt.Errorf("replay: %w", err)
		}
		defer func() { _ = f.Close() }()
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snapshots); err != nil {
			return fmt.Errorf("replay: writing %s: %w", *out, err)
		}
		fmt.Fprintf(os.Stdout, "  wrote %s\n", *out)
	}
	return nil
}

type replayMetricsStore interface {
	AssetID(context.Context, domain.Asset, domain.Asset) (int, error)
	SaveMetrics(context.Context, int, time.Time, domain.AssetRisk) (int64, bool, error)
}

// persistReplay only bridges validated replay output to the existing store. The
// completeness checks detect known gaps; they do not certify the reconstruction.
// Unknown pool coverage is refused: the API cannot label a stored result as
// order-book-only, and interpreting an unobserved pool as absent changes flags.
func persistReplay(ctx context.Context, db replayMetricsStore, res horizon.ReplayResult, sup *domain.SupportingMetrics, acceptIncomplete bool, floorLedger uint32) (int64, bool, error) {
	// THE GATE IS ASYMMETRIC SINCE DEC-022 AND THE HALVES ARE NOT NEGOTIABLE
	// AGAINST EACH OTHER. A crossed book PROVES an offer is missing, and an
	// inflated one is the single gap that makes the book look DEEPER than it was.
	// Neither has an override at any flag combination, which is why they are
	// tested before acceptIncomplete is read at all. Everything below them removes
	// offers, so it fails in the direction a warning product may fail in, and
	// -accept-incomplete admits exactly that set and records it in the row.
	_, _, crossed := res.Snapshot.Book.Crossed()
	crossed = crossed || res.Crossed
	if crossed {
		return 0, false, errors.New("refusing to persist a crossed book: a bid at or above an ask cannot exist on a ledger, so this reconstruction is provably missing an offer. There is no override")
	}
	if res.MayBeInflated() {
		return 0, false, fmt.Errorf("refusing to persist an inflated book: offers were applied from ledger %d, below the trade window at %d, so anything eaten in between is still resting here. This is the one gap that makes a book look DEEPER than it was, and it has no override", res.EarliestOfferOp, res.TradeWindowFrom)
	}

	gaps := len(res.MissingOfferIDs) + res.Truncated + res.Unsizable + res.Failed + res.StoppedAtFloor
	if gaps != 0 && !acceptIncomplete {
		return 0, false, fmt.Errorf("refusing to persist incomplete reconstruction: missing=%d truncated=%d failed=%d unsizable=%d floor=%d. Pass -accept-incomplete to store it with every one of these recorded in the row (DEC-022)", len(res.MissingOfferIDs), res.Truncated, res.Failed, res.Unsizable, res.StoppedAtFloor)
	}
	if res.StoppedAtFloor != 0 && floorLedger == 0 {
		// A walk cannot stop at a floor it was never given. If this fires, the
		// floor reaching this function is not the floor the walk ran under, and
		// storing it would put an unreadable "stopped at ledger 0" in the row.
		return 0, false, fmt.Errorf("refusing to persist: %d walk(s) stopped at a floor but no floor ledger was supplied to the writer", res.StoppedAtFloor)
	}
	if res.Snapshot.Source != domain.DataSourceOffersImplied || res.Snapshot.LedgerSeq == 0 || res.Snapshot.LedgerClosedAt.IsZero() || res.ReadAt.IsZero() {
		return 0, false, errors.New("replay persistence requires offers-implied source, ledger sequence, ledger close time and read time")
	}
	if err := validateReplayPools(res.Snapshot.Pools); err != nil {
		return 0, false, err
	}
	id, err := db.AssetID(ctx, res.Snapshot.Base, res.Snapshot.Quote)
	if err != nil {
		return 0, false, fmt.Errorf("declare the pair with keel assets before persisting: %w", err)
	}
	// ALWAYS SUPPLIED ON THIS PATH, INCLUDING WHEN EVERY COUNTER IS ZERO. Nil means
	// "this row is not a reconstruction", so a clean walk stored without it would
	// be indistinguishable from a live Horizon read, and that is the one confusion
	// the field exists to prevent. A zero-valued struct here says something true:
	// a walk ran and detected nothing, which its own warning line is careful not to
	// call proof.
	rec := &domain.Reconstruction{
		Truncated:      res.Truncated,
		StoppedAtFloor: res.StoppedAtFloor,
		Failed:         res.Failed,
		Unsizable:      res.Unsizable,
		MissingOffers:  len(res.MissingOfferIDs),
		FloorLedger:    floorLedger,
		AccountsWalked: len(res.Accounts),
	}
	// sup is nil unless -trade-metrics ran, which is the behavior every stored
	// row had before it existed: six flags unevaluated and no oracle window.
	risk, err := domain.ComputeAssetRiskFrom(res.Snapshot, domain.DefaultParams(), sup, rec)
	if err != nil {
		return 0, false, fmt.Errorf("compute replay risk: %w", err)
	}
	return db.SaveMetrics(ctx, id, res.ReadAt, risk)
}

func readReplayPoolSnapshots(path string) ([]domain.Snapshot, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("replay: pool evidence: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	// Pointers preserve omitted/null versus explicit zero at the input boundary.
	// A missing reserve or fee must not become an empty pool or a zero-fee pool.
	var input []struct {
		domain.Snapshot
		Pools []struct {
			PoolID       string
			ReserveBase  *decimal.Decimal
			ReserveQuote *decimal.Decimal
			FeeBP        *int32
		}
	}
	if err := dec.Decode(&input); err != nil {
		return nil, fmt.Errorf("replay: pool evidence: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("replay: pool evidence must contain exactly one JSON array")
	}
	snapshots := make([]domain.Snapshot, 0, len(input))
	for _, entry := range input {
		if entry.Pools == nil {
			return nil, errors.New("replay: pool evidence must explicitly supply Pools; null is unknown")
		}
		s := entry.Snapshot
		s.Pools = make([]domain.PoolReserves, 0, len(entry.Pools))
		for _, p := range entry.Pools {
			if p.ReserveBase == nil || p.ReserveQuote == nil || p.FeeBP == nil {
				return nil, fmt.Errorf("replay: pool %q must explicitly supply ReserveBase, ReserveQuote and FeeBP", p.PoolID)
			}
			s.Pools = append(s.Pools, domain.PoolReserves{PoolID: p.PoolID, ReserveBase: *p.ReserveBase, ReserveQuote: *p.ReserveQuote, FeeBP: *p.FeeBP})
		}
		if err := validateReplayPools(s.Pools); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

func replayPoolSnapshot(snapshots []domain.Snapshot, base, quote domain.Asset, ledger uint32) (domain.Snapshot, error) {
	var found *domain.Snapshot
	for i := range snapshots {
		s := &snapshots[i]
		if !s.Base.Equal(base) || !s.Quote.Equal(quote) || s.LedgerSeq != ledger {
			continue
		}
		if found != nil {
			return domain.Snapshot{}, errors.New("replay: duplicate pool evidence for pair and ledger")
		}
		found = s
	}
	if found == nil {
		return domain.Snapshot{}, fmt.Errorf("replay: no pool evidence for %s/%s at ledger %d", base, quote, ledger)
	}
	if found.Source != domain.DataSourceOffersImplied || found.LedgerClosedAt.IsZero() {
		return domain.Snapshot{}, errors.New("replay: pool evidence requires offers-implied source and ledger close time")
	}
	if err := validateReplayPools(found.Pools); err != nil {
		return domain.Snapshot{}, err
	}
	return *found, nil
}

func validateReplayPools(pools []domain.PoolReserves) error {
	if pools == nil {
		return errors.New("replay: pool coverage is unknown; supply audited historical pool evidence before persisting")
	}
	seen := make(map[string]bool)
	for _, p := range pools {
		id, err := hex.DecodeString(p.PoolID)
		if err != nil || len(id) != 32 || p.ReserveBase.IsNegative() || p.ReserveQuote.IsNegative() || p.FeeBP < 0 || p.FeeBP >= 10000 {
			return fmt.Errorf("replay: invalid historical pool %q", p.PoolID)
		}
		key := hex.EncodeToString(id)
		if seen[key] {
			return fmt.Errorf("replay: duplicate historical pool %q", p.PoolID)
		}
		seen[key] = true
	}
	return nil
}

// reportReplay prints the book and, at equal weight, the reasons to distrust it.
func reportReplay(w *os.File, p horizon.Pair, r horizon.ReplayResult) {
	s := r.Snapshot
	fmt.Fprintf(w, "%s at ledger %d\n", p, s.LedgerSeq)
	fmt.Fprintf(w, "  dataSource %s, %d bid level(s), %d ask level(s)\n",
		s.Source, len(s.Book.Bids), len(s.Book.Asks))

	for i, l := range s.Book.Asks {
		fmt.Fprintf(w, "    ask %d  price %s (%s)  amount %s\n", i, l.Price.Decimal(), l.Price, l.Amount)
	}
	for i, l := range s.Book.Bids {
		fmt.Fprintf(w, "    bid %d  price %s (%s)  amount %s\n", i, l.Price.Decimal(), l.Price, l.Amount)
	}

	fmt.Fprintf(w, "  read %d trade(s), walked %d account(s) (%d from trades, %d only from the live offer book)\n",
		r.TradesRead, len(r.Accounts), r.FromTrades, r.FromLiveOffers)
	fmt.Fprintf(w, "  %d offer operation(s) applied, %d Horizon request(s)\n", r.OfferOperations, r.Requests)

	// THE WAYS THIS CAN BE INCOMPLETE, PRINTED WHETHER OR NOT THEY FIRED. A
	// diagnostic that only appears when it is bad teaches a reader to skim.
	fmt.Fprintf(w, "  completeness: %d account walk(s) truncated, %d stopped at the ledger floor, %d failed, %d result(s) unsizable, %d offer(s) named by trades but never seen\n",
		r.Truncated, r.StoppedAtFloor, r.Failed, r.Unsizable, len(r.MissingOfferIDs))
	for _, a := range r.Accounts {
		if a.Err != "" {
			fmt.Fprintf(w, "    walk failed for %s after %d page(s): %s\n", a.Account[:8], a.Pages, a.Err)
		}
	}
	fmt.Fprintf(w, "  windows: offers applied back to ledger %d, trades read from ledger %d\n",
		r.EarliestOfferOp, r.TradeWindowFrom)
	if r.MayBeInflated() {
		// THIS ONE RUNS THE OTHER WAY AND GETS ITS OWN LINE. Every other gap loses
		// offers and makes the book look thinner. This one keeps offers that were
		// already eaten and makes it look DEEPER, which is the direction a warning
		// product must never fail in.
		fmt.Fprintf(w, "  INFLATED: offers were applied from before the trade window, so anything eaten\n")
		fmt.Fprintf(w, "  between ledger %d and %d is still on this book. It is too DEEP, not too thin\n",
			r.EarliestOfferOp, r.TradeWindowFrom)
	}
	if len(r.MissingOfferIDs) > 0 {
		show := r.MissingOfferIDs
		if len(show) > 10 {
			show = show[:10]
		}
		fmt.Fprintf(w, "    missing offer ids (first %d): %v\n", len(show), show)
	}
	if r.Crossed {
		// THE ONLY LINE HERE THAT IS A PROOF RATHER THAN A SUSPICION, so it says
		// WRONG where the others say incomplete. Both prices are printed because
		// finding the two offers afterwards is done by price_r against the trade
		// stream, which is how the February pair was resolved. See
		// docs/evidences/2026-09-12-crossed-book-ustry-february.md.
		fmt.Fprintf(w, "  CROSSED: best bid %s is at or above best ask %s, which no ledger can hold.\n",
			r.CrossedBid.Price.Decimal(), r.CrossedAsk.Price.Decimal())
		fmt.Fprintf(w, "  This book is WRONG rather than thin. The ratios are %s and %s\n",
			r.CrossedBid.Price, r.CrossedAsk.Price)
	}
	if len(r.KnownRemovalsApplied) > 0 {
		// PRINTED WHETHER OR NOT THE BOOK LOOKS BETTER FOR IT. A repaired book
		// and an unrepaired one are two readings of the same ledger, and a reader
		// who is not told which one is in front of them cannot check either.
		fmt.Fprintf(w, "  KNOWN REMOVALS APPLIED: %v. This book was repaired by hand-proven readings, not by the fold alone\n",
			r.KnownRemovalsApplied)
	}
	if r.Complete() {
		fmt.Fprintf(w, "  no hole this method can detect. That is not the same claim as correct: an offer whose owner\n")
		fmt.Fprintf(w, "  never traded and is not resting today is invisible to it, and pools are not reconstructed at all\n")
	} else {
		fmt.Fprintf(w, "  THIS BOOK IS INCOMPLETE. A missing offer reads as thin depth, so do not quote a depth figure from it\n")
	}
}

// flagIf keeps the progress line short: a marker when something happened and
// nothing at all when it did not.
func flagIf(b bool, s string) string {
	if b {
		return s
	}
	return ""
}

// reportRisk runs the methodology over a reconstructed book and prints it.
//
// IT IS ORDER BOOK ONLY AND SAYS SO TWICE. replay.go reconstructs no pool, so the
// snapshot carries none, and non-negotiable rule 4 combines SDEX and AMM at a
// shared marginal price. A depth figure from a snapshot with no pool is the SDEX
// half of the answer, and presenting it as the combination is exactly the error
// DEC-006 section 4 is about.
func reportRisk(w *os.File, s domain.Snapshot, sup *domain.SupportingMetrics) {
	reportRiskUnder(w, s, sup, "over the reconstructed book, ORDER BOOK ONLY, no pool")
}

// reportRiskUnder is reportRisk with the heading supplied by the caller.
//
// THE HEADING IS NOT DECORATION AND THAT IS WHY IT MOVED. It states what the
// figures below it were computed over, and the one reportRisk hardcoded was true
// of exactly one caller: replay reconstructs a book from operations and
// reconstructs no pool. `keel layer1` reads RECORDED bytes and those carry the
// pool response, so the same eleven lines printed under the same heading would
// have claimed a pool was excluded from figures that include it. A wrong label on
// a right number is worse than a wrong number, because nothing downstream
// disagrees with it.
func reportRiskUnder(w *os.File, s domain.Snapshot, sup *domain.SupportingMetrics, heading string) {
	r, err := domain.ComputeAssetRiskWith(s, domain.DefaultParams(), sup)
	if err != nil {
		fmt.Fprintf(w, "  compute: %v\n", err)
		return
	}
	fmt.Fprintf(w, "  --- methodology %s %s ---\n", r.MethodologyVersion, heading)
	fmt.Fprintf(w, "    P0 %s from %s", show(r.MidPrice), r.PriceSource)
	if r.SpreadPct != nil {
		fmt.Fprintf(w, ", spread %s percent", r.SpreadPct.StringFixed(7))
	}
	fmt.Fprintln(w)

	for _, d := range r.Depth {
		fmt.Fprintf(w, "    depth  delta %-5s buy %s  sell %s\n", d.Delta, d.BuySide, d.SellSide)
	}
	// BOTH LADDERS, EACH NAMED, AND THE HEADLINE ONE FIRST. This printed only the
	// orderbook-only ladder under the bare label "cost" until 17 September 2026,
	// and on a pair with an active pool the two differ completely: at ledger
	// 61340262 the orderbook-only rungs all read 0, because the single ask sits
	// above every target and there is nothing cheaper to buy, while the combined
	// rungs read 3.68 to 148.31 for the pool that has to be walked along its curve.
	// A reader of this output took the zeroes for the answer, which is exactly what
	// an unlabelled figure invites. The combined ladder is what the API serves as
	// `manipulationCostCombined` and what the band is judged on, so it leads.
	for _, m := range r.ManipulationCostCombined {
		fmt.Fprintf(w, "    cost   combined   delta %-5s target %s  cost %s  reachable %t\n",
			m.Delta, m.TargetPrice, m.Cost, m.Reachable)
	}
	for _, m := range r.ManipulationCostOrderbookOnly {
		fmt.Fprintf(w, "    cost   book only  delta %-5s target %s  cost %s  reachable %t\n",
			m.Delta, m.TargetPrice, m.Cost, m.Reachable)
	}
	fmt.Fprintf(w, "    maxReachablePrice %s  costToMaxReachablePrice %s\n",
		show(r.MaxReachablePrice), show(r.CostToMaxReachablePrice))
	if o := r.OracleResistance; o != nil {
		fmt.Fprintf(w, "    oracle  window %ds  genuine volume %s  cost %s  reachable %t  ratio %s  total %s\n",
			o.WindowSeconds, o.GenuineVolume, o.ManipulationCost, o.Reachable, show(o.Ratio), show(o.TotalAttackCost))
	}
	fmt.Fprintf(w, "    band %s (%s), flags %v\n", r.Band, r.BandConfidence, r.Flags)
	fmt.Fprintf(w, "    unevaluated %v\n", r.UnevaluatedFlags)
	for _, warn := range r.Warnings {
		fmt.Fprintf(w, "    warning: %s\n", warn)
	}
}

// show prints a nil decimal as "null" rather than as an empty string or a zero,
// because null and zero are different claims everywhere in this product.
func show(d *decimal.Decimal) string {
	if d == nil {
		return "null"
	}
	return d.String()
}

// reportResting prints the reconstructed book one OFFER per row, which the level
// view cannot: a level aggregates offers at one price and loses which offer is
// which. It exists so a disagreement with a hand computation can be taken to
// Horizon one offer at a time, by ID, as report section 5.4 needs.
//
// The price is printed as the n/d the operation result carried and never as a
// decimal, non-negotiable rule 5, and in the offer's own orientation: an ask sells
// the base, a bid sells the quote.
func reportResting(w io.Writer, offers []horizon.RestingOffer) {
	fmt.Fprintf(w, "  resting offers at the target: %d\n", len(offers))
	fmt.Fprintf(w, "  %-4s %-12s %-10s %-24s %-16s %-10s %s\n",
		"side", "offer_id", "seller", "price_r n/d", "amount", "last_ledger", "last_operation")
	for _, o := range offers {
		fmt.Fprintf(w, "  %-4s %-12d %-10s %-24s %-16s %-10d %d\n",
			o.Side, o.ID, shortAccount(o.Seller), fmt.Sprintf("%d/%d", o.PriceN, o.PriceD),
			o.Amount.StringFixed(7), o.LastLedger, o.LastOperation)
	}
}

// shortAccount is the first eight characters, the form every other line of this
// command's output names an account by.
func shortAccount(a string) string {
	if len(a) > 8 {
		return a[:8]
	}
	return a
}

// reportPoolsAt prints the reconstructed pools and the provenance of each, which
// is one effect a reader can open on Horizon.
//
// A pool that was absent at the target and one the walk could not reach are
// printed apart, because only the first is a measurement.
func reportPoolsAt(w io.Writer, pools []horizon.PoolAt, walk horizon.PoolHistoryWalk) {
	fmt.Fprintf(w, "  pools at the target: %d of %d listed, %d request(s)\n", walk.Resolved, walk.Listed, walk.Pages)
	for _, at := range pools {
		fmt.Fprintf(w, "    %s  base %s  quote %s  fee %d bp  from effect %s at %s (ledger %d)\n",
			at.Pool.PoolID[:8], at.Pool.ReserveBase, at.Pool.ReserveQuote, at.Pool.FeeBP,
			at.EffectID, at.EffectAt.UTC().Format(time.RFC3339), at.EffectLedger)
	}
	for _, id := range walk.Absent {
		fmt.Fprintf(w, "    %s  ABSENT at this ledger: no effect at or before it, or the last one removed the pool\n", id[:8])
	}
	for _, id := range walk.Unreached {
		fmt.Fprintf(w, "    %s  NOT REACHED within the page bound; pool coverage is unknown and -persist will refuse\n", id[:8])
	}
}
