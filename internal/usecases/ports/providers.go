package ports

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
)

// ActiveAlert is the minimal projection of an upstream metrics
// provider's active (firing) alert. Concrete providers translate
// their native payload into this DTO before the ingestion library
// converts it to a domain.AlertEvent.
//
// Field semantics:
//
//   - Source: provider source identifier (e.g. "prometheus"). The
//     ingestion library forwards this value verbatim into
//     domain.AlertEvent.Source where it participates in the
//     (source, canonical_fingerprint, starts_at) natural unique key.
//   - Labels / Annotations: free-form key/value metadata. Both MAY be
//     nil or empty; the ingestion library normalises nil to an empty
//     map before fingerprinting and before constructing the domain
//     entity, so downstream behaviour is identical either way.
//   - StartsAt: alert activation time. MUST be non-zero; the
//     ingestion library forwards it to domain.NewAlertEvent which
//     rejects the zero value as an invariant violation. Time-zone is
//     normalised to UTC by the domain constructor.
//   - RawPayload: provider's native JSON representation of the
//     alert. MAY be nil; the persistence layer treats the column as
//     optional JSONB.
type ActiveAlert struct {
	Source      string
	Labels      map[string]string
	Annotations map[string]string
	StartsAt    time.Time
	RawPayload  json.RawMessage
}

// MetricPoint is one timestamped Prometheus sample value rendered as a string.
// Prometheus transports sample values as strings so NaN/Inf remain representable
// in JSON; the port preserves that shape instead of forcing lossy float parsing.
type MetricPoint struct {
	Timestamp time.Time
	Value     string
}

// MetricSeries is the provider-neutral summary for one returned time series.
type MetricSeries struct {
	Metric map[string]string
	Points []MetricPoint
}

// MetricQueryRequest configures one bounded instant Prometheus query.
type MetricQueryRequest struct {
	Query   string
	Time    time.Time
	Timeout time.Duration
	Limit   int
}

// MetricRangeQueryRequest configures one bounded Prometheus range query.
type MetricRangeQueryRequest struct {
	Query   string
	Start   time.Time
	End     time.Time
	Step    time.Duration
	Timeout time.Duration
	Limit   int
}

// MetricQueryResult is the sanitized result summary returned by Prometheus-like
// providers. ResultType is the upstream Prometheus result type: vector, matrix,
// scalar, or string.
type MetricQueryResult struct {
	ResultType string
	Series     []MetricSeries
	Scalar     *MetricPoint
	String     *MetricPoint
	Warnings   []string
}

// MetricsProvider is the upstream alert source contract. Each call
// to ListActiveAlerts independently queries the upstream system and
// returns the currently-firing alerts; the provider MUST NOT carry
// across-call state that affects the returned set.
//
// Layering rules:
//
//   - This package (usecase-facing DTOs / ports) MUST depend only
//     on the Go standard library and the domain package, so the
//     usecase layer stays import-clean.
//   - Concrete providers live under internal/providers/metrics/<impl>
//     and MAY import third-party SDKs (e.g. github.com/prometheus/
//     client_golang) as needed; in exchange they MUST NOT be
//     imported by anything inside internal/usecases or
//     internal/domain, which enforces one-way dependency from
//     concrete providers towards this port.
//
// Provider-side filtering policy: implementations are responsible
// for filtering out non-firing states (Prometheus "pending" /
// "inactive", etc.) so the DTO never carries alerts the domain
// model would reject. Consumers MAY assume every returned alert is
// firing.
type MetricsProvider interface {
	ListActiveAlerts(ctx context.Context) ([]ActiveAlert, error)
	QueryMetric(ctx context.Context, req MetricQueryRequest) (MetricQueryResult, error)
	QueryMetricRange(ctx context.Context, req MetricRangeQueryRequest) (MetricQueryResult, error)
}

// ErrSecretNotFound is returned by SecretResolver implementations when the
// referenced deployment-managed secret does not exist in the configured
// resolver boundary. The error intentionally does not include the secret value.
var ErrSecretNotFound = errors.New("secret not found")

// Secret is the provider-neutral resolved secret value. It is intentionally
// small until OpenClarion needs credential shapes beyond bearer tokens.
type Secret struct {
	Value string
}

// SecretResolver resolves deployment-managed secret references into short-lived
// in-process values for backend provider calls. Resolved values must never be
// returned through OpenAPI responses, frontend state, logs, or retained proof
// artifacts.
type SecretResolver interface {
	ResolveSecret(ctx context.Context, ref string) (Secret, error)
}

// AuthRole is the small V1 role vocabulary understood by usecase-level
// authorization checks. OIDC providers map upstream claims into these values
// before the control plane evaluates RBAC.
type AuthRole string

const (
	// AuthRoleOwner permits access to diagnosis sessions owned by the subject.
	AuthRoleOwner AuthRole = "owner"
	// AuthRoleAdmin permits access to diagnosis sessions across owners.
	AuthRoleAdmin AuthRole = "admin"
)

// AuthPrincipal is the provider-neutral identity returned after bearer-token
// authentication. Subject is the stable user identifier used for ownership
// checks and audit records.
type AuthPrincipal struct {
	Subject string
	Roles   []AuthRole
	Claims  json.RawMessage
}

