package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"

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

type tenantMutationClient interface {
	Client() *ent.Client
}

type tenantRelationKey struct {
	entity string
	name   string
}

var tenantReferenceFields = map[tenantRelationKey]string{
	{ent.TypeAlertEvent, "alert_source_profile_id"}:                                ent.TypeAlertSourceProfile,
	{ent.TypeChatSession, "diagnosis_task_id"}:                                     ent.TypeDiagnosisTask,
	{ent.TypeChatSessionApproval, "chat_session_id"}:                               ent.TypeChatSession,
	{ent.TypeChatSessionSummary, "chat_session_id"}:                                ent.TypeChatSession,
	{ent.TypeChatTurn, "chat_session_id"}:                                          ent.TypeChatSession,
	{ent.TypeDiagnosisTask, "evidence_snapshot_id"}:                                ent.TypeEvidenceSnapshot,
	{ent.TypeDiagnosisTaskEvent, "task_id"}:                                        ent.TypeDiagnosisTask,
	{ent.TypeDiagnosisToolTemplate, "alert_source_profile_id"}:                     ent.TypeAlertSourceProfile,
	{ent.TypeEvidenceSnapshot, "alert_group_id"}:                                   ent.TypeAlertGroup,
	{ent.TypeNotificationChannelTestProof, "notification_channel_profile_id"}:      ent.TypeNotificationChannelProfile,
	{ent.TypeReportNotificationDelivery, "final_report_id"}:                        ent.TypeFinalReport,
	{ent.TypeReportNotificationDelivery, "report_notification_channel_profile_id"}: ent.TypeNotificationChannelProfile,
	{ent.TypeReportWorkflowPolicy, "alert_source_profile_id"}:                      ent.TypeAlertSourceProfile,
	{ent.TypeReportWorkflowPolicy, "grouping_policy_id"}:                           ent.TypeGroupingPolicy,
	{ent.TypeReportWorkflowPolicy, "report_notification_channel_profile_id"}:       ent.TypeNotificationChannelProfile,
	{ent.TypeReportWorkflowSchedule, "report_workflow_policy_id"}:                  ent.TypeReportWorkflowPolicy,
	{ent.TypeSubReport, "evidence_snapshot_id"}:                                    ent.TypeEvidenceSnapshot,
}

var tenantReferenceEdges = map[tenantRelationKey]string{
	{ent.TypeAlertEvent, "groups"}:                                         ent.TypeAlertGroup,
	{ent.TypeAlertGroup, "events"}:                                         ent.TypeAlertEvent,
	{ent.TypeAlertGroup, "snapshots"}:                                      ent.TypeEvidenceSnapshot,
	{ent.TypeChatSession, "task"}:                                          ent.TypeDiagnosisTask,
	{ent.TypeChatSession, "turns"}:                                         ent.TypeChatTurn,
	{ent.TypeChatSession, "summaries"}:                                     ent.TypeChatSessionSummary,
	{ent.TypeChatSession, "approvals"}:                                     ent.TypeChatSessionApproval,
	{ent.TypeChatSessionApproval, "session"}:                               ent.TypeChatSession,
	{ent.TypeChatSessionSummary, "session"}:                                ent.TypeChatSession,
	{ent.TypeChatTurn, "session"}:                                          ent.TypeChatSession,
	{ent.TypeDiagnosisTask, "snapshot"}:                                    ent.TypeEvidenceSnapshot,
	{ent.TypeDiagnosisTask, "events"}:                                      ent.TypeDiagnosisTaskEvent,
	{ent.TypeDiagnosisTask, "chat_sessions"}:                               ent.TypeChatSession,
	{ent.TypeDiagnosisTaskEvent, "task"}:                                   ent.TypeDiagnosisTask,
	{ent.TypeEvidenceSnapshot, "group"}:                                    ent.TypeAlertGroup,
	{ent.TypeEvidenceSnapshot, "tasks"}:                                    ent.TypeDiagnosisTask,
	{ent.TypeEvidenceSnapshot, "sub_reports"}:                              ent.TypeSubReport,
	{ent.TypeFinalReport, "sub_reports"}:                                   ent.TypeSubReport,
	{ent.TypeFinalReport, "notification_deliveries"}:                       ent.TypeReportNotificationDelivery,
	{ent.TypeNotificationChannelProfile, "test_proofs"}:                    ent.TypeNotificationChannelTestProof,
	{ent.TypeNotificationChannelTestProof, "notification_channel_profile"}: ent.TypeNotificationChannelProfile,
	{ent.TypeReportNotificationDelivery, "final_report"}:                   ent.TypeFinalReport,
	{ent.TypeSubReport, "snapshot"}:                                        ent.TypeEvidenceSnapshot,
	{ent.TypeSubReport, "final_reports"}:                                   ent.TypeFinalReport,
}

