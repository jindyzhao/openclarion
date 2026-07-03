import { describe, expect, it } from "vitest";

import {
  diagnosisAuthorizationHeader,
  diagnosisAuthorizationHeaders,
  normalizedDiagnosisAuthorization,
} from "./authorization";

describe("diagnosis authorization", () => {
  it("normalizes bearer tokens without accepting whitespace-bearing tokens", () => {
    expect(
      normalizedDiagnosisAuthorization({
        mode: "bearer",
        token: " token-1 ",
      }),
    ).toEqual({ mode: "bearer", token: "token-1" });

    expect(
      normalizedDiagnosisAuthorization({
        mode: "bearer",
        token: "token 1",
      }),
    ).toBeNull();
  });

  it("encodes LDAP credentials as Basic authorization", () => {
    expect(
      diagnosisAuthorizationHeader({
        mode: "basic",
        username: " operator-1 ",
        password: "ldap-password",
      }),
    ).toBe(`Basic ${btoa("operator-1:ldap-password")}`);
  });

  it("rejects malformed LDAP credentials before building headers", () => {
    expect(
      diagnosisAuthorizationHeader({
        mode: "basic",
        username: "operator 1",
        password: "ldap-password",
      }),
    ).toBeNull();
    expect(
      diagnosisAuthorizationHeader({
        mode: "basic",
        username: "operator-1",
        password: "ldap\npassword",
      }),
    ).toBeNull();
  });

  it("uses no explicit Authorization header for browser sessions", () => {
    expect(normalizedDiagnosisAuthorization({ mode: "session" })).toEqual({
      mode: "session",
    });
    expect(diagnosisAuthorizationHeader({ mode: "session" })).toBe("");
    expect(diagnosisAuthorizationHeaders({ mode: "session" })).toEqual({});
  });
});
