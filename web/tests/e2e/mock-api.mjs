import { createHash } from "node:crypto";
import { createServer } from "node:http";

const port = Number.parseInt(
  process.env.OPENCLARION_MOCK_API_PORT ?? "18080",
  10,
);
if (!Number.isSafeInteger(port) || port <= 0) {
  throw new Error(
    `Invalid OPENCLARION_MOCK_API_PORT: ${process.env.OPENCLARION_MOCK_API_PORT}`,
  );
}

const diagnosisMockTurnDelayMs = 500;
const diagnosisWSTickets = new Map();
const assistantMessageDigest = createHash("sha256")
  .update("mock assistant message")
  .digest("hex");
const finalConclusionDigest = createHash("sha256")
  .update("mock final conclusion")
  .digest("hex");

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
      summary: "p95 latency exceeded the warning threshold.",
    },
    {
      title: "Checkout JVM memory",
      severity: "warning",
      summary:
        "JVM memory pressure needs runtime context before confidence can rise.",
    },
  ],
  recommended_actions: [
    {
      label: "Inspect deployment",
      detail: "Compare checkout deployment timestamps with latency onset.",
      priority: "high",
    },
  ],
  notification_text: "Checkout latency incident requires review.",
  notification_deliveries: [
    {
      id: 801,
      idempotency_key: "final_report:101/notification/final",
      notification_purpose: "final",
      status: "delivered",
      provider_message_id: "wecom-final-report-101",
      provider_status: "accepted",
      delivered_at: "2026-05-28T06:00:12Z",
      created_at: "2026-05-28T06:00:00Z",
      updated_at: "2026-05-28T06:00:12Z",
    },
  ],
  content: {
    title: "Checkout latency incident",
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
          evidence_id: "alert:checkout-latency",
        },
      ],
      recommended_actions: [
        {
          label: "Inspect deployment",
          detail: "Compare checkout deployment timestamps with latency onset.",
          priority: "high",
        },
      ],
      evidence_refs: ["alert:checkout-latency"],
      content: {
        title: "Checkout API latency",
      },
      model: "gpt-4.1-mini",
      output_mode: "json_schema",
      created_by_workflow: "ReportFanOutWorkflow",
      created_at: "2026-05-28T05:59:00Z",
      diagnosis_conclusion: {
        diagnosis_task_id: 301,
        session_id: "diagnosis-session-301",
        chat_session_id: 401,
        event_kind: "diagnosis_room.final_conclusion_ready",
        status: "available",
        source: "latest_assistant_turn",
        reason: "assistant_marked_final",
        evidence_snapshot_id: 9001,
        conclusion_version: "diagnosis-room-final-ready.v1",
        supplemental_context_refs: [
          "chat_session:401/turn:500",
          "chat_session:401/turn:501",
        ],
        confidence_timeline: [
          {
            event_kind: "diagnosis_room.turn_persisted",
            confidence: "low",
            requires_human_review: true,
            conclusion_status: "needs_evidence",
            confidence_rationale:
              "Latency evidence is present but deployment timing is missing.",
            evidence_request_count: 1,
            evidence_requests: [
              {
                tool: "metric_range_query",
                reason: "Need checkout deployment timing.",
                query:
                  "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
                alert_source_profile_id: 7,
                window_seconds: 1800,
                step_seconds: 60,
                limit: 5,
              },
            ],
            evidence_collection_results: [
              {
                tool: "metric_range_query",
                status: "collected",
                reason_code: "ok",
                message: "Metric range collection succeeded.",
                request_reason: "Need checkout deployment timing.",
                query:
                  "histogram_quantile(0.95, rate(checkout_request_duration_seconds_bucket[5m]))",
                template_id: 88,
                alert_source_profile_id: 7,
                alert_source_kind: "prometheus",
                window_seconds: 1800,
                step_seconds: 60,
                limit: 5,
                observed_metric_series: 2,
                collected_at: "2026-05-28T06:02:00Z",
              },
            ],
            missing_evidence_requests: [
              {
                label: "Deployment window",
                detail:
                  "Provide checkout deployment timing before raising confidence.",
                priority: "high",
              },
            ],
            evidence_collection_suggestions: [
              {
                label: "Latency trend",
                detail:
                  "Collect a bounded checkout p95 range query for the incident window.",
                priority: "medium",
              },
            ],
            assistant_message_id: "mock-report-0/assistant",
            assistant_turn_id: 499,
            assistant_sequence: 1,
            turn_count: 1,
            occurred_at: "2026-05-28T06:01:30Z",
          },
          {
            event_kind: "diagnosis_room.turn_persisted",
            confidence: "high",
            requires_human_review: false,
            conclusion_status: "ready_for_review",
            confidence_rationale:
              "Deployment evidence explains the latency onset.",
            evidence_request_count: 0,
            assistant_message_id: "mock-report-1/assistant",
            assistant_turn_id: 501,
            assistant_sequence: 2,
            turn_count: 2,
            occurred_at: "2026-05-28T06:02:30Z",
          },
        ],
        supplemental_evidence: [
          {
            label: "Deployment window",
            detail:
              "Compare checkout deployment timestamps with the latency onset.",
            priority: "high",
            evidence:
              "The payment deployment started two minutes before checkout p95 crossed the warning threshold.",
            context_refs: [
              "chat_session:401/turn:500",
              "chat_session:401/turn:501",
            ],
            user_message_id: "mock-report-1/user",
            assistant_message_id: "mock-report-1/assistant",
            user_turn_id: 500,
            assistant_turn_id: 501,
            provided_at: "2026-05-28T06:02:30Z",
          },
        ],
        assistant_turn_id: 501,
        assistant_message_id: "msg-1/assistant",
        assistant_sequence: 2,
        assistant_occurred_at: "2026-05-28T06:03:00Z",
        content:
          "Checkout latency remains correlated with the payment deployment.",
        confidence: "high",
        requires_human_review: true,
        recorded_at: "2026-05-28T06:03:01Z",
      },
      diagnosis_room: {
        session_id: "diagnosis-session-301",
        chat_session_id: 401,
        diagnosis_task_id: 301,
        evidence_snapshot_id: 9001,
        workflow_id: "diagnosis-room-diagnosis-session-301",
        run_id: "run-301",
        task_status: "running",
        room_status: "open",
        turn_count: 2,
        started_at: "2026-05-28T06:00:30Z",
        last_activity_at: "2026-05-28T06:03:01Z",
        closed_at: null,
        close_reason: "",
        latest_conclusion: {
          event_kind: "diagnosis_room.final_conclusion_ready",
          status: "available",
          source: "latest_assistant_turn",
          reason: "assistant_marked_final",
          evidence_snapshot_id: 9001,
          conclusion_version: "diagnosis-room-final-ready.v1",
          recorded_at: "2026-05-28T06:03:01Z",
          content:
            "Checkout latency remains correlated with the payment deployment.",
          confidence: "high",
          requires_human_review: true,
        },
        notification_timeline: [
          {
            content_kind: "final_conclusion",
            content_sha256: finalConclusionDigest,
            event_kind: "diagnosis_room.final_ready_notification_sent",
            notification_channel_profile_id: 2,
            provider_status: "delivered",
            provider_message_id: "wecom-report-final-ready-301",
            assistant_message_id: "msg-1/assistant",
            assistant_turn_id: 501,
            assistant_sequence: 2,
            turn_count: 2,
            confidence: "high",
            requires_human_review: true,
            occurred_at: "2026-05-28T06:03:02Z",
          },
        ],
        created_at: "2026-05-28T06:00:30Z",
        updated_at: "2026-05-28T06:03:01Z",
      },
    },
    {
      id: 502,
      evidence_snapshot_id: 9003,
      scenario: "single_alert",
      title: "Checkout JVM memory",
      summary:
        "JVM memory pressure needs runtime context before confidence can rise.",
      severity: "warning",
      confidence: "medium",
      findings: [
        {
          label: "JVM memory pressure",
          detail: "Heap usage is elevated in the checkout runtime.",
          evidence_id: "alert:checkout-jvm-memory",
        },
      ],
      recommended_actions: [
        {
          label: "Collect runtime context",
          detail:
            "Attach recent restart and deployment details before confirming the diagnosis.",
          priority: "high",
        },
      ],
      evidence_refs: ["alert:checkout-jvm-memory"],
      content: {
        title: "Checkout JVM memory",
      },
      model: "gpt-4.1-mini",
      output_mode: "json_schema",
      created_by_workflow: "ReportFanOutWorkflow",
      created_at: "2026-05-28T05:59:20Z",
      diagnosis_progress: {
        diagnosis_task_id: 302,
        session_id: "diagnosis-session-302",
        chat_session_id: 402,
        event_kind: "diagnosis_room.turn_persisted",
        status: "in_progress",
        evidence_snapshot_id: 9003,
        confidence: "medium",
        requires_human_review: true,
        conclusion_status: "needs_evidence",
        confidence_rationale:
          "JVM pressure is visible, but restart and deployment context is still missing.",
        evidence_request_count: 1,
        evidence_requests: [
          {
            tool: "metric_range_query",
            reason: "Need checkout JVM heap trend for the incident window.",
            query: 'jvm_memory_used_bytes{service="checkout",area="heap"}',
            window_seconds: 1800,
            step_seconds: 60,
            limit: 5,
          },
        ],
        missing_evidence_requests: [
          {
            label: "Runtime context",
            detail:
              "Attach recent restart and deployment details before confirming the diagnosis.",
            priority: "high",
          },
          {
            label: "Owner mitigation",
            detail:
              "Attach the service owner's mitigation note before final confirmation.",
            priority: "medium",
          },
        ],
        evidence_collection_suggestions: [
          {
            label: "JVM heap trend",
            detail:
              "Collect a bounded JVM heap range query for the incident window.",
            priority: "medium",
          },
          {
            label: "Pod restart history",
            detail:
              "Collect recent pod restart history for the affected checkout workload.",
            priority: "medium",
          },
        ],
        confidence_timeline: [
          {
            event_kind: "diagnosis_room.turn_persisted",
            confidence: "medium",
            requires_human_review: true,
            conclusion_status: "needs_evidence",
            confidence_rationale:
              "JVM pressure is visible, but restart and deployment context is still missing.",
            evidence_request_count: 1,
            evidence_requests: [
              {
                tool: "metric_range_query",
                reason: "Need checkout JVM heap trend for the incident window.",
                query: 'jvm_memory_used_bytes{service="checkout",area="heap"}',
                window_seconds: 1800,
                step_seconds: 60,
                limit: 5,
              },
            ],
            missing_evidence_requests: [
              {
                label: "Runtime context",
                detail:
                  "Attach recent restart and deployment details before confirming the diagnosis.",
                priority: "high",
              },
              {
                label: "Owner mitigation",
                detail:
                  "Attach the service owner's mitigation note before final confirmation.",
                priority: "medium",
              },
            ],
            evidence_collection_suggestions: [
              {
                label: "JVM heap trend",
                detail:
                  "Collect a bounded JVM heap range query for the incident window.",
                priority: "medium",
              },
              {
                label: "Pod restart history",
                detail:
                  "Collect recent pod restart history for the affected checkout workload.",
                priority: "medium",
              },
            ],
            assistant_message_id: "mock-report-2/assistant",
            assistant_turn_id: 502,
            assistant_sequence: 1,
            turn_count: 1,
            occurred_at: "2026-05-28T06:02:45Z",
          },
        ],
        assistant_message_id: "mock-report-2/assistant",
        assistant_turn_id: 502,
        assistant_sequence: 1,
        turn_count: 1,
        occurred_at: "2026-05-28T06:02:45Z",
        recorded_at: "2026-05-28T06:02:46Z",
      },
    },
  ],
};

const dashboard = {
  generated_at: "2026-05-28T06:01:00Z",
  alerts: {
    total_recent: 24,
    firing: 7,
    resolved: 17,
  },
  diagnosis: {
    linked_snapshots: 2,
    rooms_started: 3,
    snapshots_needing_room: 1,
    affected_alerts_needing_room: 1,
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
      critical: 1,
    },
  },
};