func validateTenantRelations(ctx context.Context, mutation tenantMutation, identity tenancy.Identity) error {
	withClient, ok := mutation.(tenantMutationClient)
	if !ok || withClient.Client() == nil {
		return fmt.Errorf("repository: tenant-owned mutation %T lacks client access: %w", mutation, domain.ErrInvariantViolation)
	}
	client := withClient.Client()
	entityType := mutation.Type()
	for _, fieldName := range mutation.Fields() {
		targetType, referenced := tenantReferenceFields[tenantRelationKey{entityType, fieldName}]
		if !referenced {
			continue
		}
		value, exists := mutation.Field(fieldName)
		if !exists {
			continue
		}
		id, err := tenantRelationID(value)
		if err != nil {
			return fmt.Errorf("repository: %s.%s: %w", entityType, fieldName, err)
		}
		if id == 0 {
			continue
		}
		if err := requireTenantEntityIDs(ctx, client, identity, targetType, []int{id}); err != nil {
			return fmt.Errorf("repository: %s.%s: %w", entityType, fieldName, err)
		}
	}
	if entityType == ent.TypeRetrievalChunk {
		if err := validateRetrievalSourceTenant(ctx, client, mutation); err != nil {
			return err
		}
	}
	for _, edgeName := range append(append([]string(nil), mutation.AddedEdges()...), mutation.RemovedEdges()...) {
		values := append(append([]ent.Value(nil), mutation.AddedIDs(edgeName)...), mutation.RemovedIDs(edgeName)...)
		if len(values) == 0 {
			continue
		}
		if edgeName == "tenant" {
			ids, err := tenantRelationIDs(values)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if id != int(identity.ID) {
					return fmt.Errorf("repository: %s.tenant references tenant %d from tenant %d: %w", entityType, id, identity.ID, domain.ErrPreconditionFailed)
				}
			}
			continue
		}
		targetType, exists := tenantReferenceEdges[tenantRelationKey{entityType, edgeName}]
		if !exists {
			return fmt.Errorf("repository: tenant relation mapping missing for %s.%s: %w", entityType, edgeName, domain.ErrInvariantViolation)
		}
		ids, err := tenantRelationIDs(values)
		if err != nil {
			return fmt.Errorf("repository: %s.%s: %w", entityType, edgeName, err)
		}
		if err := requireTenantEntityIDs(ctx, client, identity, targetType, ids); err != nil {
			return fmt.Errorf("repository: %s.%s: %w", entityType, edgeName, err)
		}
	}
	return nil
}

