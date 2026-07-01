import { describe, expect, it } from "vitest";

import {
  diagnosisAuthorizationFromRequest,
  diagnosisRequestPublicOrigin,
  normalizedDiagnosisSessionToken
} from "./diagnosis-session";

describe("diagnosis session helpers", () => {
  it("prefers an explicit bearer authorization header over the session cookie", () => {
    const request = new Request("https://console.example.com/api/diagnosis/rooms", {
      headers: {
        authorization: "Bearer explicit.token.one",
        cookie: "openclarion_diagnosis_session=session.token.one"
      }
    });

    expect(diagnosisAuthorizationFromRequest(request)).toBe("Bearer explicit.token.one");
  });

  it("builds bearer authorization from a valid session cookie", () => {
    const request = new Request("https://console.example.com/api/diagnosis/rooms", {
      headers: {
        cookie: "other=value; openclarion_diagnosis_session=session.token.one"
      }
    });

    expect(diagnosisAuthorizationFromRequest(request)).toBe("Bearer session.token.one");
  });

  it("rejects malformed session tokens", () => {
    expect(normalizedDiagnosisSessionToken(" session.token.one")).toBeNull();
    expect(normalizedDiagnosisSessionToken("session token one")).toBeNull();
    expect(normalizedDiagnosisSessionToken("session.token")).toBeNull();
  });

  it("honors forwarded https when computing the public origin", () => {
    const request = new Request("http://console.example.com/api/diagnosis/rooms", {
      headers: {
        "x-forwarded-proto": "http, https"
      }
    });

    expect(diagnosisRequestPublicOrigin(request)).toBe("https://console.example.com");
  });
});
