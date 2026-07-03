import type {
  DiagnosisToolTemplate,
  DiagnosisToolKind,
} from "../diagnosis-tool-templates/types";
import {
  diagnosisToolSupportsSourceKind,
  diagnosisToolTemplateLaunchHref,
} from "../diagnosis-tool-templates/format";
import type { AlertSourceKind, AlertSourceLabels } from "../alert-sources/types";
import { alertSourceLaunchHref } from "../alert-sources/format";
import { groupingPolicyLaunchHref } from "../grouping-policies/format";
import {
  notificationChannelEditHref,
  notificationChannelLaunchHref,
} from "../notification-channels/format";
import type { NotificationChannelProfile } from "../notification-channels/types";
import type {
  ReportReplayTriggerResponse,
  ReportWorkflowPolicy,
  ReportWorkflowPolicyFormState,
  ReportWorkflowPolicyImpactPreviewResult,
  ReportWorkflowPolicyReplayFormState,
  ReportWorkflowPolicyReplayRequest,
  ReportWorkflowPolicyWriteRequest,
} from "./types";
import {
  reportReplayProofTrace,
  type ReportReplayProofTrace,
} from "@/features/report-replay/proof-trace";

type ParseResult<T> = { ok: true; value: T } | { ok: false; message: string };

type DiagnosisToolReadinessStatus = "ready" | "review" | "pending" | "blocked";
type ImpactPreviewReasonCode =
  ReportWorkflowPolicyImpactPreviewResult["reason_codes"][number];
type SearchParamValue = string | string[] | undefined;

export type ReportWorkflowPolicyLaunchIntentName =
  | "alertmanager-source"
  | "alertmanager-auto-diagnosis-proof"
  | "auto-room-follow-up"
  | "create-auto-room-policy"
  | "enable-ai-room-follow-up";

export type ReportWorkflowPolicyLaunchIntent = {
  alertSourceProfileID: number | null;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
  intent: ReportWorkflowPolicyLaunchIntentName;
  message: string;
  name: string;
};

export type DiagnosisToolReadiness = {
  activeAlertsForSource: number;
  detail: string;
  enabledMetricTemplates: number;
  enabledRangeTemplates: number;
  enabledTemplates: number;
  label: string;
  status: DiagnosisToolReadinessStatus;
  templateNames: string[];
};

export type NotificationChannelDeliveryReadiness = {
  detail: string;
  label: string;
  missingScopes: string[];
  requiredScopes: string[];
  status: DiagnosisToolReadinessStatus;
};

export type ReportWorkflowNotificationChannelOperatorReadiness = {
  detail: string;
  kindLabel: string;
  label: string;
  status: DiagnosisToolReadinessStatus;
};

export type AlertSourceIngressReadiness = {
  detail: string;
  label: string;
  status: DiagnosisToolReadinessStatus;
};

export type ReportWorkflowPolicyEnablementReadiness = {
  blockers: string[];
  detail: string;
  label: string;
  status: DiagnosisToolReadinessStatus;
  warnings: string[];
};

type ReportWorkflowPolicyWorkflowReturnCandidateAction =
  | "already_enabled"
  | "blocked"
  | "enable"
  | "review";

export type ReportWorkflowPolicyWorkflowReturnCandidate = {
  action: ReportWorkflowPolicyWorkflowReturnCandidateAction;
  detail: string;
  policy: ReportWorkflowPolicy;
  readiness: ReportWorkflowPolicyEnablementReadiness;
};

type ReportWorkflowPolicyDraftPlanStep = {
  detail: string;
  status: DiagnosisToolReadinessStatus;
  title: string;
};

type ReportWorkflowPolicyAutomationOutcomeItem = {
  detail: string;
  status: DiagnosisToolReadinessStatus;
  title: string;
  value: string;
};

type ReportWorkflowPolicyAutoRoomReadinessItem = {
  detail: string;
  status: DiagnosisToolReadinessStatus;
  title: string;
  value: string;
};

type ReportWorkflowPolicySetupAction = {
  actionHref?: string;
  actionLabel: string;
  detail: string;
  key: string;
  status: DiagnosisToolReadinessStatus;
  title: string;
};

type ReportWorkflowPolicySetupPhase = {
  detail: string;
  key: string;
  status: DiagnosisToolReadinessStatus;
  title: string;
  value: string;
};

export type ReportWorkflowNotificationChannelOptionState = {
  disabled: boolean;
  reasons: string[];
  reviewReasons: string[];
  status: DiagnosisToolReadinessStatus;
};

export type ReportWorkflowPolicyDraftPlan = {
  detail: string;
  status: DiagnosisToolReadinessStatus;
  steps: ReportWorkflowPolicyDraftPlanStep[];
};

export type ReportWorkflowPolicyAutomationOutcome = {
  detail: string;
  items: ReportWorkflowPolicyAutomationOutcomeItem[];
  status: DiagnosisToolReadinessStatus;
};

export type ReportWorkflowPolicyAutoRoomReadiness = {
  detail: string;
  items: ReportWorkflowPolicyAutoRoomReadinessItem[];
  label: string;
  status: DiagnosisToolReadinessStatus;
};

export type ReportWorkflowPolicySetupBlueprint = {
  actions: ReportWorkflowPolicySetupAction[];
  detail: string;
  label: string;
  phases: ReportWorkflowPolicySetupPhase[];
  status: DiagnosisToolReadinessStatus;
};

export type ReportWorkflowPolicyReplayProofTrace = ReportReplayProofTrace;

export type ReportWorkflowPolicyImpactReason = {
  code: ImpactPreviewReasonCode;
  detail: string;
  label: string;
  tagColor: string;
};

export type ReportWorkflowPolicyImpactDiagnosisEstimate = {
  detail: string;
  label: string;
  status: DiagnosisToolReadinessStatus;
  value: string;
};

export type ReportWorkflowPolicyImpactReportChannelReadiness = {
  ready: boolean;
  text: string;
};

export function emptyReportWorkflowPolicyForm(): ReportWorkflowPolicyFormState {
  return {
    name: "",
    alertSourceProfileID: null,
    groupingPolicyID: null,
    reportNotificationChannelProfileID: undefined,
    triggerMode: "manual_replay",
    reportScenario: "single_alert",
    diagnosisFollowUp: "disabled",
  };
}

export function reportWorkflowPolicyLaunchHref({
  intent,
  sourceID,
}: {
  intent: ReportWorkflowPolicyLaunchIntentName;
  sourceID?: number | null;
}): string {
  const params = new URLSearchParams({ intent });
  if (positiveInteger(sourceID ?? null)) {
    params.set("source_id", String(sourceID));
  }
  return `/settings/report-workflow-policies?${params.toString()}`;
}

export function reportWorkflowPolicyLaunchIntentFromSearchParams(
  searchParams: Record<string, SearchParamValue>,
): ReportWorkflowPolicyLaunchIntent | null {
  const sourceID = positiveSearchParamInteger(
    firstSearchParamValue(searchParams.source_id),
  );
  switch (firstSearchParamValue(searchParams.intent)) {
    case "create-auto-room-policy":
      return {
        alertSourceProfileID: sourceID,
        diagnosisFollowUp: "auto_room",
        intent: "create-auto-room-policy",
        message:
          "Prepared an automatic diagnosis workflow from the settings overview create action.",
        name: "Automatic diagnosis workflow",
      };
    case "alertmanager-source":
      return {
        alertSourceProfileID: null,
        diagnosisFollowUp: "auto_room",
        intent: "alertmanager-source",
        message:
          "Prepared an automatic diagnosis workflow that needs an enabled Alertmanager source.",
        name: "Automatic diagnosis workflow",
      };
    case "alertmanager-auto-diagnosis-proof":
      return {
        alertSourceProfileID: sourceID,
        diagnosisFollowUp: "auto_room",
        intent: "alertmanager-auto-diagnosis-proof",
        message:
          "Loaded matching automatic diagnosis workflows for retained Alertmanager proof.",
        name: "Automatic diagnosis workflow",
      };
    case "auto-room-follow-up":
      return {
        alertSourceProfileID: sourceID,
        diagnosisFollowUp: "auto_room",
        intent: "auto-room-follow-up",
        message:
          "Prepared automatic diagnosis room handoff from the settings overview action.",
        name: "Automatic diagnosis workflow",
      };
    case "enable-ai-room-follow-up":
      return {
        alertSourceProfileID: sourceID,
        diagnosisFollowUp: "auto_room",
        intent: "enable-ai-room-follow-up",
        message:
          "Prepared automatic AI diagnosis room handoff from the settings overview action.",
        name: "Automatic diagnosis workflow",
      };
    default:
      return null;
  }
}

export function reportWorkflowPolicyLaunchIntentKey(
  launchIntent: ReportWorkflowPolicyLaunchIntent | null,
): string {
  if (launchIntent === null) {
    return "default";
  }
  return `${launchIntent.intent}:${launchIntent.diagnosisFollowUp}:${launchIntent.alertSourceProfileID ?? "auto"}:${launchIntent.name}:${launchIntent.message}`;
}

