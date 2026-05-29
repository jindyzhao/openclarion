package temporal

import (
	"fmt"
	"time"

	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// TaskQueue is the Temporal task queue used by OpenClarion workers.
const TaskQueue = "openclarion"

// Non-retryable application error type strings. Activities classify
// permanent input/invariant failures with these values, and the
// ActivityOptions.RetryPolicy below stops retrying when matched.
// Keep in sync with mapActivityError in activities.go.
const (
	errTypeInvalidInput       = "InvalidInput"
	errTypeInvariantViolation = "InvariantViolation"
)

const (
	snapshotBoundStartChangeID = "diagnosis-workflow-snapshot-bound-start"
	snapshotBoundStartVersion  = 1
)

// DiagnosisWorkflowInput identifies the snapshot and diagnosis task a
// workflow run owns. EvidenceSnapshotID is required for new workflow
// histories; workflow.GetVersion keeps older histories replayable.
type DiagnosisWorkflowInput struct {
	TaskID             int64
	EvidenceSnapshotID int64
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

// RecordEventResult returns the persisted diagnosis event identity.
type RecordEventResult struct {
	EventID int64
}

// recordEventActivityInput is the workflow->activity payload. It is
// unexported on purpose: only the update handler can construct it,
// and only by copying the bound workflow input's TaskID.
type recordEventActivityInput struct {
	TaskID    int64
	Kind      string
	DedupeKey *string
}

// startDiagnosisTaskActivityInput is the workflow->activity payload
// that binds a workflow run to a single EvidenceSnapshot before any
// Update can append task events.
type startDiagnosisTaskActivityInput struct {
	TaskID             int64
	EvidenceSnapshotID int64
}

// DiagnosisWorkflow coordinates diagnosis task updates and completion.
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

	version := workflow.GetVersion(ctx, snapshotBoundStartChangeID, workflow.DefaultVersion, snapshotBoundStartVersion)
	startupComplete := version == workflow.DefaultVersion
	if version >= snapshotBoundStartVersion {
		if input.EvidenceSnapshotID == 0 {
			return temporalsdk.NewNonRetryableApplicationError(
				"diagnosis-workflow: input.evidence_snapshot_id must be non-zero",
				errTypeInvalidInput, nil)
		}
	}

	err := workflow.SetUpdateHandlerWithOptions(
		ctx,
		"record-event",
		func(ctx workflow.Context, req RecordEventRequest) (RecordEventResult, error) {
			if err := workflow.Await(ctx, func() bool { return startupComplete }); err != nil {
				return RecordEventResult{}, err
			}
			actCtx := workflow.WithActivityOptions(ctx, diagnosisActivityOptions())
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
			Validator: func(_ workflow.Context, req RecordEventRequest) error {
				if req.Kind == "" {
					return fmt.Errorf("record-event: kind must be non-empty")
				}
				// dedupe_key is required: the (task_id, dedupe_key)
				// UNIQUE index uses Postgres multi-NULL semantics,
				// so a nil/empty key would silently disable
				// idempotency. Reject pre-history so no event row
				// is written.
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

	if version >= snapshotBoundStartVersion {
		actCtx := workflow.WithActivityOptions(ctx, diagnosisActivityOptions())
		req := startDiagnosisTaskActivityInput{
			TaskID:             input.TaskID,
			EvidenceSnapshotID: input.EvidenceSnapshotID,
		}
		if err := workflow.ExecuteActivity(actCtx, (*Activities).StartDiagnosisTask, req).Get(ctx, nil); err != nil {
			return err
		}
		startupComplete = true
	}

	completeCh := workflow.GetSignalChannel(ctx, "complete")
	completeCh.Receive(ctx, nil)
	return nil
}

func diagnosisActivityOptions() workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		// Explicit RetryPolicy: infrastructure faults retry up to 3
		// attempts, but permanent input/invariant errors short-circuit
		// via NonRetryableErrorTypes.
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
	}
}
