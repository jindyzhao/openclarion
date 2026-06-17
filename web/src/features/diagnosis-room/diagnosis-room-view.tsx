"use client";

import {
  ApiOutlined,
  BulbOutlined,
  DisconnectOutlined,
  PlusCircleOutlined,
  ReloadOutlined,
  SendOutlined
} from "@ant-design/icons";
import { useMutation } from "@tanstack/react-query";
import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Descriptions,
  Empty,
  Form,
  Input,
  InputNumber,
  List,
  Space,
  Tag,
  Typography
} from "antd";
import type { DescriptionsProps } from "antd";
import { useEffect, useRef, useState } from "react";

import { ReportShell } from "@/features/reports/report-shell";

import {
  createDiagnosisRoom,
  issueDiagnosisWSTicket,
  nextDiagnosisMessageID,
  parseDiagnosisServerFrame,
  type DiagnosisRoomCreateBundle,
  type DiagnosisWSTicketBundle
} from "./transport";
import type {
  DiagnosisActiveAlert,
  DiagnosisClientFrame,
  DiagnosisConsultationEvidenceRequest,
  DiagnosisConsultationInsight,
  DiagnosisConnectionStatus,
  DiagnosisConversationTurn,
  DiagnosisEvidenceCollectionResult,
  DiagnosisEvidenceRequest,
  DiagnosisMetricSeries,
  DiagnosisServerFrame,
  DiagnosisStateFrame
} from "./types";

type ConnectionFormValues = {
  sessionID: string;
  bearerToken: string;
};

