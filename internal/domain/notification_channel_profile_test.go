package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestNewNotificationChannelProfileNormalizesInput(t *testing.T) {
	got, err := NewNotificationChannelProfile(
		" Primary notifications ",
		NotificationChannelKindWebhook,
		" secret/openclarion/ops-webhook ",
		[]NotificationDeliveryScope{
			NotificationDeliveryScopeDiagnosisClose,
			NotificationDeliveryScopeReport,
			NotificationDeliveryScopeReport,
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
	wantScopes := []NotificationDeliveryScope{
		NotificationDeliveryScopeDiagnosisClose,
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
