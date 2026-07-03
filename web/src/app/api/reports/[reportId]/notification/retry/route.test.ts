import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

describe("report notification retry route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            retry_state: "sent",
            delivery: {
              id: 41,
              idempotency_key: "final_report:11/notification/final",
              notification_purpose: "final",
              report_notification_channel_profile_id: 2,
              status: "delivered",
              provider_message_id: "msg-final-41",
              provider_status: "accepted",
              delivered_at: "2026-06-21T10:45:00Z",
              created_at: "2026-06-21T10:44:00Z",
              updated_at: "2026-06-21T10:45:00Z",
            },
          },
          { status: 200 },
        ),
      ),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards the final retry body and caller authorization to the backend", async () => {
    const body = {
      notification_purpose: "final",
      report_notification_channel_profile_id: 2,
    };

    const response = await POST(
      new Request("https://console.example.com/api/reports/11/notification/retry", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
          "x-extra-header": "must-not-forward",
        },
        body: JSON.stringify(body),
      }),
      { params: Promise.resolve({ reportId: "11" }) },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      retry_state: "sent",
      delivery: {
        id: 41,
        notification_purpose: "final",
        report_notification_channel_profile_id: 2,
        provider_message_id: "msg-final-41",
      },
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/reports/11/notification/retry",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(body));

    const headers = init.headers as Headers;
    expect(headers.get("accept")).toBe("application/json");
    expect(headers.get("content-type")).toBe("application/json");
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-header")).toBeNull();
  });

  it("forwards the local session cookie as backend authorization", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/reports/11/notification/retry", {
        method: "POST",
        headers: {
          cookie: "openclarion_diagnosis_session=session.token.one",
        },
        body: JSON.stringify({ notification_purpose: "final" }),
      }),
      { params: Promise.resolve({ reportId: "11" }) },
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
    expect(headers.get("cookie")).toBeNull();
  });

  it("accepts an empty retry body and forwards the default override object", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/reports/11/notification/retry", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
      }),
      { params: Promise.resolve({ reportId: "11" }) },
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(init.body).toBe("{}");
  });

  it("accepts a whitespace-only retry body and forwards the default override object", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/reports/11/notification/retry", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: "  \n",
      }),
      { params: Promise.resolve({ reportId: "11" }) },
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(init.body).toBe("{}");
  });

  it("rejects invalid report IDs before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/reports/not-a-number/notification/retry", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: JSON.stringify({ notification_purpose: "final" }),
      }),
      { params: Promise.resolve({ reportId: "not-a-number" }) },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Report ID must be a positive integer.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects invalid JSON before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/reports/11/notification/retry", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: "{",
      }),
      { params: Promise.resolve({ reportId: "11" }) },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toMatchObject({
      error: expect.stringContaining("JSON"),
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/reports/11/notification/retry", {
        method: "POST",
        body: JSON.stringify({ notification_purpose: "final" }),
      }),
      { params: Promise.resolve({ reportId: "11" }) },
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
