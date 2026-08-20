// Command keel adalah entrypoint tunggal Keel.
//
// Satu binary, beberapa subperintah, dijadwalkan dengan cron internal. Tidak
// ada Kubernetes dan tidak ada orkestrator terpisah; deliverable dinilai dari
// bukti yang bisa diverifikasi, bukan dari kecanggihan infrastruktur.
//
//	keel version    cetak versi metodologi dan keluar
//	keel record     rekam snapshot Horizon mentah untuk cross-validation
//	keel scan       hitung metrik untuk seluruh aset aktif, simpan ke Postgres
//	keel serve      jalankan API baca
//	keel replay     jalankan ulang rentang ledger lewat adapter historis
package main

import (
	"fmt"
	"os"

	"github.com/Keel-Official/keel/internal/domain"
)

// belumSiap adalah kode keluar untuk subperintah yang sudah punya tempat tetapi
// belum punya isi. Dibedakan dari kode 1 (salah pakai) supaya penjadwal dapat
// membedakan "belum dibangun" dari "gagal".
const belumSiap = 3

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch perintah := os.Args[1]; perintah {
	case "version":
		fmt.Printf("keel methodology %s\n", domain.MethodologyVersion)

	case "record":
		// Perekam cross-validation. Wajib mulai berjalan LEBIH DULU daripada
		// jalur historis, karena kebenaran pembanding tidak bisa dibuat surut.
		// Setiap hari tertunda adalah bukti yang hilang permanen.
		belum(perintah, "internal/horizon + perekam ke recordings/")

	case "scan":
		belum(perintah, "internal/horizon + internal/depth + internal/store")

	case "serve":
		belum(perintah, "internal/api + internal/store")

	case "replay":
		belum(perintah, "internal/hubble; ditunda, lihat docs/decisions/DEC-002-hold-bigquery.md")

	case "help", "-h", "--help":
		usage()

	default:
		fmt.Fprintf(os.Stderr, "keel: subperintah tidak dikenal %q\n\n", perintah)
		usage()
		os.Exit(1)
	}
}

func belum(perintah, butuh string) {
	fmt.Fprintf(os.Stderr, "keel %s: belum diimplementasikan (butuh %s)\n", perintah, butuh)
	os.Exit(belumSiap)
}

func usage() {
	fmt.Fprint(os.Stderr, `keel - liquidity risk engine untuk ekosistem Stellar

Pemakaian:
  keel <subperintah>

Subperintah:
  version   cetak versi metodologi
  record    rekam snapshot Horizon mentah untuk cross-validation
  scan      hitung metrik seluruh aset aktif, simpan ke Postgres
  serve     jalankan API baca
  replay    jalankan ulang rentang ledger lewat adapter historis
`)
}
