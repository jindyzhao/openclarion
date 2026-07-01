import { headers } from "next/headers";

import type { RequestJSONOptions } from "./client";
import { diagnosisAuthorizationFromHeaders } from "./diagnosis-session";

type RequestHeaders = {
  get(name: string): string | null;
};

export function diagnosisBackendRequestOptionsFromHeaders(
  incomingHeaders: RequestHeaders,
): Pick<RequestJSONOptions, "headers"> {
  const authorization = diagnosisAuthorizationFromHeaders(incomingHeaders);
  return authorization === null ? {} : { headers: { authorization } };
}

export async function diagnosisBackendRequestOptionsFromIncomingHeaders(): Promise<
  Pick<RequestJSONOptions, "headers">
> {
  return diagnosisBackendRequestOptionsFromHeaders(await headers());
}
