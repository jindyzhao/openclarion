package contextboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/contextboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, contextboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/domain/badcontext",
		"github.com/openclarion/openclarion/internal/usecases/badcontext",
		"github.com/openclarion/openclarion/internal/domain/goodcontext",
		"github.com/openclarion/openclarion/internal/transport/http/contextallowed",
	)
}
