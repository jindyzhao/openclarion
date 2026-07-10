package netbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const (
	minSupportedAPIMajor = 4
	minSupportedAPIMinor = 5
	minSupportedAPIPatch = 2
)

type pageResponse[T any] struct {
	Count   *int `json:"count"`
	Results []T  `json:"results"`
}

func decodePage[T any](raw []byte, maxResults int) ([]T, error) {
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return nil, fmt.Errorf("response is not strict JSON: %w", err)
	}
	var page pageResponse[T]
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&page); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("response has trailing JSON values")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode response trailer: %w", err)
	}
	if page.Count == nil {
		return nil, fmt.Errorf("response count must be a non-null integer")
	}
	if *page.Count < 0 {
		return nil, fmt.Errorf("response count must be non-negative")
	}
	if *page.Count > maxResults || len(page.Results) > maxResults {
		return nil, fmt.Errorf("response matched more than %d result(s)", maxResults)
	}
	if *page.Count != len(page.Results) {
		return nil, fmt.Errorf("response count does not match returned results")
	}
	return page.Results, nil
}

type objectReference struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Display string `json:"display"`
}

type ipAddressReference struct {
	Address string `json:"address"`
}

type tagReference struct {
	Name string `json:"name"`
}

type deviceResponse struct {
	ID           int64                      `json:"id"`
	Name         string                     `json:"name"`
	Display      string                     `json:"display"`
	Role         *objectReference           `json:"role"`
	Tenant       *objectReference           `json:"tenant"`
	Platform     *objectReference           `json:"platform"`
	Site         *objectReference           `json:"site"`
	Location     *objectReference           `json:"location"`
	Rack         *objectReference           `json:"rack"`
	Cluster      *objectReference           `json:"cluster"`
	Status       json.RawMessage            `json:"status"`
	PrimaryIP    *ipAddressReference        `json:"primary_ip"`
	Tags         []tagReference             `json:"tags"`
	CustomFields map[string]json.RawMessage `json:"custom_fields"`
}

type virtualMachineResponse struct {
	ID           int64                      `json:"id"`
	Name         string                     `json:"name"`
	Display      string                     `json:"display"`
	Role         *objectReference           `json:"role"`
	Tenant       *objectReference           `json:"tenant"`
	Platform     *objectReference           `json:"platform"`
	Site         *objectReference           `json:"site"`
	Cluster      *objectReference           `json:"cluster"`
	Device       *objectReference           `json:"device"`
	Status       json.RawMessage            `json:"status"`
	PrimaryIP    *ipAddressReference        `json:"primary_ip"`
	Tags         []tagReference             `json:"tags"`
	CustomFields map[string]json.RawMessage `json:"custom_fields"`
}

type contactAssignmentResponse struct {
	Contact  objectReference  `json:"contact"`
	Role     *objectReference `json:"role"`
	Priority json.RawMessage  `json:"priority"`
}

func readBoundedBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxResponseBodySize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBodySize {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxResponseBodySize)
	}
	return raw, nil
}

func sanitizeRequestError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err
	}
	return err
}

func validateAPIVersion(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ".")
	if len(parts) > 3 {
		return fmt.Errorf("API-Version header is invalid")
	}
	var version [3]uint64
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("API-Version header is invalid")
		}
		parsed, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return fmt.Errorf("API-Version header is invalid")
		}
		version[i] = parsed
	}
	// NetBox normally publishes only major.minor, even for patch releases.
	if version[0] != minSupportedAPIMajor ||
		version[1] < minSupportedAPIMinor ||
		(version[1] == minSupportedAPIMinor && len(parts) == 3 && version[2] < minSupportedAPIPatch) {
		return fmt.Errorf("NetBox API version %q is unsupported; version 4.5.2 or newer in the 4.x series is required", value)
	}
	return nil
}

func choiceString(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '{' {
		var choice struct {
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(trimmed, &choice); err != nil {
			return "", err
		}
		if len(choice.Value) == 0 {
			return "", fmt.Errorf("choice object must include value")
		}
		value, present, err := scalarString(choice.Value)
		if err != nil {
			return "", err
		}
		if !present {
			return "", nil
		}
		return value, nil
	}
	value, present, err := scalarString(trimmed)
	if err != nil {
		return "", err
	}
	if !present {
		return "", nil
	}
	return value, nil
}

func scalarString(raw json.RawMessage) (string, bool, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return "", false, err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", false, fmt.Errorf("value has trailing JSON")
		}
		return "", false, err
	}
	switch typed := value.(type) {
	case nil:
		return "", false, nil
	case string:
		return typed, true, nil
	case bool:
		return strconv.FormatBool(typed), true, nil
	case json.Number:
		return typed.String(), true, nil
	default:
		return "", false, fmt.Errorf("value is not scalar")
	}
}
