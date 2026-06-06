import { ReportShell } from "@/features/reports/report-shell";
import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { SettingsOverview } from "@/features/settings/overview/settings-overview";
import { fetchReportWorkflowPolicies } from "@/features/settings/report-workflow-policies/api";
import { fetchReportWorkflowSchedules } from "@/features/settings/report-workflow-schedules/api";

export const dynamic = "force-dynamic";

export default async function SettingsPage() {
  const [alertSources, groupingPolicies, notificationChannels, workflowPolicies, workflowSchedules] =
    await Promise.all([
      fetchAlertSourceProfiles(),
      fetchGroupingPolicies(),
      fetchNotificationChannelProfiles(),
      fetchReportWorkflowPolicies(),
      fetchReportWorkflowSchedules()
    ]);

  const counts = {
    alertSources: alertSources.ok ? alertSources.data.items.length : null,
    groupingPolicies: groupingPolicies.ok ? groupingPolicies.data.items.length : null,
    notificationChannels: notificationChannels.ok ? notificationChannels.data.items.length : null,
    workflowPolicies: workflowPolicies.ok ? workflowPolicies.data.items.length : null,
    workflowSchedules: workflowSchedules.ok ? workflowSchedules.data.items.length : null
  };
  const countValues: Array<number | null> = Object.values(counts);
  const totalConfigured = countValues.reduce<number>((total, count) => total + (count ?? 0), 0);

  return (
    <ReportShell current="settings">
      <section className="page-heading">
        <div>
          <h1>Settings</h1>
          <p>Declarative alert operations configuration and proof readiness.</p>
        </div>
        <div className="status-line">{totalConfigured} configuration objects</div>
      </section>

      <SettingsOverview counts={counts} />
    </ReportShell>
  );
}
