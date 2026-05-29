package clockboundary

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_clockboundary",
	Doc:  "checks core domain and usecase code does not read wall-clock time directly",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if !isCorePackage(pass.Pkg.Path()) {
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

func isCorePackage(packagePath string) bool {
	return paths.PackageMatches(packagePath, "/internal/domain") ||
		paths.PackageMatches(packagePath, "/internal/usecases")
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Now" {
		return
	}
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgName, ok := pass.TypesInfo.Uses[base].(*types.PkgName)
	if !ok || pkgName.Imported() == nil || pkgName.Imported().Path() != "time" {
		return
	}

	pass.Reportf(selector.Pos(), "core domain/usecase code must not call time.Now directly; pass time in or inject a clock at the boundary")
}
