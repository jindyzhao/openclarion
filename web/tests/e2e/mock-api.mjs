import { createHash } from "node:crypto";
import { createServer } from "node:http";

const port = Number.parseInt(process.env.OPENCLARION_MOCK_API_PORT ?? "18080", 10);
if (!Number.isSafeInteger(port) || port <= 0) {
  throw new Error(`Invalid OPENCLARION_MOCK_API_PORT: ${process.env.OPENCLARION_MOCK_API_PORT}`);
}

const report = {
  id: 101,
  correlation_key: "window:checkout-latency",
  title: "Checkout latency incident",
  executive_summary: "Checkout latency increased after the payment deployment.",
  severity: "warning",
  confidence: "high",
  sub_reports: [
    {
      title: "Checkout API latency",
      severity: "warning",
      summary: "p95 latency exceeded the warning threshold."
    }
  ],
  recommended_actions: [
    {
      label: "Inspect deployment",
      detail: "Compare checkout deployment timestamps with latency onset.",
      priority: "high"
    }
  ],
  notification_text: "Checkout latency incident requires review.",
  content: {
    title: "Checkout latency incident"
  },
  model: "gpt-4.1-mini",
  output_mode: "json_schema",
  created_by_workflow: "FinalReportWorkflow",
  created_at: "2026-05-28T06:00:00Z",
  linked_sub_reports: [
    {
      id: 501,
      evidence_snapshot_id: 9001,
      scenario: "single_alert",
      title: "Checkout API latency",
      summary: "p95 latency exceeded the warning threshold.",
      severity: "warning",
      confidence: "high",
      findings: [
        {
          label: "High p95 latency",
          detail: "p95 latency stayed above the warning threshold.",
          evidence_id: "alert:checkout-latency"
        }
      ],
      recommended_actions: [
        {
          label: "Inspect deployment",
          detail: "Compare checkout deployment timestamps with latency onset.",
          priority: "high"
        }
      ],
      evidence_refs: ["alert:checkout-latency"],
      content: {
        title: "Checkout API latency"
      },
      model: "gpt-4.1-mini",
      output_mode: "json_schema",
      created_by_workflow: "ReportFanOutWorkflow",
      created_at: "2026-05-28T05:59:00Z"
    }
  ]
};

const dashboard = {
  generated_at: "2026-05-28T06:01:00Z",
  alerts: {
    total_recent: 24,
    firing: 7,
    resolved: 17
  },
  reports: {
    total_recent: 12,
    delivered: 11,
    failed: 1,
    pending: 0,
    missing_delivery: 0,
    success_rate: 0.9167,
    severity: {
      info: 3,
      warning: 8,
      critical: 1
    }
  }
};

let nextAlertSourceID = 3;
const alertSources = [
  {
    id: 1,
    name: "Primary Prometheus",
    kind: "prometheus",
    base_url: "https://prometheus.example.test",
    auth_mode: "bearer",
    secret_ref: "secret/openclarion/prometheus-token",
    enabled: true,
    labels: {
      env: "prod",
      owner: "platform"
    },
    created_at: "2026-05-28T06:00:00Z",
    updated_at: "2026-05-28T06:00:00Z"
  },
  {
    id: 2,
    name: "Staging Alertmanager",
    kind: "alertmanager",
    base_url: "https://alertmanager-staging.example.test",
    auth_mode: "none",
    secret_ref: "",
    enabled: false,
    labels: {
      env: "staging"
    },
    created_at: "2026-05-28T06:00:00Z",
    updated_at: "2026-05-28T06:00:00Z"
  }
];

