import { fetchDashboard } from "@/features/dashboard/api";
import { DashboardView } from "@/features/dashboard/dashboard-view";

export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const result = await fetchDashboard();
  return <DashboardView result={result} />;
}
