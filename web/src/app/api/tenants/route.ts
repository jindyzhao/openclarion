import {
  createTenant,
  fetchTenants,
} from "@/features/settings/workspaces/api";
import type { TenantCreateRequest } from "@/features/settings/workspaces/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchTenants({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<TenantCreateRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createTenant(body.data, { headers }),
    201,
    { preserveSessionOnForbidden: true },
  );
}
