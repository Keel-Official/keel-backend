// Command holderstats reproduces the section 2 holder-concentration measures from a
// recorded /accounts?asset= pull and cross-checks the recorded trade history against it.
//
// It answers three questions in one pass:
//
//  1. What are top-1, top-N and HHI over the filtered holder population, and what is
//     circulating supply in USTRY?
//  2. Which holders never appear on either side of any recorded trade, and what share of
//     supply do they hold?
//  3. Of every account that did trade, which still holds a non-zero balance, which holds
//     an open trustline at zero, and which has no trustline at all at pull time?
//
// The script performs no network I/O. Every input is a file already recorded under
// docs/evidences/. Reruns over the same inputs produce byte-identical output.
//
// Usage:
//
//	go run ./scripts/holderstats \
//	  -holders docs/evidences/.../holders.csv \
//	  -trades docs/evidences/...2026-02-01_2026-03-01.csv,docs/evidences/...2026-08-01_2026-09-01.csv \
//	  -trade-labels 2026-02,2026-08 \
//	  -out docs/evidences/derived \
//	  -methodology-version 1.0.8-draft \
//	  -genuine-volume-ustry 5723.2370064
//
// Exit codes:
//
//	0  success
//	1  input, schema or invariant failure (details on stderr)
package main

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// USTRY asset under study. The exclusion list below is asset-scoped: run this script
// against another asset without editing it and the numbers are wrong with no warning.
// See docs/methodology/07-supporting-metrics.md section 2, "Asset scope".
const (
	assetCode   = "USTRY"
	assetIssuer = "GCRYUGD5NVARGXT56XEZI5CIFCQETYHAPQQTHO2O3IQZTHDH4LATMYWC"

	ammPoolID       = "27480d0483c8320ba4a707797526ffd67118e841491e0cbeb66db697bb66cccb"
	blendV2Contract = "CCCCIQSDILITHMM7PBSLVDT5MISSY7R26MNZXCX4H7J5JQ5FPIYOGYFS"

	sharePrecision = 40 // internal division precision; outputs are rounded below
	shareScale     = 4  // decimal places used for reported percentages and HHI
	balanceScale   = 7  // Stellar classic asset precision

	maxTradeFiles = 64 // one bit per file in the membership mask
)

// excludedPositions are removed from the holder population and from the circulating
// denominator. Section 2 and section 3 must exclude the same set.
func excludedPositions() map[string]string {
	return map[string]string{
		assetIssuer:     "issuer (holds unissued supply, not a holder)",
		ammPoolID:       "AMM pool reserve (held by the pool, not a holder)",
		blendV2Contract: "Blend V2 (YieldBlox) position (held by the contract, not a holder)",
	}
}

type holder struct {
	AccountID     string
	Balance       decimal.Decimal
	LastModified  int64
	LatestLedger  int64
	SourcePage    string
	ReadAt        string
	TradesAsBase  int
	TradesAsCount int
	SharePct      decimal.Decimal
	CumulativePct decimal.Decimal
}

func (h holder) TradesTotal() int { return h.TradesAsBase + h.TradesAsCount }
func (h holder) EverTraded() bool { return h.TradesTotal() > 0 }

