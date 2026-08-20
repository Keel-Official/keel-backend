//go:build conformance

// Uji kesesuaian metodologi terhadap golden fixture USTRY/USDC.
//
// KENAPA ADA BUILD TAG. Berkas ini mengimpor internal/depth, yang saat ini
// belum ada isinya. Tanpa tag, seluruh repo gagal build dan CI kehilangan
// makna. Tag ini bersifat SEMENTARA.
//
// SYARAT PENGHAPUSAN: begitu internal/depth berisi implementasi yang lolos,
// hapus baris //go:build di atas dan hapus target `conformance` yang terpisah
// di Makefile. Test ini harus jalan di `make test` biasa, bukan di jalur khusus
// yang mudah dilupakan.
//
// Jalankan sementara ini dengan: make conformance

package conformance

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/Keel-Official/keel/internal/depth"
	"github.com/Keel-Official/keel/internal/domain"
)

// eqDec membandingkan dalam ranah desimal, bukan float64, dan bukan lewat
// math.Abs. Versi test sebelumnya memakai keduanya, yang melanggar aturan
// paket yang sedang diujinya sendiri.
func eqDec(t *testing.T, label string, got, want decimal.Decimal) {
	t.Helper()
	if got.Sub(want).Abs().GreaterThan(Tolerance) {
		t.Errorf("%s = %s, mau %s (toleransi %s)", label, got, want, Tolerance)
	}
}

func mustRisk(t *testing.T) domain.AssetRisk {
	t.Helper()
	r, err := depth.ComputeAssetRisk(GoldenSnapshot(), DefaultParams())
	if err != nil {
		t.Fatalf("ComputeAssetRisk: %v", err)
	}
	return r
}

// ---------------------------------------------------------------- Harga

func TestMidPrice(t *testing.T) {
	got, src := depth.MidPrice(GoldenSnapshot())
	eqDec(t, "P0", got, ExpectedP0)
	if src != ExpectedPriceSource {
		t.Errorf("priceSource = %q, mau %q", src, ExpectedPriceSource)
	}
}

func TestSpreadPct(t *testing.T) {
	r := mustRisk(t)
	if r.SpreadPct == nil {
		t.Fatal("spreadPct nil, mau 196,0777141; nil berarti tidak diketahui dan kedua sisi buku terisi")
	}
	eqDec(t, "spreadPct", *r.SpreadPct, ExpectedSpreadPct)
}

// ---------------------------------------------------------------- Depth

func TestComputeDepth(t *testing.T) {
	p := DefaultParams()
	got, err := depth.ComputeDepth(GoldenSnapshot(), ExpectedP0, p.MarketDeltas)
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}
	if len(got) != len(ExpectedDepth) {
		t.Fatalf("jumlah baris depth = %d, mau %d", len(got), len(ExpectedDepth))
	}
	for i, want := range ExpectedDepth {
		g := got[i]
		eqDec(t, "delta["+want.Delta.String()+"]", g.Delta, want.Delta)
		eqDec(t, "depth("+want.Delta.String()+").BuySide  "+want.Reason, g.BuySide, want.BuySide)
		eqDec(t, "depth("+want.Delta.String()+").SellSide "+want.Reason, g.SellSide, want.SellSide)
		eqDec(t, "depth("+want.Delta.String()+").FromSdex", g.FromSdex, want.FromSdex)
		eqDec(t, "depth("+want.Delta.String()+").FromAmm (Pools kosong)", g.FromAmm, want.FromAmm)
	}
}

// ---------------------------------------------------------------- Manipulasi

