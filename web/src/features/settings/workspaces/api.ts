import {
  requestJSON,
  type ApiResult,
  type RequestJSONOptions,
} from "@/lib/api/client";
import {
  normalizedTenant,
  normalizedTenantListResponse,
  normalizedTenantMembership,
  normalizedTenantMembershipListResponse,
} from "@/lib/api/tenant-response";

import type {
  Tenant,
  TenantCreateRequest,
  TenantListResponse,
  TenantMembership,
  TenantMembershipListResponse,
  TenantMembershipWriteRequest,
  TenantStatusUpdateRequest,
} from "./types";

type BackendRequestOptions = Pick<RequestJSONOptions, "headers">;

export async function fetchTenants(
  options: BackendRequestOptions = {},
): Promise<ApiResult<TenantListResponse>> {
  return normalizedBackendResult(
    requestJSON<unknown>("/api/v1/tenants", { headers: options.headers }),
    normalizedTenantListResponse,
    "Tenant list response is invalid.",
  );
}

export async function createTenant(
  body: TenantCreateRequest,
  options: BackendRequestOptions = {},
): Promise<ApiResult<Tenant>> {
  return normalizedBackendResult(
    requestJSON<unknown>("/api/v1/tenants", {
      method: "POST",
      body,
      headers: options.headers,
    }),
    normalizedTenant,
    "Tenant response is invalid.",
  );
}

export async function updateTenantStatus(
  tenantID: number,
  body: TenantStatusUpdateRequest,
  options: BackendRequestOptions = {},
): Promise<ApiResult<Tenant>> {
  if (!positiveID(tenantID)) {
    return invalidTenantIDResult();
  }
  return normalizedBackendResult(
    requestJSON<unknown>(`/api/v1/tenants/${tenantID}`, {
      method: "PATCH",
      body,
      headers: options.headers,
    }),
    normalizedTenant,
    "Tenant response is invalid.",
  );
}

export async function fetchTenantMemberships(
  tenantID: number,
  options: BackendRequestOptions = {},
): Promise<ApiResult<TenantMembershipListResponse>> {
  if (!positiveID(tenantID)) {
    return invalidTenantIDResult();
  }
  return normalizedBackendResult(
    requestJSON<unknown>(`/api/v1/tenants/${tenantID}/memberships`, {
      headers: options.headers,
    }),
    normalizedTenantMembershipListResponse,
    "Tenant membership list response is invalid.",
  );
}

export async function setTenantMembership(
  tenantID: number,
  body: TenantMembershipWriteRequest,
  options: BackendRequestOptions = {},
): Promise<ApiResult<TenantMembership>> {
  if (!positiveID(tenantID)) {
    return invalidTenantIDResult();
  }
  return normalizedBackendResult(
    requestJSON<unknown>(`/api/v1/tenants/${tenantID}/memberships`, {
      method: "PUT",
      body,
      headers: options.headers,
    }),
    normalizedTenantMembership,
    "Tenant membership response is invalid.",
  );
}

async function normalizedBackendResult<T>(
  request: Promise<ApiResult<unknown>>,
  normalize: (value: unknown) => T | null,
  invalidMessage: string,
): Promise<ApiResult<T>> {
  const result = await request;
  if (!result.ok) {
    return result;
  }
  const normalized = normalize(result.data);
  return normalized === null
    ? { ok: false, error: { message: invalidMessage, status: 502 } }
    : { ok: true, data: normalized };
}

function positiveID(value: number): boolean {
  return Number.isSafeInteger(value) && value > 0;
}

function invalidTenantIDResult<T>(): ApiResult<T> {
  return {
    ok: false,
    error: { message: "Tenant ID must be a positive integer.", status: 400 },
  };
}
