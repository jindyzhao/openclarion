import { getTranslations } from "next-intl/server";

import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { fetchDiagnosisToolTemplates } from "@/features/settings/diagnosis-tool-templates/api";
import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { fetchReportWorkflowPolicies } from "@/features/settings/report-workflow-policies/api";
import {
  reportWorkflowPolicyLaunchIntentFromSearchParams,
  reportWorkflowPolicyLaunchIntentKey
} from "@/features/settings/report-workflow-policies/format";
import { ReportWorkflowPolicySettingsManager } from "@/features/settings/report-workflow-policies/report-workflow-policy-settings-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type ReportWorkflowPolicySettingsPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function ReportWorkflowPolicySettingsPage({
  searchParams
}: ReportWorkflowPolicySettingsPageProps) {
  const t = await getTranslations("SettingsPages");
  const launchIntent = reportWorkflowPolicyLaunchIntentFromSearchParams(await searchParams);
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [
    result,
    alertSourcesResult,
    groupingPoliciesResult,
    notificationChannelsResult,
    diagnosisToolTemplatesResult
  ] = await Promise.all([
    fetchReportWorkflowPolicies(backendRequestOptions),
    fetchAlertSourceProfiles(backendRequestOptions),
    fetchGroupingPolicies(backendRequestOptions),
    fetchNotificationChannelProfiles(backendRequestOptions),
    fetchDiagnosisToolTemplates(backendRequestOptions)
  ]);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("workflowPoliciesTitle")}</h1>
          <p>{t("workflowPoliciesSubtitle")}</p>
        </div>
        <div className="status-line">{t("policies", { count })}</div>
      </section>

      <ReportWorkflowPolicySettingsManager
        alertSourcesResult={alertSourcesResult}
        diagnosisToolTemplatesResult={diagnosisToolTemplatesResult}
        groupingPoliciesResult={groupingPoliciesResult}
        key={reportWorkflowPolicyLaunchIntentKey(launchIntent)}
        launchIntent={launchIntent}
        notificationChannelsResult={notificationChannelsResult}
        result={result}
      />
    </>
  );
}
