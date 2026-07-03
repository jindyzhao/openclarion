// Package alertmanagerwebhook ingests Alertmanager webhook receiver payloads
// through the existing AlertEvent persistence boundary.
package alertmanagerwebhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/alertdiagnosis"
	"github.com/openclarion/openclarion/internal/usecases/alertingest"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	sourceName             = "alertmanager"
	supportedVersion       = "4"
	statusFiring           = "firing"
	statusResolved         = "resolved"
	maxWebhookAlertEntries = 10000
)

var (
	// ErrUnauthorized means the inbound webhook did not present the expected
	// bearer authorization for a bearer-backed alert source profile.
	ErrUnauthorized = errors.New("alertmanager webhook authorization failed")
	// ErrSecretResolverUnavailable means the profile requires bearer auth but
	// the deployment did not configure a server-side secret resolver.
	ErrSecretResolverUnavailable = errors.New("alertmanager webhook secret resolver unavailable")
	// ErrSecretNotFound means the configured resolver could not find the
	// profile's secret reference.
	ErrSecretNotFound = errors.New("alertmanager webhook secret not found")
	// ErrSecretResolveFailed means the configured resolver failed for a reason
	// other than a missing secret.
	ErrSecretResolveFailed = errors.New("alertmanager webhook secret resolve failed")
)

// Request identifies one inbound Alertmanager webhook delivery.
type Request struct {
	ProfileID     domain.AlertSourceProfileID
	Authorization string
	Body          json.RawMessage
}

// Result is the sanitized response surface for one webhook ingest.
type Result struct {
	ProfileID         domain.AlertSourceProfileID
	Received          int
	SkippedResolved   int
	SkippedSuppressed int
	TruncatedAlerts   int
	Ingested          alertingest.Stats
	AutoDiagnosis     *alertdiagnosis.Result
}

// Service validates the bound alert source profile, checks inbound
// authorization when required, parses the webhook payload, and persists firing
// alerts through alertingest.
type Service struct {
	uowFactory           ports.UnitOfWorkFactory
	secretResolver       ports.SecretResolver
	autoDiagnosisTrigger AutoDiagnosisTrigger
}

// Option customizes Service construction.
type Option func(*Service)

// AutoDiagnosisTrigger starts automatic diagnosis work after firing alerts are
// durably ingested.
type AutoDiagnosisTrigger interface {
	Trigger(context.Context, alertdiagnosis.Request) (alertdiagnosis.Result, error)
}

// WithSecretResolver enables bearer-backed webhook authorization.
func WithSecretResolver(resolver ports.SecretResolver) Option {
	return func(s *Service) {
		s.secretResolver = resolver
	}
}

// WithAutoDiagnosisTrigger enables automatic diagnosis-room handoff for
// profiles with enabled auto_room workflow policies.
func WithAutoDiagnosisTrigger(trigger AutoDiagnosisTrigger) Option {
	return func(s *Service) {
		s.autoDiagnosisTrigger = trigger
	}
}

// NewService constructs an Alertmanager webhook ingest service.
func NewService(uowFactory ports.UnitOfWorkFactory, opts ...Option) (*Service, error) {
	if uowFactory == nil {
		return nil, fmt.Errorf("alertmanager webhook: unit of work factory must be non-nil: %w", domain.ErrInvariantViolation)
	}
	service := &Service{uowFactory: uowFactory}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}
	return service, nil
}

// Ingest validates and persists one Alertmanager webhook payload.
func (s *Service) Ingest(ctx context.Context, req Request) (Result, error) {
	if s == nil {
		return Result{}, fmt.Errorf("alertmanager webhook: service must be non-nil: %w", domain.ErrInvariantViolation)
	}
	if req.ProfileID <= 0 {
		return Result{}, fmt.Errorf("alertmanager webhook: profile_id must be positive: %w", domain.ErrInvariantViolation)
	}
	if len(req.Body) == 0 {
		return Result{}, fmt.Errorf("alertmanager webhook: request body must be non-empty: %w", domain.ErrInvariantViolation)
	}

	profile, err := s.loadProfile(ctx, req.ProfileID)
	if err != nil {
		return Result{}, err
	}
	if err := validateProfile(profile); err != nil {
		return Result{}, err
	}
	if err := s.authorize(ctx, profile, req.Authorization); err != nil {
		return Result{}, err
	}

	decoded, err := decodePayload(req.Body, req.ProfileID)
	if err != nil {
		return Result{}, err
	}
	stats, err := alertingest.IngestAlerts(ctx, decoded.alerts, s.uowFactory)
	result := Result{
		ProfileID:         req.ProfileID,
		Received:          decoded.received,
		SkippedResolved:   decoded.skippedResolved,
		SkippedSuppressed: decoded.skippedSuppressed,
		TruncatedAlerts:   decoded.truncatedAlerts,
		Ingested:          stats,
	}
	if err != nil {
		return result, err
	}
	if s.autoDiagnosisTrigger != nil && !decoded.windowStart.IsZero() && !decoded.windowEnd.IsZero() {
		alertEventIDs, rerr := s.resolveWebhookAlertEventIDs(ctx, decoded.alerts)
		if rerr != nil {
			return result, rerr
		}
		triggered, terr := s.autoDiagnosisTrigger.Trigger(ctx, alertdiagnosis.Request{
			AlertSourceProfileID: req.ProfileID,
			WindowStart:          decoded.windowStart,
			WindowEnd:            decoded.windowEnd,
			AlertEventIDs:        alertEventIDs,
			Limit:                maxWebhookAlertEntries,
		})
		result.AutoDiagnosis = &triggered
		if terr != nil {
			return result, fmt.Errorf("alertmanager webhook: auto diagnosis trigger: %w", terr)
		}
	}
	return result, nil
}

