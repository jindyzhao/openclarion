import { ReportShell } from "@/features/reports/report-shell";
import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { AlertSourceSettingsManager } from "@/features/settings/alert-sources/alert-source-settings-view";

export const dynamic = "force-dynamic";

export default async function AlertSourceSettingsPage() {
  const result = await fetchAlertSourceProfiles();
  const count = result.ok ? result.data.items.length : 0;

  return (
    <ReportShell current="sources">
      <section className="page-heading">
        <div>
          <h1>Alert Sources</h1>
          <p>Prometheus and Alertmanager connection profiles for alert operations.</p>
        </div>
        <div className="status-line">{count} profiles</div>
      </section>

      <AlertSourceSettingsManager result={result} />
    </ReportShell>
  );
}
