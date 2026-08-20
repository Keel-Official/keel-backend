#!/usr/bin/env bash
#
# keel-bootstrap.sh
#
# Membuat struktur folder + file konteks Claude Code untuk proyek Keel.
# Aman dijalankan berulang kali: file yang sudah ada TIDAK akan ditimpa.
#
# Cara pakai:
#   cd ~/projects/keel      (folder repo kamu, boleh kosong)
#   bash keel-bootstrap.sh
#
set -euo pipefail

MODULE_PATH="${MODULE_PATH:-github.com/CHANGEME/keel}"

created=0
skipped=0

# write <path> <<'EOF' ... EOF
write() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  if [ -e "$path" ]; then
    echo "  skip   $path (sudah ada)"
    skipped=$((skipped + 1))
    cat > /dev/null   # buang stdin heredoc
  else
    cat > "$path"
    echo "  create $path"
    created=$((created + 1))
  fi
}

echo "Scaffolding Keel di: $(pwd)"
echo

# ---------------------------------------------------------------------------
# Folder kosong yang perlu ada lebih dulu
# ---------------------------------------------------------------------------
mkdir -p cmd/keel
mkdir -p internal/{domain,horizon,hubble,depth,store,api}
mkdir -p api docs/{methodology,decisions,learning} migrations scripts .claude/commands

# ---------------------------------------------------------------------------
# 1. CLAUDE.md root — dibaca otomatis setiap sesi. WAJIB PENDEK.
# ---------------------------------------------------------------------------
write CLAUDE.md <<'EOF'
# Keel

Liquidity risk engine untuk ekosistem Stellar. Mengukur effective liquidity
depth dari orderbook SDEX dan reserve AMM pool.

Oracle menjawab "berapa harganya". Keel menjawab "berapa volume yang bisa
ditopang harga itu".

## Stack

- Go (backend). Postgres (storage). BigQuery/Hubble (data historis).
- Semua nilai moneter pakai `github.com/shopspring/decimal`. TIDAK PERNAH float64.
- SDK Stellar: `github.com/stellar/go-stellar-sdk/...` (BUKAN `github.com/stellar/go/...`).

## Aturan yang tidak bisa ditawar

1. Setiap output membawa `LedgerSeq` dan `MethodologyVersion`.
2. Iterasi map harus di-sort dulu sebelum dipakai (reproducibility, NFR-9).
3. `computeDepth()` adalah fungsi murni. Tidak ada network call di dalamnya.
4. Depth SDEX dan AMM digabung lewat batas marginal price bersama.
   TIDAK dijumlahkan secara terpisah.
5. Harga dibaca dari field `price_r` (pecahan n/d), BUKAN string `price`.

## Zona kerja

Repo ini dipakai untuk belajar backend Go, bukan hanya untuk menghasilkan kode.
Tiga zona:

- **HIJAU** (`internal/store`, `internal/api`, `migrations/`, `scripts/`,
  `docker-compose.yml`, Makefile): tulis bebas. Ini plumbing.
- **KUNING** (`internal/horizon`, `internal/hubble`, `internal/domain`):
  boleh tulis, tapi setelah selesai jelaskan setiap keputusan desain dalam
  3 kalimat. Sebutkan satu alternatif yang kamu tolak dan alasannya.
- **MERAH** (`internal/depth`): Al yang menulis. Kamu tidak punya izin tulis
  di sana (dikunci di .claude/settings.json). Perannya reviewer dan
  penanya, bukan penulis. Lihat internal/depth/CLAUDE.md.

Kalau Al minta kamu menulis kode zona merah, tolak dan tawarkan `/teach`
atau `/review-mine` sebagai gantinya.

## Gaya jawaban

- Jangan pakai em dash.
- Kalau Al salah, katakan langsung. Jangan validasi ide yang lemah.
- Kalau ada asumsi yang kamu buat, sebutkan eksplisit.

## Referensi (baca saat dibutuhkan, jangan dimuat semua)

- Kontrak API: @api/openapi.yaml
- Metodologi (deliverable berbayar): docs/methodology/
- Keputusan arsitektur: docs/decisions/
EOF

