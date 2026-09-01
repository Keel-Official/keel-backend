package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pinNow replaces the window check's clock for the duration of one test. The
// check refuses a window whose end has not passed, which cannot be exercised
// against the real clock: any fixture date chosen today is in the past by the
// time somebody runs the suite next year.
func pinNow(t *testing.T, at string) {
	t.Helper()
	now, err := time.Parse(time.RFC3339, at)
	if err != nil {
		t.Fatalf("bad pinned time %q: %v", at, err)
	}
	prev := backtestNow
	backtestNow = func() time.Time { return now }
	t.Cleanup(func() { backtestNow = prev })
}

// countingHorizon answers every /trades request with one empty page and counts
// the requests. An empty page ends the walk, so a run that gets this far
// completes; a run that was refused leaves the counter at zero.
func countingHorizon(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"_embedded":{"records":[]}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func writePairsFile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "pairs.json")
	body := map[string]any{
		"pairs": []map[string]any{{
			"base": map[string]string{
				"code": "USTRY", "issuer": "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC",
				"type": "credit_alphanum12",
			},
			"quote": map[string]string{
				"code": "USDC", "issuer": "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN",
				"type": "credit_alphanum4",
			},
		}},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal pairs: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write pairs: %v", err)
	}
	return path
}

// TestBacktestRefusesAWindowThatHasNotClosed is the whole point of the check.
//
// THE NETWORK ASSERTION IS NOT DECORATION. A refusal that happens after the
// walk would still exit non-zero and still look correct from the outside, while
// having spent three thousand requests and, worse, having had the opportunity
// to write a partial file before failing. Counting the requests is what pins
// the refusal to the right side of the network.
func TestBacktestRefusesAWindowThatHasNotClosed(t *testing.T) {
	pinNow(t, "2026-09-01T14:22:31Z")
	srv, hits := countingHorizon(t)
	dir := t.TempDir()

	err := runBacktest([]string{
		"-pairs", writePairsFile(t, dir),
		"-from", "2026-08-01",
		"-to", "2026-09-02", // ends 2026-09-02T00:00:00Z, still ahead of the pinned now
		"-out", dir,
		"-horizon", srv.URL,
	})
	if err == nil {
		t.Fatal("a window ending in the future was accepted")
	}
	if !errors.Is(err, errWindowNotClosed) {
		t.Fatalf("err = %v, want errWindowNotClosed", err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("made %d network calls before refusing, want 0", got)
	}
	// The message has to name all three, because the operator's next action is
	// to pick a different -to and nothing else tells them which one is legal.
	for _, want := range []string{"2026-09-02", "2026-09-01T14:22:31Z", "2026-09-01"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %q: %v", want, err)
		}
	}
	// A refused run leaves nothing behind. There is no half-written evidence.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".csv") || strings.HasSuffix(e.Name(), ".meta.txt") {
			t.Errorf("refused run still wrote %s", e.Name())
		}
	}
}

// TestBacktestAcceptsAClosedWindow is the other half. Without it the check
// passes trivially by refusing everything.
func TestBacktestAcceptsAClosedWindow(t *testing.T) {
	pinNow(t, "2026-09-01T14:22:31Z")
	srv, hits := countingHorizon(t)
	dir := t.TempDir()

	err := runBacktest([]string{
		"-pairs", writePairsFile(t, dir),
		"-from", "2026-08-01",
		"-to", "2026-09-01", // ends 2026-09-01T00:00:00Z, already passed
		"-out", dir,
		"-horizon", srv.URL,
	})
	if err != nil {
		t.Fatalf("a closed window was refused: %v", err)
	}
	if hits.Load() == 0 {
		t.Error("accepted the window but never reached the network")
	}
}

// TestWindowClosedBoundary pins the edge the prose claims: -to is exclusive, so
// the window becomes readable AT its end instant and not a day later. Testing
// this through runBacktest would need a live walk per case; the check is a pure
// function of two times, so it is tested as one.
func TestWindowClosedBoundary(t *testing.T) {
	now := time.Date(2026, 9, 1, 14, 22, 31, 0, time.UTC)
	day := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	for _, tc := range []struct {
		name    string
		to      time.Time
		wantErr bool
	}{
		{"ended yesterday", day(2026, 8, 31), false},
		{"ended at the most recent midnight", day(2026, 9, 1), false},
		{"ends at the exact instant of now", now, false},
		{"ends one second after now", now.Add(time.Second), true},
		{"ends tomorrow", day(2026, 9, 2), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := checkWindowClosed(tc.to, now)
			if tc.wantErr && err == nil {
				t.Fatal("want refusal, got none")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want acceptance, got %v", err)
			}
		})
	}
}

// TestTradesMetaIsReproducible guards the reason the sidecar carries no
// generated_at. Two runs over the same closed window must produce the same
// bytes, so that a diff means the records changed rather than the clock did.
func TestTradesMetaIsReproducible(t *testing.T) {
	pinNow(t, "2026-09-01T14:22:31Z")
	srv, _ := countingHorizon(t)
	dir := t.TempDir()
	pairs := writePairsFile(t, dir)

	read := func() string {
		if err := runBacktest([]string{
			"-pairs", pairs, "-from", "2026-08-01", "-to", "2026-09-01",
			"-out", dir, "-horizon", srv.URL,
		}); err != nil {
			t.Fatalf("backtest: %v", err)
		}
		matches, err := filepath.Glob(filepath.Join(dir, "*.meta.txt"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("want exactly one sidecar, got %v (%v)", matches, err)
		}
		b, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatalf("read sidecar: %v", err)
		}
		return string(b)
	}

	first := read()
	second := read()
	if first != second {
		t.Errorf("sidecar is not reproducible across two runs:\n--- first\n%s\n--- second\n%s", first, second)
	}
	if !strings.Contains(first, "max_closed_at_utc:") {
		t.Errorf("sidecar does not record max_closed_at_utc:\n%s", first)
	}
}