export function reportNotificationChannelReadinessForSelection({
  diagnosisConsultationNotificationChannelIDs,
  diagnosisCloseNotificationChannelIDs,
  diagnosisAIProofNotificationChannelIDs,
  diagnosisFollowUp,
  notificationChannelEnabledIDs,
  notificationChannelKindsByID,
  reportNotificationChannelIDs,
  reportNotificationChannelProfileID,
}: {
  diagnosisConsultationNotificationChannelIDs: ReadonlySet<number>;
  diagnosisCloseNotificationChannelIDs: ReadonlySet<number>;
  diagnosisAIProofNotificationChannelIDs?: ReadonlySet<number>;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
  notificationChannelEnabledIDs: ReadonlySet<number>;
  notificationChannelKindsByID?: ReadonlyMap<
    number,
    NotificationChannelProfile["kind"]
  >;
  reportNotificationChannelIDs: ReadonlySet<number>;
  reportNotificationChannelProfileID: number | null | undefined;
}): NotificationChannelDeliveryReadiness {
  const requiredScopes =
    diagnosisFollowUp === "auto_room"
      ? ["report", "diagnosis_consultation", "diagnosis_close"]
      : ["report"];
  const channelID = reportNotificationChannelProfileID ?? null;
  if (!positiveInteger(channelID)) {
    if (diagnosisFollowUp === "auto_room") {
      return {
        detail:
          "Bind an enabled report channel with diagnosis_consultation and diagnosis_close scopes before using automatic diagnosis rooms.",
        label: "Auto-room delivery blocked.",
        missingScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
        requiredScopes,
        status: "blocked",
      };
    }
    return {
      detail: "No notification channel profile is bound.",
      label: "No report channel selected.",
      missingScopes: [],
      requiredScopes,
      status: "pending",
    };
  }

  if (!notificationChannelEnabledIDs.has(channelID)) {
    return {
      detail:
        "Selected notification channel must be enabled before workflow policy enablement.",
      label: "Notification channel disabled.",
      missingScopes: [],
      requiredScopes,
      status: "blocked",
    };
  }

  const channelKind = notificationChannelKindsByID?.get(channelID);
  if (
    diagnosisFollowUp === "auto_room" &&
    channelKind !== undefined &&
    channelKind !== "wecom"
  ) {
    return {
      detail:
        "Automatic diagnosis room delivery requires an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes.",
      label: "Enterprise WeChat channel required.",
      missingScopes: [],
      requiredScopes,
      status: "blocked",
    };
  }

  const missingScopes = [
    reportNotificationChannelIDs.has(channelID) ? "" : "report",
    diagnosisFollowUp === "auto_room" &&
    !diagnosisConsultationNotificationChannelIDs.has(channelID)
      ? "diagnosis_consultation"
      : "",
    diagnosisFollowUp === "auto_room" &&
    !diagnosisCloseNotificationChannelIDs.has(channelID)
      ? "diagnosis_close"
      : "",
  ].filter((scope) => scope !== "");

  if (missingScopes.length > 0) {
    return {
      detail: `Selected notification channel is missing ${missingScopes.join(" and ")} scope.`,
      label: "Notification channel scope mismatch.",
      missingScopes,
      requiredScopes,
      status: "blocked",
    };
  }

  if (
    diagnosisFollowUp === "auto_room" &&
    diagnosisAIProofNotificationChannelIDs !== undefined &&
    !diagnosisAIProofNotificationChannelIDs.has(channelID)
  ) {
    return {
      detail:
        "Open the selected Enterprise WeChat channel and run current AI diagnosis and diagnosis close sample tests before workflow policy enablement.",
      label: "AI delivery proof missing.",
      missingScopes: [],
      requiredScopes,
      status: "blocked",
    };
  }

  if (diagnosisFollowUp === "auto_room") {
    return {
      detail:
        "Selected notification channel can deliver reports, auto-room AI diagnosis updates, and close notifications.",
      label: "Report and auto-room delivery ready.",
      missingScopes: [],
      requiredScopes,
      status: "ready",
    };
  }

  return {
    detail:
      "Selected notification channel can deliver final report notifications.",
    label: "Report delivery ready.",
    missingScopes: [],
    requiredScopes,
    status: "ready",
  };
}

export function preferredReportNotificationChannelIDForFollowUp({
  channels,
  diagnosisAIProofNotificationChannelIDs,
  diagnosisFollowUp,
}: {
  channels: NotificationChannelProfile[];
  diagnosisAIProofNotificationChannelIDs?: ReadonlySet<number>;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
}): number | undefined {
  const enabledReportChannels = channels.filter(
    (channel) => channel.enabled && channel.delivery_scopes.includes("report"),
  );
  switch (diagnosisFollowUp) {
    case "auto_room": {
      const autoRoomChannels = enabledReportChannels.filter(
        (channel) =>
          channel.kind === "wecom" &&
          channel.delivery_scopes.includes("diagnosis_consultation") &&
          channel.delivery_scopes.includes("diagnosis_close"),
      );
      if (diagnosisAIProofNotificationChannelIDs !== undefined) {
        const proofReady = preferWeComNotificationChannel(
          autoRoomChannels.filter((channel) =>
            diagnosisAIProofNotificationChannelIDs.has(channel.id),
          ),
        );
        if (proofReady !== undefined) {
          return proofReady.id;
        }
      }
      return preferWeComNotificationChannel(autoRoomChannels)?.id;
    }
    case "suggest_room":
      return preferWeComNotificationChannel(enabledReportChannels)?.id;
    case "disabled":
      return undefined;
  }
}

export function reportWorkflowNotificationChannelOperatorReadiness({
  channel,
  diagnosisFollowUp,
  readiness,
}: {
  channel: NotificationChannelProfile | null;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
  readiness: NotificationChannelDeliveryReadiness;
}): ReportWorkflowNotificationChannelOperatorReadiness {
  if (channel === null) {
    return {
      detail:
        diagnosisFollowUp === "auto_room"
          ? "Create or select an enabled WeCom channel with report, diagnosis_consultation, and diagnosis_close scopes."
          : "Select a WeCom channel when final reports should notify the operator group through Enterprise WeChat.",
      kindLabel: "No channel",
      label:
        diagnosisFollowUp === "auto_room"
          ? "Enterprise WeChat channel not selected."
          : "Operator channel optional.",
      status: diagnosisFollowUp === "auto_room" ? "blocked" : "pending",
    };
  }

  if (channel.kind === "wecom") {
    return {
      detail:
        readiness.status === "ready"
          ? weComDeliveryDetail(diagnosisFollowUp)
          : readiness.detail,
      kindLabel: "WeCom",
      label:
        readiness.status === "ready"
          ? "Enterprise WeChat delivery selected."
          : "Enterprise WeChat channel needs attention.",
      status: readiness.status,
    };
  }

  if (diagnosisFollowUp === "auto_room") {
    return {
      detail:
        readiness.status === "ready"
          ? "Automatic diagnosis room delivery requires an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes."
          : readiness.detail,
      kindLabel: "Webhook",
      label: "Enterprise WeChat channel required.",
      status: "blocked",
    };
  }

  return {
    detail:
      readiness.status === "ready"
        ? "Generic webhook delivery is supported; select a WeCom channel when operator group notification should land in Enterprise WeChat."
        : readiness.detail,
    kindLabel: "Webhook",
    label: "Generic webhook selected.",
    status: readiness.status === "ready" ? "review" : readiness.status,
  };
}

export function reportWorkflowNotificationChannelOptionState({
  diagnosisAIProofNotificationChannelIDs,
  diagnosisCloseNotificationChannelIDs,
  diagnosisConsultationNotificationChannelIDs,
  diagnosisFollowUp,
  notificationChannelEnabledIDs,
  notificationChannelKindsByID,
  notificationChannelProfileID,
}: {
  diagnosisAIProofNotificationChannelIDs: ReadonlySet<number>;
  diagnosisCloseNotificationChannelIDs: ReadonlySet<number>;
  diagnosisConsultationNotificationChannelIDs: ReadonlySet<number>;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
  notificationChannelEnabledIDs: ReadonlySet<number>;
  notificationChannelKindsByID: ReadonlyMap<
    number,
    NotificationChannelProfile["kind"]
  >;
  notificationChannelProfileID: number;
}): ReportWorkflowNotificationChannelOptionState {
  if (diagnosisFollowUp !== "auto_room") {
    return {
      disabled: false,
      reasons: [],
      reviewReasons: [],
      status: "ready",
    };
  }

  const selectedChannelKind = notificationChannelKindsByID.get(
    notificationChannelProfileID,
  );
  const reasons = [
    !notificationChannelEnabledIDs.has(notificationChannelProfileID)
      ? "disabled"
      : "",
    selectedChannelKind !== "wecom" ? "requires Enterprise WeChat" : "",
    !diagnosisConsultationNotificationChannelIDs.has(
      notificationChannelProfileID,
    )
      ? "missing diagnosis_consultation"
      : "",
    !diagnosisCloseNotificationChannelIDs.has(notificationChannelProfileID)
      ? "missing diagnosis_close"
      : "",
  ].filter((reason) => reason !== "");
  if (reasons.length > 0) {
    return {
      disabled: true,
      reasons,
      reviewReasons: [],
      status: "blocked",
    };
  }

  const reviewReasons = !diagnosisAIProofNotificationChannelIDs.has(
    notificationChannelProfileID,
  )
    ? ["missing AI proof"]
    : [];
  return {
    disabled: false,
    reasons,
    reviewReasons,
    status: reviewReasons.length === 0 ? "ready" : "review",
  };
}

export function diagnosisToolReadinessForSelection({
  alertSourceEnabledIDs,
  alertSourceKindsByID,
  alertSourceLabelsByID,
  alertSourceProfileID,
  diagnosisFollowUp,
  templates,
}: {
  alertSourceEnabledIDs: ReadonlySet<number>;
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>;
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>;
  alertSourceProfileID: number | null;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
  templates: DiagnosisToolTemplate[] | null;
}): DiagnosisToolReadiness {
  if (diagnosisFollowUp === "disabled") {
    return emptyDiagnosisToolReadiness(
      "Diagnosis follow-up disabled.",
      "Enable suggest_room or auto_room to check executable diagnosis tools.",
      "pending",
    );
  }
  if (!positiveInteger(alertSourceProfileID)) {
    return emptyDiagnosisToolReadiness(
      "Select an alert source.",
      "Choose the Alertmanager source that will trigger this workflow.",
      "pending",
    );
  }
  if (templates === null) {
    return emptyDiagnosisToolReadiness(
      "Diagnosis templates unavailable.",
      "Tool template data could not be loaded.",
      "blocked",
    );
  }

  const enabled = templates.filter((template) => template.enabled);
  const usableTemplates = enabled.filter((template) =>
    templateRunsOnReadySource(
      template,
      alertSourceEnabledIDs,
      alertSourceKindsByID,
      alertSourceLabelsByID,
    ),
  );
  const activeAlertsForSource = usableTemplates.filter(
    (template) =>
      template.alert_source_profile_id === alertSourceProfileID &&
      template.tool === "active_alerts",
  );
  const metrics = usableTemplates.filter((template) =>
    isMetricTool(template.tool),
  );
  const rangeMetrics = usableTemplates.filter(
    (template) => template.tool === "metric_range_query",
  );
  const relevantNames = [...activeAlertsForSource, ...metrics].map(
    (template) => template.name,
  );

  if (activeAlertsForSource.length > 0 && metrics.length > 0) {
    return {
      activeAlertsForSource: activeAlertsForSource.length,
      detail: "Active alert and metric collection tools are enabled.",
      enabledMetricTemplates: metrics.length,
      enabledRangeTemplates: rangeMetrics.length,
      enabledTemplates: usableTemplates.length,
      label: "Executable diagnosis tools ready.",
      status: "ready",
      templateNames: uniqueStrings(relevantNames),
    };
  }
  if (enabled.length === 0) {
    return {
      activeAlertsForSource: 0,
      detail:
        "Enable at least one active_alerts template and one metric template before relying on AI follow-up.",
      enabledMetricTemplates: 0,
      enabledRangeTemplates: 0,
      enabledTemplates: 0,
      label: "No enabled diagnosis tools.",
      status: "blocked",
      templateNames: [],
    };
  }
  if (usableTemplates.length === 0) {
    return {
      activeAlertsForSource: 0,
      detail:
        "Enabled diagnosis templates are bound only to disabled or incompatible sources.",
      enabledMetricTemplates: 0,
      enabledRangeTemplates: 0,
      enabledTemplates: 0,
      label: "No usable diagnosis tools.",
      status: "blocked",
      templateNames: [],
    };
  }

  const missing = [
    activeAlertsForSource.length === 0
      ? "active_alerts for the selected source"
      : "",
    metrics.length === 0 ? "metric_query or metric_range_query" : "",
  ].filter((item) => item !== "");
  return {
    activeAlertsForSource: activeAlertsForSource.length,
    detail: `Missing ${missing.join(" and ")}.`,
    enabledMetricTemplates: metrics.length,
    enabledRangeTemplates: rangeMetrics.length,
    enabledTemplates: usableTemplates.length,
    label: "Diagnosis tools need review.",
    status: "review",
    templateNames: uniqueStrings(relevantNames),
  };
}

