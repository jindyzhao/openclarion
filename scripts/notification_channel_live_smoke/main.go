// Command notification_channel_live_smoke exercises one configured
// notification channel through the running OpenClarion HTTP API and writes a
// sanitized proof artifact.
package main

import (
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
	"strings"
	"time"

	"github.com/openclarion/openclarion/api"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	maxResponseBytes          int64 = 1024 * 1024
	maxProviderMessageIDBytes       = 128
	maxProviderStatusBytes          = 64
)

var nowUTC = func() time.Time {
	return time.Now().UTC()
}

type config struct {
	apiBaseURL           string
	bearerToken          string
	bearerTokenEnv       string
	channelID            int64
	expectedKind         api.NotificationChannelKind
	expectedContentKinds []string
	requireAIProof       bool
	outputPath           string
	timeout              time.Duration
}

type proof struct {
	Passed     bool                                `json:"passed"`
	CheckedAt  string                              `json:"checked_at"`
	Request    proofRequest                        `json:"request"`
	HTTPStatus int                                 `json:"http_status"`
	Result     api.NotificationChannelTestResult   `json:"result"`
	Results    []api.NotificationChannelTestResult `json:"results,omitempty"`
	Evidence   string                              `json:"evidence"`
}

type proofRequest struct {
	APIBaseURL           string   `json:"api_base_url"`
	ChannelID            int64    `json:"channel_id"`
	ExpectedKind         string   `json:"expected_kind,omitempty"`
	ExpectedContentKind  string   `json:"expected_content_kind,omitempty"`
	ExpectedContentKinds []string `json:"expected_content_kinds,omitempty"`
	RequireAIProof       bool     `json:"require_ai_proof,omitempty"`
	Timeout              string   `json:"timeout"`
}