type config struct {
	holdersPath        string
	tradePaths         []string
	tradeLabels        []string
	outDir             string
	methodologyVersion string
	baseAccountColumn  string
	counterAccountCol  string
	topN               int
	maxLedgerSpread    int64
	genuineVolumeUSTRY string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "holderstats: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Fixed division precision: reproducibility (NFR-9) requires that this never depend
	// on package defaults that may change between library versions.
	decimal.DivisionPrecision = sharePrecision

	cfg := config{}
	var trades, labels string
	flag.StringVar(&cfg.holdersPath, "holders", "", "path to the recorded /accounts?asset= CSV (required)")
	flag.StringVar(&trades, "trades", "", "comma-separated paths to recorded trade CSVs (required)")
	flag.StringVar(&labels, "trade-labels", "", "comma-separated short labels for the trade files, in the same order; defaults to file base names")
	flag.StringVar(&cfg.outDir, "out", "", "directory to write the report and CSVs (optional; stdout only when empty)")
	flag.StringVar(&cfg.methodologyVersion, "methodology-version", "", "methodology version stamped on the output (required)")
	flag.StringVar(&cfg.baseAccountColumn, "base-account-column", "base_account", "trade CSV header holding the base-side account ID")
	flag.StringVar(&cfg.counterAccountCol, "counter-account-column", "counter_account", "trade CSV header holding the counter-side account ID")
	flag.IntVar(&cfg.topN, "top", 10, "size of the top-N concentration measure")
	flag.Int64Var(&cfg.maxLedgerSpread, "max-ledger-spread", 0, "reject the pull when latest_ledger spans more than this many ledgers; 0 reports the spread without rejecting (pending DEC-011)")
	flag.StringVar(&cfg.genuineVolumeUSTRY, "genuine-volume-ustry", "", "optional genuine volume in USTRY; when set, volume-to-supply is reported against several denominators")
	flag.Parse()

	if cfg.holdersPath == "" || trades == "" || cfg.methodologyVersion == "" {
		flag.Usage()
		return errors.New("-holders, -trades and -methodology-version are all required")
	}
	for _, p := range strings.Split(trades, ",") {
		if p = strings.TrimSpace(p); p != "" {
			cfg.tradePaths = append(cfg.tradePaths, p)
		}
	}
	if len(cfg.tradePaths) == 0 {
		return errors.New("-trades resolved to no paths")
	}
	if len(cfg.tradePaths) > maxTradeFiles {
		return fmt.Errorf("at most %d trade files are supported, got %d", maxTradeFiles, len(cfg.tradePaths))
	}
	if labels == "" {
		for _, p := range cfg.tradePaths {
			cfg.tradeLabels = append(cfg.tradeLabels, strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		}
	} else {
		for _, l := range strings.Split(labels, ",") {
			cfg.tradeLabels = append(cfg.tradeLabels, strings.TrimSpace(l))
		}
		if len(cfg.tradeLabels) != len(cfg.tradePaths) {
			return fmt.Errorf("-trade-labels has %d entries but -trades has %d paths", len(cfg.tradeLabels), len(cfg.tradePaths))
		}
	}
	if cfg.topN < 1 {
		return errors.New("-top must be at least 1")
	}

	all, meta, err := readHolders(cfg.holdersPath)
	if err != nil {
		return err
	}
	if err := meta.check(cfg.maxLedgerSpread); err != nil {
		return err
	}

	population, dropped, err := filterPopulation(all)
	if err != nil {
		return err
	}
	if len(population) == 0 {
		// Section 3's "denominator zero" condition. Reported, not silently divided by.
		return errors.New("holder population is empty after filtering: circulating supply is zero, so every ratio in section 3 is Unevaluated, not zero")
	}

	seen, masks, tm, err := readTrades(cfg)
	if err != nil {
		return err
	}
	for i := range population {
		c := seen[population[i].AccountID]
		population[i].TradesAsBase = c.asBase
		population[i].TradesAsCount = c.asCounter
	}

	// Classification runs over every trustline row, not the filtered population, so
	// accounts that traded and now hold a zero balance stay visible.
	class, err := classifyTraders(all, seen, masks, cfg.tradeLabels)
	if err != nil {
		return err
	}

	stats := computeStats(population, cfg.topN)

	report := renderReport(cfg, meta, tm, stats, dropped, class)
	fmt.Print(report)

	if cfg.outDir != "" {
		if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
			return fmt.Errorf("creating -out directory: %w", err)
		}
		mdPath := filepath.Join(cfg.outDir, "holder-stats.md")
		if err := os.WriteFile(mdPath, []byte(report), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", mdPath, err)
		}
		csvPath := filepath.Join(cfg.outDir, "holder-trade-crosscheck.csv")
		if err := writeCrosscheck(csvPath, cfg, meta, stats); err != nil {
			return err
		}
		traderPath := filepath.Join(cfg.outDir, "trader-trustline-status.csv")
		if err := writeTraderStatus(traderPath, cfg, meta, class); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "wrote %s\nwrote %s\nwrote %s\n", mdPath, csvPath, traderPath)
	}
	return nil
}

