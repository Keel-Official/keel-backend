package horizon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// The two page caps, tested at the level where the switch happens.
//
// WHAT THESE TESTS ARE FOR. The February 2026 series of 8 September 2026 walked
// 222 accounts, 9 of which posted a single offer between them, and truncated both
// of the two that actually hold the pair's book. A cap that has to serve both
// populations serves neither, so it was split. These tests assert the split, not
// any market figure: no book, no depth and no price appears below.

func TestThePageCapsResolveTheirDefaultsAndNeverInvertThem(t *testing.T) {
	cases := []struct {
		name                    string
		plainIn, offeringIn     int
		wantPlain, wantOffering int
	}{
		{
			name:         "both zero take both defaults",
			wantPlain:    defaultMaxPagesPerAccount,
			wantOffering: defaultMaxPagesPerOfferingAccount,
		},
		{
			name:    "an explicit pair is used as given",
			plainIn: 7, offeringIn: 90,
			wantPlain:    7,
			wantOffering: 90,
		},
		{
			// A caller who raises only the shallow cap must not silently get a
			// SHALLOWER cap for the accounts that matter, which is what the
			// default would do at plain=600.
			name:    "an offering cap below the plain one is raised to it",
			plainIn: 600, offeringIn: 60,
			wantPlain:    600,
			wantOffering: 600,
		},
		{
			name:         "a zero offering cap against a large plain one is still never smaller",
			plainIn:      defaultMaxPagesPerOfferingAccount + 100,
			wantPlain:    defaultMaxPagesPerOfferingAccount + 100,
			wantOffering: defaultMaxPagesPerOfferingAccount + 100,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ReplayQuery{
				MaxPagesPerAccount:         tc.plainIn,
				MaxPagesPerOfferingAccount: tc.offeringIn,
			}.pageCaps()

			if got.Plain != tc.wantPlain {
				t.Errorf("plain cap = %d, want %d", got.Plain, tc.wantPlain)
			}
			if got.Offering != tc.wantOffering {
				t.Errorf("offering cap = %d, want %d", got.Offering, tc.wantOffering)
			}
			if got.Offering < got.Plain {
				t.Errorf("offering cap %d is shallower than plain %d, which can never be right",
					got.Offering, got.Plain)
			}
		})
	}
}

func TestTheCapInForceSwitchesOnTheFirstOfferOperation(t *testing.T) {
	caps := walkPageCaps{Plain: 3, Offering: 40}

	if got := caps.For(0); got != 3 {
		t.Errorf("with no offer found the cap is %d, want the shallow 3", got)
	}
	if got := caps.For(1); got != 40 {
		t.Errorf("with one offer found the cap is %d, want the deep 40", got)
	}
}

// operationsFixture serves an endless /operations feed, page by page, so a walk
// can be run against the cap rather than against Horizon.
//
// EVERY PAGE CARRIES A next LINK. That is what makes these tests about the cap:
// nothing else can stop the walk, so a walk that stops was stopped by the bound
// under test.
type operationsFixture struct {
	srv *httptest.Server

	// offerOnFirstPage puts one real offer operation on the pair in page 1, so
	// the walk's OfferOperations goes above zero and the deeper cap engages.
	offerOnFirstPage bool

	pagesServed int
}

