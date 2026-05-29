package structuredlogging_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/structuredlogging"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, structuredlogging.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badlog",
		"github.com/openclarion/openclarion/internal/usecases/goodlog",
		"github.com/openclarion/openclarion/cmd/openclarion/logallowed",
		"github.com/openclarion/openclarion/scripts/logallowed",
		"github.com/openclarion/openclarion/internal/persistence/ent/logallowed",
	)
}
