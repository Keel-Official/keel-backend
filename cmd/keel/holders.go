// `keel holders`: pull the trustline holders of every asset in the demonstration
// set, compute section 2's concentration measures, and cache the result.
//
// WHY IT IS A SUBCOMMAND AND NOT PART OF `scan`. One reading costs 1 summary
// request plus 1 per page of 200 accounts, capped at 25 pages, so 26 requests per
// asset worst case. Sixty assets is up to 1,560 per round, and `scan` runs four
// rounds an hour: 6,240 requests against a budget of 3,000 and public Horizon's
// roughly 3,600 per IP. It does not fit, and the version that fits truncates every
// asset worth measuring. So this runs on its own cadence, once a day, and `scan`
// reads the newest row it wrote. migrations/0006_holder_readings.sql carries the
// same argument beside the table.
//
// THREE THINGS THIS COMMAND WILL NOT DO, each of them deliberate:
//
//  1. IT DOES NOT WORK AROUND A TRUNCATED READING. When the page cap stops a pull
//     before Horizon runs out of holders, domain.HolderConcentration returns
//     ErrHolderSetTruncated and this command stores the row with `truncated` set
//     and every figure absent. It does not fall back to the partial set. A
//     percentage over the 5,000 largest holders of an asset that has 40,000 is not
//     an approximate answer to the concentration question, it is an answer to a
//     different question that looks like the right one.
//
//  2. IT DOES NOT SKIP THE NATIVE ASSET SILENTLY. XLM has no trustlines, so
//     /accounts cannot enumerate its holders at all, and horizon.GetHolders says
//     so with ErrNativeHasNoTrustlines. That is reported as a skip with its reason
//     and is NOT counted as a failure, because a failure count that includes a
//     thing which can never succeed stops meaning anything.
//
//  3. IT DOES NOT COMPUTE ITS OWN SNAPSHOT LEDGER PROPERLY YET, AND IT SAYS SO IN
//     THE DATA. DEC-011 defines the snapshot ledger as the minimum latest_ledger
//     across ALL pages and requires counting mid-pull mutations to label a pull
//     atomic or mixed. internal/horizon/holders.go implements neither: it keeps
//     the header of the first and last page only, and horizon.Holder carries no
//     last_modified_ledger. So every row this command writes today carries
//     basis `first-and-last-page` and label `unknown`, and the day that
//     implementation lands the basis changes and old rows stay distinguishable.
//
// THE EXCLUSION LIST IS THE OPEN METHODOLOGICAL QUESTION HERE, and it is raised
// rather than buried. domain.HolderExclusions is explicit and asset-specific by
// design, and its own comment states the cost: run one asset's list against
// another asset and you get wrong numbers with no warning. A list exists for
// exactly one asset, USTRY, in internal/conformance. For the other sixty there is
// none, so this command removes the ISSUER and nothing else, and writes what it
// removed into every row. That is the honest minimum and it is not the same as
// correct: an asset whose pool or contract position holds supply under a G
// address would have that counted as a holder. Section 2 of
// docs/methodology/07-supporting-metrics.md is where the list belongs, and until
// it has one, `exclusions_applied` is what makes a row's population auditable.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func runHolders(args []string) error {
	fs := flag.NewFlagSet("holders", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dsn := fs.String("dsn", envOr(envDSN, store.DefaultDSN), "Postgres DSN, or set KEEL_DSN")
	baseURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "requests permitted per hour. Public Horizon allows about 3600 per IP")
	maxPages := fs.Int("max-pages", 0, "cap one reading at this many pages of 200 accounts; 0 uses the package default of 25")
	only := fs.String("asset", "", "pull one asset only, as CODE:ISSUER. Empty means every asset in the set")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel holders - pull trustline holders and cache the concentration

  keel holders                             every asset in the demonstration set
  keel holders -asset USTRY:GCRYUGD5...    one asset

Reads the asset list from the assets table, so `+"`keel assets -pairs`"+` has to have
run first. Writes one row per asset per snapshot into holder_readings, and opens a
run row of kind "holders" around the whole pass.

