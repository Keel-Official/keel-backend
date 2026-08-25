// The `universe` subcommand: build a candidate asset universe.
//
// IT PROPOSES. IT DOES NOT SELECT. For every ticker it is given it returns EVERY
// issuer Horizon reports, with the evidence gathered about each one and a
// verification status, and it applies no criterion to decide which belong. There
// is no minimum trustline count in this file, no minimum balance, no "active"
// flag and no top-N. Those are the inclusion criteria, they are
// docs/methodology/02-pair-selection.md section 5, that document is red and
// unwritten, and writing any of them here would be writing that document by
// accident and by the wrong hand.
//
// The line is concrete: if a comparison against a constant ever decides whether
// an asset appears in the output, this tool has crossed into methodology. The
// only numbers compared against constants here are a page cap and a concurrency
// bound, and neither can remove a candidate without saying so.
//
// WHAT THE TICKER LIST IS. An input, not a judgement. The tool has to be told
// which codes to survey because /assets with no filter is the whole network, and
// asking it to pick the interesting ones would be the selection it must not make.
// The list is Al's; every issuer of every code on it comes back.
//
// AN UNVERIFIED ASSET IS NEVER DROPPED. Verification status is a column, not a
// filter. An asset whose issuer publishes no home_domain is an ordinary asset
// with an unproven identity, and that is a finding the criteria will want, not a
// reason for this tool to hide it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/Keel-Official/keel-backend/internal/horizon"
)

// The four verification statuses. They are ordered here from strongest to
// weakest claim, and that order is the one the report prints.
const (
	// verified means the issuer account claims a domain AND that domain's
	// stellar.toml names this exact (code, issuer) pair. Both directions.
	verified = "VERIFIED"
	// tomlMismatch means the toml loaded and does NOT name this pair. That is a
	// stronger negative than an unreachable document: somebody published a list
	// and this asset is not on it.
	tomlMismatch = "TOML_MISMATCH"
	// tomlUnreachable means a domain was claimed and the document could not be
	// read. It says nothing about the asset, only about the web server.
	tomlUnreachable = "TOML_UNREACHABLE"
	// unverified means there is nothing to check: the issuer account publishes
	// no home_domain at all.
	unverified = "UNVERIFIED"
)

// candidate is one row of the universe: one (code, issuer) pair and everything
// gathered about it. Every amount is a decimal STRING exactly as Horizon sent it.
// No float32 or float64 appears on this path or in the output.
type candidate struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
	Type   string `json:"asset_type"`

	ContractID string `json:"contract_id,omitempty"`

	AuthorizedTrustlines int    `json:"authorized_trustlines"`
	AuthorizedBalance    string `json:"authorized_balance"`

	NumLiquidityPools    int    `json:"num_liquidity_pools"`
	LiquidityPoolsAmount string `json:"liquidity_pools_amount"`

	NumClaimableBalances    int    `json:"num_claimable_balances"`
	ClaimableBalancesAmount string `json:"claimable_balances_amount"`

	NumContracts    int    `json:"num_contracts"`
	ContractsAmount string `json:"contracts_amount"`

	// HomeDomain is what the ISSUER ACCOUNT claims about itself. TomlURL is the
	// document that claim was checked against. Both are recorded even when the
	// check failed, because the pair of them is the audit trail.
	HomeDomain         string `json:"home_domain"`
	TomlURL            string `json:"toml_url"`
	TomlURLFromHorizon string `json:"toml_url_reported_by_horizon,omitempty"`

	Verification string `json:"verification"`
	// VerificationDetail says WHY, in one line, for every status that is not
	// VERIFIED. A status with no reason is a status nobody can act on.
	VerificationDetail string `json:"verification_detail,omitempty"`

	AuthRequired        bool `json:"auth_required"`
	AuthRevocable       bool `json:"auth_revocable"`
	AuthImmutable       bool `json:"auth_immutable"`
	AuthClawbackEnabled bool `json:"auth_clawback_enabled"`

	// LedgerSeq and ReadAt are non-negotiable rule 1 applied to a reading rather
	// than to a computation: every row says which ledger it was taken at.
	LedgerSeq uint32 `json:"ledger_seq"`
	ReadAt    string `json:"read_at"`
}

