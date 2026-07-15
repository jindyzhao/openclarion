import { fetchTenants } from "@/features/settings/workspaces/api";
import { WorkspaceSettingsManager } from "@/features/settings/workspaces/workspace-settings-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";
import { getTranslations } from "next-intl/server";

export const dynamic = "force-dynamic";

export default async function WorkspaceSettingsPage() {
  const t = await getTranslations("SettingsPages");
  const options = await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const result = await fetchTenants(options);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>{t("workspacesTitle")}</h1>
          <p>{t("workspacesSubtitle")}</p>
        </div>
        <div className="status-line">{t("visibleWorkspaces", { count })}</div>
      </section>

      <WorkspaceSettingsManager result={result} />
    </>
  );
}
