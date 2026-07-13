import { describe, expect, it } from "vitest";

import {
  alertSourceIngressReadinessForSelection,
  diagnosisToolReadinessForSelection,
  defaultReportWorkflowPolicyReplayForm,
  emptyReportWorkflowPolicyForm,
  formStateToReplayRequest,
  formStateToWriteRequest,
  policyToFormState,
  preferredReportNotificationChannelIDForFollowUp,
  reportWorkflowPolicyAutomationOutcome,
  reportWorkflowPolicyAutoRoomReadiness,
  reportWorkflowPolicyLaunchHref,
  reportWorkflowPolicyLaunchIntentFromSearchParams,
  reportWorkflowPolicyLaunchIntentKey,
  reportWorkflowPolicyRepairBlueprint,
  reportWorkflowPolicyDraftPlan,
  reportWorkflowPolicyFormMatchesPolicy,
  reportWorkflowPolicyImpactDiagnosisEstimate,
  reportWorkflowPolicyImpactReportChannelReadiness,
  reportWorkflowPolicyImpactReason,
  reportWorkflowPolicyEnablementReadiness,
  reportWorkflowPolicyWorkflowReturnCandidates,
  reportWorkflowNotificationChannelOptionState,
  reportWorkflowNotificationChannelOperatorReadiness,
  reportWorkflowPolicyReplayProofTrace,
  reportWorkflowPolicySetupBlueprint,
  reportNotificationChannelReadinessForSelection,
} from "./format";
import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyImpactPreviewResult,
} from "./types";
import type { DiagnosisToolTemplate } from "../diagnosis-tool-templates/types";
import type { NotificationChannelProfile } from "../notification-channels/types";

