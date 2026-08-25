// The cross-validation recorder.
//
// docs/methodology/10-validation.md layer 3 compares a live Horizon reading
// against a reconstruction of the same ledger from history, and that comparison
// needs a live reading taken AT THE TIME. It cannot be produced afterwards from
// any source, which makes this the only component in the repository where a day
// of delay is a day of evidence lost permanently. That is why it is written
// before the storage layer and before the API.
//
// YELLOW ZONE. Four design decisions:
//
//  1. ONE FILE PER LEDGER, AND IT IS NEVER OVERWRITTEN. The path is
//     {root}/{pair}/{ledgerSeq}.json.gz and an existing file is left alone, which
//     makes a round inside the same ledger a skip rather than a rewrite. Same
//     rule as docs/evidences: an edited piece of evidence is not evidence.
//     Rejected alternative: one append-only JSONL stream per pair, which is
//     cheaper to write and gives up the property that a single recording can be
//     handed to somebody as a file.
//
//     GZIPPED, and not by preference. Two documents written before this code
//     already said so: the header of migrations/0001_core.sql, which explains
//     why raw snapshots are files rather than table rows, and
//     internal/store/CLAUDE.md. The first version of this recorder wrote plain
//     JSON and contradicted both. Measured on live USTRY/USDC recordings, 9292
//     bytes of JSON become 2278, about four to one, so two weeks of eight pairs
//     every thirty minutes is roughly 17 MB rather than 70.
//
//  2. WRITE TO A TEMPORARY NAME AND RENAME. A recorder killed mid-write must not
//     leave a truncated file that looks like a recording. Rename within one
//     directory is atomic on the filesystems this runs on.
//
//  3. ASSET IDENTITY IS VERIFIED ONCE, NOT PER ROUND. Verify is called at
//     startup. A wrong AssetType returns an empty order book with no error, so
//     the check is essential; paying it every thirty minutes for an answer that
//     cannot change would spend the rate limit budget on nothing.
//
//  4. THE PAIR LIST IS DATA, NOT CODE. It is loaded from a file and this package
//     ships no default. Which assets Keel measures is decision D-1 and
//     docs/methodology/02-pair-selection.md is still a worksheet, so a list
//     compiled into the binary would be this package quietly making a
//     methodology decision that belongs to its author. Rejected alternative: a
//     hardcoded list of the eight obvious Stellar assets, which is convenient
//     and would be cited later as if it had been chosen.
//
//  5. HOLDERS ARE RECORDED PER BASE ASSET, NOT PER PAIR, AND ONLY WHEN ASKED.
//     A holder reading belongs to an asset rather than to a market, so recording
//     it once per pair would write the same USDC holder set under every pair
//     that quotes in USDC. Only BASE assets are read: the base is the asset being
//     measured, and a quote asset like USDC has hundreds of thousands of
//     trustlines that would eat the hourly budget to answer a question nothing
//     asks. It is off by default because it is the one reading here whose cost
//     grows with the asset rather than staying at three requests. See
//     holders.go for why it has to be recorded at all rather than fetched later.
//
//  6. THE TWO READINGS KEEP THEIR OWN CLOCKS, IN ONE PROCESS. Holders were tied
//     to the pair tick until 25 August 2026, which set the cadence of a quantity
//     that moves over days by the cadence of one that moves in seconds. The gate
//     is a comparison against the last holder round rather than a second ticker,
//     because two tickers drift apart and then the log stops saying which round a
//     file belongs to. Rejected alternative: telling the operator to run a second
//     `keel record` process at a longer interval, which works today and needs no
//     code, but gives each process its own request budget counter with no view of
//     the other, and two counters of 3000 against one Horizon limit of about 3600
//     is a limit that is not being counted at all.

package horizon

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// Pair is one base/quote pair to record.
//
// Note is carried through from the pair file and ignored by this package. It
// exists because the pair file is the single place the demonstration set is
// declared, and the assets table has a selection_note column asking why an asset
// is in that set. Loading the file twice with two loaders, one per consumer,
// would be the worse trade.
type Pair struct {
	Base  domain.Asset
	Quote domain.Asset
	Note  string
}

func (p Pair) String() string { return p.Base.String() + "/" + p.Quote.String() }

