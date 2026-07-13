import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { diagnosisSessionCookieName } from "@/lib/api/diagnosis-session";

import { POST } from "./route";

describe("report workflow policy replay route", () => {
  const originalAPIBaseURL = process.env.OPENCLARION_API_BASE_URL;

  beforeEach(() => {
    process.env.OPENCLARION_API_BASE_URL = "https://api.example.com";
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json(validReplayResponse(), { status: 202 })),
    );
  });

  afterEach(() => {
    restoreEnv("OPENCLARION_API_BASE_URL", originalAPIBaseURL);
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("forwards the policy replay request and returns normalized auto-room links", async () => {
    const body = {
      window_start: "2026-05-28T04:39:00.000Z",
      window_end: "2026-05-28T05:11:00.000Z",
      limit: 1000,
      correlation_key: "policy-replay-7",
    };

    const response = await POST(
      new Request(
        "https://console.example.com/api/config/report-workflow-policies/7/replay-window",
        {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
            "x-extra-secret": "must-not-forward",
          },
          body: JSON.stringify(body),
        },
      ),
      { params: Promise.resolve({ policyId: "7" }) },
    );

    expect(response.status).toBe(202);
    await expect(response.json()).resolves.toMatchObject({
      auto_diagnosis: {
        rooms_started: 1,
        rooms: [
          {
            session_id: "diagnosis-session-auto-p7-s9002",
          },
        ],
      },
      correlation_key: "policy-replay-7",
    });

    const fetchMock = vi.mocked(fetch);
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      URL,
      RequestInit,
    ];
    expect(url.toString()).toBe(
      "https://api.example.com/api/v1/config/report-workflow-policies/7/replay-window",
    );
    expect(init.method).toBe("POST");
    expect(init.body).toBe(JSON.stringify(body));
    const headers = init.headers as Headers;
    expect(headers.get("authorization")).toBe("Bearer session.token.one");
    expect(headers.get("cookie")).toBeNull();
    expect(headers.get("x-extra-secret")).toBeNull();
  });

  it("rejects malformed backend replay responses before returning to the browser", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      Response.json(
        validReplayResponse({
          auto_diagnosis: {
            policies_matched: 1,
            snapshots: 1,
            rooms_started: 1,
            rooms_skipped: 0,
            skipped_snapshot_ids: [],
            rooms: [],
          },
        }),
        { status: 202 },
      ),
    );

    const response = await POST(
      new Request(
        "https://console.example.com/api/config/report-workflow-policies/7/replay-window",
        {
          method: "POST",
          headers: {
            cookie: `${diagnosisSessionCookieName}=session.token.one`,
          },
          body: JSON.stringify({
            window_start: "2026-05-28T04:39:00.000Z",
            window_end: "2026-05-28T05:11:00.000Z",
            limit: 1000,
          }),
        },
      ),
      { params: Promise.resolve({ policyId: "7" }) },
    );

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({
      error: "Report replay response is invalid",
    });
  });

  it("rejects invalid policy ids before contacting the backend", async () => {
    const response = await POST(
      new Request(
        "https://console.example.com/api/config/report-workflow-policies/not-a-number/replay-window",
        {
          method: "POST",
          body: JSON.stringify({
            window_start: "2026-05-28T04:39:00.000Z",
            window_end: "2026-05-28T05:11:00.000Z",
            limit: 1000,
          }),
        },
      ),
      { params: Promise.resolve({ policyId: "not-a-number" }) },
    );

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "Report workflow policy ID must be a positive integer.",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
});

function validReplayResponse(
  overrides: Record<string, unknown> = {},
): Record<string, unknown> {
  return {
    auto_diagnosis: {
      policies_matched: 1,
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
      rooms_skipped: 0,
      rooms_started: 1,
      skipped_snapshot_ids: [],
      snapshots: 1,
    },
    correlation_key: "policy-replay-7",
    run_id: "run-policy-replay",
    snapshots: [{ id: 9002, group_index: 0, event_count: 1 }],
    started: true,
    stats: {
      events_loaded: 1,
      failed: 0,
      groups_built: 1,
      groups_closed: 1,
      groups_existing: 0,
      groups_refreshed: 0,
      groups_saved: 1,
      ingested: { total: 1, saved: 1, duplicate: 0, failed: 0 },
      snapshots_duplicate: 0,
      snapshots_saved: 1,
    },
    workflow_id: "report-batch-policy-replay",
    ...overrides,
  };
}

function restoreEnv(name: string, value: string | undefined) {
  if (value === undefined) {
    delete process.env[name];
    return;
  }
  process.env[name] = value;
}
