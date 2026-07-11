import { fetchAlerts } from "@/features/alerts/api";
import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import {
  fetchDirectoryDepartments,
  fetchDirectorySyncRuns,
  fetchDirectoryUsers,
  fetchRBACAssignments,
} from "@/features/settings/directory-rbac/api";
import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import { fetchDiagnosisToolTemplates } from "@/features/settings/diagnosis-tool-templates/api";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { SettingsOverview } from "@/features/settings/overview/settings-overview";
import { fetchReportWorkflowPolicies } from "@/features/settings/report-workflow-policies/api";
import { fetchReportWorkflowSchedules } from "@/features/settings/report-workflow-schedules/api";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

export default async function SettingsPage() {
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [
    alerts,
    alertSources,
    groupingPolicies,
    diagnosisToolTemplates,
    notificationChannels,
    workflowPolicies,
    workflowSchedules,
    directoryUsers,
    directoryDepartments,
    directorySyncRuns,
    rbacAssignments,
  ] = await Promise.all([
    fetchAlerts(25, backendRequestOptions),
    fetchAlertSourceProfiles(backendRequestOptions),
    fetchGroupingPolicies(backendRequestOptions),
    fetchDiagnosisToolTemplates(backendRequestOptions),
    fetchNotificationChannelProfiles(backendRequestOptions),
    fetchReportWorkflowPolicies(backendRequestOptions),
    fetchReportWorkflowSchedules(backendRequestOptions),
    fetchDirectoryUsers({ limit: 100 }, backendRequestOptions),
    fetchDirectoryDepartments({ limit: 100 }, backendRequestOptions),
    fetchDirectorySyncRuns({ limit: 10 }, backendRequestOptions),
    fetchRBACAssignments(100, backendRequestOptions),
  ]);

  const counts = {
    alertSources: alertSources.ok ? alertSources.data.items.length : null,
    groupingPolicies: groupingPolicies.ok
      ? groupingPolicies.data.items.length
      : null,
    diagnosisToolTemplates: diagnosisToolTemplates.ok
      ? diagnosisToolTemplates.data.items.length
      : null,
    notificationChannels: notificationChannels.ok
      ? notificationChannels.data.items.length
      : null,
    workflowPolicies: workflowPolicies.ok
      ? workflowPolicies.data.items.length
      : null,
    workflowSchedules: workflowSchedules.ok
      ? workflowSchedules.data.items.length
      : null,
    rbacAssignments: rbacAssignments.ok
      ? rbacAssignments.data.items.length
      : null,
  };
  const countValues: Array<number | null> = Object.values(counts);
  const totalConfigured = countValues.reduce<number>(
    (total, count) => total + (count ?? 0),
    0,
  );

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Settings</h1>
          <p>Declarative alert operations configuration and proof readiness.</p>
        </div>
        <div className="status-line">
          {totalConfigured} configuration objects
        </div>
      </section>

      <SettingsOverview
        alerts={alerts.ok ? alerts.data.items : []}
        alertSources={alertSources.ok ? alertSources.data.items : []}
        counts={counts}
        diagnosisToolTemplates={
          diagnosisToolTemplates.ok ? diagnosisToolTemplates.data.items : []
        }
        groupingPolicies={
          groupingPolicies.ok ? groupingPolicies.data.items : []
        }
        notificationChannels={
          notificationChannels.ok ? notificationChannels.data.items : []
        }
        workflowPolicies={
          workflowPolicies.ok ? workflowPolicies.data.items : []
        }
        workflowSchedules={
          workflowSchedules.ok ? workflowSchedules.data.items : []
        }
        directoryUsers={directoryUsers.ok ? directoryUsers.data.items : []}
        directoryDepartments={
          directoryDepartments.ok ? directoryDepartments.data.items : []
        }
        directorySyncRuns={
          directorySyncRuns.ok ? directorySyncRuns.data.items : []
        }
        rbacAssignments={rbacAssignments.ok ? rbacAssignments.data.items : []}
      />
    </>
  );
}
