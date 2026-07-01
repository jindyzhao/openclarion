import type { components } from "@/lib/api/openapi";
import { normalizedDiagnosisSessionToken } from "@/lib/api/diagnosis-session";
import type { DiagnosisOIDCBFFReadiness } from "@/lib/api/diagnosis-oidc-login";

export type DiagnosisAuthCheckResponse =
  components["schemas"]["DiagnosisAuthCheckResponse"];
export type DiagnosisAuthSessionResponse =
  components["schemas"]["DiagnosisAuthSessionResponse"];
export type DiagnosisAuthStatusResponse =
  components["schemas"]["DiagnosisAuthStatusResponse"] & {
    oidc_bff?: DiagnosisOIDCBFFReadiness;
  };
export type DiagnosisWSTicketResponse =
  components["schemas"]["DiagnosisWSTicketResponse"];
type DiagnosisAuthRole = "owner" | "admin";

export function normalizedDiagnosisAuthCheckResponse(
  value: unknown,
): DiagnosisAuthCheckResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const subject = value.subject;
  const roles = value.roles;
  const mode = value.mode;
  const checkedAt = value.checked_at;
  const roleAuthorized = value.role_authorized;
  if (
    !isCleanText(subject) ||
    !isDiagnosisAuthMode(mode) ||
    !Array.isArray(roles) ||
    !roles.every(isDiagnosisAuthRole) ||
    typeof roleAuthorized !== "boolean" ||
    !isValidDateTime(checkedAt)
  ) {
    return null;
  }
  return {
    checked_at: checkedAt,
    mode,
    role_authorized: roleAuthorized,
    roles,
    subject,
  };
}

export function normalizedDiagnosisAuthSessionResponse(
  value: unknown,
): DiagnosisAuthSessionResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const token = normalizedDiagnosisSessionToken(
    typeof value.token === "string" ? value.token : "",
  );
  const subject = value.subject;
  const roles = value.roles;
  const mode = value.mode;
  const checkedAt = value.checked_at;
  const expiresAt = value.expires_at;
  const roleAuthorized = value.role_authorized;
  if (
    token === null ||
    !isCleanText(subject) ||
    !isDiagnosisAuthMode(mode) ||
    !Array.isArray(roles) ||
    !roles.every(isDiagnosisAuthRole) ||
    typeof roleAuthorized !== "boolean" ||
    !isValidDateTime(checkedAt) ||
    !isValidDateTime(expiresAt)
  ) {
    return null;
  }
  return {
    checked_at: checkedAt,
    expires_at: expiresAt,
    mode,
    role_authorized: roleAuthorized,
    roles,
    subject,
    token,
  };
}

export function normalizedDiagnosisAuthStatusResponse(
  value: unknown,
): DiagnosisAuthStatusResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const configured = value.configured;
  const mode = value.mode;
  if (typeof configured !== "boolean" || !isDiagnosisAuthStatusMode(mode)) {
    return null;
  }
  const supportedModes = normalizedOptionalArray(
    value.supported_modes,
    isDiagnosisAuthMode,
  );
  if (supportedModes === null) {
    return null;
  }
  const roleMapping = normalizedAuthRoleMapping(value.role_mapping);
  if (roleMapping === null) {
    return null;
  }
  const transportPolicy = normalizedAuthTransportPolicy(value.transport_policy);
  if (transportPolicy === null) {
    return null;
  }
  const oidcBFF = normalizedOIDCBFFReadiness(value.oidc_bff);
  if (oidcBFF === null) {
    return null;
  }
  return {
    configured,
    mode,
    ...(supportedModes === undefined
      ? {}
      : { supported_modes: supportedModes }),
    ...(roleMapping === undefined ? {} : { role_mapping: roleMapping }),
    ...(transportPolicy === undefined
      ? {}
      : { transport_policy: transportPolicy }),
    ...(oidcBFF === undefined ? {} : { oidc_bff: oidcBFF }),
  };
}

export function normalizedDiagnosisWSTicketResponse(
  value: unknown,
): DiagnosisWSTicketResponse | null {
  if (!isRecord(value)) {
    return null;
  }
  const ticket = value.ticket;
  const sessionID = value.session_id;
  const expiresAt = value.expires_at;
  if (
    !isURLSafeToken(ticket) ||
    !isCleanText(sessionID) ||
    !isValidDateTime(expiresAt)
  ) {
    return null;
  }
  return {
    expires_at: expiresAt,
    session_id: sessionID,
    ticket,
  };
}

