// Command diagnosis_auth_env_check validates local diagnosis auth provider
// environment shape without contacting external identity providers.
package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	authldap "github.com/openclarion/openclarion/internal/providers/auth/ldap"
	authstatic "github.com/openclarion/openclarion/internal/providers/auth/static"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisauth"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	diagnosisAuthModeEnv = "OPENCLARION_DIAGNOSIS_AUTH_MODE"

	diagnosisLDAPURLEnv            = "OPENCLARION_DIAGNOSIS_LDAP_URL"
	diagnosisLDAPBaseDNEnv         = "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN"
	diagnosisLDAPBindDNEnv         = "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN"
	diagnosisLDAPBindPasswordEnv   = "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD" // #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisLDAPUserFilterEnv     = "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER"
	diagnosisLDAPSubjectAttrEnv    = "OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE"
	diagnosisLDAPRoleAttrEnv       = "OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE"
	diagnosisLDAPOwnerRolesEnv     = "OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES"
	diagnosisLDAPAdminRolesEnv     = "OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES"
	diagnosisLDAPDefaultRolesEnv   = "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES"
	diagnosisLDAPStartTLSEnv       = "OPENCLARION_DIAGNOSIS_LDAP_START_TLS"
	diagnosisLDAPAllowPlaintextEnv = "OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT"

	diagnosisStaticBearerTokenEnv = "OPENCLARION_DIAGNOSIS_STATIC_BEARER_TOKEN" // #nosec G101 -- environment variable name only; values are read at runtime.
	diagnosisStaticSubjectEnv     = "OPENCLARION_DIAGNOSIS_STATIC_SUBJECT"
	diagnosisStaticRolesEnv       = "OPENCLARION_DIAGNOSIS_STATIC_ROLES"

	diagnosisOIDCIssuerURLEnv       = "OPENCLARION_DIAGNOSIS_OIDC_ISSUER_URL"
	diagnosisOIDCClientIDEnv        = "OPENCLARION_DIAGNOSIS_OIDC_CLIENT_ID"
	diagnosisOIDCRoleClaimEnv       = "OPENCLARION_DIAGNOSIS_OIDC_ROLE_CLAIM"
	diagnosisOIDCOwnerRolesEnv      = "OPENCLARION_DIAGNOSIS_OIDC_OWNER_ROLES"
	diagnosisOIDCAdminRolesEnv      = "OPENCLARION_DIAGNOSIS_OIDC_ADMIN_ROLES"
	diagnosisOIDCSigningAlgsEnv     = "OPENCLARION_DIAGNOSIS_OIDC_SIGNING_ALGS"
	iamOIDCIssuerEnv                = "OPENCLARION_IAM_OIDC_ISSUER"
	iamOIDCClientIDEnv              = "OPENCLARION_IAM_OIDC_CLIENT_ID"
	iamOIDCClientSecretEnv          = "OPENCLARION_IAM_OIDC_CLIENT_SECRET" // #nosec G101 -- environment variable name only; values are read at runtime.
	iamOIDCClientAuthMethodEnv      = "OPENCLARION_IAM_OIDC_CLIENT_AUTH_METHOD"
	iamOIDCRedirectURLEnv           = "OPENCLARION_IAM_OIDC_REDIRECT_URL" // #nosec G101 -- environment variable name only; values are read at runtime.
	iamOIDCScopesEnv                = "OPENCLARION_IAM_OIDC_SCOPES"
	iamOIDCUsePKCEEnv               = "OPENCLARION_IAM_OIDC_USE_PKCE"
	iamOIDCStateSigningKeyEnv       = "OPENCLARION_IAM_OIDC_STATE_SIGNING_KEY" // #nosec G101 -- environment variable name only; values are read at runtime.
	standardOIDCIssuerEnv           = "OIDC_ISSUER"
	standardOIDCClientIDEnv         = "OIDC_CLIENT_ID"
	standardOIDCClientSecretEnv     = "OIDC_CLIENT_SECRET" // #nosec G101 -- environment variable name only; values are read at runtime.
	standardOIDCClientAuthMethodEnv = "OIDC_CLIENT_AUTH_METHOD"
	standardOIDCRedirectURLEnv      = "OIDC_REDIRECT_URL" // #nosec G101 -- environment variable name only; values are read at runtime.
	standardOIDCScopesEnv           = "OIDC_SCOPES"
	standardOIDCUsePKCEEnv          = "OIDC_USE_PKCE"
	standardOIDCStateSigningKeyEnv  = "OIDC_STATE_SIGNING_KEY" // #nosec G101 -- environment variable name only; values are read at runtime.

	iamDirectoryProviderNameEnv = "OPENCLARION_IAM_DIRECTORY_PROVIDER_NAME"
	iamDirectoryIssuerEnv       = "OPENCLARION_IAM_DIRECTORY_ISSUER"
	iamDirectoryBaseURLEnv      = "OPENCLARION_IAM_DIRECTORY_BASE_URL"
	iamDirectoryTokenURLEnv     = "OPENCLARION_IAM_DIRECTORY_TOKEN_URL" // #nosec G101 -- environment variable name only; values are read at runtime.
	iamDirectoryClientIDEnv     = "OPENCLARION_IAM_DIRECTORY_CLIENT_ID"
	iamDirectoryClientSecretEnv = "OPENCLARION_IAM_DIRECTORY_CLIENT_SECRET" // #nosec G101 -- environment variable name only; values are read at runtime.
	iamDirectoryScopesEnv       = "OPENCLARION_IAM_DIRECTORY_SCOPES"
	standardDirectoryScopesEnv  = "DIRECTORY_SCOPES"

	diagnosisSessionSigningKeyEnv = "OPENCLARION_DIAGNOSIS_SESSION_SIGNING_KEY"
)

