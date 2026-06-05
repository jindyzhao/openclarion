import {
  createAlertSourceProfile,
  fetchAlertSourceProfiles
} from "@/features/settings/alert-sources/api";
import type { AlertSourceProfileWriteRequest } from "@/features/settings/alert-sources/types";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

export const dynamic = "force-dynamic";

export async function GET() {
  return apiResultResponse(await fetchAlertSourceProfiles());
}

export async function POST(request: Request) {
  const body = await readRequestJSON<AlertSourceProfileWriteRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }
  return apiResultResponse(await createAlertSourceProfile(body.data), 201);
}
