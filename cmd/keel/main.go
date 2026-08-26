// Command keel is the single entrypoint for Keel.
//
// One binary, several subcommands, scheduled by an internal cron. No Kubernetes
// and no separate orchestrator; the deliverable is judged on verifiable evidence,
// not on infrastructure sophistication.
//
//	keel version    print the methodology version and exit
//	keel record     record raw Horizon snapshots for cross-validation
//	keel assets     declare and inspect the demonstration set
//	keel universe   build a candidate asset universe (proposes, never selects)
//	keel scan       compute metrics for every active asset, store them in Postgres
//	keel serve      run the read API
//	keel backtest   the trade-implied history of a pair, as CSV
//	keel replay     rebuild a pair's order book at a past ledger
//	keel crosscheck compare the recordings against rebuilt books, validation Layer 3
//	keel divergence measure book mid against pool spot across the demonstration set
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// belumSiap is the exit code for a subcommand that has a place but no body yet.
// It is distinct from exit code 1 (misuse) so that a scheduler can tell "not
// built yet" apart from "failed".
//
// EVERY SUBCOMMAND HAS A BODY AS OF 26 AUGUST 2026, and `belum`, the helper that
// printed the "not implemented yet" line and exited with this code, went with the
// last one. The constant stays because `scan` still uses it, for a case that is
// not "unbuilt" but reads the same way to a scheduler: a round where every asset
// panicked has nothing to store and looping against a Horizon budget to store
// nothing is not honest work. See decision 2 in the header of scan.go.
const belumSiap = 3

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch perintah := os.Args[1]; perintah {
	case "version":
		fmt.Printf("keel methodology %s\n", domain.MethodologyVersion)

	case "record":
		// The cross-validation recorder, and the first subcommand with a body.
		// It had to be first for the reason recorded here while it was still
		// empty: a comparison baseline cannot be created retroactively, so
		// every day of delay is evidence lost permanently. See record.go.
		if err := runRecord(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel record: %v\n", err)
			os.Exit(1)
		}

	case "assets":
		// The demonstration set. This one has a body because `scan` has nothing
		// to scan until the assets table is populated, and because a package
		// with no caller drifts; see the header of assets.go.
		if err := runAssets(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel assets: %v\n", err)
			os.Exit(1)
		}

	case "universe":
		// The candidate asset universe. It PROPOSES and does not SELECT: every
		// issuer of every code it is given comes back, verified or not, with no
		// inclusion criterion applied. Those criteria are
		// docs/methodology/02-pair-selection.md section 5 and are not this
		// binary's to hold. See the header of universe.go.
		if err := runUniverse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel universe: %v\n", err)
			os.Exit(1)
		}

	case "scan":
		// The wiring is written and the METHODOLOGY is not. Every function in
		// internal/domain/compute.go panics, so a scan reads a real book, opens a
		// real run row, and stores nothing. That state keeps exit code 3, which
		// means "not built yet" for every other subcommand here and means the same
		// thing for this one; a scan that genuinely broke still exits 1. See the
		// header of scan.go for why it is written before the thing it calls.
		if err := runScan(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel scan: %v\n", err)
			if errors.Is(err, errComputeNotBuilt) {
				os.Exit(belumSiap)
			}
			os.Exit(1)
		}

	case "serve":
		// The read-only API. It answers every endpoint in the contract today;
		// what it has no rows to return is metrics, because producing one needs
		// the red zone. See the header of serve.go.
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel serve: %v\n", err)
			os.Exit(1)
		}

	case "backtest":
		// The trade-implied history. It is the DEC-002 section 2 substitute for a
		// historical order book, built because `replay` below needs a source this
		// repository has decided not to reach for yet, and Deliverable 2 needs a
		// February 2026 series either way. See the header of backtest.go.
		if err := runBacktest(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel backtest: %v\n", err)
			os.Exit(1)
		}

	case "crosscheck":
		// Layer 3 of docs/methodology/10-validation.md, executed rather than
		// defined. It compares the committed recordings against books rebuilt from
		// Horizon today, and reports at the four depths that document names. See
		// the header of crosscheck.go.
		if err := runCrosscheck(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel crosscheck: %v\n", err)
			os.Exit(1)
		}

	case "replay":
		// The historical path, and it is NOT internal/hubble. DEC-002 section 2.3
		// specified reconstruction from operations and gated it behind "only
		// attempt this if 2.1 and 2.2 prove insufficient"; the measurement in
		// docs/evidences/2026-08-26-ustry-february-trades-implied.md is that they
		// are. This subcommand exited 3 until 26 August 2026. See replay.go.
		if err := runReplay(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel replay: %v\n", err)
			os.Exit(1)
		}

	case "divergence":
		// A MEASUREMENT and not a metric. Methodology 1.0.3 made case 1 of the
		// reference price ladder branch on a comparison, and that branch was
		// written from one market. This reads the live book and pools for every
		// pair in the list, asks internal/domain which rung each sits on, and
		// counts them. It states nothing about which branch is correct; see the
		// header of divergence.go.
		if err := runDivergence(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "keel divergence: %v\n", err)
			os.Exit(1)
		}

	case "help", "-h", "--help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "keel: unknown subcommand %q\n\n", perintah)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `keel - liquidity risk engine for the Stellar ecosystem

Usage:
  keel <subcommand>

Subcommands:
  version   print the methodology version
  record    record raw Horizon snapshots for cross-validation ("keel record -h")
  assets    declare and inspect the demonstration set ("keel assets -h")
  universe  build a candidate asset universe ("keel universe -h")
  scan      compute metrics for every active asset, store them in Postgres
  serve     run the read API ("keel serve -h")
  backtest  the trade-implied history of a pair, as CSV ("keel backtest -h")
  replay    rebuild a pair's order book at a past ledger ("keel replay -h")
  crosscheck compare the recordings against rebuilt books ("keel crosscheck -h")
  divergence measure book mid against pool spot per pair ("keel divergence -h")
`)
}
