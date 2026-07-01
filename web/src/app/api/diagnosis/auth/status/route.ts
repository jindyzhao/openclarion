import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import { normalizedDiagnosisAuthStatusResponse } from "@/lib/api/diagnosis-auth-response";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse } from "@/lib/api/route";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

export async function GET() {
  const result = await requestJSON<unknown>("/api/v1/diagnosis/auth/status");
  if (!result.ok) {
    return apiResultResponse(result);
  }
  const status = normalizedDiagnosisAuthStatusResponse(result.data);
  if (status === null) {
    return NextResponse.json<ErrorResponse>({ error: "diagnosis auth status response is invalid" }, { status: 502 });
  }
  return NextResponse.json(status);
}
