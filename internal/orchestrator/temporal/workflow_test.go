package temporal_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/worker"

	"github.com/openclarion/openclarion/internal/domain"
	temporalpkg "github.com/openclarion/openclarion/internal/orchestrator/temporal"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/repository"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	testPGImage                          = "postgres:18-alpine"
	testDBName                           = "openclarion_test"
	testDBUser                           = "openclarion"
	testDBPassword                       = "openclarion"
	temporalWorkflowTestTimeout          = 8 * time.Minute
	temporalWorkflowDevServerStartBudget = 3 * time.Minute
	temporalWorkflowOperationTimeout     = 45 * time.Second
	temporalWorkflowCleanupTimeout       = 30 * time.Second
)

type testEnv struct {
	pgContainer *postgres.PostgresContainer
	entClient   *ent.Client
	factory     ports.UnitOfWorkFactory
	devServer   *testsuite.DevServer
	tc          client.Client
	w           worker.Worker
}

var env *testEnv

type seededDiagnosisTask struct {
	TaskID     domain.DiagnosisTaskID
	SnapshotID domain.EvidenceSnapshotID
}

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	ctx, cancel := context.WithTimeout(context.Background(), temporalWorkflowTestTimeout)
	defer cancel()

	ctr, err := postgres.Run(
		ctx,
		testPGImage,
		postgres.WithDatabase(testDBName),
		postgres.WithUsername(testDBUser),
		postgres.WithPassword(testDBPassword),
		postgres.BasicWaitStrategies(),
		postgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testcontainers postgres start: %v\n", err)
		return 1
	}
	defer func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), temporalWorkflowCleanupTimeout)
		defer cancelCleanup()
		if terr := ctr.Terminate(cleanupCtx); terr != nil {
			fmt.Fprintf(os.Stderr, "terminate postgres container: %v\n", terr)
		}
	}()

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres connection string: %v\n", err)
		return 1
	}

	migrateDB, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open postgres for migrate: %v\n", err)
		return 1
	}
	migrateDrv := entsql.OpenDB(dialect.Postgres, migrateDB)
	migrateClient := ent.NewClient(ent.Driver(migrateDrv))
	if err := migrateClient.Schema.Create(ctx); err != nil {
		_ = migrateClient.Close()
		fmt.Fprintf(os.Stderr, "create ent schema: %v\n", err)
		return 1
	}
	if err := migrateClient.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close migrate client: %v\n", err)
		return 1
	}

	entClient, err := repository.OpenPostgres(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open ent client: %v\n", err)
		return 1
	}
	defer func() { _ = entClient.Close() }()

	factory := repository.NewFactory(entClient)

	devServerStartCtx, cancelDevServerStart := context.WithTimeout(ctx, temporalWorkflowDevServerStartBudget)
	defer cancelDevServerStart()
	server, err := testsuite.StartDevServer(devServerStartCtx, testsuite.DevServerOptions{
		LogLevel: "error",
		Stdout:   io.Discard,
		Stderr:   io.Discard,
		ExtraArgs: []string{
			"--dynamic-config-value", "frontend.enableUpdateWorkflowExecution=true",
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start temporal dev server: %v\n", err)
		return 1
	}
	defer func() {
		if serr := server.Stop(); serr != nil {
			fmt.Fprintf(os.Stderr, "stop temporal dev server: %v\n", serr)
		}
	}()

	tc := server.Client()
	defer tc.Close()

	w := temporalpkg.NewWorker(
		tc,
		factory,
		temporalpkg.WithLLMProvider(newReportLLMProvider()),
		temporalpkg.WithIMProvider(&recordingIMProvider{delivery: ports.IMDelivery{
			ProviderMessageID: "msg-devserver",
			Status:            "accepted",
			Raw:               json.RawMessage(`{"message_id":"msg-devserver","status":"accepted"}`),
		}}),
	)
	if err := w.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start temporal worker: %v\n", err)
		return 1
	}
	defer w.Stop()

	env = &testEnv{
		pgContainer: ctr,
		entClient:   entClient,
		factory:     factory,
		devServer:   server,
		tc:          tc,
		w:           w,
	}

	return m.Run()
}

func workflowTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), temporalWorkflowOperationTimeout)
	t.Cleanup(cancel)
	return ctx
}