const alertEvents = [
  {
    id: 7001,
    source: "alertmanager",
    alert_source_profile_id: 1,
    source_fingerprint: "am:checkout-latency:prod",
    canonical_fingerprint: "sha256:checkout-latency-prod",
    labels: {
      alertname: "CheckoutLatencyHigh",
      namespace: "prod",
      service: "checkout",
      severity: "warning",
    },
    annotations: {
      summary: "Checkout p95 latency is above the warning threshold.",
    },
    status: "firing",
    starts_at: "2026-05-28T05:58:30Z",
    ends_at: null,
    linked_evidence_snapshots: [
      {
        id: 9001,
        alert_group_id: 3001,
        digest: "sha256:evidence-checkout-latency",
        status: "complete",
        created_by_workflow: "AlertmanagerWebhookAutoDiagnosis",
        created_at: "2026-05-28T06:00:00Z",
        diagnosis_rooms: [
          {
            session_id: "diagnosis-session-auto-p1-s9001",
            chat_session_id: 401,
            diagnosis_task_id: 301,
            evidence_snapshot_id: 9001,
            workflow_id: "diagnosis-room-diagnosis-session-auto-p1-s9001",
            run_id: "run-auto-9001",
            task_status: "running",
            room_status: "open",
            turn_count: 1,
            started_at: "2026-05-28T06:00:30Z",
            last_activity_at: "2026-05-28T06:01:30Z",
            closed_at: null,
            close_reason: "",
            latest_conclusion: {
              event_kind: "diagnosis_room.final_conclusion_ready",
              status: "available",
              source: "assistant",
              reason: "",
              evidence_snapshot_id: 9001,
              conclusion_version: "diagnosis-session-auto-p1-s9001:1",
              recorded_at: "2026-05-28T06:01:30Z",
              content:
                "Checkout latency is correlated with downstream saturation.",
              confidence: "high",
              requires_human_review: false,
            },
            notification_timeline: [
              {
                content_kind: "assistant_message",
                content_sha256: assistantMessageDigest,
                event_kind: "diagnosis_room.assistant_turn_notification_sent",
                notification_channel_profile_id: 2,
                provider_status: "delivered",
                provider_message_id: "wecom-msg-1",
                assistant_message_id: "msg-1/assistant",
                assistant_turn_id: 402,
                assistant_sequence: 2,
                turn_count: 1,
                confidence: "low",
                requires_human_review: true,
                occurred_at: "2026-05-28T06:01:31Z",
              },
              {
                content_kind: "final_conclusion",
                content_sha256: finalConclusionDigest,
                event_kind: "diagnosis_room.final_ready_notification_sent",
                notification_channel_profile_id: 2,
                provider_status: "delivered",
                provider_message_id: "wecom-msg-2",
                assistant_message_id: "msg-1/assistant",
                assistant_turn_id: 402,
                assistant_sequence: 2,
                turn_count: 1,
                confidence: "high",
                requires_human_review: false,
                occurred_at: "2026-05-28T06:01:32Z",
              },
            ],
            created_at: "2026-05-28T06:00:30Z",
            updated_at: "2026-05-28T06:01:30Z",
          },
          {
            session_id: "diagnosis-session-notification-failed",
            chat_session_id: 408,
            diagnosis_task_id: 308,
            evidence_snapshot_id: 9001,
            workflow_id: "diagnosis-room-diagnosis-session-notification-failed",
            run_id: "run-notification-failed-9001",
            task_status: "running",
            room_status: "open",
            turn_count: 1,
            started_at: "2026-05-28T06:02:30Z",
            last_activity_at: "2026-05-28T06:03:30Z",
            closed_at: null,
            close_reason: "",
            latest_conclusion: {
              event_kind: "diagnosis_room.final_conclusion_ready",
              status: "available",
              source: "assistant",
              reason: "",
              evidence_snapshot_id: 9001,
              conclusion_version: "diagnosis-session-notification-failed:1",
              recorded_at: "2026-05-28T06:03:30Z",
              content:
                "Checkout latency conclusion is ready, but final notification delivery failed.",
              confidence: "high",
              requires_human_review: false,
            },
            notification_timeline: [
              {
                event_kind: "diagnosis_room.final_ready_notification_sent",
                notification_channel_profile_id: 2,
                provider_status: "failed",
                provider_message_id: "",
                assistant_message_id: "msg-failed/assistant",
                assistant_turn_id: 4082,
                assistant_sequence: 2,
                turn_count: 1,
                confidence: "high",
                requires_human_review: false,
                occurred_at: "2026-05-28T06:03:32Z",
              },
            ],
            created_at: "2026-05-28T06:02:30Z",
            updated_at: "2026-05-28T06:03:30Z",
          },
          {
            session_id: "diagnosis-session-closed-finalized",
            chat_session_id: 410,
            diagnosis_task_id: 310,
            evidence_snapshot_id: 9001,
            workflow_id: "diagnosis-room-diagnosis-session-closed-finalized",
            run_id: "run-closed-finalized-9001",
            task_status: "completed",
            room_status: "closed",
            turn_count: 2,
            started_at: "2026-05-28T06:04:30Z",
            last_activity_at: "2026-05-28T06:06:00Z",
            closed_at: "2026-05-28T06:06:00Z",
            close_reason: "operator_confirmed_final_conclusion",
            latest_conclusion: {
              event_kind: "diagnosis_room.final_conclusion_ready",
              status: "available",
              source: "assistant",
              reason: "",
              evidence_snapshot_id: 9001,
              conclusion_version: "diagnosis-session-closed-finalized:2",
              recorded_at: "2026-05-28T06:05:30Z",
              content:
                "Checkout latency was confirmed after operator evidence review and the room was closed.",
              confidence: "high",
              requires_human_review: false,
            },
            notification_timeline: [
              {
                content_kind: "final_conclusion",
                content_sha256: finalConclusionDigest,
                event_kind: "diagnosis_room.final_ready_notification_sent",
                notification_channel_profile_id: 2,
                provider_status: "delivered",
                provider_message_id: "wecom-msg-final-ready-closed",
                assistant_message_id: "msg-closed/assistant",
                assistant_turn_id: 4102,
                assistant_sequence: 4,
                turn_count: 2,
                confidence: "high",
                requires_human_review: false,
                occurred_at: "2026-05-28T06:05:32Z",
              },
              {
                event_kind: "diagnosis_room.close_notification_sent",
                notification_channel_profile_id: 2,
                provider_status: "delivered",
                provider_message_id: "wecom-msg-close-closed",
                assistant_message_id: "msg-closed/assistant",
                assistant_turn_id: 4102,
                assistant_sequence: 4,
                turn_count: 2,
                confidence: "high",
                requires_human_review: false,
                occurred_at: "2026-05-28T06:06:01Z",
              },
            ],
            created_at: "2026-05-28T06:04:30Z",
            updated_at: "2026-05-28T06:06:00Z",
          },
        ],
      },
    ],
    created_at: "2026-05-28T05:58:31Z",
  },
  {
    id: 7002,
    source: "alertmanager",
    alert_source_profile_id: 1,
    source_fingerprint: "am:node-disk:prod",
    canonical_fingerprint: "sha256:node-disk-prod",
    labels: {
      alertname: "NodeDiskPressure",
      instance: "node-a",
      severity: "critical",
    },
    annotations: {
      summary: "Node disk pressure crossed the critical threshold.",
    },
    status: "resolved",
    starts_at: "2026-05-28T04:40:00Z",
    ends_at: "2026-05-28T05:10:00Z",
    linked_evidence_snapshots: [],
    created_at: "2026-05-28T04:40:02Z",
  },
  {
    id: 7003,
    source: "alertmanager",
    alert_source_profile_id: 1,
    source_fingerprint: "am:payment-errors:prod",
    canonical_fingerprint: "sha256:payment-errors-prod",
    labels: {
      alertname: "PaymentErrorRateHigh",
      namespace: "prod",
      service: "payments",
      severity: "warning",
    },
    annotations: {
      summary: "Payment error rate is above the warning threshold.",
    },
    status: "firing",
    starts_at: "2026-05-28T06:08:00Z",
    ends_at: null,
    linked_evidence_snapshots: [
      {
        id: 9004,
        alert_group_id: 3004,
        digest: "sha256:evidence-payment-errors",
        status: "complete",
        created_by_workflow: "AlertmanagerWebhookAutoDiagnosis",
        created_at: "2026-05-28T06:08:30Z",
        diagnosis_rooms: [],
      },
    ],
    created_at: "2026-05-28T06:08:01Z",
  },
];

const diagnosisHandoffs = [
  {
    reason: "missing_diagnosis_room",
    evidence_snapshot: alertEvents[2].linked_evidence_snapshots[0],
    alerts: [alertEvents[2]],
  },
];

const evidenceSnapshots = [
  {
    id: 9001,
    alert_group_id: 3001,
    digest: "sha256:evidence-checkout-latency",
    payload: {
      schema_version: "m1.evidence_snapshot.v1",
      group: {
        id: 3001,
        group_key: "alertname=CheckoutLatencyHigh,service=checkout",
        dimensions: {
          alertname: "CheckoutLatencyHigh",
          service: "checkout",
        },
        severity: "warning",
        event_count: 1,
        first_seen_at: "2026-05-28T05:58:30Z",
        last_seen_at: "2026-05-28T05:58:30Z",
      },
      events: [
        {
          id: 7001,
          source: "alertmanager",
          alert_source_profile_id: 1,
          source_fingerprint: "am:checkout-latency:prod",
          canonical_fingerprint: "sha256:checkout-latency-prod",
          labels: {
            alertname: "CheckoutLatencyHigh",
            namespace: "prod",
            service: "checkout",
            severity: "warning",
          },
          annotations: {
            summary: "Checkout p95 latency is above the warning threshold.",
          },
          status: "firing",
          starts_at: "2026-05-28T05:58:30Z",
          ends_at: null,
          raw_payload: null,
        },
      ],
    },
    provenance: {
      "openclarion.core": "ok",
    },
    status: "complete",
    missing_fields: [],
    created_by_workflow: "AlertmanagerWebhookAutoDiagnosis",
    created_at: "2026-05-28T06:00:00Z",
  },
];

