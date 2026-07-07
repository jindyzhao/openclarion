import type {
  NotificationChannelFormState,
  NotificationChannelProfile,
  NotificationChannelTestResult,
  NotificationChannelTestContentKind,
  NotificationChannelProfileWriteRequest,
  NotificationDeliveryScope,
} from "./types";
import {
  containsControl,
  containsControlOrWhitespace,
  utf8ByteLength,
} from "../validation";

type ParseResult<T> = { ok: true; value: T } | { ok: false; message: string };

type NotificationChannelDeliveryReadinessStatus =
  | "blocked"
  | "ready"
  | "review"
  | "pending";
type NotificationChannelCredentialReadinessStatus =
  | "ready"
  | "pending"
  | "blocked";
type NotificationChannelAIRoomReadinessStatus = "blocked" | "ready" | "review";
type NotificationChannelAIProofReadinessStatus = "blocked" | "ready" | "review";
type NotificationChannelAIProofInventoryReadinessStatus =
  | "blocked"
  | "pending"
  | "ready"
  | "review";
type NotificationChannelEnterpriseWeChatRolloutReadinessStatus =
  | "blocked"
  | "pending"
  | "ready"
  | "review";
type SearchParamValue = string | string[] | undefined;

export type NotificationChannelLaunchIntentName =
  | "diagnosis-room-channel"
  | "report-close-channel"
  | "report-channel";

export type NotificationChannelLaunchIntent = {
  deliveryScopes: NotificationDeliveryScope[];
  labelsText: string;
  message: string;
  name: string;
};

export type NotificationChannelWorkflowReturn = {
  detail: string;
  href: string;
  label: string;
  sourceID: number | null;
};

export type NotificationChannelWorkflowReturnOptions = {
  sourceID?: number | null;
};

export type NotificationChannelDeliveryReadiness = {
  detail: string;
  hasDiagnosisConsultationScope: boolean;
  hasDiagnosisCloseScope: boolean;
  hasReportScope: boolean;
  label: string;
  missingScopes: NotificationDeliveryScope[];
  status: NotificationChannelDeliveryReadinessStatus;
};

export type NotificationChannelCredentialReadiness = {
  detail: string;
  expectedCredential: string;
  kindLabel: string;
  label: string;
  resolverEnvKey: string;
  secretRefExample: string;
  secretConfigured: boolean;
  status: NotificationChannelCredentialReadinessStatus;
};

export type NotificationChannelAIRoomReadiness = {
  detail: string;
  label: string;
  missingScopes: NotificationDeliveryScope[];
  status: NotificationChannelAIRoomReadinessStatus;
  unavailableReason: string;
};

export type NotificationChannelTestProofBundle = Partial<
  Record<NotificationChannelTestContentKind, NotificationChannelTestResult>
>;

export type NotificationChannelAIProofReadiness = {
  detail: string;
  label: string;
  missingContentKinds: NotificationChannelTestContentKind[];
  status: NotificationChannelAIProofReadinessStatus;
};

export type NotificationChannelAIProofInventoryReadiness = {
  blockedChannelIDs: number[];
  candidateChannelIDs: number[];
  detail: string;
  label: string;
  missingContentKinds: NotificationChannelTestContentKind[];
  readyChannelIDs: number[];
  reviewChannelIDs: number[];
  status: NotificationChannelAIProofInventoryReadinessStatus;
};

export type NotificationChannelEnterpriseWeChatRolloutReadiness = {
  blockedChannelIDs: number[];
  candidateChannelIDs: number[];
  detail: string;
  label: string;
  missingContentKinds: NotificationChannelTestContentKind[];
  missingScopes: NotificationDeliveryScope[];
  readyChannelIDs: number[];
  reviewChannelIDs: number[];
  status: NotificationChannelEnterpriseWeChatRolloutReadinessStatus;
};

type NotificationChannelAIProofContentSummaryStatus =
  | "blocked"
  | "current"
  | "failed"
  | "invalid"
  | "missing"
  | "stale"
  | "unsupported";

type NotificationChannelAIProofContentSummary = {
  checkedAt?: string;
  contentKind: NotificationChannelTestContentKind;
  label: string;
  status: NotificationChannelAIProofContentSummaryStatus;
};

export type NotificationChannelAIProofSummary = {
  channelID: number;
  channelName: string;
  contents: NotificationChannelAIProofContentSummary[];
  missingContentKinds: NotificationChannelTestContentKind[];
  status: NotificationChannelAIProofReadiness["status"];
};

export type NotificationChannelTestSample = {
  detail: string;
  label: string;
};

export const notificationChannelSecretResolverEnvKey =
  "OPENCLARION_NOTIFICATION_CHANNEL_SECRET_REFS_JSON";

export function emptyNotificationChannelForm(): NotificationChannelFormState {
  return {
    name: "",
    kind: "webhook",
    secretRef: "",
    deliveryScopes: ["report"],
    enabled: false,
    labelsText: "",
  };
}

export function notificationChannelLaunchHref({
  intent,
  workflowReturn,
}: {
  intent: NotificationChannelLaunchIntentName;
  workflowReturn?: NotificationChannelWorkflowReturnOptions;
}): string {
  const params = new URLSearchParams({ intent });
  appendNotificationChannelWorkflowReturn(params, workflowReturn);
  return `/settings/notification-channels?${params.toString()}`;
}

