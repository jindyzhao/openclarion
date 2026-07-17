import type {
  NotificationChannelProfile,
  NotificationChannelTestContentKind,
} from "@/features/settings/notification-channels/types";
import {
  notificationChannelAIProofReadiness,
  notificationChannelEditHref,
  notificationChannelLaunchHref,
  notificationChannelTestProofBundleFromResults,
} from "@/features/settings/notification-channels/format";

export type DiagnosisNotificationChannelSetupAction = {
  href: string;
  kind: "empty" | "load-failed" | "not-ready" | "proof-review";
};

export type DiagnosisNotificationChannelProofSummary = {
  kind:
    | "load-failed"
    | "not-selected"
    | "not-found"
    | "not-ready"
    | "ready"
    | "review";
  missingContentKinds: NotificationChannelTestContentKind[];
  status: "blocked" | "pending" | "ready" | "review";
};

export type DiagnosisNotificationChannelReadinessIssue =
  | "not-wecom"
  | "disabled"
  | "missing-consultation-scope"
  | "missing-close-scope"
  | "endpoint-secret"
  | "missing-secret";

export type DiagnosisNotificationChannelSelectionStatus =
  | "none"
  | "ready"
  | "not-found"
  | "not-ready";

export type DiagnosisNotificationChannelCreateBlocker =
  | "load-failed"
  | "not-found"
  | "not-ready";

export function diagnosisDefaultNotificationChannelProfileID(
  channels: NotificationChannelProfile[],
): number | undefined {
  const readyChannels = channels.filter(
    diagnosisNotificationChannelIsReady,
  );
  if (readyChannels.length === 0) {
    return undefined;
  }
  const preferredChannelID =
    uniqueHighestPreferenceNotificationChannelID(readyChannels);
  if (preferredChannelID !== undefined) {
    return preferredChannelID;
  }
  const [candidate] = readyChannels;
  return readyChannels.length === 1 ? candidate?.id : undefined;
}

export function diagnosisNotificationChannelSetupAction({
  channels,
  failedToLoad = false,
}: {
  channels: NotificationChannelProfile[];
  failedToLoad?: boolean;
}): DiagnosisNotificationChannelSetupAction | null {
  const href = notificationChannelLaunchHref({
    intent: "diagnosis-room-channel",
  });
  if (failedToLoad) {
    return {
      href,
      kind: "load-failed",
    };
  }
  if (channels.some(diagnosisNotificationChannelIsReady)) {
    return null;
  }
  if (channels.length === 0) {
    return {
      href,
      kind: "empty",
    };
  }
  const proofReviewChannel =
    diagnosisNotificationChannelProofReviewChannel(channels);
  if (proofReviewChannel !== null) {
    return {
      href: notificationChannelEditHref(proofReviewChannel.id),
      kind: "proof-review",
    };
  }
  return {
    href,
    kind: "not-ready",
  };
}

export function diagnosisNotificationChannelSelectionStatus(
  channelID: number | null | undefined,
  channels: NotificationChannelProfile[],
): DiagnosisNotificationChannelSelectionStatus {
  if (channelID === null || channelID === undefined) {
    return "none";
  }
  const channel = channels.find((candidate) => candidate.id === channelID);
  if (channel === undefined) {
    return "not-found";
  }
  return diagnosisNotificationChannelIsReady(channel) ? "ready" : "not-ready";
}

export function diagnosisNotificationChannelCreateBlocker({
  channelID,
  channels,
  failedToLoad = false,
}: {
  channelID: number | null | undefined;
  channels: NotificationChannelProfile[];
  failedToLoad?: boolean;
}): DiagnosisNotificationChannelCreateBlocker | null {
  if (channelID === null || channelID === undefined) {
    return null;
  }
  if (failedToLoad) {
    return "load-failed";
  }
  const status = diagnosisNotificationChannelSelectionStatus(channelID, channels);
  return status === "ready" || status === "none" ? null : status;
}

