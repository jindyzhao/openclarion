// Command alert_consultation_setup creates or replaces the minimal live
// Alertmanager -> auto_room -> Enterprise WeChat consultation configuration.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	maxResponseBytes int64 = 1024 * 1024

	defaultAlertSourceName         = "Live Alertmanager"
	defaultNotificationChannelName = "AI report and diagnosis WeCom"
	defaultGroupingPolicyName      = "Alertmanager auto diagnosis grouping"
	defaultReportWorkflowName      = "Alertmanager auto diagnosis policy"
	defaultNotificationSecretRef   = "secret/openclarion/ops-wecom"
	defaultReportScenario          = "cascade"

	proofEvidence = "alertmanager_wecom_auto_room_configuration_ready"
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

var requiredNotificationAIProofContentKinds = []string{
	string(api.NotificationChannelTestResultContentKindAiDiagnosisSample),
	string(api.NotificationChannelTestResultContentKindDiagnosisCloseSample),
}

type config struct {
	apiBaseURL               string
	bearerToken              string
	bearerTokenEnv           string
	alertmanagerBaseURL      string
	alertmanagerAuthMode     api.AlertSourceAuthMode
	alertmanagerSecretRef    string
	notificationSecretRef    string
	sourceID                 int64
	channelID                int64
	groupingPolicyID         int64
	reportWorkflowPolicyID   int64
	sourceName               string
	channelName              string
	groupingPolicyName       string
	reportWorkflowPolicyName string
	reportScenario           api.ReportWorkflowScenario
	enablePolicy             bool
	outputPath               string
	envOutputPath            string
	timeout                  time.Duration
}

type setupResult struct {
	AlertSourceProfile      api.AlertSourceProfile
	NotificationChannel     api.NotificationChannelProfile
	GroupingPolicy          api.GroupingPolicy
	ReportWorkflowPolicy    api.ReportWorkflowPolicy
	NotificationAIProofs    []api.NotificationChannelTestResult
	ReportWorkflowWasEnable bool
}

type proof struct {
	CheckedAt string       `json:"checked_at"`
	Request   proofRequest `json:"request"`
	Result    proofResult  `json:"result"`
	Evidence  string       `json:"evidence"`
}

type proofRequest struct {
	APIBaseURL                  string `json:"api_base_url"`
	AlertmanagerBaseURLSet      bool   `json:"alertmanager_base_url_set"`
	AlertmanagerAuthMode        string `json:"alertmanager_auth_mode"`
	AlertmanagerSecretRef       bool   `json:"alertmanager_secret_ref_set,omitempty"`
	NotificationSecretRef       bool   `json:"notification_secret_ref_set"`
	SourceID                    int64  `json:"source_id,omitempty"`
	ChannelID                   int64  `json:"channel_id,omitempty"`
	GroupingPolicyID            int64  `json:"grouping_policy_id,omitempty"`
	ReportWorkflowPolicyID      int64  `json:"report_workflow_policy_id,omitempty"`
	ReportScenario              string `json:"report_scenario"`
	EnablePolicy                bool   `json:"enable_policy"`
	NotificationAIProofRequired bool   `json:"notification_ai_proof_required"`
	Timeout                     string `json:"timeout"`
}

type proofResult struct {
	AlertSourceProfileID   int64                      `json:"alert_source_profile_id"`
	NotificationChannelID  int64                      `json:"notification_channel_profile_id"`
	GroupingPolicyID       int64                      `json:"grouping_policy_id"`
	ReportWorkflowPolicyID int64                      `json:"report_workflow_policy_id"`
	ReportWorkflowEnabled  bool                       `json:"report_workflow_enabled"`
	NotificationKind       string                     `json:"notification_kind"`
	NotificationScopes     []string                   `json:"notification_scopes"`
	DiagnosisFollowUp      string                     `json:"diagnosis_follow_up"`
	AlertSourceKind        string                     `json:"alert_source_kind"`
	NotificationAIProofs   []proofNotificationAIProof `json:"notification_ai_proofs,omitempty"`
}

type proofNotificationAIProof struct {
	ContentKind          string `json:"content_kind"`
	Status               string `json:"status"`
	ReasonCode           string `json:"reason_code"`
	ProviderStatus       string `json:"provider_status,omitempty"`
	ContentSHA256Present bool   `json:"content_sha256_present"`
	CheckedAt            string `json:"checked_at"`
}

type apiStatusError struct {
	statusCode int
	message    string
}

func (e apiStatusError) Error() string {
	return fmt.Sprintf("API returned HTTP %d: %s", e.statusCode, e.message)
}

func main() {
	if err := run(os.Args[1:], http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "[alert-consultation-setup] FAIL: %v\n", err)
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

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	result, err := setupAlertConsultation(ctx, client, cfg)
	if err != nil {
		return err
	}
	out := proofFromResult(cfg, result)
	if err := validateProof(out); err != nil {
		return fmt.Errorf("internal proof validation failed: %w", err)
	}
	if err := writeJSONFile(cfg.outputPath, out); err != nil {
		return err
	}
	if cfg.envOutputPath != "" {
		if err := writeEnvFile(cfg.envOutputPath, out); err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "[alert-consultation-setup] OK - setup proof: %s\n", cfg.outputPath)
	if cfg.envOutputPath != "" {
		fmt.Fprintf(os.Stderr, "[alert-consultation-setup] OK - setup env: %s\n", cfg.envOutputPath)
	}
	return nil
}

