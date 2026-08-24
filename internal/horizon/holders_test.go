package horizon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// The holder reading is tested against a fake rather than against Horizon, for
// the same reason the order book is: these tests assert what the ADAPTER does
// with a shape, not what USTRY's real trustline set looks like on any given day.

// assetSummary is /assets for one asset. Both the newer balances/accounts
// objects and the older amount/num_accounts pair are served by real Horizon, so
// both appear here and one test removes the newer pair to prove the fallback.
func assetSummary(supply string, holders int, withNewFields bool) string {
	newer := ""
	if withNewFields {
		newer = fmt.Sprintf(`,
	    "balances":{"authorized":%q,"authorized_to_maintain_liabilities":"0.0000000","unauthorized":"0.0000000"},
	    "accounts":{"authorized":%d,"authorized_to_maintain_liabilities":0,"unauthorized":0}`, supply, holders)
	}
	return fmt.Sprintf(`{"_embedded":{"records":[{
	    "asset_type":"credit_alphanum12",
	    "asset_code":"USTRY",
	    "asset_issuer":%q,
	    "amount":%q,
	    "num_accounts":%d%s
	  }]}}`, testUSTRY.Issuer, supply, holders, newer)
}

// accountsPage builds one /accounts page holding n accounts, each with a balance
// in USTRY and one in USDC, because a real account holds more than the asset
// that was asked about and the balance has to be picked out.
func accountsPage(start, n int, balance string) string {
	recs := make([]string, 0, n)
	for i := start; i < start+n; i++ {
		id := accountID(i)
		recs = append(recs, fmt.Sprintf(`{
		  "id":%q,
		  "paging_token":%q,
		  "balances":[
		    {"balance":"12.0000000","asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":%q},
		    {"balance":%q,"asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}
		  ]
		}`, id, id, testUSDC.Issuer, balance, testUSTRY.Issuer))
	}
	return `{"_embedded":{"records":[` + strings.Join(recs, ",") + `]}}`
}

// accountID is a stable fake account id. Padded so that string order and
// numeric order agree, which keeps the sorting assertion readable.
func accountID(i int) string {
	return "GACCOUNT" + fmt.Sprintf("%048d", i)
}

// withHolders installs the two holder endpoints on the fake.
func (f *fakeHorizon) withHolders(summary string, pages []string) {
	f.handler["/assets"] = func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, summary) }
	f.handler["/accounts"] = func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		idx := 0
		if cursor != "" {
			// Every page but the first is identified by the paging token the
			// previous page ended on, so the fake counts how many it has served.
			idx = f.hits["/accounts"] - 1
		}
		if idx >= len(pages) {
			fmt.Fprint(w, `{"_embedded":{"records":[]}}`)
			return
		}
		fmt.Fprint(w, pages[idx])
	}
}

func TestGetHoldersReadsSupplyAndHolders(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", 3, true), []string{accountsPage(0, 3, "10.0000000")})
	c, _ := f.client()

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	if got := obs.Supply.String(); got != "1000" {
		t.Errorf("Supply = %s, want 1000", got)
	}
	if obs.HolderCount != 3 {
		t.Errorf("HolderCount = %d, want 3", obs.HolderCount)
	}
	if len(obs.Holders) != 3 {
		t.Fatalf("read %d holders, want 3", len(obs.Holders))
	}
	if got := obs.Holders[0].Balance.String(); got != "10" {
		t.Errorf("balance = %s, want 10, so the USDC balance in the same account was picked instead", got)
	}
	if obs.Truncated() {
		t.Error("Truncated is true although the only page was short")
	}
	if obs.Raw.Pages != 1 || len(obs.Raw.Accounts) != 1 {
		t.Errorf("Pages = %d and %d raw pages, want 1 and 1", obs.Raw.Pages, len(obs.Raw.Accounts))
	}
	if len(obs.Raw.AssetSummary) == 0 {
		t.Error("the asset summary bytes were not kept, so the evidence is incomplete")
	}
	if !obs.Raw.Atomic {
		t.Error("Atomic is false although one page cannot straddle a ledger")
	}
}