describe("report workflow policy form formatting", () => {
  it("builds write requests without enabled state", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowPolicyForm(),
      name: " Default report workflow ",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      reportScenario: "cascade",
      diagnosisFollowUp: "auto_room",
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        name: "Default report workflow",
        alert_source_profile_id: 1,
        grouping_policy_id: 2,
        report_notification_channel_profile_id: 3,
        trigger_mode: "manual_replay",
        report_scenario: "cascade",
        diagnosis_follow_up: "auto_room",
      },
    });
  });

  it("rejects missing bound profile IDs", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowPolicyForm(),
      name: "Default report workflow",
      alertSourceProfileID: null,
      groupingPolicyID: 2,
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Select an alert source.",
    });
  });

  it("rejects invalid optional report notification channel IDs", () => {
    const parsed = formStateToWriteRequest({
      ...emptyReportWorkflowPolicyForm(),
      name: "Default report workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 0,
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Select a valid report notification channel.",
    });
  });

  it("maps policy rows back to edit form state", () => {
    const policy: ReportWorkflowPolicy = {
      id: 7,
      name: "Default report workflow",
      alert_source_profile_id: 1,
      grouping_policy_id: 2,
      report_notification_channel_profile_id: 3,
      trigger_mode: "manual_replay",
      report_scenario: "single_alert",
      diagnosis_follow_up: "disabled",
      enabled: false,
      enabled_at: null,
      disabled_at: null,
      created_at: "2026-06-05T08:00:00Z",
      updated_at: "2026-06-05T08:00:00Z",
    };

    expect(policyToFormState(policy)).toEqual({
      name: "Default report workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      triggerMode: "manual_replay",
      reportScenario: "single_alert",
      diagnosisFollowUp: "disabled",
    });
  });

  it("detects whether form state matches the saved policy", () => {
    const policy: ReportWorkflowPolicy = {
      ...reportWorkflowPolicy(),
      report_notification_channel_profile_id: 3,
      diagnosis_follow_up: "auto_room",
    };

    expect(
      reportWorkflowPolicyFormMatchesPolicy(policyToFormState(policy), policy),
    ).toBe(true);
    expect(
      reportWorkflowPolicyFormMatchesPolicy(
        {
          ...policyToFormState(policy),
          name: "Updated workflow",
        },
        policy,
      ),
    ).toBe(false);
    expect(
      reportWorkflowPolicyFormMatchesPolicy(
        {
          ...policyToFormState(policy),
          reportNotificationChannelProfileID: undefined,
        },
        policy,
      ),
    ).toBe(false);
    expect(
      reportWorkflowPolicyFormMatchesPolicy(
        { ...policyToFormState(policy), name: "" },
        policy,
      ),
    ).toBe(false);
    expect(
      reportWorkflowPolicyFormMatchesPolicy(policyToFormState(policy), null),
    ).toBe(false);
  });

  it("parses launch intents for overview-driven workflow setup", () => {
    expect(
      reportWorkflowPolicyLaunchIntentFromSearchParams({
        intent: "create-auto-room-policy",
      }),
    ).toEqual({
      alertSourceProfileID: null,
      diagnosisFollowUp: "auto_room",
      intent: "create-auto-room-policy",
      message:
        "Prepared an automatic diagnosis workflow from the settings overview create action.",
      name: "Automatic diagnosis workflow",
    });
    expect(
      reportWorkflowPolicyLaunchIntentFromSearchParams({
        intent: "auto-room-follow-up",
        source_id: "3",
      }),
    ).toEqual({
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      intent: "auto-room-follow-up",
      message:
        "Prepared automatic diagnosis room handoff from the settings overview action.",
      name: "Automatic diagnosis workflow",
    });
    expect(
      reportWorkflowPolicyLaunchIntentFromSearchParams({
        intent: "alertmanager-auto-diagnosis-proof",
        source_id: "3",
      }),
    ).toMatchObject({
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      intent: "alertmanager-auto-diagnosis-proof",
      message:
        "Loaded matching automatic diagnosis workflows for retained Alertmanager proof.",
      name: "Automatic diagnosis workflow",
    });
    expect(
      reportWorkflowPolicyLaunchIntentFromSearchParams({
        intent: "enable-ai-room-follow-up",
        source_id: "5",
      }),
    ).toMatchObject({
      alertSourceProfileID: 5,
      diagnosisFollowUp: "auto_room",
      intent: "enable-ai-room-follow-up",
      name: "Automatic diagnosis workflow",
    });
    expect(
      reportWorkflowPolicyLaunchIntentFromSearchParams({
        intent: "alertmanager-source",
        source_id: "not-a-number",
      }),
    ).toMatchObject({
      alertSourceProfileID: null,
      diagnosisFollowUp: "auto_room",
      intent: "alertmanager-source",
      name: "Automatic diagnosis workflow",
    });
    expect(
      reportWorkflowPolicyLaunchIntentFromSearchParams({ intent: "unknown" }),
    ).toBeNull();
  });

  it("builds stable workflow policy launch hrefs and keys", () => {
    const href = reportWorkflowPolicyLaunchHref({
      intent: "auto-room-follow-up",
      sourceID: 3,
    });
    const intent = reportWorkflowPolicyLaunchIntentFromSearchParams({
      intent: "auto-room-follow-up",
      source_id: "3",
    });

    expect(href).toBe(
      "/settings/report-workflow-policies?intent=auto-room-follow-up&source_id=3",
    );
    expect(
      reportWorkflowPolicyLaunchHref({ intent: "alertmanager-source" }),
    ).toBe("/settings/report-workflow-policies?intent=alertmanager-source");
    expect(
      reportWorkflowPolicyLaunchHref({ intent: "create-auto-room-policy" }),
    ).toBe("/settings/report-workflow-policies?intent=create-auto-room-policy");
    expect(
      reportWorkflowPolicyLaunchHref({
        intent: "alertmanager-auto-diagnosis-proof",
        sourceID: 3,
      }),
    ).toBe(
      "/settings/report-workflow-policies?intent=alertmanager-auto-diagnosis-proof&source_id=3",
    );
    expect(reportWorkflowPolicyLaunchIntentKey(intent)).toBe(
      "auto-room-follow-up:auto_room:3:Automatic diagnosis workflow:Prepared automatic diagnosis room handoff from the settings overview action.",
    );
    expect(reportWorkflowPolicyLaunchIntentKey(null)).toBe("default");
  });

  it("builds replay requests from bounded windows", () => {
    const parsed = formStateToReplayRequest({
      windowStart: "2026-06-05T08:00:00Z",
      windowEnd: "2026-06-05T09:00:00Z",
      limit: 25,
      correlationKey: " incident-42 ",
      workflowID: " report-batch-42 ",
    });

    expect(parsed).toEqual({
      ok: true,
      value: {
        window_start: "2026-06-05T08:00:00Z",
        window_end: "2026-06-05T09:00:00Z",
        limit: 25,
        correlation_key: "incident-42",
        workflow_id: "report-batch-42",
      },
    });
  });

  it("rejects invalid replay windows", () => {
    const parsed = formStateToReplayRequest({
      ...defaultReportWorkflowPolicyReplayForm(
        new Date("2026-06-05T09:00:00Z"),
      ),
      windowEnd: "2026-06-05T08:00:00Z",
    });

    expect(parsed).toEqual({
      ok: false,
      message: "Window end must be after window start.",
    });
  });

  it("defaults replay windows to the previous hour", () => {
    expect(
      defaultReportWorkflowPolicyReplayForm(new Date("2026-06-05T09:00:00Z")),
    ).toEqual({
      windowStart: "2026-06-05T08:00:00Z",
      windowEnd: "2026-06-05T09:00:00Z",
      limit: 10000,
      correlationKey: "",
      workflowID: "",
    });
  });

  it("reports diagnosis tools ready when source alerts and metrics are enabled", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3, 5]),
      alertSourceKindsByID: alertSourceKinds([
        [3, "alertmanager"],
        [5, "prometheus"],
      ]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      templates: [
        diagnosisToolTemplate(5, 3, "active_alerts", "", true),
        diagnosisToolTemplate(
          24,
          5,
          "metric_query",
          `db_tablespace_pctusd{ORACLE_SID="{{label.ORACLE_SID}}",TABLESPACE="{{label.TABLESPACE}}"}`,
          true,
        ),
      ],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 1,
      enabledMetricTemplates: 1,
      status: "ready",
    });
  });

  it("reports review when AI follow-up lacks metric tools", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3]),
      alertSourceKindsByID: alertSourceKinds([[3, "alertmanager"]]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "suggest_room",
      templates: [diagnosisToolTemplate(5, 3, "active_alerts", "", true)],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 1,
      enabledMetricTemplates: 0,
      status: "review",
    });
  });

  it("ignores metric templates bound to disabled sources", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3]),
      alertSourceKindsByID: alertSourceKinds([
        [3, "alertmanager"],
        [5, "prometheus"],
      ]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      templates: [
        diagnosisToolTemplate(5, 3, "active_alerts", "", true),
        diagnosisToolTemplate(24, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 1,
      detail: "Missing metric_query or metric_range_query.",
      enabledMetricTemplates: 0,
      status: "review",
    });
  });

  it("ignores metric templates bound to incompatible enabled sources", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3, 5]),
      alertSourceKindsByID: alertSourceKinds([
        [3, "alertmanager"],
        [5, "alertmanager"],
      ]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      templates: [
        diagnosisToolTemplate(5, 3, "active_alerts", "", true),
        diagnosisToolTemplate(24, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 1,
      detail: "Missing metric_query or metric_range_query.",
      enabledMetricTemplates: 0,
      status: "review",
    });
  });

  it("ignores metric templates bound to Thanos Rule active-alert sources", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3, 5]),
      alertSourceKindsByID: alertSourceKinds([
        [3, "alertmanager"],
        [5, "prometheus"],
      ]),
      alertSourceLabelsByID: new Map([
        [5, { role: "alert-intake", source: "thanos-rule" }],
      ]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      templates: [
        diagnosisToolTemplate(5, 3, "active_alerts", "", true),
        diagnosisToolTemplate(24, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 1,
      detail: "Missing metric_query or metric_range_query.",
      enabledMetricTemplates: 0,
      status: "review",
    });
  });

  it("blocks diagnosis tools when all enabled templates are bound to disabled sources", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3]),
      alertSourceKindsByID: alertSourceKinds([
        [3, "alertmanager"],
        [4, "alertmanager"],
        [5, "prometheus"],
      ]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
      templates: [
        diagnosisToolTemplate(5, 4, "active_alerts", "", true),
        diagnosisToolTemplate(24, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 0,
      detail:
        "Enabled diagnosis templates are bound only to disabled or incompatible sources.",
      enabledMetricTemplates: 0,
      status: "blocked",
    });
  });

  it("skips diagnosis tool readiness when follow-up is disabled", () => {
    const readiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([3]),
      alertSourceKindsByID: alertSourceKinds([[3, "alertmanager"]]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "disabled",
      templates: [diagnosisToolTemplate(5, 3, "active_alerts", "", true)],
    });

    expect(readiness).toMatchObject({
      activeAlertsForSource: 0,
      enabledMetricTemplates: 0,
      status: "pending",
    });
  });

  it("marks Alertmanager webhook ingress ready for auto-room policies", () => {
    const readiness = alertSourceIngressReadinessForSelection({
      alertSourceEnabledIDs: new Set([3]),
      alertSourceKindsByID: alertSourceKinds([[3, "alertmanager"]]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "auto_room",
    });

    expect(readiness).toEqual({
      detail:
        "Alertmanager webhook deliveries can ingest firing alerts and start automatic diagnosis rooms.",
      label: "Webhook auto-room ingress ready.",
      status: "ready",
    });
  });

  it("blocks auto-room webhook ingress for non-Alertmanager sources", () => {
    const readiness = alertSourceIngressReadinessForSelection({
      alertSourceEnabledIDs: new Set([5]),
      alertSourceKindsByID: alertSourceKinds([[5, "prometheus"]]),
      alertSourceProfileID: 5,
      diagnosisFollowUp: "auto_room",
    });

    expect(readiness).toEqual({
      detail:
        "Automatic diagnosis room starts require an Alertmanager alert source because the webhook endpoint rejects non-Alertmanager profiles.",
      label: "Webhook auto-room ingress blocked.",
      status: "blocked",
    });
  });

  it("explains Thanos Rule evidence role when it is selected for auto-room ingress", () => {
    const readiness = alertSourceIngressReadinessForSelection({
      alertSourceEnabledIDs: new Set([5]),
      alertSourceKindsByID: alertSourceKinds([[5, "prometheus"]]),
      alertSourceLabelsByID: new Map([
        [5, { role: "alert-intake", source: "thanos-rule" }],
      ]),
      alertSourceProfileID: 5,
      diagnosisFollowUp: "auto_room",
    });

    expect(readiness).toEqual({
      detail:
        "Thanos Rule active-alert sources can provide firing-alert evidence, but automatic diagnosis room starts require an Alertmanager webhook source. Select or create an Alertmanager source for workflow intake, then keep Thanos Rule for active_alerts evidence templates.",
      label: "Alertmanager webhook source required.",
      status: "blocked",
    });
  });

  it("marks Alertmanager webhook ingest ready for suggest-room policies without claiming auto start", () => {
    const readiness = alertSourceIngressReadinessForSelection({
      alertSourceEnabledIDs: new Set([3]),
      alertSourceKindsByID: alertSourceKinds([[3, "alertmanager"]]),
      alertSourceProfileID: 3,
      diagnosisFollowUp: "suggest_room",
    });

    expect(readiness).toEqual({
      detail:
        "Alertmanager webhook deliveries can ingest firing alerts; suggest_room still requires operator handoff.",
      label: "Webhook ingest ready.",
      status: "ready",
    });
  });

  it("treats an unbound report notification channel as pending for non-automatic follow-up", () => {
    const readiness = reportNotificationChannelReadinessForSelection({
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      diagnosisFollowUp: "suggest_room",
      notificationChannelEnabledIDs: new Set([3]),
      reportNotificationChannelIDs: new Set([3]),
      reportNotificationChannelProfileID: undefined,
    });

    expect(readiness).toMatchObject({
      missingScopes: [],
      requiredScopes: ["report"],
      status: "pending",
    });
  });

  it("blocks auto-room delivery when no notification channel is bound", () => {
    const readiness = reportNotificationChannelReadinessForSelection({
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      diagnosisFollowUp: "auto_room",
      notificationChannelEnabledIDs: new Set([3]),
      reportNotificationChannelIDs: new Set([3]),
      reportNotificationChannelProfileID: undefined,
    });

    expect(readiness).toMatchObject({
      detail:
        "Bind an enabled report channel with diagnosis_consultation and diagnosis_close scopes before using automatic diagnosis rooms.",
      missingScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      requiredScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      status: "blocked",
    });
  });

  it("accepts report-scoped channels for non-automatic follow-up", () => {
    const readiness = reportNotificationChannelReadinessForSelection({
      diagnosisConsultationNotificationChannelIDs: new Set<number>(),
      diagnosisCloseNotificationChannelIDs: new Set<number>(),
      diagnosisFollowUp: "suggest_room",
      notificationChannelEnabledIDs: new Set([3]),
      reportNotificationChannelIDs: new Set([3]),
      reportNotificationChannelProfileID: 3,
    });

    expect(readiness).toMatchObject({
      missingScopes: [],
      requiredScopes: ["report"],
      status: "ready",
    });
  });

  it("requires diagnosis consultation and close scopes for auto-room notification channels", () => {
    const readiness = reportNotificationChannelReadinessForSelection({
      diagnosisConsultationNotificationChannelIDs: new Set<number>(),
      diagnosisCloseNotificationChannelIDs: new Set<number>(),
      diagnosisFollowUp: "auto_room",
      notificationChannelEnabledIDs: new Set([3]),
      reportNotificationChannelIDs: new Set([3]),
      reportNotificationChannelProfileID: 3,
    });

    expect(readiness).toMatchObject({
      missingScopes: ["diagnosis_consultation", "diagnosis_close"],
      requiredScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      status: "blocked",
    });
  });

  it("requires current AI delivery proof for auto-room notification channels", () => {
    const readiness = reportNotificationChannelReadinessForSelection({
      diagnosisAIProofNotificationChannelIDs: new Set<number>(),
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      diagnosisFollowUp: "auto_room",
      notificationChannelEnabledIDs: new Set([3]),
      notificationChannelKindsByID: new Map<
        number,
        NotificationChannelProfile["kind"]
      >([[3, "wecom"]]),
      reportNotificationChannelIDs: new Set([3]),
      reportNotificationChannelProfileID: 3,
    });

    expect(readiness).toMatchObject({
      detail:
        "Open the selected Enterprise WeChat channel and run current AI diagnosis and diagnosis close sample tests before workflow policy enablement.",
      label: "AI delivery proof missing.",
      missingScopes: [],
      requiredScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      status: "blocked",
    });
  });

  it("marks auto-room notification channel options with blockers and proof review", () => {
    const base = {
      diagnosisAIProofNotificationChannelIDs: new Set<number>([3]),
      diagnosisCloseNotificationChannelIDs: new Set<number>([3, 4, 5, 6]),
      diagnosisConsultationNotificationChannelIDs: new Set<number>([3, 4, 5]),
      notificationChannelEnabledIDs: new Set<number>([3, 4, 5]),
      notificationChannelKindsByID: new Map<
        number,
        NotificationChannelProfile["kind"]
      >([
        [3, "wecom"],
        [4, "wecom"],
        [5, "webhook"],
        [6, "wecom"],
      ]),
    };

    expect(
      reportWorkflowNotificationChannelOptionState({
        ...base,
        diagnosisFollowUp: "disabled",
        notificationChannelProfileID: 5,
      }),
    ).toEqual({
      disabled: false,
      reasons: [],
      reviewReasons: [],
      status: "ready",
    });

    expect(
      reportWorkflowNotificationChannelOptionState({
        ...base,
        diagnosisFollowUp: "auto_room",
        notificationChannelProfileID: 3,
      }),
    ).toEqual({
      disabled: false,
      reasons: [],
      reviewReasons: [],
      status: "ready",
    });

    expect(
      reportWorkflowNotificationChannelOptionState({
        ...base,
        diagnosisFollowUp: "auto_room",
        notificationChannelProfileID: 4,
      }),
    ).toEqual({
      disabled: false,
      reasons: [],
      reviewReasons: ["missing AI proof"],
      status: "review",
    });

    expect(
      reportWorkflowNotificationChannelOptionState({
        ...base,
        diagnosisFollowUp: "auto_room",
        notificationChannelProfileID: 5,
      }),
    ).toEqual({
      disabled: true,
      reasons: ["requires Enterprise WeChat"],
      reviewReasons: [],
      status: "blocked",
    });

    expect(
      reportWorkflowNotificationChannelOptionState({
        ...base,
        diagnosisFollowUp: "auto_room",
        notificationChannelProfileID: 6,
      }),
    ).toEqual({
      disabled: true,
      reasons: ["disabled", "missing diagnosis_consultation"],
      reviewReasons: [],
      status: "blocked",
    });
  });

  it("requires notification channels to be enabled before delivery", () => {
    const readiness = reportNotificationChannelReadinessForSelection({
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      diagnosisFollowUp: "auto_room",
      notificationChannelEnabledIDs: new Set<number>(),
      reportNotificationChannelIDs: new Set([3]),
      reportNotificationChannelProfileID: 3,
    });

    expect(readiness).toMatchObject({
      detail:
        "Selected notification channel must be enabled before workflow policy enablement.",
      missingScopes: [],
      requiredScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
      status: "blocked",
    });
  });

  it("prefers enabled Enterprise WeChat channels for automatic room delivery", () => {
    const channels = [
      notificationChannel({
        id: 1,
        kind: "webhook",
        delivery_scopes: [
          "report",
          "diagnosis_consultation",
          "diagnosis_close",
        ],
      }),
      notificationChannel({
        id: 2,
        kind: "wecom",
        delivery_scopes: [
          "report",
          "diagnosis_consultation",
          "diagnosis_close",
        ],
      }),
      notificationChannel({
        id: 3,
        kind: "wecom",
        delivery_scopes: ["report", "diagnosis_consultation"],
      }),
    ];

    expect(
      preferredReportNotificationChannelIDForFollowUp({
        channels,
        diagnosisAIProofNotificationChannelIDs: new Set<number>([4, 2]),
        diagnosisFollowUp: "auto_room",
      }),
    ).toBe(2);
    expect(
      preferredReportNotificationChannelIDForFollowUp({
        channels,
        diagnosisFollowUp: "suggest_room",
      }),
    ).toBe(2);
    expect(
      preferredReportNotificationChannelIDForFollowUp({
        channels,
        diagnosisFollowUp: "disabled",
      }),
    ).toBeUndefined();
  });

  it("prefers proof-ready Enterprise WeChat channels for automatic room defaults", () => {
    const channels = [
      notificationChannel({
        id: 4,
        kind: "wecom",
        delivery_scopes: [
          "report",
          "diagnosis_consultation",
          "diagnosis_close",
        ],
      }),
      notificationChannel({
        id: 7,
        kind: "wecom",
        delivery_scopes: [
          "report",
          "diagnosis_consultation",
          "diagnosis_close",
        ],
      }),
    ];

    expect(
      preferredReportNotificationChannelIDForFollowUp({
        channels,
        diagnosisAIProofNotificationChannelIDs: new Set<number>([7]),
        diagnosisFollowUp: "auto_room",
      }),
    ).toBe(7);

    expect(
      preferredReportNotificationChannelIDForFollowUp({
        channels,
        diagnosisAIProofNotificationChannelIDs: new Set<number>(),
        diagnosisFollowUp: "auto_room",
      }),
    ).toBe(4);
  });

  it("does not fall back to an enabled full-scope webhook for automatic room delivery", () => {
    expect(
      preferredReportNotificationChannelIDForFollowUp({
        channels: [
          notificationChannel({
            id: 1,
            kind: "wecom",
            delivery_scopes: ["report", "diagnosis_consultation"],
          }),
          notificationChannel({
            id: 2,
            kind: "webhook",
            delivery_scopes: [
              "report",
              "diagnosis_consultation",
              "diagnosis_close",
            ],
          }),
        ],
        diagnosisFollowUp: "auto_room",
      }),
    ).toBeUndefined();
  });

  it("summarizes operator channel readiness for Enterprise WeChat and generic webhooks", () => {
    const readyDelivery = reportNotificationChannelReadinessForSelection({
      diagnosisConsultationNotificationChannelIDs: new Set([2, 3]),
      diagnosisCloseNotificationChannelIDs: new Set([2, 3]),
      diagnosisFollowUp: "auto_room",
      notificationChannelEnabledIDs: new Set([2, 3]),
      reportNotificationChannelIDs: new Set([2, 3]),
      reportNotificationChannelProfileID: 2,
    });

    expect(
      reportWorkflowNotificationChannelOperatorReadiness({
        channel: notificationChannel({
          id: 2,
          kind: "wecom",
          delivery_scopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
        diagnosisFollowUp: "auto_room",
        readiness: readyDelivery,
      }),
    ).toEqual({
      detail:
        "Enterprise WeChat can receive final report delivery, AI diagnosis updates, final-ready notices, and close notifications.",
      kindLabel: "WeCom",
      label: "Enterprise WeChat delivery selected.",
      status: "ready",
    });

    expect(
      reportWorkflowNotificationChannelOperatorReadiness({
        channel: notificationChannel({
          id: 3,
          kind: "webhook",
          delivery_scopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
        diagnosisFollowUp: "auto_room",
        readiness: readyDelivery,
      }),
    ).toMatchObject({
      detail:
        "Automatic diagnosis room delivery requires an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.",
      kindLabel: "Webhook",
      label: "Enterprise WeChat channel required.",
      status: "blocked",
    });
  });

  it("blocks policy enablement when automatic follow-up has no usable diagnosis tools", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Enable at least one active_alerts template and one metric template before relying on AI follow-up.",
      ],
      status: "blocked",
    });
  });

  it("blocks auto-room policy enablement when the bound source cannot receive Alertmanager webhooks", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 1, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Automatic diagnosis room starts require an Alertmanager alert source because the webhook endpoint rejects non-Alertmanager profiles.",
      ],
      status: "blocked",
    });
  });

  it("marks policy enablement for review when no notification channel is bound", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set<number>(),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "disabled",
        report_notification_channel_profile_id: null,
      },
      reportNotificationChannelIDs: new Set<number>(),
      templates: [],
    });

    expect(readiness).toMatchObject({
      blockers: [],
      status: "review",
      warnings: ["No notification channel profile is bound."],
    });
  });

  it("blocks auto-room policy enablement when no notification channel is bound", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set<number>(),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: null,
      },
      reportNotificationChannelIDs: new Set<number>(),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Bind an enabled report channel with diagnosis_consultation and diagnosis_close scopes before using automatic diagnosis rooms.",
      ],
      status: "blocked",
      warnings: [],
    });
  });

  it("marks policy enablement for review when metric tools are missing but active alerts are available", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set<number>(),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "suggest_room",
      },
      reportNotificationChannelIDs: new Set<number>(),
      templates: [diagnosisToolTemplate(5, 1, "active_alerts", "", true)],
    });

    expect(readiness).toMatchObject({
      blockers: [],
      status: "review",
      warnings: [
        "No notification channel profile is bound.",
        "Prometheus sources support metric evidence, but they do not receive Alertmanager webhook deliveries.",
        "Missing metric_query or metric_range_query.",
      ],
    });
  });

  it("marks policy enablement for review when metric tools only exist on Thanos Rule", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      alertSourceLabelsByID: new Map([
        [5, { role: "alert-intake", source: "thanos-rule" }],
      ]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [],
      status: "review",
      warnings: ["Missing metric_query or metric_range_query."],
    });
  });

  it("blocks policy enablement when auto-room notification delivery lacks close scope", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set<number>(),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Selected notification channel is missing diagnosis_close scope.",
      ],
      status: "blocked",
    });
  });

  it("blocks policy enablement when auto-room notification delivery lacks AI proof", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisAIProofNotificationChannelIDs: new Set<number>(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Open the selected Enterprise WeChat channel and run current AI diagnosis and diagnosis close sample tests before workflow policy enablement.",
      ],
      status: "blocked",
    });
  });

  it("blocks policy enablement when auto-room notification delivery uses a generic webhook", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      notificationChannelKindsByID: new Map<
        number,
        NotificationChannelProfile["kind"]
      >([[3, "webhook"]]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Automatic diagnosis room delivery requires an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.",
      ],
      status: "blocked",
    });
  });

  it("blocks policy enablement when the bound notification channel is disabled", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      notificationChannelEnabledIDs: new Set<number>(),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Selected notification channel must be enabled before workflow policy enablement.",
      ],
      status: "blocked",
    });
  });

  it("marks policy enablement for review when metric templates are bound only to disabled sources", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 9, "metric_range_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [],
      status: "review",
      warnings: ["Missing metric_query or metric_range_query."],
    });
  });

  it("allows policy enablement when bindings and diagnosis tools are ready", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [],
      status: "ready",
      warnings: [],
    });
  });

  it("selects workflow return candidates for AI room enablement handoffs", () => {
    const launchIntent = reportWorkflowPolicyLaunchIntentFromSearchParams({
      intent: "enable-ai-room-follow-up",
      source_id: "1",
    });
    const policies: ReportWorkflowPolicy[] = [
      {
        ...reportWorkflowPolicy(),
        id: 9,
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: null,
      },
      {
        ...reportWorkflowPolicy(),
        id: 7,
        diagnosis_follow_up: "auto_room",
        enabled: true,
        enabled_at: "2026-06-05T08:05:00Z",
        report_notification_channel_profile_id: 3,
      },
      {
        ...reportWorkflowPolicy(),
        id: 8,
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      {
        ...reportWorkflowPolicy(),
        id: 10,
        alert_source_profile_id: 5,
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      {
        ...reportWorkflowPolicy(),
        id: 11,
        report_notification_channel_profile_id: 3,
      },
    ];

    const candidates = reportWorkflowPolicyWorkflowReturnCandidates({
      ...enabledAutoRoomWorkflowBindings(),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      launchIntent,
      policies,
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
      ],
    });

    expect(
      candidates.map((candidate) => ({
        action: candidate.action,
        policyID: candidate.policy.id,
        status: candidate.readiness.status,
      })),
    ).toEqual([
      { action: "enable", policyID: 8, status: "ready" },
      { action: "already_enabled", policyID: 7, status: "ready" },
      { action: "blocked", policyID: 9, status: "blocked" },
    ]);
  });

  it("selects workflow return candidates for Alertmanager auto-diagnosis proof handoffs", () => {
    const launchIntent = reportWorkflowPolicyLaunchIntentFromSearchParams({
      intent: "alertmanager-auto-diagnosis-proof",
      source_id: "1",
    });

    expect(
      reportWorkflowPolicyWorkflowReturnCandidates({
        ...enabledAutoRoomWorkflowBindings(),
        diagnosisCloseNotificationChannelIDs: new Set([3]),
        launchIntent,
        policies: [
          {
            ...reportWorkflowPolicy(),
            id: 7,
            diagnosis_follow_up: "auto_room",
            enabled: true,
            enabled_at: "2026-06-05T08:05:00Z",
            report_notification_channel_profile_id: 3,
          },
          {
            ...reportWorkflowPolicy(),
            id: 8,
            diagnosis_follow_up: "auto_room",
            report_notification_channel_profile_id: 3,
          },
        ],
        reportNotificationChannelIDs: new Set([3]),
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
        ],
      }).map((candidate) => ({
        action: candidate.action,
        policyID: candidate.policy.id,
        status: candidate.readiness.status,
      })),
    ).toEqual([
      { action: "enable", policyID: 8, status: "ready" },
      { action: "already_enabled", policyID: 7, status: "ready" },
    ]);
  });

  it("does not select workflow return candidates for create handoffs", () => {
    const launchIntent = reportWorkflowPolicyLaunchIntentFromSearchParams({
      intent: "create-auto-room-policy",
      source_id: "1",
    });

    expect(
      reportWorkflowPolicyWorkflowReturnCandidates({
        ...enabledAutoRoomWorkflowBindings(),
        diagnosisCloseNotificationChannelIDs: new Set([3]),
        launchIntent,
        policies: [
          {
            ...reportWorkflowPolicy(),
            diagnosis_follow_up: "auto_room",
            report_notification_channel_profile_id: 3,
          },
        ],
        reportNotificationChannelIDs: new Set([3]),
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
        ],
      }),
    ).toEqual([]);
  });

  it("builds a draft execution plan for an auto-room workflow", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Ready automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const plan = reportWorkflowPolicyDraftPlan({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceLabels: { 1: "#1 Ready Alertmanager (alertmanager, enabled)" },
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
        ],
      }),
      editingPolicyID: null,
      form,
      groupingPolicyLabels: {
        2: "#2 Default alert grouping (cluster, service, enabled)",
      },
      notificationChannelLabels: {
        3: "#3 Operations close webhook (report, diagnosis_consultation, diagnosis_close, enabled)",
      },
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set([3]),
          diagnosisCloseNotificationChannelIDs: new Set([3]),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(plan.status).toBe("pending");
    expect(plan.steps.map((step) => [step.title, step.status])).toEqual([
      ["Save policy", "ready"],
      ["Enable policy", "pending"],
      ["Impact preview", "pending"],
      ["Replay window", "pending"],
      ["AI handoff", "ready"],
      ["Operator notification", "ready"],
    ]);
    expect(plan.steps[0]?.detail).toContain("#1 Ready Alertmanager");
    expect(plan.steps[5]?.detail).toContain("#3 Operations close webhook");
  });

  it("keeps draft save available when auto-room notification delivery is unbound", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: undefined,
      diagnosisFollowUp: "auto_room" as const,
    };
    const plan = reportWorkflowPolicyDraftPlan({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceLabels: { 1: "#1 Ready Alertmanager (alertmanager, enabled)" },
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_query", "up", true),
        ],
      }),
      editingPolicyID: null,
      form,
      groupingPolicyLabels: {
        2: "#2 Default alert grouping (cluster, service, enabled)",
      },
      notificationChannelLabels: {},
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set<number>(),
          diagnosisCloseNotificationChannelIDs: new Set<number>(),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set<number>(),
          reportNotificationChannelIDs: new Set<number>(),
          reportNotificationChannelProfileID: undefined,
        }),
    });

    expect(plan.status).toBe("blocked");
    expect(plan.steps.map((step) => [step.title, step.status])).toEqual([
      ["Save policy", "ready"],
      ["Enable policy", "blocked"],
      ["Impact preview", "pending"],
      ["Replay window", "blocked"],
      ["AI handoff", "ready"],
      ["Operator notification", "blocked"],
    ]);
    expect(plan.steps[0]?.detail).toContain("#1 Ready Alertmanager");
    expect(plan.steps[1]).toMatchObject({
      detail:
        "Bind an enabled report channel with diagnosis_consultation and diagnosis_close scopes before using automatic diagnosis rooms.",
      status: "blocked",
      title: "Enable policy",
    });
  });

  it("summarizes the automation outcome for an auto-room workflow", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Ready automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const outcome = reportWorkflowPolicyAutomationOutcome({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceLabels: { 1: "#1 Ready Alertmanager (alertmanager, enabled)" },
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
        ],
      }),
      form,
      groupingPolicyLabels: {
        2: "#2 Default alert grouping (cluster, service, enabled)",
      },
      notificationChannelLabels: {
        3: "#3 Operations close webhook (report, diagnosis_consultation, diagnosis_close, enabled)",
      },
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set([3]),
          diagnosisCloseNotificationChannelIDs: new Set([3]),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(outcome.status).toBe("ready");
    expect(outcome.detail).toBe(
      "Alertmanager alerts can produce evidence, start AI diagnosis rooms, and notify operators.",
    );
    expect(
      outcome.items.map((item) => [item.title, item.value, item.status]),
    ).toEqual([
      ["Trigger", "Manual replay", "ready"],
      ["Alert intake", "Webhook auto-room", "ready"],
      ["Evidence", "Tool collection", "ready"],
      ["AI room", "Automatic", "ready"],
      ["Notification", "Report and AI updates", "ready"],
    ]);
    expect(outcome.items[0]?.detail).toContain("#1 Ready Alertmanager");
    expect(outcome.items[4]?.detail).toContain("#3 Operations close webhook");
  });

  it("summarizes auto-room readiness as one operator checklist", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Ready automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const alertSourceIngressReadiness = alertSourceIngressReadinessForSelection(
      {
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      },
    );
    const diagnosisToolReadiness = diagnosisToolReadinessForSelection({
      alertSourceEnabledIDs: new Set([1, 5]),
      alertSourceKindsByID: alertSourceKinds([
        [1, "alertmanager"],
        [5, "prometheus"],
      ]),
      alertSourceProfileID: 1,
      diagnosisFollowUp: "auto_room",
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
      ],
    });
    const reportNotificationChannelReadiness =
      reportNotificationChannelReadinessForSelection({
        diagnosisConsultationNotificationChannelIDs: new Set([3]),
        diagnosisCloseNotificationChannelIDs: new Set([3]),
        diagnosisFollowUp: "auto_room",
        notificationChannelEnabledIDs: new Set([3]),
        reportNotificationChannelIDs: new Set([3]),
        reportNotificationChannelProfileID: 3,
      });
    const readiness = reportWorkflowPolicyAutoRoomReadiness({
      alertSourceIngressReadiness,
      diagnosisToolReadiness,
      form,
      operatorChannelReadiness:
        reportWorkflowNotificationChannelOperatorReadiness({
          channel: notificationChannel({
            id: 3,
            delivery_scopes: [
              "report",
              "diagnosis_consultation",
              "diagnosis_close",
            ],
          }),
          diagnosisFollowUp: "auto_room",
          readiness: reportNotificationChannelReadiness,
        }),
      reportNotificationChannelReadiness,
    });

    expect(readiness.status).toBe("ready");
    expect(readiness.label).toBe("Auto-room path ready.");
    expect(
      readiness.items.map((item) => [item.title, item.value, item.status]),
    ).toEqual([
      ["Alertmanager intake", "Webhook firing alerts", "ready"],
      ["AI evidence", "1 active alert / 1 metric", "ready"],
      ["Operator channel", "WeCom", "ready"],
      [
        "Delivery scopes",
        "report, diagnosis_consultation, diagnosis_close",
        "ready",
      ],
    ]);
  });

  it("keeps auto-room readiness blocked when delivery scopes are missing", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const reportNotificationChannelReadiness =
      reportNotificationChannelReadinessForSelection({
        diagnosisConsultationNotificationChannelIDs: new Set<number>(),
        diagnosisCloseNotificationChannelIDs: new Set<number>(),
        diagnosisFollowUp: "auto_room",
        notificationChannelEnabledIDs: new Set([3]),
        reportNotificationChannelIDs: new Set([3]),
        reportNotificationChannelProfileID: 3,
      });
    const readiness = reportWorkflowPolicyAutoRoomReadiness({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_query", "up", true),
        ],
      }),
      form,
      operatorChannelReadiness:
        reportWorkflowNotificationChannelOperatorReadiness({
          channel: notificationChannel({ id: 3 }),
          diagnosisFollowUp: "auto_room",
          readiness: reportNotificationChannelReadiness,
        }),
      reportNotificationChannelReadiness,
    });

    expect(readiness.status).toBe("blocked");
    expect(readiness.items[2]).toMatchObject({
      title: "Operator channel",
      status: "blocked",
    });
    expect(readiness.items[3]).toMatchObject({
      title: "Delivery scopes",
      status: "blocked",
    });
  });

  it("builds setup blueprint actions for missing auto-room prerequisites", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      diagnosisFollowUp: "auto_room" as const,
    };
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set<number>(),
        alertSourceKindsByID: alertSourceKinds([]),
        alertSourceProfileID: null,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceKindsByID: alertSourceKinds([]),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set<number>(),
        alertSourceKindsByID: alertSourceKinds([]),
        alertSourceProfileID: null,
        diagnosisFollowUp: "auto_room",
        templates: [],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set<number>(),
          diagnosisCloseNotificationChannelIDs: new Set<number>(),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set<number>(),
          reportNotificationChannelIDs: new Set<number>(),
          reportNotificationChannelProfileID: undefined,
        }),
    });

    expect(blueprint.status).toBe("blocked");
    expect(
      blueprint.actions.map((action) => [
        action.key,
        action.actionHref,
        action.status,
      ]),
    ).toEqual([
      [
        "alert-source",
        "/settings/alert-sources?intent=alertmanager-source",
        "pending",
      ],
      [
        "grouping-policy",
        "/settings/grouping-policies?intent=default-alert-grouping",
        "pending",
      ],
      [
        "notification-channel",
        "/settings/notification-channels?intent=report-close-channel&workflow_return=auto-room-enable",
        "blocked",
      ],
    ]);
    expect(
      blueprint.actions.find((action) => action.key === "notification-channel"),
    ).toMatchObject({
      actionLabel: "Create AI channel",
      detail:
        "Create or select an enabled Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes, run AI diagnosis and close proof, then return to enable this workflow.",
      title: "Report and AI-room channel",
    });
  });

  it("describes the auto-room setup chain as operator-facing phases", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceKindsByID: alertSourceKinds([
        [1, "alertmanager"],
        [5, "prometheus"],
      ]),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
        ],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set([3]),
          diagnosisCloseNotificationChannelIDs: new Set([3]),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(
      blueprint.phases.map((phase) => [
        phase.key,
        phase.title,
        phase.value,
        phase.status,
      ]),
    ).toEqual([
      ["alert-intake", "Alertmanager intake", "Webhook auto-room", "ready"],
      ["grouping", "Grouping rule", "Policy #2", "ready"],
      ["evidence", "Evidence collection", "1 active alert / 1 metric", "ready"],
      ["ai-consultation", "AI consultation", "Automatic", "ready"],
      [
        "operator-delivery",
        "WeCom delivery and proof",
        "Report and AI updates",
        "ready",
      ],
    ]);
    expect(blueprint.phases[4]?.detail).toContain(
      "auto-room AI diagnosis updates",
    );
  });

  it("builds setup blueprint actions for missing AI evidence tools", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceKindsByID: alertSourceKinds([
        [1, "alertmanager"],
        [5, "prometheus"],
      ]),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set([3]),
          diagnosisCloseNotificationChannelIDs: new Set([3]),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(
      blueprint.actions.map((action) => [action.key, action.actionHref]),
    ).toEqual([
      [
        "active-alert-tool",
        "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=1&workflow_return=auto-room-enable&workflow_source_id=1",
      ],
      [
        "metric-evidence-source",
        "/settings/alert-sources?intent=thanos-source&workflow_return=auto-room-enable&workflow_source_id=1",
      ],
    ]);
  });

  it("builds setup blueprint repair action when Thanos Rule is selected as an auto-room trigger", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 5,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const alertSourceKindsByID = alertSourceKinds([[5, "prometheus"]]);
    const alertSourceLabelsByID = new Map([
      [5, { role: "alert-intake", source: "thanos-rule" }],
    ]);
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([5]),
        alertSourceKindsByID,
        alertSourceLabelsByID,
        alertSourceProfileID: 5,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceKindsByID,
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([5]),
        alertSourceKindsByID,
        alertSourceProfileID: 5,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 5, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_query", "up", true),
        ],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisAIProofNotificationChannelIDs: new Set([3]),
          diagnosisConsultationNotificationChannelIDs: new Set([3]),
          diagnosisCloseNotificationChannelIDs: new Set([3]),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          notificationChannelKindsByID: new Map<
            number,
            NotificationChannelProfile["kind"]
          >([[3, "wecom"]]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(blueprint.actions).toEqual([
      expect.objectContaining({
        actionHref: "/settings/alert-sources?intent=alertmanager-source",
        actionLabel: "Configure source",
        detail:
          "Thanos Rule active-alert sources can provide firing-alert evidence, but automatic diagnosis room starts require an Alertmanager webhook source. Select or create an Alertmanager source for workflow intake, then keep Thanos Rule for active_alerts evidence templates.",
        key: "alert-source",
        status: "blocked",
        title: "Alertmanager webhook source",
      }),
    ]);
  });

  it("links setup blueprint metric action to the selected Prometheus-compatible source", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Suggested diagnosis workflow",
      alertSourceProfileID: 5,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "suggest_room" as const,
    };
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([5]),
        alertSourceKindsByID: alertSourceKinds([[5, "prometheus"]]),
        alertSourceProfileID: 5,
        diagnosisFollowUp: "suggest_room",
      }),
      alertSourceKindsByID: alertSourceKinds([[5, "prometheus"]]),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([5]),
        alertSourceKindsByID: alertSourceKinds([[5, "prometheus"]]),
        alertSourceProfileID: 5,
        diagnosisFollowUp: "suggest_room",
        templates: [
          diagnosisToolTemplate(7, 5, "active_alerts", "", true),
        ],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set<number>(),
          diagnosisCloseNotificationChannelIDs: new Set<number>(),
          diagnosisFollowUp: "suggest_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(
      blueprint.actions.map((action) => [action.key, action.actionHref]),
    ).toEqual([
      [
        "metric-evidence-tool",
        "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=5",
      ],
    ]);
  });

  it("routes setup blueprint metric action away from Thanos Rule active-alert sources", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Suggested diagnosis workflow",
      alertSourceProfileID: 5,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "suggest_room" as const,
    };
    const alertSourceKindsByID = alertSourceKinds([[5, "prometheus"]]);
    const alertSourceLabelsByID = new Map([
      [5, { role: "alert-intake", source: "thanos-rule" }],
    ]);
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([5]),
        alertSourceKindsByID,
        alertSourceLabelsByID,
        alertSourceProfileID: 5,
        diagnosisFollowUp: "suggest_room",
      }),
      alertSourceKindsByID,
      alertSourceLabelsByID,
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([5]),
        alertSourceKindsByID,
        alertSourceLabelsByID,
        alertSourceProfileID: 5,
        diagnosisFollowUp: "suggest_room",
        templates: [
          diagnosisToolTemplate(7, 5, "active_alerts", "", true),
        ],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set<number>(),
          diagnosisCloseNotificationChannelIDs: new Set<number>(),
          diagnosisFollowUp: "suggest_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(blueprint.actions).toEqual([
      expect.objectContaining({
        actionHref: "/settings/alert-sources?intent=thanos-source",
        actionLabel: "Configure metric source",
        key: "metric-evidence-source",
        status: "review",
        title: "Metric evidence source",
      }),
    ]);
  });

  it("links setup blueprint to edit a selected notification channel with missing scopes", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_query", "up", true),
        ],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set<number>(),
          diagnosisCloseNotificationChannelIDs: new Set<number>(),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(blueprint.actions).toEqual([
      expect.objectContaining({
        actionHref:
          "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=1",
        actionLabel: "Add AI scopes",
        detail:
          "Add diagnosis_consultation and diagnosis_close scope, run AI delivery proof, then return to enable this workflow.",
        key: "notification-channel",
        status: "blocked",
      }),
    ]);
  });

  it("links setup blueprint to edit a selected notification channel with missing AI proof", () => {
    const form = {
      ...emptyReportWorkflowPolicyForm(),
      name: "Automatic diagnosis workflow",
      alertSourceProfileID: 1,
      groupingPolicyID: 2,
      reportNotificationChannelProfileID: 3,
      diagnosisFollowUp: "auto_room" as const,
    };
    const blueprint = reportWorkflowPolicySetupBlueprint({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set([1]),
        alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
      }),
      alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set([1, 5]),
        alertSourceKindsByID: alertSourceKinds([
          [1, "alertmanager"],
          [5, "prometheus"],
        ]),
        alertSourceProfileID: 1,
        diagnosisFollowUp: "auto_room",
        templates: [
          diagnosisToolTemplate(5, 1, "active_alerts", "", true),
          diagnosisToolTemplate(6, 5, "metric_query", "up", true),
        ],
      }),
      form,
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisAIProofNotificationChannelIDs: new Set<number>(),
          diagnosisConsultationNotificationChannelIDs: new Set([3]),
          diagnosisCloseNotificationChannelIDs: new Set([3]),
          diagnosisFollowUp: "auto_room",
          notificationChannelEnabledIDs: new Set([3]),
          notificationChannelKindsByID: new Map<
            number,
            NotificationChannelProfile["kind"]
          >([[3, "wecom"]]),
          reportNotificationChannelIDs: new Set([3]),
          reportNotificationChannelProfileID: 3,
        }),
    });

    expect(blueprint.actions).toEqual([
      expect.objectContaining({
        actionHref:
          "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=1",
        actionLabel: "Run AI proof",
        detail:
          "Open the selected Enterprise WeChat channel, run the current AI diagnosis and diagnosis close sample tests, then return to enable this workflow.",
        key: "notification-channel",
        status: "blocked",
        title: "AI delivery proof",
      }),
    ]);
  });

  it("builds persisted policy repair actions for disabled bindings", () => {
    const blueprint = reportWorkflowPolicyRepairBlueprint({
      alertSourceEnabledIDs: new Set<number>(),
      alertSourceKindsByID: alertSourceKinds([[1, "alertmanager"]]),
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      groupingPolicyEnabledIDs: new Set<number>(),
      notificationChannelEnabledIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 1, "metric_query", "up", true),
      ],
    });

    expect(blueprint.status).toBe("blocked");
    expect(
      blueprint.actions.map((action) => [
        action.key,
        action.actionHref,
        action.actionLabel,
      ]),
    ).toEqual([
      ["alert-source", "/settings/alert-sources", "Review source"],
      ["grouping-policy", "/settings/grouping-policies", "Review grouping"],
    ]);
  });

  it("builds persisted policy repair actions for missing AI evidence tools", () => {
    const blueprint = reportWorkflowPolicyRepairBlueprint({
      alertSourceEnabledIDs: new Set([1, 5]),
      alertSourceKindsByID: alertSourceKinds([
        [1, "alertmanager"],
        [5, "prometheus"],
      ]),
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      groupingPolicyEnabledIDs: new Set([2]),
      notificationChannelEnabledIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [],
    });

    expect(blueprint.status).toBe("blocked");
    expect(
      blueprint.actions.map((action) => [action.key, action.actionHref]),
    ).toEqual([
      [
        "active-alert-tool",
        "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=1&workflow_return=auto-room-enable&workflow_source_id=1",
      ],
      [
        "metric-evidence-source",
        "/settings/alert-sources?intent=thanos-source&workflow_return=auto-room-enable&workflow_source_id=1",
      ],
    ]);
  });

  it("summarizes replay proof trace with retained evidence and automatic diagnosis handoff", () => {
    const trace = reportWorkflowPolicyReplayProofTrace(
      reportReplayTriggerResponse({
        auto_diagnosis: {
          policies_matched: 1,
          snapshots: 2,
          rooms_started: 1,
          rooms_skipped: 1,
          skipped_snapshot_ids: [102],
          rooms: [
            {
              policy_id: 7,
              evidence_snapshot_id: 101,
              session_id: "diagnosis-session-auto-p7-s101",
              initial_message_id: "diagnosis-auto-initial-p7-s101",
              workflow_id: "diagnosis-room-diagnosis-session-auto-p7-s101",
              run_id: "run-diagnosis-101",
            },
          ],
        },
        snapshots: [
          { id: 101, group_index: 0, event_count: 2 },
          { id: 102, group_index: 1, event_count: 1 },
        ],
        stats: {
          ...reportReplayTriggerResponse().stats,
          events_loaded: 3,
          groups_built: 2,
          snapshots_saved: 2,
        },
      }),
    );

    expect(trace.status).toBe("review");
    expect(trace.detail).toBe(
      "Replay produced usable proof, with downstream AI or notification evidence to review.",
    );
    expect(
      trace.items.map((item) => [item.title, item.value, item.status]),
    ).toEqual([
      ["Trigger", "Workflow accepted", "ready"],
      ["Evidence", "2 saved", "ready"],
      ["AI diagnosis", "1 room", "review"],
      ["Notification proof", "Room timeline", "review"],
    ]);
    expect(trace.items[2]?.detail).toContain(
      "1 snapshot remains for manual room creation",
    );
    expect(trace.items[3]?.detail).toContain(
      "diagnosis-room notification timeline",
    );
    expect(trace.items[3]?.detail).toContain(
      "1 snapshot remains without automatic room timelines",
    );
  });

  it("keeps replay proof trace pending when replay starts no workflow", () => {
    const trace = reportWorkflowPolicyReplayProofTrace(
      reportReplayTriggerResponse({
        run_id: "",
        snapshots: [],
        started: false,
        stats: {
          ...reportReplayTriggerResponse().stats,
          events_loaded: 0,
          groups_built: 0,
          snapshots_saved: 0,
        },
        workflow_id: "",
      }),
    );

    expect(trace.status).toBe("pending");
    expect(
      trace.items.map((item) => [item.title, item.value, item.status]),
    ).toEqual([
      ["Trigger", "No workflow", "pending"],
      ["Evidence", "0 saved", "pending"],
      ["AI diagnosis", "Not started", "pending"],
      ["Notification proof", "Not available", "pending"],
    ]);
  });

  it("blocks the draft execution plan until required form fields are selected", () => {
    const plan = reportWorkflowPolicyDraftPlan({
      alertSourceIngressReadiness: alertSourceIngressReadinessForSelection({
        alertSourceEnabledIDs: new Set<number>(),
        alertSourceKindsByID: alertSourceKinds([]),
        alertSourceProfileID: null,
        diagnosisFollowUp: "disabled",
      }),
      alertSourceLabels: {},
      diagnosisToolReadiness: diagnosisToolReadinessForSelection({
        alertSourceEnabledIDs: new Set<number>(),
        alertSourceKindsByID: alertSourceKinds([]),
        alertSourceProfileID: null,
        diagnosisFollowUp: "disabled",
        templates: [],
      }),
      editingPolicyID: null,
      form: emptyReportWorkflowPolicyForm(),
      groupingPolicyLabels: {},
      notificationChannelLabels: {},
      reportNotificationChannelReadiness:
        reportNotificationChannelReadinessForSelection({
          diagnosisConsultationNotificationChannelIDs: new Set<number>(),
          diagnosisCloseNotificationChannelIDs: new Set<number>(),
          diagnosisFollowUp: "disabled",
          notificationChannelEnabledIDs: new Set<number>(),
          reportNotificationChannelIDs: new Set<number>(),
          reportNotificationChannelProfileID: undefined,
        }),
    });

    expect(plan.status).toBe("blocked");
    expect(plan.detail).toBe("Policy name is required.");
    expect(plan.steps.map((step) => step.status)).toEqual([
      "blocked",
      "blocked",
      "blocked",
      "blocked",
      "blocked",
      "blocked",
    ]);
  });

  it("maps impact preview reason codes to operator-facing explanations", () => {
    expect(reportWorkflowPolicyImpactReason("ok")).toEqual({
      code: "ok",
      detail:
        "Configuration bindings are usable and the bounded sample produced an impact estimate.",
      label: "Preview ready",
      tagColor: "green",
    });

    expect(
      reportWorkflowPolicyImpactReason(
        "notification_channel_missing_diagnosis_close_scope",
      ),
    ).toEqual({
      code: "notification_channel_missing_diagnosis_close_scope",
      detail:
        "Add the diagnosis_close scope when auto_room should deliver close notifications.",
      label: "Diagnosis close scope missing",
      tagColor: "red",
    });

    expect(
      reportWorkflowPolicyImpactReason(
        "notification_channel_missing_diagnosis_consultation_scope",
      ),
    ).toEqual({
      code: "notification_channel_missing_diagnosis_consultation_scope",
      detail:
        "Add the diagnosis_consultation scope when auto_room should deliver AI diagnosis updates.",
      label: "Diagnosis consultation scope missing",
      tagColor: "red",
    });

    expect(
      reportWorkflowPolicyImpactReason(
        "notification_channel_missing_ai_delivery_proof",
      ),
    ).toEqual({
      code: "notification_channel_missing_ai_delivery_proof",
      detail:
        "Run current AI diagnosis and diagnosis close sample tests for the bound Enterprise WeChat channel.",
      label: "AI delivery proof missing",
      tagColor: "red",
    });

    expect(
      reportWorkflowPolicyImpactReason("notification_channel_missing"),
    ).toEqual({
      code: "notification_channel_missing",
      detail:
        "Bind a notification channel before enabling auto_room AI diagnosis updates.",
      label: "Notification channel required",
      tagColor: "red",
    });

    expect(
      reportWorkflowPolicyImpactReason("notification_channel_not_wecom"),
    ).toEqual({
      code: "notification_channel_not_wecom",
      detail:
        "Use an Enterprise WeChat channel before enabling auto_room AI diagnosis updates.",
      label: "Enterprise WeChat required",
      tagColor: "red",
    });

    expect(
      reportWorkflowPolicyImpactReason("auto_room_requires_alertmanager"),
    ).toEqual({
      code: "auto_room_requires_alertmanager",
      detail:
        "Bind an Alertmanager alert source before using auto_room diagnosis follow-up.",
      label: "Alertmanager source required",
      tagColor: "red",
    });

    expect(reportWorkflowPolicyImpactReason("no_matching_events")).toEqual({
      code: "no_matching_events",
      detail:
        "Recent bounded samples did not match this source and grouping configuration.",
      label: "No matching events",
      tagColor: "gold",
    });
  });

  it("summarizes impact preview AI diagnosis estimates", () => {
    expect(
      reportWorkflowPolicyImpactDiagnosisEstimate(
        reportWorkflowPolicyImpactPreviewResult({
          diagnosis_follow_up: "auto_room",
          groups_estimated: 2,
        }),
      ),
    ).toEqual({
      detail:
        "2 estimated alert groups can start automatic AI diagnosis rooms when this policy is replayed or receives matching Alertmanager webhooks.",
      label: "Automatic diagnosis rooms estimated.",
      status: "ready",
      value: "2 rooms",
    });

    expect(
      reportWorkflowPolicyImpactDiagnosisEstimate(
        reportWorkflowPolicyImpactPreviewResult({
          diagnosis_follow_up: "suggest_room",
          groups_estimated: 1,
        }),
      ),
    ).toEqual({
      detail:
        "1 estimated alert group can be retained for operator-created diagnosis rooms.",
      label: "Operator handoff retained.",
      status: "review",
      value: "1 handoff",
    });

    expect(
      reportWorkflowPolicyImpactDiagnosisEstimate(
        reportWorkflowPolicyImpactPreviewResult({
          diagnosis_follow_up: "disabled",
          groups_estimated: 3,
        }),
      ),
    ).toEqual({
      detail:
        "This policy does not request AI diagnosis handoff for matched alert groups.",
      label: "AI diagnosis disabled.",
      status: "pending",
      value: "Report only",
    });

    expect(
      reportWorkflowPolicyImpactDiagnosisEstimate(
        reportWorkflowPolicyImpactPreviewResult({
          diagnosis_follow_up: "auto_room",
          groups_estimated: 2,
          reason_codes: ["notification_channel_missing"],
          status: "blocked",
        }),
      ),
    ).toMatchObject({
      detail:
        "Automatic diagnosis rooms will not start until the blocked preview reasons are resolved.",
      status: "blocked",
      value: "Blocked",
    });
  });

  it("summarizes impact preview report channel readiness including AI delivery proof", () => {
    expect(
      reportWorkflowPolicyImpactReportChannelReadiness(
        reportWorkflowPolicyImpactPreviewResult(),
      ),
    ).toEqual({
      ready: true,
      text: "#3 scopes and proof ready",
    });

    expect(
      reportWorkflowPolicyImpactReportChannelReadiness(
        reportWorkflowPolicyImpactPreviewResult({
          report_notification_channel_has_diagnosis_close_scope: false,
          reason_codes: ["notification_channel_missing_diagnosis_close_scope"],
          status: "blocked",
        }),
      ),
    ).toEqual({
      ready: false,
      text: "#3 missing diagnosis_close",
    });

    expect(
      reportWorkflowPolicyImpactReportChannelReadiness(
        reportWorkflowPolicyImpactPreviewResult({
          reason_codes: ["notification_channel_missing_ai_delivery_proof"],
          status: "blocked",
        }),
      ),
    ).toEqual({
      ready: false,
      text: "#3 missing AI delivery proof",
    });
  });

  it("blocks policy enablement when the bound alert source is disabled", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      alertSourceEnabledIDs: new Set<number>(),
      alertSourceKindsByID: alertSourceKinds([[1, "prometheus"]]),
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      groupingPolicyEnabledIDs: new Set([2]),
      notificationChannelEnabledIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 1, "metric_range_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Bound alert source must be enabled before workflow policy enablement.",
      ],
      status: "blocked",
    });
  });

  it("blocks policy enablement when the bound grouping policy is disabled", () => {
    const readiness = reportWorkflowPolicyEnablementReadiness({
      alertSourceEnabledIDs: new Set([1, 5]),
      alertSourceKindsByID: alertSourceKinds([
        [1, "alertmanager"],
        [5, "prometheus"],
      ]),
      diagnosisConsultationNotificationChannelIDs: new Set([3]),
      diagnosisCloseNotificationChannelIDs: new Set([3]),
      groupingPolicyEnabledIDs: new Set<number>(),
      notificationChannelEnabledIDs: new Set([3]),
      policy: {
        ...reportWorkflowPolicy(),
        diagnosis_follow_up: "auto_room",
        report_notification_channel_profile_id: 3,
      },
      reportNotificationChannelIDs: new Set([3]),
      templates: [
        diagnosisToolTemplate(5, 1, "active_alerts", "", true),
        diagnosisToolTemplate(6, 5, "metric_range_query", "up", true),
      ],
    });

    expect(readiness).toMatchObject({
      blockers: [
        "Bound grouping policy must be enabled before workflow policy enablement.",
      ],
      status: "blocked",
    });
  });
});

