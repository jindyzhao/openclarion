package envboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/envboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, envboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/domain/badenv",
		"github.com/openclarion/openclarion/internal/usecases/badenv",
		"github.com/openclarion/openclarion/internal/usecases/badflag",
		"github.com/openclarion/openclarion/internal/domain/goodenv",
		"github.com/openclarion/openclarion/cmd/openclarion/envallowed",
	)
}
