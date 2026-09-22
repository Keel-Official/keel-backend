package horizon

import (
	"context"
	"os"
	"testing"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// A LIVE probe against public Horizon, skipped unless KEEL_HORIZON_LIVE is set.
//
// IT IS THE ONLY CHECK THAT MATTERS FOR THIS FILE. The fake in the unit tests
// serves pages this repository wrote, so it can prove the filtering and the page
// bound and nothing about whether Horizon's effects carry the state this method
// reads out of them. DEC-013 established the February reserves by hand, by the
// same walk, and docs/evidences/...-pool-evidence-2026-02-22.json is the audited
// artefact. Reproducing those exact figures from code is the claim.
//
//	KEEL_HORIZON_LIVE=1 go test ./internal/horizon -run LivePoolReservesAt -v
func TestLivePoolReservesAtReproducesTheAuditedFebruaryReserves(t *testing.T) {
	if os.Getenv("KEEL_HORIZON_LIVE") == "" {
		t.Skip("set KEEL_HORIZON_LIVE=1 to run the live Horizon probe")
	}
	ustry := domain.Asset{Code: "USTRY", Issuer: "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC", Type: domain.AssetTypeAlphanum12}
	usdc := domain.Asset{Code: "USDC", Issuer: "GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN", Type: domain.AssetTypeAlphanum4}
	c := NewClient(Config{BaseURL: DefaultBaseURL, Budget: 3000})

	const (
		poolID = "27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb"
		base   = "15.4791416" // DEC-013 section 1, USTRY
		quote  = "16.3389179" // DEC-013 section 1, USDC
	)

	// All three ledgers DEC-023 authorises. The evidence sidecar records why one
	// set of reserves covers all three: the pool's last effect before them is
	// 2026-02-10T16:59:35Z and the next is 2026-02-22T22:08:33Z.
	for _, ledger := range []uint32{61340172, 61340262, 61340263} {
		pools, walk, err := c.PoolReservesAt(context.Background(), ustry, usdc, ledger, 5)
		if err != nil {
			t.Fatalf("ledger %d: %v", ledger, err)
		}
		if !walk.Complete() {
			t.Fatalf("ledger %d: pools not reached within the page bound: %v", ledger, walk.Unreached)
		}
		var found *PoolAt
		for i := range pools {
			if pools[i].Pool.PoolID == poolID {
				found = &pools[i]
			}
		}
		if found == nil {
			t.Fatalf("ledger %d: the USTRY/USDC pool is absent; listed %d, resolved %d, absent %v",
				ledger, walk.Listed, walk.Resolved, walk.Absent)
		}
		if got := found.Pool.ReserveBase.String(); got != base {
			t.Errorf("ledger %d: reserve base = %s, want %s (DEC-013)", ledger, got, base)
		}
		if got := found.Pool.ReserveQuote.String(); got != quote {
			t.Errorf("ledger %d: reserve quote = %s, want %s (DEC-013)", ledger, got, quote)
		}
		if found.Pool.FeeBP != 30 {
			t.Errorf("ledger %d: fee = %d bp, want 30", ledger, found.Pool.FeeBP)
		}
		t.Logf("ledger %d: %s base %s quote %s, from effect %s at %s (ledger %d), %d request(s)",
			ledger, found.Pool.PoolID[:8], found.Pool.ReserveBase, found.Pool.ReserveQuote,
			found.EffectID, found.EffectAt.Format("2006-01-02T15:04:05Z"), found.EffectLedger, walk.Pages)
	}
}
