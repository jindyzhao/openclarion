package domain

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	maxNotificationChannelNameLen   = 120
	maxNotificationChannelSecretLen = 256
	maxNotificationChannelLabels    = 32
	maxNotificationChannelLabelKey  = 64
	maxNotificationChannelLabelVal  = 128
)

// NotificationChannelKind identifies the concrete operator-notification
// adapter shape. Persisted channel types resolve secret-backed endpoints at
// runtime without storing endpoint credentials in profile rows.
type NotificationChannelKind string

const (
	// NotificationChannelKindWebhook represents an HTTP webhook target whose
	// endpoint is resolved from a deployment-managed secret reference.
	NotificationChannelKindWebhook NotificationChannelKind = "webhook"
	// NotificationChannelKindWeCom represents an Enterprise WeChat group robot
	// webhook endpoint resolved from a deployment-managed secret reference.
	NotificationChannelKindWeCom NotificationChannelKind = "wecom"
	// NotificationChannelKindDingTalk represents a DingTalk group robot
	// webhook endpoint resolved from a deployment-managed secret reference.
	NotificationChannelKindDingTalk NotificationChannelKind = "dingtalk"
	// NotificationChannelKindFeishu represents a Feishu or Lark custom bot
	// webhook endpoint resolved from a deployment-managed secret reference.
	NotificationChannelKindFeishu NotificationChannelKind = "feishu"
	// NotificationChannelKindSlack represents a Slack incoming webhook endpoint
	// resolved from a deployment-managed secret reference.
	NotificationChannelKindSlack NotificationChannelKind = "slack"
	// NotificationChannelKindEmail represents an SMTP email endpoint resolved
	// from a deployment-managed secret reference.
	NotificationChannelKindEmail NotificationChannelKind = "email"
)

// Valid reports whether k is a supported notification channel kind.
func (k NotificationChannelKind) Valid() bool {
	switch k {
	case NotificationChannelKindWebhook, NotificationChannelKindWeCom, NotificationChannelKindDingTalk, NotificationChannelKindFeishu, NotificationChannelKindSlack, NotificationChannelKindEmail:
		return true
	}
	return false
}

// NotificationDeliveryScope records which notification flows may use a
// channel once later workflow binding is implemented.
type NotificationDeliveryScope string

const (
	// NotificationDeliveryScopeReport covers final report notifications.
	NotificationDeliveryScopeReport NotificationDeliveryScope = "report"
	// NotificationDeliveryScopeDiagnosisConsultation covers diagnosis-room
	// assistant/final-ready consultation updates before the room is closed.
	NotificationDeliveryScopeDiagnosisConsultation NotificationDeliveryScope = "diagnosis_consultation"
	// NotificationDeliveryScopeDiagnosisClose covers diagnosis-room close
	// notifications.
	NotificationDeliveryScopeDiagnosisClose NotificationDeliveryScope = "diagnosis_close"
)

// Valid reports whether s is a supported notification delivery scope.
func (s NotificationDeliveryScope) Valid() bool {
	switch s {
	case NotificationDeliveryScopeReport, NotificationDeliveryScopeDiagnosisConsultation, NotificationDeliveryScopeDiagnosisClose:
		return true
	}
	return false
}

// NotificationChannelTestStatus is the sanitized status returned by a
// notification channel delivery test.
type NotificationChannelTestStatus string

// NotificationChannelTestStatus values describe sanitized delivery-test
// outcomes that can be returned to operators.
const (
	NotificationChannelTestStatusSuccess     NotificationChannelTestStatus = "success"
	NotificationChannelTestStatusFailed      NotificationChannelTestStatus = "failed"
	NotificationChannelTestStatusUnsupported NotificationChannelTestStatus = "unsupported"
	NotificationChannelTestStatusBlocked     NotificationChannelTestStatus = "blocked"
)

// Valid reports whether s is a supported notification channel test status.
func (s NotificationChannelTestStatus) Valid() bool {
	switch s {
	case NotificationChannelTestStatusSuccess, NotificationChannelTestStatusFailed, NotificationChannelTestStatusUnsupported, NotificationChannelTestStatusBlocked:
		return true
	}
	return false
}

// NotificationChannelTestReasonCode is the stable sanitized reason for a
// notification channel delivery test result.
type NotificationChannelTestReasonCode string

