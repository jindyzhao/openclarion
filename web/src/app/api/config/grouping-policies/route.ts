import {
  createGroupingPolicy,
  fetchGroupingPolicies
} from "@/features/settings/grouping-policies/api";
import type { GroupingPolicyWriteRequest } from "@/features/settings/grouping-policies/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchGroupingPolicies({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<GroupingPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createGroupingPolicy(body.data, { headers }),
    201,
  );
}
