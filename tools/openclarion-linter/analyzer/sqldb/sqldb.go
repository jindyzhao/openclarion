package sqldb

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_sqldb",
	Doc:  "checks database/sql open calls stay inside the persistence boundary",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if isAllowedPackage(pass.Pkg.Path()) {
		return nil, nil
	}

	for _, file := range pass.Files {
		if paths.IsTestFile(pass.Fset, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			checkCall(pass, call)
			return true
		})
	}
	return nil, nil
}

func isAllowedPackage(packagePath string) bool {
	return paths.PackageMatches(packagePath, "/internal/persistence/repository") ||
		paths.PackageMatches(packagePath, "/internal/persistence/ent") ||
		paths.PackageMatches(packagePath, "/scripts")
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Open" {
		return
	}
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgName, ok := pass.TypesInfo.Uses[base].(*types.PkgName)
	if !ok || pkgName.Imported() == nil || pkgName.Imported().Path() != "database/sql" {
		return
	}
	pass.Reportf(selector.Pos(), "production code must not call database/sql.Open outside the persistence repository boundary")
}
