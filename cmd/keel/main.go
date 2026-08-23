// Command keel is the single entrypoint for Keel.
//
// One binary, several subcommands, scheduled by an internal cron. No Kubernetes
// and no separate orchestrator; the deliverable is judged on verifiable evidence,
// not on infrastructure sophistication.
//
//	keel version    print the methodology version and exit
//	keel record     record raw Horizon snapshots for cross-validation
//	keel scan       compute metrics for every active asset, store them in Postgres
//	keel serve      run the read API
//	keel replay     replay a ledger range through the historical adapter
package main

import (
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
		// The cross-validation recorder. This one must start running BEFORE the
		// historical path, because a comparison baseline cannot be created
		// retroactively. Every day of delay is evidence lost permanently.
		belum(perintah, "internal/horizon + a recorder writing to recordings/")

	case "scan":
		belum(perintah, "internal/horizon + internal/depth + internal/store")

	case "serve":
		belum(perintah, "internal/api + internal/store")

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
  record    record raw Horizon snapshots for cross-validation
  scan      compute metrics for every active asset, store them in Postgres
  serve     run the read API
  replay    replay a ledger range through the historical adapter
`)
}
