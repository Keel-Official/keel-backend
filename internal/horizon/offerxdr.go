// Reading the offer an operation left behind, out of a transaction's result XDR.
//
// WHY THIS FILE HAS TO EXIST, and it is not a preference. Reconstructing a past
// order book means applying offer operations in order, and applying them means
// knowing WHICH offer each one produced. Horizon does not say. A create arrives
// as `"offer_id": "0"` and its effects list comes back EMPTY, both measured on
// operation 263453036239003649, the operation that posted the 106.7372828 ask of
// the 22 February 2026 incident. The only place the new offer's identity exists
// is the transaction result, which Horizon serves as base64 XDR and nothing else
// decodes for us.
//
// Three facts were read off the chain before a line of this was written, and each
// one is asserted in offerxdr_test.go against the same real transaction:
//
//	09e1a9d1… manage_sell_offer, create      -> offer 1824788980, 1.2185312 USTRY @ 266843207/2500000
//	3b504c31… manage_buy_offer,  create      -> offer 1824789082, selling 0.0001057 USDC @ 1000/1057
//	60fe039e… manage_buy_offer,  fully taken -> claims offer 1824788980 for 0.0501003 USTRY / 5.3475699 USDC, effect DELETED
//
// The first two are the two levels of testdata/fixtures/ustry_pre_exploit.md. The
// third is the manipulation itself. If this decoder is wrong, those three stop
// matching, which is the whole reason they are the test.
//
// WHAT IS DELIBERATELY NOT DECODED. The seller. An AccountID is 36 bytes of key
// type and raw ed25519, and turning it into a G... string needs base32 with a
// CRC16 that nothing else here needs. The book is prices and amounts; the seller
// is skipped by width and the width is asserted.
//
// THE ONE MEASUREMENT THAT COST A DEBUGGING PASS, recorded so it is not paid
// twice: an AccountID inside an Asset is 36 bytes, not 32. Reading it as 32 puts
// every later field four bytes early, and the failure is not loud: the second
// asset's code decodes as printable text with the type integer glued to its
// front, and the amount comes out as a plausible-looking negative int64.
//
// THE THREE SENTENCES THIS ZONE ASKS FOR. The decision: a hand written reader over
// the exact subset of the XDR needed to reach one operation's ManageOfferSuccess,
// with every other operation result skipped by computing its width. The
// alternative rejected: importing github.com/stellar/go-stellar-sdk/xdr, which
// decodes all of this correctly and is the obvious answer. Why it was rejected:
// that module pulls a large dependency tree into a repository whose go.mod has two
// direct requirements, for one struct on one path, and the surface actually needed
// here is under two hundred lines that can be asserted against three transactions
// anybody can fetch; if a second XDR need ever appears, that trade flips and this
// file should be deleted rather than extended.
package horizon

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrUnsizableResult is returned when an operation result before the one being
// read is of a type this file cannot measure, so the offset to the wanted
// operation cannot be computed.
//
// It is a NAMED error rather than a guess because guessing a width here does not
// fail, it silently decodes the wrong bytes into a plausible offer. A caller
// counts these and reports the count; see the diagnostics in replay.go.
var ErrUnsizableResult = errors.New("horizon: an earlier operation result has a width this decoder cannot compute")

// ErrNotAnOfferResult is returned when the operation at the requested index is
// not a manage offer of any kind, or did not succeed.
var ErrNotAnOfferResult = errors.New("horizon: the operation is not a successful manage offer")

// Stellar operation types whose SUCCESS result carries a body. Skipping one of
// these means computing that body's width.
const (
	opTypePathPaymentStrictReceive = 2
	opTypeManageSellOffer          = 3
	opTypeCreatePassiveSellOffer   = 4
	opTypeAccountMerge             = 8
	opTypeInflation                = 9
	opTypeManageBuyOffer           = 12
	opTypePathPaymentStrictSend    = 13
	opTypeCreateClaimableBalance   = 14
	opTypeInvokeHostFunction       = 24
)

