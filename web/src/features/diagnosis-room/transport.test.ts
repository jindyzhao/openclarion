import { describe, expect, it, vi } from "vitest";

import { issueDiagnosisWSTicket, parseDiagnosisServerFrame } from "./transport";

describe("parseDiagnosisServerFrame", () => {
  it("accepts known server frame types", () => {
    expect(parseDiagnosisServerFrame(`{"type":"ready","session_id":"s1","subject":"owner"}`)).toEqual({
      type: "ready",
      session_id: "s1",
      subject: "owner"
    });
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
              limit: 5
            }
          ],
          evidence_collection_results: [
            {
              request: {
                tool: "active_alerts",
                reason: "Need current sibling alerts.",
                limit: 5
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
                  starts_at: "2026-06-17T10:00:00Z"
                }
              ],
              collected_at: "2026-06-17T10:00:01Z"
            }
          ],
          consultation_insight: {
            confidence_rationale: "Metric evidence is present, but restart context is missing.",
            missing_evidence_requests: [
              {
                label: "Restart cause",
                detail: "Inspect previous container logs for the affected workload.",
                priority: "high"
              }
            ],
            evidence_collection_suggestions: [
              {
                label: "CPU range",
                detail: "Collect a five minute CPU saturation window around the alert.",
                priority: "medium"
              }
            ],
            conclusion_status: "needs_evidence"
          }
        })
      )
    ).toMatchObject({
      type: "turn_result",
      evidence_requests: [{ tool: "active_alerts", reason: "Need current sibling alerts.", limit: 5 }],
      evidence_collection_results: [
        {
          status: "collected",
          reason_code: "ok",
          observed_alerts: 1,
          active_alerts: [{ labels: { alertname: "CPUHigh" } }]
        }
      ],
      consultation_insight: {
        confidence_rationale: "Metric evidence is present, but restart context is missing.",
        missing_evidence_requests: [{ priority: "high" }],
        evidence_collection_suggestions: [{ priority: "medium" }],
        conclusion_status: "needs_evidence"
      }
    });
  });

  it("rejects unsupported frame types", () => {
    expect(() => parseDiagnosisServerFrame(`{"type":"unexpected"}`)).toThrow(/Unsupported/);
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
          websocket_url: "ws://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1"
        }),
        { status: 201, headers: { "content-type": "application/json" } }
      );
    });
    globalThis.fetch = fetcher as unknown as typeof fetch;

    try {
      const result = await issueDiagnosisWSTicket("token-1", " session-1 ");

      expect(result).toEqual({
        ok: true,
        data: {
          ticket: "ticket-1",
          session_id: "session-1",
          expires_at: "2026-05-28T10:00:30Z",
          websocket_url: "ws://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1"
        }
      });
      expect(fetcher).toHaveBeenCalledWith(
        "/api/diagnosis/ws-ticket",
        expect.objectContaining({
          method: "POST",
          headers: expect.any(Headers),
          body: JSON.stringify({ session_id: "session-1" })
        })
      );
      const calls = fetcher.mock.calls as unknown as Array<[RequestInfo | URL, RequestInit | undefined]>;
      const request = calls[0]?.[1];
      expect(request).toBeDefined();
      expect((request?.headers as Headers).get("authorization")).toBe("Bearer token-1");
    } finally {
      globalThis.fetch = originalFetch;
    }
  });
});
