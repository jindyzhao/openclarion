import { fetchDashboard } from "@/features/dashboard/api";
import { DashboardView } from "@/features/dashboard/dashboard-view";
import { fetchDiagnosisHandoffs, fetchDiagnosisRooms } from "@/features/diagnosis-room/api";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const backendRequestOptions =
    await diagnosisBackendRequestOptionsFromIncomingHeaders();
  const [result, roomsResult, handoffsResult] = await Promise.all([
    fetchDashboard(backendRequestOptions),
    fetchDiagnosisRooms(5, backendRequestOptions),
    fetchDiagnosisHandoffs(5, backendRequestOptions)
  ]);
  return <DashboardView handoffsResult={handoffsResult} result={result} roomsResult={roomsResult} />;
}