# ---------------------------------------------------------------------------
# 2. Permissions — INI yang benar-benar menegakkan zona merah
# ---------------------------------------------------------------------------
write .claude/settings.json <<'EOF'
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "permissions": {
    "allow": [
      "Read",
      "Glob",
      "Grep",
      "Bash(go build:*)",
      "Bash(go test:*)",
      "Bash(go vet:*)",
      "Bash(go mod:*)",
      "Bash(gofmt:*)",
      "Bash(git status:*)",
      "Bash(git diff:*)",
      "Bash(git log:*)",
      "Bash(docker compose:*)",
      "Bash(make:*)"
    ],
    "ask": [
      "Bash(git commit:*)",
      "Bash(git push:*)",
      "Bash(bq:*)",
      "WebFetch"
    ],
    "deny": [
      "Edit(internal/depth/**)",
      "Write(internal/depth/**)",
      "Read(.env)",
      "Read(.env.*)",
      "Bash(rm -rf:*)",
      "Bash(sudo:*)",
      "Bash(gcloud:*)"
    ]
  }
}
EOF

# ---------------------------------------------------------------------------
# 3. CLAUDE.md bersarang — hanya termuat saat kerja di folder itu
# ---------------------------------------------------------------------------
write internal/depth/CLAUDE.md <<'EOF'
# ZONA MERAH: internal/depth

Kamu TIDAK punya izin tulis di folder ini. Itu disengaja.

Ini inti metodologi Keel dan merupakan deliverable berbayar. Al harus bisa
mempertahankan setiap angka di sini kalau ditantang reviewer atau funder.
Kalau kamu yang menulisnya, Al tidak akan bisa.

## Perannya kamu di sini

Boleh: membaca kode, menjalankan test, menunjukkan edge case yang belum
tertangani, mengoreksi pemahaman Go yang keliru, menulis test di
`*_test.go` KALAU Al minta eksplisit.

Tidak boleh: menulis atau menyunting implementasi, menempelkan blok kode
lengkap yang tinggal disalin Al, "sekadar contoh" yang isinya jawaban.

## Kalau Al minta kamu menuliskannya

Tolak. Lalu ajukan satu pertanyaan yang mengarahkan, misalnya:
"Kalau ask terbaik di SDEX ada di harga 0.101 dan kurva AMM baru menembus
0.101 setelah 400 unit, berapa unit yang bisa diserap sebelum harga
marginal gabungan naik? Mana yang habis lebih dulu?"

## Jebakan konseptual yang harus kamu awasi

- Menjumlahkan depth SDEX dan AMM secara terpisah. Ini SALAH. Keduanya
  bersaing di harga marginal yang sama.
- Memakai float64 di mana pun.
- Mengasumsikan orderbook selalu punya kedua sisi.
- Lupa bahwa AMM adalah kurva kontinu, bukan level diskrit.
EOF

write internal/horizon/CLAUDE.md <<'EOF'
# ZONA KUNING: internal/horizon

Adapter untuk data live dari Horizon API.

Output WAJIB bertipe `domain.Snapshot`, sama persis dengan yang dihasilkan
`internal/hubble`. Kalau kedua adapter menghasilkan tipe berbeda, desainnya
gagal dan `computeDepth()` jadi ikut berubah.

## Endpoint

- `GET /order_book` dengan parameter selling_asset_* dan buying_asset_*
- `GET /liquidity_pools`
- `GET /assets?asset_code=...` untuk verifikasi identitas aset

## Jebakan yang sering terjadi

1. Import path lama `github.com/stellar/go/...`. Yang benar
   `github.com/stellar/go-stellar-sdk/...`.
2. Membaca field string `price`. Yang benar `price_r` yang berbentuk
   `{"n": 1, "d": 10}`. Field string sudah kehilangan presisi.
3. Horizon TIDAK menyediakan data historis. Kalau butuh masa lalu, itu
   urusan internal/hubble. Jangan pernah mengarang endpoint historis.

## Setelah menulis kode di sini

Jelaskan dalam 3 kalimat: keputusan desain apa yang kamu ambil, satu
alternatif yang kamu tolak, dan kenapa.
EOF

write internal/hubble/CLAUDE.md <<'EOF'
# ZONA KUNING: internal/hubble

