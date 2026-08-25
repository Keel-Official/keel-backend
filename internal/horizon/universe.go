// The reads behind the candidate universe: every issuer of a ticker, and the
// home_domain each issuer claims.
//
// THIS FILE PROPOSES AND NEVER SELECTS. It gathers evidence about assets and
// applies no threshold to any of it. There is deliberately no minimum trustline
// count, no minimum balance, no "active" flag and no top-N here. Those are the
// inclusion criteria and they belong to docs/methodology/02-pair-selection.md
// section 5, which is red and unwritten. A comparison against a constant that
// decided whether an asset belonged would be that document, written here by
// accident and by the wrong hand.
//
// THE THREE DESIGN DECISIONS THIS ZONE ASKS FOR, each with the alternative
// rejected.
//
//  1. THE TICKER IS A QUERY, NEVER AN IDENTITY. AssetsByCode asks Horizon for one
//     asset_code and returns EVERY issuer that answers, as separate rows that are
//     never merged. Identity stays the pair (code, issuer) at every step, and the
//     one place this file compares assets it compares both halves. Rejected
//     alternative: returning the single "best" record per code, chosen by holder
//     count, which is one line shorter and is exactly the bug this repository has
//     already paid for. 97 distinct assets carry the code AQUA, 13 of them on an
//     aqua-flavored domain, and the busiest-looking one is not the real one; a
//     ranked pick would have promoted an impostor holding three liquidity pools
//     and no stellar.toml.
//
//  2. PAGING FOLLOWS _links.next UNTIL A PAGE IS EMPTY, AND COUNTS THE PAGES. A
//     first page of 200 with order=asc holds the OLDEST records and looks like a
//     complete answer, which is trap 2 and how the real AQUA ends up unseen on a
//     later page. The page count is returned rather than logged, so a caller can
//     put it in the output and a reader can tell a one-page ticker from a
//     truncated walk. Rejected alternative: a single request at limit=200 with a
//     comment saying it is enough, which is true for most tickers, silently false
//     for the ones that matter, and gives a reader nothing to check.
//
//  3. THE ASSET TYPE IS READ, NEVER INFERRED FROM THE CODE LENGTH. /assets keyed
//     on asset_code alone returns both widths, and each record states its own
//     asset_type, so this file never has to guess and never asks for a width.
//     That closes trap 1 by construction rather than by care: USTRY is five
//     characters and credit_alphanum12, and asking for it as credit_alphanum4
//     returns an empty array with no error at all. Rejected alternative: querying
//     each code twice, once per width, and merging, which doubles the request
//     count against an hourly budget and reintroduces the guess it was meant to
//     remove.

package horizon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// assetsPageLimit is the page size asked for on /assets. It is the maximum
// Horizon serves, which minimizes the number of round trips; it is NOT a cap on
// the answer, because the walk follows _links.next past it.
const assetsPageLimit = 200

// maxAssetPages bounds one ticker's walk. It is a runaway guard and not a
// selection rule: reaching it means one code has more than twenty thousand
// issuers, which is a Horizon or a parsing problem rather than a market. A walk
// that hits it reports Truncated, and the caller must say so rather than
// presenting a short list as a complete one.
const maxAssetPages = 100

// AssetStat is one /assets record, kept whole.
//
// Every amount is a STRING and is carried exactly as Horizon sent it. Parsing
// them into decimals here would be this package deciding what the numbers mean;
// carrying the digits means the output file holds what the endpoint said, and a
// reader can compare the file against a fresh curl byte for byte. No float
// appears on this path at any point.
type AssetStat struct {
	Code   string           `json:"code"`
	Issuer string           `json:"issuer"`
	Type   domain.AssetType `json:"asset_type"`

	// ContractID is the Stellar Asset Contract address, present only for assets
	// that have one. Empty is a fact about the asset, not a missing reading.
	ContractID string `json:"contract_id,omitempty"`

	AuthorizedAccounts                 int `json:"authorized_trustlines"`
	AuthorizedToMaintainLiabilitiesAcc int `json:"authorized_to_maintain_liabilities_trustlines"`
	UnauthorizedAccounts               int `json:"unauthorized_trustlines"`

	AuthorizedBalance   string `json:"authorized_balance"`
	UnauthorizedBalance string `json:"unauthorized_balance"`

	NumLiquidityPools     int    `json:"num_liquidity_pools"`
	LiquidityPoolsAmount  string `json:"liquidity_pools_amount"`
	NumClaimableBalances  int    `json:"num_claimable_balances"`
	ClaimableBalancesAmt  string `json:"claimable_balances_amount"`
	NumContracts          int    `json:"num_contracts"`
	ContractsAmount       string `json:"contracts_amount"`
	TomlURLReportedByHzn  string `json:"toml_url_reported_by_horizon"`
	AuthRequired          bool   `json:"auth_required"`
	AuthRevocable         bool   `json:"auth_revocable"`
	AuthImmutable         bool   `json:"auth_immutable"`
	AuthClawbackEnabled   bool   `json:"auth_clawback_enabled"`
	PagingTokenOnThisRead string `json:"-"`
}

