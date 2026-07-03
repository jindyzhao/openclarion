import { afterEach, describe, expect, it, vi } from "vitest";

import { retryReportNotificationAction } from "./client-api";

describe("report client API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("rejects invalid report IDs before calling fetch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(retryReportNotificationAction(0, null)).resolves.toEqual({
      ok: false,
      error: {
        message: "Report ID must be a positive integer.",
        status: 400,
      },
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects invalid channel IDs before calling fetch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(retryReportNotificationAction(11, 0)).resolves.toEqual({
      ok: false,
      error: {
        message: "Report notification channel profile ID must be a positive integer.",
        status: 400,
      },
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("rejects invalid notification purpose before calling fetch", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      retryReportNotificationAction(11, null, "closed" as never),
    ).resolves.toEqual({
      ok: false,
      error: {
        message: "Report notification purpose must be handoff or final.",
        status: 400,
      },
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("posts report notification retry to the same-origin route", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              retry_state: "sent",
              delivery: {
                created_at: "2026-06-21T09:00:00Z",
                id: 31,
                idempotency_key: "final_report:11/notification/final",
                notification_purpose: "final",
                report_notification_channel_profile_id: 2,
                status: "delivered",
                updated_at: "2026-06-21T09:00:01Z",
              },
            }),
            { headers: { "content-type": "application/json" }, status: 200 },
          ),
      ),
    );

    await expect(retryReportNotificationAction(11, 2, "final")).resolves.toMatchObject({
      ok: true,
      data: {
        retry_state: "sent",
        delivery: {
          id: 31,
          status: "delivered",
        },
      },
    });

    expect(fetch).toHaveBeenCalledWith(
      "/api/reports/11/notification/retry",
      expect.objectContaining({
        body: JSON.stringify({
          notification_purpose: "final",
          report_notification_channel_profile_id: 2,
        }),
        cache: "no-store",
        method: "POST",
      }),
    );
  });
});
