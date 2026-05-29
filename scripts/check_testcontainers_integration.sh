#!/usr/bin/env bash
set -euo pipefail

# Enforce integration-test environment contracts:
# - any real Go test package that imports database/sql must also import
#   testcontainers-go in the same directory.
# - Go tests must not use direct host/public network entry points such as
#   http.Get or net.Dial. Use httptest servers, injected clients/transports, or
#   Testcontainers-backed endpoints instead.
# Analyzer fixtures under testdata are excluded because they are source samples,
# not executable integration tests.

scanner_dir="$(mktemp -d)"
trap 'rm -rf "$scanner_dir"' EXIT

cat >"$scanner_dir/main.go" <<'GO'
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const testcontainersImport = "github.com/testcontainers/testcontainers-go"

var directNetworkCalls = map[string]map[string]struct{}{
	"net": {
		"Dial":         {},
		"DialTimeout":  {},
		"Listen":       {},
		"ListenPacket": {},
	},
	"net/http": {
		"Get":      {},
		"Head":     {},
		"Post":     {},
		"PostForm": {},
	},
}

var directNetworkSelectors = map[string]map[string]struct{}{
	"net/http": {
		"DefaultClient":    {},
		"DefaultTransport": {},
	},
}

var skipDirs = map[string]struct{}{
	".git":        {},
	"node_modules": {},
	"testdata":    {},
	"vendor":      {},
}

type packageEvidence struct {
	dbFiles             []string
	testcontainersFiles []string
}

type testFileEvidence struct {
	imports            map[string]struct{}
	directNetworkUses  []string
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail("resolve working directory: %v", err)
	}

	packages := map[string]*packageEvidence{}
	directNetworkViolations := []string{}
	checked := 0
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if _, skip := skipDirs[entry.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

			fileEvidence, err := scanTestFile(path)
			if err != nil {
				return fmt.Errorf("parse %s: %w", rel, err)
			}
			checked++

			evidence := evidenceFor(packages, filepath.ToSlash(filepath.Dir(rel)))
			if hasImport(fileEvidence.imports, "database/sql") {
				evidence.dbFiles = append(evidence.dbFiles, rel)
			}
			if usesTestcontainers(fileEvidence.imports) {
				evidence.testcontainersFiles = append(evidence.testcontainersFiles, rel)
			}
			for _, use := range fileEvidence.directNetworkUses {
				directNetworkViolations = append(directNetworkViolations, rel+":"+use)
			}
			return nil
		}); err != nil {
			fail("scan failed: %v", err)
	}

	violations := []string{}
	for packageDir, evidence := range packages {
		if len(evidence.dbFiles) > 0 && len(evidence.testcontainersFiles) == 0 {
			violations = append(violations, packageDir)
		}
	}
	sort.Strings(violations)

	if len(violations) > 0 {
		fmt.Fprintln(os.Stderr, "[testcontainers-contract] database integration tests must use testcontainers-go:")
		for _, packageDir := range violations {
			fmt.Fprintf(os.Stderr, "  %s/\n", packageDir)
			dbFiles := packages[packageDir].dbFiles
			sort.Strings(dbFiles)
			for _, file := range dbFiles {
				fmt.Fprintf(os.Stderr, "    imports database/sql: %s\n", file)
			}
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Fix: use a Testcontainers-backed database harness in the same test package, or move analyzer fixtures under testdata/.")
	}

	if len(directNetworkViolations) > 0 {
		sort.Strings(directNetworkViolations)
		fmt.Fprintln(os.Stderr, "[testcontainers-contract] Go tests must not use direct host/public network entry points:")
		for _, violation := range directNetworkViolations {
			fmt.Fprintf(os.Stderr, "  %s\n", violation)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Fix: use httptest.NewServer, injected clients/transports, or Testcontainers-backed endpoints instead of package-level network clients.")
	}

	if len(violations) > 0 || len(directNetworkViolations) > 0 {
		os.Exit(1)
	}

	fmt.Printf("[testcontainers-contract] OK (%d test files scanned)\n", checked)
}

func evidenceFor(packages map[string]*packageEvidence, packageDir string) *packageEvidence {
	evidence, ok := packages[packageDir]
	if !ok {
		evidence = &packageEvidence{}
		packages[packageDir] = evidence
	}
	return evidence
}

func scanTestFile(path string) (testFileEvidence, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return testFileEvidence{}, err
	}
	imports := make(map[string]struct{}, len(file.Imports))
	importAliases := make(map[string]string, len(file.Imports))
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			return testFileEvidence{}, err
		}
		imports[importPath] = struct{}{}
		if importSpec.Name != nil {
			alias := importSpec.Name.Name
			if alias != "_" && alias != "." {
				importAliases[alias] = importPath
			}
			continue
		}
		importAliases[defaultImportName(importPath)] = importPath
	}

	evidence := testFileEvidence{imports: imports}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.CallExpr:
			if selector, ok := n.Fun.(*ast.SelectorExpr); ok {
				if importPath, ok := selectorImportPath(selector, importAliases); ok {
					if isDisallowed(directNetworkCalls, importPath, selector.Sel.Name) {
						pos := fset.Position(selector.Sel.Pos())
						evidence.directNetworkUses = append(evidence.directNetworkUses, fmt.Sprintf("%d uses %s.%s", pos.Line, importPath, selector.Sel.Name))
					}
				}
			}
		case *ast.SelectorExpr:
			if importPath, ok := selectorImportPath(n, importAliases); ok {
				if isDisallowed(directNetworkSelectors, importPath, n.Sel.Name) {
					pos := fset.Position(n.Sel.Pos())
					evidence.directNetworkUses = append(evidence.directNetworkUses, fmt.Sprintf("%d uses %s.%s", pos.Line, importPath, n.Sel.Name))
				}
			}
		}
		return true
	})

	return evidence, nil
}

func hasImport(imports map[string]struct{}, path string) bool {
	_, ok := imports[path]
	return ok
}

func usesTestcontainers(imports map[string]struct{}) bool {
	for path := range imports {
		if path == testcontainersImport || strings.HasPrefix(path, testcontainersImport+"/") {
			return true
		}
	}
	return false
}

func defaultImportName(importPath string) string {
	if lastSlash := strings.LastIndex(importPath, "/"); lastSlash >= 0 {
		return importPath[lastSlash+1:]
	}
	return importPath
}

func selectorImportPath(selector *ast.SelectorExpr, importAliases map[string]string) (string, bool) {
	ident, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	importPath, ok := importAliases[ident.Name]
	return importPath, ok
}

func isDisallowed(rules map[string]map[string]struct{}, importPath, selector string) bool {
	selectors, ok := rules[importPath]
	if !ok {
		return false
	}
	_, ok = selectors[selector]
	return ok
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[testcontainers-contract] "+format+"\n", args...)
	os.Exit(1)
}
GO

go run "$scanner_dir/main.go"
