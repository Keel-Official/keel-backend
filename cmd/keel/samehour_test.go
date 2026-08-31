package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/horizon"
)

// The clock every test here drives. Nothing in this file waits on a real one
// except the two that are explicitly about waiting, and those wait milliseconds.
var t0 = time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)

func testRunner(now func() time.Time, delay time.Duration) (*sameHourRunner, *[]string) {
	var compared []string
	r := &sameHourRunner{
		delay:   delay,
		now:     now,
		out:     io.Discard,
		logf:    func(string, ...any) {},
		compare: nil,
	}
	r.compare = func(_ context.Context, path string) crosscheckRow {
		compared = append(compared, path)
		return crosscheckRow{Path: path}
	}
	return r, &compared
}

func tickResult(pair string, path string, recordedAt time.Time, written bool) horizon.TickResult {
	return horizon.TickResult{
		Pair:    horizon.Pair{Base: domain.Asset{Code: pair, Issuer: "GISSUER", Type: domain.AssetTypeAlphanum4}},
		Path:    path,
		Written: written,
		Tick:    horizon.RawTick{RecordedAt: recordedAt.Format(time.RFC3339)},
	}
}

// The due time comes from the recording's own recorded_at and not from the clock
// when the round finished. The difference is however long the round took, and the
// quantity under test is measured from the reading.
func TestEnqueueSchedulesFromRecordedAt(t *testing.T) {
	now := func() time.Time { return t0.Add(90 * time.Second) } // the round took 90s
	r, _ := testRunner(now, 5*time.Minute)

	r.enqueue([]horizon.TickResult{
		tickResult("AQUA", "a.json.gz", t0, true),
		tickResult("USDC", "b.json.gz", t0.Add(30*time.Second), true),
	})

	if len(r.queue) != 2 {
		t.Fatalf("queued %d, want 2", len(r.queue))
	}
	if want := t0.Add(5 * time.Minute); !r.queue[0].Due.Equal(want) {
		t.Errorf("first due %s, want %s", r.queue[0].Due, want)
	}
	if want := t0.Add(30*time.Second + 5*time.Minute); !r.queue[1].Due.Equal(want) {
		t.Errorf("second due %s, want %s", r.queue[1].Due, want)
	}
}

// A tick that never reached disk has nothing to compare against.
func TestEnqueueSkipsUnwritten(t *testing.T) {
	r, _ := testRunner(func() time.Time { return t0 }, time.Minute)
	r.enqueue([]horizon.TickResult{
		tickResult("AQUA", "", t0, false),
		tickResult("USDC", "b.json.gz", t0, true),
	})
	if len(r.queue) != 1 || r.queue[0].Path != "b.json.gz" {
		t.Fatalf("queue = %+v, want only b.json.gz", r.queue)
	}
}

// An unreadable recorded_at still gets its comparison. It is scheduled from the
// clock, and the row it produces reports the gap as unknown rather than as a
// number nobody can trust.
func TestEnqueueFallsBackWhenRecordedAtIsUnreadable(t *testing.T) {
	r, _ := testRunner(func() time.Time { return t0 }, time.Minute)
	bad := tickResult("AQUA", "a.json.gz", t0, true)
	bad.Tick.RecordedAt = "not a timestamp"
	r.enqueue([]horizon.TickResult{bad})

	if len(r.queue) != 1 {
		t.Fatalf("queued %d, want 1", len(r.queue))
	}
	if !r.queue[0].RecordedAt.IsZero() {
		t.Errorf("RecordedAt = %s, want the zero time so the row reports an unknown gap", r.queue[0].RecordedAt)
	}
	if want := t0.Add(time.Minute); !r.queue[0].Due.Equal(want) {
		t.Errorf("due %s, want %s, scheduled from the clock", r.queue[0].Due, want)
	}
}

// drain runs what is due, earliest first, and leaves what is not.
func TestDrainRunsOnlyWhatIsDueEarliestFirst(t *testing.T) {
	clock := t0
	r, compared := testRunner(func() time.Time { return clock }, 0)

	r.queue = []pendingRebuild{
		{Path: "late.json.gz", Due: t0.Add(10 * time.Minute)},
		{Path: "second.json.gz", Due: t0.Add(2 * time.Minute)},
		{Path: "first.json.gz", Due: t0.Add(1 * time.Minute)},
	}

	r.drain(context.Background())
	if len(*compared) != 0 {
		t.Fatalf("compared %v before anything was due", *compared)
	}

	clock = t0.Add(2 * time.Minute)
	r.drain(context.Background())

	want := []string{"first.json.gz", "second.json.gz"}
	if strings.Join(*compared, ",") != strings.Join(want, ",") {
		t.Errorf("compared %v, want %v", *compared, want)
	}
	if len(r.queue) != 1 || r.queue[0].Path != "late.json.gz" {
		t.Errorf("queue = %+v, want only late.json.gz left", r.queue)
	}
	if len(r.rows) != 2 {
		t.Errorf("kept %d rows, want 2", len(r.rows))
	}
}

