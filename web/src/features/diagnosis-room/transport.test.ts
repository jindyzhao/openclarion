import { describe, expect, it, vi } from "vitest";

import { defaultAPIBaseURL, diagnosisWebSocketURL, issueDiagnosisWSTicket, parseDiagnosisServerFrame } from "./transport";

describe("defaultAPIBaseURL", () => {
  it("uses a stable SSR and browser-safe local API default", () => {
    expect(defaultAPIBaseURL()).toBe("http://localhost:8080");
  });
});

describe("diagnosisWebSocketURL", () => {
  it("maps HTTP API URLs to WebSocket URLs with session and ticket query values", () => {
    expect(diagnosisWebSocketURL("http://api.example.test/base", "session-1", "ticket-1")).toBe(
      "ws://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1"
    );
  });

  it("maps HTTPS API URLs to secure WebSocket URLs", () => {
    expect(diagnosisWebSocketURL("https://api.example.test", "session-1", "ticket-1")).toBe(
      "wss://api.example.test/ws/diagnosis?session_id=session-1&ticket=ticket-1"
    );
  });
});

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
  it("posts a generated-contract-shaped request with bearer auth", async () => {
    const fetcher = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          ticket: "ticket-1",
          session_id: "session-1",
          expires_at: "2026-05-28T10:00:30Z"
        }),
        { status: 201, headers: { "content-type": "application/json" } }
      );
    });

    const result = await issueDiagnosisWSTicket(
      "https://api.example.test",
      "token-1",
      " session-1 ",
      fetcher as unknown as typeof fetch
    );

    expect(result).toEqual({
      ok: true,
      data: {
        ticket: "ticket-1",
        session_id: "session-1",
        expires_at: "2026-05-28T10:00:30Z"
      }
    });
    expect(fetcher).toHaveBeenCalledWith(
      new URL("https://api.example.test/api/v1/diagnosis/ws-ticket"),
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          authorization: "Bearer token-1"
        }),
        body: JSON.stringify({ session_id: "session-1" })
      })
    );
  });
});
