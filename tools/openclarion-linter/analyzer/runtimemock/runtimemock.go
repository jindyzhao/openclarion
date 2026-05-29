package runtimemock

import (
	"strconv"
	"strings"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

const providersPrefix = paths.ModulePath + "/internal/providers/"

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_runtimemock",
	Doc:  "checks production runtime wiring does not import fake providers",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if !paths.PackageMatches(pass.Pkg.Path(), "/cmd/openclarion") {
		return nil, nil
	}

	for _, file := range pass.Files {
		if paths.IsTestFile(pass.Fset, file) {
			continue
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			if isFakeProviderImport(importPath) {
				pass.Reportf(spec.Pos(), "production cmd/openclarion code must not import fake provider %q", importPath)
			}
		}
	}
	return nil, nil
}

func isFakeProviderImport(importPath string) bool {
	if !strings.HasPrefix(importPath, providersPrefix) {
		return false
	}
	rest := strings.TrimPrefix(importPath, providersPrefix)
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "fake" {
			return true
		}
	}
	return false
}
