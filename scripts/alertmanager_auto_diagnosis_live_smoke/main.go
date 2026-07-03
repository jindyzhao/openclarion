// Command alertmanager_auto_diagnosis_live_smoke verifies that a live
// Alertmanager webhook starts an auto_room diagnosis and records an AI
// notification delivery in the room timeline.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	maxResponseBytes = int64(1024 * 1024)

	eventFinalReadyNotification    = "diagnosis_room.final_ready_notification_sent"
	eventAssistantTurnNotification = "diagnosis_room.assistant_turn_notification_sent"

	proofEvidence = "alertmanager_webhook auto_diagnosis_room ai_content_notification_delivered"

	expectedNotificationChannelSourceOperator = "operator"
	expectedNotificationChannelSourcePolicy   = "policy"

	expectedSyntheticAlertsReceived    = int64(4)
	expectedSyntheticResolvedSkipped   = int64(1)
	expectedSyntheticSuppressedSkipped = int64(2)
	expectedSyntheticIngestedTotal     = int64(1)
)

var (
	nowUTC = func() time.Time {
		return time.Now().UTC()
	}
	alertNamePattern     = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)
	contentSHA256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type config struct {
	apiBaseURL                           string
	sourceProfileID                      int64
	webhookBearerToken                   string
	webhookBearerTokenEnv                string
	expectedNotificationChannelProfileID int64
	expectedNotificationChannelSource    string
	expectedContentKind                  string
	requiredContentKinds                 []string
	outputPath                           string
	httpTimeout                          time.Duration
	roomTimeout                          time.Duration
	pollInterval                         time.Duration
	alertName                            string
}

type proof struct {
	Passed    bool         `json:"passed"`
	CheckedAt string       `json:"checked_at"`
	Request   proofRequest `json:"request"`
	Webhook   webhookProof `json:"webhook"`
	Room      roomProof    `json:"room"`
	Evidence  string       `json:"evidence"`
}

type proofRequest struct {
	APIBaseURL                           string   `json:"api_base_url"`
	SourceProfileID                      int64    `json:"source_profile_id"`
	AlertName                            string   `json:"alert_name"`
	SmokeID                              string   `json:"smoke_id"`
	StartsAt                             string   `json:"starts_at"`
	ExpectedNotificationChannelProfileID int64    `json:"expected_notification_channel_profile_id,omitempty"`
	ExpectedNotificationChannelSource    string   `json:"expected_notification_channel_source,omitempty"`
	ExpectedContentKind                  string   `json:"expected_content_kind,omitempty"`
	RequiredContentKinds                 []string `json:"required_content_kinds,omitempty"`
	HTTPTimeout                          string   `json:"http_timeout"`
	RoomTimeout                          string   `json:"room_timeout"`
	PollInterval                         string   `json:"poll_interval"`
}

type webhookProof struct {
	HTTPStatus    int                                           `json:"http_status"`
	SourceID      int64                                         `json:"source_id"`
	Received      int64                                         `json:"received"`
	Skipped       webhookSkipped                                `json:"skipped"`
	Ingested      api.AlertmanagerWebhookIngestResponseIngested `json:"ingested"`
	AutoDiagnosis autoDiagnosisProof                            `json:"auto_diagnosis"`
}

type webhookSkipped struct {
	Resolved   int64 `json:"resolved"`
	Suppressed int64 `json:"suppressed"`
}

type autoDiagnosisProof struct {
	PoliciesMatched int64                  `json:"policies_matched"`
	Snapshots       int64                  `json:"snapshots"`
	RoomsStarted    int64                  `json:"rooms_started"`
	RoomsSkipped    int64                  `json:"rooms_skipped"`
	Rooms           []autoDiagnosisRoomRef `json:"rooms"`
}

type autoDiagnosisRoomRef struct {
	PolicyID           int64  `json:"policy_id"`
	EvidenceSnapshotID int64  `json:"evidence_snapshot_id"`
	SessionID          string `json:"session_id"`
	InitialMessageID   string `json:"initial_message_id"`
	WorkflowID         string `json:"workflow_id"`
	RunID              string `json:"run_id"`
}

type roomProof struct {
	SessionID                              string                     `json:"session_id"`
	ChatSessionID                          int64                      `json:"chat_session_id"`
	DiagnosisTaskID                        int64                      `json:"diagnosis_task_id"`
	EvidenceSnapshotID                     int64                      `json:"evidence_snapshot_id"`
	WorkflowID                             string                     `json:"workflow_id"`
	RunID                                  string                     `json:"run_id"`
	TaskStatus                             string                     `json:"task_status"`
	RoomStatus                             string                     `json:"room_status"`
	TurnCount                              int                        `json:"turn_count"`
	LatestProgressStatus                   string                     `json:"latest_progress_status,omitempty"`
	LatestConclusionStatus                 string                     `json:"latest_conclusion_status,omitempty"`
	LatestConfidence                       string                     `json:"latest_confidence,omitempty"`
	LatestRequiresHumanReview              *bool                      `json:"latest_requires_human_review,omitempty"`
	LatestEvidenceRequestCount             int                        `json:"latest_evidence_request_count,omitempty"`
	LatestMissingEvidenceCount             int                        `json:"latest_missing_evidence_request_count,omitempty"`
	LatestEvidenceSuggestionCount          int                        `json:"latest_evidence_collection_suggestion_count,omitempty"`
	LatestConfidenceRationalePresent       bool                       `json:"latest_confidence_rationale_present"`
	LatestConfidenceImprovementPathPresent bool                       `json:"latest_confidence_improvement_path_present"`
	NotificationEventKinds                 []string                   `json:"notification_event_kinds"`
	NotificationProviderStatuses           []string                   `json:"notification_provider_statuses"`
	NotificationContentProofs              []notificationContentProof `json:"notification_content_proofs"`
	AINotificationDelivered                bool                       `json:"ai_notification_delivered"`
}

