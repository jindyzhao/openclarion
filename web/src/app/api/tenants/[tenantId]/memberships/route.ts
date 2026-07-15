import {
  fetchTenantMemberships,
  setTenantMembership,
} from "@/features/settings/workspaces/api";
import type { TenantMembershipWriteRequest } from "@/features/settings/workspaces/types";
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

export async function GET(request: Request, context: RouteContext) {
  const tenantID = await parsedTenantID(context);
  if (!tenantID.ok) {
    return apiResultResponse(tenantID);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    fetchTenantMemberships(tenantID.data, { headers }),
  );
}

export async function PUT(request: Request, context: RouteContext) {
  const tenantID = await parsedTenantID(context);
  if (!tenantID.ok) {
    return apiResultResponse(tenantID);
  }
  const body = await readRequestJSON<TenantMembershipWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(request, (headers) =>
    setTenantMembership(tenantID.data, body.data, { headers }),
  );
}

async function parsedTenantID(context: RouteContext) {
  const { tenantId } = await context.params;
  return parsePositiveIntegerRouteParam(tenantId, "Tenant ID");
}
