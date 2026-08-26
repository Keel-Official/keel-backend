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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
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
	quiet := fs.Bool("quiet", false, "do not print one progress line per account walked")
	compute := fs.Bool("compute", false,
		"run the methodology over the reconstructed book and print the result. ORDER BOOK ONLY, because no pool is reconstructed, so a combined depth figure from it would be wrong")
	out := fs.String("out", "", "write the reconstructed snapshot to this file as JSON. Optional")
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

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pairsPath == "" {
		return errors.New("replay: -pairs is required")
	}
	if *ledger == 0 {
		return errors.New("replay: -ledger is required")
	}

	pairs, err := horizon.LoadPairs(*pairsPath)
	if err != nil {
		return fmt.Errorf("replay: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := horizon.NewClient(horizon.Config{BaseURL: *baseURL, Budget: *budget})

	snapshots := make([]domain.Snapshot, 0, len(pairs))
	for _, p := range pairs {
		walked := 0
		res, err := client.ReconstructBook(ctx, p.Base, p.Quote, horizon.ReplayQuery{
			TargetLedger:       uint32(*ledger),
			TradesFromLedger:   uint32(*tradesFrom),
			TradeLookahead:     uint32(*lookahead),
			SinceLedger:        uint32(*since),
			MaxPagesPerAccount: *maxPages,
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
		if *compute {
			reportRisk(os.Stdout, res.Snapshot)
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
func reportRisk(w *os.File, s domain.Snapshot) {
	r, err := domain.ComputeAssetRisk(s, domain.DefaultParams())
	if err != nil {
		fmt.Fprintf(w, "  compute: %v\n", err)
		return
	}
	fmt.Fprintf(w, "  --- methodology %s over the reconstructed book, ORDER BOOK ONLY, no pool ---\n", r.MethodologyVersion)
	fmt.Fprintf(w, "    P0 %s from %s", show(r.MidPrice), r.PriceSource)
	if r.SpreadPct != nil {
		fmt.Fprintf(w, ", spread %s percent", r.SpreadPct.StringFixed(7))
	}
	fmt.Fprintln(w)

	for _, d := range r.Depth {
		fmt.Fprintf(w, "    depth  delta %-5s buy %s  sell %s\n", d.Delta, d.BuySide, d.SellSide)
	}
	for _, m := range r.ManipulationCostOrderbookOnly {
		fmt.Fprintf(w, "    cost   delta %-5s target %s  cost %s  reachable %t\n",
			m.Delta, m.TargetPrice, m.Cost, m.Reachable)
	}
	fmt.Fprintf(w, "    maxReachablePrice %s  costToMaxReachablePrice %s\n",
		show(r.MaxReachablePrice), show(r.CostToMaxReachablePrice))
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