func (f *operationsFixture) start(t *testing.T) *Client {
	t.Helper()

	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.pagesServed++
		page := f.pagesServed

		// A descending walk over ledgers, one page per 10 ledgers, starting well
		// below any floor these tests set. The ledger only has to fall, because
		// the fixture's job is to never run out and never hit a floor.
		ledger := uint64(61340000 - 10*page)
		toid := strconv.FormatUint(ledger<<32|1<<12, 10)

		rec := map[string]any{
			"paging_token":           toid,
			"type":                   "payment",
			"source_account":         "GTEST",
			"transaction_successful": true,
		}
		if page == 1 && f.offerOnFirstPage {
			rec = map[string]any{
				"paging_token":           toid,
				"type":                   "manage_sell_offer",
				"offer_id":               "0",
				"selling_asset_type":     string(testUSTRY.Type),
				"selling_asset_code":     testUSTRY.Code,
				"selling_asset_issuer":   testUSTRY.Issuer,
				"buying_asset_type":      string(testUSDC.Type),
				"buying_asset_code":      testUSDC.Code,
				"buying_asset_issuer":    testUSDC.Issuer,
				"source_account":         "GTEST",
				"transaction_successful": true,
				"transaction":            map[string]any{"result_xdr": xdrAskCreate},
			}
		}

		body := map[string]any{
			"_links":    map[string]any{"next": map[string]any{"href": f.srv.URL + fmt.Sprintf("/next/%d", page+1)}},
			"_embedded": map[string]any{"records": []any{rec}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.srv.Close)

	return NewClient(Config{BaseURL: f.srv.URL, Sleep: func(time.Duration) {}})
}

func TestAnAccountWithNoOfferOnThePairStopsAtTheShallowCap(t *testing.T) {
	f := &operationsFixture{}
	c := f.start(t)

	caps := walkPageCaps{Plain: 3, Offering: 40}
	_, walk, err := c.offerOperationsFor(context.Background(), "GTEST",
		refOf(testUSTRY), refOf(testUSDC), 61340263, 0, caps)
	if err != nil {
		t.Fatalf("offerOperationsFor: %v", err)
	}

	if walk.OfferOperations != 0 {
		t.Fatalf("the fixture served no offer on this pair and the walk found %d",
			walk.OfferOperations)
	}
	if walk.Pages != caps.Plain {
		t.Errorf("pages walked = %d, want the shallow cap %d", walk.Pages, caps.Plain)
	}
	if !walk.Truncated {
		t.Error("a walk cut by the cap must report Truncated, or the bound is a silent floor")
	}
}

func TestAnAccountThatPostedAnOfferGetsTheDeeperCap(t *testing.T) {
	f := &operationsFixture{offerOnFirstPage: true}
	c := f.start(t)

	// The shallow cap is ONE page, and page one is where the offer is. So a walk
	// that stops at one page never noticed the switch, and a walk that reaches
	// the deep cap took it.
	caps := walkPageCaps{Plain: 1, Offering: 5}
	_, walk, err := c.offerOperationsFor(context.Background(), "GTEST",
		refOf(testUSTRY), refOf(testUSDC), 61340263, 0, caps)
	if err != nil {
		t.Fatalf("offerOperationsFor: %v", err)
	}

	if walk.OfferOperations != 1 {
		t.Fatalf("the fixture served one offer on this pair and the walk found %d",
			walk.OfferOperations)
	}
	if walk.Pages != caps.Offering {
		t.Errorf("pages walked = %d, want the deep cap %d. The switch did not happen",
			walk.Pages, caps.Offering)
	}
	if !walk.Truncated {
		t.Error("the walk was still cut by a cap and must say so")
	}
}

// A walk that finds nothing must not become expensive just because a caller
// raised the deep cap. This is the guard on the 213 accounts of the 8 September
// run that held no offer: the whole point of the split is that raising the deep
// cap costs nothing on them.
func TestRaisingTheDeepCapDoesNotDeepenAWalkThatFindsNoOffer(t *testing.T) {
	shallow := &operationsFixture{}
	c := shallow.start(t)
	_, walk, err := c.offerOperationsFor(context.Background(), "GTEST",
		refOf(testUSTRY), refOf(testUSDC), 61340263, 0, walkPageCaps{Plain: 4, Offering: 4})
	if err != nil {
		t.Fatalf("offerOperationsFor: %v", err)
	}

	deep := &operationsFixture{}
	c2 := deep.start(t)
	_, walk2, err := c2.offerOperationsFor(context.Background(), "GTEST",
		refOf(testUSTRY), refOf(testUSDC), 61340263, 0, walkPageCaps{Plain: 4, Offering: 4000})
	if err != nil {
		t.Fatalf("offerOperationsFor: %v", err)
	}

	if walk.Pages != walk2.Pages {
		t.Errorf("a 4000 page deep cap changed a no-offer walk from %d pages to %d",
			walk.Pages, walk2.Pages)
	}
	if walk2.Pages != 4 {
		t.Errorf("pages walked = %d, want the shallow cap 4", walk2.Pages)
	}
}
