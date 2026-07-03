import { fetchAlerts } from "@/features/alerts/api";
import { AlertsView } from "@/features/alerts/alerts-view";
import { diagnosisBackendRequestOptionsFromIncomingHeaders } from "@/lib/api/server-authorization";

export const dynamic = "force-dynamic";

export default async function AlertsPage() {
  const alertsResult = await fetchAlerts(
    100,
    await diagnosisBackendRequestOptionsFromIncomingHeaders(),
  );

  return <AlertsView alertsResult={alertsResult} />;
}
