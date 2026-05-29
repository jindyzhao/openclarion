package panicboundary

import (
	"go/ast"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_panicboundary",
	Doc:  "checks handwritten production code only panics inside explicit crash or recover boundaries",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if paths.IsTestFile(pass.Fset, file) || ast.IsGenerated(file) {
			continue
		}
		inspectNode(pass, file, false)
	}
	return nil, nil
}

func inspectNode(pass *analysis.Pass, node ast.Node, panicAllowed bool) {
	ast.Inspect(node, func(child ast.Node) bool {
		switch child := child.(type) {
		case nil:
			return true
		case *ast.FuncDecl:
			allowed := panicAllowed || child.Name.Name == "main" || child.Name.Name == "init"
			inspectNode(pass, child.Body, allowed)
			return false
		case *ast.DeferStmt:
			if lit, ok := child.Call.Fun.(*ast.FuncLit); ok && funcLitCallsRecover(pass, lit) {
				inspectNode(pass, lit.Body, true)
				return false
			}
		case *ast.FuncLit:
			inspectNode(pass, child.Body, panicAllowed)
			return false
		case *ast.CallExpr:
			if isBuiltinCall(pass, child, "panic") && !panicAllowed {
				pass.Reportf(child.Pos(), "production code must not call panic outside main, init, or an explicit recover boundary")
			}
		}
		return true
	})
}

func funcLitCallsRecover(pass *analysis.Pass, lit *ast.FuncLit) bool {
	found := false
	ast.Inspect(lit.Body, func(node ast.Node) bool {
		if found {
			return false
		}
		switch node := node.(type) {
		case nil:
			return true
		case *ast.FuncLit:
			if node != lit {
				return false
			}
		case *ast.CallExpr:
			if isBuiltinCall(pass, node, "recover") {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func isBuiltinCall(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok || ident.Name != name {
		return false
	}
	_, ok = pass.TypesInfo.Uses[ident].(*types.Builtin)
	return ok
}