// Asset rebuilds the domain identity. Both halves, always.
func (a AssetStat) Asset() domain.Asset {
	return domain.Asset{Code: a.Code, Issuer: a.Issuer, Type: a.Type}
}

// CodeReading is every issuer Horizon returned for one ticker, plus what it cost
// to find out.
type CodeReading struct {
	Code   string
	Assets []AssetStat

	// Pages walked, and whether the walk stopped at maxAssetPages rather than at
	// the end of the collection. Truncated is the difference between "this ticker
	// has 97 issuers" and "this ticker has at least 20000".
	Pages     int
	Truncated bool

	// LedgerSeq is the Latest-Ledger of the FIRST page. A walk of several pages
	// can straddle a ledger close, so this names the ledger the reading started
	// at rather than pretending the whole walk was atomic.
	LedgerSeq uint32
	ReadAt    time.Time
}

// AssetsByCode returns every issuer of one ticker.
//
// The result is sorted by issuer, so two runs over the same ledger produce the
// same order. Horizon's own paging order is by (code, issuer, type) already, but
// relying on a server's ordering for a file that has to be byte-identical is
// relying on something nobody promised.
func (c *Client) AssetsByCode(ctx context.Context, code string) (CodeReading, error) {
	out := CodeReading{Code: code, ReadAt: c.cfg.Now().UTC()}
	if code == "" {
		return out, fmt.Errorf("horizon: asset code is empty")
	}

	q := url.Values{}
	q.Set("asset_code", code)
	q.Set("limit", fmt.Sprintf("%d", assetsPageLimit))
	q.Set("order", "asc")

	path := "/assets"
	query := q
	seen := map[string]bool{}

	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if out.Pages >= maxAssetPages {
			out.Truncated = true
			break
		}

		body, latest, err := c.get(ctx, path, query, false)
		if err != nil {
			return out, fmt.Errorf("horizon: assets %s page %d: %w", code, out.Pages+1, err)
		}
		out.Pages++
		if out.Pages == 1 {
			out.LedgerSeq = latest
		}

		var res assetsPage
		if err := json.Unmarshal(body, &res); err != nil {
			return out, fmt.Errorf("horizon: assets %s page %d: decode: %w", code, out.Pages, err)
		}
		// An EMPTY page is the end of the collection. Horizon serves a next link
		// on every page including the last, so a caller that stops when the link
		// disappears never stops.
		if len(res.Embedded.Records) == 0 {
			break
		}

		for _, r := range res.Embedded.Records {
			// Filter by the identifier that was asked for. An endpoint returning
			// an array is not a promise that every element answers the question,
			// and reading a neighbor silently is the failure mode here rather
			// than an error. Horizon has never been seen to do this on /assets;
			// the check costs nothing and its absence is not detectable later.
			if r.AssetCode != code {
				continue
			}
			// Identity is the PAIR. Two records sharing an issuer but differing
			// in type are two assets, so the dedup key carries all three fields.
			key := r.AssetCode + "\x00" + r.AssetIssuer + "\x00" + r.AssetType
			if seen[key] {
				continue
			}
			seen[key] = true
			out.Assets = append(out.Assets, r.stat())
		}

		next := strings.TrimSpace(res.Links.Next.Href)
		if next == "" {
			break
		}
		u, err := url.Parse(next)
		if err != nil {
			return out, fmt.Errorf("horizon: assets %s: next link %q: %w", code, next, err)
		}
		// Follow the server's own cursor rather than rebuilding one. The cursor
		// format is Horizon's business and has changed shape before.
		path = u.Path
		query = u.Query()
	}

	sort.Slice(out.Assets, func(i, j int) bool {
		if out.Assets[i].Issuer != out.Assets[j].Issuer {
			return out.Assets[i].Issuer < out.Assets[j].Issuer
		}
		return out.Assets[i].Type < out.Assets[j].Type
	})
	return out, nil
}

// assetsPage is the paged form of assetsResponse. It is separate because the
// existing assetsResponse is used by VerifyAsset and GetHolders, which read one
// page on purpose and would be given a paging contract they do not want.
type assetsPage struct {
	Links struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded struct {
		Records []assetStatRecord `json:"records"`
	} `json:"_embedded"`
}

