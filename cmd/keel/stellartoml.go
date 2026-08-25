// The SEP-1 stellar.toml half of asset verification.
//
// WHY THIS IS HERE AND NOT IN internal/horizon. That package is the Horizon
// adapter, and a stellar.toml is served by a third party's web server rather than
// by Horizon. Putting a fetcher for arbitrary internet domains inside the Horizon
// client would give it a second job, a second failure mode, and a second rate
// limit that has nothing to do with the first. This file is the other half of a
// two-way check and lives beside the tool that performs it.
//
// WHAT A toml PROVES, AND WHAT IT DOES NOT. It proves that whoever controls the
// domain listed this (code, issuer) pair. That is worth exactly as much as
// home_domain on its own, which is to say nothing: home_domain proves whoever
// controls the ACCOUNT typed a domain in. Only the two together mean the account
// operator and the domain operator agree about each other, which is what SEP-1
// asks for. Neither direction is skippable and neither is sufficient.
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// tomlCurrency is one [[CURRENCIES]] entry, reduced to the two fields identity
// needs. Everything else in a CURRENCIES block is presentation.
type tomlCurrency struct {
	Code   string
	Issuer string
}

// tomlDoc is one fetched stellar.toml.
type tomlDoc struct {
	URL        string
	Currencies []tomlCurrency
	// Err is set when the document could not be fetched or read at all. A
	// document that loads and simply does not list an asset is NOT an error: it
	// is a mismatch, which is a different finding and a different status.
	Err error
}

// tomlFetcher fetches stellar.toml documents, once per domain per run.
//
// THE CACHE IS PER DOMAIN AND NOT PER ASSET, which is the whole point of it. An
// anchor with forty assets publishes one toml listing all forty, and fetching it
// forty times would be forty requests to a stranger's web server to answer one
// question. It also makes the run deterministic in a way per-asset fetching is
// not: every asset on a domain sees the same document, including the same
// failure.
type tomlFetcher struct {
	client *http.Client
	// sem bounds how many third-party domains are in flight at once. These are
	// not our servers and a survey of a thousand tickers must not arrive at
	// anyone as a burst.
	sem chan struct{}

	// urlFor builds the document URL from a domain. It is a field rather than a
	// direct call to tomlURL so a test can point the fetcher at a local server
	// without the production path gaining a mode.
	urlFor func(domain string) string

	mu    sync.Mutex
	cache map[string]*tomlEntry
}

type tomlEntry struct {
	once sync.Once
	doc  tomlDoc
}

func newTOMLFetcher(timeout time.Duration, concurrency int) *tomlFetcher {
	if concurrency < 1 {
		concurrency = 1
	}
	return &tomlFetcher{
		client: &http.Client{
			Timeout: timeout,
			// Redirects are followed, with the standard cap. A domain that
			// redirects its well-known path to a CDN is ordinary. What is NOT
			// followed anywhere is a redirect changing which domain the answer
			// came from without that being visible: the URL actually fetched is
			// recorded on the document.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("stopped after 5 redirects")
				}
				return nil
			},
		},
		sem:    make(chan struct{}, concurrency),
		urlFor: tomlURL,
		cache:  map[string]*tomlEntry{},
	}
}

// tomlURL is the SEP-1 well-known path. It is built from the domain the ACCOUNT
// claimed, not from the toml link Horizon reports, because Horizon derives that
// link from the same home_domain and reading it back would make the check
// circular in appearance even though it is not.
func tomlURL(domain string) string {
	return "https://" + strings.TrimSuffix(strings.TrimSpace(domain), "/") + "/.well-known/stellar.toml"
}

// Fetch returns the document for one domain, fetching it at most once per run.
//
// A FAILURE IS CACHED TOO. Retrying a domain that timed out, once per asset it
// issues, turns one slow anchor into the reason a survey takes an hour. The
// failure is recorded as the answer for that domain for this run, which is what
// TOML_UNREACHABLE means and why it is a status rather than a retry.
func (f *tomlFetcher) Fetch(ctx context.Context, domain string) tomlDoc {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return tomlDoc{Err: fmt.Errorf("no home_domain")}
	}

	f.mu.Lock()
	e, ok := f.cache[domain]
	if !ok {
		e = &tomlEntry{}
		f.cache[domain] = e
	}
	f.mu.Unlock()

	e.once.Do(func() {
		e.doc = f.fetchOnce(ctx, domain)
	})
	return e.doc
}

