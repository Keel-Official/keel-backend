// Recording schema version 2: the raw tick.
//
// A version 1 recording (RawSnapshot, in client.go) holds the parsed conclusions
// AND the bytes they were drawn from. A version 2 recording holds ONLY the
// bytes. That is the whole difference and it is deliberate: this file is the
// evidence path, and evidence that has already been interpreted is worth less
// than evidence that has not.
//
// Nothing here parses a price, converts an amount, or looks inside a response
// body at all. A body arrives as bytes, becomes a string, gets hashed, and is
// written. There is no float32, no float64, no decimal, and no arithmetic on
// anything Horizon said, not even as an intermediate.
//
// YELLOW ZONE, so every design decision is stated with the alternative it beat.
// Six of them:
//
//  1. THE TICK STORES BYTES AND MAKES NO CLAIM ABOUT THEM. Each source keeps the
//     endpoint, the exact URL requested, the HTTP status, the body verbatim as a
//     string, and the sha256 of that body. Version 1 already kept raw bodies
//     beside a parsed snapshot, and the parsed half is what had to be revised
//     when the bid amount unit turned out to be quote-denominated rather than
//     base; a recording that never claimed anything would not have needed
//     revising at all. Rejected alternative: reusing GetSnapshot and adding the
//     pool bytes to it, which is far less code and bakes today's reading of the
//     order book into every file again, which is the exact mistake that
//     BidAmountUnit exists to document.
//
//  2. THE SHA256 IS COMPUTED IN ONE PLACE, FROM THE FIELD THAT IS STORED.
//     getRaw fills in Body and the digest is taken from Body afterwards, so
//     "body_sha256 is the digest of body" holds by construction rather than by
//     discipline. A mismatch discovered later therefore means the FILE was
//     changed after it was written, which is the only thing a checksum inside
//     the file it describes can honestly prove. Rejected alternative: hashing
//     the []byte at the point it comes off the wire and assigning both fields
//     separately, which allows the two to drift apart in a future edit and turns
//     the digest into a second opinion instead of a seal.
//
//  3. LEDGER_BEFORE AND LEDGER_AFTER BRACKET THE TICK AND COST NOTHING EXTRA.
//     They are the Latest-Ledger header of the FIRST request and of the LAST
//     request in the tick, so the pair wraps both calls without a third call
//     being made to fetch them. ledger_consistent is true only when the two
//     agree AND neither is zero, because two absent headers are two failures
//     rather than an agreement. Rejected alternative: an extra /ledgers request
//     before and after the pair, which is a genuinely tighter bracket and costs
//     50% more requests per tick for a bound that the two headers already give.
//
//  4. A NON-2XX IS AN ANSWER, NOT AN ERROR. Horizon returning 429 or 503 is
//     recorded with its status and whatever body came with it, and the tick is
//     still written; the retry loop still runs first, so a busy Horizon is given
//     its chances before its refusal is filed. The reason is operational rather
//     than aesthetic: a recorder that goes red whenever Horizon is busy is a
//     recorder whose red is ignored, and the tick it threw away cannot be taken
//     again once that ledger closes. Rejected alternative: returning the
//     StatusError the way Client.get does, which is right for a caller computing
//     a depth figure and wrong for one whose entire job is to write down what
//     happened.
//
//  5. THE FILE IS CLAIMED WITH A HARD LINK, WHICH CANNOT OVERWRITE. The body is
//     written to a temporary file and then linked into place under
//     {LEDGER}.json.gz, or {LEDGER}-1.json.gz, -2 and so on if that name is
//     taken. os.Link fails with ErrExist rather than replacing, so the "never
//     overwrite" rule is enforced by the filesystem in one atomic step instead
//     of by a stat that another process can invalidate between the check and the
//     write. Rejected alternative: the stat-then-rename that writeAtomic uses
//     for version 1, which is fine when a colliding name means "skip" and is
//     unsafe here, where a collision means "write it under the next name".
//
//  6. THE PATH IS DERIVED FROM THE TICK, NOT FROM THE CLOCK. The date directory
//     comes from parsing the tick's own recorded_at and the filename from its own
//     ledger_before, so the location of a file is a function of its contents and
//     a recording can be put back where it belongs from the file alone. Rejected
//     alternative: formatting the recorder's clock directly, which is one line
//     shorter and lets a file whose recorded_at says 23:59 land in tomorrow's
//     directory when the two clock reads straddle midnight.
package horizon

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