export function notificationChannelEditHref(
  channelID: number,
  options: { workflowReturn?: NotificationChannelWorkflowReturnOptions } = {},
): string {
  const params = new URLSearchParams({ channel_id: String(channelID) });
  appendNotificationChannelWorkflowReturn(params, options.workflowReturn);
  return `/settings/notification-channels?${params.toString()}`;
}

export function notificationChannelEditIDFromSearchParams(
  searchParams: Record<string, SearchParamValue>,
): number | null {
  return positiveSearchParamInteger(
    firstSearchParamValue(searchParams.channel_id),
  );
}

export function notificationChannelWorkflowReturnFromSearchParams(
  searchParams: Record<string, SearchParamValue>,
): NotificationChannelWorkflowReturn | null {
  if (
    firstSearchParamValue(searchParams.workflow_return) !== "auto-room-enable"
  ) {
    return null;
  }
  const sourceID = positiveSearchParamInteger(
    firstSearchParamValue(searchParams.workflow_source_id),
  );
  const params = new URLSearchParams({
    intent: "enable-ai-room-follow-up",
  });
  if (sourceID !== null) {
    params.set("source_id", String(sourceID));
  }
  return {
    detail:
      "Return to workflow policies after Enterprise WeChat channel scopes and AI delivery proof are ready.",
    href: `/settings/report-workflow-policies?${params.toString()}`,
    label: "Back to workflow",
    sourceID,
  };
}

export function notificationChannelLaunchIntentFromSearchParams(
  searchParams: Record<string, SearchParamValue>,
): NotificationChannelLaunchIntent | null {
  switch (firstSearchParamValue(searchParams.intent)) {
    case "diagnosis-room-channel":
      return {
        deliveryScopes: ["diagnosis_consultation", "diagnosis_close"],
        labelsText:
          "provider=wecom\nrole=ai-room-delivery\nscope=diagnosis-room",
        message:
          "Prepared an Enterprise WeChat channel for AI diagnosis updates and close notifications. Paste the secret reference before saving.",
        name: "AI diagnosis WeCom",
      };
    case "report-close-channel":
      return {
        deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
        labelsText:
          "provider=wecom\nrole=ai-room-delivery\nscope=report-consultation-close",
        message:
          "Prepared an Enterprise WeChat channel for final reports, automatic diagnosis updates, and close notifications. Paste the secret reference before saving.",
        name: "AI report and diagnosis WeCom",
      };
    case "report-channel":
      return {
        deliveryScopes: ["report"],
        labelsText: "provider=wecom\nrole=report-delivery\nscope=report",
        message:
          "Prepared an Enterprise WeChat channel for final report delivery. Paste the secret reference before saving.",
        name: "Report delivery WeCom",
      };
    default:
      return null;
  }
}

function appendNotificationChannelWorkflowReturn(
  params: URLSearchParams,
  workflowReturn: NotificationChannelWorkflowReturnOptions | undefined,
) {
  if (workflowReturn === undefined) {
    return;
  }
  params.set("workflow_return", "auto-room-enable");
  if (
    Number.isSafeInteger(workflowReturn.sourceID) &&
    (workflowReturn.sourceID ?? 0) > 0
  ) {
    params.set("workflow_source_id", String(workflowReturn.sourceID));
  }
}

export function notificationChannelLaunchIntentKey(
  launchIntent: NotificationChannelLaunchIntent | null,
): string {
  if (launchIntent === null) {
    return "default";
  }
  return `${launchIntent.name}:${launchIntent.deliveryScopes.join(",")}`;
}

export function notificationChannelLaunchInitialForm(
  launchIntent: NotificationChannelLaunchIntent | null,
): NotificationChannelFormState {
  if (launchIntent === null) {
    return emptyNotificationChannelForm();
  }
  return {
    ...emptyNotificationChannelForm(),
    deliveryScopes: launchIntent.deliveryScopes,
    enabled: true,
    kind: "wecom",
    labelsText: launchIntent.labelsText,
    name: launchIntent.name,
  };
}

export function channelToFormState(
  channel: NotificationChannelProfile,
): NotificationChannelFormState {
  return {
    name: channel.name,
    kind: channel.kind,
    secretRef: channel.secret_ref,
    deliveryScopes: channel.delivery_scopes,
    enabled: channel.enabled,
    labelsText: labelsToText(channel.labels),
  };
}

