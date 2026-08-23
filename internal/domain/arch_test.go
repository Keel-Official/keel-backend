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

// Architecture tests for Keel.
//
// Two scopes, deliberately different in size:
//
//	PURE PACKAGE scope (internal/domain, internal/depth): no forbidden imports,
//	no float, no time.Now, no goroutines. These packages must not know where the
//	data came from.
//
//	WHOLE REPOSITORY scope: no float, anywhere. Non-negotiable rule number 1 in
//	CLAUDE.md says monetary values never use float64, with no exception carved
//	out for adapters. Until 20 August 2026 that rule was only enforced inside the
//	two pure packages, and the gap was not theoretical: internal/adapter sat
//	outside the scan using float64 in two places for months, and nothing caught
//	it. See finding P1-16 in docs/internal/audit-2026-08-20.md.
//
// The purity rules are stated in both pure packages' doc comments. A rule that
// lives only in a document gets broken within two weeks, so they are enforced
// here.
//
// WHAT IS NOT ENFORCED HERE: iterating a map without sorting first. Detecting
// that statically needs full type information and the cost is not worth it. The
// violation is caught behaviorally by TestInvarianDeterminisme in
// internal/conformance, which compares two runs byte for byte. Delete that test
// and this hole opens.
//
// arch_test.go EXCLUDES ITSELF from the scan, because it does import os and
// path/filepath in order to read files. That is the only exception and it must
// not grow.

// paketMurni lists the directories subject to the pure package rules, relative
// to internal/domain.
var paketMurni = []string{".", "../depth"}

// akarRepo is the repository root, relative to internal/domain. The whole
// repository float scan starts here.
const akarRepo = "../.."

// floatDiizinkan lists files permitted to mention a float type or literal,
// relative to the repository root.
//
// It is EMPTY, and it must stay empty unless a written reason is added next to
// the entry. Stellar amounts are int64 stroops and prices are exact rationals;
// there is no arithmetic in this product that needs a float. If a third party
// library ever forces one at a boundary, the conversion belongs in one named
// function with the reason recorded here, not scattered.
var floatDiizinkan = map[string]string{}

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
	setiapBerkasMurni(t, func(t *testing.T, _ string, fset *token.FileSet, f *ast.File) {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if cocokTerlarang(path) {
				t.Errorf("%s: forbidden import %q. A pure package must not know where the data came from; move this to an adapter",
					fset.Position(imp.Pos()), path)
			}
		}
	})
}

// periksaFloat reports every float type and float literal in one parsed file.
func periksaFloat(t *testing.T, _ string, fset *token.FileSet, f *ast.File) {
	t.Helper()
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if x.Name == "float64" || x.Name == "float32" {
				t.Errorf("%s: the type %s is forbidden, use decimal.Decimal or int64. Float breaks determinism and cross-validation",
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
}

func TestArchTanpaFloat(t *testing.T) {
	setiapBerkasMurni(t, periksaFloat)
}

// TestArchTanpaFloatDiSeluruhRepo enforces non-negotiable rule number 1 across
// every Go file in the repository, not only the pure packages.
//
// The rule in CLAUDE.md carves out no exception for adapters, and the gap in the
// old scan was not theoretical: internal/adapter used float64 in two places
// while sitting outside it. Adapters still parse JSON, but into int64 or
// decimal, never into float.
func TestArchTanpaFloatDiSeluruhRepo(t *testing.T) {
	terpindai := 0

	err := filepath.WalkDir(akarRepo, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Nothing to scan inside version control metadata or a temporary
			// clone left by scripts/audit-verification.sh.
			switch d.Name() {
			case ".git", "node_modules", "recordings":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, relErr := filepath.Rel(akarRepo, path)
		if relErr != nil {
			return relErr
		}
		if d.Name() == "arch_test.go" {
			return nil // see the exception note at the head of this file
		}
		if alasan, ok := floatDiizinkan[rel]; ok {
			t.Logf("%s: float allowed, reason on record: %s", rel, alasan)
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		terpindai++
		periksaFloat(t, rel, fset, f)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}

	// A guard against the scan dying silently, the same guard the pure package
	// scan has. The repository always holds more Go files than the pure packages
	// alone, so this floor is deliberately above zero.
	if terpindai < 4 {
		t.Fatalf("only %d Go files were scanned; this test has died silently", terpindai)
	}
	t.Logf("scanned %d Go files across the whole repository", terpindai)
}

func TestArchTanpaJamSistem(t *testing.T) {
	terlarang := map[string]string{
		"Now":   "time.Now is forbidden; accept the time as an argument so results can be reproduced",
		"Since": "time.Since calls time.Now internally",
		"Until": "time.Until calls time.Now internally",
	}
	setiapBerkasMurni(t, func(t *testing.T, _ string, fset *token.FileSet, f *ast.File) {
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
	setiapBerkasMurni(t, func(t *testing.T, _ string, fset *token.FileSet, f *ast.File) {
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
