import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

type DiagnosisRoomCreateRequest = components["schemas"]["DiagnosisRoomCreateRequest"];
type DiagnosisRoomCreateResponse = components["schemas"]["DiagnosisRoomCreateResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const authorization = request.headers.get("authorization")?.trim() ?? "";
  if (!/^Bearer [^ \t\r\n]+$/.test(authorization)) {
    return NextResponse.json<ErrorResponse>({ error: "bearer authorization is required" }, { status: 401 });
  }

  const body = await readRequestJSON<DiagnosisRoomCreateRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  return apiResultResponse(
    await requestJSON<DiagnosisRoomCreateResponse>("/api/v1/diagnosis/rooms", {
      method: "POST",
      headers: { authorization },
      body: body.data
    }),
    201
  );
}
