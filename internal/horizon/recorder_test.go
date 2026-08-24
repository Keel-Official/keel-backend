package horizon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Keel-Official/keel-backend/internal/domain"
)

func testRecorder(t *testing.T, f *fakeHorizon, pairs ...Pair) (*Recorder, string) {
	t.Helper()
	if len(pairs) == 0 {
		pairs = []Pair{{Base: testUSTRY, Quote: testUSDC}}
	}
	c, _ := f.client()
	root := t.TempDir()
	r, err := NewRecorder(RecorderConfig{Client: c, Root: root, Pairs: pairs})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	return r, root
}

func TestRecordOnceWritesOneFilePerLedger(t *testing.T) {
	f := newFakeHorizon(t)
	r, root := testRecorder(t, f)

	results := r.RecordOnce(context.Background())
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	res := results[0]
	if res.Err != nil {
		t.Fatalf("record: %v", res.Err)
	}
	if res.Skipped {
		t.Error("the first recording of a ledger reported itself as a skip")
	}

	want := filepath.Join(root,
		"USTRY.GCRYUGD5-USDC.GA5ZSEJY", "61340263.json.gz")
	if res.Path != want {
		t.Errorf("path = %s, want %s", res.Path, want)
	}

	// Read through the package's own reader, which is what whatever compares a
	// recording against reconstructed history will use.
	raw, err := ReadRecording(res.Path)
	if err != nil {
		t.Fatalf("read the recording: %v", err)
	}

	// And prove it really is gzip rather than trusting the extension. The first
	// two bytes of a gzip member are fixed.
	onDisk, err := os.ReadFile(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk) < 2 || onDisk[0] != 0x1f || onDisk[1] != 0x8b {
		t.Error("the file does not start with the gzip magic bytes")
	}
	// And that compressing was worth doing. Measured on the three live
	// recordings taken while writing this: 9292 bytes of JSON became 2278, about
	// four to one. The fake's book is smaller than a real one, so the bar here is
	// only that the compressed form is smaller at all.
	plain, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(onDisk) >= len(plain) {
		t.Errorf("the file is %d bytes and its own JSON is %d; compression cost space",
			len(onDisk), len(plain))
	}
	if raw.LedgerSeq != 61340263 {
		t.Errorf("recorded ledger = %d, want 61340263", raw.LedgerSeq)
	}
	if !strings.Contains(string(raw.OrderBook), "266843207") {
		t.Error("the recording lost the raw price_r fraction")
	}
	if raw.RequestedBase != testUSTRY.String() || raw.RequestedQuote != testUSDC.String() {
		t.Errorf("the recording does not name its own pair: %s / %s", raw.RequestedBase, raw.RequestedQuote)
	}
}

// The same rule docs/evidences carries: an edited piece of evidence is not
// evidence. A second round inside one ledger must leave the file untouched.
func TestRecordOnceNeverOverwrites(t *testing.T) {
	f := newFakeHorizon(t)
	r, _ := testRecorder(t, f)

	first := r.RecordOnce(context.Background())[0]
	if first.Err != nil {
		t.Fatalf("first round: %v", first.Err)
	}
	sentinel := []byte("this file is evidence and must not be rewritten\n")
	if err := os.WriteFile(first.Path, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}

	second := r.RecordOnce(context.Background())[0]
	if second.Err != nil {
		t.Fatalf("second round: %v", second.Err)
	}
	if !second.Skipped {
		t.Error("the second round did not report a skip")
	}
	got, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sentinel) {
		t.Error("an existing recording was overwritten")
	}
}

