// Command keel is the single entrypoint for Keel.
//
// One binary, several subcommands, scheduled by an internal cron. No Kubernetes
// and no separate orchestrator; the deliverable is judged on verifiable evidence,
// not on infrastructure sophistication.
//
//	keel version    print the methodology version and exit
//	keel record     record raw Horizon snapshots for cross-validation
//	keel assets     declare and inspect the demonstration set
//	keel scan       compute metrics for every active asset, store them in Postgres
//	keel serve      run the read API
//	keel replay     replay a ledger range through the historical adapter
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

	case "replay":
		belum(perintah, "internal/hubble; deferred, see docs/decisions/DEC-002-hold-bigquery.md")

	case "help", "-h", "--help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "keel: unknown subcommand %q\n\n", perintah)
		usage()
		os.Exit(1)
	}
}

func belum(perintah, butuh string) {
	fmt.Fprintf(os.Stderr, "keel %s: not implemented yet (needs %s)\n", perintah, butuh)
	os.Exit(belumSiap)
}

func usage() {
	fmt.Fprint(os.Stderr, `keel - liquidity risk engine for the Stellar ecosystem

Usage:
  keel <subcommand>

Subcommands:
  version   print the methodology version
  record    record raw Horizon snapshots for cross-validation ("keel record -h")
  assets    declare and inspect the demonstration set ("keel assets -h")
  scan      compute metrics for every active asset, store them in Postgres
  serve     run the read API ("keel serve -h")
  replay    replay a ledger range through the historical adapter
`)
}