type notificationContentProof struct {
	EventKind                    string `json:"event_kind"`
	NotificationChannelProfileID int64  `json:"notification_channel_profile_id,omitempty"`
	ContentKind                  string `json:"content_kind"`
	ContentSHA256                string `json:"content_sha256"`
	RecommendedActionCount       int    `json:"recommended_action_count,omitempty"`
	EvidenceRequestCount         int    `json:"evidence_request_count,omitempty"`
}

type webhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int64             `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []webhookAlert    `json:"alerts"`
}

type webhookAlert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       *time.Time        `json:"endsAt,omitempty"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
	SilencedBy   []string          `json:"silencedBy,omitempty"`
	InhibitedBy  []string          `json:"inhibitedBy,omitempty"`
}

func main() {
	if err := run(os.Args[1:], http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "[alertmanager-auto-diagnosis-live-smoke] FAIL: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, client *http.Client) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}
	if client == nil {
		return fmt.Errorf("http client is required")
	}

	checkedAt := nowUTC()
	startsAt := checkedAt.Add(-1 * time.Minute)
	smokeID := smokeIDFromTime(checkedAt)
	webhookResult, err := postAlertmanagerWebhook(context.Background(), client, cfg, smokeID, startsAt)
	if err != nil {
		return err
	}
	if err := validateWebhookResult(cfg, webhookResult.Result); err != nil {
		return err
	}

	roomRef := webhookResult.Result.AutoDiagnosis.Rooms[0]
	cfg, err = bindExpectedNotificationChannelToPolicy(context.Background(), client, cfg, roomRef.PolicyID)
	if err != nil {
		return err
	}
	room, err := waitForDiagnosisRoomNotification(context.Background(), client, cfg, roomRef.SessionID)
	if err != nil {
		return err
	}

	out := proof{
		Passed:    true,
		CheckedAt: checkedAt.Format(time.RFC3339Nano),
		Request: proofRequest{
			APIBaseURL:                           cfg.apiBaseURL,
			SourceProfileID:                      cfg.sourceProfileID,
			AlertName:                            cfg.alertName,
			SmokeID:                              smokeID,
			StartsAt:                             startsAt.Format(time.RFC3339Nano),
			ExpectedNotificationChannelProfileID: cfg.expectedNotificationChannelProfileID,
			ExpectedNotificationChannelSource:    cfg.expectedNotificationChannelSource,
			ExpectedContentKind:                  cfg.expectedContentKind,
			RequiredContentKinds:                 cfg.requiredContentKinds,
			HTTPTimeout:                          cfg.httpTimeout.String(),
			RoomTimeout:                          cfg.roomTimeout.String(),
			PollInterval:                         cfg.pollInterval.String(),
		},
		Webhook:  webhookProofFromResponse(webhookResult),
		Room:     roomProofFromSummary(room),
		Evidence: proofEvidence,
	}
	if err := validateProof(out); err != nil {
		return fmt.Errorf("internal proof validation failed: %w", err)
	}
	if err := writeProof(cfg.outputPath, out); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "[alertmanager-auto-diagnosis-live-smoke] OK - live smoke output: %s\n", cfg.outputPath)
	return nil
}

func parseArgs(args []string) (config, error) {
	var cfg config
	var requiredContentKindsRaw string
	fs := flag.NewFlagSet("alertmanager_auto_diagnosis_live_smoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.apiBaseURL, "api-base-url", "", "OpenClarion API base URL")
	fs.Int64Var(&cfg.sourceProfileID, "source-profile-id", 0, "Alertmanager alert-source profile ID")
	fs.StringVar(&cfg.webhookBearerToken, "webhook-bearer-token", "", "optional Alertmanager webhook bearer token")
	fs.StringVar(&cfg.webhookBearerTokenEnv, "webhook-bearer-token-env", "", "optional environment variable name containing the Alertmanager webhook bearer token")
	fs.Int64Var(&cfg.expectedNotificationChannelProfileID, "expected-notification-channel-profile-id", 0, "optional expected diagnosis notification channel profile ID")
	fs.StringVar(&cfg.expectedContentKind, "expected-content-kind", "assistant_message", "expected AI notification content kind: assistant_message or final_conclusion")
	fs.StringVar(&requiredContentKindsRaw, "required-content-kinds", "", "optional comma-separated AI notification content kinds that must all be present; defaults to assistant_message")
	fs.StringVar(&cfg.outputPath, "output", "", "sanitized proof JSON output path")
	fs.DurationVar(&cfg.httpTimeout, "http-timeout", 15*time.Second, "per-request HTTP timeout")
	fs.DurationVar(&cfg.roomTimeout, "room-timeout", 10*time.Minute, "maximum time to wait for room AI notification proof")
	fs.DurationVar(&cfg.pollInterval, "poll-interval", 5*time.Second, "diagnosis room polling interval")
	fs.StringVar(&cfg.alertName, "alert-name", "OpenClarionAutoDiagnosisSmoke", "synthetic Alertmanager alertname")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments")
	}
	expectedNotificationChannelProfileIDSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "expected-notification-channel-profile-id" {
			expectedNotificationChannelProfileIDSet = true
		}
	})

	cfg.apiBaseURL = strings.TrimRight(strings.TrimSpace(cfg.apiBaseURL), "/")
	if cfg.apiBaseURL == "" {
		return config{}, fmt.Errorf("--api-base-url is required")
	}
	if err := validateBaseURL(cfg.apiBaseURL); err != nil {
		return config{}, fmt.Errorf("--api-base-url %w", err)
	}
	if cfg.sourceProfileID <= 0 {
		return config{}, fmt.Errorf("--source-profile-id must be positive")
	}
	webhookBearerToken, err := resolveWebhookBearerToken(cfg.webhookBearerToken, cfg.webhookBearerTokenEnv)
	if err != nil {
		return config{}, err
	}
	cfg.webhookBearerToken = webhookBearerToken
	if expectedNotificationChannelProfileIDSet && cfg.expectedNotificationChannelProfileID <= 0 {
		return config{}, fmt.Errorf("--expected-notification-channel-profile-id must be positive when set")
	}
	if expectedNotificationChannelProfileIDSet {
		cfg.expectedNotificationChannelSource = expectedNotificationChannelSourceOperator
	}
	cfg.expectedContentKind = strings.ToLower(strings.TrimSpace(cfg.expectedContentKind))
	if cfg.expectedContentKind != "" && !validNotificationContentKind(cfg.expectedContentKind) {
		return config{}, fmt.Errorf("--expected-content-kind must be assistant_message or final_conclusion when set")
	}
	cfg.requiredContentKinds, err = parseRequiredContentKinds(requiredContentKindsRaw)
	if err != nil {
		return config{}, fmt.Errorf("--required-content-kinds %w", err)
	}
	if len(cfg.requiredContentKinds) == 0 {
		cfg.requiredContentKinds = defaultRequiredContentKinds()
	}
	cfg.outputPath = filepath.Clean(strings.TrimSpace(cfg.outputPath))
	if cfg.outputPath == "." || cfg.outputPath == "" {
		return config{}, fmt.Errorf("--output is required")
	}
	if cfg.httpTimeout <= 0 {
		return config{}, fmt.Errorf("--http-timeout must be greater than zero")
	}
	if cfg.roomTimeout <= 0 {
		return config{}, fmt.Errorf("--room-timeout must be greater than zero")
	}
	if cfg.pollInterval <= 0 {
		return config{}, fmt.Errorf("--poll-interval must be greater than zero")
	}
	cfg.alertName = strings.TrimSpace(cfg.alertName)
	if !alertNamePattern.MatchString(cfg.alertName) {
		return config{}, fmt.Errorf("--alert-name must match %s", alertNamePattern.String())
	}
	return cfg, nil
}

func validateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("must use http or https")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain userinfo")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("must not contain query or fragment")
	}
	return nil
}

func resolveWebhookBearerToken(value, envName string) (string, error) {
	token, err := normalizeWebhookBearerToken(value, "--webhook-bearer-token")
	if err != nil {
		return "", err
	}
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return token, nil
	}
	if token != "" {
		return "", fmt.Errorf("--webhook-bearer-token and --webhook-bearer-token-env are mutually exclusive")
	}
	if !validEnvVarName(envName) {
		return "", fmt.Errorf("--webhook-bearer-token-env must be a valid environment variable name")
	}
	envValue, ok := os.LookupEnv(envName)
	if !ok {
		return "", fmt.Errorf("--webhook-bearer-token-env references an unset environment variable")
	}
	token, err = normalizeWebhookBearerToken(envValue, "--webhook-bearer-token-env value")
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("--webhook-bearer-token-env value must be non-empty")
	}
	return token, nil
}

func normalizeWebhookBearerToken(value, label string) (string, error) {
	token := strings.TrimSpace(value)
	if token == "" {
		return "", nil
	}
	if len(token) > len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		token = strings.TrimSpace(token[len("Bearer "):])
	}
	if token == "" {
		return "", fmt.Errorf("%s must be non-empty", label)
	}
	if strings.ContainsAny(token, "\r\n\t ") {
		return "", fmt.Errorf("%s must be a single token without whitespace", label)
	}
	return token, nil
}

func validEnvVarName(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' {
			continue
		}
		if i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

type webhookPOSTResult struct {
	HTTPStatus int
	Result     api.AlertmanagerWebhookIngestResponse
}