func seedDiagnosisTask(t *testing.T, label string) seededDiagnosisTask {
	t.Helper()
	ctx := workflowTestContext(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	var seeded seededDiagnosisTask
	err := env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		g, err := domain.NewAlertGroup(
			"wf-grp-"+label,
			json.RawMessage(`{"region":"test"}`),
			domain.GroupSeverityWarning,
			1,
			now, now,
			nil,
		)
		if err != nil {
			return err
		}
		savedGroup, err := uow.Alerts().SaveGroup(ctx, g)
		if err != nil {
			return err
		}

		s, err := domain.NewEvidenceSnapshot(
			savedGroup.ID,
			"wf-digest-"+label,
			json.RawMessage(`{"metric":"cpu"}`),
			json.RawMessage(`{"providers":{"prom":"ok"}}`),
			domain.SnapshotStatusComplete,
			nil,
			"DiagnosisWorkflow",
		)
		if err != nil {
			return err
		}
		savedSnap, err := uow.Evidence().Save(ctx, s)
		if err != nil {
			return err
		}
		seeded.SnapshotID = savedSnap.ID

		task, err := domain.NewDiagnosisTask(savedSnap.ID, "wf-"+label, "run-"+label)
		if err != nil {
			return err
		}
		savedTask, err := uow.Diagnosis().SaveTask(ctx, task)
		if err != nil {
			return err
		}
		seeded.TaskID = savedTask.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seedDiagnosisTask(%s): %v", label, err)
	}
	return seeded
}

func diagnosisWorkflowInput(seed seededDiagnosisTask) temporalpkg.DiagnosisWorkflowInput {
	return temporalpkg.DiagnosisWorkflowInput{
		TaskID:             int64(seed.TaskID),
		EvidenceSnapshotID: int64(seed.SnapshotID),
	}
}

func TestDiagnosisWorkflow_UpdateRecordEvent(t *testing.T) {
	seed := seedDiagnosisTask(t, "update-record")

	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-diag-%d", seed.TaskID)

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, diagnosisWorkflowInput(seed))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	dedupeKey := "bootstrap-" + workflowID
	handle, err := env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   "record-event",
		Args:         []any{temporalpkg.RecordEventRequest{Kind: "workflow_bootstrapped", DedupeKey: &dedupeKey}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflow: %v", err)
	}

	var result temporalpkg.RecordEventResult
	if err := handle.Get(ctx, &result); err != nil {
		t.Fatalf("Update.Get: %v", err)
	}
	if result.EventID == 0 {
		t.Fatal("expected non-zero EventID from Update")
	}

	err = env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, seed.TaskID)
		if err != nil {
			return err
		}
		if task.EvidenceSnapshotID != seed.SnapshotID {
			t.Fatalf("task evidence_snapshot_id = %d, want %d", task.EvidenceSnapshotID, seed.SnapshotID)
		}
		if task.Status != domain.DiagnosisStatusRunning {
			t.Fatalf("task status = %q, want running", task.Status)
		}
		if task.StartedAt == nil {
			t.Fatal("task StartedAt is nil after workflow start")
		}

		events, err := uow.Diagnosis().ListEvents(ctx, seed.TaskID, 100)
		if err != nil {
			return err
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Kind != "workflow_bootstrapped" {
			t.Fatalf("event kind = %q, want %q", events[0].Kind, "workflow_bootstrapped")
		}
		if events[0].DedupeKey == nil || *events[0].DedupeKey != dedupeKey {
			t.Fatalf("event dedupe_key = %v, want %q", events[0].DedupeKey, dedupeKey)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify events: %v", err)
	}

	if err := env.tc.SignalWorkflow(ctx, workflowID, run.GetRunID(), "complete", nil); err != nil {
		t.Fatalf("SignalWorkflow(complete): %v", err)
	}

	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("workflow did not complete cleanly: %v", err)
	}
	replayWorkflowHistory(ctx, t, workflowID, run.GetRunID())
}

func replayWorkflowHistory(ctx context.Context, t *testing.T, workflowID, runID string) {
	t.Helper()
	replayWorkflowHistoryWithRegistrations(ctx, t, workflowID, runID, temporalpkg.DiagnosisWorkflow)
}

func replayWorkflowHistoryWithRegistrations(ctx context.Context, t *testing.T, workflowID, runID string, workflows ...any) {
	t.Helper()
	history := collectWorkflowHistory(ctx, t, workflowID, runID)
	replayer := worker.NewWorkflowReplayer()
	for _, wf := range workflows {
		replayer.RegisterWorkflow(wf)
	}
	if err := replayer.ReplayWorkflowHistory(nil, history); err != nil {
		t.Fatalf("replay workflow history: %v", err)
	}
}

