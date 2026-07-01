package main

import (
	"strings"
	"testing"
)

func TestCheckAcceptsEmptyConfig(t *testing.T) {
	if err := check(mapGetenv(nil)); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckAcceptsReportOnlyGenericWebhookSecret(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		notificationSecretRefsEnv: `{"secret/openclarion/report-webhook":"https://hooks.example.invalid/openclarion"}`,
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckAcceptsWeComSecretRef(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		notificationSecretRefsEnv: `{"secret/openclarion/ops-wecom":"` + testWeComWebhookURL("placeholder-key") + `"}`,
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckRejectsWeComNamedSecretRefWithGenericEndpointWithoutLeakingValues(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		notificationSecretRefsEnv: `{"secret/openclarion/ops-wecom":"https://hooks.example.invalid/openclarion?key=secret-value"}`,
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	for _, leaked := range []string{
		"hooks.example.invalid",
		"secret-value",
		"secret/openclarion/ops-wecom",
	} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckRejectsExplicitWeComSecretRefMissingFromMap(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		notificationSecretRefsEnv:      `{"secret/openclarion/report-webhook":"https://hooks.example.invalid/openclarion"}`,
		notificationWeComSecretRefsEnv: "secret/openclarion/ops-wecom",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	if !strings.Contains(err.Error(), notificationWeComSecretRefsEnv) ||
		!strings.Contains(err.Error(), notificationSecretRefsEnv) {
		t.Fatalf("error = %q, want env names", err.Error())
	}
}

func TestCheckRejectsMalformedSecretRefsWithoutLeakingValues(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		notificationSecretRefsEnv: `{"secret/openclarion/ops-wecom":"bad webhook value"}`,
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	for _, leaked := range []string{
		"bad webhook value",
		"secret/openclarion/ops-wecom",
	} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckRejectsInvalidExplicitWeComSecretRefList(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		notificationSecretRefsEnv:      `{"secret/openclarion/report-webhook":"https://hooks.example.invalid/openclarion"}`,
		notificationWeComSecretRefsEnv: "secret/openclarion/ops wecom",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	if !strings.Contains(err.Error(), notificationWeComSecretRefsEnv) {
		t.Fatalf("error = %q, want %s", err.Error(), notificationWeComSecretRefsEnv)
	}
}

func mapGetenv(values map[string]string) getenvFunc {
	return func(key string) string {
		return values[key]
	}
}

func testWeComWebhookURL(key string) string {
	return "https://" + weComWebhookHost + weComWebhookPath + "?key=" + key
}