A reading that hit the page cap is stored WITH its truncated flag and WITHOUT any
concentration figure. That is not a failure and it is not zero: a truncated
trustline set answers the question not at all.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	s, err := openStore(ctx, *dsn)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	if applied, err := s.SchemaVersion(ctx); err != nil {
		return fmt.Errorf("holders: reading schema_migrations: %w\n  hint: run make migrate", err)
	} else if len(applied) == 0 {
		return errors.New("holders: no migrations are applied; run make migrate")
	}

	rows, err := s.Assets(ctx, true)
	if err != nil {
		return fmt.Errorf("holders: %w", err)
	}
	if len(rows) == 0 {
		return errors.New("holders: the demonstration set is empty; declare it with `keel assets -pairs <file>`")
	}

	targets, err := holderTargets(rows, *only)
	if err != nil {
		return fmt.Errorf("holders: %w", err)
	}
	if len(targets) == 0 {
		return errors.New("holders: no non-native asset matched")
	}

	client := horizon.NewClient(horizon.Config{
		BaseURL:        *baseURL,
		Budget:         *budget,
		MaxHolderPages: *maxPages,
		// No cache. A second reading that is identical because a body was reused
		// says nothing about the trustline set, which is the recorder's argument
		// and applies unchanged here.
		CacheTTL: 0,
	})

	runID, err := s.StartRun(ctx, store.RunHolders, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("holders: %w", err)
	}

	logger.Printf("%d asset(s), methodology %s, page cap %s",
		len(targets), domain.MethodologyVersion, pageCapLabel(*maxPages))

	var ok, failed, skipped, truncated, stored, alreadyThere, disagreed int

	for _, a := range targets {
		if err := ctx.Err(); err != nil {
			break
		}

		obs, err := client.GetHolders(ctx, a)
		if err != nil {
			// The native asset is the one case that cannot ever succeed. See
			// decision 2 in this file's header.
			if errors.Is(err, horizon.ErrNativeHasNoTrustlines) {
				skipped++
				logger.Printf("skip  %s: no trustlines exist for the native asset", a)
				continue
			}
			failed++
			logger.Printf("FAIL  %s: %v", a, err)
			continue
		}

		reading, err := readingFrom(obs, runID, time.Now().UTC())
		if err != nil {
			failed++
			logger.Printf("FAIL  %s: %v", a, err)
			continue
		}

		id, inserted, err := s.SaveHolderReading(ctx, reading)
		if err != nil {
			failed++
			logger.Printf("FAIL  %s: storing: %v", a, err)
			continue
		}
		ok++
		if reading.Truncated {
			truncated++
		}
		switch {
		case !inserted:
			// The snapshot was already recorded, so nothing was written. That is
			// normal. What is NOT normal is the stored row and this pull
			// disagreeing about whether the set was truncated, because the two
			// then answer different questions under one key. Decision 2 in
			// store.go forbids resolving that by overwriting, so it is reported.
			alreadyThere++
			prior, readErr := s.HolderReadingAt(ctx, a, reading.SnapshotLedger, domain.MethodologyVersion)
			if readErr == nil && prior.Truncated != reading.Truncated {
				disagreed++
				logger.Printf("SKIP  %s ledger %d, already stored, AND THE TWO PULLS DISAGREE: "+
					"stored truncated=%v, this pull truncated=%v. Nothing was overwritten, and that is a finding",
					a, reading.SnapshotLedger, prior.Truncated, reading.Truncated)
				break
			}
			logger.Printf("skip  %s ledger %d, already stored", a, reading.SnapshotLedger)
		case reading.Truncated:
			stored++
			logger.Printf("store %s ledger %d TRUNCATED at %d of %d holders, no figures -> id=%d",
				a, reading.SnapshotLedger, reading.HoldersRead, reading.HolderCountReported, id)
		default:
			stored++
			logger.Printf("store %s ledger %d pop=%d top1=%s%% top10=%s%% hhi=%s -> id=%d",
				a, reading.SnapshotLedger, deref(reading.Population),
				show(reading.Top1Pct), show(reading.Top10Pct), show(reading.HHI), id)
		}
	}

	// The notes describe the PULLS, not the rows written: a pull that hit the cap
	// is a fact about this pass even when its row was already there.
	var notes string
	switch {
	case truncated > 0 && disagreed > 0:
		notes = fmt.Sprintf("%d of %d pull(s) hit the page cap; %d collided with a stored row that disagreed about truncation",
			truncated, ok, disagreed)
	case truncated > 0:
		notes = fmt.Sprintf("%d of %d pull(s) hit the page cap and carry no concentration figures", truncated, ok)
	case disagreed > 0:
		notes = fmt.Sprintf("%d pull(s) collided with a stored row that disagreed about truncation", disagreed)
	}
	if err := s.FinishRun(ctx, runID, time.Now().UTC(), ok, failed, notes); err != nil {
		return fmt.Errorf("holders: %w", err)
	}

	logger.Printf("pass: %d ok (%d written, %d already stored), %d truncated pull(s), %d disagreement(s), %d failed, %d skipped, %d requests this window",
		ok, stored, alreadyThere, truncated, disagreed, failed, skipped, client.Requests())
	return nil
}

