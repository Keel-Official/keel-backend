#!/usr/bin/env bash
#
# verifikasi-audit.sh
#
# Menjalankan ulang setiap klaim di docs/internal/audit-2026-08-20.md.
# Tujuannya membuat audit itu bisa dibantah, bukan dipercaya.
#
# Setiap baris TERBUKTI berarti klaimnya masih benar apa adanya di repo ini.
# Setiap baris TIDAK TERBUKTI berarti klaimnya salah, atau sudah diperbaiki.
# Keduanya berguna. Setelah sebuah temuan dibereskan, barisnya WAJIB berubah
# menjadi TIDAK TERBUKTI, dan itulah tanda pekerjaan selesai.
#
# Pakai: bash scripts/verifikasi-audit.sh
# Kode keluar: 0 selalu. Berkas ini melaporkan, tidak menghakimi.

cd "$(dirname "$0")/.." || exit 1

hijau=$'\033[32m'; merah=$'\033[31m'; abu=$'\033[90m'; tebal=$'\033[1m'; nol=$'\033[0m'
terbukti=0; tidak=0

cek() {
  local id="$1" ket="$2"; shift 2
  if "$@" >/dev/null 2>&1; then
    printf "%s%-7s TERBUKTI%s  %s\n" "$hijau" "$id" "$nol" "$ket"
    terbukti=$((terbukti + 1))
  else
    printf "%s%-7s TIDAK   %s  %s\n" "$merah" "$id" "$nol" "$ket"
    tidak=$((tidak + 1))
  fi
}

bagian() { printf "\n%s%s%s\n" "$tebal" "$1" "$nol"; }

tanpa_commit()   { ! git rev-parse HEAD; }
depth_kosong()   { [ -z "$(ls internal/depth/*.go 2>/dev/null)" ]; }
conformance_mati() { ! go vet -tags conformance ./internal/conformance/; }
adapter_mati()   { ! grep -rq "internal/adapter" --include='*.go' .; }
metrics_hilang() { ! grep -riq "create table.*metrics" migrations/; }
kontrak_tanpa()  { ! grep -qE "^ +$1:" docs/api/keel-openapi.yaml; }
ikhtisar_hilang(){ [ ! -f docs/methodology/00-ikhtisar.md ]; }
learning_hilang(){ [ ! -d docs/learning ]; }
api_kosong()     { [ -z "$(ls -A api 2>/dev/null)" ]; }
spike_satu_hal() { [ "$(grep -c ledger_close_time docs/evidences/spike_results_1.txt)" = 200 ]; }

bagian "P0  Yang menghalangi segalanya"
cek P0-1 "Belum ada satu commit pun, padahal remote origin sudah terpasang" tanpa_commit
cek P0-2 "internal/depth tidak punya satu berkas .go" depth_kosong
cek P0-3 "Uji conformance gagal build, jadi golden fixture belum menguji apa pun" conformance_mati

bagian "P1  Spesifikasi bercabang"
cek P1-1 "TDD menyatakan snapshot mentah tidak disimpan di DB" \
  grep -q "Snapshot mentah tidak disimpan di database" docs/architecture/Keel_Technical_Design_Document.md
cek P1-2 "tetapi 0001_snapshots.sql justru menyimpan kolom raw JSONB" \
  grep -q "raw                 JSONB" migrations/0001_snapshots.sql
cek P1-3 "dan tabel metrics yang dibaca API tidak ada di migrations" metrics_hilang
cek P1-4 "CHECK source hanya menerima horizon dan hubble" \
  grep -qF "source IN ('horizon', 'hubble')" migrations/0001_snapshots.sql
cek P1-5 "padahal domain sudah punya DataSourceTradesImplied" \
  grep -q "DataSourceTradesImplied" internal/domain/types.go
cek P1-6 "types.go menyimpan OracleResistance sebagai skalar" \
  grep -qF "OracleResistance        *decimal.Decimal" internal/domain/types.go
cek P1-7 "padahal DEC-003 menolak bentuk skalar secara eksplisit" \
  grep -q "Alternatif yang ditolak:\*\* satu skalar hasil bagi" docs/decisions/DEC-003-api-contract-v1-1.md
cek P1-8 "CostToMaxReachablePrice ada di kode" \
  grep -q "CostToMaxReachablePrice" internal/domain/types.go