export function alertSourceIngressReadinessForSelection({
  alertSourceEnabledIDs,
  alertSourceKindsByID,
  alertSourceLabelsByID,
  alertSourceProfileID,
  diagnosisFollowUp,
}: {
  alertSourceEnabledIDs: ReadonlySet<number>;
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>;
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>;
  alertSourceProfileID: number | null;
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
}): AlertSourceIngressReadiness {
  if (diagnosisFollowUp === "disabled") {
    return {
      detail: "Diagnosis follow-up is disabled for this policy.",
      label: "Webhook ingress not used.",
      status: "pending",
    };
  }
  if (!positiveInteger(alertSourceProfileID)) {
    return {
      detail: "Select the alert source that receives the Alertmanager webhook.",
      label: "Alert source required.",
      status: "pending",
    };
  }
  if (!alertSourceEnabledIDs.has(alertSourceProfileID)) {
    return {
      detail:
        "Enable the bound alert source before relying on webhook ingestion.",
      label: "Alert source disabled.",
      status: "blocked",
    };
  }

  const sourceKind = alertSourceKindsByID.get(alertSourceProfileID);
  const sourceLabels = alertSourceLabelsByID?.get(alertSourceProfileID);
  if (sourceKind === "alertmanager") {
    if (diagnosisFollowUp === "auto_room") {
      return {
        detail:
          "Alertmanager webhook deliveries can ingest firing alerts and start automatic diagnosis rooms.",
        label: "Webhook auto-room ingress ready.",
        status: "ready",
      };
    }
    return {
      detail:
        "Alertmanager webhook deliveries can ingest firing alerts; suggest_room still requires operator handoff.",
      label: "Webhook ingest ready.",
      status: "ready",
    };
  }

  if (diagnosisFollowUp === "auto_room") {
    if (
      sourceKind === "prometheus" &&
      workflowAlertSourceLabelsIndicateThanosRule(sourceLabels)
    ) {
      return {
        detail:
          "Thanos Rule active-alert sources can provide firing-alert evidence, but automatic diagnosis room starts require an Alertmanager webhook source. Select or create an Alertmanager source for workflow intake, then keep Thanos Rule for active_alerts evidence templates.",
        label: "Alertmanager webhook source required.",
        status: "blocked",
      };
    }
    return {
      detail:
        "Automatic diagnosis room starts require an Alertmanager alert source because the webhook endpoint rejects non-Alertmanager profiles.",
      label: "Webhook auto-room ingress blocked.",
      status: "blocked",
    };
  }
  return {
    detail:
      "Prometheus sources support metric evidence, but they do not receive Alertmanager webhook deliveries.",
    label: "No Alertmanager webhook ingress.",
    status: "review",
  };
}

function workflowAlertSourceLabelsIndicateThanosRule(
  labels: AlertSourceLabels | null | undefined,
): boolean {
  return (labels?.source ?? "").trim().toLowerCase() === "thanos-rule";
}

export function reportWorkflowPolicyEnablementReadiness({
  alertSourceEnabledIDs,
  alertSourceKindsByID,
  alertSourceLabelsByID,
  diagnosisAIProofNotificationChannelIDs,
  diagnosisConsultationNotificationChannelIDs,
  diagnosisCloseNotificationChannelIDs,
  groupingPolicyEnabledIDs,
  notificationChannelEnabledIDs,
  notificationChannelKindsByID,
  policy,
  reportNotificationChannelIDs,
  templates,
}: {
  alertSourceEnabledIDs: ReadonlySet<number>;
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>;
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>;
  diagnosisAIProofNotificationChannelIDs?: ReadonlySet<number>;
  diagnosisConsultationNotificationChannelIDs: ReadonlySet<number>;
  diagnosisCloseNotificationChannelIDs: ReadonlySet<number>;
  groupingPolicyEnabledIDs: ReadonlySet<number>;
  notificationChannelEnabledIDs: ReadonlySet<number>;
  notificationChannelKindsByID?: ReadonlyMap<
    number,
    NotificationChannelProfile["kind"]
  >;
  policy: ReportWorkflowPolicy;
  reportNotificationChannelIDs: ReadonlySet<number>;
  templates: DiagnosisToolTemplate[] | null;
}): ReportWorkflowPolicyEnablementReadiness {
  const blockers: string[] = [];
  const warnings: string[] = [];
  const delivery = reportNotificationChannelReadinessForSelection({
    diagnosisAIProofNotificationChannelIDs,
    diagnosisConsultationNotificationChannelIDs,
    diagnosisCloseNotificationChannelIDs,
    diagnosisFollowUp: policy.diagnosis_follow_up,
    notificationChannelEnabledIDs,
    notificationChannelKindsByID,
    reportNotificationChannelIDs,
    reportNotificationChannelProfileID:
      policy.report_notification_channel_profile_id ?? undefined,
  });
  const diagnosisTools = diagnosisToolReadinessForSelection({
    alertSourceEnabledIDs,
    alertSourceKindsByID,
    alertSourceLabelsByID,
    alertSourceProfileID: policy.alert_source_profile_id,
    diagnosisFollowUp: policy.diagnosis_follow_up,
    templates,
  });
  const ingress = alertSourceIngressReadinessForSelection({
    alertSourceEnabledIDs,
    alertSourceKindsByID,
    alertSourceLabelsByID,
    alertSourceProfileID: policy.alert_source_profile_id,
    diagnosisFollowUp: policy.diagnosis_follow_up,
  });

  const alertSourceEnabled = alertSourceEnabledIDs.has(
    policy.alert_source_profile_id,
  );
  if (!alertSourceEnabled) {
    blockers.push(
      "Bound alert source must be enabled before workflow policy enablement.",
    );
  }
  if (!groupingPolicyEnabledIDs.has(policy.grouping_policy_id)) {
    blockers.push(
      "Bound grouping policy must be enabled before workflow policy enablement.",
    );
  }
  if (delivery.status === "blocked") {
    blockers.push(delivery.detail);
  } else if (delivery.status === "pending" || delivery.status === "review") {
    warnings.push(delivery.detail);
  }
  if (alertSourceEnabled) {
    if (ingress.status === "blocked") {
      blockers.push(ingress.detail);
    } else if (ingress.status === "review") {
      warnings.push(ingress.detail);
    }
  }
  if (alertSourceEnabled) {
    if (diagnosisTools.status === "blocked") {
      blockers.push(diagnosisTools.detail);
    } else if (diagnosisTools.status === "review") {
      warnings.push(diagnosisTools.detail);
    }
  }

  if (blockers.length > 0) {
    return {
      blockers,
      detail: blockers.join(" "),
      label: "Policy cannot be enabled.",
      status: "blocked",
      warnings,
    };
  }
  if (warnings.length > 0) {
    return {
      blockers,
      detail: warnings.join(" "),
      label: "Policy can be enabled after review.",
      status: "review",
      warnings,
    };
  }
  return {
    blockers,
    detail: "Policy bindings and diagnosis tool configuration are ready.",
    label: "Policy can be enabled.",
    status: "ready",
    warnings,
  };
}

export function reportWorkflowPolicyWorkflowReturnCandidates({
  alertSourceEnabledIDs,
  alertSourceKindsByID,
  alertSourceLabelsByID,
  diagnosisAIProofNotificationChannelIDs,
  diagnosisConsultationNotificationChannelIDs,
  diagnosisCloseNotificationChannelIDs,
  groupingPolicyEnabledIDs,
  launchIntent,
  notificationChannelEnabledIDs,
  notificationChannelKindsByID,
  policies,
  reportNotificationChannelIDs,
  templates,
}: {
  alertSourceEnabledIDs: ReadonlySet<number>;
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>;
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>;
  diagnosisAIProofNotificationChannelIDs?: ReadonlySet<number>;
  diagnosisConsultationNotificationChannelIDs: ReadonlySet<number>;
  diagnosisCloseNotificationChannelIDs: ReadonlySet<number>;
  groupingPolicyEnabledIDs: ReadonlySet<number>;
  launchIntent: ReportWorkflowPolicyLaunchIntent | null;
  notificationChannelEnabledIDs: ReadonlySet<number>;
  notificationChannelKindsByID?: ReadonlyMap<
    number,
    NotificationChannelProfile["kind"]
  >;
  policies: readonly ReportWorkflowPolicy[];
  reportNotificationChannelIDs: ReadonlySet<number>;
  templates: DiagnosisToolTemplate[] | null;
}): ReportWorkflowPolicyWorkflowReturnCandidate[] {
  if (!workflowReturnCandidateIntent(launchIntent)) {
    return [];
  }

  return policies
    .filter((policy) => {
      if (policy.diagnosis_follow_up !== "auto_room") {
        return false;
      }
      if (!positiveInteger(launchIntent.alertSourceProfileID)) {
        return true;
      }
      return policy.alert_source_profile_id === launchIntent.alertSourceProfileID;
    })
    .map((policy) => {
      const readiness = reportWorkflowPolicyEnablementReadiness({
        alertSourceEnabledIDs,
        alertSourceKindsByID,
        alertSourceLabelsByID,
        diagnosisAIProofNotificationChannelIDs,
        diagnosisConsultationNotificationChannelIDs,
        diagnosisCloseNotificationChannelIDs,
        groupingPolicyEnabledIDs,
        notificationChannelEnabledIDs,
        notificationChannelKindsByID,
        policy,
        reportNotificationChannelIDs,
        templates,
      });
      const action = workflowReturnCandidateAction(policy, readiness);
      return {
        action,
        detail: workflowReturnCandidateDetail(action, readiness),
        policy,
        readiness,
      };
    })
    .sort(compareWorkflowReturnCandidates);
}