export function diagnosisNotificationChannelProofSummary({
  channelID,
  channels,
  failedToLoad = false,
}: {
  channelID: number | null | undefined;
  channels: NotificationChannelProfile[];
  failedToLoad?: boolean;
}): DiagnosisNotificationChannelProofSummary {
  if (failedToLoad) {
    return {
      kind: "load-failed",
      missingContentKinds: [],
      status: "blocked",
    };
  }
  if (channelID === null || channelID === undefined) {
    return {
      kind: "not-selected",
      missingContentKinds: [],
      status: "pending",
    };
  }
  const channel = channels.find((candidate) => candidate.id === channelID);
  if (channel === undefined) {
    return {
      kind: "not-found",
      missingContentKinds: [],
      status: "blocked",
    };
  }
  if (diagnosisNotificationChannelReadinessIssues(channel).length > 0) {
    return {
      kind: "not-ready",
      missingContentKinds: [],
      status: "blocked",
    };
  }
  const proofBundle = notificationChannelTestProofBundleFromResults(
    channel.latest_test_results,
  );
  const proofReadiness = notificationChannelAIProofReadiness(
    channel,
    proofBundle,
  );
  if (proofReadiness.status === "ready") {
    return {
      kind: "ready",
      missingContentKinds: [],
      status: "ready",
    };
  }
  return {
    kind: proofReadiness.status === "blocked" ? "not-ready" : "review",
    missingContentKinds: proofReadiness.missingContentKinds,
    status: proofReadiness.status === "blocked" ? "blocked" : "review",
  };
}

export function diagnosisNotificationChannelReadinessIssues(
  channel: NotificationChannelProfile,
): DiagnosisNotificationChannelReadinessIssue[] {
  const issues: DiagnosisNotificationChannelReadinessIssue[] = [];
  if (channel.kind !== "wecom") {
    issues.push("not-wecom");
  }
  if (!channel.enabled) {
    issues.push("disabled");
  }
  if (!channel.delivery_scopes.includes("diagnosis_consultation")) {
    issues.push("missing-consultation-scope");
  }
  if (!channel.delivery_scopes.includes("diagnosis_close")) {
    issues.push("missing-close-scope");
  }
  if (secretRefLooksLikeEndpoint(channel.secret_ref)) {
    issues.push("endpoint-secret");
  }
  if (channel.secret_ref.trim() === "") {
    issues.push("missing-secret");
  }
  return issues;
}

function diagnosisNotificationChannelIsReady(
  channel: NotificationChannelProfile,
): boolean {
  if (diagnosisNotificationChannelReadinessIssues(channel).length > 0) {
    return false;
  }
  const proofReadiness = notificationChannelAIProofReadiness(
    channel,
    notificationChannelTestProofBundleFromResults(channel.latest_test_results),
  );
  return proofReadiness.status === "ready";
}

function diagnosisNotificationChannelProofReviewChannel(
  channels: NotificationChannelProfile[],
): NotificationChannelProfile | null {
  const candidates = channels.filter((channel) => {
    if (diagnosisNotificationChannelReadinessIssues(channel).length > 0) {
      return false;
    }
    const proofReadiness = notificationChannelAIProofReadiness(
      channel,
      notificationChannelTestProofBundleFromResults(channel.latest_test_results),
    );
    return (
      proofReadiness.status === "review" &&
      proofReadiness.missingContentKinds.length > 0
    );
  });
  if (candidates.length === 0) {
    return null;
  }
  const preferredChannelID = uniqueHighestPreferenceNotificationChannelID(candidates);
  if (preferredChannelID !== undefined) {
    return (
      candidates.find((channel) => channel.id === preferredChannelID) ?? null
    );
  }
  return candidates[0] ?? null;
}

function uniqueHighestPreferenceNotificationChannelID(
  channels: NotificationChannelProfile[],
): number | undefined {
  let highestScore = 0;
  let candidates: NotificationChannelProfile[] = [];
  for (const channel of channels) {
    const score = diagnosisNotificationChannelPreferenceScore(channel);
    if (score === 0) {
      continue;
    }
    if (score > highestScore) {
      highestScore = score;
      candidates = [channel];
      continue;
    }
    if (score === highestScore) {
      candidates = [...candidates, channel];
    }
  }
  const [candidate] = candidates;
  return candidates.length === 1 ? candidate?.id : undefined;
}

function diagnosisNotificationChannelPreferenceScore(
  channel: NotificationChannelProfile,
): number {
  const role = channel.labels.role?.trim().toLowerCase();
  const scope = channel.labels.scope?.trim().toLowerCase();
  const provider = channel.labels.provider?.trim().toLowerCase();
  let score = 0;
  if (channel.kind === "wecom" || provider === "wecom") {
    score += 1;
  }
  if (role === "ai-room-delivery" || scope === "report-consultation-close") {
    score += 2;
  }
  return score;
}

function secretRefLooksLikeEndpoint(secretRef: string): boolean {
  try {
    return ["http:", "https:", "smtp:", "smtps:"].includes(
      new URL(secretRef).protocol,
    );
  } catch {
    return false;
  }
}
