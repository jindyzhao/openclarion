import {
  createAlertSourceProfile,
  fetchAlertSourceProfiles
} from "@/features/settings/alert-sources/api";
import type { AlertSourceProfileWriteRequest } from "@/features/settings/alert-sources/types";
import { authorizedBackendResultResponse } from "@/lib/api/protected-route";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  return authorizedBackendResultResponse(request, (headers) =>
    fetchAlertSourceProfiles({ headers }),
  );
}

export async function POST(request: Request) {
  const body = await readRequestJSON<AlertSourceProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return authorizedBackendResultResponse(
    request,
    (headers) => createAlertSourceProfile(body.data, { headers }),
    201,
  );
}