export function notificationChannelDeliveryReadiness(
  form: NotificationChannelFormState,
): NotificationChannelDeliveryReadiness {
  const scopes = normalizeDeliveryScopes(form.deliveryScopes);
  const hasReportScope = scopes.includes("report");
  const hasDiagnosisConsultationScope = scopes.includes(
    "diagnosis_consultation",
  );
  const hasDiagnosisCloseScope = scopes.includes("diagnosis_close");
  const missingScopes = [
    hasReportScope ? "" : "report",
    hasDiagnosisConsultationScope ? "" : "diagnosis_consultation",
    hasDiagnosisCloseScope ? "" : "diagnosis_close",
  ].filter((scope): scope is NotificationDeliveryScope => scope !== "");

  if (scopes.length === 0) {
    return {
      detail:
        "Select report for final report delivery, diagnosis_consultation for AI diagnosis updates, and diagnosis_close for close notifications.",
      hasDiagnosisConsultationScope,
      hasDiagnosisCloseScope,
      hasReportScope,
      label: "Delivery scopes not selected.",
      missingScopes,
      status: "pending",
    };
  }
  if (
    form.kind !== "wecom" &&
    (hasDiagnosisConsultationScope || hasDiagnosisCloseScope)
  ) {
    return {
      detail:
        "Diagnosis consultation and close notifications require an Enterprise WeChat channel. Use report scope only for webhook, DingTalk, Feishu, or Slack delivery.",
      hasDiagnosisConsultationScope,
      hasDiagnosisCloseScope,
      hasReportScope,
      label: "Enterprise WeChat required for diagnosis delivery.",
      missingScopes,
      status: "blocked",
    };
  }
  if (
    hasDiagnosisConsultationScope &&
    hasDiagnosisCloseScope &&
    !hasReportScope
  ) {
    return {
      detail:
        "The channel can support AI diagnosis updates and close notifications. Add report scope only when final report delivery should use this channel.",
      hasDiagnosisConsultationScope,
      hasDiagnosisCloseScope,
      hasReportScope,
      label: "Diagnosis delivery scopes ready.",
      missingScopes,
      status: "ready",
    };
  }
  if (missingScopes.length > 0) {
    return {
      detail:
        "The channel can be saved, but workflows using the missing scope will be blocked until it is added.",
      hasDiagnosisConsultationScope,
      hasDiagnosisCloseScope,
      hasReportScope,
      label: "Delivery scopes need review.",
      missingScopes,
      status: "review",
    };
  }

  return {
    detail:
      "The channel can support final reports, auto-room AI diagnosis updates, and close notifications.",
    hasDiagnosisConsultationScope,
    hasDiagnosisCloseScope,
    hasReportScope,
    label: "Delivery scopes ready.",
    missingScopes: [],
    status: "ready",
  };
}

export function notificationChannelCredentialReadiness(
  form: NotificationChannelFormState,
): NotificationChannelCredentialReadiness {
  const secretRef = form.secretRef.trim();
  const secretConfigured = secretRef !== "";
  const kindLabel = notificationChannelKindLabel(form.kind);
  const secretRefExample = notificationChannelSecretRefExample(form.kind);
  const expectedCredential = notificationChannelExpectedCredential(form.kind);
  if (!secretConfigured) {
    return {
      detail: `Select a deployment-managed secret reference before testing the channel, then map it in ${notificationChannelSecretResolverEnvKey}.`,
      expectedCredential,
      kindLabel,
      label: "Credential secret not selected.",
      resolverEnvKey: notificationChannelSecretResolverEnvKey,
      secretRefExample,
      secretConfigured,
      status: "pending",
    };
  }
  if (notificationSecretRefLooksLikeEndpointURL(secretRef)) {
    return {
      detail:
        "Store endpoint URLs in server-side secret storage and enter only the secret reference here.",
      expectedCredential,
      kindLabel,
      label: "Endpoint URL cannot be stored as a secret reference.",
      resolverEnvKey: notificationChannelSecretResolverEnvKey,
      secretRefExample,
      secretConfigured,
      status: "blocked",
    };
  }
  if (form.kind === "wecom") {
    return {
      detail: `Backend tests resolve this secret reference through ${notificationChannelSecretResolverEnvKey} and require one HTTPS Enterprise WeChat robot webhook endpoint.`,
      expectedCredential,
      kindLabel,
      label: "WeCom credential contract selected.",
      resolverEnvKey: notificationChannelSecretResolverEnvKey,
      secretRefExample,
      secretConfigured,
      status: "ready",
    };
  }
  if (form.kind === "dingtalk") {
    return {
      detail: `Backend tests resolve this secret reference through ${notificationChannelSecretResolverEnvKey} and require one DingTalk robot webhook endpoint.`,
      expectedCredential,
      kindLabel,
      label: "DingTalk credential contract selected.",
      resolverEnvKey: notificationChannelSecretResolverEnvKey,
      secretRefExample,
      secretConfigured,
      status: "ready",
    };
  }
  if (form.kind === "feishu") {
    return {
      detail: `Backend tests resolve this secret reference through ${notificationChannelSecretResolverEnvKey} and require one Feishu or Lark custom bot webhook endpoint.`,
      expectedCredential,
      kindLabel,
      label: "Feishu credential contract selected.",
      resolverEnvKey: notificationChannelSecretResolverEnvKey,
      secretRefExample,
      secretConfigured,
      status: "ready",
    };
  }
  if (form.kind === "slack") {
    return {
      detail: `Backend tests resolve this secret reference through ${notificationChannelSecretResolverEnvKey} and require one Slack incoming webhook endpoint.`,
      expectedCredential,
      kindLabel,
      label: "Slack credential contract selected.",
      resolverEnvKey: notificationChannelSecretResolverEnvKey,
      secretRefExample,
      secretConfigured,
      status: "ready",
    };
  }
  return {
    detail: `Backend tests resolve this secret reference through ${notificationChannelSecretResolverEnvKey} and construct a generic webhook provider from the endpoint.`,
    expectedCredential,
    kindLabel,
    label: "Webhook credential contract selected.",
    resolverEnvKey: notificationChannelSecretResolverEnvKey,
    secretRefExample,
    secretConfigured,
    status: "ready",
  };
}