function workflowReturnCandidateIntent(
  launchIntent: ReportWorkflowPolicyLaunchIntent | null,
): launchIntent is ReportWorkflowPolicyLaunchIntent & {
  diagnosisFollowUp: "auto_room";
  intent: "alertmanager-auto-diagnosis-proof" | "enable-ai-room-follow-up";
} {
  return (
    launchIntent?.diagnosisFollowUp === "auto_room" &&
    (launchIntent.intent === "enable-ai-room-follow-up" ||
      launchIntent.intent === "alertmanager-auto-diagnosis-proof")
  );
}

export function reportWorkflowPolicyDraftPlan({
  alertSourceIngressReadiness,
  alertSourceLabels,
  diagnosisToolReadiness,
  editingPolicyID,
  form,
  groupingPolicyLabels,
  notificationChannelLabels,
  reportNotificationChannelReadiness,
}: {
  alertSourceIngressReadiness: AlertSourceIngressReadiness;
  alertSourceLabels: Readonly<Record<number, string>>;
  diagnosisToolReadiness: DiagnosisToolReadiness;
  editingPolicyID: number | null;
  form: ReportWorkflowPolicyFormState;
  groupingPolicyLabels: Readonly<Record<number, string>>;
  notificationChannelLabels: Readonly<Record<number, string>>;
  reportNotificationChannelReadiness: NotificationChannelDeliveryReadiness;
}): ReportWorkflowPolicyDraftPlan {
  const parsed = formStateToWriteRequest(form);
  const enableBlockers = [
    alertSourceIngressReadiness.status === "blocked"
      ? alertSourceIngressReadiness.detail
      : "",
    reportNotificationChannelReadiness.status === "blocked"
      ? reportNotificationChannelReadiness.detail
      : "",
    diagnosisToolReadiness.status === "blocked"
      ? diagnosisToolReadiness.detail
      : "",
  ].filter((item) => item !== "");
  const reviewItems = [
    alertSourceIngressReadiness.status === "review"
      ? alertSourceIngressReadiness.detail
      : "",
    reportNotificationChannelReadiness.status === "review"
      ? reportNotificationChannelReadiness.detail
      : "",
    diagnosisToolReadiness.status === "review"
      ? diagnosisToolReadiness.detail
      : "",
  ].filter((item) => item !== "");
  const saveStatus: DiagnosisToolReadinessStatus = parsed.ok
    ? "ready"
    : "blocked";
  const saveDetail = !parsed.ok
    ? parsed.message
    : `Saves ${form.name.trim()} for ${labelForID(
        alertSourceLabels,
        form.alertSourceProfileID,
        "alert source",
      )} grouped by ${labelForID(groupingPolicyLabels, form.groupingPolicyID, "grouping policy")}.`;
  const saveBlocked = saveStatus === "blocked";
  const persisted = positiveInteger(editingPolicyID);
  const enableStatus: DiagnosisToolReadinessStatus = saveBlocked
    ? "blocked"
    : enableBlockers.length > 0
      ? "blocked"
      : reviewItems.length > 0
        ? "review"
        : "pending";
  const enableDetail = saveBlocked
    ? "Resolve save blockers before this policy can be enabled."
    : enableBlockers.length > 0
      ? enableBlockers.join(" ")
      : reviewItems.length > 0
        ? `Enable after reviewing: ${reviewItems.join(" ")}`
        : persisted
          ? "Save changes, then enable this policy from the configured policies table."
          : "Save the policy first, then enable it from the configured policies table.";
  const aiHandoffStatus =
    form.diagnosisFollowUp === "disabled"
      ? "pending"
      : aggregateReadiness([
          alertSourceIngressReadiness.status,
          diagnosisToolReadiness.status,
        ]);
  const aiHandoffDetail = aiHandoffDraftDetail(
    form.diagnosisFollowUp,
    alertSourceIngressReadiness,
    diagnosisToolReadiness,
  );
  const steps: ReportWorkflowPolicyDraftPlanStep[] = [
    {
      detail: saveDetail,
      status: saveStatus,
      title: persisted ? `Update policy #${editingPolicyID}` : "Save policy",
    },
    {
      detail: enableDetail,
      status: enableStatus,
      title: "Enable policy",
    },
    {
      detail: saveBlocked
        ? "Impact preview needs valid required workflow fields."
        : "Run draft or saved impact preview to estimate matched alert groups and expose blocked enablement reasons before replay.",
      status: saveBlocked ? "blocked" : "pending",
      title: "Impact preview",
    },
    {
      detail:
        saveBlocked || enableStatus === "blocked"
          ? "Replay is blocked until the policy can be saved and enabled."
        : `Replay bounded ${form.reportScenario} windows after the policy is enabled.`,
      status:
        saveBlocked || enableStatus === "blocked" ? "blocked" : "pending",
      title: "Replay window",
    },
    {
      detail: aiHandoffDetail,
      status: saveBlocked ? "blocked" : aiHandoffStatus,
      title: "AI handoff",
    },
    {
      detail: reportChannelDraftDetail(
        form.reportNotificationChannelProfileID,
        notificationChannelLabels,
      ),
      status: saveBlocked
        ? "blocked"
        : reportNotificationChannelReadiness.status,
      title: "Operator notification",
    },
  ];

  const status = aggregateReadiness(steps.map((step) => step.status));
  return {
    detail: draftPlanDetail(status, steps),
    status,
    steps,
  };
}

function workflowReturnCandidateAction(
  policy: ReportWorkflowPolicy,
  readiness: ReportWorkflowPolicyEnablementReadiness,
): ReportWorkflowPolicyWorkflowReturnCandidateAction {
  if (policy.enabled) {
    return "already_enabled";
  }
  if (readiness.status === "blocked") {
    return "blocked";
  }
  if (readiness.status === "review") {
    return "review";
  }
  return "enable";
}

function workflowReturnCandidateDetail(
  action: ReportWorkflowPolicyWorkflowReturnCandidateAction,
  readiness: ReportWorkflowPolicyEnablementReadiness,
): string {
  switch (action) {
    case "already_enabled":
      return "This matching AI room workflow is already enabled.";
    case "blocked":
      return readiness.detail;
    case "review":
      return `This matching AI room workflow can be enabled after review: ${readiness.detail}`;
    case "enable":
      return "This matching AI room workflow is ready to enable.";
  }
}

function compareWorkflowReturnCandidates(
  left: ReportWorkflowPolicyWorkflowReturnCandidate,
  right: ReportWorkflowPolicyWorkflowReturnCandidate,
): number {
  const rankDiff =
    workflowReturnCandidateRank(left.action) -
    workflowReturnCandidateRank(right.action);
  if (rankDiff !== 0) {
    return rankDiff;
  }
  return left.policy.id - right.policy.id;
}

function workflowReturnCandidateRank(
  action: ReportWorkflowPolicyWorkflowReturnCandidateAction,
): number {
  switch (action) {
    case "enable":
      return 0;
    case "review":
      return 1;
    case "already_enabled":
      return 2;
    case "blocked":
      return 3;
  }
}

export function reportWorkflowPolicyAutomationOutcome({
  alertSourceIngressReadiness,
  alertSourceLabels,
  diagnosisToolReadiness,
  form,
  groupingPolicyLabels,
  notificationChannelLabels,
  reportNotificationChannelReadiness,
}: {
  alertSourceIngressReadiness: AlertSourceIngressReadiness;
  alertSourceLabels: Readonly<Record<number, string>>;
  diagnosisToolReadiness: DiagnosisToolReadiness;
  form: ReportWorkflowPolicyFormState;
  groupingPolicyLabels: Readonly<Record<number, string>>;
  notificationChannelLabels: Readonly<Record<number, string>>;
  reportNotificationChannelReadiness: NotificationChannelDeliveryReadiness;
}): ReportWorkflowPolicyAutomationOutcome {
  const parsed = formStateToWriteRequest(form);
  const sourceLabel = labelForID(
    alertSourceLabels,
    form.alertSourceProfileID,
    "alert source",
  );
  const groupingLabel = labelForID(
    groupingPolicyLabels,
    form.groupingPolicyID,
    "grouping policy",
  );
  const triggerStatus: DiagnosisToolReadinessStatus = parsed.ok
    ? "ready"
    : "blocked";
  const aiEnabled = form.diagnosisFollowUp !== "disabled";
  const aiHandoffStatus = automationRoomStatus(
    form.diagnosisFollowUp,
    alertSourceIngressReadiness.status,
    diagnosisToolReadiness.status,
  );
  const items: ReportWorkflowPolicyAutomationOutcomeItem[] = [
    {
      detail: parsed.ok
        ? `${reportScenarioLabel(form.reportScenario)} reports use ${sourceLabel} and ${groupingLabel}.`
        : parsed.message,
      status: triggerStatus,
      title: "Trigger",
      value: "Manual replay",
    },
    {
      detail: aiEnabled
        ? alertSourceIngressReadiness.detail
        : "This policy does not start AI diagnosis from Alertmanager webhook deliveries.",
      status: aiEnabled ? alertSourceIngressReadiness.status : "pending",
      title: "Alert intake",
      value: alertIntakeOutcomeValue(form.diagnosisFollowUp),
    },
    {
      detail: aiEnabled
        ? diagnosisToolReadiness.detail
        : "Diagnosis evidence tools are not requested while AI follow-up is disabled.",
      status: aiEnabled ? diagnosisToolReadiness.status : "pending",
      title: "Evidence",
      value: aiEnabled ? "Tool collection" : "Report-only",
    },
    {
      detail: automationRoomDetail(
        form.diagnosisFollowUp,
        alertSourceIngressReadiness,
        diagnosisToolReadiness,
      ),
      status: aiHandoffStatus,
      title: "AI room",
      value: automationRoomValue(form.diagnosisFollowUp),
    },
    {
      detail: notificationOutcomeDetail(
        form.diagnosisFollowUp,
        form.reportNotificationChannelProfileID,
        notificationChannelLabels,
        reportNotificationChannelReadiness,
      ),
      status: reportNotificationChannelReadiness.status,
      title: "Notification",
      value: notificationOutcomeValue(
        form.diagnosisFollowUp,
        form.reportNotificationChannelProfileID,
      ),
    },
  ];
  const status = aggregateReadiness(items.map((item) => item.status));

  return {
    detail: automationOutcomeDetail(status, form.diagnosisFollowUp),
    items,
    status,
  };
}