func collectWorkflowHistory(ctx context.Context, t *testing.T, workflowID, runID string) *historypb.History {
	t.Helper()
	iter := env.tc.GetWorkflowHistory(ctx, workflowID, runID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	var events []*historypb.HistoryEvent
	for iter.HasNext() {
		event, err := iter.Next()
		if err != nil {
			t.Fatalf("workflow history next: %v", err)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		t.Fatalf("workflow history for %s/%s is empty", workflowID, runID)
	}
	return &historypb.History{Events: events}
}

func TestDiagnosisWorkflow_UpdateValidation_RejectsEmptyKind(t *testing.T) {
	seed := seedDiagnosisTask(t, "validate-kind")

	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-validate-kind-%d", seed.TaskID)

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, diagnosisWorkflowInput(seed))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	dedupeKey := "validate-kind-" + workflowID
	handle, updErr := env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   "record-event",
		Args:         []any{temporalpkg.RecordEventRequest{Kind: "", DedupeKey: &dedupeKey}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	requireUpdateError(ctx, t, handle, updErr, "kind must be non-empty")

	// Validator runs pre-history for the Update: no
	// RecordDiagnosisEvent activity and no event row.
	assertNoDiagnosisEvents(t, seed.TaskID)

	if err := env.tc.SignalWorkflow(ctx, workflowID, run.GetRunID(), "complete", nil); err != nil {
		t.Fatalf("SignalWorkflow(complete): %v", err)
	}
	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("workflow did not complete cleanly: %v", err)
	}
}

func TestDiagnosisWorkflow_UpdateValidation_RejectsEmptyDedupeKey(t *testing.T) {
	seed := seedDiagnosisTask(t, "validate-dedupe")

	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-validate-dedupe-%d", seed.TaskID)

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, diagnosisWorkflowInput(seed))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	// nil DedupeKey is the misuse the validator must catch.
	handle, updErr := env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   "record-event",
		Args:         []any{temporalpkg.RecordEventRequest{Kind: "workflow_bootstrapped", DedupeKey: nil}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	requireUpdateError(ctx, t, handle, updErr, "dedupe_key must be non-empty")

	// Empty-string DedupeKey must also be rejected with the same message.
	empty := ""
	handle, updErr = env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   "record-event",
		Args:         []any{temporalpkg.RecordEventRequest{Kind: "workflow_bootstrapped", DedupeKey: &empty}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	requireUpdateError(ctx, t, handle, updErr, "dedupe_key must be non-empty")

	assertNoDiagnosisEvents(t, seed.TaskID)

	if err := env.tc.SignalWorkflow(ctx, workflowID, run.GetRunID(), "complete", nil); err != nil {
		t.Fatalf("SignalWorkflow(complete): %v", err)
	}
	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("workflow did not complete cleanly: %v", err)
	}
}

// TestDiagnosisWorkflow_RejectsZeroTaskIDOnStart starts a workflow
// with input.TaskID == 0. Because TaskID is the workflow's bound
// identity, the workflow MUST fail at entry rather than block on
// signal-wait until an Update happens to reach the activity.
func TestDiagnosisWorkflow_RejectsZeroTaskIDOnStart(t *testing.T) {
	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-zero-taskid-%d", time.Now().UnixNano())

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, temporalpkg.DiagnosisWorkflowInput{TaskID: 0})
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	runErr := run.Get(waitCtx, nil)
	if runErr == nil {
		t.Fatal("expected workflow to fail on zero TaskID, got nil")
	}
	if !strings.Contains(runErr.Error(), "task_id must be non-zero") {
		t.Fatalf("workflow error = %q, want substring %q", runErr.Error(), "task_id must be non-zero")
	}
}

