package openclarionlinter

import (
	"github.com/golangci/plugin-module-register/register"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/clockboundary"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/execboundary"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/forbiddenimports"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/goroutineboundary"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/httpclient"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/panicboundary"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/reflectionboundary"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/runtimemock"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/sqldb"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/structuredlogging"
	"github.com/openclarion/openclarion/tools/openclarion-linter/analyzer/txboundary"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("openclarion-arch", newPlugin)
}

type plugin struct{}

func newPlugin(_ any) (register.LinterPlugin, error) {
	return plugin{}, nil
}

func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{
		forbiddenimports.Analyzer,
		txboundary.Analyzer,
		runtimemock.Analyzer,
		structuredlogging.Analyzer,
		httpclient.Analyzer,
		sqldb.Analyzer,
		execboundary.Analyzer,
		clockboundary.Analyzer,
		reflectionboundary.Analyzer,
		panicboundary.Analyzer,
		goroutineboundary.Analyzer,
	}, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