// TickSchemaVersion is the version stamped into every tick written by this file.
//
// Version 1 files carry NO schema_version field at all, so PeekSchemaVersion
// reads their absence as 1. That is the whole of the compatibility story: the
// four recordings taken on 24 August 2026 are read by ReadRecording exactly as
// they always were, and nothing here rewrites them.
const TickSchemaVersion = 2

// The endpoint names that appear in a source entry. They are constants for the
// same reason recordingSuffix is: the writer, the reader and the tests have to
// agree on the spelling, and three string literals is how they stop agreeing.
const (
	// EndpointOrderBook names the /order_book source of a tick.
	EndpointOrderBook = "order_book"
	// EndpointLiquidityPools names the /liquidity_pools source of a tick.
	EndpointLiquidityPools = "liquidity_pools"
)

// tickDateLayout is the {DATE} element of recordings/{PAIR}/{DATE}/{LEDGER}.json.gz.
// UTC, always, because a recorder that changes its directory layout when the
// machine it runs on changes timezone is a recorder whose archive cannot be
// merged with another machine's.
const tickDateLayout = "2006-01-02"

// bookPageLimit is the limit sent on the order book query, and it matches what
// GetSnapshot sends. Named rather than repeated so the two paths cannot drift
// into requesting different depths and producing files that are not comparable.
const bookPageLimit = 200

// maxCollisionSuffix bounds the monotonic suffix search. It is a runaway guard
// and not a real limit: reaching it means a thousand ticks were written for one
// pair at one ledger on one day, which is a bug somewhere else.
const maxCollisionSuffix = 999

// RawSource is one HTTP exchange, kept verbatim.
//
// Body is a string rather than a json.RawMessage on purpose. A json.RawMessage
// would be re-encoded on the way out and would refuse to hold a body that is not
// valid JSON, and a 502 from a load balancer in front of Horizon is HTML. The
// bytes that arrived are the evidence whether or not they parse.
type RawSource struct {
	Endpoint   string `json:"endpoint"`
	URL        string `json:"url"`
	HTTPStatus int    `json:"http_status"`
	Body       string `json:"body"`
	BodySHA256 string `json:"body_sha256"`

	// Error is set ONLY when there was no HTTP response at all: a transport
	// failure, a canceled context, or an exhausted request budget. It is
	// omitted otherwise, so its presence is the difference between "Horizon said
	// no" and "Horizon was never reached", which HTTPStatus 0 alone cannot tell
	// apart. It never describes the CONTENT of a body; this file makes no
	// judgements about data quality.
	Error string `json:"error,omitempty"`
}

// RawTick is one reading of one pair: the order book and the AMM pool reserves,
// taken back to back, written as one file.
type RawTick struct {
	SchemaVersion int    `json:"schema_version"`
	Pair          string `json:"pair"`
	RecordedAt    string `json:"recorded_at"`

	LedgerBefore uint32 `json:"ledger_before"`
	LedgerAfter  uint32 `json:"ledger_after"`
	// LedgerConsistent is false when the two requests were served from different
	// ledgers, and the tick is stored either way. See decision 3, and note that
	// this is a statement about the READING and not about the market.
	LedgerConsistent bool `json:"ledger_consistent"`

	Sources []RawSource `json:"sources"`
}

// Source returns the entry for one endpoint. Linear over two elements, which is
// faster than a map and, more to the point, cannot iterate in a random order.
func (t RawTick) Source(endpoint string) (RawSource, bool) {
	for _, s := range t.Sources {
		if s.Endpoint == endpoint {
			return s, true
		}
	}
	return RawSource{}, false
}

