package temporal

import (
	"context"
	"testing"
	"time"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"

	"github.com/openclarion/openclarion/internal/tenancy"
)

func TestTenantContextPropagatesThroughWorkflowAndActivity(t *testing.T) {
	t.Parallel()

	identity := tenancy.Identity{ID: 7, Key: "team-seven"}
	tenantCtx, err := tenancy.WithTenant(context.Background(), identity)
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
	header := &commonpb.Header{Fields: map[string]*commonpb.Payload{}}
	if err := (tenantContextPropagator{}).Inject(
		tenantCtx,
		headerWriter{header: header},
	); err != nil {
		t.Fatalf("Inject: %v", err)
	}

	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetContextPropagators(TenantContextPropagators())
	env.SetHeader(header)
	env.RegisterWorkflow(tenantPropagationWorkflow)
	env.RegisterActivity(tenantPropagationActivity)
	env.ExecuteWorkflow(tenantPropagationWorkflow)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var got tenancy.Identity
	if err := env.GetWorkflowResult(&got); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	if got != identity {
		t.Fatalf("propagated identity = %+v, want %+v", got, identity)
	}
}

func TestTenantContextMissingHeaderDefaultsForExistingHistories(t *testing.T) {
	t.Parallel()

	ctx, err := (tenantContextPropagator{}).Extract(context.Background(), headerReader{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	identity, ok := tenancy.FromContext(ctx)
	if !ok || identity != tenancy.DefaultIdentity() {
		t.Fatalf("identity = %+v, %t", identity, ok)
	}
}

func tenantPropagationWorkflow(ctx workflow.Context) (tenancy.Identity, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: time.Minute})
	var identity tenancy.Identity
	err := workflow.ExecuteActivity(ctx, tenantPropagationActivity).Get(ctx, &identity)
	return identity, err
}

func tenantPropagationActivity(ctx context.Context) (tenancy.Identity, error) {
	return tenancy.Require(ctx)
}

type headerWriter struct {
	header *commonpb.Header
}

func (w headerWriter) Set(key string, value *commonpb.Payload) {
	w.header.Fields[key] = value
}

type headerReader struct {
	header *commonpb.Header
}

func (r headerReader) Get(key string) (*commonpb.Payload, bool) {
	if r.header == nil {
		return nil, false
	}
	value, ok := r.header.Fields[key]
	return value, ok
}

func (r headerReader) ForEachKey(handler func(string, *commonpb.Payload) error) error {
	if r.header == nil {
		return nil
	}
	for key, value := range r.header.Fields {
		if err := handler(key, value); err != nil {
			return err
		}
	}
	return nil
}