type CreateRoomFormValues = {
  evidenceSnapshotID?: number | null;
  authorizationToken: string;
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
  autoFollowUpCount: number;
  collectionResults: DiagnosisEvidenceCollectionResult[];
  confidence: string;
  evidenceRequests: DiagnosisEvidenceRequest[];
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
  const { message } = AntdApp.useApp();
  const socketRef = useRef<WebSocket | null>(null);
  const logIDRef = useRef(0);
  const [createForm] = Form.useForm<CreateRoomFormValues>();
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

  const createRoomMutation = useMutation({
    mutationFn: async (values: { bearerToken: string; evidenceSnapshotID: number }) => {
      const result = await createDiagnosisRoom(values.bearerToken, values.evidenceSnapshotID);
      if (!result.ok) {
        throw new DiagnosisActionError(result.error.message, result.error.status);
      }
      return result.data;
    }
  });

  useEffect(() => {
    const snapshotID = initialEvidenceSnapshotID();
    if (snapshotID !== undefined) {
      createForm.setFieldsValue({ evidenceSnapshotID: snapshotID });
    }
  }, [createForm]);

  useEffect(() => {
    return () => {
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, []);

  const connected = status === "connected" && socketOpen;
  const busy = ticketMutation.isPending || status === "connecting";
  const createBusy = createRoomMutation.isPending || busy;

  async function handleCreateRoom(values: CreateRoomFormValues) {
    const trimmedBearer = values.authorizationToken.trim();
    const evidenceSnapshotID = values.evidenceSnapshotID;
    if (!isPositiveSafeInteger(evidenceSnapshotID) || trimmedBearer === "") {
      pushLog("error", "Evidence snapshot and authorization token are required.");
      setStatus("error");
      return;
    }

    setStatus("ticketing");
    pushLog("info", `Creating diagnosis room from evidence snapshot #${evidenceSnapshotID}.`);
    let room: DiagnosisRoomCreateBundle;
    try {
      room = await createRoomMutation.mutateAsync({
        bearerToken: trimmedBearer,
        evidenceSnapshotID
      });
    } catch (error) {
      setStatus("error");
      pushLog("error", diagnosisActionErrorMessage(error));
      return;
    }

    message.success("Diagnosis room created.");
    connectionForm.setFieldsValue({
      bearerToken: trimmedBearer,
      sessionID: room.session_id
    });
    pushLog("info", `Created ${room.session_id} from snapshot #${room.evidence_snapshot_id}.`);
    await handleConnect({ bearerToken: trimmedBearer, sessionID: room.session_id });
  }

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
                turn_count: latestTurnCount(frame),
                in_flight: false
              }
            : current
        );
        setTranscript((current) => [...current, ...turnResultTranscript(frame)]);
        pushLog("info", `Turn ${latestTurnCount(frame)} completed.`);
        if ((frame.follow_up_turns ?? []).length > 0) {
          pushLog("info", `Auto evidence follow-up completed ${(frame.follow_up_turns ?? []).length} turn(s).`);
        }
        if (latestTurnConclusionStatus(frame) === "final") {
          sendFrame({ type: "query_state" });
        }
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
          <p>Short-conversation investigation from a frozen evidence snapshot.</p>
        </div>
        <Tag aria-label="Connection status" color={statusColor(status)} role="status">
          {statusLabel(status)}
        </Tag>
      </section>

      <div className="diagnosis-layout">
        <Card className="settings-overview-card" title="Create Room">
          <Form<CreateRoomFormValues>
            form={createForm}
            initialValues={{ authorizationToken: "" }}
            layout="vertical"
            onFinish={handleCreateRoom}
          >
            <Form.Item
              label="Evidence snapshot"
              name="evidenceSnapshotID"
              rules={[
                { required: true, message: "Evidence snapshot is required." },
                {
                  validator: (_, value: unknown) =>
                    isPositiveSafeInteger(value)
                      ? Promise.resolve()
                      : Promise.reject(new Error("Evidence snapshot must be a positive integer."))
                }
              ]}
            >
              <InputNumber disabled={createBusy} min={1} precision={0} style={{ width: "100%" }} />
            </Form.Item>
            <Form.Item
              label="Authorization token"
              name="authorizationToken"
              rules={[{ required: true, message: "Authorization token is required." }]}
            >
              <Input.Password autoComplete="off" disabled={createBusy} />
            </Form.Item>
            <Button
              disabled={createBusy}
              htmlType="submit"
              icon={<PlusCircleOutlined />}
              loading={createRoomMutation.isPending}
              type="primary"
            >
              Create Room
            </Button>
          </Form>
        </Card>

        <Card className="settings-overview-card" title="Connection">
          <Form<ConnectionFormValues>
            form={connectionForm}
            initialValues={{ bearerToken: "", sessionID: "" }}
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
              {latestInsight.autoFollowUpCount > 0 ? (
                <Tag color="processing">auto evidence x{latestInsight.autoFollowUpCount}</Tag>
              ) : null}
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
              <EvidencePlanList
                emptyDescription="No executable evidence plan"
                items={latestInsight.evidenceRequests}
                title="Executable Evidence Plan"
              />
              <EvidenceCollectionResultList items={latestInsight.collectionResults} />
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

function EvidencePlanList({
  emptyDescription,
  items,
  title
}: {
  emptyDescription: string;
  items?: DiagnosisEvidenceRequest[];
  title: string;
}) {
  return (
    <section className="diagnosis-insight-section">
      <Typography.Title level={3}>{title}</Typography.Title>
      <List
        className="diagnosis-evidence-list"
        dataSource={items ?? []}
        locale={{ emptyText: emptyDescription }}
        renderItem={(item, index) => (
          <List.Item className="diagnosis-evidence-item" key={evidenceRequestKey(item, index)}>
            <List.Item.Meta
              description={formatEvidencePlanDetails(item)}
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.reason}</span>
                  <Tag color="processing">{item.tool}</Tag>
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

function EvidenceCollectionResultList({ items }: { items?: DiagnosisEvidenceCollectionResult[] }) {
  return (
    <section className="diagnosis-insight-section">
      <Typography.Title level={3}>Collection Results</Typography.Title>
      <List
        className="diagnosis-evidence-list"
        dataSource={items ?? []}
        locale={{ emptyText: "No evidence collected yet" }}
        renderItem={(item, index) => (
          <List.Item className="diagnosis-evidence-item" key={evidenceCollectionResultKey(item, index)}>
            <List.Item.Meta
              description={
                <div>
                  <Typography.Text type="secondary">{formatEvidenceCollectionDetails(item)}</Typography.Text>
                  {item.active_alerts && item.active_alerts.length > 0 ? (
                    <div className="diagnosis-alert-chips">
                      {item.active_alerts.slice(0, 3).map((alert, alertIndex) => (
                        <Tag key={activeAlertKey(alert, alertIndex)}>{formatActiveAlert(alert)}</Tag>
                      ))}
                      {item.active_alerts.length > 3 ? <Tag>+{item.active_alerts.length - 3} more</Tag> : null}
                    </div>
                  ) : null}
                  {hasMetricResult(item) ? <MetricResultSummary item={item} /> : null}
                </div>
              }
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.message || item.tool}</span>
                  <Tag color={collectionStatusColor(item.status)}>{item.status}</Tag>
                  <Tag>{item.reason_code}</Tag>
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

function MetricResultSummary({ item }: { item: DiagnosisEvidenceCollectionResult }) {
  const result = item.metric_result;
  if (!result) {
    return null;
  }
  const series = result.series ?? [];
  return (
    <div className="diagnosis-metric-summary">
      <Space size={[6, 6]} wrap>
        {result.result_type ? <Tag color="processing">{result.result_type}</Tag> : null}
        {item.observed_metric_series !== undefined ? <Tag>series: {item.observed_metric_series}</Tag> : null}
        {result.warnings?.map((warning, index) => (
          <Tag color="warning" key={`${warning}-${index}`}>
            {warning}
          </Tag>
        ))}
      </Space>
      {series.length > 0 ? (
        <div className="diagnosis-metric-series">
          {series.slice(0, 3).map((entry, index) => (
            <Tag key={metricSeriesKey(entry, index)}>{formatMetricSeries(entry)}</Tag>
          ))}
          {series.length > 3 ? <Tag>+{series.length - 3} more</Tag> : null}
        </div>
      ) : null}
      {result.scalar ? <div className="diagnosis-metric-value">scalar: {result.scalar.value}</div> : null}
      {result.string ? <div className="diagnosis-metric-value">string: {result.string.value}</div> : null}
    </div>
  );
}

function turnResultTranscript(frame: DiagnosisTurnResultFrame): TranscriptTurn[] {
  const turns: TranscriptTurn[] = [
    {
      id: frame.assistant_message_id,
      role: "assistant",
      content: frame.assistant_message
    }
  ];
  for (const followUp of frame.follow_up_turns ?? []) {
    turns.push({
      id: followUp.message_id,
      role: "user",
      content: followUp.user_message
    });
    turns.push({
      id: followUp.assistant_message_id,
      role: "assistant",
      content: followUp.assistant_message
    });
  }
  return turns;
}

function latestTurnCount(frame: DiagnosisTurnResultFrame): number {
  return latestFollowUpTurn(frame)?.turn_count ?? frame.turn_count;
}

function latestTurnConclusionStatus(frame: DiagnosisTurnResultFrame): string | undefined {
  return latestFollowUpTurn(frame)?.consultation_insight?.conclusion_status ?? frame.consultation_insight?.conclusion_status;
}

function latestFollowUpTurn(frame: DiagnosisTurnResultFrame) {
  const followUps = frame.follow_up_turns ?? [];
  return followUps.length > 0 ? followUps.at(-1) : undefined;
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
        renderItem={(item, index) => (
          <List.Item className="diagnosis-evidence-item" key={consultationEvidenceRequestKey(item, index)}>
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
  const latestFollowUp = latestFollowUpTurn(frame);
  if (latestFollowUp) {
    return {
      autoFollowUpCount: frame.follow_up_turns?.length ?? 0,
      collectionResults: latestFollowUp.evidence_collection_results ?? [],
      confidence: latestFollowUp.confidence,
      evidenceRequests: latestFollowUp.evidence_requests ?? [],
      insight: latestFollowUp.consultation_insight ?? {},
      requiresHumanReview: latestFollowUp.requires_human_review,
      status: frame.status,
      turnCount: latestFollowUp.turn_count
    };
  }
  return {
    autoFollowUpCount: 0,
    collectionResults: frame.evidence_collection_results ?? [],
    confidence: frame.confidence,
    evidenceRequests: frame.evidence_requests ?? [],
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
    },
    {
      key: "auto-follow-up",
      label: "Auto follow-up",
      children: String(latestInsight.autoFollowUpCount)
    }
  ];
}

function formatEvidencePlanDetails(item: DiagnosisEvidenceRequest): string {
  const details: string[] = [];
  if (item.query) {
    details.push(`query: ${item.query}`);
  }
  if (item.template_id) {
    details.push(`template: ${item.template_id}`);
  }
  if (item.window_seconds) {
    details.push(`window: ${item.window_seconds}s`);
  }
  if (item.step_seconds) {
    details.push(`step: ${item.step_seconds}s`);
  }
  if (item.limit) {
    details.push(`limit: ${item.limit}`);
  }
  return details.length > 0 ? details.join(" | ") : "No additional parameters";
}

function formatEvidenceCollectionDetails(item: DiagnosisEvidenceCollectionResult): string {
  const details = [`alerts observed: ${item.observed_alerts}`, `alerts visible: ${item.active_alerts?.length ?? 0}`];
  if (item.observed_metric_series !== undefined) {
    details.push(`series observed: ${item.observed_metric_series}`);
  }
  if (item.query) {
    details.push(`query: ${item.query}`);
  }
  if (item.window_seconds) {
    details.push(`window: ${item.window_seconds}s`);
  }
  if (item.step_seconds) {
    details.push(`step: ${item.step_seconds}s`);
  }
  if (item.alert_source_kind) {
    details.push(`source: ${item.alert_source_kind}`);
  }
  if (item.template_id) {
    details.push(`template: ${item.template_id}`);
  }
  if (item.alert_source_profile_id) {
    details.push(`profile: ${item.alert_source_profile_id}`);
  }
  if (item.limit) {
    details.push(`limit: ${item.limit}`);
  }
  return details.join(" | ");
}

function formatActiveAlert(alert: DiagnosisActiveAlert): string {
  const labels = alert.labels ?? {};
  const alertName = labels.alertname ?? labels.alert ?? "alert";
  const context = [labels.namespace, labels.pod].filter(Boolean).join(" / ");
  return context ? `${alertName} / ${context}` : `${alertName} / ${alert.source}`;
}

function hasMetricResult(item: DiagnosisEvidenceCollectionResult): boolean {
  const result = item.metric_result;
  return Boolean(
    result &&
      (result.result_type ||
        result.scalar ||
        result.string ||
        (result.series && result.series.length > 0) ||
        (result.warnings && result.warnings.length > 0))
  );
}

function formatMetricSeries(series: DiagnosisMetricSeries): string {
  const metric = series.metric ?? {};
  const metricName = metric.__name__ ?? metric.job ?? "series";
  const context = [metric.namespace, metric.pod, metric.instance].filter(Boolean).join(" / ");
  const latest = series.points?.[series.points.length - 1]?.value;
  const prefix = context ? `${metricName} / ${context}` : metricName;
  return latest ? `${prefix}: ${latest}` : prefix;
}

function metricSeriesKey(series: DiagnosisMetricSeries, index: number): string {
  const metric = series.metric ?? {};
  return `${metric.__name__ ?? metric.job ?? "series"}-${metric.instance ?? metric.pod ?? "none"}-${index}`;
}

function evidenceRequestKey(item: DiagnosisEvidenceRequest, index: number): string {
  return `${item.tool}-${item.template_id ?? "none"}-${item.reason}-${index}`;
}

function evidenceCollectionResultKey(item: DiagnosisEvidenceCollectionResult, index: number): string {
  return `${item.tool}-${item.status}-${item.reason_code}-${item.collected_at}-${index}`;
}

function consultationEvidenceRequestKey(item: DiagnosisConsultationEvidenceRequest, index: number): string {
  return `${item.priority}-${item.label}-${index}`;
}

function activeAlertKey(alert: DiagnosisActiveAlert, index: number): string {
  const labels = alert.labels ?? {};
  return `${alert.source}-${labels.alertname ?? labels.alert ?? "alert"}-${labels.namespace ?? "none"}-${index}`;
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

function collectionStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case "collected":
      return "success";
    case "failed":
      return "error";
    case "unsupported":
      return "warning";
    case "skipped":
      return "default";
    default:
      return "processing";
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

function initialEvidenceSnapshotID(): number | undefined {
  if (typeof window === "undefined") {
    return undefined;
  }
  const raw = new URLSearchParams(window.location.search).get("evidence_snapshot_id");
  if (raw === null) {
    return undefined;
  }
  const parsed = Number(raw);
  return isPositiveSafeInteger(parsed) ? parsed : undefined;
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
