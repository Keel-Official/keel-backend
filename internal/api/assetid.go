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
// WHEN `quote` IS OMITTED the contract says the asset's primary pair is used,
// defined as the pair with the largest combined depth at 10 percent. THAT RULE IS
// NOT IMPLEMENTED, and not because it is hard. It is decision D-1, and
// docs/methodology/02-pair-selection.md is still a worksheet whose own checklist
// says no decisions are recorded in it yet. Picking the pair by any other rule
// here, largest depth today or first alphabetically, would be this package
// quietly making a methodology decision and having it read back as if it had been
// chosen deliberately.
//
// So: one pair for the asset resolves without a quote, and several make the
// request ambiguous and say so, listing the candidates. Note that the contract's
// error enum has no code for an ambiguous identity, so INVALID_ASSET_ID carries
// it. That gap is handoff item 18.
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
		candidates := make([]string, 0, len(pairs))
		for _, p := range pairs {
			candidates = append(candidates, p.Quote.String())
		}
		return store.Asset{}, &apiError{http.StatusBadRequest, codeInvalidAssetID,
			"This asset is measured against more than one quote asset, and which one is " +
				"primary is not decided yet. Pass ?quote= to choose.",
			map[string]any{"quoteCandidates": candidates}}
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
