package txboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/txboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, txboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badtx",
		"github.com/openclarion/openclarion/internal/persistence/repository",
		"github.com/openclarion/openclarion/internal/persistence/repository/goodtx",
	)
}
