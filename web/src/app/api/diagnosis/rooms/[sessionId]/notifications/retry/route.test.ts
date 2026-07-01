import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("diagnosis room notification retry route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            retry_state: "sent",
            notification: {
              event_kind: "diagnosis_room.final_ready_notification_sent",
              notification_channel_profile_id: 2,
              provider_status: "delivered",
              provider_message_id: "wecom-retry-1",
              confidence: "high",
              requires_human_review: true,
              occurred_at: "2026-06-21T11:30:00Z",
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

  it("forwards only authorization and retry body to the backend", async () => {
    const body = {
      event_kind: "diagnosis_room.final_ready_notification_sent",
    };
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/session-1/notifications/retry",
        {
          method: "POST",
          headers: {
            authorization: "Bearer token-1",
            "x-extra-secret": "must-not-forward",
          },
          body: JSON.stringify(body),
        },
      ),
      {
        params: Promise.resolve({ sessionId: "session-1" }),
      },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      retry_state: "sent",
      notification: {
        event_kind: "diagnosis_room.final_ready_notification_sent",
        provider_message_id: "wecom-retry-1",
      },
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/rooms/session-1/notifications/retry",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(body));

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("forwards Basic authorization for LDAP diagnosis auth", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/session-1/notifications/retry",
        {
          method: "POST",
          headers: {
            authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
          },
          body: JSON.stringify({
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
          }),
        },
      ),
      {
        params: Promise.resolve({ sessionId: "session-1" }),
      },
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe(
      "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
    );
  });

  it("clears the diagnosis session cookie when backend rejects browser-session retry", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid session" }, { status: 401 }),
    );

    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/session-1/notifications/retry",
        {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=expired.session.token`,
          },
          body: JSON.stringify({
            event_kind: "diagnosis_room.final_ready_notification_sent",
          }),
        },
      ),
      {
        params: Promise.resolve({ sessionId: "session-1" }),
      },
    );

    expect(response.status).toBe(401);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(setCookie).toContain("HttpOnly");
  });

  it("does not clear the diagnosis session cookie for explicit LDAP retry failures", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid ldap credentials" }, { status: 401 }),
    );

    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/session-1/notifications/retry",
        {
          method: "POST",
          headers: {
            authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
          },
          body: JSON.stringify({
            event_kind: "diagnosis_room.final_ready_notification_sent",
          }),
        },
      ),
      {
        params: Promise.resolve({ sessionId: "session-1" }),
      },
    );

    expect(response.status).toBe(401);
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/session-1/notifications/retry",
        {
          method: "POST",
          body: JSON.stringify({
            event_kind: "diagnosis_room.final_ready_notification_sent",
          }),
        },
      ),
      {
        params: Promise.resolve({ sessionId: "session-1" }),
      },
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects unknown body fields before contacting the backend", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/session-1/notifications/retry",
        {
          method: "POST",
          headers: {
            authorization: "Bearer token-1",
          },
          body: JSON.stringify({
            event_kind: "diagnosis_room.final_ready_notification_sent",
            provider_payload: { secret: "must-not-forward" },
          }),
        },
      ),
      {
        params: Promise.resolve({ sessionId: "session-1" }),
      },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error:
        "Request body must be an object with a supported string event_kind.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
});

function restoreEnv(key: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[key];
    return;
  }
  process.env[key] = value;
}
