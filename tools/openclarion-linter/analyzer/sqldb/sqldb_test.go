package sqldb_test

import (
	"testing"

	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/sqldb"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, sqldb.Analyzer,
		"github.com/openclarion/openclarion/internal/usecases/baddbopen",
		"github.com/openclarion/openclarion/internal/persistence/repository/gooddbopen",
		"github.com/openclarion/openclarion/internal/persistence/ent/gooddbopen",
		"github.com/openclarion/openclarion/scripts/gooddbopen",
		"github.com/openclarion/openclarion/internal/usecases/gooddbtest",
	)
}
