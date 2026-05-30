package filesystemboundary

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_filesystemboundary",
	Doc:  "checks core domain and usecase code does not access local files directly",
	Run:  run,
}

var forbiddenSelectors = map[string]map[string]struct{}{
	"io/ioutil": {
		"ReadDir":   {},
		"ReadFile":  {},
		"TempDir":   {},
		"TempFile":  {},
		"WriteFile": {},
	},
	"os": {
		"Chdir":         {},
		"Chmod":         {},
		"Chown":         {},
		"Chtimes":       {},
		"CopyFS":        {},
		"Create":        {},
		"CreateTemp":    {},
		"DirFS":         {},
		"Getwd":         {},
		"Lchown":        {},
		"Link":          {},
		"Lstat":         {},
		"Mkdir":         {},
		"MkdirAll":      {},
		"MkdirTemp":     {},
		"Open":          {},
		"OpenFile":      {},
		"ReadDir":       {},
		"ReadFile":      {},
		"Readlink":      {},
		"Remove":        {},
		"RemoveAll":     {},
		"Rename":        {},
		"Stat":          {},
		"Symlink":       {},
		"TempDir":       {},
		"Truncate":      {},
		"UserCacheDir":  {},
		"UserConfigDir": {},
		"UserHomeDir":   {},
		"WriteFile":     {},
	},
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
	if blocked, ok := forbiddenSelectors[pkgName.Imported().Path()]; !ok {
		return
	} else if _, forbidden := blocked[selector.Sel.Name]; !forbidden {
		return
	}

	pass.Reportf(selector.Pos(), "core domain/usecase code must not access local files directly; use provider, repository, or boundary-layer input")
}
