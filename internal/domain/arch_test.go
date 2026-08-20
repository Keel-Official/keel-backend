package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Uji arsitektur untuk dua paket murni Keel: internal/domain dan internal/depth.
//
// Aturan kemurnian ada di doc comment kedua paket itu. Aturan yang hanya
// ditulis di dokumen akan dilanggar dalam dua minggu, jadi ditegakkan di sini.
//
// YANG DITEGAKKAN DI SINI bersifat statis: import terlarang, tipe dan literal
// float, time.Now, dan goroutine.
//
// YANG TIDAK DITEGAKKAN DI SINI: iterasi map tanpa sort lebih dulu. Mendeteksi
// itu secara statis butuh informasi tipe penuh, dan biayanya tidak sepadan.
// Pelanggarannya tertangkap secara perilaku oleh TestInvarianDeterminisme di
// internal/conformance, yang membandingkan dua kali jalan byte per byte.
// Kalau test itu dihapus, lubang ini terbuka.
//
// arch_test.go MENGECUALIKAN DIRINYA SENDIRI dari pemindaian, karena ia memang
// mengimpor os dan path/filepath untuk membaca berkas. Itu satu-satunya
// pengecualian dan tidak boleh bertambah.

// paketMurni adalah direktori yang tunduk pada aturan ini, relatif terhadap
// internal/domain.
var paketMurni = []string{".", "../depth"}

// importTerlarang. Sebuah import cocok bila sama persis atau berawalan
// entri ini diikuti "/".
var importTerlarang = []string{
	"net",
	"net/http",
	"os",
	"os/exec",
	"io",
	"io/fs",
	"bufio",
	"database/sql",
	"math/rand",
	"crypto/rand",
	"context",
	"log",
	"log/slog",
	"path/filepath",
	"github.com/stellar",
	"cloud.google.com",
	"google.golang.org",
	"github.com/jackc",
	"github.com/lib/pq",
}

func cocokTerlarang(path string) bool {
	for _, t := range importTerlarang {
		if path == t || strings.HasPrefix(path, t+"/") {
			return true
		}
	}
	return false
}

func berkasGo(t *testing.T, dir string) []string {
	t.Helper()
	entri, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// internal/depth boleh belum ada. Kalau sudah ada, ia wajib patuh.
			return nil
		}
		t.Fatalf("baca direktori %s: %v", dir, err)
	}
	var out []string
	for _, e := range entri {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if e.Name() == "arch_test.go" {
			continue // lihat catatan pengecualian di kepala berkas
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func setiapBerkasMurni(t *testing.T, fn func(t *testing.T, nama string, fset *token.FileSet, f *ast.File)) {
	t.Helper()
	terpindai := 0
	for _, dir := range paketMurni {
		for _, nama := range berkasGo(t, dir) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, nama, nil, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse %s: %v", nama, err)
			}
			terpindai++
			fn(t, nama, fset, f)
		}
	}
	if terpindai == 0 {
		t.Fatal("tidak ada berkas yang dipindai; uji arsitektur ini mati diam-diam")
	}
}

func TestArchTanpaImportTerlarang(t *testing.T) {
	setiapBerkasMurni(t, func(t *testing.T, nama string, fset *token.FileSet, f *ast.File) {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if cocokTerlarang(path) {
				t.Errorf("%s: import terlarang %q. Paket murni tidak boleh tahu dari mana data datang; pindahkan ke adapter",
					fset.Position(imp.Pos()), path)
			}
		}
	})
}

func TestArchTanpaFloat(t *testing.T) {
	setiapBerkasMurni(t, func(t *testing.T, nama string, fset *token.FileSet, f *ast.File) {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				if x.Name == "float64" || x.Name == "float32" {
					t.Errorf("%s: tipe %s dilarang, pakai decimal.Decimal. Float merusak determinisme dan cross-validation",
						fset.Position(x.Pos()), x.Name)
				}
			case *ast.BasicLit:
				if x.Kind == token.FLOAT {
					t.Errorf("%s: literal float %s dilarang, pakai decimal.RequireFromString(%q)",
						fset.Position(x.Pos()), x.Value, x.Value)
				}
			}
			return true
		})
	})
}

func TestArchTanpaJamSistem(t *testing.T) {
	terlarang := map[string]string{
		"Now":   "time.Now dilarang; terima waktu sebagai argumen agar hasil dapat direproduksi",
		"Since": "time.Since memanggil time.Now di dalamnya",
		"Until": "time.Until memanggil time.Now di dalamnya",
	}
	setiapBerkasMurni(t, func(t *testing.T, nama string, fset *token.FileSet, f *ast.File) {
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "time" {
				return true
			}
			if alasan, bad := terlarang[sel.Sel.Name]; bad {
				t.Errorf("%s: %s", fset.Position(sel.Pos()), alasan)
			}
			return true
		})
	})
}

func TestArchTanpaGoroutine(t *testing.T) {
	setiapBerkasMurni(t, func(t *testing.T, nama string, fset *token.FileSet, f *ast.File) {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.GoStmt:
				t.Errorf("%s: goroutine dilarang di paket murni; urutan eksekusinya tidak deterministik",
					fset.Position(n.Pos()))
			case *ast.SelectStmt:
				t.Errorf("%s: select dilarang di paket murni; pemilihan case-nya acak menurut spesifikasi Go",
					fset.Position(n.Pos()))
			}
			return true
		})
	})
}

// TestArchVersiMetodologiTerisi menjaga aturan yang tidak bisa ditawar nomor 1:
// setiap keluaran membawa MethodologyVersion.
func TestArchVersiMetodologiTerisi(t *testing.T) {
	if strings.TrimSpace(MethodologyVersion) == "" {
		t.Fatal("MethodologyVersion kosong; setiap keluaran wajib membawanya")
	}
}
