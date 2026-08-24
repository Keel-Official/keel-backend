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
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

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
		// The most common cause on a developer machine is a second Postgres on
		// 5432 that is not the one docker-compose started, so the hint is here
		// rather than in a document nobody reads at the moment it fails.
		return fmt.Errorf("%w\n  hint: `make up && make migrate` first, and check that port 5432 is the container's "+
			"Postgres and not another one already running locally", err)
	}
	defer s.Close()

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
			id, err := s.UpsertAsset(ctx, p.Base, p.Quote, p.Note)
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
	fmt.Fprintln(w, "ID\tPAIR\tACTIVE\tWHY IT IS IN THE SET")
	for _, a := range rows {
		note := a.SelectionNote
		if note == "" {
			note = "(no reason recorded)"
		}
		fmt.Fprintf(w, "%d\t%s/%s\t%t\t%s\n", a.ID, a.Base, a.Quote, a.Active, note)
	}
	return w.Flush()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