// opResultVoidOnSuccess lists every operation type whose success result is void,
// so the whole OperationResult is three int32s and skipping it is free.
//
// IT IS AN ALLOW LIST AND THAT IS THE POINT. Treating "not one of the types with
// a body" as void is the version of this that was written first and it is wrong
// in the dangerous direction: a protocol version that adds an operation with a
// body would be skipped three int32s short, and the bytes that follow would
// decode into a valid looking offer rather than into an error. An unknown type
// reaches ErrUnsizableResult instead, and the caller counts it.
var opResultVoidOnSuccess = map[int32]bool{
	0:  true, // CREATE_ACCOUNT
	1:  true, // PAYMENT
	5:  true, // SET_OPTIONS
	6:  true, // CHANGE_TRUST
	7:  true, // ALLOW_TRUST
	10: true, // MANAGE_DATA
	11: true, // BUMP_SEQUENCE
	15: true, // CLAIM_CLAIMABLE_BALANCE
	16: true, // BEGIN_SPONSORING_FUTURE_RESERVES
	17: true, // END_SPONSORING_FUTURE_RESERVES
	18: true, // REVOKE_SPONSORSHIP
	19: true, // CLAWBACK
	20: true, // CLAWBACK_CLAIMABLE_BALANCE
	21: true, // SET_TRUST_LINE_FLAGS
	22: true, // LIQUIDITY_POOL_DEPOSIT
	23: true, // LIQUIDITY_POOL_WITHDRAW
	25: true, // EXTEND_FOOTPRINT_TTL
	26: true, // RESTORE_FOOTPRINT
}

// offerEffect is what a manage offer operation did to the order book, as the
// ledger recorded it rather than as the request asked for it. An operation that
// crossed the whole book leaves DELETED behind even though it was a create.
type offerEffect int32

const (
	offerCreated offerEffect = 0
	offerUpdated offerEffect = 1
	offerDeleted offerEffect = 2
)

func (e offerEffect) String() string {
	switch e {
	case offerCreated:
		return "created"
	case offerUpdated:
		return "updated"
	case offerDeleted:
		return "deleted"
	}
	return fmt.Sprintf("effect(%d)", int32(e))
}

// resultingOffer is the offer an operation left resting, plus the offers it took
// on the way.
type resultingOffer struct {
	Effect offerEffect

	// Set only when Effect is created or updated. On deleted the operation left
	// nothing resting, and every field below is zero.
	OfferID int64
	Selling assetRef
	Buying  assetRef

	// Amount is in stroops of the SELLING asset, which is the only unit an
	// OfferEntry has. It is NOT the operation's own amount field: a manage buy
	// offer states its amount in the BUYING asset, and the two differ by the
	// price. Reading the ledger's own entry is how that trap is avoided rather
	// than converted.
	Amount int64

	// PriceN over PriceD is the ledger's exact rational, in units of BUYING per
	// unit of SELLING. domain.Price.Invert() is the only conversion needed to
	// put it in quote per base, and it swaps rather than divides.
	PriceN int32
	PriceD int32

	// Claimed names the offers this operation consumed, in the order the engine
	// took them. It is decoded because the cursor has to pass over it anyway.
	// Consumption is NOT applied from here; see the header of replay.go for why
	// the trade stream is used instead.
	Claimed []claimedOffer
}

// claimedOffer is one ClaimAtom: an offer that was taken, and how much of it.
type claimedOffer struct {
	// OfferID is zero for a liquidity pool claim, which has a pool id and no
	// offer. The distinction matters because a pool is not on the book.
	OfferID      int64
	IsPool       bool
	AmountSold   int64
	AmountBought int64
}

// ParseManageOfferResult decodes the base64 transaction result and returns what
// operation opIndex left resting.
//
// opIndex is ZERO BASED and is the operation's position in the transaction. The
// TOID carries it one based in its low twelve bits; tOIDOperationIndex converts.
func ParseManageOfferResult(resultXDRBase64 string, opIndex int) (resultingOffer, error) {
	var out resultingOffer

	raw, err := base64.StdEncoding.DecodeString(resultXDRBase64)
	if err != nil {
		return out, fmt.Errorf("horizon: result xdr is not base64: %w", err)
	}
	r := &xdrReader{b: raw}

	r.i64() // feeCharged, read and discarded rather than skipped, so a short body errors here

	// TransactionResult carries a results array only for txSUCCESS and txFAILED.
	// Any other code means the transaction never reached its operations.
	code := r.i32()
	if code != 0 && code != -1 {
		return out, fmt.Errorf("%w: transaction result code %d", ErrNotAnOfferResult, code)
	}

	n := int(r.i32())
	if r.err != nil {
		return out, r.err
	}
	if opIndex < 0 || opIndex >= n {
		return out, fmt.Errorf("horizon: operation index %d is outside the %d results in this transaction", opIndex, n)
	}

	for i := 0; i < opIndex; i++ {
		if err := r.skipOperationResult(); err != nil {
			return out, fmt.Errorf("skipping operation %d of %d: %w", i, n, err)
		}
	}

	// The operation that was asked for.
	if c := r.i32(); c != 0 { // opINNER
		return out, fmt.Errorf("%w: operation result code %d", ErrNotAnOfferResult, c)
	}
	switch t := r.i32(); t {
	case opTypeManageSellOffer, opTypeCreatePassiveSellOffer, opTypeManageBuyOffer:
	default:
		return out, fmt.Errorf("%w: operation type %d", ErrNotAnOfferResult, t)
	}
	if c := r.i32(); c != 0 { // MANAGE_*_OFFER_SUCCESS
		return out, fmt.Errorf("%w: manage offer result code %d", ErrNotAnOfferResult, c)
	}

	out.Claimed = r.claimAtoms()
	out.Effect = offerEffect(r.i32())
	if out.Effect != offerDeleted {
		r.accountID() // seller, skipped by width on purpose
		out.OfferID = r.i64()
		out.Selling = r.asset()
		out.Buying = r.asset()
		out.Amount = r.i64()
		out.PriceN = r.i32()
		out.PriceD = r.i32()
		r.i32() // flags, which carry only the passive bit and change no price
		r.ext()
	}
	if r.err != nil {
		return out, r.err
	}
	return out, nil
}

