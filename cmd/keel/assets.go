// The `assets` subcommand: declare and inspect the demonstration set.
//
// It exists because the assets table has to be populated before `scan` has
// anything to scan, and because internal/store otherwise had no caller. An
// unimported package in this repository is not a neutral state: internal/adapter
// sat unimported for months, drifted, used float64 in two places, and was
// eventually deleted. That was finding P1-18.
//
// The pair file is the same one `record` reads. One declaration of the
// demonstration set, two consumers.
//
// AND THE TWO CONSUMERS DISAGREED ABOUT THE NATIVE ASSET UNTIL 26 AUGUST 2026.
// horizon.LoadPairs requires a native asset to carry NO code, because on-chain it
// has none and because every Horizon path branches on Asset.IsNative(), which
// reads Type and never Code. internal/store requires a native asset to carry the
// code "XLM", because the assets table has code NOT NULL and its unique constraint
// is what stops the native asset being stored twice under two spellings.
//
// Both rules are right inside their own layer, and no value in the pair file
// satisfies both: `keel record` had been reading configs/recorder-pairs.json for
// days while `keel assets -pairs` on the same file failed on pair 0. So the
// conversion happens HERE, at the boundary where a pair-file asset becomes a
// stored asset, and neither layer has to give up its rule. See forStore below.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func runAssets(args []string) error {
	fs := flag.NewFlagSet("assets", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	pairsPath := fs.String("pairs", "", "declare every pair in this file, then list. Copy scripts/record-pairs.example.json")
	list := fs.Bool("list", false, "list the demonstration set and exit")
	all := fs.Bool("all", false, "with -list, include deactivated pairs")
	dsn := fs.String("dsn", envOr("KEEL_DSN", store.DefaultDSN), "Postgres DSN, or set KEEL_DSN")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel assets - declare and inspect the demonstration set

  keel assets -list                       what is in the set
  keel assets -list -all                  including deactivated pairs
  keel assets -pairs my-pairs.json        declare every pair in the file

Declaring is idempotent: a pair already present keeps its id, and a re-run with
no note does not erase the note already recorded. Nothing is ever deleted, only
deactivated, because metrics rows reference these ids.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pairsPath == "" && !*list {
		fs.Usage()
		return errors.New("assets: pass -pairs or -list")
	}

	ctx := context.Background()
	s, err := store.Open(ctx, *dsn)
	if err != nil {
		// The most common cause on a developer machine is a DSN naming 5432,
		// which is a second Postgres and not the one docker-compose started, so
		// the hint is here rather than in a document nobody reads at the moment
		// it fails.
		return fmt.Errorf("%w\n  hint: `make up && make migrate` first. The container publishes 5433, not 5432, "+
			"so a DSN still naming 5432 reaches whatever else is on the host", err)
	}
	defer func() { _ = s.Close() }()

	if applied, err := s.SchemaVersion(ctx); err != nil {
		return fmt.Errorf("assets: reading schema_migrations: %w\n  hint: run make migrate", err)
	} else if len(applied) == 0 {
		return errors.New("assets: no migrations are applied; run make migrate")
	}

	if *pairsPath != "" {
		pairs, err := horizon.LoadPairs(*pairsPath)
		if err != nil {
			return fmt.Errorf("assets: %w", err)
		}
		for _, p := range pairs {
			id, err := s.UpsertAsset(ctx, forStore(p.Base), forStore(p.Quote), p.Note)
			if err != nil {
				return fmt.Errorf("assets: %w", err)
			}
			fmt.Printf("declared  id=%-4d %s\n", id, p)
		}
	}

	rows, err := s.Assets(ctx, !*all)
	if err != nil {
		return fmt.Errorf("assets: %w", err)
	}
	if len(rows) == 0 {
		fmt.Println("the demonstration set is empty; declare it with -pairs")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tPAIR\tACTIVE\tWHY IT IS IN THE SET")
	for _, a := range rows {
		note := a.SelectionNote
		if note == "" {
			note = "(no reason recorded)"
		}
		_, _ = fmt.Fprintf(w, "%d\t%s/%s\t%t\t%s\n", a.ID, a.Base, a.Quote, a.Active, note)
	}
	return w.Flush()
}

// forStore gives the native asset the code the assets table needs, and leaves
// every other asset exactly as the pair file declared it.
//
// "XLM" is not chosen here. It is what domain.Asset.String() returns for a native
// asset and what the API contract uses, and internal/store/assets.go names it in
// the error this function exists to prevent. Type is untouched, so IsNative() and
// therefore every Horizon path keep answering the same way they did before.
//
// The rejected alternative was to write "XLM" into configs/recorder-pairs.json.
// It fails on the same run: horizon.LoadPairs refuses a native asset that carries
// a code, so the file would then be readable by `keel assets` and rejected by
// `keel record`. Moving the problem is not fixing it, and a conversion at a
// boundary is what a boundary is for.
func forStore(a domain.Asset) domain.Asset {
	if a.IsNative() && a.Code == "" {
		a.Code = "XLM"
	}
	return a
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
