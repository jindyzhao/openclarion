import { getTranslations } from "next-intl/server";

import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { fetchDiagnosisToolTemplates } from "@/features/settings/diagnosis-tool-templates/api";
import { DiagnosisToolTemplateSettingsManager } from "@/features/settings/diagnosis-tool-templates/diagnosis-tool-template-settings-view";
import {
  diagnosisToolTemplateLaunchIntentFromSearchParams,
  diagnosisToolTemplateLaunchIntentKey
} from "@/features/settings/diagnosis-tool-templates/format";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type DiagnosisToolTemplateSettingsPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function DiagnosisToolTemplateSettingsPage({
  searchParams
}: DiagnosisToolTemplateSettingsPageProps) {
  const t = await getTranslations("SettingsPages");
  const launchIntent = diagnosisToolTemplateLaunchIntentFromSearchParams(await searchParams);
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [result, alertSourcesResult] = await Promise.all([
    fetchDiagnosisToolTemplates(backendRequestOptions),
    fetchAlertSourceProfiles(backendRequestOptions)
  ]);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("diagnosisToolsTitle")}</h1>
          <p>{t("diagnosisToolsSubtitle")}</p>
        </div>
        <div className="status-line">{t("templates", { count })}</div>
      </section>

      <DiagnosisToolTemplateSettingsManager
        alertSourcesResult={alertSourcesResult}
        key={diagnosisToolTemplateLaunchIntentKey(launchIntent)}
        launchIntent={launchIntent}
        result={result}
      />
    </>
  );
}
