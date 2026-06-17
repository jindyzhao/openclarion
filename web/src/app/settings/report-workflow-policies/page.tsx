import { ReportShell } from "@/features/reports/report-shell";
import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { fetchReportWorkflowPolicies } from "@/features/settings/report-workflow-policies/api";
import { ReportWorkflowPolicySettingsManager } from "@/features/settings/report-workflow-policies/report-workflow-policy-settings-view";

export const dynamic = "force-dynamic";

export default async function ReportWorkflowPolicySettingsPage() {
  const [result, alertSourcesResult, groupingPoliciesResult, notificationChannelsResult] = await Promise.all([
    fetchReportWorkflowPolicies(),
    fetchAlertSourceProfiles(),
    fetchGroupingPolicies(),
    fetchNotificationChannelProfiles()
  ]);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <ReportShell current="workflow">
      <section className="page-heading">
        <div>
          <h1>Workflow Policies</h1>
          <p>Report workflow bindings and explicit enablement.</p>
        </div>
        <div className="status-line">{count} policies</div>
      </section>

      <ReportWorkflowPolicySettingsManager
        alertSourcesResult={alertSourcesResult}
        groupingPoliciesResult={groupingPoliciesResult}
        notificationChannelsResult={notificationChannelsResult}
        result={result}
      />
    </ReportShell>
  );
}
