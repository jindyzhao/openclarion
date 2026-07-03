import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("diagnosis room close-unavailable route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            session_id: "diagnosis-session-orphaned-workflow",
            chat_session_id: 409,
            diagnosis_task_id: 309,
            evidence_snapshot_id: 9001,
            workflow_id: "diagnosis-room-diagnosis-session-orphaned-workflow",
            run_id: "run-orphaned-9001",
            task_status: "cancelled",
            room_status: "closed",
            turn_count: 1,
            started_at: "2026-05-28T06:03:30Z",
            last_activity_at: "2026-05-28T06:05:00Z",
            closed_at: "2026-05-28T06:05:00Z",
            close_reason: "workflow_unavailable",
            notification_timeline: [],
            created_at: "2026-05-28T06:03:30Z",
            updated_at: "2026-05-28T06:05:00Z",
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

  it("forwards only authorization and close body to the backend", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
        {
          method: "POST",
          headers: {
            authorization: "Bearer token-1",
            "x-extra-secret": "must-not-forward",
          },
          body: JSON.stringify({ reason: "workflow_unavailable" }),
        },
      ),
      {
        params: Promise.resolve({
          sessionId: "diagnosis-session-orphaned-workflow",
        }),
      },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      session_id: "diagnosis-session-orphaned-workflow",
      room_status: "closed",
      close_reason: "workflow_unavailable",
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ reason: "workflow_unavailable" }));

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
        {
          method: "POST",
        },
      ),
      {
        params: Promise.resolve({
          sessionId: "diagnosis-session-orphaned-workflow",
        }),
      },
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("forwards Basic authorization for LDAP diagnosis auth", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
        {
          method: "POST",
          headers: {
            authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
          },
          body: JSON.stringify({ reason: "workflow_unavailable" }),
        },
      ),
      {
        params: Promise.resolve({
          sessionId: "diagnosis-session-orphaned-workflow",
        }),
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

  it("clears the diagnosis session cookie when backend rejects browser-session close", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid session" }, { status: 401 }),
    );

    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
        {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=expired.session.token`,
          },
          body: JSON.stringify({ reason: "workflow_unavailable" }),
        },
      ),
      {
        params: Promise.resolve({
          sessionId: "diagnosis-session-orphaned-workflow",
        }),
      },
    );

    expect(response.status).toBe(401);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(setCookie).toContain("HttpOnly");
  });

  it("does not clear the diagnosis session cookie for explicit LDAP close failures", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid ldap credentials" }, { status: 401 }),
    );

    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
        {
          method: "POST",
          headers: {
            authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
          },
          body: JSON.stringify({ reason: "workflow_unavailable" }),
        },
      ),
      {
        params: Promise.resolve({
          sessionId: "diagnosis-session-orphaned-workflow",
        }),
      },
    );

    expect(response.status).toBe(401);
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("rejects unknown body fields before contacting the backend", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-orphaned-workflow/close-unavailable",
        {
          method: "POST",
          headers: {
            authorization: "Bearer token-1",
          },
          body: JSON.stringify({
            reason: "workflow_unavailable",
            workflow_payload: { secret: "must-not-forward" },
          }),
        },
      ),
      {
        params: Promise.resolve({
          sessionId: "diagnosis-session-orphaned-workflow",
        }),
      },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Request body must be an object with optional string reason.",
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