// One failing pair must not cost the round. On the day one asset is delisted, a
// recorder that gives up on the first error records nothing at all.
func TestRecordOnceContinuesPastAFailure(t *testing.T) {
	f := newFakeHorizon(t)
	// XLM against USDC. The fake echoes the USTRY pair back whatever it is
	// asked for, so this pair fails the echo check while the healthy one does
	// not, which is the arrangement under test.
	broken := Pair{Base: domain.Asset{Type: domain.AssetTypeNative}, Quote: testUSDC}
	r, _ := testRecorder(t, f, broken, Pair{Base: testUSTRY, Quote: testUSDC})

	results := r.RecordOnce(context.Background())
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Err == nil {
		t.Error("the unknown pair was recorded without complaint")
	}
	if results[1].Err != nil {
		t.Errorf("the healthy pair failed because another pair did: %v", results[1].Err)
	}
	if results[1].Path == "" {
		t.Error("the healthy pair produced no file")
	}
}

func TestRecorderVerifiesEveryDistinctAssetOnce(t *testing.T) {
	f := newFakeHorizon(t)
	f.handler["/assets"] = func(w http.ResponseWriter, req *http.Request) {
		code := req.URL.Query().Get("asset_code")
		typ := "credit_alphanum4"
		if code == "USTRY" {
			typ = "credit_alphanum12"
		}
		w.Write([]byte(`{"_embedded":{"records":[{"asset_type":"` + typ + `","asset_code":"` + code +
			`","asset_issuer":"` + req.URL.Query().Get("asset_issuer") + `"}]}}`))
	}
	// The same quote asset twice, so the deduplication is what is under test.
	r, _ := testRecorder(t, f,
		Pair{Base: testUSTRY, Quote: testUSDC},
		Pair{Base: testUSTRY, Quote: testUSDC})

	if err := r.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if f.hits["/assets"] != 2 {
		t.Errorf("/assets hit %d times, want 2 for two distinct assets", f.hits["/assets"])
	}
}

func TestNewRecorderRefusesAnEmptyPairList(t *testing.T) {
	f := newFakeHorizon(t)
	c, _ := f.client()
	_, err := NewRecorder(RecorderConfig{Client: c, Root: t.TempDir()})
	if err == nil {
		t.Fatal("a recorder with no pairs was accepted")
	}
}

// ---------------------------------------------------------------- Pair file

