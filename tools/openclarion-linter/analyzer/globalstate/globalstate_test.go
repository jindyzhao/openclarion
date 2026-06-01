package globalstate_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/globalstate"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, globalstate.Analyzer,
		"github.com/openclarion/openclarion/internal/domain/badglobalstate",
		"github.com/openclarion/openclarion/internal/usecases/badglobalstate",
		"github.com/openclarion/openclarion/internal/usecases/generatedglobalstate",
		"github.com/openclarion/openclarion/internal/domain/goodglobalstate",
		"github.com/openclarion/openclarion/cmd/openclarion/globalstateallowed",
	)
}
