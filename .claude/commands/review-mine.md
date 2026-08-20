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
