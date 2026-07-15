import { getTranslations } from "next-intl/server";

import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { fetchDiagnosisToolTemplates } from "@/features/settings/diagnosis-tool-templates/api";
import {
  fetchDirectoryDepartments,
  fetchDirectorySyncRuns,
  fetchDirectoryUsers,
  fetchRBACAssignments,
} from "@/features/settings/directory-rbac/api";
import { DirectoryRBACSettingsManager } from "@/features/settings/directory-rbac/directory-rbac-settings-view";
import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { fetchReportWorkflowPolicies } from "@/features/settings/report-workflow-policies/api";
import { fetchReportWorkflowSchedules } from "@/features/settings/report-workflow-schedules/api";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

export default async function DirectoryRBACSettingsPage() {
  const t = await getTranslations("SettingsPages");
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [
    users,
    departments,
    syncRuns,
    assignments,
    alertSources,
    groupingPolicies,
    workflowPolicies,
    workflowSchedules,
    notificationChannels,
    diagnosisToolTemplates,
  ] = await Promise.all([
    fetchDirectoryUsers({ limit: 100 }, backendRequestOptions),
    fetchDirectoryDepartments({ limit: 100 }, backendRequestOptions),
    fetchDirectorySyncRuns({ limit: 10 }, backendRequestOptions),
    fetchRBACAssignments(100, backendRequestOptions),
    fetchAlertSourceProfiles(backendRequestOptions),
    fetchGroupingPolicies(backendRequestOptions),
    fetchReportWorkflowPolicies(backendRequestOptions),
    fetchReportWorkflowSchedules(backendRequestOptions),
    fetchNotificationChannelProfiles(backendRequestOptions),
    fetchDiagnosisToolTemplates(backendRequestOptions),
  ]);
  const userCount = users.ok ? users.data.items.length : 0;
  const departmentCount = departments.ok ? departments.data.items.length : 0;
  const assignmentCount = assignments.ok ? assignments.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("directoryTitle")}</h1>
          <p>{t("directorySubtitle")}</p>
        </div>
        <div className="status-line">
          {t("directoryCounts", { assignments: assignmentCount, departments: departmentCount, users: userCount })}
        </div>
      </section>

      <DirectoryRBACSettingsManager
        alertSourcesResult={alertSources}
        assignmentsResult={assignments}
        departmentsResult={departments}
        diagnosisToolTemplatesResult={diagnosisToolTemplates}
        groupingPoliciesResult={groupingPolicies}
        notificationChannelsResult={notificationChannels}
        syncRunsResult={syncRuns}
        usersResult={users}
        workflowPoliciesResult={workflowPolicies}
        workflowSchedulesResult={workflowSchedules}
      />
    </>
  );
}