func parseArgs(args []string) (config, error) {
	var cfg config
	var authMode string
	var scenario string
	fs := flag.NewFlagSet("alert_consultation_setup", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.apiBaseURL, "api-base-url", "", "OpenClarion API base URL")
	fs.StringVar(&cfg.alertmanagerBaseURL, "alertmanager-base-url", "", "Alertmanager service, route prefix, /api/v2, or /api/v2/alerts URL")
	fs.StringVar(&authMode, "alertmanager-auth-mode", string(api.AlertSourceAuthModeNone), "Alertmanager auth mode: none or bearer")
	fs.StringVar(&cfg.alertmanagerSecretRef, "alertmanager-secret-ref", "", "server-side secret reference for Alertmanager bearer auth")
	fs.StringVar(&cfg.notificationSecretRef, "notification-secret-ref", defaultNotificationSecretRef, "server-side secret reference for Enterprise WeChat webhook URL")
	fs.Int64Var(&cfg.sourceID, "source-id", 0, "optional existing alert source profile ID to replace")
	fs.Int64Var(&cfg.channelID, "channel-id", 0, "optional existing notification channel profile ID to replace")
	fs.Int64Var(&cfg.groupingPolicyID, "grouping-policy-id", 0, "optional existing grouping policy ID to replace")
	fs.Int64Var(&cfg.reportWorkflowPolicyID, "report-workflow-policy-id", 0, "optional existing report workflow policy ID to replace")
	fs.StringVar(&cfg.sourceName, "source-name", defaultAlertSourceName, "alert source profile name")
	fs.StringVar(&cfg.channelName, "channel-name", defaultNotificationChannelName, "notification channel profile name")
	fs.StringVar(&cfg.groupingPolicyName, "grouping-policy-name", defaultGroupingPolicyName, "grouping policy name")
	fs.StringVar(&cfg.reportWorkflowPolicyName, "report-workflow-policy-name", defaultReportWorkflowName, "report workflow policy name")
	fs.StringVar(&scenario, "report-scenario", defaultReportScenario, "report scenario: single_alert, cascade, or alert_storm")
	fs.BoolVar(&cfg.enablePolicy, "enable-policy", true, "enable the report workflow policy after upsert")
	fs.StringVar(&cfg.bearerToken, "bearer-token", "", "optional bearer token for the OpenClarion API")
	fs.StringVar(&cfg.bearerTokenEnv, "bearer-token-env", "", "optional environment variable name containing the bearer token for the OpenClarion API")
	fs.StringVar(&cfg.outputPath, "output", "", "sanitized proof JSON output path")
	fs.StringVar(&cfg.envOutputPath, "env-output", "", "optional shell env output path for generated IDs")
	fs.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments")
	}

	cfg.apiBaseURL = strings.TrimRight(strings.TrimSpace(cfg.apiBaseURL), "/")
	if err := validateAbsoluteHTTPURL("--api-base-url", cfg.apiBaseURL); err != nil {
		return config{}, err
	}
	cfg.alertmanagerBaseURL = strings.TrimRight(strings.TrimSpace(cfg.alertmanagerBaseURL), "/")
	if err := validateAbsoluteHTTPURL("--alertmanager-base-url", cfg.alertmanagerBaseURL); err != nil {
		return config{}, err
	}

	authMode = strings.ToLower(strings.TrimSpace(authMode))
	switch api.AlertSourceAuthMode(authMode) {
	case api.AlertSourceAuthModeNone, api.AlertSourceAuthModeBearer:
		cfg.alertmanagerAuthMode = api.AlertSourceAuthMode(authMode)
	default:
		return config{}, fmt.Errorf("--alertmanager-auth-mode must be none or bearer")
	}
	cfg.alertmanagerSecretRef = strings.TrimSpace(cfg.alertmanagerSecretRef)
	if cfg.alertmanagerAuthMode == api.AlertSourceAuthModeNone && cfg.alertmanagerSecretRef != "" {
		return config{}, fmt.Errorf("--alertmanager-secret-ref requires --alertmanager-auth-mode=bearer")
	}
	if cfg.alertmanagerAuthMode == api.AlertSourceAuthModeBearer && cfg.alertmanagerSecretRef == "" {
		return config{}, fmt.Errorf("--alertmanager-secret-ref is required for bearer auth")
	}
	if cfg.alertmanagerSecretRef != "" && strings.ContainsAny(cfg.alertmanagerSecretRef, "\r\n\t ") {
		return config{}, fmt.Errorf("--alertmanager-secret-ref must be a single value without whitespace")
	}
	cfg.notificationSecretRef = strings.TrimSpace(cfg.notificationSecretRef)
	if cfg.notificationSecretRef == "" {
		return config{}, fmt.Errorf("--notification-secret-ref is required")
	}
	if strings.ContainsAny(cfg.notificationSecretRef, "\r\n\t ") {
		return config{}, fmt.Errorf("--notification-secret-ref must be a single value without whitespace")
	}
	if looksLikeHTTPURL(cfg.notificationSecretRef) {
		return config{}, fmt.Errorf("--notification-secret-ref must be a secret reference, not a webhook URL")
	}

	if cfg.sourceID < 0 || cfg.channelID < 0 || cfg.groupingPolicyID < 0 || cfg.reportWorkflowPolicyID < 0 {
		return config{}, fmt.Errorf("existing IDs must be non-negative")
	}
	if err := validateNonEmptySingleLine("--source-name", &cfg.sourceName); err != nil {
		return config{}, err
	}
	if err := validateNonEmptySingleLine("--channel-name", &cfg.channelName); err != nil {
		return config{}, err
	}
	if err := validateNonEmptySingleLine("--grouping-policy-name", &cfg.groupingPolicyName); err != nil {
		return config{}, err
	}
	if err := validateNonEmptySingleLine("--report-workflow-policy-name", &cfg.reportWorkflowPolicyName); err != nil {
		return config{}, err
	}

	scenario = strings.ToLower(strings.TrimSpace(scenario))
	switch api.ReportWorkflowScenario(scenario) {
	case api.ReportWorkflowScenarioSingleAlert, api.ReportWorkflowScenarioCascade, api.ReportWorkflowScenarioAlertStorm:
		cfg.reportScenario = api.ReportWorkflowScenario(scenario)
	default:
		return config{}, fmt.Errorf("--report-scenario must be single_alert, cascade, or alert_storm")
	}

	bearerToken, err := resolveBearerToken(cfg.bearerToken, cfg.bearerTokenEnv)
	if err != nil {
		return config{}, err
	}
	cfg.bearerToken = bearerToken
	cfg.outputPath = filepath.Clean(strings.TrimSpace(cfg.outputPath))
	if cfg.outputPath == "." || cfg.outputPath == "" {
		return config{}, fmt.Errorf("--output is required")
	}
	cfg.envOutputPath = strings.TrimSpace(cfg.envOutputPath)
	if cfg.envOutputPath != "" {
		cfg.envOutputPath = filepath.Clean(cfg.envOutputPath)
		if cfg.envOutputPath == "." {
			return config{}, fmt.Errorf("--env-output must be a file path when set")
		}
	}
	if cfg.timeout <= 0 {
		return config{}, fmt.Errorf("--timeout must be greater than zero")
	}
	return cfg, nil
}

