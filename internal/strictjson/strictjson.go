// Package strictjson contains small JSON helpers for retained artifacts and
// production boundaries that must reject ambiguous object members.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Unmarshal rejects duplicate object keys and unknown struct fields before
// unmarshalling raw JSON into dst.
func Unmarshal(raw []byte, dst any) error {
	if err := RejectDuplicateObjectKeys(raw); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

// RejectDuplicateObjectKeys walks a JSON value and rejects duplicate keys in
// any object. Go's standard json.Unmarshal accepts duplicate keys, which can
// hide ambiguous retained evidence or sandbox outputs.
func RejectDuplicateObjectKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := scanValue(dec, "$"); err != nil {
		return err
	}
	if tok, err := dec.Token(); err == nil {
		return fmt.Errorf("JSON contains trailing JSON values after token %v", tok)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func scanValue(dec *json.Decoder, path string) error {
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
		return scanObject(dec, path)
	case '[':
		return scanArray(dec, path)
	default:
		return fmt.Errorf("%s: unexpected JSON delimiter %q", path, delim)
	}
}

func scanObject(dec *json.Decoder, path string) error {
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
		if err := scanValue(dec, pathForKey(path, key)); err != nil {
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

func scanArray(dec *json.Decoder, path string) error {
	index := 0
	for dec.More() {
		if err := scanValue(dec, fmt.Sprintf("%s[%d]", path, index)); err != nil {
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

func pathForKey(path, key string) string {
	if key == "" {
		return path + "[\"\"]"
	}
	return path + "." + key
}
