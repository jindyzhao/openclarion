import { requestJSON, type ApiResult } from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";

export type DashboardSummary = components["schemas"]["DashboardSummary"];

export async function fetchDashboard(): Promise<ApiResult<DashboardSummary>> {
  return requestJSON<DashboardSummary>("/api/v1/dashboard");
}