cek P1-9 "tetapi tidak ada di kontrak API" kontrak_tanpa costToMaxReachablePrice
cek P1-10 "unevaluatedFlags tidak ada sebagai field kontrak" kontrak_tanpa unevaluatedFlags
cek P1-11 "bandConfidence tidak ada sebagai field kontrak" kontrak_tanpa bandConfidence
cek P1-12 "padahal 09-flag-dan-band mewajibkan keduanya masuk openapi" \
  grep -q "tambah .unevaluatedFlags., .bandConfidence." docs/methodology/09-flag-dan-band.md
cek P1-13 "kontrak memakai criticalDelta 0.5" \
  grep -q "criticalDelta: 0.5" docs/api/keel-openapi.yaml
cek P1-14 "sedangkan DefaultParams memakai delta kritis 1.0" \
  grep -qF 'ManipulationCriticalDelta: dec("1.0")' internal/conformance/fixture.go
cek P1-15 "metodologi mewajibkan kedua suku C_max dilaporkan, bukan hanya minimumnya" \
  grep -q "keduanya harus dilaporkan, bukan hanya" docs/methodology/keel-methodology-core.md
cek P1-16 "internal/adapter memakai float64" grep -q "float64" internal/adapter/horizon.go
cek P1-17 "arch_test hanya memindai internal/domain dan internal/depth" \
  grep -qF 'paketMurni = []string{".", "../depth"}' internal/domain/arch_test.go
cek P1-18 "internal/adapter tidak diimpor paket mana pun" adapter_mati
cek P1-19 "internal/adapter tidak muncul di peta zona CLAUDE.md" \
  bash -c '! grep -q "internal/adapter" CLAUDE.md'
cek P1-20 "DEC-001 masih memakai kabar 0,50 dolar dan rasio 1 banding 22 juta" \
  grep -q "1 berbanding 22 juta" docs/decisions/DEC-001-ustry-identity.md
cek P1-21 "padahal bukti di repo menunjukkan trade eksekusi 5,3475699 USDC" \
  grep -qF '"base_amount": "5.3475699"' docs/evidences/spike_result_2.txt
cek P1-22 "curl DEC-002 memberi USTRY tipe credit_alphanum4" \
  grep -q "credit_alphanum4&base_asset_code=USTRY" docs/decisions/DEC-002-hold-bigquery.md
cek P1-23 "curl DEC-001 melakukan kesalahan yang sama" \
  grep -q "credit_alphanum4&base_asset_code=USTRY" docs/decisions/DEC-001-ustry-identity.md
cek P1-24 "padahal bukti menyatakan USTRY bertipe credit_alphanum12" \
  grep -qF '"counter_asset_type": "credit_alphanum12"' docs/evidences/spike_result_2.txt
cek P1-25 "bukti spike berhenti tepat di 200 record, yaitu satu halaman Horizon" spike_satu_hal
cek P1-26 "DEC-003 masih menulis MC delta 1 sebagai reachable true" \
  grep -qF '`130.0627093`, `true`' docs/decisions/DEC-003-api-contract-v1-1.md
cek P1-27 "padahal fixture dan kontrak sudah mengoreksinya menjadi false" \
  grep -q "Entri delta 1.0 sebelumnya" docs/api/keel-openapi.yaml
cek P1-28 "documentUrl menunjuk org ciganytry, bukan Keel-Official" \
  grep -q "github.com/ciganytry/keel" docs/api/keel-openapi.yaml
cek P1-29 "dan menunjuk 00-ikhtisar.md yang tidak ada" ikhtisar_hilang
cek P1-30 "assetBrokenBook memakai ledgerClosedAt 2026-02-21T23:39:00Z" \
  grep -q "2026-02-21T23:39:00Z" docs/api/keel-openapi.yaml
cek P1-31 "padahal fixture dan bukti menyatakan 2026-02-22T00:10:21Z" \
  grep -q "2026-02-22T00:10:21Z" testdata/fixtures/ustry_pre_exploit.md
cek P1-32 "GoldenSnapshot menandai dirinya horizon" \
  grep -q "Source: domain.DataSourceHorizon" internal/conformance/fixture.go
