package horizon

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

const testPoolID = "27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb"

// poolEffect is one record of /liquidity_pools/{id}/effects. The reserves it
// carries are the pool's state AFTER it, which is the whole basis of this walk.
func poolEffect(token, typ, poolID, ustry, usdc string) string {
	return fmt.Sprintf(`{
	  "paging_token": %q,
	  "type": %q,
	  "created_at": "2026-02-10T16:59:35Z",
	  "liquidity_pool": {
	    "id": %q,
	    "fee_bp": 30,
	    "reserves": [
	      {"asset":"USDC:GA5ZSEJYB37JRC5AVCIA5MOP4RHTM335X2KGX3IHOJAPP5RE34K4KZVN","amount":%q},
	      {"asset":"USTRY:GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC","amount":%q}
	    ]
	  }
	}`, token, typ, poolID, usdc, ustry)
}

// effectPages serves the records newest first, one page each, following the
// cursor, which is the shape Horizon serves for order=desc.
func effectPages(f *fakeHorizon, pages ...[]string) {
	f.handler["/liquidity_pools/"+testPoolID+"/effects"] = func(w http.ResponseWriter, r *http.Request) {
		i := 0
		if c := r.URL.Query().Get("cursor"); c != "" {
			_, _ = fmt.Sscanf(c, "page%d", &i)
		}
		if i >= len(pages) {
			_, _ = fmt.Fprint(w, `{"_links":{"next":{"href":""}},"_embedded":{"records":[]}}`)
			return
		}
		next := fmt.Sprintf("%s/liquidity_pools/%s/effects?cursor=page%d", f.srv.URL, testPoolID, i+1)
		body := `{"_links":{"next":{"href":"` + next + `"}},"_embedded":{"records":[`
		for j, rec := range pages[i] {
			if j > 0 {
				body += ","
			}
			body += rec
		}
		_, _ = fmt.Fprint(w, body+`]}}`)
	}
}

func poolsAt(t *testing.T, c *Client, ledger uint32, maxPages int) ([]PoolAt, PoolHistoryWalk) {
	t.Helper()
	pools, walk, err := c.PoolReservesAt(context.Background(), testUSTRY, testUSDC, ledger, maxPages)
	if err != nil {
		t.Fatalf("PoolReservesAt: %v", err)
	}
	return pools, walk
}

func TestPoolReservesAtReadsThePoolsOwnEffectAndNotTheOneBeforeIt(t *testing.T) {
	// The trap the endpoint sets: a path payment puts other pools' effects in
	// this pool's feed, newest first. Reading the first record reads another
	// pool's reserves into this one. Measured on 22 September 2026 at ledger
	// 61340263, where the target pool's record is the third.
	f := newFakeHorizon(t)
	effectPages(f, []string{
		poolEffect("262733805310210049-5", "liquidity_pool_trade", "739517692334ae2c4a47ea796307106bb4380a2fd3182a788dfe118c0c1bb3e9", "1", "2"),
		poolEffect("262733805310210049-4", "liquidity_pool_trade", "caedc31f00000000000000000000000000000000000000000000000000000000", "3", "4"),
		poolEffect("262733805310210049-3", "liquidity_pool_trade", testPoolID, "15.4791416", "16.3389179"),
	})
	c, _ := f.client()

	pools, walk := poolsAt(t, c, 61340263, 5)
	if len(pools) != 1 {
		t.Fatalf("want one pool, got %d", len(pools))
	}
	got := pools[0]
	if got.Pool.ReserveBase.String() != "15.4791416" || got.Pool.ReserveQuote.String() != "16.3389179" {
		t.Errorf("reserves = %s/%s, want the target pool's own", got.Pool.ReserveBase, got.Pool.ReserveQuote)
	}
	if got.EffectID != "262733805310210049-3" || got.EffectLedger != 61172481 {
		t.Errorf("provenance = %s at ledger %d, want the matching effect and its ledger", got.EffectID, got.EffectLedger)
	}
	if !walk.Complete() || walk.Resolved != 1 || walk.Listed != 1 {
		t.Errorf("walk = %+v", walk)
	}
}

func TestPoolReservesAtCallsARemovedPoolAbsentRatherThanEmpty(t *testing.T) {
	f := newFakeHorizon(t)
	effectPages(f, []string{
		poolEffect("300000000000000000-1", "liquidity_pool_removed", testPoolID, "0", "0"),
		poolEffect("262733805310210049-3", "liquidity_pool_trade", testPoolID, "15.4791416", "16.3389179"),
	})
	c, _ := f.client()

	pools, walk := poolsAt(t, c, 61340263, 5)
	if len(pools) != 0 || walk.Resolved != 0 || len(walk.Absent) != 1 {
		t.Errorf("pools %d, walk %+v; a removed pool is absent, not an empty one", len(pools), walk)
	}
}

func TestPoolReservesAtCallsAPoolWithNoEffectAbsent(t *testing.T) {
	// A pool created after the target has nothing at or before it.
	f := newFakeHorizon(t)
	effectPages(f)
	c, _ := f.client()

	pools, walk := poolsAt(t, c, 61340263, 5)
	if len(pools) != 0 || len(walk.Absent) != 1 || !walk.Complete() {
		t.Errorf("pools %d, walk %+v; want absent and complete", len(pools), walk)
	}
}

func TestPoolReservesAtReportsAPageBoundRatherThanGuessing(t *testing.T) {
	// Two pages of other pools' effects and a bound of one: the answer is not
	// known, and that is a budget failure rather than a finding.
	other := "caedc31f00000000000000000000000000000000000000000000000000000000"
	f := newFakeHorizon(t)
	effectPages(f,
		[]string{poolEffect("2-1", "liquidity_pool_trade", other, "1", "2")},
		[]string{poolEffect("1-1", "liquidity_pool_trade", testPoolID, "15.4791416", "16.3389179")},
	)
	c, _ := f.client()

	pools, walk := poolsAt(t, c, 61340263, 1)
	if len(pools) != 0 || len(walk.Unreached) != 1 || walk.Complete() {
		t.Errorf("pools %d, walk %+v; an unreached pool must not be reported as absent", len(pools), walk)
	}
}
