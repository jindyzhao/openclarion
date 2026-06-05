import { ReportShell } from "@/features/reports/report-shell";
import { fetchGroupingPolicies } from "@/features/settings/grouping-policies/api";
import { GroupingPolicySettingsManager } from "@/features/settings/grouping-policies/grouping-policy-settings-view";

export const dynamic = "force-dynamic";

export default async function GroupingPolicySettingsPage() {
  const result = await fetchGroupingPolicies();
  const count = result.ok ? result.data.items.length : 0;

  return (
    <ReportShell current="grouping">
      <section className="page-heading">
        <div>
          <h1>Grouping Policies</h1>
          <p>Alert grouping rules used before report workflow binding.</p>
        </div>
        <div className="status-line">{count} policies</div>
      </section>

      <GroupingPolicySettingsManager result={result} />
    </ReportShell>
  );
}
