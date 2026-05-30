package filesystemboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/filesystemboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, filesystemboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/domain/badfilesystem",
		"github.com/openclarion/openclarion/internal/usecases/badfilesystem",
		"github.com/openclarion/openclarion/internal/domain/goodfilesystem",
		"github.com/openclarion/openclarion/cmd/openclarion/filesystemallowed",
	)
}