// A comparison is attempted once. A rebuild that failed is a row with an error in
// it, which is a result; retrying it against a book that has moved further would
// be a different experiment under the same name.
func TestDrainDoesNotRetry(t *testing.T) {
	r, compared := testRunner(func() time.Time { return t0 }, 0)
	r.compare = func(_ context.Context, path string) crosscheckRow {
		*compared = append(*compared, path)
		return crosscheckRow{Path: path, Err: "horizon said no"}
	}
	r.queue = []pendingRebuild{{Path: "a.json.gz", Due: t0}}

	r.drain(context.Background())
	r.drain(context.Background())

	if len(r.queue) != 0 {
		t.Errorf("queue = %+v, want empty", r.queue)
	}
	if len(r.rows) != 1 {
		t.Errorf("kept %d rows, want 1", len(r.rows))
	}
}

// A canceled context stops the draining rather than comparing against a Horizon
// nobody is waiting for any more.
func TestDrainStopsOnCanceledContext(t *testing.T) {
	r, compared := testRunner(func() time.Time { return t0 }, 0)
	r.queue = []pendingRebuild{{Path: "a.json.gz", Due: t0}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.drain(ctx)

	if len(*compared) != 0 {
		t.Errorf("compared %v after the context was canceled", *compared)
	}
}

// runOnce waits for the delay and then compares. Real milliseconds, because what
// is being tested here is the waiting.
func TestRunOnceWaitsForTheDelayThenCompares(t *testing.T) {
	// A clock anchored at t0 and advancing in real time. Anchoring it matters:
	// recorded_at is RFC3339 to the second, so a stamp taken from time.Now() is
	// up to a second in the past the moment it is written, and a 40ms delay
	// computed from it is already due. t0 formats exactly.
	start := time.Now()
	r, compared := testRunner(func() time.Time { return t0.Add(time.Since(start)) }, 40*time.Millisecond)

	if err := r.runOnce(context.Background(), []horizon.TickResult{
		tickResult("AQUA", "a.json.gz", t0, true),
	}); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	waited := time.Since(start)

	if len(*compared) != 1 {
		t.Fatalf("compared %v, want one recording", *compared)
	}
	if waited < 30*time.Millisecond {
		t.Errorf("returned after %s, which is before the recording was due", waited)
	}
}

// ---------------------------------------------------------------- the row

func TestStampTimingRecordsTheGap(t *testing.T) {
	var row crosscheckRow
	row.stampTiming(t0, t0.Add(7*time.Minute+30*time.Second))

	if !row.ElapsedKnown {
		t.Fatal("ElapsedKnown is false, want true")
	}
	if row.Elapsed != 7*time.Minute+30*time.Second {
		t.Errorf("Elapsed = %s, want 7m30s", row.Elapsed)
	}
	if got := row.elapsedSeconds().String(); got != "450" {
		t.Errorf("elapsedSeconds = %s, want 450", got)
	}
}

// A gap that is not known and a gap of zero are different findings, and an
// experiment that prints 0 for both has lost the one it is measuring.
func TestStampTimingWithoutARecordedAtLeavesTheGapUnknown(t *testing.T) {
	var row crosscheckRow
	row.stampTiming(time.Time{}, t0)

	if row.ElapsedKnown {
		t.Fatal("ElapsedKnown is true, want false")
	}
	if row.RebuiltAt.IsZero() {
		t.Error("RebuiltAt is zero: when the rebuild ran is known even when the recording's stamp is not")
	}
	rec := row.csvRecord()
	if got := rec[len(rec)-1]; got != "" {
		t.Errorf("elapsed_seconds = %q, want empty rather than a zero", got)
	}
}

// Non-negotiable rule 1: every output row carries LedgerSeq and
// MethodologyVersion.
func TestEveryOutputRowCarriesLedgerAndMethodologyVersion(t *testing.T) {
	row := crosscheckRow{Pair: "AQUA:GISSUER/XLM", Ledger: 64129586}
	row.stampTiming(t0, t0.Add(time.Minute))

	line := row.line()
	if !strings.Contains(line, "64129586") {
		t.Errorf("line has no ledger: %s", line)
	}
	if !strings.Contains(line, domain.MethodologyVersion) {
		t.Errorf("line has no methodology version: %s", line)
	}
	if !strings.Contains(line, "elapsed 1m0s") {
		t.Errorf("line has no elapsed time: %s", line)
	}

	rec := row.csvRecord()
	if len(rec) != len(crosscheckCSVHeader) {
		t.Fatalf("row has %d fields, header has %d", len(rec), len(crosscheckCSVHeader))
	}
	fields := map[string]string{}
	for i, name := range crosscheckCSVHeader {
		fields[name] = rec[i]
	}
	if fields["ledger"] != "64129586" {
		t.Errorf("ledger = %q", fields["ledger"])
	}
	if fields["methodology_version"] != domain.MethodologyVersion {
		t.Errorf("methodology_version = %q, want %q", fields["methodology_version"], domain.MethodologyVersion)
	}
	if fields["elapsed_seconds"] != "60" {
		t.Errorf("elapsed_seconds = %q, want 60", fields["elapsed_seconds"])
	}
}

// The four new columns are appended, so every column that existed before this
// change is still in the position it was read from.
func TestTheOriginalCSVColumnsDidNotMove(t *testing.T) {
	before := []string{
		"verdict", "pair", "ledger",
		"recorded_bids", "recorded_asks", "rebuilt_bids", "rebuilt_asks",
		"levels_match", "prices_match", "amounts_match", "risk_match",
		"offers_carried", "offers_changed_after_target", "offers_gone_unresolved",
		"requests", "explanation", "error", "recording",
	}
	if len(crosscheckCSVHeader) < len(before) {
		t.Fatalf("header lost columns: %v", crosscheckCSVHeader)
	}
	for i, name := range before {
		if crosscheckCSVHeader[i] != name {
			t.Errorf("column %d is %q, want %q: the pre-existing columns must not move",
				i, crosscheckCSVHeader[i], name)
		}
	}
}

// ---------------------------------------------------------------- the CSV

func TestAppenderWritesAHeaderOnceAndAppendsRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rows.csv")

	a, err := openCrosscheckAppender(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	row := crosscheckRow{Pair: "AQUA:GISSUER/XLM", Ledger: 1}
	row.stampTiming(t0, t0.Add(time.Minute))
	if err := a.append(row); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Read it back BEFORE closing: the appender flushes every row so that a
	// Ctrl-C leaves on disk everything it printed.
	mid, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n := len(strings.Split(strings.TrimSpace(string(mid)), "\n")); n != 2 {
		t.Errorf("file has %d line(s) before Close, want a header and one flushed row", n)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopening appends rather than starting a second table.
	b, err := openCrosscheckAppender(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if err := b.append(row); err != nil {
		t.Fatalf("append after reopen: %v", err)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close after reopen: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 3 {
		t.Fatalf("file has %d line(s), want a header and two rows:\n%s", len(lines), body)
	}
	if lines[0] != strings.Join(crosscheckCSVHeader, ",") {
		t.Errorf("header = %q", lines[0])
	}
}

// Appending today's columns under yesterday's header produces a file every reader
// misreads, and silently. It is refused instead.
func TestAppenderRefusesAFileWithADifferentHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rows.csv")
	if err := os.WriteFile(path, []byte("verdict,pair,ledger\nMATCH,AQUA,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openCrosscheckAppender(path); err == nil {
		t.Fatal("opened a file with a different header, want an error")
	}
}

// ---------------------------------------------------------------- the flags

// The ceiling is what makes "the same hour" a property of the flags rather than
// of somebody's intention.
func TestRecordRefusesADelayOfAnHourOrMore(t *testing.T) {
	err := runRecord([]string{"-pairs", "nothing.json", "-crosscheck", "-crosscheck-after", "1h"})
	if err == nil || !strings.Contains(err.Error(), "crosscheck-after") {
		t.Fatalf("err = %v, want a refusal naming -crosscheck-after", err)
	}
}

func TestRecordRefusesANegativeDelay(t *testing.T) {
	err := runRecord([]string{"-pairs", "nothing.json", "-crosscheck-after", "-1s"})
	if err == nil || !strings.Contains(err.Error(), "crosscheck-after") {
		t.Fatalf("err = %v, want a refusal naming -crosscheck-after", err)
	}
}

func TestRecordRefusesCrosscheckWithSchema1(t *testing.T) {
	err := runRecord([]string{"-pairs", "nothing.json", "-crosscheck", "-schema", "1"})
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("err = %v, want a refusal naming the schema", err)
	}
}

func TestRecordRefusesContinuousCrosscheckWithHolders(t *testing.T) {
	err := runRecord([]string{"-pairs", "nothing.json", "-crosscheck", "-holders"})
	if err == nil || !strings.Contains(err.Error(), "holder") {
		t.Fatalf("err = %v, want a refusal naming the holder reading", err)
	}
}

// A delay of zero is a legitimate setting and is not refused: it is the shortest
// arm the experiment can have.
func TestRecordAcceptsAZeroDelay(t *testing.T) {
	err := runRecord([]string{"-pairs", "nothing.json", "-crosscheck", "-crosscheck-after", "0"})
	if err == nil {
		t.Fatal("want the missing pair file to be the failure, got none")
	}
	if strings.Contains(err.Error(), "crosscheck-after") {
		t.Fatalf("err = %v, want zero to be accepted and the pair file to be the failure", err)
	}
}