// TOIDOperationIndex returns the ZERO BASED position of an operation inside its
// transaction, from the operation's TOID.
//
// A TOID packs the ledger sequence in the high 32 bits, the transaction's order
// in the ledger in the next 20, and the operation's order in the transaction in
// the low 12, ONE based. This is decoding an identifier and never a time; rule 4
// of 00-overview.md governs the other direction.
func TOIDOperationIndex(toid int64) int {
	i := int(toid & 0xFFF)
	if i == 0 {
		return 0
	}
	return i - 1
}

// TOIDLedger returns the ledger sequence a TOID names.
func TOIDLedger(toid int64) uint32 { return uint32(toid >> 32) }

// ---------------------------------------------------------------- the reader

// xdrReader walks a buffer and remembers the first thing that went wrong.
//
// It is sticky rather than returning an error per field, because every read here
// is on a path where one short buffer means the whole record is unusable, and
// threading an error through thirty reads would bury the parsing under the
// checking. Nothing acts on a value until err is inspected.
type xdrReader struct {
	b   []byte
	o   int
	err error
}

func (r *xdrReader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if r.o+n > len(r.b) {
		r.err = fmt.Errorf("horizon: result xdr is short: wanted %d bytes at offset %d of %d", n, r.o, len(r.b))
		return false
	}
	return true
}

func (r *xdrReader) i32() int32 {
	if !r.need(4) {
		return 0
	}
	v := int32(binary.BigEndian.Uint32(r.b[r.o:]))
	r.o += 4
	return v
}

func (r *xdrReader) i64() int64 {
	if !r.need(8) {
		return 0
	}
	v := int64(binary.BigEndian.Uint64(r.b[r.o:]))
	r.o += 8
	return v
}

func (r *xdrReader) skip(n int) {
	if !r.need(n) {
		return
	}
	r.o += n
}

// accountID is a PublicKey union: a 4 byte key type and 32 bytes of ed25519.
// THIRTY SIX, and the header records what reading it as 32 does.
func (r *xdrReader) accountID() { r.skip(36) }

// ext is the trailing extension union present on several structs: a
// discriminant of 0 and nothing after it. Anything else is a protocol this
// decoder has not seen.
func (r *xdrReader) ext() {
	if v := r.i32(); v != 0 && r.err == nil {
		r.err = fmt.Errorf("horizon: unexpected extension discriminant %d at offset %d", v, r.o-4)
	}
}

// asset reads an Asset union and keeps enough of it to compare against a pair.
func (r *xdrReader) asset() assetRef {
	switch t := r.i32(); t {
	case 0:
		return assetRef{AssetType: "native"}
	case 1:
		return assetRef{AssetType: "credit_alphanum4", AssetCode: r.assetCode(4), AssetIssuer: r.issuer()}
	case 2:
		return assetRef{AssetType: "credit_alphanum12", AssetCode: r.assetCode(12), AssetIssuer: r.issuer()}
	default:
		if r.err == nil {
			r.err = fmt.Errorf("horizon: unknown asset type %d at offset %d", t, r.o-4)
		}
		return assetRef{}
	}
}

// assetCode reads a fixed width code and trims the RIGHT hand null padding. The
// padding is on the right because Stellar left aligns the code, which is why a
// four byte drift shows up as leading rubbish rather than as a decode failure.
func (r *xdrReader) assetCode(n int) string {
	if !r.need(n) {
		return ""
	}
	raw := r.b[r.o : r.o+n]
	r.o += n
	end := len(raw)
	for end > 0 && raw[end-1] == 0 {
		end--
	}
	return string(raw[:end])
}

