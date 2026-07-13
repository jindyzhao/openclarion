import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

describe("report replay route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json(
          {
            started: true,
            correlation_key: "alert-replay-7002",
            workflow_id: "report-batch-alert-replay",
            run_id: "run-alert-replay",
            stats: {
              ingested: { total: 1, saved: 1, duplicate: 0, failed: 0 },
              events_loaded: 1,
              groups_built: 1,
              groups_saved: 1,
              groups_refreshed: 0,
              groups_existing: 0,
              snapshots_saved: 1,
              snapshots_duplicate: 0,
              groups_closed: 1,
              failed: 0
            },
            snapshots: [{ id: 9002, group_index: 0, event_count: 1 }]
          },
          { status: 202 }
        )
      )
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards the replay request body and caller authorization to the backend", async () => {
    const body = {
      window_start: "2026-05-28T04:39:00.000Z",
      window_end: "2026-05-28T05:11:00.000Z",
      limit: 1000,
      scenario: "single_alert",
      correlation_key: "alert-replay-7002"
    };

    const response = await POST(
      new Request("https://console.example.com/api/report-triggers/replay-window", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
          "x-extra-header": "must-not-forward",
        },
        body: JSON.stringify(body)
      })
    );

    expect(response.status).toBe(202);
    await expect(response.json()).resolves.toMatchObject({
      started: true,
      correlation_key: "alert-replay-7002",
      workflow_id: "report-batch-alert-replay",
      snapshots: [{ id: 9002, group_index: 0, event_count: 1 }]
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [URL, RequestInit];
    expect(url.toString()).toBe("https://api.example.com/api/v1/report-triggers/replay-window");
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(body));

    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer token-1");
    expect(headers.get("x-extra-header")).toBeNull();
  });

  it("returns normalized automatic diagnosis room links", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          started: true,
          correlation_key: "alert-replay-7002",
          workflow_id: "report-batch-alert-replay",
          run_id: "run-alert-replay",
          stats: {
            ingested: { total: 1, saved: 1, duplicate: 0, failed: 0 },
            events_loaded: 1,
            groups_built: 1,
            groups_saved: 1,
            groups_refreshed: 0,
            groups_existing: 0,
            snapshots_saved: 1,
            snapshots_duplicate: 0,
            groups_closed: 1,
            failed: 0,
          },
          snapshots: [{ id: 9002, group_index: 0, event_count: 1 }],
          auto_diagnosis: {
            policies_matched: 1,
            snapshots: 1,
            rooms_started: 1,
            rooms_skipped: 0,
            skipped_snapshot_ids: [],
            rooms: [
              {
                policy_id: 7,
                evidence_snapshot_id: 9002,
                session_id: "diagnosis-session-auto-p7-s9002",
                initial_message_id: "diagnosis-auto-initial-p7-s9002",
                workflow_id: "diagnosis-room-diagnosis-session-auto-p7-s9002",
                run_id: "run-9002",
              },
            ],
          },
        },
        { status: 202 },
      ),
    );

    const response = await POST(
      new Request("https://console.example.com/api/report-triggers/replay-window", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: JSON.stringify({
          window_start: "2026-05-28T04:39:00.000Z",
          window_end: "2026-05-28T05:11:00.000Z",
          limit: 1000,
          scenario: "single_alert",
          correlation_key: "alert-replay-7002",
        }),
      }),
    );

    expect(response.status).toBe(202);
    await expect(response.json()).resolves.toMatchObject({
      auto_diagnosis: {
        rooms_started: 1,
        rooms: [
          {
            session_id: "diagnosis-session-auto-p7-s9002",
            workflow_id: "diagnosis-room-diagnosis-session-auto-p7-s9002",
          },
        ],
      },
    });
  });

  it("rejects malformed backend replay responses before returning to the browser", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        {
          started: true,
          correlation_key: "alert-replay-7002",
          workflow_id: "report-batch-alert-replay",
          run_id: "run-alert-replay",
          stats: {
            ingested: { total: 1, saved: 1, duplicate: 0, failed: 0 },
            events_loaded: 1,
            groups_built: 1,
            groups_saved: 1,
            groups_refreshed: 0,
            groups_existing: 0,
            snapshots_saved: 1,
            snapshots_duplicate: 0,
            groups_closed: 1,
            failed: 0,
          },
          snapshots: [{ id: 9002, group_index: 0, event_count: 1 }],
          auto_diagnosis: {
            policies_matched: 1,
            snapshots: 1,
            rooms_started: 1,
            rooms_skipped: 0,
            skipped_snapshot_ids: [],
            rooms: [],
          },
        },
        { status: 202 },
      ),
    );

    const response = await POST(
      new Request("https://console.example.com/api/report-triggers/replay-window", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: JSON.stringify({
          window_start: "2026-05-28T04:39:00.000Z",
          window_end: "2026-05-28T05:11:00.000Z",
          limit: 1000,
          scenario: "single_alert",
          correlation_key: "alert-replay-7002",
        }),
      }),
    );

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({
      error: "Report replay response is invalid",
    });
  });

  it("rejects invalid JSON before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/report-triggers/replay-window", {
        method: "POST",
        headers: {
          authorization: "Bearer token-1",
        },
        body: "{"
      })
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toMatchObject({ error: expect.stringContaining("JSON") });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rejects missing authorization before contacting the backend", async () => {
    const response = await POST(
      new Request("https://console.example.com/api/report-triggers/replay-window", {
        method: "POST",
        body: JSON.stringify({
          window_start: "2026-05-28T04:39:00.000Z",
          window_end: "2026-05-28T05:11:00.000Z",
          limit: 1000,
        }),
      }),
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