// tickerReading records what one ticker cost to walk, so a reader can tell a
// complete answer from a truncated one without rerunning anything.
type tickerReading struct {
	Code      string `json:"code"`
	Issuers   int    `json:"issuers"`
	Pages     int    `json:"pages_walked"`
	Truncated bool   `json:"truncated"`
	LedgerSeq uint32 `json:"ledger_seq"`
	Error     string `json:"error,omitempty"`
}

// universeFile is the machine-readable artifact. Field order here is the field
// order on disk, and every slice inside it is explicitly sorted, so two runs over
// the same ledger produce a byte-identical file. Nothing is written from a map.
type universeFile struct {
	Kind        string `json:"kind"`
	Note        string `json:"note"`
	Generator   string `json:"generator"`
	GeneratedAt string `json:"generated_at"`
	Horizon     string `json:"horizon"`

	// Provenance, per field, so no number in this file has an unexplained origin.
	FieldSources map[string]string `json:"field_sources"`

	// VolumeWindow is empty and says why. Volume is not on /assets and this tool
	// does not take it from anywhere else; a figure whose window is undocumented
	// is worse than an absent one.
	VolumeWindow string `json:"volume_window"`

	Throttled429 int             `json:"horizon_429_responses"`
	Tickers      []tickerReading `json:"tickers"`
	Candidates   []candidate     `json:"candidates"`
}

