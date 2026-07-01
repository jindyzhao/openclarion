import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

describe("diagnosis ws-ticket route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;
  const originalWSBaseURL = process.env.OPENCLARION_BROWSER_WS_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    delete process.env.OPENCLARION_BROWSER_WS_BASE_URL;
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json({ session_id: "session-1", ticket: "ticket-1" }, { status: 201 }))
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    restoreEnv("OPENCLARION_BROWSER_WS_BASE_URL", originalWSBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards only the bearer header and generated request body to the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
          "x-extra-secret": "must-not-forward"
        },
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(201);
    await expect(response.json()).resolves.toEqual({
      session_id: "session-1",
      ticket: "ticket-1",
      websocket_url: "wss://console.example.com/ws/diagnosis?session_id=session-1&ticket=ticket-1"
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe("https://api.example.com/api/v1/diagnosis/ws-ticket");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ session_id: "session-1" }));

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("uses the configured browser WebSocket base URL when provided", async () => {
    process.env.OPENCLARION_BROWSER_WS_BASE_URL = "http://ws.example.com/base";

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        headers: { authorization: "Bearer token-1" },
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(201);
    const body = (await response.json()) as { websocket_url: string };
    expect(body.websocket_url).toBe("ws://ws.example.com/ws/diagnosis?session_id=session-1&ticket=ticket-1");
  });

  it("uses the diagnosis session cookie when Authorization is absent", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        headers: {
          cookie: "openclarion_diagnosis_session=session.token.one"
        },
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(201);
    const fetchMock = vi.mocked(fetch);
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
  });

  it("rejects missing bearer authorization before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({ error: "authorization is required" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("returns a bounded JSON error for invalid WebSocket deployment configuration", async () => {
    process.env.OPENCLARION_BROWSER_WS_BASE_URL = "ftp://ws.example.com";

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        headers: { authorization: "Bearer token-1" },
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(500);
    await expect(response.json()).resolves.toEqual({ error: "diagnosis WebSocket URL is not configured" });
  });

  it("rejects userinfo-bearing WebSocket deployment configuration", async () => {
    process.env.OPENCLARION_BROWSER_WS_BASE_URL = "https://operator:secret@ws.example.com";

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        headers: { authorization: "Bearer token-1" },
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(500);
    await expect(response.json()).resolves.toEqual({ error: "diagnosis WebSocket URL is not configured" });
  });

  it("rejects state-bearing WebSocket deployment configuration", async () => {
    process.env.OPENCLARION_BROWSER_WS_BASE_URL = "wss://ws.example.com?token=secret#diagnosis";

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/ws-ticket", {
        method: "POST",
        headers: { authorization: "Bearer token-1" },
        body: JSON.stringify({ session_id: "session-1" })
      })
    );

    expect(response.status).toBe(500);
    await expect(response.json()).resolves.toEqual({ error: "diagnosis WebSocket URL is not configured" });
  });
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
