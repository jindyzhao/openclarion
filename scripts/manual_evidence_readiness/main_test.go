package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunReportsMissingEvidencePrerequisitesWithoutValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run(nil, []string{
		"DATABASE_URL=postgres://secret@example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(stdout.String(), "secret@example.test") || strings.Contains(stdout.String(), "127.0.0.1:7233") {
		t.Fatalf("output leaked environment value: %s", stdout.String())
	}

	out := decodeOutput(t, stdout.Bytes())
	if out.Status != "blocked" {
		t.Fatalf("Status = %q, want blocked", out.Status)
	}
	report := targetByName(t, out, "report-live-smoke")
	if !contains(report.MissingEnv, "OPENCLARION_PROMETHEUS_URL") {
		t.Fatalf("report missing env = %v, want OPENCLARION_PROMETHEUS_URL", report.MissingEnv)
	}
	if len(report.UnsatisfiedAlternatives) != 1 {
		t.Fatalf("report alternatives = %v, want worker provider alternative", report.UnsatisfiedAlternatives)
	}
}

func TestRunAlertOperationsLiveInputsReadyWithoutValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "alert-operations-live-inputs"}, []string{
		"OPENCLARION_PROMETHEUS_URL=https://thanos-query.example.test",
		"OPENCLARION_ALERTMANAGER_URL=https://alertmanager.example.test/api/v2/alerts",
		"OPENCLARION_THANOS_RULE_URL=https://thanos-rule.example.test/alerts",
		"OPENCLARION_LLM_BASE_URL=https://llm-gateway.example.test/v1",
		"OPENCLARION_LLM_API_KEY=placeholder-api-key",
		"OPENCLARION_LLM_MODEL=example-llm-model",
		"OPENCLARION_IM_WEBHOOK_URL=https://wecom-webhook.example.test/cgi-bin/webhook/send?key=placeholder-webhook-key",
		"OPENCLARION_IM_WEBHOOK_FORMAT=wecom",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "alert-operations-live-inputs")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"thanos-query.example.test",
		"alertmanager.example.test",
		"thanos-rule.example.test",
		"llm-gateway.example.test",
		"placeholder-api-key",
		"example-llm-model",
		"wecom-webhook.example.test",
		"placeholder-webhook-key",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked live input value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunAlertOperationsLiveInputsAcceptsWebhookFormats(t *testing.T) {
	for _, format := range []string{"generic", "wecom", "dingtalk", "feishu", "slack"} {
		t.Run(format, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run([]string{"--target", "alert-operations-live-inputs"}, []string{
				"OPENCLARION_PROMETHEUS_URL=https://thanos-query.example.test",
				"OPENCLARION_LLM_MODEL=example-llm-model",
				"OPENCLARION_IM_WEBHOOK_URL=https://webhook.example.test/notify",
				"OPENCLARION_IM_WEBHOOK_FORMAT=" + format,
			}, &stdout)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			out := decodeOutput(t, stdout.Bytes())
			target := targetByName(t, out, "alert-operations-live-inputs")
			if target.Status != "ready" {
				t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
			}
			if len(target.MissingEnv) != 0 || len(target.InvalidEnv) != 0 {
				t.Fatalf("target has unexpected blockers: %#v", target)
			}
		})
	}
}

func TestRunAlertOperationsLiveInputsRejectsRobotBearerTokens(t *testing.T) {
	for _, format := range []string{"wecom", "dingtalk", "feishu", "slack"} {
		t.Run(format, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run([]string{"--target", "alert-operations-live-inputs"}, []string{
				"OPENCLARION_PROMETHEUS_URL=https://thanos-query.example.test",
				"OPENCLARION_LLM_MODEL=example-llm-model",
				"OPENCLARION_IM_WEBHOOK_URL=https://webhook.example.test/notify",
				"OPENCLARION_IM_WEBHOOK_FORMAT=" + format,
				"OPENCLARION_IM_WEBHOOK_BEARER_TOKEN=secret-bearer-token",
			}, &stdout)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			out := decodeOutput(t, stdout.Bytes())
			target := targetByName(t, out, "alert-operations-live-inputs")
			if target.Status != "blocked" {
				t.Fatalf("target status = %q, want blocked: %#v", target.Status, target)
			}
			if !invalidEnvByName(target.InvalidEnv, "OPENCLARION_IM_WEBHOOK_BEARER_TOKEN") {
				t.Fatalf("invalid env = %#v, want OPENCLARION_IM_WEBHOOK_BEARER_TOKEN", target.InvalidEnv)
			}
			for _, secret := range []string{"webhook.example.test", "secret-bearer-token"} {
				if strings.Contains(stdout.String(), secret) {
					t.Fatalf("output leaked robot bearer value %q: %s", secret, stdout.String())
				}
			}
		})
	}
}

func TestRunReportsReadyNotificationChannelLiveSmoke(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"NOTIFICATION_CHANNEL_EXPECTED_KIND=wecom",
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND=ai_diagnosis_sample",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT=10s",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"secret-token",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked notification channel value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyNotificationChannelLiveSmokeWithLiveAlias(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND=WeCom",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND=Diagnosis_Close_Sample",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	if strings.Contains(stdout.String(), "api.example.test") {
		t.Fatalf("output leaked notification channel alias value: %s", stdout.String())
	}
}

func TestRunReportsReadyNotificationChannelLiveSmokeWithExpandedKinds(t *testing.T) {
	for _, kind := range []string{"dingtalk", "feishu", "slack", "email"} {
		t.Run(kind, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
				"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
				"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=2",
				"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND=" + kind,
				"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND=transport_sample",
			}, &stdout)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			out := decodeOutput(t, stdout.Bytes())
			target := targetByName(t, out, "notification-channel-live-smoke")
			if target.Status != "ready" {
				t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
			}
			if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
				t.Fatalf("target has unexpected blockers: %#v", target)
			}
			if strings.Contains(stdout.String(), "api.example.test") {
				t.Fatalf("output leaked notification channel value: %s", stdout.String())
			}
		})
	}
}

func TestRunReportsReadyNotificationChannelLiveSmokeWithContentKindSuite(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS=AI_Diagnosis_Sample, Diagnosis_Close_Sample",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	if strings.Contains(stdout.String(), "api.example.test") {
		t.Fatalf("output leaked notification channel suite value: %s", stdout.String())
	}
}

func TestRunReportsReadyNotificationChannelLiveSmokeWithAIProofRequirement(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND=WeCom",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=true",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=true",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	if strings.Contains(stdout.String(), "api.example.test") {
		t.Fatalf("output leaked notification channel AI proof value: %s", stdout.String())
	}
}

