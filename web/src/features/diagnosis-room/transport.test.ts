import { describe, expect, it, vi } from "vitest";

import {
  checkDiagnosisAuthorization,
  clearDiagnosisBrowserSession,
  createDiagnosisBrowserSession,
  createDiagnosisRoom,
  fetchDiagnosisBrowserSession,
  fetchDiagnosisAuthStatus,
  issueDiagnosisWSTicket,
  parseDiagnosisServerFrame,
  retryDiagnosisRoomNotification,
} from "./transport";

describe("parseDiagnosisServerFrame", () => {
  it("accepts known server frame types", () => {
    expect(
      parseDiagnosisServerFrame(
        `{"type":"ready","session_id":"s1","subject":"owner"}`,
      ),
    ).toEqual({
      type: "ready",
      session_id: "s1",
      subject: "owner",
    });
  });

  it("accepts bounded turn stream snapshots and rejects invalid retries", () => {
    expect(
      parseDiagnosisServerFrame(
        JSON.stringify({
          type: "turn_stream",
          phase: "delta",
          session_id: "s1",
          message_id: "message-1",
          assistant_message_id: "message-1/assistant",
          activity_attempt: 1,
          generation_attempt: 2,
          sequence: 3,
          assistant_message: "Checking the current alert evidence.",
        }),
      ),
    ).toMatchObject({
      type: "turn_stream",
      generation_attempt: 2,
      sequence: 3,
    });
    expect(
      parseDiagnosisServerFrame(
        JSON.stringify({
          type: "turn_stream",
          phase: "reset",
          session_id: "s1",
          message_id: "message-1",
          assistant_message_id: "message-1/assistant",
          activity_attempt: 1,
          generation_attempt: 3,
          sequence: 0,
          assistant_message: "",
        }),
      ),
    ).toMatchObject({
      phase: "reset",
      generation_attempt: 3,
      sequence: 0,
    });

    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({
          type: "turn_stream",
          phase: "started",
          session_id: "s1",
          message_id: "message-1",
          assistant_message_id: "message-1/assistant",
          activity_attempt: 2,
          generation_attempt: 1,
          sequence: 0,
          assistant_message: "stale",
        }),
      ),
    ).toThrow("Invalid turn_stream diagnosis frame.");
  });

  it("preserves consultation insight on turn result frames", () => {
    expect(
      parseDiagnosisServerFrame(
        JSON.stringify({
          type: "turn_result",
          session_id: "s1",
          chat_session_id: 7,
          message_id: "user-1",
          assistant_message_id: "assistant-1",
          user_turn_id: 11,
          assistant_turn_id: 12,
          user_sequence: 1,
          assistant_sequence: 2,
          turn_count: 1,
          context_bytes: 256,
          status: "open",
          assistant_message: "Need restart evidence before finalizing.",
          requires_human_review: true,
          confidence: "medium",
          evidence_requests: [
            {
              tool: "active_alerts",
              reason: "Need current sibling alerts.",
              limit: 5,
            },
          ],
          evidence_collection_results: [
            {
              request: {
                tool: "active_alerts",
                reason: "Need current sibling alerts.",
                limit: 5,
              },
              tool: "active_alerts",
              status: "collected",
              reason_code: "ok",
              message: "Active alert collection succeeded.",
              observed_alerts: 1,
              active_alerts: [
                {
                  source: "alertmanager",
                  labels: { alertname: "CPUHigh", namespace: "prod" },
                  starts_at: "2026-06-17T10:00:00Z",
                },
              ],
              collected_at: "2026-06-17T10:00:01Z",
            },
          ],
	          evidence_timeline: [
	            {
	              turn_count: 1,
	              message_id: "user-1",
	              assistant_message_id: "assistant-1",
	              actor_subject: "reviewer-1",
	              trigger: "operator_turn",
	              evidence_requests: [
                {
                  tool: "active_alerts",
                  reason: "Need current sibling alerts.",
                  limit: 5,
                },
              ],
              evidence_collection_results: [
                {
                  request: {
                    tool: "active_alerts",
                    reason: "Need current sibling alerts.",
                    limit: 5,
                  },
                  tool: "active_alerts",
                  status: "collected",
                  reason_code: "ok",
                  message: "Active alert collection succeeded.",
                  observed_alerts: 1,
                  collected_at: "2026-06-17T10:00:01Z",
                },
              ],
            },
          ],
          consultation_insight: {
            confidence_rationale:
              "Metric evidence is present, but restart context is missing.",
            missing_evidence_requests: [
              {
                label: "Restart cause",
                detail:
                  "Inspect previous container logs for the affected workload.",
                priority: "high",
              },
            ],
            evidence_collection_suggestions: [
              {
                label: "CPU range",
                detail:
                  "Collect a five minute CPU saturation window around the alert.",
                priority: "medium",
              },
            ],
            conclusion_status: "needs_evidence",
          },
          follow_up_turns: [
            {
              message_id: "user-1/auto-evidence-1",
              user_message: "OpenClarion automatic evidence follow-up.",
              assistant_message_id: "user-1/auto-evidence-1/assistant",
              user_turn_id: 13,
              assistant_turn_id: 14,
              user_sequence: 3,
              assistant_sequence: 4,
              turn_count: 2,
              context_bytes: 512,
              assistant_message: "Collected evidence confirms CPU saturation.",
              requires_human_review: false,
              confidence: "high",
              consultation_insight: {
                conclusion_status: "final",
              },
              trigger: "collected_evidence",
            },
          ],
          latest_error: {
            code: "notification_failed",
            message:
              "AI diagnosis was saved, but downstream diagnosis notification delivery failed; review notification channel configuration.",
            message_id: "assistant-1",
            occurred_at: "2026-06-17T10:00:02Z",
          },
        }),
      ),
    ).toMatchObject({
      type: "turn_result",
      evidence_requests: [
        {
          tool: "active_alerts",
          reason: "Need current sibling alerts.",
          limit: 5,
        },
      ],
      evidence_collection_results: [
        {
          status: "collected",
          reason_code: "ok",
          observed_alerts: 1,
          active_alerts: [{ labels: { alertname: "CPUHigh" } }],
        },
      ],
	      evidence_timeline: [
	        {
	          turn_count: 1,
	          actor_subject: "reviewer-1",
	          trigger: "operator_turn",
	          evidence_collection_results: [{ status: "collected" }],
	        },
      ],
      consultation_insight: {
        confidence_rationale:
          "Metric evidence is present, but restart context is missing.",
        missing_evidence_requests: [{ priority: "high" }],
        evidence_collection_suggestions: [{ priority: "medium" }],
        conclusion_status: "needs_evidence",
      },
      follow_up_turns: [
        {
          message_id: "user-1/auto-evidence-1",
          assistant_message: "Collected evidence confirms CPU saturation.",
          consultation_insight: { conclusion_status: "final" },
          trigger: "collected_evidence",
        },
      ],
      latest_error: {
        code: "notification_failed",
        message_id: "assistant-1",
      },
    });
  });

  it("preserves latest error on state frames", () => {
    expect(
      parseDiagnosisServerFrame(
        JSON.stringify({
          type: "state",
          session_id: "s1",
          chat_session_id: 7,
          diagnosis_task_id: 11,
          owner_subject: "owner",
          status: "open",
          turn_count: 1,
          started_at: "2026-06-19T10:00:00Z",
          last_activity_at: "2026-06-19T10:01:00Z",
          approval_mode: "owner_and_leader",
          conclusion_digest: "a".repeat(64),
          approvals: [
            {
              id: 31,
              conclusion_digest: "a".repeat(64),
              actor_subject: "owner",
              authority: "owner",
              reason: "human_confirmed",
              approved_at: "2026-06-19T10:01:00Z",
            },
          ],
          pending_approval_authorities: ["leader"],
          approval_in_flight: false,
          latest_error: {
            code: "llm_timeout",
            message:
              "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
            message_id: "msg-1",
            occurred_at: "2026-06-19T10:01:00Z",
          },
          follow_up_turns: [
            {
              message_id: "collect-1/auto-evidence-1",
              user_message: "OpenClarion automatic evidence follow-up.",
              assistant_message_id: "collect-1/auto-evidence-1/assistant",
              user_turn_id: 13,
              assistant_turn_id: 14,
              user_sequence: 3,
              assistant_sequence: 4,
              turn_count: 2,
              context_bytes: 512,
              assistant_message: "Collected evidence has been reassessed.",
              requires_human_review: true,
              confidence: "medium",
              consultation_insight: {
                conclusion_status: "needs_evidence",
              },
              trigger: "collected_evidence",
            },
          ],
          supplemental_evidence: [
            {
              label: "Restart cause",
              detail: "Collect previous container logs.",
              priority: "high",
              evidence: "Previous logs show OOMKilled.",
              actor_subject: "reviewer-1",
              user_message_id: "msg-2",
              assistant_message_id: "msg-2/assistant",
              user_turn_id: 33,
              assistant_turn_id: 34,
              user_sequence: 3,
              assistant_sequence: 4,
              provided_at: "2026-06-19T10:01:00Z",
            },
          ],
          in_flight: false,
          seen_message_ids: ["msg-1"],
          conversation: [
            {
              role: "user",
              actor_subject: "reviewer-1",
              content: "Please investigate",
            },
            {
              role: "assistant",
              actor_subject: "openclarion:auto-diagnosis",
              content: "CPU alert is still firing.",
            },
          ],
        }),
      ),
    ).toMatchObject({
      type: "state",
      latest_error: {
        code: "llm_timeout",
        message_id: "msg-1",
      },
      follow_up_turns: [
        {
          message_id: "collect-1/auto-evidence-1",
          consultation_insight: {
            conclusion_status: "needs_evidence",
          },
          trigger: "collected_evidence",
        },
      ],
      supplemental_evidence: [
        {
          actor_subject: "reviewer-1",
          label: "Restart cause",
        },
      ],
      conversation: [
        {
          actor_subject: "reviewer-1",
        },
        {
          actor_subject: "openclarion:auto-diagnosis",
        },
      ],
      in_flight: false,
      approval_mode: "owner_and_leader",
      approvals: [
        {
          actor_subject: "owner",
          authority: "owner",
        },
      ],
      pending_approval_authorities: ["leader"],
    });
  });

  it("rejects unsupported frame types", () => {
    expect(() => parseDiagnosisServerFrame(`{"type":"unexpected"}`)).toThrow(
      /Unsupported/,
    );
  });

  it("rejects inconsistent conclusion approval state", () => {
    const base = {
      type: "state",
      session_id: "s1",
      chat_session_id: 7,
      diagnosis_task_id: 11,
      owner_subject: "owner",
      status: "open",
      turn_count: 1,
      started_at: "2026-06-20T00:00:00Z",
      last_activity_at: "2026-06-20T00:01:00Z",
      approval_mode: "owner_and_leader",
      conclusion_digest: "a".repeat(64),
      approval_in_flight: false,
      in_flight: false,
      seen_message_ids: [],
      conversation: [],
    };
    const approval = {
      id: 31,
      conclusion_digest: "b".repeat(64),
      actor_subject: "owner",
      authority: "owner",
      reason: "human_confirmed",
      approved_at: "2026-06-20T00:01:00Z",
    };

    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({ ...base, approvals: [approval] }),
      ),
    ).toThrow(/Invalid state/);
    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({
          ...base,
          pending_approval_authorities: ["leader", "leader"],
        }),
      ),
    ).toThrow(/Invalid state/);
    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({
          ...base,
          approvals: [
            {
              ...approval,
              conclusion_digest: "a".repeat(64),
            },
            {
              ...approval,
              id: 32,
              conclusion_digest: "a".repeat(64),
              actor_subject: "owner-2",
            },
          ],
        }),
      ),
    ).toThrow(/Invalid state/);
    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({
          ...base,
          approvals: [
            {
              ...approval,
              conclusion_digest: "a".repeat(64),
            },
          ],
          pending_approval_authorities: ["owner"],
        }),
      ),
    ).toThrow(/Invalid state/);
    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({
          ...base,
          approvals: [
            {
              ...approval,
              conclusion_digest: "a".repeat(64),
              actor_subject: "not-the-owner",
            },
          ],
          pending_approval_authorities: ["leader"],
        }),
      ),
    ).toThrow(/Invalid state/);
    expect(() =>
      parseDiagnosisServerFrame(
        JSON.stringify({
          ...base,
          approval_mode: "single",
          approvals: [
            {
              ...approval,
              conclusion_digest: "a".repeat(64),
            },
            {
              ...approval,
              id: 32,
              conclusion_digest: "a".repeat(64),
              actor_subject: "leader",
              authority: "leader",
            },
          ],
        }),
      ),
    ).toThrow(/Invalid state/);
  });

  it("rejects known frame types when required fields are missing", () => {
    for (const [raw, message] of [
      [`{"type":"ready","session_id":"s1"}`, /Invalid ready/],
      [
        `{"type":"turn_stream","phase":"delta","session_id":"s1"}`,
        /Invalid turn_stream/,
      ],
      [
        JSON.stringify({
          type: "turn_result",
          session_id: "s1",
          chat_session_id: 7,
          message_id: "msg-1",
          assistant_message_id: "msg-1/assistant",
          user_turn_id: 11,
          assistant_turn_id: 12,
          user_sequence: 1,
          assistant_sequence: 2,
          turn_count: 1,
          context_bytes: 128,
          status: "open",
          requires_human_review: true,
          confidence: "medium",
        }),
        /Invalid turn_result/,
      ],
      [
        JSON.stringify({
          type: "state",
          session_id: "s1",
          chat_session_id: 7,
          diagnosis_task_id: 11,
          owner_subject: "owner",
          status: "open",
          turn_count: 1,
          started_at: "2026-06-20T00:00:00Z",
          last_activity_at: "2026-06-20T00:01:00Z",
          approval_mode: "single",
          approval_in_flight: false,
          in_flight: false,
          seen_message_ids: ["msg-1"],
        }),
        /Invalid state/,
      ],
      [`{"type":"error","code":"bad_frame"}`, /Invalid error/],
    ] as const) {
      expect(() => parseDiagnosisServerFrame(raw)).toThrow(message);
    }
  });
});

