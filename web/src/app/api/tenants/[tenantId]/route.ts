import { updateTenantStatus } from "@/features/settings/workspaces/api";
import type { TenantStatusUpdateRequest } from "@/features/settings/workspaces/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import {
  apiResultResponse,
  parsePositiveIntegerRouteParam,
  readRequestJSON,
} from "@/lib/api/route";

type RouteContext = {
  params: Promise<{ tenantId: string }>;
};

export const dynamic = "force-dynamic";

export async function PATCH(request: Request, context: RouteContext) {
  const { tenantId } = await context.params;
  const parsedID = parsePositiveIntegerRouteParam(tenantId, "Tenant ID");
  if (!parsedID.ok) {
    return apiResultResponse(parsedID);
  }
  const body = await readRequestJSON<TenantStatusUpdateRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    updateTenantStatus(parsedID.data, body.data, { headers }),
  );
}