// assetStatRecord is the whole /assets record. assetRecord in decode.go reads the
// subset that VerifyAsset and GetHolders need and is left alone: widening it
// would give two callers fields they do not use and a third a reason to think
// those fields are checked somewhere.
type assetStatRecord struct {
	Links struct {
		Toml struct {
			Href string `json:"href"`
		} `json:"toml"`
	} `json:"_links"`

	AssetType   string `json:"asset_type"`
	AssetCode   string `json:"asset_code"`
	AssetIssuer string `json:"asset_issuer"`
	PagingToken string `json:"paging_token"`
	ContractID  string `json:"contract_id"`

	NumClaimableBalances int `json:"num_claimable_balances"`
	NumLiquidityPools    int `json:"num_liquidity_pools"`
	NumContracts         int `json:"num_contracts"`

	ClaimableBalancesAmount string `json:"claimable_balances_amount"`
	LiquidityPoolsAmount    string `json:"liquidity_pools_amount"`
	ContractsAmount         string `json:"contracts_amount"`

	Accounts struct {
		Authorized                     int `json:"authorized"`
		AuthorizedToMaintainLiabilties int `json:"authorized_to_maintain_liabilities"`
		Unauthorized                   int `json:"unauthorized"`
	} `json:"accounts"`
	Balances struct {
		Authorized                     string `json:"authorized"`
		AuthorizedToMaintainLiabilties string `json:"authorized_to_maintain_liabilities"`
		Unauthorized                   string `json:"unauthorized"`
	} `json:"balances"`
	Flags struct {
		AuthRequired        bool `json:"auth_required"`
		AuthRevocable       bool `json:"auth_revocable"`
		AuthImmutable       bool `json:"auth_immutable"`
		AuthClawbackEnabled bool `json:"auth_clawback_enabled"`
	} `json:"flags"`
}

func (r assetStatRecord) stat() AssetStat {
	return AssetStat{
		Code:                               r.AssetCode,
		Issuer:                             r.AssetIssuer,
		Type:                               domain.AssetType(r.AssetType),
		ContractID:                         r.ContractID,
		AuthorizedAccounts:                 r.Accounts.Authorized,
		AuthorizedToMaintainLiabilitiesAcc: r.Accounts.AuthorizedToMaintainLiabilties,
		UnauthorizedAccounts:               r.Accounts.Unauthorized,
		AuthorizedBalance:                  amountOrZero(r.Balances.Authorized),
		UnauthorizedBalance:                amountOrZero(r.Balances.Unauthorized),
		NumLiquidityPools:                  r.NumLiquidityPools,
		LiquidityPoolsAmount:               amountOrZero(r.LiquidityPoolsAmount),
		NumClaimableBalances:               r.NumClaimableBalances,
		ClaimableBalancesAmt:               amountOrZero(r.ClaimableBalancesAmount),
		NumContracts:                       r.NumContracts,
		ContractsAmount:                    amountOrZero(r.ContractsAmount),
		TomlURLReportedByHzn:               r.Links.Toml.Href,
		AuthRequired:                       r.Flags.AuthRequired,
		AuthRevocable:                      r.Flags.AuthRevocable,
		AuthImmutable:                      r.Flags.AuthImmutable,
		AuthClawbackEnabled:                r.Flags.AuthClawbackEnabled,
		PagingTokenOnThisRead:              r.PagingToken,
	}
}

// amountOrZero normalises a MISSING amount to "0" and leaves a present one
// exactly as Horizon wrote it, trailing zeros included.
//
// The two cases are different and only one is being changed: an absent field
// means Horizon reported nothing, and writing "" into a column of amounts makes
// the file harder to read than writing the zero it means. A present "0.0000000"
// keeps all seven of its decimals, because rewriting it to "0" would be this
// package reformatting evidence.
func amountOrZero(s string) string {
	if strings.TrimSpace(s) == "" {
		return "0"
	}
	return s
}

// ---------------------------------------------------------------- Accounts

// HomeDomain returns the home_domain an issuer account claims for itself, and
// whether the account sets one at all.
//
// THIS IS ONE HALF OF AN IDENTITY PROOF AND IS WORTHLESS ALONE. home_domain is
// written by whoever controls the account, so it proves only that the issuer
// typed a domain in. The other half is that domain's SEP-1 stellar.toml naming
// the exact (code, issuer) pair back, which is fetched outside this package
// because it is not a Horizon endpoint. Neither direction alone establishes
// anything, and a tool that checked only this one would accept any issuer
// willing to type "circle.com".
//
// An account with no home_domain is a fact to record, not an error: it is the
// ordinary state of an asset whose issuer never filled the field in.
func (c *Client) HomeDomain(ctx context.Context, issuer string) (string, error) {
	if issuer == "" {
		return "", fmt.Errorf("horizon: issuer is empty")
	}
	body, _, err := c.get(ctx, "/accounts/"+url.PathEscape(issuer), nil, false)
	if err != nil {
		return "", fmt.Errorf("horizon: account %s: %w", issuer, err)
	}
	var res struct {
		AccountID  string `json:"account_id"`
		HomeDomain string `json:"home_domain"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", fmt.Errorf("horizon: account %s: decode: %w", issuer, err)
	}
	// Filter by the identifier that was asked for, even on a single-object
	// endpoint. A response describing a different account is the failure mode
	// that looks like data.
	if res.AccountID != "" && res.AccountID != issuer {
		return "", fmt.Errorf("horizon: asked for account %s and got %s", issuer, res.AccountID)
	}
	return strings.TrimSpace(res.HomeDomain), nil
}
