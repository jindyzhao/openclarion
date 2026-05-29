package reflectionboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/reflectionboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, reflectionboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badreflection",
		"github.com/openclarion/openclarion/internal/usecases/generatedreflection",
		"github.com/openclarion/openclarion/internal/usecases/goodreflection",
	)
}
