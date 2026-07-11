import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { AlertSourceSettingsManager } from "@/features/settings/alert-sources/alert-source-settings-view";
import {
  alertSourceLaunchIntentFromSearchParams,
  alertSourceLaunchIntentKey
} from "@/features/settings/alert-sources/format";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type AlertSourceSettingsPageProps = {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function AlertSourceSettingsPage({
  searchParams
}: AlertSourceSettingsPageProps) {
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const result = await fetchAlertSourceProfiles(backendRequestOptions);
  const launchIntent = alertSourceLaunchIntentFromSearchParams(await searchParams);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Alert Sources</h1>
          <p>Prometheus and Alertmanager connection profiles for alert operations.</p>
        </div>
        <div className="status-line">{count} profiles</div>
      </section>

      <AlertSourceSettingsManager
        key={alertSourceLaunchIntentKey(launchIntent)}
        launchIntent={launchIntent}
        result={result}
      />
    </>
  );
}
