package httpclient

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_httpclient",
	Doc:  "checks production code does not use bare net/http client entrypoints",
	Run:  run,
}

var forbiddenHTTPSelectors = map[string]string{
	"DefaultClient": "use an injected *http.Client with timeout, tracing, and request-id propagation",
	"Get":           "use http.NewRequestWithContext plus an injected *http.Client",
	"Head":          "use http.NewRequestWithContext plus an injected *http.Client",
	"Post":          "use http.NewRequestWithContext plus an injected *http.Client",
	"PostForm":      "use http.NewRequestWithContext plus an injected *http.Client",
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
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			checkSelector(pass, selector)
			return true
		})
	}
	return nil, nil
}

func isAllowedPackage(packagePath string) bool {
	return paths.PackageMatches(packagePath, "/cmd") ||
		paths.PackageMatches(packagePath, "/scripts")
}

func checkSelector(pass *analysis.Pass, selector *ast.SelectorExpr) {
	base, ok := selector.X.(*ast.Ident)
	if !ok {
		return
	}
	pkgName, ok := pass.TypesInfo.Uses[base].(*types.PkgName)
	if !ok || pkgName.Imported() == nil || pkgName.Imported().Path() != "net/http" {
		return
	}
	fix, ok := forbiddenHTTPSelectors[selector.Sel.Name]
	if !ok {
		return
	}
	pass.Reportf(selector.Pos(), "production code must not use net/http.%s directly; %s", selector.Sel.Name, fix)
}
