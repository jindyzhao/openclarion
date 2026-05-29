package runtimemock_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/runtimemock"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, runtimemock.Analyzer,
		"github.com/openclarion/openclarion/cmd/openclarion/badllm",
		"github.com/openclarion/openclarion/cmd/openclarion/badmain",
		"github.com/openclarion/openclarion/cmd/openclarion/goodtest",
	)
}