// Degraded reports whether any source is something other than a 2xx.
//
// It is a question a CALLER asks in order to log or to summarize, and the answer
// is deliberately NOT stored in the file. Nothing about the tick changes because
// of it; see decision 4.
func (t RawTick) Degraded() bool {
	for _, s := range t.Sources {
		if s.HTTPStatus < 200 || s.HTTPStatus > 299 {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- Fetching

// GetRawTick reads the order book and the liquidity pools for one pair and
// returns them as a schema 2 tick. Two requests against the budget, where
// GetSnapshot costs three, because nothing here needs the ledger close time.
//
// The error is returned ONLY when the context is done. Every other failure,
// including a non-2xx and a transport error, lands inside the tick and the tick
// is still worth writing.
func (c *Client) GetRawTick(ctx context.Context, base, quote domain.Asset) (RawTick, error) {
	tick := RawTick{
		SchemaVersion: TickSchemaVersion,
		Pair:          base.String() + "/" + quote.String(),
		RecordedAt:    c.cfg.Now().UTC().Format(time.RFC3339),
		Sources:       make([]RawSource, 0, 2),
	}

	bookQ := url.Values{}
	addAsset(bookQ, "selling", base)
	addAsset(bookQ, "buying", quote)
	bookQ.Set("limit", strconv.Itoa(bookPageLimit))

	// The reserves filter is ONE parameter holding two canonical asset strings
	// separated by a comma: `native` for XLM and `CODE:ISSUER` otherwise, with no
	// asset type anywhere in it. Measured on 25 August 2026 rather than assumed,
	// including that the order of the two does not change the result and that the
	// filter is an AND over both:
	// docs/evidences/liquidity_pools_reserves_2026-08-25.txt.
	//
	// base,quote is sent because that ordering is then the one written into the
	// recording, and a reader never has to wonder which way round it went.
	poolQ := url.Values{}
	poolQ.Set("reserves", horizonAsset(base)+","+horizonAsset(quote))
	poolQ.Set("limit", strconv.Itoa(poolPageLimit))

	// A fixed slice and not a map. Non-negotiable rule 2 says map keys are sorted
	// before iteration; the simpler way to obey it is to have no map, and the
	// order of these two requests is load-bearing because it defines which header
	// becomes ledger_before and which becomes ledger_after.
	plan := []struct {
		endpoint string
		path     string
		query    url.Values
	}{
		{EndpointOrderBook, "/order_book", bookQ},
		{EndpointLiquidityPools, "/liquidity_pools", poolQ},
	}

	for i, step := range plan {
		if err := ctx.Err(); err != nil {
			return tick, err
		}
		src, latest := c.getRaw(ctx, step.endpoint, step.path, step.query)
		tick.Sources = append(tick.Sources, src)
		if i == 0 {
			tick.LedgerBefore = latest
		}
		tick.LedgerAfter = latest
	}

	tick.LedgerConsistent = tick.LedgerBefore != 0 && tick.LedgerBefore == tick.LedgerAfter
	return tick, nil
}

// getRaw performs one request and returns the exchange verbatim.
//
// It deliberately does NOT use the response cache. The cache exists so that
// `scan` over fifty assets sharing one quote asset does not ask the same
// question fifty times; a recorder that served a cached body would write two
// recordings that are identical for a reason having nothing to do with the
// market, which is the one thing this archive must never contain.
func (c *Client) getRaw(ctx context.Context, endpoint, path string, q url.Values) (RawSource, uint32) {
	src, latest := c.getRawExchange(ctx, endpoint, path, q)
	// One place, one digest, taken from the field that is stored. See decision 2.
	sum := sha256.Sum256([]byte(src.Body))
	src.BodySHA256 = hex.EncodeToString(sum[:])
	return src, latest
}

func (c *Client) getRawExchange(ctx context.Context, endpoint, path string, q url.Values) (RawSource, uint32) {
	full := c.cfg.BaseURL + path
	if len(q) > 0 {
		full += "?" + q.Encode()
	}
	src := RawSource{Endpoint: endpoint, URL: full}

	var (
		lastErr    error
		lastLatest uint32
		answered   bool
	)
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			c.cfg.Sleep(c.backoff(attempt, lastErr))
		}
		if err := c.spend(); err != nil {
			if !answered {
				src.Error = err.Error()
			}
			return src, lastLatest
		}

		body, status, latest, retryAfter, err := c.rawAttempt(ctx, full)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				if !answered {
					src.Error = ctxErr.Error()
				}
				return src, lastLatest
			}
			lastErr = err
			if !answered {
				src.Error = err.Error()
			}
			continue
		}

		// Any HTTP status is an answer and replaces whatever a previous attempt
		// left behind, including the Error of an earlier transport failure.
		answered = true
		src.HTTPStatus = status
		src.Body = string(body)
		src.Error = ""
		lastLatest = latest

		if status == http.StatusTooManyRequests || status >= 500 {
			// Retryable. Keep it as the answer in case the retries run out, and
			// go round again; the backoff honors Horizon's Retry-After through
			// the same StatusError the parsed path uses.
			lastErr = &StatusError{
				Status: status,
				URL:    full,
				Body:   truncate(string(body), 400),
				retry:  retryAfter,
			}
			continue
		}
		return src, latest
	}
	return src, lastLatest
}