// ---------- holders ----------

type pullMeta struct {
	Rows         int
	Pages        []string
	MinLedger    int64
	MaxLedger    int64
	FirstReadAt  string
	LastReadAt   string
	FirstAccount string
	FirstBalance decimal.Decimal
	ZeroBalance  int
	NonZero      int
}

func (m pullMeta) ledgerSpread() int64 { return m.MaxLedger - m.MinLedger }

// check enforces that the pull can be described by a single ledger. The pull is a
// current-state read that cannot be replayed, so a spread here is permanent: it cannot
// be fixed by refetching a past ledger.
func (m pullMeta) check(maxSpread int64) error {
	if m.Rows == 0 {
		return errors.New("holders file contained no data rows")
	}
	if maxSpread > 0 && m.ledgerSpread() > maxSpread {
		return fmt.Errorf(
			"pull spans %d ledgers (%d..%d), above -max-ledger-spread=%d: no single LedgerSeq describes this snapshot",
			m.ledgerSpread(), m.MinLedger, m.MaxLedger, maxSpread)
	}
	return nil
}

func readHolders(path string) ([]holder, pullMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, pullMeta{}, fmt.Errorf("opening holders file: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, pullMeta{}, fmt.Errorf("reading holders header: %w", err)
	}
	idx, err := columnIndex(header, path,
		"account_id", "balance", "last_modified_ledger", "latest_ledger", "source_page", "read_at_utc")
	if err != nil {
		return nil, pullMeta{}, err
	}

	var (
		out   []holder
		meta  pullMeta
		pages = map[string]bool{}
		line  = 1
	)
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			return nil, pullMeta{}, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		bal, err := decimal.NewFromString(strings.TrimSpace(rec[idx["balance"]]))
		if err != nil {
			return nil, pullMeta{}, fmt.Errorf("%s line %d: parsing balance: %w", path, line, err)
		}
		lm, err := strconv.ParseInt(strings.TrimSpace(rec[idx["last_modified_ledger"]]), 10, 64)
		if err != nil {
			return nil, pullMeta{}, fmt.Errorf("%s line %d: parsing last_modified_ledger: %w", path, line, err)
		}
		ll, err := strconv.ParseInt(strings.TrimSpace(rec[idx["latest_ledger"]]), 10, 64)
		if err != nil {
			return nil, pullMeta{}, fmt.Errorf("%s line %d: parsing latest_ledger: %w", path, line, err)
		}
		h := holder{
			AccountID:    strings.TrimSpace(rec[idx["account_id"]]),
			Balance:      bal,
			LastModified: lm,
			LatestLedger: ll,
			SourcePage:   strings.TrimSpace(rec[idx["source_page"]]),
			ReadAt:       strings.TrimSpace(rec[idx["read_at_utc"]]),
		}
		if meta.Rows == 0 {
			meta.MinLedger, meta.MaxLedger = ll, ll
			meta.FirstReadAt, meta.LastReadAt = h.ReadAt, h.ReadAt
			meta.FirstAccount, meta.FirstBalance = h.AccountID, h.Balance
		}
		if ll < meta.MinLedger {
			meta.MinLedger = ll
		}
		if ll > meta.MaxLedger {
			meta.MaxLedger = ll
		}
		if h.ReadAt < meta.FirstReadAt {
			meta.FirstReadAt = h.ReadAt
		}
		if h.ReadAt > meta.LastReadAt {
			meta.LastReadAt = h.ReadAt
		}
		if h.Balance.IsZero() {
			meta.ZeroBalance++
		} else {
			meta.NonZero++
		}
		pages[h.SourcePage] = true
		meta.Rows++
		out = append(out, h)
	}
	for p := range pages {
		meta.Pages = append(meta.Pages, p)
	}
	sort.Strings(meta.Pages) // NFR-9: map keys are sorted before use
	return out, meta, nil
}

