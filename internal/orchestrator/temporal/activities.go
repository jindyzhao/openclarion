package temporal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

type Activities struct {
	uowFactory ports.UnitOfWorkFactory
}

func NewActivities(uowFactory ports.UnitOfWorkFactory) *Activities {
	return &Activities{uowFactory: uowFactory}
}

// RecordDiagnosisEvent appends a DiagnosisTaskEvent for the bound
// task and is idempotent on (task_id, dedupe_key): a duplicate
// invocation returns the existing event's ID instead of failing.
//
// The flow uses three independent transactions because Postgres
// poisons a transaction after a 23505 unique violation — the same
// tx cannot be reused to SELECT the conflicting row, so retries
// must run in fresh transactions:
//
//  1. Pre-check: look up (task_id, dedupe_key); short-circuit on hit.
//  2. Insert:    append event in its own tx.
//  3. Race-lost: on ErrAlreadyExists, re-fetch in a fresh tx.
//
// Permanent input/invariant failures are wrapped as non-retryable
// application errors so Temporal's RetryPolicy stops retrying. The
// error type strings are kept in sync with workflow.go via the
// errType* constants.
func (a *Activities) RecordDiagnosisEvent(ctx context.Context, req recordEventActivityInput) (RecordEventResult, error) {
	if req.TaskID == 0 {
		return RecordEventResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-event: task_id must be non-zero", errTypeInvalidInput, nil)
	}
	if req.Kind == "" {
		return RecordEventResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-event: kind must be non-empty", errTypeInvalidInput, nil)
	}
	if req.DedupeKey == nil || *req.DedupeKey == "" {
		return RecordEventResult{}, temporalsdk.NewNonRetryableApplicationError(
			"record-event: dedupe_key must be non-empty", errTypeInvalidInput, nil)
	}
	dedupeKey := *req.DedupeKey
	taskID := domain.DiagnosisTaskID(req.TaskID)

	// 1) Pre-check in its own tx: cheapest path on duplicates.
	if id, found, err := a.lookupExisting(ctx, taskID, dedupeKey); err != nil {
		return RecordEventResult{}, mapActivityError(err, "record-event pre-check")
	} else if found {
		return RecordEventResult{EventID: id}, nil
	}

	// 2) Build the domain event then attempt insert in its own tx.
	evt, err := domain.NewDiagnosisTaskEvent(
		taskID,
		req.Kind,
		json.RawMessage("{}"),
		req.DedupeKey,
		time.Now(),
	)
	if err != nil {
		return RecordEventResult{}, mapActivityError(err, "record-event build")
	}

	var insertedID int64
	insertErr := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		saved, appendErr := uow.Diagnosis().AppendEvent(ctx, evt)
		if appendErr != nil {
			return appendErr
		}
		insertedID = int64(saved.ID)
		return nil
	})
	if insertErr == nil {
		return RecordEventResult{EventID: insertedID}, nil
	}
	if !errors.Is(insertErr, domain.ErrAlreadyExists) {
		return RecordEventResult{}, mapActivityError(insertErr, "record-event append")
	}

	// 3) Race lost: another caller inserted between our pre-check
	// and this insert. Re-fetch in a fresh tx (the failed insert
	// tx is poisoned and cannot serve the SELECT).
	id, found, err := a.lookupExisting(ctx, taskID, dedupeKey)
	if err != nil {
		return RecordEventResult{}, mapActivityError(err, "record-event re-fetch")
	}
	if !found {
		return RecordEventResult{}, fmt.Errorf(
			"record-event: race-lost re-fetch missing for task %d dedupe %q",
			req.TaskID, dedupeKey)
	}
	return RecordEventResult{EventID: id}, nil
}

// lookupExisting runs FindEventByTaskAndDedupeKey inside its own tx
// and translates ErrNotFound into (0, false, nil) so callers can
// branch without inspecting domain sentinels.
func (a *Activities) lookupExisting(ctx context.Context, taskID domain.DiagnosisTaskID, dedupeKey string) (int64, bool, error) {
	var (
		id    int64
		found bool
	)
	err := a.uowFactory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		existing, ferr := uow.Diagnosis().FindEventByTaskAndDedupeKey(ctx, taskID, dedupeKey)
		if ferr != nil {
			if errors.Is(ferr, domain.ErrNotFound) {
				return nil
			}
			return ferr
		}
		id = int64(existing.ID)
		found = true
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	return id, found, nil
}

// mapActivityError classifies a domain/persistence error as either a
// non-retryable application error (input/invariant) or a generic
// retryable error (infrastructure). The non-retryable type strings
// are matched by ActivityOptions.RetryPolicy.NonRetryableErrorTypes.
func mapActivityError(err error, where string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrInvariantViolation) {
		return temporalsdk.NewNonRetryableApplicationError(
			fmt.Sprintf("%s: %v", where, err), errTypeInvariantViolation, err)
	}
	return fmt.Errorf("%s: %w", where, err)
}
