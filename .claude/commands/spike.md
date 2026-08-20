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