// Slug is the directory name for this pair. The issuer is truncated to eight
// characters, which is enough to tell two issuers of the same code apart while
// keeping the path readable. Both halves are always present, so a directory name
// cannot be mistaken for a different pair.
func (p Pair) Slug() string { return assetSlug(p.Base) + "-" + assetSlug(p.Quote) }

func assetSlug(a domain.Asset) string {
	if a.IsNative() {
		return "XLM.native"
	}
	issuer := a.Issuer
	if len(issuer) > 8 {
		issuer = issuer[:8]
	}
	return a.Code + "." + issuer
}

// RecorderConfig configures a Recorder. Root and Pairs are required.
type RecorderConfig struct {
	Client *Client
	Root   string
	Pairs  []Pair

	// Holders turns on the holder distribution reading, one file per BASE asset
	// per ledger under {Root}/holders. It is off by default and that is a
	// budget decision rather than a preference: a holder reading costs one
	// request per two hundred accounts, where a pair snapshot costs three
	// regardless of how deep the book is. See decision 5.
	Holders bool

	// HolderInterval is how often the holder reading is taken, and zero means
	// every round, which is what it did when Holders was the only knob.
	//
	// It exists because the two readings measure quantities that move at
	// different speeds. An order book turns over in seconds and is the reason
	// the pair interval is thirty minutes. A trustline balance moves over days,
	// so recording one every thirty minutes writes forty-eight near-identical
	// files per asset per day for a distribution that changed once, and this is
	// the reading whose file size grows with the asset. See decision 6.
	HolderInterval time.Duration

	// Schema is which recording schema a round writes, 1 or 2, and zero means
	// TickSchemaVersion.
	//
	// THE DEFAULT IS 2, which is a change of behavior and is the point. Schema 1
	// writes the parsed conclusions beside the bytes, and the parsed half is the
	// half that had to be revised once already; schema 2 writes bytes and makes
	// no claim, so the hourly production path defaults to the one that cannot go
	// stale. Schema 1 stays reachable, and every file it ever wrote stays
	// readable through ReadRecording. Rejected alternative: leaving 1 as the
	// default and letting the workflow opt in, which makes the correct schema
	// depend on somebody remembering a flag and records the wrong one silently
	// when they do not.
	Schema int

	Now  func() time.Time
	Logf func(format string, args ...any)
}

// Recorder writes raw Horizon readings to disk, one file per pair per ledger,
// never overwriting one that exists. It is the only component here whose work
// cannot be caught up later: a ledger that closed unrecorded is gone.
type Recorder struct {
	cfg RecorderConfig
}

// NewRecorder builds a Recorder and refuses a config that would record nothing.
// An empty pair list is an error rather than a quiet no-op, because a recorder
// that runs for a week writing no files looks identical to one that is working.
func NewRecorder(cfg RecorderConfig) (*Recorder, error) {
	if cfg.Client == nil {
		return nil, errors.New("recorder: no client")
	}
	if cfg.Root == "" {
		return nil, errors.New("recorder: no output root")
	}
	if len(cfg.Pairs) == 0 {
		return nil, errors.New("recorder: no pairs; see -pairs and decision 4 in recorder.go")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Schema == 0 {
		cfg.Schema = TickSchemaVersion
	}
	if cfg.Schema != 1 && cfg.Schema != TickSchemaVersion {
		return nil, fmt.Errorf("recorder: schema %d is not 1 or %d", cfg.Schema, TickSchemaVersion)
	}
	return &Recorder{cfg: cfg}, nil
}

// recordRound records one round in the configured schema and returns how much
// evidence failed to reach disk. It is the single place the two schemas are
// chosen between, so Run and the one-shot path cannot disagree about which one
// is in force.
func (r *Recorder) recordRound(ctx context.Context) int {
	if r.cfg.Schema == 1 {
		return r.Report(r.RecordOnce(ctx))
	}
	return r.ReportTicks(r.RecordTicksOnce(ctx)).Unwritten
}

// Result is the outcome for one pair in one round.
type Result struct {
	Pair      Pair
	Path      string
	LedgerSeq uint32
	// Atomic is false when the book and the pools were served from different
	// ledgers. The recording is kept either way, with the flag inside it; a
	// difference that is recorded can be explained later, and one that was
	// discarded cannot.
	Atomic bool
	// Skipped means this ledger was already on disk for this pair.
	Skipped bool
	Err     error
}

// Verify checks every distinct asset in the pair list against Horizon once.
func (r *Recorder) Verify(ctx context.Context) error {
	seen := map[string]bool{}
	var assets []domain.Asset
	for _, p := range r.cfg.Pairs {
		for _, a := range []domain.Asset{p.Base, p.Quote} {
			key := string(a.Type) + "|" + a.Code + "|" + a.Issuer
			if !seen[key] {
				seen[key] = true
				assets = append(assets, a)
			}
		}
	}
	// Sorted so the request order is reproducible, which is the same rule
	// non-negotiable number 2 states for map iteration.
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].String() < assets[j].String()
	})
	for _, a := range assets {
		if err := r.cfg.Client.VerifyAsset(ctx, a); err != nil {
			// Name the PAIRS, not only the asset. A message that says an issuer
			// is unknown leaves the operator to work out which line of the pair
			// file to look at, and the whole point of failing loudly at startup
			// is that they do not have to.
			return fmt.Errorf("%w (used by %s)", err, strings.Join(r.PairsUsing(a), ", "))
		}
		r.cfg.Logf("verified %s as %s", a, a.Type)
	}
	return nil
}

