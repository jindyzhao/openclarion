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

export function parsePositiveIntegerRouteParam(value: string, label: string): ApiResult<number> {
  const trimmed = value.trim();
  if (!/^[0-9]+$/.test(trimmed)) {
    return positiveIntegerRouteParamError(label);
  }
  const parsed = Number(trimmed);
  if (!Number.isSafeInteger(parsed) || parsed < 1) {
    return positiveIntegerRouteParamError(label);
  }
  return { ok: true, data: parsed };
}

function positiveIntegerRouteParamError(label: string): ApiResult<number> {
  return {
    ok: false,
    error: { message: `${label} must be a positive integer.`, status: 400 }
  };
}