// NotificationChannelTestReasonCode values describe sanitized, stable failure
// causes for notification channel delivery tests.
const (
	NotificationChannelTestReasonOK                     NotificationChannelTestReasonCode = "ok"
	NotificationChannelTestReasonUnsupportedKind        NotificationChannelTestReasonCode = "unsupported_kind"
	NotificationChannelTestReasonCredentialsUnavailable NotificationChannelTestReasonCode = "credentials_unavailable"
	NotificationChannelTestReasonProviderUnreachable    NotificationChannelTestReasonCode = "provider_unreachable"
	NotificationChannelTestReasonProviderError          NotificationChannelTestReasonCode = "provider_error"
	NotificationChannelTestReasonInvalidProfile         NotificationChannelTestReasonCode = "invalid_profile"
)

// Valid reports whether c is a supported notification channel test reason.
func (c NotificationChannelTestReasonCode) Valid() bool {
	switch c {
	case NotificationChannelTestReasonOK, NotificationChannelTestReasonUnsupportedKind, NotificationChannelTestReasonCredentialsUnavailable, NotificationChannelTestReasonProviderUnreachable, NotificationChannelTestReasonProviderError, NotificationChannelTestReasonInvalidProfile:
		return true
	}
	return false
}

// NotificationChannelTestContentKind identifies sanitized test notification
// content prepared for a provider delivery attempt.
type NotificationChannelTestContentKind string

// NotificationChannelTestContentKind values describe sanitized sample payload
// categories sent by notification channel delivery tests.
const (
	NotificationChannelTestContentTransportSample      NotificationChannelTestContentKind = "transport_sample"
	NotificationChannelTestContentAIDiagnosisSample    NotificationChannelTestContentKind = "ai_diagnosis_sample"
	NotificationChannelTestContentDiagnosisCloseSample NotificationChannelTestContentKind = "diagnosis_close_sample"
)

// Valid reports whether k is a supported notification channel test content
// kind.
func (k NotificationChannelTestContentKind) Valid() bool {
	switch k {
	case NotificationChannelTestContentTransportSample, NotificationChannelTestContentAIDiagnosisSample, NotificationChannelTestContentDiagnosisCloseSample:
		return true
	}
	return false
}

