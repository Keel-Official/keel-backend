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

	Now  func() time.Time
	Logf func(format string, args ...any)
}

type Recorder struct {
	cfg RecorderConfig
}

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
	return &Recorder{cfg: cfg}, nil
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
			return err
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
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return raw, fmt.Errorf("%s: %w", path, err)
	}
	defer zr.Close()

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
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Run records once immediately and then on every tick until the context is
// cancelled. Recording immediately matters: a recorder started and left for an
// hour before its first write is an hour of evidence that does not exist.
func (r *Recorder) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("recorder: interval must be positive, got %s", interval)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		r.Report(r.RecordOnce(ctx))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
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
		return domain.Asset{Code: s.Code, Issuer: s.Issuer, Type: domain.AssetType(s.Type)}, nil
	default:
		// The type is never inferred from the code length. See trap 4 in this
		// package's CLAUDE.md: a five character code read as alphanum4 returns
		// an empty book and no error, which looks exactly like a dead market.
		return domain.Asset{}, fmt.Errorf("asset type %q must be one of native, credit_alphanum4, credit_alphanum12", s.Type)
	}
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
		base, err := p.Base.asset()
		if err != nil {
			return nil, fmt.Errorf("%s: pair %d base: %w", path, i, err)
		}
		quote, err := p.Quote.asset()
		if err != nil {
			return nil, fmt.Errorf("%s: pair %d quote: %w", path, i, err)
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
