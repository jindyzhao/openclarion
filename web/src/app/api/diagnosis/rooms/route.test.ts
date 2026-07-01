import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

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

  it("forwards only the bearer header and create body to the backend", async () => {
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

  it("uses the diagnosis session cookie when Authorization is absent", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          cookie: "openclarion_diagnosis_session=session.token.one"
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 })
      })
    );

    expect(response.status).toBe(201);
    const fetchMock = vi.mocked(fetch);
    const [, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
  });

  it("rejects malformed explicit authorization instead of using the session cookie", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          authorization: "Basic not-a-diagnosis-bearer",
          cookie: "openclarion_diagnosis_session=session.token.one"
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 })
      })
    );

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({ error: "authorization is required" });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("keeps the session cookie on backend authorization denials", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(Response.json({ error: "unauthorized" }, { status: 403 }));

    const response = await POST(
      new Request("https://console.example.com/api/diagnosis/rooms", {
        method: "POST",
        headers: {
          cookie: "openclarion_diagnosis_session=session.token.one"
        },
        body: JSON.stringify({ evidence_snapshot_id: 7 })
      })
    );

    expect(response.status).toBe(403);
    expect(response.headers.get("set-cookie")).toBeNull();
  });

  it("rejects missing bearer authorization before contacting the backend", async () => {
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
});

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