func TestDiagnosisWorkflow_RejectsZeroEvidenceSnapshotIDOnStart(t *testing.T) {
	seed := seedDiagnosisTask(t, "zero-snapshot")

	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-zero-snapshot-%d", seed.TaskID)

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, temporalpkg.DiagnosisWorkflowInput{
		TaskID:             int64(seed.TaskID),
		EvidenceSnapshotID: 0,
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	runErr := run.Get(waitCtx, nil)
	if runErr == nil {
		t.Fatal("expected workflow to fail on zero EvidenceSnapshotID, got nil")
	}
	if !strings.Contains(runErr.Error(), "evidence_snapshot_id must be non-zero") {
		t.Fatalf("workflow error = %q, want substring %q", runErr.Error(), "evidence_snapshot_id must be non-zero")
	}
	assertDiagnosisTaskPending(t, seed.TaskID)
}

func TestDiagnosisWorkflow_RejectsMismatchedEvidenceSnapshotIDOnStart(t *testing.T) {
	taskA := seedDiagnosisTask(t, "snapshot-mismatch-a")
	taskB := seedDiagnosisTask(t, "snapshot-mismatch-b")

	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-snapshot-mismatch-%d", taskA.TaskID)

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, temporalpkg.DiagnosisWorkflowInput{
		TaskID:             int64(taskA.TaskID),
		EvidenceSnapshotID: int64(taskB.SnapshotID),
	})
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	runErr := run.Get(waitCtx, nil)
	if runErr == nil {
		t.Fatal("expected workflow to fail on mismatched EvidenceSnapshotID, got nil")
	}
	if !strings.Contains(runErr.Error(), "does not match input evidence_snapshot_id") {
		t.Fatalf("workflow error = %q, want snapshot mismatch substring", runErr.Error())
	}
	assertDiagnosisTaskPending(t, taskA.TaskID)
}

// TestDiagnosisWorkflow_UpdateBoundToWorkflowTask verifies the
// workflow input.TaskID is the only task an Update can write to:
// even when two workflows are running for two different tasks, an
// Update sent to workflow A only ever produces events on task A.
// Because RecordEventRequest no longer carries TaskID, this is now
// structurally enforced — the test guards against regression.
func TestDiagnosisWorkflow_UpdateBoundToWorkflowTask(t *testing.T) {
	taskA := seedDiagnosisTask(t, "bound-a")
	taskB := seedDiagnosisTask(t, "bound-b")

	ctx := workflowTestContext(t)
	wfA := fmt.Sprintf("test-bound-a-%d", taskA.TaskID)
	wfB := fmt.Sprintf("test-bound-b-%d", taskB.TaskID)

	runA, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        wfA,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, diagnosisWorkflowInput(taskA))
	if err != nil {
		t.Fatalf("ExecuteWorkflow A: %v", err)
	}
	runB, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        wfB,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, diagnosisWorkflowInput(taskB))
	if err != nil {
		t.Fatalf("ExecuteWorkflow B: %v", err)
	}

	dedupeKey := "bound-" + wfA
	handle, err := env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   wfA,
		UpdateName:   "record-event",
		Args:         []any{temporalpkg.RecordEventRequest{Kind: "workflow_bootstrapped", DedupeKey: &dedupeKey}},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflow A: %v", err)
	}
	var result temporalpkg.RecordEventResult
	if err := handle.Get(ctx, &result); err != nil {
		t.Fatalf("Update.Get: %v", err)
	}
	if result.EventID == 0 {
		t.Fatal("expected non-zero EventID")
	}

	eventsA := listDiagnosisEvents(t, taskA.TaskID)
	if len(eventsA) != 1 {
		t.Fatalf("task A: expected 1 event, got %d", len(eventsA))
	}
	eventsB := listDiagnosisEvents(t, taskB.TaskID)
	if len(eventsB) != 0 {
		t.Fatalf("task B: expected 0 events (workflow B never received an Update), got %d", len(eventsB))
	}

	for _, item := range []struct {
		id  string
		run client.WorkflowRun
	}{{wfA, runA}, {wfB, runB}} {
		if err := env.tc.SignalWorkflow(ctx, item.id, item.run.GetRunID(), "complete", nil); err != nil {
			t.Fatalf("SignalWorkflow(complete) %s: %v", item.id, err)
		}
		if err := item.run.Get(ctx, nil); err != nil {
			t.Fatalf("workflow %s did not complete cleanly: %v", item.id, err)
		}
	}
}

