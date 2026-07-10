package netbox

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/openclarion/openclarion/internal/providers/cmdb/internal/cmdbnorm"
)

func normalizeAPIBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("netbox cmdb: base URL must be valid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("netbox cmdb: base URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("netbox cmdb: base URL host must be non-empty")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("netbox cmdb: base URL must not include userinfo")
	}
	if parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("netbox cmdb: base URL must not include query or fragment")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(path, "/api") {
		path += "/api"
	}
	parsed.Path = path + "/"
	parsed.RawPath = ""
	return parsed, nil
}

func joinAPIEndpoint(apiRoot *url.URL, segments ...string) string {
	copyURL := *apiRoot
	copyURL.Path = strings.TrimRight(copyURL.Path, "/") + "/" + strings.Join(segments, "/") + "/"
	return copyURL.String()
}

func normalizeLookupLabel(raw string) (string, error) {
	value, err := cmdbnorm.NormalizeRequiredString("lookup label", raw)
	if err != nil {
		return "", fmt.Errorf("netbox cmdb: %w", err)
	}
	if !validLabelName(value) {
		return "", fmt.Errorf("netbox cmdb: lookup label contains unsupported characters")
	}
	return value, nil
}

func normalizeLookupFilter(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = "name"
	}
	if value != raw && raw != "" {
		return "", fmt.Errorf("netbox cmdb: lookup filter must not contain leading or trailing whitespace")
	}
	if !validNetBoxFieldName(value) {
		return "", fmt.Errorf("netbox cmdb: lookup filter contains unsupported characters")
	}
	switch value {
	case "brief", "exclude", "fields", "format", "limit", "offset", "omit", "ordering", "start":
		return "", fmt.Errorf("netbox cmdb: lookup filter %q is reserved", value)
	}
	return value, nil
}

func normalizeObjectType(raw ObjectType) (ObjectType, error) {
	value := ObjectType(strings.ToLower(strings.TrimSpace(string(raw))))
	if value == "" {
		value = ObjectTypeAuto
	}
	switch value {
	case ObjectTypeAuto, ObjectTypeDevice, ObjectTypeVirtualMachine:
		return value, nil
	default:
		return "", fmt.Errorf("netbox cmdb: object type must be auto, device, or virtual_machine")
	}
}

func normalizeCustomAttributeFields(in []string) ([]string, error) {
	if len(in) > maxCustomAttributeFields {
		return nil, fmt.Errorf("netbox cmdb: attribute custom fields exceed %d entries", maxCustomAttributeFields)
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	for i, field := range out {
		if field != strings.TrimSpace(field) || !validNetBoxFieldName(field) {
			return nil, fmt.Errorf("netbox cmdb: attribute custom field %q is invalid", field)
		}
		if i > 0 && field == out[i-1] {
			return nil, fmt.Errorf("netbox cmdb: attribute custom field %q is duplicated", field)
		}
	}
	return out, nil
}

func authorizationHeader(rawToken string, rawScheme TokenScheme) (string, error) {
	token := strings.TrimSpace(rawToken)
	if token != rawToken {
		return "", fmt.Errorf("netbox cmdb: API token must not contain leading or trailing whitespace")
	}
	if len(token) > maxTokenBytes {
		return "", fmt.Errorf("netbox cmdb: API token exceeds %d bytes", maxTokenBytes)
	}
	for _, r := range token {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("netbox cmdb: API token must not contain whitespace or control characters")
		}
	}
	scheme := TokenScheme(strings.ToLower(strings.TrimSpace(string(rawScheme))))
	if scheme == "" {
		scheme = TokenSchemeAuto
	}
	if token == "" {
		if scheme != TokenSchemeAuto {
			return "", fmt.Errorf("netbox cmdb: token scheme requires an API token")
		}
		return "", nil
	}
	if scheme == TokenSchemeAuto {
		if strings.HasPrefix(token, "nbt_") {
			scheme = TokenSchemeBearer
		} else {
			scheme = TokenSchemeToken
		}
	}
	switch scheme {
	case TokenSchemeBearer:
		return "Bearer " + token, nil
	case TokenSchemeToken:
		return "Token " + token, nil
	default:
		return "", fmt.Errorf("netbox cmdb: token scheme must be auto, bearer, or token")
	}
}

func validLabelName(value string) bool {
	for i, r := range value {
		if unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r)) || (i > 0 && (r == '.' || r == '-')) {
			continue
		}
		return false
	}
	return value != ""
}

func validNetBoxFieldName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func withoutRedirects(client *http.Client) *http.Client {
	copyClient := *client
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &copyClient
}