func runUniverse(args []string) error {
	fs := flag.NewFlagSet("universe", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	codes := fs.String("codes", "", "comma separated asset codes to survey, e.g. AQUA,USDC,yXLM")
	codesFile := fs.String("codes-file", "", "a file of asset codes, one per line, # comments allowed")
	out := fs.String("out", "configs", "directory for the two artifacts (GREEN path)")
	name := fs.String("name", "candidate-universe", "base name for the two artifacts")
	horizonURL := fs.String("horizon", horizon.DefaultBaseURL, "Horizon base URL")
	budget := fs.Int("budget", 3000, "Horizon requests permitted per hour")
	tomlTimeout := fs.Duration("toml-timeout", 10*time.Second, "per-domain stellar.toml timeout")
	tomlConc := fs.Int("toml-concurrency", 8, "how many third-party domains to fetch at once")
	workers := fs.Int("workers", 8, "how many candidates to verify at once")
	timeout := fs.Duration("timeout", 30*time.Minute, "overall deadline for the run")

	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `keel universe - build a candidate asset universe

It PROPOSES and does not SELECT. Every issuer of every code you name comes back,
verified or not. No threshold is applied anywhere; the inclusion criteria are
docs/methodology/02-pair-selection.md section 5 and they are not this tool's.

  keel universe -codes AQUA,USDC
  keel universe -codes-file configs/candidate-codes.txt -out configs

Writes two files: <name>.json (deterministic) and <name>.txt (the report).
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	wanted, err := collectCodes(*codes, *codesFile)
	if err != nil {
		return err
	}
	if len(wanted) == 0 {
		fs.Usage()
		return fmt.Errorf("no asset codes given; use -codes or -codes-file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := horizon.NewClient(horizon.Config{
		BaseURL:      *horizonURL,
		Budget:       *budget,
		BudgetWindow: time.Hour,
	})
	fetcher := newTOMLFetcher(*tomlTimeout, *tomlConc)

	file := universeFile{
		Kind: "keel.candidate-universe",
		Note: "PROPOSES ONLY. Every issuer of every code surveyed, verified or not, with no " +
			"inclusion criterion applied. Selection is docs/methodology/02-pair-selection.md " +
			"section 5 and is not this file. An UNVERIFIED row is a candidate with an unproven " +
			"identity, never a rejected one.",
		Generator:    "keel universe",
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Horizon:      *horizonURL,
		VolumeWindow: "none. Volume is not served by /assets and this tool takes it from nowhere else, so no volume figure appears above.",
		FieldSources: map[string]string{
			"asset_type, contract_id, authorized_trustlines, authorized_balance": "GET /assets?asset_code={code}, paged",
			"num_liquidity_pools, liquidity_pools_amount":                        "GET /assets?asset_code={code}, paged",
			"num_claimable_balances, claimable_balances_amount":                  "GET /assets?asset_code={code}, paged",
			"num_contracts, contracts_amount":                                    "GET /assets?asset_code={code}, paged",
			"toml_url_reported_by_horizon":                                       "GET /assets, _links.toml.href",
			"home_domain":                                                        "GET /accounts/{issuer}, home_domain",
			"verification":                                                       "home_domain, then https://{home_domain}/.well-known/stellar.toml [[CURRENCIES]]",
			"ledger_seq":                                                         "Latest-Ledger header on the first /assets page of the ticker",
		},
	}

	// pendingCandidate keeps the ledger and clock stamps of the ticker walk that
	// produced the asset, so the verification pass can run after every walk
	// without a row losing the reading it belongs to.
	var pending []pendingCandidate

	for _, code := range wanted {
		reading, err := client.AssetsByCode(ctx, code)
		tr := tickerReading{
			Code:      code,
			Pages:     reading.Pages,
			Truncated: reading.Truncated,
			LedgerSeq: reading.LedgerSeq,
			Issuers:   len(reading.Assets),
		}
		if err != nil {
			// A ticker that failed is RECORDED and does not stop the run. A
			// survey that abandons everything on the first 429 reports a short
			// list, and a short list caused by throttling looks exactly like a
			// short list caused by the market.
			tr.Error = err.Error()
			file.Tickers = append(file.Tickers, tr)
			fmt.Fprintf(os.Stderr, "universe: %s: %v\n", code, err)
			continue
		}
		file.Tickers = append(file.Tickers, tr)
		for _, a := range reading.Assets {
			pending = append(pending, pendingCandidate{a, reading.LedgerSeq, reading.ReadAt})
		}
	}

	// THE VERIFICATION PASS IS CONCURRENT AND BOUNDED, and both halves matter.
	//
	// Bounded, because these requests go to Horizon and to a few hundred
	// strangers' web servers, and a survey must not arrive anywhere as a burst.
	// Concurrent, because it was serial until it was measured: one ticker with 97
	// issuers took about four minutes, nearly all of it waiting on dead domains
	// one at a time, and the toml fetcher's own semaphore never had more than a
	// single fetch in flight. A bound that is never reached is not a bound, it is
	// a comment.
	//
	// DETERMINISM SURVIVES IT. Each worker writes to its own index in a
	// pre-sized slice, so completion order cannot reach the output, and the whole
	// slice is sorted on identity afterwards regardless.
	file.Candidates = make([]candidate, len(pending))
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	for i, p := range pending {
		wg.Add(1)
		go func(i int, p pendingCandidate) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			file.Candidates[i] = buildCandidate(ctx, client, fetcher, p.asset, p.ledger, p.readAt)
		}(i, p)
	}
	wg.Wait()

	file.Throttled429 = client.Throttled()

	// DETERMINISM. Both slices are sorted on the full identity before writing, so
	// the file does not depend on the order the codes were given or on the order
	// Horizon happened to page. Nothing here is written from a map except
	// field_sources, and encoding/json sorts map keys itself.
	sortUniverse(&file)

	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	jsonPath := filepath.Join(*out, *name+".json")
	textPath := filepath.Join(*out, *name+".txt")

	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, append(body, '\n'), 0o644); err != nil {
		return err
	}

	report := renderUniverseReport(file)
	if err := os.WriteFile(textPath, []byte(report), 0o644); err != nil {
		return err
	}

	fmt.Print(report)
	fmt.Fprintf(os.Stderr, "\nwrote %s\nwrote %s\n", jsonPath, textPath)
	return nil
}

// buildCandidate performs the two-way check for one asset.
func buildCandidate(ctx context.Context, client *horizon.Client, fetcher *tomlFetcher,
	a horizon.AssetStat, ledger uint32, readAt time.Time) candidate {

	c := candidate{
		Code:                    a.Code,
		Issuer:                  a.Issuer,
		Type:                    string(a.Type),
		ContractID:              a.ContractID,
		AuthorizedTrustlines:    a.AuthorizedAccounts,
		AuthorizedBalance:       a.AuthorizedBalance,
		NumLiquidityPools:       a.NumLiquidityPools,
		LiquidityPoolsAmount:    a.LiquidityPoolsAmount,
		NumClaimableBalances:    a.NumClaimableBalances,
		ClaimableBalancesAmount: a.ClaimableBalancesAmt,
		NumContracts:            a.NumContracts,
		ContractsAmount:         a.ContractsAmount,
		TomlURLFromHorizon:      a.TomlURLReportedByHzn,
		AuthRequired:            a.AuthRequired,
		AuthRevocable:           a.AuthRevocable,
		AuthImmutable:           a.AuthImmutable,
		AuthClawbackEnabled:     a.AuthClawbackEnabled,
		LedgerSeq:               ledger,
		ReadAt:                  readAt.UTC().Format(time.RFC3339),
	}

	// DIRECTION ONE: the account claims a domain.
	domainName, err := client.HomeDomain(ctx, a.Issuer)
	if err != nil {
		c.Verification = tomlUnreachable
		c.VerificationDetail = "the issuer account could not be read: " + err.Error()
		return c
	}
	c.HomeDomain = domainName
	if domainName == "" {
		c.Verification = unverified
		c.VerificationDetail = "the issuer account publishes no home_domain, so there is nothing to check against"
		return c
	}

	// DIRECTION TWO: the domain claims the account, naming this exact pair.
	doc := fetcher.Fetch(ctx, domainName)
	c.TomlURL = doc.URL
	c.Verification = classify(a.Code, a.Issuer, domainName, doc)
	c.VerificationDetail = explain(a.Code, a.Issuer, domainName, doc, c.Verification)
	return c
}

// classify is the whole verification rule, in one pure function so it can be
// read and tested without a network.
//
// AN ASSET IS VERIFIED ONLY WHEN BOTH DIRECTIONS AGREE. The account named a
// domain, and that domain's toml names this exact (code, issuer) pair. Every
// other outcome is a status that gets recorded, and none of them is a reason to
// drop the row.
func classify(code, issuer, homeDomain string, doc tomlDoc) string {
	if strings.TrimSpace(homeDomain) == "" {
		return unverified
	}
	if doc.Err != nil {
		return tomlUnreachable
	}
	if doc.listsExactly(code, issuer) {
		return verified
	}
	return tomlMismatch
}

func explain(code, issuer, homeDomain string, doc tomlDoc, status string) string {
	switch status {
	case verified:
		return ""
	case unverified:
		return "the issuer account publishes no home_domain, so there is nothing to check against"
	case tomlUnreachable:
		return "home_domain is " + homeDomain + " and its stellar.toml could not be read: " + doc.Err.Error()
	default:
		return fmt.Sprintf("%s serves a stellar.toml with %d CURRENCIES entries and none names %s:%s",
			homeDomain, len(doc.Currencies), code, issuer)
	}
}

// sortUniverse puts both slices into identity order. It is the only thing
// standing between this file and the order Horizon happened to page in, which is
// why it is a named function with a test rather than two inline calls.
func sortUniverse(f *universeFile) {
	sort.Slice(f.Tickers, func(i, j int) bool { return f.Tickers[i].Code < f.Tickers[j].Code })
	sort.Slice(f.Candidates, func(i, j int) bool {
		a, b := f.Candidates[i], f.Candidates[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Issuer != b.Issuer {
			return a.Issuer < b.Issuer
		}
		return a.Type < b.Type
	})
}

// renderUniverseReport is the human artifact.
func renderUniverseReport(f universeFile) string {
	var b strings.Builder

	byStatus := map[string]int{}
	byTicker := map[string][]candidate{}
	for _, c := range f.Candidates {
		byStatus[c.Verification]++
		byTicker[c.Code] = append(byTicker[c.Code], c)
	}

	fmt.Fprintf(&b, "Keel candidate universe\n")
	fmt.Fprintf(&b, "generated %s against %s\n\n", f.GeneratedAt, f.Horizon)
	fmt.Fprintf(&b, "PROPOSES ONLY. No inclusion criterion is applied here. Selection is\n")
	fmt.Fprintf(&b, "docs/methodology/02-pair-selection.md section 5 and is not this tool's.\n\n")

	fmt.Fprintf(&b, "candidates found : %d\n", len(f.Candidates))
	fmt.Fprintf(&b, "tickers surveyed : %d\n", len(f.Tickers))
	// Sorted, never map order.
	for _, s := range []string{verified, tomlMismatch, tomlUnreachable, unverified} {
		fmt.Fprintf(&b, "%-17s: %d\n", strings.ToLower(s), byStatus[s])
	}
	fmt.Fprintf(&b, "\nvolume           : %s\n", f.VolumeWindow)

	// THE THROTTLING LINE IS ALWAYS PRINTED, including when it is zero. A short
	// candidate list next to a silent report is a list nobody can trust; a short
	// list next to "429 responses: 0" is a list about the market.
	fmt.Fprintf(&b, "horizon 429s     : %d", f.Throttled429)
	if f.Throttled429 > 0 {
		fmt.Fprintf(&b, "   <-- this run was rate limited. Counts below may be short for that reason.")
	}
	b.WriteString("\n")

	var truncated, failed []tickerReading
	for _, t := range f.Tickers {
		if t.Truncated {
			truncated = append(truncated, t)
		}
		if t.Error != "" {
			failed = append(failed, t)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(&b, "\nTICKERS THAT FAILED, so the universe is incomplete by that much:\n")
		for _, t := range failed {
			fmt.Fprintf(&b, "  %-12s %s\n", t.Code, t.Error)
		}
	}
	if len(truncated) > 0 {
		fmt.Fprintf(&b, "\nTICKERS WHOSE WALK HIT THE PAGE CAP, so more issuers exist than are listed:\n")
		for _, t := range truncated {
			fmt.Fprintf(&b, "  %-12s %d pages\n", t.Code, t.Pages)
		}
	}

	// THE TICKER COLLISION TABLE. This is the reason the tool exists: a ticker
	// with more than one issuer is a ticker where matching on the name picks an
	// asset at random, and the count is how badly.
	type collision struct {
		code     string
		issuers  int
		verified int
		pages    int
	}
	var collisions []collision
	pagesFor := map[string]int{}
	for _, t := range f.Tickers {
		pagesFor[t.Code] = t.Pages
	}
	for code, rows := range byTicker {
		if len(rows) < 2 {
			continue
		}
		n := 0
		for _, r := range rows {
			if r.Verification == verified {
				n++
			}
		}
		collisions = append(collisions, collision{code, len(rows), n, pagesFor[code]})
	}
	sort.Slice(collisions, func(i, j int) bool {
		if collisions[i].issuers != collisions[j].issuers {
			return collisions[i].issuers > collisions[j].issuers
		}
		return collisions[i].code < collisions[j].code
	})

	fmt.Fprintf(&b, "\nTICKERS WITH MORE THAN ONE ISSUER (%d)\n", len(collisions))
	fmt.Fprintf(&b, "Every row here is a ticker that cannot be used as an identifier.\n\n")
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  CODE\tISSUERS\tVERIFIED\tPAGES")
	for _, c := range collisions {
		fmt.Fprintf(tw, "  %s\t%d\t%d\t%d\n", c.code, c.issuers, c.verified, c.pages)
	}
	_ = tw.Flush()

	fmt.Fprintf(&b, "\nVERIFIED CANDIDATES\n")
	fmt.Fprintf(&b, "Both directions agreed: the account named the domain and the domain's\n")
	fmt.Fprintf(&b, "stellar.toml named this exact (code, issuer) pair.\n\n")
	tw = tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  CODE\tISSUER\tTYPE\tTRUSTLINES\tPOOLS\tDOMAIN")
	for _, c := range f.Candidates {
		if c.Verification != verified {
			continue
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\t%d\t%s\n",
			c.Code, c.Issuer, shortType(c.Type), c.AuthorizedTrustlines, c.NumLiquidityPools, c.HomeDomain)
	}
	_ = tw.Flush()
	return b.String()
}

func shortType(t string) string {
	switch t {
	case "credit_alphanum4":
		return "alnum4"
	case "credit_alphanum12":
		return "alnum12"
	}
	return t
}

// collectCodes reads the ticker list. Codes are de-duplicated and sorted, so the
// order they were typed in cannot reach the output file.
//
// A code's CASE IS PRESERVED. Stellar asset codes are case sensitive, yXLM and
// YXLM are different assets, and upper-casing the input here would silently
// survey the wrong one.
func collectCodes(inline, path string) ([]string, error) {
	seen := map[string]bool{}
	var out []string

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.HasPrefix(s, "#") || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	for _, s := range strings.Split(inline, ",") {
		add(s)
	}
	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("codes file: %w", err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			if i := strings.IndexByte(line, '#'); i >= 0 {
				line = line[:i]
			}
			add(line)
		}
	}
	sort.Strings(out)
	return out, nil
}

// pendingCandidate is one asset waiting for its two-way verification, carrying
// the ledger and timestamp of the walk that found it.
type pendingCandidate struct {
	asset  horizon.AssetStat
	ledger uint32
	readAt time.Time
}