export function notificationChannelAIRoomReadiness(
  channel: NotificationChannelProfile,
): NotificationChannelAIRoomReadiness {
  const form = channelToFormState(channel);
  const credentials = notificationChannelCredentialReadiness(form);
  const delivery = notificationChannelDeliveryReadiness(form);
  const missingScopes = [
    delivery.hasDiagnosisConsultationScope ? "" : "diagnosis_consultation",
    delivery.hasDiagnosisCloseScope ? "" : "diagnosis_close",
  ].filter((scope): scope is NotificationDeliveryScope => scope !== "");

  if (!channel.enabled) {
    return {
      detail:
        "Enable this channel before it can deliver AI diagnosis updates or close notifications.",
      label: "Channel disabled.",
      missingScopes,
      status: "blocked",
      unavailableReason: notificationChannelAIRoomUnavailableReason(channel),
    };
  }
  if (channel.kind !== "wecom") {
    return {
      detail:
        "AI diagnosis updates and close notifications require an Enterprise WeChat channel.",
      label: "Enterprise WeChat required.",
      missingScopes,
      status: "blocked",
      unavailableReason: notificationChannelAIRoomUnavailableReason(channel),
    };
  }
  if (credentials.status !== "ready") {
    return {
      detail: credentials.detail,
      label: credentials.label,
      missingScopes,
      status: credentials.status === "pending" ? "review" : "blocked",
      unavailableReason: notificationChannelAIRoomUnavailableReason(channel),
    };
  }
  if (missingScopes.length > 0) {
    return {
      detail:
        "Add diagnosis_consultation and diagnosis_close scopes before using this channel for AI diagnosis rooms.",
      label: "AI diagnosis scopes need review.",
      missingScopes,
      status: "review",
      unavailableReason: notificationChannelAIRoomUnavailableReason(channel),
    };
  }
  return {
    detail:
      "Ready for AI diagnosis updates and close notifications through Enterprise WeChat.",
    label: "AI diagnosis delivery ready.",
    missingScopes: [],
    status: "ready",
    unavailableReason: "",
  };
}

export function notificationChannelAIRoomUnavailableReason(
  channel: NotificationChannelProfile,
): string {
  const missingScopes = (
    [
      "diagnosis_consultation",
      "diagnosis_close",
    ] as const satisfies readonly NotificationDeliveryScope[]
  ).filter((scope) => !channel.delivery_scopes.includes(scope));
  const reasons = [
    channel.kind === "wecom" ? "" : "not Enterprise WeChat",
    channel.enabled ? "" : "disabled",
    missingScopes.length === 0 ? "" : `missing ${missingScopes.join(" and ")}`,
    notificationSecretRefLooksLikeEndpointURL(channel.secret_ref)
      ? "credential secret reference stores an endpoint URL"
      : "",
    channel.secret_ref.trim() === ""
      ? "missing credential secret reference"
      : "",
  ].filter((reason) => reason !== "");
  return reasons.join("; ");
}

export function notificationChannelAIProofReadiness(
  channel: NotificationChannelProfile,
  proof: NotificationChannelTestProofBundle = {},
): NotificationChannelAIProofReadiness {
  const readiness = notificationChannelAIRoomReadiness(channel);
  const requiredContentKinds =
    notificationChannelAIRequiredTestContentKinds(channel);
  if (readiness.status === "blocked") {
    return {
      detail: readiness.detail,
      label: readiness.label,
      missingContentKinds: requiredContentKinds,
      status: "blocked",
    };
  }
  if (requiredContentKinds.length === 0) {
    return {
      detail:
        "Select diagnosis_consultation and diagnosis_close scopes before collecting AI delivery test proof.",
      label: "AI delivery test proof not applicable.",
      missingContentKinds: [],
      status: "review",
    };
  }
  const missingContentKinds = notificationChannelMissingAIProofContentKinds(
    channel,
    proof,
  );
  if (missingContentKinds.length === 0) {
    return {
      detail:
        "AI diagnosis update and close notification samples both succeeded after the latest channel update.",
      label: "AI delivery test proof ready.",
      missingContentKinds: [],
      status: "ready",
    };
  }
  return {
    detail: `Run ${missingContentKinds.map(notificationChannelTestContentKindLabel).join(" and ")} tests after the latest channel update before relying on this channel for AI diagnosis rooms.`,
    label: "AI delivery test proof needs review.",
    missingContentKinds,
    status: "review",
  };
}