Adapter untuk data historis dari Hubble (BigQuery dataset
`crypto-stellar.crypto_stellar`).

Output WAJIB bertipe `domain.Snapshot`, identik dengan internal/horizon.

## Batasan biaya

Berjalan di BigQuery Sandbox: 1TB query per bulan, gratis, tanpa billing.
Setiap query WAJIB:

- memfilter partisi (batasi rentang ledger atau tanggal) sebelum apa pun
- memilih kolom secara eksplisit, tidak pernah SELECT *
- didahului dry run untuk mengecek byte yang akan dipindai

Query tanpa filter partisi bisa memindai ratusan GB dalam sekali jalan.

## Aturan

Jangan pernah menuliskan issuer address aset dari ingatan. Kalau butuh
identitas aset, ambil dari `docs/decisions/` atau minta Al mengonfirmasi
dari sumber primer lebih dulu.
EOF

# ---------------------------------------------------------------------------
# 4. Slash commands — mesin belajarnya ada di sini
# ---------------------------------------------------------------------------
write .claude/commands/teach.md <<'EOF'
---
description: Jelaskan satu konsep Go atau Stellar tanpa menuliskan kode proyek
argument-hint: [konsep]
allowed-tools: Read, Grep, Glob
---

Al sedang belajar backend Go lewat proyek ini. Ajarkan konsep berikut: $ARGUMENTS

Aturan:

1. Mulai dari kenapa konsep ini ada, masalah apa yang dipecahkannya.
2. Beri SATU contoh Go minimal yang berdiri sendiri, maksimal 15 baris,
   dan tidak menyentuh domain Keel sama sekali.
3. Baru setelah itu, jelaskan bagaimana konsep ini muncul di Keel, dalam
   bentuk prosa. Tanpa kode.
4. Tutup dengan dua pertanyaan pengecek pemahaman. Jangan jawab sendiri.

Jangan menuliskan kode yang bisa langsung ditempel ke repo. Kalau Al
tampak ingin jalan pintas, katakan langsung.
EOF

write .claude/commands/review-mine.md <<'EOF'
---
description: Review kode yang ditulis Al sendiri, tanpa menulis ulang
argument-hint: [path file]
allowed-tools: Read, Grep, Glob, Bash(go vet:*), Bash(go test:*)
---

Review kode yang ditulis Al di: $ARGUMENTS

JANGAN menulis ulang kodenya. JANGAN menempelkan versi perbaikan.
Keluarkan temuan sebagai daftar, masing-masing dengan format:

- [BLOKIR / SERIUS / KECIL] baris ~N: apa masalahnya, kenapa penting,
  dan pertanyaan yang mengarahkan Al ke perbaikannya sendiri.

Urutan prioritas pemeriksaan:

1. Kebenaran finansial. Ada float64? Ada pembulatan yang tidak disengaja?
   Ada pembagian yang bisa nol?
2. Kebenaran domain. Depth SDEX dan AMM digabung di batas marginal price,
   bukan dijumlahkan terpisah?
3. Reproducibility. Iterasi map sudah di-sort? LedgerSeq dan
   MethodologyVersion terbawa?
4. Error handling. Ada error yang ditelan diam-diam?
5. Idiomatik Go. Baru terakhir, dan tandai KECIL.

Kalau tidak ada temuan BLOKIR, katakan begitu. Jangan mengarang masalah
supaya kelihatan teliti.
EOF

write .claude/commands/why.md <<'EOF'
---
description: Jelaskan kenapa sepotong kode di repo ini ditulis begitu
argument-hint: [path file atau nama fungsi]
allowed-tools: Read, Grep, Glob
---

Al ingin paham kode berikut, bukan sekadar memakainya: $ARGUMENTS

Jelaskan:

1. Apa yang dilakukan kode ini, dalam bahasa manusia.
2. Kenapa ditulis dengan cara ini dan bukan cara yang lebih sederhana.
   Kalau memang tidak ada alasan kuat, katakan bahwa ini bisa
   disederhanakan.
3. Apa yang rusak kalau baris kuncinya dihapus.

