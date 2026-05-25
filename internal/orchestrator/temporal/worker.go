package temporal

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/openclarion/openclarion/internal/usecases/ports"
)

func NewWorker(c client.Client, uowFactory ports.UnitOfWorkFactory) worker.Worker {
	w := worker.New(c, TaskQueue, worker.Options{})
	w.RegisterWorkflow(DiagnosisWorkflow)
	w.RegisterActivity(NewActivities(uowFactory))
	return w
}