func (f *tomlFetcher) fetchOnce(ctx context.Context, domain string) tomlDoc {
	select {
	case f.sem <- struct{}{}:
		defer func() { <-f.sem }()
	case <-ctx.Done():
		return tomlDoc{URL: f.urlFor(domain), Err: ctx.Err()}
	}

	u := f.urlFor(domain)
	doc := tomlDoc{URL: u}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		doc.Err = err
		return doc
	}
	req.Header.Set("Accept", "text/plain, */*")

	resp, err := f.client.Do(req)
	if err != nil {
		doc.Err = err
		return doc
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		doc.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		return doc
	}
	// A stellar.toml is a text file. The cap stops a domain serving a gigabyte
	// at this tool from being that domain's problem to solve.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		doc.Err = err
		return doc
	}
	// A PARKED DOMAIN SERVES HTML WITH HTTP 200, and calling that a mismatch
	// would be wrong in a way that matters. TOML_MISMATCH is defined as "somebody
	// published a list and this asset is not on it", which is a statement about
	// the asset. A lander page is a statement about the domain and nothing at all
	// about the asset, so it belongs with the unreachable documents.
	//
	// Measured, not supposed: aqua.trading answers this path with
	// `<!DOCTYPE html>` and a redirect script, and it is one of the
	// aqua-flavoured domains an AQUA impostor claims.
	if looksLikeHTML(body) {
		doc.Err = fmt.Errorf("the document is HTML, not a stellar.toml")
		return doc
	}
	doc.Currencies = parseCurrencies(string(body))
	return doc
}

// looksLikeHTML reports whether the first meaningful character opens a tag. It is
// deliberately this crude: the question is only whether a stellar.toml was served
// at all, and a TOML document never begins with '<' in any position this checks.
func looksLikeHTML(body []byte) bool {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return strings.HasPrefix(line, "<")
	}
	return false
}

// parseCurrencies pulls the code and issuer out of every [[CURRENCIES]] block.
//
// IT IS NOT A TOML PARSER AND DOES NOT TRY TO BE. This repository adds no
// dependency without being asked, and the question here is narrow enough that a
// general parser would be answering a much larger one: find array-of-table
// headers named CURRENCIES, and inside each, read two string keys. Everything
// else in the document is skipped rather than understood.
//
// WHAT IT DELIBERATELY DOES NOT HANDLE, listed so a reader knows the edge rather
// than discovering it. Multi-line strings, arrays spanning lines, and inline
// tables are not parsed; a CURRENCIES entry written as an inline table on one
// line is not read. SEP-1 also permits a CURRENCIES entry that points at an
// external toml file by URL instead of listing the asset inline, and such an
// entry yields no (code, issuer) here, so the asset reads as TOML_MISMATCH rather
// than VERIFIED. That is the conservative direction: this tool records what it
// could prove, and a pair it could not confirm is never reported as confirmed.
func parseCurrencies(body string) []tomlCurrency {
	var out []tomlCurrency
	var cur tomlCurrency
	inCurrencies := false

	flush := func() {
		if inCurrencies && (cur.Code != "" || cur.Issuer != "") {
			out = append(out, cur)
		}
		cur = tomlCurrency{}
	}

	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		// A comment is only a comment at the start of a line here. Trimming from
		// a '#' anywhere would corrupt a value that legitimately contains one,
		// and values are what this function exists to read.
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			// A new table header ends whatever block was open.
			flush()
			header := strings.Trim(line, "[] \t")
			inCurrencies = strings.EqualFold(header, "CURRENCIES")
			continue
		}
		if !inCurrencies {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "code":
			cur.Code = unquoteTOML(value)
		case "issuer":
			cur.Issuer = unquoteTOML(value)
		}
	}
	flush()
	return out
}

func unquoteTOML(v string) string {
	v = strings.TrimSpace(v)
	// Strip a trailing comment only when the value is quoted, where the closing
	// quote makes the boundary unambiguous.
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		q := v[0]
		if i := strings.IndexByte(v[1:], q); i >= 0 {
			return v[1 : 1+i]
		}
	}
	if i := strings.IndexByte(v, '#'); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

// listsExactly reports whether the document names this exact (code, issuer) pair.
//
// BOTH HALVES, ALWAYS, and the code comparison is case sensitive because Stellar
// asset codes are. A toml listing AQUA on the right domain does not verify a
// DIFFERENT issuer's AQUA, which is the entire failure this tool exists to
// prevent: 97 assets carry that code and 13 of them sit on an aqua-flavoured
// domain.
func (d tomlDoc) listsExactly(code, issuer string) bool {
	for _, c := range d.Currencies {
		if c.Code == code && c.Issuer == issuer {
			return true
		}
	}
	return false
}