type dropRecord struct {
	AccountID string
	Balance   decimal.Decimal
	Reason    string
}

// filterPopulation removes zero balances and the explicit excluded positions. It also
// rejects duplicate account IDs, which would double-count a holder.
func filterPopulation(all []holder) ([]holder, []dropRecord, error) {
	excluded := excludedPositions()
	seen := make(map[string]bool, len(all))
	var (
		keep    []holder
		dropped []dropRecord
	)
	for _, h := range all {
		if seen[h.AccountID] {
			return nil, nil, fmt.Errorf("duplicate account %s in holders file: the pull is not a clean set", h.AccountID)
		}
		seen[h.AccountID] = true

		if reason, ok := excluded[h.AccountID]; ok {
			// Observed on the 2026-08-31 pull: none of the three appear. If one does,
			// section 2's Definition needs revising, not this filter silently absorbing it.
			dropped = append(dropped, dropRecord{h.AccountID, h.Balance, reason})
			continue
		}
		if h.Balance.IsNegative() {
			return nil, nil, fmt.Errorf("account %s has a negative balance %s", h.AccountID, h.Balance)
		}
		if h.Balance.IsZero() {
			dropped = append(dropped, dropRecord{h.AccountID, h.Balance, "zero-balance trustline (not a holder)"})
			continue
		}
		keep = append(keep, h)
	}
	return keep, dropped, nil
}

// ---------- trades ----------

type sideCount struct{ asBase, asCounter int }

type tradeMeta struct {
	Files         []string
	Labels        []string
	RowsPerFile   []int
	Rows          int
	DistinctAccts int
}

// readTrades returns per-account side counts and a per-account bitmask of which trade
// files the account appears in. The mask is what lets the report separate an account
// that traded only in February from one that traded in both windows.
func readTrades(cfg config) (map[string]sideCount, map[string]uint64, tradeMeta, error) {
	seen := make(map[string]sideCount)
	masks := make(map[string]uint64)
	meta := tradeMeta{Files: cfg.tradePaths, Labels: cfg.tradeLabels}

	for fi, path := range cfg.tradePaths {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, meta, fmt.Errorf("opening trade file: %w", err)
		}
		r := csv.NewReader(f)
		r.FieldsPerRecord = -1
		header, err := r.Read()
		if err != nil {
			f.Close()
			return nil, nil, meta, fmt.Errorf("reading header of %s: %w", path, err)
		}
		idx, err := columnIndex(header, path, cfg.baseAccountColumn, cfg.counterAccountCol)
		if err != nil {
			f.Close()
			return nil, nil, meta, err
		}
		bit := uint64(1) << uint(fi)
		line, rows := 1, 0
		for {
			rec, err := r.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			line++
			if err != nil {
				f.Close()
				return nil, nil, meta, fmt.Errorf("%s line %d: %w", path, line, err)
			}
			rows++
			// An empty account cell is a pool side, not an account. Skipping it is correct;
			// a pool is not a holder.
			if v := strings.TrimSpace(rec[idx[cfg.baseAccountColumn]]); v != "" {
				c := seen[v]
				c.asBase++
				seen[v] = c
				masks[v] |= bit
			}
			if v := strings.TrimSpace(rec[idx[cfg.counterAccountCol]]); v != "" {
				c := seen[v]
				c.asCounter++
				seen[v] = c
				masks[v] |= bit
			}
		}
		f.Close()
		meta.RowsPerFile = append(meta.RowsPerFile, rows)
		meta.Rows += rows
	}
	meta.DistinctAccts = len(seen)
	return seen, masks, meta, nil
}

// ---------- trader trustline classification ----------

// Trustline status of an account that appears in the recorded trade history.
const (
	statusHoldsNonZero  = "holds a non-zero balance"
	statusTrustlineZero = "trustline open, zero balance"
	statusNoTrustline   = "no trustline at pull"
)

