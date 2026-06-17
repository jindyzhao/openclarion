package temporal

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisRoomCollectedEvidenceKey       = "openclarion_collected_evidence"
	maxDiagnosisRoomEvidenceContextBatches  = 5
	maxDiagnosisRoomEvidenceContextWarnings = 5
)

type diagnosisRoomEvidenceContextBatch struct {
	TurnCount          int                                `json:"turn_count"`
	AssistantMessageID string                             `json:"assistant_message_id,omitempty"`
	CollectedAt        time.Time                          `json:"collected_at,omitempty"`
	Items              []diagnosisRoomEvidenceContextItem `json:"items"`
}

type diagnosisRoomEvidenceContextItem struct {
	Request              diagnosisRoomEvidenceContextRequest     `json:"request"`
	TemplateID           int64                                   `json:"template_id,omitempty"`
	AlertSourceProfileID int64                                   `json:"alert_source_profile_id,omitempty"`
	AlertSourceKind      string                                  `json:"alert_source_kind,omitempty"`
	Tool                 string                                  `json:"tool"`
	Status               string                                  `json:"status"`
	ReasonCode           string                                  `json:"reason_code"`
	Message              string                                  `json:"message,omitempty"`
	Limit                int                                     `json:"limit,omitempty"`
	ObservedAlerts       int                                     `json:"observed_alerts,omitempty"`
	ActiveAlerts         []diagnosisRoomEvidenceContextAlert     `json:"active_alerts,omitempty"`
	Query                string                                  `json:"query,omitempty"`
	WindowSeconds        int                                     `json:"window_seconds,omitempty"`
	StepSeconds          int                                     `json:"step_seconds,omitempty"`
	ObservedMetricSeries int                                     `json:"observed_metric_series,omitempty"`
	MetricResult         diagnosisRoomEvidenceContextMetricQuery `json:"metric_result,omitempty"`
	CollectedAt          time.Time                               `json:"collected_at,omitempty"`
}

type diagnosisRoomEvidenceContextRequest struct {
	TemplateID    int64  `json:"template_id,omitempty"`
	Tool          string `json:"tool"`
	Reason        string `json:"reason,omitempty"`
	Query         string `json:"query,omitempty"`
	WindowSeconds int    `json:"window_seconds,omitempty"`
	StepSeconds   int    `json:"step_seconds,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type diagnosisRoomEvidenceContextAlert struct {
	Source      string            `json:"source,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	StartsAt    time.Time         `json:"starts_at,omitempty"`
}

type diagnosisRoomEvidenceContextMetricQuery struct {
	ResultType string                                     `json:"result_type,omitempty"`
	Series     []diagnosisRoomEvidenceContextMetricSeries `json:"series,omitempty"`
	Scalar     *diagnosisRoomEvidenceContextMetricPoint   `json:"scalar,omitempty"`
	String     *diagnosisRoomEvidenceContextMetricPoint   `json:"string,omitempty"`
	Warnings   []string                                   `json:"warnings,omitempty"`
}

type diagnosisRoomEvidenceContextMetricSeries struct {
	Metric map[string]string                         `json:"metric,omitempty"`
	Points []diagnosisRoomEvidenceContextMetricPoint `json:"points,omitempty"`
}

type diagnosisRoomEvidenceContextMetricPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     string    `json:"value"`
}

func diagnosisRoomEvidenceContext(
	base json.RawMessage,
	batches []diagnosisRoomEvidenceContextBatch,
) (json.RawMessage, error) {
	if len(batches) == 0 {
		return cloneRawMessage(base), nil
	}
	if err := validateDiagnosisRoomEvidenceJSON("diagnosis-room: evidence context base", base); err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(base, &top); err != nil {
		return nil, fmt.Errorf("diagnosis-room: unmarshal evidence context base: %w", err)
	}
	trimmed := tailEvidenceContextBatches(batches)
	collected, err := json.Marshal(trimmed)
	if err != nil {
		return nil, fmt.Errorf("diagnosis-room: marshal collected evidence context: %w", err)
	}
	top[diagnosisRoomCollectedEvidenceKey] = collected
	out, err := json.Marshal(top)
	if err != nil {
		return nil, fmt.Errorf("diagnosis-room: marshal evidence context: %w", err)
	}
	if err := validateDiagnosisRoomEvidenceJSON("diagnosis-room: evidence context", out); err != nil {
		return nil, err
	}
	return out, nil
}