// rawAttempt is the transport half of getRaw. Unlike Client.attempt it does not
// turn a status into an error and does not require the Latest-Ledger header:
// a missing header is a zero, which ledger_consistent then reports as false
// rather than the client refusing to record anything at all.
func (c *Client) rawAttempt(ctx context.Context, full string) (body []byte, status int, latest uint32, retryAfter string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, 0, 0, "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return nil, 0, 0, "", &transportError{err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, 0, 0, "", &transportError{err: err}
	}

	if raw := resp.Header.Get("Latest-Ledger"); raw != "" {
		// A header that does not parse leaves latest at zero and is not an
		// error. The bytes are still evidence, and refusing them over a
		// malformed header would discard a reading to protect a label.
		if n, perr := strconv.ParseUint(raw, 10, 32); perr == nil {
			latest = uint32(n)
		}
	}
	return body, resp.StatusCode, latest, resp.Header.Get("Retry-After"), nil
}

// ---------------------------------------------------------------- Recording

// TickResult is the outcome for one pair in one schema 2 round.
type TickResult struct {
	Pair Pair
	Path string
	Tick RawTick

	// Written says the tick reached disk. It is the only thing that decides an
	// exit code: a tick recorded with a 429 in it is a success, and a tick that
	// could not be written is lost evidence.
	Written bool
	// Collided means the name derived from ledger_before was taken and a
	// monotonic suffix was appended. Nothing was overwritten either way.
	Collided bool
	// Err is set only when the tick could NOT be written.
	Err error
}

// RecordTicksOnce records every pair once, in the order the pair file lists
// them. One pair failing does not stop the rest, for the reason RecordOnce
// gives: a round that gives up on the first error records nothing on the day one
// asset is delisted.
func (r *Recorder) RecordTicksOnce(ctx context.Context) []TickResult {
	out := make([]TickResult, 0, len(r.cfg.Pairs))
	for _, p := range r.cfg.Pairs {
		if err := ctx.Err(); err != nil {
			out = append(out, TickResult{Pair: p, Err: err})
			return out
		}
		out = append(out, r.recordTick(ctx, p))
	}
	return out
}

func (r *Recorder) recordTick(ctx context.Context, p Pair) TickResult {
	res := TickResult{Pair: p}

	tick, err := r.cfg.Client.GetRawTick(ctx, p.Base, p.Quote)
	if err != nil {
		res.Err = err
		return res
	}
	res.Tick = tick

	// The date directory comes from the tick's own recorded_at rather than from
	// a second read of the clock. See decision 6. This cannot fail on a string
	// this package just formatted, and it is checked anyway, because the
	// alternative to checking is a silent zero-value year on a path.
	recordedAt, err := time.Parse(time.RFC3339, tick.RecordedAt)
	if err != nil {
		res.Err = fmt.Errorf("tick recorded_at %q: %w", tick.RecordedAt, err)
		return res
	}

	dir := filepath.Join(r.cfg.Root, p.Slug(), recordedAt.UTC().Format(tickDateLayout))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		res.Err = err
		return res
	}

	body, err := json.MarshalIndent(tick, "", "  ")
	if err != nil {
		res.Err = err
		return res
	}
	packed, err := gzipBytes(append(body, '\n'))
	if err != nil {
		res.Err = err
		return res
	}

	// ledger_before names the file. When it is zero, because the first request
	// carried no usable Latest-Ledger, the file is called 0.json.gz and says so;
	// that is a louder record of a bad tick than inventing a sequence would be.
	base := strconv.FormatUint(uint64(tick.LedgerBefore), 10)
	path, err := writeNewFile(dir, base, recordingSuffix, packed)
	if err != nil {
		res.Err = err
		return res
	}
	res.Path = path
	res.Written = true
	res.Collided = filepath.Base(path) != base+recordingSuffix
	return res
}