type getenvFunc func(string) string

func main() {
	if err := check(os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "[diagnosis-auth-env-check] %v\n", err)
		os.Exit(1)
	}
}

func check(getenv getenvFunc) error {
	mode := strings.ToLower(strings.TrimSpace(getenv(diagnosisAuthModeEnv)))
	if mode == "" {
		mode = inferDiagnosisAuthMode(getenv)
		if mode == "" {
			return validateOptionalIAMIntegration(getenv)
		}
	}

	if err := validatePrimaryAuthMode(mode, getenv); err != nil {
		return err
	}
	return validateOptionalIAMIntegration(getenv)
}

func inferDiagnosisAuthMode(getenv getenvFunc) string {
	switch {
	case oidcAuthConfigured(getenv):
		return "oidc"
	case staticAuthConfigured(getenv):
		return "static"
	default:
		return ""
	}
}

func validatePrimaryAuthMode(mode string, getenv getenvFunc) error {
	switch mode {
	case "ldap":
		return validateLDAPAuth(getenv)
	case "static":
		return validateStaticAuth(getenv)
	case "oidc":
		return validateOIDCAuth(getenv)
	case "wecom":
		return fmt.Errorf("%s=wecom is no longer supported; use oidc for IAM browser login and Enterprise WeChat only for app-message callbacks", diagnosisAuthModeEnv)
	default:
		return fmt.Errorf("%s must be ldap, static, or oidc", diagnosisAuthModeEnv)
	}
}

func validateLDAPAuth(getenv getenvFunc) error {
	defaultRoles, err := authRolesFromCSV(getenv(diagnosisLDAPDefaultRolesEnv), false, diagnosisLDAPDefaultRolesEnv)
	if err != nil {
		return err
	}
	startTLS, err := boolFromEnv(getenv, diagnosisLDAPStartTLSEnv)
	if err != nil {
		return err
	}
	allowPlaintext, err := boolFromEnv(getenv, diagnosisLDAPAllowPlaintextEnv)
	if err != nil {
		return err
	}
	if _, err := authldap.NewProvider(authldap.Config{
		URL:                    getenv(diagnosisLDAPURLEnv),
		BaseDN:                 getenv(diagnosisLDAPBaseDNEnv),
		BindDN:                 getenv(diagnosisLDAPBindDNEnv),
		BindPassword:           getenv(diagnosisLDAPBindPasswordEnv),
		UserFilter:             getenv(diagnosisLDAPUserFilterEnv),
		SubjectAttribute:       getenv(diagnosisLDAPSubjectAttrEnv),
		RoleAttribute:          getenv(diagnosisLDAPRoleAttrEnv),
		OwnerRoleValues:        csvValues(getenv(diagnosisLDAPOwnerRolesEnv)),
		AdminRoleValues:        csvValues(getenv(diagnosisLDAPAdminRolesEnv)),
		DefaultRoles:           defaultRoles,
		StartTLS:               startTLS,
		AllowInsecurePlaintext: allowPlaintext,
	}); err != nil {
		return fmt.Errorf("configure diagnosis LDAP auth provider: %w", err)
	}
	return nil
}