// classOrder fixes the column order of the report. Iterating a map would break NFR-9.
var classOrder = []string{statusHoldsNonZero, statusTrustlineZero, statusNoTrustline}

type traderRow struct {
	AccountID string
	Status    string
	Balance   decimal.Decimal
	Mask      uint64
	Windows   string
	Trades    int
	AsBase    int
	AsCounter int
}

type classification struct {
	Labels     []string
	Rows       []traderRow // sorted by account ID
	Counts     map[string]map[string]int
	MaskOrder  []string
	Totals     map[string]int
	HeldQty    map[string]decimal.Decimal // per window group, non-zero status only
	TotalAccts int
}

func maskLabel(mask uint64, labels []string) string {
	var parts []string
	for i, l := range labels {
		if mask&(uint64(1)<<uint(i)) != 0 {
			parts = append(parts, l)
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, " + ")
}

func classifyTraders(all []holder, seen map[string]sideCount, masks map[string]uint64, labels []string) (classification, error) {
	// Trustline presence is looked up over EVERY row of the pull, including zero
	// balances. That is the whole point of this pass.
	byAccount := make(map[string]decimal.Decimal, len(all))
	for _, h := range all {
		byAccount[h.AccountID] = h.Balance
	}

	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids) // NFR-9: map keys are sorted before iteration

	c := classification{
		Labels:  labels,
		Counts:  map[string]map[string]int{},
		Totals:  map[string]int{},
		HeldQty: map[string]decimal.Decimal{},
	}
	maskSeen := map[uint64]bool{}
	var maskValues []uint64

	for _, id := range ids {
		bal, has := byAccount[id]
		status := statusNoTrustline
		switch {
		case has && !bal.IsZero():
			status = statusHoldsNonZero
		case has:
			status = statusTrustlineZero
		}
		mask := masks[id]
		group := maskLabel(mask, labels)
		if !maskSeen[mask] {
			maskSeen[mask] = true
			maskValues = append(maskValues, mask)
		}
		if c.Counts[group] == nil {
			c.Counts[group] = map[string]int{}
			c.HeldQty[group] = decimal.Zero
		}
		c.Counts[group][status]++
		c.Totals[status]++
		if status == statusHoldsNonZero {
			c.HeldQty[group] = c.HeldQty[group].Add(bal)
		}

		sc := seen[id]
		c.Rows = append(c.Rows, traderRow{
			AccountID: id,
			Status:    status,
			Balance:   bal, // zero value when the account has no trustline row
			Mask:      mask,
			Windows:   group,
			Trades:    sc.asBase + sc.asCounter,
			AsBase:    sc.asBase,
			AsCounter: sc.asCounter,
		})
		c.TotalAccts++
	}

	sort.Slice(maskValues, func(i, j int) bool { return maskValues[i] < maskValues[j] })
	for _, m := range maskValues {
		c.MaskOrder = append(c.MaskOrder, maskLabel(m, labels))
	}

	// Invariant: every trading account lands in exactly one status.
	sum := 0
	for _, s := range classOrder {
		sum += c.Totals[s]
	}
	if sum != c.TotalAccts {
		return classification{}, fmt.Errorf("classification lost accounts: %d classified, %d trading accounts", sum, c.TotalAccts)
	}
	return c, nil
}

// ---------- statistics ----------

type stats struct {
	Population     []holder // sorted: balance desc, then account ID asc
	Total          decimal.Decimal
	Top1Pct        decimal.Decimal
	TopNPct        decimal.Decimal
	TopN           int
	HHI            decimal.Decimal
	NeverTraded    int
	NeverTradedQty decimal.Decimal
	NeverTradedPct decimal.Decimal
	EverTraded     int
	EverTradedQty  decimal.Decimal
}