describe("checkDiagnosisAuthorization", () => {
  it("posts LDAP credentials as Basic authorization to the same-origin auth check route", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          checked_at: "2026-06-21T04:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await checkDiagnosisAuthorization({
        mode: "basic",
        username: "operator-1",
        password: "ldap-password",
      });

      expect(result).toEqual({
        ok: true,
        data: {
          checked_at: "2026-06-21T04:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/auth/check",
        expect.objectContaining({
          method: "POST",
          headers: expect.any(Headers),
          body: undefined,
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect((request?.headers as Headers).get("authorization")).toBe(
        `Basic ${btoa("operator-1:ldap-password")}`,
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

describe("fetchDiagnosisAuthStatus", () => {
  it("fetches backend auth wiring status without credentials", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          configured: true,
          mode: "ldap",
          supported_modes: ["ldap", "oidc"],
        }),
        {
          status: 200,
          headers: { "content-type": "application/json" },
        },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await fetchDiagnosisAuthStatus();

      expect(result).toEqual({
        ok: true,
        data: {
          configured: true,
          mode: "ldap",
          supported_modes: ["ldap", "oidc"],
        },
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/auth/status",
        expect.objectContaining({
          method: "GET",
          headers: expect.any(Headers),
          body: undefined,
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect((request?.headers as Headers).get("authorization")).toBeNull();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

describe("diagnosis browser session transport", () => {
  it("fetches the current browser session without explicit credentials", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          authenticated: true,
          checked_at: "2026-06-22T10:00:00Z",
          mode: "wecom",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await fetchDiagnosisBrowserSession();

      expect(result).toEqual({
        ok: true,
        data: {
          authenticated: true,
          checked_at: "2026-06-22T10:00:00Z",
          mode: "wecom",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/auth/session",
        expect.objectContaining({
          method: "GET",
          headers: expect.any(Headers),
          body: undefined,
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect((request?.headers as Headers).get("authorization")).toBeNull();
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it("creates a browser session from LDAP Basic credentials", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          authenticated: true,
          checked_at: "2026-06-22T10:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        }),
        { status: 201, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await createDiagnosisBrowserSession({
        mode: "basic",
        username: "operator-1",
        password: "ldap-password",
      });

      expect(result).toEqual({
        ok: true,
        data: {
          authenticated: true,
          checked_at: "2026-06-22T10:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/auth/session",
        expect.objectContaining({
          method: "POST",
          headers: expect.any(Headers),
          body: undefined,
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect((request?.headers as Headers).get("authorization")).toBe(
        `Basic ${btoa("operator-1:ldap-password")}`,
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it("does not renew a browser session from the existing session cookie", async () => {
    const result = await createDiagnosisBrowserSession({ mode: "session" });

    expect(result).toEqual({
      ok: false,
      error: {
        message: "Authorization credentials are required.",
        status: 401,
      },
    });
  });

  it("clears the browser session without a request body", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(null, { status: 204 });
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await clearDiagnosisBrowserSession();

      expect(result).toEqual({ ok: true, data: undefined });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/auth/session",
        expect.objectContaining({
          method: "DELETE",
          headers: expect.any(Headers),
          body: undefined,
        }),
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

describe("issueDiagnosisWSTicket", () => {
  it("posts a generated-contract-shaped request to the same-origin ticket route", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          ticket: "ticket-1",
          session_id: "session-1",
          expires_at: "2026-05-28T10:00:30Z",
          websocket_url:
            "ws://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1",
        }),
        { status: 201, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await issueDiagnosisWSTicket(
        { mode: "bearer", token: "token-1" },
        " session-1 ",
      );

      expect(result).toEqual({
        ok: true,
        data: {
          ticket: "ticket-1",
          session_id: "session-1",
          expires_at: "2026-05-28T10:00:30Z",
          websocket_url:
            "ws://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1",
        },
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/ws-ticket",
        expect.objectContaining({
          method: "POST",
          headers: expect.any(Headers),
          body: JSON.stringify({ session_id: "session-1" }),
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect(request).toBeDefined();
      expect((request?.headers as Headers).get("authorization")).toBe(
        "Bearer token-1",
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });

  it("posts LDAP credentials as Basic authorization", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          ticket: "ticket-1",
          session_id: "session-1",
          websocket_url:
            "ws://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1",
        }),
        { status: 201, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await issueDiagnosisWSTicket(
        { mode: "basic", username: "operator-1", password: "ldap-password" },
        "session-1",
      );

      expect(result.ok).toBe(true);
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect((request?.headers as Headers).get("authorization")).toBe(
        `Basic ${btoa("operator-1:ldap-password")}`,
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

describe("createDiagnosisRoom", () => {
  it("posts the approval mode and an optional notification channel profile id", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          session_id: "diagnosis-session-1",
          evidence_snapshot_id: 42,
          diagnosis_task_id: 101,
          chat_session_id: 202,
          workflow_id: "diagnosis-room-diagnosis-session-1",
          run_id: "run-1",
          approval_mode: "owner_and_leader",
        }),
        { status: 201, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await createDiagnosisRoom(
        { mode: "bearer", token: " token-1 " },
        42,
        5,
        "owner_and_leader",
      );

      expect(result).toEqual({
        ok: true,
        data: {
          session_id: "diagnosis-session-1",
          evidence_snapshot_id: 42,
          diagnosis_task_id: 101,
          chat_session_id: 202,
          workflow_id: "diagnosis-room-diagnosis-session-1",
          run_id: "run-1",
          approval_mode: "owner_and_leader",
        },
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/rooms",
        expect.objectContaining({
          method: "POST",
          headers: expect.any(Headers),
          body: JSON.stringify({
            approval_mode: "owner_and_leader",
            evidence_snapshot_id: 42,
            close_notification_channel_profile_id: 5,
          }),
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect(request).toBeDefined();
      expect((request?.headers as Headers).get("authorization")).toBe(
        "Bearer token-1",
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});

describe("retryDiagnosisRoomNotification", () => {
  it("posts a diagnosis notification retry request with LDAP Basic authorization", async () => {
    const originalFetch = globalThis.fetch;
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          retry_state: "sent",
          notification: {
            event_kind: "diagnosis_room.final_ready_notification_sent",
            notification_channel_profile_id: 2,
            provider_status: "delivered",
            provider_message_id: "wecom-retry-1",
            occurred_at: "2026-06-21T11:30:00Z",
          },
        }),
        { status: 200, headers: { "content-type": "application/json" } },
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await retryDiagnosisRoomNotification(
        { mode: "basic", username: "operator-1", password: "ldap-password" },
        " session-1 ",
        "diagnosis_room.final_ready_notification_sent",
      );

      expect(result.ok).toBe(true);
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/rooms/session-1/notifications/retry",
        expect.objectContaining({
          method: "POST",
          headers: expect.any(Headers),
          body: JSON.stringify({
            event_kind: "diagnosis_room.final_ready_notification_sent",
          }),
        }),
      );
      const calls = fetcher.mock.calls as unknown as Array<
        [RequestInfo | URL, RequestInit | undefined]
      >;
      const request = calls[0]?.[1];
      expect(request).toBeDefined();
      expect((request?.headers as Headers).get("authorization")).toBe(
        `Basic ${btoa("operator-1:ldap-password")}`,
      );
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});
