// The holder distribution reading, and the second thing in this repository that
// cannot be taken later.
//
// docs/methodology/07-supporting-metrics.md promises holder concentration and a
// volume-to-supply ratio. Both are computed from who holds the asset and how
// much of it exists, and Horizon serves NEITHER of those for a past ledger. An
// order book can be reconstructed from the operations that built it, which is
// what `offers-implied` in the golden fixture means, but a trustline balance at
// ledger N is not derivable from anything Horizon exposes: only the current
// balance is served, and the history lives in Hubble, which DEC-002 defers.
//
// So this is the recorder's argument again, one asset over. A day without a
// holder reading is a day of holder distribution lost permanently, and the loss
// is silent, because the endpoint answers today's question perfectly well.
//
// YELLOW ZONE, four design decisions:
//
//  1. NOTHING IS EXCLUDED HERE. Every account with a trustline is returned, the
//     issuer among them and flagged as such. Which accounts belong in a
//     concentration population is decision D-5, and its file says of itself that
//     no decisions are recorded in it yet. Rejected alternative: dropping the
//     issuer in this package, which is the obvious exclusion and still the wrong
//     place for it, because it would bake a methodology choice into an adapter
//     and leave the recorded evidence unable to answer the question the other
//     way round.
//
//  2. TRUNCATION IS EXPLICIT AND NEVER SILENT. Paging stops at MaxHolderPages
//     and the observation says so, carries how many pages it read, and carries
//     Horizon's own account count beside the number actually read. A truncated
//     reading CANNOT answer a concentration question, because the holder it is
//     missing may be the largest one, and it is still worth keeping as a lower
//     bound on the holder count. Rejected alternative: paging until Horizon runs
//     out, which on an asset with a hundred thousand trustlines spends an entire
//     hourly budget on one asset in one round and starves every other reading.
//
//  3. A HOLDER READING IS NOT ATOMIC AND SAYS SO. Pages are separate requests,
//     so they can straddle a ledger boundary while accounts are being created or
//     closed underneath. The Latest-Ledger of the first and last page are both
//     recorded and Atomic is false when they differ, which is decision 2 in
//     client.go applied to a second endpoint rather than a new idea.
//
//  4. THE ORDER IS BY ACCOUNT ID, NOT BY BALANCE. Ranking holders is the
//     computation's job and sorting by balance here would be the first line of
//     it. Sorting by account id is what makes two readings of an unchanged
//     ledger byte-identical no matter what order Horizon paged them in, which is
//     non-negotiable rule 2 in the repository brief. Rejected alternative:
//     keeping Horizon's paging order, which is deterministic for one run and not
//     across two.
package horizon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// ErrNativeHasNoTrustlines is returned for XLM. The native asset has no
// trustlines at all, so /accounts cannot enumerate its holders, and a holder
// concentration figure for XLM would need a different source entirely. Returning
// an empty list instead would be indistinguishable from an asset nobody holds.
var ErrNativeHasNoTrustlines = errors.New("horizon: the native asset has no trustlines to enumerate")

// holderPageLimit is the page size sent to /accounts. Horizon's own maximum.
const holderPageLimit = 200

// defaultMaxHolderPages caps one reading at 5000 accounts, which is 25 requests
// out of an hourly budget of 3000. See decision 2 for why the cap exists rather
// than an unbounded loop.
const defaultMaxHolderPages = 25

// Holder is one account holding the asset, and deliberately nothing more.
type Holder struct {
	AccountID string
	Balance   decimal.Decimal

	// IsIssuer is a FLAG, not a filter. The issuer holds supply that has not
	// been distributed, and whether that belongs in a concentration denominator
	// is decision D-5. Flagging it here means the decision can be applied later
	// without re-fetching, and applied both ways against the same recording.
	IsIssuer bool
}

// HolderObservation is one holder reading plus the evidence for it.
type HolderObservation struct {
	Asset   domain.Asset
	Holders []Holder

	// Supply is the issued amount Horizon reports for the asset, authorized
	// balances only. See assetRecord.supply for what is deliberately left out.
	Supply decimal.Decimal

	// HolderCount is HORIZON's count, which is the number to compare len(Holders)
	// against. They differ exactly when the reading was truncated, and that
	// comparison is the only way a reader of the file can tell.
	HolderCount int

	Raw RawHolders
}

// Truncated reports whether the page cap stopped this reading before Horizon ran
// out of accounts. A truncated reading answers a holder COUNT question as a
// lower bound and answers a concentration question not at all.
func (o HolderObservation) Truncated() bool { return o.Raw.Truncated }

// RawHolders is what gets written to disk, and it holds the bytes as well as the
// conclusions, for the reason stated in decision 3 of client.go: parsing is a
// claim about what the bytes mean, and the bytes have to outlive the claim.
type RawHolders struct {
	RequestedAsset string    `json:"requested_asset"`
	FetchedAt      time.Time `json:"fetched_at"`

	// FirstLedger and LastLedger are Horizon's Latest-Ledger on the first and
	// last account page. Atomic is false when they differ, which means the pages
	// describe two different ledgers. See decision 3.
	FirstLedger uint32 `json:"first_page_latest_ledger"`
	LastLedger  uint32 `json:"last_page_latest_ledger"`
	Atomic      bool   `json:"atomic"`

	Pages     int  `json:"pages_read"`
	PageLimit int  `json:"page_limit"`
	MaxPages  int  `json:"max_pages"`
	Truncated bool `json:"truncated"`

	// HoldersRead is len(Holders) and HolderCount is Horizon's own figure. Both
	// are written because their disagreement is the truncation, and a file has
	// to be readable on its own.
	HoldersRead int `json:"holders_read"`
	HolderCount int `json:"holder_count_reported"`

	MethodologyVersion string `json:"methodology_version"`

	AssetSummary json.RawMessage   `json:"asset_summary"`
	Accounts     []json.RawMessage `json:"account_pages"`
}