// AuthProvider authenticates an inbound bearer token and maps upstream claims
// into OpenClarion's V1 role vocabulary. It does not authorize access to a
// specific diagnosis session; usecase code owns RBAC.
type AuthProvider interface {
	AuthenticateBearer(ctx context.Context, bearerToken string) (AuthPrincipal, error)
}

// LLMMessageRole is the small role vocabulary accepted by the
// provider-neutral LLM request DTO. It intentionally mirrors the
// OpenAI-compatible chat-completions roles without importing any
// provider SDK into the usecase layer.
type LLMMessageRole string

const (
	// LLMRoleSystem carries system/developer instructions for a model.
	LLMRoleSystem LLMMessageRole = "system"
	// LLMRoleUser carries user or validation-feedback content.
	LLMRoleUser LLMMessageRole = "user"
	// LLMRoleAssistant carries prior assistant content during retry.
	LLMRoleAssistant LLMMessageRole = "assistant"
)

// LLMOutputMode records which output-format capability the concrete
// provider used for a request. OpenAI-compatible implementations
// should prefer strict JSON Schema outputs when available and fall
// back to json_object only when strict mode is unsupported.
type LLMOutputMode string

const (
	// LLMOutputModeJSONSchema means strict JSON Schema output was used.
	LLMOutputModeJSONSchema LLMOutputMode = "json_schema"
	// LLMOutputModeJSONObject means json_object fallback output was used.
	LLMOutputModeJSONObject LLMOutputMode = "json_object"
)

// LLMMessage is one chat message passed to an LLMProvider.
type LLMMessage struct {
	Role    LLMMessageRole
	Content string
}

// LLMRequest is the provider-neutral input for one headless report
// generation call. The caller owns prompt construction, schema
// selection, validation, and retry policy; the provider only performs
// the external model call and returns the raw assistant JSON payload
// plus completion metadata.
//
// IdempotencyKey is required by Temporal activities that call the
// provider so activity retry can be correlated by the concrete
// provider or by deterministic fakes. For M2 report generation the
// intended key shape is snapshotID + groupIndex.
type LLMRequest struct {
	Messages       []LLMMessage
	OutputSchema   json.RawMessage
	OutputSchemaID string
	IdempotencyKey string
}

// LLMResponse is the provider-neutral output from one model call.
// Content is intentionally raw JSON: callers must still validate it
// against OutputSchema, and must reject non-stop finish reasons or
// non-nil Refusal before persistence.
type LLMResponse struct {
	Content      json.RawMessage
	FinishReason string
	Refusal      *string
	OutputMode   LLMOutputMode
	Model        string
}

// LLMProvider is the headless structured-output generation contract.
//
// Responsibilities:
//   - execute one OpenAI-compatible chat-completions style request;
//   - return the assistant JSON payload as json.RawMessage;
//   - surface finish_reason, refusal, model, and selected output mode
//     so the caller can enforce acceptance rules before persistence.
//
// Non-responsibilities:
//   - constructing prompts or choosing report sections;
//   - validating output against JSON Schema;
//   - retrying validation failures;
//   - persisting SubReport / FinalReport rows;
//   - notifying users.
type LLMProvider interface {
	GenerateJSON(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

// IMNotification is the provider-neutral outbound notification DTO.
// Workflows own persistence and ordering; IM providers only deliver
// already-accepted operator-facing messages.
//
// IdempotencyKey is required so Temporal Activity retries can be
// correlated by webhook receivers and deterministic fakes. For M2
// final report notification the intended key shape is
// "final_report:<id>/notification"; M5 diagnosis-room notifications
// use their own diagnosis-room scoped key and set DiagnosisTaskID.
// Notification-channel test actions set NotificationChannelID so test
// messages do not masquerade as report or diagnosis-task notifications.
type IMNotification struct {
	IdempotencyKey        string
	FinalReportID         int64
	DiagnosisTaskID       int64
	NotificationChannelID int64
	CorrelationKey        string
	Title                 string
	Body                  string
	Severity              string
}

// IMDelivery records provider-level delivery metadata. Concrete
// providers may leave ProviderMessageID empty when the upstream does
// not return a stable message identifier.
type IMDelivery struct {
	ProviderMessageID string
	Status            string
	Raw               json.RawMessage
}

// IMError classifies provider failures so Temporal Activities can
// choose retryable vs non-retryable application errors without
// depending on concrete provider packages.
type IMError struct {
	Message    string
	StatusCode int
	Retryable  bool
}

func (e *IMError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.StatusCode == 0 {
		return e.Message
	}
	return e.Message + " (status " + strconv.Itoa(e.StatusCode) + ")"
}

// IMProvider sends operator-facing notifications. Implementations
// must treat IdempotencyKey as required and must not mutate the input
// request.
type IMProvider interface {
	SendNotification(ctx context.Context, req IMNotification) (IMDelivery, error)
}

// NotificationChannelProviderResolver resolves a persisted notification channel
// profile into a provider for one report notification Activity. Implementations
// may read mutable configuration and resolve deployment-managed secrets, so
// callers must invoke it from Activities or other non-workflow boundaries.
type NotificationChannelProviderResolver interface {
	ResolveReportNotificationProvider(ctx context.Context, channelProfileID domain.NotificationChannelProfileID) (IMProvider, error)
}