export function reportWorkflowPolicyAutoRoomReadiness({
  alertSourceIngressReadiness,
  diagnosisToolReadiness,
  form,
  operatorChannelReadiness,
  reportNotificationChannelReadiness,
}: {
  alertSourceIngressReadiness: AlertSourceIngressReadiness;
  diagnosisToolReadiness: DiagnosisToolReadiness;
  form: ReportWorkflowPolicyFormState;
  operatorChannelReadiness: ReportWorkflowNotificationChannelOperatorReadiness;
  reportNotificationChannelReadiness: NotificationChannelDeliveryReadiness;
}): ReportWorkflowPolicyAutoRoomReadiness {
  if (form.diagnosisFollowUp !== "auto_room") {
    return {
      detail:
        "Select Auto room to enable automatic AI consultation readiness checks.",
      items: [],
      label: "Automatic diagnosis rooms disabled.",
      status: "pending",
    };
  }

  const items: ReportWorkflowPolicyAutoRoomReadinessItem[] = [
    {
      detail: alertSourceIngressReadiness.detail,
      status: alertSourceIngressReadiness.status,
      title: "Alertmanager intake",
      value: "Webhook firing alerts",
    },
    {
      detail: diagnosisToolReadiness.detail,
      status: diagnosisToolReadiness.status,
      title: "AI evidence",
      value: `${diagnosisToolReadiness.activeAlertsForSource} active alert / ${diagnosisToolReadiness.enabledMetricTemplates} metric`,
    },
    {
      detail: operatorChannelReadiness.detail,
      status: operatorChannelReadiness.status,
      title: "Operator channel",
      value: operatorChannelReadiness.kindLabel,
    },
    {
      detail: reportNotificationChannelReadiness.detail,
      status: reportNotificationChannelReadiness.status,
      title: "Delivery scopes",
      value: reportNotificationChannelReadiness.requiredScopes.join(", "),
    },
  ];
  const status = aggregateReadiness(items.map((item) => item.status));
  return {
    detail: autoRoomReadinessDetail(status),
    items,
    label: autoRoomReadinessLabel(status),
    status,
  };
}

export function reportWorkflowPolicySetupBlueprint({
  alertSourceIngressReadiness,
  alertSourceKindsByID,
  alertSourceLabelsByID,
  diagnosisToolReadiness,
  form,
  reportNotificationChannelReadiness,
}: {
  alertSourceIngressReadiness: AlertSourceIngressReadiness;
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>;
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>;
  diagnosisToolReadiness: DiagnosisToolReadiness;
  form: ReportWorkflowPolicyFormState;
  reportNotificationChannelReadiness: NotificationChannelDeliveryReadiness;
}): ReportWorkflowPolicySetupBlueprint {
  const actions: ReportWorkflowPolicySetupAction[] = [];
  const parsed = formStateToWriteRequest(form);
  const sourceID = positiveInteger(form.alertSourceProfileID)
    ? form.alertSourceProfileID
    : null;
  const sourceKind =
    sourceID === null ? null : (alertSourceKindsByID.get(sourceID) ?? null);
  const sourceLabels =
    sourceID === null ? null : alertSourceLabelsByID?.get(sourceID);
  const aiEnabled = form.diagnosisFollowUp !== "disabled";
  const phases = reportWorkflowPolicySetupPhases({
    alertSourceIngressReadiness,
    diagnosisToolReadiness,
    form,
    parsed,
    reportNotificationChannelReadiness,
  });

  if (
    !positiveInteger(form.alertSourceProfileID) ||
    alertSourceIngressReadiness.status === "blocked"
  ) {
    actions.push({
      actionHref: alertSourceLaunchHref({
        intent:
          form.diagnosisFollowUp === "auto_room"
            ? "alertmanager-source"
            : "prometheus-source",
      }),
      actionLabel: "Configure source",
      detail: alertSourceIngressReadiness.detail,
      key: "alert-source",
      status:
        alertSourceIngressReadiness.status === "blocked"
          ? "blocked"
          : "pending",
      title:
        form.diagnosisFollowUp === "auto_room"
          ? "Alertmanager webhook source"
          : "Alert source profile",
    });
  }

  if (!positiveInteger(form.groupingPolicyID)) {
    actions.push({
      actionHref: groupingPolicyLaunchHref({
        intent: "default-alert-grouping",
      }),
      actionLabel: "Create grouping",
      detail: "Create an enabled grouping policy before saving this workflow.",
      key: "grouping-policy",
      status: parsed.ok ? "ready" : "pending",
      title: "Alert grouping policy",
    });
  }

  if (
    aiEnabled &&
    diagnosisToolReadiness.status !== "ready" &&
    sourceID !== null
  ) {
    if (diagnosisToolReadiness.activeAlertsForSource === 0) {
      actions.push({
        actionHref: diagnosisToolTemplateLaunchHref({
          intent: "active-alert-tool",
          sourceID,
          workflowReturn:
            form.diagnosisFollowUp === "auto_room"
              ? { sourceID }
              : undefined,
        }),
        actionLabel: "Create active-alert tool",
        detail:
          "Add an active_alerts template bound to the workflow alert source so AI can confirm sibling firing alerts.",
        key: "active-alert-tool",
        status: diagnosisToolReadiness.status,
        title: "Active alert evidence tool",
      });
    }
    if (diagnosisToolReadiness.enabledMetricTemplates === 0) {
      actions.push(
        metricEvidenceSetupAction({
          diagnosisToolReadiness,
          sourceLabels,
          sourceID,
          sourceKind,
          workflowReturn:
            form.diagnosisFollowUp === "auto_room"
              ? { sourceID }
              : undefined,
        }),
      );
    }
  }

  if (reportNotificationChannelReadiness.status !== "ready") {
    const selectedChannelID = positiveInteger(
      form.reportNotificationChannelProfileID ?? null,
    )
      ? (form.reportNotificationChannelProfileID ?? null)
      : null;
    const actionCopy = reportNotificationChannelSetupActionCopy({
      diagnosisFollowUp: form.diagnosisFollowUp,
      readiness: reportNotificationChannelReadiness,
      selectedChannelID,
    });
    actions.push({
      actionHref:
        selectedChannelID === null
          ? notificationChannelLaunchHref({
              intent:
                form.diagnosisFollowUp === "auto_room"
                  ? "report-close-channel"
                  : "report-channel",
              workflowReturn:
                form.diagnosisFollowUp === "auto_room"
                  ? { sourceID }
                  : undefined,
            })
          : notificationChannelEditHref(selectedChannelID, {
              workflowReturn:
                form.diagnosisFollowUp === "auto_room"
                  ? { sourceID }
                  : undefined,
            }),
      actionLabel: actionCopy.actionLabel,
      detail: actionCopy.detail,
      key: "notification-channel",
      status: reportNotificationChannelReadiness.status,
      title: actionCopy.title,
    });
  }

  if (!parsed.ok && actions.length === 0) {
    actions.push({
      actionLabel: "Fix form",
      detail: parsed.message,
      key: "form",
      status: "blocked",
      title: "Workflow policy fields",
    });
  }

  const status =
    actions.length === 0
      ? "ready"
      : aggregateReadiness(actions.map((action) => action.status));
  return {
    actions,
    detail: setupBlueprintDetail(status, actions),
    label: setupBlueprintLabel(status),
    phases,
    status,
  };
}

function reportWorkflowPolicySetupPhases({
  alertSourceIngressReadiness,
  diagnosisToolReadiness,
  form,
  parsed,
  reportNotificationChannelReadiness,
}: {
  alertSourceIngressReadiness: AlertSourceIngressReadiness;
  diagnosisToolReadiness: DiagnosisToolReadiness;
  form: ReportWorkflowPolicyFormState;
  parsed: ParseResult<ReportWorkflowPolicyWriteRequest>;
  reportNotificationChannelReadiness: NotificationChannelDeliveryReadiness;
}): ReportWorkflowPolicySetupPhase[] {
  const aiEnabled = form.diagnosisFollowUp !== "disabled";
  const triggerStatus: DiagnosisToolReadinessStatus = parsed.ok
    ? "ready"
    : "blocked";
  const groupingSelected = positiveInteger(form.groupingPolicyID);
  const consultationStatus = automationRoomStatus(
    form.diagnosisFollowUp,
    alertSourceIngressReadiness.status,
    diagnosisToolReadiness.status,
  );

  return [
    {
      detail: aiEnabled
        ? alertSourceIngressReadiness.detail
        : parsed.ok
          ? "Manual replay uses the selected alert source without starting AI follow-up from webhooks."
          : parsed.message,
      key: "alert-intake",
      status: aiEnabled ? alertSourceIngressReadiness.status : triggerStatus,
      title:
        form.diagnosisFollowUp === "auto_room"
          ? "Alertmanager intake"
          : "Alert source",
      value: alertIntakeOutcomeValue(form.diagnosisFollowUp),
    },
    {
      detail: groupingSelected
        ? "Grouping runs before report generation so related alerts share one report and consultation room."
        : "Select or create an enabled grouping policy before saving this workflow.",
      key: "grouping",
      status: groupingSelected ? "ready" : "pending",
      title: "Grouping rule",
      value: groupingSelected ? `Policy #${form.groupingPolicyID}` : "Not selected",
    },
    {
      detail: aiEnabled
        ? diagnosisToolReadiness.detail
        : "Evidence tools are not requested while diagnosis follow-up is disabled.",
      key: "evidence",
      status: aiEnabled ? diagnosisToolReadiness.status : "pending",
      title: "Evidence collection",
      value: aiEnabled
        ? `${diagnosisToolReadiness.activeAlertsForSource} active alert / ${diagnosisToolReadiness.enabledMetricTemplates} metric`
        : "Report-only",
    },
    {
      detail: automationRoomDetail(
        form.diagnosisFollowUp,
        alertSourceIngressReadiness,
        diagnosisToolReadiness,
      ),
      key: "ai-consultation",
      status: consultationStatus,
      title: "AI consultation",
      value: automationRoomValue(form.diagnosisFollowUp),
    },
    {
      detail: notificationOutcomeDetail(
        form.diagnosisFollowUp,
        form.reportNotificationChannelProfileID,
        {},
        reportNotificationChannelReadiness,
      ),
      key: "operator-delivery",
      status: reportNotificationChannelReadiness.status,
      title:
        form.diagnosisFollowUp === "auto_room"
          ? "WeCom delivery and proof"
          : "Operator notification",
      value: notificationOutcomeValue(
        form.diagnosisFollowUp,
        form.reportNotificationChannelProfileID,
      ),
    },
  ];
}

