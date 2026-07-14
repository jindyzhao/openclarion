package repository

import (
	"context"
	"fmt"
	"sync"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/openclarion/openclarion/internal/domain"
	"github.com/openclarion/openclarion/internal/persistence/ent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertevent"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertgroup"
	"github.com/openclarion/openclarion/internal/persistence/ent/alertsourceprofile"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatsession"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatsessionapproval"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatsessionsummary"
	"github.com/openclarion/openclarion/internal/persistence/ent/chatturn"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistask"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistaskevent"
	"github.com/openclarion/openclarion/internal/persistence/ent/diagnosistooltemplate"
	"github.com/openclarion/openclarion/internal/persistence/ent/directorydepartment"
	"github.com/openclarion/openclarion/internal/persistence/ent/directorysyncrun"
	"github.com/openclarion/openclarion/internal/persistence/ent/directoryuser"
	"github.com/openclarion/openclarion/internal/persistence/ent/evidencesnapshot"
	"github.com/openclarion/openclarion/internal/persistence/ent/finalreport"
	"github.com/openclarion/openclarion/internal/persistence/ent/groupingpolicy"
	"github.com/openclarion/openclarion/internal/persistence/ent/notificationchannelprofile"
	"github.com/openclarion/openclarion/internal/persistence/ent/notificationchanneltestproof"
	"github.com/openclarion/openclarion/internal/persistence/ent/rbacassignment"
	"github.com/openclarion/openclarion/internal/persistence/ent/reportnotificationdelivery"
	"github.com/openclarion/openclarion/internal/persistence/ent/reportworkflowpolicy"
	"github.com/openclarion/openclarion/internal/persistence/ent/reportworkflowschedule"
	"github.com/openclarion/openclarion/internal/persistence/ent/retrievalchunk"
	"github.com/openclarion/openclarion/internal/persistence/ent/subreport"
	"github.com/openclarion/openclarion/internal/tenancy"
)

var tenantScopedClients sync.Map

// configureTenantScope installs one query interceptor and mutation hook per
// Ent client. Transactions inherit both through the generated client config.
func configureTenantScope(client *ent.Client) {
	if client == nil {
		return
	}
	if _, loaded := tenantScopedClients.LoadOrStore(client, struct{}{}); loaded {
		return
	}
	client.Intercept(ent.TraverseFunc(filterTenantQuery))
	client.Use(tenantMutationHook())
}

func filterTenantQuery(ctx context.Context, query ent.Query) error {
	switch query.(type) {
	case *ent.TenantQuery, *ent.TenantMembershipQuery, *ent.DiagnosisAuthTicketQuery:
		return nil
	}
	identity, err := tenancy.Require(ctx)
	if err != nil {
		return err
	}
	tenantID := int(identity.ID)
	switch query := query.(type) {
	case *ent.AlertEventQuery:
		query.Where(alertevent.TenantIDEQ(tenantID))
	case *ent.AlertGroupQuery:
		query.Where(alertgroup.TenantIDEQ(tenantID))
	case *ent.AlertSourceProfileQuery:
		query.Where(alertsourceprofile.TenantIDEQ(tenantID))
	case *ent.ChatSessionQuery:
		query.Where(chatsession.TenantIDEQ(tenantID))
	case *ent.ChatSessionApprovalQuery:
		query.Where(chatsessionapproval.TenantIDEQ(tenantID))
	case *ent.ChatSessionSummaryQuery:
		query.Where(chatsessionsummary.TenantIDEQ(tenantID))
	case *ent.ChatTurnQuery:
		query.Where(chatturn.TenantIDEQ(tenantID))
	case *ent.DiagnosisTaskQuery:
		query.Where(diagnosistask.TenantIDEQ(tenantID))
	case *ent.DiagnosisTaskEventQuery:
		query.Where(diagnosistaskevent.TenantIDEQ(tenantID))
	case *ent.DiagnosisToolTemplateQuery:
		query.Where(diagnosistooltemplate.TenantIDEQ(tenantID))
	case *ent.DirectoryDepartmentQuery:
		query.Where(directorydepartment.TenantIDEQ(tenantID))
	case *ent.DirectorySyncRunQuery:
		query.Where(directorysyncrun.TenantIDEQ(tenantID))
	case *ent.DirectoryUserQuery:
		query.Where(directoryuser.TenantIDEQ(tenantID))
	case *ent.EvidenceSnapshotQuery:
		query.Where(evidencesnapshot.TenantIDEQ(tenantID))
	case *ent.FinalReportQuery:
		query.Where(finalreport.TenantIDEQ(tenantID))
	case *ent.GroupingPolicyQuery:
		query.Where(groupingpolicy.TenantIDEQ(tenantID))
	case *ent.NotificationChannelProfileQuery:
		query.Where(notificationchannelprofile.TenantIDEQ(tenantID))
	case *ent.NotificationChannelTestProofQuery:
		query.Where(notificationchanneltestproof.TenantIDEQ(tenantID))
	case *ent.RBACAssignmentQuery:
		query.Where(rbacassignment.TenantIDEQ(tenantID))
	case *ent.ReportNotificationDeliveryQuery:
		query.Where(reportnotificationdelivery.TenantIDEQ(tenantID))
	case *ent.ReportWorkflowPolicyQuery:
		query.Where(reportworkflowpolicy.TenantIDEQ(tenantID))
	case *ent.ReportWorkflowScheduleQuery:
		query.Where(reportworkflowschedule.TenantIDEQ(tenantID))
	case *ent.RetrievalChunkQuery:
		query.Where(retrievalchunk.TenantIDEQ(tenantID))
	case *ent.SubReportQuery:
		query.Where(subreport.TenantIDEQ(tenantID))
	default:
		return fmt.Errorf("repository: query %T lacks tenant scope mapping: %w", query, domain.ErrInvariantViolation)
	}
	return nil
}

type tenantMutation interface {
	ent.Mutation
	SetTenantID(int)
	TenantID() (int, bool)
	WhereP(...func(*entsql.Selector))
}

func tenantMutationHook() ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			if globalTenantEntity(mutation.Type()) {
				return next.Mutate(ctx, mutation)
			}
			identity, err := tenancy.Require(ctx)
			if err != nil {
				return nil, err
			}
			scoped, ok := mutation.(tenantMutation)
			if !ok {
				return nil, fmt.Errorf("repository: tenant-owned mutation %T lacks tenant contract: %w", mutation, domain.ErrInvariantViolation)
			}
			tenantID := int(identity.ID)
			if assigned, exists := scoped.TenantID(); exists && assigned != tenantID {
				return nil, fmt.Errorf("repository: mutation tenant %d does not match operation tenant %d: %w", assigned, tenantID, domain.ErrPreconditionFailed)
			}
			if mutation.Op().Is(ent.OpCreate) {
				scoped.SetTenantID(tenantID)
			} else {
				scoped.WhereP(entsql.FieldEQ("tenant_id", tenantID))
			}
			visible, err := tenantMutationTargetVisible(ctx, scoped, identity)
			if err != nil {
				return nil, err
			}
			if visible {
				err = validateTenantRelations(ctx, scoped, identity)
			}
			if err != nil {
				return nil, err
			}
			return next.Mutate(ctx, mutation)
		})
	}
}

func globalTenantEntity(entityType string) bool {
	return entityType == ent.TypeTenant ||
		entityType == ent.TypeTenantMembership ||
		entityType == ent.TypeDiagnosisAuthTicket
}