func postAlertmanagerWebhook(
	parent context.Context,
	client *http.Client,
	cfg config,
	smokeID string,
	startsAt time.Time,
) (webhookPOSTResult, error) {
	endpoint, err := alertmanagerWebhookURL(cfg.apiBaseURL, cfg.sourceProfileID)
	if err != nil {
		return webhookPOSTResult{}, err
	}
	raw, err := json.Marshal(buildWebhookPayload(cfg.alertName, smokeID, startsAt))
	if err != nil {
		return webhookPOSTResult{}, fmt.Errorf("encode webhook payload: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, cfg.httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return webhookPOSTResult{}, fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if cfg.webhookBearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.webhookBearerToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return webhookPOSTResult{}, fmt.Errorf("call Alertmanager webhook endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return webhookPOSTResult{}, err
	}
	if resp.StatusCode != http.StatusAccepted {
		return webhookPOSTResult{}, fmt.Errorf("Alertmanager webhook endpoint returned HTTP %d", resp.StatusCode)
	}
	var result api.AlertmanagerWebhookIngestResponse
	if err := strictjson.Unmarshal(body, &result); err != nil {
		return webhookPOSTResult{}, fmt.Errorf("decode Alertmanager webhook response: %w", err)
	}
	return webhookPOSTResult{HTTPStatus: resp.StatusCode, Result: result}, nil
}

func buildWebhookPayload(alertName, smokeID string, startsAt time.Time) webhookPayload {
	commonLabels := syntheticAlertLabels(alertName, smokeID, "")
	annotations := map[string]string{
		"summary":     "OpenClarion auto diagnosis live smoke",
		"description": "Synthetic Alertmanager webhook used to verify automatic AI diagnosis notification delivery.",
	}
	resolvedAt := startsAt.Add(time.Minute)
	return webhookPayload{
		Version:         "4",
		GroupKey:        fmt.Sprintf(`{}:{alertname="%s", smoke_id="%s"}`, alertName, smokeID),
		TruncatedAlerts: 0,
		Status:          "firing",
		Receiver:        "openclarion-auto-diagnosis-smoke",
		GroupLabels: map[string]string{
			"alertname": alertName,
			"smoke_id":  smokeID,
		},
		CommonLabels:      commonLabels,
		CommonAnnotations: annotations,
		ExternalURL:       "https://openclarion.example.invalid/alertmanager-auto-diagnosis-smoke",
		Alerts: []webhookAlert{
			{
				Status:       "firing",
				Labels:       syntheticAlertLabels(alertName, smokeID, "active"),
				Annotations:  annotations,
				StartsAt:     startsAt,
				GeneratorURL: "https://openclarion.example.invalid/alertmanager-auto-diagnosis-smoke/source",
				Fingerprint:  smokeID + "-active",
			},
			{
				Status:       "resolved",
				Labels:       syntheticAlertLabels(alertName, smokeID, "resolved"),
				Annotations:  annotations,
				StartsAt:     startsAt.Add(-1 * time.Minute),
				EndsAt:       &resolvedAt,
				GeneratorURL: "https://openclarion.example.invalid/alertmanager-auto-diagnosis-smoke/resolved",
				Fingerprint:  smokeID + "-resolved",
			},
			{
				Status:       "firing",
				Labels:       syntheticAlertLabels(alertName, smokeID, "silenced"),
				Annotations:  annotations,
				StartsAt:     startsAt,
				GeneratorURL: "https://openclarion.example.invalid/alertmanager-auto-diagnosis-smoke/silenced",
				Fingerprint:  smokeID + "-silenced",
				SilencedBy:   []string{"openclarion-live-smoke-silence"},
			},
			{
				Status:       "firing",
				Labels:       syntheticAlertLabels(alertName, smokeID, "inhibited"),
				Annotations:  annotations,
				StartsAt:     startsAt,
				GeneratorURL: "https://openclarion.example.invalid/alertmanager-auto-diagnosis-smoke/inhibited",
				Fingerprint:  smokeID + "-inhibited",
				InhibitedBy:  []string{"openclarion-live-smoke-inhibit"},
			},
		},
	}
}

func syntheticAlertLabels(alertName, smokeID, instanceSuffix string) map[string]string {
	labels := map[string]string{
		"alertname": alertName,
		"severity":  "warning",
		"smoke_id":  smokeID,
		"source":    "openclarion-live-smoke",
	}
	if instanceSuffix != "" {
		labels["instance"] = "auto-diagnosis-smoke-" + instanceSuffix
	}
	return labels
}

func alertmanagerWebhookURL(base string, sourceID int64) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "api", "v1", "alert-sources", fmt.Sprintf("%d", sourceID), "webhooks", "alertmanager")
	return u.String(), nil
}

func validateWebhookResult(cfg config, result api.AlertmanagerWebhookIngestResponse) error {
	if result.SourceID != cfg.sourceProfileID {
		return fmt.Errorf("webhook response source_id=%d, want %d", result.SourceID, cfg.sourceProfileID)
	}
	if result.Received != expectedSyntheticAlertsReceived {
		return fmt.Errorf("webhook response received=%d, want %d for synthetic smoke", result.Received, expectedSyntheticAlertsReceived)
	}
	if result.SkippedResolved != expectedSyntheticResolvedSkipped {
		return fmt.Errorf("webhook response skipped_resolved=%d, want %d for synthetic smoke", result.SkippedResolved, expectedSyntheticResolvedSkipped)
	}
	if result.SkippedSuppressed != expectedSyntheticSuppressedSkipped {
		return fmt.Errorf("webhook response skipped_suppressed=%d, want %d for synthetic smoke", result.SkippedSuppressed, expectedSyntheticSuppressedSkipped)
	}
	if result.Ingested.Total != expectedSyntheticIngestedTotal {
		return fmt.Errorf("webhook response ingested.total=%d, want %d for synthetic smoke", result.Ingested.Total, expectedSyntheticIngestedTotal)
	}
	if result.Ingested.Failed != 0 {
		return fmt.Errorf("webhook response ingested.failed=%d, want 0", result.Ingested.Failed)
	}
	if result.AutoDiagnosis == nil {
		return fmt.Errorf("webhook response missing auto_diagnosis; ensure an enabled auto_room policy is bound to the Alertmanager source")
	}
	if result.AutoDiagnosis.PoliciesMatched <= 0 {
		return fmt.Errorf("auto_diagnosis.policies_matched=%d, want positive", result.AutoDiagnosis.PoliciesMatched)
	}
	if result.AutoDiagnosis.Snapshots <= 0 {
		return fmt.Errorf("auto_diagnosis.snapshots=%d, want positive", result.AutoDiagnosis.Snapshots)
	}
	if result.AutoDiagnosis.RoomsStarted <= 0 || len(result.AutoDiagnosis.Rooms) == 0 {
		return fmt.Errorf("auto_diagnosis did not start a diagnosis room")
	}
	for i, room := range result.AutoDiagnosis.Rooms {
		if strings.TrimSpace(room.SessionID) == "" {
			return fmt.Errorf("auto_diagnosis.rooms[%d].session_id is empty", i)
		}
		if room.EvidenceSnapshotID <= 0 {
			return fmt.Errorf("auto_diagnosis.rooms[%d].evidence_snapshot_id must be positive", i)
		}
	}
	return nil
}

