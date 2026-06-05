import {
  createGroupingPolicy,
  fetchGroupingPolicies
} from "@/features/settings/grouping-policies/api";
import type { GroupingPolicyWriteRequest } from "@/features/settings/grouping-policies/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET() {
  return apiResultResponse(await fetchGroupingPolicies());
}

export async function POST(request: Request) {
  const body = await readRequestJSON<GroupingPolicyWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await createGroupingPolicy(body.data), 201);
}