func validateRetrievalSourceTenant(ctx context.Context, client *ent.Client, mutation tenantMutation) error {
	kindValue, hasKind := mutation.Field("source_kind")
	idValue, hasID := mutation.Field("source_id")
	if !hasKind || !hasID {
		return nil
	}
	kind, ok := kindValue.(string)
	if !ok {
		return fmt.Errorf("repository: RetrievalChunk.source_kind %v is unsupported: %w", kindValue, domain.ErrInvariantViolation)
	}
	id, err := tenantRelationID(idValue)
	if err != nil {
		return fmt.Errorf("repository: RetrievalChunk.source_id: %w", err)
	}
	identity, err := tenancy.Require(ctx)
	if err != nil {
		return err
	}
	var targetType string
	switch domain.RetrievalSourceKind(kind) {
	case domain.RetrievalSourceSubReport:
		targetType = ent.TypeSubReport
	case domain.RetrievalSourceFinalReport:
		targetType = ent.TypeFinalReport
	default:
		return fmt.Errorf("repository: RetrievalChunk.source_kind %v is unsupported: %w", kindValue, domain.ErrInvariantViolation)
	}
	return requireTenantEntityIDs(ctx, client, identity, targetType, []int{id})
}

type tenantMutationWithID interface {
	ID() (int, bool)
}

// tenantMutationTargetVisible preserves not-found semantics for scoped
// update/delete operations. Relation validation is only meaningful when the
// row being mutated is visible in the current tenant.
func tenantMutationTargetVisible(ctx context.Context, mutation tenantMutation, identity tenancy.Identity) (bool, error) {
	if !mutation.Op().Is(ent.OpUpdateOne | ent.OpDeleteOne) {
		return true, nil
	}
	withID, ok := mutation.(tenantMutationWithID)
	if !ok {
		return false, fmt.Errorf("repository: tenant update mutation %T lacks id access: %w", mutation, domain.ErrInvariantViolation)
	}
	id, exists := withID.ID()
	if !exists {
		return true, nil
	}
	withClient, ok := mutation.(tenantMutationClient)
	if !ok || withClient.Client() == nil {
		return false, fmt.Errorf("repository: tenant update mutation %T lacks client access: %w", mutation, domain.ErrInvariantViolation)
	}
	err := requireTenantEntityIDs(ctx, withClient.Client(), identity, mutation.Type(), []int{id})
	if errors.Is(err, domain.ErrPreconditionFailed) {
		return false, nil
	}
	return err == nil, err
}

