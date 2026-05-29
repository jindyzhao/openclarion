package temporal

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

// NewWorker registers OpenClarion workflows and activities on a Temporal worker.
func NewWorker(c client.Client, uowFactory ports.UnitOfWorkFactory, opts ...ActivityOption) worker.Worker {
	w := worker.New(c, TaskQueue, worker.Options{})
	w.RegisterWorkflow(DiagnosisWorkflow)
	w.RegisterWorkflow(DiagnosisRoomWorkflow)
	w.RegisterWorkflow(ReportFanOutWorkflow)
	w.RegisterWorkflow(ReportBatchWorkflow)
	w.RegisterWorkflow(FinalReportWorkflow)
	w.RegisterActivity(NewActivities(uowFactory, opts...))
	return w
}
