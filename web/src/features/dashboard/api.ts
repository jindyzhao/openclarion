import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";

export type DashboardSummary = components["schemas"]["DashboardSummary"];
type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchDashboard(
  options: BackendRequestOptions = {},
): Promise<ApiResult<DashboardSummary>> {
  return requestJSON<DashboardSummary>("/api/v1/dashboard", {
    headers: options.headers,
  });
}
