package execboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/execboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, execboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badexec",
		"github.com/openclarion/openclarion/cmd/openclarion/execallowed",
		"github.com/openclarion/openclarion/scripts/execallowed",
		"github.com/openclarion/openclarion/internal/sandbox/execallowed",
		"github.com/openclarion/openclarion/internal/usecases/goodexectest",
	)
}
