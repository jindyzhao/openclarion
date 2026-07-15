import type { components } from "@/lib/api/openapi";

export type Tenant = components["schemas"]["Tenant"];
export type TenantListResponse = components["schemas"]["TenantListResponse"];
export type TenantMembership = components["schemas"]["TenantMembership"];
export type TenantMembershipListResponse =
  components["schemas"]["TenantMembershipListResponse"];

export function normalizedTenantListResponse(
  value: unknown,
): TenantListResponse | null {
  if (
    !isRecord(value) ||
    !Array.isArray(value.items) ||
    value.items.length > 500
  ) {
    return null;
  }
  const items: Tenant[] = [];
  const ids = new Set<number>();
  const keys = new Set<string>();
  for (const item of value.items) {
    const tenant = normalizedTenant(item);
    if (tenant === null || ids.has(tenant.id) || keys.has(tenant.key)) {
      return null;
    }
    ids.add(tenant.id);
    keys.add(tenant.key);
    items.push(tenant);
  }
  return { items };
}

export function normalizedTenant(value: unknown): Tenant | null {
  if (!isRecord(value)) {
    return null;
  }
  const { id, key, name, status, created_at: createdAt, updated_at: updatedAt } =
    value;
  if (
    !Number.isSafeInteger(id) ||
    (id as number) <= 0 ||
    typeof key !== "string" ||
    key.length === 0 ||
    key.length > 63 ||
    !/^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/.test(key) ||
    !isCleanText(name, 120) ||
    (status !== "active" && status !== "disabled") ||
    !isDateTime(createdAt) ||
    !isDateTime(updatedAt)
  ) {
    return null;
  }
  return {
    id: id as number,
    key,
    name,
    status,
    created_at: createdAt,
    updated_at: updatedAt,
  };
}

export function normalizedTenantMembershipListResponse(
  value: unknown,
): TenantMembershipListResponse | null {
  if (
    !isRecord(value) ||
    !Array.isArray(value.items) ||
    value.items.length > 500
  ) {
    return null;
  }
  const items: TenantMembership[] = [];
  const ids = new Set<number>();
  const subjects = new Set<string>();
  for (const item of value.items) {
    const membership = normalizedTenantMembership(item);
    if (
      membership === null ||
      ids.has(membership.id) ||
      subjects.has(membership.subject)
    ) {
      return null;
    }
    ids.add(membership.id);
    subjects.add(membership.subject);
    items.push(membership);
  }
  return { items };
}

export function normalizedTenantMembership(
  value: unknown,
): TenantMembership | null {
  if (!isRecord(value)) {
    return null;
  }
  const {
    id,
    tenant_id: tenantID,
    subject,
    role,
    enabled,
    created_by: createdBy,
    created_at: createdAt,
    updated_at: updatedAt,
  } = value;
  if (
    !isPositiveInteger(id) ||
    !isPositiveInteger(tenantID) ||
    !isCleanText(subject, 256) ||
    (role !== "owner" && role !== "member") ||
    typeof enabled !== "boolean" ||
    !isCleanText(createdBy, 256) ||
    !isDateTime(createdAt) ||
    !isDateTime(updatedAt)
  ) {
    return null;
  }
  return {
    id,
    tenant_id: tenantID,
    subject,
    role,
    enabled,
    created_by: createdBy,
    created_at: createdAt,
    updated_at: updatedAt,
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isCleanText(value: unknown, maxLength: number): value is string {
  if (typeof value !== "string") {
    return false;
  }
  const length = Array.from(value).length;
  return (
    value.trim() === value &&
    length > 0 &&
    length <= maxLength &&
    !/[\0\r\n]/.test(value)
  );
}

function isPositiveInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) > 0;
}

function isDateTime(value: unknown): value is string {
  if (typeof value !== "string" || value.trim() !== value) {
    return false;
  }
  const match =
    /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(?:Z|[+-]\d{2}:\d{2})$/.exec(
      value,
    );
  if (match === null || Number.isNaN(Date.parse(value))) {
    return false;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const monthDays = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <= monthDays[month - 1]! &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59
  );
}