export function notificationChannelAIProofInventoryReadiness(
  channels: NotificationChannelProfile[],
  proofByChannelID: Record<number, NotificationChannelTestProofBundle> = {},
): NotificationChannelAIProofInventoryReadiness {
  const candidateChannels = channels.filter(
    notificationChannelIsAIProofCandidate,
  );
  if (candidateChannels.length === 0) {
    return {
      blockedChannelIDs: [],
      candidateChannelIDs: [],
      detail:
        "Create or enable an Enterprise WeChat channel with diagnosis_consultation and diagnosis_close scopes before enabling automatic diagnosis room delivery.",
      label: "No AI diagnosis delivery channel configured.",
      missingContentKinds: [],
      readyChannelIDs: [],
      reviewChannelIDs: [],
      status: "pending",
    };
  }

  const blockedChannelIDs: number[] = [];
  const missingContentKinds = new Set<NotificationChannelTestContentKind>();
  const readyChannelIDs: number[] = [];
  const reviewChannelIDs: number[] = [];

  for (const channel of candidateChannels) {
    const proof =
      proofByChannelID[channel.id] ??
      notificationChannelTestProofBundleFromResults(
        channel.latest_test_results,
      );
    const readiness = notificationChannelAIProofReadiness(channel, proof);
    if (readiness.status === "ready") {
      readyChannelIDs.push(channel.id);
      continue;
    }
    for (const contentKind of readiness.missingContentKinds) {
      missingContentKinds.add(contentKind);
    }
    if (readiness.status === "blocked") {
      blockedChannelIDs.push(channel.id);
      continue;
    }
    reviewChannelIDs.push(channel.id);
  }

  const candidateChannelIDs = candidateChannels.map((channel) => channel.id);
  if (readyChannelIDs.length > 0) {
    const reviewCount = reviewChannelIDs.length + blockedChannelIDs.length;
    return {
      blockedChannelIDs,
      candidateChannelIDs,
      detail:
        reviewCount === 0
          ? "All AI diagnosis delivery candidates have current AI diagnosis and close notification sample proof."
          : `${readyChannelIDs.length} AI diagnosis delivery channel${readyChannelIDs.length === 1 ? "" : "s"} can be used now; ${reviewCount} candidate channel${reviewCount === 1 ? "" : "s"} still need setup or proof review.`,
      label: `${readyChannelIDs.length} AI delivery channel${readyChannelIDs.length === 1 ? "" : "s"} proof-ready.`,
      missingContentKinds: Array.from(missingContentKinds),
      readyChannelIDs,
      reviewChannelIDs,
      status: reviewCount === 0 ? "ready" : "review",
    };
  }

  if (reviewChannelIDs.length > 0) {
    return {
      blockedChannelIDs,
      candidateChannelIDs,
      detail:
        missingContentKinds.size > 0
          ? "Run current AI diagnosis sample and diagnosis close sample tests before relying on Enterprise WeChat for automatic diagnosis room delivery."
          : "Add diagnosis_consultation and diagnosis_close scopes before collecting AI delivery proof for Enterprise WeChat.",
      label: "AI delivery proof needs review.",
      missingContentKinds: Array.from(missingContentKinds),
      readyChannelIDs,
      reviewChannelIDs,
      status: "review",
    };
  }

  return {
    blockedChannelIDs,
    candidateChannelIDs,
    detail:
      "Configured AI diagnosis delivery candidates are blocked by kind, state, scope, or secret-reference readiness.",
    label: "AI diagnosis delivery channel setup blocked.",
    missingContentKinds: Array.from(missingContentKinds),
    readyChannelIDs,
    reviewChannelIDs,
    status: "blocked",
  };
}

export function notificationChannelEnterpriseWeChatRolloutReadiness(
  channels: NotificationChannelProfile[],
  proofByChannelID: Record<number, NotificationChannelTestProofBundle> = {},
): NotificationChannelEnterpriseWeChatRolloutReadiness {
  const candidateChannels = channels.filter(
    notificationChannelIsEnterpriseWeChatRolloutCandidate,
  );
  if (candidateChannels.length === 0) {
    return {
      blockedChannelIDs: [],
      candidateChannelIDs: [],
      detail:
        "Create or enable an Enterprise WeChat channel with report, diagnosis_consultation, and diagnosis_close scopes before accepting automatic diagnosis rollout.",
      label: "No Enterprise WeChat rollout channel configured.",
      missingContentKinds: [],
      missingScopes: enterpriseWeChatRolloutScopes(),
      readyChannelIDs: [],
      reviewChannelIDs: [],
      status: "pending",
    };
  }

  const blockedChannelIDs: number[] = [];
  const missingContentKinds = new Set<NotificationChannelTestContentKind>();
  const missingScopes = new Set<NotificationDeliveryScope>();
  const readyChannelIDs: number[] = [];
  const reviewChannelIDs: number[] = [];

  for (const channel of candidateChannels) {
    const channelMissingScopes = missingEnterpriseWeChatRolloutScopes(channel);
    const proof =
      proofByChannelID[channel.id] ??
      notificationChannelTestProofBundleFromResults(
        channel.latest_test_results,
      );
    const proofReadiness = notificationChannelAIProofReadiness(channel, proof);

    for (const scope of channelMissingScopes) {
      missingScopes.add(scope);
    }
    for (const contentKind of proofReadiness.missingContentKinds) {
      missingContentKinds.add(contentKind);
    }

    if (
      channel.kind !== "wecom" ||
      !channel.enabled ||
      notificationChannelCredentialReadiness(channelToFormState(channel))
        .status !== "ready"
    ) {
      blockedChannelIDs.push(channel.id);
      continue;
    }
    if (channelMissingScopes.length > 0) {
      reviewChannelIDs.push(channel.id);
      continue;
    }
    if (proofReadiness.status === "ready") {
      readyChannelIDs.push(channel.id);
      continue;
    }
    if (proofReadiness.status === "blocked") {
      blockedChannelIDs.push(channel.id);
      continue;
    }
    reviewChannelIDs.push(channel.id);
  }

  const candidateChannelIDs = candidateChannels.map((channel) => channel.id);
  if (readyChannelIDs.length > 0) {
    const reviewCount = reviewChannelIDs.length + blockedChannelIDs.length;
    return {
      blockedChannelIDs,
      candidateChannelIDs,
      detail:
        reviewCount === 0
          ? "Enterprise WeChat rollout proof is ready for automatic diagnosis: report delivery, AI diagnosis updates, and close notifications are all covered."
          : `${readyChannelIDs.length} Enterprise WeChat rollout channel${readyChannelIDs.length === 1 ? "" : "s"} can be used now; ${reviewCount} candidate channel${reviewCount === 1 ? "" : "s"} still need scope, state, credential, or proof review.`,
      label: `${readyChannelIDs.length} Enterprise WeChat rollout channel${readyChannelIDs.length === 1 ? "" : "s"} ready.`,
      missingContentKinds: Array.from(missingContentKinds),
      missingScopes: Array.from(missingScopes),
      readyChannelIDs,
      reviewChannelIDs,
      status: reviewCount === 0 ? "ready" : "review",
    };
  }

  if (reviewChannelIDs.length > 0) {
    return {
      blockedChannelIDs,
      candidateChannelIDs,
      detail:
        missingScopes.size > 0
          ? "Add report, diagnosis_consultation, and diagnosis_close scopes to one Enterprise WeChat channel before accepting automatic diagnosis rollout."
          : "Run current AI diagnosis sample and diagnosis close sample tests before accepting Enterprise WeChat automatic diagnosis rollout.",
      label: "Enterprise WeChat rollout needs review.",
      missingContentKinds: Array.from(missingContentKinds),
      missingScopes: Array.from(missingScopes),
      readyChannelIDs,
      reviewChannelIDs,
      status: "review",
    };
  }

  return {
    blockedChannelIDs,
    candidateChannelIDs,
    detail:
      "Configured Enterprise WeChat rollout candidates are blocked by kind, state, or credential readiness.",
    label: "Enterprise WeChat rollout channel blocked.",
    missingContentKinds: Array.from(missingContentKinds),
    missingScopes: Array.from(missingScopes),
    readyChannelIDs,
    reviewChannelIDs,
    status: "blocked",
  };
}