func (s *Service) resolveWebhookAlertEventIDs(ctx context.Context, alerts []ports.ActiveAlert) ([]domain.AlertEventID, error) {
	keys := make([]ports.AlertEventNaturalKey, 0, len(alerts))
	seen := make(map[ports.AlertEventNaturalKey]struct{}, len(alerts))
	for i, alert := range alerts {
		_, canonical, err := alertingest.EventFingerprints(alert.Labels)
		if err != nil {
			return nil, fmt.Errorf("alertmanager webhook: build alert event key for alert[%d]: %w", i, err)
		}
		key := ports.AlertEventNaturalKey{
			Source:               alert.Source,
			AlertSourceProfileID: alert.AlertSourceProfileID,
			CanonicalFingerprint: canonical,
			StartsAt:             domain.NormalizeUTCMicro(alert.StartsAt),
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, nil
	}

	ids := make([]domain.AlertEventID, 0, len(keys))
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		rows, err := uow.Alerts().ListEventsByNaturalKeys(ctx, keys, len(keys)+1)
		if err != nil {
			return err
		}
		if len(rows) > len(keys) {
			return fmt.Errorf("alertmanager webhook: natural-key lookup returned more rows than requested: %w", domain.ErrInvariantViolation)
		}
		byKey := make(map[ports.AlertEventNaturalKey]domain.AlertEventID, len(rows))
		for _, row := range rows {
			key := ports.AlertEventNaturalKey{
				Source:               row.Source,
				AlertSourceProfileID: row.AlertSourceProfileID,
				CanonicalFingerprint: row.CanonicalFingerprint,
				StartsAt:             domain.NormalizeUTCMicro(row.StartsAt),
			}
			byKey[key] = row.ID
		}
		for _, key := range keys {
			id := byKey[key]
			if id <= 0 {
				return fmt.Errorf("alertmanager webhook: persisted alert event was not found after ingest: %w", domain.ErrInvariantViolation)
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("alertmanager webhook: resolve ingested alert events: %w", err)
	}
	return ids, nil
}

func (s *Service) loadProfile(ctx context.Context, id domain.AlertSourceProfileID) (domain.AlertSourceProfile, error) {
	var profile domain.AlertSourceProfile
	err := s.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		var err error
		profile, err = uow.Config().FindAlertSourceProfileByID(ctx, id)
		return err
	})
	return profile, err
}

func validateProfile(profile domain.AlertSourceProfile) error {
	if profile.Kind != domain.AlertSourceKindAlertmanager {
		return fmt.Errorf("alertmanager webhook: alert source profile kind must be alertmanager: %w", domain.ErrInvariantViolation)
	}
	if !profile.Enabled {
		return fmt.Errorf("alertmanager webhook: alert source profile must be enabled: %w", domain.ErrInvariantViolation)
	}
	return nil
}

func (s *Service) authorize(ctx context.Context, profile domain.AlertSourceProfile, header string) error {
	switch profile.AuthMode {
	case domain.AlertSourceAuthModeNone:
		return nil
	case domain.AlertSourceAuthModeBearer:
		if s.secretResolver == nil {
			return ErrSecretResolverUnavailable
		}
		secret, err := s.secretResolver.ResolveSecret(ctx, profile.SecretRef)
		if errors.Is(err, ports.ErrSecretNotFound) {
			return ErrSecretNotFound
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrSecretResolveFailed, err)
		}
		if secret.Value == "" {
			return ErrUnauthorized
		}
		token, ok := bearerToken(header)
		if !ok {
			return ErrUnauthorized
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret.Value)) != 1 {
			return ErrUnauthorized
		}
		return nil
	default:
		return fmt.Errorf("alertmanager webhook: unsupported auth mode %q: %w", profile.AuthMode, domain.ErrInvariantViolation)
	}
}

type decodedPayload struct {
	received          int
	skippedResolved   int
	skippedSuppressed int
	truncatedAlerts   int
	windowStart       time.Time
	windowEnd         time.Time
	alerts            []ports.ActiveAlert
}

type webhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []json.RawMessage `json:"alerts"`
}

