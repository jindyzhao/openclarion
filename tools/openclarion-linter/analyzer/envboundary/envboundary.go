package envboundary

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_envboundary",
	Doc:  "checks core domain and usecase code does not read or mutate process environment directly",
	Run:  run,
}

var forbiddenOSCalls = map[string]struct{}{
	"Clearenv":  {},
	"Environ":   {},
	"ExpandEnv": {},
	"Getenv":    {},
	"LookupEnv": {},
	"Setenv":    {},
	"Unsetenv":  {},
}

var forbiddenFlagCalls = map[string]struct{}{
	"Arg":         {},
	"Args":        {},
	"Bool":        {},
	"BoolVar":     {},
	"Duration":    {},
	"DurationVar": {},
	"Float64":     {},
	"Float64Var":  {},
	"Int":         {},
	"Int64":       {},
	"Int64Var":    {},
	"IntVar":      {},
	"NArg":        {},
	"NFlag":       {},
	"Parse":       {},
	"Parsed":      {},
	"String":      {},
	"StringVar":   {},
	"Uint":        {},
	"Uint64":      {},
	"Uint64Var":   {},
	"UintVar":     {},
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
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgName, ok := pass.TypesInfo.Uses[base].(*types.PkgName)
	if !ok || pkgName.Imported() == nil {
		return
	}
	switch pkgName.Imported().Path() {
	case "os":
		if _, forbidden := forbiddenOSCalls[selector.Sel.Name]; forbidden {
			pass.Reportf(selector.Pos(), "core domain/usecase code must not read or mutate process environment directly; pass configuration in from boundary wiring")
		}
	case "flag":
		if _, forbidden := forbiddenFlagCalls[selector.Sel.Name]; forbidden {
			pass.Reportf(selector.Pos(), "core domain/usecase code must not parse process flags directly; pass configuration in from boundary wiring")
		}
	}
}
