// Package diagnosiscontext builds sanitized context blocks mounted into
// diagnosis-room evidence.
package diagnosiscontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/diagnosisquery"
	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// AvailableDiagnosisToolsKey is the reserved evidence.json object key used
	// to tell diagnosis runners which backend-approved tools can be requested.
	AvailableDiagnosisToolsKey = "openclarion_available_diagnosis_tools"

	defaultAvailableDiagnosisToolsLimit = 100

	availableDiagnosisToolScopeMatched      = "matched"
	availableDiagnosisToolScopeSupplemental = "supplemental"
	availableDiagnosisToolScopeUnscoped     = "unscoped"

	availableDiagnosisToolsUsage = "Use these backend-approved tools before asking for human-supplied evidence whenever a listed tool can answer the gap. " +
		"Executable evidence_requests must copy evidence_request_example into evidence_requests with template_id and alert_source_profile_id as JSON number fields. " +
		"When snapshot_source_scope is present, prefer matched tools before supplemental tools. " +
		"Do not output tool_request_suggestions. " +
		"Do not put executable tool calls in tool_request_suggestions, missing_evidence_requests, or evidence_collection_suggestions. " +
		"Do not invent IDs or queries. " +
		"Always prefer the active_alerts evidence_request_example over a collection suggestion when current active alerts are relevant. " +
		"active_alerts requests must only use the copied active_alerts evidence_request_example fields and must not include query, window_seconds, window_minutes, or step_seconds. " +
		"For metric_query or metric_range_query, copy the example query exactly. " +
		"Use missing_evidence_requests or evidence_collection_suggestions only for evidence that cannot be collected with a listed tool."
)

// AvailableDiagnosisTool is a sanitized, non-secret descriptor for one enabled
// executable diagnosis evidence template.
type AvailableDiagnosisTool struct {
	TemplateID           int64                                `json:"template_id"`
	Name                 string                               `json:"name"`
	AlertSourceProfileID int64                                `json:"alert_source_profile_id"`
	AlertSourceName      string                               `json:"alert_source_name"`
	AlertSourceKind      string                               `json:"alert_source_kind"`
	SnapshotSourceScope  string                               `json:"snapshot_source_scope"`
	Tool                 string                               `json:"tool"`
	QueryTemplate        string                               `json:"query_template,omitempty"`
	DefaultLimit         int                                  `json:"default_limit"`
	DefaultWindowSeconds int                                  `json:"default_window_seconds,omitempty"`
	MaxWindowSeconds     int                                  `json:"max_window_seconds,omitempty"`
	DefaultStepSeconds   int                                  `json:"default_step_seconds,omitempty"`
	EvidenceRequest      AvailableDiagnosisToolRequestExample `json:"evidence_request_example"`
}

// AvailableDiagnosisToolRequestExample is the exact executable request shape a
// runner can copy into diagnosis_turn.v1 evidence_requests.
type AvailableDiagnosisToolRequestExample struct {
	TemplateID           int64  `json:"template_id"`
	AlertSourceProfileID int64  `json:"alert_source_profile_id"`
	Tool                 string `json:"tool"`
	Reason               string `json:"reason"`
	Query                string `json:"query,omitempty"`
	WindowSeconds        int    `json:"window_seconds,omitempty"`
	StepSeconds          int    `json:"step_seconds,omitempty"`
	Limit                int    `json:"limit,omitempty"`
}

type availableDiagnosisToolsCatalog struct {
	Usage string                   `json:"usage"`
	Items []AvailableDiagnosisTool `json:"items"`
}

// EvidenceWithAvailableDiagnosisTools returns base evidence plus a sanitized
// catalog of enabled diagnosis tool templates. It never includes provider URLs,
// credential references, or secret values.
func EvidenceWithAvailableDiagnosisTools(
	ctx context.Context,
	uowFactory ports.UnitOfWorkFactory,
	base json.RawMessage,
) (json.RawMessage, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis context: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	tools, err := LoadAvailableDiagnosisTools(ctx, uowFactory, defaultAvailableDiagnosisToolsLimit)
	if err != nil {
		return nil, err
	}
	tools = filterAvailableDiagnosisToolsForEvidence(base, tools)
	tools = applyEvidenceTemplateValues(base, tools)
	tools = filterExecutableMetricDiagnosisTools(tools)
	tools = prioritizeAvailableDiagnosisToolsForEvidence(base, tools)
	return AppendAvailableDiagnosisTools(base, tools)
}

// LoadAvailableDiagnosisTools loads enabled templates with enabled bound
// sources and projects them into the sanitized runner-facing catalog shape.
func LoadAvailableDiagnosisTools(
	ctx context.Context,
	uowFactory ports.UnitOfWorkFactory,
	limit int,
) ([]AvailableDiagnosisTool, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("diagnosis context: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if limit <= 0 {
		return nil, fmt.Errorf("diagnosis context: tool limit must be > 0 (got %d): %w", limit, domain.ErrInvariantViolation)
	}
	var tools []AvailableDiagnosisTool
	err := uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		templates, err := uow.Config().ListDiagnosisToolTemplates(ctx, limit)
		if err != nil {
			return err
		}
		tools = make([]AvailableDiagnosisTool, 0, len(templates))
		for _, template := range templates {
			if !template.Enabled {
				continue
			}
			source, err := uow.Config().FindAlertSourceProfileByID(ctx, template.AlertSourceProfileID)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					continue
				}
				return err
			}
			if !source.Enabled {
				continue
			}
			tools = append(tools, availableDiagnosisTool(template, source))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("diagnosis context: load available tools: %w", err)
	}
	return tools, nil
}

