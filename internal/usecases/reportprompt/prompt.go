// Package reportprompt builds provider-neutral LLM requests for the
// M2 headless report loop.
package reportprompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
	"github.com/openclarion/openclarion/internal/usecases/reportdraft"
)

// Scenario identifies the report prompt variant selected for an alert
// group.
type Scenario string

const (
	// ScenarioSingleAlert is for one isolated alert group.
	ScenarioSingleAlert Scenario = "single_alert"
	// ScenarioCascade is for causally related alerts across services.
	ScenarioCascade Scenario = "cascade"
	// ScenarioAlertStorm is for broad alert bursts with shared context.
	ScenarioAlertStorm Scenario = "alert_storm"
)

// Valid reports whether s is a supported prompt scenario.
func (s Scenario) Valid() bool {
	switch s {
	case ScenarioSingleAlert, ScenarioCascade, ScenarioAlertStorm:
		return true
	default:
		return false
	}
}

// SubReportInput holds the evidence needed to draft one SubReport.
type SubReportInput struct {
	Snapshot          domain.EvidenceSnapshot
	Scenario          Scenario
	GroupIndex        int
	HistoricalReports []HistoricalReport
}

// HistoricalReport is bounded advisory context from a previously accepted
// report. It is never a substitute for the current EvidenceSnapshot.
type HistoricalReport struct {
	SourceRef      string                     `json:"source_ref"`
	SourceKind     domain.RetrievalSourceKind `json:"source_kind"`
	Content        string                     `json:"content"`
	CosineDistance float64                    `json:"cosine_distance"`
}

// FinalReportInput holds validated SubReports for the fan-in draft.
type FinalReportInput struct {
	CorrelationKey string
	SubReports     []reportdraft.SubReport
}

