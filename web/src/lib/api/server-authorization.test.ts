import { describe, expect, it } from "vitest";

import { diagnosisSessionCookieName } from "./diagnosis-session";
import { diagnosisBackendRequestOptionsFromHeaders } from "./server-authorization";

describe("server authorization request options", () => {
  it("omits backend headers when the incoming request is anonymous", () => {
    expect(diagnosisBackendRequestOptionsFromHeaders(new Headers())).toEqual({});
  });

  it("forwards explicit incoming Authorization headers", () => {
    expect(
      diagnosisBackendRequestOptionsFromHeaders(
        new Headers({
          authorization: "Basic b3BlcmF0b3I6cGFzcw==",
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        }),
      ),
    ).toEqual({
      headers: { authorization: "Basic b3BlcmF0b3I6cGFzcw==" },
    });
  });

  it("falls back to the diagnosis session cookie as Bearer auth", () => {
    expect(
      diagnosisBackendRequestOptionsFromHeaders(
        new Headers({
          cookie: `other=1; ${diagnosisSessionCookieName}=session.token.one`,
        }),
      ),
    ).toEqual({
      headers: { authorization: "Bearer session.token.one" },
    });
  });
});