func computeStats(pop []holder, topN int) stats {
	// Deterministic order: balance descending, account ID ascending as tie-break. Two
	// holders with identical balances must not swap places between runs (NFR-9).
	sort.SliceStable(pop, func(i, j int) bool {
		if c := pop[j].Balance.Cmp(pop[i].Balance); c != 0 {
			return c < 0
		}
		return pop[i].AccountID < pop[j].AccountID
	})

	total := decimal.Zero
	for _, h := range pop {
		total = total.Add(h.Balance)
	}

	hundred := decimal.NewFromInt(100)
	s := stats{
		Population:     pop,
		Total:          total,
		TopN:           topN,
		HHI:            decimal.Zero,
		NeverTradedQty: decimal.Zero,
		EverTradedQty:  decimal.Zero,
	}
	cum := decimal.Zero
	topSum := decimal.Zero
	for i := range pop {
		share := pop[i].Balance.Div(total).Mul(hundred)
		pop[i].SharePct = share
		cum = cum.Add(share)
		pop[i].CumulativePct = cum

		s.HHI = s.HHI.Add(share.Mul(share))
		if i < topN {
			topSum = topSum.Add(share)
		}
		if pop[i].EverTraded() {
			s.EverTraded++
			s.EverTradedQty = s.EverTradedQty.Add(pop[i].Balance)
		} else {
			s.NeverTraded++
			s.NeverTradedQty = s.NeverTradedQty.Add(pop[i].Balance)
		}
	}
	s.Top1Pct = pop[0].SharePct
	s.TopNPct = topSum
	s.NeverTradedPct = s.NeverTradedQty.Div(total).Mul(hundred)
	return s
}

// ---------- output ----------

