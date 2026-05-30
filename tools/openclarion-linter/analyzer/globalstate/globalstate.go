package globalstate

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_globalstate",
	Doc:  "checks core domain and usecase code does not keep mutable package-level collection state",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if !isCorePackage(pass.Pkg.Path()) {
		return nil, nil
	}

	for _, file := range pass.Files {
		if paths.IsTestFile(pass.Fset, file) || ast.IsGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			checkVarDecl(pass, gen)
		}
	}
	return nil, nil
}

func isCorePackage(packagePath string) bool {
	return paths.PackageMatches(packagePath, "/internal/domain") ||
		paths.PackageMatches(packagePath, "/internal/usecases")
}

func checkVarDecl(pass *analysis.Pass, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for index, name := range value.Names {
			if name.Name == "_" {
				continue
			}
			obj := pass.TypesInfo.Defs[name]
			if obj != nil && isMutableCollectionState(obj.Type()) {
				reportMutableGlobal(pass, name)
				continue
			}
			if isMutableCollectionState(initializerType(pass, value, index)) {
				reportMutableGlobal(pass, name)
			}
		}
	}
}

func initializerType(pass *analysis.Pass, value *ast.ValueSpec, nameIndex int) types.Type {
	switch {
	case len(value.Values) == 0:
		return nil
	case len(value.Values) == len(value.Names):
		return pass.TypesInfo.TypeOf(value.Values[nameIndex])
	case len(value.Values) == 1:
		typ := pass.TypesInfo.TypeOf(value.Values[0])
		if tuple, ok := typ.(*types.Tuple); ok && nameIndex < tuple.Len() {
			return tuple.At(nameIndex).Type()
		}
		return typ
	default:
		return nil
	}
}

func isMutableCollectionState(typ types.Type) bool {
	if typ == nil {
		return false
	}
	for {
		ptr, ok := typ.Underlying().(*types.Pointer)
		if !ok {
			break
		}
		typ = ptr.Elem()
	}
	switch typ.Underlying().(type) {
	case *types.Array, *types.Chan, *types.Map, *types.Slice:
		return true
	default:
		return false
	}
}

func reportMutableGlobal(pass *analysis.Pass, name *ast.Ident) {
	pass.Reportf(name.Pos(), "core domain/usecase code must not keep mutable package-level collection state; use constants, immutable constructors, or boundary-injected state")
}