// holderTargets is every DISTINCT non-native asset in the set, sorted.
//
// Distinct because an asset quoted against two pairs is one asset and one
// trustline set, and paying for it twice spends the budget on an answer already
// given. Sorted because non-negotiable rule 2 says so, and because a pass that
// reports a different asset first on every run is harder to read than one that
// does not. verifyAssets in scan.go does the same thing for the same reasons.
func holderTargets(rows []store.Asset, only string) ([]domain.Asset, error) {
	seen := map[string]domain.Asset{}
	for _, r := range rows {
		for _, a := range []domain.Asset{r.Base, r.Quote} {
			if a.IsNative() {
				continue
			}
			seen[a.String()] = a
		}
	}

	if only != "" {
		a, found := seen[only]
		if !found {
			return nil, fmt.Errorf("asset %q is not in the demonstration set; `keel assets -list` shows what is", only)
		}
		return []domain.Asset{a}, nil
	}

	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]domain.Asset, 0, len(keys))
	for _, k := range keys {
		out = append(out, seen[k])
	}
	return out, nil
}

// readingFrom turns one observation into the row that records it.
//
// It computes the concentration here rather than at read time so that the figures
// and the evidence they came from are written in one transaction and can never
// drift apart. The exclusions are the issuer alone; the file header says why, and
// says why that is a minimum rather than a correct answer.
func readingFrom(obs horizon.HolderObservation, runID int64, now time.Time) (store.HolderReading, error) {
	excl := domain.HolderExclusions{Issuer: obs.Asset.Issuer}

	balances := make([]domain.HolderBalance, 0, len(obs.Holders))
	for _, h := range obs.Holders {
		balances = append(balances, domain.HolderBalance{AccountID: h.AccountID, Balance: h.Balance})
	}

	snapshot, span := snapshotFrom(obs.Raw.FirstLedger, obs.Raw.LastLedger)

	r := store.HolderReading{
		Asset:     obs.Asset,
		RunID:     &runID,
		FetchedAt: now,

		SnapshotLedger: snapshot,
		// DEC-011's definition is not implemented upstream yet, so this records
		// which definition DID produce the number. Decision 3 in the header.
		SnapshotBasis: store.BasisFirstAndLastPage,
		LedgerSpan:    span,
		// Not atomic. Nothing counted the mutated rows, and claiming the value a
		// reader trusts most without having measured it is the failure this
		// column exists to prevent.
		SnapshotLabel: store.LabelUnknown,

		HoldersRead:         len(obs.Holders),
		HolderCountReported: obs.HolderCount,
		Truncated:           obs.Truncated(),

		Exclusions: store.HolderExclusionsJSON{Issuer: obs.Asset.Issuer},

		MethodologyVersion: domain.MethodologyVersion,
	}

	stats, err := domain.HolderConcentration(balances, excl, obs.Truncated())
	switch {
	case errors.Is(err, domain.ErrHolderSetTruncated):
		// The row is stored, and it is stored empty. Not an error to the caller.
		return r, nil
	case errors.Is(err, domain.ErrHolderSetEmpty):
		// Every holder was excluded or held nothing, so the denominator is zero
		// and the percentages are undefined rather than zero. Stored the same way
		// a truncated reading is: the pull happened, and it answered nothing.
		return r, nil
	case err != nil:
		return store.HolderReading{}, fmt.Errorf("concentration: %w", err)
	}

	top1, top10, hhi := stats.Top1Pct, stats.Top10Pct, stats.HHI
	supply := stats.CirculatingSupply
	pop, zero, dropped := stats.Population, stats.ZeroBalanceDropped, stats.ExcludedDropped

	r.Top1Pct, r.Top10Pct, r.HHI = &top1, &top10, &hhi
	r.CirculatingSupply = &supply
	r.Population, r.ZeroBalanceDropped, r.ExcludedDropped = &pop, &zero, &dropped
	return r, nil
}

// snapshotFrom picks the ledger a pull is stamped with, and the span it covered.
//
// DEC-011 wants the minimum across every page; only the first and last are
// recorded upstream today, so the minimum of those two is what is available. The
// MIN and not the first: taking the earlier of the two is the conservative
// direction, because a figure attributed to an earlier ledger understates how
// current it is rather than overstating it.
func snapshotFrom(first, last uint32) (snapshot uint32, span int) {
	lo, hi := first, last
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo, int(hi - lo)
}

func pageCapLabel(n int) string {
	if n <= 0 {
		return "default"
	}
	return fmt.Sprintf("%d page(s)", n)
}

func deref(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}
