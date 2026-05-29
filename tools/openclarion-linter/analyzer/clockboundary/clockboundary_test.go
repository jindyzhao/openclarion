package clockboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/clockboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, clockboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/domain/badclock",
		"github.com/openclarion/openclarion/internal/usecases/badclock",
		"github.com/openclarion/openclarion/internal/domain/goodclock",
		"github.com/openclarion/openclarion/internal/transport/http/clockallowed",
	)
}
