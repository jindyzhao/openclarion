import type { ApiResult } from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";

import type { DiagnosisServerFrame } from "./types";

type DiagnosisWSTicketRequest = components["schemas"]["DiagnosisWSTicketRequest"];
type DiagnosisWSTicketResponse = components["schemas"]["DiagnosisWSTicketResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

type Fetcher = typeof fetch;

export async function issueDiagnosisWSTicket(
  apiBaseURL: string,
  bearerToken: string,
  sessionID: string,
  fetcher: Fetcher = fetch
): Promise<ApiResult<DiagnosisWSTicketResponse>> {
  const body: DiagnosisWSTicketRequest = { session_id: sessionID.trim() };
  let response: Response;
  try {
    response = await fetcher(new URL("/api/v1/diagnosis/ws-ticket", normalizeAPIBaseURL(apiBaseURL)), {
      method: "POST",
      cache: "no-store",
      headers: {
        accept: "application/json",
        authorization: `Bearer ${bearerToken.trim()}`,
        "content-type": "application/json"
      },
      body: JSON.stringify(body)
    });
  } catch (error) {
    return {
      ok: false,
      error: { message: error instanceof Error ? error.message : "Request failed." }
    };
  }

  if (!response.ok) {
    return {
      ok: false,
      error: {
        message: await errorMessage(response),
        status: response.status
      }
    };
  }

  return { ok: true, data: (await response.json()) as DiagnosisWSTicketResponse };
}

export function diagnosisWebSocketURL(apiBaseURL: string, sessionID: string, ticket: string): string {
  const url = new URL("/ws/diagnosis", normalizeAPIBaseURL(apiBaseURL));
  if (url.protocol === "https:") {
    url.protocol = "wss:";
  } else if (url.protocol === "http:") {
    url.protocol = "ws:";
  } else {
    throw new Error(`Unsupported API URL protocol: ${url.protocol}`);
  }
  url.searchParams.set("session_id", sessionID.trim());
  url.searchParams.set("ticket", ticket.trim());
  return url.toString();
}

export function parseDiagnosisServerFrame(raw: string): DiagnosisServerFrame {
  const parsed = JSON.parse(raw) as unknown;
  if (!isRecord(parsed) || typeof parsed.type !== "string") {
    throw new Error("Diagnosis frame must contain a string type.");
  }
  switch (parsed.type) {
    case "ready":
    case "turn_result":
    case "state":
    case "error":
      return parsed as DiagnosisServerFrame;
    default:
      throw new Error(`Unsupported diagnosis frame type: ${parsed.type}`);
  }
}

export function nextDiagnosisMessageID(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `msg-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export function defaultAPIBaseURL(): string {
  if (typeof window === "undefined") {
    return "http://localhost:8080";
  }
  return window.location.origin;
}

function normalizeAPIBaseURL(raw: string): URL {
  const trimmed = raw.trim();
  if (trimmed === "") {
    throw new Error("API base URL must be non-empty.");
  }
  return new URL(trimmed);
}

async function errorMessage(response: Response): Promise<string> {
  try {
    const body = (await response.json()) as Partial<ErrorResponse>;
    if (typeof body.error === "string" && body.error.trim() !== "") {
      return body.error;
    }
  } catch {
    // Fall through to the HTTP status line.
  }
  return response.statusText || `HTTP ${response.status}`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