function enabledWorkflowBindings() {
  return {
    alertSourceEnabledIDs: new Set([1]),
    alertSourceKindsByID: alertSourceKinds([[1, "prometheus"]]),
    diagnosisConsultationNotificationChannelIDs: new Set([3]),
    groupingPolicyEnabledIDs: new Set([2]),
    notificationChannelEnabledIDs: new Set([3]),
    notificationChannelKindsByID: new Map<
      number,
      NotificationChannelProfile["kind"]
    >([[3, "wecom"]]),
  };
}

function enabledAutoRoomWorkflowBindings() {
  return {
    alertSourceEnabledIDs: new Set([1, 5]),
    alertSourceKindsByID: alertSourceKinds([
      [1, "alertmanager"],
      [5, "prometheus"],
    ]),
    diagnosisAIProofNotificationChannelIDs: new Set([3]),
    diagnosisConsultationNotificationChannelIDs: new Set([3]),
    groupingPolicyEnabledIDs: new Set([2]),
    notificationChannelEnabledIDs: new Set([3]),
    notificationChannelKindsByID: new Map<
      number,
      NotificationChannelProfile["kind"]
    >([[3, "wecom"]]),
  };
}

function alertSourceKinds(
  entries: Array<[number, "alertmanager" | "prometheus"]>,
): ReadonlyMap<number, "alertmanager" | "prometheus"> {
  return new Map(entries);
}

