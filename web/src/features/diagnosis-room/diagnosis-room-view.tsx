"use client";

import { FormEvent, useEffect, useRef, useState } from "react";

import { ReportShell } from "@/features/reports/report-shell";

import {
  defaultAPIBaseURL,
  diagnosisWebSocketURL,
  issueDiagnosisWSTicket,
  nextDiagnosisMessageID,
  parseDiagnosisServerFrame
} from "./transport";
import type {
  DiagnosisClientFrame,
  DiagnosisConnectionStatus,
  DiagnosisConversationTurn,
  DiagnosisServerFrame,
  DiagnosisStateFrame
} from "./types";

type LogEntry = {
  id: number;
  level: "info" | "error";
  message: string;
};

type TranscriptTurn = DiagnosisConversationTurn & {
  id: string;
};

export function DiagnosisRoomView() {
  const socketRef = useRef<WebSocket | null>(null);
  const logIDRef = useRef(0);
  const [apiBaseURL, setAPIBaseURL] = useState(defaultAPIBaseURL);
  const [sessionID, setSessionID] = useState("diagnosis-session-42");
  const [bearerToken, setBearerToken] = useState("");
  const [message, setMessage] = useState("");
  const [status, setStatus] = useState<DiagnosisConnectionStatus>("idle");
  const [socketOpen, setSocketOpen] = useState(false);
  const [readySubject, setReadySubject] = useState("");
  const [roomState, setRoomState] = useState<DiagnosisStateFrame | null>(null);
  const [transcript, setTranscript] = useState<TranscriptTurn[]>([]);
  const [log, setLog] = useState<LogEntry[]>([]);

  useEffect(() => {
    return () => {
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, []);

  const connected = status === "connected" && socketOpen;
  const busy = status === "ticketing" || status === "connecting";

  async function handleConnect(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmedSessionID = sessionID.trim();
    const trimmedBearer = bearerToken.trim();
    if (trimmedSessionID === "" || trimmedBearer === "") {
      pushLog("error", "Session and bearer token are required.");
      setStatus("error");
      return;
    }

    socketRef.current?.close();
    socketRef.current = null;
    setSocketOpen(false);
    setStatus("ticketing");
    setReadySubject("");
    setRoomState(null);
    setTranscript([]);
    pushLog("info", "Requesting WebSocket ticket.");

    const ticket = await issueDiagnosisWSTicket(apiBaseURL, trimmedBearer, trimmedSessionID);
    if (!ticket.ok) {
      setStatus("error");
      pushLog("error", ticket.error.status ? `HTTP ${ticket.error.status}: ${ticket.error.message}` : ticket.error.message);
      return;
    }

    setStatus("connecting");
    const url = diagnosisWebSocketURL(apiBaseURL, ticket.data.session_id, ticket.data.ticket);
    const socket = new WebSocket(url);
    socketRef.current = socket;

    socket.onopen = () => {
      setSocketOpen(true);
    };
    socket.onmessage = (messageEvent: MessageEvent<string>) => {
      try {
        handleServerFrame(parseDiagnosisServerFrame(messageEvent.data));
      } catch (error) {
        pushLog("error", error instanceof Error ? error.message : "Invalid diagnosis frame.");
      }
    };
    socket.onerror = () => {
      setSocketOpen(false);
      setStatus("error");
      pushLog("error", "WebSocket error.");
    };
    socket.onclose = () => {
      setSocketOpen(false);
      setStatus((current) => (current === "error" ? current : "closed"));
      pushLog("info", "WebSocket closed.");
    };
  }

  function handleServerFrame(frame: DiagnosisServerFrame) {
    switch (frame.type) {
      case "ready":
        setStatus("connected");
        setReadySubject(frame.subject);
        pushLog("info", `Connected as ${frame.subject}.`);
        sendFrame({ type: "query_state" });
        break;
      case "state":
        setRoomState(frame);
        setTranscript(
          frame.conversation.map((turn, index) => ({
            id: `state-${index}-${turn.role}`,
            role: turn.role,
            content: turn.content
          }))
        );
        pushLog("info", `Loaded state: ${frame.status}, ${frame.turn_count} turn(s).`);
        break;
      case "turn_result":
        setRoomState((current) =>
          current
            ? {
                ...current,
                status: frame.status,
                turn_count: frame.turn_count,
                in_flight: false
              }
            : current
        );
        setTranscript((current) => [
          ...current,
          {
            id: frame.assistant_message_id,
            role: "assistant",
            content: frame.assistant_message
          }
        ]);
        pushLog("info", `Turn ${frame.turn_count} completed.`);
        break;
      case "error":
        setStatus((current) => (current === "connected" ? current : "error"));
        pushLog("error", `${frame.code}: ${frame.message}`);
        break;
    }
  }

  function handleSend(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = message.trim();
    if (!connected || trimmed === "") {
      return;
    }
    const messageID = nextDiagnosisMessageID();
    sendFrame({ type: "submit_turn", message_id: messageID, message: trimmed });
    setTranscript((current) => [
      ...current,
      {
        id: messageID,
        role: "user",
        content: trimmed
      }
    ]);
    setMessage("");
  }

  function handleQueryState() {
    if (connected) {
      sendFrame({ type: "query_state" });
    }
  }

  function handleDisconnect() {
    socketRef.current?.close();
    socketRef.current = null;
    setSocketOpen(false);
  }

  function sendFrame(frame: DiagnosisClientFrame) {
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      pushLog("error", "WebSocket is not connected.");
      return;
    }
    socket.send(JSON.stringify(frame));
  }

  function pushLog(level: LogEntry["level"], entryMessage: string) {
    logIDRef.current += 1;
    setLog((current) => [{ id: logIDRef.current, level, message: entryMessage }, ...current].slice(0, 8));
  }

  return (
    <ReportShell current="diagnosis">
      <section className="page-heading">
        <div>
          <h1>Diagnosis Room</h1>
          <p>Short-conversation investigation for an alert diagnosis session.</p>
        </div>
        <span aria-label="Connection status" className={statusClass(status)} role="status">
          {statusLabel(status)}
        </span>
      </section>

      <div className="diagnosis-layout">
        <section className="panel">
          <div className="panel-header">
            <h2>Connection</h2>
          </div>
          <div className="panel-body">
            <form className="diagnosis-form" onSubmit={handleConnect}>
              <label>
                <span>API base URL</span>
                <input
                  autoComplete="url"
                  name="apiBaseURL"
                  onChange={(event) => setAPIBaseURL(event.target.value)}
                  type="url"
                  value={apiBaseURL}
                />
              </label>
              <label>
                <span>Session ID</span>
                <input
                  autoComplete="off"
                  name="sessionID"
                  onChange={(event) => setSessionID(event.target.value)}
                  value={sessionID}
                />
              </label>
              <label>
                <span>Bearer token</span>
                <input
                  autoComplete="off"
                  name="bearerToken"
                  onChange={(event) => setBearerToken(event.target.value)}
                  type="password"
                  value={bearerToken}
                />
              </label>
              <div className="button-row">
                <button className="button-primary" disabled={busy} type="submit">
                  Connect
                </button>
                <button className="button-secondary" disabled={!connected} onClick={handleQueryState} type="button">
                  Refresh State
                </button>
                <button className="button-secondary" disabled={status === "idle"} onClick={handleDisconnect} type="button">
                  Disconnect
                </button>
              </div>
            </form>
          </div>
        </section>

        <section className="panel">
          <div className="panel-header">
            <h2>Room State</h2>
          </div>
          <div className="panel-body">
            <dl className="diagnosis-state">
              <StateRow label="Subject" value={readySubject || roomState?.owner_subject || "-"} />
              <StateRow label="Session" value={roomState?.session_id || sessionID || "-"} />
              <StateRow label="Status" value={roomState?.status || statusLabel(status)} />
              <StateRow label="Turns" value={roomState ? String(roomState.turn_count) : "-"} />
              <StateRow label="Close reason" value={roomState?.close_reason || "-"} />
              <StateRow label="Conclusion" value={finalConclusionLabel(roomState)} />
              <StateRow label="In flight" value={roomState?.in_flight ? "yes" : "no"} />
            </dl>
            {roomState?.final_conclusion ? (
              <div className="diagnosis-conclusion">
                <div className="diagnosis-conclusion-title">Final conclusion</div>
                <p>{finalConclusionText(roomState)}</p>
              </div>
            ) : null}
          </div>
        </section>
      </div>

      <section className="panel diagnosis-room-panel">
        <div className="panel-header diagnosis-room-header">
          <h2>Transcript</h2>
          <span className="muted">{transcript.length} message(s)</span>
        </div>
        <div className="panel-body diagnosis-transcript" aria-live="polite">
          {transcript.length === 0 ? (
            <div className="notice">No transcript messages.</div>
          ) : (
            transcript.map((turn) => (
              <article className={`diagnosis-turn diagnosis-turn-${turn.role}`} key={turn.id}>
                <div className="diagnosis-turn-role">{turn.role}</div>
                <p>{turn.content}</p>
              </article>
            ))
          )}
        </div>
        <form className="diagnosis-composer" onSubmit={handleSend}>
          <label>
            <span>Message</span>
            <textarea
              name="message"
              onChange={(event) => setMessage(event.target.value)}
              rows={3}
              value={message}
            />
          </label>
          <button className="button-primary" disabled={!connected || message.trim() === ""} type="submit">
            Send
          </button>
        </form>
      </section>

      {log.length > 0 ? (
        <section className="panel">
          <div className="panel-header">
            <h2>Events</h2>
          </div>
          <div className="panel-body">
            <ul className="diagnosis-log">
              {log.map((entry) => (
                <li className={entry.level === "error" ? "diagnosis-log-error" : undefined} key={entry.id}>
                  {entry.message}
                </li>
              ))}
            </ul>
          </div>
        </section>
      ) : null}
    </ReportShell>
  );
}

function StateRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function finalConclusionLabel(state: DiagnosisStateFrame | null): string {
  const conclusion = state?.final_conclusion;
  if (!conclusion) {
    return "-";
  }
  if (conclusion.status === "available") {
    return conclusion.confidence ? `${conclusion.status} (${conclusion.confidence})` : conclusion.status;
  }
  return conclusion.status;
}

function finalConclusionText(state: DiagnosisStateFrame): string {
  const conclusion = state.final_conclusion;
  if (!conclusion) {
    return "";
  }
  const content = conclusion.content?.trim();
  if (content) {
    return content;
  }
  const reason = conclusion.reason?.trim();
  if (reason) {
    return reason.replaceAll("_", " ");
  }
  return conclusion.status;
}

function statusLabel(status: DiagnosisConnectionStatus): string {
  switch (status) {
    case "ticketing":
      return "requesting ticket";
    case "connecting":
      return "connecting";
    case "connected":
      return "connected";
    case "closed":
      return "closed";
    case "error":
      return "error";
    case "idle":
      return "idle";
  }
}

function statusClass(status: DiagnosisConnectionStatus): string {
  switch (status) {
    case "connected":
      return "pill pill-ok";
    case "error":
      return "pill pill-critical";
    case "ticketing":
    case "connecting":
      return "pill pill-warning";
    default:
      return "pill pill-info";
  }
}
