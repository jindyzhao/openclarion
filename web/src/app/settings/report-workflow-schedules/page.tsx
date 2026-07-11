import { fetchReportWorkflowPolicies } from "@/features/settings/report-workflow-policies/api";
import { fetchReportWorkflowSchedules } from "@/features/settings/report-workflow-schedules/api";
import {
  reportWorkflowScheduleLaunchIntentFromSearchParams,
  reportWorkflowScheduleLaunchIntentKey
} from "@/features/settings/report-workflow-schedules/format";
import { ReportWorkflowScheduleSettingsManager } from "@/features/settings/report-workflow-schedules/report-workflow-schedule-settings-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type ReportWorkflowScheduleSettingsPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function ReportWorkflowScheduleSettingsPage({
  searchParams
}: ReportWorkflowScheduleSettingsPageProps) {
  const launchIntent = reportWorkflowScheduleLaunchIntentFromSearchParams(await searchParams);
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [result, reportWorkflowPoliciesResult] = await Promise.all([
    fetchReportWorkflowSchedules(backendRequestOptions),
    fetchReportWorkflowPolicies(backendRequestOptions)
  ]);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Workflow Schedules</h1>
          <p>Server-owned report trigger cadence and explicit enablement.</p>
        </div>
        <div className="status-line">{count} schedules</div>
      </section>

      <ReportWorkflowScheduleSettingsManager
        key={reportWorkflowScheduleLaunchIntentKey(launchIntent)}
        launchIntent={launchIntent}
        reportWorkflowPoliciesResult={reportWorkflowPoliciesResult}
        result={result}
      />
    </>
  );
}