function normalizedAuthRoleMapping(
  value: unknown,
): DiagnosisAuthStatusResponse["role_mapping"] | null {
  if (value === undefined) {
    return undefined;
  }
  if (!isRecord(value)) {
    return null;
  }
  const ownerMappingCount = value.owner_mapping_count;
  const adminMappingCount = value.admin_mapping_count;
  const defaultRoles = normalizedOptionalArray(
    value.default_roles,
    isDiagnosisAuthRole,
  );
  const configured = value.configured;
  if (
    !isNonNegativeInteger(ownerMappingCount) ||
    !isNonNegativeInteger(adminMappingCount) ||
    defaultRoles === null ||
    defaultRoles === undefined ||
    typeof configured !== "boolean"
  ) {
    return null;
  }
  return {
    admin_mapping_count: adminMappingCount,
    configured,
    default_roles: defaultRoles,
    owner_mapping_count: ownerMappingCount,
  };
}

function normalizedAuthTransportPolicy(
  value: unknown,
): DiagnosisAuthStatusResponse["transport_policy"] | null {
  if (value === undefined) {
    return undefined;
  }
  if (!isRecord(value)) {
    return null;
  }
  const security = value.security;
  if (!isDiagnosisAuthTransportSecurity(security)) {
    return null;
  }
  return { security };
}

function normalizedOIDCBFFReadiness(
  value: unknown,
): DiagnosisAuthStatusResponse["oidc_bff"] | null {
  if (value === undefined) {
    return undefined;
  }
  if (!isRecord(value)) {
    return null;
  }
  const clientAuthMethod = value.client_auth_method;
  const missing = normalizedOptionalArray(value.missing, isOIDCBFFMissingKey);
  if (
    (clientAuthMethod !== "auto" &&
      clientAuthMethod !== "invalid" &&
      clientAuthMethod !== "client_secret_basic" &&
      clientAuthMethod !== "client_secret_post" &&
      clientAuthMethod !== "none") ||
    typeof value.browser_session_signing_key_configured !== "boolean" ||
    typeof value.client_id_configured !== "boolean" ||
    typeof value.client_secret_configured !== "boolean" ||
    typeof value.configured !== "boolean" ||
    typeof value.issuer_configured !== "boolean" ||
    missing === null ||
    missing === undefined ||
    typeof value.pkce_enabled !== "boolean" ||
    typeof value.redirect_url_configured !== "boolean" ||
    typeof value.scopes_include_openid !== "boolean" ||
    typeof value.state_signing_key_configured !== "boolean" ||
    (value.status !== "ready" && value.status !== "blocked")
  ) {
    return null;
  }
  return {
    browser_session_signing_key_configured:
      value.browser_session_signing_key_configured,
    client_auth_method: clientAuthMethod,
    client_id_configured: value.client_id_configured,
    client_secret_configured: value.client_secret_configured,
    configured: value.configured,
    issuer_configured: value.issuer_configured,
    missing,
    pkce_enabled: value.pkce_enabled,
    redirect_url_configured: value.redirect_url_configured,
    scopes_include_openid: value.scopes_include_openid,
    state_signing_key_configured: value.state_signing_key_configured,
    status: value.status,
  };
}

function isOIDCBFFMissingKey(
  value: unknown,
): value is DiagnosisOIDCBFFReadiness["missing"][number] {
  return (
    value === "client_auth_method" ||
    value === "client_id" ||
    value === "client_secret" ||
    value === "email_scope" ||
    value === "issuer" ||
    value === "openid_scope" ||
    value === "pkce" ||
    value === "profile_scope" ||
    value === "session_signing_key" ||
    value === "state_signing_key"
  );
}

function isDiagnosisAuthTransportSecurity(
  value: unknown,
): value is NonNullable<
  DiagnosisAuthStatusResponse["transport_policy"]
>["security"] {
  return (
    value === "tls" || value === "start_tls" || value === "insecure_plaintext"
  );
}

function normalizedOptionalArray<T>(
  value: unknown,
  predicate: (item: unknown) => item is T,
): T[] | undefined | null {
  if (value === undefined) {
    return undefined;
  }
  if (!Array.isArray(value) || !value.every(predicate)) {
    return null;
  }
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isDiagnosisAuthMode(
  value: unknown,
): value is DiagnosisAuthCheckResponse["mode"] {
  return (
    value === "ldap" ||
    value === "static" ||
    value === "oidc" ||
    value === "unknown"
  );
}

function isDiagnosisAuthStatusMode(
  value: unknown,
): value is DiagnosisAuthStatusResponse["mode"] {
  return isDiagnosisAuthMode(value) || value === "none";
}

function isDiagnosisAuthRole(value: unknown): value is DiagnosisAuthRole {
  return value === "owner" || value === "admin";
}

function isNonNegativeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && typeof value === "number" && value >= 0;
}

function isCleanText(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.trim() === value &&
    value !== "" &&
    !/[\u0000-\u001f\u007f]/u.test(value)
  );
}

function isURLSafeToken(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.trim() === value &&
    value !== "" &&
    /^[A-Za-z0-9_-]+$/u.test(value)
  );
}

function isValidDateTime(value: unknown): value is string {
  if (typeof value !== "string" || value.trim() !== value || value === "") {
    return false;
  }
  return !Number.isNaN(new Date(value).getTime());
}