// GetHolders reads the asset summary and every page of trustline holders up to
// the page cap. One request for the summary plus one per page.
//
// The asset type is checked against the summary for the same reason VerifyAsset
// exists: naming the wrong type returns an answer rather than an error, and here
// the answer would be an empty holder list, which reads exactly like an asset
// nobody holds.
func (c *Client) GetHolders(ctx context.Context, a domain.Asset) (HolderObservation, error) {
	var obs HolderObservation
	if a.IsNative() {
		return obs, fmt.Errorf("holders %s: %w", a, ErrNativeHasNoTrustlines)
	}
	obs.Asset = a

	sumQ := url.Values{}
	sumQ.Set("asset_code", a.Code)
	sumQ.Set("asset_issuer", a.Issuer)
	sumBody, _, err := c.get(ctx, "/assets", sumQ, false)
	if err != nil {
		return obs, fmt.Errorf("holders %s: asset summary: %w", a, err)
	}
	var sum assetsResponse
	if err := json.Unmarshal(sumBody, &sum); err != nil {
		return obs, fmt.Errorf("holders %s: decode asset summary: %w", a, err)
	}
	rec, found := findAssetRecord(sum, a)
	if !found {
		return obs, fmt.Errorf("holders %s: no such asset on Horizon", a)
	}
	if rec.AssetType != string(a.Type) {
		return obs, fmt.Errorf("holders %s: declared %s, Horizon says %s", a, a.Type, rec.AssetType)
	}
	if s := rec.supply(); s != "" {
		supply, err := decimal.NewFromString(s)
		if err != nil {
			return obs, fmt.Errorf("holders %s: supply %q: %w", a, s, err)
		}
		obs.Supply = supply
	}
	obs.HolderCount = rec.holderCount()

	maxPages := c.cfg.MaxHolderPages

	raw := RawHolders{
		RequestedAsset:     a.String(),
		FetchedAt:          c.cfg.Now().UTC(),
		PageLimit:          holderPageLimit,
		MaxPages:           maxPages,
		HolderCount:        obs.HolderCount,
		MethodologyVersion: domain.MethodologyVersion,
		AssetSummary:       json.RawMessage(sumBody),
	}

	cursor := ""
	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		q.Set("asset", horizonAsset(a))
		q.Set("limit", strconv.Itoa(holderPageLimit))
		q.Set("order", "asc")
		if cursor != "" {
			q.Set("cursor", cursor)
		}

		// The Latest-Ledger header is REQUIRED here, unlike on /ledgers. A holder
		// reading with no ledger number is a set of balances at an unknown moment,
		// which cannot be filed under a ledger and cannot be compared with the
		// pair snapshot taken beside it. Failing is better than filing it under a
		// guess.
		body, latest, err := c.get(ctx, "/accounts", q, true)
		if err != nil {
			return obs, fmt.Errorf("holders %s: page %d: %w", a, page+1, err)
		}
		if page == 0 {
			raw.FirstLedger = latest
		}
		raw.LastLedger = latest
		raw.Pages++
		raw.Accounts = append(raw.Accounts, json.RawMessage(body))

		var res accountsResponse
		if err := json.Unmarshal(body, &res); err != nil {
			return obs, fmt.Errorf("holders %s: decode page %d: %w", a, page+1, err)
		}
		for _, r := range res.Embedded.Records {
			balance, ok := r.balanceOf(a)
			if !ok {
				return obs, fmt.Errorf("holders %s: account %s carries no readable balance in this asset, "+
					"although Horizon returned it as a holder", a, r.ID)
			}
			obs.Holders = append(obs.Holders, Holder{
				AccountID: r.ID,
				Balance:   balance,
				IsIssuer:  r.ID == a.Issuer,
			})
			cursor = r.PagingToken
		}
		if len(res.Embedded.Records) < holderPageLimit {
			raw.Truncated = false
			raw.HoldersRead = len(obs.Holders)
			raw.Atomic = raw.FirstLedger == raw.LastLedger
			obs.Raw = raw
			sortHolders(obs.Holders)
			return obs, nil
		}
	}

	// Fell out of the loop with a full page every time, so there may be more.
	raw.Truncated = true
	raw.HoldersRead = len(obs.Holders)
	raw.Atomic = raw.FirstLedger == raw.LastLedger
	obs.Raw = raw
	sortHolders(obs.Holders)
	return obs, nil
}

// sortHolders puts the list in account id order. See decision 4.
func sortHolders(h []Holder) {
	sort.Slice(h, func(i, j int) bool { return h[i].AccountID < h[j].AccountID })
}

func findAssetRecord(res assetsResponse, a domain.Asset) (assetRecord, bool) {
	for _, r := range res.Embedded.Records {
		if r.AssetCode == a.Code && r.AssetIssuer == a.Issuer {
			return r, true
		}
	}
	return assetRecord{}, false
}
