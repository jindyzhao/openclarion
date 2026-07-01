import { fetchDirectoryDepartments } from "@/features/settings/directory-rbac/api";
import type { ApiResult } from "@/lib/api/client";
import {
  diagnosisAuthorizationHeaders,
  protectedApiResultResponse,
} from "@/lib/api/protected-route";
import { apiResultResponse } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const headers = diagnosisAuthorizationHeaders(request);
  if (!headers.ok) {
    return apiResultResponse(headers);
  }
  const options = directoryListOptions(request.url);
  if (!options.ok) {
    return apiResultResponse(options);
  }
  return protectedApiResultResponse(
    request,
    await fetchDirectoryDepartments(options.data, { headers: headers.data }),
  );
}

function directoryListOptions(
  url: string,
): ApiResult<{ limit?: number; provider?: string }> {
  const searchParams = new URL(url).searchParams;
  const limit = listLimit(searchParams.get("limit"));
  if (!limit.ok) {
    return { ok: false, error: limit.error };
  }
  return {
    ok: true,
    data: {
      limit: limit.data,
      provider: searchParams.get("provider") ?? undefined,
    },
  };
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
      message: "Directory list limit must be a positive integer.",
      status: 400,
    },
  };
}
