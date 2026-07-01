import type { components } from "./openapi";

export type DiagnosisAuthCheckResponse = components["schemas"]["DiagnosisAuthCheckResponse"];
export type DiagnosisAuthSessionResponse = components["schemas"]["DiagnosisAuthSessionResponse"];
export type DiagnosisAuthStatusResponse = components["schemas"]["DiagnosisAuthStatusResponse"];

export function normalizedDiagnosisAuthCheckResponse(value: unknown): DiagnosisAuthCheckResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const roles = normalizedRoles(value.roles);
  if (
    typeof value.subject !== "string" ||
    value.subject.trim() === "" ||
    value.subject !== value.subject.trim() ||
    roles === null ||
    !isAuthMode(value.mode) ||
    typeof value.checked_at !== "string" ||
    Number.isNaN(Date.parse(value.checked_at)) ||
    typeof value.role_authorized !== "boolean"
  ) {
    return null;
  }
  return {
    subject: value.subject,
    roles,
    mode: value.mode,
    checked_at: value.checked_at,
    role_authorized: value.role_authorized
  };
}

export function normalizedDiagnosisAuthSessionResponse(value: unknown): DiagnosisAuthSessionResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const checked = normalizedDiagnosisAuthCheckResponse(value);
  if (
    checked === null ||
    typeof value.token !== "string" ||
    value.token.trim() === "" ||
    value.token !== value.token.trim() ||
    typeof value.expires_at !== "string" ||
    Number.isNaN(Date.parse(value.expires_at))
  ) {
    return null;
  }
  return {
    token: value.token,
    ...checked,
    expires_at: value.expires_at
  };
}

export function normalizedDiagnosisAuthStatusResponse(value: unknown): DiagnosisAuthStatusResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  if (typeof value.configured !== "boolean" || !isAuthStatusMode(value.mode)) {
    return null;
  }
  const supportedModes = Array.isArray(value.supported_modes)
    ? value.supported_modes.filter(isAuthMode)
    : [];
  if (Array.isArray(value.supported_modes) && supportedModes.length !== value.supported_modes.length) {
    return null;
  }
  return {
    configured: value.configured,
    mode: value.mode,
    supported_modes: supportedModes
  };
}

function normalizedRoles(value: unknown): string[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const roles: string[] = [];
  for (const role of value) {
    if (role !== "owner" && role !== "admin") {
      return null;
    }
    roles.push(role);
  }
  return roles;
}

function isAuthMode(value: unknown): value is "oidc" | "unknown" {
  return value === "oidc" || value === "unknown";
}

function isAuthStatusMode(value: unknown): value is "oidc" | "unknown" | "none" {
  return value === "oidc" || value === "unknown" || value === "none";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
