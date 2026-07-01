package diagnosiscontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func TestEvidenceWithAvailableDiagnosisToolsAppendsSanitizedCatalog(t *testing.T) {
	enabledTemplate := mustToolTemplate(t, 7, 1, true)
	disabledTemplate := mustToolTemplate(t, 8, 1, false)
	disabledSourceTemplate := mustToolTemplate(t, 9, 2, true)
	factory := fakeFactory{
		config: &fakeConfigRepo{
			templates: []domain.DiagnosisToolTemplate{disabledTemplate, disabledSourceTemplate, enabledTemplate},
			sources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{
				1: mustAlertSource(t, 1, true),
				2: mustAlertSource(t, 2, false),
			},
		},
	}

	got, err := EvidenceWithAvailableDiagnosisTools(context.Background(), factory, json.RawMessage(`{"alert":"cpu"}`))
	if err != nil {
		t.Fatalf("EvidenceWithAvailableDiagnosisTools: %v", err)
	}
	if strings.Contains(string(got), "https://") || strings.Contains(string(got), "secret/") {
		t.Fatalf("catalog leaked sensitive provider configuration: %s", got)
	}

	var decoded struct {
		Alert string `json:"alert"`
		Tools struct {
			Usage string                   `json:"usage"`
			Items []AvailableDiagnosisTool `json:"items"`
		} `json:"openclarion_available_diagnosis_tools"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if decoded.Alert != "cpu" {
		t.Fatalf("alert = %q, want cpu", decoded.Alert)
	}
	if len(decoded.Tools.Items) != 1 {
		t.Fatalf("tools len = %d, want 1: %+v", len(decoded.Tools.Items), decoded.Tools.Items)
	}
	for _, fragment := range []string{
		"Use these backend-approved tools before asking for human-supplied evidence",
		"template_id",
		"alert_source_profile_id",
		"snapshot_source_scope",
		"prefer matched tools before supplemental tools",
		"Do not output tool_request_suggestions",
		"Do not invent IDs",
		"tool_request_suggestions, missing_evidence_requests, or evidence_collection_suggestions",
		"JSON number fields",
		"Always prefer the active_alerts evidence_request_example",
		"must not include query",
		"copy the example query exactly",
		"only for evidence that cannot be collected with a listed tool",
	} {
		t.Run("usage/"+fragment, func(t *testing.T) {
			if !strings.Contains(decoded.Tools.Usage, fragment) {
				t.Fatalf("usage = %q, want fragment %q", decoded.Tools.Usage, fragment)
			}
		})
	}
	tool := decoded.Tools.Items[0]
	if tool.TemplateID != 7 ||
		tool.AlertSourceProfileID != 1 ||
		tool.AlertSourceKind != string(domain.AlertSourceKindPrometheus) ||
		tool.SnapshotSourceScope != availableDiagnosisToolScopeUnscoped ||
		tool.Tool != string(domain.DiagnosisToolKindMetricRangeQuery) ||
		tool.QueryTemplate != "up" ||
		tool.DefaultWindowSeconds != 3600 ||
		tool.MaxWindowSeconds != 21600 ||
		tool.DefaultStepSeconds != 60 {
		t.Fatalf("tool = %+v", tool)
	}
	if tool.EvidenceRequest.TemplateID != tool.TemplateID ||
		tool.EvidenceRequest.AlertSourceProfileID != tool.AlertSourceProfileID ||
		tool.EvidenceRequest.Tool != tool.Tool ||
		tool.EvidenceRequest.Reason != "Collect bounded evidence with CPU range." ||
		tool.EvidenceRequest.Query != tool.QueryTemplate ||
		tool.EvidenceRequest.WindowSeconds != tool.DefaultWindowSeconds ||
		tool.EvidenceRequest.StepSeconds != tool.DefaultStepSeconds ||
		tool.EvidenceRequest.Limit != tool.DefaultLimit {
		t.Fatalf("evidence request example = %+v, want copyable request for %+v", tool.EvidenceRequest, tool)
	}
}

func TestEvidenceWithAvailableDiagnosisToolsExpandsTemplateQueryFromSnapshotLabels(t *testing.T) {
	queryTemplate := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
	enabledTemplate := mustMetricQueryToolTemplate(t, 11, 5, true, queryTemplate)
	factory := fakeFactory{
		config: &fakeConfigRepo{
			templates: []domain.DiagnosisToolTemplate{enabledTemplate},
			sources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{
				5: mustAlertSource(t, 5, true),
			},
		},
	}

	got, err := EvidenceWithAvailableDiagnosisTools(context.Background(), factory, json.RawMessage(`{
		"schema_version": "m1.evidence_snapshot.v1",
		"events": [{
			"alert_source_profile_id": 5,
			"labels": {
				"ORACLE_SID": "sapprd1",
				"TABLESPACE": "PSAPSR3USR"
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("EvidenceWithAvailableDiagnosisTools: %v", err)
	}

	var decoded struct {
		Tools struct {
			Items []AvailableDiagnosisTool `json:"items"`
		} `json:"openclarion_available_diagnosis_tools"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if len(decoded.Tools.Items) != 1 {
		t.Fatalf("tools len = %d, want 1", len(decoded.Tools.Items))
	}
	tool := decoded.Tools.Items[0]
	wantQuery := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="sapprd1",TABLESPACE="PSAPSR3USR"}`
	if tool.QueryTemplate != queryTemplate || tool.EvidenceRequest.Query != wantQuery {
		t.Fatalf("tool = %+v, want template %q and query %q", tool, queryTemplate, wantQuery)
	}
}

func TestEvidenceWithAvailableDiagnosisToolsFiltersCatalogToSnapshotProfiles(t *testing.T) {
	inScopeActiveAlerts := mustActiveAlertsToolTemplate(t, 10, 3, true)
	outOfScopeActiveAlerts := mustActiveAlertsToolTemplate(t, 11, 4, true)
	liveThanosMetric := mustMetricQueryToolTemplate(t, 12, 5, true, `up{job="{{label.job}}"}`)
	factory := fakeFactory{
		config: &fakeConfigRepo{
			templates: []domain.DiagnosisToolTemplate{outOfScopeActiveAlerts, liveThanosMetric, inScopeActiveAlerts},
			sources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{
				3: mustAlertSourceOfKind(t, 3, domain.AlertSourceKindAlertmanager, true),
				4: mustAlertSourceOfKind(t, 4, domain.AlertSourceKindAlertmanager, true),
				5: mustAlertSourceOfKind(t, 5, domain.AlertSourceKindPrometheus, true),
			},
		},
	}

	got, err := EvidenceWithAvailableDiagnosisTools(context.Background(), factory, json.RawMessage(`{
		"schema_version": "m1.evidence_snapshot.v1",
		"events": [{
			"alert_source_profile_id": 3,
			"labels": {"job": "node-exporter"}
		}]
	}`))
	if err != nil {
		t.Fatalf("EvidenceWithAvailableDiagnosisTools: %v", err)
	}

	var decoded struct {
		Tools struct {
			Items []AvailableDiagnosisTool `json:"items"`
		} `json:"openclarion_available_diagnosis_tools"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if len(decoded.Tools.Items) != 2 {
		t.Fatalf("tools len = %d, want origin active alerts plus metric supplement: %+v", len(decoded.Tools.Items), decoded.Tools.Items)
	}
	if decoded.Tools.Items[0].TemplateID != int64(inScopeActiveAlerts.ID) ||
		decoded.Tools.Items[0].SnapshotSourceScope != availableDiagnosisToolScopeMatched ||
		decoded.Tools.Items[1].TemplateID != int64(liveThanosMetric.ID) ||
		decoded.Tools.Items[1].SnapshotSourceScope != availableDiagnosisToolScopeSupplemental {
		t.Fatalf("tools order/scope = %+v, want matched alert-source tool before supplemental metric tool", decoded.Tools.Items)
	}
	byID := map[int64]AvailableDiagnosisTool{}
	for _, tool := range decoded.Tools.Items {
		byID[tool.TemplateID] = tool
	}
	if _, ok := byID[int64(outOfScopeActiveAlerts.ID)]; ok {
		t.Fatalf("out-of-scope active alert template was exposed: %+v", decoded.Tools.Items)
	}
	active := byID[int64(inScopeActiveAlerts.ID)]
	if active.Tool != string(domain.DiagnosisToolKindActiveAlerts) ||
		active.AlertSourceProfileID != int64(inScopeActiveAlerts.AlertSourceProfileID) {
		t.Fatalf("active alert tool = %+v, want source-scoped active alerts", active)
	}
	metric := byID[int64(liveThanosMetric.ID)]
	if metric.Tool != string(domain.DiagnosisToolKindMetricQuery) ||
		metric.AlertSourceProfileID != int64(liveThanosMetric.AlertSourceProfileID) ||
		metric.EvidenceRequest.Query != `up{job="node-exporter"}` {
		t.Fatalf("metric tool = %+v, want retained metric supplement with expanded query", metric)
	}
}

func TestEvidenceWithAvailableDiagnosisToolsOmitsUnexecutableMetricTool(t *testing.T) {
	queryTemplate := `db_tablespace_pctusd{job="oracle_exporter",ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`
	enabledTemplate := mustMetricQueryToolTemplate(t, 11, 5, true, queryTemplate)
	factory := fakeFactory{
		config: &fakeConfigRepo{
			templates: []domain.DiagnosisToolTemplate{enabledTemplate},
			sources: map[domain.AlertSourceProfileID]domain.AlertSourceProfile{
				5: mustAlertSource(t, 5, true),
			},
		},
	}

	got, err := EvidenceWithAvailableDiagnosisTools(context.Background(), factory, json.RawMessage(`{
		"schema_version": "m1.evidence_snapshot.v1",
		"events": [{
			"alert_source_profile_id": 5,
			"labels": {
				"ORACLE_SID": "sapprd1"
			}
		}]
	}`))
	if err != nil {
		t.Fatalf("EvidenceWithAvailableDiagnosisTools: %v", err)
	}

	var decoded struct {
		Tools struct {
			Items []AvailableDiagnosisTool `json:"items"`
		} `json:"openclarion_available_diagnosis_tools"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if len(decoded.Tools.Items) != 0 {
		t.Fatalf("tools len = %d, want no unexecutable metric tools: %+v", len(decoded.Tools.Items), decoded.Tools.Items)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(got, &top); err != nil {
		t.Fatalf("unmarshal evidence map: %v", err)
	}
	if _, ok := top[AvailableDiagnosisToolsKey]; ok {
		t.Fatalf("catalog was preserved for an unexecutable metric tool: %s", got)
	}
}

func TestAppendAvailableDiagnosisToolsLeavesEmptyCatalogUnchanged(t *testing.T) {
	base := json.RawMessage(`{"alert":"cpu"}`)
	got, err := AppendAvailableDiagnosisTools(base, nil)
	if err != nil {
		t.Fatalf("AppendAvailableDiagnosisTools: %v", err)
	}
	if string(got) != string(base) {
		t.Fatalf("evidence = %s, want %s", got, base)
	}
	got[0] = '['
	if string(base) != `{"alert":"cpu"}` {
		t.Fatalf("base was mutated: %s", base)
	}
}

func TestAppendAvailableDiagnosisToolsStripsReservedCatalogWhenEmpty(t *testing.T) {
	base := json.RawMessage(`{
		"alert":"cpu",
		"openclarion_available_diagnosis_tools":{"items":[{"template_id":999}]}
	}`)
	got, err := AppendAvailableDiagnosisTools(base, nil)
	if err != nil {
		t.Fatalf("AppendAvailableDiagnosisTools: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	if _, ok := decoded[AvailableDiagnosisToolsKey]; ok {
		t.Fatalf("reserved catalog key was preserved: %s", got)
	}
	if string(decoded["alert"]) != `"cpu"` {
		t.Fatalf("alert = %s, want cpu", decoded["alert"])
	}
}

func TestAppendAvailableDiagnosisToolsRejectsNonObjectEvidence(t *testing.T) {
	_, err := AppendAvailableDiagnosisTools(json.RawMessage(`["cpu"]`), []AvailableDiagnosisTool{{TemplateID: 1}})
	if err == nil || !errors.Is(err, domain.ErrInvariantViolation) {
		t.Fatalf("error = %v, want ErrInvariantViolation", err)
	}
}

func mustToolTemplate(
	t *testing.T,
	id domain.DiagnosisToolTemplateID,
	sourceID domain.AlertSourceProfileID,
	enabled bool,
) domain.DiagnosisToolTemplate {
	t.Helper()
	enabledAt := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	template, err := domain.NewDiagnosisToolTemplate(
		"CPU range",
		sourceID,
		domain.DiagnosisToolKindMetricRangeQuery,
		"up",
		5,
		time.Hour,
		6*time.Hour,
		time.Minute,
		enabled,
		&enabledAt,
		nil,
	)
	if !enabled {
		template, err = domain.NewDiagnosisToolTemplate(
			"CPU range",
			sourceID,
			domain.DiagnosisToolKindMetricRangeQuery,
			"up",
			5,
			time.Hour,
			6*time.Hour,
			time.Minute,
			false,
			nil,
			&enabledAt,
		)
	}
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	template.ID = id
	return template
}

func mustMetricQueryToolTemplate(
	t *testing.T,
	id domain.DiagnosisToolTemplateID,
	sourceID domain.AlertSourceProfileID,
	enabled bool,
	queryTemplate string,
) domain.DiagnosisToolTemplate {
	t.Helper()
	enabledAt := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	template, err := domain.NewDiagnosisToolTemplate(
		"Metric query",
		sourceID,
		domain.DiagnosisToolKindMetricQuery,
		queryTemplate,
		5,
		0,
		0,
		0,
		enabled,
		&enabledAt,
		nil,
	)
	if !enabled {
		template, err = domain.NewDiagnosisToolTemplate(
			"Metric query",
			sourceID,
			domain.DiagnosisToolKindMetricQuery,
			queryTemplate,
			5,
			0,
			0,
			0,
			false,
			nil,
			&enabledAt,
		)
	}
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	template.ID = id
	return template
}

func mustActiveAlertsToolTemplate(
	t *testing.T,
	id domain.DiagnosisToolTemplateID,
	sourceID domain.AlertSourceProfileID,
	enabled bool,
) domain.DiagnosisToolTemplate {
	t.Helper()
	enabledAt := time.Date(2026, 6, 18, 1, 2, 3, 0, time.UTC)
	template, err := domain.NewDiagnosisToolTemplate(
		"Active alerts",
		sourceID,
		domain.DiagnosisToolKindActiveAlerts,
		"",
		5,
		0,
		0,
		0,
		enabled,
		&enabledAt,
		nil,
	)
	if !enabled {
		template, err = domain.NewDiagnosisToolTemplate(
			"Active alerts",
			sourceID,
			domain.DiagnosisToolKindActiveAlerts,
			"",
			5,
			0,
			0,
			0,
			false,
			nil,
			&enabledAt,
		)
	}
	if err != nil {
		t.Fatalf("NewDiagnosisToolTemplate: %v", err)
	}
	template.ID = id
	return template
}

func mustAlertSource(t *testing.T, id domain.AlertSourceProfileID, enabled bool) domain.AlertSourceProfile {
	t.Helper()
	return mustAlertSourceOfKind(t, id, domain.AlertSourceKindPrometheus, enabled)
}

func mustAlertSourceOfKind(
	t *testing.T,
	id domain.AlertSourceProfileID,
	kind domain.AlertSourceKind,
	enabled bool,
) domain.AlertSourceProfile {
	t.Helper()
	source, err := domain.NewAlertSourceProfile(
		string(kind),
		kind,
		"https://prometheus.example.invalid",
		domain.AlertSourceAuthModeBearer,
		"secret/prometheus",
		enabled,
		nil,
	)
	if err != nil {
		t.Fatalf("NewAlertSourceProfile: %v", err)
	}
	source.ID = id
	return source
}

type fakeFactory struct {
	config *fakeConfigRepo
}

func (f fakeFactory) Begin(context.Context) (ports.UnitOfWork, error) {
	return fakeUOW{config: f.config}, nil
}

func (f fakeFactory) WithinTx(ctx context.Context, fn func(context.Context, ports.UnitOfWork) error) error {
	return fn(ctx, fakeUOW{config: f.config})
}

type fakeUOW struct {
	ports.UnitOfWork
	config *fakeConfigRepo
}

func (u fakeUOW) Config() ports.ConfigurationRepository {
	return u.config
}

type fakeConfigRepo struct {
	ports.ConfigurationRepository
	templates []domain.DiagnosisToolTemplate
	sources   map[domain.AlertSourceProfileID]domain.AlertSourceProfile
}

func (r *fakeConfigRepo) ListDiagnosisToolTemplates(
	context.Context,
	int,
) ([]domain.DiagnosisToolTemplate, error) {
	return append([]domain.DiagnosisToolTemplate(nil), r.templates...), nil
}

func (r *fakeConfigRepo) FindAlertSourceProfileByID(
	_ context.Context,
	id domain.AlertSourceProfileID,
) (domain.AlertSourceProfile, error) {
	source, ok := r.sources[id]
	if !ok {
		return domain.AlertSourceProfile{}, domain.ErrNotFound
	}
	return source, nil
}
