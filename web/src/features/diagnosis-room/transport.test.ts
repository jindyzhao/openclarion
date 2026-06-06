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