const server = createServer((request, response) => {
  const url = new URL(request.url ?? "/", `http://127.0.0.1:${port}`);
  if (request.method === "OPTIONS") {
    writeJSON(response, 204, null);
    return;
  }

  if (url.pathname === "/api/v1/config/alert-sources") {
    handleAlertSourceCollection(request, response);
    return;
  }
  const alertSourceMatch = url.pathname.match(/^\/api\/v1\/config\/alert-sources\/(\d+)$/);
  if (alertSourceMatch) {
    handleAlertSourceProfile(request, response, Number.parseInt(alertSourceMatch[1], 10));
    return;
  }

  switch (url.pathname) {
    case "/healthz":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { status: "ok" });
      return;
    case "/api/v1/dashboard":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, dashboard);
      return;
    case "/api/v1/reports":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { items: [report] });
      return;
    case "/api/v1/reports/101":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, report);
      return;
    case "/api/v1/diagnosis/ws-ticket":
      if (request.method !== "POST") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      if (!request.headers.authorization?.startsWith("Bearer ")) {
        writeJSON(response, 401, { error: "authentication failed" });
        return;
      }
      readJSON(request)
        .then((body) => {
          const sessionID = typeof body.session_id === "string" ? body.session_id.trim() : "";
          if (sessionID === "") {
            writeJSON(response, 400, { error: "session_id must be non-empty" });
            return;
          }
          writeJSON(response, 201, {
            ticket: `ticket-${sessionID}`,
            session_id: sessionID,
            expires_at: "2026-05-28T10:00:30Z"
          });
        })
        .catch((error) => {
          writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
        });
      return;
    default:
      writeJSON(response, 404, { error: `not found: ${url.pathname}` });
  }
});

server.on("upgrade", (request, socket) => {
  const url = new URL(request.url ?? "/", `http://127.0.0.1:${port}`);
  if (url.pathname !== "/ws/diagnosis") {
    socket.destroy();
    return;
  }
  const key = request.headers["sec-websocket-key"];
  const sessionID = url.searchParams.get("session_id") ?? "";
  const ticket = url.searchParams.get("ticket") ?? "";
  if (typeof key !== "string" || sessionID === "" || ticket === "") {
    socket.destroy();
    return;
  }
  const accept = createHash("sha1")
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");
  socket.write(
    [
      "HTTP/1.1 101 Switching Protocols",
      "Upgrade: websocket",
      "Connection: Upgrade",
      `Sec-WebSocket-Accept: ${accept}`,
      "\r\n"
    ].join("\r\n")
  );

  const state = {
    turnCount: 0,
    conversation: []
  };
  sendWebSocketJSON(socket, {
    type: "ready",
    session_id: sessionID,
    subject: "owner-1"
  });

  let buffer = Buffer.alloc(0);
  socket.on("data", (chunk) => {
    buffer = Buffer.concat([buffer, chunk]);
    for (;;) {
      const decoded = decodeWebSocketFrame(buffer);
      if (!decoded) {
        return;
      }
      buffer = buffer.subarray(decoded.bytes);
      if (decoded.opcode === 8) {
        socket.end();
        return;
      }
      if (decoded.opcode !== 1) {
        continue;
      }
      handleDiagnosisFrame(socket, sessionID, state, decoded.payload);
    }
  });
});

server.listen(port, "127.0.0.1");

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    server.close(() => {
      process.exit(0);
    });
  });
}

function writeJSON(response, status, body) {
  response.writeHead(status, {
    "access-control-allow-headers": "authorization, content-type, accept",
    "access-control-allow-methods": "GET, POST, PUT, OPTIONS",
    "access-control-allow-origin": "*",
    "content-type": "application/json"
  });
  response.end(body === null ? "" : JSON.stringify(body));
}

function readJSON(request) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    request.on("data", (chunk) => chunks.push(chunk));
    request.on("end", () => {
      try {
        resolve(JSON.parse(Buffer.concat(chunks).toString("utf8")));
      } catch (error) {
        reject(error);
      }
    });
    request.on("error", reject);
  });
}

function handleAlertSourceCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: alertSources });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const profile = alertSourceProfileFromBody(nextAlertSourceID, body);
      nextAlertSourceID += 1;
      alertSources.unshift(profile);
      writeJSON(response, 201, profile);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function handleAlertSourceProfile(request, response, profileID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = alertSources.findIndex((profile) => profile.id === profileID);
      if (index < 0) {
        writeJSON(response, 404, { error: "alert source profile not found" });
        return;
      }
      const current = alertSources[index];
      const profile = alertSourceProfileFromBody(profileID, body, current.created_at);
      alertSources[index] = profile;
      writeJSON(response, 200, profile);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error instanceof Error ? error.message : "invalid JSON" });
    });
}