func setupAlertConsultation(ctx context.Context, client *http.Client, cfg config) (setupResult, error) {
	source, err := upsertAlertSourceProfile(ctx, client, cfg)
	if err != nil {
		return setupResult{}, err
	}
	channel, err := upsertNotificationChannelProfile(ctx, client, cfg)
	if err != nil {
		return setupResult{}, err
	}
	grouping, err := upsertGroupingPolicy(ctx, client, cfg)
	if err != nil {
		return setupResult{}, err
	}
	var notificationAIProofs []api.NotificationChannelTestResult
	if cfg.enablePolicy {
		notificationAIProofs, err = testNotificationAIProofs(ctx, client, cfg, channel.ID)
		if err != nil {
			return setupResult{}, err
		}
	}
	policy, err := upsertReportWorkflowPolicy(ctx, client, cfg, source.ID, grouping.ID, channel.ID)
	if err != nil {
		return setupResult{}, err
	}
	enabled := false
	if cfg.enablePolicy {
		policy, err = enableReportWorkflowPolicy(ctx, client, cfg, policy.ID)
		if err != nil {
			return setupResult{}, err
		}
		enabled = true
	}
	return setupResult{
		AlertSourceProfile:      source,
		NotificationChannel:     channel,
		GroupingPolicy:          grouping,
		ReportWorkflowPolicy:    policy,
		NotificationAIProofs:    notificationAIProofs,
		ReportWorkflowWasEnable: enabled,
	}, nil
}

func upsertAlertSourceProfile(ctx context.Context, client *http.Client, cfg config) (api.AlertSourceProfile, error) {
	enabled := true
	labels := api.AlertSourceLabels{
		"provider": "alertmanager",
		"role":     "auto-diagnosis",
	}
	req := api.AlertSourceProfileWriteRequest{
		Name:     cfg.sourceName,
		Kind:     api.Alertmanager,
		BaseURL:  cfg.alertmanagerBaseURL,
		AuthMode: cfg.alertmanagerAuthMode,
		Enabled:  &enabled,
		Labels:   &labels,
	}
	if cfg.alertmanagerSecretRef != "" {
		req.SecretRef = &cfg.alertmanagerSecretRef
	}
	method := http.MethodPost
	endpoint := configURL(cfg.apiBaseURL, "alert-sources")
	wantStatus := http.StatusCreated
	if cfg.sourceID > 0 {
		method = http.MethodPut
		endpoint = configURL(cfg.apiBaseURL, "alert-sources", strconv.FormatInt(cfg.sourceID, 10))
		wantStatus = http.StatusOK
	}
	var out api.AlertSourceProfile
	if err := callJSON(ctx, client, cfg, method, endpoint, req, wantStatus, &out); err != nil {
		if cfg.sourceID == 0 && isConflictAPIStatus(err) {
			existing, findErr := findAlertSourceProfileByName(ctx, client, cfg, cfg.sourceName)
			if findErr != nil {
				return api.AlertSourceProfile{}, fmt.Errorf("upsert alert source profile: %w; lookup existing profile: %w", err, findErr)
			}
			endpoint = configURL(cfg.apiBaseURL, "alert-sources", strconv.FormatInt(existing.ID, 10))
			if replaceErr := callJSON(ctx, client, cfg, http.MethodPut, endpoint, req, http.StatusOK, &out); replaceErr != nil {
				return api.AlertSourceProfile{}, fmt.Errorf("upsert alert source profile: replace existing profile %d: %w", existing.ID, replaceErr)
			}
			return out, nil
		}
		return api.AlertSourceProfile{}, fmt.Errorf("upsert alert source profile: %w", err)
	}
	return out, nil
}