export function notificationChannelAIProofRunnableChannelIDs(
  channels: NotificationChannelProfile[],
  proofByChannelID: Record<number, NotificationChannelTestProofBundle> = {},
): number[] {
  return channels
    .filter((channel) => {
      const proof =
        proofByChannelID[channel.id] ??
        notificationChannelTestProofBundleFromResults(
          channel.latest_test_results,
        );
      const readiness = notificationChannelAIProofReadiness(channel, proof);
      return (
        readiness.status === "review" &&
        readiness.missingContentKinds.length > 0
      );
    })
    .map((channel) => channel.id);
}

export function notificationChannelAIProofReadyChannelIDs(
  channels: NotificationChannelProfile[],
  proofByChannelID: Record<number, NotificationChannelTestProofBundle> = {},
): number[] {
  return channels
    .filter((channel) => {
      const proof =
        proofByChannelID[channel.id] ??
        notificationChannelTestProofBundleFromResults(
          channel.latest_test_results,
        );
      return (
        notificationChannelAIProofReadiness(channel, proof).status === "ready"
      );
    })
    .map((channel) => channel.id);
}

export function notificationChannelAIProofSummaries(
  channels: NotificationChannelProfile[],
  proofByChannelID: Record<number, NotificationChannelTestProofBundle> = {},
): NotificationChannelAIProofSummary[] {
  return channels
    .map((channel) => {
      const requiredContentKinds =
        notificationChannelAIRequiredTestContentKinds(channel);
      if (requiredContentKinds.length === 0) {
        return null;
      }
      const proof =
        proofByChannelID[channel.id] ??
        notificationChannelTestProofBundleFromResults(
          channel.latest_test_results,
        );
      const readiness = notificationChannelAIProofReadiness(channel, proof);
      return {
        channelID: channel.id,
        channelName: channel.name,
        contents: requiredContentKinds.map((contentKind) =>
          notificationChannelAIProofContentSummary(channel, proof, contentKind),
        ),
        missingContentKinds: readiness.missingContentKinds,
        status: readiness.status,
      };
    })
    .filter(
      (summary): summary is NotificationChannelAIProofSummary =>
        summary !== null,
    );
}

function notificationChannelAIProofContentSummary(
  channel: NotificationChannelProfile,
  proof: NotificationChannelTestProofBundle,
  contentKind: NotificationChannelTestContentKind,
): NotificationChannelAIProofContentSummary {
  const result = proof[contentKind];
  if (result === undefined) {
    return {
      contentKind,
      label: notificationChannelTestContentKindLabel(contentKind),
      status: "missing",
    };
  }
  const checkedAt =
    result.checked_at.trim() === "" ? undefined : result.checked_at;
  const base = {
    checkedAt,
    contentKind,
    label: notificationChannelTestContentKindLabel(contentKind),
  };
  if (
    result.channel_id !== channel.id ||
    result.kind !== channel.kind ||
    result.content_kind !== contentKind ||
    result.content_sha256 === undefined ||
    !validLowercaseSHA256(result.content_sha256) ||
    Number.isNaN(Date.parse(result.checked_at))
  ) {
    return { ...base, status: "invalid" };
  }
  if (result.status !== "success") {
    return { ...base, status: result.status };
  }
  if (!timestampAtOrAfter(result.checked_at, channel.updated_at)) {
    return { ...base, status: "stale" };
  }
  return { ...base, status: "current" };
}