function reportWorkflowPolicy(): ReportWorkflowPolicy {
  return {
    id: 7,
    name: "Default report workflow",
    alert_source_profile_id: 1,
    grouping_policy_id: 2,
    report_notification_channel_profile_id: null,
    trigger_mode: "manual_replay",
    report_scenario: "single_alert",
    diagnosis_follow_up: "disabled",
    enabled: false,
    enabled_at: null,
    disabled_at: null,
    created_at: "2026-06-05T08:00:00Z",
    updated_at: "2026-06-05T08:00:00Z",
  };
}

function notificationChannel(
  overrides: Partial<NotificationChannelProfile> = {},
): NotificationChannelProfile {
  return {
    id: 1,
    name: "Operations WeCom",
    kind: "wecom",
    secret_ref: "secret/openclarion/ops-wecom",
    delivery_scopes: ["report"],
    enabled: true,
    labels: {},
    latest_test_results: [],
    created_at: "2026-06-05T08:00:00Z",
    updated_at: "2026-06-05T08:00:00Z",
    ...overrides,
  };
}

function reportWorkflowPolicyImpactPreviewResult(
  overrides: Partial<ReportWorkflowPolicyImpactPreviewResult> = {},
): ReportWorkflowPolicyImpactPreviewResult {
  return {
    alert_source_auth_mode: "none",
    alert_source_enabled: true,
    alert_source_kind: "alertmanager",
    alert_source_profile_id: 1,
    checked_at: "2026-06-05T10:00:00Z",
    diagnosis_follow_up: "auto_room",
    events_matched: 2,
    events_scanned: 3,
    grouping_dimension_keys: ["alertname", "service"],
    grouping_policy_enabled: true,
    grouping_policy_id: 2,
    grouping_severity_key: "severity",
    grouping_source_filter: ["alertmanager"],
    groups: [],
    groups_estimated: 2,
    message: "Report workflow policy impact preview is ready.",
    policy_id: 7,
    reason_codes: ["ok"],
    report_notification_channel_bound: true,
    report_notification_channel_enabled: true,
    report_notification_channel_has_diagnosis_close_scope: true,
    report_notification_channel_has_diagnosis_consultation_scope: true,
    report_notification_channel_has_report_scope: true,
    report_notification_channel_profile_id: 3,
    report_scenario: "single_alert",
    status: "ready",
    trigger_mode: "manual_replay",
    ...overrides,
  };
}

