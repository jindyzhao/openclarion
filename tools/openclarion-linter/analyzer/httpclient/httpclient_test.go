package httpclient_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/httpclient"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, httpclient.Analyzer,
		"github.com/openclarion/openclarion/internal/providers/badhttp",
		"github.com/openclarion/openclarion/internal/providers/goodhttp",
		"github.com/openclarion/openclarion/cmd/openclarion/httpallowed",
		"github.com/openclarion/openclarion/scripts/httpallowed",
	)
}