func main() {
	if err := run(os.Args[1:], http.DefaultClient); err != nil {
		fmt.Fprintf(os.Stderr, "[notification-channel-live-smoke] FAIL: %v\n", err)
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

	responses, err := testNotificationChannels(ctx, client, cfg)
	if err != nil {
		return err
	}
	result := responses[0].Result
	results := make([]api.NotificationChannelTestResult, len(responses))
	for i, resp := range responses {
		results[i] = resp.Result
	}
	expectedContentKind := ""
	expectedContentKinds := append([]string(nil), cfg.expectedContentKinds...)
	if len(expectedContentKinds) == 1 {
		expectedContentKind = expectedContentKinds[0]
		expectedContentKinds = nil
	}
	out := proof{
		Passed:    notificationChannelResultsPassed(results),
		CheckedAt: nowUTC().Format(time.RFC3339Nano),
		Request: proofRequest{
			APIBaseURL:           cfg.apiBaseURL,
			ChannelID:            cfg.channelID,
			ExpectedKind:         string(cfg.expectedKind),
			ExpectedContentKind:  expectedContentKind,
			ExpectedContentKinds: expectedContentKinds,
			RequireAIProof:       cfg.requireAIProof,
			Timeout:              cfg.timeout.String(),
		},
		HTTPStatus: responses[0].HTTPStatus,
		Result:     result,
		Evidence:   notificationChannelEvidence(results),
	}
	if len(results) > 1 {
		out.Results = results
	}
	if err := validateProof(out, false); err != nil {
		return fmt.Errorf("internal proof validation failed: %w", err)
	}
	if err := writeProof(cfg.outputPath, out); err != nil {
		return err
	}
	if !out.Passed {
		failed := firstNonSuccessResult(results)
		contentKind := ""
		if failed.ContentKind != nil {
			contentKind = *failed.ContentKind
		}
		return fmt.Errorf("notification channel test content_kind=%s status=%s reason_code=%s; proof written to %s", contentKind, failed.Status, failed.ReasonCode, cfg.outputPath)
	}
	fmt.Fprintf(os.Stderr, "[notification-channel-live-smoke] OK - live smoke output: %s\n", cfg.outputPath)
	return nil
}

func parseArgs(args []string) (config, error) {
	var cfg config
	var expectedKind string
	var expectedContentKind string
	var expectedContentKinds string
	fs := flag.NewFlagSet("notification_channel_live_smoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.apiBaseURL, "api-base-url", "", "OpenClarion API base URL")
	fs.Int64Var(&cfg.channelID, "channel-id", 0, "notification channel profile ID to test")
	fs.StringVar(&expectedKind, "expected-kind", "", "optional expected channel kind: webhook or wecom; diagnosis samples imply wecom")
	fs.StringVar(&expectedContentKind, "expected-content-kind", "", "optional expected test content kind: transport_sample, ai_diagnosis_sample, or diagnosis_close_sample")
	fs.StringVar(&expectedContentKinds, "expected-content-kinds", "", "optional comma-separated expected test content kinds")
	fs.BoolVar(&cfg.requireAIProof, "require-ai-proof", false, "require both Enterprise WeChat AI diagnosis and diagnosis close sample tests")
	fs.StringVar(&cfg.bearerToken, "bearer-token", "", "unsupported direct bearer token value; use --bearer-token-env")
	fs.StringVar(&cfg.bearerTokenEnv, "bearer-token-env", "", "optional environment variable name containing the bearer token for the API")
	fs.StringVar(&cfg.outputPath, "output", "", "sanitized proof JSON output path")
	fs.DurationVar(&cfg.timeout, "timeout", 15*time.Second, "HTTP request timeout")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments")
	}
	cfg.apiBaseURL = strings.TrimRight(strings.TrimSpace(cfg.apiBaseURL), "/")
	if cfg.apiBaseURL == "" {
		return config{}, fmt.Errorf("--api-base-url is required")
	}
	parsedBase, err := url.Parse(cfg.apiBaseURL)
	if err != nil || parsedBase.Scheme == "" || parsedBase.Host == "" {
		return config{}, fmt.Errorf("--api-base-url must be an absolute URL")
	}
	if parsedBase.User != nil {
		return config{}, fmt.Errorf("--api-base-url must not contain userinfo")
	}
	if parsedBase.RawQuery != "" || parsedBase.Fragment != "" {
		return config{}, fmt.Errorf("--api-base-url must not contain query or fragment")
	}
	if cfg.channelID <= 0 {
		return config{}, fmt.Errorf("--channel-id must be positive")
	}
	expectedKind = strings.ToLower(strings.TrimSpace(expectedKind))
	switch api.NotificationChannelKind(expectedKind) {
	case "":
	case api.Webhook, api.Wecom:
		cfg.expectedKind = api.NotificationChannelKind(expectedKind)
	default:
		return config{}, fmt.Errorf("--expected-kind must be webhook or wecom when set")
	}
	if cfg.requireAIProof && (strings.TrimSpace(expectedContentKind) != "" || strings.TrimSpace(expectedContentKinds) != "") {
		return config{}, fmt.Errorf("--require-ai-proof cannot be combined with --expected-content-kind or --expected-content-kinds")
	}
	contentKinds, err := parseExpectedContentKinds(expectedContentKind, expectedContentKinds)
	if err != nil {
		return config{}, err
	}
	if cfg.requireAIProof {
		contentKinds = aiProofContentKinds()
	}
	cfg.expectedContentKinds = contentKinds
	if anyDiagnosisNotificationContentKind(cfg.expectedContentKinds) {
		if cfg.expectedKind == "" {
			cfg.expectedKind = api.Wecom
		}
		if cfg.expectedKind != api.Wecom {
			return config{}, fmt.Errorf("--expected-kind must be wecom when diagnosis content samples are expected")
		}
	}
	cfg.bearerToken, err = resolveOptionalSecretEnvBackedValue(cfg.bearerToken, cfg.bearerTokenEnv, "--bearer-token")
	if err != nil {
		return config{}, err
	}
	cfg.bearerToken = strings.TrimSpace(cfg.bearerToken)
	if strings.ContainsAny(cfg.bearerToken, "\r\n\t ") {
		return config{}, fmt.Errorf("--bearer-token must be a single token without whitespace")
	}
	cfg.outputPath = filepath.Clean(strings.TrimSpace(cfg.outputPath))
	if cfg.outputPath == "." || cfg.outputPath == "" {
		return config{}, fmt.Errorf("--output is required")
	}
	if cfg.timeout <= 0 {
		return config{}, fmt.Errorf("--timeout must be greater than zero")
	}
	return cfg, nil
}

func resolveEnvBackedValue(value, envName, label string) (string, error) {
	envName = strings.TrimSpace(envName)
	if envName == "" {
		return value, nil
	}
	if value != "" {
		return "", fmt.Errorf("%s and %s-env cannot both be set", label, label)
	}
	if !validEnvVarName(envName) {
		return "", fmt.Errorf("%s-env must be a valid environment variable name", label)
	}
	envValue, ok := os.LookupEnv(envName)
	if !ok || envValue == "" {
		return "", fmt.Errorf("%s-env environment variable must be set and non-empty", label)
	}
	return envValue, nil
}

func resolveOptionalSecretEnvBackedValue(value, envName, label string) (string, error) {
	if value != "" {
		return "", fmt.Errorf("%s must be provided with %s-env to avoid leaking credentials in process arguments", label, label)
	}
	return resolveEnvBackedValue("", envName, label)
}

func parseExpectedContentKinds(single, multi string) ([]string, error) {
	single = strings.TrimSpace(single)
	multi = strings.TrimSpace(multi)
	if single != "" && multi != "" {
		return nil, fmt.Errorf("--expected-content-kind and --expected-content-kinds cannot both be set")
	}
	raw := single
	if multi != "" {
		raw = multi
	}
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		value := strings.ToLower(strings.TrimSpace(part))
		if value == "" {
			return nil, fmt.Errorf("--expected-content-kinds must not contain empty entries")
		}
		if !validContentKind(value) {
			return nil, fmt.Errorf("--expected-content-kind must be transport_sample, ai_diagnosis_sample, or diagnosis_close_sample when set")
		}
		if seen[value] {
			return nil, fmt.Errorf("--expected-content-kinds must not contain duplicates")
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}

func aiProofContentKinds() []string {
	return []string{"ai_diagnosis_sample", "diagnosis_close_sample"}
}

type apiTestResponse struct {
	HTTPStatus int
	Result     api.NotificationChannelTestResult
}

func testNotificationChannels(ctx context.Context, client *http.Client, cfg config) ([]apiTestResponse, error) {
	contentKinds := cfg.expectedContentKinds
	if len(contentKinds) == 0 {
		contentKinds = []string{""}
	}
	out := make([]apiTestResponse, 0, len(contentKinds))
	for _, contentKind := range contentKinds {
		resp, err := testNotificationChannel(ctx, client, cfg, contentKind)
		if err != nil {
			return nil, err
		}
		out = append(out, resp)
	}
	return out, nil
}

func testNotificationChannel(ctx context.Context, client *http.Client, cfg config, contentKind string) (apiTestResponse, error) {
	endpoint, err := notificationChannelTestURL(cfg.apiBaseURL, cfg.channelID, contentKind)
	if err != nil {
		return apiTestResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return apiTestResponse{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if cfg.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.bearerToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return apiTestResponse{}, fmt.Errorf("call notification channel test endpoint: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return apiTestResponse{}, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(raw)) > maxResponseBytes {
		return apiTestResponse{}, fmt.Errorf("response body exceeds %d bytes", maxResponseBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return apiTestResponse{}, fmt.Errorf("test endpoint returned HTTP %d", resp.StatusCode)
	}
	var result api.NotificationChannelTestResult
	if err := strictjson.Unmarshal(raw, &result); err != nil {
		return apiTestResponse{}, fmt.Errorf("decode notification channel test response: %w", err)
	}
	return apiTestResponse{HTTPStatus: resp.StatusCode, Result: result}, nil
}

func notificationChannelTestURL(base string, channelID int64, contentKind string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "api", "v1", "config", "notification-channels", fmt.Sprintf("%d", channelID), "test")
	if contentKind != "" {
		query := u.Query()
		query.Set("content_kind", contentKind)
		u.RawQuery = query.Encode()
	}
	return u.String(), nil
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

func notificationChannelEvidence(results []api.NotificationChannelTestResult) string {
	if notificationChannelResultsPassed(results) {
		contentKinds := make([]string, 0, len(results))
		for _, result := range results {
			if result.ContentKind != nil && *result.ContentKind != "" {
				contentKinds = append(contentKinds, *result.ContentKind)
			}
		}
		if len(contentKinds) > 0 {
			return "notification_channel_test success:" + strings.Join(contentKinds, ",")
		}
		return "notification_channel_test success"
	}
	return "notification_channel_test completed_without_success"
}

func notificationChannelResultsPassed(results []api.NotificationChannelTestResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if result.Status != api.NotificationChannelTestStatusSuccess {
			return false
		}
	}
	return true
}

func firstNonSuccessResult(results []api.NotificationChannelTestResult) api.NotificationChannelTestResult {
	for _, result := range results {
		if result.Status != api.NotificationChannelTestStatusSuccess {
			return result
		}
	}
	if len(results) == 0 {
		return api.NotificationChannelTestResult{}
	}
	return results[0]
}

func notificationChannelSingleResultEvidence(result api.NotificationChannelTestResult) string {
	if result.Status == api.NotificationChannelTestStatusSuccess {
		if result.ContentKind != nil && *result.ContentKind != "" {
			return "notification_channel_test success:" + *result.ContentKind
		}
		return "notification_channel_test success"
	}
	return "notification_channel_test completed_without_success"
}

func validateProof(out proof, requireSuccess bool) error {
	checkedAt, err := validateCheckedAt(out.CheckedAt)
	if err != nil {
		return err
	}
	if err := validateRequest(out.Request, checkedAt); err != nil {
		return err
	}
	if out.HTTPStatus != http.StatusOK {
		return fmt.Errorf("http_status must be 200")
	}
	if err := validateResult(out.Result, requireSuccess); err != nil {
		return err
	}
	if err := validateResultMirror(out); err != nil {
		return err
	}
	results := proofResults(out)
	for _, result := range results {
		if err := validateResult(result, requireSuccess); err != nil {
			return err
		}
	}
	if err := validateResultChannelIDs(out.Request.ChannelID, results); err != nil {
		return err
	}
	wantPassed := notificationChannelResultsPassed(results)
	if out.Passed != wantPassed {
		return fmt.Errorf("passed must be %t for notification channel test results", wantPassed)
	}
	if out.Request.ExpectedKind != "" && out.Result.Kind != api.NotificationChannelKind(out.Request.ExpectedKind) {
		return fmt.Errorf("result.kind = %q, want %s", out.Result.Kind, out.Request.ExpectedKind)
	}
	for _, result := range results {
		if out.Request.ExpectedKind != "" && result.Kind != api.NotificationChannelKind(out.Request.ExpectedKind) {
			return fmt.Errorf("result.kind = %q, want %s", result.Kind, out.Request.ExpectedKind)
		}
	}
	if err := validateExpectedProofContentKinds(out.Request, results); err != nil {
		return err
	}
	wantEvidence := notificationChannelEvidence(results)
	if out.Evidence != wantEvidence {
		return fmt.Errorf("evidence must be %q", wantEvidence)
	}
	return nil
}

func validateResultMirror(out proof) error {
	if len(out.Results) == 0 {
		return nil
	}
	matches, err := sameNotificationChannelTestResult(out.Result, out.Results[0])
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("result must mirror results[0]")
	}
	return nil
}

func sameNotificationChannelTestResult(left, right api.NotificationChannelTestResult) (bool, error) {
	leftRaw, err := json.Marshal(left)
	if err != nil {
		return false, fmt.Errorf("encode result mirror left: %w", err)
	}
	rightRaw, err := json.Marshal(right)
	if err != nil {
		return false, fmt.Errorf("encode result mirror right: %w", err)
	}
	return string(leftRaw) == string(rightRaw), nil
}

func proofResults(out proof) []api.NotificationChannelTestResult {
	if len(out.Results) > 0 {
		return append([]api.NotificationChannelTestResult(nil), out.Results...)
	}
	return []api.NotificationChannelTestResult{out.Result}
}

func validateCheckedAt(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, fmt.Errorf("checked_at must be non-empty")
	}
	if value != raw {
		return time.Time{}, fmt.Errorf("checked_at must not contain leading or trailing whitespace")
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("checked_at must be RFC3339: %w", err)
	}
	if checkedAt.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, fmt.Errorf("checked_at must be canonical UTC RFC3339")
	}
	if checkedAt.After(nowUTC()) {
		return time.Time{}, fmt.Errorf("checked_at must not be in the future")
	}
	return checkedAt.UTC(), nil
}

func validateRequest(req proofRequest, checkedAt time.Time) error {
	if req.ChannelID <= 0 {
		return fmt.Errorf("request.channel_id must be positive")
	}
	base, err := url.Parse(req.APIBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("request.api_base_url must be an absolute URL")
	}
	if base.User != nil {
		return fmt.Errorf("request.api_base_url must not contain userinfo")
	}
	if base.RawQuery != "" || base.Fragment != "" {
		return fmt.Errorf("request.api_base_url must not contain query or fragment")
	}
	timeout, err := time.ParseDuration(req.Timeout)
	if err != nil || timeout <= 0 {
		return fmt.Errorf("request.timeout must be a positive duration")
	}
	if checkedAt.IsZero() {
		return fmt.Errorf("checked_at must be valid")
	}
	switch api.NotificationChannelKind(req.ExpectedKind) {
	case "":
	case api.Webhook, api.Wecom:
	default:
		return fmt.Errorf("request.expected_kind must be webhook or wecom when set")
	}
	if req.ExpectedContentKind != "" && !validContentKind(req.ExpectedContentKind) {
		return fmt.Errorf("request.expected_content_kind is unsupported")
	}
	if diagnosisNotificationContentKind(req.ExpectedContentKind) && api.NotificationChannelKind(req.ExpectedKind) != api.Wecom {
		return fmt.Errorf("request.expected_kind must be wecom for diagnosis notification content")
	}
	if len(req.ExpectedContentKinds) > 0 {
		seen := map[string]bool{}
		for _, contentKind := range req.ExpectedContentKinds {
			if !validContentKind(contentKind) {
				return fmt.Errorf("request.expected_content_kinds contains unsupported content kind")
			}
			if seen[contentKind] {
				return fmt.Errorf("request.expected_content_kinds must not contain duplicates")
			}
			seen[contentKind] = true
			if diagnosisNotificationContentKind(contentKind) && api.NotificationChannelKind(req.ExpectedKind) != api.Wecom {
				return fmt.Errorf("request.expected_kind must be wecom for diagnosis notification content")
			}
		}
	}
	if req.ExpectedContentKind != "" && len(req.ExpectedContentKinds) > 0 {
		return fmt.Errorf("request expected content kind fields are mutually exclusive")
	}
	if req.RequireAIProof {
		if api.NotificationChannelKind(req.ExpectedKind) != api.Wecom {
			return fmt.Errorf("request.expected_kind must be wecom when AI proof is required")
		}
		if req.ExpectedContentKind != "" {
			return fmt.Errorf("request.expected_content_kind must be empty when AI proof is required")
		}
		want := aiProofContentKinds()
		if len(req.ExpectedContentKinds) != len(want) {
			return fmt.Errorf("request.expected_content_kinds must include AI diagnosis and diagnosis close samples when AI proof is required")
		}
		for i, contentKind := range want {
			if req.ExpectedContentKinds[i] != contentKind {
				return fmt.Errorf("request.expected_content_kinds[%d] = %q, want %s when AI proof is required", i, req.ExpectedContentKinds[i], contentKind)
			}
		}
	}
	return nil
}

func validateExpectedProofContentKinds(req proofRequest, results []api.NotificationChannelTestResult) error {
	expected := expectedProofContentKinds(req)
	if len(expected) == 0 {
		return nil
	}
	if len(expected) != len(results) {
		return fmt.Errorf("result count = %d, want %d expected content kinds", len(results), len(expected))
	}
	for index, want := range expected {
		got := ""
		if results[index].ContentKind != nil {
			got = *results[index].ContentKind
		}
		if got != want {
			if len(expected) == 1 {
				return fmt.Errorf("result.content_kind = %q, want %s", got, want)
			}
			return fmt.Errorf("results[%d].content_kind = %q, want %s", index, got, want)
		}
	}
	return nil
}

func expectedProofContentKinds(req proofRequest) []string {
	if req.ExpectedContentKind != "" {
		return []string{req.ExpectedContentKind}
	}
	return append([]string(nil), req.ExpectedContentKinds...)
}

func validateResultChannelIDs(want int64, results []api.NotificationChannelTestResult) error {
	for index, result := range results {
		if result.ChannelID == want {
			continue
		}
		if len(results) == 1 {
			return fmt.Errorf("result.channel_id = %d, want %d", result.ChannelID, want)
		}
		return fmt.Errorf("results[%d].channel_id = %d, want %d", index, result.ChannelID, want)
	}
	return nil
}

func validateResult(result api.NotificationChannelTestResult, requireSuccess bool) error {
	if result.ChannelID <= 0 {
		return fmt.Errorf("result.channel_id must be positive")
	}
	switch result.Kind {
	case api.Webhook, api.Wecom:
	default:
		return fmt.Errorf("result.kind is unsupported")
	}
	switch result.Status {
	case api.NotificationChannelTestStatusSuccess:
		if result.ContentKind == nil || strings.TrimSpace(*result.ContentKind) == "" {
			return fmt.Errorf("result.content_kind must be present for success")
		}
		if !validContentKind(*result.ContentKind) {
			return fmt.Errorf("result.content_kind is unsupported")
		}
		if diagnosisNotificationContentKind(*result.ContentKind) && result.Kind != api.Wecom {
			return fmt.Errorf("result.kind must be wecom for diagnosis notification content")
		}
		if result.ContentSha256 == nil || !validLowercaseSHA256(*result.ContentSha256) {
			return fmt.Errorf("result.content_sha256 must be a lowercase SHA-256 digest for success")
		}
	case api.NotificationChannelTestStatusFailed,
		api.NotificationChannelTestStatusUnsupported,
		api.NotificationChannelTestStatusBlocked:
		if requireSuccess {
			return fmt.Errorf("result.status = %q, want success", result.Status)
		}
	default:
		return fmt.Errorf("result.status is unsupported")
	}
	switch result.ReasonCode {
	case api.NotificationChannelTestReasonCodeOk,
		api.NotificationChannelTestReasonCodeUnsupportedKind,
		api.NotificationChannelTestReasonCodeCredentialsUnavailable,
		api.NotificationChannelTestReasonCodeProviderUnreachable,
		api.NotificationChannelTestReasonCodeProviderError,
		api.NotificationChannelTestReasonCodeInvalidProfile:
	default:
		return fmt.Errorf("result.reason_code is unsupported")
	}
	if result.Status == api.NotificationChannelTestStatusSuccess &&
		result.ReasonCode != api.NotificationChannelTestReasonCodeOk {
		return fmt.Errorf("successful result must use ok reason_code")
	}
	if result.Status != api.NotificationChannelTestStatusSuccess &&
		result.ReasonCode == api.NotificationChannelTestReasonCodeOk {
		return fmt.Errorf("non-success result must not use ok reason_code")
	}
	if err := validateSingleLine("result.message", result.Message, 512); err != nil {
		return err
	}
	if result.CheckedAt.IsZero() {
		return fmt.Errorf("result.checked_at must be present")
	}
	if err := validateOptionalSingleLine("result.provider_message_id", result.ProviderMessageID, maxProviderMessageIDBytes); err != nil {
		return err
	}
	if err := validateOptionalSingleLine("result.provider_status", result.ProviderStatus, maxProviderStatusBytes); err != nil {
		return err
	}
	if result.ContentKind != nil && !validContentKind(*result.ContentKind) {
		return fmt.Errorf("result.content_kind is unsupported")
	}
	if result.ContentSha256 != nil && !validLowercaseSHA256(*result.ContentSha256) {
		return fmt.Errorf("result.content_sha256 must be a lowercase SHA-256 digest")
	}
	return nil
}

func validateSingleLine(field, value string, maxBytes int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be non-empty", field)
	}
	return validateOptionalSingleLine(field, value, maxBytes)
}

func validateOptionalSingleLine(field, value string, maxBytes int) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("%s must be a single line", field)
	}
	if len([]byte(value)) > maxBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maxBytes)
	}
	return nil
}

func validContentKind(value string) bool {
	switch value {
	case "transport_sample", "ai_diagnosis_sample", "diagnosis_close_sample":
		return true
	default:
		return false
	}
}

func diagnosisNotificationContentKind(value string) bool {
	switch value {
	case "ai_diagnosis_sample", "diagnosis_close_sample":
		return true
	default:
		return false
	}
}

func anyDiagnosisNotificationContentKind(values []string) bool {
	for _, value := range values {
		if diagnosisNotificationContentKind(value) {
			return true
		}
	}
	return false
}

func validEnvVarName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
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