function reportReplayTriggerResponse(
  overrides: Partial<ReportReplayTriggerResponse> = {},
): ReportReplayTriggerResponse {
  return {
    started: true,
    correlation_key: "policy-window-smoke",
    workflow_id: "report-batch-policy-smoke",
    run_id: "run-policy-smoke",
    stats: {
      ingested: {
        total: 3,
        saved: 3,
        duplicate: 0,
        failed: 0,
      },
      events_loaded: 3,
      groups_built: 1,
      groups_saved: 1,
      groups_refreshed: 0,
      groups_existing: 0,
      snapshots_saved: 1,
      snapshots_duplicate: 0,
      groups_closed: 0,
      failed: 0,
    },
    snapshots: [{ id: 101, group_index: 0, event_count: 3 }],
    ...overrides,
  };
}

function diagnosisToolTemplate(
  id: number,
  sourceID: number,
  tool: DiagnosisToolTemplate["tool"],
  queryTemplate: string,
  enabled: boolean,
): DiagnosisToolTemplate {
  return {
    id,
    name: `Template ${id}`,
    alert_source_profile_id: sourceID,
    tool,
    query_template: queryTemplate,
    default_limit: 5,
    default_window_seconds: tool === "metric_range_query" ? 3600 : 0,
    max_window_seconds: tool === "metric_range_query" ? 21600 : 0,
    default_step_seconds: tool === "metric_range_query" ? 60 : 0,
    enabled,
    enabled_at: enabled ? "2026-06-05T08:00:00Z" : null,
    disabled_at: enabled ? null : "2026-06-05T08:00:00Z",
    created_at: "2026-06-05T08:00:00Z",
    updated_at: "2026-06-05T08:00:00Z",
  };
}
