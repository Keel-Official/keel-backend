// Resolving the contract's `assetId` into a stored pair.
//
// THE PROBLEM THIS FILE EXISTS FOR. The contract's assetId is `CODE:ISSUER`, or
// `XLM` for the native asset. It carries no asset TYPE. But an asset's identity
// on Stellar includes its type, and querying with the wrong one returns an empty
// result and no error: USTRY has a five character code and is credit_alphanum12,
// so a length rule would call it alphanum4 and measure a different asset, or
// nothing at all. That trap is recorded on domain.Asset, in this package's brief,
// and in two decision records that both contain the mistake.
//
// So the type is never inferred here. It is looked up: the assets table holds the
// type that was declared when the pair entered the demonstration set, and that
// row is the authority. An asset that is not in the set gets ASSET_NOT_MONITORED,
// which is an ordinary condition and not a failure.

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Keel-Official/keel-backend/internal/domain"
	"github.com/Keel-Official/keel-backend/internal/store"
)

// assetIDPattern is the contract's own pattern for the parameter. It is repeated
// here rather than trusted, because a request that does not match it must be
// rejected with INVALID_ASSET_ID before it reaches a query.
var assetIDPattern = regexp.MustCompile(`^(XLM|[A-Za-z0-9]{1,12}:G[A-Z2-7]{55})$`)

// apiError carries a status and a contract error code out of a helper.
type apiError struct {
	status  int
	code    string
	message string
	detail  map[string]any
}

func (s *Server) writeAPIError(w http.ResponseWriter, e *apiError) {
	s.writeError(w, e.status, e.code, e.message, e.detail)
}

// parseAssetID splits CODE:ISSUER, accepting the percent-encoded colon the
// contract permits. It returns the code and issuer only: the TYPE is not
// knowable from this string and is deliberately not guessed.
func parseAssetID(raw string) (code, issuer string, err error) {
	// A strict URL builder on the consumer's side may send %3A. Go has already
	// decoded the path segment by the time a handler sees it, but a doubly
	// encoded value arrives as a literal %3A, so both spellings are accepted.
	decoded := raw
	if strings.Contains(raw, "%3A") || strings.Contains(raw, "%3a") {
		if u, e := url.PathUnescape(raw); e == nil {
			decoded = u
		}
	}
	if !assetIDPattern.MatchString(decoded) {
		return "", "", fmt.Errorf(
			"Invalid assetId format. Use CODE:ISSUER for an issued asset or XLM for the native asset.")
	}
	if decoded == "XLM" {
		return "XLM", "", nil
	}
	code, issuer, _ = strings.Cut(decoded, ":")
	return code, issuer, nil
}

// resolvePair turns the assetId path value and the optional quote query into one
// stored pair.
//
// WHEN `quote` IS OMITTED the primary pair is used, and the primary pair is the
// GLOBAL QUOTE ASSET: USDC, issuer GA5ZSEJY..., domain.GlobalQuote().
//
// IMPLEMENTED 11 SEPTEMBER 2026, AND THE HISTORY IS THE POINT. Until then this
// returned an ambiguity error whenever an asset had more than one pair, and the
// comment here said the rule was undecided: "decision D-1, and
// docs/methodology/02-pair-selection.md is still a worksheet whose own checklist
// says no decisions are recorded in it yet". Refusing to guess was right while
// that was true. It stopped being true on 5 September 2026, when Al resolved Q7
// and DEC-015 made the quote asset global; section 2 of that document now reads
// "The primary pair is USDC, always. It follows from section 1 and requires no
// rule." So the refusal outlived its reason by six days and went on reporting a
// decision as missing after it had been made, which is the stale-lock pattern
// this repository has paid for more than once.
//
// NOTE WHAT THE CONTRACT SAID, because it was wrong in a different way. Its
// `quote` parameter described the primary pair as "the pair with the largest
// combined depth at 10 percent", a rule nobody ever adopted and which DEC-015
// supersedes: a band could then move because depth moved, not because risk did.
// Corrected in contract 1.5.1 rather than implemented.
//
// THE AMBIGUITY ERROR IS KEPT for the one case it still describes: an asset with
// several pairs and no USDC pair among them. That is not a decided case, because
// the candidate set in section 1 is exactly USDC and native XLM, so such an asset
// is measured only against XLM and calling that pair "primary" would assert a
// rule the methodology does not contain. The contract's error enum has no code
// for an ambiguous identity, so INVALID_ASSET_ID still carries it, which remains
// handoff item 18.
func (s *Server) resolvePair(ctx context.Context, r *http.Request) (store.Asset, *apiError) {
	code, issuer, err := parseAssetID(r.PathValue("assetId"))
	if err != nil {
		return store.Asset{}, &apiError{http.StatusBadRequest, codeInvalidAssetID, err.Error(), nil}
	}

	pairs, err := s.cfg.Reader.PairsForAsset(ctx, code, issuer)
	if err != nil {
		s.cfg.Logf("api: pairs for %s: %v", code, err)
		return store.Asset{}, &apiError{http.StatusInternalServerError, "INTERNAL",
			"The request could not be served. The failure has been logged.", nil}
	}
	if len(pairs) == 0 {
		return store.Asset{}, &apiError{http.StatusNotFound, codeAssetNotMonitored,
			"This asset is not part of the demonstration set. See GET " + BasePath +
				"/assets for the list of monitored assets.", nil}
	}

	quoteRaw := r.URL.Query().Get("quote")
	if quoteRaw == "" {
		if len(pairs) == 1 {
			return pairs[0], nil
		}
		// The primary pair, by identity and not by depth. Equal compares all
		// three fields, so a pair quoted in some other issuer's USDC does not
		// match and is not silently treated as the primary.
		primary := domain.GlobalQuote()
		for _, p := range pairs {
			if p.Quote.Equal(primary) {
				return p, nil
			}
		}
		candidates := make([]string, 0, len(pairs))
		for _, p := range pairs {
			candidates = append(candidates, p.Quote.String())
		}
		return store.Asset{}, &apiError{http.StatusBadRequest, codeInvalidAssetID,
			"This asset is measured against more than one quote asset and none of them " +
				"is " + primary.String() + ", which is the primary quote. Pass ?quote= to choose.",
			map[string]any{"quoteCandidates": candidates, "primaryQuote": primary.String()}}
	}

	quoteCode, quoteIssuer, err := parseAssetID(quoteRaw)
	if err != nil {
		return store.Asset{}, &apiError{http.StatusBadRequest, codeInvalidAssetID,
			"Invalid quote format. Use CODE:ISSUER or XLM.", nil}
	}
	for _, p := range pairs {
		if p.Quote.Code == quoteCode && p.Quote.Issuer == quoteIssuer {
			return p, nil
		}
	}
	return store.Asset{}, &apiError{http.StatusNotFound, codeAssetNotMonitored,
		"This asset is monitored, but not against that quote asset.", nil}
}
