import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import { normalizedDiagnosisWSTicketResponse } from "@/lib/api/diagnosis-auth-response";
import {
  diagnosisAuthorizationFromRequest,
  diagnosisRequestPublicOrigin,
  expireDiagnosisSessionCookieOnAuthFailure,
} from "@/lib/api/diagnosis-session";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

type DiagnosisWSTicketRequest = components["schemas"]["DiagnosisWSTicketRequest"];
type DiagnosisWSTicketResponse = components["schemas"]["DiagnosisWSTicketResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const authorization = diagnosisAuthorizationFromRequest(request);
  if (authorization === null) {
    const response = NextResponse.json<ErrorResponse>(
      { error: "authorization is required" },
      { status: 401 },
    );
    expireDiagnosisSessionCookieOnAuthFailure(response, request, 401);
    return response;
  }

  const body = await readRequestJSON<DiagnosisWSTicketRequest>(request);
  if (!body.ok) {
    return apiResultResponse(body);
  }

  const ticket = await requestJSON<DiagnosisWSTicketResponse>("/api/v1/diagnosis/ws-ticket", {
    method: "POST",
    headers: { authorization },
    body: body.data
  });
  if (!ticket.ok) {
    const response = apiResultResponse(ticket);
    expireDiagnosisSessionCookieOnAuthFailure(
      response,
      request,
      ticket.error.status,
    );
    return response;
  }

  const normalizedTicket = normalizedDiagnosisWSTicketResponse(ticket.data);
  if (normalizedTicket === null) {
    return NextResponse.json<ErrorResponse>(
      { error: "diagnosis WebSocket ticket response is invalid" },
      { status: 502 },
    );
  }

  let websocketURL: string;
  try {
    websocketURL = diagnosisWebSocketURL(
      diagnosisRequestPublicOrigin(request),
      normalizedTicket.session_id,
      normalizedTicket.ticket,
    );
  } catch {
    return NextResponse.json<ErrorResponse>({ error: "diagnosis WebSocket URL is not configured" }, { status: 500 });
  }
  return NextResponse.json({ ...normalizedTicket, websocket_url: websocketURL }, { status: 201 });
}

function diagnosisWebSocketURL(requestOrigin: string, sessionID: string, ticket: string): string {
  const websocketBaseURL = process.env.OPENCLARION_BROWSER_WS_BASE_URL?.trim() || requestOrigin;
  const baseURL = new URL(websocketBaseURL);
  if (baseURL.username !== "" || baseURL.password !== "" || baseURL.search !== "" || baseURL.hash !== "") {
    throw new Error("Diagnosis WebSocket URL must not include userinfo.");
  }
  const url = new URL("/ws/diagnosis", baseURL);
  switch (url.protocol) {
    case "https:":
      url.protocol = "wss:";
      break;
    case "http:":
      url.protocol = "ws:";
      break;
    case "wss:":
    case "ws:":
      break;
    default:
      throw new Error(`Unsupported diagnosis WebSocket URL protocol: ${url.protocol}`);
  }
  url.searchParams.set("session_id", sessionID.trim());
  url.searchParams.set("ticket", ticket.trim());
  return url.toString();
}