func validateStaticAuth(getenv getenvFunc) error {
	roles, err := authRolesFromCSV(getenv(diagnosisStaticRolesEnv), true, diagnosisStaticRolesEnv)
	if err != nil {
		return err
	}
	if _, err := authstatic.NewProvider(authstatic.Config{
		Token:   strings.TrimSpace(getenv(diagnosisStaticBearerTokenEnv)),
		Subject: strings.TrimSpace(getenv(diagnosisStaticSubjectEnv)),
		Roles:   roles,
	}); err != nil {
		return fmt.Errorf("configure diagnosis static auth provider: %w", err)
	}
	return nil
}

func validateOIDCAuth(getenv getenvFunc) error {
	issuer := firstNonEmptyEnv(getenv, iamOIDCIssuerEnv, standardOIDCIssuerEnv, diagnosisOIDCIssuerURLEnv)
	clientID := firstNonEmptyEnv(getenv, iamOIDCClientIDEnv, standardOIDCClientIDEnv, diagnosisOIDCClientIDEnv)
	if issuer == "" {
		return fmt.Errorf("diagnosis OIDC issuer url must be configured")
	}
	if clientID == "" {
		return fmt.Errorf("diagnosis OIDC client id must be configured")
	}
	parsed, err := url.Parse(issuer)
	if err != nil {
		return fmt.Errorf("diagnosis OIDC issuer url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("diagnosis OIDC issuer url scheme must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("diagnosis OIDC issuer url must be absolute")
	}
	if parsed.User != nil {
		return fmt.Errorf("diagnosis OIDC issuer url must not include userinfo")
	}
	if strings.ContainsAny(clientID, "\x00\r\n\t ") {
		return fmt.Errorf("diagnosis OIDC client id must not contain whitespace")
	}
	return nil
}

func validateOptionalIAMIntegration(getenv getenvFunc) error {
	if err := validateOptionalOIDCBrowserBFF(getenv); err != nil {
		return err
	}
	return validateOptionalIAMDirectorySync(getenv)
}

func validateOptionalOIDCBrowserBFF(getenv getenvFunc) error {
	if !oidcBrowserBFFConfigured(getenv) {
		return nil
	}
	issuer := firstNonEmptyEnv(getenv, iamOIDCIssuerEnv, standardOIDCIssuerEnv, diagnosisOIDCIssuerURLEnv)
	clientID := firstNonEmptyEnv(getenv, iamOIDCClientIDEnv, standardOIDCClientIDEnv, diagnosisOIDCClientIDEnv)
	if issuer == "" {
		return fmt.Errorf("OIDC browser login issuer url must be configured")
	}
	if clientID == "" {
		return fmt.Errorf("OIDC browser login client id must be configured")
	}
	if err := validateHTTPURL(issuer, "OIDC browser login issuer url"); err != nil {
		return err
	}
	if strings.ContainsAny(clientID, "\x00\r\n\t ") {
		return fmt.Errorf("OIDC browser login client id must not contain whitespace")
	}
	if redirectURL := firstNonEmptyEnv(getenv, iamOIDCRedirectURLEnv, standardOIDCRedirectURLEnv); redirectURL != "" {
		if err := validateHTTPURL(redirectURL, "OIDC browser login redirect url"); err != nil {
			return err
		}
	}
	stateSigningKey := firstConfiguredEnv(getenv, iamOIDCStateSigningKeyEnv, standardOIDCStateSigningKeyEnv, diagnosisSessionSigningKeyEnv)
	if stateSigningKey == "" {
		return fmt.Errorf("%s, %s, or %s is required for OIDC browser login state sealing", iamOIDCStateSigningKeyEnv, standardOIDCStateSigningKeyEnv, diagnosisSessionSigningKeyEnv)
	}
	if err := validateSigningKey(stateSigningKey, "OIDC browser login state signing key"); err != nil {
		return err
	}
	if strings.TrimSpace(getenv(diagnosisSessionSigningKeyEnv)) == "" {
		return fmt.Errorf("%s is required for OIDC browser login session issuance", diagnosisSessionSigningKeyEnv)
	}
	if err := validateSigningKey(getenv(diagnosisSessionSigningKeyEnv), "diagnosis session signing key"); err != nil {
		return err
	}
	clientSecret := firstConfiguredEnv(getenv, iamOIDCClientSecretEnv, standardOIDCClientSecretEnv)
	if clientSecret != "" {
		if err := validateSecretValue(clientSecret, "OIDC browser login client secret"); err != nil {
			return err
		}
	}
	usePKCE := true
	if raw := firstNonEmptyEnv(getenv, iamOIDCUsePKCEEnv, standardOIDCUsePKCEEnv); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("OIDC browser login PKCE flag must be true or false")
		}
		usePKCE = value
	}
	authMethod := firstNonEmptyEnv(getenv, iamOIDCClientAuthMethodEnv, standardOIDCClientAuthMethodEnv)
	switch authMethod {
	case "":
	case "client_secret_basic", "client_secret_post":
		if clientSecret == "" {
			return fmt.Errorf("OIDC browser login client secret is required for %s", authMethod)
		}
	case "none":
		if clientSecret != "" {
			return fmt.Errorf("OIDC browser login client secret must be unset when client auth method is none")
		}
		if !usePKCE {
			return fmt.Errorf("OIDC browser login PKCE must be enabled when client auth method is none")
		}
	default:
		return fmt.Errorf("OIDC browser login client auth method must be client_secret_basic, client_secret_post, or none")
	}
	if scopes := firstNonEmptyEnv(getenv, iamOIDCScopesEnv, standardOIDCScopesEnv); scopes != "" && !scopeValues(scopes)["openid"] {
		return fmt.Errorf("OIDC browser login scopes must include openid")
	}
	return nil
}

