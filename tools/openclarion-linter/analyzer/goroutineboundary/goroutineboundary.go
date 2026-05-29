package goroutineboundary

import (
	"go/ast"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_goroutineboundary",
	Doc:  "checks handwritten production code starts goroutines only through supervision boundaries",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if isAllowedPackage(pass.Pkg.Path()) {
		return nil, nil
	}
	for _, file := range pass.Files {
		if paths.IsTestFile(pass.Fset, file) || ast.IsGenerated(file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			goStmt, ok := node.(*ast.GoStmt)
			if !ok {
				return true
			}
			pass.Reportf(goStmt.Pos(), "production code must not start goroutines with raw go statements; use errgroup.Group.Go or an approved supervision helper")
			return true
		})
	}
	return nil, nil
}

func isAllowedPackage(packagePath string) bool {
	return paths.PackageMatches(packagePath, "/internal/supervision")
}