// The supply fields Horizon deprecated are still the only ones some deployments
// send. Reading only the newer pair would report a supply of zero, which divides
// rather than fails.
func TestGetHoldersFallsBackToTheOlderSupplyFields(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("777.0000000", 2, false), []string{accountsPage(0, 2, "1.0000000")})
	c, _ := f.client()

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	if got := obs.Supply.String(); got != "777" {
		t.Errorf("Supply = %s, want 777 from the amount field", got)
	}
	if obs.HolderCount != 2 {
		t.Errorf("HolderCount = %d, want 2 from num_accounts", obs.HolderCount)
	}
}

// The issuer is FLAGGED and kept, never dropped. Decision 1: the exclusion is
// decision D-5's and it is not this package's to make.
func TestGetHoldersKeepsTheIssuerAndFlagsIt(t *testing.T) {
	f := newFakeHorizon(t)
	page := fmt.Sprintf(`{"_embedded":{"records":[
	  {"id":%q,"paging_token":"a","balances":[{"balance":"900.0000000","asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]},
	  {"id":%q,"paging_token":"b","balances":[{"balance":"100.0000000","asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]}
	]}}`, testUSTRY.Issuer, testUSTRY.Issuer, accountID(1), testUSTRY.Issuer)
	f.withHolders(assetSummary("1000.0000000", 2, true), []string{page})
	c, _ := f.client()

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	if len(obs.Holders) != 2 {
		t.Fatalf("read %d holders, want 2: the issuer must not be filtered out here", len(obs.Holders))
	}
	var issuers int
	for _, h := range obs.Holders {
		if h.IsIssuer {
			issuers++
			if h.AccountID != testUSTRY.Issuer {
				t.Errorf("IsIssuer set on %s, which is not the issuer", h.AccountID)
			}
		}
	}
	if issuers != 1 {
		t.Errorf("%d holders flagged as the issuer, want exactly 1", issuers)
	}
}

// Decision 4: account id order, whatever order the pages arrived in.
func TestGetHoldersSortsByAccountID(t *testing.T) {
	f := newFakeHorizon(t)
	page := fmt.Sprintf(`{"_embedded":{"records":[
	  {"id":%q,"paging_token":"a","balances":[{"balance":"1.0000000","asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]},
	  {"id":%q,"paging_token":"b","balances":[{"balance":"2.0000000","asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]},
	  {"id":%q,"paging_token":"c","balances":[{"balance":"3.0000000","asset_type":"credit_alphanum12","asset_code":"USTRY","asset_issuer":%q}]}
	]}}`, accountID(9), testUSTRY.Issuer, accountID(2), testUSTRY.Issuer, accountID(5), testUSTRY.Issuer)
	f.withHolders(assetSummary("6.0000000", 3, true), []string{page})
	c, _ := f.client()

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	for i := 1; i < len(obs.Holders); i++ {
		if obs.Holders[i-1].AccountID >= obs.Holders[i].AccountID {
			t.Fatalf("holders are not in account id order: %s before %s",
				obs.Holders[i-1].AccountID, obs.Holders[i].AccountID)
		}
	}
}

func TestGetHoldersPagesUntilAShortPage(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", holderPageLimit+3, true), []string{
		accountsPage(0, holderPageLimit, "1.0000000"),
		accountsPage(holderPageLimit, 3, "1.0000000"),
	})
	c, _ := f.client()

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	if len(obs.Holders) != holderPageLimit+3 {
		t.Errorf("read %d holders, want %d", len(obs.Holders), holderPageLimit+3)
	}
	if obs.Truncated() {
		t.Error("Truncated is true although the last page was short")
	}
	if obs.Raw.Pages != 2 {
		t.Errorf("Pages = %d, want 2", obs.Raw.Pages)
	}
	if f.hits["/accounts"] != 2 {
		t.Errorf("%d requests to /accounts, want 2", f.hits["/accounts"])
	}
}

