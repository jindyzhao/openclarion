import type { ApiResult } from "./client";
import {
  diagnosisAuthorizationFromRequest,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "./diagnosis-session";
import { apiResultResponse } from "./route";

type ProtectedResponseOptions = {
  preserveSessionOnForbidden?: boolean;
};

export function diagnosisAuthorizationHeaders(
  request: Request,
): ApiResult<HeadersInit> {
  const authorization = diagnosisAuthorizationFromRequest(request);
  if (authorization === null) {
    return {
      ok: false,
      error: { message: "Authentication required.", status: 401 },
    };
  }
  return { ok: true, data: { authorization } };
}

export function protectedApiResultResponse<T>(
  request: Request,
  result: ApiResult<T>,
  successStatus = 200,
  options: ProtectedResponseOptions = {},
) {
  const response = apiResultResponse(result, successStatus);
  if (
    !result.ok &&
    !(options.preserveSessionOnForbidden && result.error.status === 403)
  ) {
    expireDiagnosisSessionCookieOnAuthFailure(
      response,
      request,
      result.error.status,
    );
  }
  return response;
}

export async function authorizedBackendResultResponse<T>(
  request: Request,
  action: (headers: HeadersInit) => Promise<ApiResult<T>>,
  successStatus = 200,
  options: ProtectedResponseOptions = {},
) {
  const headers = diagnosisAuthorizationHeaders(request);
  if (!headers.ok) {
    return protectedApiResultResponse(request, headers);
  }
  return protectedApiResultResponse(
    request,
    await action(headers.data),
    successStatus,
    options,
  );
}