func bindExpectedNotificationChannelToPolicy(
	parent context.Context,
	client *http.Client,
	cfg config,
	policyID int64,
) (config, error) {
	if policyID <= 0 {
		return config{}, fmt.Errorf("auto_diagnosis room policy_id=%d, want positive", policyID)
	}
	policy, err := getReportWorkflowPolicy(parent, client, cfg, policyID)
	if err != nil {
		return config{}, err
	}
	if policy.ID != policyID {
		return config{}, fmt.Errorf("report workflow policy response id=%d, want %d", policy.ID, policyID)
	}
	policyNotificationChannelProfileID, err := requiredPolicyNotificationChannelProfileID(policy)
	if err != nil {
		return config{}, err
	}
	if cfg.expectedNotificationChannelProfileID > 0 {
		if cfg.expectedNotificationChannelProfileID != policyNotificationChannelProfileID {
			return config{}, fmt.Errorf(
				"expected notification channel profile id %d does not match policy %d report_notification_channel_profile_id %d",
				cfg.expectedNotificationChannelProfileID,
				policyID,
				policyNotificationChannelProfileID,
			)
		}
		return cfg, nil
	}
	cfg.expectedNotificationChannelProfileID = policyNotificationChannelProfileID
	cfg.expectedNotificationChannelSource = expectedNotificationChannelSourcePolicy
	return cfg, nil
}

func getReportWorkflowPolicy(
	parent context.Context,
	client *http.Client,
	cfg config,
	policyID int64,
) (api.ReportWorkflowPolicy, error) {
	endpoint, err := reportWorkflowPolicyURL(cfg.apiBaseURL, policyID)
	if err != nil {
		return api.ReportWorkflowPolicy{}, err
	}
	ctx, cancel := context.WithTimeout(parent, cfg.httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return api.ReportWorkflowPolicy{}, fmt.Errorf("build report workflow policy request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return api.ReportWorkflowPolicy{}, fmt.Errorf("call report workflow policy endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return api.ReportWorkflowPolicy{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return api.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy endpoint returned HTTP %d", resp.StatusCode)
	}
	var policy api.ReportWorkflowPolicy
	if err := strictjson.Unmarshal(body, &policy); err != nil {
		return api.ReportWorkflowPolicy{}, fmt.Errorf("decode report workflow policy response: %w", err)
	}
	return policy, nil
}

func reportWorkflowPolicyURL(base string, policyID int64) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "api", "v1", "config", "report-workflow-policies", fmt.Sprintf("%d", policyID))
	return u.String(), nil
}