const diagnosisRooms = [
  {
    session_id: "diagnosis-session-auto-p1-s9001",
    chat_session_id: 401,
    diagnosis_task_id: 301,
    evidence_snapshot_id: 9001,
    workflow_id: "diagnosis-room-diagnosis-session-auto-p1-s9001",
    run_id: "run-auto-9001",
    task_status: "running",
    room_status: "open",
    turn_count: 1,
    started_at: "2026-05-28T06:00:30Z",
    last_activity_at: "2026-05-28T06:01:30Z",
    closed_at: null,
    close_reason: "",
    latest_conclusion: {
      event_kind: "diagnosis_room.final_conclusion_ready",
      status: "available",
      source: "assistant",
      reason: "",
      evidence_snapshot_id: 9001,
      conclusion_version: "diagnosis-session-auto-p1-s9001:1",
      recorded_at: "2026-05-28T06:01:30Z",
      content: "Checkout latency is correlated with downstream saturation.",
      confidence: "high",
      requires_human_review: false,
    },
    notification_timeline: [
      {
        content_kind: "assistant_message",
        content_sha256: assistantMessageDigest,
        event_kind: "diagnosis_room.assistant_turn_notification_sent",
        notification_channel_profile_id: 2,
        provider_status: "delivered",
        provider_message_id: "wecom-msg-1",
        assistant_message_id: "msg-1/assistant",
        assistant_turn_id: 402,
        assistant_sequence: 2,
        turn_count: 1,
        confidence: "low",
        requires_human_review: true,
        occurred_at: "2026-05-28T06:01:31Z",
      },
      {
        content_kind: "final_conclusion",
        content_sha256: finalConclusionDigest,
        event_kind: "diagnosis_room.final_ready_notification_sent",
        notification_channel_profile_id: 2,
        provider_status: "delivered",
        provider_message_id: "wecom-msg-2",
        assistant_message_id: "msg-1/assistant",
        assistant_turn_id: 402,
        assistant_sequence: 2,
        turn_count: 1,
        confidence: "high",
        requires_human_review: false,
        occurred_at: "2026-05-28T06:01:32Z",
      },
    ],
    created_at: "2026-05-28T06:00:30Z",
    updated_at: "2026-05-28T06:01:30Z",
  },
  {
    session_id: "diagnosis-session-notification-failed",
    chat_session_id: 408,
    diagnosis_task_id: 308,
    evidence_snapshot_id: 9001,
    workflow_id: "diagnosis-room-diagnosis-session-notification-failed",
    run_id: "run-notification-failed-9001",
    task_status: "running",
    room_status: "open",
    turn_count: 1,
    started_at: "2026-05-28T06:02:30Z",
    last_activity_at: "2026-05-28T06:03:30Z",
    closed_at: null,
    close_reason: "",
    latest_conclusion: {
      event_kind: "diagnosis_room.final_conclusion_ready",
      status: "available",
      source: "assistant",
      reason: "",
      evidence_snapshot_id: 9001,
      conclusion_version: "diagnosis-session-notification-failed:1",
      recorded_at: "2026-05-28T06:03:30Z",
      content:
        "Checkout latency conclusion is ready, but final notification delivery failed.",
      confidence: "high",
      requires_human_review: false,
    },
    notification_timeline: [
      {
        event_kind: "diagnosis_room.final_ready_notification_sent",
        notification_channel_profile_id: 2,
        provider_status: "failed",
        provider_message_id: "",
        assistant_message_id: "msg-failed/assistant",
        assistant_turn_id: 4082,
        assistant_sequence: 2,
        turn_count: 1,
        confidence: "high",
        requires_human_review: false,
        occurred_at: "2026-05-28T06:03:32Z",
      },
    ],
    created_at: "2026-05-28T06:02:30Z",
    updated_at: "2026-05-28T06:03:30Z",
  },
  {
    session_id: "diagnosis-session-closed-finalized",
    chat_session_id: 410,
    diagnosis_task_id: 310,
    evidence_snapshot_id: 9001,
    workflow_id: "diagnosis-room-diagnosis-session-closed-finalized",
    run_id: "run-closed-finalized-9001",
    task_status: "completed",
    room_status: "closed",
    turn_count: 2,
    started_at: "2026-05-28T06:04:30Z",
    last_activity_at: "2026-05-28T06:06:00Z",
    closed_at: "2026-05-28T06:06:00Z",
    close_reason: "operator_confirmed_final_conclusion",
    latest_conclusion: {
      event_kind: "diagnosis_room.final_conclusion_ready",
      status: "available",
      source: "assistant",
      reason: "",
      evidence_snapshot_id: 9001,
      conclusion_version: "diagnosis-session-closed-finalized:2",
      recorded_at: "2026-05-28T06:05:30Z",
      content:
        "Checkout latency was confirmed after operator evidence review and the room was closed.",
      confidence: "high",
      requires_human_review: false,
    },
    notification_timeline: [
      {
        content_kind: "final_conclusion",
        content_sha256: finalConclusionDigest,
        event_kind: "diagnosis_room.final_ready_notification_sent",
        notification_channel_profile_id: 2,
        provider_status: "delivered",
        provider_message_id: "wecom-msg-final-ready-closed",
        assistant_message_id: "msg-closed/assistant",
        assistant_turn_id: 4102,
        assistant_sequence: 4,
        turn_count: 2,
        confidence: "high",
        requires_human_review: false,
        occurred_at: "2026-05-28T06:05:32Z",
      },
      {
        event_kind: "diagnosis_room.close_notification_sent",
        notification_channel_profile_id: 2,
        provider_status: "delivered",
        provider_message_id: "wecom-msg-close-closed",
        assistant_message_id: "msg-closed/assistant",
        assistant_turn_id: 4102,
        assistant_sequence: 4,
        turn_count: 2,
        confidence: "high",
        requires_human_review: false,
        occurred_at: "2026-05-28T06:06:01Z",
      },
    ],
    created_at: "2026-05-28T06:04:30Z",
    updated_at: "2026-05-28T06:06:00Z",
  },
  {
    session_id: "diagnosis-session-orphaned-workflow",
    chat_session_id: 409,
    diagnosis_task_id: 309,
    evidence_snapshot_id: 9001,
    workflow_id: "diagnosis-room-diagnosis-session-orphaned-workflow",
    run_id: "run-orphaned-9001",
    task_status: "running",
    room_status: "open",
    turn_count: 1,
    started_at: "2026-05-28T06:03:30Z",
    last_activity_at: "2026-05-28T06:04:30Z",
    closed_at: null,
    close_reason: "",
    workflow_visibility: {
      status: "not_found",
      task_queue: "diagnosis-room",
      history_length: 0,
      close_time: "2026-05-28T06:05:00Z",
    },
    notification_timeline: [],
    created_at: "2026-05-28T06:03:30Z",
    updated_at: "2026-05-28T06:04:30Z",
  },
];

let nextAlertSourceID = 3;
const alertSources = [
  {
    id: 1,
    name: "Primary Prometheus",
    kind: "prometheus",
    base_url: "https://prometheus.example.test",
    auth_mode: "bearer",
    secret_ref: "secret/openclarion/prometheus-bearer",
    enabled: true,
    labels: {
      env: "prod",
      owner: "platform",
    },
    created_at: "2026-05-28T06:00:00Z",
    updated_at: "2026-05-28T06:00:00Z",
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
      env: "staging",
    },
    created_at: "2026-05-28T06:00:00Z",
    updated_at: "2026-05-28T06:00:00Z",
  },
];

let nextGroupingPolicyID = 2;
const groupingPolicies = [
  {
    id: 1,
    name: "Default alert grouping",
    dimension_keys: ["alertname", "service"],
    severity_key: "severity",
    source_filter: ["prometheus"],
    enabled: true,
    created_at: "2026-06-05T04:00:00Z",
    updated_at: "2026-06-05T04:00:00Z",
  },
];

let nextReportWorkflowPolicyID = 3;
const diagnosisFollowUpModes = new Set([
  "disabled",
  "suggest_room",
  "auto_room",
]);
const reportWorkflowPolicies = [
  {
    id: 1,
    name: "Default report workflow",
    alert_source_profile_id: 1,
    grouping_policy_id: 1,
    report_notification_channel_profile_id: null,
    trigger_mode: "manual_replay",
    report_scenario: "single_alert",
    diagnosis_follow_up: "suggest_room",
    enabled: true,
    enabled_at: "2026-06-05T08:05:00Z",
    disabled_at: null,
    created_at: "2026-06-05T08:00:00Z",
    updated_at: "2026-06-05T08:05:00Z",
  },
  {
    id: 2,
    name: "Disabled report workflow",
    alert_source_profile_id: 1,
    grouping_policy_id: 1,
    report_notification_channel_profile_id: null,
    trigger_mode: "manual_replay",
    report_scenario: "single_alert",
    diagnosis_follow_up: "disabled",
    enabled: false,
    enabled_at: null,
    disabled_at: "2026-06-05T08:10:00Z",
    created_at: "2026-06-05T08:00:00Z",
    updated_at: "2026-06-05T08:10:00Z",
  },
];

let nextReportWorkflowScheduleID = 3;
const reportWorkflowSchedules = [
  {
    id: 1,
    name: "Daily report window",
    report_workflow_policy_id: 1,
    temporal_schedule_id: "openclarion-report-policy-1-daily",
    interval_seconds: 86400,
    offset_seconds: 21600,
    replay_window_seconds: 3600,
    replay_delay_seconds: 300,
    replay_limit: 10000,
    catchup_window_seconds: 3600,
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-06T02:00:00Z",
    updated_at: "2026-06-06T02:00:00Z",
  },
  {
    id: 2,
    name: "Disabled policy report window",
    report_workflow_policy_id: 2,
    temporal_schedule_id: "openclarion-report-policy-2-daily",
    interval_seconds: 86400,
    offset_seconds: 0,
    replay_window_seconds: 3600,
    replay_delay_seconds: 300,
    replay_limit: 10000,
    catchup_window_seconds: 3600,
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-06T02:00:00Z",
    updated_at: "2026-06-06T02:00:00Z",
  },
];

let nextDiagnosisToolTemplateID = 2;
const diagnosisToolTemplates = [
  {
    id: 1,
    name: "CPU saturation range",
    alert_source_profile_id: 1,
    tool: "metric_range_query",
    query_template: "rate(container_cpu_usage_seconds_total[5m])",
    default_limit: 5,
    default_window_seconds: 3600,
    max_window_seconds: 21600,
    default_step_seconds: 60,
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-08T08:00:00Z",
    updated_at: "2026-06-08T08:00:00Z",
  },
];

