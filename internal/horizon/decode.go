// The shapes Horizon actually sends, and the conversions into domain types.
//
// Kept apart from client.go so the transport concerns and the wire format do not
// read as one thing. Every struct here is unexported: they are a description of
// somebody else's JSON, and letting one escape into another package would make
// Horizon's field names part of Keel's internal API.
//
// Rejected alternative: decoding into map[string]any and reaching in by key,
// which survives an upstream field rename by silently producing zero. A struct
// that fails to decode is the louder failure, and a silent zero in a price or a
// reserve is the worst outcome this adapter has.
package horizon

import (
	"fmt"
	"net/url"
	"time"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// ---------------------------------------------------------------- Order book

// GET /order_book
//
// The response carries no ledger sequence of its own, which is why the client
// reads the Latest-Ledger header. `base` and `counter` echo the request and are
// checked, because naming the wrong asset type returns an empty book rather than
// an error.
type orderBookResponse struct {
	Base    assetRef     `json:"base"`
	Counter assetRef     `json:"counter"`
	Bids    []priceLevel `json:"bids"`
	Asks    []priceLevel `json:"asks"`
}

type priceLevel struct {
	// PriceR is the exact fraction and the only price this adapter reads.
	//
	// Price is decoded and never used in arithmetic. It is Horizon's rounded
	// string, which has already lost precision, and it is kept only so a raw
	// recording and a parsed level can be compared by eye. Naming it here does
	// NOT make an upstream removal fail: encoding/json leaves a missing field at
	// its zero value without complaint, so nothing in this struct detects a
	// field that stops arriving. What detects a missing price_r is
	// priceFraction.price, which rejects a zero denominator.
	PriceR priceFraction `json:"price_r"`
	Price  string        `json:"price"`
	Amount string        `json:"amount"`
}

type assetRef struct {
	AssetType   string `json:"asset_type"`
	AssetCode   string `json:"asset_code"`
	AssetIssuer string `json:"asset_issuer"`
}

func (r assetRef) matches(a domain.Asset) bool {
	if a.IsNative() {
		return r.AssetType == string(domain.AssetTypeNative)
	}
	return r.AssetType == string(a.Type) && r.AssetCode == a.Code && r.AssetIssuer == a.Issuer
}

func (r assetRef) describe() string {
	if r.AssetType == string(domain.AssetTypeNative) {
		return "native"
	}
	return r.AssetType + " " + r.AssetCode + ":" + r.AssetIssuer
}

type side int

const (
	sideBid side = iota
	sideAsk
)

// levels converts one side of the book into domain levels denominated in BASE
// units, which is what domain.Level.Amount is defined as.
//
// The bid conversion is the open question documented on BidAmountUnit. When the
// quote reading applies, amountBase = amount × d / n rather than
// amount / (n/d): the multiplication is exact and only the final division
// rounds, at shopspring's DivisionPrecision. Notional then recovers the original
// quote amount to within that precision, which is well inside the fixture
// tolerance of 1e-7 at these magnitudes, and the raw bytes keep the unrounded
// figure regardless.
func (c *Client) levels(in []priceLevel, s side) ([]domain.Level, error) {
	out := make([]domain.Level, 0, len(in))
	for i, l := range in {
		price, err := l.PriceR.price()
		if err != nil {
			return nil, fmt.Errorf("level %d: %w", i, err)
		}
		amount, err := decimal.NewFromString(l.Amount)
		if err != nil {
			return nil, fmt.Errorf("level %d: amount %q: %w", i, l.Amount, err)
		}
		if !amount.IsPositive() {
			return nil, fmt.Errorf("level %d: amount %q is not positive", i, l.Amount)
		}
		if s == sideBid && c.cfg.BidAmountUnit == BidAmountUnitQuote {
			amount = amount.Mul(decimal.NewFromInt(price.D)).Div(decimal.NewFromInt(price.N))
		}
		out = append(out, domain.Level{Price: price, Amount: amount})
	}
	return out, nil
}

// ---------------------------------------------------------------- Pools

// GET /liquidity_pools?reserves=A,B
//
// Shape confirmed against docs/evidences/reserves_pool.txt rather than taken
// from documentation. There is no paging loop: the reserves filter names an
// exact asset pair, so the result is one pool per fee tier. That is an
// assumption about the protocol rather than a guarantee, so poolReserves refuses
// a full page instead of silently treating it as the whole answer.
type poolsResponse struct {
	Embedded struct {
		Records []poolRecord `json:"records"`
	} `json:"_embedded"`
}

type poolRecord struct {
	ID       string `json:"id"`
	FeeBP    int32  `json:"fee_bp"`
	Reserves []struct {
		Asset  string `json:"asset"`
		Amount string `json:"amount"`
	} `json:"reserves"`
	LastModifiedLedger uint32 `json:"last_modified_ledger"`
}

// horizonAsset is Horizon's own spelling of an asset inside a reserves list or a
// reserves filter: "native", or "CODE:ISSUER".
//
// domain.Asset.String() cannot be used for this. It returns "XLM" for the native
// asset, which is the human name and not a value Horizon accepts.
func horizonAsset(a domain.Asset) string {
	if a.IsNative() {
		return "native"
	}
	return a.Code + ":" + a.Issuer
}

// poolPageLimit is the limit sent on the pools query. A response holding exactly
// this many records means there may be more behind it, and dropping a pool
// silently would remove AMM liquidity from a depth figure with nothing in the
// output to say so, so a full page is an error rather than an answer.
const poolPageLimit = 200

func poolReserves(res poolsResponse, base, quote domain.Asset) ([]domain.PoolReserves, error) {
	if len(res.Embedded.Records) >= poolPageLimit {
		return nil, fmt.Errorf("%d pools returned for one asset pair, which fills the page; paging is not implemented",
			len(res.Embedded.Records))
	}
	wantBase, wantQuote := horizonAsset(base), horizonAsset(quote)
	out := make([]domain.PoolReserves, 0, len(res.Embedded.Records))

	for _, rec := range res.Embedded.Records {
		var rb, rq *decimal.Decimal
		for _, r := range rec.Reserves {
			amount, err := decimal.NewFromString(r.Amount)
			if err != nil {
				return nil, fmt.Errorf("pool %s: reserve %q: %w", rec.ID, r.Amount, err)
			}
			switch r.Asset {
			case wantBase:
				v := amount
				rb = &v
			case wantQuote:
				v := amount
				rq = &v
			}
		}
		// The filter asked for exactly this pair, so a record missing either
		// side means Horizon answered a different question than it was asked.
		// Skipping it quietly would drop AMM liquidity out of the depth figure
		// with nothing in the output to say so.
		if rb == nil || rq == nil {
			return nil, fmt.Errorf("pool %s does not hold both %s and %s", rec.ID, wantBase, wantQuote)
		}
		// FeeBP comes from the response. domain.PoolReserves says not to
		// hardcode 30, and a fee is grossed into every AMM depth figure.
		out = append(out, domain.PoolReserves{
			PoolID:       rec.ID,
			ReserveBase:  *rb,
			ReserveQuote: *rq,
			FeeBP:        rec.FeeBP,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------- Ledger

// GET /ledgers/{sequence}, for the close time. /order_book has none.
//
// closed_at is RFC 3339, which encoding/json decodes into time.Time without
// help. Sequence is read back and compared against the ledger that was asked
// for, because a snapshot carrying one ledger's sequence and another's close time
// would be wrong in a way nothing downstream could detect.
type ledgerResponse struct {
	Sequence uint32    `json:"sequence"`
	ClosedAt time.Time `json:"closed_at"`
}

// ---------------------------------------------------------------- Query params

// addAsset writes the selling_asset_* or buying_asset_* triple for one asset.
//
// The type is written from domain.Asset.Type and is never inferred from
// len(Code). That inference is the trap recorded on domain.Asset: USTRY has a
// five character code, so a length rule would call it alphanum4 and Horizon
// would answer with an empty book and no error.
func addAsset(q url.Values, prefix string, a domain.Asset) {
	q.Set(prefix+"_asset_type", string(a.Type))
	if a.IsNative() {
		return
	}
	q.Set(prefix+"_asset_code", a.Code)
	q.Set(prefix+"_asset_issuer", a.Issuer)
}

// ---------------------------------------------------------------- Assets

// GET /assets?asset_code=&asset_issuer=, used only by VerifyAsset.
type assetsResponse struct {
	Embedded struct {
		Records []struct {
			AssetType   string `json:"asset_type"`
			AssetCode   string `json:"asset_code"`
			AssetIssuer string `json:"asset_issuer"`
		} `json:"records"`
	} `json:"_embedded"`
}