func validateOptionalIAMDirectorySync(getenv getenvFunc) error {
	if !iamDirectorySyncConfigured(getenv) {
		return nil
	}
	issuer := firstNonEmptyEnv(getenv, iamDirectoryIssuerEnv, iamOIDCIssuerEnv, standardOIDCIssuerEnv)
	clientID := firstNonEmptyEnv(getenv, iamDirectoryClientIDEnv, iamOIDCClientIDEnv, standardOIDCClientIDEnv)
	clientSecret := firstConfiguredEnv(getenv, iamDirectoryClientSecretEnv, iamOIDCClientSecretEnv, standardOIDCClientSecretEnv)
	if issuer == "" {
		return fmt.Errorf("%s, %s, or %s is required when IAM directory sync is configured", iamDirectoryIssuerEnv, iamOIDCIssuerEnv, standardOIDCIssuerEnv)
	}
	if clientID == "" {
		return fmt.Errorf("%s, %s, or %s is required when IAM directory sync is configured", iamDirectoryClientIDEnv, iamOIDCClientIDEnv, standardOIDCClientIDEnv)
	}
	if clientSecret == "" {
		return fmt.Errorf("%s, %s, or %s is required when IAM directory sync is configured", iamDirectoryClientSecretEnv, iamOIDCClientSecretEnv, standardOIDCClientSecretEnv)
	}
	if err := validateHTTPURL(issuer, "IAM directory issuer url"); err != nil {
		return err
	}
	if strings.ContainsAny(clientID, "\x00\r\n\t ") {
		return fmt.Errorf("IAM directory client id must not contain whitespace")
	}
	if err := validateSecretValue(clientSecret, "IAM directory client secret"); err != nil {
		return err
	}
	if baseURL := strings.TrimSpace(getenv(iamDirectoryBaseURLEnv)); baseURL != "" {
		if err := validateHTTPURL(baseURL, "IAM directory base url"); err != nil {
			return err
		}
	}
	if tokenURL := strings.TrimSpace(getenv(iamDirectoryTokenURLEnv)); tokenURL != "" {
		if err := validateHTTPURL(tokenURL, "IAM directory token url"); err != nil {
			return err
		}
	}
	scopes := firstNonEmptyEnv(getenv, iamDirectoryScopesEnv, standardDirectoryScopesEnv)
	if scopes != "" && !scopeValues(scopes)["directory:read"] {
		return fmt.Errorf("IAM directory scopes must include directory:read")
	}
	return nil
}

func staticAuthConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv, diagnosisStaticBearerTokenEnv, diagnosisStaticSubjectEnv, diagnosisStaticRolesEnv)
}

func oidcAuthConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv,
		diagnosisOIDCIssuerURLEnv,
		diagnosisOIDCClientIDEnv,
		diagnosisOIDCRoleClaimEnv,
		diagnosisOIDCOwnerRolesEnv,
		diagnosisOIDCAdminRolesEnv,
		diagnosisOIDCSigningAlgsEnv,
		iamOIDCIssuerEnv,
		iamOIDCClientIDEnv,
		standardOIDCIssuerEnv,
		standardOIDCClientIDEnv,
	)
}

func oidcBrowserBFFConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv,
		iamOIDCClientSecretEnv,
		iamOIDCClientAuthMethodEnv,
		iamOIDCRedirectURLEnv,
		iamOIDCScopesEnv,
		iamOIDCUsePKCEEnv,
		iamOIDCStateSigningKeyEnv,
		standardOIDCClientSecretEnv,
		standardOIDCClientAuthMethodEnv,
		standardOIDCRedirectURLEnv,
		standardOIDCScopesEnv,
		standardOIDCUsePKCEEnv,
		standardOIDCStateSigningKeyEnv,
	)
}

func iamDirectorySyncConfigured(getenv getenvFunc) bool {
	return anyEnv(getenv,
		iamDirectoryProviderNameEnv,
		iamDirectoryIssuerEnv,
		iamDirectoryBaseURLEnv,
		iamDirectoryTokenURLEnv,
		iamDirectoryClientIDEnv,
		iamDirectoryClientSecretEnv,
		iamDirectoryScopesEnv,
		standardDirectoryScopesEnv,
	)
}

func anyEnv(getenv getenvFunc, keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(getenv(key)) != "" {
			return true
		}
	}
	return false
}

func firstNonEmptyEnv(getenv getenvFunc, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func firstConfiguredEnv(getenv getenvFunc, keys ...string) string {
	for _, key := range keys {
		value := getenv(key)
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func validateHTTPURL(raw string, label string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid", label)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s scheme must be http or https", label)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must be absolute", label)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not include userinfo", label)
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("%s must not include query", label)
	}
	if parsed.Fragment != "" || parsed.RawFragment != "" {
		return fmt.Errorf("%s must not include fragment", label)
	}
	return nil
}

func validateSigningKey(value string, label string) error {
	if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must not contain leading/trailing whitespace, NUL, CR, or LF", label)
	}
	if len([]byte(value)) < diagnosisauth.MinSessionSigningKeyBytes {
		return fmt.Errorf("%s must be at least %d bytes", label, diagnosisauth.MinSessionSigningKeyBytes)
	}
	return nil
}

func validateSecretValue(value string, label string) error {
	if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s must not contain leading/trailing whitespace, NUL, CR, or LF", label)
	}
	return nil
}

func boolFromEnv(getenv getenvFunc, key string) (bool, error) {
	raw := strings.TrimSpace(getenv(key))
	if raw == "" {
		return false, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func authRolesFromCSV(raw string, required bool, key string) ([]ports.AuthRole, error) {
	values := csvValues(raw)
	if len(values) == 0 {
		if required {
			return nil, fmt.Errorf("%s is required when diagnosis auth roles are enabled", key)
		}
		return nil, nil
	}
	roles := make([]ports.AuthRole, 0, len(values))
	for _, value := range values {
		switch strings.ToLower(value) {
		case string(ports.AuthRoleOwner):
			roles = append(roles, ports.AuthRoleOwner)
		case string(ports.AuthRoleAdmin):
			roles = append(roles, ports.AuthRoleAdmin)
		default:
			return nil, fmt.Errorf("%s contains unsupported role %q", key, value)
		}
	}
	return roles, nil
}

func csvValues(raw string) []string {
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func scopeValues(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out[part] = true
		}
	}
	return out
}
