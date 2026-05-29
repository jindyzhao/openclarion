// Package reportdraft defines the structured JSON contracts used by
// the M2 headless report loop before persistence schemas exist.
package reportdraft

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/openclarion/openclarion/internal/usecases/llmoutput"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	// SubReportSchemaID is the response_format schema name for one
	// per-group report draft.
	SubReportSchemaID = "openclarion_sub_report"
	// FinalReportSchemaID is the response_format schema name for the
	// fan-in report draft.
	FinalReportSchemaID = "openclarion_final_report"
)

const (
	maxTitleRunes        = 160
	maxSummaryRunes      = 4000
	maxLabelRunes        = 120
	maxDetailRunes       = 2000
	maxEvidenceIDRunes   = 120
	maxSubReportFindings = 20
	maxSubReportActions  = 20
	maxEvidenceRefs      = 50
	maxFinalSubReports   = 50
	maxFinalActions      = 30
)

// Severity is the report severity vocabulary shared by subreports and
// final reports.
type Severity string

const (
	// SeverityInfo means the report is informational.
	SeverityInfo Severity = "info"
	// SeverityWarning means the report requires operator attention.
	SeverityWarning Severity = "warning"
	// SeverityCritical means the report represents critical impact.
	SeverityCritical Severity = "critical"
)

// Confidence captures how strongly the report is supported by the
// evidence snapshot.
type Confidence string

const (
	// ConfidenceLow means the draft is weakly supported by evidence.
	ConfidenceLow Confidence = "low"
	// ConfidenceMedium means the draft is moderately supported by evidence.
	ConfidenceMedium Confidence = "medium"
	// ConfidenceHigh means the draft is strongly supported by evidence.
	ConfidenceHigh Confidence = "high"
)

// Priority is the operator-action priority vocabulary.
type Priority string

const (
	// PriorityLow is a low-priority operator action.
	PriorityLow Priority = "low"
	// PriorityMedium is a medium-priority operator action.
	PriorityMedium Priority = "medium"
	// PriorityHigh is a high-priority operator action.
	PriorityHigh Priority = "high"
)

// Finding is one evidence-backed observation.
type Finding struct {
	Label      string `json:"label"`
	Detail     string `json:"detail"`
	EvidenceID string `json:"evidence_id"`
}

// Action is one recommended operator action.
type Action struct {
	Label    string   `json:"label"`
	Detail   string   `json:"detail"`
	Priority Priority `json:"priority"`
}

// SubReport is the structured output for one alert group.
type SubReport struct {
	Title              string     `json:"title"`
	Summary            string     `json:"summary"`
	Severity           Severity   `json:"severity"`
	Confidence         Confidence `json:"confidence"`
	Findings           []Finding  `json:"findings"`
	RecommendedActions []Action   `json:"recommended_actions"`
	EvidenceRefs       []string   `json:"evidence_refs"`
}

