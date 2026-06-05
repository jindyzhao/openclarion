import { NextResponse } from "next/server";

import type { ApiResult } from "./client";
import type { components } from "./openapi";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export async function readRequestJSON<T>(request: Request): Promise<ApiResult<T>> {
  try {
    return { ok: true, data: (await request.json()) as T };
  } catch (error) {
    return {
      ok: false,
      error: { message: error instanceof Error ? error.message : "Request body must be valid JSON.", status: 400 }
    };
  }
}

export function apiResultResponse<T>(result: ApiResult<T>, successStatus = 200): NextResponse<T | ErrorResponse> {
  if (result.ok) {
    return NextResponse.json(result.data, { status: successStatus });
  }
  return NextResponse.json(
    { error: result.error.message },
    { status: result.error.status ?? 502 }
  );
}