function metricEvidenceSetupAction({
  diagnosisToolReadiness,
  sourceLabels,
  sourceID,
  sourceKind,
  workflowReturn,
}: {
  diagnosisToolReadiness: DiagnosisToolReadiness;
  sourceLabels: AlertSourceLabels | null | undefined;
  sourceID: number;
  sourceKind: AlertSourceKind | null;
  workflowReturn?: { sourceID?: number | null };
}): ReportWorkflowPolicySetupAction {
  if (
    sourceKind === "prometheus" &&
    !workflowAlertSourceLabelsIndicateThanosRule(sourceLabels)
  ) {
    return {
      actionHref: diagnosisToolTemplateLaunchHref({
        intent: "metric-evidence-tool",
        sourceID,
        workflowReturn,
      }),
      actionLabel: "Create metric tool",
      detail:
        "Add a metric_query or metric_range_query template on the selected Prometheus-compatible source so AI can raise confidence with measured evidence.",
      key: "metric-evidence-tool",
      status: diagnosisToolReadiness.status,
      title: "Metric evidence tool",
    };
  }

  return {
    actionHref: alertSourceLaunchHref({
      intent: "thanos-source",
      workflowReturn,
    }),
    actionLabel: "Configure metric source",
    detail:
      "Add a Thanos Query or Prometheus metric evidence source, then use Recommended by sources to create metric_query or metric_range_query templates.",
    key: "metric-evidence-source",
    status: diagnosisToolReadiness.status,
    title: "Metric evidence source",
  };
}

function reportNotificationChannelSetupActionCopy({
  diagnosisFollowUp,
  readiness,
  selectedChannelID,
}: {
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"];
  readiness: NotificationChannelDeliveryReadiness;
  selectedChannelID: number | null;
}): {
  actionLabel: string;
  detail: string;
  title: string;
} {
  if (diagnosisFollowUp !== "auto_room") {
    return {
      actionLabel:
        selectedChannelID === null ? "Configure channel" : "Edit channel",
      detail: readiness.detail,
      title: "Report notification channel",
    };
  }

  if (selectedChannelID === null) {
    return {
      actionLabel: "Create AI channel",
      detail:
        "Create or select an enabled Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes, run AI diagnosis and close proof, then return to enable this workflow.",
      title: "Report and AI-room channel",
    };
  }

  if (readiness.label === "AI delivery proof missing.") {
    return {
      actionLabel: "Run AI proof",
      detail:
        "Open the selected Enterprise WeChat channel, run the current AI diagnosis and diagnosis close sample tests, then return to enable this workflow.",
      title: "AI delivery proof",
    };
  }

  if (readiness.label === "Enterprise WeChat channel required.") {
    return {
      actionLabel: "Switch to WeCom",
      detail:
        "Use an Enterprise WeChat channel for automatic diagnosis room delivery, then run AI diagnosis and close proof before enablement.",
      title: "Enterprise WeChat channel",
    };
  }

  if (readiness.missingScopes.length > 0) {
    return {
      actionLabel: "Add AI scopes",
      detail: `Add ${readiness.missingScopes.join(" and ")} scope, run AI delivery proof, then return to enable this workflow.`,
      title: "Report and AI-room channel",
    };
  }

  return {
    actionLabel:
      readiness.label === "Notification channel disabled."
        ? "Enable channel"
        : "Edit channel",
    detail: readiness.detail,
    title: "Report and AI-room channel",
  };
}

export function reportWorkflowPolicyRepairBlueprint({
  alertSourceEnabledIDs,
  alertSourceKindsByID,
  alertSourceLabelsByID,
  diagnosisAIProofNotificationChannelIDs,
  diagnosisConsultationNotificationChannelIDs,
  diagnosisCloseNotificationChannelIDs,
  groupingPolicyEnabledIDs,
  notificationChannelEnabledIDs,
  notificationChannelKindsByID,
  policy,
  reportNotificationChannelIDs,
  templates,
}: {
  alertSourceEnabledIDs: ReadonlySet<number>;
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>;
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>;
  diagnosisAIProofNotificationChannelIDs?: ReadonlySet<number>;
  diagnosisConsultationNotificationChannelIDs: ReadonlySet<number>;
  diagnosisCloseNotificationChannelIDs: ReadonlySet<number>;
  groupingPolicyEnabledIDs: ReadonlySet<number>;
  notificationChannelEnabledIDs: ReadonlySet<number>;
  notificationChannelKindsByID?: ReadonlyMap<
    number,
    NotificationChannelProfile["kind"]
  >;
  policy: ReportWorkflowPolicy;
  reportNotificationChannelIDs: ReadonlySet<number>;
  templates: DiagnosisToolTemplate[] | null;
}): ReportWorkflowPolicySetupBlueprint {
  const form = policyToFormState(policy);
  const alertSourceIngressReadiness = alertSourceIngressReadinessForSelection({
    alertSourceEnabledIDs,
    alertSourceKindsByID,
    alertSourceLabelsByID,
    alertSourceProfileID: policy.alert_source_profile_id,
    diagnosisFollowUp: policy.diagnosis_follow_up,
  });
  const diagnosisToolReadiness = diagnosisToolReadinessForSelection({
    alertSourceEnabledIDs,
    alertSourceKindsByID,
    alertSourceLabelsByID,
    alertSourceProfileID: policy.alert_source_profile_id,
    diagnosisFollowUp: policy.diagnosis_follow_up,
    templates,
  });
  const reportNotificationChannelReadiness =
    reportNotificationChannelReadinessForSelection({
      diagnosisAIProofNotificationChannelIDs,
      diagnosisConsultationNotificationChannelIDs,
      diagnosisCloseNotificationChannelIDs,
      diagnosisFollowUp: policy.diagnosis_follow_up,
      notificationChannelEnabledIDs,
      notificationChannelKindsByID,
      reportNotificationChannelIDs,
      reportNotificationChannelProfileID:
        policy.report_notification_channel_profile_id ?? undefined,
    });
  const blueprint = reportWorkflowPolicySetupBlueprint({
    alertSourceIngressReadiness,
    alertSourceKindsByID,
    alertSourceLabelsByID,
    diagnosisToolReadiness,
    form,
    reportNotificationChannelReadiness,
  });
  let actions = blueprint.actions;

  if (!alertSourceEnabledIDs.has(policy.alert_source_profile_id)) {
    actions = actions.filter(
      (action) =>
        action.key !== "active-alert-tool" &&
        action.key !== "metric-evidence-source" &&
        action.key !== "metric-evidence-tool",
    );
    actions = replaceSetupAction(actions, {
      actionHref: "/settings/alert-sources",
      actionLabel: "Review source",
      detail:
        "Bound alert source must be enabled before workflow policy enablement.",
      key: "alert-source",
      status: "blocked",
      title: "Bound alert source",
    });
  }
  if (!groupingPolicyEnabledIDs.has(policy.grouping_policy_id)) {
    actions = replaceSetupAction(actions, {
      actionHref: "/settings/grouping-policies",
      actionLabel: "Review grouping",
      detail:
        "Bound grouping policy must be enabled before workflow policy enablement.",
      key: "grouping-policy",
      status: "blocked",
      title: "Bound grouping policy",
    });
  }

  const status =
    actions.length === 0
      ? "ready"
      : aggregateReadiness(actions.map((action) => action.status));
  return {
    actions,
    detail: setupBlueprintDetail(status, actions),
    label: setupBlueprintLabel(status),
    phases: blueprint.phases,
    status,
  };
}

export function reportWorkflowPolicyImpactReason(
  code: ImpactPreviewReasonCode,
): ReportWorkflowPolicyImpactReason {
  return {
    code,
    ...impactReasonDetails[code],
  };
}

export function reportWorkflowPolicyImpactDiagnosisEstimate(
  result: ReportWorkflowPolicyImpactPreviewResult,
): ReportWorkflowPolicyImpactDiagnosisEstimate {
  if (result.diagnosis_follow_up === "disabled") {
    return {
      detail:
        "This policy does not request AI diagnosis handoff for matched alert groups.",
      label: "AI diagnosis disabled.",
      status: "pending",
      value: "Report only",
    };
  }
  if (result.diagnosis_follow_up === "suggest_room") {
    return {
      detail:
        result.groups_estimated > 0
          ? `${result.groups_estimated} estimated alert group${result.groups_estimated === 1 ? "" : "s"} can be retained for operator-created diagnosis rooms.`
          : "No matching alert groups in this sample, so no diagnosis handoff is expected.",
      label: "Operator handoff retained.",
      status: result.groups_estimated > 0 ? "review" : "pending",
      value:
        result.groups_estimated > 0
          ? `${result.groups_estimated} handoff${result.groups_estimated === 1 ? "" : "s"}`
          : "No handoff",
    };
  }
  if (result.status === "blocked") {
    return {
      detail:
        "Automatic diagnosis rooms will not start until the blocked preview reasons are resolved.",
      label: "Automatic diagnosis blocked.",
      status: "blocked",
      value: "Blocked",
    };
  }
  if (result.groups_estimated === 0) {
    return {
      detail:
        "No matching alert groups in this sample, so no automatic diagnosis room is expected.",
      label: "No automatic rooms expected.",
      status: "pending",
      value: "No rooms",
    };
  }
  return {
    detail: `${result.groups_estimated} estimated alert group${result.groups_estimated === 1 ? "" : "s"} can start automatic AI diagnosis rooms when this policy is replayed or receives matching Alertmanager webhooks.`,
    label: "Automatic diagnosis rooms estimated.",
    status: result.status === "review" ? "review" : "ready",
    value: `${result.groups_estimated} room${result.groups_estimated === 1 ? "" : "s"}`,
  };
}