func writePairs(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pairs.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPairsReadsAValidFile(t *testing.T) {
	path := writePairs(t, `{
	  "note": "one pair",
	  "pairs": [
	    {"base":  {"code":"USTRY","issuer":"`+testUSTRY.Issuer+`","type":"credit_alphanum12"},
	     "quote": {"code":"USDC","issuer":"`+testUSDC.Issuer+`","type":"credit_alphanum4"}},
	    {"base":  {"type":"native"},
	     "quote": {"code":"USDC","issuer":"`+testUSDC.Issuer+`","type":"credit_alphanum4"}}
	  ]}`)

	pairs, err := LoadPairs(path)
	if err != nil {
		t.Fatalf("LoadPairs: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2", len(pairs))
	}
	if !pairs[0].Base.Equal(testUSTRY) {
		t.Errorf("base = %s, want %s", pairs[0].Base, testUSTRY)
	}
	if pairs[1].Slug() != "XLM.native-USDC.GA5ZSEJY" {
		t.Errorf("native slug = %s", pairs[1].Slug())
	}
}

func TestLoadPairsRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"an asset type that does not exist": `{"pairs":[{"base":{"code":"A","issuer":"G1","type":"alphanum4"},"quote":{"type":"native"}}]}`,
		"a credit asset with no issuer":     `{"pairs":[{"base":{"code":"A","type":"credit_alphanum4"},"quote":{"type":"native"}}]}`,
		"a native asset carrying a code":    `{"pairs":[{"base":{"code":"XLM","type":"native"},"quote":{"code":"A","issuer":"G1","type":"credit_alphanum4"}}]}`,
		"the same asset on both sides":      `{"pairs":[{"base":{"type":"native"},"quote":{"type":"native"}}]}`,
		"no pairs at all":                   `{"note":"empty"}`,
		"an unknown field, likely a typo":   `{"pairs":[{"base":{"type":"native"},"quote":{"type":"native"},"delta":1}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadPairs(writePairs(t, body)); err == nil {
				t.Errorf("accepted %s", name)
			}
		})
	}
}

func TestLoadPairsRejectsADuplicate(t *testing.T) {
	one := `{"base":{"code":"USTRY","issuer":"` + testUSTRY.Issuer + `","type":"credit_alphanum12"},` +
		`"quote":{"code":"USDC","issuer":"` + testUSDC.Issuer + `","type":"credit_alphanum4"}}`
	_, err := LoadPairs(writePairs(t, `{"pairs":[`+one+`,`+one+`]}`))
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("error = %v, want a complaint about the duplicate", err)
	}
}

func TestLoadPairsReportsAMissingFile(t *testing.T) {
	_, err := LoadPairs(filepath.Join(t.TempDir(), "absent.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

// The example file shipped in scripts/ has to stay loadable, because the
// Makefile's record target points at it and README tells a newcomer to copy it.
func TestTheShippedExamplePairFileLoads(t *testing.T) {
	pairs, err := LoadPairs("../../scripts/record-pairs.example.json")
	if err != nil {
		t.Fatalf("the shipped example does not load: %v", err)
	}
	if len(pairs) == 0 {
		t.Fatal("the shipped example holds no pairs")
	}
}

// ---------------------------------------------------------------- Holders

// holderRecorder is testRecorder with the holder reading turned on. It is a
// separate helper rather than a parameter because every existing recorder test
// asserts that nothing but pair snapshots gets written, and that assertion is
// worth keeping exactly as it is.
func holderRecorder(t *testing.T, f *fakeHorizon, pairs ...Pair) (*Recorder, string) {
	t.Helper()
	if len(pairs) == 0 {
		pairs = []Pair{{Base: testUSTRY, Quote: testUSDC}}
	}
	c, _ := f.client()
	root := t.TempDir()
	r, err := NewRecorder(RecorderConfig{Client: c, Root: root, Pairs: pairs, Holders: true})
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	return r, root
}

func TestRecordHoldersWritesOneFilePerAssetPerLedger(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", 2, true), []string{accountsPage(0, 2, "500.0000000")})
	r, root := holderRecorder(t, f)

	results := r.RecordHoldersOnce(context.Background())
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: only the BASE asset is read", len(results))
	}
	res := results[0]
	if res.Err != nil {
		t.Fatalf("recordHolders: %v", res.Err)
	}
	want := filepath.Join(root, "holders", "USTRY.GCRYUGD5", "61340263.json.gz")
	if res.Path != want {
		t.Errorf("path = %s, want %s", res.Path, want)
	}
	if res.Holders != 2 {
		t.Errorf("Holders = %d, want 2", res.Holders)
	}

	raw, err := ReadHolderRecording(res.Path)
	if err != nil {
		t.Fatalf("ReadHolderRecording: %v", err)
	}
	if raw.RequestedAsset != testUSTRY.String() {
		t.Errorf("RequestedAsset = %q, want %q", raw.RequestedAsset, testUSTRY.String())
	}
	if raw.HoldersRead != 2 || raw.HolderCount != 2 {
		t.Errorf("HoldersRead = %d and HolderCount = %d, want 2 and 2", raw.HoldersRead, raw.HolderCount)
	}
	if len(raw.Accounts) != 1 || len(raw.AssetSummary) == 0 {
		t.Error("the raw bytes did not survive the round trip, which is the point of the file")
	}
	if raw.MethodologyVersion != domain.MethodologyVersion {
		t.Errorf("MethodologyVersion = %q, want %q", raw.MethodologyVersion, domain.MethodologyVersion)
	}
}

// The quote asset is NOT read. USDC has hundreds of thousands of trustlines and
// nothing in the methodology asks about them. See decision 5.
func TestRecordHoldersReadsOnlyBaseAssets(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", 1, true), []string{accountsPage(0, 1, "1.0000000")})
	r, _ := holderRecorder(t, f, Pair{Base: testUSTRY, Quote: testUSDC}, Pair{Base: testUSTRY, Quote: domain.Asset{Type: domain.AssetTypeNative}})

	assets := r.HolderAssets()
	if len(assets) != 1 || !assets[0].Equal(testUSTRY) {
		t.Fatalf("HolderAssets = %v, want only %s", assets, testUSTRY)
	}
}

// A native BASE asset is skipped rather than failed: XLM has no trustlines, and
// a pair list holding XLM/USDC is legitimate.
func TestRecordHoldersSkipsANativeBaseAsset(t *testing.T) {
	f := newFakeHorizon(t)
	r, _ := holderRecorder(t, f, Pair{Base: domain.Asset{Type: domain.AssetTypeNative}, Quote: testUSDC})

	if got := r.HolderAssets(); len(got) != 0 {
		t.Fatalf("HolderAssets = %v, want none", got)
	}
	if results := r.RecordHoldersOnce(context.Background()); len(results) != 0 {
		t.Fatalf("got %d results, want none", len(results))
	}
}

func TestRecordHoldersNeverOverwrites(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", 1, true), []string{accountsPage(0, 1, "1.0000000")})
	r, _ := holderRecorder(t, f)

	first := r.RecordHoldersOnce(context.Background())
	if first[0].Err != nil || first[0].Skipped {
		t.Fatalf("first round: err=%v skipped=%t", first[0].Err, first[0].Skipped)
	}
	second := r.RecordHoldersOnce(context.Background())
	if second[0].Err != nil {
		t.Fatalf("second round: %v", second[0].Err)
	}
	if !second[0].Skipped {
		t.Error("the same ledger was recorded twice, so an existing recording was overwritten")
	}
}

// Off by default, and nil rather than empty, so a caller can tell "not asked
// for" apart from "nothing to do".
func TestRecordHoldersIsOffByDefault(t *testing.T) {
	f := newFakeHorizon(t)
	f.withHolders(assetSummary("1000.0000000", 1, true), []string{accountsPage(0, 1, "1.0000000")})
	r, root := testRecorder(t, f)

	if results := r.RecordHoldersOnce(context.Background()); results != nil {
		t.Fatalf("got %v, want nil when holder recording is off", results)
	}
	if f.hits["/accounts"] != 0 {
		t.Error("a holder request was made although holder recording is off")
	}
	if _, err := os.Stat(filepath.Join(root, "holders")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a holders directory was created although holder recording is off")
	}
}

// dueForHolders is the whole of the holder cadence rule, extracted from Run so it
// can be tested without a ticker. The cases that matter are the two ends: zero
// interval must behave exactly as the code did before the interval existed, and a
// first round must always be due, because a holder reading missed is a reading
// that cannot be taken later.
func TestDueForHolders(t *testing.T) {
	base := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		last     time.Time
		now      time.Time
		interval time.Duration
		want     bool
	}{
		{"zero interval records every round", base, base, 0, true},
		{"negative interval is treated as zero", base, base, -time.Hour, true},
		{"the first round is always due", time.Time{}, base, 6 * time.Hour, true},
		{"not due before the interval has passed", base, base.Add(5 * time.Hour), 6 * time.Hour, false},
		{"due exactly on the interval", base, base.Add(6 * time.Hour), 6 * time.Hour, true},
		{"due after the interval", base, base.Add(7 * time.Hour), 6 * time.Hour, true},
		{"a stamp in the future is not due", base, base.Add(-time.Hour), 6 * time.Hour, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dueForHolders(c.last, c.now, c.interval); got != c.want {
				t.Errorf("dueForHolders(%s, %s, %s) = %t, want %t",
					c.last, c.now, c.interval, got, c.want)
			}
		})
	}
}