func diagnosisRoomEvidenceContextBatchFromItems(
	turnCount int,
	assistantMessageID string,
	items []diagnosisevidence.Item,
) (diagnosisRoomEvidenceContextBatch, bool) {
	if len(items) == 0 {
		return diagnosisRoomEvidenceContextBatch{}, false
	}
	out := diagnosisRoomEvidenceContextBatch{
		TurnCount:          turnCount,
		AssistantMessageID: assistantMessageID,
		Items:              make([]diagnosisRoomEvidenceContextItem, 0, len(items)),
	}
	for _, item := range items {
		if item.CollectedAt.After(out.CollectedAt) {
			out.CollectedAt = item.CollectedAt
		}
		out.Items = append(out.Items, diagnosisRoomEvidenceContextItemFromItem(item))
	}
	return out, true
}

func diagnosisRoomEvidenceContextItemFromItem(item diagnosisevidence.Item) diagnosisRoomEvidenceContextItem {
	return diagnosisRoomEvidenceContextItem{
		Request:              diagnosisRoomEvidenceContextRequestFromItem(item),
		TemplateID:           int64(item.TemplateID),
		AlertSourceProfileID: int64(item.AlertSourceProfileID),
		AlertSourceKind:      string(item.AlertSourceKind),
		Tool:                 string(item.Tool),
		Status:               string(item.Status),
		ReasonCode:           string(item.ReasonCode),
		Message:              item.Message,
		Limit:                item.Limit,
		ObservedAlerts:       item.ObservedAlerts,
		ActiveAlerts:         diagnosisRoomEvidenceContextAlerts(item.ActiveAlerts),
		Query:                item.Query,
		WindowSeconds:        item.WindowSeconds,
		StepSeconds:          item.StepSeconds,
		ObservedMetricSeries: item.ObservedMetricSeries,
		MetricResult:         diagnosisRoomEvidenceContextMetricResult(item.MetricResult),
		CollectedAt:          item.CollectedAt,
	}
}

func diagnosisRoomEvidenceContextRequestFromItem(item diagnosisevidence.Item) diagnosisRoomEvidenceContextRequest {
	return diagnosisRoomEvidenceContextRequest{
		TemplateID:    item.Request.TemplateID,
		Tool:          string(item.Request.Tool),
		Reason:        item.Request.Reason,
		Query:         item.Request.Query,
		WindowSeconds: item.Request.WindowSeconds,
		StepSeconds:   item.Request.StepSeconds,
		Limit:         item.Request.Limit,
	}
}

func diagnosisRoomEvidenceContextAlerts(in []ports.ActiveAlert) []diagnosisRoomEvidenceContextAlert {
	if in == nil {
		return nil
	}
	out := make([]diagnosisRoomEvidenceContextAlert, len(in))
	for i, alert := range in {
		out[i] = diagnosisRoomEvidenceContextAlert{
			Source:      alert.Source,
			Labels:      cloneStringMap(alert.Labels),
			Annotations: cloneStringMap(alert.Annotations),
			StartsAt:    alert.StartsAt,
		}
	}
	return out
}

func diagnosisRoomEvidenceContextMetricResult(in ports.MetricQueryResult) diagnosisRoomEvidenceContextMetricQuery {
	out := diagnosisRoomEvidenceContextMetricQuery{
		ResultType: in.ResultType,
		Warnings:   tailStringSlice(in.Warnings, maxDiagnosisRoomEvidenceContextWarnings),
	}
	if in.Scalar != nil {
		scalar := diagnosisRoomEvidenceContextMetricPointFromPort(*in.Scalar)
		out.Scalar = &scalar
	}
	if in.String != nil {
		value := diagnosisRoomEvidenceContextMetricPointFromPort(*in.String)
		out.String = &value
	}
	if in.Series != nil {
		out.Series = make([]diagnosisRoomEvidenceContextMetricSeries, len(in.Series))
		for i, series := range in.Series {
			out.Series[i] = diagnosisRoomEvidenceContextMetricSeries{
				Metric: cloneStringMap(series.Metric),
				Points: diagnosisRoomEvidenceContextMetricPoints(series.Points),
			}
		}
	}
	return out
}

