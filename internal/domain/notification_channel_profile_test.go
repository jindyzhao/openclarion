package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewNotificationChannelProfileNormalizesInput(t *testing.T) {
	got, err := NewNotificationChannelProfile(
		" Primary notifications ",
		NotificationChannelKindWeCom,
		" secret/openclarion/ops-webhook ",
		[]NotificationDeliveryScope{
			NotificationDeliveryScopeDiagnosisClose,
			NotificationDeliveryScopeDiagnosisConsultation,
			NotificationDeliveryScopeReport,
			NotificationDeliveryScopeReport,
			NotificationDeliveryScopeDiagnosisConsultation,
		},
		true,
		map[string]string{" owner ": " sre ", "env": " prod "},
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelProfile: %v", err)
	}
	if got.Name != "Primary notifications" {
		t.Fatalf("Name = %q", got.Name)
	}
	if got.SecretRef != "secret/openclarion/ops-webhook" {
		t.Fatalf("SecretRef = %q", got.SecretRef)
	}
	if got.Kind != NotificationChannelKindWeCom {
		t.Fatalf("Kind = %q, want %q", got.Kind, NotificationChannelKindWeCom)
	}
	wantScopes := []NotificationDeliveryScope{
		NotificationDeliveryScopeDiagnosisClose,
		NotificationDeliveryScopeDiagnosisConsultation,
		NotificationDeliveryScopeReport,
	}
	if !reflect.DeepEqual(got.DeliveryScopes, wantScopes) {
		t.Fatalf("DeliveryScopes = %#v, want %#v", got.DeliveryScopes, wantScopes)
	}
	if got.Labels["owner"] != "sre" || got.Labels["env"] != "prod" {
		t.Fatalf("Labels = %#v", got.Labels)
	}
	if !got.Enabled {
		t.Fatal("Enabled = false, want true")
	}
}