function notificationChannelIsAIProofCandidate(
  channel: NotificationChannelProfile,
): boolean {
  return (
    channel.kind === "wecom" ||
    channel.delivery_scopes.includes("diagnosis_consultation") ||
    channel.delivery_scopes.includes("diagnosis_close")
  );
}

function notificationChannelIsEnterpriseWeChatRolloutCandidate(
  channel: NotificationChannelProfile,
): boolean {
  return (
    channel.kind === "wecom" ||
    channel.delivery_scopes.includes("diagnosis_consultation") ||
    channel.delivery_scopes.includes("diagnosis_close")
  );
}

function missingEnterpriseWeChatRolloutScopes(
  channel: NotificationChannelProfile,
): NotificationDeliveryScope[] {
  return enterpriseWeChatRolloutScopes().filter(
    (scope) => !channel.delivery_scopes.includes(scope),
  );
}

function enterpriseWeChatRolloutScopes(): NotificationDeliveryScope[] {
  return ["report", "diagnosis_consultation", "diagnosis_close"];
}

export function mergeNotificationChannelTestProofBundle(
  current: NotificationChannelTestProofBundle | undefined,
  result: NotificationChannelTestResult,
): NotificationChannelTestProofBundle {
  if (result.content_kind === undefined) {
    return { ...(current ?? {}) };
  }
  const existing = current?.[result.content_kind];
  if (
    existing !== undefined &&
    !notificationChannelTestResultIsAtLeastAsNew(result, existing)
  ) {
    return { ...(current ?? {}) };
  }
  return {
    ...(current ?? {}),
    [result.content_kind]: result,
  };
}

export function notificationChannelTestProofBundleFromResults(
  results: NotificationChannelTestResult[] | undefined,
): NotificationChannelTestProofBundle {
  let bundle: NotificationChannelTestProofBundle = {};
  for (const result of results ?? []) {
    bundle = mergeNotificationChannelTestProofBundle(bundle, result);
  }
  return bundle;
}

function notificationChannelTestResultIsAtLeastAsNew(
  candidate: NotificationChannelTestResult,
  existing: NotificationChannelTestResult,
): boolean {
  const candidateTime = Date.parse(candidate.checked_at);
  if (Number.isNaN(candidateTime)) {
    return false;
  }
  const existingTime = Date.parse(existing.checked_at);
  if (Number.isNaN(existingTime)) {
    return true;
  }
  return candidateTime >= existingTime;
}

export function notificationChannelMissingAIProofContentKinds(
  channel: NotificationChannelProfile,
  proof: NotificationChannelTestProofBundle = {},
): NotificationChannelTestContentKind[] {
  return notificationChannelAIRequiredTestContentKinds(channel).filter(
    (contentKind) =>
      !notificationChannelTestResultIsCurrentSuccess(
        channel,
        proof[contentKind],
        contentKind,
      ),
  );
}

function notificationChannelAIRequiredTestContentKinds(
  channel: NotificationChannelProfile,
): NotificationChannelTestContentKind[] {
  const required: NotificationChannelTestContentKind[] = [];
  if (channel.delivery_scopes.includes("diagnosis_consultation")) {
    required.push("ai_diagnosis_sample");
  }
  if (channel.delivery_scopes.includes("diagnosis_close")) {
    required.push("diagnosis_close_sample");
  }
  return required;
}

function notificationChannelTestResultIsCurrentSuccess(
  channel: NotificationChannelProfile,
  result: NotificationChannelTestResult | undefined,
  expectedContentKind: NotificationChannelTestContentKind,
): boolean {
  if (
    result === undefined ||
    result.channel_id !== channel.id ||
    result.kind !== channel.kind ||
    result.status !== "success" ||
    result.content_kind !== expectedContentKind ||
    result.content_sha256 === undefined ||
    !validLowercaseSHA256(result.content_sha256)
  ) {
    return false;
  }
  return timestampAtOrAfter(result.checked_at, channel.updated_at);
}

function validLowercaseSHA256(value: string): boolean {
  return /^[a-f0-9]{64}$/.test(value);
}

function timestampAtOrAfter(left: string, right: string): boolean {
  const leftTime = Date.parse(left);
  const rightTime = Date.parse(right);
  if (Number.isNaN(leftTime) || Number.isNaN(rightTime)) {
    return false;
  }
  return leftTime >= rightTime;
}

export function notificationChannelTestSample(
  form: NotificationChannelFormState,
): NotificationChannelTestSample {
  const scopes = normalizeDeliveryScopes(form.deliveryScopes);
  if (scopes.includes("diagnosis_consultation")) {
    return {
      detail:
        "Test sends an AI diagnosis update sample, not a raw Alertmanager alert.",
      label: "AI diagnosis sample",
    };
  }
  if (scopes.includes("diagnosis_close")) {
    return {
      detail: "Test sends a diagnosis room close notification sample.",
      label: "Diagnosis close sample",
    };
  }
  return {
    detail: "Test sends a generic transport notification sample.",
    label: "Transport sample",
  };
}

export function notificationChannelPrimaryTestContentKind(
  channel: NotificationChannelProfile,
  proof: NotificationChannelTestProofBundle | undefined,
): NotificationChannelTestContentKind | undefined {
  const requiredContentKinds =
    notificationChannelAIRequiredTestContentKinds(channel);
  for (const contentKind of requiredContentKinds) {
    if (
      !notificationChannelTestResultIsCurrentSuccess(
        channel,
        proof?.[contentKind],
        contentKind,
      )
    ) {
      return contentKind;
    }
  }
  return undefined;
}