func requiredPolicyNotificationChannelProfileID(policy api.ReportWorkflowPolicy) (int64, error) {
	if !policy.ReportNotificationChannelProfileID.IsSpecified() || policy.ReportNotificationChannelProfileID.IsNull() {
		return 0, fmt.Errorf(
			"report workflow policy %d has no report_notification_channel_profile_id; bind an Enterprise WeChat notification channel before running this smoke",
			policy.ID,
		)
	}
	id, err := policy.ReportNotificationChannelProfileID.Get()
	if err != nil {
		return 0, fmt.Errorf("read report workflow policy %d notification channel binding: %w", policy.ID, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("report workflow policy %d report_notification_channel_profile_id=%d, want positive", policy.ID, id)
	}
	return id, nil
}

func waitForDiagnosisRoomNotification(
	parent context.Context,
	client *http.Client,
	cfg config,
	sessionID string,
) (api.DiagnosisRoomSummary, error) {
	deadline := time.Now().Add(cfg.roomTimeout)
	var lastRoom *api.DiagnosisRoomSummary
	var lastErr error
	for {
		room, err := getDiagnosisRoom(parent, client, cfg, sessionID)
		if err == nil {
			lastRoom = &room
			if diagnosisRoomHasAIProof(room, cfg) {
				return room, nil
			}
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastRoom != nil {
				summary := roomProofFromSummary(*lastRoom)
				return api.DiagnosisRoomSummary{}, fmt.Errorf(
					"timed out waiting for AI notification delivery; last turn_count=%d progress=%q conclusion=%q events=%v statuses=%v",
					summary.TurnCount,
					summary.LatestProgressStatus,
					summary.LatestConclusionStatus,
					summary.NotificationEventKinds,
					summary.NotificationProviderStatuses,
				)
			}
			if lastErr != nil {
				return api.DiagnosisRoomSummary{}, fmt.Errorf("timed out waiting for diagnosis room; last error: %w", lastErr)
			}
			return api.DiagnosisRoomSummary{}, fmt.Errorf("timed out waiting for diagnosis room")
		}
		remaining := time.Until(deadline)
		sleepFor := cfg.pollInterval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		timer := time.NewTimer(sleepFor)
		select {
		case <-parent.Done():
			timer.Stop()
			return api.DiagnosisRoomSummary{}, parent.Err()
		case <-timer.C:
		}
	}
}

func getDiagnosisRoom(parent context.Context, client *http.Client, cfg config, sessionID string) (api.DiagnosisRoomSummary, error) {
	endpoint, err := diagnosisRoomURL(cfg.apiBaseURL, sessionID)
	if err != nil {
		return api.DiagnosisRoomSummary{}, err
	}
	ctx, cancel := context.WithTimeout(parent, cfg.httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return api.DiagnosisRoomSummary{}, fmt.Errorf("build diagnosis room request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return api.DiagnosisRoomSummary{}, fmt.Errorf("call diagnosis room endpoint: %w", err)
	}
	defer resp.Body.Close()
	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return api.DiagnosisRoomSummary{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return api.DiagnosisRoomSummary{}, fmt.Errorf("diagnosis room endpoint returned HTTP %d", resp.StatusCode)
	}
	var room api.DiagnosisRoomSummary
	if err := strictjson.Unmarshal(body, &room); err != nil {
		return api.DiagnosisRoomSummary{}, fmt.Errorf("decode diagnosis room response: %w", err)
	}
	return room, nil
}

func diagnosisRoomURL(base, sessionID string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "api", "v1", "diagnosis", "rooms", sessionID)
	return u.String(), nil
}

func diagnosisRoomHasAIProof(room api.DiagnosisRoomSummary, cfg config) bool {
	if strings.TrimSpace(room.SessionID) == "" || room.TurnCount <= 0 {
		return false
	}
	if room.LatestProgress == nil && room.LatestConclusion == nil {
		return false
	}
	proofs := []notificationContentProof{}
	for _, item := range room.NotificationTimeline {
		if proof, ok := notificationContentProofFromTimeline(item); ok {
			proofs = append(proofs, proof)
		}
	}
	return notificationProofsMatchConfig(proofs, cfg)
}

func isAcceptedAINotification(item api.DiagnosisRoomNotificationTimelineEntry) bool {
	if item.EventKind != eventAssistantTurnNotification && item.EventKind != eventFinalReadyNotification {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(item.ProviderStatus))
	return status != "" && status != "failed" && status != "error"
}

func notificationContentProofFromTimeline(item api.DiagnosisRoomNotificationTimelineEntry) (notificationContentProof, bool) {
	if !isAcceptedAINotification(item) {
		return notificationContentProof{}, false
	}
	if item.ContentKind == nil || item.ContentSha256 == nil {
		return notificationContentProof{}, false
	}
	contentKind := strings.TrimSpace(*item.ContentKind)
	if contentKind != "assistant_message" && contentKind != "final_conclusion" {
		return notificationContentProof{}, false
	}
	contentSHA := strings.TrimSpace(*item.ContentSha256)
	if !contentSHA256Pattern.MatchString(contentSHA) {
		return notificationContentProof{}, false
	}
	out := notificationContentProof{
		EventKind:     item.EventKind,
		ContentKind:   contentKind,
		ContentSHA256: contentSHA,
	}
	if item.NotificationChannelProfileID != nil {
		out.NotificationChannelProfileID = *item.NotificationChannelProfileID
	}
	if item.RecommendedActionCount != nil {
		out.RecommendedActionCount = *item.RecommendedActionCount
	}
	if item.EvidenceRequestCount != nil {
		out.EvidenceRequestCount = *item.EvidenceRequestCount
	}
	return out, true
}

func notificationProofMatchesConfig(proof notificationContentProof, cfg config) bool {
	if !notificationProofMatchesChannel(proof, cfg) {
		return false
	}
	if cfg.expectedContentKind != "" && proof.ContentKind != cfg.expectedContentKind {
		return false
	}
	return true
}

func notificationProofMatchesChannel(proof notificationContentProof, cfg config) bool {
	if cfg.expectedNotificationChannelProfileID > 0 &&
		proof.NotificationChannelProfileID != cfg.expectedNotificationChannelProfileID {
		return false
	}
	return true
}

func notificationProofsMatchConfig(proofs []notificationContentProof, cfg config) bool {
	if len(proofs) == 0 {
		return false
	}
	if len(cfg.requiredContentKinds) > 0 {
		required := make(map[string]bool, len(cfg.requiredContentKinds))
		for _, kind := range cfg.requiredContentKinds {
			required[kind] = false
		}
		for _, proof := range proofs {
			if !notificationProofMatchesChannel(proof, cfg) {
				continue
			}
			if _, ok := required[proof.ContentKind]; ok {
				required[proof.ContentKind] = true
			}
		}
		for _, matched := range required {
			if !matched {
				return false
			}
		}
	}
	if cfg.expectedContentKind == "" {
		if len(cfg.requiredContentKinds) > 0 {
			return true
		}
		for _, proof := range proofs {
			if notificationProofMatchesChannel(proof, cfg) {
				return true
			}
		}
		return false
	}
	for _, proof := range proofs {
		if notificationProofMatchesConfig(proof, cfg) {
			return true
		}
	}
	return false
}

func validExpectedNotificationChannelSource(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "", expectedNotificationChannelSourceOperator, expectedNotificationChannelSourcePolicy:
		return true
	default:
		return false
	}
}

func validNotificationContentKind(raw string) bool {
	switch strings.TrimSpace(raw) {
	case "assistant_message", "final_conclusion":
		return true
	default:
		return false
	}
}

func parseRequiredContentKinds(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	kinds := []string{}
	for _, part := range strings.Split(raw, ",") {
		kind := strings.ToLower(strings.TrimSpace(part))
		if kind == "" {
			return nil, fmt.Errorf("must not contain empty values")
		}
		if !validNotificationContentKind(kind) {
			return nil, fmt.Errorf("must contain only assistant_message or final_conclusion")
		}
		if _, ok := seen[kind]; ok {
			return nil, fmt.Errorf("must not contain duplicate value %q", kind)
		}
		seen[kind] = struct{}{}
		kinds = append(kinds, kind)
	}
	return kinds, nil
}

func defaultRequiredContentKinds() []string {
	return []string{"assistant_message"}
}

func validateRequiredContentKinds(kinds []string) error {
	seen := map[string]struct{}{}
	for _, raw := range kinds {
		kind := strings.TrimSpace(raw)
		if kind == "" {
			return fmt.Errorf("must not contain empty values")
		}
		if kind != raw {
			return fmt.Errorf("must not contain surrounding whitespace")
		}
		if !validNotificationContentKind(kind) {
			return fmt.Errorf("must contain only assistant_message or final_conclusion")
		}
		if _, ok := seen[kind]; ok {
			return fmt.Errorf("must not contain duplicate value %q", kind)
		}
		seen[kind] = struct{}{}
	}
	return nil
}

func readLimitedBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(raw)) > maxResponseBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBytes)
	}
	return raw, nil
}