// writeNewFile writes body under {dir}/{base}{ext}, or under {base}-1{ext},
// {base}-2{ext} and so on when earlier names are taken. It NEVER overwrites.
// See decision 5 for why this links rather than renames.
func writeNewFile(dir, base, ext string, body []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".partial-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		// The write error is the one being returned. Close and Remove are
		// cleanup on a file that is already known to be bad.
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	// The temporary name is removed whichever way this goes. On success the
	// content survives under the linked name, because a hard link is another
	// name for the same inode and not a copy.
	defer func() { _ = os.Remove(name) }()

	for n := 0; n <= maxCollisionSuffix; n++ {
		candidate := filepath.Join(dir, base+ext)
		if n > 0 {
			candidate = filepath.Join(dir, base+"-"+strconv.Itoa(n)+ext)
		}
		err := os.Link(name, candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("%s: %s already has %d files and the suffix search gave up",
		dir, base, maxCollisionSuffix+1)
}

// ---------------------------------------------------------------- Reporting

// TickRound is the tally for one schema 2 round. It is returned rather than
// printed so that the caller deciding an exit code and the caller writing a
// build summary read the same numbers.
type TickRound struct {
	Written   int
	Degraded  int
	Collided  int
	Straddled int
	Unwritten int
}

// ReportTicks logs one round and tallies it. Exported for the reason Report is:
// two call sites formatting the same results their own way is how a log stops
// being comparable across runs.
func (r *Recorder) ReportTicks(results []TickResult) TickRound {
	var round TickRound
	for _, res := range results {
		if res.Err != nil {
			round.Unwritten++
			r.cfg.Logf("FAIL  %s: nothing written: %v", res.Pair, res.Err)
			continue
		}
		round.Written++
		if res.Collided {
			round.Collided++
		}
		if !res.Tick.LedgerConsistent {
			round.Straddled++
		}
		if res.Tick.Degraded() {
			round.Degraded++
		}
		r.cfg.Logf("write %s ledger %d->%d consistent=%t -> %s",
			res.Pair, res.Tick.LedgerBefore, res.Tick.LedgerAfter,
			res.Tick.LedgerConsistent, res.Path)

		// Every source that is not a 2xx is named individually. A round summary
		// saying "1 degraded" does not say WHICH endpoint refused, and the two
		// endpoints fail for different reasons.
		for _, s := range res.Tick.Sources {
			switch {
			case s.Error != "":
				r.cfg.Logf("  %s %s: no HTTP response: %s", res.Pair, s.Endpoint, s.Error)
			case s.HTTPStatus < 200 || s.HTTPStatus > 299:
				r.cfg.Logf("  %s %s: HTTP %d, recorded and kept", res.Pair, s.Endpoint, s.HTTPStatus)
			}
		}
	}
	r.cfg.Logf("round: %d written, %d degraded, %d straddled a ledger boundary, %d name collisions, "+
		"%d unwritten, %d requests this window",
		round.Written, round.Degraded, round.Straddled, round.Collided, round.Unwritten,
		r.cfg.Client.Requests())
	return round
}

// ---------------------------------------------------------------- Reading

// ReadTick reads one schema 2 recording back. ReadRecording still reads version
// 1 and is untouched; PeekSchemaVersion is how a caller holding an unknown file
// decides which of the two to call.
func ReadTick(path string) (RawTick, error) {
	var tick RawTick
	body, err := readRecordingBytes(path)
	if err != nil {
		return tick, err
	}
	if err := json.Unmarshal(body, &tick); err != nil {
		return tick, fmt.Errorf("%s: %w", path, err)
	}
	return tick, nil
}

// PeekSchemaVersion reports which schema a recording on disk is written in.
//
// A VERSION 1 FILE CARRIES NO schema_version FIELD, so its absence is read as 1
// rather than as a malformed file. That is the entire backward compatibility
// mechanism and it is worth being explicit about: the four recordings taken on
// 24 August 2026 answer 1 here, are read by ReadRecording, and are never
// rewritten.
func PeekSchemaVersion(path string) (int, error) {
	body, err := readRecordingBytes(path)
	if err != nil {
		return 0, err
	}
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(body, &head); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if head.SchemaVersion == 0 {
		return 1, nil
	}
	return head.SchemaVersion, nil
}

func readRecordingBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = zr.Close() }()

	body, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return body, nil
}

// ---------------------------------------------------------------- Pair naming

// PairsUsing names every pair in the list that carries a given asset, sorted.
//
// It exists so that a verification failure can say WHICH PAIR is broken rather
// than only which asset. An operator reading "verify AUDD:GDC7...: no such asset
// on Horizon" has to go and find the pair themselves; the point of failing
// loudly is that they do not have to.
func (r *Recorder) PairsUsing(a domain.Asset) []string {
	var out []string
	for _, p := range r.cfg.Pairs {
		if p.Base.Equal(a) || p.Quote.Equal(a) {
			out = append(out, p.String())
		}
	}
	sort.Strings(out)
	return out
}
