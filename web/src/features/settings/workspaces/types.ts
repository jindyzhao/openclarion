import type { components } from "@/lib/api/openapi";

export type Tenant = components["schemas"]["Tenant"];
export type TenantCreateRequest = components["schemas"]["TenantCreateRequest"];
export type TenantListResponse = components["schemas"]["TenantListResponse"];
export type TenantMembership = components["schemas"]["TenantMembership"];
export type TenantMembershipListResponse =
  components["schemas"]["TenantMembershipListResponse"];
export type TenantMembershipWriteRequest =
  components["schemas"]["TenantMembershipWriteRequest"];
export type TenantStatusUpdateRequest =
  components["schemas"]["TenantStatusUpdateRequest"];
