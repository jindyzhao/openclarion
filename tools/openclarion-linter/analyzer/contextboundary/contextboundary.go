package contextboundary

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_contextboundary",
	Doc:  "checks core domain and usecase code does not create root or detached contexts",
	Run:  run,
}

var forbiddenContextCalls = map[string]struct{}{
	"Background":    {},
	"TODO":          {},
	"WithoutCancel": {},
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
	if !ok {
		return
	}
	if _, forbidden := forbiddenContextCalls[selector.Sel.Name]; !forbidden {
		return
	}
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgName, ok := pass.TypesInfo.Uses[base].(*types.PkgName)
	if !ok || pkgName.Imported() == nil || pkgName.Imported().Path() != "context" {
		return
	}

	pass.Reportf(selector.Pos(), "core domain/usecase code must not create root or detached contexts; receive context from caller or boundary orchestration")
}
