// The `layer1` subcommand: the engine's figures for a book that was RECORDED,
// printed so they can be set beside a Layer 1 hand recomputation.
//
// docs/methodology/10-validation.md section 1 defines Layer 1: transcribe a raw
// order book, compute the methodology by hand, compare against engine output. The
// last four words had no instrument. `keel replay` prints the same quantities but
// REBUILDS the book from operations first, so a disagreement could be the
// reconstruction rather than the formula, and the reconstruction is what Layer 3
// tests. This command reads the recorded bytes and computes over exactly them.
//
// IT DECODES THROUGH THE SAME FUNCTIONS THE LIVE PATH USES, ParseOrderBook and
// ParsePools, for the reason ParseOrderBook's own header gives: two decoders that
// agree today are two decoders that disagree the first time one is corrected.
//
// THE ORDERING RULE IS WHY THIS COMMAND REFUSES TO RUN BY DEFAULT, and the refusal
// is the whole design of it rather than a courtesy. A hand recomputation is
// evidence only if it was worked out BEFORE its answer was visible. Nothing in a
// permission layer can tell whether a number was computed before or after the code
// that satisfies it, which DEC-008 says in those words about compute.go. What a
// command CAN do is make the ordering deliberate: -after-writing names the file in
// testdata/manual/ that already holds the hand figures, the command checks it
// exists and is not empty, and only then prints. Anyone who wants the numbers
// early can still get them from `keel scan` or by editing this file, so this stops
// an accident and not a determined shortcut. That is the honest claim for it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
)

func runLayer1(args []string) error {
	fs := flag.NewFlagSet("layer1", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	recording := fs.String("recording", "", "path to a schema 2 recording, required")
	afterWriting := fs.String("after-writing", "",
		"path to the hand recomputation this will be compared against, required. It must exist and be "+
			"non-empty, which is what makes the ordering deliberate rather than assumed")
	bidUnit := fs.String("bid-amount-unit", string(horizon.BidAmountUnitQuote),
		"which asset an order book bid amount is denominated in: quote or base. See trap 5 in internal/horizon/CLAUDE.md")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel layer1 - the engine's figures for a RECORDED book, for Layer 1 comparison

Reads one recording, decodes the order book and the liquidity pools out of the
bytes it holds, runs the methodology over them, and prints every quantity a hand
recomputation produces. No network request is made and nothing is reconstructed:
the figures are computed over exactly the bytes that were recorded.

WRITE YOUR NUMBERS FIRST. This command requires -after-writing to name the file in
testdata/manual/ that already holds them. A hand recomputation is evidence only if
it was worked out before its answer was visible, and a Layer 1 figure produced
after reading this output tests nothing at all: it is the engine agreeing with
itself through a spreadsheet.

Where this output and your file disagree, THE DISAGREEMENT IS THE FINDING. Adjust
the code to match your numbers. Never adjust your numbers to match the code.
docs/methodology/10-validation.md section 1, and the same rule is written in
internal/conformance/fixture.go and in the CLAUDE.md zone map.

`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *recording == "" || *afterWriting == "" {
		fs.Usage()
		return errors.New("layer1: -recording and -after-writing are both required")
	}
	unit := horizon.BidAmountUnit(*bidUnit)
	if unit != horizon.BidAmountUnitQuote && unit != horizon.BidAmountUnitBase {
		return fmt.Errorf("layer1: -bid-amount-unit must be %q or %q",
			horizon.BidAmountUnitQuote, horizon.BidAmountUnitBase)
	}

	if err := handFiguresExist(*afterWriting); err != nil {
		return err
	}

	snap, err := snapshotFromRecording(*recording, unit)
	if err != nil {
		return err
	}

	fmt.Printf("Layer 1 comparison for %s/%s at ledger %d\n", snap.Base, snap.Quote, snap.LedgerSeq)
	fmt.Printf("  recording:   %s\n", *recording)
	fmt.Printf("  your figures: %s\n", *afterWriting)
	fmt.Printf("  book: %d bid level(s), %d ask level(s); pools: %d\n\n",
		len(snap.Book.Bids), len(snap.Book.Asks), len(snap.Pools))
	heading := "over the RECORDED book"
	if len(snap.Pools) > 0 {
		heading += fmt.Sprintf(" and its %d recorded pool(s), combined", len(snap.Pools))
	} else {
		heading += ", and Horizon returned no pool for this pair"
	}
	reportRiskUnder(os.Stdout, snap, heading)
	fmt.Print(`
Compare every line above against your file. Where they disagree, report the
disagreement rather than resolving it by editing either side.
`)
	return nil
}

// handFiguresExist refuses an empty or missing hand recomputation.
//
// It asserts nothing about the CONTENT, and cannot: whether the figures inside
// were worked out by hand is not a property any check can read. What it asserts is
// that they were written down before this output was seen, which is the one half
// of the ordering rule a file system can carry.
func handFiguresExist(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("layer1: -after-writing %s: %w\n"+
			"Write your hand figures first. This command exists to be read AFTER they are on disk", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("layer1: -after-writing %s is a directory; name the recomputation file itself", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("layer1: -after-writing %s is empty", path)
	}
	return nil
}

// snapshotFromRecording builds the snapshot out of recorded bytes and nothing else.
func snapshotFromRecording(path string, unit horizon.BidAmountUnit) (domain.Snapshot, error) {
	var out domain.Snapshot

	tick, err := horizon.ReadTick(path)
	if err != nil {
		return out, fmt.Errorf("layer1: %w", err)
	}
	base, quote, err := pairFromTick(tick)
	if err != nil {
		return out, fmt.Errorf("layer1: %w", err)
	}
	book, err := recordedBook(tick, base, quote, unit)
	if err != nil {
		return out, fmt.Errorf("layer1: %w", err)
	}

	// The pool body is optional in a recording only in the sense that a pair may
	// have none. A recording that carries the response and fails to decode it is
	// an error rather than a pair without a pool, because those two states differ
	// by whether AMM liquidity is missing from the depth figure.
	var pools []domain.PoolReserves
	for _, src := range tick.Sources {
		if src.Endpoint != "liquidity_pools" {
			continue
		}
		pools, err = horizon.ParsePools([]byte(src.Body), base, quote)
		if err != nil {
			return out, fmt.Errorf("layer1: pools: %w", err)
		}
	}

	return domain.Snapshot{
		Base:      base,
		Quote:     quote,
		LedgerSeq: tick.LedgerBefore,
		Book:      book,
		Pools:     pools,
		Source:    domain.DataSourceHorizon,
	}, nil
}