type webhookAlert struct {
	Status       string             `json:"status"`
	Labels       *map[string]string `json:"labels"`
	Annotations  *map[string]string `json:"annotations"`
	StartsAt     time.Time          `json:"startsAt"`
	EndsAt       time.Time          `json:"endsAt"`
	GeneratorURL string             `json:"generatorURL"`
	Fingerprint  string             `json:"fingerprint"`
	SilencedBy   []string           `json:"silencedBy,omitempty"`
	InhibitedBy  []string           `json:"inhibitedBy,omitempty"`
	MutedBy      []string           `json:"mutedBy,omitempty"`
}

func decodePayload(raw json.RawMessage, profileID domain.AlertSourceProfileID) (decodedPayload, error) {
	var payload webhookPayload
	if err := strictjson.Unmarshal(raw, &payload); err != nil {
		return decodedPayload{}, fmt.Errorf("alertmanager webhook: decode payload: %w", err)
	}
	if payload.Version != supportedVersion {
		return decodedPayload{}, fmt.Errorf("alertmanager webhook: version must be %q: %w", supportedVersion, domain.ErrInvariantViolation)
	}
	if !validStatus(payload.Status) {
		return decodedPayload{}, fmt.Errorf("alertmanager webhook: status %q is unsupported: %w", payload.Status, domain.ErrInvariantViolation)
	}
	if payload.TruncatedAlerts < 0 {
		return decodedPayload{}, fmt.Errorf("alertmanager webhook: truncatedAlerts must be >= 0: %w", domain.ErrInvariantViolation)
	}
	if payload.Alerts == nil {
		return decodedPayload{}, fmt.Errorf("alertmanager webhook: alerts must be present: %w", domain.ErrInvariantViolation)
	}
	if len(payload.Alerts) > maxWebhookAlertEntries {
		return decodedPayload{}, fmt.Errorf("alertmanager webhook: alerts exceeds %d entries: %w", maxWebhookAlertEntries, domain.ErrInvariantViolation)
	}

	decoded := decodedPayload{
		received:        len(payload.Alerts),
		truncatedAlerts: payload.TruncatedAlerts,
		alerts:          make([]ports.ActiveAlert, 0, len(payload.Alerts)),
	}
	for i, encoded := range payload.Alerts {
		alert, err := decodeAlert(encoded)
		if err != nil {
			return decodedPayload{}, fmt.Errorf("alertmanager webhook: alerts[%d]: %w", i, err)
		}
		switch alert.Status {
		case statusFiring:
			if alertSuppressed(alert) {
				decoded.skippedSuppressed++
				continue
			}
			if alert.StartsAt.IsZero() {
				return decodedPayload{}, fmt.Errorf("startsAt must be set for firing alert: %w", domain.ErrInvariantViolation)
			}
			startsAt := domain.NormalizeUTCMicro(alert.StartsAt)
			if decoded.windowStart.IsZero() || startsAt.Before(decoded.windowStart) {
				decoded.windowStart = startsAt
			}
			if decoded.windowEnd.IsZero() || startsAt.After(decoded.windowEnd) {
				decoded.windowEnd = startsAt
			}
			decoded.alerts = append(decoded.alerts, ports.ActiveAlert{
				Source:               sourceName,
				AlertSourceProfileID: profileID,
				Labels:               *alert.Labels,
				Annotations:          *alert.Annotations,
				StartsAt:             startsAt,
				RawPayload:           append(json.RawMessage(nil), encoded...),
			})
		case statusResolved:
			decoded.skippedResolved++
		default:
			return decodedPayload{}, fmt.Errorf("status %q is unsupported: %w", alert.Status, domain.ErrInvariantViolation)
		}
	}
	if !decoded.windowEnd.IsZero() {
		decoded.windowEnd = decoded.windowEnd.Add(time.Microsecond)
	}
	return decoded, nil
}

func decodeAlert(raw json.RawMessage) (webhookAlert, error) {
	var alert webhookAlert
	if err := strictjson.Unmarshal(raw, &alert); err != nil {
		return webhookAlert{}, fmt.Errorf("decode alert: %w", err)
	}
	if !validStatus(alert.Status) {
		return webhookAlert{}, fmt.Errorf("status %q is unsupported: %w", alert.Status, domain.ErrInvariantViolation)
	}
	if alert.Labels == nil {
		return webhookAlert{}, fmt.Errorf("labels must be present: %w", domain.ErrInvariantViolation)
	}
	if alert.Annotations == nil {
		return webhookAlert{}, fmt.Errorf("annotations must be present: %w", domain.ErrInvariantViolation)
	}
	return alert, nil
}

func alertSuppressed(alert webhookAlert) bool {
	return len(alert.SilencedBy) > 0 || len(alert.InhibitedBy) > 0 || len(alert.MutedBy) > 0
}

func bearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}

func validStatus(status string) bool {
	switch status {
	case statusFiring, statusResolved:
		return true
	default:
		return false
	}
}
