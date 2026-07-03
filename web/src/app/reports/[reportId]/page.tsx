import { fetchDiagnosisRooms } from "@/features/diagnosis-room/api";
import { diagnosisReviewReturnState } from "@/features/diagnosis-room/report-return";
import { fetchReportDetail } from "@/features/reports/api";
import { ReportDetailView } from "@/features/reports/report-detail-view";
import { fetchDirectoryUsers } from "@/features/settings/directory-rbac/api";
import { fetchNotificationChannelProfiles } from "@/features/settings/notification-channels/api";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

type ReportDetailPageProps = {
  params: Promise<{ reportId: string }>;
  searchParams: Promise<Record<string, string | string[] | undefined>>;
};

export default async function ReportDetailPage({ params, searchParams }: ReportDetailPageProps) {
  const { reportId } = await params;
  const query = await searchParams;
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [
    result,
    roomsResult,
    notificationChannelsResult,
    directoryUsersResult,
  ] = await Promise.all([
    fetchReportDetail(reportId, backendRequestOptions),
    fetchDiagnosisRooms(100, backendRequestOptions),
    fetchNotificationChannelProfiles(backendRequestOptions),
    fetchDirectoryUsers({ limit: 100 }, backendRequestOptions),
  ]);

  return (
    <ReportDetailView
      directoryUsersResult={directoryUsersResult}
      diagnosisReviewReturn={diagnosisReviewReturnState(query.diagnosis_review)}
      reportId={reportId}
      result={result}
      roomsResult={roomsResult}
      notificationChannelsResult={notificationChannelsResult}
    />
  );
}