// BuildSubReportRequest returns a strict-JSON LLM request for one
// snapshot-backed SubReport. The idempotency key identifies the immutable
// snapshot/group/scenario report projection; advisory corpus changes do not
// create a second logical report.
func BuildSubReportRequest(in SubReportInput) (ports.LLMRequest, error) {
	if in.Snapshot.ID == 0 {
		return ports.LLMRequest{}, fmt.Errorf("report prompt: snapshot ID must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if in.Snapshot.Digest == "" {
		return ports.LLMRequest{}, fmt.Errorf("report prompt: snapshot digest must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if in.GroupIndex < 0 {
		return ports.LLMRequest{}, fmt.Errorf("report prompt: group index must be >= 0: %w", domain.ErrInvariantViolation)
	}
	if !in.Scenario.Valid() {
		return ports.LLMRequest{}, fmt.Errorf("report prompt: scenario %q is unsupported: %w", in.Scenario, domain.ErrInvariantViolation)
	}
	payload, err := compactJSON("snapshot payload", in.Snapshot.Payload)
	if err != nil {
		return ports.LLMRequest{}, err
	}

	historicalReports, err := validateHistoricalReports(in.HistoricalReports)
	if err != nil {
		return ports.LLMRequest{}, err
	}

	return ports.LLMRequest{
		Messages: []ports.LLMMessage{
			{Role: ports.LLMRoleSystem, Content: subReportSystemPrompt},
			{Role: ports.LLMRoleUser, Content: subReportUserPrompt(in, payload, historicalReports)},
		},
		OutputSchema:   reportdraft.SubReportSchema(),
		OutputSchemaID: reportdraft.SubReportSchemaID,
		IdempotencyKey: SubReportIdempotencyKey(in.Snapshot.ID, in.GroupIndex, in.Scenario),
	}, nil
}

// BuildFinalReportRequest returns a strict-JSON LLM request that
// reduces validated SubReports into one FinalReport draft.
func BuildFinalReportRequest(in FinalReportInput) (ports.LLMRequest, error) {
	correlationKey := strings.TrimSpace(in.CorrelationKey)
	if correlationKey == "" {
		return ports.LLMRequest{}, fmt.Errorf("report prompt: correlation key must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if len(in.SubReports) == 0 {
		return ports.LLMRequest{}, fmt.Errorf("report prompt: subreports must be non-empty: %w", domain.ErrInvariantViolation)
	}
	subReports, err := marshalCompact("subreports", in.SubReports)
	if err != nil {
		return ports.LLMRequest{}, err
	}

	return ports.LLMRequest{
		Messages: []ports.LLMMessage{
			{Role: ports.LLMRoleSystem, Content: finalReportSystemPrompt},
			{Role: ports.LLMRoleUser, Content: finalReportUserPrompt(correlationKey, subReports)},
		},
		OutputSchema:   reportdraft.FinalReportSchema(),
		OutputSchemaID: reportdraft.FinalReportSchemaID,
		IdempotencyKey: "final_report:" + correlationKey,
	}, nil
}

// SubReportIdempotencyKey identifies one scenario-specific report projection.
func SubReportIdempotencyKey(snapshotID domain.EvidenceSnapshotID, groupIndex int, scenario Scenario) string {
	return fmt.Sprintf("snapshot:%d/group:%d/scenario:%s/sub_report", snapshotID, groupIndex, scenario)
}

func subReportUserPrompt(in SubReportInput, payload, historicalReports string) string {
	prompt := fmt.Sprintf(`Task: produce one SubReport as JSON for OpenClarion.
Scenario: %s
Scenario guidance: %s
Evidence snapshot id: %d
Evidence snapshot ref: snapshot:%d
Evidence snapshot digest: %s
Group index: %d

Use only the evidence in this snapshot. Do not invent evidence IDs. Include the evidence snapshot ref in evidence_refs. If evidence is weak, set confidence to low and explain the uncertainty in the summary.
Return all required top-level fields: title, summary, severity, confidence, findings, recommended_actions, and evidence_refs.
Every findings item must include label, detail, and evidence_id. Every findings[].evidence_id must appear verbatim in evidence_refs. A safe valid choice is to use the evidence snapshot ref snapshot:%d as each finding evidence_id and include snapshot:%d in evidence_refs.
Every recommended_actions item must include label, detail, and priority. Use priority only as low, medium, or high.

Evidence snapshot JSON:
%s`, in.Scenario, scenarioGuidance(in.Scenario), in.Snapshot.ID, in.Snapshot.ID, in.Snapshot.Digest, in.GroupIndex, in.Snapshot.ID, in.Snapshot.ID, payload)
	if historicalReports == "" {
		return prompt
	}
	return prompt + fmt.Sprintf(`

Historical accepted reports (advisory context only; they may be stale):
%s

Use historical reports only to form hypotheses. Do not treat them as current evidence, do not copy their evidence IDs, and do not put their source_ref values in evidence_refs. Every claim and evidence_refs value in this SubReport must remain supported by the current Evidence snapshot JSON above.`, historicalReports)
}

func validateHistoricalReports(reports []HistoricalReport) (string, error) {
	if len(reports) == 0 {
		return "", nil
	}
	if len(reports) > domain.RetrievalReferenceLimit {
		return "", fmt.Errorf("report prompt: historical reports exceed %d values: %w", domain.RetrievalReferenceLimit, domain.ErrInvariantViolation)
	}
	seen := make(map[string]struct{}, len(reports))
	validated := make([]HistoricalReport, len(reports))
	for i, report := range reports {
		report.SourceRef = strings.TrimSpace(report.SourceRef)
		kind, _, err := domain.ParseRetrievalSourceRef(report.SourceRef)
		if err != nil || kind != report.SourceKind {
			return "", fmt.Errorf("report prompt: historical report[%d] source is invalid: %w", i, domain.ErrInvariantViolation)
		}
		if _, duplicate := seen[report.SourceRef]; duplicate {
			return "", fmt.Errorf("report prompt: historical report[%d] duplicates %q: %w", i, report.SourceRef, domain.ErrInvariantViolation)
		}
		seen[report.SourceRef] = struct{}{}
		report.Content = strings.TrimSpace(report.Content)
		if report.Content == "" || len([]byte(report.Content)) > domain.RetrievalChunkMaxBytes {
			return "", fmt.Errorf("report prompt: historical report[%d] content is invalid: %w", i, domain.ErrInvariantViolation)
		}
		if math.IsNaN(report.CosineDistance) || math.IsInf(report.CosineDistance, 0) || report.CosineDistance < 0 || report.CosineDistance > 2 {
			return "", fmt.Errorf("report prompt: historical report[%d] distance is invalid: %w", i, domain.ErrInvariantViolation)
		}
		validated[i] = report
	}
	raw, err := marshalCompact("historical reports", validated)
	if err != nil {
		return "", err
	}
	if len([]byte(raw)) > domain.RetrievalContextMaxBytes {
		return "", fmt.Errorf("report prompt: serialized historical reports exceed %d bytes: %w", domain.RetrievalContextMaxBytes, domain.ErrInvariantViolation)
	}
	return raw, nil
}

func finalReportUserPrompt(correlationKey string, subReports string) string {
	return fmt.Sprintf(`Task: produce one FinalReport as JSON for OpenClarion.
Correlation key: %s

Use only these validated SubReports. Do not add facts that are not present in the SubReport JSON. Notification text must be concise and operator-facing.
Return all required top-level fields: title, executive_summary, severity, confidence, sub_reports, recommended_actions, and notification_text.
Every recommended_actions item must include label, detail, and priority. Use priority only as low, medium, or high.

Validated SubReports JSON:
%s`, correlationKey, subReports)
}

func scenarioGuidance(s Scenario) string {
	switch s {
	case ScenarioSingleAlert:
		return "Treat the evidence as one isolated alert group and focus on direct operator impact."
	case ScenarioCascade:
		return "Look for causal ordering across services and identify the most likely upstream symptom."
	case ScenarioAlertStorm:
		return "Summarize the common trigger across many related alerts and avoid one finding per duplicate alert."
	default:
		return "Use the generic OpenClarion incident report structure."
	}
}

func compactJSON(label string, raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("report prompt: %s must be non-empty: %w", label, domain.ErrInvariantViolation)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return "", fmt.Errorf("report prompt: %s must be valid JSON: %w", label, domain.ErrInvariantViolation)
	}
	return buf.String(), nil
}

func marshalCompact(label string, value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("report prompt: marshal %s: %w", label, err)
	}
	return compactJSON(label, raw)
}

const subReportSystemPrompt = `You are OpenClarion's headless incident SubReport writer.
Return only JSON that matches the supplied schema. Do not include markdown, prose outside JSON, or tool-call text.
Use evidence IDs exactly as supplied. Prefer concise operational language over speculation.
For findings, use only label, detail, and evidence_id fields.
For recommended_actions, use only label, detail, and priority fields; do not use an action field.`

const finalReportSystemPrompt = `You are OpenClarion's headless incident FinalReport writer.
Return only JSON that matches the supplied schema. Do not include markdown, prose outside JSON, or tool-call text.
Reduce validated SubReports into one operator-facing incident report and preserve uncertainty.
For recommended_actions, use only label, detail, and priority fields; do not use an action field.`
