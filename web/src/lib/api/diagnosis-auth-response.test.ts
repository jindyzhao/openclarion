import { describe, expect, it } from "vitest";

import {
  normalizedDiagnosisAuthCheckResponse,
  normalizedDiagnosisAuthSessionResponse,
} from "./diagnosis-auth-response";

const checkedAt = "2026-07-15T05:30:00Z";

describe("diagnosis auth response normalization", () => {
  it("preserves a validated tenant binding", () => {
    expect(
      normalizedDiagnosisAuthCheckResponse({
        checked_at: checkedAt,
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
        tenant_id: 7,
        tenant_key: "team-seven",
      }),
    ).toEqual({
      checked_at: checkedAt,
      mode: "oidc",
      role_authorized: true,
      roles: ["owner"],
      subject: "operator-1",
      tenant_id: 7,
      tenant_key: "team-seven",
    });

    expect(
      normalizedDiagnosisAuthSessionResponse({
        checked_at: checkedAt,
        expires_at: "2026-07-15T13:30:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
        tenant_id: 7,
        tenant_key: "team-seven",
        token: "session.token.value",
      }),
    ).toMatchObject({ tenant_id: 7, tenant_key: "team-seven" });
  });

  it("rejects missing or malformed tenant bindings", () => {
    const response = {
      checked_at: checkedAt,
      mode: "oidc",
      role_authorized: true,
      roles: ["owner"],
      subject: "operator-1",
      tenant_id: 7,
      tenant_key: "team-seven",
    };

    expect(
      normalizedDiagnosisAuthCheckResponse({ ...response, tenant_id: 0 }),
    ).toBeNull();
    expect(
      normalizedDiagnosisAuthCheckResponse({
        ...response,
        tenant_key: "Team Seven",
      }),
    ).toBeNull();
    expect(
      normalizedDiagnosisAuthCheckResponse({
        checked_at: checkedAt,
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
        tenant_id: 7,
      }),
    ).toBeNull();
  });
});
