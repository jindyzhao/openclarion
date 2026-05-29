import { fetchReportDetail } from "@/features/reports/api";
import { ReportDetailView } from "@/features/reports/report-detail-view";

export const dynamic = "force-dynamic";

type ReportDetailPageProps = {
  params: Promise<{ reportId: string }>;
};

export default async function ReportDetailPage({ params }: ReportDetailPageProps) {
  const { reportId } = await params;
  const result = await fetchReportDetail(reportId);

  return <ReportDetailView reportId={reportId} result={result} />;
}
