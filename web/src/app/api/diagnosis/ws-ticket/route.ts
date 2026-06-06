import { NextResponse } from "next/server";

import { requestJSON } from "@/lib/api/client";
import type { components } from "@/lib/api/openapi";
import { apiResultResponse, readRequestJSON } from "@/lib/api/route";

type DiagnosisWSTicketRequest = components["schemas"]["DiagnosisWSTicketRequest"];
type DiagnosisWSTicketResponse = components["schemas"]["DiagnosisWSTicketResponse"];
type ErrorResponse = components["schemas"]["ErrorResponse"];

export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  const authorization = request.headers.get("authorization")?.trim() ?? "";
  if (!/^Bearer [^ \t\r\n]+$/.test(authorization)) {
    return NextResponse.json<ErrorResponse>({ error: "bearer authorization is required" }, { status: 401 });
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
    return apiResultResponse(ticket);
  }

  let websocketURL: string;
  try {
    websocketURL = diagnosisWebSocketURL(request.url, ticket.data.session_id, ticket.data.ticket);
  } catch {
    return NextResponse.json<ErrorResponse>({ error: "diagnosis WebSocket URL is not configured" }, { status: 500 });
  }
  return NextResponse.json({ ...ticket.data, websocket_url: websocketURL }, { status: 201 });
}

function diagnosisWebSocketURL(requestURL: string, sessionID: string, ticket: string): string {
  const sameOrigin = new URL(requestURL).origin;
  const websocketBaseURL = process.env.OPENCLARION_BROWSER_WS_BASE_URL?.trim() || sameOrigin;
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