func upsertNotificationChannelProfile(ctx context.Context, client *http.Client, cfg config) (api.NotificationChannelProfile, error) {
	enabled := true
	labels := api.NotificationChannelLabels{
		"provider": "wecom",
		"role":     "ai-room-delivery",
	}
	req := api.NotificationChannelProfileWriteRequest{
		Name:      cfg.channelName,
		Kind:      api.Wecom,
		SecretRef: cfg.notificationSecretRef,
		DeliveryScopes: []api.NotificationDeliveryScope{
			api.Report,
			api.DiagnosisConsultation,
			api.DiagnosisClose,
		},
		Enabled: &enabled,
		Labels:  &labels,
	}
	method := http.MethodPost
	endpoint := configURL(cfg.apiBaseURL, "notification-channels")
	wantStatus := http.StatusCreated
	if cfg.channelID > 0 {
		method = http.MethodPut
		endpoint = configURL(cfg.apiBaseURL, "notification-channels", strconv.FormatInt(cfg.channelID, 10))
		wantStatus = http.StatusOK
	}
	var out api.NotificationChannelProfile
	if err := callJSON(ctx, client, cfg, method, endpoint, req, wantStatus, &out); err != nil {
		if cfg.channelID == 0 && isConflictAPIStatus(err) {
			existing, findErr := findNotificationChannelProfileByName(ctx, client, cfg, cfg.channelName)
			if findErr != nil {
				return api.NotificationChannelProfile{}, fmt.Errorf("upsert notification channel profile: %w; lookup existing profile: %w", err, findErr)
			}
			endpoint = configURL(cfg.apiBaseURL, "notification-channels", strconv.FormatInt(existing.ID, 10))
			if replaceErr := callJSON(ctx, client, cfg, http.MethodPut, endpoint, req, http.StatusOK, &out); replaceErr != nil {
				return api.NotificationChannelProfile{}, fmt.Errorf("upsert notification channel profile: replace existing profile %d: %w", existing.ID, replaceErr)
			}
			return out, nil
		}
		return api.NotificationChannelProfile{}, fmt.Errorf("upsert notification channel profile: %w", err)
	}
	return out, nil
}

func upsertGroupingPolicy(ctx context.Context, client *http.Client, cfg config) (api.GroupingPolicy, error) {
	enabled := true
	req := api.GroupingPolicyWriteRequest{
		Name:          cfg.groupingPolicyName,
		DimensionKeys: []string{"alertname", "namespace", "service", "instance"},
		SeverityKey:   "severity",
		SourceFilter:  []string{"alertmanager"},
		Enabled:       &enabled,
	}
	method := http.MethodPost
	endpoint := configURL(cfg.apiBaseURL, "grouping-policies")
	wantStatus := http.StatusCreated
	if cfg.groupingPolicyID > 0 {
		method = http.MethodPut
		endpoint = configURL(cfg.apiBaseURL, "grouping-policies", strconv.FormatInt(cfg.groupingPolicyID, 10))
		wantStatus = http.StatusOK
	}
	var out api.GroupingPolicy
	if err := callJSON(ctx, client, cfg, method, endpoint, req, wantStatus, &out); err != nil {
		if cfg.groupingPolicyID == 0 && isConflictAPIStatus(err) {
			existing, findErr := findGroupingPolicyByName(ctx, client, cfg, cfg.groupingPolicyName)
			if findErr != nil {
				return api.GroupingPolicy{}, fmt.Errorf("upsert grouping policy: %w; lookup existing policy: %w", err, findErr)
			}
			endpoint = configURL(cfg.apiBaseURL, "grouping-policies", strconv.FormatInt(existing.ID, 10))
			if replaceErr := callJSON(ctx, client, cfg, http.MethodPut, endpoint, req, http.StatusOK, &out); replaceErr != nil {
				return api.GroupingPolicy{}, fmt.Errorf("upsert grouping policy: replace existing policy %d: %w", existing.ID, replaceErr)
			}
			return out, nil
		}
		return api.GroupingPolicy{}, fmt.Errorf("upsert grouping policy: %w", err)
	}
	return out, nil
}

