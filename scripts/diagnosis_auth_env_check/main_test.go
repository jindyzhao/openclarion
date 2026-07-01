package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestCheckAcceptsLDAPConfig(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:           "ldap",
		diagnosisLDAPURLEnv:            "ldaps://ldap.example.test:636",
		diagnosisLDAPBaseDNEnv:         "dc=example,dc=test",
		diagnosisLDAPBindDNEnv:         "cn=openclarion,dc=example,dc=test",
		diagnosisLDAPBindPasswordEnv:   "service-password",
		diagnosisLDAPUserFilterEnv:     "(&(objectClass=person)(uid={username}))",
		diagnosisLDAPSubjectAttrEnv:    "mail",
		diagnosisLDAPRoleAttrEnv:       "memberOf",
		diagnosisLDAPOwnerRolesEnv:     "cn=openclarion-operators,dc=example,dc=test",
		diagnosisLDAPAdminRolesEnv:     "cn=openclarion-admins,dc=example,dc=test",
		diagnosisLDAPDefaultRolesEnv:   "owner",
		diagnosisLDAPAllowPlaintextEnv: "false",
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckRejectsLDAPConfigWithoutLeakingValues(t *testing.T) {
	// #nosec G101 -- test-only credential-bearing URL and placeholder password verify sanitization.
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:         "ldap",
		diagnosisLDAPURLEnv:          "ldap://user:secret@ldap.example.test:389",
		diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
		diagnosisLDAPBindDNEnv:       "cn=openclarion,dc=example,dc=test",
		diagnosisLDAPBindPasswordEnv: "service-password",
		diagnosisLDAPUserFilterEnv:   "(uid=*)",
		diagnosisLDAPDefaultRolesEnv: "owner",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	if !strings.Contains(text, "LDAP") && !strings.Contains(text, "ldap") {
		t.Fatalf("error = %q, want LDAP context", text)
	}
	for _, leaked := range []string{
		"user:secret",
		"ldap.example.test",
		"dc=example,dc=test",
		"service-password",
		"cn=openclarion",
	} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckAcceptsStaticConfig(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:          "static",
		diagnosisStaticBearerTokenEnv: "static-token",
		diagnosisStaticSubjectEnv:     "operator-1",
		diagnosisStaticRolesEnv:       "owner,admin",
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckAcceptsStandardOIDCConfig(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		standardOIDCIssuerEnv:   "https://iam.example.test",
		standardOIDCClientIDEnv: "openclarion-web",
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckAcceptsStandardOIDCBrowserAndDirectoryConfig(t *testing.T) {
	// #nosec G101 -- test-only env fixture uses placeholder secret values.
	err := check(mapGetenv(map[string]string{
		standardOIDCIssuerEnv:           "https://iam.example.test",
		standardOIDCClientIDEnv:         "openclarion-web",
		standardOIDCClientSecretEnv:     "client-secret",
		standardOIDCClientAuthMethodEnv: "client_secret_basic",
		standardOIDCRedirectURLEnv:      "https://openclarion.example.test/api/diagnosis/auth/oidc/callback",
		standardOIDCScopesEnv:           "openid profile email phone",
		standardOIDCUsePKCEEnv:          "true",
		standardOIDCStateSigningKeyEnv:  "unit-test-state-signing-key-32-bytes",
		diagnosisSessionSigningKeyEnv:   "unit-test-session-signing-key-32-bytes",
		standardDirectoryScopesEnv:      "directory:read",
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckRejectsOIDCBrowserConfigWithoutSessionSigningKey(t *testing.T) {
	// #nosec G101 -- test-only env fixture names secret-bearing settings.
	err := check(mapGetenv(map[string]string{
		standardOIDCIssuerEnv:          "https://iam.example.test",
		standardOIDCClientIDEnv:        "openclarion-web",
		standardOIDCRedirectURLEnv:     "https://openclarion.example.test/api/diagnosis/auth/oidc/callback",
		standardOIDCStateSigningKeyEnv: "unit-test-state-signing-key-32-bytes",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	if !strings.Contains(err.Error(), diagnosisSessionSigningKeyEnv) {
		t.Fatalf("error = %q, want session signing key requirement", err)
	}
}

func TestCheckRejectsInvalidOIDCBrowserFlowConfig(t *testing.T) {
	base := map[string]string{
		standardOIDCIssuerEnv:          "https://iam.example.test",
		standardOIDCClientIDEnv:        "openclarion-web",
		standardOIDCStateSigningKeyEnv: "unit-test-state-signing-key-32-bytes",
		diagnosisSessionSigningKeyEnv:  "unit-test-session-signing-key-32-bytes",
	}
	tests := map[string]struct {
		values map[string]string
		want   string
	}{
		"invalid auth method": {
			values: map[string]string{
				standardOIDCClientAuthMethodEnv: "client_secret_jwt",
			},
			want: "client auth method",
		},
		"secret method without secret": {
			values: map[string]string{
				standardOIDCClientAuthMethodEnv: "client_secret_basic",
			},
			want: "client secret",
		},
		"scopes without openid": {
			values: map[string]string{
				standardOIDCScopesEnv: "profile email phone",
			},
			want: "openid",
		},
		"public client without PKCE": {
			values: map[string]string{
				standardOIDCClientAuthMethodEnv: "none",
				standardOIDCUsePKCEEnv:          "false",
			},
			want: "PKCE",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			values := cloneEnvMap(base)
			for key, value := range tc.values {
				values[key] = value
			}
			err := check(mapGetenv(values))
			if err == nil {
				t.Fatal("check succeeded")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckRejectsOIDCBrowserConfigWithoutLeakingValues(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		standardOIDCIssuerEnv:   "https://iam.example.test",
		standardOIDCClientIDEnv: "openclarion-web",
		standardOIDCRedirectURLEnv: (&url.URL{
			Scheme:   "https",
			User:     url.UserPassword("operator", "secret"),
			Host:     "openclarion.example.test",
			Path:     "/api/diagnosis/auth/oidc/callback",
			RawQuery: "code=secret",
		}).String(),
		standardOIDCStateSigningKeyEnv: "unit-test-state-signing-key-32-bytes",
		diagnosisSessionSigningKeyEnv:  "unit-test-session-signing-key-32-bytes",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	for _, leaked := range []string{
		"operator",
		"secret",
		"openclarion.example.test",
		"0123456789abcdef",
	} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckRejectsIAMDirectorySyncWithoutClientSecret(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		standardOIDCIssuerEnv:      "https://iam.example.test",
		standardOIDCClientIDEnv:    "openclarion-web",
		standardDirectoryScopesEnv: "directory:read",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	if !strings.Contains(err.Error(), "client secret") && !strings.Contains(err.Error(), standardOIDCClientSecretEnv) {
		t.Fatalf("error = %q, want directory client secret requirement", err)
	}
}

func TestCheckRejectsIAMDirectorySyncScopesWithoutLeakingValues(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		iamDirectoryIssuerEnv:       "https://iam.example.test",
		iamDirectoryClientIDEnv:     "openclarion-directory",
		iamDirectoryClientSecretEnv: "directory-secret",
		standardDirectoryScopesEnv:  "profile",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	if !strings.Contains(text, "directory:read") {
		t.Fatalf("error = %q, want directory scope requirement", text)
	}
	for _, leaked := range []string{"directory-secret", "iam.example.test", "openclarion-directory"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckPrefersCurrentOIDCConfigOverLegacyDiagnosisAliases(t *testing.T) {
	tests := map[string]map[string]string{
		"IAM": {
			diagnosisOIDCIssuerURLEnv: "https://legacy.example.test",
			diagnosisOIDCClientIDEnv:  "legacy client",
			standardOIDCIssuerEnv:     "https://standard-iam.example.test",
			standardOIDCClientIDEnv:   "standard-openclarion-web",
			iamOIDCIssuerEnv:          "https://iam.example.test",
			iamOIDCClientIDEnv:        "iam-openclarion-web",
		},
		"standard": {
			diagnosisOIDCIssuerURLEnv: "https://legacy.example.test",
			diagnosisOIDCClientIDEnv:  "legacy client",
			standardOIDCIssuerEnv:     "https://standard-iam.example.test",
			standardOIDCClientIDEnv:   "standard-openclarion-web",
		},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			if err := check(mapGetenv(values)); err != nil {
				t.Fatalf("check: %v", err)
			}
		})
	}
}

func TestCheckInfersOIDCFromRolePolicyConfig(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisOIDCRoleClaimEnv: "openclarion_roles",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	if !strings.Contains(err.Error(), "issuer") {
		t.Fatalf("error = %q, want issuer context", err)
	}
}

func TestCheckRejectsOIDCConfigWithoutLeakingValues(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv: "oidc",
		standardOIDCIssuerEnv: (&url.URL{
			Scheme: "https",
			User:   url.UserPassword("operator", "secret"),
			Host:   "iam.example.test",
		}).String(),
		standardOIDCClientIDEnv: "openclarion-web",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	if !strings.Contains(text, "userinfo") {
		t.Fatalf("error = %q, want userinfo rejection", text)
	}
	for _, leaked := range []string{"operator", "secret", "iam.example.test", "openclarion-web"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckRejectsStaticConfig(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:          "static",
		diagnosisStaticBearerTokenEnv: "static token",
		diagnosisStaticSubjectEnv:     " operator-1 ",
		diagnosisStaticRolesEnv:       "owner",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	for _, leaked := range []string{"static token", "operator-1"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckRejectsLegacyWeComAuthModeWithoutLeakingValues(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:          "wecom",
		diagnosisSessionSigningKeyEnv: "unit-test-state-signing-key-32-bytes",
	}))
	if err == nil {
		t.Fatal("check succeeded")
	}
	text := err.Error()
	if !strings.Contains(text, "no longer supported") || !strings.Contains(text, "oidc") {
		t.Fatalf("error = %q, want legacy WeCom auth migration guidance", text)
	}
	for _, leaked := range []string{
		"0123456789abcdef",
	} {
		if strings.Contains(text, leaked) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
}

func TestCheckIgnoresWeComCallbackConfigOutsideExplicitAuthMode(t *testing.T) {
	err := check(mapGetenv(map[string]string{
		diagnosisAuthModeEnv:               "ldap",
		diagnosisLDAPURLEnv:                "ldaps://ldap.example.test:636",
		diagnosisLDAPBaseDNEnv:             "dc=example,dc=test",
		diagnosisLDAPBindDNEnv:             "cn=openclarion,dc=example,dc=test",
		diagnosisLDAPBindPasswordEnv:       "service-password",
		diagnosisLDAPUserFilterEnv:         "(&(objectClass=person)(uid={username}))",
		diagnosisLDAPSubjectAttrEnv:        "mail",
		diagnosisLDAPRoleAttrEnv:           "memberOf",
		diagnosisLDAPOwnerRolesEnv:         "cn=openclarion-operators,dc=example,dc=test",
		diagnosisLDAPAdminRolesEnv:         "cn=openclarion-admins,dc=example,dc=test",
		diagnosisLDAPDefaultRolesEnv:       "owner",
		diagnosisLDAPAllowPlaintextEnv:     "false",
		diagnosisSessionSigningKeyEnv:      "unit-test-state-signing-key-32-bytes",
		"OPENCLARION_WECOM_CORP_ID":        "ww-openclarion",
		"OPENCLARION_WECOM_CALLBACK_TOKEN": "callback-token-1",
	}))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckDoesNotInferLDAPOrWeComCallbackCompatibilityModes(t *testing.T) {
	tests := map[string]map[string]string{
		"LDAP": {
			diagnosisLDAPURLEnv:          "ldap://ldap.example.test:389",
			diagnosisLDAPBaseDNEnv:       "dc=example,dc=test",
			diagnosisLDAPBindDNEnv:       "cn=openclarion,dc=example,dc=test",
			diagnosisLDAPBindPasswordEnv: "service-password",
			diagnosisLDAPUserFilterEnv:   "(uid=*)",
			diagnosisLDAPDefaultRolesEnv: "owner",
		},
		"WeCom callback": {
			diagnosisSessionSigningKeyEnv:                 "unit-test-state-signing-key-32-bytes",
			"OPENCLARION_WECOM_CORP_ID":                   "ww-openclarion",
			"OPENCLARION_WECOM_CALLBACK_TOKEN":            "callback-token-1",
			"OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY": "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG",
			"OPENCLARION_WECOM_CALLBACK_RECEIVE_ID":       "ww-openclarion",
		},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			if err := check(mapGetenv(values)); err != nil {
				t.Fatalf("check: %v", err)
			}
		})
	}
}

func mapGetenv(values map[string]string) getenvFunc {
	return func(key string) string {
		return values[key]
	}
}

func cloneEnvMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