func TestRunReportsReadyDiagnosisAuthLiveSmokeWithLDAPCredentials(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-auth-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator-1",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder-ldap-password",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE=ldap",
		"DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT=10s",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-auth-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if target.Milestone != "M5" || target.Sequence != 19 || target.Command != "make diagnosis-auth-live-smoke" {
		t.Fatalf("target metadata = milestone %q sequence %d command %q, want M5/19/make target",
			target.Milestone, target.Sequence, target.Command)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"operator-1",
		"placeholder-ldap-password",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked diagnosis auth LDAP value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisAuthLiveSmokeWithLocalLDAPBackendConfig(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-auth-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator-1",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder-ldap-password",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE=ldap",
		"OPENCLARION_DIAGNOSIS_AUTH_MODE=ldap",
		"OPENCLARION_DIAGNOSIS_LDAP_URL=ldaps://ldap.example.test:636",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN=dc=example,dc=test",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_DN=cn=openclarion,dc=example,dc=test",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD=placeholder-service-password",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER=(&(objectClass=person)(uid={username}))",
		"OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE=mail",
		"OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE=memberOf",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES=owner,admin",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-auth-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"ldap.example.test",
		"dc=example,dc=test",
		"operator-1",
		"placeholder-ldap-password",
		"placeholder-service-password",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked diagnosis LDAP backend value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunBlocksDiagnosisAuthLiveSmokeWithIncompleteLocalLDAPBackendConfig(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-auth-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator-1",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder-ldap-password",
		"OPENCLARION_DIAGNOSIS_AUTH_MODE=ldap",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-auth-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked: %#v", target.Status, target)
	}
	for _, name := range []string{
		"OPENCLARION_DIAGNOSIS_LDAP_URL",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
	} {
		if !contains(target.MissingEnv, name) {
			t.Fatalf("missing env = %#v, want %s", target.MissingEnv, name)
		}
	}
	if len(target.UnsatisfiedAlternatives) != 1 ||
		target.UnsatisfiedAlternatives[0].Description != "diagnosis LDAP role mapping" {
		t.Fatalf("alternatives = %#v, want LDAP role mapping alternative", target.UnsatisfiedAlternatives)
	}
	for _, secret := range []string{"api.example.test", "operator-1", "placeholder-ldap-password"} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked incomplete LDAP value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadDiagnosisLDAPBackendConfigWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-auth-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator-1",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder-ldap-password",
		"OPENCLARION_DIAGNOSIS_AUTH_MODE=ldap",
		"OPENCLARION_DIAGNOSIS_LDAP_URL=ldap://user:secret@ldap.example.test:389?ignored=true",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN= dc=example,dc=test ",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_DN=cn=openclarion,dc=example,dc=test",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD=placeholder\nservice-password",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER=(uid=*)",
		"OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE=mail primary",
		"OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE=memberOf",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES=leader",
		"OPENCLARION_DIAGNOSIS_LDAP_START_TLS=sometimes",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-auth-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked: %#v", target.Status, target)
	}
	for _, name := range []string{
		"OPENCLARION_DIAGNOSIS_LDAP_URL",
		"OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
		"OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
		"OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER",
		"OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE",
		"OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
		"OPENCLARION_DIAGNOSIS_LDAP_START_TLS",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"api.example.test",
		"user:secret",
		"ldap.example.test",
		"ignored=true",
		"dc=example,dc=test",
		"placeholder\nservice-password",
		"operator-1",
		"placeholder-ldap-password",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked bad LDAP backend value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisAuthLiveSmokeWithBearerToken(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-auth-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=bearer",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE=static",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-auth-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{"api.example.test", "secret-token"} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked diagnosis auth bearer value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadNotificationChannelLiveSmokeInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://operator:secret@api.example.test?token=value",
		"NOTIFICATION_CHANNEL_PROFILE_ID=0",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=bad-alias",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND=pager",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND=pager",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS=ai_diagnosis_sample,ai_diagnosis_sample",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=sometimes",
		"OPENCLARION_LIVE_BEARER_TOKEN=bad token",
		"NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT=0s",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_API_BASE_URL",
		"NOTIFICATION_CHANNEL_PROFILE_ID",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_KIND",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF",
		"OPENCLARION_LIVE_BEARER_TOKEN",
		"NOTIFICATION_CHANNEL_LIVE_SMOKE_TIMEOUT",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"operator:secret",
		"api.example.test",
		"token=value",
		"bad token",
		"bad-alias",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked bad notification channel value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsNotificationChannelAIProofConflictsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"NOTIFICATION_CHANNEL_EXPECTED_KIND=wecom",
		"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KINDS=ai_diagnosis_sample,diagnosis_close_sample",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=true",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if !invalidEnvByName(target.InvalidEnv, "NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF") {
		t.Fatalf("invalid env = %#v, want NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), "api.example.test") {
		t.Fatalf("output leaked notification channel AI proof conflict value: %s", stdout.String())
	}
}

func TestRunRejectsNonWeComForDiagnosisNotificationSmoke(t *testing.T) {
	for _, kind := range []string{"webhook", "dingtalk", "feishu", "slack", "email"} {
		t.Run(kind, func(t *testing.T) {
			var stdout bytes.Buffer
			err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
				"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
				"NOTIFICATION_CHANNEL_PROFILE_ID=2",
				"NOTIFICATION_CHANNEL_EXPECTED_KIND=" + kind,
				"NOTIFICATION_CHANNEL_EXPECTED_CONTENT_KIND=ai_diagnosis_sample",
				"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
			}, &stdout)
			if err != nil {
				t.Fatalf("run: %v", err)
			}

			out := decodeOutput(t, stdout.Bytes())
			target := targetByName(t, out, "notification-channel-live-smoke")
			if target.Status != "blocked" {
				t.Fatalf("target status = %q, want blocked", target.Status)
			}
			if !invalidEnvByName(target.InvalidEnv, "NOTIFICATION_CHANNEL_EXPECTED_KIND") {
				t.Fatalf("invalid env = %#v, want NOTIFICATION_CHANNEL_EXPECTED_KIND", target.InvalidEnv)
			}
			for _, secret := range []string{"api.example.test", "secret-token"} {
				if strings.Contains(stdout.String(), secret) {
					t.Fatalf("output leaked diagnosis notification value %q: %s", secret, stdout.String())
				}
			}
		})
	}
}

func TestRunRejectsNonWeComForNotificationChannelAIProof(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "notification-channel-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"NOTIFICATION_CHANNEL_EXPECTED_KIND=slack",
		"NOTIFICATION_CHANNEL_REQUIRE_AI_PROOF=true",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "notification-channel-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if !invalidEnvByName(target.InvalidEnv, "NOTIFICATION_CHANNEL_EXPECTED_KIND") {
		t.Fatalf("invalid env = %#v, want NOTIFICATION_CHANNEL_EXPECTED_KIND", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), "api.example.test") {
		t.Fatalf("output leaked notification channel AI proof value: %s", stdout.String())
	}
}

func TestRunRejectsBadDiagnosisAuthLiveSmokeInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-auth-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://operator:secret@api.example.test?token=value",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator one",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder\npassword",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE=legacy",
		"DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT=0s",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-auth-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_API_BASE_URL",
		"OPENCLARION_LIVE_LDAP_USERNAME",
		"OPENCLARION_LIVE_LDAP_PASSWORD",
		"OPENCLARION_LIVE_DIAGNOSIS_AUTH_EXPECTED_MODE",
		"DIAGNOSIS_AUTH_LIVE_SMOKE_TIMEOUT",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"operator:secret",
		"api.example.test",
		"token=value",
		"operator one",
		"placeholder\npassword",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked bad diagnosis auth value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunSupportsRepeatedTargetFlags(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{
		"--target", "alert-operations-live-inputs",
		"--target", "notification-channel-live-smoke",
		"--target", "diagnosis-live-browser-smoke",
	}, []string{
		"OPENCLARION_PROMETHEUS_URL=https://thanos-query.example.test",
		"OPENCLARION_LLM_MODEL=example-llm-model",
		"OPENCLARION_IM_WEBHOOK_URL=https://wecom-webhook.example.test/cgi-bin/webhook/send?key=placeholder-webhook-key",
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=2",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if len(out.Targets) != 3 {
		t.Fatalf("targets = %v, want three selected targets", targetNames(out.Targets))
	}
	if got := targetByName(t, out, "alert-operations-live-inputs").Status; got != "ready" {
		t.Fatalf("alert operations status = %q, want ready", got)
	}
	if got := targetByName(t, out, "notification-channel-live-smoke").Status; got != "ready" {
		t.Fatalf("notification channel status = %q, want ready", got)
	}
	if got := targetByName(t, out, "diagnosis-live-browser-smoke").Status; got != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", got)
	}
	if hasTarget(out, "report-live-smoke") {
		t.Fatalf("unexpected report-live-smoke target in repeated selection: %v", targetNames(out.Targets))
	}
	if strings.Contains(stdout.String(), "secret-token") || strings.Contains(stdout.String(), "wecom-webhook.example.test") {
		t.Fatalf("output leaked selected target env values: %s", stdout.String())
	}
}

func TestRunReportsReadyDiagnosisLiveSmokeWithStaticBearerFallback(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_DIAGNOSIS_AUTH_MODE=static",
		"OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN=Bearer static-secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{"api.example.test", "static-secret-token"} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked static bearer value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunSupportsCommaSeparatedTargets(t *testing.T) {
	var stdout bytes.Buffer
	windowStart, windowEnd := pastReportWindow()
	err := run([]string{
		"--target", "alert-operations-live-inputs,report-policy-live-smoke",
	}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_PROMETHEUS_URL=https://thanos-query.example.test",
		"OPENCLARION_LLM_MODEL=example-llm-model",
		"OPENCLARION_IM_WEBHOOK_URL=https://wecom-webhook.example.test/cgi-bin/webhook/send?key=placeholder-webhook-key",
		"REPORT_WORKFLOW_POLICY_ID=7",
		"REPORT_WINDOW_START=" + windowStart,
		"REPORT_WINDOW_END=" + windowEnd,
		"REPORT_POLICY_LIVE_SMOKE_OUTPUT=" + filepath.Join(t.TempDir(), "proof.json"),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if len(out.Targets) != 2 {
		t.Fatalf("targets = %v, want two selected targets", targetNames(out.Targets))
	}
	if got := targetByName(t, out, "alert-operations-live-inputs").Status; got != "ready" {
		t.Fatalf("alert operations status = %q, want ready", got)
	}
	if got := targetByName(t, out, "report-policy-live-smoke").Status; got != "ready" {
		t.Fatalf("report policy status = %q, want ready", got)
	}
}

func TestRunRejectsAllMixedWithSpecificTargets(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "all", "--target", "diagnosis-live-browser-smoke"}, nil, &stdout)
	if err == nil {
		t.Fatal("run error = nil, want all/specific target rejection")
	}
	if !strings.Contains(err.Error(), `target "all" cannot be combined`) {
		t.Fatalf("run error = %v, want all/specific rejection", err)
	}
}

func TestRunRejectsBadAlertOperationsLiveInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "alert-operations-live-inputs"}, []string{
		"OPENCLARION_PROMETHEUS_URL=https://operator:secret@thanos-query.example.test?token=placeholder",
		"OPENCLARION_ALERTMANAGER_URL=ftp://alertmanager.example.test/api/v2/alerts",
		"OPENCLARION_THANOS_RULE_URL=https://thanos-rule.example.test/alerts#placeholder",
		"OPENCLARION_LLM_BASE_URL=https://operator:secret@llm-gateway.example.test/v1",
		"OPENCLARION_LLM_API_KEY=placeholder key with spaces",
		"OPENCLARION_LLM_MODEL= example-llm-model",
		"OPENCLARION_IM_WEBHOOK_URL=https://operator:secret@wecom-webhook.example.test/cgi-bin/webhook/send?key=placeholder-webhook-key",
		"OPENCLARION_IM_WEBHOOK_FORMAT=pager",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "alert-operations-live-inputs")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_PROMETHEUS_URL",
		"OPENCLARION_ALERTMANAGER_URL",
		"OPENCLARION_THANOS_RULE_URL",
		"OPENCLARION_LLM_BASE_URL",
		"OPENCLARION_LLM_API_KEY",
		"OPENCLARION_LLM_MODEL",
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_IM_WEBHOOK_FORMAT",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"operator",
		"secret",
		"thanos-query.example.test",
		"alertmanager.example.test",
		"thanos-rule.example.test",
		"llm-gateway.example.test",
		"placeholder key with spaces",
		"example-llm-model",
		"wecom-webhook.example.test",
		"placeholder-webhook-key",
		"pager",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid live input value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyLiveTargets(t *testing.T) {
	var stdout bytes.Buffer
	windowStart, windowEnd := pastReportWindow()
	err := run(nil, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_PROMETHEUS_URL=https://prometheus.example.test",
		"REPORT_WINDOW_START=" + windowStart,
		"REPORT_WINDOW_END=" + windowEnd,
		"REPORT_SCENARIO=cascade",
		"REPORT_REPLAY_LIMIT=20",
		"REPORT_WAIT_TIMEOUT=3m",
		"REPORT_CORRELATION_KEY=manual-proof-001",
		"REPORT_WORKFLOW_ID=report-live-smoke-manual-proof-001",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_IM_WEBHOOK_URL=https://webhook.example.test",
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if got := targetByName(t, out, "report-live-smoke").Status; got != "ready" {
		t.Fatalf("report status = %q, want ready", got)
	}
	if got := targetByName(t, out, "alert-operations-live-inputs").Status; got != "ready" {
		t.Fatalf("alert operations live inputs status = %q, want ready", got)
	}
	if got := targetByName(t, out, "diagnosis-live-browser-smoke").Status; got != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", got)
	}
	if strings.Contains(stdout.String(), "secret-token") {
		t.Fatalf("output leaked bearer token: %s", stdout.String())
	}
}