// TestDiagnosisWorkflow_UpdateIdempotent_SameDedupeKey verifies the
// three-tx idempotent producer chain in the Activity: a second
// Update with the same dedupe_key returns the original EventID and
// produces no second row in the DB.
func TestDiagnosisWorkflow_UpdateIdempotent_SameDedupeKey(t *testing.T) {
	seed := seedDiagnosisTask(t, "idempotent")

	ctx := workflowTestContext(t)
	workflowID := fmt.Sprintf("test-idempotent-%d", seed.TaskID)

	run, err := env.tc.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: temporalpkg.TaskQueue,
	}, temporalpkg.DiagnosisWorkflow, diagnosisWorkflowInput(seed))
	if err != nil {
		t.Fatalf("ExecuteWorkflow: %v", err)
	}

	dedupeKey := "idempotent-" + workflowID
	req := temporalpkg.RecordEventRequest{Kind: "workflow_bootstrapped", DedupeKey: &dedupeKey}

	firstHandle, err := env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   "record-event",
		Args:         []any{req},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflow first: %v", err)
	}
	var firstResult temporalpkg.RecordEventResult
	if err := firstHandle.Get(ctx, &firstResult); err != nil {
		t.Fatalf("first Update.Get: %v", err)
	}
	if firstResult.EventID == 0 {
		t.Fatal("expected non-zero EventID on first Update")
	}

	secondHandle, err := env.tc.UpdateWorkflow(ctx, client.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		UpdateName:   "record-event",
		Args:         []any{req},
		WaitForStage: client.WorkflowUpdateStageCompleted,
	})
	if err != nil {
		t.Fatalf("UpdateWorkflow second: %v", err)
	}
	var secondResult temporalpkg.RecordEventResult
	if err := secondHandle.Get(ctx, &secondResult); err != nil {
		t.Fatalf("second Update.Get: %v", err)
	}
	if secondResult.EventID != firstResult.EventID {
		t.Fatalf("idempotent EventID mismatch: first=%d second=%d", firstResult.EventID, secondResult.EventID)
	}

	events := listDiagnosisEvents(t, seed.TaskID)
	if len(events) != 1 {
		t.Fatalf("expected 1 event after idempotent re-Update, got %d", len(events))
	}

	if err := env.tc.SignalWorkflow(ctx, workflowID, run.GetRunID(), "complete", nil); err != nil {
		t.Fatalf("SignalWorkflow(complete): %v", err)
	}
	if err := run.Get(ctx, nil); err != nil {
		t.Fatalf("workflow did not complete cleanly: %v", err)
	}
}

func listDiagnosisEvents(t *testing.T, taskID domain.DiagnosisTaskID) []domain.DiagnosisTaskEvent {
	t.Helper()
	ctx := workflowTestContext(t)
	var out []domain.DiagnosisTaskEvent
	err := env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		events, err := uow.Diagnosis().ListEvents(ctx, taskID, 100)
		if err != nil {
			return err
		}
		out = events
		return nil
	})
	if err != nil {
		t.Fatalf("list events for task %d: %v", taskID, err)
	}
	return out
}

func assertNoDiagnosisEvents(t *testing.T, taskID domain.DiagnosisTaskID) {
	t.Helper()
	if events := listDiagnosisEvents(t, taskID); len(events) != 0 {
		t.Fatalf("expected 0 events after rejected validator, got %d", len(events))
	}
}

func assertDiagnosisTaskPending(t *testing.T, taskID domain.DiagnosisTaskID) {
	t.Helper()
	ctx := workflowTestContext(t)
	err := env.factory.WithinTx(ctx, func(ctx context.Context, uow ports.UnitOfWork) error {
		task, err := uow.Diagnosis().FindTaskByID(ctx, taskID)
		if err != nil {
			return err
		}
		if task.Status != domain.DiagnosisStatusPending {
			t.Fatalf("task status = %q, want pending", task.Status)
		}
		if task.StartedAt != nil {
			t.Fatalf("task StartedAt = %v, want nil", task.StartedAt)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("load task %d: %v", taskID, err)
	}
}

// requireUpdateError asserts the Update produced a non-nil error
// whose message contains wantSubstr. The validator's rejection may
// surface either as the synchronous UpdateWorkflow error or as the
// handle.Get error depending on which stage the server short-circuits
// on, so we accept either path but require the substring match. This
// guards against silently passing tests when the validator is
// removed but the activity still rejects on the same condition.
func requireUpdateError(ctx context.Context, t *testing.T, handle client.WorkflowUpdateHandle, updateErr error, wantSubstr string) {
	t.Helper()
	if updateErr != nil {
		if !strings.Contains(updateErr.Error(), wantSubstr) {
			t.Fatalf("UpdateWorkflow err = %q, want substring %q", updateErr.Error(), wantSubstr)
		}
		return
	}
	var result temporalpkg.RecordEventResult
	getErr := handle.Get(ctx, &result)
	if getErr == nil {
		t.Fatalf("expected validation error containing %q, got nil", wantSubstr)
	}
	if !strings.Contains(getErr.Error(), wantSubstr) {
		t.Fatalf("handle.Get err = %q, want substring %q", getErr.Error(), wantSubstr)
	}
}
