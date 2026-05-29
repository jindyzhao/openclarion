package txboundary

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

const entPackage = paths.ModulePath + "/internal/persistence/ent"

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_txboundary",
	Doc:  "checks Ent transaction calls stay inside the repository boundary",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if allowedPackage(pass.Pkg.Path()) {
		return nil, nil
	}

	for _, file := range pass.Files {
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

func allowedPackage(packagePath string) bool {
	basePath := strings.TrimSuffix(packagePath, "_test")
	return paths.PackageMatches(basePath, "/internal/persistence/ent") ||
		paths.PackageMatches(basePath, "/internal/persistence/repository")
}

func checkCall(pass *analysis.Pass, call *ast.CallExpr) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	receiverPackage, receiverName := namedReceiver(pass.TypesInfo.TypeOf(selector.X))
	switch selector.Sel.Name {
	case "Tx":
		if receiverPackage == entPackage && receiverName == "Client" {
			pass.Reportf(selector.Sel.Pos(), "only repository boundary packages may call ent.Client.Tx")
		}
	case "Commit", "Rollback":
		if receiverPackage == entPackage && receiverName == "Tx" {
			pass.Reportf(selector.Sel.Pos(), "only repository boundary packages may call ent.Tx.%s", selector.Sel.Name)
		}
	}
}

func namedReceiver(typ types.Type) (string, string) {
	for {
		pointer, ok := typ.(*types.Pointer)
		if !ok {
			break
		}
		typ = pointer.Elem()
	}

	named, ok := typ.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return "", ""
	}
	return named.Obj().Pkg().Path(), named.Obj().Name()
}