func renderReport(cfg config, m pullMeta, tm tradeMeta, s stats, dropped []dropRecord, c classification) string {
	var b strings.Builder
	p := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }

	p("# Holder concentration and trade cross-check: %s\n\n", assetCode)
	p("Asset: `%s:%s`\n\n", assetCode, assetIssuer)
	p("Methodology version: %s\n\n", cfg.methodologyVersion)
	p("Generated by `scripts/holderstats` from recorded files. No network access.\n\n")

	p("## Pull provenance\n\n")
	p("| Field | Value |\n|---|---|\n")
	p("| Source file | `%s` |\n", cfg.holdersPath)
	p("| Trustline rows | %d |\n", m.Rows)
	p("| Pages | %d (%s) |\n", len(m.Pages), strings.Join(m.Pages, ", "))
	p("| `latest_ledger` range | %d..%d (spread %d) |\n", m.MinLedger, m.MaxLedger, m.ledgerSpread())
	p("| `read_at_utc` range | %s..%s |\n", m.FirstReadAt, m.LastReadAt)
	p("| First record returned | `%s`, balance %s |\n", m.FirstAccount, m.FirstBalance.StringFixed(balanceScale))
	p("| Zero-balance trustlines | %d |\n", m.ZeroBalance)
	p("| Non-zero trustlines | %d |\n\n", m.NonZero)

	if m.ledgerSpread() > 0 {
		p("> The pull spans %d ledgers, so no single `LedgerSeq` describes it. ", m.ledgerSpread())
		p("Which ledger stamps the snapshot is an open decision (DEC-011 pending); this script reports the spread and does not choose.\n\n")
	}

	p("## Excluded positions\n\n")
	if len(dropped) == 0 {
		p("None.\n\n")
	} else {
		explicit := excludedPositions()
		var named []dropRecord
		zero := 0
		for _, d := range dropped {
			if _, ok := explicit[d.AccountID]; ok {
				named = append(named, d)
			} else {
				zero++
			}
		}
		p("Zero-balance trustlines dropped: %d\n\n", zero)
		if len(named) == 0 {
			p("None of the three named positions (issuer, AMM pool `%s`, Blend V2 `%s`) appear in the pull. ",
				short(ammPoolID), short(blendV2Contract))
			p("The exclusion is therefore a no-op on this data: pool-held supply is absent from the trustline set by construction, not subtracted from it.\n\n")
		} else {
			p("| Position | Balance | Reason |\n|---|---|---|\n")
			for _, d := range named {
				p("| `%s` | %s | %s |\n", d.AccountID, d.Balance.StringFixed(balanceScale), d.Reason)
			}
			p("\n")
		}
	}

	p("## Concentration\n\n")
	p("| Measure | Value |\n|---|---|\n")
	p("| Population (non-zero, filtered) | %d |\n", len(s.Population))
	p("| Circulating supply | %s %s |\n", s.Total.StringFixed(balanceScale), assetCode)
	p("| Top 1 | %s%% |\n", s.Top1Pct.StringFixed(shareScale))
	p("| Top %d | %s%% |\n", s.TopN, s.TopNPct.StringFixed(shareScale))
	p("| HHI | %s |\n\n", s.HHI.StringFixed(shareScale))

	p("## Trade cross-check\n\n")
	p("| Label | File | Rows |\n|---|---|---|\n")
	for i, f := range tm.Files {
		p("| %s | `%s` | %d |\n", tm.Labels[i], f, tm.RowsPerFile[i])
	}
	p("\nTotal %d rows, %d distinct accounts on either side.\n\n", tm.Rows, tm.DistinctAccts)

	p("| Group | Holders | Balance | Share |\n|---|---|---|---|\n")
	p("| Never appears in any recorded trade | %d | %s | %s%% |\n",
		s.NeverTraded, s.NeverTradedQty.StringFixed(balanceScale), s.NeverTradedPct.StringFixed(shareScale))
	p("| Appears at least once | %d | %s | %s%% |\n\n",
		s.EverTraded, s.EverTradedQty.StringFixed(balanceScale),
		decimal.NewFromInt(100).Sub(s.NeverTradedPct).StringFixed(shareScale))
	p("A holder that never appears is a holder over the *recorded* window only. It is not evidence the account has never traded, only that it did not trade in the files supplied.\n\n")

	p("## Trustline status of accounts that traded\n\n")
	p("Every account appearing on either side of a recorded trade, classified against the full pull including zero-balance trustlines, and split by the windows it traded in.\n\n")
	p("| Traded in | %s | %s | %s | Total |\n", statusHoldsNonZero, statusTrustlineZero, statusNoTrustline)
	p("|---|---|---|---|---|\n")
	for _, g := range c.MaskOrder {
		row := c.Counts[g]
		total := 0
		for _, st := range classOrder {
			total += row[st]
		}
		p("| %s | %d | %d | %d | %d |\n", g, row[statusHoldsNonZero], row[statusTrustlineZero], row[statusNoTrustline], total)
	}
	p("| **All** | %d | %d | %d | %d |\n\n",
		c.Totals[statusHoldsNonZero], c.Totals[statusTrustlineZero], c.Totals[statusNoTrustline], c.TotalAccts)
	p("Holding %s requires a trustline, so an account that traded necessarily held one at the time of the trade. ", assetCode)
	p("An account in the third column therefore had its trustline removed between its last recorded trade and the pull. ")
	p("This script reports the category; what it means for the metric is section 2's call.\n\n")
	p("The split by window matters: an account that traded only in the earlier window had months in which to close its trustline for any reason, ")
	p("while one that traded in the later window and is already gone is a much shorter round trip.\n\n")

	p("## Largest holders\n\n")
	p("| # | Account | Balance | Share | Cumulative | Trades | as base | as counter |\n")
	p("|---|---|---|---|---|---|---|---|\n")
	limit := s.TopN
	if limit > len(s.Population) {
		limit = len(s.Population)
	}
	for i := 0; i < limit; i++ {
		h := s.Population[i]
		p("| %d | `%s` | %s | %s%% | %s%% | %d | %d | %d |\n",
			i+1, h.AccountID, h.Balance.StringFixed(balanceScale),
			h.SharePct.StringFixed(shareScale), h.CumulativePct.StringFixed(shareScale),
			h.TradesTotal(), h.TradesAsBase, h.TradesAsCount)
	}
	p("\n`as base` and `as counter` are which side of the trade record the account occupied. They are **not** buy and sell; direction lives in `base_is_seller` and is not read here.\n\n")

	if cfg.genuineVolumeUSTRY != "" {
		vol, err := decimal.NewFromString(cfg.genuineVolumeUSTRY)
		if err == nil && !vol.IsZero() {
			p("## Volume-to-supply denominator sensitivity\n\n")
			p("Genuine volume supplied: %s %s. The denominator choice is not a detail; it moves the ratio by orders of magnitude.\n\n",
				vol.StringFixed(balanceScale), assetCode)
			p("| Denominator | %s | Ratio |\n|---|---|---|\n", assetCode)
			p("| Circulating (section 3 as written) | %s | %s |\n",
				s.Total.StringFixed(balanceScale), vol.Div(s.Total).StringFixed(9))
			if !s.EverTradedQty.IsZero() {
				p("| Held by holders seen trading | %s | %s |\n",
					s.EverTradedQty.StringFixed(balanceScale), vol.Div(s.EverTradedQty).StringFixed(9))
			}
			p("\nThis table is diagnostic. Section 3 defines the denominator; this script does not choose one.\n\n")
		}
	}
	return b.String()
}