func TestRunReportsReadyDiagnosisWithDevOIDCTokenURL(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL=http://127.0.0.1:32109/token?ttl=45m",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"127.0.0.1:32109",
		"ttl=45m",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked dev OIDC value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisConvergenceWithDevOIDCTokenURL(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-convergence-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL=http://127.0.0.1:32109/token?ttl=45m",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE=true",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE=yes",
		"OPENCLARION_LIVE_CONFIRM_CONCLUSION=1",
		"OPENCLARION_LIVE_TURN_TIMEOUT_MS=360000",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=7",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_TIMEOUT_MS=60000",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_POLL_MS=5000",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-convergence-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis convergence status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"127.0.0.1:32109",
		"ttl=45m",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked convergence env value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadDiagnosisConvergenceNotificationInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-convergence-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL=http://127.0.0.1:32109/token?ttl=45m",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=bad-channel",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_TIMEOUT_MS=soon",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_POLL_MS=never",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-convergence-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis convergence status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_TIMEOUT_MS",
		"OPENCLARION_LIVE_NOTIFICATION_PROOF_POLL_MS",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"api.example.test",
		"127.0.0.1:32109",
		"ttl=45m",
		"bad-channel",
		"soon",
		"never",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked convergence env value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsDiagnosisConvergenceProfileSecretWithoutWeComEndpoint(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-convergence-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL=http://127.0.0.1:32109/token?ttl=45m",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=7",
		`OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={"secret/openclarion/ops-wecom":"https://webhook.example.test/openclarion/fixture"}`,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-convergence-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis convergence status = %q, want blocked", target.Status)
	}
	if !invalidEnvByName(target.InvalidEnv, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		t.Fatalf("invalid env = %#v, want notification channel secret refs", target.InvalidEnv)
	}
	for _, secret := range []string{
		"api.example.test",
		"127.0.0.1:32109",
		"ttl=45m",
		"webhook.example.test",
		"fixture",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid convergence secret ref value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyAlertmanagerAutoDiagnosisLiveSmoke(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "alertmanager-auto-diagnosis-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID=42",
		"ALERTMANAGER_WEBHOOK_BEARER_TOKEN=Bearer secret-token",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID=5",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND=assistant_message",
		"ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS=assistant_message, Final_Conclusion",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_HTTP_TIMEOUT=10s",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ROOM_TIMEOUT=2m",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_POLL_INTERVAL=1s",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME=OpenClarionAutoDiagnosisSmoke",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "alertmanager-auto-diagnosis-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"secret-token",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked alertmanager auto diagnosis value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyAlertmanagerAutoDiagnosisLiveSmokeWithLiveAlias(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "alertmanager-auto-diagnosis-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID=42",
		"OPENCLARION_ALERTMANAGER_WEBHOOK_BEARER_TOKEN=Bearer secret-token",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND=Final_Conclusion",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "alertmanager-auto-diagnosis-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready: %#v", target.Status, target)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"secret-token",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked alertmanager auto diagnosis alias value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsAlertmanagerAutoDiagnosisProfileSecretWithoutWeComEndpoint(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "alertmanager-auto-diagnosis-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID=42",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID=5",
		`OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={"secret/openclarion/ops-wecom":"https://webhook.example.test/openclarion/fixture"}`,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "alertmanager-auto-diagnosis-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if !invalidEnvByName(target.InvalidEnv, "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON") {
		t.Fatalf("invalid env = %#v, want notification channel secret refs", target.InvalidEnv)
	}
	for _, secret := range []string{
		"api.example.test",
		"webhook.example.test",
		"fixture",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid alertmanager auto diagnosis secret ref value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadAlertmanagerAutoDiagnosisLiveSmokeInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "alertmanager-auto-diagnosis-live-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://operator:secret@api.example.test?token=value",
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID=0",
		"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID=bad-alias",
		"ALERTMANAGER_WEBHOOK_BEARER_TOKEN=bad token",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID=0",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND=raw_alert",
		"ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS=assistant_message,assistant_message",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_HTTP_TIMEOUT=0s",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME=Bad Alert",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "alertmanager-auto-diagnosis-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_API_BASE_URL",
		"ALERTMANAGER_WEBHOOK_SOURCE_PROFILE_ID",
		"OPENCLARION_LIVE_ALERT_SOURCE_PROFILE_ID",
		"ALERTMANAGER_WEBHOOK_BEARER_TOKEN",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_NOTIFICATION_CHANNEL_PROFILE_ID",
		"ALERTMANAGER_AUTO_DIAGNOSIS_EXPECTED_CONTENT_KIND",
		"ALERTMANAGER_AUTO_DIAGNOSIS_REQUIRED_CONTENT_KINDS",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_HTTP_TIMEOUT",
		"ALERTMANAGER_AUTO_DIAGNOSIS_LIVE_SMOKE_ALERT_NAME",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"operator:secret",
		"api.example.test",
		"token=value",
		"bad token",
		"bad-alias",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked bad alertmanager auto diagnosis value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisBrowserSmokeWithLDAPCredentials(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator-1",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder-ldap-password",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"operator-1",
		"placeholder-ldap-password",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked LDAP live value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisConvergenceWithLDAPCredentials(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-convergence-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator-1",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder-ldap-password",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE=true",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE=yes",
		"OPENCLARION_LIVE_CONFIRM_CONCLUSION=1",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-convergence-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis convergence status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"operator-1",
		"placeholder-ldap-password",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked LDAP convergence value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadDiagnosisLDAPCredentialsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_AUTH_MODE=ldap",
		"OPENCLARION_LIVE_LDAP_USERNAME=operator one",
		"OPENCLARION_LIVE_LDAP_PASSWORD=placeholder\npassword",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_LDAP_USERNAME",
		"OPENCLARION_LIVE_LDAP_PASSWORD",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"api.example.test",
		"operator one",
		"placeholder\npassword",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid LDAP value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsDiagnosisDevOIDCTokenURLWithoutLeakingValue(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL=https://issuer.example.test/token?ttl=45m",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	if !invalidEnvByName(target.InvalidEnv, "OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL") {
		t.Fatalf("invalid env = %#v, want OPENCLARION_LIVE_DEV_OIDC_TOKEN_URL", target.InvalidEnv)
	}
	for _, secret := range []string{
		"api.example.test",
		"issuer.example.test",
		"ttl=45m",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked dev OIDC value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsNextTargetUsesMilestoneSequence(t *testing.T) {
	var stdout bytes.Buffer
	windowStart, windowEnd := pastReportWindow()
	err := run(nil, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_PROMETHEUS_URL=https://prometheus.example.test",
		"REPORT_WINDOW_START=" + windowStart,
		"REPORT_WINDOW_END=" + windowEnd,
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_IM_WEBHOOK_URL=https://webhook.example.test",
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_NOTIFICATION_CHANNEL_PROFILE_ID=2",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if got := targetByName(t, out, "report-live-smoke").Status; got != "ready" {
		t.Fatalf("report status = %q, want ready", got)
	}
	if out.Summary.NextTarget == nil {
		t.Fatal("summary next target is nil, want report policy target")
	}
	if out.Summary.NextTarget.Name != "report-policy-live-smoke" {
		t.Fatalf("summary next target = %#v, want report-policy-live-smoke", out.Summary.NextTarget)
	}
	reportPolicy := targetByName(t, out, "report-policy-live-smoke")
	if reportPolicy.Milestone != "M3.1" || reportPolicy.Sequence != 15 || reportPolicy.EvidenceGoal == "" {
		t.Fatalf("report policy sequencing metadata = milestone %q sequence %d goal %q, want M3.1/15/goal",
			reportPolicy.Milestone, reportPolicy.Sequence, reportPolicy.EvidenceGoal)
	}
	diagnosis := targetByName(t, out, "diagnosis-live-browser-smoke")
	if diagnosis.Milestone != "M5" || diagnosis.Sequence != 20 || diagnosis.EvidenceGoal == "" {
		t.Fatalf("diagnosis sequencing metadata = milestone %q sequence %d goal %q, want M5/20/goal",
			diagnosis.Milestone, diagnosis.Sequence, diagnosis.EvidenceGoal)
	}
	if strings.Contains(stdout.String(), "prometheus.example.test") || strings.Contains(stdout.String(), "webhook.example.test") {
		t.Fatalf("output leaked configured URL: %s", stdout.String())
	}
}

func TestRunOrdersAllTargetsByMilestoneSequence(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, nil, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	wantNames := []string{
		"alert-operations-live-inputs",
		"notification-channel-live-smoke",
		"report-live-smoke",
		"report-policy-live-smoke",
		"report-schedule-live-smoke",
		"diagnosis-auth-live-smoke",
		"diagnosis-live-browser-smoke",
		"diagnosis-live-convergence-smoke",
		"alertmanager-auto-diagnosis-live-smoke",
		"sandbox-m4-baseline-audit",
		"sandbox-m4-runtime-smoke-artifacts",
		"sandbox-m4-quality-sample-export",
		"sandbox-m4-quality-manifest-prepare",
		"sandbox-m4-quality-compare",
		"sandbox-m4-review-evidence-template",
		"sandbox-m4-decision",
		"sandbox-m4-evidence-packet",
		"sandbox-m4-evidence-chain",
	}
	if len(out.Targets) != len(wantNames) {
		t.Fatalf("targets = %d, want %d", len(out.Targets), len(wantNames))
	}
	for i, want := range wantNames {
		if got := out.Targets[i].Name; got != want {
			t.Fatalf("targets[%d].name = %q, want %q", i, got, want)
		}
	}
	if out.Summary.NextTarget == nil || out.Summary.NextTarget.Name != "alert-operations-live-inputs" {
		t.Fatalf("summary next target = %#v, want alert-operations-live-inputs", out.Summary.NextTarget)
	}
}

func TestRunRejectsBadReportLiveInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	futureStart := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	futureEnd := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	err := run([]string{"--target", "report-live-smoke"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_PROMETHEUS_URL=https://operator:secret@prometheus.example.test?token=secret",
		"REPORT_WINDOW_START=" + futureStart,
		"REPORT_WINDOW_END=" + futureEnd,
		"REPORT_SCENARIO= single_alert",
		"REPORT_REPLAY_LIMIT=0",
		"REPORT_WAIT_TIMEOUT=soon",
		"REPORT_CORRELATION_KEY=manual proof",
		"REPORT_WORKFLOW_ID=workflow\nsecret",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_IM_WEBHOOK_URL=https://placeholder-user-abc123:placeholder-pass-abc123@webhook.example.test",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("report status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_PROMETHEUS_URL",
		"OPENCLARION_IM_WEBHOOK_URL",
		"REPORT_WINDOW_START/REPORT_WINDOW_END",
		"REPORT_SCENARIO",
		"REPORT_REPLAY_LIMIT",
		"REPORT_WAIT_TIMEOUT",
		"REPORT_CORRELATION_KEY",
		"REPORT_WORKFLOW_ID",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"placeholder-user-abc123",
		"placeholder-pass-abc123",
		"prometheus.example.test",
		"webhook.example.test",
		"manual proof",
		"workflow",
		"soon",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid environment value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyReportPolicyLiveTarget(t *testing.T) {
	var stdout bytes.Buffer
	windowStart, windowEnd := pastReportWindow()
	output := filepath.Join(t.TempDir(), "policy-proof.json")
	err := run([]string{"--target", "report-policy-live-smoke"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"REPORT_WORKFLOW_POLICY_ID=42",
		"REPORT_WINDOW_START=" + windowStart,
		"REPORT_WINDOW_END=" + windowEnd,
		"REPORT_POLICY_LIVE_SMOKE_OUTPUT=" + output,
		"REPORT_REPLAY_LIMIT=20",
		"REPORT_WAIT_TIMEOUT=3m",
		"REPORT_CORRELATION_KEY=policy-proof-001",
		"REPORT_WORKFLOW_ID=report-policy-live-smoke-proof-001",
		"OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON={\"secret/openclarion/prometheus\":\"resolved-token\"}",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={\"secret/openclarion/report-webhook\":\"https://webhook.example.test/path\"}",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-policy-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("report policy status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "REPORT_POLICY_LIVE_SMOKE_OUTPUT").Status; got != "ok" {
		t.Fatalf("REPORT_POLICY_LIVE_SMOKE_OUTPUT status = %q, want ok", got)
	}
	for _, secret := range []string{
		"example.test",
		"127.0.0.1:7233",
		"resolved-token",
		"webhook.example.test",
		"policy-proof-001",
		output,
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked policy live value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRequireReadyFailsWhenSelectedTargetIsBlocked(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "report-policy-live-smoke", "--require-ready"}, nil, &stdout)
	if err == nil {
		t.Fatal("run succeeded, want blocked target error")
	}
	if !strings.Contains(err.Error(), "selected readiness target is blocked") {
		t.Fatalf("error = %v, want blocked target message", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-policy-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if !missingEnvByName(target.MissingEnv, "REPORT_POLICY_LIVE_SMOKE_OUTPUT") {
		t.Fatalf("missing env = %#v, want REPORT_POLICY_LIVE_SMOKE_OUTPUT", target.MissingEnv)
	}
}

func TestRunReportsReadyReportScheduleLiveTarget(t *testing.T) {
	var stdout bytes.Buffer
	output := filepath.Join(t.TempDir(), "schedule-proof.json")
	err := run([]string{"--target", "report-schedule-live-smoke"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"REPORT_WORKFLOW_SCHEDULE_ID=9",
		"REPORT_WORKFLOW_POLICY_ID=42",
		"TEMPORAL_SCHEDULE_ID=openclarion-report-policy-42-hourly",
		"REPORT_SCHEDULE_WAIT_TIMEOUT=30m",
		"REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT=" + output,
		"OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON={\"secret/openclarion/prometheus\":\"resolved-token\"}",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={\"secret/openclarion/report-webhook\":\"https://webhook.example.test/path\"}",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-schedule-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("report schedule status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"example.test",
		"127.0.0.1:7233",
		"resolved-token",
		"webhook.example.test",
		"openclarion-report-policy-42-hourly",
		output,
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked schedule live value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadReportScheduleLiveInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	output := filepath.Join(t.TempDir(), "schedule-proof.json")
	if err := os.WriteFile(output, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	err := run([]string{"--target", "report-schedule-live-smoke"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"REPORT_WORKFLOW_SCHEDULE_ID=0",
		"REPORT_WORKFLOW_POLICY_ID=0",
		"TEMPORAL_SCHEDULE_ID=bad id",
		"REPORT_SCHEDULE_WAIT_TIMEOUT=soon",
		"REPORT_SCHEDULE_LIVE_SMOKE_OUTPUT=" + output,
		"OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON={\"bad ref\":\"resolved-token\"}",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_IM_WEBHOOK_URL=https://schedule-user-abc123:schedule-pass-abc123@webhook.example.test",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={\"secret/openclarion/report-webhook\":\"bad value\"}",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-schedule-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("report schedule status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"REPORT_WORKFLOW_SCHEDULE_ID",
		"REPORT_WORKFLOW_POLICY_ID",
		"TEMPORAL_SCHEDULE_ID",
		"REPORT_SCHEDULE_WAIT_TIMEOUT",
		"OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON",
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	if len(target.FileChecks) != 1 || target.FileChecks[0].Status != "exists" {
		t.Fatalf("file checks = %#v, want existing output rejection", target.FileChecks)
	}
	for _, secret := range []string{
		"example.test",
		"schedule-user-abc123",
		"schedule-pass-abc123",
		"webhook.example.test",
		"resolved-token",
		"bad value",
		"bad id",
		"soon",
		output,
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid schedule live value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReportPolicyLiveAcceptsWorkerReadyAlternative(t *testing.T) {
	var stdout bytes.Buffer
	windowStart, windowEnd := pastReportWindow()
	output := filepath.Join(t.TempDir(), "policy-proof.json")
	err := run([]string{"--target", "report-policy-live-smoke"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"REPORT_WORKFLOW_POLICY_ID=42",
		"REPORT_WINDOW_START=" + windowStart,
		"REPORT_WINDOW_END=" + windowEnd,
		"REPORT_POLICY_LIVE_SMOKE_OUTPUT=" + output,
		"REPORT_LIVE_SMOKE_ASSUME_WORKER_READY=1",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-policy-live-smoke")
	if target.Status != "ready" {
		t.Fatalf("report policy status = %q, want ready", target.Status)
	}
	if len(target.UnsatisfiedAlternatives) != 0 {
		t.Fatalf("alternatives = %#v, want none", target.UnsatisfiedAlternatives)
	}
}

func TestRunRejectsBadReportPolicyLiveInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	futureStart := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	futureEnd := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	output := filepath.Join(t.TempDir(), "policy-proof.json")
	if err := os.WriteFile(output, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write output: %v", err)
	}
	err := run([]string{"--target", "report-policy-live-smoke"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"REPORT_WORKFLOW_POLICY_ID=0",
		"REPORT_WINDOW_START=" + futureStart,
		"REPORT_WINDOW_END=" + futureEnd,
		"REPORT_POLICY_LIVE_SMOKE_OUTPUT=" + output,
		"REPORT_REPLAY_LIMIT=0",
		"REPORT_WAIT_TIMEOUT=soon",
		"REPORT_CORRELATION_KEY=policy proof",
		"REPORT_WORKFLOW_ID=workflow\nsecret",
		"OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON={\"bad ref\":\"resolved-token\"}",
		"OPENCLARION_LLM_MODEL=gpt-example",
		"OPENCLARION_IM_WEBHOOK_URL=https://operator:secret@webhook.example.test",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={\"secret/openclarion/report-webhook\":\"bad value\"}",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "report-policy-live-smoke")
	if target.Status != "blocked" {
		t.Fatalf("report policy status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"REPORT_WORKFLOW_POLICY_ID",
		"REPORT_WINDOW_START/REPORT_WINDOW_END",
		"REPORT_REPLAY_LIMIT",
		"REPORT_WAIT_TIMEOUT",
		"REPORT_CORRELATION_KEY",
		"REPORT_WORKFLOW_ID",
		"OPENCLARION_ALERT_SOURCE_SECRET_REFS_JSON",
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	if got := fileCheckByEnv(t, target.FileChecks, "REPORT_POLICY_LIVE_SMOKE_OUTPUT").Status; got != "exists" {
		t.Fatalf("REPORT_POLICY_LIVE_SMOKE_OUTPUT status = %q, want exists", got)
	}
	for _, secret := range []string{
		"example.test",
		"operator",
		"secret",
		"webhook.example.test",
		"resolved-token",
		"bad value",
		"policy proof",
		"workflow\nsecret",
		"soon",
		output,
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid policy live value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisCloseNotificationPrerequisites(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_WEB_BASE_URL=https://web.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=1",
		"OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT=2m",
		"OPENCLARION_LIVE_CLOSE_REASON=live_smoke_completed",
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_IM_WEBHOOK_URL=https://webhook.example.test/diagnosis",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"web.example.test",
		"secret-token",
		"example.test/openclarion",
		"127.0.0.1:7233",
		"webhook.example.test",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked environment value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisCloseNotificationWithProfileSecrets(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_WEB_BASE_URL=https://web.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=1",
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		`OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={"secret/openclarion/ops-wecom":"` + testReadinessWeComWebhookURL("placeholder-webhook-key") + `"}`,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"web.example.test",
		"secret-token",
		"example.test/openclarion",
		"127.0.0.1:7233",
		"placeholder-webhook-key",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked profile-backed close notification value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunBlocksDiagnosisCloseNotificationProfileSecretWithoutWeComEndpoint(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_WEB_BASE_URL=https://web.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=1",
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		`OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON={"secret/openclarion/ops-wecom":"https://webhook.example.test/openclarion/fixture"}`,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 ||
		target.InvalidEnv[0].Name != "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON" {
		t.Fatalf("invalid env = %#v, want notification channel secret refs", target.InvalidEnv)
	}
	for _, secret := range []string{
		"api.example.test",
		"web.example.test",
		"secret-token",
		"example.test/openclarion",
		"127.0.0.1:7233",
		"webhook.example.test",
		"fixture",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid profile-backed close notification value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunBlocksDiagnosisCloseNotificationMissingPrerequisites(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=secret-token",
		"OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID=session-123",
		"OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=true",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	if !contains(target.MissingEnv, "DATABASE_URL") || !contains(target.MissingEnv, "TEMPORAL_HOST_PORT") {
		t.Fatalf("missing env = %#v, want database and temporal prerequisites", target.MissingEnv)
	}
	if len(target.UnsatisfiedAlternatives) != 1 ||
		target.UnsatisfiedAlternatives[0].Description != "close-notification worker IM configuration" {
		t.Fatalf("alternatives = %#v, want close-notification worker alternative", target.UnsatisfiedAlternatives)
	}
	if strings.Contains(stdout.String(), "api.example.test") || strings.Contains(stdout.String(), "secret-token") || strings.Contains(stdout.String(), "session-123") {
		t.Fatalf("output leaked live diagnosis values: %s", stdout.String())
	}
}

func TestRunAcceptsDiagnosisCloseNotificationWorkerReadyAlternative(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=secret-token",
		"OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID=session-123",
		"OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=YES",
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"DIAGNOSIS_LIVE_SMOKE_ASSUME_WORKER_READY=1",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.UnsatisfiedAlternatives) != 0 {
		t.Fatalf("alternatives = %#v, want none", target.UnsatisfiedAlternatives)
	}
}

func TestRunReportsReadyDiagnosisSupplementalEvidencePrerequisites(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_WEB_BASE_URL=https://web.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE=yes",
		"OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE=true",
		"OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE=operator verified {label} with requested detail {detail}",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"web.example.test",
		"secret-token",
		"operator verified {label} with requested detail {detail}",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked supplemental live value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunReportsReadyDiagnosisToolRequestsPrerequisites(t *testing.T) {
	toolRequests := `[` +
		`{"tool":"active_alerts","reason":"Collect current active alerts.","limit":10},` +
		`{"tool":"metric_query","reason":"Collect target availability.","template_id":1,"alert_source_profile_id":2,"limit":20},` +
		`{"tool":"metric_range_query","reason":"Collect pod CPU trend.","query":"sum by (pod) (rate(container_cpu_usage_seconds_total{namespace=\"prod\"}[5m]))","window_seconds":3600,"step_seconds":60,"limit":10}` +
		`]`

	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_WEB_BASE_URL=https://web.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE=yes",
		"OPENCLARION_LIVE_TOOL_REQUESTS_JSON=" + toolRequests,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "ready" {
		t.Fatalf("diagnosis status = %q, want ready", target.Status)
	}
	if len(target.MissingEnv) != 0 || len(target.UnsatisfiedAlternatives) != 0 || len(target.InvalidEnv) != 0 {
		t.Fatalf("target has unexpected blockers: %#v", target)
	}
	for _, secret := range []string{
		"api.example.test",
		"web.example.test",
		"secret-token",
		"Collect current active alerts.",
		"Collect target availability.",
		"Collect pod CPU trend.",
		"container_cpu_usage_seconds_total",
		`namespace=\"prod\"`,
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked tool request value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsDiagnosisToolRequestTemplateWithoutProfileWithoutLeakingValues(t *testing.T) {
	toolRequests := `[{"tool":"metric_query","reason":"Collect target availability secret.","template_id":1,"limit":20}]`

	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_TOOL_REQUESTS_JSON=" + toolRequests,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "OPENCLARION_LIVE_TOOL_REQUESTS_JSON" {
		t.Fatalf("invalid env = %#v, want tool requests rejection", target.InvalidEnv)
	}
	if !strings.Contains(target.InvalidEnv[0].Reason, "template_id requires alert_source_profile_id") {
		t.Fatalf("invalid reason = %q, want template/profile rejection", target.InvalidEnv[0].Reason)
	}
	for _, secret := range []string{
		"api.example.test",
		"secret-token",
		"Collect target availability secret.",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked template request value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadDiagnosisLiveInputsWithoutLeakingValues(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://operator:secret@api.example.test?token=secret",
		"OPENCLARION_LIVE_WEB_BASE_URL=https://web.example.test/#secret",
		"OPENCLARION_LIVE_BROWSER_WS_BASE_URL=wss://operator:secret@ws.example.test/socket?token=secret",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer token with spaces",
		"OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID=session-123",
		"OPENCLARION_LIVE_REQUIRE_CLOSE_NOTIFICATION=1",
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE=maybe",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE=maybe",
		"OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE=yes",
		"OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT= supplemental evidence with leading whitespace",
		"OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE= template evidence with leading whitespace",
		"OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT=soon",
		"OPENCLARION_LIVE_CLOSE_REASON= live_smoke_completed",
		"DATABASE_URL=postgres://example.test/openclarion",
		"TEMPORAL_HOST_PORT=127.0.0.1:7233",
		"OPENCLARION_IM_WEBHOOK_URL=https://operator:secret@webhook.example.test",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	for _, name := range []string{
		"OPENCLARION_LIVE_API_BASE_URL",
		"OPENCLARION_LIVE_WEB_BASE_URL",
		"OPENCLARION_LIVE_BROWSER_WS_BASE_URL",
		"OPENCLARION_LIVE_BEARER_TOKEN",
		"OPENCLARION_IM_WEBHOOK_URL",
		"OPENCLARION_LIVE_CLOSE_WAIT_TIMEOUT",
		"OPENCLARION_LIVE_CLOSE_REASON",
		"OPENCLARION_LIVE_COLLECT_PLANNED_EVIDENCE",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE",
		"OPENCLARION_LIVE_REQUIRE_SUPPLEMENTAL_EVIDENCE",
		"OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT",
		"OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEMPLATE",
	} {
		if !invalidEnvByName(target.InvalidEnv, name) {
			t.Fatalf("invalid env = %#v, want %s", target.InvalidEnv, name)
		}
	}
	for _, secret := range []string{
		"operator",
		"secret",
		"api.example.test",
		"web.example.test",
		"ws.example.test",
		"token with spaces",
		"webhook.example.test",
		"soon",
		"live_smoke_completed",
		"maybe",
		"supplemental evidence with leading whitespace",
		"template evidence with leading whitespace",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid environment value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsBadDiagnosisToolRequestsWithoutLeakingValues(t *testing.T) {
	toolRequests := `[{"tool":"metric_range_query","reason":"metric trend secret note","query":"up{job=\"secret-job\"}","window_seconds":60,"step_seconds":120,"limit":21}]`

	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=7",
		"OPENCLARION_LIVE_TOOL_REQUESTS_JSON=" + toolRequests,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "OPENCLARION_LIVE_TOOL_REQUESTS_JSON" {
		t.Fatalf("invalid env = %#v, want tool requests rejection", target.InvalidEnv)
	}
	for _, secret := range []string{
		"api.example.test",
		"secret-token",
		"metric trend secret note",
		"secret-job",
		"up{job",
	} {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("output leaked invalid tool request value %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsOversizedDiagnosisSupplementalEvidenceText(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=secret-token",
		"OPENCLARION_LIVE_DIAGNOSIS_SESSION_ID=session-123",
		"OPENCLARION_LIVE_SUBMIT_SUPPLEMENTAL_EVIDENCE=1",
		"OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT=" + strings.Repeat("a", maxReadinessSupplementalBytes+1),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "diagnosis-live-browser-smoke")
	if target.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "OPENCLARION_LIVE_SUPPLEMENTAL_EVIDENCE_TEXT" {
		t.Fatalf("invalid env = %#v, want supplemental evidence text rejection", target.InvalidEnv)
	}
}

func TestRunValidatesM4EvidenceFilesAndPacketOutputDir(t *testing.T) {
	root := t.TempDir()
	baseline := writeFile(t, root, "baseline.json")
	quality := writeFile(t, root, "quality.json")
	review := writeFile(t, root, "review.json")
	manifest := writeFile(t, root, "manifest.json")
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, sampleRoot, "cascade", "checkout-latency")
	writeQualitySamplePair(t, sampleRoot, "alert_storm", "billing-errors")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}
	collectedRuntimeArtifacts := filepath.Join(root, "collected-runtime-artifacts")
	outDir := filepath.Join(root, "packet")
	manifestOut := filepath.Join(root, "prepared-quality-manifest.json")

	var stdout bytes.Buffer
	err := run(nil, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + manifestOut,
		"BASELINE_AUDIT=" + baseline,
		"QUALITY_COMPARISON=" + quality,
		"REVIEW_EVIDENCE=" + review,
		"QUALITY_MANIFEST=" + manifest,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + collectedRuntimeArtifacts,
		"OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE=registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"REVIEWER=openclarion-maintainer",
		"OUT_DIR=" + outDir,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	if got := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts").Status; got != "ready" {
		t.Fatalf("runtime smoke artifacts status = %q, want ready", got)
	}
	manifestTarget := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	if manifestTarget.Status != "ready" {
		t.Fatalf("quality manifest prepare status = %q, want ready", manifestTarget.Status)
	}
	if len(manifestTarget.QualitySampleChecks) != 1 || manifestTarget.QualitySampleChecks[0].PairedCases != 3 {
		t.Fatalf("quality sample checks = %#v, want three paired cases", manifestTarget.QualitySampleChecks)
	}
	if got := targetByName(t, out, "sandbox-m4-baseline-audit").Status; got != "ready" {
		t.Fatalf("baseline audit status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-quality-compare").Status; got != "ready" {
		t.Fatalf("quality compare status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-review-evidence-template").Status; got != "ready" {
		t.Fatalf("review evidence template status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-decision").Status; got != "ready" {
		t.Fatalf("decision status = %q, want ready", got)
	}
	if got := targetByName(t, out, "sandbox-m4-evidence-packet").Status; got != "ready" {
		t.Fatalf("packet status = %q, want ready", got)
	}
}

func TestRunReportsM4QualityManifestSampleReadiness(t *testing.T) {
	root := t.TempDir()
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, sampleRoot, "cascade", "checkout-latency")
	writeQualitySamplePair(t, sampleRoot, "alert_storm", "billing-errors")
	outPath := filepath.Join(root, "quality-manifest.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-manifest-prepare"}, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if len(target.QualitySampleChecks) != 1 {
		t.Fatalf("quality sample checks = %#v, want one", target.QualitySampleChecks)
	}
	check := target.QualitySampleChecks[0]
	if check.Status != "ok" || check.DirectReports != 3 || check.SandboxReports != 3 || check.PairedCases != 3 {
		t.Fatalf("quality sample check = %#v, want ok counts", check)
	}
	fileCheck := fileCheckByEnv(t, target.FileChecks, "OUT")
	if fileCheck.Status != "ok" {
		t.Fatalf("OUT check = %#v, want ok", fileCheck)
	}
	if strings.Contains(stdout.String(), sampleRoot) || strings.Contains(stdout.String(), outPath) || strings.Contains(stdout.String(), "payments-cpu") {
		t.Fatalf("output leaked sample path or case id: %s", stdout.String())
	}
}

func TestRunReportsM4QualitySampleExportReadiness(t *testing.T) {
	root := t.TempDir()
	selection := writeFile(t, root, "selection.json")
	outRoot := filepath.Join(root, "exported-quality-samples")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-sample-export"}, []string{
		"DATABASE_URL=postgres://example.test/openclarion",
		"SELECTION=" + selection,
		"ROOT=" + outRoot,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-sample-export")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "SELECTION").Status; got != "ok" {
		t.Fatalf("SELECTION status = %q, want ok", got)
	}
	if got := directoryCheckByEnv(t, target.DirectoryChecks, "ROOT").Status; got != "ok" {
		t.Fatalf("ROOT status = %q, want ok", got)
	}
	if strings.Contains(stdout.String(), "example.test") || strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked environment values: %s", stdout.String())
	}
}

func TestRunReportsM4BaselineAuditReadiness(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "baseline-audit.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-baseline-audit"}, []string{
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-baseline-audit")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "ok" {
		t.Fatalf("OUT status = %q, want ok", got)
	}
	if strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked baseline audit output path: %s", stdout.String())
	}
}

func TestRunBlocksM4BaselineAuditExistingOutputWithoutLeakingPath(t *testing.T) {
	root := t.TempDir()
	outPath := writeFile(t, root, "baseline-audit.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-baseline-audit"}, []string{
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-baseline-audit")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "exists" {
		t.Fatalf("OUT status = %q, want exists", got)
	}
	if strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked baseline audit output path: %s", stdout.String())
	}
}

func TestRunReportsM4QualityCompareReadiness(t *testing.T) {
	root := t.TempDir()
	manifest := writeFile(t, root, "quality-manifest.json")
	outPath := filepath.Join(root, "quality-comparison.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-compare"}, []string{
		"QUALITY_MANIFEST=" + manifest,
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-compare")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "QUALITY_MANIFEST").Status; got != "ok" {
		t.Fatalf("QUALITY_MANIFEST status = %q, want ok", got)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "ok" {
		t.Fatalf("OUT status = %q, want ok", got)
	}
	if strings.Contains(stdout.String(), manifest) || strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked quality compare paths: %s", stdout.String())
	}
}

func TestRunBlocksM4QualityCompareExistingOutputWithoutLeakingPath(t *testing.T) {
	root := t.TempDir()
	manifest := writeFile(t, root, "quality-manifest.json")
	outPath := writeFile(t, root, "quality-comparison.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-compare"}, []string{
		"QUALITY_MANIFEST=" + manifest,
		"OUT=" + outPath,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-compare")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if got := fileCheckByEnv(t, target.FileChecks, "OUT").Status; got != "exists" {
		t.Fatalf("OUT status = %q, want exists", got)
	}
	if strings.Contains(stdout.String(), manifest) || strings.Contains(stdout.String(), outPath) {
		t.Fatalf("output leaked quality compare paths: %s", stdout.String())
	}
}

func TestRunBlocksM4QualityManifestSampleGapsWithoutLeakingCases(t *testing.T) {
	root := t.TempDir()
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySampleReport(t, sampleRoot, "direct", "cascade", "checkout-latency")
	if err := os.MkdirAll(filepath.Join(sampleRoot, "sandbox", "cascade"), 0o700); err != nil {
		t.Fatalf("mkdir sandbox cascade: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-manifest-prepare"}, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + filepath.Join(root, "quality-manifest.json"),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	check := target.QualitySampleChecks[0]
	if check.Status != "missing_counterparts" {
		t.Fatalf("sample status = %q, want missing_counterparts", check.Status)
	}
	if check.MissingSandboxReports != 1 {
		t.Fatalf("missing sandbox reports = %d, want 1", check.MissingSandboxReports)
	}
	if strings.Contains(stdout.String(), sampleRoot) || strings.Contains(stdout.String(), "checkout-latency") {
		t.Fatalf("output leaked sample path or case id: %s", stdout.String())
	}
}

func TestRunBlocksM4QualityManifestMissingScenario(t *testing.T) {
	root := t.TempDir()
	sampleRoot := filepath.Join(root, "quality-samples")
	writeQualitySamplePair(t, sampleRoot, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, sampleRoot, "cascade", "checkout-latency")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-quality-manifest-prepare"}, []string{
		"ROOT=" + sampleRoot,
		"SAMPLE_BASIS=representative retained alert cases",
		"OUT=" + filepath.Join(root, "quality-manifest.json"),
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-quality-manifest-prepare")
	check := target.QualitySampleChecks[0]
	if check.Status != "missing_scenario_coverage" {
		t.Fatalf("sample status = %q, want missing_scenario_coverage", check.Status)
	}
	if !contains(check.MissingScenarios, "alert_storm") {
		t.Fatalf("missing scenarios = %#v, want alert_storm", check.MissingScenarios)
	}
}

func TestRunRejectsBadM4RuntimeSmokeArtifactEnv(t *testing.T) {
	root := t.TempDir()
	artifactsDir := filepath.Join(root, "runtime-artifacts")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-runtime-smoke-artifacts"}, []string{
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + artifactsDir,
		"OPENCLARION_AGENT_RUNTIME_IMAGE=registry.example.com/openclarion/runtime-candidate-a:latest",
		"OPENCLARION_M4_RUNTIME_SMOKE_PULL=sometimes",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 2 {
		t.Fatalf("invalid env = %#v, want image and pull rejections", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), "latest") || strings.Contains(stdout.String(), "sometimes") {
		t.Fatalf("output leaked invalid environment value: %s", stdout.String())
	}
}

func TestRunReportsCustomThinRunnerArtifactAlternative(t *testing.T) {
	root := t.TempDir()
	customArtifacts := filepath.Join(root, "custom-runtime-artifacts")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-runtime-smoke-artifacts"}, []string{
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + filepath.Join(root, "direct-runtime-artifacts"),
		"OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=" + customArtifacts,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked without primary runtime image", target.Status)
	}
	if !contains(target.MissingEnv, "OPENCLARION_AGENT_RUNTIME_IMAGE") {
		t.Fatalf("missing env = %#v, want primary runtime image", target.MissingEnv)
	}
	if len(target.AlternateCommands) != 1 || !strings.Contains(target.AlternateCommands[0].Command, "custom-thin-runner-smoke") {
		t.Fatalf("alternate commands = %#v, want custom thin runner artifact command", target.AlternateCommands)
	}
	check := directoryCheckByEnv(t, target.OptionalDirectoryChecks, "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR")
	if check.Status != "ok" {
		t.Fatalf("custom artifact dir status = %q, want ok", check.Status)
	}
	if strings.Contains(stdout.String(), customArtifacts) {
		t.Fatalf("output leaked custom artifact path: %s", stdout.String())
	}
}

func TestRunRejectsReusedCustomThinRunnerArtifactDir(t *testing.T) {
	root := t.TempDir()
	customArtifacts := filepath.Join(root, "custom-runtime-artifacts")
	if err := os.Mkdir(customArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir custom artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customArtifacts, "old.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write old artifact: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-runtime-smoke-artifacts"}, []string{
		"OPENCLARION_M4_RUNTIME_SMOKE_ARTIFACTS_DIR=" + filepath.Join(root, "direct-runtime-artifacts"),
		"OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR=" + customArtifacts,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-runtime-smoke-artifacts")
	check := directoryCheckByEnv(t, target.OptionalDirectoryChecks, "OPENCLARION_CUSTOM_THIN_RUNNER_ARTIFACTS_DIR")
	if check.Status != "not_empty" {
		t.Fatalf("custom artifact dir status = %q, want not_empty", check.Status)
	}
	if strings.Contains(stdout.String(), customArtifacts) {
		t.Fatalf("output leaked custom artifact path: %s", stdout.String())
	}
}

func TestRunRejectsBadM4RuntimeCandidateEnv(t *testing.T) {
	root := t.TempDir()
	quality := writeFile(t, root, "quality.json")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-review-evidence-template"}, []string{
		"QUALITY_COMPARISON=" + quality,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE=runtime-candidate-a:latest",
		"REVIEWER=openclarion-maintainer",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-review-evidence-template")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "RUNTIME_CANDIDATE" {
		t.Fatalf("invalid env = %#v, want runtime candidate rejection", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), "latest") {
		t.Fatalf("output leaked runtime candidate value: %s", stdout.String())
	}
}

func TestRunAcceptsM4RuntimeCandidateFile(t *testing.T) {
	root := t.TempDir()
	quality := writeFile(t, root, "quality.json")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}
	candidateFile := filepath.Join(root, "digest-ref.txt")
	if err := os.WriteFile(candidateFile, []byte("registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatalf("write runtime candidate file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-review-evidence-template"}, []string{
		"QUALITY_COMPARISON=" + quality,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE_FILE=" + candidateFile,
		"REVIEWER=openclarion-maintainer",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-review-evidence-template")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	check := fileCheckByEnv(t, target.FileChecks, "RUNTIME_CANDIDATE_FILE")
	if check.Status != "ok" {
		t.Fatalf("runtime candidate file status = %q, want ok", check.Status)
	}
	if strings.Contains(stdout.String(), candidateFile) || strings.Contains(stdout.String(), "sha256:0123456789abcdef") {
		t.Fatalf("output leaked runtime candidate file path or value: %s", stdout.String())
	}
}

func TestRunRejectsBadM4RuntimeCandidateFile(t *testing.T) {
	root := t.TempDir()
	quality := writeFile(t, root, "quality.json")
	runtimeArtifacts := filepath.Join(root, "runtime-artifacts")
	if err := os.Mkdir(runtimeArtifacts, 0o700); err != nil {
		t.Fatalf("mkdir runtime artifacts: %v", err)
	}
	candidateFile := filepath.Join(root, "digest-ref.txt")
	if err := os.WriteFile(candidateFile, []byte("runtime-candidate-a:latest\n"), 0o600); err != nil {
		t.Fatalf("write runtime candidate file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-review-evidence-template"}, []string{
		"QUALITY_COMPARISON=" + quality,
		"RUNTIME_SMOKE_ARTIFACTS_ROOT=" + runtimeArtifacts,
		"SELECTED_CANDIDATE=runtime-candidate-a",
		"RUNTIME_CANDIDATE_FILE=" + candidateFile,
		"REVIEWER=openclarion-maintainer",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-review-evidence-template")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if len(target.InvalidEnv) != 1 || target.InvalidEnv[0].Name != "RUNTIME_CANDIDATE_FILE" {
		t.Fatalf("invalid env = %#v, want runtime candidate file rejection", target.InvalidEnv)
	}
	if strings.Contains(stdout.String(), candidateFile) || strings.Contains(stdout.String(), "latest") {
		t.Fatalf("output leaked runtime candidate file path or value: %s", stdout.String())
	}
}

func TestRunReportsM4EvidenceChainGapsWithoutLeakingRoot(t *testing.T) {
	root := t.TempDir()
	writeM4EvidenceChainRuntimeArtifacts(t, root)
	writeFile(t, root, "baseline-audit.json")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-chain"}, []string{
		"OPENCLARION_M4_EVIDENCE_ROOT=" + root,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-evidence-chain")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	if got := directoryCheckByEnv(t, target.DirectoryChecks, "OPENCLARION_M4_EVIDENCE_ROOT").Status; got != "ok" {
		t.Fatalf("evidence root status = %q, want ok", got)
	}
	if got := evidenceChainCheckByName(t, target.EvidenceChainChecks, "baseline_audit").Status; got != "ok" {
		t.Fatalf("baseline status = %q, want ok", got)
	}
	if got := evidenceChainCheckByName(t, target.EvidenceChainChecks, "quality_manifest").Status; got != "missing" {
		t.Fatalf("quality manifest status = %q, want missing", got)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked evidence root: %s", stdout.String())
	}
}

func TestRunReportsReadyM4EvidenceChainWithDigests(t *testing.T) {
	withM4PacketVerifier(t, func(_ string) error {
		return nil
	})
	root := t.TempDir()
	writeM4EvidenceChainRuntimeArtifacts(t, root)
	writeQualitySamplePair(t, root, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, root, "cascade", "checkout-latency")
	writeQualitySamplePair(t, root, "alert_storm", "billing-errors")
	for _, name := range []string{
		"baseline-audit.json",
		"quality-manifest.json",
		"quality-comparison.json",
		"review-evidence.json",
		"packet.json",
	} {
		writeFile(t, root, name)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-chain"}, []string{
		"OPENCLARION_M4_EVIDENCE_ROOT=" + root,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-evidence-chain")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	check := evidenceChainCheckByName(t, target.EvidenceChainChecks, "packet_summary")
	if check.Status != "ok" || !lowerHexDigest(check.SHA256) {
		t.Fatalf("packet check = %#v, want ok with sha256", check)
	}
	semantic := evidenceChainCheckByName(t, target.EvidenceChainChecks, "packet_semantic_verification")
	if semantic.Status != "ok" || semantic.SHA256 != "" {
		t.Fatalf("semantic check = %#v, want ok without sha256", semantic)
	}
	direct := evidenceChainCheckByName(t, target.EvidenceChainChecks, "direct_quality_samples")
	if direct.Status != "ok" || direct.SHA256 != "" {
		t.Fatalf("direct sample check = %#v, want directory ok without sha256", direct)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked evidence root: %s", stdout.String())
	}
}

func TestRunReportsReadyM4PacketEvidenceChainWithSemanticVerification(t *testing.T) {
	var verifiedRoot string
	withM4PacketVerifier(t, func(root string) error {
		verifiedRoot = root
		return nil
	})
	root := t.TempDir()
	writeM4PacketEvidenceChainArtifacts(t, root)

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-chain"}, []string{
		"OPENCLARION_M4_EVIDENCE_ROOT=" + root,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-evidence-chain")
	if target.Status != "ready" {
		t.Fatalf("target status = %q, want ready", target.Status)
	}
	if verifiedRoot != filepath.Clean(root) {
		t.Fatalf("verified root = %q, want cleaned packet root", verifiedRoot)
	}
	qualityManifest := evidenceChainCheckByName(t, target.EvidenceChainChecks, "quality_manifest")
	if qualityManifest.Status != "ok" || qualityManifest.Artifact != "quality-inputs/quality-manifest.json" || !lowerHexDigest(qualityManifest.SHA256) {
		t.Fatalf("quality manifest check = %#v, want packet-local JSON with sha256", qualityManifest)
	}
	qualityReports := evidenceChainCheckByName(t, target.EvidenceChainChecks, "quality_reports")
	if qualityReports.Status != "ok" || qualityReports.Artifact != "quality-inputs/reports" || qualityReports.SHA256 != "" {
		t.Fatalf("quality reports check = %#v, want packet-local directory ok without sha256", qualityReports)
	}
	runtimeArtifacts := evidenceChainCheckByName(t, target.EvidenceChainChecks, "runtime_smoke_artifacts")
	if runtimeArtifacts.Status != "ok" || runtimeArtifacts.Artifact != "runtime-smoke-artifacts" || runtimeArtifacts.SHA256 != "" {
		t.Fatalf("runtime artifacts check = %#v, want packet-local directory ok without sha256", runtimeArtifacts)
	}
	semantic := evidenceChainCheckByName(t, target.EvidenceChainChecks, "packet_semantic_verification")
	if semantic.Status != "ok" || semantic.SHA256 != "" {
		t.Fatalf("semantic check = %#v, want ok without sha256", semantic)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked evidence root: %s", stdout.String())
	}
}

func TestRunRejectsM4EvidenceChainWhenPacketVerifierFails(t *testing.T) {
	withM4PacketVerifier(t, func(_ string) error {
		return os.ErrInvalid
	})
	root := t.TempDir()
	writeM4EvidenceChainRuntimeArtifacts(t, root)
	writeQualitySamplePair(t, root, "single_alert", "payments-cpu")
	writeQualitySamplePair(t, root, "cascade", "checkout-latency")
	writeQualitySamplePair(t, root, "alert_storm", "billing-errors")
	for _, name := range []string{
		"baseline-audit.json",
		"quality-manifest.json",
		"quality-comparison.json",
		"review-evidence.json",
		"packet.json",
	} {
		writeFile(t, root, name)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-chain"}, []string{
		"OPENCLARION_M4_EVIDENCE_ROOT=" + root,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-evidence-chain")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	semantic := evidenceChainCheckByName(t, target.EvidenceChainChecks, "packet_semantic_verification")
	if semantic.Status != "invalid_packet" || semantic.Reason != "invalid argument" {
		t.Fatalf("semantic check = %#v, want invalid_packet with sanitized reason", semantic)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked evidence root: %s", stdout.String())
	}
}

func TestRunRejectsM4PacketEvidenceChainWhenVerifierFails(t *testing.T) {
	withM4PacketVerifier(t, func(_ string) error {
		return os.ErrInvalid
	})
	root := t.TempDir()
	writeM4PacketEvidenceChainArtifacts(t, root)

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-chain"}, []string{
		"OPENCLARION_M4_EVIDENCE_ROOT=" + root,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-evidence-chain")
	if target.Status != "blocked" {
		t.Fatalf("target status = %q, want blocked", target.Status)
	}
	semantic := evidenceChainCheckByName(t, target.EvidenceChainChecks, "packet_semantic_verification")
	if semantic.Status != "invalid_packet" || semantic.Reason != "invalid argument" {
		t.Fatalf("semantic check = %#v, want invalid_packet with sanitized reason", semantic)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("output leaked evidence root: %s", stdout.String())
	}
}

func TestRunRejectsM4EvidenceChainDuplicateJSON(t *testing.T) {
	root := t.TempDir()
	writeM4EvidenceChainRuntimeArtifacts(t, root)
	writeFileBody(t, root, "baseline-audit.json", `{"status":"pass","status":"fail"}`+"\n")

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-chain"}, []string{
		"OPENCLARION_M4_EVIDENCE_ROOT=" + root,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	target := targetByName(t, out, "sandbox-m4-evidence-chain")
	check := evidenceChainCheckByName(t, target.EvidenceChainChecks, "baseline_audit")
	if check.Status != "invalid_json" || check.SHA256 != "" {
		t.Fatalf("baseline check = %#v, want invalid_json without digest", check)
	}
	if strings.Contains(stdout.String(), root) || strings.Contains(stdout.String(), "fail") {
		t.Fatalf("output leaked evidence root or JSON value: %s", stdout.String())
	}
}

func TestRunRejectsIndirectOrReusedM4EvidencePaths(t *testing.T) {
	root := t.TempDir()
	target := writeFile(t, root, "target.json")
	link := filepath.Join(root, "linked.json")
	createSymlinkOrSkip(t, target, link)
	outDir := filepath.Join(root, "packet")
	if err := os.Mkdir(outDir, 0o700); err != nil {
		t.Fatalf("mkdir out dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "old.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write old packet file: %v", err)
	}

	var stdout bytes.Buffer
	err := run([]string{"--target", "sandbox-m4-evidence-packet"}, []string{
		"QUALITY_MANIFEST=" + link,
		"REVIEW_EVIDENCE=" + target,
		"OUT_DIR=" + outDir,
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	packet := targetByName(t, out, "sandbox-m4-evidence-packet")
	if packet.Status != "blocked" {
		t.Fatalf("packet status = %q, want blocked", packet.Status)
	}
	if got := packet.FileChecks[0].Status; got != "not_regular" {
		t.Fatalf("QUALITY_MANIFEST status = %q, want not_regular", got)
	}
	if got := packet.DirectoryChecks[0].Status; got != "not_empty" {
		t.Fatalf("OUT_DIR status = %q, want not_empty", got)
	}
}

func TestRunRejectsBadDiagnosisSnapshotID(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "diagnosis-live-browser-smoke"}, []string{
		"OPENCLARION_LIVE_API_BASE_URL=https://api.example.test",
		"OPENCLARION_LIVE_BEARER_TOKEN=Bearer secret-token",
		"OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID=007",
	}, &stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	out := decodeOutput(t, stdout.Bytes())
	diagnosis := targetByName(t, out, "diagnosis-live-browser-smoke")
	if diagnosis.Status != "blocked" {
		t.Fatalf("diagnosis status = %q, want blocked", diagnosis.Status)
	}
	if len(diagnosis.InvalidEnv) != 1 || diagnosis.InvalidEnv[0].Name != "OPENCLARION_LIVE_EVIDENCE_SNAPSHOT_ID" {
		t.Fatalf("invalid env = %#v, want snapshot id rejection", diagnosis.InvalidEnv)
	}
}

func TestRunRejectsUnknownTarget(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"--target", "unknown"}, nil, &stdout)
	if err == nil {
		t.Fatal("run err = nil, want unknown target error")
	}
	if !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("run err = %v, want unknown target", err)
	}
}

func decodeOutput(t *testing.T, raw []byte) readinessOutput {
	t.Helper()
	var out readinessOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, raw)
	}
	return out
}

func targetByName(t *testing.T, out readinessOutput, name string) targetReadiness {
	t.Helper()
	for _, target := range out.Targets {
		if target.Name == name {
			return target
		}
	}
	t.Fatalf("target %q not found in %#v", name, out.Targets)
	return targetReadiness{}
}

func hasTarget(out readinessOutput, name string) bool {
	for _, target := range out.Targets {
		if target.Name == name {
			return true
		}
	}
	return false
}

func targetNames(targets []targetReadiness) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Name)
	}
	return names
}

func directoryCheckByEnv(t *testing.T, checks []directoryCheck, name string) directoryCheck {
	t.Helper()
	for _, check := range checks {
		if check.Env == name {
			return check
		}
	}
	t.Fatalf("directory check %q not found in %#v", name, checks)
	return directoryCheck{}
}

func fileCheckByEnv(t *testing.T, checks []fileCheck, name string) fileCheck {
	t.Helper()
	for _, check := range checks {
		if check.Env == name {
			return check
		}
	}
	t.Fatalf("file check %q not found in %#v", name, checks)
	return fileCheck{}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func invalidEnvByName(values []invalidEnv, want string) bool {
	for _, value := range values {
		if value.Name == want {
			return true
		}
	}
	return false
}

func missingEnvByName(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func pastReportWindow() (string, string) {
	end := time.Now().UTC().Add(-1 * time.Hour).Truncate(time.Second)
	start := end.Add(-1 * time.Hour)
	return start.Format(time.RFC3339), end.Format(time.RFC3339)
}

func writeFile(t *testing.T, dir, name string) string {
	t.Helper()
	return writeFileBody(t, dir, name, "{}\n")
}

func writeFileBody(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func writeQualitySamplePair(t *testing.T, root, scenario, id string) {
	t.Helper()
	writeQualitySampleReport(t, root, directRole, scenario, id)
	writeQualitySampleReport(t, root, sandboxRole, scenario, id)
}

func writeQualitySampleReport(t *testing.T, root, role, scenario, id string) {
	t.Helper()
	dir := filepath.Join(root, role, scenario)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir quality sample dir: %v", err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write quality sample report: %v", err)
	}
}

func createSymlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
}

func writeM4EvidenceChainRuntimeArtifacts(t *testing.T, root string) {
	t.Helper()
	writeFileBody(t, root, "runtime-smokes/digest-ref.txt", "registry.example.com/openclarion/runtime-candidate-a@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n")
	for _, name := range []string{
		"agent-runtime-smoke.json",
		"container-provider-smoke.json",
		"container-provider-timeout-smoke.json",
		"container-provider-output-cap-smoke.json",
		"egress-allowdeny-smoke.json",
	} {
		writeFileBody(t, root, filepath.Join("runtime-smokes", name), `{"status":"pass"}`+"\n")
	}
}

func writeM4PacketEvidenceChainArtifacts(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{
		"baseline-audit.json",
		"quality-comparison.json",
		"review-evidence.json",
		"decision.json",
	} {
		writeFile(t, root, name)
	}
	writeFileBody(t, root, "packet.json", `{"tool":"sandbox_m4_evidence_packet"}`+"\n")
	writeFile(t, root, "quality-inputs/quality-manifest.json")
	writeFile(t, root, "quality-inputs/reports/direct/single_alert/payments-cpu.json")
	writeFile(t, root, "quality-inputs/reports/sandbox/single_alert/payments-cpu.json")
	writeFile(t, root, "runtime-smoke-artifacts/agent-runtime-smoke.json")
}

func evidenceChainCheckByName(t *testing.T, checks []evidenceChainCheck, name string) evidenceChainCheck {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("evidence chain check %q not found in %#v", name, checks)
	return evidenceChainCheck{}
}

func lowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func testReadinessWeComWebhookURL(key string) string {
	return "https://" + readinessWeComWebhookHost + readinessWeComWebhookPath + "?key=" + key
}

func withM4PacketVerifier(t *testing.T, verifier func(string) error) {
	t.Helper()
	previous := verifyM4EvidencePacket
	verifyM4EvidencePacket = verifier
	t.Cleanup(func() {
		verifyM4EvidencePacket = previous
	})
}
