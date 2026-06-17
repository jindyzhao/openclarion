import { ReportShell } from "@/features/reports/report-shell";
import { fetchAlertSourceProfiles } from "@/features/settings/alert-sources/api";
import { fetchDiagnosisToolTemplates } from "@/features/settings/diagnosis-tool-templates/api";
import { DiagnosisToolTemplateSettingsManager } from "@/features/settings/diagnosis-tool-templates/diagnosis-tool-template-settings-view";

export const dynamic = "force-dynamic";

export default async function DiagnosisToolTemplateSettingsPage() {
  const [result, alertSourcesResult] = await Promise.all([
    fetchDiagnosisToolTemplates(),
    fetchAlertSourceProfiles()
  ]);
  const count = result.ok ? result.data.items.length : 0;

  return (
    <ReportShell current="tools">
      <section className="page-heading">
        <div>
          <h1>Diagnosis Tools</h1>
          <p>Operator-reviewed evidence collection templates for diagnosis rooms.</p>
        </div>
        <div className="status-line">{count} templates</div>
      </section>

      <DiagnosisToolTemplateSettingsManager alertSourcesResult={alertSourcesResult} result={result} />
    </ReportShell>
  );
}
