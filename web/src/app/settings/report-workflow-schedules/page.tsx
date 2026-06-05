import { ReportShell } from "@/features/reports/report-shell";
import { fetchReportWorkflowSchedules } from "@/features/settings/report-workflow-schedules/api";
import { ReportWorkflowScheduleSettingsManager } from "@/features/settings/report-workflow-schedules/report-workflow-schedule-settings-view";

export const dynamic = "force-dynamic";

export default async function ReportWorkflowScheduleSettingsPage() {
  const result = await fetchReportWorkflowSchedules();
  const count = result.ok ? result.data.items.length : 0;

  return (
    <ReportShell current="schedules">
      <section className="page-heading">
        <div>
          <h1>Workflow Schedules</h1>
          <p>Server-owned report trigger cadence and explicit enablement.</p>
        </div>
        <div className="status-line">{count} schedules</div>
      </section>

      <ReportWorkflowScheduleSettingsManager result={result} />
    </ReportShell>
  );
}