let nextNotificationChannelID = 2;
const notificationChannels = [
  {
    id: 1,
    name: "Operations WeCom",
    kind: "wecom",
    secret_ref: "secret/example/ops-wecom",
    delivery_scopes: ["report"],
    enabled: false,
    labels: {
      team: "ops",
    },
    created_at: "2026-06-05T09:00:00Z",
    updated_at: "2026-06-05T09:00:00Z",
  },
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
  if (url.pathname === "/api/v1/config/grouping-policies") {
    handleGroupingPolicyCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/report-workflow-policies") {
    handleReportWorkflowPolicyCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/report-workflow-schedules") {
    handleReportWorkflowScheduleCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/diagnosis-tool-templates") {
    handleDiagnosisToolTemplateCollection(request, response);
    return;
  }
  if (url.pathname === "/api/v1/config/notification-channels") {
    handleNotificationChannelCollection(request, response);
    return;
  }
  const alertSourceTestMatch = url.pathname.match(
    /^\/api\/v1\/config\/alert-sources\/(\d+)\/test$/,
  );
  if (alertSourceTestMatch) {
    handleAlertSourceConnectionTest(
      request,
      response,
      Number.parseInt(alertSourceTestMatch[1], 10),
    );
    return;
  }
  const alertSourceMatch = url.pathname.match(
    /^\/api\/v1\/config\/alert-sources\/(\d+)$/,
  );
  if (alertSourceMatch) {
    handleAlertSourceProfile(
      request,
      response,
      Number.parseInt(alertSourceMatch[1], 10),
    );
    return;
  }
  const groupingPolicyPreviewMatch = url.pathname.match(
    /^\/api\/v1\/config\/grouping-policies\/(\d+)\/preview$/,
  );
  if (groupingPolicyPreviewMatch) {
    handleGroupingPolicyPreview(
      request,
      response,
      Number.parseInt(groupingPolicyPreviewMatch[1], 10),
    );
    return;
  }
  const groupingPolicyMatch = url.pathname.match(
    /^\/api\/v1\/config\/grouping-policies\/(\d+)$/,
  );
  if (groupingPolicyMatch) {
    handleGroupingPolicy(
      request,
      response,
      Number.parseInt(groupingPolicyMatch[1], 10),
    );
    return;
  }
  const reportWorkflowPolicyEnableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/enable$/,
  );
  if (reportWorkflowPolicyEnableMatch) {
    handleReportWorkflowPolicyEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyEnableMatch[1], 10),
      true,
    );
    return;
  }
  const reportWorkflowPolicyDisableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/disable$/,
  );
  if (reportWorkflowPolicyDisableMatch) {
    handleReportWorkflowPolicyEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyDisableMatch[1], 10),
      false,
    );
    return;
  }
  const reportWorkflowPolicyImpactPreviewMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/impact-preview$/,
  );
  if (reportWorkflowPolicyImpactPreviewMatch) {
    handleReportWorkflowPolicyImpactPreview(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyImpactPreviewMatch[1], 10),
    );
    return;
  }
  const reportWorkflowPolicyReplayMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)\/replay-window$/,
  );
  if (reportWorkflowPolicyReplayMatch) {
    handleReportWorkflowPolicyReplay(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyReplayMatch[1], 10),
    );
    return;
  }
  const reportWorkflowPolicyMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-policies\/(\d+)$/,
  );
  if (reportWorkflowPolicyMatch) {
    handleReportWorkflowPolicy(
      request,
      response,
      Number.parseInt(reportWorkflowPolicyMatch[1], 10),
    );
    return;
  }
  const reportWorkflowScheduleEnableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-schedules\/(\d+)\/enable$/,
  );
  if (reportWorkflowScheduleEnableMatch) {
    handleReportWorkflowScheduleEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowScheduleEnableMatch[1], 10),
      true,
    );
    return;
  }
  const reportWorkflowScheduleDisableMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-schedules\/(\d+)\/disable$/,
  );
  if (reportWorkflowScheduleDisableMatch) {
    handleReportWorkflowScheduleEnablement(
      request,
      response,
      Number.parseInt(reportWorkflowScheduleDisableMatch[1], 10),
      false,
    );
    return;
  }
  const reportWorkflowScheduleMatch = url.pathname.match(
    /^\/api\/v1\/config\/report-workflow-schedules\/(\d+)$/,
  );
  if (reportWorkflowScheduleMatch) {
    handleReportWorkflowSchedule(
      request,
      response,
      Number.parseInt(reportWorkflowScheduleMatch[1], 10),
    );
    return;
  }
  const diagnosisToolTemplateEnableMatch = url.pathname.match(
    /^\/api\/v1\/config\/diagnosis-tool-templates\/(\d+)\/enable$/,
  );
  if (diagnosisToolTemplateEnableMatch) {
    handleDiagnosisToolTemplateEnablement(
      request,
      response,
      Number.parseInt(diagnosisToolTemplateEnableMatch[1], 10),
      true,
    );
    return;
  }
  const diagnosisToolTemplateDisableMatch = url.pathname.match(
    /^\/api\/v1\/config\/diagnosis-tool-templates\/(\d+)\/disable$/,
  );
  if (diagnosisToolTemplateDisableMatch) {
    handleDiagnosisToolTemplateEnablement(
      request,
      response,
      Number.parseInt(diagnosisToolTemplateDisableMatch[1], 10),
      false,
    );
    return;
  }
  const diagnosisToolTemplateMatch = url.pathname.match(
    /^\/api\/v1\/config\/diagnosis-tool-templates\/(\d+)$/,
  );
  if (diagnosisToolTemplateMatch) {
    handleDiagnosisToolTemplate(
      request,
      response,
      Number.parseInt(diagnosisToolTemplateMatch[1], 10),
    );
    return;
  }
  const notificationChannelTestMatch = url.pathname.match(
    /^\/api\/v1\/config\/notification-channels\/(\d+)\/test$/,
  );
  if (notificationChannelTestMatch) {
    handleNotificationChannelTest(
      request,
      response,
      Number.parseInt(notificationChannelTestMatch[1], 10),
    );
    return;
  }
  const notificationChannelMatch = url.pathname.match(
    /^\/api\/v1\/config\/notification-channels\/(\d+)$/,
  );
  if (notificationChannelMatch) {
    handleNotificationChannel(
      request,
      response,
      Number.parseInt(notificationChannelMatch[1], 10),
    );
    return;
  }
  const diagnosisRoomCloseUnavailableMatch = url.pathname.match(
    /^\/api\/v1\/diagnosis\/rooms\/([^/]+)\/close-unavailable$/,
  );
  if (diagnosisRoomCloseUnavailableMatch) {
    handleDiagnosisRoomCloseUnavailable(
      request,
      response,
      decodeURIComponent(diagnosisRoomCloseUnavailableMatch[1]),
    );
    return;
  }
  const diagnosisRoomMatch = url.pathname.match(
    /^\/api\/v1\/diagnosis\/rooms\/([^/]+)$/,
  );
  if (diagnosisRoomMatch) {
    handleDiagnosisRoom(
      request,
      response,
      decodeURIComponent(diagnosisRoomMatch[1]),
    );
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
    case "/api/v1/alerts":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { items: alertEvents });
      return;
    case "/api/v1/report-triggers/replay-window":
      handleReportReplay(request, response);
      return;
    case "/api/v1/evidence-snapshots":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { items: evidenceSnapshots });
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
    case "/api/v1/diagnosis/rooms":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { items: diagnosisRooms });
      return;
    case "/api/v1/diagnosis/handoffs":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, { items: diagnosisHandoffs });
      return;
    case "/api/v1/diagnosis/auth/status":
      if (request.method !== "GET") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      writeJSON(response, 200, {
        configured: true,
        mode: "ldap",
        role_mapping: {
          admin_mapping_count: 0,
          configured: true,
          default_roles: [],
          owner_mapping_count: 1,
        },
        supported_modes: ["ldap", "oidc"],
      });
      return;
    case "/api/v1/config/rbac/current-authorizations":
      if (request.method !== "POST") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      {
        const principal = requireDiagnosisAuthorization(request, response);
        if (!principal) {
          return;
        }
        readJSON(request)
          .then((body) => {
            const requests = Array.isArray(body.requests)
              ? body.requests
              : [];
            const allowed = principal.roles.some(
              (role) => role === "owner" || role === "admin",
            );
            writeJSON(response, 200, {
              subject: principal.subject,
              department_keys: ["dep-ops"],
              directory_users: [directoryUserForPrincipal(principal)],
              decisions: requests.map((item) => ({
                permission:
                  typeof item.permission === "string"
                    ? item.permission
                    : "directory.read",
                scope_kind:
                  typeof item.scope_kind === "string"
                    ? item.scope_kind
                    : "global",
                scope_key:
                  typeof item.scope_key === "string" ? item.scope_key : "",
                allowed,
                checked_at: "2026-05-28T10:00:00Z",
              })),
            });
          })
          .catch((error) => {
            writeJSON(response, 400, {
              error: error instanceof Error ? error.message : "invalid JSON",
            });
          });
        return;
      }
    case "/api/v1/diagnosis/auth/session":
      if (request.method !== "POST") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      {
        const principal = requireDiagnosisAuthorization(request, response);
        if (!principal) {
          return;
        }
        writeJSON(response, 201, {
          checked_at: "2026-05-28T10:00:00Z",
          expires_at: "2027-05-28T18:00:00Z",
          mode: principal.mode,
          role_authorized: principal.roles.some(
            (role) => role === "owner" || role === "admin",
          ),
          roles: principal.roles,
          subject: principal.subject,
          token: diagnosisBrowserSessionToken(principal),
        });
        return;
      }
    case "/api/v1/diagnosis/auth/check": {
      if (request.method !== "POST") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      const principal = requireDiagnosisAuthorization(request, response);
      if (!principal) {
        return;
      }
      writeJSON(response, 200, {
        checked_at: "2026-05-28T10:00:00Z",
        mode: principal.mode,
        role_authorized: principal.roles.some(
          (role) => role === "owner" || role === "admin",
        ),
        roles: principal.roles,
        subject: principal.subject,
      });
      return;
    }
    case "/api/v1/diagnosis/ws-ticket":
      if (request.method !== "POST") {
        writeJSON(response, 405, { error: "method not allowed" });
        return;
      }
      {
        const principal = requireDiagnosisAuthorization(request, response);
        if (!principal) {
          return;
        }
        readJSON(request)
          .then((body) => {
            const sessionID =
              typeof body.session_id === "string"
                ? body.session_id.trim()
                : "";
            if (sessionID === "") {
              writeJSON(response, 400, {
                error: "session_id must be non-empty",
              });
              return;
            }
            const ticket = `ticket-${sessionID}`;
            diagnosisWSTickets.set(ticket, {
              sessionID,
              subject: diagnosisWebSocketSubject(principal),
            });
            writeJSON(response, 201, {
              ticket,
              session_id: sessionID,
              expires_at: "2026-05-28T10:00:30Z",
            });
          })
          .catch((error) => {
            writeJSON(response, 400, {
              error: error instanceof Error ? error.message : "invalid JSON",
            });
          });
        return;
      }
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
  const ticketBinding = diagnosisWSTickets.get(ticket);
  if (ticketBinding?.sessionID !== sessionID) {
    socket.destroy();
    return;
  }
  diagnosisWSTickets.delete(ticket);
  const accept = createHash("sha1")
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest("base64");
  socket.write(
    [
      "HTTP/1.1 101 Switching Protocols",
      "Upgrade: websocket",
      "Connection: Upgrade",
      `Sec-WebSocket-Accept: ${accept}`,
      "\r\n",
    ].join("\r\n"),
  );

  const state = {
    status: "open",
    turnCount: 0,
    conversation: [],
    evidenceTimeline: [],
    confidenceTimeline: [],
    finalConclusion: null,
    closedAt: "",
    closeReason: "",
  };
  sendWebSocketJSON(socket, {
    type: "ready",
    session_id: sessionID,
    subject: ticketBinding.subject,
  });

  let buffer = Buffer.alloc(0);
  socket.on("error", (error) => {
    if (error?.code === "ECONNRESET") {
      return;
    }
    console.error(error);
  });
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

function handleDiagnosisRoomCloseUnavailable(request, response, sessionID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  if (!requireDiagnosisAuthorization(request, response)) {
    return;
  }
  const room = diagnosisRooms.find((item) => item.session_id === sessionID);
  if (!room) {
    writeJSON(response, 404, { error: "diagnosis room not found" });
    return;
  }
  if (room.workflow_visibility?.status === "running") {
    writeJSON(response, 400, {
      error: "diagnosis room workflow is still running",
    });
    return;
  }
  readJSON(request)
    .then((body) => {
      const reason =
        typeof body.reason === "string" && body.reason.trim() !== ""
          ? body.reason.trim()
          : "workflow_unavailable";
      const closedAt = "2026-05-28T06:05:00Z";
      room.task_status = "cancelled";
      room.room_status = "closed";
      room.last_activity_at = closedAt;
      room.closed_at = closedAt;
      room.close_reason = reason;
      room.updated_at = closedAt;
      writeJSON(response, 200, room);
    })
    .catch((error) => {
      writeJSON(response, 400, { error: error.message });
    });
}

function handleDiagnosisRoom(request, response, sessionID) {
  if (request.method !== "GET") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const room = findDiagnosisRoom(sessionID);
  if (!room) {
    writeJSON(response, 404, { error: "diagnosis room not found" });
    return;
  }
  writeJSON(response, 200, room);
}

function findDiagnosisRoom(sessionID) {
  return (
    diagnosisRooms.find((room) => room.session_id === sessionID) ??
    report.linked_sub_reports
      .map((subReport) => subReport.diagnosis_room)
      .find((room) => room?.session_id === sessionID)
  );
}

function requireDiagnosisAuthorization(request, response) {
  const principal = diagnosisAuthorizationPrincipal(
    request.headers.authorization,
  );
  if (principal === null) {
    writeJSON(response, 401, { error: "authentication failed" });
    return null;
  }
  return principal;
}

function diagnosisAuthorizationPrincipal(rawAuthorization) {
  if (typeof rawAuthorization !== "string") {
    return null;
  }
  const bearer = rawAuthorization.match(/^Bearer ([^\s]+)$/i);
  if (bearer) {
    if (bearer[1] === "session.token.one") {
      return { mode: "oidc", roles: ["owner"], subject: "operator-1" };
    }
    if (bearer[1] === "ldap.session.one") {
      return { mode: "ldap", roles: ["owner"], subject: "operator-1" };
    }
    return { mode: "static", roles: ["owner"], subject: "owner-1" };
  }
  const basic = rawAuthorization.match(/^Basic ([A-Za-z0-9+/=]+)$/i);
  if (!basic) {
    return null;
  }
  const decoded = Buffer.from(basic[1], "base64").toString("utf8");
  const separatorIndex = decoded.indexOf(":");
  if (separatorIndex <= 0) {
    return null;
  }
  const username = decoded.slice(0, separatorIndex);
  const password = decoded.slice(separatorIndex + 1);
  if (
    username.trim() !== username ||
    /[\u0000\s]/.test(username) ||
    password === "" ||
    /[\u0000\r\n]/.test(password)
  ) {
    return null;
  }
  return { mode: "ldap", roles: ["owner"], subject: username };
}

function diagnosisBrowserSessionToken(principal) {
  return principal.mode === "ldap" ? "ldap.session.one" : "session.token.one";
}

function directoryUserForPrincipal(principal) {
  return {
    id: 1,
    provider: "ops_iam",
    subject: principal.subject,
    external_id: principal.subject,
    username: principal.subject,
    display_name: principal.subject,
    email: `${principal.subject}@example.test`,
    job_title: "Operator",
    department: "Operations",
    section: "SRE",
    department_path: "IT/Operations/SRE",
    department_paths: ["IT/Operations/SRE"],
    department_external_ids: ["dep-ops"],
    active: true,
    synced_at: "2026-05-28T10:00:00Z",
    created_at: "2026-05-28T10:00:00Z",
    updated_at: "2026-05-28T10:00:00Z",
  };
}

