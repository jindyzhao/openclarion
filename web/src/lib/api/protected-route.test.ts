import { describe, expect, it } from "vitest";

import { diagnosisSessionCookieName } from "./diagnosis-session";
import { authorizedBackendResultResponse } from "./protected-route";

describe("protected API route helpers", () => {
  it("forwards only normalized diagnosis authorization headers", async () => {
    const forwardedHeaders: Headers[] = [];

    const response = await authorizedBackendResultResponse(
      new Request("https://console.example.com/api/config/directory/users", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
          "x-extra-private-header": "must-not-forward",
        },
      }),
      async (headers) => {
        forwardedHeaders.push(new Headers(headers));
        return { ok: true, data: { items: [] } };
      },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ items: [] });
    expect(forwardedHeaders).toHaveLength(1);
    const capturedHeaders = forwardedHeaders[0];
    expect(capturedHeaders?.get("authorization")).toBe(
      "Bearer session.token.one",
    );
    expect(capturedHeaders?.get("cookie")).toBeNull();
    expect(capturedHeaders?.get("x-extra-private-header")).toBeNull();
  });

  it("rejects unauthenticated requests before calling the backend", async () => {
    let called = false;

    const response = await authorizedBackendResultResponse(
      new Request("https://console.example.com/api/config/directory/users"),
      async () => {
        called = true;
        return { ok: true, data: { items: [] } };
      },
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "Authentication required.",
    });
    expect(called).toBe(false);
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("expires malformed browser-session cookies on local auth failure", async () => {
    const response = await authorizedBackendResultResponse(
      new Request("https://console.example.com/api/config/directory/users", {
        headers: {
          cookie: `${diagnosisSessionCookieName}=bad-token`,
        },
      }),
      async () => ({ ok: true, data: { items: [] } }),
    );

    expect(response.status).toBe(401);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(setCookie).toContain("HttpOnly");
  });
});