// SubReportSummary is the final-report fan-in projection of a
// validated SubReport.
type SubReportSummary struct {
	Title    string   `json:"title"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
}

// FinalReport is the structured output for the incident-level report.
type FinalReport struct {
	Title              string             `json:"title"`
	ExecutiveSummary   string             `json:"executive_summary"`
	Severity           Severity           `json:"severity"`
	Confidence         Confidence         `json:"confidence"`
	SubReports         []SubReportSummary `json:"sub_reports"`
	RecommendedActions []Action           `json:"recommended_actions"`
	NotificationText   string             `json:"notification_text"`
}

// SubReportSchema returns a copy of the strict JSON Schema for
// SubReport.
func SubReportSchema() json.RawMessage {
	return cloneRawMessage(subReportSchema)
}

// FinalReportSchema returns a copy of the strict JSON Schema for
// FinalReport.
func FinalReportSchema() json.RawMessage {
	return cloneRawMessage(finalReportSchema)
}

// ParseSubReport validates resp against the SubReport schema and
// unmarshals the accepted JSON into a typed draft.
func ParseSubReport(resp ports.LLMResponse) (SubReport, error) {
	accepted, err := llmoutput.Validate(schemaRequest(SubReportSchemaID, subReportSchema), resp)
	if err != nil {
		return SubReport{}, err
	}
	var out SubReport
	if err := json.Unmarshal(accepted.Content, &out); err != nil {
		return SubReport{}, fmt.Errorf("parse sub report: %w", err)
	}
	if err := validateSubReport(out); err != nil {
		return SubReport{}, err
	}
	return out, nil
}

// ParseFinalReport validates resp against the FinalReport schema and
// unmarshals the accepted JSON into a typed draft.
func ParseFinalReport(resp ports.LLMResponse) (FinalReport, error) {
	accepted, err := llmoutput.Validate(schemaRequest(FinalReportSchemaID, finalReportSchema), resp)
	if err != nil {
		return FinalReport{}, err
	}
	var out FinalReport
	if err := json.Unmarshal(accepted.Content, &out); err != nil {
		return FinalReport{}, fmt.Errorf("parse final report: %w", err)
	}
	if err := validateFinalReport(out); err != nil {
		return FinalReport{}, err
	}
	return out, nil
}

func schemaRequest(id string, schema json.RawMessage) ports.LLMRequest {
	return ports.LLMRequest{
		OutputSchemaID: id,
		OutputSchema:   schema,
		IdempotencyKey: id + "/parse",
	}
}

func cloneRawMessage(in json.RawMessage) json.RawMessage {
	if in == nil {
		return nil
	}
	out := make(json.RawMessage, len(in))
	copy(out, in)
	return out
}

func validateSubReport(out SubReport) error {
	if err := validateCommonReportFields(out.Title, "summary", out.Summary, out.Severity, out.Confidence); err != nil {
		return reportSchemaViolation(err)
	}
	if len(out.Findings) == 0 {
		return reportSchemaViolation(fmt.Errorf("findings must contain at least one item"))
	}
	if len(out.Findings) > maxSubReportFindings {
		return reportSchemaViolation(fmt.Errorf("findings contains %d items, max %d", len(out.Findings), maxSubReportFindings))
	}
	for i, finding := range out.Findings {
		if err := validateFinding(i, finding); err != nil {
			return reportSchemaViolation(err)
		}
	}
	if len(out.RecommendedActions) > maxSubReportActions {
		return reportSchemaViolation(fmt.Errorf("recommended_actions contains %d items, max %d", len(out.RecommendedActions), maxSubReportActions))
	}
	for i, action := range out.RecommendedActions {
		if err := validateAction(i, action); err != nil {
			return reportSchemaViolation(err)
		}
	}
	if len(out.EvidenceRefs) > maxEvidenceRefs {
		return reportSchemaViolation(fmt.Errorf("evidence_refs contains %d items, max %d", len(out.EvidenceRefs), maxEvidenceRefs))
	}
	for i, ref := range out.EvidenceRefs {
		if err := validateRequiredString(fmt.Sprintf("evidence_refs[%d]", i), ref, maxEvidenceIDRunes); err != nil {
			return reportSchemaViolation(err)
		}
	}
	return nil
}

func validateFinalReport(out FinalReport) error {
	if err := validateCommonReportFields(out.Title, "executive_summary", out.ExecutiveSummary, out.Severity, out.Confidence); err != nil {
		return reportSchemaViolation(err)
	}
	if len(out.SubReports) == 0 {
		return reportSchemaViolation(fmt.Errorf("sub_reports must contain at least one item"))
	}
	if len(out.SubReports) > maxFinalSubReports {
		return reportSchemaViolation(fmt.Errorf("sub_reports contains %d items, max %d", len(out.SubReports), maxFinalSubReports))
	}
	for i, subReport := range out.SubReports {
		if err := validateSubReportSummary(i, subReport); err != nil {
			return reportSchemaViolation(err)
		}
	}
	if len(out.RecommendedActions) > maxFinalActions {
		return reportSchemaViolation(fmt.Errorf("recommended_actions contains %d items, max %d", len(out.RecommendedActions), maxFinalActions))
	}
	for i, action := range out.RecommendedActions {
		if err := validateAction(i, action); err != nil {
			return reportSchemaViolation(err)
		}
	}
	if err := validateRequiredString("notification_text", out.NotificationText, maxDetailRunes); err != nil {
		return reportSchemaViolation(err)
	}
	return nil
}

func validateCommonReportFields(title string, summaryPath string, summary string, severity Severity, confidence Confidence) error {
	if err := validateRequiredString("title", title, maxTitleRunes); err != nil {
		return err
	}
	if err := validateRequiredString(summaryPath, summary, maxSummaryRunes); err != nil {
		return err
	}
	if !validSeverity(severity) {
		return fmt.Errorf("severity %q is invalid", severity)
	}
	if !validConfidence(confidence) {
		return fmt.Errorf("confidence %q is invalid", confidence)
	}
	return nil
}

func validateFinding(index int, finding Finding) error {
	prefix := fmt.Sprintf("findings[%d]", index)
	if err := validateRequiredString(prefix+".label", finding.Label, maxLabelRunes); err != nil {
		return err
	}
	if err := validateRequiredString(prefix+".detail", finding.Detail, maxDetailRunes); err != nil {
		return err
	}
	if err := validateRequiredString(prefix+".evidence_id", finding.EvidenceID, maxEvidenceIDRunes); err != nil {
		return err
	}
	return nil
}

func validateAction(index int, action Action) error {
	prefix := fmt.Sprintf("recommended_actions[%d]", index)
	if err := validateRequiredString(prefix+".label", action.Label, maxLabelRunes); err != nil {
		return err
	}
	if err := validateRequiredString(prefix+".detail", action.Detail, maxDetailRunes); err != nil {
		return err
	}
	if !validPriority(action.Priority) {
		return fmt.Errorf("%s.priority %q is invalid", prefix, action.Priority)
	}
	return nil
}

func validateSubReportSummary(index int, summary SubReportSummary) error {
	prefix := fmt.Sprintf("sub_reports[%d]", index)
	if err := validateRequiredString(prefix+".title", summary.Title, maxTitleRunes); err != nil {
		return err
	}
	if err := validateRequiredString(prefix+".summary", summary.Summary, 1000); err != nil {
		return err
	}
	if !validSeverity(summary.Severity) {
		return fmt.Errorf("%s.severity %q is invalid", prefix, summary.Severity)
	}
	return nil
}

func validateRequiredString(path string, value string, maxRunes int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be non-empty", path)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s exceeds %d runes", path, maxRunes)
	}
	return nil
}

func validSeverity(value Severity) bool {
	switch value {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	default:
		return false
	}
}

func validConfidence(value Confidence) bool {
	switch value {
	case ConfidenceLow, ConfidenceMedium, ConfidenceHigh:
		return true
	default:
		return false
	}
}

func validPriority(value Priority) bool {
	switch value {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return true
	default:
		return false
	}
}

func reportSchemaViolation(err error) error {
	return &llmoutput.Error{
		Reason:    llmoutput.ReasonSchemaViolation,
		Retryable: true,
		Err:       err,
	}
}

var subReportSchema = json.RawMessage(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["title", "summary", "severity", "confidence", "findings", "recommended_actions", "evidence_refs"],
  "properties": {
    "title": {"type": "string", "pattern": "\\S"},
    "summary": {"type": "string", "pattern": "\\S"},
    "severity": {"type": "string", "enum": ["info", "warning", "critical"]},
    "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
    "findings": {
      "type": "array",
      "minItems": 1,
      "maxItems": 20,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["label", "detail", "evidence_id"],
        "properties": {
          "label": {"type": "string", "pattern": "\\S"},
          "detail": {"type": "string", "pattern": "\\S"},
          "evidence_id": {"type": "string", "pattern": "\\S"}
        }
      }
    },
    "recommended_actions": {
      "type": "array",
      "maxItems": 20,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["label", "detail", "priority"],
        "properties": {
          "label": {"type": "string", "pattern": "\\S"},
          "detail": {"type": "string", "pattern": "\\S"},
          "priority": {"type": "string", "enum": ["low", "medium", "high"]}
        }
      }
    },
    "evidence_refs": {
      "type": "array",
      "maxItems": 50,
      "items": {"type": "string", "pattern": "\\S"}
    }
  }
}`)

var finalReportSchema = json.RawMessage(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": ["title", "executive_summary", "severity", "confidence", "sub_reports", "recommended_actions", "notification_text"],
  "properties": {
    "title": {"type": "string", "pattern": "\\S"},
    "executive_summary": {"type": "string", "pattern": "\\S"},
    "severity": {"type": "string", "enum": ["info", "warning", "critical"]},
    "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
    "sub_reports": {
      "type": "array",
      "minItems": 1,
      "maxItems": 50,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["title", "severity", "summary"],
        "properties": {
          "title": {"type": "string", "pattern": "\\S"},
          "severity": {"type": "string", "enum": ["info", "warning", "critical"]},
          "summary": {"type": "string", "pattern": "\\S"}
        }
      }
    },
    "recommended_actions": {
      "type": "array",
      "maxItems": 30,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["label", "detail", "priority"],
        "properties": {
          "label": {"type": "string", "pattern": "\\S"},
          "detail": {"type": "string", "pattern": "\\S"},
          "priority": {"type": "string", "enum": ["low", "medium", "high"]}
        }
      }
    },
    "notification_text": {"type": "string", "pattern": "\\S"}
  }
}`)