export function notificationChannelTestContentKindLabel(
  kind: NotificationChannelTestResult["content_kind"] | undefined,
): string {
  switch (kind) {
    case "ai_diagnosis_sample":
      return "AI diagnosis sample";
    case "diagnosis_close_sample":
      return "Diagnosis close sample";
    case "transport_sample":
      return "Transport sample";
    case undefined:
      return "Content proof missing";
  }
}

export function formStateToWriteRequest(
  form: NotificationChannelFormState,
): ParseResult<NotificationChannelProfileWriteRequest> {
  const name = form.name.trim();
  if (name === "") {
    return { ok: false, message: "Channel name is required." };
  }
  if (utf8ByteLength(name) > 120) {
    return { ok: false, message: "Channel name must be 120 bytes or fewer." };
  }

  const secretRef = form.secretRef.trim();
  if (secretRef === "") {
    return { ok: false, message: "Secret reference is required." };
  }
  if (utf8ByteLength(secretRef) > 256) {
    return {
      ok: false,
      message: "Secret reference must be 256 bytes or fewer.",
    };
  }
  if (containsControlOrWhitespace(secretRef)) {
    return {
      ok: false,
      message:
        "Secret reference must not contain whitespace or control characters.",
    };
  }
  if (notificationSecretRefLooksLikeEndpointURL(secretRef)) {
    return {
      ok: false,
      message: "Secret reference must not be an endpoint URL.",
    };
  }

  const scopes = normalizeDeliveryScopes(form.deliveryScopes);
  if (scopes.length === 0) {
    return { ok: false, message: "Select at least one delivery scope." };
  }
  if (
    form.kind !== "wecom" &&
    scopes.some(
      (scope) =>
        scope === "diagnosis_consultation" || scope === "diagnosis_close",
    )
  ) {
    return {
      ok: false,
      message:
        "Diagnosis delivery scopes require an Enterprise WeChat channel.",
    };
  }

  const labels = parseLabels(form.labelsText);
  if (!labels.ok) {
    return labels;
  }

  return {
    ok: true,
    value: {
      name,
      kind: form.kind,
      secret_ref: secretRef,
      delivery_scopes: scopes,
      enabled: form.enabled,
      labels: labels.value,
    },
  };
}

function normalizeDeliveryScopes(
  scopes: NotificationDeliveryScope[],
): NotificationDeliveryScope[] {
  return Array.from(new Set(scopes)).sort();
}

function notificationChannelKindLabel(
  kind: NotificationChannelFormState["kind"],
): string {
  switch (kind) {
    case "wecom":
      return "WeCom";
    case "dingtalk":
      return "DingTalk";
    case "feishu":
      return "Feishu";
    case "slack":
      return "Slack";
    case "webhook":
      return "Webhook";
  }
}

function notificationChannelExpectedCredential(
  kind: NotificationChannelFormState["kind"],
): string {
  switch (kind) {
    case "wecom":
      return "Enterprise WeChat robot webhook URL";
    case "dingtalk":
      return "DingTalk robot webhook URL";
    case "feishu":
      return "Feishu or Lark custom bot webhook URL";
    case "slack":
      return "Slack incoming webhook URL";
    case "webhook":
      return "HTTP webhook URL";
  }
}

function notificationChannelSecretRefExample(
  kind: NotificationChannelFormState["kind"],
): string {
  switch (kind) {
    case "wecom":
      return "secret/openclarion/ops-wecom";
    case "dingtalk":
      return "secret/openclarion/ops-dingtalk";
    case "feishu":
      return "secret/openclarion/ops-feishu";
    case "slack":
      return "secret/openclarion/ops-slack";
    case "webhook":
      return "secret/openclarion/ops-webhook";
  }
}

function notificationSecretRefLooksLikeEndpointURL(secretRef: string): boolean {
  try {
    const parsed = new URL(secretRef);
    return parsed.protocol === "http:" || parsed.protocol === "https:";
  } catch {
    return false;
  }
}

function parseLabels(text: string): ParseResult<Record<string, string>> {
  const trimmed = text.trim();
  if (trimmed === "") {
    return { ok: true, value: {} };
  }
  const labels: Record<string, string> = {};
  for (const rawLine of trimmed.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (line === "") {
      continue;
    }
    const eq = line.indexOf("=");
    if (eq <= 0) {
      return { ok: false, message: "Labels must use key=value lines." };
    }
    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim();
    if (key === "") {
      return { ok: false, message: "Label keys must be non-empty." };
    }
    if (Object.hasOwn(labels, key)) {
      return { ok: false, message: `Label key "${key}" is duplicated.` };
    }
    if (Object.keys(labels).length >= 32) {
      return { ok: false, message: "Labels must contain 32 entries or fewer." };
    }
    if (utf8ByteLength(key) > 64 || utf8ByteLength(value) > 128) {
      return { ok: false, message: "Labels exceed the allowed length." };
    }
    if (containsControl(key) || containsControl(value)) {
      return {
        ok: false,
        message: "Labels must not contain control characters.",
      };
    }
    labels[key] = value;
  }
  return { ok: true, value: labels };
}

function labelsToText(labels: Record<string, string>): string {
  return Object.entries(labels)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
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
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : null;
}