// AppendAvailableDiagnosisTools adds the supplied catalog to a duplicate-key
// free JSON object. Empty catalogs leave ordinary evidence unchanged but strip
// the reserved catalog key when it is already present.
func AppendAvailableDiagnosisTools(
	base json.RawMessage,
	tools []AvailableDiagnosisTool,
) (json.RawMessage, error) {
	if len(tools) == 0 {
		if stripped, ok, err := stripAvailableDiagnosisTools(base); err != nil {
			return nil, err
		} else if ok {
			return stripped, nil
		}
		return cloneRawMessage(base), nil
	}
	top, err := decodeEvidenceObject(base)
	if err != nil {
		return nil, err
	}
	catalog, err := json.Marshal(availableDiagnosisToolsCatalog{
		Usage: availableDiagnosisToolsUsage,
		Items: tools,
	})
	if err != nil {
		return nil, fmt.Errorf("diagnosis context: marshal available tools: %w", err)
	}
	top[AvailableDiagnosisToolsKey] = catalog
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("diagnosis context: marshal evidence: %w", err)
	}
	if _, err := decodeEvidenceObject(out); err != nil {
		return nil, err
	}
	return out, nil
}

func stripAvailableDiagnosisTools(base json.RawMessage) (json.RawMessage, bool, error) {
	top, err := decodeEvidenceObject(base)
	if err != nil {
		return nil, false, err
	}
	if _, ok := top[AvailableDiagnosisToolsKey]; !ok {
		return nil, false, nil
	}
	delete(top, AvailableDiagnosisToolsKey)
	out, err := json.Marshal(top)
	if err != nil {
		return nil, false, fmt.Errorf("diagnosis context: marshal evidence: %w", err)
	}
	if _, err := decodeEvidenceObject(out); err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func availableDiagnosisTool(
	template domain.DiagnosisToolTemplate,
	source domain.AlertSourceProfile,
) AvailableDiagnosisTool {
	tool := AvailableDiagnosisTool{
		TemplateID:           int64(template.ID),
		Name:                 strings.TrimSpace(template.Name),
		AlertSourceProfileID: int64(template.AlertSourceProfileID),
		AlertSourceName:      strings.TrimSpace(source.Name),
		AlertSourceKind:      string(source.Kind),
		SnapshotSourceScope:  availableDiagnosisToolScopeUnscoped,
		Tool:                 string(template.Tool),
		QueryTemplate:        template.QueryTemplate,
		DefaultLimit:         template.DefaultLimit,
		DefaultWindowSeconds: durationSeconds(template.DefaultWindow),
		MaxWindowSeconds:     durationSeconds(template.MaxWindow),
		DefaultStepSeconds:   durationSeconds(template.DefaultStep),
	}
	tool.EvidenceRequest = evidenceRequestExample(tool)
	return tool
}

func evidenceRequestExample(tool AvailableDiagnosisTool) AvailableDiagnosisToolRequestExample {
	example := AvailableDiagnosisToolRequestExample{
		TemplateID:           tool.TemplateID,
		AlertSourceProfileID: tool.AlertSourceProfileID,
		Tool:                 tool.Tool,
		Reason:               evidenceRequestExampleReason(tool.Name),
		Limit:                tool.DefaultLimit,
	}
	switch tool.Tool {
	case string(domain.DiagnosisToolKindMetricQuery):
		if query, ok := diagnosisquery.ResolveExecutableQuery(tool.QueryTemplate, ""); ok {
			example.Query = query
		}
	case string(domain.DiagnosisToolKindMetricRangeQuery):
		if query, ok := diagnosisquery.ResolveExecutableQuery(tool.QueryTemplate, ""); ok {
			example.Query = query
		}
		example.WindowSeconds = tool.DefaultWindowSeconds
		example.StepSeconds = tool.DefaultStepSeconds
	}
	return example
}

func evidenceRequestExampleReason(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Collect bounded evidence with this approved diagnosis tool."
	}
	return fmt.Sprintf("Collect bounded evidence with %s.", name)
}

func applyEvidenceTemplateValues(
	base json.RawMessage,
	tools []AvailableDiagnosisTool,
) []AvailableDiagnosisTool {
	values := diagnosisQueryValuesFromEvidence(base)
	if len(values) == 0 {
		return tools
	}
	out := append([]AvailableDiagnosisTool(nil), tools...)
	for i := range out {
		if out[i].QueryTemplate == "" {
			continue
		}
		for _, value := range values {
			query, ok := diagnosisquery.ExpandTemplate(out[i].QueryTemplate, value)
			if !ok {
				continue
			}
			out[i].EvidenceRequest.Query = query
			break
		}
	}
	return out
}

