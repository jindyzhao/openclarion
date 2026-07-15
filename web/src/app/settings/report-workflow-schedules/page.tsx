import { getTranslations } from "next-intl/server";

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
  const t = await getTranslations("SettingsPages");
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
          <h1>{t("workflowSchedulesTitle")}</h1>
          <p>{t("workflowSchedulesSubtitle")}</p>
        </div>
        <div className="status-line">{t("schedules", { count })}</div>
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