// Decision 2. The cap stops the read, the flag says so, and Horizon's own count
// stays beside the number actually read so the gap is visible in the file.
func TestGetHoldersTruncatesLoudly(t *testing.T) {
	f := newFakeHorizon(t)
	full := accountsPage(0, holderPageLimit, "1.0000000")
	f.withHolders(assetSummary("9999.0000000", 100000, true), []string{full, full, full})
	c, _ := f.client(func(cfg *Config) { cfg.MaxHolderPages = 2 })

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	if !obs.Truncated() {
		t.Fatal("Truncated is false although the cap stopped the read")
	}
	if obs.Raw.Pages != 2 {
		t.Errorf("Pages = %d, want the cap of 2", obs.Raw.Pages)
	}
	if f.hits["/accounts"] != 2 {
		t.Errorf("%d requests to /accounts, want the cap of 2", f.hits["/accounts"])
	}
	if obs.Raw.HoldersRead == obs.Raw.HolderCount {
		t.Error("HoldersRead equals HolderCount on a truncated read, so the gap is invisible in the file")
	}
	if obs.Raw.HolderCount != 100000 {
		t.Errorf("HolderCount = %d, want Horizon's own 100000", obs.Raw.HolderCount)
	}
}

// Decision 3. Two pages served from two ledgers is a reading that describes
// neither of them exactly, and it is kept with the flag rather than discarded.
func TestGetHoldersRecordsALedgerStraddle(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", holderPageLimit+1, true), []string{
		accountsPage(0, holderPageLimit, "1.0000000"),
		accountsPage(holderPageLimit, 1, "1.0000000"),
	})
	base := f.handler["/accounts"]
	f.handler["/accounts"] = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") != "" {
			w.Header().Set("Latest-Ledger", strconv.Itoa(testLedger+1))
		}
		base(w, r)
	}
	c, _ := f.client()

	obs, err := c.GetHolders(context.Background(), testUSTRY)
	if err != nil {
		t.Fatalf("GetHolders: %v", err)
	}
	if obs.Raw.Atomic {
		t.Error("Atomic is true although the two pages came from different ledgers")
	}
	if obs.Raw.FirstLedger != testLedger || obs.Raw.LastLedger != testLedger+1 {
		t.Errorf("ledgers = %d and %d, want %d and %d",
			obs.Raw.FirstLedger, obs.Raw.LastLedger, testLedger, testLedger+1)
	}
}

// The same trap VerifyAsset exists for, one endpoint over: a wrong asset type
// returns an answer, and the answer is an empty holder list, which is what a
// genuinely unheld asset looks like.
func TestGetHoldersRefusesAWrongAssetType(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", 1, true), []string{accountsPage(0, 1, "1.0000000")})
	c, _ := f.client()

	wrong := testUSTRY
	wrong.Type = "credit_alphanum4"

	_, err := c.GetHolders(context.Background(), wrong)
	if err == nil {
		t.Fatal("a declared type Horizon disagrees with was accepted")
	}
	if !strings.Contains(err.Error(), "Horizon says credit_alphanum12") {
		t.Errorf("error does not name what Horizon said: %v", err)
	}
}

// A skip here would shrink the denominator of every concentration figure and
// leave nothing anywhere saying it happened, so it is an error instead.
func TestGetHoldersRefusesAnAccountWithNoBalanceInTheAsset(t *testing.T) {
	f := newFakeHorizon(t)
	page := fmt.Sprintf(`{"_embedded":{"records":[
	  {"id":%q,"paging_token":"a","balances":[{"balance":"5.0000000","asset_type":"credit_alphanum4","asset_code":"USDC","asset_issuer":%q}]}
	]}}`, accountID(1), testUSDC.Issuer)
	f.withHolders(assetSummary("1000.0000000", 1, true), []string{page})
	c, _ := f.client()

	_, err := c.GetHolders(context.Background(), testUSTRY)
	if err == nil {
		t.Fatal("an account with no balance in the asset was skipped silently")
	}
	if !strings.Contains(err.Error(), "no readable balance") {
		t.Errorf("error does not say what happened: %v", err)
	}
}

func TestGetHoldersRefusesTheNativeAsset(t *testing.T) {
	f := newFakeHorizon(t)
	c, _ := f.client()

	_, err := c.GetHolders(context.Background(), domain.Asset{Type: domain.AssetTypeNative})
	if !errors.Is(err, ErrNativeHasNoTrustlines) {
		t.Fatalf("err = %v, want ErrNativeHasNoTrustlines", err)
	}
	if f.hits["/accounts"] != 0 {
		t.Error("a request was made for an asset that has no trustlines to enumerate")
	}
}
