import type { useTranslations } from "next-intl";

import {
  notificationChannelAIProofReadiness,
  notificationChannelTestProofBundleFromResults,
} from "@/features/settings/notification-channels/format";
import type {
  NotificationChannelProfile,
  NotificationChannelTestContentKind,
} from "@/features/settings/notification-channels/types";
import { formatDateTime } from "@/features/settings/format";

import {
  diagnosisNotificationChannelCreateBlocker,
  diagnosisNotificationChannelProofSummary,
  diagnosisNotificationChannelReadinessIssues,
  diagnosisNotificationChannelSelectionStatus,
  diagnosisNotificationChannelSetupAction,
  type DiagnosisNotificationChannelProofSummary as DiagnosisNotificationChannelProofState,
} from "./notification-channel-options";

export type DiagnosisNotificationChannelTranslator = ReturnType<
  typeof useTranslations<"DiagnosisRoom.notificationChannel">
>;

type DiagnosisNotificationChannelOption = {
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
  status: DiagnosisNotificationChannelProofState["status"];
};

export function localizeDiagnosisNotificationChannelOptions(
  channels: NotificationChannelProfile[],
  locale: string,
  t: DiagnosisNotificationChannelTranslator,
): DiagnosisNotificationChannelOption[] {
  const collator = new Intl.Collator(locale);
  return channels
    .map((channel) => {
      const unavailableReason = localizeNotificationChannelUnavailableReason(
        channel,
        t,
      );
      const disabled = unavailableReason !== "";
      return {
        disabled: disabled ? true : undefined,
        label: disabled
          ? t("optionUnavailable", {
              id: channel.id,
              name: channel.name,
              reason: unavailableReason,
            })
          : t("optionReady", { id: channel.id, name: channel.name }),
        title: disabled
          ? t("optionUnavailableTitle", {
              name: channel.name,
              reason: unavailableReason,
            })
          : t("optionReadyTitle", { name: channel.name }),
        value: channel.id,
      };
    })
    .sort((left, right) => {
      const leftDisabled = left.disabled === true ? 1 : 0;
      const rightDisabled = right.disabled === true ? 1 : 0;
      return leftDisabled === rightDisabled
        ? collator.compare(left.label, right.label)
        : leftDisabled - rightDisabled;
    });
}

export function localizeDiagnosisNotificationChannelSetupAction(
  input: {
    channels: NotificationChannelProfile[];
    failedToLoad?: boolean;
  },
  t: DiagnosisNotificationChannelTranslator,
): DiagnosisNotificationChannelSetupAction | null {
  const action = diagnosisNotificationChannelSetupAction(input);
  if (action === null) {
    return null;
  }
  switch (action.kind) {
    case "load-failed":
      return {
        href: action.href,
        detail: t("setupLoadFailedDetail"),
        label: t("setupOpen"),
      };
    case "empty":
      return {
        href: action.href,
        detail: t("setupEmptyDetail"),
        label: t("setupCreate"),
      };
    case "proof-review":
      return {
        href: action.href,
        detail: t("setupProofDetail"),
        label: t("setupProof"),
      };
    case "not-ready":
      return {
        href: action.href,
        detail: t("setupPrepareDetail"),
        label: t("setupPrepare"),
      };
  }
}

export function localizeDiagnosisNotificationChannelSelectionError(
  channelID: number | null | undefined,
  channels: NotificationChannelProfile[],
  t: DiagnosisNotificationChannelTranslator,
): string {
  const status = diagnosisNotificationChannelSelectionStatus(
    channelID,
    channels,
  );
  switch (status) {
    case "none":
    case "ready":
      return "";
    case "not-found":
      return t("selectionNotFound");
    case "not-ready": {
      const channel = channels.find((candidate) => candidate.id === channelID);
      return t("selectionNotReady", {
        reason:
          channel === undefined
            ? t("reasonProofReview")
            : localizeNotificationChannelUnavailableReason(channel, t),
      });
    }
  }
}

