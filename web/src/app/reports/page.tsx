import { fetchReports } from "@/features/reports/api";
import { ReportListView } from "@/features/reports/report-list-view";

export const dynamic = "force-dynamic";

export default async function ReportsPage() {
  const result = await fetchReports();

  return <ReportListView result={result} />;
}