// NotificationChannelProfile is operator-managed notification target metadata.
// Secret material is intentionally excluded; SecretRef points at a
// deployment-managed secret boundary that can resolve endpoint credentials
// later, outside OpenAPI responses and browser state.
type NotificationChannelProfile struct {
	ID               NotificationChannelProfileID
	Name             string
	Kind             NotificationChannelKind
	SecretRef        string
	DeliveryScopes   []NotificationDeliveryScope
	Enabled          bool
	Labels           map[string]string
	LatestTestProofs []NotificationChannelTestProof
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NotificationChannelTestProof is the persisted sanitized proof of one channel
// test. It deliberately excludes endpoint URLs, secret references, raw provider
// responses, and raw provider errors.
type NotificationChannelTestProof struct {
	ID                           NotificationChannelTestProofID
	NotificationChannelProfileID NotificationChannelProfileID
	Kind                         NotificationChannelKind
	Status                       NotificationChannelTestStatus
	ReasonCode                   NotificationChannelTestReasonCode
	Message                      string
	ContentKind                  NotificationChannelTestContentKind
	ContentSHA256                string
	CheckedAt                    time.Time
	ProviderMessageID            string
	ProviderStatus               string
	CreatedAt                    time.Time
}

// MissingAIDiagnosisProofContentKinds returns AI diagnosis delivery sample
// kinds that do not have a current successful sanitized delivery proof. A
// proof is current only when it belongs to this channel, matches the channel
// kind, succeeded, has a content digest, and was checked no earlier than the
// profile's last update.
func (p NotificationChannelProfile) MissingAIDiagnosisProofContentKinds() []NotificationChannelTestContentKind {
	required := []NotificationChannelTestContentKind{
		NotificationChannelTestContentAIDiagnosisSample,
		NotificationChannelTestContentDiagnosisCloseSample,
	}
	missing := make([]NotificationChannelTestContentKind, 0, len(required))
	for _, contentKind := range required {
		if !p.hasCurrentSuccessfulTestProof(contentKind) {
			missing = append(missing, contentKind)
		}
	}
	return missing
}

func (p NotificationChannelProfile) hasCurrentSuccessfulTestProof(contentKind NotificationChannelTestContentKind) bool {
	for _, proof := range p.LatestTestProofs {
		if proof.NotificationChannelProfileID != p.ID ||
			proof.Kind != p.Kind ||
			proof.Status != NotificationChannelTestStatusSuccess ||
			proof.ContentKind != contentKind ||
			!validNotificationContentSHA256(strings.TrimSpace(proof.ContentSHA256)) ||
			proof.CheckedAt.IsZero() {
			continue
		}
		if !p.UpdatedAt.IsZero() && proof.CheckedAt.Before(p.UpdatedAt) {
			continue
		}
		return true
	}
	return false
}

// NewNotificationChannelProfile constructs a validated notification channel
// profile. Delivery scopes are trimmed, deduplicated, and sorted for stable
// persistence. It does not resolve SecretRef or construct an IM provider.
func NewNotificationChannelProfile(
	name string,
	kind NotificationChannelKind,
	secretRef string,
	deliveryScopes []NotificationDeliveryScope,
	enabled bool,
	labels map[string]string,
) (NotificationChannelProfile, error) {
	name = strings.TrimSpace(name)
	secretRef = strings.TrimSpace(secretRef)
	if name == "" {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: name must be non-empty: %w", ErrInvariantViolation)
	}
	if len(name) > maxNotificationChannelNameLen {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: name exceeds %d bytes: %w", maxNotificationChannelNameLen, ErrInvariantViolation)
	}
	if !kind.Valid() {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: kind %q is unsupported: %w", kind, ErrInvariantViolation)
	}
	if err := validateNotificationChannelSecretRef(secretRef); err != nil {
		return NotificationChannelProfile{}, err
	}
	normalizedScopes, err := normalizeNotificationDeliveryScopes(deliveryScopes)
	if err != nil {
		return NotificationChannelProfile{}, err
	}
	if kind != NotificationChannelKindWeCom && notificationScopesRequireWeCom(normalizedScopes) {
		return NotificationChannelProfile{}, fmt.Errorf("notification channel profile: diagnosis delivery scopes require an Enterprise WeChat channel: %w", ErrInvariantViolation)
	}
	normalizedLabels, err := validateNotificationChannelLabels(labels)
	if err != nil {
		return NotificationChannelProfile{}, err
	}
	return NotificationChannelProfile{
		Name:           name,
		Kind:           kind,
		SecretRef:      secretRef,
		DeliveryScopes: normalizedScopes,
		Enabled:        enabled,
		Labels:         normalizedLabels,
	}, nil
}

// NewNotificationChannelTestProof constructs a persisted sanitized proof for a
// notification channel test result.
func NewNotificationChannelTestProof(
	profileID NotificationChannelProfileID,
	kind NotificationChannelKind,
	status NotificationChannelTestStatus,
	reasonCode NotificationChannelTestReasonCode,
	message string,
	contentKind NotificationChannelTestContentKind,
	contentSHA256 string,
	checkedAt time.Time,
	providerMessageID string,
	providerStatus string,
) (NotificationChannelTestProof, error) {
	message = strings.TrimSpace(message)
	contentSHA256 = strings.TrimSpace(contentSHA256)
	providerMessageID = strings.TrimSpace(providerMessageID)
	providerStatus = strings.TrimSpace(providerStatus)
	if profileID <= 0 {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: profile id must be positive: %w", ErrInvariantViolation)
	}
	if !kind.Valid() {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: kind %q is unsupported: %w", kind, ErrInvariantViolation)
	}
	if !status.Valid() {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: status %q is unsupported: %w", status, ErrInvariantViolation)
	}
	if !reasonCode.Valid() {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: reason_code %q is unsupported: %w", reasonCode, ErrInvariantViolation)
	}
	if message == "" {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: message must be non-empty: %w", ErrInvariantViolation)
	}
	if len(message) > 240 {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: message exceeds 240 bytes: %w", ErrInvariantViolation)
	}
	if containsControl(message) {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: message must not contain control characters: %w", ErrInvariantViolation)
	}
	if contentKind != "" && !contentKind.Valid() {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: content_kind %q is unsupported: %w", contentKind, ErrInvariantViolation)
	}
	if contentKind == "" && contentSHA256 != "" {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: content_sha256 requires content_kind: %w", ErrInvariantViolation)
	}
	if contentKind != "" && !validNotificationContentSHA256(contentSHA256) {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: content_sha256 must be a lowercase SHA-256 hex digest: %w", ErrInvariantViolation)
	}
	if checkedAt.IsZero() {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: checked_at must be set: %w", ErrInvariantViolation)
	}
	if len(providerMessageID) > 128 {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: provider_message_id exceeds 128 bytes: %w", ErrInvariantViolation)
	}
	if len(providerStatus) > 64 {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: provider_status exceeds 64 bytes: %w", ErrInvariantViolation)
	}
	if containsControl(providerMessageID) || containsControl(providerStatus) {
		return NotificationChannelTestProof{}, fmt.Errorf("notification channel test proof: provider fields must not contain control characters: %w", ErrInvariantViolation)
	}
	return NotificationChannelTestProof{
		NotificationChannelProfileID: profileID,
		Kind:                         kind,
		Status:                       status,
		ReasonCode:                   reasonCode,
		Message:                      message,
		ContentKind:                  contentKind,
		ContentSHA256:                contentSHA256,
		CheckedAt:                    NormalizeUTCMicro(checkedAt),
		ProviderMessageID:            providerMessageID,
		ProviderStatus:               providerStatus,
	}, nil
}

func validateNotificationChannelSecretRef(secretRef string) error {
	if secretRef == "" {
		return fmt.Errorf("notification channel profile: secret_ref must be non-empty: %w", ErrInvariantViolation)
	}
	if len(secretRef) > maxNotificationChannelSecretLen {
		return fmt.Errorf("notification channel profile: secret_ref exceeds %d bytes: %w", maxNotificationChannelSecretLen, ErrInvariantViolation)
	}
	if containsControlOrSpace(secretRef) {
		return fmt.Errorf("notification channel profile: secret_ref must not contain whitespace or control characters: %w", ErrInvariantViolation)
	}
	if notificationSecretRefLooksLikeEndpointURL(secretRef) {
		return fmt.Errorf("notification channel profile: secret_ref must reference server-side secret storage, not an endpoint URL: %w", ErrInvariantViolation)
	}
	return nil
}

func notificationSecretRefLooksLikeEndpointURL(secretRef string) bool {
	parsed, err := url.Parse(secretRef)
	if err != nil || !parsed.IsAbs() {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func validNotificationContentSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') {
			continue
		}
		return false
	}
	return true
}

