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

// Architecture tests for Keel's two pure packages: internal/domain and
// internal/depth.
//
// The purity rules are stated in both packages' doc comments. A rule that lives
// only in a document gets broken within two weeks, so they are enforced here.
//
// WHAT IS ENFORCED HERE is static: forbidden imports, float types and literals,
// time.Now, and goroutines.
//
// WHAT IS NOT ENFORCED HERE: iterating a map without sorting first. Detecting
// that statically needs full type information and the cost is not worth it. The
// violation is caught behaviourally by TestInvarianDeterminisme in
// internal/conformance, which compares two runs byte for byte. Delete that test
// and this hole opens.
//
// arch_test.go EXCLUDES ITSELF from the scan, because it does import os and
// path/filepath in order to read files. That is the only exception and it must
// not grow.

// paketMurni lists the directories subject to these rules, relative to
// internal/domain.
var paketMurni = []string{".", "../depth"}

// importTerlarang. An import matches when it is exactly one of these entries, or
// starts with one followed by "/".
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
			// internal/depth is allowed not to exist yet. Once it does, it must
			// comply.
			return nil
		}
		t.Fatalf("read directory %s: %v", dir, err)
	}
	var out []string
	for _, e := range entri {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if e.Name() == "arch_test.go" {
			continue // see the exception note at the head of this file
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
		t.Fatal("no files were scanned; this architecture test has died silently")
	}
}

func TestArchTanpaImportTerlarang(t *testing.T) {
	setiapBerkasMurni(t, func(t *testing.T, nama string, fset *token.FileSet, f *ast.File) {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if cocokTerlarang(path) {
				t.Errorf("%s: forbidden import %q. A pure package must not know where the data came from; move this to an adapter",
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
					t.Errorf("%s: the type %s is forbidden, use decimal.Decimal. Float breaks determinism and cross-validation",
						fset.Position(x.Pos()), x.Name)
				}
			case *ast.BasicLit:
				if x.Kind == token.FLOAT {
					t.Errorf("%s: the float literal %s is forbidden, use decimal.RequireFromString(%q)",
						fset.Position(x.Pos()), x.Value, x.Value)
				}
			}
			return true
		})
	})
}

func TestArchTanpaJamSistem(t *testing.T) {
	terlarang := map[string]string{
		"Now":   "time.Now is forbidden; accept the time as an argument so results can be reproduced",
		"Since": "time.Since calls time.Now internally",
		"Until": "time.Until calls time.Now internally",
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
				t.Errorf("%s: goroutines are forbidden in a pure package; their execution order is not deterministic",
					fset.Position(n.Pos()))
			case *ast.SelectStmt:
				t.Errorf("%s: select is forbidden in a pure package; the Go specification makes its case choice random",
					fset.Position(n.Pos()))
			}
			return true
		})
	})
}

// TestArchVersiMetodologiTerisi guards non-negotiable rule number 1: every
// output carries a MethodologyVersion.
func TestArchVersiMetodologiTerisi(t *testing.T) {
	if strings.TrimSpace(MethodologyVersion) == "" {
		t.Fatal("MethodologyVersion is empty; every output is required to carry it")
	}
}
