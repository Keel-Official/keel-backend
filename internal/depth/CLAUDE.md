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
