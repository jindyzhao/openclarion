package panicboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/panicboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, panicboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badpanic",
		"github.com/openclarion/openclarion/internal/persistence/repository/recoverboundary",
		"github.com/openclarion/openclarion/cmd/openclarion/panicallowed",
		"github.com/openclarion/openclarion/internal/usecases/generatedpanic",
		"github.com/openclarion/openclarion/internal/usecases/goodpanictest",
	)
}
