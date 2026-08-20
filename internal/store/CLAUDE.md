# ZONA HIJAU: internal/store

Persistensi Postgres. Tulis bebas, ini plumbing.

Paket ini **bodoh dengan sengaja**. Ia menyimpan dan membaca, tidak menghitung
apa pun. Kalau ada rumus yang mulai muncul di sini, tempatnya salah dan
seharusnya di `internal/depth`.

## Aturan

1. Harga disimpan sebagai pecahan `price_n` dan `price_d`, bukan satu kolom
   desimal hasil pembagian. Jangan pernah menambah kolom `float`, `real`, atau
   `double precision` di skema mana pun.
2. Amount memakai `NUMERIC`, dan dibaca ke `decimal.Decimal`, bukan `float64`.
3. Setiap baris hasil membawa `ledger_seq` dan `methodology_version`. Hasil dari
   versi metodologi berbeda adalah baris berbeda, bukan saling menimpa. Itu yang
   membuat cross-validation bisa dikerjakan dengan satu query.
4. Kolom `data_source` menerima tiga nilai: `horizon`, `hubble`, dan
   `trades-implied`. Nilai ketiga sering terlupa, dan constraint yang menolaknya
   akan gagal justru pada jalur historis yang dipakai studi kasus Blend.

## Yang belum beres sebelum menulis di sini

`migrations/0001_snapshots.sql` masih bertentangan dengan TDD bagian 5. TDD
menyatakan snapshot mentah TIDAK disimpan di database dan mendefinisikan tabel
`assets`, `metrics`, dan `runs`. Migrasi yang ada melakukan sebaliknya, dan tabel
`metrics` yang dibaca `keel serve` belum ada sama sekali.

Selesaikan itu lebih dulu, jangan menulis query di atas skema yang masih
bercabang. Lihat `docs/internal/audit-2026-08-20.md` temuan P1-1 sampai P1-5.
