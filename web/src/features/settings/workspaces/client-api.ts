"use client";

import { requestSameOriginJSON } from "@/lib/api/browser";
import type { ApiResult } from "@/lib/api/client";

import type {
  Tenant,
  TenantCreateRequest,
  TenantListResponse,
  TenantMembership,
  TenantMembershipListResponse,
  TenantMembershipWriteRequest,
  TenantStatusUpdateRequest,
} from "./types";

export function refreshTenants(): Promise<ApiResult<TenantListResponse>> {
  return requestSameOriginJSON<TenantListResponse>("/api/tenants");
}

export function submitTenant(
  body: TenantCreateRequest,
): Promise<ApiResult<Tenant>> {
  return requestSameOriginJSON<Tenant>("/api/tenants", {
    method: "POST",
    body,
  });
}

export function submitTenantStatus(
  tenantID: number,
  body: TenantStatusUpdateRequest,
): Promise<ApiResult<Tenant>> {
  return requestSameOriginJSON<Tenant>(`/api/tenants/${tenantID}`, {
    method: "PATCH",
    body,
  });
}

export function refreshTenantMemberships(
  tenantID: number,
): Promise<ApiResult<TenantMembershipListResponse>> {
  return requestSameOriginJSON<TenantMembershipListResponse>(
    `/api/tenants/${tenantID}/memberships`,
  );
}

export function submitTenantMembership(
  tenantID: number,
  body: TenantMembershipWriteRequest,
): Promise<ApiResult<TenantMembership>> {
  return requestSameOriginJSON<TenantMembership>(
    `/api/tenants/${tenantID}/memberships`,
    { method: "PUT", body },
  );
}
