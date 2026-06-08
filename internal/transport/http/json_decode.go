package http

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/openclarion/openclarion/internal/strictjson"
)

const maxJSONRequestBodyBytes = 1 << 20

func decodeStrictJSONRequestBody(w http.ResponseWriter, r *http.Request, dst any) error {
	raw, err := readJSONRequestBody(w, r)
	if err != nil {
		return err
	}
	if err := strictjson.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid JSON request body: %w", err)
	}
	return nil
}

func readJSONRequestBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body := http.MaxBytesReader(w, r.Body, maxJSONRequestBodyBytes)
	defer func() {
		_, _ = io.Copy(io.Discard, body)
		_ = body.Close()
	}()

	raw, err := io.ReadAll(body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, fmt.Errorf("request body exceeds %d bytes", maxJSONRequestBodyBytes)
		}
		return nil, fmt.Errorf("read request body: %w", err)
	}
	return raw, nil
}
