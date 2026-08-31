// The same-hour cross-check: every recording paired with a rebuild of the same
// book inside the hour the recording was taken.
//
// WHAT IT EXISTS TO TEST, and it is one sentence in
// docs/methodology/10-validation.md that this file is the instrument for. The
// first Layer 3 run, on 26 August 2026, compared the sixty committed recordings
// against books rebuilt about seven hours later and returned 37 MATCH, 0
// MISMATCH and 23 PARTIAL. Those 23 were attributed to the seven hours. That
// attribution is plausible, it is written down in three places as though it were
// established, and NOTHING HAS TESTED IT: every row in that run had the same
// delay, so the run holds no contrast at all. A hypothesis with one arm is a
// belief.
//
// The second arm is a batch recorded and compared at a short delay. This file
// schedules that comparison from the recorder itself, so the pairing is made of
// the exact file that was just written rather than of whatever a later sweep
// happens to find on disk.
//
// THE DELAY IS THE INDEPENDENT VARIABLE AND IT IS NAMED. defaultCrosscheckDelay
// below is the default, maxCrosscheckDelay is the ceiling that makes the words
// "same hour" a property of the code rather than of somebody's intention, and
// both are stated in the output at startup. A magic number here would be the
// experiment's own variable hidden in a literal.
//
// WHAT THIS FILE MUST NOT DO, and the whole result depends on it: it does not
// touch the comparison. compareRecording and everything it calls are exactly as
// they were. Two runs at two delays are only comparable if the delay is the only
// thing that differs between them, so this file schedules, stamps and writes, and
// asks crosscheck.go the same question it has always asked.
package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Keel-Official/keel-backend/internal/horizon"
)

// defaultCrosscheckDelay is how long a recording waits before the rebuild that
// compares against it, when -crosscheck-after is not given.
//
// FIVE MINUTES AND NOT ZERO. Zero is available and is a legitimate setting, but it
// is a poor default for the thing being measured: a rebuild started in the same
// second as the recording asks the live book whether it has moved since a ledger
// that has barely closed, and a delay short enough that the answer is trivially
// yes tests the delay rather than the reconstruction. Five minutes is roughly
// twenty-five ledgers, so offers have had real opportunity to move, and it is two
// orders of magnitude below the seven hours whose effect is in question.
//
// IT IS ALSO SHORT ENOUGH TO SIT INSIDE ONE HOURLY RECORDER ROUND, which matters
// for the workflow case: a delay longer than the recording interval leaves every
// round's comparisons still queued when the next round starts.
const defaultCrosscheckDelay = 5 * time.Minute

// maxCrosscheckDelay is the ceiling on -crosscheck-after, exclusive.
//
// It is not a safety limit, it is the definition of the thing. The task this file
// answers is a comparison made INSIDE THE HOUR the recording was taken; a delay of
// an hour or more is the run that already exists and does not need this scheduler.
// Enforcing it here means the claim "same hour" is checkable from the flags rather
// than from a promise.
const maxCrosscheckDelay = time.Hour

// pendingRebuild is one written recording waiting for its rebuild.
type pendingRebuild struct {
	Path       string
	Pair       string
	RecordedAt time.Time
	Due        time.Time
}

// sameHourRunner records rounds and compares each recording once its delay has
// passed.
//
// THE QUEUE IS A SLICE AND THERE IS NO MAP IN THIS FILE. Non-negotiable rule 2
// says map keys are sorted before iteration; the cheaper way to obey it is to hold
// no map, and a queue that is scanned for its earliest entry needs none. Two
// recordings of the same pair at different ledgers are two separate entries, which
// a map keyed by pair would have silently collapsed into one.
type sameHourRunner struct {
	rec   *horizon.Recorder
	delay time.Duration
	now   func() time.Time
	out   io.Writer
	logf  func(format string, args ...any)
	csv   *crosscheckAppender

	// compare is the comparison step. It is a field rather than a direct call so
	// that the schedule can be tested without Horizon: what is worth testing here
	// is when a rebuild is run and against which file, and driving a real rebuild
	// to find that out would test the network instead.
	compare func(ctx context.Context, path string) crosscheckRow

	queue []pendingRebuild
	rows  []crosscheckRow
}

// sameHourConfig is what runRecord hands over.
type sameHourConfig struct {
	Recorder *horizon.Recorder
	Client   *horizon.Client
	Unit     horizon.BidAmountUnit
	Delay    time.Duration
	Out      io.Writer
	Logf     func(format string, args ...any)
	CSV      *crosscheckAppender
}