func upsertReportWorkflowPolicy(
	ctx context.Context,
	client *http.Client,
	cfg config,
	sourceID int64,
	groupingPolicyID int64,
	channelID int64,
) (api.ReportWorkflowPolicy, error) {
	channel := api.Nullable[int64]{}
	channel.Set(channelID)
	triggerMode := string(api.ReportWorkflowTriggerModeManualReplay)
	reportScenario := string(cfg.reportScenario)
	diagnosisFollowUp := string(api.DiagnosisFollowUpModeAutoRoom)
	req := api.ReportWorkflowPolicyWriteRequest{
		Name:                               cfg.reportWorkflowPolicyName,
		AlertSourceProfileID:               sourceID,
		GroupingPolicyID:                   groupingPolicyID,
		ReportNotificationChannelProfileID: channel,
		TriggerMode:                        &triggerMode,
		ReportScenario:                     &reportScenario,
		DiagnosisFollowUp:                  &diagnosisFollowUp,
	}
	method := http.MethodPost
	endpoint := configURL(cfg.apiBaseURL, "report-workflow-policies")
	wantStatus := http.StatusCreated
	if cfg.reportWorkflowPolicyID > 0 {
		method = http.MethodPut
		endpoint = configURL(cfg.apiBaseURL, "report-workflow-policies", strconv.FormatInt(cfg.reportWorkflowPolicyID, 10))
		wantStatus = http.StatusOK
	}
	var out api.ReportWorkflowPolicy
	if err := callJSON(ctx, client, cfg, method, endpoint, req, wantStatus, &out); err != nil {
		if cfg.reportWorkflowPolicyID == 0 && isConflictAPIStatus(err) {
			existing, findErr := findReportWorkflowPolicyByName(ctx, client, cfg, cfg.reportWorkflowPolicyName)
			if findErr != nil {
				return api.ReportWorkflowPolicy{}, fmt.Errorf("upsert report workflow policy: %w; lookup existing policy: %w", err, findErr)
			}
			endpoint = configURL(cfg.apiBaseURL, "report-workflow-policies", strconv.FormatInt(existing.ID, 10))
			if replaceErr := callJSON(ctx, client, cfg, http.MethodPut, endpoint, req, http.StatusOK, &out); replaceErr != nil {
				return api.ReportWorkflowPolicy{}, fmt.Errorf("upsert report workflow policy: replace existing policy %d: %w", existing.ID, replaceErr)
			}
			return out, nil
		}
		return api.ReportWorkflowPolicy{}, fmt.Errorf("upsert report workflow policy: %w", err)
	}
	return out, nil
}

func findAlertSourceProfileByName(ctx context.Context, client *http.Client, cfg config, name string) (api.AlertSourceProfile, error) {
	endpoint, err := listURL(configURL(cfg.apiBaseURL, "alert-sources"))
	if err != nil {
		return api.AlertSourceProfile{}, err
	}
	var out api.AlertSourceProfileListResponse
	if err := callJSON(ctx, client, cfg, http.MethodGet, endpoint, nil, http.StatusOK, &out); err != nil {
		return api.AlertSourceProfile{}, err
	}
	for _, item := range out.Items {
		if item.Name == name {
			return item, nil
		}
	}
	return api.AlertSourceProfile{}, fmt.Errorf("alert source profile named %q was not found", name)
}

func findNotificationChannelProfileByName(ctx context.Context, client *http.Client, cfg config, name string) (api.NotificationChannelProfile, error) {
	endpoint, err := listURL(configURL(cfg.apiBaseURL, "notification-channels"))
	if err != nil {
		return api.NotificationChannelProfile{}, err
	}
	var out api.NotificationChannelProfileListResponse
	if err := callJSON(ctx, client, cfg, http.MethodGet, endpoint, nil, http.StatusOK, &out); err != nil {
		return api.NotificationChannelProfile{}, err
	}
	for _, item := range out.Items {
		if item.Name == name {
			return item, nil
		}
	}
	return api.NotificationChannelProfile{}, fmt.Errorf("notification channel profile named %q was not found", name)
}

func findGroupingPolicyByName(ctx context.Context, client *http.Client, cfg config, name string) (api.GroupingPolicy, error) {
	endpoint, err := listURL(configURL(cfg.apiBaseURL, "grouping-policies"))
	if err != nil {
		return api.GroupingPolicy{}, err
	}
	var out api.GroupingPolicyListResponse
	if err := callJSON(ctx, client, cfg, http.MethodGet, endpoint, nil, http.StatusOK, &out); err != nil {
		return api.GroupingPolicy{}, err
	}
	for _, item := range out.Items {
		if item.Name == name {
			return item, nil
		}
	}
	return api.GroupingPolicy{}, fmt.Errorf("grouping policy named %q was not found", name)
}

func findReportWorkflowPolicyByName(ctx context.Context, client *http.Client, cfg config, name string) (api.ReportWorkflowPolicy, error) {
	endpoint, err := listURL(configURL(cfg.apiBaseURL, "report-workflow-policies"))
	if err != nil {
		return api.ReportWorkflowPolicy{}, err
	}
	var out api.ReportWorkflowPolicyListResponse
	if err := callJSON(ctx, client, cfg, http.MethodGet, endpoint, nil, http.StatusOK, &out); err != nil {
		return api.ReportWorkflowPolicy{}, err
	}
	for _, item := range out.Items {
		if item.Name == name {
			return item, nil
		}
	}
	return api.ReportWorkflowPolicy{}, fmt.Errorf("report workflow policy named %q was not found", name)
}

func enableReportWorkflowPolicy(ctx context.Context, client *http.Client, cfg config, policyID int64) (api.ReportWorkflowPolicy, error) {
	endpoint := configURL(cfg.apiBaseURL, "report-workflow-policies", strconv.FormatInt(policyID, 10), "enable")
	var out api.ReportWorkflowPolicy
	if err := callJSON(ctx, client, cfg, http.MethodPost, endpoint, nil, http.StatusOK, &out); err != nil {
		return api.ReportWorkflowPolicy{}, fmt.Errorf("enable report workflow policy: %w", err)
	}
	return out, nil
}