func TestComputeManipulationCost(t *testing.T) {
	p := DefaultParams()
	got, err := depth.ComputeManipulationCost(GoldenSnapshot(), ExpectedP0, p.ManipulationDeltas)
	if err != nil {
		t.Fatalf("ComputeManipulationCost: %v", err)
	}
	if len(got) != len(ExpectedManipulation) {
		t.Fatalf("jumlah baris manipulationCost = %d, mau %d", len(got), len(ExpectedManipulation))
	}
	for i, want := range ExpectedManipulation {
		g := got[i]
		d := want.Delta.String()
		eqDec(t, "MC("+d+").Delta", g.Delta, want.Delta)
		eqDec(t, "MC("+d+").TargetPrice", g.TargetPrice, want.TargetPrice)
		eqDec(t, "MC("+d+").Cost", g.Cost, want.Cost)
		if g.Reachable != want.Reachable {
			t.Errorf("MC(%s).Reachable = %v, mau %v. Alasan: %s",
				d, g.Reachable, want.Reachable, want.Reason)
		}
	}
}

func TestMaxReachablePrice(t *testing.T) {
	r := mustRisk(t)
	if r.MaxReachablePrice == nil {
		t.Fatal("maxReachablePrice nil, mau 106,7372828; buku punya ask sehingga nilainya terdefinisi")
	}
	eqDec(t, "maxReachablePrice", *r.MaxReachablePrice, ExpectedMaxReachablePrice)

	if r.CostToMaxReachablePrice == nil {
		t.Fatal("costToMaxReachablePrice nil, mau 0")
	}
	eqDec(t, "costToMaxReachablePrice (gratis, tak ada ask lebih murah)",
		*r.CostToMaxReachablePrice, ExpectedCostToMaxReachablePrice)
}

// ---------------------------------------------------------------- Flag

func flagSet(fs []domain.Flag) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = string(f)
	}
	sort.Strings(out)
	return out
}

func compareFlags(t *testing.T, label string, got, want []domain.Flag) {
	t.Helper()
	g, w := flagSet(got), flagSet(want)
	if len(g) != len(w) {
		t.Errorf("%s = %v, mau %v", label, g, w)
		return
	}
	for i := range g {
		if g[i] != w[i] {
			t.Errorf("%s = %v, mau %v", label, g, w)
			return
		}
	}
}

func TestFlagsAndBand(t *testing.T) {
	r := mustRisk(t)
	compareFlags(t, "flags", r.Flags, ExpectedFlags)
	compareFlags(t, "unevaluatedFlags", r.UnevaluatedFlags, ExpectedUnevaluatedFlags)

	if r.Band != ExpectedBand {
		t.Errorf("band = %q, mau %q", r.Band, ExpectedBand)
	}
	if r.BandConfidence != ExpectedBandConfidence {
		t.Errorf("bandConfidence = %q, mau %q", r.BandConfidence, ExpectedBandConfidence)
	}
}

// TestClearFlagsTidakIkutUnevaluated menjaga pembedaan yang justru jadi alasan
// versi 1.0.2 ada: clear dan unevaluated tidak boleh tertukar.
func TestClearFlagsTidakIkutUnevaluated(t *testing.T) {
	r := mustRisk(t)
	for _, clear := range ExpectedClearFlags {
		for _, u := range r.UnevaluatedFlags {
			if u == clear {
				t.Errorf("flag %q dilaporkan unevaluated, padahal dapat diperiksa dan hasilnya clear", clear)
			}
		}
		for _, f := range r.Flags {
			if f == clear {
				t.Errorf("flag %q dilaporkan triggered, padahal seharusnya clear", clear)
			}
		}
	}
}

// ---------------------------------------------------------------- Invarian

