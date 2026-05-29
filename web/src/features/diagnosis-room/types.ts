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
  | { type: "submit_turn"; message_id: string; message: string };
