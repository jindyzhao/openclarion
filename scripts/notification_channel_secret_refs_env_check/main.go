// Command notification_channel_secret_refs_env_check validates notification
// channel secret-ref environment shape without printing secret values.
package main

import (
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/openclarion/openclarion/internal/providers/secrets/envmap"
	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	notificationSecretRefsEnv      = "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON"  // #nosec G101 -- environment variable name only; values are read at runtime.
	notificationWeComSecretRefsEnv = "OPENCLARION_NOTIFICATION_CHANNEL_WECOM_SECRET_REFS" // #nosec G101 -- environment variable name only; values are read at runtime.

	weComWebhookHost = "qyapi.weixin.qq.com"
	weComWebhookPath = "/cgi-bin/webhook/send"
)

type getenvFunc func(string) string

func main() {
	if err := check(os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "[notification-channel-secret-refs-env-check] %v\n", err)
		os.Exit(1)
	}
}

func check(getenv getenvFunc) error {
	secrets, err := secretRefsFromEnv(getenv(notificationSecretRefsEnv))
	if err != nil {
		return err
	}
	if len(secrets) == 0 {
		return nil
	}
	requiredRefs, err := weComRequiredSecretRefs(getenv(notificationWeComSecretRefsEnv), secrets)
	if err != nil {
		return err
	}
	for _, ref := range requiredRefs {
		value, ok := secrets[ref]
		if !ok {
			return fmt.Errorf("%s lists a WeCom notification secret reference that is not present in %s", notificationWeComSecretRefsEnv, notificationSecretRefsEnv)
		}
		if !validWeComWebhookEndpoint(value) {
			return fmt.Errorf("%s contains a WeCom notification secret reference that must resolve to an Enterprise WeChat group robot webhook endpoint", notificationSecretRefsEnv)
		}
	}
	return nil
}

func secretRefsFromEnv(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if _, err := envmap.NewResolverFromJSON(raw); err != nil {
		return nil, fmt.Errorf("%s must be a strict JSON object with non-empty single-token string keys and values", notificationSecretRefsEnv)
	}
	var secrets map[string]string
	if err := strictjson.Unmarshal([]byte(raw), &secrets); err != nil {
		return nil, fmt.Errorf("%s must be a strict JSON object with non-empty single-token string keys and values", notificationSecretRefsEnv)
	}
	return secrets, nil
}

func weComRequiredSecretRefs(raw string, secrets map[string]string) ([]string, error) {
	values := csvValues(raw)
	refs := make([]string, 0, len(values))
	for _, value := range values {
		if strings.ContainsFunc(value, unicode.IsSpace) || strings.ContainsFunc(value, unicode.IsControl) {
			return nil, fmt.Errorf("%s must contain comma-separated secret references without whitespace or control characters", notificationWeComSecretRefsEnv)
		}
		if !slices.Contains(refs, value) {
			refs = append(refs, value)
		}
	}
	for ref := range secrets {
		if strings.Contains(strings.ToLower(ref), "wecom") && !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	slices.Sort(refs)
	return refs, nil
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

func validWeComWebhookEndpoint(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if !strings.EqualFold(parsed.Hostname(), weComWebhookHost) ||
		!strings.EqualFold(parsed.EscapedPath(), weComWebhookPath) {
		return false
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	keys, ok := values["key"]
	if !ok || len(values) != 1 || len(keys) != 1 {
		return false
	}
	key := keys[0]
	return key != "" && !strings.ContainsFunc(key, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	})
}
