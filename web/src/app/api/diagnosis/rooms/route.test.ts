import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { GET, POST } from "./route";

describe("diagnosis rooms route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            session_id: "diagnosis-session-1",
            evidence_snapshot_id: 7,
            diagnosis_task_id: 101,
            chat_session_id: 202,
            workflow_id: "diagnosis-room-diagnosis-session-1",
            run_id: "run-1"
          },
          { status: 201 }
        )
      )
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards only the authorization header and create body to the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
          "x-extra-secret": "must-not-forward"
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 })
      })
    );

    expect(response.status).toBe(201);
    await expect(response.json()).resolves.toEqual({
      session_id: "diagnosis-session-1",
      evidence_snapshot_id: 7,
      diagnosis_task_id: 101,
      chat_session_id: 202,
      workflow_id: "diagnosis-room-diagnosis-session-1",
      run_id: "run-1"
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe("https://api.example.com/api/v1/diagnosis/rooms");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify({ evidence_snapshot_id: 7 }));

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("forwards Basic authorization for LDAP diagnosis auth", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 })
      })
    );

    expect(response.status).toBe(201);
    const fetchMock = vi.mocked(fetch);
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe(
      "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
    );
  });

  it("clears the diagnosis session cookie when backend rejects browser-session create", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid session" }, { status: 401 }),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          cookie: `${diagnosisSessionCookieName}=expired.session.token`,
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 }),
      }),
    );

    expect(response.status).toBe(401);
    const setCookie = response.headers.get("set-cookie") ?? "";
    expect(setCookie).toContain(`${diagnosisSessionCookieName}=`);
    expect(setCookie).toContain("Expires=Thu, 01 Jan 1970 00:00:00 GMT");
    expect(setCookie).toContain("HttpOnly");
  });

  it("does not clear the diagnosis session cookie for explicit LDAP create failures", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json({ error: "invalid ldap credentials" }, { status: 401 }),
    );

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          authorization: "Basic b3BlcmF0b3ItMTpsZGFwLXBhc3N3b3Jk",
          cookie: `${diagnosisSessionCookieName}=session.token.one`,
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 }),
      }),
    );

    expect(response.status).toBe(401);
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("proxies list requests with a bounded limit", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          items: [
            {
              session_id: "diagnosis-session-1",
              chat_session_id: 202,
              diagnosis_task_id: 101,
              evidence_snapshot_id: 7,
              workflow_id: "diagnosis-room-diagnosis-session-1",
              run_id: "run-1",
              room_status: "open",
              task_status: "running",
              turn_count: 1,
              owner_subject: "operator-1",
              created_at: "2026-06-20T00:00:00Z",
              updated_at: "2026-06-20T00:01:00Z",
              notification_timeline: [],
            },
          ],
        },
        { status: 200 },
      ),
    );

    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/rooms?limit=25", {
        headers: {
          authorization: "Bearer token-1",
          "x-extra-secret": "must-not-forward",
        },
      }),
    );

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toMatchObject({
      items: [{ session_id: "diagnosis-session-1" }],
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/diagnosis/rooms?limit=25",
    );
    expect(init.method).toBe("GET");
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("rejects invalid list limits before contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/rooms?limit=500", {
        headers: { authorization: "Bearer token-1" },
      }),
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "limit must be between 1 and 100.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        body: JSON.stringify({ evidence_snapshot_id: 7 })
      })
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({ error: "authorization is required" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects missing list authorization before contacting the backend", async () => {
    const response = await GET(
      new Request("https://console.example.com/api/diagnosis/rooms?limit=25"),
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
