package domain

import (
	"encoding/json"
	"fmt"

	"github.com/openclarion/openclarion/internal/strictjson"
)

func requireValidJSON(label string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s must be non-empty JSON: %w", label, ErrInvariantViolation)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("%s must be duplicate-key-free JSON: %w: %w", label, err, ErrInvariantViolation)
	}
	return nil
}