Kalau kode ini kamu yang tulis di sesi sebelumnya, sebutkan itu.
EOF

write .claude/commands/journal.md <<'EOF'
---
description: Tutup sesi belajar hari ini dan catat ke jurnal
allowed-tools: Read, Edit, Bash(git log:*), Bash(git diff:*)
---

Tutup sesi hari ini.

1. Lihat `git log` dan `git diff` sesi ini. Ringkas apa yang berubah.
2. Pisahkan tegas: mana yang ditulis Al, mana yang kamu tulis.
3. Ajukan satu pertanyaan tentang kode yang KAMU tulis hari ini. Kalau
   Al tidak bisa menjawabnya, itu utang pemahaman dan harus dicatat.
4. Tambahkan entri ke `docs/learning/journal.md` dengan format yang sudah
   ada di file itu. Isi bagian "Utang pemahaman" dengan jujur.

Jangan memuji. Catat apa adanya.
EOF

write .claude/commands/spike.md <<'EOF'
---
description: Jalankan spike terbatas waktu, tulis temuan, jangan tulis kode produksi
argument-hint: [pertanyaan yang mau dijawab]
allowed-tools: Read, Write, Grep, Glob, Bash(go run:*), WebFetch
---

Spike untuk menjawab: $ARGUMENTS

Ini spike, bukan implementasi. Aturannya:

1. Tujuannya menjawab SATU pertanyaan faktual, bukan membangun fitur.
2. Kode spike ditulis di `scripts/spike/` dan bersifat sekali pakai.
   Jangan menyentuh `internal/`.
3. Selesai spike, tulis temuan ke `docs/decisions/` sebagai catatan
   pendek: pertanyaan, apa yang dicoba, apa hasilnya, apa yang diputuskan.
4. Kalau jawabannya "tidak bisa" atau "datanya tidak ada", itu hasil yang
   sah dan berharga. Katakan apa adanya.

Jangan lanjut ke implementasi tanpa Al menyetujui dulu.
EOF

# ---------------------------------------------------------------------------
# 5. Jurnal belajar
# ---------------------------------------------------------------------------
write docs/learning/journal.md <<'EOF'
# Jurnal Belajar Keel

Diisi di akhir setiap sesi lewat `/journal`. Tujuannya melacak utang
pemahaman: kode yang ada di repo tapi belum benar-benar dikuasai.

Aturan main: kalau sebuah item bertahan di "Utang pemahaman" lebih dari
tiga sesi, hentikan penambahan fitur dan bereskan dulu.

---

## Template

### YYYY-MM-DD

**Ditulis Al:**

**Ditulis Claude Code:**

**Konsep baru yang dipahami:**

**Utang pemahaman (kode yang ada tapi belum dikuasai):**

**Pertanyaan terbuka untuk besok:**

---
EOF

write docs/decisions/README.md <<'EOF'
# Catatan Keputusan

Satu file per keputusan. Nama file: `NNNN-judul-singkat.md`.

Format, maksimal satu halaman:

```
# NNNN. Judul

Tanggal:
Status: diusulkan | diterima | digantikan oleh NNNN

## Konteks
Apa yang memaksa keputusan ini diambil.

## Keputusan
Apa yang diputuskan.

## Alternatif yang ditolak
Apa saja, dan kenapa ditolak.

## Konsekuensi
Apa yang jadi lebih mudah, apa yang jadi lebih sulit.
```

Keputusan yang WAJIB dicatat di sini sebelum kode ditulis:

- Identitas aset USTRY (issuer address, asset code, sumber primernya)
- Rentang ledger sequence insiden Blend Mei 2026
- Definisi persis batas marginal price gabungan SDEX + AMM
- Versi metodologi dan apa yang berubah di tiap versi
EOF

# ---------------------------------------------------------------------------
# 6. Infrastruktur lokal
# ---------------------------------------------------------------------------
write docker-compose.yml <<'EOF'
services:
  postgres:
    image: postgres:16-alpine
    container_name: keel-postgres
    environment:
      POSTGRES_USER: keel
      POSTGRES_PASSWORD: keel_dev_only
      POSTGRES_DB: keel
    ports:
      - "5432:5432"
    volumes:
      - keel_pgdata:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U keel"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  keel_pgdata:
