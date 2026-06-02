package temporal

import (
	"encoding/json"
	"fmt"

	"github.com/openclarion/openclarion/internal/strictjson"
)

func validateDiagnosisRoomEvidenceJSON(label string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s must be non-empty JSON object", label)
	}
	if err := strictjson.RejectDuplicateObjectKeys(raw); err != nil {
		return fmt.Errorf("%s must be duplicate-key-free JSON object: %w", label, err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s must be valid JSON object: %w", label, err)
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("%s must be a JSON object", label)
	}
	return nil
}
