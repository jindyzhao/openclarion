package forbiddenimports_test

import (
	"slices"
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/forbiddenimports"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, forbiddenimports.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/badfake",
		"github.com/openclarion/openclarion/internal/usecases/badimport",
		"github.com/openclarion/openclarion/internal/usecases/badlegacyimports",
		"github.com/openclarion/openclarion/internal/usecases/badmodule",
		"github.com/openclarion/openclarion/internal/usecases/badprovider",
		"github.com/openclarion/openclarion/internal/usecases/badpromtest",
		"github.com/openclarion/openclarion/internal/usecases/goodimport",
	)
}

func TestForbiddenModulePrefixesMatchRetiredLegacyScript(t *testing.T) {
	retiredLegacyPrefixes := []string{
		"github.com/gin-gonic/gin",
		"github.com/labstack/echo",
		"github.com/gofiber/fiber",
		"github.com/go-redis/redis",
		"github.com/redis/go-redis",
		"github.com/gomodule/redigo",
		"go.mongodb.org/mongo-driver",
		"github.com/qdrant/go-client",
		"github.com/milvus-io/milvus-sdk-go",
		"github.com/weaviate/weaviate-go-client",
	}

	got := forbiddenimports.ForbiddenModulePrefixes()
	if !slices.Equal(got, retiredLegacyPrefixes) {
		t.Fatalf("forbidden import list drifted from retired legacy script:\nretired=%v\nanalyzer=%v", retiredLegacyPrefixes, got)
	}
}