export function reportWorkflowPolicyImpactReportChannelReadiness(
  result: ReportWorkflowPolicyImpactPreviewResult,
): ReportWorkflowPolicyImpactReportChannelReadiness {
  if (
    !result.report_notification_channel_bound ||
    result.report_notification_channel_profile_id === null
  ) {
    return {
      ready: true,
      text: "No report channel bound",
    };
  }
  if (!result.report_notification_channel_enabled) {
    return {
      ready: false,
      text: `#${result.report_notification_channel_profile_id} disabled`,
    };
  }
  const missingScopes =
    reportWorkflowPolicyImpactReportChannelMissingScopes(result);
  if (missingScopes.length > 0) {
    return {
      ready: false,
      text: `#${result.report_notification_channel_profile_id} missing ${missingScopes.join(", ")}`,
    };
  }
  if (
    result.reason_codes.includes(
      "notification_channel_missing_ai_delivery_proof",
    )
  ) {
    return {
      ready: false,
      text: `#${result.report_notification_channel_profile_id} missing AI delivery proof`,
    };
  }
  return {
    ready: true,
    text: `#${result.report_notification_channel_profile_id} scopes and proof ready`,
  };
}

function reportWorkflowPolicyImpactReportChannelMissingScopes(
  result: ReportWorkflowPolicyImpactPreviewResult,
): string[] {
  return [
    result.report_notification_channel_has_report_scope ? "" : "report",
    result.diagnosis_follow_up === "auto_room" &&
    !result.report_notification_channel_has_diagnosis_consultation_scope
      ? "diagnosis_consultation"
      : "",
    result.diagnosis_follow_up === "auto_room" &&
    !result.report_notification_channel_has_diagnosis_close_scope
      ? "diagnosis_close"
      : "",
  ].filter((scope) => scope !== "");
}

export function defaultReportWorkflowPolicyReplayForm(
  now = new Date(),
): ReportWorkflowPolicyReplayFormState {
  const end = new Date(now);
  const start = new Date(end.getTime() - 60 * 60 * 1000);
  return {
    windowStart: isoSeconds(start),
    windowEnd: isoSeconds(end),
    limit: 10000,
    correlationKey: "",
    workflowID: "",
  };
}

export function policyToFormState(
  policy: ReportWorkflowPolicy,
): ReportWorkflowPolicyFormState {
  return {
    name: policy.name,
    alertSourceProfileID: policy.alert_source_profile_id,
    groupingPolicyID: policy.grouping_policy_id,
    reportNotificationChannelProfileID:
      policy.report_notification_channel_profile_id ?? undefined,
    triggerMode: policy.trigger_mode,
    reportScenario: policy.report_scenario,
    diagnosisFollowUp: policy.diagnosis_follow_up,
  };
}

export function formStateToReplayRequest(
  form: ReportWorkflowPolicyReplayFormState,
): ParseResult<ReportWorkflowPolicyReplayRequest> {
  const windowStart = form.windowStart.trim();
  const windowEnd = form.windowEnd.trim();
  if (windowStart === "") {
    return { ok: false, message: "Window start is required." };
  }
  if (windowEnd === "") {
    return { ok: false, message: "Window end is required." };
  }
  const start = Date.parse(windowStart);
  if (!Number.isFinite(start)) {
    return { ok: false, message: "Window start must be a valid date-time." };
  }
  const end = Date.parse(windowEnd);
  if (!Number.isFinite(end)) {
    return { ok: false, message: "Window end must be a valid date-time." };
  }
  if (end <= start) {
    return { ok: false, message: "Window end must be after window start." };
  }
  if (!positiveInteger(form.limit) || form.limit > 100000) {
    return { ok: false, message: "Limit must be between 1 and 100000." };
  }

  const correlationKey = form.correlationKey.trim();
  const workflowID = form.workflowID.trim();
  return {
    ok: true,
    value: {
      window_start: isoSeconds(new Date(start)),
      window_end: isoSeconds(new Date(end)),
      limit: form.limit,
      ...(correlationKey === "" ? {} : { correlation_key: correlationKey }),
      ...(workflowID === "" ? {} : { workflow_id: workflowID }),
    },
  };
}

export function reportWorkflowPolicyReplayProofTrace(
  result: ReportReplayTriggerResponse,
): ReportWorkflowPolicyReplayProofTrace {
  return reportReplayProofTrace(result);
}

export function formStateToWriteRequest(
  form: ReportWorkflowPolicyFormState,
): ParseResult<ReportWorkflowPolicyWriteRequest> {
  const name = form.name.trim();
  if (name === "") {
    return { ok: false, message: "Policy name is required." };
  }
  if (name.length > 120) {
    return {
      ok: false,
      message: "Policy name must be 120 characters or fewer.",
    };
  }
  if (!positiveInteger(form.alertSourceProfileID)) {
    return { ok: false, message: "Select an alert source." };
  }
  if (!positiveInteger(form.groupingPolicyID)) {
    return { ok: false, message: "Select a grouping policy." };
  }
  if (
    form.reportNotificationChannelProfileID !== undefined &&
    !positiveInteger(form.reportNotificationChannelProfileID)
  ) {
    return {
      ok: false,
      message: "Select a valid report notification channel.",
    };
  }
  return {
    ok: true,
    value: {
      name,
      alert_source_profile_id: form.alertSourceProfileID,
      grouping_policy_id: form.groupingPolicyID,
      report_notification_channel_profile_id:
        form.reportNotificationChannelProfileID ?? null,
      trigger_mode: form.triggerMode,
      report_scenario: form.reportScenario,
      diagnosis_follow_up: form.diagnosisFollowUp,
    },
  };
}

export function reportWorkflowPolicyFormMatchesPolicy(
  form: ReportWorkflowPolicyFormState,
  policy: ReportWorkflowPolicy | null,
): boolean {
  if (policy === null) {
    return false;
  }
  const parsed = formStateToWriteRequest(form);
  if (!parsed.ok) {
    return false;
  }
  const request = parsed.value;
  return (
    request.name === policy.name &&
    request.alert_source_profile_id === policy.alert_source_profile_id &&
    request.grouping_policy_id === policy.grouping_policy_id &&
    request.report_notification_channel_profile_id ===
      policy.report_notification_channel_profile_id &&
    request.trigger_mode === policy.trigger_mode &&
    request.report_scenario === policy.report_scenario &&
    request.diagnosis_follow_up === policy.diagnosis_follow_up
  );
}

function positiveInteger(value: number | null): value is number {
  return Number.isSafeInteger(value) && value !== null && value > 0;
}

function preferWeComNotificationChannel(
  channels: NotificationChannelProfile[],
): NotificationChannelProfile | undefined {
  return channels.find((channel) => channel.kind === "wecom") ?? channels[0];
}

function weComDeliveryDetail(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
): string {
  switch (diagnosisFollowUp) {
    case "auto_room":
      return "Enterprise WeChat can receive final report delivery, AI diagnosis updates, final-ready notices, and close notifications.";
    case "suggest_room":
      return "Enterprise WeChat can receive final report delivery while AI room handoff remains operator-controlled.";
    case "disabled":
      return "Enterprise WeChat can receive final report delivery without starting or suggesting AI diagnosis rooms.";
  }
}

function isMetricTool(tool: DiagnosisToolKind): boolean {
  return tool === "metric_query" || tool === "metric_range_query";
}

function templateRunsOnReadySource(
  template: DiagnosisToolTemplate,
  alertSourceEnabledIDs: ReadonlySet<number>,
  alertSourceKindsByID: ReadonlyMap<number, AlertSourceKind>,
  alertSourceLabelsByID?: ReadonlyMap<number, AlertSourceLabels>,
): boolean {
  if (!alertSourceEnabledIDs.has(template.alert_source_profile_id)) {
    return false;
  }
  const sourceKind = alertSourceKindsByID.get(template.alert_source_profile_id);
  const sourceLabels = alertSourceLabelsByID?.get(
    template.alert_source_profile_id,
  );
  if (
    sourceKind === "prometheus" &&
    workflowAlertSourceLabelsIndicateThanosRule(sourceLabels) &&
    template.tool !== "active_alerts"
  ) {
    return false;
  }
  return (
    sourceKind !== undefined &&
    diagnosisToolSupportsSourceKind(template.tool, sourceKind)
  );
}

function emptyDiagnosisToolReadiness(
  label: string,
  detail: string,
  status: DiagnosisToolReadinessStatus,
): DiagnosisToolReadiness {
  return {
    activeAlertsForSource: 0,
    detail,
    enabledMetricTemplates: 0,
    enabledRangeTemplates: 0,
    enabledTemplates: 0,
    label,
    status,
    templateNames: [],
  };
}

function uniqueStrings(values: string[]): string[] {
  return [...new Set(values)];
}

function aggregateReadiness(
  statuses: DiagnosisToolReadinessStatus[],
): DiagnosisToolReadinessStatus {
  if (statuses.includes("blocked")) {
    return "blocked";
  }
  if (statuses.includes("review")) {
    return "review";
  }
  if (statuses.includes("pending")) {
    return "pending";
  }
  return "ready";
}

function replaceSetupAction(
  actions: ReportWorkflowPolicySetupAction[],
  replacement: ReportWorkflowPolicySetupAction,
): ReportWorkflowPolicySetupAction[] {
  const index = actions.findIndex((action) => action.key === replacement.key);
  if (index < 0) {
    return [...actions, replacement];
  }
  return actions.map((action, currentIndex) =>
    currentIndex === index ? replacement : action,
  );
}

function setupBlueprintLabel(status: DiagnosisToolReadinessStatus): string {
  switch (status) {
    case "ready":
      return "Workflow setup ready.";
    case "review":
      return "Workflow setup needs review.";
    case "pending":
      return "Workflow setup pending.";
    case "blocked":
      return "Workflow setup blocked.";
  }
}

