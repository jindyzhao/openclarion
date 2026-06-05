package temporal

import (
	"fmt"
	"strings"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// NewWorker registers OpenClarion workflows and activities on a Temporal worker.
func NewWorker(c client.Client, uowFactory ports.UnitOfWorkFactory, opts ...ActivityOption) worker.Worker {
	w := worker.New(c, TaskQueue, worker.Options{})
	registerOpenClarionWorker(w, uowFactory, opts...)
	return w
}

// NewWorkerWithTaskQueue registers OpenClarion workflows and activities on a
// Temporal worker that polls the given task queue.
func NewWorkerWithTaskQueue(
	c client.Client,
	uowFactory ports.UnitOfWorkFactory,
	taskQueue string,
	opts ...ActivityOption,
) (worker.Worker, error) {
	taskQueue = strings.TrimSpace(taskQueue)
	if taskQueue == "" {
		return nil, fmt.Errorf("temporal worker: task queue must be non-empty: %w", domain.ErrInvariantViolation)
	}
	w := worker.New(c, taskQueue, worker.Options{})
	registerOpenClarionWorker(w, uowFactory, opts...)
	return w, nil
}

func registerOpenClarionWorker(w worker.Worker, uowFactory ports.UnitOfWorkFactory, opts ...ActivityOption) {
	w.RegisterWorkflow(DiagnosisWorkflow)
	w.RegisterWorkflow(DiagnosisRoomWorkflow)
	w.RegisterWorkflow(ReportFanOutWorkflow)
	w.RegisterWorkflow(ReportBatchWorkflow)
	w.RegisterWorkflow(FinalReportWorkflow)
	w.RegisterActivity(NewActivities(uowFactory, opts...))
}