func normalizeNotificationDeliveryScopes(scopes []NotificationDeliveryScope) ([]NotificationDeliveryScope, error) {
	if len(scopes) == 0 {
		return nil, fmt.Errorf("notification channel profile: delivery_scopes must be non-empty: %w", ErrInvariantViolation)
	}
	seen := map[NotificationDeliveryScope]struct{}{}
	for _, raw := range scopes {
		scope := NotificationDeliveryScope(strings.TrimSpace(string(raw)))
		if scope == "" {
			return nil, fmt.Errorf("notification channel profile: delivery scope must be non-empty: %w", ErrInvariantViolation)
		}
		if !scope.Valid() {
			return nil, fmt.Errorf("notification channel profile: delivery scope %q is unsupported: %w", scope, ErrInvariantViolation)
		}
		seen[scope] = struct{}{}
	}
	out := make([]NotificationDeliveryScope, 0, len(seen))
	for scope := range seen {
		out = append(out, scope)
	}
	slices.Sort(out)
	return out, nil
}

func notificationScopesRequireWeCom(scopes []NotificationDeliveryScope) bool {
	for _, scope := range scopes {
		switch scope {
		case NotificationDeliveryScopeDiagnosisConsultation, NotificationDeliveryScopeDiagnosisClose:
			return true
		}
	}
	return false
}

func validateNotificationChannelLabels(labels map[string]string) (map[string]string, error) {
	if labels == nil {
		return map[string]string{}, nil
	}
	if len(labels) > maxNotificationChannelLabels {
		return nil, fmt.Errorf("notification channel profile: labels exceed %d entries: %w", maxNotificationChannelLabels, ErrInvariantViolation)
	}
	out := make(map[string]string, len(labels))
	for rawKey, rawVal := range labels {
		key := strings.TrimSpace(rawKey)
		val := strings.TrimSpace(rawVal)
		if key == "" {
			return nil, fmt.Errorf("notification channel profile: label key must be non-empty: %w", ErrInvariantViolation)
		}
		if len(key) > maxNotificationChannelLabelKey {
			return nil, fmt.Errorf("notification channel profile: label key exceeds %d bytes: %w", maxNotificationChannelLabelKey, ErrInvariantViolation)
		}
		if len(val) > maxNotificationChannelLabelVal {
			return nil, fmt.Errorf("notification channel profile: label value exceeds %d bytes: %w", maxNotificationChannelLabelVal, ErrInvariantViolation)
		}
		if containsControl(key) || containsControl(val) {
			return nil, fmt.Errorf("notification channel profile: labels must not contain control characters: %w", ErrInvariantViolation)
		}
		out[key] = val
	}
	return out, nil
}