EOF

write migrations/0001_snapshots.sql <<'EOF'
-- Snapshot orderbook + AMM. Satu baris per pengambilan per pasangan aset.

CREATE TABLE IF NOT EXISTS snapshots (
    id                  BIGSERIAL PRIMARY KEY,
    captured_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    ledger_seq          BIGINT      NOT NULL,
    methodology_version TEXT        NOT NULL,
    source              TEXT        NOT NULL CHECK (source IN ('horizon', 'hubble')),
    base_asset          TEXT        NOT NULL,
    counter_asset       TEXT        NOT NULL,
    raw                 JSONB       NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_pair_time
    ON snapshots (base_asset, counter_asset, captured_at DESC);

CREATE INDEX IF NOT EXISTS idx_snapshots_ledger
    ON snapshots (ledger_seq);

-- Level orderbook yang sudah dinormalisasi.
-- Harga disimpan sebagai pecahan (price_n / price_d) supaya tidak ada
-- kehilangan presisi. JANGAN menambahkan kolom float di sini.

CREATE TABLE IF NOT EXISTS snapshot_levels (
    id           BIGSERIAL PRIMARY KEY,
    snapshot_id  BIGINT NOT NULL REFERENCES snapshots(id) ON DELETE CASCADE,
    venue        TEXT   NOT NULL CHECK (venue IN ('sdex', 'amm')),
    side         TEXT   NOT NULL CHECK (side IN ('bid', 'ask')),
    price_n      BIGINT NOT NULL,
    price_d      BIGINT NOT NULL CHECK (price_d > 0),
    amount       NUMERIC(38, 18) NOT NULL,
    level_index  INT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_levels_snapshot
    ON snapshot_levels (snapshot_id, venue, side, level_index);
EOF

write Makefile <<'EOF'
.PHONY: up down psql test vet fmt record

up:
	docker compose up -d
	@echo "Postgres siap di localhost:5432 (user keel / db keel)"

down:
	docker compose down

psql:
	docker compose exec postgres psql -U keel -d keel

test:
	go test ./... -race -count=1

vet:
	go vet ./...

fmt:
	gofmt -l -w .

record:
	go run ./cmd/keel record
EOF

write .gitignore <<'EOF'
# Binaries
/bin/
keel

# Env
.env
.env.*

# Claude Code personal settings
.claude/settings.local.json

# Spike sekali pakai
/scripts/spike/out/

# OS
.DS_Store
EOF

write go.mod <<EOF
module ${MODULE_PATH}

go 1.23
EOF

write README.md <<'EOF'
# Keel

Liquidity risk engine untuk ekosistem Stellar.

Oracle menjawab "berapa harganya". Keel menjawab "berapa volume yang bisa
ditopang harga itu".

## Mulai

```bash
make up          # nyalakan Postgres lokal
make test        # jalankan test
make record      # jalankan snapshot recorder
```

## Struktur

- `cmd/keel` entrypoint
- `internal/domain` tipe bersama, terutama `Snapshot`
- `internal/horizon` adapter data live
- `internal/hubble` adapter data historis (BigQuery)
- `internal/depth` inti metodologi, ditulis manual
- `internal/store` persistensi
- `internal/api` HTTP handler
- `docs/methodology` deliverable metodologi
- `docs/decisions` catatan keputusan
- `docs/learning` jurnal belajar
EOF

# ---------------------------------------------------------------------------
echo
echo "Selesai. $created file dibuat, $skipped dilewati."
echo
echo "Langkah berikutnya:"
echo "  1. Ganti CHANGEME di go.mod dengan username GitHub kamu"
echo "  2. Pindahkan dokumen yang sudah ada:"
echo "       OpenAPI spec        -> api/openapi.yaml"
echo "       PRD dan TDD         -> docs/"
echo "       11 file metodologi  -> docs/methodology/"
echo "       CLAUDE.md lama      -> gabungkan manual dengan CLAUDE.md baru"
echo "  3. git init && git add -A && git commit -m 'scaffold'"
echo "  4. make up"
echo "  5. Buka Claude Code, jalankan /permissions untuk verifikasi zona merah terkunci"
