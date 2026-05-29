package execboundary

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_execboundary",
	Doc:  "checks production code starts subprocesses only inside approved process boundaries",
	Run:  run,
}

var forbiddenExecCalls = map[string]struct{}{
	"Command":        {},
	"CommandContext": {},
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
	return paths.PackageMatches(packagePath, "/cmd") ||
		paths.PackageMatches(packagePath, "/scripts") ||
		paths.PackageMatches(packagePath, "/internal/sandbox")
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	if _, ok := forbiddenExecCalls[selector.Sel.Name]; !ok {
		return
	}
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgName, ok := pass.TypesInfo.Uses[base].(*types.PkgName)
	if !ok || pkgName.Imported() == nil || pkgName.Imported().Path() != "os/exec" {
		return
	}

	pass.Reportf(selector.Pos(), "production code must not call os/exec.%s outside cmd, scripts, or the sandbox boundary", selector.Sel.Name)
}