export function localizeDiagnosisNotificationChannelCreateBlockReason(
  input: {
    channelID: number | null | undefined;
    channels: NotificationChannelProfile[];
    failedToLoad?: boolean;
  },
  t: DiagnosisNotificationChannelTranslator,
): string {
  const blocker = diagnosisNotificationChannelCreateBlocker(input);
  switch (blocker) {
    case null:
      return "";
    case "load-failed":
      return t("createLoadFirst");
    case "not-found":
    case "not-ready":
      return localizeDiagnosisNotificationChannelSelectionError(
        input.channelID,
        input.channels,
        t,
      );
  }
}

export function localizeDiagnosisNotificationChannelProofSummary(
  input: {
    channelID: number | null | undefined;
    channels: NotificationChannelProfile[];
    failedToLoad?: boolean;
  },
  locale: string,
  t: DiagnosisNotificationChannelTranslator,
): DiagnosisNotificationChannelProofSummary {
  const summary = diagnosisNotificationChannelProofSummary(input);
  switch (summary.kind) {
    case "load-failed":
      return {
        detail: t("proofLoadFailedDetail"),
        label: t("proofUnavailable"),
        status: summary.status,
      };
    case "not-selected":
      return {
        detail: t("proofPendingDetail"),
        label: t("proofPending"),
        status: summary.status,
      };
    case "not-found":
      return {
        detail: t("selectionNotFound"),
        label: t("proofUnavailable"),
        status: summary.status,
      };
    case "not-ready": {
      const channel = input.channels.find(
        (candidate) => candidate.id === input.channelID,
      );
      return {
        detail:
          channel === undefined
            ? t("reasonProofReview")
            : localizeNotificationChannelUnavailableReason(channel, t),
        label: t("proofChannelNotReady"),
        status: summary.status,
      };
    }
    case "ready":
      return {
        detail: t("proofReadyDetail", {
          aiDiagnosisCheckedAt: formatDateTime(
            summary.proof.aiDiagnosisCheckedAt,
            locale,
          ),
          diagnosisCloseCheckedAt: formatDateTime(
            summary.proof.diagnosisCloseCheckedAt,
            locale,
          ),
        }),
        label: t("proofReady"),
        status: summary.status,
      };
    case "review":
      return {
        detail: t("proofReviewDetail", {
          kinds: summary.missingContentKinds
            .map((kind) => localizeProofKind(kind, t))
            .join(" / "),
        }),
        label: t("proofReview"),
        status: summary.status,
      };
  }
}

function localizeNotificationChannelUnavailableReason(
  channel: NotificationChannelProfile,
  t: DiagnosisNotificationChannelTranslator,
): string {
  const roomReason = localizeNotificationChannelRoomUnavailableReason(channel, t);
  if (roomReason !== "") {
    return roomReason;
  }
  const readiness = notificationChannelAIProofReadiness(
    channel,
    notificationChannelTestProofBundleFromResults(channel.latest_test_results),
  );
  if (readiness.status === "ready") {
    return "";
  }
  if (readiness.missingContentKinds.length > 0) {
    return t("reasonMissingProof", {
      kinds: readiness.missingContentKinds
        .map((kind) => localizeProofKind(kind, t))
        .join(" / "),
    });
  }
  return t("reasonProofReview");
}

function localizeNotificationChannelRoomUnavailableReason(
  channel: NotificationChannelProfile,
  t: DiagnosisNotificationChannelTranslator,
): string {
  return diagnosisNotificationChannelReadinessIssues(channel)
    .map((issue) => {
      switch (issue) {
        case "not-wecom":
          return t("reasonNotWeCom");
        case "disabled":
          return t("reasonDisabled");
        case "missing-consultation-scope":
          return t("reasonMissingConsultationScope");
        case "missing-close-scope":
          return t("reasonMissingCloseScope");
        case "endpoint-secret":
          return t("reasonEndpointSecret");
        case "missing-secret":
          return t("reasonMissingSecret");
      }
    })
    .join("; ");
}

function localizeProofKind(
  kind: NotificationChannelTestContentKind,
  t: DiagnosisNotificationChannelTranslator,
): string {
  switch (kind) {
    case "ai_diagnosis_sample":
      return t("proofKindDiagnosis");
    case "diagnosis_close_sample":
      return t("proofKindClose");
    case "transport_sample":
      return t("proofKindTransport");
  }
}
