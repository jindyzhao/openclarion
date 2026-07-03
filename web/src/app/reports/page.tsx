import { fetchReports } from "@/features/reports/api";
import { ReportListView } from "@/features/reports/report-list-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

export default async function ReportsPage() {
  const result = await fetchReports(
    await diagnosisBackendRequestOptionsFromIncomingHeaders(),
  );

  return <ReportListView result={result} />;
}