func writeCrosscheck(path string, cfg config, m pullMeta, s stats) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating crosscheck CSV: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"rank", "account_id", "balance", "share_pct", "cumulative_pct",
		"trades_total", "trades_as_base", "trades_as_counter", "ever_traded",
		"snapshot_min_ledger", "snapshot_max_ledger", "methodology_version",
	}); err != nil {
		return err
	}
	for i, h := range s.Population {
		rec := []string{
			strconv.Itoa(i + 1),
			h.AccountID,
			h.Balance.StringFixed(balanceScale),
			h.SharePct.StringFixed(shareScale),
			h.CumulativePct.StringFixed(shareScale),
			strconv.Itoa(h.TradesTotal()),
			strconv.Itoa(h.TradesAsBase),
			strconv.Itoa(h.TradesAsCount),
			strconv.FormatBool(h.EverTraded()),
			strconv.FormatInt(m.MinLedger, 10),
			strconv.FormatInt(m.MaxLedger, 10),
			cfg.methodologyVersion,
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return w.Error()
}

func writeTraderStatus(path string, cfg config, m pullMeta, c classification) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating trader status CSV: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{
		"account_id", "trustline_status", "balance", "traded_in",
		"trades_total", "trades_as_base", "trades_as_counter",
		"snapshot_min_ledger", "snapshot_max_ledger", "methodology_version",
	}); err != nil {
		return err
	}
	for _, r := range c.Rows {
		bal := ""
		if r.Status != statusNoTrustline {
			bal = r.Balance.StringFixed(balanceScale)
		}
		rec := []string{
			r.AccountID,
			r.Status,
			bal,
			r.Windows,
			strconv.Itoa(r.Trades),
			strconv.Itoa(r.AsBase),
			strconv.Itoa(r.AsCounter),
			strconv.FormatInt(m.MinLedger, 10),
			strconv.FormatInt(m.MaxLedger, 10),
			cfg.methodologyVersion,
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return w.Error()
}

// ---------- helpers ----------

// columnIndex resolves column names to positions, failing loud with the full header list
// when a name is missing. Positional indexing is deliberately not offered: it breaks
// silently when a column is added, which is exactly how the liquidity_pool_id defect hid.
func columnIndex(header []string, path string, want ...string) (map[string]int, error) {
	pos := make(map[string]int, len(header))
	for i, h := range header {
		pos[strings.TrimSpace(h)] = i
	}
	idx := make(map[string]int, len(want))
	var missing []string
	for _, w := range want {
		i, ok := pos[w]
		if !ok {
			missing = append(missing, w)
			continue
		}
		idx[w] = i
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf(
			"%s is missing required column(s) %s; header has %d columns: %s",
			path, strings.Join(missing, ", "), len(header), strings.Join(header, ", "))
	}
	return idx, nil
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:8] + "..."
}
