package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/api"
	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

func replayEmptyHorizon(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/trades", "/offers":
			fmt.Fprint(w, `{"_embedded":{"records":[]}}`)
		case "/ledgers/61340260":
			fmt.Fprint(w, `{"sequence":61340260,"closed_at":"2026-02-22T00:10:03Z"}`)
		default:
			t.Errorf("unexpected Horizon request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestReplayWithoutPersistNeverConnectsToPostgres(t *testing.T) {
	s := replayEmptyHorizon(t)
	output := filepath.Join(t.TempDir(), "snapshot.json")
	if err := runReplay([]string{"-pairs", "../../scripts/record-pairs.example.json", "-ledger", "61340260", "-horizon", s.URL, "-dsn", "not a valid DSN", "-quiet", "-out", output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatal(err)
	}
}

// KEEL_REPLAY_TEST_DSN must name a disposable migrated database. Unlike the store
// package's transaction tests this exercises the public Store and CLI APIs, so it
// writes fixture rows. It must never be pointed at an evidence or production DB.
func TestReplayPersistenceThroughPostgresAndHistoricalAPI(t *testing.T) {
	dsn := os.Getenv("KEEL_REPLAY_TEST_DSN")
	if dsn == "" {
		t.Skip("set KEEL_REPLAY_TEST_DSN to a disposable migrated database")
	}
	ctx := context.Background()
	db, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	r := replayForPersistence()
	if _, err := db.UpsertAsset(ctx, r.Snapshot.Base, r.Snapshot.Quote, "Track B integration fixture, not historical evidence"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := persistReplay(ctx, db, r); err != nil {
		t.Fatal(err)
	}
	// A duplicate may never replace an already stored value.
	r.Snapshot.Book.Asks[0].Price = domain.Price{N: 108, D: 1}
	if _, inserted, err := persistReplay(ctx, db, r); err != nil || inserted {
		t.Fatalf("duplicate: inserted=%t error=%v", inserted, err)
	}
	server, err := api.New(api.Config{Reader: db, Params: domain.DefaultParams(), HistoricalAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/asset/"+r.Snapshot.Base.String()+"/depth?ledger=61340262", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("historical API: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		LedgerSeq          uint32 `json:"ledgerSeq"`
		MethodologyVersion string `json:"methodologyVersion"`
		DataSource         string `json:"dataSource"`
		MidPrice           string `json:"midPrice"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.LedgerSeq != 61340262 || body.MethodologyVersion != domain.MethodologyVersion || body.DataSource != "offers-implied" || body.MidPrice != "53.8971414" {
		t.Fatalf("stored fixture changed or lost provenance: %s", w.Body.String())
	}
	// Also exercise flag parsing, ledger-time reading and the actual CLI writer.
	h := replayEmptyHorizon(t)
	pool := replayForPersistence().Snapshot
	pool.LedgerSeq = 61340260
	pool.LedgerClosedAt = pool.LedgerClosedAt.Add(-12 * time.Second)
	poolFile := filepath.Join(t.TempDir(), "controlled-pool-evidence.json")
	poolBody, err := json.Marshal([]domain.Snapshot{pool})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(poolFile, poolBody, 0600); err != nil {
		t.Fatal(err)
	}
	if err := runReplay([]string{"-pairs", "../../scripts/record-pairs.example.json", "-ledger", "61340260", "-horizon", h.URL, "-dsn", dsn, "-quiet", "-persist", "-pool-snapshots", poolFile}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/asset/"+r.Snapshot.Base.String()+"/depth?ledger=61340260", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("CLI row unavailable: %d %s", w.Code, w.Body.String())
	}
}
