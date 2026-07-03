import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import { normalizedDiagnosisAuthStatusResponse } from "@/lib/api/diagnosis-auth-response";
import { diagnosisOIDCBFFReadinessFromEnv } from "@/lib/api/diagnosis-oidc-login";
import { apiResultResponse } from "@/lib/api/route";
import type { components } from "@/lib/api/openapi";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

export async function GET(request: Request) {
  const result = await requestJSON<unknown>(
    "/api/v1/diagnosis/auth/status",
  );
  if (!result.ok) {
    return apiResultResponse(result);
  }
  const status = normalizedDiagnosisAuthStatusResponse(result.data);
  if (status === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "diagnosis auth status response is invalid" },
      { status: 502 },
    );
  }
  const oidcAdvertised =
    status.mode === "oidc" || status.supported_modes?.includes("oidc") === true;
  const oidcBFF = diagnosisOIDCBFFReadinessFromEnv(request, oidcAdvertised);
  return NextResponse.json(
    oidcBFF === undefined ? status : { ...status, oidc_bff: oidcBFF },
  );
}