// RecordOnce records every pair once. One pair failing does not stop the rest:
// a round that gives up on the first error records nothing on the day one asset
// is delisted.
func (r *Recorder) RecordOnce(ctx context.Context) []Result {
	out := make([]Result, 0, len(r.cfg.Pairs))
	for _, p := range r.cfg.Pairs {
		if err := ctx.Err(); err != nil {
			out = append(out, Result{Pair: p, Err: err})
			return out
		}
		out = append(out, r.record(ctx, p))
	}
	return out
}

func (r *Recorder) record(ctx context.Context, p Pair) Result {
	res := Result{Pair: p}

	obs, err := r.cfg.Client.GetSnapshot(ctx, p.Base, p.Quote)
	if err != nil {
		res.Err = err
		return res
	}
	res.LedgerSeq = obs.Snapshot.LedgerSeq
	res.Atomic = obs.Raw.Atomic

	dir := filepath.Join(r.cfg.Root, p.Slug())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		res.Err = err
		return res
	}
	res.Path = filepath.Join(dir, strconv.FormatUint(uint64(res.LedgerSeq), 10)+recordingSuffix)

	if _, err := os.Stat(res.Path); err == nil {
		res.Skipped = true
		return res
	} else if !errors.Is(err, os.ErrNotExist) {
		res.Err = err
		return res
	}

	body, err := json.MarshalIndent(obs.Raw, "", "  ")
	if err != nil {
		res.Err = err
		return res
	}
	packed, err := gzipBytes(append(body, '\n'))
	if err != nil {
		res.Err = err
		return res
	}
	if err := writeAtomic(res.Path, packed); err != nil {
		res.Err = err
	}
	return res
}

// recordingSuffix is the extension of a recording. It is a constant because the
// skip check, the writer and the tests all have to agree on it, and a literal
// repeated three times is how they stop agreeing.
const recordingSuffix = ".json.gz"

// holdersDir is the subdirectory holder readings live under. It sits beside the
// pair directories rather than inside one, because a holder reading belongs to
// an asset and not to a market. See decision 5.
const holdersDir = "holders"

// HolderResult is the outcome for one asset in one holder round.
type HolderResult struct {
	Asset     domain.Asset
	Path      string
	LedgerSeq uint32
	// Holders is how many accounts were read, which is NOT the holder count when
	// Truncated is true.
	Holders   int
	Truncated bool
	// Atomic is false when the pages came from different ledgers.
	Atomic bool
	// Skipped means this ledger was already on disk for this asset.
	Skipped bool
	Err     error
}