function diagnosisWebSocketSubject(principal) {
  return principal.subject === "wecom-user-1" ? principal.subject : "owner-1";
}

function writeJSON(response, status, body) {
  response.writeHead(status, {
    "access-control-allow-headers": "authorization, content-type, accept",
    "access-control-allow-methods": "GET, POST, PUT, OPTIONS",
    "access-control-allow-origin": "*",
    "content-type": "application/json",
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
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleAlertSourceProfile(request, response, profileID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = alertSources.findIndex(
        (profile) => profile.id === profileID,
      );
      if (index < 0) {
        writeJSON(response, 404, { error: "alert source profile not found" });
        return;
      }
      const current = alertSources[index];
      const profile = alertSourceProfileFromBody(
        profileID,
        body,
        current.created_at,
      );
      alertSources[index] = profile;
      writeJSON(response, 200, profile);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleAlertSourceConnectionTest(request, response, profileID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const profile = alertSources.find((item) => item.id === profileID);
  if (!profile) {
    writeJSON(response, 404, { error: "alert source profile not found" });
    return;
  }
  if (profile.auth_mode === "bearer") {
    writeJSON(response, 200, {
      source_id: profile.id,
      kind: profile.kind,
      auth_mode: profile.auth_mode,
      status: "blocked",
      reason_code: "credentials_unavailable",
      message:
        "Secret-backed connection tests require a server-side secret resolver.",
      checked_at: "2026-06-05T04:00:00Z",
      observed_alerts: 0,
    });
    return;
  }
  if (profile.kind === "alertmanager") {
    writeJSON(response, 200, {
      source_id: profile.id,
      kind: profile.kind,
      auth_mode: profile.auth_mode,
      status: "success",
      reason_code: "ok",
      message: "Alertmanager alert listing succeeded.",
      checked_at: "2026-06-05T04:00:00Z",
      observed_alerts: 1,
    });
    return;
  }
  writeJSON(response, 200, {
    source_id: profile.id,
    kind: profile.kind,
    auth_mode: profile.auth_mode,
    status: "success",
    reason_code: "ok",
    message: "Prometheus alert listing succeeded.",
    checked_at: "2026-06-05T04:00:00Z",
    observed_alerts: 2,
  });
}

function alertSourceProfileFromBody(
  id,
  body,
  createdAt = "2026-05-28T06:02:00Z",
) {
  const now = "2026-05-28T06:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    kind: body.kind === "alertmanager" ? "alertmanager" : "prometheus",
    base_url: String(body.base_url ?? ""),
    auth_mode: body.auth_mode === "bearer" ? "bearer" : "none",
    secret_ref: typeof body.secret_ref === "string" ? body.secret_ref : "",
    enabled: Boolean(body.enabled),
    labels:
      body.labels &&
      typeof body.labels === "object" &&
      !Array.isArray(body.labels)
        ? body.labels
        : {},
    created_at: createdAt,
    updated_at: now,
  };
}

function handleGroupingPolicyCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: groupingPolicies });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const policy = groupingPolicyFromBody(nextGroupingPolicyID, body);
      nextGroupingPolicyID += 1;
      groupingPolicies.unshift(policy);
      writeJSON(response, 201, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleGroupingPolicy(request, response, policyID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = groupingPolicies.findIndex(
        (policy) => policy.id === policyID,
      );
      if (index < 0) {
        writeJSON(response, 404, { error: "grouping policy not found" });
        return;
      }
      const current = groupingPolicies[index];
      const policy = groupingPolicyFromBody(policyID, body, current.created_at);
      groupingPolicies[index] = policy;
      writeJSON(response, 200, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleGroupingPolicyPreview(request, response, policyID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const policy = groupingPolicies.find((item) => item.id === policyID);
  if (!policy) {
    writeJSON(response, 404, { error: "grouping policy not found" });
    return;
  }
  writeJSON(response, 200, {
    policy_id: policy.id,
    events_scanned: 3,
    events_matched:
      policy.source_filter.includes("prometheus") ||
      policy.source_filter.length === 0
        ? 2
        : 0,
    groups:
      policy.source_filter.includes("prometheus") ||
      policy.source_filter.length === 0
        ? [
            {
              group_key:
                "0000000000000000000000000000000000000000000000000000000000000001",
              dimensions: {
                alertname: "HighCPU",
                service: "checkout",
              },
              severity: "critical",
              event_count: 2,
              first_seen_at: "2026-06-05T04:00:00Z",
              last_seen_at: "2026-06-05T04:01:00Z",
              event_ids: [101, 102],
            },
          ]
        : [],
  });
}

function groupingPolicyFromBody(id, body, createdAt = "2026-06-05T04:02:00Z") {
  const now = "2026-06-05T04:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    dimension_keys: Array.isArray(body.dimension_keys)
      ? body.dimension_keys.map(String)
      : [],
    severity_key: String(body.severity_key ?? ""),
    source_filter: Array.isArray(body.source_filter)
      ? body.source_filter.map(String)
      : [],
    enabled: Boolean(body.enabled),
    created_at: createdAt,
    updated_at: now,
  };
}

function handleReportWorkflowPolicyCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: reportWorkflowPolicies });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const policy = reportWorkflowPolicyFromBody(
        nextReportWorkflowPolicyID,
        body,
      );
      nextReportWorkflowPolicyID += 1;
      reportWorkflowPolicies.unshift(policy);
      writeJSON(response, 201, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleReportWorkflowPolicy(request, response, policyID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = reportWorkflowPolicies.findIndex(
        (policy) => policy.id === policyID,
      );
      if (index < 0) {
        writeJSON(response, 404, { error: "report workflow policy not found" });
        return;
      }
      const current = reportWorkflowPolicies[index];
      const policy = {
        ...reportWorkflowPolicyFromBody(policyID, body, current.created_at),
        enabled: current.enabled,
        enabled_at: current.enabled_at,
        disabled_at: current.disabled_at,
      };
      reportWorkflowPolicies[index] = policy;
      writeJSON(response, 200, policy);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleReportWorkflowPolicyEnablement(
  request,
  response,
  policyID,
  enabled,
) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const index = reportWorkflowPolicies.findIndex(
    (policy) => policy.id === policyID,
  );
  if (index < 0) {
    writeJSON(response, 404, { error: "report workflow policy not found" });
    return;
  }
  const policy = reportWorkflowPolicies[index];
  if (enabled) {
    const source = alertSources.find(
      (item) => item.id === policy.alert_source_profile_id,
    );
    const grouping = groupingPolicies.find(
      (item) => item.id === policy.grouping_policy_id,
    );
    if (!source?.enabled || !grouping?.enabled) {
      writeJSON(response, 400, {
        error:
          "report workflow policy: alert source profile must be enabled before workflow policy enablement",
      });
      return;
    }
  }
  const now = "2026-06-05T08:05:00Z";
  const updated = {
    ...policy,
    enabled,
    enabled_at: enabled ? now : null,
    disabled_at: enabled ? null : now,
    updated_at: now,
  };
  reportWorkflowPolicies[index] = updated;
  writeJSON(response, 200, updated);
}

function handleReportWorkflowPolicyImpactPreview(request, response, policyID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const policy = reportWorkflowPolicies.find((item) => item.id === policyID);
  if (!policy) {
    writeJSON(response, 404, { error: "report workflow policy not found" });
    return;
  }
  const source = alertSources.find(
    (item) => item.id === policy.alert_source_profile_id,
  );
  const grouping = groupingPolicies.find(
    (item) => item.id === policy.grouping_policy_id,
  );
  const channel =
    policy.report_notification_channel_profile_id === null
      ? null
      : (notificationChannels.find(
          (item) => item.id === policy.report_notification_channel_profile_id,
        ) ?? null);
  if (!source || !grouping) {
    writeJSON(response, 404, {
      error: "report workflow policy binding not found",
    });
    return;
  }
  const reasonCodes = [];
  if (!source.enabled) {
    reasonCodes.push("alert_source_disabled");
  }
  if (!grouping.enabled) {
    reasonCodes.push("grouping_policy_disabled");
  }
  if (channel && !channel.enabled) {
    reasonCodes.push("notification_channel_disabled");
  }
  if (channel && !channel.delivery_scopes.includes("report")) {
    reasonCodes.push("notification_channel_missing_report_scope");
  }
  if (
    channel &&
    policy.diagnosis_follow_up === "auto_room" &&
    !channel.delivery_scopes.includes("diagnosis_consultation")
  ) {
    reasonCodes.push(
      "notification_channel_missing_diagnosis_consultation_scope",
    );
  }
  if (
    channel &&
    policy.diagnosis_follow_up === "auto_room" &&
    !channel.delivery_scopes.includes("diagnosis_close")
  ) {
    reasonCodes.push("notification_channel_missing_diagnosis_close_scope");
  }
  const blocked = reasonCodes.length > 0;
  const status = blocked ? "blocked" : "ready";
  writeJSON(response, 200, {
    policy_id: policy.id,
    status,
    reason_codes: blocked ? reasonCodes : ["ok"],
    message: blocked
      ? "Report workflow policy impact preview is blocked by configuration readiness."
      : "Report workflow policy impact preview is ready.",
    checked_at: "2026-06-05T10:00:00Z",
    trigger_mode: policy.trigger_mode,
    report_scenario: policy.report_scenario,
    diagnosis_follow_up: policy.diagnosis_follow_up,
    alert_source_profile_id: source.id,
    alert_source_kind: source.kind,
    alert_source_auth_mode: source.auth_mode,
    alert_source_enabled: source.enabled,
    grouping_policy_id: grouping.id,
    grouping_policy_enabled: grouping.enabled,
    grouping_dimension_keys: grouping.dimension_keys,
    grouping_severity_key: grouping.severity_key,
    grouping_source_filter: grouping.source_filter,
    report_notification_channel_profile_id:
      policy.report_notification_channel_profile_id,
    report_notification_channel_bound: channel !== null,
    report_notification_channel_enabled: channel?.enabled ?? false,
    report_notification_channel_has_report_scope:
      channel?.delivery_scopes.includes("report") ?? false,
    report_notification_channel_has_diagnosis_consultation_scope:
      channel?.delivery_scopes.includes("diagnosis_consultation") ?? false,
    report_notification_channel_has_diagnosis_close_scope:
      channel?.delivery_scopes.includes("diagnosis_close") ?? false,
    events_scanned: 2,
    events_matched: blocked ? 0 : 1,
    groups_estimated: blocked ? 0 : 1,
    groups: blocked
      ? []
      : [
          {
            group_key:
              "0000000000000000000000000000000000000000000000000000000000000001",
            dimensions: {
              alertname: "HighCPU",
              service: "checkout",
            },
            severity: "critical",
            event_count: 1,
            first_seen_at: "2026-06-05T04:00:00Z",
            last_seen_at: "2026-06-05T04:01:00Z",
            event_ids: [101],
          },
        ],
  });
}

function handleReportWorkflowPolicyReplay(request, response, policyID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const policy = reportWorkflowPolicies.find(
        (item) => item.id === policyID,
      );
      if (!policy) {
        writeJSON(response, 404, { error: "report workflow policy not found" });
        return;
      }
      if (!policy.enabled) {
        writeJSON(response, 400, {
          error:
            "report policy trigger: report workflow policy must be enabled before replay",
        });
        return;
      }
      if (!body.window_start || !body.window_end) {
        writeJSON(response, 400, {
          error: "window_start and window_end are required",
        });
        return;
      }
      writeJSON(response, 202, {
        started: true,
        correlation_key: String(
          body.correlation_key ??
            `report-workflow-policy:${policyID}:manual-replay`,
        ),
        workflow_id: "report-batch-policy-smoke",
        run_id: "run-policy-smoke",
        stats: {
          ingested: {
            total: 1,
            saved: 1,
            duplicate: 0,
            failed: 0,
          },
          events_loaded: 1,
          groups_built: 1,
          groups_saved: 1,
          groups_refreshed: 0,
          groups_existing: 0,
          snapshots_saved: policy.diagnosis_follow_up === "auto_room" ? 2 : 1,
          snapshots_duplicate: 0,
          groups_closed: 1,
          failed: 0,
        },
        snapshots: [
          {
            id: 7,
            group_index: 0,
            event_count: 1,
          },
          ...(policy.diagnosis_follow_up === "auto_room"
            ? [
                {
                  id: 8,
                  group_index: 1,
                  event_count: 1,
                },
              ]
            : []),
        ],
        ...(policy.diagnosis_follow_up === "auto_room"
          ? {
              auto_diagnosis: {
                policies_matched: 1,
                snapshots: 2,
                rooms_started: 1,
                rooms_skipped: 1,
                skipped_snapshot_ids: [8],
                rooms: [
                  {
                    policy_id: policy.id,
                    evidence_snapshot_id: 7,
                    session_id: `diagnosis-session-auto-p${policy.id}-s7`,
                    initial_message_id: `diagnosis-auto-initial-p${policy.id}-s7`,
                    workflow_id: `diagnosis-room-diagnosis-session-auto-p${policy.id}-s7`,
                    run_id: "run-diagnosis-7",
                  },
                ],
              },
            }
          : {}),
      });
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleReportReplay(request, response) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      if (!body.window_start || !body.window_end) {
        writeJSON(response, 400, {
          error: "window_start and window_end are required",
        });
        return;
      }
      attachReplaySnapshotToDiskAlert();
      writeJSON(response, 202, {
        started: true,
        correlation_key: String(body.correlation_key ?? "alert-replay-7002"),
        workflow_id: "report-batch-alert-replay",
        run_id: "run-alert-replay",
        stats: {
          ingested: {
            total: 1,
            saved: 1,
            duplicate: 0,
            failed: 0,
          },
          events_loaded: 1,
          groups_built: 1,
          groups_saved: 1,
          groups_refreshed: 0,
          groups_existing: 0,
          snapshots_saved: 1,
          snapshots_duplicate: 0,
          groups_closed: 1,
          failed: 0,
        },
        snapshots: [
          {
            id: 9002,
            group_index: 0,
            event_count: 1,
          },
        ],
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 1,
          rooms_started: 1,
          rooms_skipped: 0,
          skipped_snapshot_ids: [],
          rooms: [
            {
              policy_id: 1,
              evidence_snapshot_id: 9002,
              session_id: "diagnosis-session-auto-p1-s9002",
              initial_message_id: "msg-auto-9002/user",
              workflow_id: "diagnosis-room-diagnosis-session-auto-p1-s9002",
              run_id: "run-auto-9002",
            },
          ],
        },
      });
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function attachReplaySnapshotToDiskAlert() {
  const alert = alertEvents.find((item) => item.id === 7002);
  if (!alert) {
    return;
  }
  if (
    !alert.linked_evidence_snapshots.some((snapshot) => snapshot.id === 9002)
  ) {
    alert.linked_evidence_snapshots = [
      {
        id: 9002,
        alert_group_id: 3002,
        digest: "sha256:evidence-node-disk-pressure",
        status: "complete",
        created_by_workflow: "HTTPReportReplay",
        created_at: "2026-05-28T05:11:00Z",
        diagnosis_rooms: [replayedDiskDiagnosisRoom()],
      },
    ];
  }
  if (!evidenceSnapshots.some((snapshot) => snapshot.id === 9002)) {
    evidenceSnapshots.push({
      id: 9002,
      alert_group_id: 3002,
      digest: "sha256:evidence-node-disk-pressure",
      payload: {
        schema_version: "m1.evidence_snapshot.v1",
        group: {
          id: 3002,
          group_key: "alertname=NodeDiskPressure,instance=node-a",
          dimensions: {
            alertname: "NodeDiskPressure",
            instance: "node-a",
          },
          severity: "critical",
          event_count: 1,
          first_seen_at: "2026-05-28T04:40:00Z",
          last_seen_at: "2026-05-28T04:40:00Z",
        },
        events: [
          {
            id: 7002,
            source: "alertmanager",
            source_fingerprint: "am:node-disk:prod",
            canonical_fingerprint: "sha256:node-disk-prod",
            labels: {
              alertname: "NodeDiskPressure",
              instance: "node-a",
              severity: "critical",
            },
            annotations: {
              summary: "Node disk pressure crossed the critical threshold.",
            },
            status: "resolved",
            starts_at: "2026-05-28T04:40:00Z",
            ends_at: "2026-05-28T05:10:00Z",
            raw_payload: null,
          },
        ],
      },
      provenance: {
        "openclarion.core": "ok",
      },
      status: "complete",
      missing_fields: [],
      created_by_workflow: "HTTPReportReplay",
      created_at: "2026-05-28T05:11:00Z",
    });
  }
  if (
    !diagnosisRooms.some(
      (room) => room.session_id === "diagnosis-session-auto-p1-s9002",
    )
  ) {
    diagnosisRooms.push(replayedDiskDiagnosisRoom());
  }
}

function replayedDiskDiagnosisRoom() {
  return {
    session_id: "diagnosis-session-auto-p1-s9002",
    chat_session_id: 409,
    diagnosis_task_id: 309,
    evidence_snapshot_id: 9002,
    workflow_id: "diagnosis-room-diagnosis-session-auto-p1-s9002",
    run_id: "run-auto-9002",
    task_status: "running",
    room_status: "open",
    turn_count: 0,
    started_at: "2026-05-28T05:11:05Z",
    last_activity_at: "2026-05-28T05:11:05Z",
    closed_at: null,
    close_reason: "",
    notification_timeline: [],
    created_at: "2026-05-28T05:11:05Z",
    updated_at: "2026-05-28T05:11:05Z",
  };
}

function reportWorkflowPolicyFromBody(
  id,
  body,
  createdAt = "2026-06-05T08:02:00Z",
) {
  const now = "2026-06-05T08:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    alert_source_profile_id: Number.parseInt(
      String(body.alert_source_profile_id ?? "0"),
      10,
    ),
    grouping_policy_id: Number.parseInt(
      String(body.grouping_policy_id ?? "0"),
      10,
    ),
    report_notification_channel_profile_id:
      body.report_notification_channel_profile_id === null ||
      body.report_notification_channel_profile_id === undefined
        ? null
        : Number.parseInt(
            String(body.report_notification_channel_profile_id),
            10,
          ),
    trigger_mode: "manual_replay",
    report_scenario: ["single_alert", "cascade", "alert_storm"].includes(
      body.report_scenario,
    )
      ? body.report_scenario
      : "single_alert",
    diagnosis_follow_up: diagnosisFollowUpModes.has(body.diagnosis_follow_up)
      ? body.diagnosis_follow_up
      : "disabled",
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: createdAt,
    updated_at: now,
  };
}

function handleReportWorkflowScheduleCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: reportWorkflowSchedules });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const schedule = reportWorkflowScheduleFromBody(
        nextReportWorkflowScheduleID,
        body,
      );
      nextReportWorkflowScheduleID += 1;
      reportWorkflowSchedules.unshift(schedule);
      writeJSON(response, 201, schedule);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleReportWorkflowSchedule(request, response, scheduleID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = reportWorkflowSchedules.findIndex(
        (schedule) => schedule.id === scheduleID,
      );
      if (index < 0) {
        writeJSON(response, 404, {
          error: "report workflow schedule not found",
        });
        return;
      }
      const current = reportWorkflowSchedules[index];
      const schedule = {
        ...reportWorkflowScheduleFromBody(scheduleID, body, current.created_at),
        enabled: current.enabled,
        enabled_at: current.enabled_at,
        disabled_at: current.disabled_at,
      };
      reportWorkflowSchedules[index] = schedule;
      writeJSON(response, 200, schedule);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleReportWorkflowScheduleEnablement(
  request,
  response,
  scheduleID,
  enabled,
) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const index = reportWorkflowSchedules.findIndex(
    (schedule) => schedule.id === scheduleID,
  );
  if (index < 0) {
    writeJSON(response, 404, { error: "report workflow schedule not found" });
    return;
  }
  const schedule = reportWorkflowSchedules[index];
  if (enabled) {
    const policy = reportWorkflowPolicies.find(
      (item) => item.id === schedule.report_workflow_policy_id,
    );
    if (!policy?.enabled) {
      writeJSON(response, 400, {
        error:
          "report workflow schedule: report workflow policy must be enabled before schedule enablement",
      });
      return;
    }
  }
  const now = "2026-06-06T02:05:00Z";
  const updated = {
    ...schedule,
    enabled,
    enabled_at: enabled ? now : null,
    disabled_at: enabled ? null : now,
    updated_at: now,
  };
  reportWorkflowSchedules[index] = updated;
  writeJSON(response, 200, updated);
}

function reportWorkflowScheduleFromBody(
  id,
  body,
  createdAt = "2026-06-06T02:02:00Z",
) {
  const now = "2026-06-06T02:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    report_workflow_policy_id: Number.parseInt(
      String(body.report_workflow_policy_id ?? "0"),
      10,
    ),
    temporal_schedule_id: String(body.temporal_schedule_id ?? ""),
    interval_seconds: Number.parseInt(String(body.interval_seconds ?? "0"), 10),
    offset_seconds: Number.parseInt(String(body.offset_seconds ?? "0"), 10),
    replay_window_seconds: Number.parseInt(
      String(body.replay_window_seconds ?? "0"),
      10,
    ),
    replay_delay_seconds: Number.parseInt(
      String(body.replay_delay_seconds ?? "0"),
      10,
    ),
    replay_limit: Number.parseInt(String(body.replay_limit ?? "0"), 10),
    catchup_window_seconds: Number.parseInt(
      String(body.catchup_window_seconds ?? "0"),
      10,
    ),
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: createdAt,
    updated_at: now,
  };
}

function handleDiagnosisToolTemplateCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: diagnosisToolTemplates });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const template = diagnosisToolTemplateFromBody(
        nextDiagnosisToolTemplateID,
        body,
      );
      nextDiagnosisToolTemplateID += 1;
      diagnosisToolTemplates.unshift(template);
      writeJSON(response, 201, template);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleDiagnosisToolTemplate(request, response, templateID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = diagnosisToolTemplates.findIndex(
        (template) => template.id === templateID,
      );
      if (index < 0) {
        writeJSON(response, 404, {
          error: "diagnosis tool template not found",
        });
        return;
      }
      const current = diagnosisToolTemplates[index];
      const template = {
        ...diagnosisToolTemplateFromBody(templateID, body, current.created_at),
        enabled: current.enabled,
        enabled_at: current.enabled_at,
        disabled_at: current.disabled_at,
      };
      diagnosisToolTemplates[index] = template;
      writeJSON(response, 200, template);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleDiagnosisToolTemplateEnablement(
  request,
  response,
  templateID,
  enabled,
) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const index = diagnosisToolTemplates.findIndex(
    (template) => template.id === templateID,
  );
  if (index < 0) {
    writeJSON(response, 404, { error: "diagnosis tool template not found" });
    return;
  }
  const template = diagnosisToolTemplates[index];
  if (enabled) {
    const source = alertSources.find(
      (item) => item.id === template.alert_source_profile_id,
    );
    if (
      !source?.enabled ||
      !diagnosisToolTemplateSupportsSource(template, source)
    ) {
      writeJSON(response, 400, {
        error:
          "diagnosis tool template: bound alert source must be enabled and compatible before template enablement",
      });
      return;
    }
  }
  const now = "2026-06-08T08:05:00Z";
  const updated = {
    ...template,
    enabled,
    enabled_at: enabled ? now : null,
    disabled_at: enabled ? null : now,
    updated_at: now,
  };
  diagnosisToolTemplates[index] = updated;
  writeJSON(response, 200, updated);
}

function diagnosisToolTemplateFromBody(
  id,
  body,
  createdAt = "2026-06-08T08:02:00Z",
) {
  const now = "2026-06-08T08:03:00Z";
  return {
    id,
    name: String(body.name ?? ""),
    alert_source_profile_id: Number.parseInt(
      String(body.alert_source_profile_id ?? "0"),
      10,
    ),
    tool: ["active_alerts", "metric_query", "metric_range_query"].includes(
      body.tool,
    )
      ? body.tool
      : "active_alerts",
    query_template: String(body.query_template ?? ""),
    default_limit: Number.parseInt(String(body.default_limit ?? "0"), 10),
    default_window_seconds: Number.parseInt(
      String(body.default_window_seconds ?? "0"),
      10,
    ),
    max_window_seconds: Number.parseInt(
      String(body.max_window_seconds ?? "0"),
      10,
    ),
    default_step_seconds: Number.parseInt(
      String(body.default_step_seconds ?? "0"),
      10,
    ),
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: createdAt,
    updated_at: now,
  };
}