func diagnosisRoomEvidenceContextMetricPoints(in []ports.MetricPoint) []diagnosisRoomEvidenceContextMetricPoint {
	if in == nil {
		return nil
	}
	out := make([]diagnosisRoomEvidenceContextMetricPoint, len(in))
	for i, point := range in {
		out[i] = diagnosisRoomEvidenceContextMetricPoint(point)
	}
	return out
}

func diagnosisRoomEvidenceContextMetricPointFromPort(point ports.MetricPoint) diagnosisRoomEvidenceContextMetricPoint {
	return diagnosisRoomEvidenceContextMetricPoint{
		Timestamp: point.Timestamp,
		Value:     point.Value,
	}
}

func tailEvidenceContextBatches(
	in []diagnosisRoomEvidenceContextBatch,
) []diagnosisRoomEvidenceContextBatch {
	if len(in) <= maxDiagnosisRoomEvidenceContextBatches {
		return cloneEvidenceContextBatches(in)
	}
	return cloneEvidenceContextBatches(in[len(in)-maxDiagnosisRoomEvidenceContextBatches:])
}

func cloneEvidenceContextBatches(in []diagnosisRoomEvidenceContextBatch) []diagnosisRoomEvidenceContextBatch {
	if in == nil {
		return nil
	}
	out := make([]diagnosisRoomEvidenceContextBatch, len(in))
	for i, batch := range in {
		out[i] = batch
		out[i].Items = cloneEvidenceContextItems(batch.Items)
	}
	return out
}

func cloneEvidenceContextItems(in []diagnosisRoomEvidenceContextItem) []diagnosisRoomEvidenceContextItem {
	if in == nil {
		return nil
	}
	out := make([]diagnosisRoomEvidenceContextItem, len(in))
	for i, item := range in {
		out[i] = item
		out[i].ActiveAlerts = cloneEvidenceContextAlerts(item.ActiveAlerts)
		out[i].MetricResult = cloneEvidenceContextMetricResult(item.MetricResult)
	}
	return out
}

func cloneEvidenceContextAlerts(in []diagnosisRoomEvidenceContextAlert) []diagnosisRoomEvidenceContextAlert {
	if in == nil {
		return nil
	}
	out := make([]diagnosisRoomEvidenceContextAlert, len(in))
	for i, alert := range in {
		out[i] = diagnosisRoomEvidenceContextAlert{
			Source:      alert.Source,
			Labels:      cloneStringMap(alert.Labels),
			Annotations: cloneStringMap(alert.Annotations),
			StartsAt:    alert.StartsAt,
		}
	}
	return out
}

func cloneEvidenceContextMetricResult(in diagnosisRoomEvidenceContextMetricQuery) diagnosisRoomEvidenceContextMetricQuery {
	out := diagnosisRoomEvidenceContextMetricQuery{
		ResultType: in.ResultType,
		Warnings:   append([]string(nil), in.Warnings...),
	}
	if in.Scalar != nil {
		scalar := *in.Scalar
		out.Scalar = &scalar
	}
	if in.String != nil {
		value := *in.String
		out.String = &value
	}
	if in.Series != nil {
		out.Series = make([]diagnosisRoomEvidenceContextMetricSeries, len(in.Series))
		for i, series := range in.Series {
			out.Series[i] = diagnosisRoomEvidenceContextMetricSeries{
				Metric: cloneStringMap(series.Metric),
				Points: append([]diagnosisRoomEvidenceContextMetricPoint(nil), series.Points...),
			}
		}
	}
	return out
}

func tailStringSlice(in []string, limit int) []string {
	if in == nil {
		return nil
	}
	if limit > 0 && len(in) > limit {
		in = in[len(in)-limit:]
	}
	return append([]string(nil), in...)
}

func appendDiagnosisRoomEvidenceContextBatch(
	in []diagnosisRoomEvidenceContextBatch,
	batch diagnosisRoomEvidenceContextBatch,
) []diagnosisRoomEvidenceContextBatch {
	out := append(cloneEvidenceContextBatches(in), batch)
	return tailEvidenceContextBatches(out)
}

func diagnosisRoomEvidenceContextError(err error) error {
	return fmt.Errorf("%w: %w", domain.ErrInvariantViolation, err)
}
