export type DiagnosisConnectionStatus =
  | "idle"
  | "ticketing"
  | "connecting"
  | "connected"
  | "closed"
  | "error";

export type DiagnosisConversationTurn = {
  role: string;
  content: string;
};

type DiagnosisFinalConclusion = {
  status: string;
  source: string;
  reason?: string;
  evidence_snapshot_id?: number;
  conclusion_version?: string;
  recorded_at?: string;
  confirmed_by?: string;
  supplemental_context_refs?: string[];
  assistant_turn_id?: number;
  assistant_message_id?: string;
  assistant_sequence?: number;
  assistant_occurred_at?: string;
  content?: string;
  confidence?: string;
  requires_human_review?: boolean;
};

export type DiagnosisConsultationEvidenceRequest = {
  label: string;
  detail: string;
  priority: string;
};

export type DiagnosisConsultationInsight = {
  confidence_rationale?: string;
  missing_evidence_requests?: DiagnosisConsultationEvidenceRequest[];
  evidence_collection_suggestions?: DiagnosisConsultationEvidenceRequest[];
  conclusion_status?: string;
};

export type DiagnosisEvidenceRequest = {
  template_id?: number;
  tool: string;
  reason: string;
  query?: string;
  window_seconds?: number;
  step_seconds?: number;
  limit?: number;
};

export type DiagnosisActiveAlert = {
  source: string;
  labels?: Record<string, string> | null;
  annotations?: Record<string, string> | null;
  starts_at: string;
};

type DiagnosisMetricPoint = {
  timestamp: string;
  value: string;
};

export type DiagnosisMetricSeries = {
  metric?: Record<string, string> | null;
  points?: DiagnosisMetricPoint[];
};

type DiagnosisMetricQueryResult = {
  result_type?: string;
  series?: DiagnosisMetricSeries[];
  scalar?: DiagnosisMetricPoint;
  string?: DiagnosisMetricPoint;
  warnings?: string[];
};

export type DiagnosisEvidenceCollectionResult = {
  request: DiagnosisEvidenceRequest;
  template_id?: number;
  alert_source_profile_id?: number;
  alert_source_kind?: string;
  tool: string;
  status: string;
  reason_code: string;
  message: string;
  limit?: number;
  observed_alerts: number;
  active_alerts?: DiagnosisActiveAlert[];
  query?: string;
  window_seconds?: number;
  step_seconds?: number;
  observed_metric_series?: number;
  metric_result?: DiagnosisMetricQueryResult;
  collected_at: string;
};

type DiagnosisReadyFrame = {
  type: "ready";
  session_id: string;
  subject: string;
};

type DiagnosisTurnResultFrame = {
  type: "turn_result";
  session_id: string;
  chat_session_id: number;
  message_id: string;
  assistant_message_id: string;
  user_turn_id: number;
  assistant_turn_id: number;
  user_sequence: number;
  assistant_sequence: number;
  turn_count: number;
  context_bytes: number;
  status: string;
  assistant_message: string;
  requires_human_review: boolean;
  confidence: string;
  evidence_requests?: DiagnosisEvidenceRequest[];
  evidence_collection_results?: DiagnosisEvidenceCollectionResult[];
  consultation_insight?: DiagnosisConsultationInsight;
  follow_up_turns?: DiagnosisFollowUpTurn[];
};

type DiagnosisFollowUpTurn = {
  message_id: string;
  user_message: string;
  assistant_message_id: string;
  user_turn_id: number;
  assistant_turn_id: number;
  user_sequence: number;
  assistant_sequence: number;
  turn_count: number;
  context_bytes: number;
  assistant_message: string;
  requires_human_review: boolean;
  confidence: string;
  evidence_requests?: DiagnosisEvidenceRequest[];
  evidence_collection_results?: DiagnosisEvidenceCollectionResult[];
  consultation_insight?: DiagnosisConsultationInsight;
  trigger: string;
};

export type DiagnosisStateFrame = {
  type: "state";
  session_id: string;
  chat_session_id: number;
  diagnosis_task_id: number;
  owner_subject: string;
  status: string;
  turn_count: number;
  started_at: string;
  last_activity_at: string;
  closed_at?: string;
  close_reason?: string;
  final_conclusion?: DiagnosisFinalConclusion;
  confidence?: string;
  requires_human_review?: boolean;
  consultation_insight?: DiagnosisConsultationInsight;
  in_flight: boolean;
  seen_message_ids: string[];
  conversation: DiagnosisConversationTurn[];
};

type DiagnosisErrorFrame = {
  type: "error";
  code: string;
  message: string;
};

export type DiagnosisServerFrame =
  | DiagnosisReadyFrame
  | DiagnosisTurnResultFrame
  | DiagnosisStateFrame
  | DiagnosisErrorFrame;

export type DiagnosisClientFrame =
  | { type: "query_state" }
  | { type: "confirm_conclusion"; reason?: string }
  | { type: "submit_turn"; message_id: string; message: string };
