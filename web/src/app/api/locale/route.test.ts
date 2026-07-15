import { describe, expect, it } from "vitest";

import { appLocaleCookieName } from "@/i18n/config";

import { PUT } from "./route";

describe("locale preference route", () => {
  it("stores a validated locale in a hardened cookie", async () => {
    const response = await PUT(
      new Request("http://next.internal/api/locale", {
        method: "PUT",
        headers: {
          "content-type": "application/json",
          "x-forwarded-proto": "https",
        },
        body: JSON.stringify({ locale: "zh-CN" }),
      }),
    );

    expect(response.status).toBe(204);
    expect(response.cookies.get(appLocaleCookieName)?.value).toBe("zh-CN");
    const cookie = response.headers.get("set-cookie");
    expect(cookie).toContain("Max-Age=31536000");
    expect(cookie).toContain("Path=/");
    expect(cookie).toContain("HttpOnly");
    expect(cookie).toContain("SameSite=lax");
    expect(cookie).toContain("Secure");
  });

  it("rejects unsupported locales without setting a cookie", async () => {
    const response = await PUT(
      new Request("https://console.example.com/api/locale", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ locale: "fr" }),
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Locale must be en or zh-CN.",
    });
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("rejects malformed JSON", async () => {
    const response = await PUT(
      new Request("https://console.example.com/api/locale", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: "{",
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Request body must be valid JSON.",
    });
  });
});
