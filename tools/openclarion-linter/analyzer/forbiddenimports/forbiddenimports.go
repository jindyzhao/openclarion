package forbiddenimports

import (
	"go/ast"
	"strconv"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/internal/paths"
	"golang.org/x/tools/go/analysis"
)

const (
	entPrefix        = paths.ModulePath + "/internal/persistence/ent"
	fakePrefix       = paths.ModulePath + "/internal/providers/metrics/fake"
	metricsPrefix    = paths.ModulePath + "/internal/providers/metrics"
	prometheusPrefix = paths.ModulePath + "/internal/providers/metrics/prometheus"
)

var Analyzer = &analysis.Analyzer{
	Name: "openclarion_forbiddenimports",
	Doc:  "checks forbidden imports and OpenClarion architecture boundaries",
	Run:  run,
}

var forbiddenModulePrefixes = []string{
	"github.com/gin-gonic/" + "gin",
	"github.com/labstack/" + "echo",
	"github.com/gofiber/" + "fiber",
	"github.com/go-redis/" + "redis",
	"github.com/redis/" + "go-redis",
	"github.com/gomodule/" + "redigo",
	"go.mongodb.org/" + "mongo-driver",
	"github.com/qdrant/" + "go-client",
	"github.com/milvus-io/" + "milvus-sdk-go",
	"github.com/weaviate/" + "weaviate-go-client",
}

func ForbiddenModulePrefixes() []string {
	out := make([]string, len(forbiddenModulePrefixes))
	copy(out, forbiddenModulePrefixes)
	return out
}

func run(pass *analysis.Pass) (any, error) {
	inDomainOrUsecase := paths.PackageMatches(pass.Pkg.Path(), "/internal/domain") ||
		paths.PackageMatches(pass.Pkg.Path(), "/internal/usecases")

	for _, file := range pass.Files {
		testFile := paths.IsTestFile(pass.Fset, file)
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			checkForbiddenModule(pass, spec, importPath)
			if inDomainOrUsecase {
				checkBoundaryImport(pass, spec, importPath, testFile)
			}
		}
	}
	return nil, nil
}

func checkForbiddenModule(pass *analysis.Pass, spec *ast.ImportSpec, importPath string) {
	for _, forbidden := range forbiddenModulePrefixes {
		if paths.ImportMatches(importPath, forbidden) {
			pass.Reportf(spec.Pos(), "forbidden module import %q is blocked by ADR-0001/ADR-0012", importPath)
			return
		}
	}
}

func checkBoundaryImport(pass *analysis.Pass, spec *ast.ImportSpec, importPath string, testFile bool) {
	switch {
	case !testFile && paths.ImportMatches(importPath, entPrefix):
		pass.Reportf(spec.Pos(), "production domain/usecase code must not import Ent package %q", importPath)
	case paths.ImportMatches(importPath, prometheusPrefix):
		pass.Reportf(spec.Pos(), "domain/usecase code, including tests, must not import Prometheus provider %q", importPath)
	case !testFile && paths.ImportMatches(importPath, fakePrefix):
		pass.Reportf(spec.Pos(), "production domain/usecase code must not import fake provider %q", importPath)
	case paths.ImportMatches(importPath, metricsPrefix) && !(testFile && paths.ImportMatches(importPath, fakePrefix)):
		pass.Reportf(spec.Pos(), "domain/usecase code must not import concrete metrics provider %q", importPath)
	}
}
