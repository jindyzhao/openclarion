package temporal

import (
	"fmt"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const TaskQueue = "openclarion"

// Non-retryable application error type strings. Activities classify
// permanent input/invariant failures with these values, and the
// ActivityOptions.RetryPolicy below stops retrying when matched.
// Keep in sync with mapActivityError in activities.go.
const (
	errTypeInvalidInput       = "InvalidInput"
	errTypeInvariantViolation = "InvariantViolation"
)

type DiagnosisWorkflowInput struct {
	TaskID int64
}

// RecordEventRequest is the public payload for the "record-event"
// Update. TaskID is intentionally absent: every Update is bound to
// the workflow's input.TaskID by the handler closure, which makes
// "Update writes to a different task than the workflow it was sent
// to" structurally impossible.
type RecordEventRequest struct {
	Kind      string
	DedupeKey *string
}

type RecordEventResult struct {
	EventID int64
}

// recordEventActivityInput is the workflow→activity payload. It is
// unexported on purpose: only the update handler can construct it,
// and only by copying the bound workflow input's TaskID.
type recordEventActivityInput struct {
	TaskID    int64
	Kind      string
	DedupeKey *string
}

func DiagnosisWorkflow(ctx workflow.Context, input DiagnosisWorkflowInput) error {
	// Reject zero TaskID at workflow entry: input.TaskID is the
	// workflow's bound identity, so a zero value would let the
	// workflow enter signal-wait and only fail when an Update
	// reaches the activity. NonRetryable so Temporal does not retry
	// the workflow task forever on a permanent input error.
	if input.TaskID == 0 {
		return temporalsdk.NewNonRetryableApplicationError(
			"diagnosis-workflow: input.task_id must be non-zero",
			errTypeInvalidInput, nil)
	}

	err := workflow.SetUpdateHandlerWithOptions(
		ctx,
		"record-event",
		func(ctx workflow.Context, req RecordEventRequest) (RecordEventResult, error) {
			actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
				StartToCloseTimeout: 30 * time.Second,
				// Explicit RetryPolicy: infrastructure faults retry
				// up to 3 attempts, but permanent input/invariant
				// errors short-circuit via NonRetryableErrorTypes.
				RetryPolicy: &temporalsdk.RetryPolicy{
					InitialInterval:    time.Second,
					BackoffCoefficient: 2.0,
					MaximumInterval:    30 * time.Second,
					MaximumAttempts:    3,
					NonRetryableErrorTypes: []string{
						errTypeInvalidInput,
						errTypeInvariantViolation,
					},
				},
			})
			payload := recordEventActivityInput{
				TaskID:    input.TaskID,
				Kind:      req.Kind,
				DedupeKey: req.DedupeKey,
			}
			var result RecordEventResult
			if err := workflow.ExecuteActivity(actCtx, (*Activities).RecordDiagnosisEvent, payload).Get(ctx, &result); err != nil {
				return RecordEventResult{}, err
			}
			return result, nil
		},
		workflow.UpdateHandlerOptions{
			Validator: func(ctx workflow.Context, req RecordEventRequest) error {
				if req.Kind == "" {
					return fmt.Errorf("record-event: kind must be non-empty")
				}
				// dedupe_key is required: the (task_id, dedupe_key)
				// UNIQUE index uses Postgres multi-NULL semantics,
				// so a nil/empty key would silently disable
				// idempotency. Reject pre-history so no DB write
				// happens.
				if req.DedupeKey == nil || *req.DedupeKey == "" {
					return fmt.Errorf("record-event: dedupe_key must be non-empty")
				}
				return nil
			},
		},
	)
	if err != nil {
		return err
	}

	completeCh := workflow.GetSignalChannel(ctx, "complete")
	completeCh.Receive(ctx, nil)
	return nil
}