func webhookProofFromResponse(result webhookPOSTResult) webhookProof {
	out := webhookProof{
		HTTPStatus: result.HTTPStatus,
		SourceID:   result.Result.SourceID,
		Received:   result.Result.Received,
		Skipped: webhookSkipped{
			Resolved:   result.Result.SkippedResolved,
			Suppressed: result.Result.SkippedSuppressed,
		},
		Ingested: result.Result.Ingested,
	}
	if result.Result.AutoDiagnosis != nil {
		out.AutoDiagnosis = autoDiagnosisProofFromResponse(*result.Result.AutoDiagnosis)
	}
	return out
}

func autoDiagnosisProofFromResponse(summary api.AlertmanagerWebhookAutoDiagnosisSummary) autoDiagnosisProof {
	rooms := make([]autoDiagnosisRoomRef, 0, len(summary.Rooms))
	for _, room := range summary.Rooms {
		rooms = append(rooms, autoDiagnosisRoomRef{
			PolicyID:           room.PolicyID,
			EvidenceSnapshotID: room.EvidenceSnapshotID,
			SessionID:          room.SessionID,
			InitialMessageID:   room.InitialMessageID,
			WorkflowID:         room.WorkflowID,
			RunID:              room.RunID,
		})
	}
	return autoDiagnosisProof{
		PoliciesMatched: summary.PoliciesMatched,
		Snapshots:       summary.Snapshots,
		RoomsStarted:    summary.RoomsStarted,
		RoomsSkipped:    summary.RoomsSkipped,
		Rooms:           rooms,
	}
}

func roomProofFromSummary(room api.DiagnosisRoomSummary) roomProof {
	out := roomProof{
		SessionID:          room.SessionID,
		ChatSessionID:      room.ChatSessionID,
		DiagnosisTaskID:    room.DiagnosisTaskID,
		EvidenceSnapshotID: room.EvidenceSnapshotID,
		WorkflowID:         room.WorkflowID,
		RunID:              room.RunID,
		TaskStatus:         string(room.TaskStatus),
		RoomStatus:         string(room.RoomStatus),
		TurnCount:          room.TurnCount,
	}
	if room.LatestProgress != nil {
		out.LatestProgressStatus = room.LatestProgress.Status
		out.LatestConfidence = string(room.LatestProgress.Confidence)
		out.LatestRequiresHumanReview = boolPtr(room.LatestProgress.RequiresHumanReview)
		out.LatestEvidenceRequestCount = room.LatestProgress.EvidenceRequestCount
		out.LatestMissingEvidenceCount = len(room.LatestProgress.MissingEvidenceRequests)
		out.LatestEvidenceSuggestionCount = len(room.LatestProgress.EvidenceCollectionSuggestions)
		out.LatestConfidenceRationalePresent = nonEmptyStringPtr(room.LatestProgress.ConfidenceRationale)
	}
	if room.LatestConclusion != nil {
		out.LatestConclusionStatus = room.LatestConclusion.Status
		if out.LatestConfidence == "" && room.LatestConclusion.Confidence != nil {
			out.LatestConfidence = string(*room.LatestConclusion.Confidence)
		}
		if out.LatestRequiresHumanReview == nil {
			out.LatestRequiresHumanReview = room.LatestConclusion.RequiresHumanReview
		}
		if out.LatestEvidenceRequestCount == 0 {
			out.LatestEvidenceRequestCount = len(room.LatestConclusion.EvidenceRequests)
		}
		if out.LatestMissingEvidenceCount == 0 {
			out.LatestMissingEvidenceCount = len(room.LatestConclusion.MissingEvidenceRequests)
		}
		if out.LatestEvidenceSuggestionCount == 0 {
			out.LatestEvidenceSuggestionCount = len(room.LatestConclusion.EvidenceCollectionSuggestions)
		}
		if !out.LatestConfidenceRationalePresent {
			out.LatestConfidenceRationalePresent = nonEmptyStringPtr(room.LatestConclusion.ConfidenceRationale)
		}
	}
	out.LatestConfidenceImprovementPathPresent =
		out.LatestEvidenceRequestCount > 0 ||
			out.LatestMissingEvidenceCount > 0 ||
			out.LatestEvidenceSuggestionCount > 0
	for _, item := range room.NotificationTimeline {
		out.NotificationEventKinds = append(out.NotificationEventKinds, item.EventKind)
		out.NotificationProviderStatuses = append(out.NotificationProviderStatuses, item.ProviderStatus)
		if contentProof, ok := notificationContentProofFromTimeline(item); ok {
			out.NotificationContentProofs = append(out.NotificationContentProofs, contentProof)
			out.AINotificationDelivered = true
		}
	}
	return out
}

func boolPtr(value bool) *bool {
	out := value
	return &out
}