func TestNewNotificationChannelProfileRejectsInvalid(t *testing.T) {
	valid := func() (NotificationChannelProfile, error) {
		return NewNotificationChannelProfile(
			"Primary notifications",
			NotificationChannelKindWebhook,
			"secret/openclarion/ops-webhook",
			[]NotificationDeliveryScope{NotificationDeliveryScopeReport},
			false,
			map[string]string{"owner": "sre"},
		)
	}
	tests := []struct {
		name string
		edit func() (NotificationChannelProfile, error)
	}{
		{
			name: "blank name",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile(" ", NotificationChannelKindWebhook, "secret/ref", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, nil)
			},
		},
		{
			name: "unsupported kind",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", "email", "secret/ref", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, nil)
			},
		},
		{
			name: "missing secret",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, " ", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, nil)
			},
		},
		{
			name: "secret whitespace",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref value", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, nil)
			},
		},
		{
			name: "secret endpoint url",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWeCom, "https://qyapi.example.test/cgi-bin/webhook/send?key=secret", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, nil)
			},
		},
		{
			name: "empty scopes",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref", nil, false, nil)
			},
		},
		{
			name: "unsupported scope",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref", []NotificationDeliveryScope{"weekly"}, false, nil)
			},
		},
		{
			name: "diagnosis consultation scope on generic webhook",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref", []NotificationDeliveryScope{NotificationDeliveryScopeDiagnosisConsultation}, false, nil)
			},
		},
		{
			name: "diagnosis close scope on generic webhook",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref", []NotificationDeliveryScope{NotificationDeliveryScopeReport, NotificationDeliveryScopeDiagnosisClose}, false, nil)
			},
		},
		{
			name: "blank label key",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, map[string]string{" ": "value"})
			},
		},
		{
			name: "label control",
			edit: func() (NotificationChannelProfile, error) {
				return NewNotificationChannelProfile("name", NotificationChannelKindWebhook, "secret/ref", []NotificationDeliveryScope{NotificationDeliveryScopeReport}, false, map[string]string{"owner": "sre\nops"})
			},
		},
		{
			name: "valid baseline",
			edit: valid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.edit()
			if tt.name == "valid baseline" {
				if err != nil {
					t.Fatalf("valid baseline err = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestNotificationChannelKindValidAcceptsReportWebhookKinds(t *testing.T) {
	for _, kind := range []NotificationChannelKind{
		NotificationChannelKindWebhook,
		NotificationChannelKindWeCom,
		NotificationChannelKindDingTalk,
		NotificationChannelKindFeishu,
	} {
		t.Run(string(kind), func(t *testing.T) {
			got, err := NewNotificationChannelProfile(
				"Report notifications",
				kind,
				"secret/openclarion/report-webhook",
				[]NotificationDeliveryScope{NotificationDeliveryScopeReport},
				false,
				nil,
			)
			if err != nil {
				t.Fatalf("NewNotificationChannelProfile: %v", err)
			}
			if got.Kind != kind {
				t.Fatalf("Kind = %q, want %q", got.Kind, kind)
			}
		})
	}
}

func TestNewNotificationChannelTestProofNormalizesInput(t *testing.T) {
	checkedAt := time.Date(2026, 6, 22, 10, 0, 0, 123456789, time.UTC)
	got, err := NewNotificationChannelTestProof(
		7,
		NotificationChannelKindWeCom,
		NotificationChannelTestStatusSuccess,
		NotificationChannelTestReasonOK,
		" Notification channel test delivery succeeded. ",
		NotificationChannelTestContentAIDiagnosisSample,
		"a"+strings.Repeat("0", 63),
		checkedAt,
		" provider-message-1 ",
		" accepted ",
	)
	if err != nil {
		t.Fatalf("NewNotificationChannelTestProof: %v", err)
	}
	if got.NotificationChannelProfileID != 7 ||
		got.Kind != NotificationChannelKindWeCom ||
		got.Status != NotificationChannelTestStatusSuccess ||
		got.ReasonCode != NotificationChannelTestReasonOK ||
		got.ContentKind != NotificationChannelTestContentAIDiagnosisSample ||
		got.Message != "Notification channel test delivery succeeded." ||
		got.ProviderMessageID != "provider-message-1" ||
		got.ProviderStatus != "accepted" {
		t.Fatalf("proof = %+v", got)
	}
	if got.CheckedAt.Nanosecond() != 123456000 {
		t.Fatalf("checked_at = %s, want microsecond normalized", got.CheckedAt)
	}
}

func TestNewNotificationChannelTestProofRejectsInvalid(t *testing.T) {
	valid := func() (NotificationChannelTestProof, error) {
		return NewNotificationChannelTestProof(
			7,
			NotificationChannelKindWeCom,
			NotificationChannelTestStatusBlocked,
			NotificationChannelTestReasonCredentialsUnavailable,
			"Secret reference could not be resolved by the server-side resolver.",
			"",
			"",
			time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC),
			"",
			"",
		)
	}
	tests := []struct {
		name string
		edit func() (NotificationChannelTestProof, error)
	}{
		{
			name: "zero profile id",
			edit: func() (NotificationChannelTestProof, error) {
				return NewNotificationChannelTestProof(0, NotificationChannelKindWeCom, NotificationChannelTestStatusSuccess, NotificationChannelTestReasonOK, "ok", "", "", time.Now(), "", "")
			},
		},
		{
			name: "unsupported status",
			edit: func() (NotificationChannelTestProof, error) {
				return NewNotificationChannelTestProof(7, NotificationChannelKindWeCom, "maybe", NotificationChannelTestReasonOK, "ok", "", "", time.Now(), "", "")
			},
		},
		{
			name: "content sha without content kind",
			edit: func() (NotificationChannelTestProof, error) {
				return NewNotificationChannelTestProof(7, NotificationChannelKindWeCom, NotificationChannelTestStatusSuccess, NotificationChannelTestReasonOK, "ok", "", strings.Repeat("a", 64), time.Now(), "", "")
			},
		},
		{
			name: "bad content sha",
			edit: func() (NotificationChannelTestProof, error) {
				return NewNotificationChannelTestProof(7, NotificationChannelKindWeCom, NotificationChannelTestStatusSuccess, NotificationChannelTestReasonOK, "ok", NotificationChannelTestContentAIDiagnosisSample, strings.Repeat("A", 64), time.Now(), "", "")
			},
		},
		{
			name: "zero checked at",
			edit: func() (NotificationChannelTestProof, error) {
				return NewNotificationChannelTestProof(7, NotificationChannelKindWeCom, NotificationChannelTestStatusSuccess, NotificationChannelTestReasonOK, "ok", "", "", time.Time{}, "", "")
			},
		},
		{
			name: "valid blocked result",
			edit: valid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.edit()
			if tt.name == "valid blocked result" {
				if err != nil {
					t.Fatalf("valid blocked result err = %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvariantViolation) {
				t.Fatalf("err = %v, want ErrInvariantViolation", err)
			}
		})
	}
}

func TestNotificationChannelProfileMissingAIDiagnosisProofContentKinds(t *testing.T) {
	updatedAt := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	profile := NotificationChannelProfile{
		ID:        7,
		Kind:      NotificationChannelKindWeCom,
		UpdatedAt: updatedAt,
	}

	if got := profile.MissingAIDiagnosisProofContentKinds(); !reflect.DeepEqual(got, []NotificationChannelTestContentKind{
		NotificationChannelTestContentAIDiagnosisSample,
		NotificationChannelTestContentDiagnosisCloseSample,
	}) {
		t.Fatalf("missing proof = %#v, want both samples", got)
	}

	profile.LatestTestProofs = []NotificationChannelTestProof{
		notificationChannelTestProofFixture(profile, NotificationChannelTestContentAIDiagnosisSample, updatedAt),
		notificationChannelTestProofFixture(profile, NotificationChannelTestContentDiagnosisCloseSample, updatedAt.Add(time.Second)),
	}
	if got := profile.MissingAIDiagnosisProofContentKinds(); len(got) != 0 {
		t.Fatalf("missing proof = %#v, want none", got)
	}
}

func TestNotificationChannelProfileMissingAIDiagnosisProofRejectsStaleOrMismatchedProof(t *testing.T) {
	updatedAt := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	profile := NotificationChannelProfile{
		ID:        7,
		Kind:      NotificationChannelKindWeCom,
		UpdatedAt: updatedAt,
		LatestTestProofs: []NotificationChannelTestProof{
			notificationChannelTestProofFixture(NotificationChannelProfile{ID: 99, Kind: NotificationChannelKindWeCom}, NotificationChannelTestContentAIDiagnosisSample, updatedAt.Add(time.Minute)),
			notificationChannelTestProofFixture(NotificationChannelProfile{ID: 7, Kind: NotificationChannelKindWebhook}, NotificationChannelTestContentAIDiagnosisSample, updatedAt.Add(time.Minute)),
			func() NotificationChannelTestProof {
				proof := notificationChannelTestProofFixture(NotificationChannelProfile{ID: 7, Kind: NotificationChannelKindWeCom}, NotificationChannelTestContentAIDiagnosisSample, updatedAt.Add(time.Minute))
				proof.Status = NotificationChannelTestStatusFailed
				return proof
			}(),
			func() NotificationChannelTestProof {
				proof := notificationChannelTestProofFixture(NotificationChannelProfile{ID: 7, Kind: NotificationChannelKindWeCom}, NotificationChannelTestContentAIDiagnosisSample, updatedAt.Add(time.Minute))
				proof.ContentSHA256 = ""
				return proof
			}(),
			notificationChannelTestProofFixture(NotificationChannelProfile{ID: 7, Kind: NotificationChannelKindWeCom}, NotificationChannelTestContentAIDiagnosisSample, updatedAt.Add(-time.Nanosecond)),
			notificationChannelTestProofFixture(NotificationChannelProfile{ID: 7, Kind: NotificationChannelKindWeCom}, NotificationChannelTestContentDiagnosisCloseSample, updatedAt.Add(time.Minute)),
		},
	}

	if got := profile.MissingAIDiagnosisProofContentKinds(); !reflect.DeepEqual(got, []NotificationChannelTestContentKind{
		NotificationChannelTestContentAIDiagnosisSample,
	}) {
		t.Fatalf("missing proof = %#v, want stale/mismatched AI sample rejected", got)
	}
}

func notificationChannelTestProofFixture(
	profile NotificationChannelProfile,
	contentKind NotificationChannelTestContentKind,
	checkedAt time.Time,
) NotificationChannelTestProof {
	return NotificationChannelTestProof{
		NotificationChannelProfileID: profile.ID,
		Kind:                         profile.Kind,
		Status:                       NotificationChannelTestStatusSuccess,
		ReasonCode:                   NotificationChannelTestReasonOK,
		Message:                      "Notification channel test delivery succeeded.",
		ContentKind:                  contentKind,
		ContentSHA256:                strings.Repeat("a", 64),
		CheckedAt:                    checkedAt,
		ProviderMessageID:            "provider-message-1",
		ProviderStatus:               "delivered",
	}
}