// Invarian 1 dan 2 pada testdata/fixtures/ustry_pre_exploit.md.
func TestInvarianMonotonisitas(t *testing.T) {
	p := DefaultParams()

	d, err := depth.ComputeDepth(GoldenSnapshot(), ExpectedP0, p.MarketDeltas)
	if err != nil {
		t.Fatalf("ComputeDepth: %v", err)
	}
	for i := 1; i < len(d); i++ {
		if d[i].BuySide.LessThan(d[i-1].BuySide) {
			t.Errorf("depth sisi beli menurun dari delta %s ke %s: %s lalu %s",
				d[i-1].Delta, d[i].Delta, d[i-1].BuySide, d[i].BuySide)
		}
		if d[i].SellSide.LessThan(d[i-1].SellSide) {
			t.Errorf("depth sisi jual menurun dari delta %s ke %s: %s lalu %s",
				d[i-1].Delta, d[i].Delta, d[i-1].SellSide, d[i].SellSide)
		}
	}

	mc, err := depth.ComputeManipulationCost(GoldenSnapshot(), ExpectedP0, p.ManipulationDeltas)
	if err != nil {
		t.Fatalf("ComputeManipulationCost: %v", err)
	}
	for i := 1; i < len(mc); i++ {
		if mc[i].Cost.LessThan(mc[i-1].Cost) {
			t.Errorf("biaya manipulasi menurun dari delta %s ke %s: %s lalu %s",
				mc[i-1].Delta, mc[i].Delta, mc[i-1].Cost, mc[i].Cost)
		}
	}
}

// Invarian 3: maxReachablePrice sama persis dengan harga ask tertinggi di buku.
func TestInvarianMaxReachableAdalahAskTertinggi(t *testing.T) {
	s := GoldenSnapshot()
	if len(s.Book.Asks) == 0 {
		t.Skip("fixture tanpa ask")
	}
	tertinggi := s.Book.Asks[0].Price
	for _, a := range s.Book.Asks[1:] {
		if a.Price.Cmp(tertinggi) > 0 {
			tertinggi = a.Price
		}
	}
	r := mustRisk(t)
	if r.MaxReachablePrice == nil {
		t.Fatal("maxReachablePrice nil padahal buku punya ask")
	}
	eqDec(t, "maxReachablePrice vs ask tertinggi di buku",
		*r.MaxReachablePrice, tertinggi.Decimal())
}

// Invarian 5: NFR-9. Menjalankan perhitungan dua kali menghasilkan JSON
// identik byte per byte.
//
// Test ini murah dan menangkap pelanggaran larangan time.Now, math/rand, dan
// iterasi map tanpa sort secara otomatis, tanpa perlu membaca kodenya.
func TestInvarianDeterminisme(t *testing.T) {
	a, err := depth.ComputeAssetRisk(GoldenSnapshot(), DefaultParams())
	if err != nil {
		t.Fatalf("jalan pertama: %v", err)
	}
	b, err := depth.ComputeAssetRisk(GoldenSnapshot(), DefaultParams())
	if err != nil {
		t.Fatalf("jalan kedua: %v", err)
	}

	ja, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal jalan pertama: %v", err)
	}
	jb, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal jalan kedua: %v", err)
	}
	if string(ja) != string(jb) {
		t.Errorf("dua jalan menghasilkan JSON berbeda\npertama: %s\nkedua  : %s", ja, jb)
	}
}

// ---------------------------------------------------------------- Metadata

// Setiap keluaran wajib membawa LedgerSeq dan MethodologyVersion.
func TestMetadataWajib(t *testing.T) {
	r := mustRisk(t)
	s := GoldenSnapshot()

	if r.LedgerSeq != s.LedgerSeq {
		t.Errorf("ledgerSeq = %d, mau %d", r.LedgerSeq, s.LedgerSeq)
	}
	if r.MethodologyVersion != domain.MethodologyVersion {
		t.Errorf("methodologyVersion = %q, mau %q", r.MethodologyVersion, domain.MethodologyVersion)
	}
	if r.DataSource != s.Source {
		t.Errorf("dataSource = %q, mau %q", r.DataSource, s.Source)
	}
	if !r.LedgerClosedAt.Equal(s.LedgerClosedAt) {
		t.Errorf("ledgerClosedAt = %v, mau %v", r.LedgerClosedAt, s.LedgerClosedAt)
	}
}
