package paths

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
)

const ModulePath = "github.com/openclarion/openclarion"

func ImportMatches(importPath, prefix string) bool {
	return importPath == prefix || strings.HasPrefix(importPath, prefix+"/")
}

func PackageMatches(packagePath, suffix string) bool {
	return packagePath == ModulePath+suffix || strings.HasPrefix(packagePath, ModulePath+suffix+"/")
}

func IsTestFile(fset *token.FileSet, file *ast.File) bool {
	return strings.HasSuffix(filepath.ToSlash(fset.Position(file.Pos()).Filename), "_test.go")
}
