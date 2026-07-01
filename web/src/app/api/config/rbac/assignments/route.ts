import {
  fetchRBACAssignments,
  upsertRBACAssignment,
} from "@/features/settings/directory-rbac/api";
import type { RBACAssignmentWriteRequest } from "@/features/settings/directory-rbac/types";
import type { ApiResult } from "@/lib/api/client";
import {
  diagnosisAuthorizationHeaders,
  protectedApiResultResponse,
} from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const headers = diagnosisAuthorizationHeaders(request);
  if (!headers.ok) {
    return apiResultResponse(headers);
  }
  const limit = listLimit(new URL(request.url).searchParams.get("limit"));
  if (!limit.ok) {
    return apiResultResponse(limit);
  }
  return protectedApiResultResponse(
    request,
    await fetchRBACAssignments(limit.data ?? 100, { headers: headers.data }),
  );
}

export async function POST(request: Request) {
  const headers = diagnosisAuthorizationHeaders(request);
  if (!headers.ok) {
    return apiResultResponse(headers);
  }
  const body = await readRequestJSON<RBACAssignmentWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return protectedApiResultResponse(
    request,
    await upsertRBACAssignment(body.data, { headers: headers.data }),
  );
}

function listLimit(value: string | null): ApiResult<number | undefined> {
  if (value === null || value.trim() === "") {
    return { ok: true, data: undefined };
  }
  const parsed = Number(value);
  if (Number.isSafeInteger(parsed) && parsed > 0) {
    return { ok: true, data: parsed };
  }
  return {
    ok: false,
    error: {
      message: "RBAC assignment list limit must be a positive integer.",
      status: 400,
    },
  };
}