function alertSourceProfileFromBody(id, body, createdAt = "2026-05-28T06:02:00Z") {
  const now = "2026-05-28T06:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    kind: body.kind === "alertmanager" ? "alertmanager" : "prometheus",
    base_url: String(body.base_url ?? ""),
    auth_mode: body.auth_mode === "bearer" ? "bearer" : "none",
    secret_ref: typeof body.secret_ref === "string" ? body.secret_ref : "",
    enabled: Boolean(body.enabled),
    labels: body.labels && typeof body.labels === "object" && !Array.isArray(body.labels) ? body.labels : {},
    created_at: createdAt,
    updated_at: now
  };
}

function handleDiagnosisFrame(socket, sessionID, state, payload) {
  let frame;
  try {
    frame = JSON.parse(payload);
  } catch {
    sendWebSocketJSON(socket, { type: "error", code: "bad_frame", message: "invalid JSON frame" });
    return;
  }
  if (frame.type === "query_state") {
    sendWebSocketJSON(socket, diagnosisState(sessionID, state));
    return;
  }
  if (frame.type === "submit_turn") {
    const text = typeof frame.message === "string" ? frame.message.trim() : "";
    const messageID = typeof frame.message_id === "string" ? frame.message_id : "";
    if (text === "" || messageID === "") {
      sendWebSocketJSON(socket, { type: "error", code: "invalid_request", message: "message is required" });
      return;
    }
    state.turnCount += 1;
    state.conversation.push({ role: "user", content: text });
    const assistant = `Mock diagnosis response for: ${text}`;
    state.conversation.push({ role: "assistant", content: assistant });
    sendWebSocketJSON(socket, {
      type: "turn_result",
      session_id: sessionID,
      chat_session_id: 42,
      message_id: messageID,
      assistant_message_id: `${messageID}/assistant`,
      user_turn_id: state.turnCount * 2 - 1,
      assistant_turn_id: state.turnCount * 2,
      user_sequence: state.turnCount * 2 - 1,
      assistant_sequence: state.turnCount * 2,
      turn_count: state.turnCount,
      context_bytes: 512,
      status: "open",
      assistant_message: assistant,
      requires_human_review: true,
      confidence: "medium"
    });
    return;
  }
  sendWebSocketJSON(socket, { type: "error", code: "bad_frame", message: "unsupported frame type" });
}

function diagnosisState(sessionID, state) {
  return {
    type: "state",
    session_id: sessionID,
    chat_session_id: 42,
    diagnosis_task_id: 7,
    owner_subject: "owner-1",
    status: "open",
    turn_count: state.turnCount,
    started_at: "2026-05-28T10:00:00Z",
    last_activity_at: "2026-05-28T10:00:05Z",
    in_flight: false,
    seen_message_ids: [],
    conversation: state.conversation
  };
}

function sendWebSocketJSON(socket, payload) {
  socket.write(encodeWebSocketFrame(JSON.stringify(payload)));
}

function encodeWebSocketFrame(text) {
  const payload = Buffer.from(text);
  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }
  const header = Buffer.alloc(4);
  header[0] = 0x81;
  header[1] = 126;
  header.writeUInt16BE(payload.length, 2);
  return Buffer.concat([header, payload]);
}

function decodeWebSocketFrame(buffer) {
  if (buffer.length < 2) {
    return null;
  }
  const opcode = buffer[0] & 0x0f;
  const masked = (buffer[1] & 0x80) !== 0;
  let length = buffer[1] & 0x7f;
  let offset = 2;
  if (length === 126) {
    if (buffer.length < offset + 2) {
      return null;
    }
    length = buffer.readUInt16BE(offset);
    offset += 2;
  } else if (length === 127) {
    if (buffer.length < offset + 8) {
      return null;
    }
    const bigLength = buffer.readBigUInt64BE(offset);
    if (bigLength > BigInt(Number.MAX_SAFE_INTEGER)) {
      throw new Error("WebSocket frame is too large");
    }
    length = Number(bigLength);
    offset += 8;
  }
  if (!masked) {
    return null;
  }
  if (buffer.length < offset + 4 + length) {
    return null;
  }
  const mask = buffer.subarray(offset, offset + 4);
  offset += 4;
  const payload = Buffer.alloc(length);
  for (let index = 0; index < length; index += 1) {
    payload[index] = buffer[offset + index] ^ mask[index % 4];
  }
  return {
    bytes: offset + length,
    opcode,
    payload: payload.toString("utf8")
  };
}