// issuer is skipped rather than encoded, and the reason is in the header. The
// empty string it returns is a deliberate hole: an assetRef from this decoder
// can be compared on type and code but NOT with matches(), which checks the
// issuer. sameAsset below is what compares them.
func (r *xdrReader) issuer() string {
	r.accountID()
	return ""
}

// claimAtoms reads the offers an operation took.
func (r *xdrReader) claimAtoms() []claimedOffer {
	n := int(r.i32())
	if r.err != nil || n == 0 {
		return nil
	}
	if n < 0 || n > 1<<16 {
		r.err = fmt.Errorf("horizon: implausible claim count %d", n)
		return nil
	}
	out := make([]claimedOffer, 0, n)
	for i := 0; i < n && r.err == nil; i++ {
		var c claimedOffer
		switch t := r.i32(); t {
		case 0: // CLAIM_ATOM_TYPE_V0: a bare ed25519, not a full AccountID
			r.skip(32)
			c.OfferID = r.i64()
		case 1: // CLAIM_ATOM_TYPE_ORDER_BOOK
			r.accountID()
			c.OfferID = r.i64()
		case 2: // CLAIM_ATOM_TYPE_LIQUIDITY_POOL: a pool id and no offer
			r.skip(32)
			c.IsPool = true
		default:
			r.err = fmt.Errorf("horizon: unknown claim atom type %d at offset %d", t, r.o-4)
			return out
		}
		r.asset()
		c.AmountSold = r.i64()
		r.asset()
		c.AmountBought = r.i64()
		out = append(out, c)
	}
	return out
}

// skipOperationResult advances over one OperationResult without interpreting it.
//
// Most operation results are three int32s: the opINNER code, the operation type,
// and a result code with no body. The ones with a body are listed explicitly, and
// anything not listed returns ErrUnsizableResult rather than a guess, because a
// wrong width here decodes the following bytes into a plausible offer instead of
// failing.
func (r *xdrReader) skipOperationResult() error {
	if c := r.i32(); c != 0 { // not opINNER: the union has no body at all
		return r.err
	}
	opType := r.i32()
	resultCode := r.i32()
	if r.err != nil {
		return r.err
	}
	// Every operation result union is void on failure. Only success carries a
	// body, and only for the types below.
	if resultCode != 0 {
		return nil
	}

	switch opType {
	case opTypeManageSellOffer, opTypeCreatePassiveSellOffer, opTypeManageBuyOffer:
		r.claimAtoms()
		if effect := offerEffect(r.i32()); effect != offerDeleted {
			r.accountID()
			r.skip(8) // offerID
			r.asset() // selling
			r.asset() // buying
			r.skip(8) // amount
			r.skip(8) // price, two int32
			r.skip(4) // flags
			r.ext()
		}
	case opTypePathPaymentStrictReceive, opTypePathPaymentStrictSend:
		r.claimAtoms()
		// SimplePaymentResult: destination AccountID, Asset, int64 amount.
		r.accountID()
		r.asset()
		r.skip(8)
	case opTypeAccountMerge:
		r.skip(8) // sourceAccountBalance
	case opTypeCreateClaimableBalance:
		r.skip(4 + 32) // ClaimableBalanceID union plus its 32 byte hash
	case opTypeInflation:
		n := int(r.i32())
		if r.err != nil {
			return r.err
		}
		if n < 0 || n > 1<<16 {
			return fmt.Errorf("horizon: implausible inflation payout count %d", n)
		}
		r.skip(n * (36 + 8)) // AccountID plus int64 per payout
	default:
		if opResultVoidOnSuccess[opType] {
			return r.err
		}
		// Soroban's invokeHostFunction returns a recursive SCVal, and anything
		// this decoder has never seen could carry anything. Both refuse here
		// rather than being skipped by a guessed width.
		return fmt.Errorf("%w: operation type %d", ErrUnsizableResult, opType)
	}
	return r.err
}

// sameAsset compares an assetRef that came out of this decoder, which has no
// issuer, against one that came off the JSON, which does.
//
// It compares TYPE AND CODE ONLY, and that is a real limitation rather than an
// oversight: two assets with the same code and different issuers are different
// assets, which is the identity rule this repository states everywhere. It is
// safe HERE and only here because the caller has already pinned the pair in the
// request it made, so the operations being read are the ones Horizon returned for
// that account, and the pair filter in replay.go checks the JSON side where the
// issuer is present.
func sameAsset(fromXDR assetRef, want assetRef) bool {
	return fromXDR.AssetType == want.AssetType && fromXDR.AssetCode == want.AssetCode
}
