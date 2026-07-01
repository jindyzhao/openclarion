import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { GET } from "./route";

describe("diagnosis room route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            session_id: "diagnosis-session-1",
            chat_session_id: 202,
            diagnosis_task_id: 101,
            evidence_snapshot_id: 7,
            workflow_id: "diagnosis-room-diagnosis-session-1",
            run_id: "run-1",
            task_status: "running",
            room_status: "open",
            turn_count: 1,
            notification_timeline: [],
            created_at: "2026-06-20T00:00:00Z",
            updated_at: "2026-06-20T00:01:00Z",
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

  it("proxies exact room lookup with authorization only", async () => {
    const response = await GET(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-1",
        {
          headers: {
            authorization: "Bearer browser-token",
            "x-extra-secret": "must-not-forward",
          },
        },
      ),
      {
        params: Promise.resolve({ sessionId: "diagnosis-session-1" }),
      },
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      session_id: "diagnosis-session-1",
      evidence_snapshot_id: 7,
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/rooms/diagnosis-session-1",
    );
    expect(init.method).toBe("GET");
    const headers = init.headers as Headers;
    expect(headers.get("accept")).toBe("application/json");
    expect(headers.get("authorization")).toBe("Bearer browser-token");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("encodes the session id path segment", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/rooms/session%2F1", {
        headers: { authorization: "Bearer browser-token" },
      }),
      {
        params: Promise.resolve({ sessionId: "session/1" }),
      },
    );

    expect(response.status).toBe(200);
    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/rooms/session%2F1",
    );
  });

  it("rejects blank session id before contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/rooms/%20", {
        headers: { authorization: "Bearer browser-token" },
      }),
      {
        params: Promise.resolve({ sessionId: " " }),
      },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "session_id is required",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await GET(
      new Request(
        "https://console.example.com/api/diagnosis/rooms/diagnosis-session-1",
      ),
      {
        params: Promise.resolve({ sessionId: "diagnosis-session-1" }),
      },
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: "authorization is required",
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