func nonEmptyStringPtr(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

func validateProof(out proof) error {
	if !out.Passed {
		return fmt.Errorf("passed must be true")
	}
	if _, err := time.Parse(time.RFC3339Nano, out.CheckedAt); err != nil {
		return fmt.Errorf("checked_at must be RFC3339Nano: %w", err)
	}
	if err := validateBaseURL(out.Request.APIBaseURL); err != nil {
		return fmt.Errorf("request.api_base_url %w", err)
	}
	if out.Request.SourceProfileID <= 0 || out.Webhook.SourceID != out.Request.SourceProfileID {
		return fmt.Errorf("source profile proof is inconsistent")
	}
	if !alertNamePattern.MatchString(out.Request.AlertName) {
		return fmt.Errorf("request.alert_name is invalid")
	}
	if strings.TrimSpace(out.Request.SmokeID) == "" {
		return fmt.Errorf("request.smoke_id is required")
	}
	if _, err := time.Parse(time.RFC3339Nano, out.Request.StartsAt); err != nil {
		return fmt.Errorf("request.starts_at must be RFC3339Nano: %w", err)
	}
	if out.Request.ExpectedNotificationChannelProfileID < 0 {
		return fmt.Errorf("request.expected_notification_channel_profile_id must be non-negative")
	}
	if !validExpectedNotificationChannelSource(out.Request.ExpectedNotificationChannelSource) {
		return fmt.Errorf("request.expected_notification_channel_source is invalid")
	}
	if out.Request.ExpectedNotificationChannelSource != "" && out.Request.ExpectedNotificationChannelProfileID == 0 {
		return fmt.Errorf("request.expected_notification_channel_source requires expected_notification_channel_profile_id")
	}
	if out.Request.ExpectedContentKind != "" && !validNotificationContentKind(out.Request.ExpectedContentKind) {
		return fmt.Errorf("request.expected_content_kind is invalid")
	}
	if err := validateRequiredContentKinds(out.Request.RequiredContentKinds); err != nil {
		return fmt.Errorf("request.required_content_kinds %w", err)
	}
	for name, raw := range map[string]string{
		"request.http_timeout":  out.Request.HTTPTimeout,
		"request.room_timeout":  out.Request.RoomTimeout,
		"request.poll_interval": out.Request.PollInterval,
	} {
		if duration, err := time.ParseDuration(raw); err != nil || duration <= 0 {
			return fmt.Errorf("%s must be a positive duration", name)
		}
	}
	if out.Webhook.HTTPStatus != http.StatusAccepted {
		return fmt.Errorf("webhook.http_status=%d, want %d", out.Webhook.HTTPStatus, http.StatusAccepted)
	}
	if out.Webhook.Received != expectedSyntheticAlertsReceived ||
		out.Webhook.Skipped.Resolved != expectedSyntheticResolvedSkipped ||
		out.Webhook.Skipped.Suppressed != expectedSyntheticSuppressedSkipped ||
		out.Webhook.Ingested.Total != expectedSyntheticIngestedTotal ||
		out.Webhook.Ingested.Failed != 0 {
		return fmt.Errorf("webhook counters do not prove exactly one active firing-alert ingest with resolved and suppressed alerts ignored")
	}
	if out.Webhook.AutoDiagnosis.PoliciesMatched <= 0 ||
		out.Webhook.AutoDiagnosis.Snapshots <= 0 ||
		out.Webhook.AutoDiagnosis.RoomsStarted <= 0 ||
		len(out.Webhook.AutoDiagnosis.Rooms) == 0 {
		return fmt.Errorf("webhook.auto_diagnosis does not prove automatic room start")
	}
	if out.Room.SessionID == "" || out.Room.DiagnosisTaskID <= 0 || out.Room.EvidenceSnapshotID <= 0 {
		return fmt.Errorf("room identity is incomplete")
	}
	if out.Room.TurnCount <= 0 {
		return fmt.Errorf("room.turn_count must be positive")
	}
	if out.Room.LatestProgressStatus == "" && out.Room.LatestConclusionStatus == "" {
		return fmt.Errorf("room must include latest progress or conclusion")
	}
	if !validReportConfidence(out.Room.LatestConfidence) {
		return fmt.Errorf("room.latest_confidence must be low, medium, or high")
	}
	if out.Room.LatestRequiresHumanReview == nil {
		return fmt.Errorf("room.latest_requires_human_review must be retained")
	}
	if !out.Room.LatestConfidenceRationalePresent {
		return fmt.Errorf("room.latest_confidence_rationale_present must prove confidence rationale retention")
	}
	if !out.Room.LatestConfidenceImprovementPathPresent {
		return fmt.Errorf("room.latest_confidence_improvement_path_present must prove an executable or operator evidence path")
	}
	if !out.Room.AINotificationDelivered {
		return fmt.Errorf("room must include an accepted AI notification timeline event with content proof")
	}
	if len(out.Room.NotificationEventKinds) == 0 || len(out.Room.NotificationProviderStatuses) == 0 {
		return fmt.Errorf("room notification timeline is empty")
	}
	if len(out.Room.NotificationContentProofs) == 0 {
		return fmt.Errorf("room notification content proof is empty")
	}
	for i, contentProof := range out.Room.NotificationContentProofs {
		if contentProof.EventKind != eventAssistantTurnNotification && contentProof.EventKind != eventFinalReadyNotification {
			return fmt.Errorf("room.notification_content_proofs[%d].event_kind is not an AI notification", i)
		}
		if !validNotificationContentKind(contentProof.ContentKind) {
			return fmt.Errorf("room.notification_content_proofs[%d].content_kind is invalid", i)
		}
		if !contentSHA256Pattern.MatchString(contentProof.ContentSHA256) {
			return fmt.Errorf("room.notification_content_proofs[%d].content_sha256 is invalid", i)
		}
	}
	if out.Request.ExpectedNotificationChannelProfileID > 0 ||
		out.Request.ExpectedContentKind != "" ||
		len(out.Request.RequiredContentKinds) > 0 {
		if !notificationProofsMatchConfig(out.Room.NotificationContentProofs, config{
			expectedNotificationChannelProfileID: out.Request.ExpectedNotificationChannelProfileID,
			expectedContentKind:                  out.Request.ExpectedContentKind,
			requiredContentKinds:                 out.Request.RequiredContentKinds,
		}) {
			return fmt.Errorf("room notification content proof does not match expected channel/content constraints")
		}
	}
	if out.Evidence != proofEvidence {
		return fmt.Errorf("evidence marker is invalid")
	}
	return nil
}

func validReportConfidence(value string) bool {
	switch strings.TrimSpace(value) {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func writeProof(outputPath string, out proof) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proof: %w", err)
	}
	raw = append(raw, '\n')
	// #nosec G304 -- manual smoke output path is supplied by the operator.
	if err := os.WriteFile(outputPath, raw, 0o600); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

func smokeIDFromTime(t time.Time) string {
	return "openclarion-auto-diagnosis-smoke-" + t.UTC().Format("20060102T150405.000000000Z")
}