// HolderAssets is the list this recorder reads holders for: every distinct
// non-native BASE asset in the pair list, in sorted order.
//
// It is exported because the answer is not obvious from the pair file and a
// caller about to spend a request budget should be able to see it first. Sorted
// for the same reason the verification order is: non-negotiable rule 2.
func (r *Recorder) HolderAssets() []domain.Asset {
	seen := map[string]bool{}
	var out []domain.Asset
	for _, p := range r.cfg.Pairs {
		if p.Base.IsNative() {
			continue
		}
		key := string(p.Base.Type) + "|" + p.Base.Code + "|" + p.Base.Issuer
		if !seen[key] {
			seen[key] = true
			out = append(out, p.Base)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// RecordHoldersOnce reads the holder distribution for every base asset once.
// One asset failing does not stop the rest, for the reason RecordOnce gives.
//
// It returns nil when Holders is off, rather than an empty slice, so a caller
// can tell "not asked for" apart from "asked for and nothing to do".
func (r *Recorder) RecordHoldersOnce(ctx context.Context) []HolderResult {
	if !r.cfg.Holders {
		return nil
	}
	assets := r.HolderAssets()
	out := make([]HolderResult, 0, len(assets))
	for _, a := range assets {
		if err := ctx.Err(); err != nil {
			out = append(out, HolderResult{Asset: a, Err: err})
			return out
		}
		out = append(out, r.recordHolders(ctx, a))
	}
	return out
}

func (r *Recorder) recordHolders(ctx context.Context, a domain.Asset) HolderResult {
	res := HolderResult{Asset: a}

	obs, err := r.cfg.Client.GetHolders(ctx, a)
	if err != nil {
		res.Err = err
		return res
	}
	res.LedgerSeq = obs.Raw.FirstLedger
	res.Holders = len(obs.Holders)
	res.Truncated = obs.Truncated()
	res.Atomic = obs.Raw.Atomic

	dir := filepath.Join(r.cfg.Root, holdersDir, assetSlug(a))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		res.Err = err
		return res
	}
	res.Path = filepath.Join(dir, strconv.FormatUint(uint64(res.LedgerSeq), 10)+recordingSuffix)

	if _, err := os.Stat(res.Path); err == nil {
		res.Skipped = true
		return res
	} else if !errors.Is(err, os.ErrNotExist) {
		res.Err = err
		return res
	}

	body, err := json.MarshalIndent(obs.Raw, "", "  ")
	if err != nil {
		res.Err = err
		return res
	}
	packed, err := gzipBytes(append(body, '\n'))
	if err != nil {
		res.Err = err
		return res
	}
	if err := writeAtomic(res.Path, packed); err != nil {
		res.Err = err
	}
	return res
}

// ReadHolderRecording reads one holder recording back, and exists for the same
// reason ReadRecording does: one reader and one writer for a format.
func ReadHolderRecording(path string) (RawHolders, error) {
	var raw RawHolders
	f, err := os.Open(path)
	if err != nil {
		return raw, err
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return raw, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	if err := json.NewDecoder(zr).Decode(&raw); err != nil {
		return raw, fmt.Errorf("%s: %w", path, err)
	}
	return raw, nil
}

// gzipBytes compresses in memory rather than streaming into the file, so that a
// compression failure happens before the file exists. writeAtomic can then keep
// its promise that a file which exists is a complete recording.
func gzipBytes(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ReadRecording reads one recording back. It exists so that whatever compares a
// recording against reconstructed history does not have to know how the file was
// written, and so the format has exactly one reader and one writer.
func ReadRecording(path string) (RawSnapshot, error) {
	var raw RawSnapshot
	f, err := os.Open(path)
	if err != nil {
		return raw, err
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return raw, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	if err := json.NewDecoder(zr).Decode(&raw); err != nil {
		return raw, fmt.Errorf("%s: %w", path, err)
	}
	return raw, nil
}

// writeAtomic writes through a temporary file in the same directory. See
// decision 2.
func writeAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".partial-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// Run records once immediately and then on every tick until the context is
// canceled. Recording immediately matters: a recorder started and left for an
// hour before its first write is an hour of evidence that does not exist.
func (r *Recorder) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("recorder: interval must be positive, got %s", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// The zero value means no holder round has happened yet, and dueForHolders
	// reads it as due. The first round therefore always takes one, for the same
	// reason the pair round is taken before the first tick: a recorder that waits
	// six hours for its first holder reading is six hours of a distribution that
	// cannot be fetched afterwards.
	var lastHolders time.Time

	for {
		r.recordRound(ctx)
		if now := r.cfg.Now(); dueForHolders(lastHolders, now, r.cfg.HolderInterval) {
			r.ReportHolders(r.RecordHoldersOnce(ctx))
			lastHolders = now
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// dueForHolders decides whether this round takes a holder reading.
//
// It is a function rather than three lines inside Run so that the rule can be
// tested without driving a ticker, which is the only part of Run that is worth
// testing at all. The comparison is >= and not >, so an interval that divides the
// pair interval exactly does not skip a round to floating point luck: with a
// 30 minute tick and a 6 hour holder interval, round 12 is due, not round 13.
//
// A stamp in the future, which a clock correction can produce, reads as not due
// rather than as due forever. That is the conservative direction here: the cost of
// a missed round is one gap in a series recorded every few hours, and the cost of
// the other reading is a holder sweep on every single tick.
func dueForHolders(last, now time.Time, interval time.Duration) bool {
	if interval <= 0 {
		return true
	}
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= interval
}

// Report logs one round and returns how many pairs failed. It is exported so a
// single round and the loop print the same lines: two call sites formatting the
// same results their own way is how a log stops being comparable across runs.
func (r *Recorder) Report(results []Result) int {
	var written, skipped, failed, straddled int
	for _, res := range results {
		switch {
		case res.Err != nil:
			failed++
			r.cfg.Logf("FAIL  %s: %v", res.Pair, res.Err)
		case res.Skipped:
			skipped++
			r.cfg.Logf("skip  %s ledger %d, already recorded", res.Pair, res.LedgerSeq)
		default:
			written++
			if !res.Atomic {
				straddled++
			}
			r.cfg.Logf("write %s ledger %d atomic=%t -> %s",
				res.Pair, res.LedgerSeq, res.Atomic, res.Path)
		}
	}
	r.cfg.Logf("round: %d written, %d skipped, %d failed, %d straddled a ledger boundary, %d requests this window",
		written, skipped, failed, straddled, r.cfg.Client.Requests())
	return failed
}

// ReportHolders logs one holder round and returns how many assets failed.
// Nothing is logged when holder recording is off, which is what a nil slice
// from RecordHoldersOnce means.
//
// A TRUNCATED reading is logged as loudly as a failure without being counted as
// one, because it is a recording that was written and cannot answer the question
// it was taken for. Counting it as a failure would make a run of a large asset
// look broken; logging it quietly would let a concentration figure be computed
// later from a subset nobody remembers was a subset.
func (r *Recorder) ReportHolders(results []HolderResult) int {
	if results == nil {
		return 0
	}
	var written, skipped, failed, truncated int
	for _, res := range results {
		switch {
		case res.Err != nil:
			failed++
			r.cfg.Logf("FAIL  holders %s: %v", res.Asset, res.Err)
		case res.Skipped:
			skipped++
			r.cfg.Logf("skip  holders %s ledger %d, already recorded", res.Asset, res.LedgerSeq)
		default:
			written++
			r.cfg.Logf("write holders %s ledger %d holders=%d atomic=%t -> %s",
				res.Asset, res.LedgerSeq, res.Holders, res.Atomic, res.Path)
		}
		if res.Truncated {
			truncated++
			r.cfg.Logf("TRUNCATED holders %s: the page cap stopped at %d accounts, so this reading "+
				"is a lower bound on the holder count and answers no concentration question",
				res.Asset, res.Holders)
		}
	}
	r.cfg.Logf("holder round: %d written, %d skipped, %d failed, %d truncated, %d requests this window",
		written, skipped, failed, truncated, r.cfg.Client.Requests())
	return failed
}

// ---------------------------------------------------------------- Pair file

// pairFile is the on-disk shape of the pair list. Note is decoded so that a
// human explanation can live in the same file as the data without a parser
// complaining about it.
type pairFile struct {
	Note  string `json:"note"`
	Pairs []struct {
		Base  assetSpec `json:"base"`
		Quote assetSpec `json:"quote"`
		// Note is why this pair is in the demonstration set. It ends up in the
		// assets table's selection_note column.
		Note string `json:"note"`
	} `json:"pairs"`
}

type assetSpec struct {
	Code   string `json:"code"`
	Issuer string `json:"issuer"`
	Type   string `json:"type"`
}

func (s assetSpec) asset() (domain.Asset, error) {
	switch domain.AssetType(s.Type) {
	case domain.AssetTypeNative:
		if s.Code != "" || s.Issuer != "" {
			return domain.Asset{}, fmt.Errorf("native asset must carry no code or issuer")
		}
		return domain.Asset{Type: domain.AssetTypeNative}, nil
	case domain.AssetTypeAlphanum4, domain.AssetTypeAlphanum12:
		if s.Code == "" || s.Issuer == "" {
			return domain.Asset{}, fmt.Errorf("%s asset needs both a code and an issuer", s.Type)
		}
		if err := checkIssuer(s.Issuer); err != nil {
			return domain.Asset{}, err
		}
		return domain.Asset{Code: s.Code, Issuer: s.Issuer, Type: domain.AssetType(s.Type)}, nil
	default:
		// The type is never inferred from the code length. See trap 4 in this
		// package's CLAUDE.md: a five character code read as alphanum4 returns
		// an empty book and no error, which looks exactly like a dead market.
		return domain.Asset{}, fmt.Errorf("asset type %q must be one of native, credit_alphanum4, credit_alphanum12", s.Type)
	}
}

// describe names an asset spec for an error message, including a malformed one.
// It reads the RAW fields rather than a parsed domain.Asset, because the errors
// it feeds are the ones raised when parsing failed.
func (s assetSpec) describe() string {
	if s.Code == "" && s.Issuer == "" {
		return s.Type
	}
	return s.Code + ":" + s.Issuer
}

// issuerLen is the length of a strkey encoded Stellar account ID: the 56
// characters of base32 that every G... address is.
const issuerLen = 56

// checkIssuer rejects a malformed issuer BEFORE any request is made.
//
// It exists because a malformed issuer fails in three different places
// otherwise, none of them early and none of them naming the pair. Horizon
// answers /assets with a 400 whose body says "is not a valid asset issuer",
// /liquidity_pools quietly returns no pools, and /order_book returns an empty
// book, which is indistinguishable from a dead market and is trap 4 in this
// package's CLAUDE.md wearing a different hat. This was not hypothetical: the
// AUDD issuer supplied for configs/recorder-pairs.json was 57 characters, one
// too many, and Horizon rejected it. See
// docs/evidences/liquidity_pools_reserves_2026-08-25.txt section 7.
//
// It checks the SHAPE and not the checksum. The last two bytes of a strkey are a
// CRC16 and verifying them needs a base32 decoder here, which would be the third
// place in this repository that knows the strkey format; the shape check catches
// every transcription error seen so far, and VerifyAsset then asks Horizon,
// which owns the real answer. NEVER FALLS BACK TO A DEFAULT: the pair file is
// data and a recorder that substitutes its own issuer for a bad one is recording
// a market nobody asked for.
func checkIssuer(issuer string) error {
	if len(issuer) != issuerLen {
		return fmt.Errorf("issuer %q is %d characters and a Stellar account ID is %d",
			issuer, len(issuer), issuerLen)
	}
	if issuer[0] != 'G' {
		return fmt.Errorf("issuer %q does not start with G, so it is not an account ID", issuer)
	}
	for i := 0; i < len(issuer); i++ {
		switch c := issuer[i]; {
		case c >= 'A' && c <= 'Z', c >= '2' && c <= '7':
		default:
			return fmt.Errorf("issuer %q holds %q at position %d, which is not in the base32 alphabet",
				issuer, string(issuer[i]), i)
		}
	}
	return nil
}

// LoadPairs reads a pair list. It rejects a duplicate pair rather than recording
// it twice, because two entries for one pair double the request cost and produce
// one file, so the waste is invisible in the output.
func LoadPairs(path string) ([]Pair, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f pairFile
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(f.Pairs) == 0 {
		return nil, fmt.Errorf("%s: holds no pairs", path)
	}

	seen := map[string]bool{}
	out := make([]Pair, 0, len(f.Pairs))
	for i, p := range f.Pairs {
		// Every error below names the offending pair as the FILE spells it, so a
		// list of eight is not eight lines to check by hand.
		spelled := p.Base.describe() + "/" + p.Quote.describe()
		base, err := p.Base.asset()
		if err != nil {
			return nil, fmt.Errorf("%s: pair %d (%s) base: %w", path, i, spelled, err)
		}
		quote, err := p.Quote.asset()
		if err != nil {
			return nil, fmt.Errorf("%s: pair %d (%s) quote: %w", path, i, spelled, err)
		}
		if base.Equal(quote) {
			return nil, fmt.Errorf("%s: pair %d has the same asset on both sides", path, i)
		}
		pair := Pair{Base: base, Quote: quote, Note: p.Note}
		if seen[pair.String()] {
			return nil, fmt.Errorf("%s: pair %d duplicates %s", path, i, pair)
		}
		seen[pair.String()] = true
		out = append(out, pair)
	}
	return out, nil
}