func newSameHourRunner(cfg sameHourConfig) *sameHourRunner {
	r := &sameHourRunner{
		rec:   cfg.Recorder,
		delay: cfg.Delay,
		now:   time.Now,
		out:   cfg.Out,
		logf:  cfg.Logf,
		csv:   cfg.CSV,
	}
	r.compare = func(ctx context.Context, path string) crosscheckRow {
		return compareOne(ctx, cfg.Client, path, cfg.Unit, r.now)
	}
	return r
}

// enqueue schedules the rebuild for every recording a round actually wrote.
//
// The due time is computed from the recording's OWN recorded_at rather than from
// the clock now. The two differ by however long the round took, and the quantity
// under test is the gap since the reading, not the gap since the round ended.
func (r *sameHourRunner) enqueue(results []horizon.TickResult) {
	now := r.now().UTC()
	for _, res := range results {
		if !res.Written || res.Path == "" {
			continue
		}
		p := pendingRebuild{Path: res.Path, Pair: res.Pair.String()}
		at, err := time.Parse(time.RFC3339, res.Tick.RecordedAt)
		if err != nil {
			// The recorder formatted this stamp seconds ago, so reaching here
			// means something is wrong upstream. The comparison is still worth
			// running, so the schedule falls back to the clock and the row will
			// report its gap as unknown rather than as a number nobody can trust.
			r.logf("crosscheck: %s carries an unreadable recorded_at %q, scheduling from now",
				res.Path, res.Tick.RecordedAt)
			p.Due = now.Add(r.delay)
		} else {
			p.RecordedAt = at.UTC()
			p.Due = p.RecordedAt.Add(r.delay)
		}
		r.queue = append(r.queue, p)
		r.logf("crosscheck: %s queued, rebuild due %s (%s after recording)",
			p.Pair, p.Due.Format(time.RFC3339), r.delay)
	}
}

// earliest returns the index of the entry due soonest, or -1 when the queue is
// empty. A linear scan over a queue that holds one round of pairs, which is eight
// in the workflow and sixty at its widest.
func (r *sameHourRunner) earliest() int {
	best := -1
	for i := range r.queue {
		if best < 0 || r.queue[i].Due.Before(r.queue[best].Due) {
			best = i
		}
	}
	return best
}

// untilNextDue is how long until the next rebuild is due, and false when nothing
// is queued. A negative wait reads as zero: an entry that came due while a
// previous comparison was running is due now, not overdue by a negative amount.
func (r *sameHourRunner) untilNextDue() (time.Duration, bool) {
	i := r.earliest()
	if i < 0 {
		return 0, false
	}
	d := r.queue[i].Due.Sub(r.now().UTC())
	if d < 0 {
		d = 0
	}
	return d, true
}

// drain runs every comparison that has come due, earliest first.
//
// An entry is removed from the queue BEFORE its comparison runs, so a rebuild that
// fails is not retried in a loop. A failed comparison is a row with an error in it,
// which is a result; retrying it against a book that has moved further would be a
// different experiment run under the same name.
func (r *sameHourRunner) drain(ctx context.Context) {
	for ctx.Err() == nil {
		i := r.earliest()
		if i < 0 || r.now().UTC().Before(r.queue[i].Due) {
			return
		}
		p := r.queue[i]
		r.queue = append(r.queue[:i:i], r.queue[i+1:]...)
		r.emit(r.compare(ctx, p.Path))
	}
}

// emit keeps one row and writes it out.
func (r *sameHourRunner) emit(row crosscheckRow) {
	r.rows = append(r.rows, row)
	fmt.Fprintf(r.out, "[%3d] %s\n", len(r.rows), row.line())
	if r.csv == nil {
		return
	}
	// A CSV that cannot be written is logged and does not stop the recorder. The
	// recording is the evidence that cannot be recreated; the comparison can be
	// run again from the file on disk, at a longer gap, which is worse but is not
	// nothing.
	if err := r.csv.append(row); err != nil {
		r.logf("crosscheck: could not write the CSV row for %s: %v", row.Path, err)
	}
}

// waitDraining blocks until wake fires or the context is done, running comparisons
// as they come due. A nil wake means nothing else is coming, so it returns once the
// queue is empty, which is the one-shot case.
func (r *sameHourRunner) waitDraining(ctx context.Context, wake <-chan time.Time) error {
	for {
		r.drain(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}

		var timer *time.Timer
		var next <-chan time.Time
		if d, ok := r.untilNextDue(); ok {
			timer = time.NewTimer(d)
			next = timer.C
		}
		if next == nil && wake == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			stopTimer(timer)
			return ctx.Err()
		case <-wake:
			stopTimer(timer)
			return nil
		case <-next:
			// Something came due. Loop and drain it.
		}
	}
}

