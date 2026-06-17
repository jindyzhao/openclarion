"use client";

import {
  ApiOutlined,
  BulbOutlined,
  DisconnectOutlined,
  ReloadOutlined,
  SendOutlined
} from "@ant-design/icons";
import { useMutation } from "@tanstack/react-query";
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  List,
  Space,
  Tag,
  Typography
} from "antd";
import type { DescriptionsProps } from "antd";
import { useEffect, useRef, useState } from "react";

import { ReportShell } from "@/features/reports/report-shell";

import {
  issueDiagnosisWSTicket,
  nextDiagnosisMessageID,
  parseDiagnosisServerFrame,
  type DiagnosisWSTicketBundle
} from "./transport";
import type {
  DiagnosisClientFrame,
  DiagnosisConsultationEvidenceRequest,
  DiagnosisConsultationInsight,
  DiagnosisConnectionStatus,
  DiagnosisConversationTurn,
  DiagnosisServerFrame,
  DiagnosisStateFrame
} from "./types";

type ConnectionFormValues = {
  sessionID: string;
  bearerToken: string;
};

type ComposerValues = {
  message: string;
};

type LogEntry = {
  id: number;
  level: "info" | "error";
  message: string;
};

type TranscriptTurn = DiagnosisConversationTurn & {
  id: string;
};

type DiagnosisTurnResultFrame = Extract<DiagnosisServerFrame, { type: "turn_result" }>;

type LatestConsultationInsight = {
  confidence: string;
  insight: DiagnosisConsultationInsight;
  requiresHumanReview: boolean;
  status: string;
  turnCount: number;
};

class DiagnosisActionError extends Error {
  constructor(message: string, readonly status?: number) {
    super(message);
    this.name = "DiagnosisActionError";
  }
}

