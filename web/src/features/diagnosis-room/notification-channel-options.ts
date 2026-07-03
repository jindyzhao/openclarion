import type {
  NotificationChannelProfile,
} from "@/features/settings/notification-channels/types";
import {
  notificationChannelAIProofReadiness,
  notificationChannelAIRoomUnavailableReason,
  notificationChannelEditHref,
  notificationChannelLaunchHref,
  notificationChannelTestContentKindLabel,
  notificationChannelTestProofBundleFromResults,
} from "@/features/settings/notification-channels/format";

export type DiagnosisNotificationChannelOption = {
  disabled?: boolean;
  label: string;
  title: string;
  value: number;
};

export type DiagnosisNotificationChannelSetupAction = {
  detail: string;
  href: string;
  label: string;
};

export type DiagnosisNotificationChannelProofSummary = {
  detail: string;
  label: string;
  status: "blocked" | "pending" | "ready" | "review";
};

export function diagnosisNotificationChannelOptions(
  channels: NotificationChannelProfile[],
): DiagnosisNotificationChannelOption[] {
  return channels
    .map((channel) => {
      const unavailableReason =
        diagnosisNotificationChannelUnavailableReason(channel);
      const disabled = unavailableReason !== "";
      return {
        disabled: disabled ? true : undefined,
        label: disabled
          ? `#${channel.id} ${channel.name} (${unavailableReason})`
          : `#${channel.id} ${channel.name}`,
        title: disabled
          ? `${channel.name}: ${unavailableReason}`
          : `${channel.name}: ready for diagnosis room notifications`,
        value: channel.id,
      };
    })
    .sort((left, right) => {
      const leftDisabled = left.disabled === true ? 1 : 0;
      const rightDisabled = right.disabled === true ? 1 : 0;
      if (leftDisabled !== rightDisabled) {
        return leftDisabled - rightDisabled;
      }
      return left.label.localeCompare(right.label);
    });
}

export function diagnosisDefaultNotificationChannelProfileID(
  channels: NotificationChannelProfile[],
): number | undefined {
  const readyChannels = channels.filter(
    (channel) => diagnosisNotificationChannelUnavailableReason(channel) === "",
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
      detail:
        "Notification channels could not be loaded. Open the Enterprise WeChat preset to create or review a channel with diagnosis update and close scopes.",
      href,
      label: "Open WeCom channel setup",
    };
  }
  if (
    channels.some(
      (channel) => diagnosisNotificationChannelUnavailableReason(channel) === "",
    )
  ) {
    return null;
  }
  if (channels.length === 0) {
    return {
      detail:
        "Create an Enterprise WeChat channel before relying on AI diagnosis room updates and close notifications.",
      href,
      label: "Create WeCom channel",
    };
  }
  const proofReviewChannel =
    diagnosisNotificationChannelProofReviewChannel(channels);
  if (proofReviewChannel !== null) {
    return {
      detail:
        "The selected Enterprise WeChat channel has the required diagnosis scopes. Open it and run the missing AI diagnosis and close notification proof tests.",
      href: notificationChannelEditHref(proofReviewChannel.id),
      label: "Run WeCom AI proof",
    };
  }
  return {
    detail:
      "No configured channel is ready for both AI diagnosis updates and close notifications. Use the Enterprise WeChat preset to add the required scopes.",
    href,
    label: "Prepare WeCom channel",
  };
}

export function diagnosisNotificationChannelSelectionError(
  channelID: number | null | undefined,
  channels: NotificationChannelProfile[],
): string {
  if (channelID === null || channelID === undefined) {
    return "";
  }
  const channel = channels.find((candidate) => candidate.id === channelID);
  if (channel === undefined) {
    return "Selected notification channel was not found.";
  }
  const unavailableReason =
    diagnosisNotificationChannelUnavailableReason(channel);
  if (unavailableReason === "") {
    return "";
  }
  return `Selected notification channel is not ready: ${unavailableReason}.`;
}

export function diagnosisNotificationChannelCreateBlockReason({
  channelID,
  channels,
  failedToLoad = false,
}: {
  channelID: number | null | undefined;
  channels: NotificationChannelProfile[];
  failedToLoad?: boolean;
}): string {
  if (channelID === null || channelID === undefined) {
    return "";
  }
  if (failedToLoad) {
    return "Load notification channels before creating a diagnosis room with Enterprise WeChat delivery.";
  }
  return diagnosisNotificationChannelSelectionError(channelID, channels);
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
      detail:
        "Notification channels could not be loaded, so Enterprise WeChat proof cannot be checked.",
      label: "Channel proof unavailable.",
      status: "blocked",
    };
  }
  if (channelID === null || channelID === undefined) {
    return {
      detail:
        "Select an Enterprise WeChat channel with current AI diagnosis and close sample proof.",
      label: "Channel proof pending.",
      status: "pending",
    };
  }
  const channel = channels.find((candidate) => candidate.id === channelID);
  if (channel === undefined) {
    return {
      detail: "Selected notification channel was not found.",
      label: "Channel proof unavailable.",
      status: "blocked",
    };
  }
  const roomReason = notificationChannelAIRoomUnavailableReason(channel);
  if (roomReason !== "") {
    return {
      detail: roomReason,
      label: "Channel is not ready.",
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
      detail: diagnosisNotificationChannelProofReadyDetail(proofBundle),
      label: "Channel proof ready.",
      status: "ready",
    };
  }
  return {
    detail: proofReadiness.detail,
    label: proofReadiness.label,
    status: proofReadiness.status,
  };
}

function diagnosisNotificationChannelUnavailableReason(
  channel: NotificationChannelProfile,
): string {
  const roomReason = notificationChannelAIRoomUnavailableReason(channel);
  if (roomReason !== "") {
    return roomReason;
  }
  const proofReadiness = notificationChannelAIProofReadiness(
    channel,
    notificationChannelTestProofBundleFromResults(channel.latest_test_results),
  );
  if (proofReadiness.status === "ready") {
    return "";
  }
  if (proofReadiness.missingContentKinds.length > 0) {
    return `missing current ${proofReadiness.missingContentKinds
      .map((kind) => notificationChannelTestContentKindLabel(kind).toLowerCase())
      .join(" and ")} proof`;
  }
  return proofReadiness.label;
}

function diagnosisNotificationChannelProofReviewChannel(
  channels: NotificationChannelProfile[],
): NotificationChannelProfile | null {
  const candidates = channels.filter((channel) => {
    if (notificationChannelAIRoomUnavailableReason(channel) !== "") {
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

function diagnosisNotificationChannelProofReadyDetail(
  proof: ReturnType<typeof notificationChannelTestProofBundleFromResults>,
): string {
  const parts = [
    proof.ai_diagnosis_sample === undefined
      ? ""
      : `AI diagnosis sample checked at ${proof.ai_diagnosis_sample.checked_at}`,
    proof.diagnosis_close_sample === undefined
      ? ""
      : `Diagnosis close sample checked at ${proof.diagnosis_close_sample.checked_at}`,
  ].filter((part) => part !== "");
  if (parts.length === 0) {
    return "Selected Enterprise WeChat channel has current AI delivery test proof.";
  }
  return parts.join("; ");
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