func stopTimer(t *time.Timer) {
	if t != nil {
		t.Stop()
	}
}

// runOnce is the one-shot path: one round, then its comparisons when they come
// due, then stop. It returns the round's tally the way recordOneRound does.
func (r *sameHourRunner) runOnce(ctx context.Context, results []horizon.TickResult) error {
	r.enqueue(results)
	return r.waitDraining(ctx, nil)
}

// run records on every interval and compares on every due time, in one goroutine.
//
// IT DOES NOT CALL horizon.Recorder.Run, and that is the only reason this loop
// exists rather than reusing it. Run reports a round as a count of failures and
// keeps the paths to itself, and a pairing that cannot name the file that was just
// written is a sweep of a directory, not a pairing. The two loops agree on the part
// that matters: record immediately, then on the tick, because a recorder that waits
// an interval before its first write loses that interval permanently.
//
// One goroutine and no locking. The comparisons happen between rounds rather than
// beside them, which costs a round that is late by however long its predecessor's
// comparisons took, and buys a Horizon request budget that only one thing is
// spending at a time.
func (r *sameHourRunner) run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("crosscheck: interval must be positive, got %s", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		results := r.rec.RecordTicksOnce(ctx)
		r.rec.ReportTicks(results)
		r.enqueue(results)
		if err := r.waitDraining(ctx, ticker.C); err != nil {
			return err
		}
	}
}

// summarise prints the Layer 3 tally over whatever was compared, plus what was
// still queued when this stopped.
func (r *sameHourRunner) summarise(w io.Writer) {
	if len(r.rows) == 0 {
		fmt.Fprintf(w, "\nno recording reached its rebuild. %d still queued\n", len(r.queue))
		return
	}
	summarise3(w, r.rows)
	if n := len(r.queue); n > 0 {
		fmt.Fprintf(w, "  %d recording(s) were still waiting for a rebuild when this stopped. "+
			"They are on disk, and \"keel crosscheck\" can still compare them at a longer gap\n", n)
	}
}

// ---------------------------------------------------------------- CSV

// crosscheckAppender writes comparison rows to a CSV one at a time.
//
// The batch command builds every row and then writes the file. This one cannot: a
// recorder paired with a crosscheck runs for as long as the recorder does, and a
// file written at the end is a file that does not exist when the process is
// stopped, which is how it will usually end.
type crosscheckAppender struct {
	f *os.File
	w *csv.Writer
}

// openCrosscheckAppender opens or creates the CSV and writes the header when the
// file is new.
//
// AN EXISTING FILE IS APPENDED TO, so a recorder restarted mid-batch continues the
// same table rather than starting a second one. It is refused when its first line
// is not this build's header: appending today's columns under yesterday's header
// produces a file that every reader will misread, and silently.
func openCrosscheckAppender(path string) (*crosscheckAppender, error) {
	want := strings.Join(crosscheckCSVHeader, ",")

	existing, err := os.Open(path)
	switch {
	case err == nil:
		first, readErr := bufio.NewReader(existing).ReadString('\n')
		_ = existing.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, fmt.Errorf("%s: %w", path, readErr)
		}
		if got := strings.TrimRight(first, "\r\n"); got != "" && got != want {
			return nil, fmt.Errorf("%s already exists with a different header, so appending would "+
				"produce two tables in one file. Move it aside or name another file.\n  its header:  %s\n  this build:  %s",
				path, got, want)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	a := &crosscheckAppender{f: f, w: csv.NewWriter(f)}

	info, err := f.Stat()
	if err != nil {
		_ = a.Close()
		return nil, err
	}
	if info.Size() == 0 {
		if err := a.w.Write(crosscheckCSVHeader); err != nil {
			_ = a.Close()
			return nil, err
		}
		a.w.Flush()
		if err := a.w.Error(); err != nil {
			_ = a.Close()
			return nil, err
		}
	}
	return a, nil
}

// append writes one row and FLUSHES IT. Flushing every row costs a write syscall
// per comparison, which is nothing against a rebuild that costs a dozen HTTP
// requests, and it means a Ctrl-C leaves every row that was printed also on disk.
func (a *crosscheckAppender) append(row crosscheckRow) error {
	if err := a.w.Write(row.csvRecord()); err != nil {
		return err
	}
	a.w.Flush()
	return a.w.Error()
}

func (a *crosscheckAppender) Close() error {
	a.w.Flush()
	if err := a.w.Error(); err != nil {
		_ = a.f.Close()
		return err
	}
	return a.f.Close()
}