cek P1-33 "padahal kontrak menandai buku yang sama trades-implied" \
  grep -q "dataSource: trades-implied" docs/api/keel-openapi.yaml

bagian "P2  Kebersihan yang murah"
cek P2-1 "CLAUDE.md memuat paksa keel-openapi.yaml ke konteks setiap sesi" \
  grep -qF "@docs/api/keel-openapi.yaml" CLAUDE.md
cek P2-2 "README menunjuk docs/learning yang tidak ada" \
  bash -c 'grep -q "docs/learning" README.md' 
cek P2-3 "dan direktori itu memang tidak ada" learning_hilang
cek P2-4 "README menjanjikan make record jalan, padahal keluar dengan kode 3" \
  grep -q "belumSiap = 3" cmd/keel/main.go
cek P2-5 "direktori api di akar repo kosong dan tidak disebut dokumen mana pun" api_kosong
cek P2-6 "kunci zona merah hanya menutup Edit dan Write, tidak Bash" \
  bash -c 'grep -q "Write(internal/depth/\*\*)" .claude/settings.json && ! grep -q "Bash(sed" .claude/settings.json'
cek P2-7 "keputusan struktur berkas metodologi masih terbuka di README metodologi" \
  grep -q "Keputusan yang harus diambil" docs/methodology/README.md
cek P2-8 "fixture menulis Keempatnya untuk daftar yang berisi enam flag" \
  grep -q "Keempatnya harus dilaporkan" testdata/fixtures/ustry_pre_exploit.md

bagian "Aritmetika golden fixture, dihitung ulang dari nol"
python3 - <<'PY'
from decimal import Decimal, getcontext
getcontext().prec = 60
ask = Decimal(266843207) / Decimal(2500000)
bid = Decimal(1057) / Decimal(1000)
amt = Decimal("1.2185312")
p0 = (ask + bid) / 2
spread = (ask - bid) / p0 * 100
cost = ask * amt
def bandingkan(nama, dihitung, tertulis, tol=Decimal("0.0000001")):
    ok = abs(dihitung - Decimal(tertulis)) <= tol
    tanda = "\033[32mCOCOK \033[0m" if ok else "\033[31mBEDA  \033[0m"
    print(f"{tanda} {nama:<26} hitung {dihitung:<24} fixture {tertulis}")
bandingkan("P0", p0, "53.8971414")
bandingkan("spreadPct", spread, "196.0777140585048")
bandingkan("cost ask tunggal", cost, "130.06270929502336")
bandingkan("target delta 0,5", p0 * Decimal("1.5"), "80.8457121")
bandingkan("target delta 1", p0 * 2, "107.7942828")
bandingkan("target delta 10", p0 * 11, "592.8685554")
bandingkan("target delta 100", p0 * 101, "5443.6112814")
bandingkan("maxReachablePrice", ask, "106.7372828")
print(f"       target beli 2/5/10 persen  {p0*Decimal('1.02')} {p0*Decimal('1.05')} {p0*Decimal('1.10')}")
print(f"       target jual 2/5/10 persen  {p0*Decimal('0.98')} {p0*Decimal('0.95')} {p0*Decimal('0.90')}")
print("       satu-satunya ask 106,7372828 di luar seluruh target beli, satu-satunya bid 1,057 di luar seluruh target jual")
print("       karena itu depth nol pada enam sel, dan nol itu benar")
PY

bagian "Biaya manipulasi yang benar-benar dibayar penyerang"
printf "%s" "$abu"
grep -A3 '"ledger_close_time": "2026-02-22T00:10:21Z"' docs/evidences/spike_result_final.txt \
  | grep -E 'base_amount|counter_amount' || true
printf "%s" "$nol"
echo "       5,3475699 USDC ditukar 0,0501003 USTRY pada 106,7372828. Ini angka utama Deliverable 2."
echo "       Terhadap sekitar 10,97 juta dolar yang dipinjam, rasionya sekitar 1 banding 2,05 juta."
echo "       Bukan 1 banding 22 juta seperti tertulis di DEC-001."

bagian "Ringkasan"
printf "  %s%d klaim terbukti%s, %s%d tidak terbukti%s\n" "$hijau" "$terbukti" "$nol" "$merah" "$tidak" "$nol"
echo "  Audit lengkap: docs/internal/audit-2026-08-20.md"