func testNotificationAIProofs(ctx context.Context, client *http.Client, cfg config, channelID int64) ([]api.NotificationChannelTestResult, error) {
	results := make([]api.NotificationChannelTestResult, 0, len(requiredNotificationAIProofContentKinds))
	for _, contentKind := range requiredNotificationAIProofContentKinds {
		result, err := testNotificationChannelContentKind(ctx, client, cfg, channelID, contentKind)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func testNotificationChannelContentKind(
	ctx context.Context,
	client *http.Client,
	cfg config,
	channelID int64,
	contentKind string,
) (api.NotificationChannelTestResult, error) {
	endpoint, err := notificationChannelTestURL(cfg.apiBaseURL, channelID, contentKind)
	if err != nil {
		return api.NotificationChannelTestResult{}, err
	}
	var out api.NotificationChannelTestResult
	if err := callJSON(ctx, client, cfg, http.MethodPost, endpoint, nil, http.StatusOK, &out); err != nil {
		return api.NotificationChannelTestResult{}, fmt.Errorf("test notification channel content %q: %w", contentKind, err)
	}
	if err := validateNotificationAIProofResult(out, channelID, contentKind); err != nil {
		return api.NotificationChannelTestResult{}, fmt.Errorf("test notification channel content %q: %w", contentKind, err)
	}
	return out, nil
}

func notificationChannelTestURL(base string, channelID int64, contentKind string) (string, error) {
	endpoint := configURL(base, "notification-channels", strconv.FormatInt(channelID, 10), "test")
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("build notification channel test URL: %w", err)
	}
	q := u.Query()
	q.Set("content_kind", contentKind)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func validateNotificationAIProofResult(result api.NotificationChannelTestResult, channelID int64, contentKind string) error {
	if result.ChannelID != channelID {
		return fmt.Errorf("result.channel_id = %d, want %d", result.ChannelID, channelID)
	}
	if result.Kind != api.Wecom {
		return fmt.Errorf("result.kind = %q, want wecom", result.Kind)
	}
	if result.Status != api.NotificationChannelTestStatusSuccess {
		return fmt.Errorf("result.status = %q, want success; reason_code=%q", result.Status, result.ReasonCode)
	}
	if result.ReasonCode != api.NotificationChannelTestReasonCodeOk {
		return fmt.Errorf("successful result.reason_code = %q, want ok", result.ReasonCode)
	}
	if result.ContentKind == nil || *result.ContentKind != contentKind {
		got := ""
		if result.ContentKind != nil {
			got = *result.ContentKind
		}
		return fmt.Errorf("result.content_kind = %q, want %s", got, contentKind)
	}
	if result.ContentSha256 == nil || !validLowercaseSHA256(*result.ContentSha256) {
		return fmt.Errorf("result.content_sha256 must be a lowercase SHA-256 digest")
	}
	if result.CheckedAt.IsZero() {
		return fmt.Errorf("result.checked_at must be present")
	}
	if !notificationProviderStatusAccepted(result.ProviderStatus) {
		return fmt.Errorf("result.provider_status = %q, want accepted, delivered, sent, or success", result.ProviderStatus)
	}
	return nil
}

func callJSON(
	ctx context.Context,
	client *http.Client,
	cfg config,
	method string,
	endpoint string,
	body any,
	wantStatus int,
	out any,
) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cfg.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.bearerToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if int64(len(raw)) > maxResponseBytes {
		return fmt.Errorf("response body exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode != wantStatus {
		message := sanitizedAPIError(raw)
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return apiStatusError{statusCode: resp.StatusCode, message: message}
	}
	if out != nil {
		if len(bytes.TrimSpace(raw)) == 0 {
			return fmt.Errorf("response body is empty")
		}
		if err := strictjson.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

func isConflictAPIStatus(err error) bool {
	var statusErr apiStatusError
	return errors.As(err, &statusErr) && statusErr.statusCode == http.StatusConflict
}

func sanitizedAPIError(raw []byte) string {
	var errResp api.ErrorResponse
	if err := json.Unmarshal(raw, &errResp); err == nil {
		return singleLine(strings.TrimSpace(errResp.Error), 512)
	}
	return ""
}

func configURL(base string, parts ...string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	segments := append([]string{strings.Trim(u.Path, "/"), "api", "v1", "config"}, parts...)
	u.Path = "/" + path.Join(segments...)
	return u.String()
}

func listURL(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("build list URL: %w", err)
	}
	q := u.Query()
	q.Set("limit", strconv.Itoa(500))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func proofFromResult(cfg config, result setupResult) proof {
	scopes := make([]string, 0, len(result.NotificationChannel.DeliveryScopes))
	for _, scope := range result.NotificationChannel.DeliveryScopes {
		scopes = append(scopes, string(scope))
	}
	return proof{
		CheckedAt: nowUTC().Format(time.RFC3339Nano),
		Request: proofRequest{
			APIBaseURL:                  cfg.apiBaseURL,
			AlertmanagerBaseURLSet:      cfg.alertmanagerBaseURL != "",
			AlertmanagerAuthMode:        string(cfg.alertmanagerAuthMode),
			AlertmanagerSecretRef:       cfg.alertmanagerSecretRef != "",
			NotificationSecretRef:       cfg.notificationSecretRef != "",
			SourceID:                    cfg.sourceID,
			ChannelID:                   cfg.channelID,
			GroupingPolicyID:            cfg.groupingPolicyID,
			ReportWorkflowPolicyID:      cfg.reportWorkflowPolicyID,
			ReportScenario:              string(cfg.reportScenario),
			EnablePolicy:                cfg.enablePolicy,
			NotificationAIProofRequired: cfg.enablePolicy,
			Timeout:                     cfg.timeout.String(),
		},
		Result: proofResult{
			AlertSourceProfileID:   result.AlertSourceProfile.ID,
			NotificationChannelID:  result.NotificationChannel.ID,
			GroupingPolicyID:       result.GroupingPolicy.ID,
			ReportWorkflowPolicyID: result.ReportWorkflowPolicy.ID,
			ReportWorkflowEnabled:  result.ReportWorkflowPolicy.Enabled,
			NotificationKind:       string(result.NotificationChannel.Kind),
			NotificationScopes:     scopes,
			DiagnosisFollowUp:      string(result.ReportWorkflowPolicy.DiagnosisFollowUp),
			AlertSourceKind:        string(result.AlertSourceProfile.Kind),
			NotificationAIProofs:   notificationAIProofSummaries(result.NotificationAIProofs),
		},
		Evidence: proofEvidence,
	}
}

func notificationAIProofSummaries(results []api.NotificationChannelTestResult) []proofNotificationAIProof {
	if len(results) == 0 {
		return nil
	}
	out := make([]proofNotificationAIProof, 0, len(results))
	for _, result := range results {
		contentKind := ""
		if result.ContentKind != nil {
			contentKind = *result.ContentKind
		}
		checkedAt := ""
		if !result.CheckedAt.IsZero() {
			checkedAt = result.CheckedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, proofNotificationAIProof{
			ContentKind:          contentKind,
			Status:               string(result.Status),
			ReasonCode:           string(result.ReasonCode),
			ProviderStatus:       result.ProviderStatus,
			ContentSHA256Present: result.ContentSha256 != nil && *result.ContentSha256 != "",
			CheckedAt:            checkedAt,
		})
	}
	return out
}

func validateProof(out proof) error {
	if err := validateCheckedAt(out.CheckedAt); err != nil {
		return err
	}
	if err := validateAbsoluteHTTPURL("request.api_base_url", out.Request.APIBaseURL); err != nil {
		return err
	}
	if !out.Request.AlertmanagerBaseURLSet {
		return fmt.Errorf("request.alertmanager_base_url_set must be true")
	}
	switch api.AlertSourceAuthMode(out.Request.AlertmanagerAuthMode) {
	case api.AlertSourceAuthModeNone, api.AlertSourceAuthModeBearer:
	default:
		return fmt.Errorf("request.alertmanager_auth_mode is unsupported")
	}
	switch api.ReportWorkflowScenario(out.Request.ReportScenario) {
	case api.ReportWorkflowScenarioSingleAlert, api.ReportWorkflowScenarioCascade, api.ReportWorkflowScenarioAlertStorm:
	default:
		return fmt.Errorf("request.report_scenario is unsupported")
	}
	if timeout, err := time.ParseDuration(out.Request.Timeout); err != nil || timeout <= 0 {
		return fmt.Errorf("request.timeout must be a positive duration")
	}
	if out.Result.AlertSourceProfileID <= 0 {
		return fmt.Errorf("result.alert_source_profile_id must be positive")
	}
	if out.Result.NotificationChannelID <= 0 {
		return fmt.Errorf("result.notification_channel_profile_id must be positive")
	}
	if out.Result.GroupingPolicyID <= 0 {
		return fmt.Errorf("result.grouping_policy_id must be positive")
	}
	if out.Result.ReportWorkflowPolicyID <= 0 {
		return fmt.Errorf("result.report_workflow_policy_id must be positive")
	}
	if out.Request.EnablePolicy && !out.Result.ReportWorkflowEnabled {
		return fmt.Errorf("result.report_workflow_enabled must be true when enable_policy=true")
	}
	if out.Request.EnablePolicy && !out.Request.NotificationAIProofRequired {
		return fmt.Errorf("request.notification_ai_proof_required must be true when enable_policy=true")
	}
	if out.Result.NotificationKind != string(api.Wecom) {
		return fmt.Errorf("result.notification_kind must be wecom")
	}
	if !hasAllNotificationScopes(out.Result.NotificationScopes) {
		return fmt.Errorf("result.notification_scopes must include report, diagnosis_consultation, and diagnosis_close")
	}
	if out.Result.DiagnosisFollowUp != string(api.DiagnosisFollowUpModeAutoRoom) {
		return fmt.Errorf("result.diagnosis_follow_up must be auto_room")
	}
	if out.Result.AlertSourceKind != string(api.Alertmanager) {
		return fmt.Errorf("result.alert_source_kind must be alertmanager")
	}
	if err := validateNotificationAIProofSummaries(out.Result.NotificationAIProofs, out.Request.NotificationAIProofRequired); err != nil {
		return err
	}
	if out.Evidence != proofEvidence {
		return fmt.Errorf("evidence must be %q", proofEvidence)
	}
	return nil
}

func validateNotificationAIProofSummaries(proofs []proofNotificationAIProof, required bool) error {
	if len(proofs) == 0 {
		if required {
			return fmt.Errorf("result.notification_ai_proofs must include AI diagnosis and diagnosis close samples")
		}
		return nil
	}
	if len(proofs) != len(requiredNotificationAIProofContentKinds) {
		return fmt.Errorf("result.notification_ai_proofs count = %d, want %d", len(proofs), len(requiredNotificationAIProofContentKinds))
	}
	for i, proof := range proofs {
		wantContentKind := requiredNotificationAIProofContentKinds[i]
		if proof.ContentKind != wantContentKind {
			return fmt.Errorf("result.notification_ai_proofs[%d].content_kind = %q, want %s", i, proof.ContentKind, wantContentKind)
		}
		if proof.Status != string(api.NotificationChannelTestStatusSuccess) {
			return fmt.Errorf("result.notification_ai_proofs[%d].status = %q, want success", i, proof.Status)
		}
		if proof.ReasonCode != string(api.NotificationChannelTestReasonCodeOk) {
			return fmt.Errorf("result.notification_ai_proofs[%d].reason_code = %q, want ok", i, proof.ReasonCode)
		}
		if !proof.ContentSHA256Present {
			return fmt.Errorf("result.notification_ai_proofs[%d].content_sha256_present must be true", i)
		}
		if err := validateCheckedAt(proof.CheckedAt); err != nil {
			return fmt.Errorf("result.notification_ai_proofs[%d].checked_at: %w", i, err)
		}
		if !notificationProviderStatusAccepted(proof.ProviderStatus) {
			return fmt.Errorf("result.notification_ai_proofs[%d].provider_status = %q, want accepted, delivered, sent, or success", i, proof.ProviderStatus)
		}
	}
	return nil
}

func writeJSONFile(outputPath string, out proof) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode proof: %w", err)
	}
	raw = append(raw, '\n')
	// #nosec G304 -- manual setup output path is supplied by the operator.
	if err := os.WriteFile(outputPath, raw, 0o600); err != nil {
		return fmt.Errorf("write proof: %w", err)
	}
	return nil
}

func writeEnvFile(outputPath string, out proof) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("create env output directory: %w", err)
	}
	lines := []string{
		"# Generated by alert_consultation_setup. Contains IDs only; no secrets.",
		"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID=" + shellQuoteInt(out.Result.AlertSourceProfileID),
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID=" + shellQuoteInt(out.Result.AlertSourceProfileID),
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=" + shellQuoteInt(out.Result.NotificationChannelID),
		"NOTIFICATION_CHANNEL_PROFILE_ID=" + shellQuoteInt(out.Result.NotificationChannelID),
		"OPENCLARION_LIVE_GROUPING_POLICY_ID=" + shellQuoteInt(out.Result.GroupingPolicyID),
		"OPENCLARION_LIVE_REPORT_WORKFLOW_POLICY_ID=" + shellQuoteInt(out.Result.ReportWorkflowPolicyID),
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID=" + shellQuoteInt(out.Result.NotificationChannelID),
		"NOTIFICATION_CHANNEL_EXPECTED_KIND='wecom'",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND='wecom'",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF='true'",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF='true'",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND='final_conclusion'",
	}
	raw := []byte(strings.Join(lines, "\n") + "\n")
	// #nosec G304 -- manual setup env output path is supplied by the operator.
	if err := os.WriteFile(outputPath, raw, 0o600); err != nil {
		return fmt.Errorf("write setup env: %w", err)
	}
	return nil
}