function diagnosisToolTemplateSupportsSource(template, source) {
  if (template.tool === "active_alerts") {
    return source.kind === "alertmanager" || source.kind === "prometheus";
  }
  return source.kind === "prometheus";
}

function handleNotificationChannelCollection(request, response) {
  if (request.method === "GET") {
    writeJSON(response, 200, { items: notificationChannels });
    return;
  }
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const channel = notificationChannelFromBody(
        nextNotificationChannelID,
        body,
      );
      nextNotificationChannelID += 1;
      notificationChannels.unshift(channel);
      writeJSON(response, 201, channel);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleNotificationChannel(request, response, channelID) {
  if (request.method !== "PUT") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  readJSON(request)
    .then((body) => {
      const index = notificationChannels.findIndex(
        (channel) => channel.id === channelID,
      );
      if (index < 0) {
        writeJSON(response, 404, {
          error: "notification channel profile not found",
        });
        return;
      }
      const current = notificationChannels[index];
      const channel = notificationChannelFromBody(
        channelID,
        body,
        current.created_at,
      );
      notificationChannels[index] = channel;
      writeJSON(response, 200, channel);
    })
    .catch((error) => {
      writeJSON(response, 400, {
        error: error instanceof Error ? error.message : "invalid JSON",
      });
    });
}

function handleNotificationChannelTest(request, response, channelID) {
  if (request.method !== "POST") {
    writeJSON(response, 405, { error: "method not allowed" });
    return;
  }
  const channel = notificationChannels.find((item) => item.id === channelID);
  if (!channel) {
    writeJSON(response, 404, {
      error: "notification channel profile not found",
    });
    return;
  }
  writeJSON(response, 200, {
    channel_id: channel.id,
    kind: channel.kind,
    status: "blocked",
    reason_code: "credentials_unavailable",
    message:
      "Secret-backed notification channel tests require a server-side secret resolver.",
    checked_at: "2026-06-05T10:00:00Z",
    provider_message_id: "",
    provider_status: "",
  });
}

function notificationChannelFromBody(
  id,
  body,
  createdAt = "2026-06-05T09:02:00Z",
) {
  const now = "2026-06-05T09:03:00Z";
  const scopes = Array.isArray(body.delivery_scopes)
    ? body.delivery_scopes.map(String)
    : [];
  const kind = notificationChannelKindFromBody(body.kind);
  if (
    kind !== "wecom" &&
    scopes.some(
      (scope) =>
        scope === "diagnosis_consultation" || scope === "diagnosis_close",
    )
  ) {
    throw new Error(
      "notification channel profile: diagnosis delivery scopes require an Enterprise WeChat channel",
    );
  }
  return {
    id,
    name: String(body.name ?? ""),
    kind,
    secret_ref: String(body.secret_ref ?? ""),
    delivery_scopes: scopes.filter(
      (scope) =>
        scope === "report" ||
        scope === "diagnosis_consultation" ||
        scope === "diagnosis_close",
    ),
    enabled: Boolean(body.enabled),
    latest_test_results: notificationChannelTestResultsFromBody(
      id,
      kind,
      body.latest_test_results,
    ),
    labels:
      body.labels &&
      typeof body.labels === "object" &&
      !Array.isArray(body.labels)
        ? body.labels
        : {},
    created_at: createdAt,
    updated_at: now,
  };
}

function notificationChannelKindFromBody(value) {
  return ["webhook", "wecom", "dingtalk", "feishu", "slack", "email"].includes(
    value,
  )
    ? value
    : "webhook";
}

function notificationChannelTestResultsFromBody(channelID, kind, value) {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((result) => notificationChannelTestResultFromBody(channelID, kind, result))
    .filter((result) => result !== null);
}

function notificationChannelTestResultFromBody(channelID, kind, value) {
  if (!value || typeof value !== "object") {
    return null;
  }
  const contentKind = notificationChannelTestContentKind(value.content_kind);
  if (!contentKind) {
    return null;
  }
  const contentSHA256 =
    typeof value.content_sha256 === "string" &&
    /^[a-f0-9]{64}$/.test(value.content_sha256)
      ? value.content_sha256
      : "5c6ffbdd40d9556b73a21e63c3e0e9047c7f534c2ab09dc7ed89b889f0d011e7";
  return {
    channel_id: channelID,
    kind,
    status: value.status === "success" ? "success" : "blocked",
    reason_code: value.reason_code === "ok" ? "ok" : "credentials_unavailable",
    message: String(value.message ?? "Notification channel test completed."),
    content_kind: contentKind,
    content_sha256: contentSHA256,
    checked_at: String(value.checked_at ?? "2026-06-05T10:00:00Z"),
    provider_message_id: String(value.provider_message_id ?? ""),
    provider_status: String(value.provider_status ?? ""),
  };
}

function notificationChannelTestContentKind(value) {
  if (
    value === "transport_sample" ||
    value === "ai_diagnosis_sample" ||
    value === "diagnosis_close_sample"
  ) {
    return value;
  }
  return "";
}

function handleDiagnosisFrame(socket, sessionID, state, payload) {
  let frame;
  try {
    frame = JSON.parse(payload);
  } catch {
    sendWebSocketJSON(socket, {
      type: "error",
      code: "bad_frame",
      message: "invalid JSON frame",
    });
    return;
  }
  if (frame.type === "query_state") {
    sendWebSocketJSON(socket, diagnosisState(sessionID, state));
    return;
  }
  if (frame.type === "collect_evidence") {
    const text = typeof frame.message === "string" ? frame.message.trim() : "";
    const messageID =
      typeof frame.message_id === "string" ? frame.message_id : "";
    const evidenceRequests = Array.isArray(frame.evidence_requests)
      ? frame.evidence_requests
      : [];
    if (text === "" || messageID === "" || evidenceRequests.length === 0) {
      sendWebSocketJSON(socket, {
        type: "error",
        code: "invalid_request",
        message: "evidence_requests are required",
      });
      return;
    }
    const collectionResults = evidenceRequests.map((request) => ({
      request,
      alert_source_profile_id: request.alert_source_profile_id,
      alert_source_kind:
        request.tool === "active_alerts" ? "alertmanager" : "prometheus",
      tool: request.tool,
      status: "collected",
      reason_code: "ok",
      message:
        request.tool === "active_alerts"
          ? "Active alert collection succeeded."
          : "Metric range collection succeeded.",
      limit: request.limit,
      observed_alerts: request.tool === "active_alerts" ? 1 : 0,
      query: request.query,
      window_seconds: request.window_seconds,
      step_seconds: request.step_seconds,
      observed_metric_series: request.tool === "active_alerts" ? 0 : 2,
      collected_at: "2026-06-17T10:00:02Z",
    }));
    state.evidenceTimeline = [
      ...(state.evidenceTimeline ?? []),
      {
        turn_count: state.turnCount,
        message_id: messageID,
        trigger: "manual_evidence_collection",
        evidence_requests: evidenceRequests,
        evidence_collection_results: collectionResults,
      },
    ];
    state.turnCount += 1;
    state.conversation.push({
      role: "user",
      content: "OpenClarion automatic evidence follow-up.",
    });
    state.conversation.push({
      role: "assistant",
      content: "Mock diagnosis response after planned evidence collection.",
    });
    state.status = "open";
    state.latestConfidence = "medium";
    state.latestRequiresHumanReview = true;
    state.latestConsultationInsight = mockDiagnosisAutoFollowUpInsight();
    state.latestEvidenceRequests = [];
    state.latestCollectionResults = collectionResults;
    state.confidenceTimeline = [
      ...(state.confidenceTimeline ?? []),
      mockDiagnosisConfidenceCheckpoint({
        collectionResults,
        confidence: state.latestConfidence,
        insight: state.latestConsultationInsight,
        messageID,
        requiresHumanReview: state.latestRequiresHumanReview,
        turnCount: state.turnCount,
        trigger: "manual_evidence_collection",
      }),
    ];
    setTimeout(() => {
      sendWebSocketJSON(socket, diagnosisState(sessionID, state));
    }, diagnosisMockTurnDelayMs);
    return;
  }
  if (
    frame.type === "submit_turn" ||
    frame.type === "submit_supplemental_evidence"
  ) {
    const text = typeof frame.message === "string" ? frame.message.trim() : "";
    const messageID =
      typeof frame.message_id === "string" ? frame.message_id : "";
    if (text === "" || messageID === "") {
      sendWebSocketJSON(socket, {
        type: "error",
        code: "invalid_request",
        message: "message is required",
      });
      return;
    }
    if (
      frame.type === "submit_supplemental_evidence" &&
      !validSupplementalEvidence(frame.supplemental_evidence)
    ) {
      sendWebSocketJSON(socket, {
        type: "error",
        code: "invalid_request",
        message: "valid supplemental_evidence is required",
      });
      return;
    }
    if (text === "Trigger backend error.") {
      sendWebSocketJSON(socket, {
        type: "error",
        code: "mock_backend_error",
        message: "mock backend rejected the diagnosis request",
      });
      return;
    }
    if (text === "Trigger confirm rejection.") {
      sendWebSocketJSON(socket, {
        type: "error",
        code: "confirm_rejected",
        message: "resolve missing evidence requests before confirming",
      });
      return;
    }
    if (text === "Trigger retained state error.") {
      state.latestError = {
        code: "llm_timeout",
        message:
          "Diagnosis turn failed before an assistant response; upstream LLM request timed out.",
        message_id: messageID,
        occurred_at: "2026-05-28T10:00:05Z",
      };
      sendWebSocketJSON(socket, diagnosisState(sessionID, state));
      return;
    }
    state.turnCount += 1;
    const primaryTurnCount = state.turnCount;
    state.conversation.push({ role: "user", content: text });
    const supplemental = frame.type === "submit_supplemental_evidence";
    const supplementalStillNeedsEvidence =
      supplemental &&
      /\bstill insufficient\b/i.test(frame.supplemental_evidence.evidence);
    const autoFollowUp =
      !supplemental && /\bauto evidence fallback\b/i.test(text);
    const assistant = supplemental
      ? `Mock supplemental evidence response for: ${frame.supplemental_evidence.label}`
      : `Mock diagnosis response for: ${text}`;
    const consultationInsight = supplemental
      ? supplementalStillNeedsEvidence
        ? mockDiagnosisStillNeedsEvidenceInsight()
        : mockDiagnosisReadyInsight()
      : mockDiagnosisConsultationInsight();
    state.conversation.push({ role: "assistant", content: assistant });
    state.status = "open";
    state.latestError = null;
    state.latestConfidence =
      supplemental && !supplementalStillNeedsEvidence ? "high" : "medium";
    state.latestRequiresHumanReview =
      !supplemental || supplementalStillNeedsEvidence;
    state.latestConsultationInsight = consultationInsight;
    const evidenceRequests = supplemental
      ? []
      : [
          {
            alert_source_profile_id: 3,
            template_id: 5,
            tool: "active_alerts",
            reason: "Current active alerts",
            limit: 7,
          },
          {
            alert_source_profile_id: 4,
            tool: "metric_range_query",
            reason: "CPU and memory saturation window",
            query: "avg(rate(container_cpu_usage_seconds_total[5m]))",
            window_seconds: 300,
            step_seconds: 60,
            limit: 10,
          },
        ];
    const evidenceCollectionResults = supplemental
      ? []
      : [
          {
            request: {
              alert_source_profile_id: 3,
              template_id: 5,
              tool: "active_alerts",
              reason: "Current active alerts",
              limit: 7,
            },
            alert_source_profile_id: 3,
            alert_source_kind: "alertmanager",
            template_id: 5,
            tool: "active_alerts",
            status: "collected",
            reason_code: "ok",
            message: "Active alert collection succeeded.",
            limit: 7,
            observed_alerts: 1,
            active_alerts: [
              {
                source: "alertmanager",
                alert_source_profile_id: 3,
                labels: {
                  alertname: "CPUHigh",
                  namespace: "prod",
                },
                annotations: {
                  summary: "CPU is high",
                },
                starts_at: "2026-06-17T10:00:00Z",
              },
            ],
            collected_at: "2026-06-17T10:00:01Z",
          },
        ];
    state.latestEvidenceRequests = evidenceRequests;
    state.latestCollectionResults = evidenceCollectionResults;
    const confidenceTimelineEntry = mockDiagnosisConfidenceCheckpoint({
      collectionResults: evidenceCollectionResults,
      confidence:
        supplemental && !supplementalStillNeedsEvidence ? "high" : "medium",
      evidenceRequests,
      insight: consultationInsight,
      messageID,
      requiresHumanReview: !supplemental || supplementalStillNeedsEvidence,
      turnCount: primaryTurnCount,
      trigger: "operator_turn",
    });
    const newEvidenceTimelineEntries =
      evidenceRequests.length > 0 || evidenceCollectionResults.length > 0
        ? [
            {
              turn_count: primaryTurnCount,
              message_id: messageID,
              assistant_message_id: `${messageID}/assistant`,
              trigger: "operator_turn",
              evidence_requests: evidenceRequests,
              evidence_collection_results: evidenceCollectionResults,
            },
          ]
        : [];
    state.evidenceTimeline = [
      ...(state.evidenceTimeline ?? []),
      ...newEvidenceTimelineEntries,
    ];
    state.confidenceTimeline = [
      ...(state.confidenceTimeline ?? []),
      confidenceTimelineEntry,
    ];
    const followUpTurns = autoFollowUp
      ? [
          {
            message_id: `${messageID}/auto-evidence-1`,
            user_message: "OpenClarion automatic evidence follow-up.",
            assistant_message_id: `${messageID}/auto-evidence-1/assistant`,
            user_turn_id: primaryTurnCount * 2 + 1,
            assistant_turn_id: primaryTurnCount * 2 + 2,
            user_sequence: primaryTurnCount * 2 + 1,
            assistant_sequence: primaryTurnCount * 2 + 2,
            turn_count: primaryTurnCount + 1,
            context_bytes: 768,
            assistant_message: "Mock auto evidence follow-up response.",
            requires_human_review: true,
            confidence: "medium",
            consultation_insight: mockDiagnosisAutoFollowUpInsight(),
            trigger: "collected_evidence",
          },
        ]
      : [];
    if (followUpTurns.length > 0) {
      const followUp = followUpTurns[0];
      state.turnCount = followUp.turn_count;
      state.conversation.push({ role: "user", content: followUp.user_message });
      state.conversation.push({
        role: "assistant",
        content: followUp.assistant_message,
      });
      state.latestConfidence = followUp.confidence;
      state.latestRequiresHumanReview = followUp.requires_human_review;
      state.latestConsultationInsight = followUp.consultation_insight;
      state.confidenceTimeline = [
        ...(state.confidenceTimeline ?? []),
        mockDiagnosisConfidenceCheckpoint({
          confidence: followUp.confidence,
          insight: followUp.consultation_insight,
          messageID: followUp.message_id,
          requiresHumanReview: followUp.requires_human_review,
          turnCount: followUp.turn_count,
          trigger: followUp.trigger,
        }),
      ];
    }
    if (supplemental) {
      state.supplementalEvidence = [
        ...(state.supplementalEvidence ?? []),
        {
          ...frame.supplemental_evidence,
          user_message_id: messageID,
          assistant_message_id: `${messageID}/assistant`,
          user_turn_id: state.turnCount * 2 - 1,
          assistant_turn_id: state.turnCount * 2,
          user_sequence: state.turnCount * 2 - 1,
          assistant_sequence: state.turnCount * 2,
          provided_at: "2026-05-28T10:01:00Z",
        },
      ];
    }
    setTimeout(() => {
      sendWebSocketJSON(socket, {
        type: "turn_result",
        session_id: sessionID,
        chat_session_id: 42,
        message_id: messageID,
        assistant_message_id: `${messageID}/assistant`,
        user_turn_id: primaryTurnCount * 2 - 1,
        assistant_turn_id: primaryTurnCount * 2,
        user_sequence: primaryTurnCount * 2 - 1,
        assistant_sequence: primaryTurnCount * 2,
        turn_count: primaryTurnCount,
        context_bytes: 512,
        status: "open",
        assistant_message: assistant,
        requires_human_review: !supplemental || supplementalStillNeedsEvidence,
        confidence:
          supplemental && !supplementalStillNeedsEvidence ? "high" : "medium",
        evidence_requests: evidenceRequests,
        evidence_collection_results: evidenceCollectionResults,
        evidence_timeline: state.evidenceTimeline,
        confidence_timeline: state.confidenceTimeline,
        consultation_insight: consultationInsight,
        follow_up_turns: followUpTurns,
      });
    }, diagnosisMockTurnDelayMs);
    return;
  }
  if (frame.type === "confirm_conclusion") {
    const blockReason = diagnosisConfirmBlockReason(state);
    if (blockReason !== "") {
      sendWebSocketJSON(socket, {
        type: "error",
        code: "confirm_rejected",
        message: blockReason,
      });
      return;
    }
    const latestAssistant = [...state.conversation]
      .reverse()
      .find((turn) => turn.role === "assistant");
    const closeReason =
      typeof frame.reason === "string" && frame.reason.trim() !== ""
        ? frame.reason.trim()
        : "human_confirmed";
    state.status = "closed";
    state.closedAt = "2026-05-28T10:02:00Z";
    state.closeReason = closeReason;
    state.finalConclusion = {
      status: "available",
      source: "latest_assistant_turn",
      evidence_snapshot_id: 9001,
      conclusion_version: "diagnosis-room-close.v1",
      recorded_at: state.closedAt,
      confirmed_by: "owner-1",
      supplemental_context_refs: [
        "chat_session:42/turn:1",
        "chat_session:42/turn:2",
      ],
      assistant_turn_id: state.turnCount * 2,
      assistant_message_id: `mock-${state.turnCount}/assistant`,
      assistant_sequence: state.turnCount * 2,
      assistant_occurred_at: "2026-05-28T10:01:00Z",
      content:
        latestAssistant?.content ?? "No assistant conclusion is available.",
      confidence: state.latestConfidence ?? "medium",
      requires_human_review: state.latestRequiresHumanReview ?? true,
    };
    sendWebSocketJSON(socket, diagnosisState(sessionID, state));
    return;
  }
  sendWebSocketJSON(socket, {
    type: "error",
    code: "bad_frame",
    message: "unsupported frame type",
  });
}

function validSupplementalEvidence(value) {
  return (
    value &&
    typeof value === "object" &&
    typeof value.label === "string" &&
    value.label.trim() !== "" &&
    typeof value.detail === "string" &&
    value.detail.trim() !== "" &&
    typeof value.priority === "string" &&
    value.priority.trim() !== "" &&
    typeof value.evidence === "string" &&
    value.evidence.trim() !== ""
  );
}

function diagnosisConfirmBlockReason(state) {
  const insight = state.latestConsultationInsight ?? {};
  if ((insight.missing_evidence_requests ?? []).length > 0) {
    return "resolve missing evidence requests before confirming";
  }
  const requests = state.latestEvidenceRequests ?? [];
  const results = state.latestCollectionResults ?? [];
  const resultKeys = new Set(
    results.map((result) => evidenceRequestIdentity(result.request ?? {})),
  );
  const pending = requests.filter(
    (request) => !resultKeys.has(evidenceRequestIdentity(request)),
  );
  if (pending.length > 0) {
    return "collect planned executable evidence before confirming";
  }
  return "";
}

function evidenceRequestIdentity(request) {
  return [
    request.template_id ?? "no-template",
    request.alert_source_profile_id ?? "no-profile",
    request.tool ?? "",
    request.reason ?? "",
    request.query ?? "no-query",
    request.window_seconds ?? "no-window",
    request.step_seconds ?? "no-step",
    request.limit ?? "no-limit",
  ].join(":");
}

function mockDiagnosisConsultationInsight() {
  return {
    confidence_rationale:
      "Alert labels and metric context are available, but restart evidence is still missing.",
    missing_evidence_requests: [
      {
        label: "Restart cause",
        detail:
          "Collect previous container logs and recent workload events before final review.",
        priority: "high",
      },
    ],
    evidence_collection_suggestions: [
      {
        label: "Metric window",
        detail:
          "Collect a five minute CPU and memory range around the alert firing time.",
        priority: "medium",
      },
    ],
    conclusion_status: "needs_evidence",
  };
}

function mockDiagnosisReadyInsight() {
  return {
    confidence_rationale: "Collected evidence supports final review.",
    evidence_collection_suggestions: [
      {
        label: "Owner confirmation",
        detail: "Confirm the retained conclusion with the service owner.",
        priority: "medium",
      },
    ],
    conclusion_status: "ready_for_review",
  };
}

function mockDiagnosisStillNeedsEvidenceInsight() {
  return {
    confidence_rationale:
      "Submitted restart evidence is retained, but the latest review still needs timestamped previous logs.",
    missing_evidence_requests: [
      {
        label: "Restart cause",
        detail:
          "Attach previous container logs with restart timestamps and the observed termination reason.",
        priority: "medium",
      },
    ],
    conclusion_status: "needs_evidence",
  };
}

function mockDiagnosisAutoFollowUpInsight() {
  return {
    confidence_rationale:
      "Collected active alert evidence supports the follow-up diagnosis, but restart evidence is still missing.",
    missing_evidence_requests: [
      {
        label: "Restart cause",
        detail:
          "Collect previous container logs and recent workload events before final review.",
        priority: "high",
      },
    ],
    conclusion_status: "needs_evidence",
  };
}

function mockDiagnosisConfidenceCheckpoint({
  collectionResults = [],
  confidence,
  evidenceRequests = [],
  insight,
  messageID,
  requiresHumanReview,
  turnCount,
  trigger,
}) {
  return {
    turn_count: turnCount,
    message_id: messageID,
    assistant_message_id: `${messageID}/assistant`,
    assistant_turn_id: turnCount * 2,
    assistant_sequence: turnCount * 2,
    occurred_at: "2026-05-28T10:00:05Z",
    trigger,
    confidence,
    requires_human_review: requiresHumanReview,
    conclusion_status: insight?.conclusion_status,
    confidence_rationale: insight?.confidence_rationale,
    evidence_requests: evidenceRequests,
    evidence_collection_results: collectionResults,
    missing_evidence_requests: insight?.missing_evidence_requests ?? [],
    evidence_collection_suggestions:
      insight?.evidence_collection_suggestions ?? [],
  };
}

function diagnosisState(sessionID, state) {
  const payload = {
    type: "state",
    session_id: sessionID,
    chat_session_id: 42,
    diagnosis_task_id: 7,
    owner_subject: "owner-1",
    status: state.status,
    turn_count: state.turnCount,
    started_at: "2026-05-28T10:00:00Z",
    last_activity_at: state.closedAt || "2026-05-28T10:00:05Z",
    in_flight: false,
    seen_message_ids: [],
    conversation: state.conversation,
  };
  if (state.closedAt) {
    payload.closed_at = state.closedAt;
    payload.close_reason = state.closeReason;
  }
  if (state.finalConclusion) {
    payload.final_conclusion = state.finalConclusion;
  }
  if (state.latestConsultationInsight) {
    payload.confidence = state.latestConfidence;
    payload.requires_human_review = state.latestRequiresHumanReview;
    payload.evidence_requests = state.latestEvidenceRequests ?? [];
    payload.evidence_collection_results = state.latestCollectionResults ?? [];
    payload.evidence_timeline = state.evidenceTimeline ?? [];
    payload.confidence_timeline = state.confidenceTimeline ?? [];
    payload.supplemental_evidence = state.supplementalEvidence ?? [];
    payload.consultation_insight = state.latestConsultationInsight;
  }
  if (state.latestError) {
    payload.latest_error = state.latestError;
  }
  return payload;
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
    payload: payload.toString("utf8"),
  };
}