export function DiagnosisRoomView() {
  const socketRef = useRef<WebSocket | null>(null);
  const logIDRef = useRef(0);
  const [connectionForm] = Form.useForm<ConnectionFormValues>();
  const [composerForm] = Form.useForm<ComposerValues>();
  const [status, setStatus] = useState<DiagnosisConnectionStatus>("idle");
  const [socketOpen, setSocketOpen] = useState(false);
  const [readySubject, setReadySubject] = useState("");
  const [roomState, setRoomState] = useState<DiagnosisStateFrame | null>(null);
  const [transcript, setTranscript] = useState<TranscriptTurn[]>([]);
  const [latestInsight, setLatestInsight] = useState<LatestConsultationInsight | null>(null);
  const [log, setLog] = useState<LogEntry[]>([]);

  const ticketMutation = useMutation<DiagnosisWSTicketBundle, DiagnosisActionError, ConnectionFormValues>({
    mutationFn: async (values) => {
      const result = await issueDiagnosisWSTicket(values.bearerToken, values.sessionID);
      if (!result.ok) {
        throw new DiagnosisActionError(result.error.message, result.error.status);
      }
      return result.data;
    }
  });

  useEffect(() => {
    return () => {
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, []);

  const connected = status === "connected" && socketOpen;
  const busy = ticketMutation.isPending || status === "connecting";

  async function handleConnect(values: ConnectionFormValues) {
    const trimmedSessionID = values.sessionID.trim();
    const trimmedBearer = values.bearerToken.trim();
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
    setLatestInsight(null);
    ticketMutation.reset();
    pushLog("info", "Requesting WebSocket ticket.");

    let ticket: DiagnosisWSTicketBundle;
    try {
      ticket = await ticketMutation.mutateAsync({
        bearerToken: trimmedBearer,
        sessionID: trimmedSessionID
      });
    } catch (error) {
      setStatus("error");
      pushLog("error", diagnosisActionErrorMessage(error));
      return;
    }

    setStatus("connecting");
    const socket = new WebSocket(ticket.websocket_url);
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
        setLatestInsight(latestConsultationInsight(frame));
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

  function handleSend(values: ComposerValues) {
    const trimmed = values.message.trim();
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
    composerForm.resetFields();
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

  const roomStateItems = roomStateDescriptionItems(
    roomState,
    readySubject,
    connectionForm.getFieldValue("sessionID"),
    status
  );

  return (
    <ReportShell current="diagnosis">
      <section className="page-heading">
        <div>
          <h1>Diagnosis Room</h1>
          <p>Short-conversation investigation for an alert diagnosis session.</p>
        </div>
        <Tag aria-label="Connection status" color={statusColor(status)} role="status">
          {statusLabel(status)}
        </Tag>
      </section>

      <div className="diagnosis-layout">
        <Card className="settings-overview-card" title="Connection">
          <Form<ConnectionFormValues>
            form={connectionForm}
            initialValues={{ bearerToken: "", sessionID: "diagnosis-session-42" }}
            layout="vertical"
            onFinish={handleConnect}
          >
            <Form.Item
              label="Session ID"
              name="sessionID"
              rules={[{ required: true, message: "Session ID is required." }]}
            >
              <Input autoComplete="off" disabled={busy} />
            </Form.Item>
            <Form.Item
              label="Bearer token"
              name="bearerToken"
              rules={[{ required: true, message: "Bearer token is required." }]}
            >
              <Input.Password autoComplete="off" disabled={busy} />
            </Form.Item>
            <Space wrap>
              <Button
                disabled={busy}
                htmlType="submit"
                icon={<ApiOutlined />}
                loading={ticketMutation.isPending}
                type="primary"
              >
                Connect
              </Button>
              <Button disabled={!connected} icon={<ReloadOutlined />} onClick={handleQueryState}>
                Refresh State
              </Button>
              <Button disabled={status === "idle"} icon={<DisconnectOutlined />} onClick={handleDisconnect}>
                Disconnect
              </Button>
            </Space>
          </Form>
        </Card>

        <Card className="settings-overview-card" title="Room State">
          <Descriptions column={1} items={roomStateItems} size="small" />
          {roomState?.final_conclusion ? (
            <Alert
              className="diagnosis-conclusion"
              description={finalConclusionText(roomState)}
              message="Final conclusion"
              showIcon
              type="success"
            />
          ) : null}
        </Card>
      </div>

      <Card
        className="diagnosis-room-panel settings-overview-card"
        extra={
          latestInsight ? (
            <Space className="diagnosis-insight-meta" size={[6, 6]} wrap>
              <Tag color={confidenceColor(latestInsight.confidence)}>{latestInsight.confidence || "unknown"}</Tag>
              <Tag color={latestInsight.requiresHumanReview ? "warning" : "success"}>
                {latestInsight.requiresHumanReview ? "review required" : "review optional"}
              </Tag>
            </Space>
          ) : null
        }
        title={
          <Space className="diagnosis-insight-title" size={8}>
            <BulbOutlined />
            <span>Consultation Insight</span>
          </Space>
        }
      >
        {latestInsight ? (
          <>
            <Descriptions column={{ xs: 1, sm: 2 }} items={consultationInsightItems(latestInsight)} size="small" />
            {latestInsight.insight.confidence_rationale ? (
              <Alert
                className="diagnosis-insight-rationale"
                description={latestInsight.insight.confidence_rationale}
                message="Confidence rationale"
                showIcon
                type="info"
              />
            ) : null}
            <div className="diagnosis-insight-grid">
              <EvidenceRequestList
                emptyDescription="No missing evidence requests"
                items={latestInsight.insight.missing_evidence_requests}
                title="Missing Evidence"
              />
              <EvidenceRequestList
                emptyDescription="No collection suggestions"
                items={latestInsight.insight.evidence_collection_suggestions}
                title="Collection Suggestions"
              />
            </div>
          </>
        ) : (
          <Empty description="No consultation insight yet" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        )}
      </Card>

      <Card
        className="diagnosis-room-panel settings-overview-card"
        extra={<Typography.Text type="secondary">{transcript.length} message(s)</Typography.Text>}
        title="Transcript"
      >
        {transcript.length === 0 ? (
          <Empty description="No transcript messages" image={Empty.PRESENTED_IMAGE_SIMPLE} />
        ) : (
          <div aria-live="polite" className="diagnosis-transcript">
            {transcript.map((turn) => (
              <article className={`diagnosis-turn diagnosis-turn-${turn.role}`} key={turn.id}>
                <div className="diagnosis-turn-role">{turn.role}</div>
                <p>{turn.content}</p>
              </article>
            ))}
          </div>
        )}

        <Form<ComposerValues> className="diagnosis-composer" form={composerForm} layout="vertical" onFinish={handleSend}>
          <Form.Item label="Message" name="message" rules={[{ required: true, message: "Message is required." }]}>
            <Input.TextArea autoSize={{ minRows: 3, maxRows: 6 }} disabled={!connected} />
          </Form.Item>
          <Button disabled={!connected} htmlType="submit" icon={<SendOutlined />} type="primary">
            Send
          </Button>
        </Form>
      </Card>

      {log.length > 0 ? (
        <Card className="settings-overview-card" title="Events">
          <List
            dataSource={log}
            renderItem={(entry) => (
              <List.Item className={entry.level === "error" ? "diagnosis-log-error" : undefined}>
                {entry.message}
              </List.Item>
            )}
            size="small"
          />
        </Card>
      ) : null}
    </ReportShell>
  );
}

function EvidenceRequestList({
  emptyDescription,
  items,
  title
}: {
  emptyDescription: string;
  items?: DiagnosisConsultationEvidenceRequest[];
  title: string;
}) {
  return (
    <section className="diagnosis-insight-section">
      <Typography.Title level={3}>{title}</Typography.Title>
      <List
        className="diagnosis-evidence-list"
        dataSource={items ?? []}
        locale={{ emptyText: emptyDescription }}
        renderItem={(item) => (
          <List.Item className="diagnosis-evidence-item">
            <List.Item.Meta
              description={item.detail}
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.label}</span>
                  <Tag color={priorityColor(item.priority)}>{item.priority}</Tag>
                </Space>
              }
            />
          </List.Item>
        )}
        size="small"
      />
    </section>
  );
}

function latestConsultationInsight(frame: DiagnosisTurnResultFrame): LatestConsultationInsight {
  return {
    confidence: frame.confidence,
    insight: frame.consultation_insight ?? {},
    requiresHumanReview: frame.requires_human_review,
    status: frame.status,
    turnCount: frame.turn_count
  };
}

function roomStateDescriptionItems(
  state: DiagnosisStateFrame | null,
  readySubject: string,
  sessionID: string | undefined,
  connectionStatus: DiagnosisConnectionStatus
): DescriptionsProps["items"] {
  return [
    { key: "subject", label: "Subject", children: readySubject || state?.owner_subject || "-" },
    { key: "session", label: "Session", children: state?.session_id || sessionID || "-" },
    { key: "status", label: "Status", children: state?.status || statusLabel(connectionStatus) },
    { key: "turns", label: "Turns", children: state ? String(state.turn_count) : "-" },
    { key: "close-reason", label: "Close reason", children: state?.close_reason || "-" },
    { key: "conclusion", label: "Conclusion", children: finalConclusionLabel(state) },
    { key: "in-flight", label: "In flight", children: state?.in_flight ? "yes" : "no" }
  ];
}

function consultationInsightItems(latestInsight: LatestConsultationInsight): DescriptionsProps["items"] {
  return [
    { key: "turn", label: "Turn", children: String(latestInsight.turnCount) },
    { key: "status", label: "Room status", children: latestInsight.status || "-" },
    {
      key: "conclusion-status",
      label: "Conclusion status",
      children: latestInsight.insight.conclusion_status || "-"
    },
    {
      key: "review",
      label: "Human review",
      children: latestInsight.requiresHumanReview ? "required" : "optional"
    }
  ];
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

function statusColor(status: DiagnosisConnectionStatus): string {
  switch (status) {
    case "connected":
      return "success";
    case "error":
      return "error";
    case "ticketing":
    case "connecting":
      return "warning";
    default:
      return "processing";
  }
}

function confidenceColor(confidence: string): string {
  switch (confidence.toLowerCase()) {
    case "high":
      return "success";
    case "medium":
      return "warning";
    case "low":
      return "error";
    default:
      return "default";
  }
}

function priorityColor(priority: string): string {
  switch (priority.toLowerCase()) {
    case "high":
      return "error";
    case "medium":
      return "warning";
    case "low":
      return "processing";
    default:
      return "default";
  }
}

function diagnosisActionErrorMessage(error: unknown): string {
  if (error instanceof DiagnosisActionError && error.status) {
    return `HTTP ${error.status}: ${error.message}`;
  }
  if (error instanceof Error && error.message.trim() !== "") {
    return error.message;
  }
  return "Request failed.";
}
