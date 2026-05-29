package goroutineboundary_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/goroutineboundary"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, goroutineboundary.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badgoroutine",
		"github.com/openclarion/openclarion/internal/usecases/goodgroup",
		"github.com/openclarion/openclarion/internal/supervision/goroutineallowed",
		"github.com/openclarion/openclarion/internal/usecases/generatedgoroutine",
		"github.com/openclarion/openclarion/internal/usecases/goodgoroutinetest",
	)
}
