package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func requireValidJSON(label string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s must be non-empty JSON: %w", label, ErrInvariantViolation)
	}
	if err := rejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("%s must be duplicate-key-free JSON: %w: %w", label, err, ErrInvariantViolation)
	}
	return nil
}

// Keep this scanner local to preserve the domain package's zero non-stdlib
// dependency contract documented in doc.go.
func rejectDuplicateObjectKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := scanJSONValue(dec, "$"); err != nil {
		return err
	}
	if tok, err := dec.Token(); err == nil {
		return fmt.Errorf("JSON contains trailing JSON values after token %v", tok)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func scanJSONValue(dec *json.Decoder, path string) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: decode JSON token: %w", path, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return scanJSONObject(dec, path)
	case '[':
		return scanJSONArray(dec, path)
	default:
		return fmt.Errorf("%s: unexpected JSON delimiter %q", path, delim)
	}
}

func scanJSONObject(dec *json.Decoder, path string) error {
	seen := map[string]struct{}{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("%s: decode object key: %w", path, err)
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("%s: object key token has type %T", path, tok)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s: duplicate object key %q", path, key)
		}
		seen[key] = struct{}{}
		if err := scanJSONValue(dec, pathForJSONKey(path, key)); err != nil {
			return err
		}
	}
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: close object: %w", path, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '}' {
		return fmt.Errorf("%s: object closed by %v, want }", path, tok)
	}
	return nil
}

func scanJSONArray(dec *json.Decoder, path string) error {
	index := 0
	for dec.More() {
		if err := scanJSONValue(dec, fmt.Sprintf("%s[%d]", path, index)); err != nil {
			return err
		}
		index++
	}
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("%s: close array: %w", path, err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != ']' {
		return fmt.Errorf("%s: array closed by %v, want ]", path, tok)
	}
	return nil
}

func pathForJSONKey(path, key string) string {
	if key == "" {
		return path + "[\"\"]"
	}
	return path + "." + key
}