function setupBlueprintDetail(
  status: DiagnosisToolReadinessStatus,
  actions: ReportWorkflowPolicySetupAction[],
): string {
  if (actions.length === 0) {
    return "All required bindings are selected; save the policy, run impact preview, then replay a bounded window.";
  }
  const blocked = actions.filter(
    (action) => action.status === "blocked",
  ).length;
  const review = actions.filter((action) => action.status === "review").length;
  const pending = actions.filter(
    (action) => action.status === "pending",
  ).length;
  switch (status) {
    case "blocked":
      return `${blocked} blocking setup action${blocked === 1 ? "" : "s"} must be resolved before this workflow is ready.`;
    case "review":
      return `${review} setup action${review === 1 ? "" : "s"} should be reviewed before enablement.`;
    case "pending":
      return `${pending} setup action${pending === 1 ? "" : "s"} remain before the workflow can be exercised.`;
    case "ready":
      return "Workflow setup actions are ready.";
  }
}

function labelForID(
  labels: Readonly<Record<number, string>>,
  id: number | null | undefined,
  fallbackKind: string,
): string {
  const value = id ?? null;
  if (!positiveInteger(value)) {
    return `unselected ${fallbackKind}`;
  }
  return labels[value] ?? `${fallbackKind} #${value}`;
}

function aiHandoffDraftDetail(
  diagnosisFollowUp: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  alertSourceIngressReadiness: AlertSourceIngressReadiness,
  diagnosisToolReadiness: DiagnosisToolReadiness,
): string {
  switch (diagnosisFollowUp) {
    case "auto_room":
      return `${alertSourceIngressReadiness.detail} ${diagnosisToolReadiness.detail}`;
    case "suggest_room":
      return `AI room will be suggested for operator handoff. ${diagnosisToolReadiness.detail}`;
    case "disabled":
      return "No diagnosis room will be suggested or started by this policy.";
  }
}

function reportChannelDraftDetail(
  reportNotificationChannelProfileID: number | undefined,
  notificationChannelLabels: Readonly<Record<number, string>>,
): string {
  if (!positiveInteger(reportNotificationChannelProfileID ?? null)) {
    return "No report notification channel is bound.";
  }
  return `Deliver report notifications through ${labelForID(
    notificationChannelLabels,
    reportNotificationChannelProfileID,
    "notification channel",
  )}.`;
}

function draftPlanDetail(
  status: DiagnosisToolReadinessStatus,
  steps: ReportWorkflowPolicyDraftPlanStep[],
): string {
  switch (status) {
    case "blocked":
      return (
        steps.find((step) => step.status === "blocked")?.detail ??
        "Resolve blocked workflow configuration."
      );
    case "review":
      return (
        steps.find((step) => step.status === "review")?.detail ??
        "Review workflow warnings before enablement."
      );
    case "pending":
      return "Save the policy, preview impact, then replay a bounded window after enablement.";
    case "ready":
      return "Workflow policy draft is ready for the next operator action.";
  }
}

function reportScenarioLabel(
  scenario: ReportWorkflowPolicyFormState["reportScenario"],
): string {
  switch (scenario) {
    case "single_alert":
      return "Single-alert";
    case "cascade":
      return "Cascade";
    case "alert_storm":
      return "Alert-storm";
  }
}

function alertIntakeOutcomeValue(
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
): string {
  switch (mode) {
    case "auto_room":
      return "Webhook auto-room";
    case "suggest_room":
      return "Webhook handoff";
    case "disabled":
      return "Report-only";
  }
}

function automationRoomStatus(
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  ingressStatus: DiagnosisToolReadinessStatus,
  toolStatus: DiagnosisToolReadinessStatus,
): DiagnosisToolReadinessStatus {
  switch (mode) {
    case "auto_room":
      return aggregateReadiness([ingressStatus, toolStatus]);
    case "suggest_room":
      return "review";
    case "disabled":
      return "pending";
  }
}

function automationRoomValue(
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
): string {
  switch (mode) {
    case "auto_room":
      return "Automatic";
    case "suggest_room":
      return "Operator handoff";
    case "disabled":
      return "Disabled";
  }
}

function automationRoomDetail(
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  alertSourceIngressReadiness: AlertSourceIngressReadiness,
  diagnosisToolReadiness: DiagnosisToolReadiness,
): string {
  switch (mode) {
    case "auto_room":
      return `${alertSourceIngressReadiness.label} ${diagnosisToolReadiness.label}`;
    case "suggest_room":
      return "A handoff is retained for an operator to create the AI diagnosis room.";
    case "disabled":
      return "No AI diagnosis room will be suggested or started by this policy.";
  }
}

function notificationOutcomeValue(
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  reportNotificationChannelProfileID: number | undefined,
): string {
  if (!positiveInteger(reportNotificationChannelProfileID ?? null)) {
    return mode === "auto_room" ? "Channel review" : "No report channel";
  }
  return mode === "auto_room" ? "Report and AI updates" : "Report notification";
}

function notificationOutcomeDetail(
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
  reportNotificationChannelProfileID: number | undefined,
  notificationChannelLabels: Readonly<Record<number, string>>,
  readiness: NotificationChannelDeliveryReadiness,
): string {
  const channelDetail = reportChannelDraftDetail(
    reportNotificationChannelProfileID,
    notificationChannelLabels,
  );
  if (mode === "auto_room") {
    return `${channelDetail} ${readiness.detail}`;
  }
  return readiness.status === "pending"
    ? channelDetail
    : `${channelDetail} ${readiness.detail}`;
}

function automationOutcomeDetail(
  status: DiagnosisToolReadinessStatus,
  mode: ReportWorkflowPolicyFormState["diagnosisFollowUp"],
): string {
  if (status === "blocked") {
    return "Resolve blocked bindings before relying on this workflow automation path.";
  }
  if (status === "review") {
    return "Review the retained handoff or delivery gap before treating this workflow as fully automated.";
  }
  if (mode === "auto_room") {
    return "Alertmanager alerts can produce evidence, start AI diagnosis rooms, and notify operators.";
  }
  if (mode === "suggest_room") {
    return "Alerts can prepare an AI handoff, but an operator still starts the diagnosis room.";
  }
  return "This policy generates report workflow output without AI diagnosis-room automation.";
}

function autoRoomReadinessLabel(status: DiagnosisToolReadinessStatus): string {
  switch (status) {
    case "ready":
      return "Auto-room path ready.";
    case "review":
      return "Auto-room path needs review.";
    case "pending":
      return "Auto-room path pending.";
    case "blocked":
      return "Auto-room path blocked.";
  }
}

function autoRoomReadinessDetail(status: DiagnosisToolReadinessStatus): string {
  switch (status) {
    case "ready":
      return "Matching Alertmanager webhooks can start AI diagnosis rooms, collect evidence, and notify the operator channel.";
    case "review":
      return "The automatic diagnosis path can be saved, but one or more operator-facing choices need review before production use.";
    case "pending":
      return "Complete the required automatic diagnosis selections before enabling this path.";
    case "blocked":
      return "Resolve blocked intake, evidence, or notification requirements before automatic diagnosis rooms can run.";
  }
}

function firstSearchParamValue(value: SearchParamValue): string | null {
  if (Array.isArray(value)) {
    return value[0]?.trim() || null;
  }
  return value?.trim() || null;
}

function positiveSearchParamInteger(value: string | null): number | null {
  if (value === null || !/^[1-9][0-9]*$/.test(value)) {
    return null;
  }
  const parsed = Number(value);
  return positiveInteger(parsed) ? parsed : null;
}

function isoSeconds(value: Date): string {
  return value.toISOString().replace(".000Z", "Z");
}

const impactReasonDetails = {
  ok: {
    detail:
      "Configuration bindings are usable and the bounded sample produced an impact estimate.",
    label: "Preview ready",
    tagColor: "green",
  },
  alert_source_disabled: {
    detail: "Enable the bound alert source before activating this workflow.",
    label: "Alert source disabled",
    tagColor: "red",
  },
  auto_room_requires_alertmanager: {
    detail:
      "Bind an Alertmanager alert source before using auto_room diagnosis follow-up.",
    label: "Alertmanager source required",
    tagColor: "red",
  },
  grouping_policy_disabled: {
    detail:
      "Enable the bound grouping policy so sampled alerts can be grouped.",
    label: "Grouping policy disabled",
    tagColor: "red",
  },
  notification_channel_disabled: {
    detail: "Enable the bound notification channel before report delivery.",
    label: "Notification channel disabled",
    tagColor: "red",
  },
  notification_channel_missing: {
    detail:
      "Bind a notification channel before enabling auto_room AI diagnosis updates.",
    label: "Notification channel required",
    tagColor: "red",
  },
  notification_channel_not_wecom: {
    detail:
      "Use an Enterprise WeChat channel before enabling auto_room AI diagnosis updates.",
    label: "Enterprise WeChat required",
    tagColor: "red",
  },
  notification_channel_missing_report_scope: {
    detail: "Add the report delivery scope to the bound notification channel.",
    label: "Report scope missing",
    tagColor: "red",
  },
  notification_channel_missing_diagnosis_consultation_scope: {
    detail:
      "Add the diagnosis_consultation scope when auto_room should deliver AI diagnosis updates.",
    label: "Diagnosis consultation scope missing",
    tagColor: "red",
  },
  notification_channel_missing_diagnosis_close_scope: {
    detail:
      "Add the diagnosis_close scope when auto_room should deliver close notifications.",
    label: "Diagnosis close scope missing",
    tagColor: "red",
  },
  notification_channel_missing_ai_delivery_proof: {
    detail:
      "Run current AI diagnosis and diagnosis close sample tests for the bound Enterprise WeChat channel.",
    label: "AI delivery proof missing",
    tagColor: "red",
  },
  unsupported_trigger_mode: {
    detail:
      "Use a trigger mode supported by impact preview before enabling this policy.",
    label: "Trigger mode unsupported",
    tagColor: "red",
  },
  no_matching_events: {
    detail:
      "Recent bounded samples did not match this source and grouping configuration.",
    label: "No matching events",
    tagColor: "gold",
  },
} satisfies Record<
  ImpactPreviewReasonCode,
  Omit<ReportWorkflowPolicyImpactReason, "code">
>;
