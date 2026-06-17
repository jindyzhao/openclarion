package temporal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisevidence"
	"github.com/openclarion/openclarion/internal/usecases/diagnosisroom"
)

// CollectDiagnosisEvidenceInput carries one accepted assistant evidence plan to
// the provider-backed collection Activity.
type CollectDiagnosisEvidenceInput struct {
	SessionID       string
	DiagnosisTaskID int64
	Requests        []diagnosisroom.EvidenceRequest
}

// CollectDiagnosisEvidenceResult is returned to the workflow after bounded
// provider-side evidence collection finishes.
type CollectDiagnosisEvidenceResult struct {
	Items []diagnosisevidence.Item
}

// CollectDiagnosisEvidence executes supported diagnosis evidence requests
// outside the deterministic workflow boundary.
func (a *Activities) CollectDiagnosisEvidence(
	ctx context.Context,
	req CollectDiagnosisEvidenceInput,
) (CollectDiagnosisEvidenceResult, error) {
	if err := validateCollectDiagnosisEvidenceInput(req); err != nil {
		return CollectDiagnosisEvidenceResult{}, mapActivityError(err, "collect-diagnosis-evidence input")
	}
	if len(req.Requests) == 0 {
		return CollectDiagnosisEvidenceResult{}, nil
	}
	if a == nil || a.uowFactory == nil || a.alertSourceProviders == nil {
		return CollectDiagnosisEvidenceResult{
			Items: unavailableEvidenceItems(req.Requests, time.Now().UTC()),
		}, nil
	}
	svc, err := diagnosisevidence.NewService(
		a.uowFactory,
		a.alertSourceProviders,
		diagnosisevidence.WithClock(func() time.Time { return time.Now().UTC() }),
	)
	if err != nil {
		return CollectDiagnosisEvidenceResult{}, mapActivityError(err, "collect-diagnosis-evidence service")
	}
	result, err := svc.Collect(ctx, diagnosisevidence.Request{Requests: req.Requests})
	if err != nil {
		return CollectDiagnosisEvidenceResult{}, mapActivityError(err, "collect-diagnosis-evidence")
	}
	return CollectDiagnosisEvidenceResult{Items: diagnosisevidence.CloneItems(result.Items)}, nil
}

func validateCollectDiagnosisEvidenceInput(req CollectDiagnosisEvidenceInput) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("diagnosis evidence: session_id must be non-empty: %w", domain.ErrInvariantViolation)
	}
	if req.DiagnosisTaskID == 0 {
		return fmt.Errorf("diagnosis evidence: diagnosis_task_id must be non-zero: %w", domain.ErrInvariantViolation)
	}
	if len(req.Requests) > 5 {
		return fmt.Errorf("diagnosis evidence: requests exceeds 5 items: %w", domain.ErrInvariantViolation)
	}
	for i, request := range req.Requests {
		if !request.Tool.Valid() {
			return fmt.Errorf("diagnosis evidence: requests[%d].tool is unsupported: %w", i, domain.ErrInvariantViolation)
		}
	}
	return nil
}

func unavailableEvidenceItems(
	requests []diagnosisroom.EvidenceRequest,
	collectedAt time.Time,
) []diagnosisevidence.Item {
	out := make([]diagnosisevidence.Item, len(requests))
	for i, request := range requests {
		out[i] = diagnosisevidence.Item{
			Request:     request,
			Tool:        request.Tool,
			Status:      diagnosisevidence.StatusSkipped,
			ReasonCode:  diagnosisevidence.ReasonProviderUnavailable,
			Message:     "Diagnosis evidence collection is not configured.",
			CollectedAt: collectedAt,
		}
	}
	return out
}