func validateAbsoluteHTTPURL(label, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s is required", label)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("%s must be an absolute URL", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", label)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not contain userinfo", label)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain query or fragment", label)
	}
	return nil
}

func validateNonEmptySingleLine(label string, value *string) error {
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return fmt.Errorf("%s is required", label)
	}
	if strings.ContainsAny(trimmed, "\r\n\t") {
		return fmt.Errorf("%s must be a single line", label)
	}
	*value = trimmed
	return nil
}

func validateCheckedAt(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("checked_at must be non-empty")
	}
	if value != raw {
		return fmt.Errorf("checked_at must not contain leading or trailing whitespace")
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fmt.Errorf("checked_at must be RFC3339: %w", err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return fmt.Errorf("checked_at must be canonical UTC RFC3339")
	}
	if checkedAt.After(nowUTC()) {
		return fmt.Errorf("checked_at must not be in the future")
	}
	return nil
}

func hasAllNotificationScopes(scopes []string) bool {
	seen := map[string]bool{}
	for _, scope := range scopes {
		seen[scope] = true
	}
	return seen[string(api.Report)] &&
		seen[string(api.DiagnosisConsultation)] &&
		seen[string(api.DiagnosisClose)]
}

func notificationProviderStatusAccepted(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted", "delivered", "sent", "success":
		return true
	default:
		return false
	}
}

func validLowercaseSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func looksLikeHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func resolveBearerToken(value, envName string) (string, error) {
	token, err := normalizeBearerToken(value, "--bearer-token")
	if err != nil {
		return "", err
	}
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return token, nil
	}
	if token != "" {
		return "", fmt.Errorf("--bearer-token and --bearer-token-env are mutually exclusive")
	}
	if !validEnvVarName(envName) {
		return "", fmt.Errorf("--bearer-token-env must be a valid environment variable name")
	}
	envValue, ok := os.LookupEnv(envName)
	if !ok {
		return "", fmt.Errorf("--bearer-token-env references an unset environment variable")
	}
	token, err = normalizeBearerToken(envValue, "--bearer-token-env value")
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("--bearer-token-env value must be non-empty")
	}
	return token, nil
}

func normalizeBearerToken(value, label string) (string, error) {
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

func singleLine(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\t':
			return ' '
		default:
			return r
		}
	}, value)
	for strings.Contains(value, "  ") {
		value = strings.ReplaceAll(value, "  ", " ")
	}
	if len([]byte(value)) <= maxBytes {
		return value
	}
	return string([]byte(value)[:maxBytes])
}

func shellQuoteInt(value int64) string {
	return "'" + strconv.FormatInt(value, 10) + "'"
}
