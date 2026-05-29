package reflectionboundary

import (
	"go/ast"
	"strconv"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_reflectionboundary",
	Doc:  "checks handwritten production code does not import reflection escape hatches",
	Run:  run,
}

var forbiddenImports = map[string]string{
	"reflect": "reflection must stay in generated code or an explicitly reviewed infra boundary",
	"unsafe":  "unsafe code must stay in generated code or an explicitly reviewed infra boundary",
}

func run(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if paths.IsTestFile(pass.Fset, file) || ast.IsGenerated(file) {
			continue
		}
		for _, importSpec := range file.Imports {
			importPath, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				continue
			}
			reason, ok := forbiddenImports[importPath]
			if !ok {
				continue
			}
			pass.Reportf(importSpec.Pos(), "production code must not import %s directly; %s", importPath, reason)
		}
	}
	return nil, nil
}
