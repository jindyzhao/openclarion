import { syncDirectoryProjection } from "@/features/settings/directory-rbac/api";
import type { DirectorySyncRequestBody } from "@/features/settings/directory-rbac/types";
import {
  diagnosisAuthorizationHeaders,
  protectedApiResultResponse,
} from "@/lib/api/protected-route";
import { apiResultResponse, readOptionalRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const headers = diagnosisAuthorizationHeaders(request);
  if (!headers.ok) {
    return apiResultResponse(headers);
  }
  const body = await readOptionalRequestJSON<DirectorySyncRequestBody>(
    request,
    {},
  );
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return protectedApiResultResponse(
    request,
    await syncDirectoryProjection(body.data, { headers: headers.data }),
  );
}