func tenantRelationIDs(values []ent.Value) ([]int, error) {
	unique := make(map[int]struct{}, len(values))
	for _, value := range values {
		id, err := tenantRelationID(value)
		if err != nil {
			return nil, err
		}
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	ids := make([]int, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, nil
}

func tenantRelationID(value any) (int, error) {
	var id64 int64
	switch value := value.(type) {
	case int:
		id64 = int64(value)
	case int64:
		id64 = value
	default:
		return 0, fmt.Errorf("relation id type %T is unsupported: %w", value, domain.ErrInvariantViolation)
	}
	if id64 < 0 || int64(int(id64)) != id64 {
		return 0, fmt.Errorf("relation id %d is outside supported range: %w", id64, domain.ErrInvariantViolation)
	}
	return int(id64), nil
}

func requireTenantEntityIDs(ctx context.Context, client *ent.Client, identity tenancy.Identity, entityType string, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	var (
		count int
		err   error
	)
	switch entityType {
	case ent.TypeAlertEvent:
		count, err = client.AlertEvent.Query().Where(alertevent.IDIn(ids...)).Count(ctx)
	case ent.TypeAlertGroup:
		count, err = client.AlertGroup.Query().Where(alertgroup.IDIn(ids...)).Count(ctx)
	case ent.TypeAlertSourceProfile:
		count, err = client.AlertSourceProfile.Query().Where(alertsourceprofile.IDIn(ids...)).Count(ctx)
	case ent.TypeChatSession:
		count, err = client.ChatSession.Query().Where(chatsession.IDIn(ids...)).Count(ctx)
	case ent.TypeChatSessionApproval:
		count, err = client.ChatSessionApproval.Query().Where(chatsessionapproval.IDIn(ids...)).Count(ctx)
	case ent.TypeChatSessionSummary:
		count, err = client.ChatSessionSummary.Query().Where(chatsessionsummary.IDIn(ids...)).Count(ctx)
	case ent.TypeChatTurn:
		count, err = client.ChatTurn.Query().Where(chatturn.IDIn(ids...)).Count(ctx)
	case ent.TypeDiagnosisTask:
		count, err = client.DiagnosisTask.Query().Where(diagnosistask.IDIn(ids...)).Count(ctx)
	case ent.TypeDiagnosisTaskEvent:
		count, err = client.DiagnosisTaskEvent.Query().Where(diagnosistaskevent.IDIn(ids...)).Count(ctx)
	case ent.TypeDiagnosisToolTemplate:
		count, err = client.DiagnosisToolTemplate.Query().Where(diagnosistooltemplate.IDIn(ids...)).Count(ctx)
	case ent.TypeDirectoryDepartment:
		count, err = client.DirectoryDepartment.Query().Where(directorydepartment.IDIn(ids...)).Count(ctx)
	case ent.TypeDirectorySyncRun:
		count, err = client.DirectorySyncRun.Query().Where(directorysyncrun.IDIn(ids...)).Count(ctx)
	case ent.TypeDirectoryUser:
		count, err = client.DirectoryUser.Query().Where(directoryuser.IDIn(ids...)).Count(ctx)
	case ent.TypeEvidenceSnapshot:
		count, err = client.EvidenceSnapshot.Query().Where(evidencesnapshot.IDIn(ids...)).Count(ctx)
	case ent.TypeFinalReport:
		count, err = client.FinalReport.Query().Where(finalreport.IDIn(ids...)).Count(ctx)
	case ent.TypeGroupingPolicy:
		count, err = client.GroupingPolicy.Query().Where(groupingpolicy.IDIn(ids...)).Count(ctx)
	case ent.TypeNotificationChannelProfile:
		count, err = client.NotificationChannelProfile.Query().Where(notificationchannelprofile.IDIn(ids...)).Count(ctx)
	case ent.TypeNotificationChannelTestProof:
		count, err = client.NotificationChannelTestProof.Query().Where(notificationchanneltestproof.IDIn(ids...)).Count(ctx)
	case ent.TypeReportNotificationDelivery:
		count, err = client.ReportNotificationDelivery.Query().Where(reportnotificationdelivery.IDIn(ids...)).Count(ctx)
	case ent.TypeReportWorkflowPolicy:
		count, err = client.ReportWorkflowPolicy.Query().Where(reportworkflowpolicy.IDIn(ids...)).Count(ctx)
	case ent.TypeReportWorkflowSchedule:
		count, err = client.ReportWorkflowSchedule.Query().Where(reportworkflowschedule.IDIn(ids...)).Count(ctx)
	case ent.TypeRetrievalChunk:
		count, err = client.RetrievalChunk.Query().Where(retrievalchunk.IDIn(ids...)).Count(ctx)
	case ent.TypeRBACAssignment:
		count, err = client.RBACAssignment.Query().Where(rbacassignment.IDIn(ids...)).Count(ctx)
	case ent.TypeSubReport:
		count, err = client.SubReport.Query().Where(subreport.IDIn(ids...)).Count(ctx)
	case ent.TypeTenant:
		for _, id := range ids {
			if id != int(identity.ID) {
				return fmt.Errorf("tenant id %d does not match operation tenant %d: %w", id, identity.ID, domain.ErrPreconditionFailed)
			}
		}
		return nil
	default:
		return fmt.Errorf("tenant entity lookup missing for %s: %w", entityType, domain.ErrInvariantViolation)
	}
	if err != nil {
		return fmt.Errorf("query %s relation ids: %w", entityType, err)
	}
	if count != len(ids) {
		return fmt.Errorf("%s relation ids are missing or belong to another tenant: %w", entityType, domain.ErrPreconditionFailed)
	}
	return nil
}