func filterExecutableMetricDiagnosisTools(tools []AvailableDiagnosisTool) []AvailableDiagnosisTool {
	out := make([]AvailableDiagnosisTool, 0, len(tools))
	for _, tool := range tools {
		if isMetricDiagnosisTool(tool.Tool) && strings.TrimSpace(tool.EvidenceRequest.Query) == "" {
			continue
		}
		out = append(out, tool)
	}
	return out
}

func isMetricDiagnosisTool(tool string) bool {
	return tool == string(domain.DiagnosisToolKindMetricQuery) ||
		tool == string(domain.DiagnosisToolKindMetricRangeQuery)
}

func filterAvailableDiagnosisToolsForEvidence(
	base json.RawMessage,
	tools []AvailableDiagnosisTool,
) []AvailableDiagnosisTool {
	profileIDs := diagnosisSourceProfileIDsFromEvidence(base)
	if len(profileIDs) == 0 {
		return tools
	}
	out := make([]AvailableDiagnosisTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Tool == string(domain.DiagnosisToolKindActiveAlerts) {
			if _, ok := profileIDs[tool.AlertSourceProfileID]; ok {
				out = append(out, tool)
			}
			continue
		}
		if isMetricDiagnosisTool(tool.Tool) {
			out = append(out, tool)
		}
	}
	return out
}

func prioritizeAvailableDiagnosisToolsForEvidence(
	base json.RawMessage,
	tools []AvailableDiagnosisTool,
) []AvailableDiagnosisTool {
	profileIDs := diagnosisSourceProfileIDsFromEvidence(base)
	if len(profileIDs) == 0 {
		return tools
	}
	out := append([]AvailableDiagnosisTool(nil), tools...)
	for i := range out {
		if _, ok := profileIDs[out[i].AlertSourceProfileID]; ok {
			out[i].SnapshotSourceScope = availableDiagnosisToolScopeMatched
		} else {
			out[i].SnapshotSourceScope = availableDiagnosisToolScopeSupplemental
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftScope := availableDiagnosisToolScopePriority(out[i].SnapshotSourceScope)
		rightScope := availableDiagnosisToolScopePriority(out[j].SnapshotSourceScope)
		if leftScope != rightScope {
			return leftScope < rightScope
		}
		leftTool := availableDiagnosisToolKindPriority(out[i].Tool)
		rightTool := availableDiagnosisToolKindPriority(out[j].Tool)
		if leftTool != rightTool {
			return leftTool < rightTool
		}
		return out[i].TemplateID < out[j].TemplateID
	})
	return out
}

func availableDiagnosisToolScopePriority(scope string) int {
	switch scope {
	case availableDiagnosisToolScopeMatched:
		return 0
	case availableDiagnosisToolScopeSupplemental:
		return 1
	default:
		return 2
	}
}

func availableDiagnosisToolKindPriority(tool string) int {
	switch tool {
	case string(domain.DiagnosisToolKindActiveAlerts):
		return 0
	case string(domain.DiagnosisToolKindMetricRangeQuery):
		return 1
	case string(domain.DiagnosisToolKindMetricQuery):
		return 2
	default:
		return 3
	}
}

func diagnosisSourceProfileIDsFromEvidence(base json.RawMessage) map[int64]struct{} {
	var snapshot struct {
		Events []struct {
			AlertSourceProfileID int64 `json:"alert_source_profile_id"`
		} `json:"events"`
	}
	if err := json.Unmarshal(base, &snapshot); err != nil {
		return nil
	}
	profileIDs := make(map[int64]struct{})
	for _, event := range snapshot.Events {
		if event.AlertSourceProfileID <= 0 {
			continue
		}
		profileIDs[event.AlertSourceProfileID] = struct{}{}
	}
	return profileIDs
}

func diagnosisQueryValuesFromEvidence(base json.RawMessage) []diagnosisquery.Values {
	var snapshot struct {
		Events []struct {
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
		} `json:"events"`
	}
	if err := json.Unmarshal(base, &snapshot); err != nil {
		return nil
	}
	values := make([]diagnosisquery.Values, 0, len(snapshot.Events))
	for _, event := range snapshot.Events {
		if len(event.Labels) == 0 && len(event.Annotations) == 0 {
			continue
		}
		values = append(values, diagnosisquery.Values{
			Labels:      event.Labels,
			Annotations: event.Annotations,
		})
	}
	return values
}

func decodeEvidenceObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("diagnosis context: evidence must be non-empty JSON object: %w", domain.ErrInvariantViolation)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("diagnosis context: evidence must be a JSON object: %w", domain.ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return nil, fmt.Errorf("diagnosis context: evidence has duplicate keys: %w", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("diagnosis context: evidence must be valid JSON object: %w", err)
	}
	if top == nil {
		return nil, fmt.Errorf("diagnosis context: evidence must be a JSON object: %w", domain.ErrInvariantViolation)
	}
	return top, nil
}

func durationSeconds(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int(value / time.Second)
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}
