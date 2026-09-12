package horizon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplayLedgerCloseTimeRequiresTheRequestedLedgerAndAnActualTime(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		bad        bool
	}{
		{"valid", `{"sequence":61340262,"closed_at":"2026-02-22T00:10:15Z"}`, false},
		{"wrong ledger", `{"sequence":61340263,"closed_at":"2026-02-22T00:10:21Z"}`, true},
		{"missing time", `{"sequence":61340262}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/ledgers/61340262" {
					t.Errorf("path = %s", r.URL.Path)
				}
				fmt.Fprint(w, tc.body)
			}))
			defer s.Close()
			got, err := NewClient(Config{BaseURL: s.URL}).ReplayLedgerCloseTime(context.Background(), 61340262)
			if (err != nil) != tc.bad {
				t.Fatalf("time=%v error=%v", got, err)
			}
			if !tc.bad && got.UTC().Format("2006-01-02T15:04:05Z") != "2026-02-22T00:10:15Z" {
				t.Fatalf("time = %v", got)
			}
		})
	}
}
