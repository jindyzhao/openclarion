"use client";

import {
  ApiOutlined,
  BulbOutlined,
  CheckCircleOutlined,
  DisconnectOutlined,
  FormOutlined,
  LoginOutlined,
  SafetyCertificateOutlined,
  PlayCircleOutlined,
  PlusCircleOutlined,
  ReloadOutlined,
  SendOutlined,
  WechatOutlined,
} from "@ant-design/icons";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
  Progress,
  Segmented,
  Select,
  Space,
  Tag,
  Timeline,
  Tooltip,
  Typography,
} from "antd";
import type { DescriptionsProps, FormInstance, TimelineProps } from "antd";
import type { Route } from "next";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";

import { formatDateTime } from "@/features/reports/format";
import { refreshDiagnosisToolTemplates } from "@/features/settings/diagnosis-tool-templates/client-api";
import type {
  DiagnosisToolKind,
  DiagnosisToolTemplate,
  DiagnosisToolTemplateListResponse,
} from "@/features/settings/diagnosis-tool-templates/types";
import { directoryUserLabel } from "@/features/settings/directory-rbac/format";
import {
  useCurrentRBACAuthorizations,
  type CurrentRBACAuthorizations,
} from "@/features/settings/rbac-capabilities";
import { refreshNotificationChannelProfiles } from "@/features/settings/notification-channels/client-api";
import { notificationChannelEditHref } from "@/features/settings/notification-channels/format";
import type { NotificationChannelProfileListResponse } from "@/features/settings/notification-channels/types";

import type {
  AlertEventSummary,
  AlertEvidenceSnapshotLink,
} from "@/features/alerts/api";
import type { ApiResult } from "@/lib/api/client";
import { useClientReady } from "@/lib/react/use-client-ready";
import {
  normalizedDiagnosisAuthorization,
  type DiagnosisAuthorization,
} from "./authorization";
import {
  diagnosisAuthActionBlockReason,
  diagnosisAuthBackendCredentialListLabel,
  diagnosisAuthBackendModeDisplayItems,
  diagnosisAuthBackendModeListLabel,
  diagnosisAuthBackendReadiness,
  diagnosisAuthBackendStatusModes,
  diagnosisAuthWeComQuickSignInPrompt,
  diagnosisAuthCheckSuccessFeedback,
  diagnosisAuthLDAPBrowserSessionPromotionNotice,
  diagnosisAuthBrowserSessionDisplaySummary,
  diagnosisAuthBrowserSessionBlockReason,
  diagnosisAuthBrowserSessionShouldClearAfterError,
  diagnosisAuthCheckBlockReason,
  diagnosisAuthCoercedMode,
  diagnosisAutoBrowserSessionConnectionPlan,
  diagnosisAutoBrowserSessionAuthCheckPlan,
  diagnosisAuthInputFieldsChanged,
  diagnosisAuthInputReadiness,
  diagnosisAuthModeOptions,
  diagnosisAuthRoleMappingStatusReadiness,
  type DiagnosisAuthBackendCheck,
  type DiagnosisAuthBackendStatusSnapshot,
  type DiagnosisAuthMode,
  type DiagnosisAuthInputValues,
  diagnosisAutoBrowserSessionCreateRoomPlan,
} from "./auth-readiness";
import { DiagnosisAuthModeSelector } from "./auth-mode-selector";
import {
  diagnosisConnectionTargetSessionID,
  diagnosisReconnectDecision,
} from "./connection-recovery";
import {
  diagnosisConsultationConclusionLifecycleStatus,
  diagnosisConsultationReassessmentStatus,
  type DiagnosisConsultationConclusionLifecycleStatus,
} from "./consultation-progress";
import {
  diagnosisCollaborationActorProfile,
  diagnosisCollaborationDirectoryIndex,
  diagnosisCollaborationDirectoryUsersFromDirectoryUsers,
  diagnosisCollaborationDirectoryUsersFromRooms,
  diagnosisCollaborationIdentityCoverage,
  diagnosisCollaborationParticipantProfile,
  diagnosisCollaborationParticipants,
  diagnosisCollaborationParticipantsFromSummary,
  diagnosisCollaborationRoleLabel,
  diagnosisCollaborationSubjectIsSystem,
  type DiagnosisCollaborationDirectoryUser,
  type DiagnosisCollaborationParticipant,
  type DiagnosisCollaborationParticipantRole,
} from "./collaboration";
import {
  diagnosisEvidencePlanIdentity,
  diagnosisEvidencePlanSearchParam,
} from "./evidence-plan-url";
import {
  diagnosisFinalConclusionConfidenceProgress,
  diagnosisFinalConclusionReviewItems,
  diagnosisFinalConclusionRetentionState,
  diagnosisFinalConclusionStatusLabel,
  diagnosisFinalConclusionText,
  diagnosisFinalConclusionTraceabilityStatus,
} from "./final-conclusion";
import {
  diagnosisHandoffListLimit,
  diagnosisHandoffListQueryKey,
  diagnosisRoomAuthStatusQueryKey,
  diagnosisRoomBrowserSessionQueryKey,
  diagnosisRoomDetailQueryKey,
  diagnosisRoomListLimit,
  diagnosisRoomListQueryKey,
  diagnosisRoomNotificationChannelsQueryKey,
  diagnosisRoomRefreshQueryKeys,
  diagnosisRoomToolTemplatesQueryKey,
} from "./cache";
import {
  checkDiagnosisAuthorization,
  clearDiagnosisBrowserSession,
  closeUnavailableDiagnosisRoom,
  createDiagnosisBrowserSession,
  createDiagnosisRoom,
  fetchDiagnosisBrowserSession,
  fetchDiagnosisAuthStatus,
  issueDiagnosisWSTicket,
  isDiagnosisNotificationRetryEventKind,
  nextDiagnosisMessageID,
  parseDiagnosisServerFrame,
  retryDiagnosisRoomNotification,
  type DiagnosisAuthCheck,
  type DiagnosisBrowserSessionStatus,
  type DiagnosisAuthStatus,
  type DiagnosisNotificationRetryEventKind,
  type DiagnosisNotificationRetryBundle,
  type DiagnosisRoomCreateBundle,
  type DiagnosisWSTicketBundle,
} from "./transport";
import {
  diagnosisStateTranscript,
  diagnosisTurnResultTranscript,
  type DiagnosisTranscriptTurn,
} from "./transcript";
import {
  diagnosisServerErrorDisplay,
  type DiagnosisServerError,
} from "./server-error";
import {
  latestDiagnosisFollowUpTurn,
  latestDiagnosisTurnCount,
  shouldQueryDiagnosisStateAfterTurn,
} from "./state-refresh";
import {
  refreshDiagnosisHandoffs,
  refreshDiagnosisRoom,
  refreshDiagnosisRooms,
} from "./client-api";
import {
  diagnosisNotificationChannelCreateBlockReason,
  diagnosisNotificationChannelOptions,
  diagnosisNotificationChannelProofSummary,
  diagnosisNotificationChannelSelectionError,
  diagnosisNotificationChannelSetupAction,
} from "./notification-channel-options";
import {
  diagnosisNotificationContentProofRetryEntry,
  diagnosisNotificationContentProofDisplay,
  diagnosisNotificationContentProofRetryRequired,
  diagnosisNotificationContentProofSummary,
  diagnosisNotificationDeliveryCoverage,
  diagnosisNotificationDeliveryCoveragePhaseColor,
  diagnosisNotificationDeliveryProofExpected,
  diagnosisNotificationDeliveryRecoveryHint,
  diagnosisNotificationTimelineReviewActionRequired,
  diagnosisNotificationTimelineAnchorID,
  diagnosisNotificationTimelineHref,
} from "./notification-content-proof";
import type {
  DiagnosisHandoffListResponse,
  DiagnosisRoomListResponse,
  DiagnosisRoomNotificationTimelineEntry,
  DiagnosisRoomSummary,
} from "./api";
import {
  diagnosisReviewQueueActionGate,
  diagnosisReviewQueueActionPlan,
  diagnosisReviewQueueBlockingReason,
  diagnosisReviewQueueConnectionGateAllowsPreparation,
  diagnosisReviewQueueItems,
  diagnosisReviewQueueNextAction,
  diagnosisReviewQueuePostEvidenceStatus,
  diagnosisReviewQueueReassessmentInput,
  diagnosisReviewQueueSummary,
  diagnosisReviewQueueTaskProgress,
  finalConclusionReviewQueueInput,
  type DiagnosisReviewQueueActionGate,
  type DiagnosisReviewQueueInput,
  type DiagnosisReviewQueueItem,
  type DiagnosisReviewQueuePostEvidenceStatus,
  type DiagnosisReviewQueueTaskPhaseAction,
} from "./review-queue";
import { diagnosisRoomSummaryReviewQueueInput } from "./rest-review-queue";
import {
  diagnosisRoomNextStep,
  diagnosisRoomQueueOptions,
  diagnosisRoomWorkflowUnavailable,
  filterDiagnosisRoomsByQueue,
  type DiagnosisRoomQueueFilter,
} from "./next-step";
import {
  operatorEvidenceTemplateHasParameterizedQuery,
  operatorEvidenceTemplateQuery,
  operatorEvidenceTemplateSourceDisabledReason,
} from "./operator-evidence";
import {
  supplementalEvidencePriorityFromText,
  supplementalEvidenceReassessmentMessage,
  supplementalEvidenceResidualBoundaryTemplate,
  supplementalEvidenceSubmissionMessage,
  supplementalEvidenceWirePayload,
} from "./supplemental-evidence";
import { diagnosisReportReturnHref } from "./report-return";
import {
  canCreateDiagnosisRoomByRBAC,
  diagnosisRoomAdministerAuthorizationKey,
  diagnosisRoomParticipateAuthorizationKey,
  diagnosisRoomRBACAuthorizationChecks,
  diagnosisRoomRBACBlockReason,
  diagnosisRoomRBACPermissionItems,
  diagnosisRoomReadAuthorizationKey,
  type DiagnosisRoomRBACPermissionItem,
  type DiagnosisRoomRBACPermissionStatus,
} from "./rbac-capabilities";
import { diagnosisOIDCLoginHref } from "./oidc-login";
import {
  boundedURLTextValue,
  diagnosisRoomAnchorHref,
  diagnosisRoomWeComAutoLoginSearchParam,
  diagnosisRoomWeComAuthErrorSearchParam,
  diagnosisRoomWeComLaunchContextSearchParam,
  diagnosisRoomURLWithSelectedRoom,
  diagnosisRoomURLWithoutOneShotParams,
  type DiagnosisRoomWeComAuthError,
  type DiagnosisRoomWeComLaunchContext,
} from "./url-state";
import {
  diagnosisActionIdentityBlockReason,
  diagnosisWorkflowReadiness,
  diagnosisWorkflowReadinessReviewQueueSource,
  type DiagnosisWorkflowReadinessItem,
  type DiagnosisWorkflowReadinessStatus,
} from "./workflow-readiness";
import type {
  DiagnosisActiveAlert,
  DiagnosisClientFrame,
  DiagnosisConfidenceTimelineEntry,
  DiagnosisConsultationEvidenceRequest,
  DiagnosisConsultationInsight,
  DiagnosisConnectionStatus,
  DiagnosisEvidenceCollectionResult,
  DiagnosisEvidenceRequest,
  DiagnosisEvidenceTimelineEntry,
  DiagnosisFinalConclusion,
  DiagnosisMetricSeries,
  DiagnosisServerFrame,
  DiagnosisStateFrame,
  DiagnosisSupplementalEvidenceRecord,
} from "./types";

type AuthFormValues = DiagnosisAuthInputValues;

type AuthCheckContext = "create" | "connection";
type AuthCheckRevisions = Record<AuthCheckContext, number>;

type AuthCheckResult = DiagnosisAuthBackendCheck & {
  context: AuthCheckContext;
  inputRevision: number;
};

type DiagnosisConnectionCredentials = {
  authorization: DiagnosisAuthorization;
  sessionID: string;
};

type ConnectionFormValues = AuthFormValues & {
  sessionID: string;
};

type ConnectOptions = {
  preserveState?: boolean;
  reconnect?: boolean;
};

type CreateRoomFormValues = {
  closeNotificationChannelProfileID?: number | null;
  evidenceSnapshotID?: number | null;
} & AuthFormValues;

type ComposerValues = {
  message: string;
};

function diagnosisAuthorizationFromFormValues(
  values: AuthFormValues,
): DiagnosisAuthorization {
  const mode = values.authMode ?? "session";
  if (mode === "session" || mode === "wecom") {
    return { mode: "session" };
  }
  if (mode === "bearer") {
    return { mode: "bearer", token: values.bearerToken ?? "" };
  }
  return {
    mode: "basic",
    username: values.ldapUsername ?? "",
    password: values.ldapPassword ?? "",
  };
}

function normalizedConnectionCredentials(
  values: ConnectionFormValues | DiagnosisConnectionCredentials,
): DiagnosisConnectionCredentials | null {
  const sessionID = values.sessionID.trim();
  const authorization =
    "authorization" in values
      ? normalizedDiagnosisAuthorization(values.authorization)
      : normalizedDiagnosisAuthorization(
          diagnosisAuthorizationFromFormValues(values),
        );
  if (sessionID === "" || authorization === null) {
    return null;
  }
  return { authorization, sessionID };
}

function connectionFormValuesFromCredentials(
  credentials: DiagnosisConnectionCredentials,
): Partial<ConnectionFormValues> {
  if (credentials.authorization.mode === "session") {
    return {
      authMode: "session",
      sessionID: credentials.sessionID,
    };
  }
  if (credentials.authorization.mode === "bearer") {
    return {
      authMode: "bearer",
      bearerToken: credentials.authorization.token,
      sessionID: credentials.sessionID,
    };
  }
  return {
    authMode: "ldap",
    ldapPassword: credentials.authorization.password,
    ldapUsername: credentials.authorization.username,
    sessionID: credentials.sessionID,
  };
}

function diagnosisAuthInputValuesFromWatchedFields(
  values: Required<DiagnosisAuthInputValues>,
): DiagnosisAuthInputValues {
  return {
    authMode: values.authMode,
    bearerToken: values.bearerToken,
    ldapPassword: values.ldapPassword,
    ldapUsername: values.ldapUsername,
  };
}

function diagnosisAuthModeFromFieldValue(value: unknown): DiagnosisAuthMode {
  if (value === "bearer" || value === "session" || value === "wecom") {
    return value;
  }
  return "session";
}

function DiagnosisAuthReadinessPreview({
  authStatus,
  authStatusLoading,
  backendStatus,
  checking,
  expectedSubject,
  inputRevision,
  lastCheck,
  values,
}: {
  authStatus?: ApiResult<DiagnosisAuthStatus>;
  authStatusLoading?: boolean;
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  checking: boolean;
  expectedSubject?: string;
  inputRevision: number;
  lastCheck: AuthCheckResult | null;
  values: DiagnosisAuthInputValues;
}) {
  const readiness = diagnosisAuthInputReadiness(values);
  const displayedLastCheck =
    lastCheck !== null && lastCheck.mode === readiness.mode ? lastCheck : null;
  const backendReadiness = diagnosisAuthBackendReadiness({
    backendStatus,
    checking,
    expectedSubject,
    inputRevision,
    lastCheck: displayedLastCheck,
    values,
  });
  const backendAuthStatus = authStatus?.ok === true ? authStatus.data : null;
  const supportedModeItems =
    diagnosisAuthBackendModeDisplayItems(backendAuthStatus);
  const roles =
    displayedLastCheck === null || displayedLastCheck.roles.length === 0
      ? "no roles"
      : displayedLastCheck.roles.join(", ");
  return (
    <div
      aria-label="Diagnosis auth readiness"
      className="settings-preview-panel"
    >
      <Space direction="vertical" size={8}>
        <Space wrap>
          <Tag color={authReadinessTagColor(readiness.status)}>
            {authReadinessStatusLabel(readiness.status)}
          </Tag>
          <Tag color={diagnosisAuthModeTagColor(readiness.mode)}>
            {diagnosisAuthModeLabel(readiness.mode)}
          </Tag>
          <Tag color={backendReadiness.color}>{backendReadiness.status}</Tag>
          <Tag
            color={diagnosisAuthStatusTagColor(authStatus, authStatusLoading)}
          >
            {diagnosisAuthStatusTagLabel(authStatus, authStatusLoading)}
          </Tag>
        </Space>
        <Typography.Text strong>{readiness.label}</Typography.Text>
        <Typography.Text type="secondary">
          {diagnosisAuthStatusDetail(authStatus, authStatusLoading)}
        </Typography.Text>
        <DiagnosisAuthRoleMappingSummary
          loading={authStatusLoading ?? false}
          status={backendAuthStatus}
        />
        {supportedModeItems.length > 0 ? (
          <Space size={6} wrap>
            <Typography.Text type="secondary">Supported modes</Typography.Text>
            {supportedModeItems.map((item) => (
              <Tag color={item.color} key={item.mode}>
                {item.label}
              </Tag>
            ))}
          </Space>
        ) : null}
        <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
        <Typography.Text type="secondary">
          {backendReadiness.detail}
        </Typography.Text>
        {displayedLastCheck === null ? (
          <Typography.Text type="secondary">
            No backend auth check has been run for this form.
          </Typography.Text>
        ) : (
          <Typography.Text
            type={
              displayedLastCheck.status === "success" ? "secondary" : "danger"
            }
          >
            {displayedLastCheck.status === "success"
              ? `${displayedLastCheck.message} Roles: ${roles}.`
              : `Last auth check failed: ${displayedLastCheck.message}`}
          </Typography.Text>
        )}
      </Space>
    </div>
  );
}

function DiagnosisAuthRoleMappingSummary({
  loading,
  status,
}: {
  loading: boolean;
  status: DiagnosisAuthStatus | null;
}) {
  const readiness = diagnosisAuthRoleMappingStatusReadiness(status, loading);
  return (
    <Space direction="vertical" size={4}>
      <Tag color={readiness.color}>{readiness.label}</Tag>
      <Typography.Text type="secondary">{readiness.detail}</Typography.Text>
    </Space>
  );
}

function DiagnosisBrowserSessionActions({
  busy,
  checkAuthDisabledReason,
  checkAuthLoading,
  directoryUsersBySubject,
  onCheckAuth,
  onLogout,
  onRefreshSession,
  oidcLoginHref,
  session,
  sessionLoading,
  sessionRefreshing,
  logoutBusy,
}: {
  busy: boolean;
  checkAuthDisabledReason: string;
  checkAuthLoading: boolean;
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  onCheckAuth: () => void;
  onLogout: () => void;
  onRefreshSession: () => void;
  oidcLoginHref: Route;
  session?: ApiResult<DiagnosisBrowserSessionStatus>;
  sessionLoading: boolean;
  sessionRefreshing: boolean;
  logoutBusy: boolean;
}) {
  const activeSession =
    session?.ok === true && session.data.authenticated ? session.data : null;
  const sessionDisplay = diagnosisAuthBrowserSessionDisplaySummary({
    authenticated: activeSession !== null,
    checkFailed: session?.ok === false,
    loading: sessionLoading,
    mode: activeSession?.mode,
    roleAuthorized: activeSession?.role_authorized,
    roles: activeSession?.roles ?? [],
    subject: activeSession?.subject ?? "",
    unauthenticatedDetail:
      "No OpenClarion browser session is active. Sign in with IAM before using diagnosis-room auth.",
  });
  return (
    <Alert
      className="diagnosis-browser-session-alert"
      action={
        <Space wrap>
          {activeSession === null ? (
            <Button
              disabled={busy}
              href={oidcLoginHref}
              icon={<LoginOutlined />}
              type="primary"
            >
              Sign in with IAM
            </Button>
          ) : null}
          <TooltipAction
            disabled={checkAuthDisabledReason !== ""}
            title={checkAuthDisabledReason}
          >
            <Button
              disabled={
                busy || checkAuthLoading || checkAuthDisabledReason !== ""
              }
              icon={<CheckCircleOutlined />}
              loading={checkAuthLoading}
              onClick={onCheckAuth}
              type="primary"
            >
              Check auth
            </Button>
          </TooltipAction>
          <Button
            disabled={busy || sessionRefreshing}
            icon={<ReloadOutlined />}
            loading={sessionRefreshing}
            onClick={onRefreshSession}
          >
            Refresh session
          </Button>
          {activeSession !== null ? (
            <Button
              disabled={busy}
              icon={<DisconnectOutlined />}
              loading={logoutBusy}
              onClick={onLogout}
            >
              Sign out
            </Button>
          ) : null}
        </Space>
      }
      description={
        <BrowserSessionDescription
          detail={sessionDisplay.detail}
          directoryUsersBySubject={directoryUsersBySubject}
          subject={activeSession?.subject}
        />
      }
      message="IAM browser session"
      showIcon
      type={sessionDisplay.alertType}
    />
  );
}

function LDAPBrowserSessionPromotionNotice() {
  const notice = diagnosisAuthLDAPBrowserSessionPromotionNotice();
  return (
    <Alert
      description={notice.detail}
      message={notice.message}
      showIcon
      type="info"
    />
  );
}

function DiagnosisWeComLoginActions({
  busy,
  checkAuthDisabledReason,
  checkAuthLoading,
  directoryUsersBySubject,
  iamLoginHref,
  onCheckAuth,
  onLogout,
  onRefreshSession,
  session,
  sessionLoading,
  sessionRefreshing,
  logoutBusy,
}: {
  busy: boolean;
  checkAuthDisabledReason: string;
  checkAuthLoading: boolean;
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  iamLoginHref: Route;
  onCheckAuth: () => void;
  onLogout: () => void;
  onRefreshSession: () => void;
  session?: ApiResult<DiagnosisBrowserSessionStatus>;
  sessionLoading: boolean;
  sessionRefreshing: boolean;
  logoutBusy: boolean;
}) {
  const activeSession =
    session?.ok === true && session.data.authenticated ? session.data : null;
  const sessionCheckFailed = session?.ok === false;
  const migrationDetail =
    "Enterprise WeChat browser login has been replaced by IAM OIDC. Use IAM sign-in for OpenClarion browser sessions; Enterprise WeChat remains available for app messages, notification delivery, and diagnosis-room collaboration callbacks.";
  const sessionDisplay = diagnosisAuthBrowserSessionDisplaySummary({
    authenticated: activeSession !== null,
    checkFailed: sessionCheckFailed,
    loading: sessionLoading,
    mode: activeSession?.mode,
    roleAuthorized: activeSession?.role_authorized,
    roles: activeSession?.roles ?? [],
    subject: activeSession?.subject ?? "",
    unauthenticatedDetail: migrationDetail,
  });
  const loginDescription =
    sessionDisplay.active || sessionLoading || sessionCheckFailed ? (
      <BrowserSessionDescription
        detail={sessionDisplay.detail}
        directoryUsersBySubject={directoryUsersBySubject}
        subject={activeSession?.subject}
      />
    ) : (
      <Space direction="vertical" size={6}>
        <Typography.Text>{migrationDetail}</Typography.Text>
      </Space>
    );
  return (
    <Alert
      className="diagnosis-browser-session-alert"
      action={
        <Space wrap>
          {activeSession === null ? (
            <Button
              disabled={busy}
              href={iamLoginHref}
              icon={<LoginOutlined />}
              type="primary"
            >
              Sign in with IAM
            </Button>
          ) : (
            <TooltipAction
              disabled={checkAuthDisabledReason !== ""}
              title={checkAuthDisabledReason}
            >
              <Button
                disabled={
                  busy || checkAuthLoading || checkAuthDisabledReason !== ""
                }
                icon={<SafetyCertificateOutlined />}
                loading={checkAuthLoading}
                onClick={onCheckAuth}
                type="primary"
              >
                Use browser session
              </Button>
            </TooltipAction>
          )}
          <Button
            disabled={busy || sessionRefreshing}
            icon={<ReloadOutlined />}
            loading={sessionRefreshing}
            onClick={onRefreshSession}
          >
            Refresh session
          </Button>
          {activeSession !== null ? (
            <Button
              disabled={busy}
              icon={<DisconnectOutlined />}
              loading={logoutBusy}
              onClick={onLogout}
            >
              Sign out
            </Button>
          ) : null}
        </Space>
      }
      description={loginDescription}
      message="IAM browser session"
      showIcon
      type={
        sessionDisplay.active || sessionLoading || sessionCheckFailed
          ? sessionDisplay.alertType
          : "warning"
      }
    />
  );
}

function BrowserSessionDescription({
  detail,
  directoryUsersBySubject,
  subject,
}: {
  detail: string;
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  subject?: string;
}) {
  const normalizedSubject = subject?.trim() ?? "";
  if (normalizedSubject === "") {
    return detail;
  }
  return (
    <Space direction="vertical" size={6}>
      <Typography.Text>{detail}</Typography.Text>
      <ActorSubjectTags
        directoryUsersBySubject={directoryUsersBySubject}
        label="Directory"
        subject={normalizedSubject}
      />
    </Space>
  );
}

function authReadinessTagColor(
  status: ReturnType<typeof diagnosisAuthInputReadiness>["status"],
): string {
  switch (status) {
    case "ready":
      return "green";
    case "pending":
      return "gold";
    case "blocked":
      return "red";
  }
}

function diagnosisAuthModeTagColor(mode: DiagnosisAuthMode): string {
  switch (mode) {
    case "ldap":
      return "blue";
    case "bearer":
      return "default";
    case "session":
      return "cyan";
    case "wecom":
      return "green";
  }
}

function diagnosisAuthModeLabel(mode: DiagnosisAuthMode): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "bearer":
      return "Bearer";
    case "session":
      return "Browser session";
    case "wecom":
      return "WeCom";
  }
}

function authCheckHasUsableRole(check: DiagnosisAuthCheck): boolean {
  if (check.role_authorized !== undefined) {
    return check.role_authorized;
  }
  return check.roles.some((role) => role === "owner" || role === "admin");
}

function diagnosisAuthStatusTagColor(
  result: ApiResult<DiagnosisAuthStatus> | undefined,
  loading = false,
): string {
  if (loading) {
    return "processing";
  }
  if (
    result === undefined ||
    !result.ok ||
    !result.data.configured ||
    result.data.mode === "none"
  ) {
    return "red";
  }
  if (result.data.mode === "ldap") {
    return "blue";
  }
  if (result.data.mode === "oidc" && result.data.oidc_bff?.status === "blocked") {
    return "red";
  }
  if (
    result.data.mode === "static" ||
    result.data.mode === "oidc"
  ) {
    return "gold";
  }
  return "default";
}

function diagnosisAuthStatusTagLabel(
  result: ApiResult<DiagnosisAuthStatus> | undefined,
  loading = false,
): string {
  if (loading) {
    return "Backend loading";
  }
  if (result === undefined) {
    return "Backend unknown";
  }
  if (!result.ok) {
    return "Backend unavailable";
  }
  const modesLabel = diagnosisAuthBackendModeListLabel(result.data);
  if (modesLabel !== "") {
    return `Backend ${modesLabel}`;
  }
  switch (result.data.mode) {
    case "ldap":
      return "Backend LDAP";
    case "static":
      return "Backend static";
    case "oidc":
      return "Backend OIDC";
    case "unknown":
      return "Backend unknown";
    case "none":
      return "Backend not configured";
  }
}

function diagnosisAuthStatusDetail(
  result: ApiResult<DiagnosisAuthStatus> | undefined,
  loading = false,
): string {
  if (loading) {
    return "Checking backend diagnosis auth wiring without sending credentials.";
  }
  if (result === undefined) {
    return "Backend diagnosis auth wiring has not been loaded.";
  }
  if (!result.ok) {
    return `Backend diagnosis auth wiring could not be loaded: ${result.error.message}`;
  }
  if (!result.data.configured || result.data.mode === "none") {
    return "Diagnosis auth is not configured in the running backend.";
  }
  const supportedModes = diagnosisAuthBackendStatusModes(result.data);
  if (supportedModes.length > 1) {
    return diagnosisAuthMixedStatusDetail(result.data);
  }
  switch (result.data.mode) {
    case "ldap":
      return "The running backend expects LDAP Basic credentials.";
    case "static":
      return "The running backend expects a static Bearer token.";
    case "oidc":
      if (result.data.oidc_bff?.status === "blocked") {
        return `The running backend expects IAM OIDC authentication, but browser IAM sign-in is not ready: ${diagnosisOIDCBFFReadinessDetail(result.data.oidc_bff)}.`;
      }
      return "The running backend expects IAM OIDC authentication. Use Sign in with IAM to establish an OpenClarion browser session before creating or connecting to a diagnosis room.";
    case "unknown":
      return "The running backend has an auth provider, but it did not report a named mode.";
  }
}

function diagnosisAuthMixedStatusDetail(status: DiagnosisAuthStatus): string {
  const credentialLabel = diagnosisAuthBackendCredentialListLabel(status);
  return `The running backend accepts ${credentialLabel}.`;
}

function diagnosisOIDCBFFReadinessDetail(
  readiness: NonNullable<DiagnosisAuthStatus["oidc_bff"]>,
): string {
  if (readiness.status === "ready") {
    return "OIDC browser BFF prerequisites are configured";
  }
  const labels = readiness.missing.map((key) => {
    switch (key) {
      case "client_auth_method":
        return "client authentication method";
      case "client_id":
        return "client ID";
      case "client_secret":
        return "client secret";
      case "issuer":
        return "issuer";
      case "openid_scope":
        return "openid scope";
      case "pkce":
        return "PKCE for public client";
      case "session_signing_key":
        return "browser session signing key";
      case "state_signing_key":
        return "state signing key";
    }
  });
  return `missing ${labels.join(", ")}`;
}

function diagnosisAuthBackendStatusSnapshot(
  result: ApiResult<DiagnosisAuthStatus> | undefined,
): DiagnosisAuthBackendStatusSnapshot | undefined {
  if (result === undefined || !result.ok) {
    return undefined;
  }
  return {
    configured: result.data.configured,
    mode: result.data.mode,
    supportedModes: result.data.supported_modes,
  };
}

function authReadinessStatusLabel(
  status: ReturnType<typeof diagnosisAuthInputReadiness>["status"],
): string {
  switch (status) {
    case "ready":
      return "Ready";
    case "pending":
      return "Pending";
    case "blocked":
      return "Blocked";
  }
}

type SupplementalEvidenceFormValues = {
  evidence: string;
};

type OperatorEvidenceTool =
  "active_alerts" | "metric_query" | "metric_range_query";

type OperatorEvidenceFormValues = {
  alertSourceProfileID?: number | null;
  limit?: number | null;
  query?: string;
  reason: string;
  selectedTemplateID?: number | null;
  stepSeconds?: number | null;
  templateID?: number | null;
  tool: OperatorEvidenceTool;
  windowSeconds?: number | null;
};

type OperatorEvidenceRecommendation = {
  detail: string;
  disabledReason: string;
  formValues: Partial<OperatorEvidenceFormValues>;
  key: string;
  ready: boolean;
  sourceMatches: boolean;
  tag: string;
  template: DiagnosisToolTemplate;
  title: string;
};

type LogEntry = {
  id: number;
  level: "info" | "error";
  message: string;
};

const diagnosisReconnectDelayMS = 2_000;
const diagnosisReconnectMaxAttempts = 3;

type DiagnosisTurnResultFrame = Extract<
  DiagnosisServerFrame,
  { type: "turn_result" }
>;

type LatestConsultationInsight = {
  assistantSequence?: number;
  autoFollowUpCount: number;
  collectionResults: DiagnosisEvidenceCollectionResult[];
  confidence: string;
  confidenceTimeline: DiagnosisConfidenceTimelineEntry[];
  evidenceTimeline: DiagnosisEvidenceTimelineEntry[];
  evidenceRequests: DiagnosisEvidenceRequest[];
  insight: DiagnosisConsultationInsight;
  requiresHumanReview: boolean;
  status: string;
  turnCount: number;
};

type EvidenceCollectionSummaryStats = {
  collected: number;
  failed: number;
  observedAlerts: number;
  observedMetricSeries: number;
  skipped: number;
  total: number;
  unresolved: number;
  unsupported: number;
};

type DiagnosisPageContext = {
  authError?: DiagnosisRoomWeComAuthError;
  authMode?: DiagnosisAuthMode;
  backHref?: string;
  description: string;
  evidencePlan?: DiagnosisEvidenceRequest;
  evidenceSnapshotID?: number;
  hasContext: boolean;
  sessionID?: string;
  suggestedPrompt: string;
  supplementalFollowUp?: DiagnosisConsultationEvidenceRequest;
  title: string;
  oidcAuthError?: DiagnosisRoomOIDCAuthError;
  weComAutoLogin?: boolean;
  weComLaunchContext?: DiagnosisRoomWeComLaunchContext;
};

type DiagnosisRoomOIDCAuthError =
  | "oidc_auth_failed"
  | "oidc_callback_failed"
  | "oidc_callback_missing"
  | "oidc_login_failed"
  | "oidc_not_configured"
  | "oidc_role_unauthorized";

type DiagnosisRoomViewProps = {
  alertContext?: DiagnosisAlertContext;
  handoffsResult?: ApiResult<DiagnosisHandoffListResponse>;
  initialEvidenceSnapshotID?: number;
  initialSessionID?: string;
  recentRoomsResult?: ApiResult<DiagnosisRoomListResponse>;
};

type DiagnosisHandoffBacklogItem =
  DiagnosisHandoffListResponse["items"][number];
type DiagnosisWorkQueueFilter = DiagnosisRoomQueueFilter | "handoffs";
type DiagnosisWorkbenchSection =
  | "queue"
  | "setup"
  | "room"
  | "insight"
  | "conversation";

export type DiagnosisAlertContext = {
  alert: AlertEventSummary;
  snapshot: AlertEvidenceSnapshotLink;
};

class DiagnosisActionError extends Error {
  constructor(
    message: string,
    readonly status?: number,
  ) {
    super(message);
    this.name = "DiagnosisActionError";
  }
}

export function DiagnosisRoomView({
  alertContext,
  handoffsResult,
  initialEvidenceSnapshotID,
  initialSessionID,
  recentRoomsResult,
}: DiagnosisRoomViewProps) {
  const { message } = AntdApp.useApp();
  const queryClient = useQueryClient();
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const socketRef = useRef<WebSocket | null>(null);
  const socketGenerationRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const autoBrowserSessionAuthCheckAttemptKeyRef = useRef("");
  const autoBrowserSessionConnectionAttemptKeyRef = useRef("");
  const autoBrowserSessionCreateRoomAttemptKeyRef = useRef("");
  const reconnectAttemptRef = useRef(0);
  const manualDisconnectRef = useRef(false);
  const lastConnectionRef = useRef<DiagnosisConnectionCredentials | null>(null);
  const pendingConnectionReturnRef = useRef<"review_queue" | null>(null);
  const selectedSessionIDRef = useRef<string | undefined>(undefined);
  const clientReady = useClientReady();
  const logIDRef = useRef(0);
  const createRoomPanelRef = useRef<HTMLDivElement | null>(null);
  const connectionPanelRef = useRef<HTMLDivElement | null>(null);
  const notificationTimelinePanelRef = useRef<HTMLDivElement | null>(null);
  const permissionsPanelRef = useRef<HTMLDivElement | null>(null);
  const reviewQueuePanelRef = useRef<HTMLElement | null>(null);
  const roomSelectionPanelRef = useRef<HTMLDivElement | null>(null);
  const roomStatePanelRef = useRef<HTMLDivElement | null>(null);
  const insightPanelRef = useRef<HTMLDivElement | null>(null);
  const conversationPanelRef = useRef<HTMLDivElement | null>(null);
  const supplementalEvidencePanelRef = useRef<HTMLDivElement | null>(null);
  const [createForm] = Form.useForm<CreateRoomFormValues>();
  const [connectionForm] = Form.useForm<ConnectionFormValues>();
  const [composerForm] = Form.useForm<ComposerValues>();
  const [operatorEvidenceForm] = Form.useForm<OperatorEvidenceFormValues>();
  const [supplementalEvidenceForm] =
    Form.useForm<SupplementalEvidenceFormValues>();
  const watchedConnectionSessionID = Form.useWatch("sessionID", connectionForm);
  const watchedCreateAuthMode =
    Form.useWatch("authMode", createForm) ?? "session";
  const watchedCreateBearerToken =
    Form.useWatch("bearerToken", createForm) ?? "";
  const watchedCreateLDAPPassword =
    Form.useWatch("ldapPassword", createForm) ?? "";
  const watchedCreateLDAPUsername =
    Form.useWatch("ldapUsername", createForm) ?? "";
  const watchedConnectionAuthMode =
    Form.useWatch("authMode", connectionForm) ?? "session";
  const watchedConnectionBearerToken =
    Form.useWatch("bearerToken", connectionForm) ?? "";
  const watchedConnectionLDAPPassword =
    Form.useWatch("ldapPassword", connectionForm) ?? "";
  const watchedConnectionLDAPUsername =
    Form.useWatch("ldapUsername", connectionForm) ?? "";
  const watchedCreateNotificationChannelID = Form.useWatch(
    "closeNotificationChannelProfileID",
    createForm,
  );
  const diagnosisToolTemplateQuery = useQuery({
    queryKey: diagnosisRoomToolTemplatesQueryKey,
    queryFn: refreshDiagnosisToolTemplates,
    staleTime: 60_000,
  });
  const notificationChannelsQuery = useQuery({
    queryKey: diagnosisRoomNotificationChannelsQueryKey,
    queryFn: refreshNotificationChannelProfiles,
    staleTime: 60_000,
  });
  const diagnosisAuthStatusQuery = useQuery({
    queryKey: diagnosisRoomAuthStatusQueryKey,
    queryFn: fetchDiagnosisAuthStatus,
    staleTime: 60_000,
  });
  const diagnosisBrowserSessionQuery = useQuery({
    queryKey: diagnosisRoomBrowserSessionQueryKey,
    queryFn: fetchDiagnosisBrowserSession,
    staleTime: 30_000,
  });
  const [status, setStatus] = useState<DiagnosisConnectionStatus>("idle");
  const [workbenchSection, setWorkbenchSection] =
    useState<DiagnosisWorkbenchSection>(() => {
      if (initialSessionID || searchParams.get("session_id")?.trim()) {
        return "room";
      }
      if (
        alertContext ||
        initialEvidenceSnapshotID !== undefined ||
        searchParams.get("evidence_snapshot_id")?.trim()
      ) {
        return "setup";
      }
      return "queue";
    });
  const [socketOpen, setSocketOpen] = useState(false);
  const [readySubject, setReadySubject] = useState("");
  const [localTurnInFlight, setLocalTurnInFlight] = useState(false);
  const [confirmInFlight, setConfirmInFlight] = useState(false);
  const [closingUnavailableSessionID, setClosingUnavailableSessionID] =
    useState("");
  const [retryingNotificationKey, setRetryingNotificationKey] = useState("");
  const weComAutoLoginAttemptRef = useRef("");
  const [roomState, setRoomState] = useState<DiagnosisStateFrame | null>(null);
  const [transcript, setTranscript] = useState<DiagnosisTranscriptTurn[]>([]);
  const [latestInsight, setLatestInsight] =
    useState<LatestConsultationInsight | null>(null);
  const [
    manualPendingSupplementalEvidence,
    setManualPendingSupplementalEvidence,
  ] = useState<DiagnosisConsultationEvidenceRequest | null>(null);
  const [manualPendingEvidencePlan, setManualPendingEvidencePlan] =
    useState<DiagnosisEvidenceRequest | null>(null);
  const [clearedURLFollowUpKey, setClearedURLFollowUpKey] = useState<
    string | null
  >(null);
  const [clearedURLEvidencePlanKey, setClearedURLEvidencePlanKey] = useState<
    string | null
  >(null);
  const [serverError, setServerError] = useState<DiagnosisServerError | null>(
    null,
  );
  const [log, setLog] = useState<LogEntry[]>([]);
  const [authCheckContext, setAuthCheckContext] =
    useState<AuthCheckContext | null>(null);
  const [lastAuthCheck, setLastAuthCheck] = useState<AuthCheckResult | null>(
    null,
  );
  const [authCheckRevision, setAuthCheckRevision] =
    useState<AuthCheckRevisions>({
      connection: 0,
      create: 0,
    });
  const [workQueueFilter, setWorkQueueFilter] =
    useState<DiagnosisWorkQueueFilter>("all");
  const pageContext = diagnosisPageContext(searchParams, alertContext);
  const hasWeComAuthErrorParam = searchParams.has("wecom_auth_error");
  const weComAuthErrorSource = hasWeComAuthErrorParam
    ? searchParams.toString()
    : "";
  const [dismissedWeComAuthErrorSource, setDismissedWeComAuthErrorSource] =
    useState("");
  const currentWeComAuthError =
    pageContext.authError === undefined
      ? null
      : { error: pageContext.authError, source: weComAuthErrorSource };
  const visibleWeComAuthError =
    currentWeComAuthError !== null &&
    currentWeComAuthError.source !== dismissedWeComAuthErrorSource
      ? currentWeComAuthError.error
      : null;
  const weComAuthErrorDetail =
    visibleWeComAuthError === null
      ? null
      : diagnosisWeComAuthErrorDetail(visibleWeComAuthError);
  const hasOIDCAuthErrorParam = searchParams.has("oidc_auth_error");
  const oidcAuthErrorSource = hasOIDCAuthErrorParam
    ? searchParams.toString()
    : "";
  const [dismissedOIDCAuthErrorSource, setDismissedOIDCAuthErrorSource] =
    useState("");
  const currentOIDCAuthError =
    pageContext.oidcAuthError === undefined
      ? null
      : { error: pageContext.oidcAuthError, source: oidcAuthErrorSource };
  const visibleOIDCAuthError =
    currentOIDCAuthError !== null &&
    currentOIDCAuthError.source !== dismissedOIDCAuthErrorSource
      ? currentOIDCAuthError.error
      : null;
  const oidcAuthErrorDetail =
    visibleOIDCAuthError === null
      ? null
      : diagnosisOIDCAuthErrorDetail(visibleOIDCAuthError);
  const [connectionSessionID, setConnectionSessionID] = useState<string | null>(
    null,
  );
  const watchedSessionID =
    typeof watchedConnectionSessionID === "string"
      ? watchedConnectionSessionID.trim()
      : undefined;
  const selectedSessionID =
    connectionSessionID ?? pageContext.sessionID ?? watchedSessionID ?? "";
  const oidcLoginHref = useMemo(() => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("auth_mode", "session");
    params.delete("oidc_auth_error");
    const query = params.toString();
    const returnTo = query === "" ? pathname : `${pathname}?${query}`;
    return diagnosisOIDCLoginHref(returnTo);
  }, [pathname, searchParams]);
  const recentRoomsQuery = useQuery({
    queryKey: diagnosisRoomListQueryKey,
    queryFn: () => refreshDiagnosisRooms(diagnosisRoomListLimit),
    initialData: recentRoomsResult,
    staleTime: 15_000,
    refetchInterval: 30_000,
  });
  const exactRoomSessionID = pageContext.sessionID ?? "";
  const exactRoomQuery = useQuery({
    queryKey: diagnosisRoomDetailQueryKey(exactRoomSessionID),
    queryFn: () => refreshDiagnosisRoom(exactRoomSessionID),
    enabled: exactRoomSessionID !== "",
    staleTime: 15_000,
    refetchInterval: exactRoomSessionID !== "" ? 30_000 : false,
  });
  const handoffsQuery = useQuery({
    queryKey: diagnosisHandoffListQueryKey,
    queryFn: () => refreshDiagnosisHandoffs(diagnosisHandoffListLimit),
    initialData: handoffsResult,
    staleTime: 15_000,
    refetchInterval: 30_000,
  });
  const selectedExactRoomSummary =
    exactRoomSessionID !== "" &&
    selectedSessionID === exactRoomSessionID &&
    exactRoomQuery.data?.ok === true
      ? exactRoomQuery.data.data
      : undefined;
  const selectedRoomSummary =
    selectedExactRoomSummary ??
    selectedDiagnosisRoomSummary(recentRoomsQuery.data, selectedSessionID);
  const selectedNotificationDeliveryCoverage =
    selectedRoomSummary === undefined
      ? undefined
      : diagnosisNotificationDeliveryCoverage(
          selectedRoomSummary.notification_timeline ?? [],
        );
  const diagnosisRBACSessionIDs = useMemo(() => {
    const sessionIDs = new Set<string>();
    if (selectedSessionID.trim() !== "") {
      sessionIDs.add(selectedSessionID.trim());
    }
    if (recentRoomsQuery.data?.ok === true) {
      for (const room of recentRoomsQuery.data.data.items) {
        sessionIDs.add(room.session_id);
      }
    }
    return [...sessionIDs];
  }, [recentRoomsQuery.data, selectedSessionID]);
  const diagnosisRoomAuthorizationChecks = useMemo(
    () =>
      diagnosisRoomRBACAuthorizationChecks(diagnosisRBACSessionIDs, {
        closeNotificationChannelProfileID: watchedCreateNotificationChannelID,
      }),
    [diagnosisRBACSessionIDs, watchedCreateNotificationChannelID],
  );
  const diagnosisRoomAuthorization = useCurrentRBACAuthorizations(
    diagnosisRoomAuthorizationChecks,
    clientReady &&
      diagnosisBrowserSessionQuery.data?.ok === true &&
      diagnosisBrowserSessionQuery.data.data.authenticated,
  );
  const diagnosisRoomAuthorizationEnforced =
    diagnosisBrowserSessionQuery.data?.ok === true &&
    diagnosisBrowserSessionQuery.data.data.authenticated;
  const diagnosisRoomAuthorizationChecking =
    diagnosisRoomAuthorizationEnforced && diagnosisRoomAuthorization.isChecking;
  const canCreateDiagnosisRoom =
    canCreateDiagnosisRoomByRBAC({
      can: diagnosisRoomAuthorization.can,
      closeNotificationChannelProfileID: watchedCreateNotificationChannelID,
      enforced: diagnosisRoomAuthorizationEnforced,
    });
  const canReadSelectedRoomByRBAC =
    selectedSessionID.trim() !== "" &&
    (!diagnosisRoomAuthorizationEnforced ||
      diagnosisRoomAuthorization.can(
        diagnosisRoomReadAuthorizationKey(selectedSessionID.trim()),
      ));
  const canParticipateSelectedRoomByRBAC =
    selectedSessionID.trim() !== "" &&
    (!diagnosisRoomAuthorizationEnforced ||
      diagnosisRoomAuthorization.can(
        diagnosisRoomParticipateAuthorizationKey(selectedSessionID.trim()),
      ));
  const canAdministerSelectedRoomByRBAC =
    selectedSessionID.trim() !== "" &&
    (!diagnosisRoomAuthorizationEnforced ||
      diagnosisRoomAuthorization.can(
        diagnosisRoomAdministerAuthorizationKey(selectedSessionID.trim()),
      ));
  const createRBACBlockReason = diagnosisRoomRBACBlockReason({
    action: "create",
    allowed: canCreateDiagnosisRoom,
    checking: diagnosisRoomAuthorizationChecking,
    enforced: diagnosisRoomAuthorizationEnforced,
  });
  const selectedParticipateRBACBlockReason = diagnosisRoomRBACBlockReason({
    action: "participate",
    allowed: canParticipateSelectedRoomByRBAC,
    checking: diagnosisRoomAuthorizationChecking,
    enforced:
      diagnosisRoomAuthorizationEnforced && selectedSessionID.trim() !== "",
  });
  const selectedReadRBACBlockReason = diagnosisRoomRBACBlockReason({
    action: "read",
    allowed: canReadSelectedRoomByRBAC,
    checking: diagnosisRoomAuthorizationChecking,
    enforced:
      diagnosisRoomAuthorizationEnforced && selectedSessionID.trim() !== "",
  });
  const selectedAdministerRBACBlockReason = diagnosisRoomRBACBlockReason({
    action: "administer",
    allowed: canAdministerSelectedRoomByRBAC,
    checking: diagnosisRoomAuthorizationChecking,
    enforced:
      diagnosisRoomAuthorizationEnforced && selectedSessionID.trim() !== "",
  });
  const canAdministerDiagnosisRoom = useCallback(
    (sessionID: string) =>
      !diagnosisRoomAuthorizationEnforced ||
      diagnosisRoomAuthorization.can(
        diagnosisRoomAdministerAuthorizationKey(sessionID),
      ),
    [diagnosisRoomAuthorization, diagnosisRoomAuthorizationEnforced],
  );
  const diagnosisRoomAdministerBlockReason = useCallback(
    (sessionID: string) =>
      diagnosisRoomRBACBlockReason({
        action: "administer",
        allowed: canAdministerDiagnosisRoom(sessionID),
        checking: diagnosisRoomAuthorizationChecking,
        enforced: diagnosisRoomAuthorizationEnforced,
      }),
    [
      canAdministerDiagnosisRoom,
      diagnosisRoomAuthorizationChecking,
      diagnosisRoomAuthorizationEnforced,
    ],
  );
  const operatorEvidenceTemplates = useMemo(
    () => enabledDiagnosisToolTemplates(diagnosisToolTemplateQuery.data),
    [diagnosisToolTemplateQuery.data],
  );
  const notificationChannels = useMemo(
    () => notificationChannelItems(notificationChannelsQuery.data),
    [notificationChannelsQuery.data],
  );
  const notificationChannelOptions = useMemo(
    () => diagnosisNotificationChannelOptions(notificationChannels),
    [notificationChannels],
  );
  const notificationChannelSetupAction = useMemo(
    () =>
      diagnosisNotificationChannelSetupAction({
        channels: notificationChannels,
        failedToLoad: notificationChannelsQuery.data?.ok === false,
      }),
    [notificationChannels, notificationChannelsQuery.data],
  );
  const authBackendStatusSnapshot = useMemo(
    () => diagnosisAuthBackendStatusSnapshot(diagnosisAuthStatusQuery.data),
    [diagnosisAuthStatusQuery.data],
  );
  const authenticatedBrowserSessionSubject =
    diagnosisBrowserSessionQuery.data?.ok === true &&
    diagnosisBrowserSessionQuery.data.data.authenticated
      ? diagnosisBrowserSessionQuery.data.data.subject
      : undefined;
  const currentActorSubject = (
    readySubject ||
    authenticatedBrowserSessionSubject ||
    ""
  ).trim();
  const authModeOptions = useMemo(
    () => diagnosisAuthModeOptions(authBackendStatusSnapshot),
    [authBackendStatusSnapshot],
  );
  const weComQuickSignInPrompt = useMemo(
    () =>
      diagnosisAuthWeComQuickSignInPrompt({
        backendStatus: authBackendStatusSnapshot,
        selectedModes: [watchedCreateAuthMode, watchedConnectionAuthMode],
      }),
    [
      authBackendStatusSnapshot,
      watchedConnectionAuthMode,
      watchedCreateAuthMode,
    ],
  );
  useEffect(() => {
    if (
      pageContext.weComAutoLogin !== true ||
      visibleWeComAuthError !== null ||
      authBackendStatusSnapshot === undefined ||
      diagnosisBrowserSessionQuery.isPending ||
      (diagnosisBrowserSessionQuery.data?.ok === true &&
        diagnosisBrowserSessionQuery.data.data.authenticated)
    ) {
      return;
    }
    const redirectHref = oidcLoginHref;
    if (weComAutoLoginAttemptRef.current === redirectHref) {
      return;
    }
    weComAutoLoginAttemptRef.current = redirectHref;
    globalThis.history.replaceState(
      globalThis.history.state,
      "",
      diagnosisRoomURLWithoutOneShotParams({
        pathname,
        search: searchParams.toString(),
        weComAutoLogin: true,
      }),
    );
    globalThis.location.assign(redirectHref);
  }, [
    authBackendStatusSnapshot,
    diagnosisBrowserSessionQuery.data,
    diagnosisBrowserSessionQuery.isPending,
    pageContext.authMode,
    pageContext.weComAutoLogin,
    oidcLoginHref,
    pathname,
    searchParams,
    visibleWeComAuthError,
  ]);
  useEffect(() => {
    if (authBackendStatusSnapshot === undefined) {
      return;
    }
    const createMode = diagnosisAuthModeFromFieldValue(
      createForm.getFieldValue("authMode"),
    );
    const coercedCreateMode = diagnosisAuthCoercedMode(
      createMode,
      authBackendStatusSnapshot,
    );
    if (coercedCreateMode !== createMode) {
      createForm.setFieldValue("authMode", coercedCreateMode);
    }

    const connectionMode = diagnosisAuthModeFromFieldValue(
      connectionForm.getFieldValue("authMode"),
    );
    const coercedConnectionMode = diagnosisAuthCoercedMode(
      connectionMode,
      authBackendStatusSnapshot,
    );
    if (coercedConnectionMode !== connectionMode) {
      connectionForm.setFieldValue("authMode", coercedConnectionMode);
    }
  }, [authBackendStatusSnapshot, connectionForm, createForm]);
  const operatorEvidenceTemplateError =
    diagnosisToolTemplateQuery.data?.ok === false
      ? diagnosisToolTemplateQuery.data.error.message
      : "";
  const urlSupplementalFollowUp = pageContext.supplementalFollowUp;
  const urlSupplementalFollowUpKey =
    urlSupplementalFollowUp !== undefined
      ? supplementalEvidenceRequestIdentity(urlSupplementalFollowUp)
      : "";
  const pendingSupplementalEvidence =
    manualPendingSupplementalEvidence ??
    (urlSupplementalFollowUp !== undefined &&
    urlSupplementalFollowUpKey !== clearedURLFollowUpKey
      ? urlSupplementalFollowUp
      : null);
  const urlEvidencePlan = pageContext.evidencePlan;
  const urlEvidencePlanKey =
    urlEvidencePlan !== undefined
      ? diagnosisEvidencePlanIdentity(urlEvidencePlan)
      : "";
  const pendingEvidencePlan =
    manualPendingEvidencePlan ??
    (urlEvidencePlan !== undefined &&
    urlEvidencePlanKey !== clearedURLEvidencePlanKey
      ? urlEvidencePlan
      : null);

  function pushLog(level: LogEntry["level"], entryMessage: string) {
    logIDRef.current += 1;
    setLog((current) =>
      [
        { id: logIDRef.current, level, message: entryMessage },
        ...current,
      ].slice(0, 8),
    );
  }

  const ticketMutation = useMutation<
    DiagnosisWSTicketBundle,
    DiagnosisActionError,
    DiagnosisConnectionCredentials
  >({
    mutationFn: async (values) => {
      const result = await issueDiagnosisWSTicket(
        values.authorization,
        values.sessionID,
      );
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
      return result.data;
    },
  });

  const createRoomMutation = useMutation({
    mutationFn: async (values: {
      authorization: DiagnosisAuthorization;
      closeNotificationChannelProfileID?: number;
      evidenceSnapshotID: number;
    }) => {
      const result = await createDiagnosisRoom(
        values.authorization,
        values.evidenceSnapshotID,
        values.closeNotificationChannelProfileID,
      );
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
      return result.data;
    },
  });

  const authCheckMutation = useMutation<
    DiagnosisAuthCheck,
    DiagnosisActionError,
    DiagnosisAuthorization
  >({
    mutationFn: async (authorization) => {
      const result = await checkDiagnosisAuthorization(authorization);
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
      return result.data;
    },
  });

  const createBrowserSessionMutation = useMutation<
    DiagnosisBrowserSessionStatus,
    DiagnosisActionError,
    DiagnosisAuthorization
  >({
    mutationFn: async (authorization) => {
      const result = await createDiagnosisBrowserSession(authorization);
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
      return result.data;
    },
    onSuccess: (session) => {
      const result = {
        data: session,
        ok: true,
      } satisfies ApiResult<DiagnosisBrowserSessionStatus>;
      queryClient.setQueryData(diagnosisRoomBrowserSessionQueryKey, result);
      void queryClient.invalidateQueries({
        queryKey: diagnosisRoomBrowserSessionQueryKey,
      });
    },
  });

  const clearBrowserSessionMutation = useMutation<void, DiagnosisActionError>({
    mutationFn: async () => {
      const result = await clearDiagnosisBrowserSession();
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
    },
    onSuccess: () => {
      autoBrowserSessionAuthCheckAttemptKeyRef.current = "";
      autoBrowserSessionConnectionAttemptKeyRef.current = "";
      autoBrowserSessionCreateRoomAttemptKeyRef.current = "";
      setLastAuthCheck(null);
      const result = {
        data: { authenticated: false },
        ok: true,
      } satisfies ApiResult<DiagnosisBrowserSessionStatus>;
      queryClient.setQueryData(diagnosisRoomBrowserSessionQueryKey, result);
      void queryClient.invalidateQueries({
        queryKey: diagnosisRoomBrowserSessionQueryKey,
      });
      message.success("OpenClarion browser session cleared.");
    },
    onError: (error) => {
      const errorMessage = diagnosisActionErrorMessage(error);
      pushLog(
        "error",
        `Failed to clear OpenClarion browser session: ${errorMessage}`,
      );
      message.error("Failed to clear OpenClarion browser session.");
    },
  });

  const closeUnavailableRoomMutation = useMutation({
    mutationFn: async (values: {
      authorization: DiagnosisAuthorization;
      room: DiagnosisRoomSummary;
    }) => {
      const result = await closeUnavailableDiagnosisRoom(
        values.authorization,
        values.room.session_id,
      );
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
      return result.data;
    },
  });

  const diagnosisNotificationRetryMutation = useMutation<
    DiagnosisNotificationRetryBundle,
    DiagnosisActionError,
    {
      authorization: DiagnosisAuthorization;
      eventKind: DiagnosisNotificationRetryEventKind;
      room: DiagnosisRoomSummary;
    }
  >({
    mutationFn: async (values) => {
      const result = await retryDiagnosisRoomNotification(
        values.authorization,
        values.room.session_id,
        values.eventKind,
      );
      if (!result.ok) {
        throw new DiagnosisActionError(
          result.error.message,
          result.error.status,
        );
      }
      return result.data;
    },
  });

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  const resetSelectedRoomRuntimeState = useCallback(() => {
    manualDisconnectRef.current = true;
    clearReconnectTimer();
    socketGenerationRef.current += 1;
    socketRef.current?.close();
    socketRef.current = null;
    lastConnectionRef.current = null;
    pendingConnectionReturnRef.current = null;
    setSocketOpen(false);
    setStatus("idle");
    setReadySubject("");
    setLocalTurnInFlight(false);
    setConfirmInFlight(false);
    setRoomState(null);
    setTranscript([]);
    setLatestInsight(null);
    setManualPendingSupplementalEvidence(null);
    setManualPendingEvidencePlan(null);
    setServerError(null);
  }, [clearReconnectTimer, setServerError]);

  useEffect(() => {
    if (pageContext.authMode !== undefined) {
      createForm.setFieldValue("authMode", pageContext.authMode);
      connectionForm.setFieldValue("authMode", pageContext.authMode);
    }
    if (pageContext.evidenceSnapshotID !== undefined) {
      createForm.setFieldValue(
        "evidenceSnapshotID",
        pageContext.evidenceSnapshotID,
      );
    }
    if (pageContext.sessionID !== undefined) {
      connectionForm.setFieldValue("sessionID", pageContext.sessionID);
    }
    if (pageContext.suggestedPrompt !== "") {
      const currentMessage = composerForm.getFieldValue("message");
      if (typeof currentMessage !== "string" || currentMessage.trim() === "") {
        composerForm.setFieldValue("message", pageContext.suggestedPrompt);
      }
    }
  }, [
    composerForm,
    connectionForm,
    createForm,
    pageContext.authMode,
    pageContext.evidenceSnapshotID,
    pageContext.sessionID,
    pageContext.suggestedPrompt,
  ]);

  useEffect(() => {
    if (
      selectedSessionIDRef.current !== undefined &&
      selectedSessionIDRef.current !== selectedSessionID
    ) {
      resetSelectedRoomRuntimeState();
    }
    selectedSessionIDRef.current = selectedSessionID;
  }, [resetSelectedRoomRuntimeState, selectedSessionID]);

  useEffect(() => {
    return () => {
      manualDisconnectRef.current = true;
      clearReconnectTimer();
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [clearReconnectTimer]);

  const runtimeStateMatchesSelection =
    roomState?.session_id === selectedSessionID;
  const selectedRoomState = runtimeStateMatchesSelection ? roomState : null;
  const selectedLatestInsight = runtimeStateMatchesSelection
    ? latestInsight
    : null;
  const connected = clientReady && status === "connected" && socketOpen;
  const hasCurrentActorSubject = currentActorSubject.trim() !== "";
  const turnInFlight =
    localTurnInFlight ||
    confirmInFlight ||
    selectedRoomState?.in_flight === true;
  const canSubmitTurn =
    connected &&
    hasCurrentActorSubject &&
    !turnInFlight &&
    selectedParticipateRBACBlockReason === "";
  const canSubmitComposer =
    canSubmitTurn && pendingSupplementalEvidence === null;
  const submitTurnBlockReason = diagnosisSubmitTurnBlockReason({
    actorSubject: currentActorSubject,
    connected,
    rbacBlockReason: selectedParticipateRBACBlockReason,
    turnInFlight,
  });

  function scrollToWorkbenchSection<T extends HTMLElement>(
    section: DiagnosisWorkbenchSection,
    target: RefObject<T | null>,
  ) {
    setWorkbenchSection(section);
    window.requestAnimationFrame(() => {
      target.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    });
  }

  useEffect(() => {
    if (!connected || pendingConnectionReturnRef.current !== "review_queue") {
      return;
    }
    pendingConnectionReturnRef.current = null;
    setWorkbenchSection("insight");
    window.requestAnimationFrame(() => {
      reviewQueuePanelRef.current?.scrollIntoView({
        behavior: "smooth",
        block: "start",
      });
    });
  }, [connected]);

  function showActionBlockReason(reason: string): boolean {
    if (reason === "") {
      return false;
    }
    pushLog("error", reason);
    message.error(reason);
    return true;
  }
  const busy =
    !clientReady || ticketMutation.isPending || status === "connecting";
  const createBusy = createRoomMutation.isPending || busy;
  const authMutationPending =
    authCheckMutation.isPending || createBrowserSessionMutation.isPending;
  const createAuthCheckPending =
    authCheckContext === "create" && authMutationPending;
  const connectionAuthCheckPending =
    authCheckContext === "connection" && authMutationPending;
  const connectDisabledReason =
    selectedRoomSummary?.room_status === "closed"
      ? "Closed diagnosis rooms are read-only. Review the retained conclusion or create a replacement room from the evidence snapshot."
      : "";
  const createAuthValues = diagnosisAuthInputValuesFromWatchedFields({
    authMode: watchedCreateAuthMode,
    bearerToken: watchedCreateBearerToken,
    ldapPassword: watchedCreateLDAPPassword,
    ldapUsername: watchedCreateLDAPUsername,
  });
  const connectionAuthValues = diagnosisAuthInputValuesFromWatchedFields({
    authMode: watchedConnectionAuthMode,
    bearerToken: watchedConnectionBearerToken,
    ldapPassword: watchedConnectionLDAPPassword,
    ldapUsername: watchedConnectionLDAPUsername,
  });
  const createBackendAuthCheckDisabledReason = diagnosisAuthCheckBlockReason({
    backendStatus: authBackendStatusSnapshot,
    values: createAuthValues,
  });
  const createAuthCheckDisabledReason =
    createBackendAuthCheckDisabledReason ||
    browserSessionBlockReason(createAuthValues, "check");
  const connectionBackendAuthCheckDisabledReason =
    diagnosisAuthCheckBlockReason({
      backendStatus: authBackendStatusSnapshot,
      values: connectionAuthValues,
    });
  const connectionAuthCheckDisabledReason =
    connectionBackendAuthCheckDisabledReason ||
    browserSessionBlockReason(connectionAuthValues, "check");
  const createBackendSubmitDisabledReason = diagnosisAuthActionBlockReason({
    action: "create",
    backendStatus: authBackendStatusSnapshot,
    checking: createAuthCheckPending,
    expectedSubject: authenticatedBrowserSessionSubject,
    inputRevision: authCheckRevision.create,
    lastCheck: lastAuthCheck?.context === "create" ? lastAuthCheck : null,
    values: createAuthValues,
  });
  const createSubmitDisabledReason =
    createBackendSubmitDisabledReason ||
    browserSessionBlockReason(createAuthValues, "action");
  const createNotificationChannelBlockReason =
    diagnosisNotificationChannelCreateBlockReason({
      channelID: watchedCreateNotificationChannelID,
      channels: notificationChannels,
      failedToLoad: notificationChannelsQuery.data?.ok === false,
    });
  const createNotificationChannelProofSummary = useMemo(
    () =>
      diagnosisNotificationChannelProofSummary({
        channelID: watchedCreateNotificationChannelID,
        channels: notificationChannels,
        failedToLoad: notificationChannelsQuery.data?.ok === false,
      }),
    [
      notificationChannels,
      notificationChannelsQuery.data,
      watchedCreateNotificationChannelID,
    ],
  );
  const createActionDisabledReason =
    createSubmitDisabledReason !== ""
      ? createSubmitDisabledReason
      : createNotificationChannelBlockReason !== ""
        ? createNotificationChannelBlockReason
        : createRBACBlockReason;
  const connectionBackendAuthBlockReason = diagnosisAuthActionBlockReason({
    action: "connect",
    backendStatus: authBackendStatusSnapshot,
    checking: connectionAuthCheckPending,
    expectedSubject: authenticatedBrowserSessionSubject,
    inputRevision: authCheckRevision.connection,
    lastCheck: lastAuthCheck?.context === "connection" ? lastAuthCheck : null,
    values: connectionAuthValues,
  });
  const connectionAuthBlockReason =
    connectionBackendAuthBlockReason ||
    browserSessionBlockReason(connectionAuthValues, "action");
  const connectSubmitDisabledReason =
    connectDisabledReason !== ""
      ? connectDisabledReason
      : connectionAuthBlockReason !== ""
        ? connectionAuthBlockReason
        : selectedParticipateRBACBlockReason;

  useEffect(() => {
    const sessionResult = diagnosisBrowserSessionQuery.data;
    if (sessionResult?.ok !== true || !sessionResult.data.authenticated) {
      return;
    }
    const plan = diagnosisAutoBrowserSessionAuthCheckPlan({
      authenticatedSubject: sessionResult.data.subject,
      backendStatus: authBackendStatusSnapshot,
      checking: authMutationPending,
      connectionDisabledReason: connectionAuthCheckDisabledReason,
      connectionLastCheck:
        lastAuthCheck?.context === "connection" ? lastAuthCheck : null,
      connectionValues: connectionAuthValues,
      createDisabledReason: createAuthCheckDisabledReason,
      createLastCheck:
        lastAuthCheck?.context === "create" ? lastAuthCheck : null,
      createValues: createAuthValues,
      inputRevisions: authCheckRevision,
      previousAttemptKey: autoBrowserSessionAuthCheckAttemptKeyRef.current,
      selectedSessionID,
    });
    if (plan === null) {
      return;
    }
    const authorization = normalizedDiagnosisAuthorization(
      diagnosisAuthorizationFromFormValues(plan.values),
    );
    if (authorization === null) {
      return;
    }

    autoBrowserSessionAuthCheckAttemptKeyRef.current = plan.attemptKey;
    const attemptKey = plan.attemptKey;
    void Promise.resolve().then(async () => {
      setAuthCheckContext(plan.context);
      try {
        const result = await authCheckMutation.mutateAsync(authorization);
        if (autoBrowserSessionAuthCheckAttemptKeyRef.current !== attemptKey) {
          return;
        }
        setLastAuthCheck({
          checkedAt: result.checked_at,
          context: plan.context,
          inputRevision: plan.inputRevision,
          message: `Authenticated as ${result.subject}.`,
          mode: plan.values.authMode ?? "session",
          roleAuthorized: result.role_authorized,
          roles: result.roles,
          status: "success",
          subject: result.subject,
        });
      } catch (error) {
        if (autoBrowserSessionAuthCheckAttemptKeyRef.current !== attemptKey) {
          return;
        }
        setLastAuthCheck({
          context: plan.context,
          inputRevision: plan.inputRevision,
          message: diagnosisActionErrorMessage(error),
          mode: plan.values.authMode ?? "session",
          roles: [],
          status: "failed",
          subject: "",
        });
      } finally {
        if (autoBrowserSessionAuthCheckAttemptKeyRef.current === attemptKey) {
          setAuthCheckContext(null);
        }
      }
    });
  }, [
    authBackendStatusSnapshot,
    authCheckMutation,
    authCheckRevision,
    authMutationPending,
    connectionAuthCheckDisabledReason,
    connectionAuthValues,
    createAuthCheckDisabledReason,
    createAuthValues,
    diagnosisBrowserSessionQuery.data,
    lastAuthCheck,
    selectedSessionID,
  ]);

  const confirmConclusionBlockReason = diagnosisConfirmConclusionBlockReason({
    actorSubject: currentActorSubject,
    connected,
    confirmInFlight,
    latestInsight: selectedLatestInsight,
    rbacBlockReason: selectedAdministerRBACBlockReason,
    state: selectedRoomState,
  });
  const canConfirmConclusion =
    clientReady && confirmConclusionBlockReason === "";
  const selectedFinalConclusionQueueInput = finalConclusionReviewQueueInput({
    canConfirmConclusion,
    collectionResults: selectedRoomState?.evidence_collection_results ?? [],
    finalConclusion: selectedRoomState?.final_conclusion,
    supplementalEvidence: selectedRoomState?.supplemental_evidence ?? [],
  });
  const selectedStateCollaborationParticipants = useMemo(
    () =>
      diagnosisCollaborationParticipants({
        ownerSubject: selectedRoomState?.owner_subject,
        conversation: selectedRoomState?.conversation ?? [],
        evidenceTimeline:
          selectedLatestInsight !== null
            ? evidenceTimelineForDisplay(selectedLatestInsight)
            : (selectedRoomState?.evidence_timeline ?? []),
        supplementalEvidence: selectedRoomState?.supplemental_evidence ?? [],
        finalConclusion: selectedRoomState?.final_conclusion,
      }),
    [selectedLatestInsight, selectedRoomState],
  );
  const selectedRestCollaborationParticipants = useMemo(
    () =>
      diagnosisCollaborationParticipantsFromSummary(
        selectedRoomSummary?.participants,
      ),
    [selectedRoomSummary?.participants],
  );
  const selectedCollaborationParticipants =
    selectedRoomState === null &&
    selectedRestCollaborationParticipants.length > 0
      ? selectedRestCollaborationParticipants
      : selectedStateCollaborationParticipants;
  const currentAuthorizationDirectoryUsers =
    diagnosisRoomAuthorization.state.kind === "ready"
      ? diagnosisRoomAuthorization.state.directoryUsers
      : undefined;
  const collaborationDirectoryUsersBySubject = useMemo(
    () =>
      diagnosisCollaborationDirectoryIndex(
        [
          ...diagnosisCollaborationDirectoryUsersFromRooms([
            selectedRoomSummary,
            ...(recentRoomsQuery.data?.ok === true
              ? recentRoomsQuery.data.data.items
              : []),
          ]),
          ...diagnosisCollaborationDirectoryUsersFromDirectoryUsers(
            currentAuthorizationDirectoryUsers,
          ),
        ],
      ),
    [
      currentAuthorizationDirectoryUsers,
      recentRoomsQuery.data,
      selectedRoomSummary,
    ],
  );
  const selectedSavedReviewQueueInput =
    selectedLatestInsight === null
      ? diagnosisRoomSummaryReviewQueueInput({
          canConfirmConclusion,
          room: selectedRoomSummary,
        })
      : null;
  const showSelectedFinalConclusionQueue =
    shouldRenderFinalConclusionReviewQueue(selectedFinalConclusionQueueInput);
  const visibleConfirmBlockReason =
    selectedRoomState !== null &&
    selectedRoomState.status !== "closed" &&
    confirmConclusionBlockReason !== ""
      ? confirmConclusionBlockReason
      : "";
  const serverErrorDisplay = diagnosisServerErrorDisplay(serverError);
  const alertSnapshotNeedsRoom =
    alertContext !== undefined &&
    pageContext.evidenceSnapshotID !== undefined &&
    alertContext.snapshot.diagnosis_rooms.length === 0;

  function invalidateDiagnosisRoomQueries(sessionID?: string) {
    return Promise.all(
      diagnosisRoomRefreshQueryKeys(sessionID).map((queryKey) =>
        queryClient.invalidateQueries({ queryKey }),
      ),
    );
  }

  async function handleCheckAuthorization(
    values: AuthFormValues,
    context: AuthCheckContext,
  ) {
    const mode = values.authMode ?? "session";
    const inputRevision = authCheckRevision[context];
    const blockReason =
      diagnosisAuthCheckBlockReason({
        backendStatus: authBackendStatusSnapshot,
        values,
      }) || browserSessionBlockReason(values, "check");
    if (blockReason !== "") {
      pushLog("error", blockReason);
      message.error(blockReason);
      return;
    }
    const authorization = normalizedDiagnosisAuthorization(
      diagnosisAuthorizationFromFormValues(values),
    );
    if (authorization === null) {
      pushLog("error", "Authorization credentials are required.");
      message.error("Authorization credentials are required.");
      return;
    }

    setAuthCheckContext(context);
    try {
      const result = await authCheckMutation.mutateAsync(authorization);
      const feedback = diagnosisAuthCheckSuccessFeedback({
        mode,
        roleAuthorized: result.role_authorized,
        roles: result.roles,
        subject: result.subject,
      });
      setLastAuthCheck({
        checkedAt: result.checked_at,
        context,
        inputRevision,
        message: `Authenticated as ${result.subject}.`,
        mode,
        roleAuthorized: result.role_authorized,
        roles: result.roles,
        status: "success",
        subject: result.subject,
      });
      pushLog(feedback.logLevel, feedback.logMessage);
      message[feedback.toastType](feedback.toastMessage);
      if (authorization.mode === "basic" && authCheckHasUsableRole(result)) {
        await promoteBasicAuthorizationToBrowserSession({
          authorization,
          context,
          inputRevision,
          result,
        });
      }
    } catch (error) {
      if (handleBrowserSessionRejected(error, authorization)) {
        return;
      }
      const errorMessage = diagnosisActionErrorMessage(error);
      setLastAuthCheck({
        context,
        inputRevision,
        message: errorMessage,
        mode,
        roles: [],
        status: "failed",
        subject: "",
      });
      pushLog("error", errorMessage);
      message.error(errorMessage);
    } finally {
      setAuthCheckContext(null);
    }
  }

  function handleUseBrowserSession(
    context: AuthCheckContext,
    authMode: Extract<DiagnosisAuthMode, "session" | "wecom"> = "session",
  ) {
    const form = context === "create" ? createForm : connectionForm;
    form.setFieldValue("authMode", authMode);
    void handleCheckAuthorization(
      {
        ...(form.getFieldsValue(true) as AuthFormValues),
        authMode,
      },
      context,
    );
  }

  async function promoteBasicAuthorizationToBrowserSession({
    authorization,
    context,
    inputRevision,
    result,
  }: {
    authorization: Extract<DiagnosisAuthorization, { mode: "basic" }>;
    context: AuthCheckContext;
    inputRevision: number;
    result: DiagnosisAuthCheck;
  }) {
    try {
      const session =
        await createBrowserSessionMutation.mutateAsync(authorization);
      if (!session.authenticated) {
        pushLog(
          "info",
          "Warning: backend accepted LDAP credentials, but did not return an authenticated browser session.",
        );
        return;
      }
      const form = context === "create" ? createForm : connectionForm;
      form.setFieldsValue({
        authMode: "session",
        ldapPassword: "",
      });
      setLastAuthCheck({
        checkedAt: session.checked_at,
        context,
        inputRevision,
        message: `Authenticated as ${session.subject}.`,
        mode: "session",
        roleAuthorized: session.role_authorized,
        roles: session.roles,
        status: "success",
        subject: session.subject,
      });
      pushLog("info", `Browser session established for ${session.subject}.`);
      message.success(`Browser session established for ${session.subject}.`);
    } catch (error) {
      const errorMessage = diagnosisActionErrorMessage(error);
      pushLog(
        "info",
        `Warning: LDAP auth succeeded, but browser session issuance failed: ${errorMessage}`,
      );
      message.warning(
        "LDAP auth succeeded, but browser session issuance is unavailable.",
      );
      setLastAuthCheck({
        checkedAt: result.checked_at,
        context,
        inputRevision,
        message: `Authenticated as ${result.subject}.`,
        mode: "ldap",
        roleAuthorized: result.role_authorized,
        roles: result.roles,
        status: "success",
        subject: result.subject,
      });
    }
  }

  function clearAuthCheckForContext(context: AuthCheckContext) {
    setAuthCheckRevision((current) => ({
      ...current,
      [context]: current[context] + 1,
    }));
    setLastAuthCheck((current) =>
      current?.context === context ? null : current,
    );
  }

  function clearRejectedBrowserSessionConnectionRuntime() {
    if (lastConnectionRef.current?.authorization.mode !== "session") {
      return;
    }
    manualDisconnectRef.current = true;
    clearReconnectTimer();
    socketGenerationRef.current += 1;
    socketRef.current?.close();
    socketRef.current = null;
    lastConnectionRef.current = null;
    reconnectAttemptRef.current = 0;
    setSocketOpen(false);
    setReadySubject("");
    setConfirmInFlight(false);
    clearTurnInFlight();
    setStatus((current) =>
      current === "idle" || current === "closed" ? current : "error",
    );
  }

  function handleBrowserSessionRejected(
    error: unknown,
    authorization: DiagnosisAuthorization,
  ): boolean {
    if (
      !(error instanceof DiagnosisActionError) ||
      !diagnosisAuthBrowserSessionShouldClearAfterError({
        authMode: authorization.mode,
        status: error.status,
      })
    ) {
      return false;
    }
    autoBrowserSessionAuthCheckAttemptKeyRef.current = "";
    autoBrowserSessionConnectionAttemptKeyRef.current = "";
    autoBrowserSessionCreateRoomAttemptKeyRef.current = "";
    setLastAuthCheck(null);
    clearRejectedBrowserSessionConnectionRuntime();
    const result = {
      data: { authenticated: false },
      ok: true,
    } satisfies ApiResult<DiagnosisBrowserSessionStatus>;
    queryClient.setQueryData(diagnosisRoomBrowserSessionQueryKey, result);
    void queryClient.invalidateQueries({
      queryKey: diagnosisRoomBrowserSessionQueryKey,
    });
    pushLog(
      "error",
      `OpenClarion browser session was rejected by the backend: ${diagnosisActionErrorMessage(error)}`,
    );
    message.warning(
      "OpenClarion browser session was rejected. Sign in again before continuing.",
    );
    return true;
  }

  function scheduleReconnect(reason: string): boolean {
    const decision = diagnosisReconnectDecision({
      attempts: reconnectAttemptRef.current,
      clientReady,
      connection: lastConnectionRef.current,
      maxAttempts: diagnosisReconnectMaxAttempts,
      manualDisconnect: manualDisconnectRef.current,
      timerActive: reconnectTimerRef.current !== null,
    });
    if (decision.kind === "skip") {
      return false;
    }
    if (decision.kind === "already_scheduled") {
      return true;
    }
    if (decision.kind === "exhausted") {
      pushLog("error", `${reason} Reconnect attempts exhausted.`);
      return false;
    }
    reconnectAttemptRef.current = decision.attempt;
    setStatus("connecting");
    pushLog(
      "info",
      `${reason} Reconnecting (${decision.attempt}/${diagnosisReconnectMaxAttempts}).`,
    );
    reconnectTimerRef.current = setTimeout(() => {
      reconnectTimerRef.current = null;
      void handleConnect(decision.connection, {
        preserveState: true,
        reconnect: true,
      });
    }, diagnosisReconnectDelayMS);
    return true;
  }

  function browserSessionBlockReason(
    values: AuthFormValues,
    intent: "action" | "check",
  ): string {
    return diagnosisAuthBrowserSessionBlockReason({
      intent,
      sessionAuthenticated:
        diagnosisBrowserSessionQuery.data?.ok === true &&
        diagnosisBrowserSessionQuery.data.data.authenticated,
      sessionLoading: diagnosisBrowserSessionQuery.isPending,
      sessionMode:
        diagnosisBrowserSessionQuery.data?.ok === true &&
        diagnosisBrowserSessionQuery.data.data.authenticated
          ? diagnosisBrowserSessionQuery.data.data.mode
          : undefined,
      sessionStatusAvailable:
        diagnosisBrowserSessionQuery.data === undefined
          ? diagnosisBrowserSessionQuery.isPending
          : diagnosisBrowserSessionQuery.data.ok,
      values,
    });
  }

  function authCheckBlockReasonForContext(
    values: AuthFormValues,
    context: AuthCheckContext,
  ): string {
    return (
      diagnosisAuthActionBlockReason({
        action: context === "create" ? "create" : "connect",
        backendStatus: authBackendStatusSnapshot,
        checking:
          authCheckContext === context &&
          (authCheckMutation.isPending ||
            createBrowserSessionMutation.isPending),
        expectedSubject: authenticatedBrowserSessionSubject,
        inputRevision: authCheckRevision[context],
        lastCheck: lastAuthCheck?.context === context ? lastAuthCheck : null,
        values,
      }) || browserSessionBlockReason(values, "action")
    );
  }

  function replaceRoomURL(sessionID: string, evidenceSnapshotID: number) {
    const keepReportContext =
      pageContext.evidenceSnapshotID === evidenceSnapshotID;
    router.replace(
      diagnosisRoomURLWithSelectedRoom({
        evidenceSnapshotID,
        keepReportContext,
        pathname,
        search: searchParams.toString(),
        sessionID,
      }) as Route,
      {
        scroll: false,
      },
    );
  }

  function replaceRoomURLWithoutOneShotParams(options: {
    authError?: boolean;
    evidencePlan?: boolean;
    supplementalFollowUp?: boolean;
  }) {
    if (
      !options.authError &&
      !options.evidencePlan &&
      !options.supplementalFollowUp
    ) {
      return;
    }
    router.replace(
      diagnosisRoomURLWithoutOneShotParams({
        authError: options.authError,
        evidencePlan: options.evidencePlan,
        pathname,
        search: searchParams.toString(),
        supplementalFollowUp: options.supplementalFollowUp,
      }) as Route,
      {
        scroll: false,
      },
    );
  }

  async function handleConnect(
    values: ConnectionFormValues | DiagnosisConnectionCredentials,
    options: ConnectOptions = {},
  ) {
    const authBlockReason =
      "authorization" in values
        ? ""
        : authCheckBlockReasonForContext(values, "connection");
    if (authBlockReason !== "") {
      pushLog("error", authBlockReason);
      message.error(authBlockReason);
      setStatus("error");
      return;
    }
    const credentials = normalizedConnectionCredentials(values);
    if (credentials === null) {
      pushLog("error", "Session and authorization credentials are required.");
      setStatus("error");
      return;
    }

    clearReconnectTimer();
    manualDisconnectRef.current = false;
    lastConnectionRef.current = credentials;
    if (!options.reconnect) {
      reconnectAttemptRef.current = 0;
    }
    const socketGeneration = socketGenerationRef.current + 1;
    socketGenerationRef.current = socketGeneration;
    socketRef.current?.close();
    socketRef.current = null;
    setSocketOpen(false);
    setStatus("ticketing");
    if (!options.preserveState) {
      setReadySubject("");
      setLocalTurnInFlight(false);
      setConfirmInFlight(false);
      setRoomState(null);
      setTranscript([]);
      setLatestInsight(null);
      setManualPendingSupplementalEvidence(null);
      setServerError(null);
    }
    ticketMutation.reset();
    pushLog(
      "info",
      options.reconnect
        ? "Reconnecting diagnosis room WebSocket."
        : "Requesting WebSocket ticket.",
    );

    let ticket: DiagnosisWSTicketBundle;
    try {
      ticket = await ticketMutation.mutateAsync({
        authorization: credentials.authorization,
        sessionID: credentials.sessionID,
      });
    } catch (error) {
      setStatus("error");
      if (handleBrowserSessionRejected(error, credentials.authorization)) {
        return;
      }
      pushLog("error", diagnosisActionErrorMessage(error));
      return;
    }

    const selectedEvidenceSnapshotID =
      createForm.getFieldValue("evidenceSnapshotID");
    if (
      !options.reconnect &&
      isPositiveSafeInteger(selectedEvidenceSnapshotID)
    ) {
      replaceRoomURL(credentials.sessionID, selectedEvidenceSnapshotID);
    }

    setStatus("connecting");
    const socket = new WebSocket(ticket.websocket_url);
    socketRef.current = socket;

    socket.onopen = () => {
      if (socketGenerationRef.current !== socketGeneration) {
        return;
      }
      setSocketOpen(true);
    };
    socket.onmessage = (messageEvent: MessageEvent<string>) => {
      if (socketGenerationRef.current !== socketGeneration) {
        return;
      }
      try {
        handleServerFrame(parseDiagnosisServerFrame(messageEvent.data));
      } catch (error) {
        pushLog(
          "error",
          error instanceof Error ? error.message : "Invalid diagnosis frame.",
        );
      }
    };
    socket.onerror = () => {
      if (socketGenerationRef.current !== socketGeneration) {
        return;
      }
      setSocketOpen(false);
      if (!scheduleReconnect("WebSocket error.")) {
        setStatus("error");
        setConfirmInFlight(false);
        clearTurnInFlight();
        pushLog("error", "WebSocket error.");
      }
    };
    socket.onclose = () => {
      if (socketGenerationRef.current !== socketGeneration) {
        return;
      }
      setSocketOpen(false);
      if (!scheduleReconnect("WebSocket closed.")) {
        setStatus((current) => (current === "error" ? current : "closed"));
        setConfirmInFlight(false);
        clearTurnInFlight();
        pushLog("info", "WebSocket closed.");
      }
    };
  }

  async function handleCreateRoom(values: CreateRoomFormValues) {
    const authBlockReason = authCheckBlockReasonForContext(values, "create");
    if (authBlockReason !== "") {
      pushLog("error", authBlockReason);
      message.error(authBlockReason);
      setStatus("error");
      return;
    }
    const authorization = normalizedDiagnosisAuthorization(
      diagnosisAuthorizationFromFormValues(values),
    );
    const evidenceSnapshotID = values.evidenceSnapshotID;
    const closeNotificationChannelProfileID =
      values.closeNotificationChannelProfileID ?? undefined;
    const notificationChannelBlockReason =
      diagnosisNotificationChannelCreateBlockReason({
        channelID: closeNotificationChannelProfileID,
        channels: notificationChannels,
        failedToLoad: notificationChannelsQuery.data?.ok === false,
      });
    if (!isPositiveSafeInteger(evidenceSnapshotID) || authorization === null) {
      pushLog(
        "error",
        "Evidence snapshot and authorization credentials are required.",
      );
      setStatus("error");
      return;
    }
    if (notificationChannelBlockReason !== "") {
      pushLog("error", notificationChannelBlockReason);
      message.error(notificationChannelBlockReason);
      setStatus("error");
      return;
    }
    if (
      closeNotificationChannelProfileID !== undefined &&
      !isPositiveSafeInteger(closeNotificationChannelProfileID)
    ) {
      pushLog(
        "error",
        "Notification channel must be a positive integer when provided.",
      );
      setStatus("error");
      return;
    }
    const notificationChannelError = diagnosisNotificationChannelSelectionError(
      closeNotificationChannelProfileID,
      notificationChannels,
    );
    if (notificationChannelError !== "") {
      pushLog("error", notificationChannelError);
      setStatus("error");
      return;
    }
    if (createRBACBlockReason !== "") {
      pushLog("error", createRBACBlockReason);
      message.error(createRBACBlockReason);
      setStatus("error");
      return;
    }

    setStatus("ticketing");
    const notificationChannelSuffix =
      closeNotificationChannelProfileID === undefined
        ? ""
        : ` with notification channel #${closeNotificationChannelProfileID}`;
    pushLog(
      "info",
      `Creating diagnosis room from evidence snapshot #${evidenceSnapshotID}${notificationChannelSuffix}.`,
    );
    let room: DiagnosisRoomCreateBundle;
    try {
      room = await createRoomMutation.mutateAsync({
        authorization,
        closeNotificationChannelProfileID,
        evidenceSnapshotID,
      });
    } catch (error) {
      setStatus("error");
      if (handleBrowserSessionRejected(error, authorization)) {
        return;
      }
      pushLog("error", diagnosisActionErrorMessage(error));
      return;
    }

    message.success("Diagnosis room created.");
    const connectionCredentials = {
      authorization,
      sessionID: room.session_id,
    };
    connectionForm.setFieldsValue(
      connectionFormValuesFromCredentials(connectionCredentials),
    );
    clearAuthCheckForContext("connection");
    setConnectionSessionID(room.session_id);
    replaceRoomURL(room.session_id, room.evidence_snapshot_id);
    pushLog(
      "info",
      `Created ${room.session_id} from snapshot #${room.evidence_snapshot_id}.`,
    );
    void invalidateDiagnosisRoomQueries(room.session_id);
    await handleConnect(connectionCredentials);
  }

  useEffect(() => {
    const sessionResult = diagnosisBrowserSessionQuery.data;
    if (
      sessionResult?.ok !== true ||
      !sessionResult.data.authenticated ||
      createRoomMutation.isPending ||
      status !== "idle"
    ) {
      return;
    }
    const plan = diagnosisAutoBrowserSessionCreateRoomPlan({
      authenticatedSubject: sessionResult.data.subject,
      backendStatus: authBackendStatusSnapshot,
      closeNotificationChannelProfileID: watchedCreateNotificationChannelID,
      createDisabledReason: createActionDisabledReason,
      evidenceSnapshotID: pageContext.evidenceSnapshotID,
      inputRevision: authCheckRevision.create,
      lastCheck: lastAuthCheck?.context === "create" ? lastAuthCheck : null,
      previousAttemptKey: autoBrowserSessionCreateRoomAttemptKeyRef.current,
      selectedSessionID,
      snapshotNeedsRoom: alertSnapshotNeedsRoom,
      values: createAuthValues,
    });
    if (plan === null) {
      return;
    }
    autoBrowserSessionCreateRoomAttemptKeyRef.current = plan.attemptKey;
    void handleCreateRoom({
      authMode: createAuthValues.authMode,
      bearerToken: createAuthValues.bearerToken,
      closeNotificationChannelProfileID: plan.closeNotificationChannelProfileID,
      evidenceSnapshotID: plan.evidenceSnapshotID,
      ldapPassword: createAuthValues.ldapPassword,
      ldapUsername: createAuthValues.ldapUsername,
    });
    // handleCreateRoom is guarded by status, auth verification, and attempt key;
    // including it would retrigger this effect on every render of the large view.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    alertSnapshotNeedsRoom,
    authBackendStatusSnapshot,
    authCheckRevision.create,
    createActionDisabledReason,
    createAuthValues,
    createRoomMutation.isPending,
    diagnosisBrowserSessionQuery.data,
    lastAuthCheck,
    pageContext.evidenceSnapshotID,
    selectedSessionID,
    status,
    watchedCreateNotificationChannelID,
  ]);

  useEffect(() => {
    const sessionResult = diagnosisBrowserSessionQuery.data;
    if (sessionResult?.ok !== true || !sessionResult.data.authenticated) {
      return;
    }
    const plan = diagnosisAutoBrowserSessionConnectionPlan({
      authenticatedSubject: sessionResult.data.subject,
      backendStatus: authBackendStatusSnapshot,
      connectionDisabledReason: connectSubmitDisabledReason,
      connectionStatus: status,
      inputRevision: authCheckRevision.connection,
      lastCheck: lastAuthCheck?.context === "connection" ? lastAuthCheck : null,
      manualDisconnected: manualDisconnectRef.current,
      previousAttemptKey: autoBrowserSessionConnectionAttemptKeyRef.current,
      selectedSessionID,
      values: connectionAuthValues,
    });
    if (plan === null) {
      return;
    }
    const authorization = normalizedDiagnosisAuthorization({
      mode: "session",
    });
    if (authorization === null) {
      return;
    }
    autoBrowserSessionConnectionAttemptKeyRef.current = plan.attemptKey;
    void handleConnect({
      authorization,
      sessionID: plan.sessionID,
    });
    // handleConnect is guarded by connection status and attempt key; including it
    // would re-evaluate this effect on every render of the large diagnosis view.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    authBackendStatusSnapshot,
    authCheckRevision.connection,
    connectSubmitDisabledReason,
    connectionAuthValues,
    diagnosisBrowserSessionQuery.data,
    lastAuthCheck,
    selectedSessionID,
    status,
  ]);

  function handleServerFrame(frame: DiagnosisServerFrame) {
    switch (frame.type) {
      case "ready":
        reconnectAttemptRef.current = 0;
        setServerError(null);
        setStatus("connected");
        setReadySubject(frame.subject);
        pushLog("info", `Connected as ${frame.subject}.`);
        sendFrame({ type: "query_state" });
        break;
      case "state":
        if (frame.latest_error && !frame.in_flight) {
          setStatus("connected");
          setServerError({
            code: frame.latest_error.code,
            message: frame.latest_error.message,
          });
          if (
            serverError?.code !== frame.latest_error.code ||
            serverError.message !== frame.latest_error.message
          ) {
            pushLog(
              "error",
              `${frame.latest_error.code}: ${frame.latest_error.message}`,
            );
          }
        } else {
          setServerError(null);
          setStatus((current) => (current === "error" ? "connected" : current));
        }
        setConfirmInFlight(false);
        setLocalTurnInFlight(frame.in_flight);
        setRoomState(frame);
        setTranscript(diagnosisStateTranscript(frame));
        setLatestInsight(latestConsultationInsightFromState(frame));
        void invalidateDiagnosisRoomQueries(frame.session_id);
        pushLog(
          "info",
          `Loaded state: ${frame.status}, ${frame.turn_count} turn(s).`,
        );
        if ((frame.follow_up_turns ?? []).length > 0) {
          pushLog(
            "info",
            `Auto evidence follow-up completed ${(frame.follow_up_turns ?? []).length} turn(s).`,
          );
        }
        break;
      case "turn_result":
        if (frame.latest_error) {
          setServerError({
            code: frame.latest_error.code,
            message: frame.latest_error.message,
          });
          pushLog(
            "error",
            `${frame.latest_error.code}: ${frame.latest_error.message}`,
          );
        } else {
          setServerError(null);
        }
        clearTurnInFlight();
        setLatestInsight(latestConsultationInsight(frame));
        setRoomState((current) =>
          current
            ? {
                ...current,
                status: frame.status,
                turn_count: latestDiagnosisTurnCount(frame),
                in_flight: false,
                latest_error: frame.latest_error,
              }
            : current,
        );
        setTranscript((current) => [
          ...current,
          ...diagnosisTurnResultTranscript(frame),
        ]);
        void invalidateDiagnosisRoomQueries(frame.session_id);
        pushLog("info", `Turn ${latestDiagnosisTurnCount(frame)} completed.`);
        if ((frame.follow_up_turns ?? []).length > 0) {
          pushLog(
            "info",
            `Auto evidence follow-up completed ${(frame.follow_up_turns ?? []).length} turn(s).`,
          );
        }
        if (shouldQueryDiagnosisStateAfterTurn(frame)) {
          sendFrame({ type: "query_state" });
        }
        break;
      case "error":
        setStatus((current) => (current === "connected" ? current : "error"));
        setConfirmInFlight(false);
        clearTurnInFlight();
        setServerError({ code: frame.code, message: frame.message });
        pushLog("error", `${frame.code}: ${frame.message}`);
        break;
    }
  }

  function handleSend(values: ComposerValues) {
    const trimmed = values.message.trim();
    if (showActionBlockReason(submitTurnBlockReason) || trimmed === "") {
      return;
    }
    if (!canSubmitComposer) {
      return;
    }
    const messageID = nextDiagnosisMessageID();
    setServerError(null);
    const frame: DiagnosisClientFrame = {
      type: "submit_turn",
      message_id: messageID,
      message: trimmed,
    };
    if (!sendFrame(frame)) {
      return;
    }
    markTurnInFlight();
    setTranscript((current) => [
      ...current,
      {
        id: messageID,
        role: "user",
        actor_subject: currentActorSubject,
        content: trimmed,
      },
    ]);
    setManualPendingSupplementalEvidence(null);
    setManualPendingEvidencePlan(null);
    if (urlSupplementalFollowUpKey !== "") {
      setClearedURLFollowUpKey(urlSupplementalFollowUpKey);
    }
    replaceRoomURLWithoutOneShotParams({
      supplementalFollowUp: urlSupplementalFollowUpKey !== "",
    });
    composerForm.resetFields();
  }

  function handleSubmitSupplementalEvidence(
    values: SupplementalEvidenceFormValues,
  ) {
    const request = pendingSupplementalEvidence;
    const evidence = values.evidence.trim();
    if (showActionBlockReason(submitTurnBlockReason)) {
      return;
    }
    if (!canSubmitTurn || request === null || evidence === "") {
      return;
    }
    const messageID = nextDiagnosisMessageID();
    const outboundMessage = supplementalEvidenceSubmissionMessage(
      request,
      evidence,
    );
    if (
      !sendFrame({
        type: "submit_supplemental_evidence",
        message_id: messageID,
        message: outboundMessage,
        supplemental_evidence: supplementalEvidenceWirePayload(
          request,
          evidence,
        ),
      })
    ) {
      return;
    }
    markTurnInFlight();
    setTranscript((current) => [
      ...current,
      {
        id: messageID,
        role: "user",
        actor_subject: currentActorSubject,
        content: outboundMessage,
      },
    ]);
    setManualPendingSupplementalEvidence(null);
    setManualPendingEvidencePlan(null);
    if (urlSupplementalFollowUpKey !== "") {
      setClearedURLFollowUpKey(urlSupplementalFollowUpKey);
    }
    if (urlEvidencePlanKey !== "") {
      setClearedURLEvidencePlanKey(urlEvidencePlanKey);
    }
    replaceRoomURLWithoutOneShotParams({
      evidencePlan: urlEvidencePlanKey !== "",
      supplementalFollowUp: urlSupplementalFollowUpKey !== "",
    });
    supplementalEvidenceForm.resetFields();
    composerForm.resetFields();
    pushLog("info", `Submitted supplemental evidence for ${request.label}.`);
  }

  function handleQueryState() {
    if (!connected) {
      return;
    }
    if (selectedReadRBACBlockReason !== "") {
      showActionBlockReason(selectedReadRBACBlockReason);
      return;
    }
    sendFrame({ type: "query_state" });
  }

  function handleConfirmConclusion() {
    if (showActionBlockReason(confirmConclusionBlockReason)) {
      return;
    }
    if (!canConfirmConclusion) {
      return;
    }
    setServerError(null);
    pushLog("info", "Confirming final conclusion.");
    setConfirmInFlight(true);
    if (!sendFrame({ type: "confirm_conclusion", reason: "human_confirmed" })) {
      setConfirmInFlight(false);
    }
  }

  function handleUseSupplementalEvidence(
    request: DiagnosisConsultationEvidenceRequest,
  ) {
    setManualPendingSupplementalEvidence(request);
    setManualPendingEvidencePlan(null);
    setClearedURLFollowUpKey(null);
    supplementalEvidenceForm.resetFields();
    composerForm.resetFields();
    setWorkbenchSection("conversation");
    window.requestAnimationFrame(() => {
      supplementalEvidencePanelRef.current?.scrollIntoView({
        behavior: "smooth",
        block: "start",
      });
    });
    pushLog(
      "info",
      `Prepared supplemental evidence follow-up for ${request.label}.`,
    );
    message.info("Supplemental evidence follow-up prepared.");
  }

  function handleReviewEvidenceTasks() {
    setWorkbenchSection("insight");
    window.requestAnimationFrame(() => {
      reviewQueuePanelRef.current?.scrollIntoView({
        behavior: "smooth",
        block: "start",
      });
    });
  }

  function handleOpenConnectionControls(
    options: { returnToReviewQueue?: boolean } = {},
  ) {
    if (options.returnToReviewQueue === true) {
      pendingConnectionReturnRef.current = "review_queue";
    }
    const targetSessionID = diagnosisConnectionTargetSessionID({
      formSessionID: connectionForm.getFieldValue("sessionID"),
      selectedRoomSessionID: selectedRoomSummary?.session_id,
      selectedSessionID,
    });
    if (targetSessionID !== "") {
      const currentSessionID =
        connectionForm.getFieldValue("sessionID")?.trim() ?? "";
      if (currentSessionID !== targetSessionID) {
        connectionForm.setFieldValue("sessionID", targetSessionID);
        clearAuthCheckForContext("connection");
      }
      setConnectionSessionID(targetSessionID);
    }
    scrollToWorkbenchSection("setup", connectionPanelRef);
  }

  function handleUseEvidencePlan(request: DiagnosisEvidenceRequest) {
    if (
      !canSubmitTurn &&
      diagnosisReviewQueueConnectionGateAllowsPreparation({
        actionDisabledReason: submitTurnBlockReason,
        connected,
      })
    ) {
      setManualPendingEvidencePlan(request);
      setManualPendingSupplementalEvidence(null);
      supplementalEvidenceForm.resetFields();
      handleOpenConnectionControls({ returnToReviewQueue: true });
      pushLog("info", `Staged planned evidence for ${request.tool}.`);
      message.info("Evidence plan staged. Connect before collecting evidence.");
      return;
    }
    if (showActionBlockReason(submitTurnBlockReason)) {
      return;
    }
    if (!canSubmitTurn) {
      return;
    }
    const messageID = nextDiagnosisMessageID();
    const outboundMessage = evidencePlanFollowUpMessage(request);
    setServerError(null);
    if (
      !sendFrame({
        type: "collect_evidence",
        message_id: messageID,
        message: outboundMessage,
        evidence_requests: [request],
      })
    ) {
      return;
    }
    setManualPendingSupplementalEvidence(null);
    setManualPendingEvidencePlan(null);
    if (urlSupplementalFollowUpKey !== "") {
      setClearedURLFollowUpKey(urlSupplementalFollowUpKey);
    }
    if (urlEvidencePlanKey !== "") {
      setClearedURLEvidencePlanKey(urlEvidencePlanKey);
    }
    replaceRoomURLWithoutOneShotParams({
      evidencePlan: urlEvidencePlanKey !== "",
      supplementalFollowUp: urlSupplementalFollowUpKey !== "",
    });
    supplementalEvidenceForm.resetFields();
    markTurnInFlight();
    setTranscript((current) => [
      ...current,
      {
        id: messageID,
        role: "user",
        actor_subject: currentActorSubject,
        content: outboundMessage,
      },
    ]);
    pushLog("info", `Collecting planned evidence for ${request.tool}.`);
    message.info("Collecting planned evidence.");
  }

  function handleRequestAIReassessment() {
    if (showActionBlockReason(submitTurnBlockReason)) {
      return;
    }
    if (!canSubmitTurn) {
      return;
    }
    const messageID = nextDiagnosisMessageID();
    const savedReassessmentInput =
      selectedSavedReviewQueueInput === null
        ? undefined
        : diagnosisReviewQueueReassessmentInput(selectedSavedReviewQueueInput);
    const outboundMessage = supplementalEvidenceReassessmentMessage({
      collectionResults:
        selectedLatestInsight?.collectionResults ??
        selectedRoomState?.evidence_collection_results ??
        savedReassessmentInput?.collectionResults ??
        [],
      latestAssistantSequence:
        selectedLatestInsight?.assistantSequence ??
        selectedRoomState?.final_conclusion?.assistant_sequence ??
        savedReassessmentInput?.latestAssistantSequence,
      records:
        selectedRoomState?.supplemental_evidence ??
        savedReassessmentInput?.records ??
        [],
    });
    setServerError(null);
    if (
      !sendFrame({
        type: "submit_turn",
        message_id: messageID,
        message: outboundMessage,
      })
    ) {
      return;
    }
    markTurnInFlight();
    setTranscript((current) => [
      ...current,
      {
        id: messageID,
        role: "user",
        actor_subject: currentActorSubject,
        content: outboundMessage,
      },
    ]);
    pushLog("info", "Requested AI reassessment of submitted evidence.");
    message.info("AI reassessment requested.");
  }

  function handleCollectOperatorEvidence(values: OperatorEvidenceFormValues) {
    if (showActionBlockReason(submitTurnBlockReason)) {
      return;
    }
    if (!canSubmitTurn) {
      return;
    }

    let request: DiagnosisEvidenceRequest;
    try {
      validateOperatorSelectedTemplate(values, operatorEvidenceTemplates);
      request = operatorEvidenceRequest(values);
    } catch (error) {
      const detail =
        error instanceof Error
          ? error.message
          : "Operator evidence request is invalid.";
      pushLog("error", detail);
      message.error(detail);
      return;
    }

    const messageID = nextDiagnosisMessageID();
    const outboundMessage = operatorEvidenceFollowUpMessage(request);
    setServerError(null);
    if (
      !sendFrame({
        type: "collect_evidence",
        message_id: messageID,
        message: outboundMessage,
        evidence_requests: [request],
      })
    ) {
      return;
    }
    setManualPendingSupplementalEvidence(null);
    setManualPendingEvidencePlan(null);
    if (urlSupplementalFollowUpKey !== "") {
      setClearedURLFollowUpKey(urlSupplementalFollowUpKey);
    }
    if (urlEvidencePlanKey !== "") {
      setClearedURLEvidencePlanKey(urlEvidencePlanKey);
    }
    replaceRoomURLWithoutOneShotParams({
      evidencePlan: urlEvidencePlanKey !== "",
      supplementalFollowUp: urlSupplementalFollowUpKey !== "",
    });
    supplementalEvidenceForm.resetFields();
    markTurnInFlight();
    setTranscript((current) => [
      ...current,
      {
        id: messageID,
        role: "user",
        actor_subject: currentActorSubject,
        content: outboundMessage,
      },
    ]);
    pushLog("info", `Collecting operator evidence for ${request.tool}.`);
    message.info("Collecting operator evidence.");
  }

  function handleClearSupplementalEvidence() {
    setManualPendingSupplementalEvidence(null);
    supplementalEvidenceForm.resetFields();
    if (urlSupplementalFollowUpKey !== "") {
      setClearedURLFollowUpKey(urlSupplementalFollowUpKey);
    }
    replaceRoomURLWithoutOneShotParams({
      supplementalFollowUp: urlSupplementalFollowUpKey !== "",
    });
    pushLog("info", "Cleared supplemental evidence follow-up.");
  }

  function handleClearEvidencePlan() {
    setManualPendingEvidencePlan(null);
    if (urlEvidencePlanKey !== "") {
      setClearedURLEvidencePlanKey(urlEvidencePlanKey);
    }
    replaceRoomURLWithoutOneShotParams({
      evidencePlan: urlEvidencePlanKey !== "",
    });
    pushLog("info", "Cleared URL evidence plan.");
  }

  function handleDisconnect() {
    manualDisconnectRef.current = true;
    clearReconnectTimer();
    socketGenerationRef.current += 1;
    socketRef.current?.close();
    socketRef.current = null;
    pendingConnectionReturnRef.current = null;
    setSocketOpen(false);
    setStatus("closed");
    setConfirmInFlight(false);
    clearTurnInFlight();
  }

  function handleSelectRoom(room: DiagnosisRoomSummary) {
    resetSelectedRoomRuntimeState();
    setConnectionSessionID(room.session_id);
    connectionForm.setFieldValue("sessionID", room.session_id);
    createForm.setFieldValue("evidenceSnapshotID", room.evidence_snapshot_id);
    replaceRoomURL(room.session_id, room.evidence_snapshot_id);
    scrollToWorkbenchSection(
      room.room_status === "closed" ? "room" : "setup",
      room.room_status === "closed" ? roomStatePanelRef : connectionPanelRef,
    );
    pushLog("info", `Selected ${room.session_id}.`);
    message.info("Diagnosis room selected.");
  }

  function handlePrepareAlertRoom() {
    if (pageContext.evidenceSnapshotID !== undefined) {
      createForm.setFieldValue(
        "evidenceSnapshotID",
        pageContext.evidenceSnapshotID,
      );
    }
    if (pageContext.suggestedPrompt !== "") {
      composerForm.setFieldValue("message", pageContext.suggestedPrompt);
    }
    scrollToWorkbenchSection("setup", createRoomPanelRef);
    pushLog("info", "Prepared alert snapshot for diagnosis room creation.");
    message.info(
      "Enter authorization credentials to create the diagnosis room.",
    );
  }

  function handlePrepareHandoffRoom(item: DiagnosisHandoffBacklogItem) {
    const evidenceSnapshotID = item.evidence_snapshot.id;
    createForm.setFieldValue("evidenceSnapshotID", evidenceSnapshotID);
    const alert = item.alerts[0];
    if (alert !== undefined) {
      composerForm.setFieldValue(
        "message",
        diagnosisSuggestedPrompt({
          alertContext: { alert, snapshot: item.evidence_snapshot },
          contextRef: `evidence snapshot #${evidenceSnapshotID}`,
          intent: "confidence_review",
        }),
      );
    }
    scrollToWorkbenchSection("setup", createRoomPanelRef);
    pushLog(
      "info",
      `Prepared handoff snapshot #${evidenceSnapshotID} for diagnosis room creation.`,
    );
    message.info(
      "Enter authorization credentials to create the diagnosis room.",
    );
  }

  async function handlePrepareRoomRebuild(room: DiagnosisRoomSummary) {
    const administerBlockReason = diagnosisRoomAdministerBlockReason(
      room.session_id,
    );
    if (administerBlockReason !== "") {
      pushLog("error", administerBlockReason);
      message.error(administerBlockReason);
      return;
    }
    createForm.setFieldValue("evidenceSnapshotID", room.evidence_snapshot_id);
    scrollToWorkbenchSection("setup", createRoomPanelRef);
    pushLog(
      "info",
      `Prepared replacement room from evidence snapshot #${room.evidence_snapshot_id}.`,
    );
    const authorization = normalizedDiagnosisAuthorization(
      diagnosisAuthorizationFromFormValues(connectionForm.getFieldsValue()),
    );
    if (authorization === null) {
      message.warning(
        "Evidence snapshot prepared. Enter authorization credentials to close the unavailable room before rebuilding.",
      );
      return;
    }
    setClosingUnavailableSessionID(room.session_id);
    try {
      const closedRoom = await closeUnavailableRoomMutation.mutateAsync({
        authorization,
        room,
      });
      pushLog(
        "info",
        `Closed unavailable diagnosis room ${closedRoom.session_id}.`,
      );
      message.success(
        "Unavailable room closed. Create a replacement room with the prepared evidence snapshot.",
      );
      void invalidateDiagnosisRoomQueries(room.session_id);
    } catch (error) {
      pushLog("error", diagnosisActionErrorMessage(error));
      message.error(diagnosisActionErrorMessage(error));
    } finally {
      setClosingUnavailableSessionID("");
    }
  }

  async function handleRetryNotification(
    entry: DiagnosisRoomNotificationTimelineEntry,
    room: DiagnosisRoomSummary | undefined = selectedRoomSummary,
  ) {
    if (room === undefined) {
      return;
    }
    const administerBlockReason = diagnosisRoomAdministerBlockReason(
      room.session_id,
    );
    if (administerBlockReason !== "") {
      pushLog("error", administerBlockReason);
      message.error(administerBlockReason);
      return;
    }
    const authorization = normalizedDiagnosisAuthorization(
      diagnosisAuthorizationFromFormValues(connectionForm.getFieldsValue()),
    );
    if (authorization === null) {
      const detail =
        "Authorization credentials are required before retrying a notification.";
      pushLog("error", detail);
      message.error(detail);
      return;
    }
    if (!isDiagnosisNotificationRetryEventKind(entry.event_kind)) {
      const detail = "Notification event kind cannot be retried.";
      pushLog("error", detail);
      message.error(detail);
      return;
    }
    const retryKey = notificationTimelineRetryKey(entry, room.session_id);
    setRetryingNotificationKey(retryKey);
    pushLog(
      "info",
      `Retrying ${notificationEventLabel(entry.event_kind)} for ${room.session_id}.`,
    );
    try {
      const result = await diagnosisNotificationRetryMutation.mutateAsync({
        authorization,
        eventKind: entry.event_kind,
        room,
      });
      const alreadyDelivered =
        result.retry_state === "already_delivered"
          ? " The notification was already delivered by a later attempt."
          : "";
      pushLog(
        "info",
        `Notification retry result: ${result.retry_state}.${alreadyDelivered}`,
      );
      message.success(
        result.retry_state === "already_delivered"
          ? "Notification already delivered."
          : "Notification retry sent.",
      );
      void invalidateDiagnosisRoomQueries(room.session_id);
    } catch (error) {
      pushLog("error", diagnosisActionErrorMessage(error));
      message.error(diagnosisActionErrorMessage(error));
    } finally {
      setRetryingNotificationKey("");
    }
  }

  function handleUseSuggestedPrompt() {
    if (pageContext.suggestedPrompt === "") {
      return;
    }
    composerForm.setFieldValue("message", pageContext.suggestedPrompt);
    pushLog("info", "Prepared suggested diagnosis prompt.");
    message.info("Suggested prompt prepared.");
  }

  function sendFrame(frame: DiagnosisClientFrame): boolean {
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      pushLog("error", "WebSocket is not connected.");
      return false;
    }
    socket.send(JSON.stringify(frame));
    return true;
  }

  function markTurnInFlight() {
    setLocalTurnInFlight(true);
    setRoomState((current) =>
      current
        ? { ...current, in_flight: true, latest_error: undefined }
        : current,
    );
    pushLog("info", "AI review in progress.");
  }

  function clearTurnInFlight() {
    setLocalTurnInFlight(false);
    setRoomState((current) =>
      current && current.in_flight ? { ...current, in_flight: false } : current,
    );
  }

  const roomStateItems = roomStateDescriptionItems(
    selectedRoomState,
    readySubject,
    connectionForm.getFieldValue("sessionID"),
    status,
    collaborationDirectoryUsersBySubject,
  );
  const roomPermissionItems = diagnosisRoomRBACPermissionItems({
    can: diagnosisRoomAuthorization.can,
    checking: diagnosisRoomAuthorizationChecking,
    closeNotificationChannelProfileID: watchedCreateNotificationChannelID,
    enforced: diagnosisRoomAuthorizationEnforced,
    sessionID: selectedSessionID,
  });
  const selectedRetainedConclusion =
    selectedRoomState?.final_conclusion ?? selectedRoomSummary?.latest_conclusion;
  const selectedSummaryEvidence =
    selectedSavedReviewQueueInput === null
      ? null
      : {
          collectionResults: selectedSavedReviewQueueInput.collectionResults,
          evidenceCollectionSuggestions:
            selectedSavedReviewQueueInput.evidenceCollectionSuggestions,
          evidenceRequests: selectedSavedReviewQueueInput.evidenceRequests,
          missingEvidenceRequests:
            selectedSavedReviewQueueInput.missingEvidenceRequests,
          supplementalEvidence: selectedSavedReviewQueueInput.supplementalEvidence,
        };
  const workflowReadinessItems = diagnosisWorkflowReadiness({
    actorSubject: currentActorSubject,
    canConfirmConclusion,
    confirmConclusionBlockReason,
    connected,
    connectionStatus: status,
    latestInsight:
      selectedLatestInsight === null
        ? null
        : {
            collectionResults: selectedLatestInsight.collectionResults,
            evidenceRequests: selectedLatestInsight.evidenceRequests,
            insight: selectedLatestInsight.insight,
            supplementalEvidence: selectedRoomState?.supplemental_evidence ?? [],
          },
    notificationDelivery: selectedNotificationDeliveryCoverage,
    permissionItems: roomPermissionItems,
    retainedConclusion: selectedRetainedConclusion,
    summaryEvidence: selectedSummaryEvidence,
    selectedRoomStatus: selectedRoomSummary?.room_status,
    selectedSessionID,
    state: selectedRoomState,
    workflowVisibility: selectedRoomSummary?.workflow_visibility,
  });
  const retainedConclusionNeedsDeliveryReview =
    (selectedRetainedConclusion?.confirmed_by?.trim() ?? "") !== "" &&
    selectedNotificationDeliveryCoverage?.status !== "ready";
  const workflowReadinessReviewQueueSource =
    diagnosisWorkflowReadinessReviewQueueSource({
      latestInsightLoaded: selectedLatestInsight !== null,
      savedReviewQueueLoaded: selectedSavedReviewQueueInput !== null,
    });
  const workflowReadinessReviewQueueLoaded =
    workflowReadinessReviewQueueSource !== "none";
  const workflowReadinessActions: DiagnosisWorkflowReadinessActions = {
    conclusion: {
      disabled:
        !canConfirmConclusion &&
        !workflowReadinessReviewQueueLoaded &&
        !retainedConclusionNeedsDeliveryReview,
      icon: canConfirmConclusion ? (
        <CheckCircleOutlined />
      ) : retainedConclusionNeedsDeliveryReview ? (
        <WechatOutlined />
      ) : (
        <FormOutlined />
      ),
      label: canConfirmConclusion
        ? "Confirm"
        : retainedConclusionNeedsDeliveryReview
          ? "Review delivery"
          : "Review blockers",
      onClick: canConfirmConclusion
        ? handleConfirmConclusion
        : retainedConclusionNeedsDeliveryReview
          ? () =>
              scrollToWorkbenchSection("room", notificationTimelinePanelRef)
          : handleReviewEvidenceTasks,
      title: canConfirmConclusion
        ? "Confirm and retain the AI conclusion."
        : retainedConclusionNeedsDeliveryReview
          ? "Open the notification timeline for delivery proof."
          : "Open the review queue for remaining conclusion blockers.",
    },
    connection: {
      icon: connected ? <ReloadOutlined /> : <ApiOutlined />,
      label: connected ? "Refresh state" : "Open connection",
      onClick: connected
        ? handleQueryState
        : handleOpenConnectionControls,
      title: connected
        ? "Refresh the selected room state."
        : "Open the connection controls.",
    },
    evidence: {
      disabled:
        selectedSessionID.trim() === "" && !workflowReadinessReviewQueueLoaded,
      icon: workflowReadinessReviewQueueLoaded ? (
        <FormOutlined />
      ) : (
        <ApiOutlined />
      ),
      label: workflowReadinessReviewQueueLoaded
        ? "Review evidence"
        : "Open connection",
      onClick:
        workflowReadinessReviewQueueLoaded
          ? handleReviewEvidenceTasks
          : handleOpenConnectionControls,
      title:
        workflowReadinessReviewQueueLoaded
          ? "Open the evidence review queue."
          : "Open the connection controls to load room state.",
    },
    identity: {
      icon: currentActorSubject ? (
        <SafetyCertificateOutlined />
      ) : (
        <LoginOutlined />
      ),
      label: currentActorSubject ? "Review identity" : "Authenticate",
      onClick: () =>
        scrollToWorkbenchSection(
          "setup",
          connectionPanelRef.current !== null
            ? connectionPanelRef
            : createRoomPanelRef,
        ),
      title: currentActorSubject
        ? "Open authentication and session controls."
        : "Open authentication controls.",
    },
    permissions: {
      icon: <SafetyCertificateOutlined />,
      label: "Review permissions",
      onClick: () =>
        scrollToWorkbenchSection("room", permissionsPanelRef),
      title: "Open selected-room permission details.",
    },
    room: {
      icon: selectedSessionID.trim() === "" ? <FormOutlined /> : <ReloadOutlined />,
      label: selectedSessionID.trim() === "" ? "Select room" : "Refresh room",
      onClick:
        selectedSessionID.trim() === ""
          ? () => scrollToWorkbenchSection("queue", roomSelectionPanelRef)
          : handleQueryState,
      title:
        selectedSessionID.trim() === ""
          ? "Open the room queue."
          : "Refresh the selected room state.",
    },
  };
  function handleWorkbenchSectionChange(section: DiagnosisWorkbenchSection) {
    switch (section) {
      case "queue":
        scrollToWorkbenchSection(section, roomSelectionPanelRef);
        return;
      case "setup":
        scrollToWorkbenchSection(section, createRoomPanelRef);
        return;
      case "room":
        scrollToWorkbenchSection(section, roomStatePanelRef);
        return;
      case "insight":
        scrollToWorkbenchSection(section, insightPanelRef);
        return;
      case "conversation":
        scrollToWorkbenchSection(section, conversationPanelRef);
    }
  }
  const confirmedReportReturnHref =
    selectedRoomState?.final_conclusion?.confirmed_by && pageContext.backHref
      ? diagnosisReportReturnHref(pageContext.backHref, "confirmed")
      : undefined;

  return (
    <>
      <section className="page-heading">
        <div>
          <h1>Diagnosis Room</h1>
          <p>
            Short-conversation investigation from a frozen evidence snapshot.
          </p>
        </div>
        <Tag
          aria-label="Connection status"
          color={statusColor(status)}
          role="status"
        >
          {statusLabel(status)}
        </Tag>
      </section>

      {pageContext.hasContext ? (
        <Alert
          action={
            pageContext.backHref ? (
              <Link
                className="link-button"
                href={pageContext.backHref as Route}
              >
                Back to report
              </Link>
            ) : undefined
          }
          className="diagnosis-context"
          description={
            <div className="diagnosis-context-body">
              <span>{pageContext.description}</span>
              <Typography.Text className="diagnosis-context-prompt">
                {pageContext.suggestedPrompt}
              </Typography.Text>
            </div>
          }
          message={pageContext.title}
          showIcon
          type="info"
        />
      ) : null}

      {weComAuthErrorDetail !== null ? (
        <Alert
          action={
            <Space wrap>
              <Button href={oidcLoginHref} icon={<LoginOutlined />}>
                Sign in with IAM
              </Button>
            </Space>
          }
          closable
          description={weComAuthErrorDetail.description}
          message={weComAuthErrorDetail.message}
          onClose={() =>
            setDismissedWeComAuthErrorSource(
              pageContext.authError !== undefined
                ? weComAuthErrorSource
                : (currentWeComAuthError?.source ?? ""),
            )
          }
          showIcon
          type="warning"
        />
      ) : null}

      {oidcAuthErrorDetail !== null ? (
        <Alert
          action={
            <Button href={oidcLoginHref} icon={<LoginOutlined />}>
              Sign in with IAM
            </Button>
          }
          closable
          description={oidcAuthErrorDetail.description}
          message={oidcAuthErrorDetail.message}
          onClose={() =>
            setDismissedOIDCAuthErrorSource(
              pageContext.oidcAuthError !== undefined
                ? oidcAuthErrorSource
                : (currentOIDCAuthError?.source ?? ""),
            )
          }
          showIcon
          type="warning"
        />
      ) : null}

      {diagnosisRoomAuthorizationEnforced &&
      diagnosisRoomAuthorization.notice ? (
        <Alert
          description={diagnosisRoomAuthorization.notice.message}
          message="Diagnosis room permissions unavailable"
          showIcon
          type="warning"
        />
      ) : null}

      {alertContext ? (
        <DiagnosisAlertContextPanel context={alertContext} />
      ) : null}

      {alertContext && alertSnapshotNeedsRoom ? (
        <AlertRoomCreationNotice
          context={alertContext}
          disabled={!clientReady}
          onPrepare={handlePrepareAlertRoom}
        />
      ) : null}

      <nav aria-label="Diagnosis workspace sections" className="diagnosis-workbench-nav">
        <Segmented
          aria-label="Diagnosis workspace section"
          onChange={(value) =>
            handleWorkbenchSectionChange(value as DiagnosisWorkbenchSection)
          }
          options={[
            { label: "Queue", value: "queue" },
            { label: "Setup", value: "setup" },
            { label: "State", value: "room" },
            { label: "Evidence", value: "insight" },
            { label: "Chat", value: "conversation" },
          ]}
          value={workbenchSection}
        />
        <Typography.Text className="diagnosis-workbench-context" type="secondary">
          {selectedSessionID.trim() === ""
            ? "No room selected"
            : selectedSessionID}
        </Typography.Text>
      </nav>

      <div ref={roomSelectionPanelRef}>
        <DiagnosisWorkQueuePanel
          filter={workQueueFilter}
          handoffsResult={handoffsQuery.data}
          onFilterChange={setWorkQueueFilter}
          roomsResult={recentRoomsQuery.data}
        />

        {workQueueShowsHandoffs(workQueueFilter) && handoffsQuery.data ? (
          <DiagnosisHandoffBacklogPanel
            clientReady={clientReady}
            isFetching={handoffsQuery.isFetching}
            onPrepareRoom={handlePrepareHandoffRoom}
            onRefresh={() => void handoffsQuery.refetch()}
            result={handoffsQuery.data}
            selectedEvidenceSnapshotID={pageContext.evidenceSnapshotID}
          />
        ) : null}

        {workQueueShowsRooms(workQueueFilter) && recentRoomsQuery.data ? (
          <RecentDiagnosisRoomsPanel
            clientReady={clientReady}
            closingSessionID={closingUnavailableSessionID}
            filter={diagnosisRoomFilterForWorkQueue(workQueueFilter)}
            isFetching={recentRoomsQuery.isFetching}
            onAdministerRoomBlocked={diagnosisRoomAdministerBlockReason}
            onPrepareRoomRebuild={handlePrepareRoomRebuild}
            onRefresh={() => void recentRoomsQuery.refetch()}
            onRetryNotification={(room, entry) =>
              void handleRetryNotification(entry, room)
            }
            onSelectRoom={handleSelectRoom}
            result={recentRoomsQuery.data}
            retryingNotificationKey={retryingNotificationKey}
            selectedSessionID={selectedSessionID}
          />
        ) : null}
      </div>

      <DiagnosisHandoffPanel
        alertContext={alertContext}
        alertSnapshotNeedsRoom={alertSnapshotNeedsRoom}
        onUseSuggestedPrompt={handleUseSuggestedPrompt}
        pageContext={pageContext}
        selectedRoom={selectedRoomSummary}
      />

      {serverErrorDisplay ? (
        <Alert
          action={
            serverErrorDisplay.actionLabel ? (
              <TooltipAction
                disabled={false}
                title={serverErrorDisplay.actionTitle ?? ""}
              >
                <Button
                  icon={<FormOutlined />}
                  onClick={handleReviewEvidenceTasks}
                >
                  {serverErrorDisplay.actionLabel}
                </Button>
              </TooltipAction>
            ) : undefined
          }
          className="diagnosis-server-error"
          closable
          description={serverErrorDisplay.description}
          message={serverErrorDisplay.message}
          onClose={() => setServerError(null)}
          showIcon
          type={serverErrorDisplay.type}
        />
      ) : null}

      {weComQuickSignInPrompt !== null ? (
        <Alert
          action={
            <Space wrap>
              <Button href={oidcLoginHref} icon={<LoginOutlined />} type="primary">
                Sign in with IAM
              </Button>
            </Space>
          }
          className="diagnosis-channel-setup-alert"
          description="Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in through IAM before creating or joining a diagnosis room."
          message="IAM sign-in required"
          showIcon
          type="info"
        />
      ) : null}

      <div className="diagnosis-layout">
        <div ref={createRoomPanelRef}>
          <Card
            className="settings-overview-card"
            title="Create Diagnosis Room"
          >
            <Form<CreateRoomFormValues>
              form={createForm}
              initialValues={{
                authMode: pageContext.authMode ?? "session",
                bearerToken: "",
                evidenceSnapshotID:
                  pageContext.evidenceSnapshotID ?? initialEvidenceSnapshotID,
                ldapPassword: "",
                ldapUsername: "",
              }}
              layout="vertical"
              onFinish={handleCreateRoom}
              onValuesChange={(changedValues) => {
                if (diagnosisAuthInputFieldsChanged(changedValues)) {
                  clearAuthCheckForContext("create");
                }
              }}
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
                        : Promise.reject(
                            new Error(
                              "Evidence snapshot must be a positive integer.",
                            ),
                          ),
                  },
                ]}
              >
                <InputNumber
                  disabled={createBusy}
                  min={1}
                  precision={0}
                  style={{ width: "100%" }}
                />
              </Form.Item>
              <Form.Item
                label="Notification channel"
                name="closeNotificationChannelProfileID"
                rules={[
                  {
                    validator: (_, value: unknown) =>
                      Promise.resolve().then(() => {
                        if (value !== undefined && value !== null) {
                          if (!isPositiveSafeInteger(value)) {
                            throw new Error(
                              "Notification channel must be a positive integer.",
                            );
                          }
                          const selectionError =
                            diagnosisNotificationChannelSelectionError(
                              value,
                              notificationChannels,
                            );
                          if (selectionError !== "") {
                            throw new Error(selectionError);
                          }
                          return;
                        }
                        const blockReason =
                          diagnosisNotificationChannelCreateBlockReason({
                            channelID: undefined,
                            channels: notificationChannels,
                            failedToLoad:
                              notificationChannelsQuery.data?.ok === false,
                          });
                        if (blockReason !== "") {
                          throw new Error(blockReason);
                        }
                      }),
                  },
                ]}
              >
                <Select
                  allowClear
                  disabled={createBusy}
                  loading={notificationChannelsQuery.isFetching}
                  notFoundContent="No notification channels"
                  optionFilterProp="label"
                  options={notificationChannelOptions}
                  placeholder="No notification channel"
                  showSearch
                  style={{ width: "100%" }}
                />
              </Form.Item>
              {isPositiveSafeInteger(watchedCreateNotificationChannelID) ? (
                <Alert
                  className="diagnosis-channel-setup-alert"
                  description={createNotificationChannelProofSummary.detail}
                  message={createNotificationChannelProofSummary.label}
                  showIcon
                  type={
                    createNotificationChannelProofSummary.status === "ready"
                      ? "success"
                      : createNotificationChannelProofSummary.status ===
                          "blocked"
                        ? "error"
                        : "warning"
                  }
                />
              ) : null}
              {notificationChannelSetupAction === null &&
              createNotificationChannelBlockReason !== "" ? (
                <Alert
                  className="diagnosis-channel-setup-alert"
                  description={createNotificationChannelBlockReason}
                  message="WeCom notification channel required"
                  showIcon
                  type="warning"
                />
              ) : null}
              {notificationChannelSetupAction !== null ? (
                <Alert
                  action={
                    <Button
                      href={notificationChannelSetupAction.href}
                      icon={<PlusCircleOutlined />}
                      size="small"
                    >
                      {notificationChannelSetupAction.label}
                    </Button>
                  }
                  className="diagnosis-channel-setup-alert"
                  description={notificationChannelSetupAction.detail}
                  message="WeCom notification channel setup"
                  showIcon
                  type="warning"
                />
              ) : null}
              <Form.Item label="Authentication" name="authMode">
                <DiagnosisAuthModeSelector
                  disabled={createBusy}
                  options={authModeOptions}
                />
              </Form.Item>
              {watchedCreateAuthMode === "wecom" ? (
                <Form.Item>
                  <DiagnosisWeComLoginActions
                    busy={createBusy}
                    checkAuthDisabledReason={createAuthCheckDisabledReason}
                    checkAuthLoading={createAuthCheckPending}
                    directoryUsersBySubject={
                      collaborationDirectoryUsersBySubject
                    }
                    iamLoginHref={oidcLoginHref}
                    logoutBusy={clearBrowserSessionMutation.isPending}
                    onCheckAuth={() =>
                      handleUseBrowserSession("create", "session")
                    }
                    onLogout={() => clearBrowserSessionMutation.mutate()}
                    onRefreshSession={() => {
                      void diagnosisBrowserSessionQuery.refetch();
                    }}
                    session={diagnosisBrowserSessionQuery.data}
                    sessionLoading={diagnosisBrowserSessionQuery.isPending}
                    sessionRefreshing={diagnosisBrowserSessionQuery.isFetching}
                  />
                </Form.Item>
              ) : watchedCreateAuthMode === "session" ? (
                <Form.Item>
                  <DiagnosisBrowserSessionActions
                    busy={createBusy}
                    checkAuthDisabledReason={createAuthCheckDisabledReason}
                    checkAuthLoading={createAuthCheckPending}
                    directoryUsersBySubject={
                      collaborationDirectoryUsersBySubject
                    }
                    logoutBusy={clearBrowserSessionMutation.isPending}
                    onCheckAuth={() =>
                      void handleCheckAuthorization(
                        createForm.getFieldsValue(),
                        "create",
                      )
                    }
                    onLogout={() => clearBrowserSessionMutation.mutate()}
                    onRefreshSession={() => {
                      void diagnosisBrowserSessionQuery.refetch();
                    }}
                    oidcLoginHref={oidcLoginHref}
                    session={diagnosisBrowserSessionQuery.data}
                    sessionLoading={diagnosisBrowserSessionQuery.isPending}
                    sessionRefreshing={diagnosisBrowserSessionQuery.isFetching}
                  />
                </Form.Item>
              ) : watchedCreateAuthMode === "bearer" ? (
                <Form.Item
                  label="Bearer token"
                  name="bearerToken"
                  rules={[
                    {
                      required: true,
                      message: "Bearer token is required.",
                    },
                  ]}
                >
                  <Input.Password autoComplete="off" disabled={createBusy} />
                </Form.Item>
              ) : (
                <>
                  <Form.Item
                    label="LDAP username"
                    name="ldapUsername"
                    rules={[
                      {
                        required: true,
                        message: "LDAP username is required.",
                      },
                    ]}
                  >
                    <Input autoComplete="username" disabled={createBusy} />
                  </Form.Item>
                  <Form.Item
                    label="LDAP password"
                    name="ldapPassword"
                    rules={[
                      {
                        required: true,
                        message: "LDAP password is required.",
                      },
                    ]}
                  >
                    <Input.Password
                      autoComplete="current-password"
                      disabled={createBusy}
                    />
                  </Form.Item>
                  <LDAPBrowserSessionPromotionNotice />
                </>
              )}
              <Form.Item noStyle shouldUpdate>
                {(authForm) => (
                  <DiagnosisAuthReadinessPreview
                    authStatus={diagnosisAuthStatusQuery.data}
                    authStatusLoading={diagnosisAuthStatusQuery.isPending}
                    backendStatus={authBackendStatusSnapshot}
                    checking={createAuthCheckPending}
                    expectedSubject={authenticatedBrowserSessionSubject}
                    inputRevision={authCheckRevision.create}
                    lastCheck={
                      lastAuthCheck?.context === "create" ? lastAuthCheck : null
                    }
                    values={
                      authForm.getFieldsValue(true) as CreateRoomFormValues
                    }
                  />
                )}
              </Form.Item>
              <Space wrap>
                <TooltipAction
                  disabled={createAuthCheckDisabledReason !== ""}
                  title={createAuthCheckDisabledReason}
                >
                  <Button
                    disabled={
                      createBusy ||
                      authMutationPending ||
                      createAuthCheckDisabledReason !== ""
                    }
                    icon={<CheckCircleOutlined />}
                    loading={createAuthCheckPending}
                    onClick={() =>
                      void handleCheckAuthorization(
                        createForm.getFieldsValue(),
                        "create",
                      )
                    }
                  >
                    Check auth
                  </Button>
                </TooltipAction>
                <TooltipAction
                  disabled={createActionDisabledReason !== ""}
                  title={createActionDisabledReason}
                >
                  <Button
                    disabled={
                      createBusy ||
                      authMutationPending ||
                      createActionDisabledReason !== ""
                    }
                    htmlType="submit"
                    icon={<PlusCircleOutlined />}
                    loading={createRoomMutation.isPending}
                    type="primary"
                  >
                    Create diagnosis room
                  </Button>
                </TooltipAction>
              </Space>
            </Form>
          </Card>
        </div>

        <div ref={connectionPanelRef}>
          <Card
            aria-label="Connection controls"
            className="settings-overview-card"
            title="Connection"
          >
            <Form<ConnectionFormValues>
              form={connectionForm}
              initialValues={{
                authMode: pageContext.authMode ?? "session",
                bearerToken: "",
                ldapPassword: "",
                ldapUsername: "",
                sessionID: pageContext.sessionID ?? initialSessionID ?? "",
              }}
              layout="vertical"
              onFinish={handleConnect}
              onValuesChange={(changedValues) => {
                if (
                  typeof (changedValues as Partial<ConnectionFormValues>)
                    .sessionID === "string"
                ) {
                  setConnectionSessionID(
                    (changedValues as Partial<ConnectionFormValues>)
                      .sessionID ?? "",
                  );
                  resetSelectedRoomRuntimeState();
                }
                if (diagnosisAuthInputFieldsChanged(changedValues)) {
                  clearAuthCheckForContext("connection");
                }
              }}
            >
            <Form.Item
              label="Session ID"
              name="sessionID"
              rules={[{ required: true, message: "Session ID is required." }]}
            >
              <Input
                autoComplete="off"
                disabled={busy}
                onChange={(event) => {
                  setConnectionSessionID(event.target.value);
                  resetSelectedRoomRuntimeState();
                }}
              />
            </Form.Item>
            <Form.Item label="Authentication" name="authMode">
              <DiagnosisAuthModeSelector
                disabled={busy}
                options={authModeOptions}
              />
            </Form.Item>
            {watchedConnectionAuthMode === "wecom" ? (
              <Form.Item>
                <DiagnosisWeComLoginActions
                  busy={busy}
                  checkAuthDisabledReason={connectionAuthCheckDisabledReason}
                  checkAuthLoading={connectionAuthCheckPending}
                  directoryUsersBySubject={collaborationDirectoryUsersBySubject}
                  iamLoginHref={oidcLoginHref}
                  logoutBusy={clearBrowserSessionMutation.isPending}
                  onCheckAuth={() =>
                    handleUseBrowserSession("connection", "session")
                  }
                  onLogout={() => clearBrowserSessionMutation.mutate()}
                  onRefreshSession={() => {
                    void diagnosisBrowserSessionQuery.refetch();
                  }}
                  session={diagnosisBrowserSessionQuery.data}
                  sessionLoading={diagnosisBrowserSessionQuery.isPending}
                  sessionRefreshing={diagnosisBrowserSessionQuery.isFetching}
                />
              </Form.Item>
            ) : watchedConnectionAuthMode === "session" ? (
              <Form.Item>
                <DiagnosisBrowserSessionActions
                  busy={busy}
                  checkAuthDisabledReason={connectionAuthCheckDisabledReason}
                  checkAuthLoading={connectionAuthCheckPending}
                  directoryUsersBySubject={collaborationDirectoryUsersBySubject}
                  logoutBusy={clearBrowserSessionMutation.isPending}
                  onCheckAuth={() =>
                    void handleCheckAuthorization(
                      connectionForm.getFieldsValue(),
                      "connection",
                    )
                  }
                  onLogout={() => clearBrowserSessionMutation.mutate()}
                  onRefreshSession={() => {
                    void diagnosisBrowserSessionQuery.refetch();
                  }}
                  oidcLoginHref={oidcLoginHref}
                  session={diagnosisBrowserSessionQuery.data}
                  sessionLoading={diagnosisBrowserSessionQuery.isPending}
                  sessionRefreshing={diagnosisBrowserSessionQuery.isFetching}
                />
              </Form.Item>
            ) : watchedConnectionAuthMode === "bearer" ? (
              <Form.Item
                label="Bearer token"
                name="bearerToken"
                rules={[
                  { required: true, message: "Bearer token is required." },
                ]}
              >
                <Input.Password autoComplete="off" disabled={busy} />
              </Form.Item>
            ) : (
              <>
                <Form.Item
                  label="LDAP username"
                  name="ldapUsername"
                  rules={[
                    { required: true, message: "LDAP username is required." },
                  ]}
                >
                  <Input autoComplete="username" disabled={busy} />
                </Form.Item>
                <Form.Item
                  label="LDAP password"
                  name="ldapPassword"
                  rules={[
                    { required: true, message: "LDAP password is required." },
                  ]}
                >
                  <Input.Password
                    autoComplete="current-password"
                    disabled={busy}
                  />
                </Form.Item>
                <LDAPBrowserSessionPromotionNotice />
              </>
            )}
            <Form.Item noStyle shouldUpdate>
              {(authForm) => (
                <DiagnosisAuthReadinessPreview
                  authStatus={diagnosisAuthStatusQuery.data}
                  authStatusLoading={diagnosisAuthStatusQuery.isPending}
                  backendStatus={authBackendStatusSnapshot}
                  checking={connectionAuthCheckPending}
                  expectedSubject={authenticatedBrowserSessionSubject}
                  inputRevision={authCheckRevision.connection}
                  lastCheck={
                    lastAuthCheck?.context === "connection"
                      ? lastAuthCheck
                      : null
                  }
                  values={authForm.getFieldsValue(true) as ConnectionFormValues}
                />
              )}
            </Form.Item>
            <Space wrap>
              <TooltipAction
                disabled={connectionAuthCheckDisabledReason !== ""}
                title={connectionAuthCheckDisabledReason}
              >
                <Button
                  disabled={
                    busy ||
                    authMutationPending ||
                    connectionAuthCheckDisabledReason !== ""
                  }
                  icon={<CheckCircleOutlined />}
                  loading={connectionAuthCheckPending}
                  onClick={() =>
                    void handleCheckAuthorization(
                      connectionForm.getFieldsValue(),
                      "connection",
                    )
                  }
                >
                  Check auth
                </Button>
              </TooltipAction>
              <TooltipAction
                disabled={connectSubmitDisabledReason !== ""}
                title={connectSubmitDisabledReason}
              >
                <Button
                  disabled={
                    busy ||
                    authMutationPending ||
                    connectSubmitDisabledReason !== ""
                  }
                  htmlType="submit"
                  icon={<ApiOutlined />}
                  loading={ticketMutation.isPending}
                  type="primary"
                >
                  Connect
                </Button>
              </TooltipAction>
              <Button
                disabled={
                  !clientReady ||
                  !connected ||
                  selectedReadRBACBlockReason !== ""
                }
                icon={<ReloadOutlined />}
                onClick={handleQueryState}
              >
                Refresh State
              </Button>
              <Button
                disabled={!clientReady || status === "idle"}
                icon={<DisconnectOutlined />}
                onClick={handleDisconnect}
              >
                Disconnect
              </Button>
            </Space>
            </Form>
          </Card>
        </div>

        <div className="diagnosis-room-state-section" ref={roomStatePanelRef}>
          <Card
            aria-label="Room state"
            className="settings-overview-card"
            extra={
            <TooltipAction
              disabled={!canConfirmConclusion}
              title={
                confirmConclusionBlockReason ||
                "Retain this diagnosis conclusion."
              }
            >
              <Button
                disabled={!canConfirmConclusion}
                icon={<CheckCircleOutlined />}
                loading={confirmInFlight}
                onClick={handleConfirmConclusion}
              >
                Confirm Conclusion
              </Button>
            </TooltipAction>
            }
            title="Room State"
          >
          <DiagnosisWorkflowReadinessPanel
            actions={workflowReadinessActions}
            items={workflowReadinessItems}
          />
          <Descriptions column={1} items={roomStateItems} size="small" />
          <div ref={permissionsPanelRef}>
            <DiagnosisRoomPermissionPanel
              authorization={diagnosisRoomAuthorization}
              enforced={diagnosisRoomAuthorizationEnforced}
              items={roomPermissionItems}
            />
          </div>
          <CollaborationParticipantsPanel
            directoryUsersBySubject={collaborationDirectoryUsersBySubject}
            participants={selectedCollaborationParticipants}
          />
          {visibleConfirmBlockReason ? (
            <Alert
              className="diagnosis-confirm-block-reason"
              description={visibleConfirmBlockReason}
              message="Confirmation blocked"
              showIcon
              type="warning"
            />
          ) : null}
          {selectedRoomState?.final_conclusion ? (
            <Alert
              className="diagnosis-conclusion"
              description={
                <FinalConclusionDetails
                  actionDisabledReason={submitTurnBlockReason}
                  connected={canSubmitTurn}
                  directoryUsersBySubject={collaborationDirectoryUsersBySubject}
                  notificationDeliveryCoverage={
                    selectedNotificationDeliveryCoverage
                  }
                  onUseEvidencePlan={handleUseEvidencePlan}
                  onUseFollowUp={handleUseSupplementalEvidence}
                  state={selectedRoomState}
                />
              }
              message="Final conclusion"
              showIcon
              type="success"
            />
          ) : null}
          {confirmedReportReturnHref ? (
            <Alert
              action={
                <Link
                  className="link-button"
                  href={confirmedReportReturnHref as Route}
                >
                  Return to report
                </Link>
              }
              className="diagnosis-confirmed-return"
              description="The report detail will reload server-side readiness and expose the final notification action when all linked conclusions are confirmed."
              message="Operator confirmation recorded"
              showIcon
              type="success"
            />
          ) : null}
          {showSelectedFinalConclusionQueue &&
          selectedFinalConclusionQueueInput ? (
            <DiagnosisReviewQueuePanel
              actionDisabledReason={submitTurnBlockReason}
              canConfirmConclusion={canConfirmConclusion}
              connected={canSubmitTurn}
              onOpenConnection={() =>
                handleOpenConnectionControls({ returnToReviewQueue: true })
              }
              onConfirmConclusion={handleConfirmConclusion}
              onRequestReassessment={handleRequestAIReassessment}
              onUseEvidencePlan={handleUseEvidencePlan}
              onUseFollowUp={handleUseSupplementalEvidence}
              panelRef={reviewQueuePanelRef}
              queueInput={selectedFinalConclusionQueueInput}
              title="Final Conclusion Review Queue"
            />
          ) : null}
          {!selectedRoomState?.final_conclusion &&
          selectedRoomSummary?.latest_conclusion ? (
            <RetainedFinalConclusionSummary
              directoryUsersBySubject={collaborationDirectoryUsersBySubject}
              onRefreshDeliveryProof={() =>
                void invalidateDiagnosisRoomQueries(selectedRoomSummary.session_id)
              }
              onReviewDelivery={() =>
                scrollToWorkbenchSection("room", notificationTimelinePanelRef)
              }
              refreshingDeliveryProof={
                selectedExactRoomSummary !== undefined
                  ? exactRoomQuery.isFetching
                  : recentRoomsQuery.isFetching
              }
              room={selectedRoomSummary}
            />
          ) : null}
          <div ref={notificationTimelinePanelRef}>
            <DiagnosisNotificationTimelineSection
              administerDisabledReason={selectedAdministerRBACBlockReason}
              onRetryNotification={(entry) =>
                void handleRetryNotification(entry)
              }
              retryingNotificationKey={retryingNotificationKey}
              room={selectedRoomSummary}
            />
          </div>
          </Card>
        </div>
      </div>

      <div className="diagnosis-workbench-section" ref={insightPanelRef}>
        <Card
          className="diagnosis-room-panel settings-overview-card"
          extra={
          selectedLatestInsight ? (
            <Space className="diagnosis-insight-meta" size={[6, 6]} wrap>
              <Tag color={confidenceColor(selectedLatestInsight.confidence)}>
                {selectedLatestInsight.confidence || "unknown"}
              </Tag>
              <Tag
                color={
                  selectedLatestInsight.requiresHumanReview
                    ? "warning"
                    : "success"
                }
              >
                {selectedLatestInsight.requiresHumanReview
                  ? "review required"
                  : "review optional"}
              </Tag>
              {selectedLatestInsight.autoFollowUpCount > 0 ? (
                <Tag color="processing">
                  auto evidence x{selectedLatestInsight.autoFollowUpCount}
                </Tag>
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
        {selectedLatestInsight ? (
          <>
            <Descriptions
              column={{ xs: 1, sm: 2 }}
              items={consultationInsightItems(selectedLatestInsight)}
              size="small"
            />
            {selectedLatestInsight.insight.confidence_rationale ? (
              <Alert
                className="diagnosis-insight-rationale"
                description={selectedLatestInsight.insight.confidence_rationale}
                message="Confidence rationale"
                showIcon
                type="info"
              />
            ) : null}
            <ConsultationProgressPanel
              finalConclusion={selectedRoomState?.final_conclusion}
              latestInsight={selectedLatestInsight}
              notificationDeliveryCoverage={
                selectedNotificationDeliveryCoverage
              }
              supplementalEvidence={
                selectedRoomState?.supplemental_evidence ?? []
              }
            />
            <ConfidenceTimelineSection
              items={selectedLatestInsight.confidenceTimeline}
            />
            <DiagnosisReviewQueuePanel
              actionDisabledReason={submitTurnBlockReason}
              canConfirmConclusion={canConfirmConclusion}
              connected={canSubmitTurn}
              onOpenConnection={() =>
                handleOpenConnectionControls({ returnToReviewQueue: true })
              }
              onConfirmConclusion={handleConfirmConclusion}
              onRequestReassessment={handleRequestAIReassessment}
              onUseEvidencePlan={handleUseEvidencePlan}
              onUseFollowUp={handleUseSupplementalEvidence}
              panelRef={reviewQueuePanelRef}
              queueInput={latestInsightReviewQueueInput(
                selectedLatestInsight,
                canConfirmConclusion,
                selectedRoomState?.supplemental_evidence ?? [],
              )}
            />
            <SupplementalEvidenceHistoryPanel
              directoryUsersBySubject={collaborationDirectoryUsersBySubject}
              items={selectedRoomState?.supplemental_evidence ?? []}
              missingEvidenceRequests={
                selectedLatestInsight.insight.missing_evidence_requests ?? []
              }
            />
            <EvidenceTimelineSection
              directoryUsersBySubject={collaborationDirectoryUsersBySubject}
              items={evidenceTimelineForDisplay(selectedLatestInsight)}
            />
            <OperatorEvidenceCollectionPanel
              alertContext={alertContext}
              clientReady={clientReady}
              connected={canSubmitTurn}
              form={operatorEvidenceForm}
              onRefreshTemplates={() => {
                void diagnosisToolTemplateQuery.refetch();
              }}
              onSubmit={handleCollectOperatorEvidence}
              templateError={operatorEvidenceTemplateError}
              templates={operatorEvidenceTemplates}
              templatesLoading={diagnosisToolTemplateQuery.isFetching}
            />
            <div className="diagnosis-insight-grid">
              <EvidencePlanList
                emptyDescription="No executable evidence plan"
                items={selectedLatestInsight.evidenceRequests}
                title="Executable Evidence Plan"
              />
              <EvidenceCollectionResultList
                items={selectedLatestInsight.collectionResults}
              />
              <EvidenceRequestList
                emptyDescription="No missing evidence requests"
                items={selectedLatestInsight.insight.missing_evidence_requests}
                onUseFollowUp={handleUseSupplementalEvidence}
                followUpDisabled={!canSubmitTurn}
                followUpDisabledReason={submitTurnBlockReason}
                title="Missing Evidence"
              />
              <EvidenceRequestList
                emptyDescription="No collection suggestions"
                items={
                  selectedLatestInsight.insight.evidence_collection_suggestions
                }
                onUseFollowUp={handleUseSupplementalEvidence}
                followUpDisabled={!canSubmitTurn}
                followUpDisabledReason={submitTurnBlockReason}
                title="Collection Suggestions"
              />
            </div>
          </>
        ) : selectedSavedReviewQueueInput ? (
          <DiagnosisReviewQueuePanel
            actionDisabledReason={submitTurnBlockReason}
            canConfirmConclusion={canConfirmConclusion}
            connected={canSubmitTurn}
            onOpenConnection={() =>
              handleOpenConnectionControls({ returnToReviewQueue: true })
            }
            onConfirmConclusion={handleConfirmConclusion}
            onRequestReassessment={handleRequestAIReassessment}
            onUseEvidencePlan={handleUseEvidencePlan}
            onUseFollowUp={handleUseSupplementalEvidence}
            panelRef={reviewQueuePanelRef}
            queueInput={selectedSavedReviewQueueInput}
            title="Saved Review Queue"
          />
        ) : (
          <Empty
            description="No consultation insight yet"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        )}
        </Card>
      </div>

      <div className="diagnosis-workbench-section" ref={conversationPanelRef}>
        <Card
          className="diagnosis-room-panel settings-overview-card"
          extra={
          <Typography.Text type="secondary">
            {transcript.length} message(s)
          </Typography.Text>
          }
          title="Transcript"
        >
        {transcript.length === 0 ? (
          <Empty
            description="No transcript messages"
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          />
        ) : (
          <div aria-live="polite" className="diagnosis-transcript">
            {transcript.map((turn) => (
              <article
                className={`diagnosis-turn diagnosis-turn-${turn.role}`}
                key={turn.id}
              >
                <div className="diagnosis-turn-role">
                  {turn.role}
                  <ActorSubjectTags
                    directoryUsersBySubject={collaborationDirectoryUsersBySubject}
                    subject={turn.actor_subject}
                  />
                </div>
                <p>{turn.content}</p>
              </article>
            ))}
          </div>
        )}

        {turnInFlight ? <DiagnosisTurnProgressNotice /> : null}

        {pendingSupplementalEvidence ? (
          <SupplementalEvidenceEntryPanel
            actionDisabledReason={submitTurnBlockReason}
            connected={canSubmitTurn}
            form={supplementalEvidenceForm}
            onClear={handleClearSupplementalEvidence}
            onSubmit={handleSubmitSupplementalEvidence}
            panelRef={supplementalEvidencePanelRef}
            request={pendingSupplementalEvidence}
          />
        ) : null}
        {pendingEvidencePlan ? (
          <Alert
            action={
              <Space size={4} wrap>
                <TooltipAction
                  disabled={!canSubmitTurn}
                  title={
                    submitTurnBlockReason || "Collect this planned evidence."
                  }
                >
                  <Button
                    disabled={!canSubmitTurn}
                    icon={<FormOutlined />}
                    onClick={() => handleUseEvidencePlan(pendingEvidencePlan)}
                    size="small"
                    type="link"
                  >
                    Use plan
                  </Button>
                </TooltipAction>
                <Button
                  onClick={handleClearEvidencePlan}
                  size="small"
                  type="link"
                >
                  Clear
                </Button>
              </Space>
            }
            className="diagnosis-supplemental-pending"
            description={
              <Space direction="vertical" size={4}>
                <Typography.Text>{pendingEvidencePlan.reason}</Typography.Text>
                <EvidenceRequestMetadata
                  fallbackText="No additional parameters"
                  request={pendingEvidencePlan}
                />
              </Space>
            }
            message={
              <Space size={[6, 6]} wrap>
                <span>Planned evidence from report</span>
                <Tag color="processing">{pendingEvidencePlan.tool}</Tag>
              </Space>
            }
            showIcon
            type="info"
          />
        ) : null}

        <Form<ComposerValues>
          className="diagnosis-composer"
          form={composerForm}
          layout="vertical"
          onFinish={handleSend}
        >
          {pendingSupplementalEvidence ? (
            <Alert
              action={
                <Button
                  disabled={!canSubmitTurn}
                  onClick={handleClearSupplementalEvidence}
                  size="small"
                  type="link"
                >
                  Clear
                </Button>
              }
              className="diagnosis-supplemental-pending"
              message={
                <Space size={[6, 6]} wrap>
                  <span>Supplemental evidence</span>
                  <Tag
                    color={priorityColor(pendingSupplementalEvidence.priority)}
                  >
                    {pendingSupplementalEvidence.label}
                  </Tag>
                </Space>
              }
              showIcon
              type="warning"
            />
          ) : null}
          <ActionBlockedNotice
            message="Evidence updates unavailable"
            reason={submitTurnBlockReason}
          />
          <Form.Item
            label="Message"
            name="message"
            rules={[{ required: true, message: "Message is required." }]}
          >
            <Input.TextArea
              autoSize={{ minRows: 3, maxRows: 6 }}
              disabled={!canSubmitComposer}
            />
          </Form.Item>
          <Space wrap>
            <Button
              disabled={!canSubmitComposer}
              htmlType="submit"
              icon={<SendOutlined />}
              loading={turnInFlight}
              type="primary"
            >
              Send
            </Button>
            {pageContext.suggestedPrompt !== "" ? (
              <Button
                disabled={!canSubmitComposer}
                icon={<FormOutlined />}
                onClick={handleUseSuggestedPrompt}
              >
                Use suggested prompt
              </Button>
            ) : null}
          </Space>
        </Form>
        </Card>
      </div>

      {log.length > 0 ? (
        <Card className="settings-overview-card" title="Events">
          <List
            dataSource={log}
            renderItem={(entry) => (
              <List.Item
                className={
                  entry.level === "error" ? "diagnosis-log-error" : undefined
                }
              >
                {entry.message}
              </List.Item>
            )}
            size="small"
          />
        </Card>
      ) : null}
    </>
  );
}

function TooltipAction({
  children,
  disabled,
  title,
}: {
  children: ReactNode;
  disabled: boolean;
  title: string;
}) {
  return (
    <Tooltip title={title} trigger={["hover", "focus"]}>
      <span
        aria-label={disabled && title ? title : undefined}
        tabIndex={disabled ? 0 : undefined}
      >
        {children}
      </span>
    </Tooltip>
  );
}

function DiagnosisTurnProgressNotice() {
  return (
    <Alert
      className="diagnosis-turn-progress"
      description={
        <Progress
          aria-label="Diagnosis turn in progress"
          percent={100}
          showInfo={false}
          size="small"
          status="active"
        />
      }
      message="AI review in progress"
      showIcon
      type="info"
    />
  );
}

function ActionBlockedNotice({
  message,
  reason,
}: {
  message: string;
  reason: string;
}) {
  if (reason === "") {
    return null;
  }
  return (
    <Alert
      aria-label={message}
      className="diagnosis-action-blocked"
      description={reason}
      message={message}
      showIcon
      type={reason.startsWith("Connect ") ? "info" : "warning"}
    />
  );
}

function SupplementalEvidenceEntryPanel({
  actionDisabledReason,
  connected,
  form,
  onClear,
  onSubmit,
  panelRef,
  request,
}: {
  actionDisabledReason: string;
  connected: boolean;
  form: FormInstance<SupplementalEvidenceFormValues>;
  onClear: () => void;
  onSubmit: (values: SupplementalEvidenceFormValues) => void;
  panelRef: RefObject<HTMLDivElement | null>;
  request: DiagnosisConsultationEvidenceRequest;
}) {
  const actionDisabled = !connected || actionDisabledReason !== "";

  function useResidualBoundaryTemplate() {
    form.setFieldsValue({
      evidence: supplementalEvidenceResidualBoundaryTemplate(request),
    });
  }

  return (
    <section
      aria-label="Supplemental evidence entry"
      className="diagnosis-supplemental-entry"
      ref={panelRef}
    >
      <div className="diagnosis-supplemental-entry-header">
        <div>
          <Typography.Title level={3}>Supplemental Evidence</Typography.Title>
          <Typography.Text type="secondary">{request.detail}</Typography.Text>
        </div>
        <Tag color={priorityColor(request.priority)}>{request.priority}</Tag>
      </div>
      <Alert
        description="Provide only evidence you have verified. The assistant will use this update to revise confidence and decide whether the conclusion is ready."
        message={request.label}
        showIcon
        type="warning"
      />
      <ActionBlockedNotice
        message="Supplemental evidence unavailable"
        reason={actionDisabledReason}
      />
      <Form<SupplementalEvidenceFormValues>
        form={form}
        layout="vertical"
        onFinish={onSubmit}
      >
        <Form.Item
          label="Evidence"
          name="evidence"
          rules={[
            { required: true, message: "Evidence is required." },
            {
              validator: (_, value: unknown) =>
                typeof value === "string" && value.trim() !== ""
                  ? Promise.resolve()
                  : Promise.reject(new Error("Evidence must not be blank.")),
            },
          ]}
        >
          <Input.TextArea
            autoSize={{ minRows: 4, maxRows: 8 }}
            disabled={actionDisabled}
          />
        </Form.Item>
        <Space wrap>
          <TooltipAction
            disabled={actionDisabled}
            title={
              actionDisabledReason ||
              "Use a bounded residual-uncertainty template."
            }
          >
            <Button
              disabled={actionDisabled}
              icon={<FormOutlined />}
              onClick={useResidualBoundaryTemplate}
            >
              Use residual boundary
            </Button>
          </TooltipAction>
          <TooltipAction
            disabled={actionDisabled}
            title={actionDisabledReason || "Submit supplemental evidence."}
          >
            <Button
              disabled={actionDisabled}
              htmlType="submit"
              icon={<SendOutlined />}
              type="primary"
            >
              Submit supplemental evidence
            </Button>
          </TooltipAction>
          <TooltipAction
            disabled={actionDisabled}
            title={actionDisabledReason || "Clear supplemental evidence."}
          >
            <Button disabled={actionDisabled} onClick={onClear}>
              Clear
            </Button>
          </TooltipAction>
        </Space>
      </Form>
    </section>
  );
}

function DiagnosisAlertContextPanel({
  context,
}: {
  context: DiagnosisAlertContext;
}) {
  const { alert, snapshot } = context;
  const labelItems = sortedRecordEntries(alert.labels);
  const annotationItems = sortedRecordEntries(alert.annotations);

  return (
    <Card
      aria-label="Alert context"
      className="diagnosis-alert-context settings-overview-card"
      title={
        <Space size={[6, 6]} wrap>
          <span>{alertName(alert)}</span>
          <Tag color={alert.status === "firing" ? "red" : "green"}>
            {alert.status}
          </Tag>
          <Tag color={severityColor(alertSeverity(alert))}>
            {alertSeverity(alert)}
          </Tag>
          <Tag color={snapshotStatusColor(snapshot.status)}>
            snapshot #{snapshot.id}
          </Tag>
        </Space>
      }
    >
      <Descriptions
        column={{ xs: 1, md: 2 }}
        items={alertContextDescriptionItems(alert, snapshot)}
        size="small"
      />
      <div className="diagnosis-alert-context-grid">
        <KeyValueSummary
          emptyText="No labels"
          entries={labelItems}
          title="Labels"
        />
        <KeyValueSummary
          emptyText="No annotations"
          entries={annotationItems}
          title="Annotations"
        />
      </div>
    </Card>
  );
}

function KeyValueSummary({
  emptyText,
  entries,
  title,
}: {
  emptyText: string;
  entries: Array<[string, string]>;
  title: string;
}) {
  return (
    <section className="diagnosis-alert-context-block">
      <div className="diagnosis-alert-context-block-header">
        <Typography.Title level={3}>{title}</Typography.Title>
        <Tag>{entries.length}</Tag>
      </div>
      {entries.length === 0 ? (
        <Typography.Text type="secondary">{emptyText}</Typography.Text>
      ) : (
        <div className="diagnosis-alert-context-values">
          {entries.map(([key, value]) => (
            <Tag className="diagnosis-alert-context-tag" key={key}>
              {key}: {value}
            </Tag>
          ))}
        </div>
      )}
    </section>
  );
}

function AlertRoomCreationNotice({
  context,
  disabled,
  onPrepare,
}: {
  context: DiagnosisAlertContext;
  disabled: boolean;
  onPrepare: () => void;
}) {
  return (
    <Alert
      action={
        <Button
          disabled={disabled}
          icon={<PlusCircleOutlined />}
          onClick={onPrepare}
          size="small"
          type="primary"
        >
          Prepare room
        </Button>
      }
      className="diagnosis-alert-room-notice"
      description={`Snapshot #${context.snapshot.id} has evidence for ${alertName(
        context.alert,
      )}, but no diagnosis room is linked yet. Enter verified authorization credentials in the create form to start the room safely.`}
      message="No diagnosis room linked"
      showIcon
      type="warning"
    />
  );
}

function DiagnosisHandoffPanel({
  alertContext,
  alertSnapshotNeedsRoom,
  onUseSuggestedPrompt,
  pageContext,
  selectedRoom,
}: {
  alertContext?: DiagnosisAlertContext;
  alertSnapshotNeedsRoom: boolean;
  onUseSuggestedPrompt: () => void;
  pageContext: DiagnosisPageContext;
  selectedRoom?: DiagnosisRoomSummary;
}) {
  if (
    !pageContext.hasContext &&
    selectedRoom === undefined &&
    alertContext === undefined
  ) {
    return null;
  }

  const handoff = diagnosisHandoffStatus({
    alertContext,
    alertSnapshotNeedsRoom,
    pageContext,
    selectedRoom,
  });
  const conclusion = selectedRoom?.latest_conclusion;
  const failedNotification =
    selectedRoom === undefined
      ? null
      : latestFailedDiagnosisRoomNotification(selectedRoom);
  const showNotificationTimelineReview =
    selectedRoom !== undefined &&
    diagnosisNotificationTimelineReviewActionRequired(
      selectedRoom,
      failedNotification,
    );

  return (
    <Card
      aria-label="Diagnosis handoff"
      className="diagnosis-handoff settings-overview-card"
      extra={
        <Space size={[6, 6]} wrap>
          <Tag color={handoff.color}>{handoff.label}</Tag>
          {selectedRoom?.latest_conclusion?.confidence ? (
            <Tag
              color={confidenceColor(selectedRoom.latest_conclusion.confidence)}
            >
              {selectedRoom.latest_conclusion.confidence}
            </Tag>
          ) : null}
          {selectedRoom?.latest_conclusion?.requires_human_review ? (
            <Tag color="warning">review</Tag>
          ) : null}
        </Space>
      }
      title="Diagnosis Handoff"
    >
      <div className="diagnosis-handoff-body">
        <Alert
          description={handoff.detail}
          message={handoff.message}
          showIcon
          type={handoff.alertType}
        />
        <Descriptions
          column={{ xs: 1, md: 2 }}
          items={diagnosisHandoffItems(pageContext, selectedRoom)}
          size="small"
        />
        {conclusion ? (
          <Alert
            className="diagnosis-handoff-conclusion"
            description={conclusion.content}
            message="Latest AI conclusion"
            showIcon
            type={conclusion.requires_human_review ? "warning" : "success"}
          />
        ) : null}
        <Space className="diagnosis-handoff-actions" size={[8, 8]} wrap>
          {failedNotification !== null ? (
            <a
              className="link-button"
              href={notificationChannelReviewHref(failedNotification)}
            >
              Review notification channel
            </a>
          ) : null}
          {showNotificationTimelineReview ? (
            <a className="link-button" href={diagnosisNotificationTimelineHref}>
              Review notification timeline
            </a>
          ) : null}
          {pageContext.suggestedPrompt !== "" ? (
            <Button icon={<FormOutlined />} onClick={onUseSuggestedPrompt}>
              {handoff.promptActionLabel}
            </Button>
          ) : null}
        </Space>
      </div>
    </Card>
  );
}

function diagnosisHandoffStatus({
  alertContext,
  alertSnapshotNeedsRoom,
  pageContext,
  selectedRoom,
}: {
  alertContext?: DiagnosisAlertContext;
  alertSnapshotNeedsRoom: boolean;
  pageContext: DiagnosisPageContext;
  selectedRoom?: DiagnosisRoomSummary;
}): {
  alertType: "info" | "success" | "warning";
  color: string;
  detail: string;
  label: string;
  message: string;
  promptActionLabel: string;
} {
  if (alertSnapshotNeedsRoom) {
    return {
      alertType: "warning",
      color: "warning",
      detail: `Evidence snapshot #${pageContext.evidenceSnapshotID ?? alertContext?.snapshot.id} is ready for ${alertContext ? alertName(alertContext.alert) : "diagnosis"}, but no AI room is linked yet.`,
      label: "Create room",
      message: "Evidence ready",
      promptActionLabel: "Use diagnosis prompt",
    };
  }
  const failedNotification =
    selectedRoom === undefined
      ? null
      : latestFailedDiagnosisRoomNotification(selectedRoom);
  if (failedNotification !== null) {
    return {
      alertType: "warning",
      color: "error",
      detail: `${notificationEventLabel(failedNotification.event_kind)} failed at ${formatDateTime(failedNotification.occurred_at)}. Review notification channel delivery before relying on downstream handoff.`,
      label: "Notification failed",
      message: "Notification delivery failed",
      promptActionLabel: "Use review prompt",
    };
  }
  const notificationDeliveryCoverage =
    selectedRoom === undefined
      ? undefined
      : diagnosisNotificationDeliveryCoverage(
          selectedRoom.notification_timeline ?? [],
        );
  if (
    notificationDeliveryCoverage !== undefined &&
    (notificationDeliveryCoverage.status === "blocked" ||
      notificationDeliveryCoverage.status === "review")
  ) {
    return {
      alertType: "warning",
      color: notificationDeliveryCoverage.color,
      detail: notificationDeliveryCoverage.detail,
      label: notificationDeliveryCoverage.label,
      message: "AI notification delivery incomplete",
      promptActionLabel: "Use review prompt",
    };
  }
  const notificationProofSummary =
    selectedRoom === undefined
      ? undefined
      : diagnosisNotificationContentProofSummary(
          selectedRoom.notification_timeline ?? [],
        );
  if (
    notificationProofSummary !== undefined &&
    notificationProofSummary.missingCount > 0
  ) {
    return {
      alertType: "warning",
      color: "warning",
      detail: notificationProofSummary.detail,
      label: "AI proof missing",
      message: "AI notification proof incomplete",
      promptActionLabel: "Use review prompt",
    };
  }
  if (selectedRoom?.room_status === "closed") {
    return {
      alertType: "success",
      color: "default",
      detail:
        selectedRoom.close_reason || "The diagnosis room has been closed.",
      label: "Closed",
      message: "Room closed",
      promptActionLabel: "Use review prompt",
    };
  }
  if (selectedRoom?.latest_conclusion) {
    const conclusion = selectedRoom.latest_conclusion;
    if (conclusion.requires_human_review) {
      return {
        alertType: "warning",
        color: "warning",
        detail:
          "AI produced a retained conclusion that still needs operator review.",
        label: "Human review",
        message: "Conclusion needs review",
        promptActionLabel: "Use review prompt",
      };
    }
    return {
      alertType: "success",
      color: "success",
      detail: "AI produced a retained conclusion for operator confirmation.",
      label: "Review conclusion",
      message: "Conclusion ready",
      promptActionLabel: "Use review prompt",
    };
  }
  if (selectedRoom) {
    return selectedRoom.turn_count > 0
      ? {
          alertType: "info",
          color: "processing",
          detail: `Room ${selectedRoom.session_id} has ${selectedRoom.turn_count} turn(s) and no retained conclusion yet.`,
          label: "Continue review",
          message: "AI review in progress",
          promptActionLabel: "Use diagnosis prompt",
        }
      : {
          alertType: "info",
          color: "processing",
          detail: `Room ${selectedRoom.session_id} is ready for the first AI diagnosis turn.`,
          label: "Start review",
          message: "Room ready",
          promptActionLabel: "Use diagnosis prompt",
        };
  }
  if (pageContext.evidenceSnapshotID !== undefined) {
    return {
      alertType: "info",
      color: "processing",
      detail: `Evidence snapshot #${pageContext.evidenceSnapshotID} is selected for AI diagnosis.`,
      label: "Evidence selected",
      message: "Snapshot selected",
      promptActionLabel: "Use diagnosis prompt",
    };
  }
  return {
    alertType: "info",
    color: "default",
    detail: pageContext.description,
    label: "Room selected",
    message: pageContext.title || "Diagnosis room selected",
    promptActionLabel: "Use diagnosis prompt",
  };
}

function diagnosisHandoffItems(
  pageContext: DiagnosisPageContext,
  selectedRoom?: DiagnosisRoomSummary,
): DescriptionsProps["items"] {
  const latestNotification =
    selectedRoom === undefined
      ? null
      : latestDiagnosisRoomNotification(selectedRoom);
  return [
    {
      key: "snapshot",
      label: "Evidence snapshot",
      children: String(
        selectedRoom?.evidence_snapshot_id ??
          pageContext.evidenceSnapshotID ??
          "-",
      ),
    },
    {
      key: "session",
      label: "Session",
      children: selectedRoom?.session_id ?? pageContext.sessionID ?? "-",
    },
    {
      key: "room-status",
      label: "Room status",
      children: selectedRoom?.room_status ?? "-",
    },
    {
      key: "task-status",
      label: "Task status",
      children: selectedRoom?.task_status ?? "-",
    },
    {
      key: "turns",
      label: "Turns",
      children: selectedRoom ? String(selectedRoom.turn_count) : "-",
    },
    {
      key: "notification",
      label: "Latest notification",
      children: latestNotification
        ? `${notificationEventLabel(latestNotification.event_kind)} / ${latestNotification.provider_status}`
        : "-",
    },
    {
      key: "updated",
      label: "Last activity",
      children: selectedRoom ? formatDateTime(selectedRoom.updated_at) : "-",
    },
  ];
}

function notificationChannelReviewHref(
  notification: DiagnosisRoomNotificationTimelineEntry,
): string {
  return notification.notification_channel_profile_id === undefined
    ? "/settings/notification-channels"
    : notificationChannelEditHref(notification.notification_channel_profile_id);
}

function DiagnosisReviewQueuePanel({
  actionDisabledReason,
  canConfirmConclusion,
  connected,
  onConfirmConclusion,
  onOpenConnection,
  onRequestReassessment,
  onUseEvidencePlan,
  onUseFollowUp,
  panelRef,
  queueInput,
  title = "Review Queue",
}: {
  actionDisabledReason: string;
  canConfirmConclusion: boolean;
  connected: boolean;
  onConfirmConclusion: () => void;
  onOpenConnection?: () => void;
  onRequestReassessment: () => void;
  onUseEvidencePlan: (item: DiagnosisEvidenceRequest) => void;
  onUseFollowUp: (item: DiagnosisConsultationEvidenceRequest) => void;
  panelRef?: RefObject<HTMLElement | null>;
  queueInput: DiagnosisReviewQueueInput;
  title?: string;
}) {
  const items = diagnosisReviewQueueItems(queueInput);
  const summary = diagnosisReviewQueueSummary(items, queueInput);
  const actionPlan = diagnosisReviewQueueActionPlan(items, summary);
  const postEvidenceStatus = diagnosisReviewQueuePostEvidenceStatus(queueInput);
  const taskProgress = diagnosisReviewQueueTaskProgress(
    items,
    summary,
    postEvidenceStatus,
  );
  const actionGate = diagnosisReviewQueueActionGate({
    actionDisabledReason,
    connected,
  });

  return (
    <section
      aria-label="Diagnosis review queue"
      className="diagnosis-review-queue"
      ref={panelRef}
    >
      <div className="diagnosis-review-queue-header">
        <div>
          <Typography.Title level={3}>{title}</Typography.Title>
          <Typography.Text type="secondary">{summary.message}</Typography.Text>
        </div>
        <Space size={[6, 6]} wrap>
          {summary.blockingReason !== "" ? (
            <Tag color="warning">blocked</Tag>
          ) : null}
          {summary.canConfirm ? <Tag color="success">confirmable</Tag> : null}
          <Tag color="error">{summary.attention} attention</Tag>
          <Tag color="processing">{summary.pending} pending</Tag>
          <Tag color="success">{summary.ready} ready</Tag>
          <Tag>{summary.done} done</Tag>
        </Space>
      </div>
      <div
        aria-label="Diagnosis review task progress"
        className="diagnosis-review-task-progress"
      >
        <div className="diagnosis-review-task-progress-header">
          <Space size={[6, 6]} wrap>
            <Typography.Text strong>{taskProgress.statusLabel}</Typography.Text>
            <Tag color={reviewQueueTaskProgressTagColor(taskProgress.status)}>
              {taskProgress.completed}/{taskProgress.total} phases complete
            </Tag>
          </Space>
          <Progress
            percent={taskProgress.percent}
            size="small"
            status={reviewQueueTaskProgressBarStatus(taskProgress.status)}
          />
          <Typography.Text
            type={
              taskProgress.status === "blocked"
                ? "danger"
                : taskProgress.status === "ready"
                  ? "success"
                  : "secondary"
            }
          >
            {taskProgress.summary}
          </Typography.Text>
        </div>
        <Timeline
          className="diagnosis-review-task-timeline"
          items={reviewQueueTaskProgressTimelineItems({
            canConfirmConclusion,
            connected: !actionGate.disabled,
            actionDisabledReason: actionGate.reason,
            items,
            onConfirmConclusion,
            onOpenConnection,
            onRequestReassessment,
            onUseEvidencePlan,
            onUseFollowUp,
            taskProgress,
          })}
        />
      </div>
      <Alert
        aria-label="Review queue action plan"
        className="diagnosis-review-action-plan"
        description={
          actionPlan.actions.length === 0 ? (
            <Typography.Text>{actionPlan.message}</Typography.Text>
          ) : (
            <Space direction="vertical" size={6}>
              <Typography.Text>{actionPlan.message}</Typography.Text>
              <Space size={[6, 6]} wrap>
                {actionPlan.actions.map((item) => (
                  <Tag
                    color={reviewQueueStatusColor(item.status)}
                    key={item.key}
                  >
                    {item.title}
                  </Tag>
                ))}
                {actionPlan.remaining > 0 ? (
                  <Tag>+{actionPlan.remaining} more</Tag>
                ) : null}
              </Space>
            </Space>
          )
        }
        message="Confirmation action plan"
        showIcon
        type={reviewQueueActionPlanAlertType(actionPlan.status)}
      />
      {actionGate.disabled ? (
        <Alert
          action={
            actionGate.kind === "connection" && onOpenConnection ? (
              <Button
                icon={<ApiOutlined />}
                onClick={onOpenConnection}
                size="small"
              >
                Open connection
              </Button>
            ) : undefined
          }
          aria-label="Review queue action gate"
          className="diagnosis-review-action-plan"
          description={actionGate.reason}
          message={
            actionGate.kind === "connection"
              ? "Connect before submitting"
              : "Review actions unavailable"
          }
          showIcon
          type={actionGate.kind === "connection" ? "info" : "warning"}
        />
      ) : null}
      {postEvidenceStatus.status !== "none" ? (
        <Alert
          action={postEvidenceStatusAction({
            actionGate,
            onOpenConnection,
            onRequestReassessment,
            status: postEvidenceStatus,
          })}
          aria-label="Post supplemental evidence review status"
          className="diagnosis-review-action-plan"
          description={
            <Space direction="vertical" size={6}>
              <Typography.Text>{postEvidenceStatus.detail}</Typography.Text>
              <Space size={[6, 6]} wrap>
                <Tag color="processing">
                  submitted {postEvidenceStatus.submitted}
                </Tag>
                <Tag color="success">
                  reviewed {postEvidenceStatus.reviewed}
                </Tag>
                <Tag
                  color={
                    postEvidenceStatus.unresolved > 0 ? "warning" : "success"
                  }
                >
                  unresolved {postEvidenceStatus.unresolved}
                </Tag>
              </Space>
            </Space>
          }
          message={postEvidenceStatus.label}
          showIcon
          type={reviewQueuePostEvidenceAlertType(postEvidenceStatus.status)}
        />
      ) : null}
      <List
        className="diagnosis-review-list"
        dataSource={items}
        renderItem={(item) => (
          <List.Item
            actions={reviewQueueActions(
              item,
              !actionGate.disabled,
              canConfirmConclusion,
              onUseFollowUp,
              onUseEvidencePlan,
              onConfirmConclusion,
              actionGate.reason,
            )}
          >
            <List.Item.Meta
              description={item.detail}
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.title}</span>
                  <Tag color={reviewQueueStatusColor(item.status)}>
                    {item.status}
                  </Tag>
                  <Tag>{item.tag}</Tag>
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

function postEvidenceStatusAction({
  actionGate,
  onOpenConnection,
  onRequestReassessment,
  status,
}: {
  actionGate: DiagnosisReviewQueueActionGate;
  onOpenConnection?: () => void;
  onRequestReassessment: () => void;
  status: DiagnosisReviewQueuePostEvidenceStatus;
}): ReactNode {
  if (status.status !== "submitted") {
    return undefined;
  }
  if (!actionGate.disabled) {
    return (
      <Button
        icon={<ReloadOutlined />}
        onClick={onRequestReassessment}
        size="small"
        type="primary"
      >
        Ask AI to reassess
      </Button>
    );
  }
  if (actionGate.kind === "connection" && onOpenConnection !== undefined) {
    return (
      <Button icon={<ApiOutlined />} onClick={onOpenConnection} size="small">
        Open connection
      </Button>
    );
  }
  return undefined;
}

function SupplementalEvidenceHistoryPanel({
  directoryUsersBySubject,
  items,
  missingEvidenceRequests,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  items: DiagnosisSupplementalEvidenceRecord[];
  missingEvidenceRequests: DiagnosisConsultationEvidenceRequest[];
}) {
  if (items.length === 0) {
    return null;
  }
  const unresolvedRequestsByTopic = new Map(
    missingEvidenceRequests.map((request) => [
      supplementalEvidenceTopicKey(request.label),
      request,
    ]),
  );
  return (
    <section
      aria-label="Supplemental evidence history"
      className="diagnosis-supplemental-history"
    >
      <div className="diagnosis-supplemental-history-header">
        <Typography.Title level={3}>
          Supplemental Evidence History
        </Typography.Title>
        <Typography.Text type="secondary">
          {items.length} update(s)
        </Typography.Text>
      </div>
      <List
        className="diagnosis-evidence-list"
        dataSource={items}
        renderItem={(item, index) => {
          const unresolvedRequest = unresolvedRequestsByTopic.get(
            supplementalEvidenceTopicKey(item.label),
          );
          return (
            <List.Item
              className="diagnosis-evidence-item"
              key={supplementalEvidenceRecordKey(item, index)}
            >
              <List.Item.Meta
                description={
                  <Space direction="vertical" size={4}>
                    <Typography.Text>{item.detail}</Typography.Text>
                    {unresolvedRequest ? (
                      <Typography.Text type="secondary">
                        Latest request: {unresolvedRequest.detail}
                      </Typography.Text>
                    ) : null}
                    <Typography.Text>{item.evidence}</Typography.Text>
                    <ActorSubjectTags
                      directoryUsersBySubject={directoryUsersBySubject}
                      label="Provided by"
                      subject={item.actor_subject}
                    />
                    <Typography.Text type="secondary">
                      Turn {item.user_sequence} to {item.assistant_sequence} -{" "}
                      {formatDateTime(item.provided_at)}
                    </Typography.Text>
                  </Space>
                }
                title={
                  <Space size={[6, 6]} wrap>
                    <span>{item.label}</span>
                    <Tag
                      color={priorityColor(
                        unresolvedRequest?.priority ?? item.priority,
                      )}
                    >
                      {unresolvedRequest?.priority ?? item.priority}
                    </Tag>
                    <Tag color={unresolvedRequest ? "warning" : "success"}>
                      {unresolvedRequest ? "latest request" : "retained"}
                    </Tag>
                    <Tag>user turn {item.user_turn_id}</Tag>
                    <Tag>assistant turn {item.assistant_turn_id}</Tag>
                  </Space>
                }
              />
            </List.Item>
          );
        }}
        size="small"
      />
    </section>
  );
}

function CollaborationParticipantsPanel({
  directoryUsersBySubject,
  participants,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  participants: DiagnosisCollaborationParticipant[];
}) {
  if (participants.length === 0) {
    return null;
  }
  const identityCoverage = diagnosisCollaborationIdentityCoverage(
    participants,
    directoryUsersBySubject,
  );
  return (
    <section
      aria-label="Diagnosis room participants"
      className="diagnosis-collaboration"
    >
      <div className="diagnosis-collaboration-header">
        <Typography.Title level={3}>Participants</Typography.Title>
        <Typography.Text type="secondary">
          {participants.length} actor(s)
        </Typography.Text>
      </div>
      <Alert
        description={identityCoverage.detail}
        message={`Identity coverage: ${identityCoverage.summary}`}
        showIcon
        type={diagnosisCollaborationIdentityCoverageAlertType(
          identityCoverage.status,
        )}
      />
      <List
        className="diagnosis-collaboration-list"
        dataSource={participants}
        renderItem={(participant) => {
          const profile = diagnosisCollaborationParticipantProfile(
            participant,
            directoryUsersBySubject,
          );
          return (
            <List.Item key={participant.subject}>
              <List.Item.Meta
                description={
                  <Space size={[6, 6]} wrap>
                    <Tag>{participantActivityLabel(participant)}</Tag>
                    {profile.detailTags.map((tag) => (
                      <Tag key={tag}>{tag}</Tag>
                    ))}
                    {profile.active === false ? (
                      <Tag color="warning">inactive</Tag>
                    ) : null}
                    {!participant.isSystem && !profile.matchedDirectoryUser ? (
                      <Tag color="default">not synced</Tag>
                    ) : null}
                    {participant.isSystem ? (
                      <Tag color="default">system</Tag>
                    ) : null}
                  </Space>
                }
                title={
                  <Space size={[6, 6]} wrap>
                    <Typography.Text strong>
                      {profile.displayName}
                    </Typography.Text>
                    {participant.roles.map((role) => (
                      <Tag color={collaborationRoleColor(role)} key={role}>
                        {diagnosisCollaborationRoleLabel(role)}
                      </Tag>
                    ))}
                  </Space>
                }
              />
            </List.Item>
          );
        }}
        size="small"
      />
    </section>
  );
}

function diagnosisCollaborationIdentityCoverageAlertType(
  status: ReturnType<typeof diagnosisCollaborationIdentityCoverage>["status"],
): "info" | "success" | "warning" {
  switch (status) {
    case "empty":
      return "info";
    case "ready":
      return "success";
    case "review":
      return "warning";
  }
}

type DiagnosisWorkflowReadinessAction = {
  disabled?: boolean;
  icon: ReactNode;
  label: string;
  onClick: () => void;
  title: string;
};

type DiagnosisWorkflowReadinessActions = Partial<
  Record<DiagnosisWorkflowReadinessItem["key"], DiagnosisWorkflowReadinessAction>
>;

function DiagnosisWorkflowReadinessPanel({
  actions,
  items,
}: {
  actions: DiagnosisWorkflowReadinessActions;
  items: DiagnosisWorkflowReadinessItem[];
}) {
  const readyCount = items.filter((item) => item.status === "ready").length;
  const percent =
    items.length === 0 ? 0 : Math.round((readyCount / items.length) * 100);
  const nextItem =
    items.find((item) => item.status === "blocked") ??
    items.find((item) => item.status === "attention") ??
    items.find((item) => item.status === "pending") ??
    items[0];
  const nextAction = nextItem ? actions[nextItem.key] : undefined;
  const summaryStatus = workflowReadinessSummaryStatus(items);
  return (
    <section
      aria-label="Diagnosis workflow readiness"
      className="diagnosis-workflow-readiness"
    >
      <div className="diagnosis-workflow-readiness-header">
        <Space size={[8, 8]} wrap>
          <Typography.Title level={3}>Operational Readiness</Typography.Title>
          <Tag color={workflowReadinessStatusColor(summaryStatus)}>
            {workflowReadinessStatusLabel(summaryStatus)}
          </Tag>
        </Space>
        <Typography.Text type="secondary">
          {readyCount}/{items.length} ready
        </Typography.Text>
      </div>
      <Progress
        percent={percent}
        showInfo={false}
        size="small"
        status={workflowReadinessProgressStatus(summaryStatus)}
      />
      {nextItem ? (
        <Alert
          action={
            nextAction ? (
              <WorkflowReadinessActionButton action={nextAction} primary />
            ) : undefined
          }
          className="diagnosis-workflow-readiness-next"
          description={nextItem.detail}
          message={`${nextItem.label}: ${workflowReadinessStatusLabel(nextItem.status)}`}
          showIcon
          type={workflowReadinessAlertType(nextItem.status)}
        />
      ) : null}
      <div className="diagnosis-workflow-readiness-grid">
        {items.map((item) => {
          const action = actions[item.key];
          return (
            <div
              className={`diagnosis-workflow-readiness-item diagnosis-workflow-readiness-item-${item.status}`}
              key={item.key}
            >
              <div className="diagnosis-workflow-readiness-item-header">
                <Space size={[6, 6]} wrap>
                  <Typography.Text strong>{item.label}</Typography.Text>
                  <Tag color={workflowReadinessStatusColor(item.status)}>
                    {workflowReadinessStatusLabel(item.status)}
                  </Tag>
                  {item.metric ? <Tag>{item.metric}</Tag> : null}
                </Space>
                {action ? (
                  <WorkflowReadinessActionButton action={action} />
                ) : null}
              </div>
              <Typography.Text type="secondary">{item.detail}</Typography.Text>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function WorkflowReadinessActionButton({
  action,
  primary,
}: {
  action: DiagnosisWorkflowReadinessAction;
  primary?: boolean;
}) {
  return (
    <TooltipAction disabled={action.disabled === true} title={action.title}>
      <Button
        disabled={action.disabled === true}
        icon={action.icon}
        onClick={action.onClick}
        size="small"
        type={primary ? "primary" : "default"}
      >
        {action.label}
      </Button>
    </TooltipAction>
  );
}

function DiagnosisRoomPermissionPanel({
  authorization,
  enforced,
  items,
}: {
  authorization: CurrentRBACAuthorizations;
  enforced: boolean;
  items: DiagnosisRoomRBACPermissionItem[];
}) {
  const hasDeniedPermission = items.some((item) => item.status === "denied");
  const hasCheckingPermission = items.some((item) => item.status === "checking");
  return (
    <section
      aria-label="Diagnosis room permissions"
      className="diagnosis-room-permissions"
    >
      <div className="diagnosis-room-permissions-header">
        <Typography.Title level={3}>Permissions</Typography.Title>
        <Space size={[6, 6]} wrap>
          <Tag color={enforced ? "green" : "blue"}>
            {enforced ? "RBAC enforced" : "RBAC not enforced"}
          </Tag>
          {hasCheckingPermission ? <Tag color="processing">Checking</Tag> : null}
          {hasDeniedPermission ? <Tag color="red">Denied actions</Tag> : null}
        </Space>
      </div>
      <Descriptions
        column={1}
        items={diagnosisRoomAuthorizationDescriptionItems({
          authorization,
          enforced,
        })}
        size="small"
      />
      <List
        className="diagnosis-room-permissions-list"
        dataSource={items}
        renderItem={(item) => (
          <List.Item key={item.action}>
            <List.Item.Meta
              description={
                <Space size={[6, 6]} wrap>
                  <Tag>{item.permission}</Tag>
                  <Tag>{item.scopeLabel}</Tag>
                </Space>
              }
              title={
                <Space size={[6, 6]} wrap>
                  <Typography.Text strong>{item.label}</Typography.Text>
                  <Tag
                    color={diagnosisRoomRBACPermissionStatusColor(item.status)}
                  >
                    {diagnosisRoomRBACPermissionStatusLabel(item.status)}
                  </Tag>
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

function workflowReadinessSummaryStatus(
  items: DiagnosisWorkflowReadinessItem[],
): DiagnosisWorkflowReadinessStatus {
  if (items.some((item) => item.status === "blocked")) {
    return "blocked";
  }
  if (items.some((item) => item.status === "attention")) {
    return "attention";
  }
  if (items.some((item) => item.status === "pending")) {
    return "pending";
  }
  return "ready";
}

function workflowReadinessStatusColor(
  status: DiagnosisWorkflowReadinessStatus,
): string {
  switch (status) {
    case "attention":
      return "warning";
    case "blocked":
      return "error";
    case "pending":
      return "processing";
    case "ready":
      return "success";
  }
}

function workflowReadinessStatusLabel(
  status: DiagnosisWorkflowReadinessStatus,
): string {
  switch (status) {
    case "attention":
      return "Needs attention";
    case "blocked":
      return "Blocked";
    case "pending":
      return "Pending";
    case "ready":
      return "Ready";
  }
}

function workflowReadinessProgressStatus(
  status: DiagnosisWorkflowReadinessStatus,
): "active" | "exception" | "normal" | "success" {
  switch (status) {
    case "blocked":
      return "exception";
    case "ready":
      return "success";
    case "attention":
    case "pending":
      return "active";
  }
}

function workflowReadinessAlertType(
  status: DiagnosisWorkflowReadinessStatus,
): "error" | "info" | "success" | "warning" {
  switch (status) {
    case "attention":
      return "warning";
    case "blocked":
      return "error";
    case "pending":
      return "info";
    case "ready":
      return "success";
  }
}

function diagnosisRoomAuthorizationDescriptionItems({
  authorization,
  enforced,
}: {
  authorization: CurrentRBACAuthorizations;
  enforced: boolean;
}): DescriptionsProps["items"] {
  if (!enforced) {
    return [
      {
        key: "subject",
        label: "Subject",
        children: "Direct credential flow",
      },
      {
        key: "directory",
        label: "Directory profile",
        children: <Tag color="blue">Not required</Tag>,
      },
      {
        key: "departments",
        label: "Departments",
        children: <Tag color="blue">Not required</Tag>,
      },
    ];
  }
  if (authorization.state.kind === "error") {
    return [
      {
        key: "subject",
        label: "Subject",
        children: <Tag color="orange">Unavailable</Tag>,
      },
      {
        key: "directory",
        label: "Directory profile",
        children: <Tag color="orange">Unavailable</Tag>,
      },
      {
        key: "departments",
        label: "Departments",
        children: <Tag color="orange">Unavailable</Tag>,
      },
    ];
  }
  if (authorization.state.kind === "loading" || authorization.isChecking) {
    return [
      {
        key: "subject",
        label: "Subject",
        children: <Tag color="processing">Checking</Tag>,
      },
      {
        key: "directory",
        label: "Directory profile",
        children: <Tag color="processing">Checking</Tag>,
      },
      {
        key: "departments",
        label: "Departments",
        children: <Tag color="processing">Checking</Tag>,
      },
    ];
  }

  const directoryUsers = authorization.state.directoryUsers;
  const activeDirectoryUsers = directoryUsers.filter((user) => user.active);
  const primaryDirectoryUser = activeDirectoryUsers[0] ?? directoryUsers[0];
  return [
    {
      key: "subject",
      label: "Subject",
      children: (
        <Typography.Text copyable>{authorization.state.subject}</Typography.Text>
      ),
    },
    {
      key: "directory",
      label: "Directory profile",
      children: primaryDirectoryUser ? (
        <Space size={[6, 6]} wrap>
          <Typography.Text>
            {directoryUserLabel(primaryDirectoryUser)}
          </Typography.Text>
          <Tag color={primaryDirectoryUser.active ? "green" : "orange"}>
            {primaryDirectoryUser.active ? "Active" : "Inactive"}
          </Tag>
        </Space>
      ) : (
        <Tag color="orange">Not synced</Tag>
      ),
    },
    {
      key: "departments",
      label: "Departments",
      children:
        authorization.state.departmentKeys.length > 0 ? (
          <Space size={[6, 6]} wrap>
            {authorization.state.departmentKeys.map((departmentKey) => (
              <Tag key={departmentKey}>{departmentKey}</Tag>
            ))}
          </Space>
        ) : (
          <Tag color="orange">None</Tag>
        ),
    },
  ];
}

function diagnosisRoomRBACPermissionStatusColor(
  status: DiagnosisRoomRBACPermissionStatus,
): string {
  switch (status) {
    case "allowed":
      return "green";
    case "checking":
      return "processing";
    case "denied":
      return "red";
    case "not-enforced":
      return "blue";
    case "not-selected":
      return "default";
  }
}

function diagnosisRoomRBACPermissionStatusLabel(
  status: DiagnosisRoomRBACPermissionStatus,
): string {
  switch (status) {
    case "allowed":
      return "Allowed";
    case "checking":
      return "Checking";
    case "denied":
      return "Denied";
    case "not-enforced":
      return "Not enforced";
    case "not-selected":
      return "No room";
  }
}

function ActorSubjectTags({
  directoryUsersBySubject,
  label,
  subject,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  label?: string;
  subject?: string;
}) {
  const normalizedSubject = subject?.trim() ?? "";
  if (normalizedSubject === "") {
    return null;
  }
  const profile = diagnosisCollaborationActorProfile(
    normalizedSubject,
    directoryUsersBySubject,
  );
  const isSystem = diagnosisCollaborationSubjectIsSystem(normalizedSubject);
  return (
    <Space size={[6, 6]} wrap>
      {label ? <Typography.Text type="secondary">{label}</Typography.Text> : null}
      <Tag color={profile.matchedDirectoryUser ? "processing" : "default"}>
        {profile.displayName}
      </Tag>
      {profile.displayName !== profile.subject ? (
        <Tag>{profile.subject}</Tag>
      ) : null}
      {profile.detailTags.slice(0, 2).map((tag) => (
        <Tag key={tag}>{tag}</Tag>
      ))}
      {profile.active === false ? <Tag color="warning">inactive</Tag> : null}
      {!isSystem && !profile.matchedDirectoryUser ? (
        <Tag color="default">not synced</Tag>
      ) : null}
      {isSystem ? <Tag color="default">system</Tag> : null}
    </Space>
  );
}

function participantActivityLabel(
  participant: DiagnosisCollaborationParticipant,
): string {
  const parts: string[] = [];
  if (participant.messageCount > 0) {
    parts.push(`${participant.messageCount} message`);
  }
  if (participant.evidenceCollectionCount > 0) {
    parts.push(`${participant.evidenceCollectionCount} evidence`);
  }
  if (participant.supplementalEvidenceCount > 0) {
    parts.push(`${participant.supplementalEvidenceCount} supplemental`);
  }
  if (participant.confirmedConclusion) {
    parts.push("confirmed");
  }
  return parts.length > 0 ? parts.join(" / ") : "no activity";
}

function collaborationRoleColor(
  role: DiagnosisCollaborationParticipantRole,
): string {
  switch (role) {
    case "owner":
      return "processing";
    case "message":
      return "blue";
    case "evidence":
      return "purple";
    case "supplemental_evidence":
      return "geekblue";
    case "confirmation":
      return "success";
    case "assistant":
      return "default";
  }
}

function OperatorEvidenceCollectionPanel({
  alertContext,
  clientReady,
  connected,
  form,
  onRefreshTemplates,
  onSubmit,
  templateError,
  templates,
  templatesLoading,
}: {
  alertContext?: DiagnosisAlertContext;
  clientReady: boolean;
  connected: boolean;
  form: FormInstance<OperatorEvidenceFormValues>;
  onRefreshTemplates: () => void;
  onSubmit: (values: OperatorEvidenceFormValues) => void;
  templateError: string;
  templates: DiagnosisToolTemplate[];
  templatesLoading: boolean;
}) {
  const selectedTool = Form.useWatch("tool", form) ?? "active_alerts";
  const selectedTemplateID = Form.useWatch("selectedTemplateID", form) ?? null;
  const requiresMetricInput =
    selectedTool === "metric_query" || selectedTool === "metric_range_query";
  const requiresRangeInput = selectedTool === "metric_range_query";
  const selectedTemplate = useMemo(
    () =>
      templates.find((template) => template.id === selectedTemplateID) ?? null,
    [selectedTemplateID, templates],
  );
  const selectedTemplateParameterized =
    selectedTemplate !== null &&
    operatorEvidenceTemplateHasParameterizedQuery(selectedTemplate);
  const selectedTemplateLocksQuery =
    selectedTemplate !== null &&
    selectedTemplate.tool !== "active_alerts" &&
    !selectedTemplateParameterized;
  const recommendations = useMemo(
    () => operatorEvidenceRecommendations(templates, alertContext),
    [alertContext, templates],
  );

  return (
    <section
      aria-label="Operator evidence collection"
      className="diagnosis-operator-evidence"
    >
      <div className="diagnosis-operator-evidence-header">
        <div>
          <Typography.Title level={3}>
            Operator Evidence Collection
          </Typography.Title>
          <Typography.Text type="secondary">
            Direct evidence collection for the active diagnosis room.
          </Typography.Text>
        </div>
        <Space size={[6, 6]} wrap>
          <Tag color={connected ? "processing" : "default"}>
            {connected ? "ready" : "disconnected"}
          </Tag>
          <Button
            disabled={!clientReady || templatesLoading}
            icon={<ReloadOutlined />}
            loading={templatesLoading}
            onClick={onRefreshTemplates}
            size="small"
          >
            Refresh templates
          </Button>
        </Space>
      </div>
      {templateError !== "" ? (
        <Alert
          description={templateError}
          message="Tool templates unavailable"
          showIcon
          type="warning"
        />
      ) : templates.length === 0 ? (
        <Alert
          description="Create and enable diagnosis tool templates in Settings to use template-backed collection."
          message="No enabled tool templates"
          showIcon
          type="info"
        />
      ) : null}
      <OperatorEvidenceRecommendationPanel
        connected={connected}
        form={form}
        recommendations={recommendations}
      />
      <Form<OperatorEvidenceFormValues>
        form={form}
        initialValues={{
          reason: "Collect operator-selected evidence.",
          tool: "active_alerts",
        }}
        layout="vertical"
        onFinish={onSubmit}
      >
        <Form.Item label="Template" name="selectedTemplateID">
          <Select
            allowClear
            aria-label="Operator evidence template"
            disabled={!connected}
            loading={templatesLoading}
            onChange={(value: number | undefined) => {
              applyOperatorEvidenceTemplate(form, templates, value);
            }}
            optionFilterProp="label"
            options={operatorEvidenceTemplateOptions(templates)}
            placeholder="Select enabled tool template"
            showSearch
          />
        </Form.Item>
        {selectedTemplate ? (
          <Alert
            description={operatorEvidenceTemplateSummary(selectedTemplate)}
            message={`Template #${selectedTemplate.id}: ${selectedTemplate.name}`}
            showIcon
            type={
              operatorEvidenceTemplateHasParameterizedQuery(selectedTemplate)
                ? "warning"
                : "info"
            }
          />
        ) : null}
        <div className="diagnosis-operator-evidence-grid">
          <Form.Item
            label="Tool"
            name="tool"
            rules={[{ required: true, message: "Tool is required." }]}
          >
            <Select
              aria-label="Operator evidence tool"
              className="diagnosis-operator-evidence-tool-select"
              disabled={!connected}
              onChange={() => form.setFieldValue("selectedTemplateID", null)}
              options={[
                { label: "active_alerts", value: "active_alerts" },
                { label: "metric_query", value: "metric_query" },
                { label: "metric_range_query", value: "metric_range_query" },
              ]}
            />
          </Form.Item>
          <Form.Item
            label="Reason"
            name="reason"
            rules={[
              { required: true, message: "Reason is required." },
              {
                validator: (_, value: unknown) =>
                  operatorEvidenceSingleLine(value, "Reason", 500),
              },
            ]}
          >
            <Input disabled={!connected} />
          </Form.Item>
          <Form.Item
            label="Template ID"
            name="templateID"
            rules={[
              {
                validator: (_, value: unknown) =>
                  operatorEvidenceOptionalInteger(value, "Template ID"),
              },
            ]}
          >
            <InputNumber
              disabled={!connected}
              min={1}
              precision={0}
              style={{ width: "100%" }}
            />
          </Form.Item>
          <Form.Item
            label="Alert source profile"
            name="alertSourceProfileID"
            rules={[
              {
                validator: (_, value: unknown) =>
                  operatorEvidenceOptionalInteger(
                    value,
                    "Alert source profile",
                  ),
              },
            ]}
          >
            <InputNumber
              disabled={!connected}
              min={1}
              precision={0}
              style={{ width: "100%" }}
            />
          </Form.Item>
          {requiresMetricInput ? (
            <Form.Item
              dependencies={["selectedTemplateID", "templateID", "tool"]}
              label="Query"
              name="query"
              preserve={false}
              rules={[
                ({ getFieldValue }) => ({
                  validator: (_, value: unknown) => {
                    const currentTool = getFieldValue(
                      "tool",
                    ) as OperatorEvidenceTool;
                    const currentTemplateID = getFieldValue("templateID");
                    if (
                      (currentTool === "metric_query" ||
                        currentTool === "metric_range_query") &&
                      !isPositiveSafeInteger(currentTemplateID) &&
                      (typeof value !== "string" || value.trim() === "")
                    ) {
                      return Promise.reject(
                        new Error("Query is required without a template ID."),
                      );
                    }
                    if (
                      selectedTemplateParameterized &&
                      (typeof value !== "string" || value.trim() === "")
                    ) {
                      return Promise.reject(
                        new Error(
                          "Concrete query is required for this template.",
                        ),
                      );
                    }
                    return operatorEvidenceOptionalSingleLine(
                      value,
                      "Query",
                      500,
                    );
                  },
                }),
              ]}
            >
              <Input
                disabled={!connected}
                readOnly={selectedTemplateLocksQuery}
              />
            </Form.Item>
          ) : null}
          {requiresRangeInput ? (
            <>
              <Form.Item
                dependencies={["stepSeconds", "templateID"]}
                label="Window seconds"
                name="windowSeconds"
                preserve={false}
                rules={[
                  ({ getFieldValue }) => ({
                    validator: (_, value: unknown) =>
                      operatorEvidenceRangeSeconds(
                        value,
                        getFieldValue("stepSeconds"),
                        getFieldValue("templateID"),
                        "Window seconds",
                      ),
                  }),
                ]}
              >
                <InputNumber
                  disabled={!connected}
                  min={15}
                  precision={0}
                  style={{ width: "100%" }}
                />
              </Form.Item>
              <Form.Item
                dependencies={["windowSeconds", "templateID"]}
                label="Step seconds"
                name="stepSeconds"
                preserve={false}
                rules={[
                  ({ getFieldValue }) => ({
                    validator: (_, value: unknown) =>
                      operatorEvidenceRangeSeconds(
                        value,
                        getFieldValue("windowSeconds"),
                        getFieldValue("templateID"),
                        "Step seconds",
                      ),
                  }),
                ]}
              >
                <InputNumber
                  disabled={!connected}
                  min={15}
                  precision={0}
                  style={{ width: "100%" }}
                />
              </Form.Item>
            </>
          ) : null}
          <Form.Item
            label="Limit"
            name="limit"
            rules={[
              {
                validator: (_, value: unknown) =>
                  operatorEvidenceOptionalInteger(value, "Limit"),
              },
            ]}
          >
            <InputNumber
              disabled={!connected}
              min={1}
              precision={0}
              style={{ width: "100%" }}
            />
          </Form.Item>
        </div>
        <Button
          disabled={!connected}
          htmlType="submit"
          icon={<PlayCircleOutlined />}
          type="primary"
        >
          Collect operator evidence
        </Button>
      </Form>
    </section>
  );
}

function OperatorEvidenceRecommendationPanel({
  connected,
  form,
  recommendations,
}: {
  connected: boolean;
  form: FormInstance<OperatorEvidenceFormValues>;
  recommendations: OperatorEvidenceRecommendation[];
}) {
  if (recommendations.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Recommended operator evidence"
      className="diagnosis-operator-recommendations"
    >
      <div className="diagnosis-operator-recommendations-header">
        <div>
          <Typography.Title level={4}>Recommended Evidence</Typography.Title>
          <Typography.Text type="secondary">
            Template-backed collection candidates prepared from the current
            diagnosis context.
          </Typography.Text>
        </div>
        <Tag color="processing">{recommendations.length} candidate(s)</Tag>
      </div>
      <List
        className="diagnosis-review-list"
        dataSource={recommendations}
        renderItem={(item) => (
          <List.Item
            actions={[
              <TooltipAction
                disabled={!connected || !item.ready}
                key="use-recommendation"
                title={
                  item.ready
                    ? "Prefill operator evidence collection."
                    : item.disabledReason
                }
              >
                <Button
                  aria-label={`Use recommendation ${item.template.name}`}
                  disabled={!connected || !item.ready}
                  icon={<FormOutlined />}
                  onClick={() => form.setFieldsValue(item.formValues)}
                  size="small"
                  type="link"
                >
                  Use
                </Button>
              </TooltipAction>,
            ]}
          >
            <List.Item.Meta
              description={
                <Space direction="vertical" size={4}>
                  <Typography.Text>{item.detail}</Typography.Text>
                  {item.disabledReason !== "" ? (
                    <Typography.Text type="secondary">
                      {item.disabledReason}
                    </Typography.Text>
                  ) : null}
                </Space>
              }
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.title}</span>
                  <Tag color={item.ready ? "success" : "warning"}>
                    {item.ready ? "ready" : "needs context"}
                  </Tag>
                  {item.sourceMatches ? (
                    <Tag color="blue">source match</Tag>
                  ) : null}
                  <Tag>{item.tag}</Tag>
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

function DiagnosisWorkQueuePanel({
  filter,
  handoffsResult,
  onFilterChange,
  roomsResult,
}: {
  filter: DiagnosisWorkQueueFilter;
  handoffsResult?: ApiResult<DiagnosisHandoffListResponse>;
  onFilterChange: (filter: DiagnosisWorkQueueFilter) => void;
  roomsResult?: ApiResult<DiagnosisRoomListResponse>;
}) {
  const options = diagnosisWorkQueueOptions(roomsResult, handoffsResult).map(
    (option) => ({
      label: `${option.label} ${option.count}`,
      value: option.value,
    }),
  );
  return (
    <Card
      className="diagnosis-room-panel diagnosis-work-queue-panel settings-overview-card"
      title="Diagnosis Work Queue"
    >
      <Segmented
        aria-label="Diagnosis work queue filter"
        onChange={(value) => onFilterChange(value as DiagnosisWorkQueueFilter)}
        options={options}
        value={filter}
      />
    </Card>
  );
}

function diagnosisWorkQueueOptions(
  roomsResult?: ApiResult<DiagnosisRoomListResponse>,
  handoffsResult?: ApiResult<DiagnosisHandoffListResponse>,
): Array<{ count: number; label: string; value: DiagnosisWorkQueueFilter }> {
  const rooms = roomsResult?.ok ? roomsResult.data.items : [];
  const handoffCount = handoffsResult?.ok
    ? handoffsResult.data.items.length
    : 0;
  const roomOptions = diagnosisRoomQueueOptions(rooms);
  const roomOptionCount = (filter: DiagnosisRoomQueueFilter) =>
    roomOptions.find((option) => option.value === filter)?.count ?? 0;
  return [
    {
      count: handoffCount + roomOptionCount("all"),
      label: "All",
      value: "all",
    },
    { count: handoffCount, label: "Needs room", value: "handoffs" },
    {
      count: roomOptionCount("attention"),
      label: "Attention",
      value: "attention",
    },
    { count: roomOptionCount("ready"), label: "Ready", value: "ready" },
    { count: roomOptionCount("active"), label: "Active", value: "active" },
    { count: roomOptionCount("closed"), label: "Closed", value: "closed" },
  ];
}

function workQueueShowsHandoffs(filter: DiagnosisWorkQueueFilter): boolean {
  return filter === "all" || filter === "handoffs";
}

function workQueueShowsRooms(filter: DiagnosisWorkQueueFilter): boolean {
  return filter !== "handoffs";
}

function diagnosisRoomFilterForWorkQueue(
  filter: DiagnosisWorkQueueFilter,
): DiagnosisRoomQueueFilter {
  return filter === "handoffs" ? "all" : filter;
}

function DiagnosisHandoffBacklogPanel({
  clientReady,
  isFetching,
  onPrepareRoom,
  onRefresh,
  result,
  selectedEvidenceSnapshotID,
}: {
  clientReady: boolean;
  isFetching: boolean;
  onPrepareRoom: (item: DiagnosisHandoffBacklogItem) => void;
  onRefresh: () => void;
  result: ApiResult<DiagnosisHandoffListResponse>;
  selectedEvidenceSnapshotID?: number;
}) {
  return (
    <Card
      className="diagnosis-room-panel settings-overview-card"
      extra={
        <Space size={8}>
          {result.ok ? (
            <Typography.Text type="secondary">
              {result.data.items.length} handoff(s)
              {isFetching ? " / refreshing" : ""}
            </Typography.Text>
          ) : null}
          <Tooltip title="Refresh AI handoff backlog">
            <Button
              aria-label="Refresh AI handoff backlog"
              icon={<ReloadOutlined />}
              loading={isFetching}
              onClick={onRefresh}
              size="small"
            />
          </Tooltip>
        </Space>
      }
      title="AI Handoff Backlog"
    >
      {!result.ok ? (
        <Alert
          description={result.error.message}
          message={
            result.error.status
              ? `HTTP ${result.error.status}`
              : "Request failed"
          }
          showIcon
          type="warning"
        />
      ) : result.data.items.length === 0 ? (
        <Empty
          description="No handoffs need a diagnosis room"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      ) : (
        <List
          aria-label="AI handoff backlog"
          className="diagnosis-room-list"
          dataSource={result.data.items}
          renderItem={(item) => {
            const snapshot = item.evidence_snapshot;
            const primaryAlert = item.alerts[0];
            const selected = snapshot.id === selectedEvidenceSnapshotID;
            const severity =
              primaryAlert === undefined ? "info" : alertSeverity(primaryAlert);
            return (
              <List.Item
                actions={[
                  <TooltipAction
                    disabled={!clientReady}
                    key="prepare"
                    title="Prefill the create form with this evidence snapshot."
                  >
                    <Button
                      aria-label={`Prepare handoff snapshot ${snapshot.id}`}
                      disabled={!clientReady}
                      icon={<PlusCircleOutlined />}
                      onClick={() => onPrepareRoom(item)}
                      size="small"
                      type="link"
                    >
                      Prepare
                    </Button>
                  </TooltipAction>,
                ]}
                className={`diagnosis-room-list-item${selected ? " diagnosis-room-list-item-selected" : ""}`}
                key={snapshot.id}
              >
                <List.Item.Meta
                  description={
                    <Space
                      className="diagnosis-room-list-meta"
                      size={[6, 6]}
                      wrap
                    >
                      <span>group #{snapshot.alert_group_id}</span>
                      <span>{item.alerts.length} alert(s)</span>
                      <span>{snapshot.created_by_workflow || "manual"}</span>
                      <span>created {formatDateTime(snapshot.created_at)}</span>
                      {primaryAlert !== undefined ? (
                        <span>{alertSummary(primaryAlert)}</span>
                      ) : null}
                    </Space>
                  }
                  title={
                    <Space size={[6, 6]} wrap>
                      <span>
                        {primaryAlert === undefined
                          ? `Evidence snapshot #${snapshot.id}`
                          : alertName(primaryAlert)}
                      </span>
                      {selected ? <Tag color="processing">selected</Tag> : null}
                      <Tag color={severityColor(severity)}>{severity}</Tag>
                      <Tag color={snapshotStatusColor(snapshot.status)}>
                        snapshot #{snapshot.id}
                      </Tag>
                      <Tag color="warning">needs room</Tag>
                    </Space>
                  }
                />
              </List.Item>
            );
          }}
          size="small"
        />
      )}
    </Card>
  );
}

function RecentDiagnosisRoomsPanel({
  clientReady,
  closingSessionID,
  filter,
  isFetching,
  onAdministerRoomBlocked,
  onPrepareRoomRebuild,
  onRefresh,
  onRetryNotification,
  onSelectRoom,
  result,
  retryingNotificationKey,
  selectedSessionID,
}: {
  clientReady: boolean;
  closingSessionID: string;
  filter: DiagnosisRoomQueueFilter;
  isFetching: boolean;
  onAdministerRoomBlocked: (sessionID: string) => string;
  onPrepareRoomRebuild: (room: DiagnosisRoomSummary) => void;
  onRefresh: () => void;
  onRetryNotification: (
    room: DiagnosisRoomSummary,
    entry: DiagnosisRoomNotificationTimelineEntry,
  ) => void;
  onSelectRoom: (room: DiagnosisRoomSummary) => void;
  result: ApiResult<DiagnosisRoomListResponse>;
  retryingNotificationKey: string;
  selectedSessionID: string;
}) {
  const rooms = result.ok ? result.data.items : [];
  const filteredRooms = result.ok
    ? filterDiagnosisRoomsByQueue(rooms, filter)
    : [];
  return (
    <Card
      className="diagnosis-room-panel settings-overview-card"
      extra={
        <Space size={8}>
          {result.ok ? (
            <Typography.Text type="secondary">
              {result.data.items.length} room(s)
              {isFetching ? " / refreshing" : ""}
            </Typography.Text>
          ) : null}
          <Tooltip title="Refresh diagnosis rooms">
            <Button
              aria-label="Refresh diagnosis rooms"
              icon={<ReloadOutlined />}
              loading={isFetching}
              onClick={onRefresh}
              size="small"
            />
          </Tooltip>
        </Space>
      }
      title="Recent Diagnosis Rooms"
    >
      {!result.ok ? (
        <Alert
          description={result.error.message}
          message={
            result.error.status
              ? `HTTP ${result.error.status}`
              : "Request failed"
          }
          showIcon
          type="warning"
        />
      ) : result.data.items.length === 0 ? (
        <Empty
          description="No diagnosis rooms available"
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      ) : (
        <Space className="diagnosis-room-queue" direction="vertical" size={12}>
          {filteredRooms.length === 0 ? (
            <Empty
              description="No diagnosis rooms in this queue"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          ) : (
            <List
              aria-label="Recent diagnosis rooms"
              className="diagnosis-room-list"
              dataSource={filteredRooms}
              renderItem={(room) => {
                const selected = room.session_id === selectedSessionID;
                const latestNotification =
                  latestDiagnosisRoomNotification(room);
                const failedNotification =
                  latestFailedDiagnosisRoomNotification(room);
                const proofRetryNotification =
                  failedNotification === null
                    ? diagnosisNotificationContentProofRetryEntry(
                        room.notification_timeline ?? [],
                      )
                    : null;
                const notificationDeliveryCoverage =
                  diagnosisNotificationDeliveryCoverage(
                    room.notification_timeline ?? [],
                  );
                const showNotificationTimelineReview =
                  diagnosisNotificationTimelineReviewActionRequired(
                    room,
                    failedNotification,
                    notificationDeliveryCoverage,
                  );
                const nextStep = diagnosisRoomNextStep(room);
                const workflowUnavailable =
                  diagnosisRoomWorkflowUnavailable(room);
                const closeUnavailableInFlight =
                  closingSessionID === room.session_id;
                const selectDisabledReason = diagnosisRoomSelectDisabledReason(
                  room,
                  clientReady,
                  selected,
                );
                const selectActionLabel = diagnosisRoomSelectActionLabel(
                  room,
                  selected,
                );
                const actions: ReactNode[] = [
                  <TooltipAction
                    disabled={selectDisabledReason !== ""}
                    key="select"
                    title={selectDisabledReason}
                  >
                    <Button
                      aria-label={`${selectActionLabel} ${room.session_id}`}
                      disabled={selectDisabledReason !== ""}
                      onClick={() => onSelectRoom(room)}
                      size="small"
                      type="link"
                    >
                      {selectActionLabel}
                    </Button>
                  </TooltipAction>,
                ];
                if (failedNotification !== null) {
                  const retryKey = notificationTimelineRetryKey(
                    failedNotification,
                    room.session_id,
                  );
                  const retryDisabledReason = onAdministerRoomBlocked(
                    room.session_id,
                  );
                  actions.push(
                    <Tooltip
                      key="notification-channel"
                      title="Review the notification channel before relying on downstream handoff."
                    >
                      <Button
                        aria-label={`Review notification channel for ${room.session_id}`}
                        href={notificationChannelReviewHref(failedNotification)}
                        size="small"
                        type="link"
                      >
                        Review channel
                      </Button>
                    </Tooltip>,
                    <Tooltip
                      key="notification-retry"
                      title={
                        retryDisabledReason ||
                        "Retry the failed notification through the configured notification channel."
                      }
                    >
                      <Button
                        aria-label={`Retry notification for ${room.session_id}`}
                        disabled={!clientReady || retryDisabledReason !== ""}
                        icon={<ReloadOutlined />}
                        loading={retryingNotificationKey === retryKey}
                        onClick={() =>
                          onRetryNotification(room, failedNotification)
                        }
                        size="small"
                        type="link"
                      >
                        Retry
                      </Button>
                    </Tooltip>,
                  );
                }
                if (proofRetryNotification !== null) {
                  const retryKey = notificationTimelineRetryKey(
                    proofRetryNotification,
                    room.session_id,
                  );
                  const retryDisabledReason = onAdministerRoomBlocked(
                    room.session_id,
                  );
                  actions.push(
                    <Tooltip
                      key="notification-proof-retry"
                      title={
                        retryDisabledReason ||
                        "Re-send the AI notification so the retained timeline includes output digest proof."
                      }
                    >
                      <Button
                        aria-label={`Retry notification proof for ${room.session_id}`}
                        disabled={!clientReady || retryDisabledReason !== ""}
                        icon={<ReloadOutlined />}
                        loading={retryingNotificationKey === retryKey}
                        onClick={() =>
                          onRetryNotification(room, proofRetryNotification)
                        }
                        size="small"
                        type="link"
                      >
                        Retry proof
                      </Button>
                    </Tooltip>,
                  );
                }
                if (showNotificationTimelineReview) {
                  actions.push(
                    <Tooltip
                      key="notification-proof"
                      title="Review retained AI output proof before relying on downstream handoff."
                    >
                      <Link
                        aria-label={`Review notification proof for ${room.session_id}`}
                        href={
                          diagnosisRoomAnchorHref({
                            anchorID: diagnosisNotificationTimelineAnchorID,
                            evidenceSnapshotID: room.evidence_snapshot_id,
                            sessionID: room.session_id,
                          }) as Route
                        }
                      >
                        Review proof
                      </Link>
                    </Tooltip>,
                  );
                }
                if (workflowUnavailable) {
                  const rebuildDisabledReason = onAdministerRoomBlocked(
                    room.session_id,
                  );
                  actions.push(
                    <Tooltip
                      key="rebuild"
                      title={
                        rebuildDisabledReason ||
                        "Prepare the create form with this evidence snapshot."
                      }
                    >
                      <Button
                        aria-label={`Prepare rebuild ${room.session_id}`}
                        disabled={
                          closeUnavailableInFlight ||
                          rebuildDisabledReason !== ""
                        }
                        icon={<PlusCircleOutlined />}
                        loading={closeUnavailableInFlight}
                        onClick={() => onPrepareRoomRebuild(room)}
                        size="small"
                        type="link"
                      >
                        Close/Rebuild
                      </Button>
                    </Tooltip>,
                  );
                }
                return (
                  <List.Item
                    actions={actions}
                    className={`diagnosis-room-list-item${selected ? " diagnosis-room-list-item-selected" : ""}`}
                    key={room.chat_session_id}
                  >
                    <List.Item.Meta
                      description={
                        <Space
                          className="diagnosis-room-list-meta"
                          size={[6, 6]}
                          wrap
                        >
                          <span>task #{room.diagnosis_task_id}</span>
                          <span>evidence #{room.evidence_snapshot_id}</span>
                          <span>updated {formatDateTime(room.updated_at)}</span>
                          <span>{nextStep.detail}</span>
                          {latestNotification ? (
                            <span>
                              notified{" "}
                              {formatDateTime(latestNotification.occurred_at)}
                            </span>
                          ) : null}
                        </Space>
                      }
                      title={
                        <Space size={[6, 6]} wrap>
                          <Typography.Text copyable>
                            {room.session_id}
                          </Typography.Text>
                          {selected ? (
                            <Tag color="processing">selected</Tag>
                          ) : null}
                          <Tag color={nextStep.color}>{nextStep.label}</Tag>
                          <Tag color={roomStatusColor(room.room_status)}>
                            {room.room_status}
                          </Tag>
                          <Tag color={taskStatusColor(room.task_status)}>
                            {room.task_status}
                          </Tag>
                          {room.workflow_visibility ? (
                            <Tag
                              color={workflowVisibilityStatusColor(
                                room.workflow_visibility.status,
                              )}
                            >
                              workflow {room.workflow_visibility.status}
                            </Tag>
                          ) : null}
                          {latestNotification ? (
                            <>
                              <Tag
                                color={notificationStatusColor(
                                  latestNotification.provider_status,
                                )}
                              >
                                {notificationEventLabel(
                                  latestNotification.event_kind,
                                )}
                              </Tag>
                              <Tag
                                color={notificationStatusColor(
                                  latestNotification.provider_status,
                                )}
                              >
                                notify {latestNotification.provider_status}
                              </Tag>
                            </>
                          ) : null}
                          {room.latest_conclusion ? (
                            <>
                              <Tag color="success">
                                {room.latest_conclusion.status}
                              </Tag>
                              {room.latest_conclusion.confidence ? (
                                <Tag
                                  color={confidenceColor(
                                    room.latest_conclusion.confidence,
                                  )}
                                >
                                  {room.latest_conclusion.confidence}
                                </Tag>
                              ) : null}
                              {room.latest_conclusion.requires_human_review ? (
                                <Tag color="warning">review</Tag>
                              ) : null}
                            </>
                          ) : null}
                          {!room.latest_conclusion && room.latest_progress ? (
                            <RoomProgressTags progress={room.latest_progress} />
                          ) : null}
                          <Tag>{room.turn_count} turn(s)</Tag>
                        </Space>
                      }
                    />
                  </List.Item>
                );
              }}
              size="small"
            />
          )}
        </Space>
      )}
    </Card>
  );
}

function RoomProgressTags({
  progress,
}: {
  progress: NonNullable<DiagnosisRoomSummary["latest_progress"]>;
}) {
  const missingCount = progress.missing_evidence_requests?.length ?? 0;
  const suggestionCount = progress.evidence_collection_suggestions?.length ?? 0;
  const status = progress.conclusion_status || progress.status;
  return (
    <>
      <Tag color={conclusionStatusColor(status)}>{status}</Tag>
      <Tag color={confidenceColor(progress.confidence)}>
        {progress.confidence}
      </Tag>
      {progress.requires_human_review ? (
        <Tag color="warning">review</Tag>
      ) : null}
      {progress.evidence_request_count > 0 ? (
        <Tag>{progress.evidence_request_count} planned</Tag>
      ) : null}
      {missingCount > 0 ? (
        <Tag color="warning">{missingCount} missing</Tag>
      ) : null}
      {suggestionCount > 0 ? <Tag>{suggestionCount} suggestion(s)</Tag> : null}
    </>
  );
}

function diagnosisRoomSelectDisabledReason(
  room: DiagnosisRoomSummary,
  clientReady: boolean,
  selected: boolean,
): string {
  if (!clientReady) {
    return "Diagnosis room controls are still loading.";
  }
  if (selected) {
    return "This diagnosis room is already selected.";
  }
  if (room.room_status !== "open" && room.room_status !== "closed") {
    return "Diagnosis room cannot be selected from its current state.";
  }
  if (diagnosisRoomWorkflowUnavailable(room)) {
    const status = room.workflow_visibility?.status ?? "unknown";
    return `Workflow is ${status}; inspect or restart it before opening.`;
  }
  return "";
}

function diagnosisRoomSelectActionLabel(
  room: DiagnosisRoomSummary,
  selected: boolean,
): string {
  if (selected) {
    return "Selected";
  }
  return room.room_status === "closed" ? "Review" : "Use";
}

function latestDiagnosisRoomNotification(
  room: DiagnosisRoomSummary,
): DiagnosisRoomNotificationTimelineEntry | null {
  const timeline = room.notification_timeline ?? [];
  return timeline.length > 0 ? (timeline[timeline.length - 1] ?? null) : null;
}

function latestFailedDiagnosisRoomNotification(
  room: DiagnosisRoomSummary,
): DiagnosisRoomNotificationTimelineEntry | null {
  const timeline = room.notification_timeline ?? [];
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const entry = timeline[index];
    if (entry && diagnosisRoomNotificationFailed(entry.provider_status)) {
      return entry;
    }
  }
  return null;
}

function diagnosisRoomNotificationFailed(status: string): boolean {
  switch (status.toLowerCase()) {
    case "failed":
    case "error":
      return true;
    default:
      return false;
  }
}

function selectedDiagnosisRoomSummary(
  result: ApiResult<DiagnosisRoomListResponse> | undefined,
  sessionID: string,
): DiagnosisRoomSummary | undefined {
  if (!result?.ok || sessionID.trim() === "") {
    return undefined;
  }
  return result.data.items.find((room) => room.session_id === sessionID);
}

function DiagnosisNotificationTimelineSection({
  administerDisabledReason,
  onRetryNotification,
  retryingNotificationKey,
  room,
}: {
  administerDisabledReason?: string;
  onRetryNotification?: (entry: DiagnosisRoomNotificationTimelineEntry) => void;
  retryingNotificationKey?: string;
  room?: DiagnosisRoomSummary;
}) {
  const entries = room?.notification_timeline ?? [];
  const proofExpected =
    room !== undefined && diagnosisNotificationDeliveryProofExpected(room);
  if (entries.length === 0 && !proofExpected) {
    return null;
  }
  const proofSummary = diagnosisNotificationContentProofSummary(entries);
  const deliveryCoverage = diagnosisNotificationDeliveryCoverage(entries);
  const recoveryHint = diagnosisNotificationDeliveryRecoveryHint(
    deliveryCoverage,
  );
  const showProofSummary = entries.length > 0;

  return (
    <section
      aria-label="Notification timeline"
      className="diagnosis-notification-timeline"
      id={diagnosisNotificationTimelineAnchorID}
    >
      <div className="diagnosis-notification-timeline-header">
        <Typography.Title level={3}>Notification Timeline</Typography.Title>
        <Space size={[6, 6]} wrap>
          <Typography.Text type="secondary">
            {entries.length} delivery event(s)
          </Typography.Text>
          {showProofSummary ? (
            <Tag color={proofSummary.color}>{proofSummary.label}</Tag>
          ) : null}
          <Tag color={deliveryCoverage.color}>{deliveryCoverage.label}</Tag>
        </Space>
        {showProofSummary ? (
          <Typography.Text
            type={
              proofSummary.missingCount > 0 ||
              deliveryCoverage.status === "blocked"
                ? "danger"
                : "secondary"
            }
          >
            {proofSummary.detail}
          </Typography.Text>
        ) : null}
        <Typography.Text
          type={deliveryCoverage.status === "blocked" ? "danger" : "secondary"}
        >
          {deliveryCoverage.detail}
        </Typography.Text>
        <Space size={[6, 6]} wrap>
          {deliveryCoverage.phases.map((phase) => (
            <Tag
              color={diagnosisNotificationDeliveryCoveragePhaseColor(
                phase.status,
              )}
              key={phase.key}
            >
              {phase.label}: {phase.status}
            </Tag>
          ))}
        </Space>
      </div>
      {recoveryHint !== null && deliveryCoverage.status !== "ready" ? (
        <Alert
          description={recoveryHint.detail}
          message={recoveryHint.label}
          showIcon
          type={recoveryHint.color}
        />
      ) : null}
      {entries.length > 0 ? (
        <Timeline
          className="diagnosis-notification-timeline-list"
          items={notificationTimelineItems(entries, {
            administerDisabledReason,
            onRetryNotification,
            retryingNotificationKey,
            sessionID: room?.session_id,
          })}
        />
      ) : (
        <Empty
          description="No notification delivery events have been retained for this closed, operator-confirmed diagnosis room. There is no retained notification event to retry yet; check the close notification channel wiring, then refresh the room state."
          image={Empty.PRESENTED_IMAGE_SIMPLE}
        />
      )}
    </section>
  );
}

function notificationTimelineItems(
  entries: DiagnosisRoomNotificationTimelineEntry[],
  options: {
    administerDisabledReason?: string;
    onRetryNotification?: (
      entry: DiagnosisRoomNotificationTimelineEntry,
    ) => void;
    retryingNotificationKey?: string;
    sessionID?: string;
  } = {},
): TimelineProps["items"] {
  return entries.map((entry, index) => ({
    key: notificationTimelineEntryKey(entry, index),
    color: notificationTimelineColor(entry.provider_status),
    children: notificationTimelineItem(entry, options),
  }));
}

function notificationTimelineItem(
  entry: DiagnosisRoomNotificationTimelineEntry,
  options: {
    administerDisabledReason?: string;
    onRetryNotification?: (
      entry: DiagnosisRoomNotificationTimelineEntry,
    ) => void;
    retryingNotificationKey?: string;
    sessionID?: string;
  },
) {
  const proof = diagnosisNotificationContentProofDisplay(entry);
  const retryAction = notificationTimelineRetryAction(entry);
  return (
    <div className="diagnosis-notification-timeline-entry">
      <Space
        className="diagnosis-notification-timeline-heading"
        size={[6, 6]}
        wrap
      >
        <Typography.Text strong>
          {notificationEventLabel(entry.event_kind)}
        </Typography.Text>
        <Tag color={notificationStatusColor(entry.provider_status)}>
          {entry.provider_status}
        </Tag>
        <Tag color={proof.color}>{proof.label}</Tag>
        {entry.confidence ? (
          <Tag color={confidenceColor(entry.confidence)}>
            {entry.confidence}
          </Tag>
        ) : null}
        {entry.requires_human_review ? <Tag color="warning">review</Tag> : null}
        {retryAction && options.onRetryNotification ? (
          <Tooltip
            title={options.administerDisabledReason || retryAction.title}
          >
            <Button
              aria-label={retryAction.ariaLabel}
              disabled={Boolean(options.administerDisabledReason)}
              icon={<ReloadOutlined />}
              loading={
                options.retryingNotificationKey ===
                notificationTimelineRetryKey(entry, options.sessionID)
              }
              onClick={() => options.onRetryNotification?.(entry)}
              size="small"
              type="link"
            >
              {retryAction.label}
            </Button>
          </Tooltip>
        ) : null}
      </Space>
      <Typography.Text type="secondary">
        {formatDateTime(entry.occurred_at)}
      </Typography.Text>
      <Typography.Text type="secondary">
        {notificationTimelineDetails(entry)}
      </Typography.Text>
      <Typography.Text type="secondary">{proof.detail}</Typography.Text>
    </div>
  );
}

function notificationTimelineRetryAction(
  entry: DiagnosisRoomNotificationTimelineEntry,
): { ariaLabel: string; label: string; title: string } | null {
  if (!isDiagnosisNotificationRetryEventKind(entry.event_kind)) {
    return null;
  }
  if (diagnosisRoomNotificationFailed(entry.provider_status)) {
    return {
      ariaLabel: `Retry ${notificationEventLabel(entry.event_kind)}`,
      label: "Retry",
      title:
        "Retry the failed notification through the configured notification channel.",
    };
  }
  if (diagnosisNotificationContentProofRetryRequired(entry)) {
    return {
      ariaLabel: `Retry AI content proof for ${notificationEventLabel(entry.event_kind)}`,
      label: "Retry proof",
      title:
        "Re-send the AI notification so the retained timeline includes output digest proof.",
    };
  }
  return null;
}

function notificationTimelineRetryKey(
  entry: DiagnosisRoomNotificationTimelineEntry,
  sessionID?: string,
): string {
  return `${sessionID?.trim() ?? ""}:${entry.event_kind}:${entry.occurred_at}`;
}

function notificationTimelineEntryKey(
  entry: DiagnosisRoomNotificationTimelineEntry,
  index: number,
): string {
  return `${entry.event_kind}-${entry.occurred_at}-${index}`;
}

function notificationTimelineDetails(
  entry: DiagnosisRoomNotificationTimelineEntry,
): string {
  const details = [
    entry.notification_channel_profile_id !== undefined
      ? `channel #${entry.notification_channel_profile_id}`
      : "",
    entry.turn_count !== undefined ? `turn ${entry.turn_count}` : "",
    entry.assistant_sequence !== undefined
      ? `assistant sequence ${entry.assistant_sequence}`
      : "",
    entry.provider_message_id
      ? `provider message ${entry.provider_message_id}`
      : "",
  ].filter(Boolean);
  return details.length > 0 ? details.join(" / ") : "No delivery metadata";
}

function notificationEventLabel(eventKind: string): string {
  switch (eventKind) {
    case "diagnosis_room.assistant_turn_notification_sent":
      return "AI update notification";
    case "diagnosis_room.final_ready_notification_sent":
      return "Final-ready notification";
    case "diagnosis_room.close_notification_sent":
      return "Close notification";
    default:
      return eventKind;
  }
}

function notificationTimelineColor(status: string): string {
  switch (notificationStatusColor(status)) {
    case "success":
      return "green";
    case "error":
      return "red";
    case "warning":
      return "blue";
    default:
      return "gray";
  }
}

function reviewQueueActions(
  item: DiagnosisReviewQueueItem,
  connected: boolean,
  canConfirmConclusion: boolean,
  onUseFollowUp: (item: DiagnosisConsultationEvidenceRequest) => void,
  onUseEvidencePlan: (item: DiagnosisEvidenceRequest) => void,
  onConfirmConclusion: () => void,
  actionDisabledReason: string,
) {
  const actionDisabled = !connected || actionDisabledReason !== "";
  const canPrepareConnectionGatedAction =
    diagnosisReviewQueueConnectionGateAllowsPreparation({
      actionDisabledReason,
      connected,
    });
  if (item.kind === "supplemental_evidence") {
    const disabled = actionDisabled && !canPrepareConnectionGatedAction;
    return [
      <TooltipAction
        disabled={disabled}
        key="use-follow-up"
        title={
          canPrepareConnectionGatedAction
            ? "Prepare follow-up now; connect before submitting evidence."
            : actionDisabledReason ||
              `Prepare follow-up for ${item.request.label}.`
        }
      >
        <Button
          aria-label={`Use follow-up for ${item.request.label}`}
          disabled={disabled}
          icon={<FormOutlined />}
          onClick={() => onUseFollowUp(item.request)}
          size="small"
          type="link"
        >
          Use follow-up
        </Button>
      </TooltipAction>,
    ];
  }
  if (item.kind === "supplemental_evidence_record" && item.unresolvedRequest) {
    const unresolvedRequest = item.unresolvedRequest;
    const disabled = actionDisabled && !canPrepareConnectionGatedAction;
    return [
      <TooltipAction
        disabled={disabled}
        key="use-latest-request"
        title={
          canPrepareConnectionGatedAction
            ? "Prepare latest request now; connect before submitting evidence."
            : actionDisabledReason ||
              `Prepare latest request for ${unresolvedRequest.label}.`
        }
      >
        <Button
          aria-label={`Use latest request for ${unresolvedRequest.label}`}
          disabled={disabled}
          icon={<FormOutlined />}
          onClick={() => onUseFollowUp(unresolvedRequest)}
          size="small"
          type="link"
        >
          Use latest request
        </Button>
      </TooltipAction>,
    ];
  }
  if (item.kind === "collection_result" && item.retryable) {
    const disabled = actionDisabled && !canPrepareConnectionGatedAction;
    return [
      <TooltipAction
        disabled={disabled}
        key="retry-collection"
        title={
          canPrepareConnectionGatedAction
            ? "Stage retry now; connect before collecting evidence."
            : actionDisabledReason ||
              `Retry ${item.result.tool} evidence collection.`
        }
      >
        <Button
          aria-label={`Retry collection for ${item.result.tool}`}
          disabled={disabled}
          icon={<ReloadOutlined />}
          onClick={() =>
            onUseEvidencePlan(collectionResultRequest(item.result))
          }
          size="small"
          type="link"
        >
          Retry collection
        </Button>
      </TooltipAction>,
    ];
  }
  if (item.kind === "collection_result" && item.recoveryRequest) {
    const recoveryRequest = item.recoveryRequest;
    const disabled = actionDisabled && !canPrepareConnectionGatedAction;
    return [
      <TooltipAction
        disabled={disabled}
        key="add-recovery-evidence"
        title={
          canPrepareConnectionGatedAction
            ? "Prepare recovery evidence now; connect before submitting evidence."
            : actionDisabledReason ||
              `Add recovery evidence for ${item.result.tool}.`
        }
      >
        <Button
          aria-label={`Add recovery evidence for ${item.result.tool}`}
          disabled={disabled}
          icon={<FormOutlined />}
          onClick={() => onUseFollowUp(recoveryRequest)}
          size="small"
          type="link"
        >
          Add evidence
        </Button>
      </TooltipAction>,
    ];
  }
  if (item.kind === "executable_evidence") {
    const disabled = actionDisabled && !canPrepareConnectionGatedAction;
    return [
      <TooltipAction
        disabled={disabled}
        key="use-evidence-plan"
        title={
          canPrepareConnectionGatedAction
            ? "Stage this plan now; connect before collecting evidence."
            : actionDisabledReason || `Collect ${item.request.tool} evidence.`
        }
      >
        <Button
          aria-label={`Use collection plan for ${item.request.tool}`}
          disabled={disabled}
          icon={<FormOutlined />}
          onClick={() => onUseEvidencePlan(item.request)}
          size="small"
          type="link"
        >
          Use plan
        </Button>
      </TooltipAction>,
    ];
  }
  if (item.kind === "confirm") {
    return [
      <TooltipAction
        disabled={!canConfirmConclusion}
        key="confirm"
        title={
          canConfirmConclusion
            ? "Confirm and retain the AI conclusion."
            : "Resolve review blockers before confirming the conclusion."
        }
      >
        <Button
          disabled={!canConfirmConclusion}
          icon={<CheckCircleOutlined />}
          onClick={onConfirmConclusion}
          size="small"
          type="link"
        >
          Confirm
        </Button>
      </TooltipAction>,
    ];
  }
  return undefined;
}

function reviewQueueStatusColor(
  status: DiagnosisReviewQueueItem["status"],
): string {
  switch (status) {
    case "attention":
      return "error";
    case "done":
      return "success";
    case "ready":
      return "success";
    case "pending":
      return "processing";
  }
}

function reviewQueueTaskProgressTimelineItems({
  actionDisabledReason,
  canConfirmConclusion,
  connected,
  items,
  onConfirmConclusion,
  onOpenConnection,
  onRequestReassessment,
  onUseEvidencePlan,
  onUseFollowUp,
  taskProgress,
}: {
  actionDisabledReason: string;
  canConfirmConclusion: boolean;
  connected: boolean;
  items: DiagnosisReviewQueueItem[];
  onConfirmConclusion: () => void;
  onOpenConnection?: () => void;
  onRequestReassessment: () => void;
  onUseEvidencePlan: (item: DiagnosisEvidenceRequest) => void;
  onUseFollowUp: (item: DiagnosisConsultationEvidenceRequest) => void;
  taskProgress: ReturnType<typeof diagnosisReviewQueueTaskProgress>;
}): TimelineProps["items"] {
  const itemByKey = new Map(items.map((item) => [item.key, item]));
  return taskProgress.phases.map((phase) => ({
    key: phase.key,
    color: reviewQueueTaskPhaseTimelineColor(phase.status),
    children: (
      <TimelineStep
        action={reviewQueueTaskPhaseActionButton({
          action: phase.action,
          actionDisabledReason,
          canConfirmConclusion,
          connected,
          itemByKey,
          onConfirmConclusion,
          onOpenConnection,
          onRequestReassessment,
          onUseEvidencePlan,
          onUseFollowUp,
        })}
        detail={phase.detail}
        tags={[
          {
            color: reviewQueueTaskPhaseTagColor(phase.status),
            label: phase.statusLabel,
          },
        ]}
        title={phase.label}
      />
    ),
  }));
}

function reviewQueueTaskPhaseActionButton({
  action,
  actionDisabledReason,
  canConfirmConclusion,
  connected,
  itemByKey,
  onConfirmConclusion,
  onOpenConnection,
  onRequestReassessment,
  onUseEvidencePlan,
  onUseFollowUp,
}: {
  action?: DiagnosisReviewQueueTaskPhaseAction;
  actionDisabledReason: string;
  canConfirmConclusion: boolean;
  connected: boolean;
  itemByKey: Map<string, DiagnosisReviewQueueItem>;
  onConfirmConclusion: () => void;
  onOpenConnection?: () => void;
  onRequestReassessment: () => void;
  onUseEvidencePlan: (item: DiagnosisEvidenceRequest) => void;
  onUseFollowUp: (item: DiagnosisConsultationEvidenceRequest) => void;
}): ReactNode {
  if (!action) {
    return null;
  }
  const actionDisabled = !connected || actionDisabledReason !== "";
  const canPrepareConnectionGatedAction =
    diagnosisReviewQueueConnectionGateAllowsPreparation({
      actionDisabledReason,
      connected,
    });
  if (action.kind === "request_reassessment") {
    const canOpenConnection =
      actionDisabled &&
      canPrepareConnectionGatedAction &&
      onOpenConnection !== undefined;
    const disabled = actionDisabled && !canOpenConnection;
    return (
      <TooltipAction
        disabled={disabled}
        title={
          canOpenConnection
            ? "Open the live room connection, then return to ask AI for reassessment."
            : actionDisabledReason ||
              "Ask AI to reassess retained evidence and update confidence."
        }
      >
        <Button
          disabled={disabled}
          icon={canOpenConnection ? <ApiOutlined /> : <ReloadOutlined />}
          onClick={canOpenConnection ? onOpenConnection : onRequestReassessment}
          size="small"
          type="primary"
        >
          {canOpenConnection ? "Open connection" : action.label}
        </Button>
      </TooltipAction>
    );
  }
  if (action.itemKey === undefined) {
    return null;
  }
  const item = itemByKey.get(action.itemKey);
  if (!item) {
    return null;
  }
  if (action.kind === "confirm") {
    return (
      <TooltipAction
        disabled={!canConfirmConclusion}
        title={
          canConfirmConclusion
            ? "Confirm and retain the AI conclusion."
            : "Resolve review blockers before confirming the conclusion."
        }
      >
        <Button
          disabled={!canConfirmConclusion}
          icon={<CheckCircleOutlined />}
          onClick={onConfirmConclusion}
          size="small"
          type="primary"
        >
          {action.label}
        </Button>
      </TooltipAction>
    );
  }
  if (action.kind === "use_evidence_plan") {
    const request =
      item.kind === "executable_evidence"
        ? item.request
        : item.kind === "collection_result"
          ? collectionResultRequest(item.result)
          : null;
    if (!request) {
      return null;
    }
    const disabled = actionDisabled && !canPrepareConnectionGatedAction;
    return (
      <TooltipAction
        disabled={disabled}
        title={
          canPrepareConnectionGatedAction
            ? "Stage this plan now; connect before collecting evidence."
            : actionDisabledReason || `Collect ${request.tool} evidence.`
        }
      >
        <Button
          disabled={disabled}
          icon={
            action.label === "Retry collection" ? (
              <ReloadOutlined />
            ) : (
              <FormOutlined />
            )
          }
          onClick={() => onUseEvidencePlan(request)}
          size="small"
          type="primary"
        >
          {action.label}
        </Button>
      </TooltipAction>
    );
  }
  const request =
    item.kind === "supplemental_evidence"
      ? item.request
      : item.kind === "supplemental_evidence_record"
        ? item.unresolvedRequest
        : item.kind === "collection_result"
          ? item.recoveryRequest
          : undefined;
  if (!request) {
    return null;
  }
  const disabled = actionDisabled && !canPrepareConnectionGatedAction;
  return (
    <TooltipAction
      disabled={disabled}
      title={
        canPrepareConnectionGatedAction
          ? "Prepare follow-up now; connect before submitting evidence."
          : actionDisabledReason || `Prepare follow-up for ${request.label}.`
      }
    >
      <Button
        disabled={disabled}
        icon={<FormOutlined />}
        onClick={() => onUseFollowUp(request)}
        size="small"
        type="primary"
      >
        {action.label}
      </Button>
    </TooltipAction>
  );
}

function reviewQueueTaskProgressBarStatus(
  status: ReturnType<typeof diagnosisReviewQueueTaskProgress>["status"],
): "active" | "exception" | "normal" | "success" {
  switch (status) {
    case "blocked":
      return "exception";
    case "done":
      return "success";
    case "pending":
    case "ready":
      return "active";
  }
}

function reviewQueueTaskProgressTagColor(
  status: ReturnType<typeof diagnosisReviewQueueTaskProgress>["status"],
): string {
  switch (status) {
    case "blocked":
      return "error";
    case "done":
      return "success";
    case "pending":
      return "processing";
    case "ready":
      return "success";
  }
}

function reviewQueueTaskPhaseTimelineColor(
  status: ReturnType<
    typeof diagnosisReviewQueueTaskProgress
  >["phases"][number]["status"],
): string {
  switch (status) {
    case "attention":
      return "red";
    case "done":
      return "green";
    case "pending":
      return "blue";
    case "ready":
      return "green";
  }
}

function reviewQueueTaskPhaseTagColor(
  status: ReturnType<
    typeof diagnosisReviewQueueTaskProgress
  >["phases"][number]["status"],
): string {
  switch (status) {
    case "attention":
      return "error";
    case "done":
      return "success";
    case "pending":
      return "processing";
    case "ready":
      return "success";
  }
}

function reviewQueueActionPlanAlertType(
  status: ReturnType<typeof diagnosisReviewQueueActionPlan>["status"],
) {
  switch (status) {
    case "blocked":
      return "warning";
    case "ready":
      return "success";
    case "pending":
      return "info";
    case "done":
      return "info";
  }
}

function reviewQueuePostEvidenceAlertType(
  status: ReturnType<typeof diagnosisReviewQueuePostEvidenceStatus>["status"],
) {
  switch (status) {
    case "blocked":
      return "warning";
    case "ready":
      return "success";
    case "submitted":
      return "info";
    case "none":
      return "info";
  }
}

function ConsultationProgressPanel({
  finalConclusion,
  latestInsight,
  notificationDeliveryCoverage,
  supplementalEvidence,
}: {
  finalConclusion?: DiagnosisFinalConclusion;
  latestInsight: LatestConsultationInsight;
  notificationDeliveryCoverage?: ReturnType<
    typeof diagnosisNotificationDeliveryCoverage
  >;
  supplementalEvidence: DiagnosisSupplementalEvidenceRecord[];
}) {
  const confidence = confidencePercent(latestInsight.confidence);
  const collectedCount = latestInsight.collectionResults.filter(
    (item) => item.status === "collected",
  ).length;
  const missingCount =
    latestInsight.insight.missing_evidence_requests?.length ?? 0;
  const suggestionCount =
    latestInsight.insight.evidence_collection_suggestions?.length ?? 0;
  const confidenceProgress = diagnosisFinalConclusionConfidenceProgress(
    {
      confidence: latestInsight.confidence,
      confidence_rationale: latestInsight.insight.confidence_rationale,
      status: "available",
    },
    latestInsight.confidenceTimeline,
  );
  const showConfidenceProgress =
    confidenceProgress.status !== "unknown" ||
    latestInsight.confidenceTimeline.length > 1;

  return (
    <section
      aria-label="Diagnosis consultation progress"
      className="diagnosis-progress"
    >
      <div className="diagnosis-progress-summary">
        <div className="diagnosis-progress-confidence">
          <div className="diagnosis-progress-heading">
            <Typography.Text strong>Confidence</Typography.Text>
            <Tag color={confidenceColor(latestInsight.confidence)}>
              {latestInsight.confidence || "unknown"}
            </Tag>
          </div>
          <Progress
            aria-label="Diagnosis confidence"
            aria-valuetext={`${latestInsight.confidence || "unknown"} confidence`}
            percent={confidence}
            size="small"
            status={confidenceProgressStatus(latestInsight)}
          />
        </div>
        <div
          aria-label="Evidence readiness"
          className="diagnosis-progress-metrics"
        >
          <ProgressMetric
            label="Plan"
            value={latestInsight.evidenceRequests.length}
          />
          <ProgressMetric label="Collected" value={collectedCount} />
          <ProgressMetric label="Missing" value={missingCount} />
          <ProgressMetric label="Suggestions" value={suggestionCount} />
          <ProgressMetric
            label="Next"
            value={nextDiagnosisAction(latestInsight, supplementalEvidence)}
            wide
          />
        </div>
      </div>
      {showConfidenceProgress ? (
        <Alert
          className="diagnosis-progress-confidence-change"
          description={confidenceProgress.detail}
          message={confidenceProgress.label}
          showIcon
          type={confidenceProgressAlertType(confidenceProgress.status)}
        />
      ) : null}
      <Timeline
        className="diagnosis-progress-timeline"
        items={consultationTimelineItems(
          latestInsight,
          supplementalEvidence,
          finalConclusion,
          notificationDeliveryCoverage,
        )}
      />
    </section>
  );
}

function ConfidenceTimelineSection({
  items,
}: {
  items: DiagnosisConfidenceTimelineEntry[];
}) {
  if (items.length === 0) {
    return null;
  }
  return (
    <section
      aria-label="Confidence timeline"
      className="diagnosis-confidence-timeline"
    >
      <div className="diagnosis-confidence-timeline-header">
        <Typography.Title level={3}>Confidence Timeline</Typography.Title>
        <Typography.Text type="secondary">
          {items.length} checkpoint(s)
        </Typography.Text>
      </div>
      <Timeline
        className="diagnosis-confidence-timeline-list"
        items={confidenceTimelineItems(items)}
      />
    </section>
  );
}

function confidenceTimelineItems(
  items: DiagnosisConfidenceTimelineEntry[],
): TimelineProps["items"] {
  return items.map((item, index) => ({
    key: confidenceTimelineKey(item, index),
    color: confidenceTimelineColor(item),
    children: <ConfidenceTimelineCheckpoint item={item} />,
  }));
}

function ConfidenceTimelineCheckpoint({
  item,
}: {
  item: DiagnosisConfidenceTimelineEntry;
}) {
  const evidenceRequestCount = item.evidence_requests?.length ?? 0;
  const collectionResultCount = item.evidence_collection_results?.length ?? 0;
  const missingCount = item.missing_evidence_requests?.length ?? 0;
  const suggestionCount = item.evidence_collection_suggestions?.length ?? 0;

  return (
    <div className="diagnosis-confidence-checkpoint">
      <Space size={[6, 6]} wrap>
        <Typography.Text strong>
          Turn {item.turn_count} / {item.confidence || "unknown"} confidence
        </Typography.Text>
        {item.conclusion_status ? (
          <Tag color={conclusionStatusColor(item.conclusion_status)}>
            {item.conclusion_status}
          </Tag>
        ) : null}
        <Tag color={item.requires_human_review ? "warning" : "success"}>
          {item.requires_human_review ? "review required" : "review optional"}
        </Tag>
        {item.trigger ? <Tag>{item.trigger}</Tag> : null}
      </Space>
      {item.confidence_rationale ? (
        <Typography.Text>{item.confidence_rationale}</Typography.Text>
      ) : null}
      <Space
        className="diagnosis-confidence-checkpoint-meta"
        size={[6, 6]}
        wrap
      >
        <Tag>{evidenceRequestCount} planned</Tag>
        <Tag color={collectionResultCount > 0 ? "success" : "default"}>
          {collectionResultCount} collected
        </Tag>
        <Tag color={missingCount > 0 ? "warning" : "default"}>
          {missingCount} missing
        </Tag>
        <Tag>{suggestionCount} suggestion(s)</Tag>
        <Typography.Text type="secondary">
          {formatDateTime(item.occurred_at)}
        </Typography.Text>
      </Space>
    </div>
  );
}

function ProgressMetric({
  label,
  value,
  wide,
}: {
  label: string;
  value: number | string;
  wide?: boolean;
}) {
  return (
    <div
      className={
        wide
          ? "diagnosis-progress-metric diagnosis-progress-metric-wide"
          : "diagnosis-progress-metric"
      }
    >
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function consultationTimelineItems(
  latestInsight: LatestConsultationInsight,
  supplementalEvidence: DiagnosisSupplementalEvidenceRecord[],
  finalConclusion?: DiagnosisFinalConclusion,
  notificationDeliveryCoverage?: ReturnType<
    typeof diagnosisNotificationDeliveryCoverage
  >,
): TimelineProps["items"] {
  const conclusionStatus = latestInsight.insight.conclusion_status || "unknown";
  const items: NonNullable<TimelineProps["items"]> = [
    {
      key: "draft",
      color: conclusionStatus === "needs_evidence" ? "blue" : "green",
      children: (
        <TimelineStep
          detail={`Turn ${latestInsight.turnCount} produced a ${latestInsight.confidence || "unknown"} confidence diagnosis.`}
          title="AI drafted diagnosis"
        />
      ),
    },
  ];

  const supplementalRequests = [
    ...(latestInsight.insight.missing_evidence_requests ?? []),
    ...(latestInsight.insight.evidence_collection_suggestions ?? []),
  ];
  if (
    latestInsight.evidenceRequests.length > 0 ||
    supplementalRequests.length > 0
  ) {
    items.push({
      key: "supplemental-evidence",
      color: "blue",
      children: (
        <TimelineStep
          detail={formatSupplementalEvidenceSummary(
            latestInsight.evidenceRequests.length,
            supplementalRequests.length,
          )}
          tags={supplementalRequests.slice(0, 3).map((request) => ({
            color: priorityColor(request.priority),
            label: request.label,
          }))}
          title="Supplemental evidence requested"
        />
      ),
    });
  }

  if (latestInsight.collectionResults.length > 0) {
    const reassessmentStatus = diagnosisConsultationReassessmentStatus({
      autoFollowUpCount: latestInsight.autoFollowUpCount,
      collectionResults: latestInsight.collectionResults,
      confidenceTimeline: latestInsight.confidenceTimeline,
    });
    items.push({
      key: "collection",
      color: latestInsight.collectionResults.some(
        (item) => item.status === "failed",
      )
        ? "red"
        : "green",
      children: (
        <TimelineStep
          detail={formatCollectionProgressSummary(
            latestInsight.collectionResults,
          )}
          tags={latestInsight.collectionResults.slice(0, 3).map((item) => ({
            color: collectionStatusColor(item.status),
            label: item.tool,
          }))}
          title="Executable evidence collected"
        />
      ),
    });
    if (reassessmentStatus.status !== "not_needed") {
      items.push({
        key: "ai-reassessment",
        color: reassessmentStatus.status === "reviewed" ? "green" : "blue",
        children: (
          <TimelineStep
            detail={reassessmentStatus.detail}
            tags={[
              ...(reassessmentStatus.turnCount === undefined
                ? []
                : [
                    {
                      color: "processing",
                      label: `turn ${reassessmentStatus.turnCount}`,
                    },
                  ]),
              ...(reassessmentStatus.confidence === undefined
                ? []
                : [
                    {
                      color: confidenceColor(reassessmentStatus.confidence),
                      label: `${reassessmentStatus.confidence} confidence`,
                    },
                  ]),
              ...(reassessmentStatus.conclusionStatus === undefined
                ? []
                : [
                    {
                      color: conclusionStatusColor(
                        reassessmentStatus.conclusionStatus,
                      ),
                      label: reassessmentStatus.conclusionStatus,
                    },
                  ]),
            ]}
            title={reassessmentStatus.label}
          />
        ),
      });
    }
  }

  const conclusionLifecycle = diagnosisConsultationConclusionLifecycleStatus({
    conclusionStatus: latestInsight.insight.conclusion_status,
    finalConclusion,
    notificationDelivery: notificationDeliveryCoverage,
  });
  if (conclusionLifecycle.status !== "not_ready") {
    items.push({
      key: "conclusion-lifecycle",
      color: consultationConclusionLifecycleColor(conclusionLifecycle.status),
      children: (
        <TimelineStep
          detail={conclusionLifecycle.detail}
          tags={consultationConclusionLifecycleTags(conclusionLifecycle)}
          title={conclusionLifecycle.label}
        />
      ),
    });
  }

  items.push({
    key: "next-action",
    color:
      conclusionStatus === "final" || conclusionStatus === "ready_for_review"
        ? "green"
        : "gray",
    children: (
      <TimelineStep
        detail={`Conclusion status is ${conclusionStatus}.`}
        tags={[
          {
            color: conclusionStatusColor(conclusionStatus),
            label: conclusionStatus,
          },
        ]}
        title={nextDiagnosisAction(latestInsight, supplementalEvidence)}
      />
    ),
  });

  return items;
}

function TimelineStep({
  action,
  detail,
  tags,
  title,
}: {
  action?: ReactNode;
  detail: string;
  tags?: Array<{ color: string; label: string }>;
  title: string;
}) {
  return (
    <div className="diagnosis-progress-step">
      <Typography.Text strong>{title}</Typography.Text>
      <Typography.Text type="secondary">{detail}</Typography.Text>
      {tags && tags.length > 0 ? (
        <Space size={[6, 6]} wrap>
          {tags.map((tag, index) => (
            <Tag color={tag.color} key={`${tag.label}-${index}`}>
              {tag.label}
            </Tag>
          ))}
        </Space>
      ) : null}
      {action ? (
        <div className="diagnosis-progress-step-action">{action}</div>
      ) : null}
    </div>
  );
}

function EvidencePlanList({
  emptyDescription,
  items,
  title,
}: {
  emptyDescription: string;
  items?: DiagnosisEvidenceRequest[];
  title: string;
}) {
  return (
    <section aria-label={title} className="diagnosis-insight-section">
      <Typography.Title level={3}>{title}</Typography.Title>
      <List
        className="diagnosis-evidence-list"
        dataSource={items ?? []}
        locale={{ emptyText: emptyDescription }}
        renderItem={(item, index) => (
          <List.Item
            className="diagnosis-evidence-item"
            key={evidenceRequestKey(item, index)}
          >
            <List.Item.Meta
              description={
                <EvidenceRequestMetadata
                  fallbackText="No additional parameters"
                  request={item}
                />
              }
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

function EvidenceCollectionResultList({
  items,
}: {
  items?: DiagnosisEvidenceCollectionResult[];
}) {
  const results = items ?? [];
  const summary = evidenceCollectionSummary(results);

  return (
    <section
      aria-label="Collection Results"
      className="diagnosis-insight-section"
    >
      <Typography.Title level={3}>Collection Results</Typography.Title>
      {summary.total > 0 ? (
        <EvidenceCollectionSummaryBar summary={summary} />
      ) : null}
      <List
        className="diagnosis-evidence-list"
        dataSource={results}
        locale={{ emptyText: "No evidence collected yet" }}
        renderItem={(item, index) => (
          <List.Item
            className="diagnosis-evidence-item"
            key={evidenceCollectionResultKey(item, index)}
          >
            <List.Item.Meta
              description={
                <EvidenceCollectionResultDetails item={item}>
                  {item.active_alerts && item.active_alerts.length > 0 ? (
                    <div className="diagnosis-alert-chips">
                      {item.active_alerts
                        .slice(0, 3)
                        .map((alert, alertIndex) => (
                          <Tag key={activeAlertKey(alert, alertIndex)}>
                            {formatActiveAlert(alert)}
                          </Tag>
                        ))}
                      {item.active_alerts.length > 3 ? (
                        <Tag>+{item.active_alerts.length - 3} more</Tag>
                      ) : null}
                    </div>
                  ) : null}
                  {hasMetricResult(item) ? (
                    <MetricResultSummary item={item} />
                  ) : null}
                </EvidenceCollectionResultDetails>
              }
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.message || item.tool}</span>
                  <Tag color="processing">{item.tool}</Tag>
                  <Tag color={collectionStatusColor(item.status)}>
                    {item.status}
                  </Tag>
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

function EvidenceTimelineSection({
  directoryUsersBySubject,
  items,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  items?: DiagnosisEvidenceTimelineEntry[];
}) {
  const entries = items ?? [];
  if (entries.length === 0) {
    return null;
  }

  const timelineItems: TimelineProps["items"] = entries.map((entry, index) => {
    const requests = entry.evidence_requests ?? [];
    const results = entry.evidence_collection_results ?? [];
    return {
      key: evidenceTimelineEntryKey(entry, index),
      children: (
        <div className="diagnosis-evidence-timeline-entry">
          <Space
            className="diagnosis-evidence-timeline-heading"
            size={[6, 6]}
            wrap
          >
            <Typography.Text strong>
              Turn {entry.turn_count} evidence
            </Typography.Text>
            <ActorSubjectTags
              directoryUsersBySubject={directoryUsersBySubject}
              subject={entry.actor_subject}
            />
            {entry.trigger ? <Tag>{entry.trigger}</Tag> : null}
            <Tag>{requests.length} planned</Tag>
            <Tag color={results.length > 0 ? "success" : "default"}>
              {results.length} collected
            </Tag>
          </Space>
          {requests.length > 0 ? (
            <div className="diagnosis-evidence-timeline-row">
              <Typography.Text type="secondary">Plan</Typography.Text>
              <div className="diagnosis-evidence-timeline-items">
                {requests.map((request, requestIndex) => (
                  <div
                    className="diagnosis-evidence-timeline-chip"
                    key={evidenceRequestKey(request, requestIndex)}
                  >
                    <Space size={[6, 6]} wrap>
                      <Tag color="processing">{request.tool}</Tag>
                      <Typography.Text>
                        {request.reason || "No request reason"}
                      </Typography.Text>
                    </Space>
                    <EvidenceRequestMetadata request={request} />
                  </div>
                ))}
              </div>
            </div>
          ) : null}
          {results.length > 0 ? (
            <div className="diagnosis-evidence-timeline-row">
              <Typography.Text type="secondary">Results</Typography.Text>
              <div className="diagnosis-evidence-timeline-items">
                {results.map((result, resultIndex) => (
                  <div
                    className="diagnosis-evidence-timeline-chip"
                    key={evidenceCollectionResultKey(result, resultIndex)}
                  >
                    <Space size={[6, 6]} wrap>
                      <Tag color="processing">{result.tool}</Tag>
                      <Tag color={collectionStatusColor(result.status)}>
                        {result.status}
                      </Tag>
                      <Tag>{result.reason_code}</Tag>
                    </Space>
                    <EvidenceRequestMetadata
                      request={collectionResultRequest(result)}
                      sourceKind={result.alert_source_kind}
                    />
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      ),
    };
  });

  return (
    <section
      aria-label="Evidence timeline"
      className="diagnosis-evidence-timeline"
    >
      <div className="diagnosis-evidence-timeline-header">
        <Typography.Title level={3}>Evidence Timeline</Typography.Title>
        <Typography.Text type="secondary">
          {entries.length} collection cycle(s)
        </Typography.Text>
      </div>
      <Timeline
        className="diagnosis-evidence-timeline-list"
        items={timelineItems}
      />
    </section>
  );
}

function EvidenceCollectionSummaryBar({
  summary,
}: {
  summary: EvidenceCollectionSummaryStats;
}) {
  return (
    <div
      aria-label="Evidence collection summary"
      className="diagnosis-collection-summary"
    >
      <Tag color={summary.unresolved > 0 ? "warning" : "success"}>
        {summary.collected}/{summary.total} collected
      </Tag>
      {summary.unresolved > 0 ? (
        <Tag color="warning">{summary.unresolved} unresolved</Tag>
      ) : null}
      {summary.failed > 0 ? (
        <Tag color="error">{summary.failed} failed</Tag>
      ) : null}
      {summary.skipped > 0 ? <Tag>{summary.skipped} skipped</Tag> : null}
      {summary.unsupported > 0 ? (
        <Tag color="warning">{summary.unsupported} unsupported</Tag>
      ) : null}
      {summary.observedAlerts > 0 ? (
        <Tag>{summary.observedAlerts} alerts</Tag>
      ) : null}
      {summary.observedMetricSeries > 0 ? (
        <Tag>{summary.observedMetricSeries} series</Tag>
      ) : null}
    </div>
  );
}

function MetricResultSummary({
  item,
}: {
  item: DiagnosisEvidenceCollectionResult;
}) {
  const result = item.metric_result;
  if (!result) {
    return null;
  }
  const series = result.series ?? [];
  return (
    <div className="diagnosis-metric-summary">
      <Space size={[6, 6]} wrap>
        {result.result_type ? (
          <Tag color="processing">{result.result_type}</Tag>
        ) : null}
        {item.observed_metric_series !== undefined ? (
          <Tag>series: {item.observed_metric_series}</Tag>
        ) : null}
        {result.warnings?.map((warning, index) => (
          <Tag color="warning" key={`${warning}-${index}`}>
            {warning}
          </Tag>
        ))}
      </Space>
      {series.length > 0 ? (
        <div className="diagnosis-metric-series">
          {series.slice(0, 3).map((entry, index) => (
            <Tag key={metricSeriesKey(entry, index)}>
              {formatMetricSeries(entry)}
            </Tag>
          ))}
          {series.length > 3 ? <Tag>+{series.length - 3} more</Tag> : null}
        </div>
      ) : null}
      {result.scalar ? (
        <div className="diagnosis-metric-value">
          scalar: {result.scalar.value}
        </div>
      ) : null}
      {result.string ? (
        <div className="diagnosis-metric-value">
          string: {result.string.value}
        </div>
      ) : null}
    </div>
  );
}

function EvidenceCollectionResultDetails({
  children,
  item,
}: {
  children?: ReactNode;
  item: DiagnosisEvidenceCollectionResult;
}) {
  return (
    <div className="diagnosis-evidence-result-details">
      <Space className="diagnosis-evidence-metadata" size={[6, 6]} wrap>
        <Tag>alerts observed: {item.observed_alerts}</Tag>
        <Tag>alerts visible: {item.active_alerts?.length ?? 0}</Tag>
        {item.observed_metric_series !== undefined ? (
          <Tag>series observed: {item.observed_metric_series}</Tag>
        ) : null}
      </Space>
      <EvidenceRequestMetadata
        request={collectionResultRequest(item)}
        sourceKind={item.alert_source_kind}
      />
      {children}
    </div>
  );
}

function EvidenceRequestMetadata({
  fallbackText,
  request,
  sourceKind,
}: {
  fallbackText?: string;
  request: DiagnosisEvidenceRequest;
  sourceKind?: string;
}) {
  const tags: ReactNode[] = [];
  if (request.template_id) {
    tags.push(<Tag key="template">template #{request.template_id}</Tag>);
  }
  if (request.alert_source_profile_id) {
    tags.push(
      <Tag key="profile">profile #{request.alert_source_profile_id}</Tag>,
    );
  }
  if (sourceKind) {
    tags.push(<Tag key="source">source: {sourceKind}</Tag>);
  }
  if (request.limit) {
    tags.push(<Tag key="limit">limit: {request.limit}</Tag>);
  }
  if (request.window_seconds) {
    tags.push(<Tag key="window">window: {request.window_seconds}s</Tag>);
  }
  if (request.step_seconds) {
    tags.push(<Tag key="step">step: {request.step_seconds}s</Tag>);
  }
  if (request.query) {
    tags.push(
      <Typography.Text className="diagnosis-evidence-query" code key="query">
        {request.query}
      </Typography.Text>,
    );
  }
  if (tags.length === 0) {
    return fallbackText ? (
      <Typography.Text type="secondary">{fallbackText}</Typography.Text>
    ) : null;
  }
  return (
    <Space className="diagnosis-evidence-metadata" size={[6, 6]} wrap>
      {tags}
    </Space>
  );
}

function collectionResultRequest(
  item: DiagnosisEvidenceCollectionResult,
): DiagnosisEvidenceRequest {
  return {
    ...item.request,
    alert_source_profile_id:
      item.alert_source_profile_id ?? item.request.alert_source_profile_id,
    limit: item.limit ?? item.request.limit,
    query: item.query ?? item.request.query,
    step_seconds: item.step_seconds ?? item.request.step_seconds,
    template_id: item.template_id ?? item.request.template_id,
    tool: item.tool || item.request.tool,
    window_seconds: item.window_seconds ?? item.request.window_seconds,
  };
}

function EvidenceRequestList({
  emptyDescription,
  followUpDisabled,
  followUpDisabledReason = "",
  items,
  onUseFollowUp,
  title,
}: {
  emptyDescription: string;
  followUpDisabled?: boolean;
  followUpDisabledReason?: string;
  items?: DiagnosisConsultationEvidenceRequest[];
  onUseFollowUp?: (item: DiagnosisConsultationEvidenceRequest) => void;
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
          <List.Item
            actions={
              onUseFollowUp
                ? [
                    <TooltipAction
                      disabled={Boolean(followUpDisabled)}
                      key="use-follow-up"
                      title={
                        followUpDisabledReason ||
                        `Prepare follow-up for ${item.label}.`
                      }
                    >
                      <Button
                        aria-label={`Use follow-up for ${item.label}`}
                        disabled={followUpDisabled}
                        icon={<FormOutlined />}
                        onClick={() => onUseFollowUp(item)}
                        size="small"
                        type="link"
                      >
                        Use follow-up
                      </Button>
                    </TooltipAction>,
                  ]
                : undefined
            }
            className="diagnosis-evidence-item"
            key={consultationEvidenceRequestKey(item, index)}
          >
            <List.Item.Meta
              description={item.detail}
              title={
                <Space size={[6, 6]} wrap>
                  <span>{item.label}</span>
                  <Tag color={priorityColor(item.priority)}>
                    {item.priority}
                  </Tag>
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

function latestConsultationInsight(
  frame: DiagnosisTurnResultFrame,
): LatestConsultationInsight {
  const latestFollowUp = latestDiagnosisFollowUpTurn(frame);
  const evidenceTimeline = evidenceTimelineFromTurnResult(frame);
  if (latestFollowUp) {
    const fallbackEvidenceRequests = frame.evidence_requests ?? [];
    const fallbackCollectionResults = frame.evidence_collection_results ?? [];
    const followUpEvidenceRequests = latestFollowUp.evidence_requests ?? [];
    const followUpCollectionResults =
      latestFollowUp.evidence_collection_results ?? [];
    const selectedCollectionResults =
      followUpCollectionResults.length > 0 ||
      followUpEvidenceRequests.length > 0
        ? followUpCollectionResults
        : fallbackCollectionResults;
    return {
      assistantSequence: latestFollowUp.assistant_sequence,
      autoFollowUpCount: frame.follow_up_turns?.length ?? 0,
      collectionResults: collectionResultsForDisplay(
        evidenceTimeline,
        selectedCollectionResults,
      ),
      confidence: latestFollowUp.confidence,
      confidenceTimeline: frame.confidence_timeline ?? [],
      evidenceTimeline,
      evidenceRequests:
        followUpCollectionResults.length > 0 ||
        followUpEvidenceRequests.length > 0
          ? followUpEvidenceRequests
          : fallbackEvidenceRequests,
      insight: latestFollowUp.consultation_insight ?? {},
      requiresHumanReview: latestFollowUp.requires_human_review,
      status: frame.status,
      turnCount: latestFollowUp.turn_count,
    };
  }
  return {
    assistantSequence: frame.assistant_sequence,
    autoFollowUpCount: 0,
    collectionResults: collectionResultsForDisplay(
      evidenceTimeline,
      frame.evidence_collection_results ?? [],
    ),
    confidence: frame.confidence,
    confidenceTimeline: frame.confidence_timeline ?? [],
    evidenceTimeline,
    evidenceRequests: frame.evidence_requests ?? [],
    insight: frame.consultation_insight ?? {},
    requiresHumanReview: frame.requires_human_review,
    status: frame.status,
    turnCount: frame.turn_count,
  };
}

function latestConsultationInsightFromState(
  frame: DiagnosisStateFrame,
): LatestConsultationInsight | null {
  if (!hasConsultationInsight(frame.consultation_insight)) {
    return null;
  }
  const evidenceTimeline = evidenceTimelineFromState(frame);
  return {
    assistantSequence:
      latestConfidenceTimelineAssistantSequence(frame.confidence_timeline) ??
      frame.final_conclusion?.assistant_sequence,
    autoFollowUpCount: frame.follow_up_turns?.length ?? 0,
    collectionResults: collectionResultsForDisplay(
      evidenceTimeline,
      frame.evidence_collection_results ?? [],
    ),
    confidence: frame.confidence ?? frame.final_conclusion?.confidence ?? "",
    confidenceTimeline: frame.confidence_timeline ?? [],
    evidenceTimeline,
    evidenceRequests: frame.evidence_requests ?? [],
    insight: frame.consultation_insight,
    requiresHumanReview:
      frame.requires_human_review ??
      frame.final_conclusion?.requires_human_review ??
      false,
    status: frame.status,
    turnCount: frame.turn_count,
  };
}

function latestConfidenceTimelineAssistantSequence(
  items: DiagnosisConfidenceTimelineEntry[] | undefined,
): number | undefined {
  const latest = (items ?? [])
    .slice()
    .reverse()
    .find(
      (item) =>
        typeof item.assistant_sequence === "number" &&
        item.assistant_sequence > 0,
    );
  return latest?.assistant_sequence;
}

function evidenceTimelineForDisplay(
  latestInsight: LatestConsultationInsight,
): DiagnosisEvidenceTimelineEntry[] {
  if ((latestInsight.evidenceTimeline?.length ?? 0) > 0) {
    return latestInsight.evidenceTimeline;
  }
  if (
    latestInsight.evidenceRequests.length === 0 &&
    latestInsight.collectionResults.length === 0
  ) {
    return [];
  }
  return [
    {
      turn_count: Math.max(
        1,
        latestInsight.turnCount - latestInsight.autoFollowUpCount,
      ),
      trigger: "operator_turn",
      evidence_requests: latestInsight.evidenceRequests,
      evidence_collection_results: latestInsight.collectionResults,
    },
  ];
}

function collectionResultsForDisplay(
  evidenceTimeline: DiagnosisEvidenceTimelineEntry[],
  fallbackCollectionResults: DiagnosisEvidenceCollectionResult[],
): DiagnosisEvidenceCollectionResult[] {
  const timelineResults = evidenceTimeline.flatMap(
    (entry) => entry.evidence_collection_results ?? [],
  );
  if (timelineResults.length === 0) {
    return fallbackCollectionResults;
  }
  return uniqueEvidenceCollectionResults([
    ...timelineResults,
    ...fallbackCollectionResults,
  ]);
}

function uniqueEvidenceCollectionResults(
  items: DiagnosisEvidenceCollectionResult[],
): DiagnosisEvidenceCollectionResult[] {
  const seen = new Set<string>();
  const out: DiagnosisEvidenceCollectionResult[] = [];
  for (const item of items) {
    const key = evidenceCollectionResultIdentity(item);
    if (seen.has(key)) {
      continue;
    }
    seen.add(key);
    out.push(item);
  }
  return out;
}

function evidenceTimelineFromTurnResult(
  frame: DiagnosisTurnResultFrame,
): DiagnosisEvidenceTimelineEntry[] {
  if ((frame.evidence_timeline?.length ?? 0) > 0) {
    return frame.evidence_timeline ?? [];
  }
  const entries: DiagnosisEvidenceTimelineEntry[] = [];
  if (
    (frame.evidence_requests?.length ?? 0) > 0 ||
    (frame.evidence_collection_results?.length ?? 0) > 0
  ) {
    entries.push({
      turn_count: frame.turn_count,
      message_id: frame.message_id,
      assistant_message_id: frame.assistant_message_id,
      trigger: "operator_turn",
      evidence_requests: frame.evidence_requests,
      evidence_collection_results: frame.evidence_collection_results,
    });
  }
  for (const followUp of frame.follow_up_turns ?? []) {
    if (
      (followUp.evidence_requests?.length ?? 0) === 0 &&
      (followUp.evidence_collection_results?.length ?? 0) === 0
    ) {
      continue;
    }
    entries.push({
      turn_count: followUp.turn_count,
      message_id: followUp.message_id,
      assistant_message_id: followUp.assistant_message_id,
      trigger: followUp.trigger,
      evidence_requests: followUp.evidence_requests,
      evidence_collection_results: followUp.evidence_collection_results,
    });
  }
  return entries;
}

function evidenceTimelineFromState(
  frame: DiagnosisStateFrame,
): DiagnosisEvidenceTimelineEntry[] {
  if ((frame.evidence_timeline?.length ?? 0) > 0) {
    return frame.evidence_timeline ?? [];
  }
  if (
    (frame.evidence_requests?.length ?? 0) === 0 &&
    (frame.evidence_collection_results?.length ?? 0) === 0
  ) {
    return [];
  }
  return [
    {
      turn_count: frame.turn_count,
      trigger: "latest_evidence",
      evidence_requests: frame.evidence_requests,
      evidence_collection_results: frame.evidence_collection_results,
    },
  ];
}

function hasConsultationInsight(
  insight: DiagnosisConsultationInsight | undefined,
): insight is DiagnosisConsultationInsight {
  return Boolean(
    insight &&
    ((insight.confidence_rationale ?? "").trim() !== "" ||
      (insight.conclusion_status ?? "").trim() !== "" ||
      (insight.missing_evidence_requests?.length ?? 0) > 0 ||
      (insight.evidence_collection_suggestions?.length ?? 0) > 0),
  );
}

function diagnosisSubmitTurnBlockReason({
  actorSubject,
  connected,
  rbacBlockReason,
  turnInFlight,
}: {
  actorSubject: string;
  connected: boolean;
  rbacBlockReason: string;
  turnInFlight: boolean;
}): string {
  if (!connected) {
    return "Connect to a diagnosis room before sending evidence updates.";
  }
  const identityBlockReason = diagnosisActionIdentityBlockReason(
    actorSubject,
    "sending evidence updates",
  );
  if (identityBlockReason !== "") {
    return identityBlockReason;
  }
  if (rbacBlockReason !== "") {
    return rbacBlockReason;
  }
  if (turnInFlight) {
    return "Wait for the current AI turn or confirmation request to finish before sending another evidence update.";
  }
  return "";
}

function diagnosisConfirmConclusionBlockReason({
  actorSubject,
  connected,
  confirmInFlight,
  latestInsight,
  rbacBlockReason,
  state,
}: {
  actorSubject: string;
  connected: boolean;
  confirmInFlight: boolean;
  latestInsight: LatestConsultationInsight | null;
  rbacBlockReason: string;
  state: DiagnosisStateFrame | null;
}): string {
  if (!connected) {
    return "Connect to a diagnosis room before confirming.";
  }
  const identityBlockReason = diagnosisActionIdentityBlockReason(
    actorSubject,
    "confirming the diagnosis conclusion",
  );
  if (identityBlockReason !== "") {
    return identityBlockReason;
  }
  if (rbacBlockReason !== "") {
    return rbacBlockReason;
  }
  if (confirmInFlight) {
    return "Confirmation is in progress.";
  }
  if (!state) {
    return "Load the room state before confirming.";
  }
  if (state.status === "closed") {
    return "This diagnosis room is already closed.";
  }
  if (state.in_flight) {
    return "Wait for the current diagnosis turn to finish.";
  }
  const unresolvedEvidenceReason = latestInsight
    ? diagnosisReviewQueueBlockingReason(
        latestInsightReviewQueueInput(
          latestInsight,
          false,
          state.supplemental_evidence ?? [],
        ),
      )
    : "";
  if (unresolvedEvidenceReason !== "") {
    return unresolvedEvidenceReason;
  }
  const finalConclusionQueueInput = finalConclusionReviewQueueInput({
    collectionResults: state.evidence_collection_results ?? [],
    finalConclusion: state.final_conclusion,
    supplementalEvidence: state.supplemental_evidence ?? [],
  });
  const finalConclusionEvidenceReason = finalConclusionQueueInput
    ? diagnosisReviewQueueBlockingReason(finalConclusionQueueInput)
    : "";
  if (finalConclusionEvidenceReason !== "") {
    return finalConclusionEvidenceReason;
  }
  if (state.final_conclusion?.status === "available") {
    return "";
  }
  const conclusionStatus = latestInsight?.insight.conclusion_status?.trim();
  if (conclusionStatus === "final" || conclusionStatus === "ready_for_review") {
    return "";
  }
  return "Wait until AI marks the diagnosis final or ready for review.";
}

function roomStateDescriptionItems(
  state: DiagnosisStateFrame | null,
  readySubject: string,
  sessionID: string | undefined,
  connectionStatus: DiagnosisConnectionStatus,
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>,
): DescriptionsProps["items"] {
  const conclusion = state?.final_conclusion;
  const subject = readySubject || state?.owner_subject || "";
  const items: DescriptionsProps["items"] = [
    {
      key: "subject",
      label: "Subject",
      children:
        subject === "" ? (
          "-"
        ) : (
          <ActorSubjectTags
            directoryUsersBySubject={directoryUsersBySubject}
            subject={subject}
          />
        ),
    },
    {
      key: "session",
      label: "Session",
      children: state?.session_id || sessionID || "-",
    },
    {
      key: "status",
      label: "Status",
      children: state?.status || statusLabel(connectionStatus),
    },
    {
      key: "turns",
      label: "Turns",
      children: state ? String(state.turn_count) : "-",
    },
    {
      key: "close-reason",
      label: "Close reason",
      children: state?.close_reason || "-",
    },
    {
      key: "conclusion",
      label: "Conclusion",
      children: finalConclusionLabel(state),
    },
  ];
  if (conclusion) {
    if (conclusion.evidence_snapshot_id !== undefined) {
      items.push({
        key: "evidence-snapshot",
        label: "Evidence snapshot",
        children: String(conclusion.evidence_snapshot_id),
      });
    }
    if (conclusion.conclusion_version) {
      items.push({
        key: "conclusion-version",
        label: "Conclusion version",
        children: conclusion.conclusion_version,
      });
    }
    if (conclusion.confirmed_by) {
      items.push({
        key: "confirmed-by",
        label: "Confirmed by",
        children: (
          <ActorSubjectTags
            directoryUsersBySubject={directoryUsersBySubject}
            subject={conclusion.confirmed_by}
          />
        ),
      });
    }
    if (conclusion.recorded_at) {
      items.push({
        key: "recorded-at",
        label: "Recorded at",
        children: formatDateTime(conclusion.recorded_at),
      });
    }
  }
  items.push({
    key: "in-flight",
    label: "In flight",
    children: state?.in_flight ? "yes" : "no",
  });
  if (state?.latest_error) {
    items.push({
      key: "latest-error",
      label: "Latest error",
      children: `${state.latest_error.code}: ${state.latest_error.message}`,
    });
  }
  return items;
}

function consultationInsightItems(
  latestInsight: LatestConsultationInsight,
): DescriptionsProps["items"] {
  return [
    { key: "turn", label: "Turn", children: String(latestInsight.turnCount) },
    {
      key: "status",
      label: "Room status",
      children: latestInsight.status || "-",
    },
    {
      key: "conclusion-status",
      label: "Conclusion status",
      children: latestInsight.insight.conclusion_status || "-",
    },
    {
      key: "review",
      label: "Human review",
      children: latestInsight.requiresHumanReview ? "required" : "optional",
    },
    {
      key: "auto-follow-up",
      label: "Auto follow-up",
      children: String(latestInsight.autoFollowUpCount),
    },
  ];
}

function formatActiveAlert(alert: DiagnosisActiveAlert): string {
  const labels = alert.labels ?? {};
  const alertName = labels.alertname ?? labels.alert ?? "alert";
  const context = [labels.namespace, labels.pod].filter(Boolean).join(" / ");
  return context
    ? `${alertName} / ${context}`
    : `${alertName} / ${alert.source}`;
}

function hasMetricResult(item: DiagnosisEvidenceCollectionResult): boolean {
  const result = item.metric_result;
  return Boolean(
    result &&
    (result.result_type ||
      result.scalar ||
      result.string ||
      (result.series && result.series.length > 0) ||
      (result.warnings && result.warnings.length > 0)),
  );
}

function formatMetricSeries(series: DiagnosisMetricSeries): string {
  const metric = series.metric ?? {};
  const metricName = metric.__name__ ?? metric.job ?? "series";
  const context = [metric.namespace, metric.pod, metric.instance]
    .filter(Boolean)
    .join(" / ");
  const latest = series.points?.[series.points.length - 1]?.value;
  const prefix = context ? `${metricName} / ${context}` : metricName;
  return latest ? `${prefix}: ${latest}` : prefix;
}

function metricSeriesKey(series: DiagnosisMetricSeries, index: number): string {
  const metric = series.metric ?? {};
  return `${metric.__name__ ?? metric.job ?? "series"}-${metric.instance ?? metric.pod ?? "none"}-${index}`;
}

function evidenceRequestKey(
  item: DiagnosisEvidenceRequest,
  index: number,
): string {
  return `${item.tool}-${item.template_id ?? "none"}-${item.reason}-${index}`;
}

function evidenceCollectionResultKey(
  item: DiagnosisEvidenceCollectionResult,
  index: number,
): string {
  return `${item.tool}-${item.status}-${item.reason_code}-${item.collected_at}-${index}`;
}

function evidenceCollectionResultIdentity(
  item: DiagnosisEvidenceCollectionResult,
): string {
  const request = collectionResultRequest(item);
  return [
    item.tool,
    item.status,
    item.reason_code,
    item.collected_at,
    request.template_id ?? "no-template",
    request.alert_source_profile_id ?? "no-profile",
    request.query ?? "no-query",
    request.window_seconds ?? "no-window",
    request.step_seconds ?? "no-step",
    request.limit ?? "no-limit",
  ].join(":");
}

function evidenceTimelineEntryKey(
  item: DiagnosisEvidenceTimelineEntry,
  index: number,
): string {
  return `${item.turn_count}-${item.message_id ?? item.assistant_message_id ?? item.trigger ?? "evidence"}-${index}`;
}

function confidenceTimelineKey(
  item: DiagnosisConfidenceTimelineEntry,
  index: number,
): string {
  return `${item.turn_count}-${item.assistant_message_id ?? item.message_id ?? item.occurred_at}-${index}`;
}

function consultationEvidenceRequestKey(
  item: DiagnosisConsultationEvidenceRequest,
  index: number,
): string {
  return `${item.priority}-${item.label}-${index}`;
}

function supplementalEvidenceRecordKey(
  item: DiagnosisSupplementalEvidenceRecord,
  index: number,
): string {
  return `${item.user_message_id}-${item.assistant_message_id}-${index}`;
}

function supplementalEvidenceRequestIdentity(
  item: DiagnosisConsultationEvidenceRequest,
): string {
  return `${item.priority}:${item.label}:${item.detail}`;
}

function supplementalEvidenceTopicKey(label: string): string {
  return label.trim().toLowerCase().replace(/\s+/g, " ");
}

function activeAlertKey(alert: DiagnosisActiveAlert, index: number): string {
  const labels = alert.labels ?? {};
  return `${alert.source}-${labels.alertname ?? labels.alert ?? "alert"}-${labels.namespace ?? "none"}-${index}`;
}

function finalConclusionLabel(state: DiagnosisStateFrame | null): string {
  return diagnosisFinalConclusionStatusLabel(state?.final_conclusion);
}

function finalConclusionText(state: DiagnosisStateFrame): string {
  return diagnosisFinalConclusionText(state.final_conclusion);
}

function RetainedFinalConclusionSummary({
  directoryUsersBySubject,
  onRefreshDeliveryProof,
  onReviewDelivery,
  refreshingDeliveryProof,
  room,
}: {
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  onRefreshDeliveryProof: () => void;
  onReviewDelivery: () => void;
  refreshingDeliveryProof: boolean;
  room: DiagnosisRoomSummary;
}) {
  const conclusion = room.latest_conclusion;
  if (!conclusion) {
    return null;
  }
  const confidence = conclusion.confidence || "unknown";
  const notificationDeliveryCoverage = diagnosisNotificationDeliveryCoverage(
    room.notification_timeline ?? [],
  );
  const traceability = diagnosisFinalConclusionTraceabilityStatus({
    conclusion,
    notificationDelivery: notificationDeliveryCoverage,
  });
  const deliveryRecoveryHint = diagnosisNotificationDeliveryRecoveryHint(
    notificationDeliveryCoverage,
  );
  const lifecycle = diagnosisConsultationConclusionLifecycleStatus({
    finalConclusion: conclusion,
    notificationDelivery: notificationDeliveryCoverage,
  });
  const proofActionType =
    traceability.status === "complete" ? "default" : "primary";
  const items: DescriptionsProps["items"] = [
    {
      key: "confidence",
      label: "Confidence",
      children: <Tag color={confidenceColor(confidence)}>{confidence}</Tag>,
    },
    {
      key: "human-review",
      label: "Human review",
      children: conclusion.requires_human_review ? "required" : "not required",
    },
    {
      key: "confirmed-by",
      label: "Confirmed by",
      children: conclusion.confirmed_by ? (
        <ActorSubjectTags
          directoryUsersBySubject={directoryUsersBySubject}
          subject={conclusion.confirmed_by}
        />
      ) : (
        "-"
      ),
    },
    {
      key: "recorded",
      label: "Recorded",
      children: formatDateTime(conclusion.recorded_at),
    },
    {
      key: "version",
      label: "Conclusion version",
      children: conclusion.conclusion_version || "-",
    },
    {
      key: "delivery",
      label: "Delivery",
      children: (
        <Tag
          color={notificationDeliveryCoverageStatusColor(
            lifecycle.notificationStatus ?? "pending",
          )}
        >
          {notificationDeliveryCoverage.label}
        </Tag>
      ),
    },
  ];
  return (
    <Alert
      action={
        <Space size={[6, 6]} wrap>
          <Button
            icon={<WechatOutlined />}
            onClick={onReviewDelivery}
            size="small"
          >
            Review delivery
          </Button>
          <Button
            icon={<ReloadOutlined />}
            loading={refreshingDeliveryProof}
            onClick={onRefreshDeliveryProof}
            size="small"
            type={proofActionType}
          >
            Refresh proof
          </Button>
        </Space>
      }
      className="diagnosis-conclusion"
      description={
        <div className="diagnosis-final-conclusion-summary">
          <Space size={[6, 6]} wrap>
            <Tag color={consultationConclusionLifecycleColor(lifecycle.status)}>
              {lifecycle.label}
            </Tag>
            <Tag color={traceability.color}>{traceability.label}</Tag>
            <Typography.Text
              type={traceability.status === "blocked" ? "danger" : "secondary"}
            >
              {traceability.detail}
            </Typography.Text>
          </Space>
          <Typography.Paragraph>{conclusion.content}</Typography.Paragraph>
          <Descriptions column={{ xs: 1, md: 2 }} items={items} size="small" />
          <Space size={[6, 6]} wrap>
            {notificationDeliveryCoverage.phases.map((phase) => (
              <Tag
                color={diagnosisNotificationDeliveryCoveragePhaseColor(
                  phase.status,
                )}
                key={phase.key}
                title={phase.detail}
              >
                {phase.label}: {phase.status}
              </Tag>
            ))}
          </Space>
          {deliveryRecoveryHint !== null &&
          notificationDeliveryCoverage.status !== "ready" ? (
            <Alert
              description={deliveryRecoveryHint.detail}
              message={deliveryRecoveryHint.label}
              showIcon
              type={deliveryRecoveryHint.color}
            />
          ) : null}
        </div>
      }
      message="Retained final conclusion"
      showIcon
      type={finalConclusionTraceabilityAlertType(traceability.status)}
    />
  );
}

function FinalConclusionDetails({
  actionDisabledReason,
  connected,
  directoryUsersBySubject,
  notificationDeliveryCoverage,
  onUseEvidencePlan,
  onUseFollowUp,
  state,
}: {
  actionDisabledReason: string;
  connected: boolean;
  directoryUsersBySubject: ReadonlyMap<string, DiagnosisCollaborationDirectoryUser>;
  notificationDeliveryCoverage?: ReturnType<
    typeof diagnosisNotificationDeliveryCoverage
  >;
  onUseEvidencePlan: (item: DiagnosisEvidenceRequest) => void;
  onUseFollowUp: (item: DiagnosisConsultationEvidenceRequest) => void;
  state: DiagnosisStateFrame;
}) {
  const conclusion = state.final_conclusion;
  if (!conclusion) {
    return null;
  }
  const retention = diagnosisFinalConclusionRetentionState(conclusion);
  const findings = conclusion.findings ?? [];
  const recommendedActions = conclusion.recommended_actions ?? [];
  const missingEvidence = conclusion.missing_evidence_requests ?? [];
  const collectionSuggestions =
    conclusion.evidence_collection_suggestions ?? [];
  const evidenceRequests = conclusion.evidence_requests ?? [];
  const reviewItems = diagnosisFinalConclusionReviewItems(conclusion);
  const confidenceProgress = diagnosisFinalConclusionConfidenceProgress(
    conclusion,
    state.confidence_timeline,
  );
  const traceability = diagnosisFinalConclusionTraceabilityStatus({
    conclusion,
    notificationDelivery: notificationDeliveryCoverage,
  });
  const actionDisabled = !connected || actionDisabledReason !== "";
  const traceabilityReviewLabel =
    traceability.reviewOpenCount > 0
      ? `${traceability.reviewOpenCount} blocking review item(s)`
      : traceability.reviewResidualCount > 0
        ? `${traceability.reviewResidualCount} residual review item(s)`
        : "Review clear";

  return (
    <Space
      className="diagnosis-final-conclusion-detail"
      direction="vertical"
      size={10}
    >
      <Space size={[6, 6]} wrap>
        <Tag color={finalConclusionRetentionColor(retention.status)}>
          {retention.label}
        </Tag>
        <Typography.Text type="secondary">{retention.detail}</Typography.Text>
      </Space>
      <Space size={[6, 6]} wrap>
        <Tag color={traceability.color}>{traceability.label}</Tag>
        <Tag>{traceability.notificationLabel}</Tag>
        <Tag>{traceabilityReviewLabel}</Tag>
        <Typography.Text
          type={traceability.status === "blocked" ? "danger" : "secondary"}
        >
          {traceability.detail}
        </Typography.Text>
      </Space>
      {notificationDeliveryCoverage !== undefined ? (
        <section aria-label="Final conclusion delivery proof">
          <Space direction="vertical" size={4}>
            <Typography.Text strong>Closure delivery proof</Typography.Text>
            <Space size={[6, 6]} wrap>
              {notificationDeliveryCoverage.phases.map((phase) => (
                <Tag
                  color={diagnosisNotificationDeliveryCoveragePhaseColor(
                    phase.status,
                  )}
                  key={phase.key}
                  title={phase.detail}
                >
                  {phase.label}: {phase.status}
                </Tag>
              ))}
            </Space>
            <Typography.Text
              type={
                notificationDeliveryCoverage.status === "blocked"
                  ? "danger"
                  : "secondary"
              }
            >
              {notificationDeliveryCoverage.detail}
            </Typography.Text>
          </Space>
        </section>
      ) : null}
      <Space size={[6, 6]} wrap>
        <Tag
          color={finalConclusionConfidenceProgressColor(
            confidenceProgress.status,
          )}
        >
          {confidenceProgress.label}
        </Tag>
        <Typography.Text type="secondary">
          {confidenceProgress.detail}
        </Typography.Text>
      </Space>
      <Typography.Paragraph>{finalConclusionText(state)}</Typography.Paragraph>
      <ActorSubjectTags
        directoryUsersBySubject={directoryUsersBySubject}
        label="Confirmed by"
        subject={conclusion.confirmed_by}
      />
      {conclusion.confidence_rationale ? (
        <Typography.Text type="secondary">
          {conclusion.confidence_rationale}
        </Typography.Text>
      ) : null}
      <FinalConclusionReviewChecklist items={reviewItems} />
      <FinalConclusionStringList items={findings} title="Findings" />
      <FinalConclusionStringList
        items={recommendedActions}
        title="Recommended actions"
      />
      <FinalConclusionEvidenceList
        items={missingEvidence}
        onUseFollowUp={onUseFollowUp}
        actionDisabledReason={actionDisabledReason}
        connected={connected}
        actionLabel="Add evidence"
        title="Missing evidence"
      />
      <FinalConclusionEvidenceList
        items={collectionSuggestions}
        onUseFollowUp={onUseFollowUp}
        actionDisabledReason={actionDisabledReason}
        connected={connected}
        actionLabel="Prepare follow-up"
        title="Evidence collection suggestions"
      />
      {evidenceRequests.length > 0 ? (
        <section aria-label="Final conclusion executable evidence requests">
          <Typography.Text strong>Executable evidence requests</Typography.Text>
          <List
            dataSource={evidenceRequests}
            renderItem={(item, index) => (
              <List.Item
                actions={[
                  <TooltipAction
                    disabled={actionDisabled}
                    key="collect-evidence"
                    title={
                      actionDisabledReason || `Collect ${item.tool} evidence.`
                    }
                  >
                    <Button
                      aria-label={`Collect evidence for ${item.tool}`}
                      disabled={actionDisabled}
                      icon={<PlayCircleOutlined />}
                      onClick={() => onUseEvidencePlan(item)}
                      size="small"
                      type="link"
                    >
                      Collect evidence
                    </Button>
                  </TooltipAction>,
                ]}
                key={evidenceRequestKey(item, index)}
              >
                <Space direction="vertical" size={2}>
                  <Space size={[6, 6]} wrap>
                    <Tag>{item.tool}</Tag>
                    {item.reason ? (
                      <Typography.Text>{item.reason}</Typography.Text>
                    ) : null}
                  </Space>
                  <EvidenceRequestMetadata
                    fallbackText="No additional parameters"
                    request={item}
                  />
                </Space>
              </List.Item>
            )}
            size="small"
          />
        </section>
      ) : null}
    </Space>
  );
}

function FinalConclusionStringList({
  items,
  title,
}: {
  items: string[];
  title: string;
}) {
  const visibleItems = items
    .map((item) => item.trim())
    .filter((item) => item !== "");
  if (visibleItems.length === 0) {
    return null;
  }
  return (
    <section aria-label={`Final conclusion ${title.toLowerCase()}`}>
      <Typography.Text strong>{title}</Typography.Text>
      <List
        dataSource={visibleItems}
        renderItem={(item, index) => (
          <List.Item key={`${title}-${index}`}>{item}</List.Item>
        )}
        size="small"
      />
    </section>
  );
}

function FinalConclusionReviewChecklist({
  items,
}: {
  items: ReturnType<typeof diagnosisFinalConclusionReviewItems>;
}) {
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-label="Final conclusion review checklist">
      <Typography.Text strong>Review checklist</Typography.Text>
      <List
        dataSource={items}
        renderItem={(item) => (
          <List.Item key={item.key}>
            <List.Item.Meta
              description={item.detail}
              title={
                <Space size={[6, 6]} wrap>
                  <Tag color={finalConclusionReviewItemColor(item.status)}>
                    {item.status}
                  </Tag>
                  <span>{item.title}</span>
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

function finalConclusionRetentionColor(
  status: ReturnType<typeof diagnosisFinalConclusionRetentionState>["status"],
): string {
  switch (status) {
    case "retained":
      return "green";
    case "ready":
      return "blue";
    case "needs_review":
      return "gold";
    case "missing":
      return "default";
  }
}

function finalConclusionReviewItemColor(
  status: ReturnType<
    typeof diagnosisFinalConclusionReviewItems
  >[number]["status"],
): string {
  switch (status) {
    case "attention":
      return "warning";
    case "done":
      return "success";
    case "residual":
      return "default";
    case "ready":
      return "processing";
  }
}

function finalConclusionConfidenceProgressColor(
  status: ReturnType<
    typeof diagnosisFinalConclusionConfidenceProgress
  >["status"],
): string {
  switch (status) {
    case "improved":
      return "success";
    case "declined":
      return "error";
    case "stable":
      return "processing";
    case "unknown":
      return "default";
  }
}

function confidenceProgressAlertType(
  status: ReturnType<
    typeof diagnosisFinalConclusionConfidenceProgress
  >["status"],
): "success" | "warning" | "info" {
  switch (status) {
    case "improved":
      return "success";
    case "declined":
      return "warning";
    case "stable":
      return "info";
    case "unknown":
      return "info";
  }
}

function FinalConclusionEvidenceList({
  actionDisabledReason,
  actionLabel,
  connected,
  items,
  onUseFollowUp,
  title,
}: {
  actionDisabledReason: string;
  actionLabel: string;
  connected: boolean;
  items: DiagnosisConsultationEvidenceRequest[];
  onUseFollowUp: (item: DiagnosisConsultationEvidenceRequest) => void;
  title: string;
}) {
  if (items.length === 0) {
    return null;
  }
  const actionDisabled = !connected || actionDisabledReason !== "";
  return (
    <section aria-label={`Final conclusion ${title.toLowerCase()}`}>
      <Typography.Text strong>{title}</Typography.Text>
      <List
        dataSource={items}
        renderItem={(item, index) => (
          <List.Item
            actions={[
              <TooltipAction
                disabled={actionDisabled}
                key="use-follow-up"
                title={
                  actionDisabledReason || `Prepare follow-up for ${item.label}.`
                }
              >
                <Button
                  aria-label={`${actionLabel} for ${item.label}`}
                  disabled={actionDisabled}
                  icon={<FormOutlined />}
                  onClick={() => onUseFollowUp(item)}
                  size="small"
                  type="link"
                >
                  {actionLabel}
                </Button>
              </TooltipAction>,
            ]}
            key={consultationEvidenceRequestKey(item, index)}
          >
            <Space direction="vertical" size={2}>
              <Space size={[6, 6]} wrap>
                <Tag color={priorityColor(item.priority)}>{item.priority}</Tag>
                <Typography.Text>{item.label}</Typography.Text>
              </Space>
              <Typography.Text type="secondary">{item.detail}</Typography.Text>
            </Space>
          </List.Item>
        )}
        size="small"
      />
    </section>
  );
}

function alertContextDescriptionItems(
  alert: AlertEventSummary,
  snapshot: AlertEvidenceSnapshotLink,
): DescriptionsProps["items"] {
  return [
    {
      key: "summary",
      label: "Summary",
      children: alertSummary(alert),
    },
    {
      key: "source",
      label: "Source",
      children: alert.source,
    },
    {
      key: "source-profile",
      label: "Alert source profile",
      children:
        alert.alert_source_profile_id > 0
          ? `#${alert.alert_source_profile_id}`
          : "legacy provider-only",
    },
    {
      key: "canonical",
      label: "Canonical fingerprint",
      children: (
        <Typography.Text className="settings-event-ids" copyable>
          {alert.canonical_fingerprint}
        </Typography.Text>
      ),
    },
    {
      key: "source-fingerprint",
      label: "Source fingerprint",
      children: (
        <Typography.Text className="settings-event-ids" copyable>
          {alert.source_fingerprint}
        </Typography.Text>
      ),
    },
    {
      key: "started",
      label: "Started",
      children: formatDateTime(alert.starts_at),
    },
    {
      key: "ended",
      label: "Ended",
      children: alert.ends_at ? formatDateTime(alert.ends_at) : "Still firing",
    },
    {
      key: "snapshot",
      label: "Evidence snapshot",
      children: `#${snapshot.id} from ${snapshot.created_by_workflow || "manual"}`,
    },
    {
      key: "snapshot-created",
      label: "Snapshot created",
      children: formatDateTime(snapshot.created_at),
    },
  ];
}

function alertName(alert: AlertEventSummary): string {
  return (
    alert.labels.alertname || alert.labels.alert_name || `Alert #${alert.id}`
  );
}

function alertSummary(alert: AlertEventSummary): string {
  return (
    alert.annotations.summary ||
    alert.annotations.description ||
    alert.annotations.message ||
    alert.canonical_fingerprint
  );
}

function alertSeverity(alert: AlertEventSummary): string {
  const value = alert.labels.severity || alert.labels.priority || "info";
  return value.trim() === "" ? "info" : value;
}

function sortedRecordEntries(
  record: Record<string, string>,
): Array<[string, string]> {
  return Object.entries(record)
    .filter(([, value]) => value.trim() !== "")
    .sort(([left], [right]) => left.localeCompare(right));
}

function severityColor(severity: string): string {
  switch (severity.toLowerCase()) {
    case "critical":
      return "error";
    case "warning":
      return "warning";
    default:
      return "processing";
  }
}

function snapshotStatusColor(
  status: AlertEvidenceSnapshotLink["status"],
): string {
  switch (status) {
    case "complete":
      return "success";
    case "partial":
      return "warning";
    case "failed":
      return "error";
  }
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

function roomStatusColor(status: string): string {
  return status === "open" ? "success" : "default";
}

function taskStatusColor(status: string): string {
  switch (status) {
    case "running":
      return "processing";
    case "succeeded":
      return "success";
    case "failed":
      return "error";
    case "cancelled":
      return "default";
    default:
      return "warning";
  }
}

function workflowVisibilityStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case "running":
      return "processing";
    case "completed":
      return "success";
    case "not_found":
    case "failed":
    case "canceled":
    case "cancelled":
    case "terminated":
    case "timed_out":
      return "error";
    default:
      return "default";
  }
}

function notificationStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case "delivered":
    case "sent":
    case "success":
    case "accepted":
      return "success";
    case "failed":
    case "error":
      return "error";
    case "pending":
    case "retrying":
      return "warning";
    default:
      return "default";
  }
}

function notificationDeliveryCoverageStatusColor(
  status: NonNullable<
    DiagnosisConsultationConclusionLifecycleStatus["notificationStatus"]
  >,
): string {
  switch (status) {
    case "ready":
      return "success";
    case "blocked":
      return "error";
    case "review":
      return "warning";
    case "pending":
      return "default";
  }
}

function finalConclusionTraceabilityAlertType(
  status: ReturnType<
    typeof diagnosisFinalConclusionTraceabilityStatus
  >["status"],
): "success" | "info" | "warning" | "error" {
  switch (status) {
    case "complete":
      return "success";
    case "blocked":
      return "error";
    case "review":
      return "warning";
    case "pending":
      return "info";
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

function confidencePercent(confidence: string): number {
  switch (confidence.toLowerCase()) {
    case "high":
      return 90;
    case "medium":
      return 60;
    case "low":
      return 35;
    default:
      return 10;
  }
}

function confidenceProgressStatus(
  latestInsight: LatestConsultationInsight,
): "success" | "exception" | "normal" {
  const status = latestInsight.insight.conclusion_status?.toLowerCase();
  if (status === "final" || status === "ready_for_review") {
    return "success";
  }
  if (latestInsight.confidence.toLowerCase() === "low") {
    return "exception";
  }
  return "normal";
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

function consultationConclusionLifecycleColor(
  status: DiagnosisConsultationConclusionLifecycleStatus["status"],
): string {
  switch (status) {
    case "delivered":
    case "retained":
      return "green";
    case "delivery_pending":
      return "blue";
    case "delivery_blocked":
      return "red";
    case "delivery_review":
      return "orange";
    case "available":
    case "ready_for_review":
      return "blue";
    case "not_ready":
      return "gray";
  }
}

function consultationConclusionLifecycleTags(
  lifecycle: DiagnosisConsultationConclusionLifecycleStatus,
): Array<{ color: string; label: string }> {
  const tags: Array<{ color: string; label: string }> = [];
  if (lifecycle.conclusionStatus !== undefined) {
    tags.push({
      color: conclusionStatusColor(lifecycle.conclusionStatus),
      label: lifecycle.conclusionStatus,
    });
  }
  if (lifecycle.confirmedBy !== undefined) {
    tags.push({
      color: "success",
      label: "operator confirmed",
    });
  }
  if (lifecycle.notificationStatus !== undefined) {
    tags.push({
      color: notificationDeliveryCoverageStatusColor(
        lifecycle.notificationStatus,
      ),
      label: `delivery ${lifecycle.notificationStatus}`,
    });
  }
  return tags;
}

function conclusionStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case "final":
    case "ready_for_review":
      return "success";
    case "needs_evidence":
      return "warning";
    case "blocked":
      return "error";
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

function confidenceTimelineColor(
  item: DiagnosisConfidenceTimelineEntry,
): string {
  if (item.requires_human_review) {
    return "orange";
  }
  switch (confidenceColor(item.confidence)) {
    case "success":
      return "green";
    case "warning":
      return "orange";
    case "error":
      return "red";
    default:
      return "blue";
  }
}

function nextDiagnosisAction(
  latestInsight: LatestConsultationInsight,
  supplementalEvidence: DiagnosisSupplementalEvidenceRecord[],
): string {
  return diagnosisReviewQueueNextAction(
    latestInsightReviewQueueInput(latestInsight, false, supplementalEvidence),
  );
}

function latestInsightReviewQueueInput(
  latestInsight: LatestConsultationInsight,
  canConfirmConclusion: boolean,
  supplementalEvidence: DiagnosisSupplementalEvidenceRecord[],
): DiagnosisReviewQueueInput {
  return {
    canConfirmConclusion,
    collectionResults: latestInsight.collectionResults,
    confidence: latestInsight.confidence,
    confidenceProgress: diagnosisFinalConclusionConfidenceProgress(
      latestInsight,
      latestInsight.confidenceTimeline,
    ),
    conclusionStatus: latestInsight.insight.conclusion_status,
    evidenceCollectionSuggestions:
      latestInsight.insight.evidence_collection_suggestions ?? [],
    evidenceRequests: latestInsight.evidenceRequests,
    latestAssistantSequence: latestInsight.assistantSequence,
    missingEvidenceRequests:
      latestInsight.insight.missing_evidence_requests ?? [],
    requiresHumanReview: latestInsight.requiresHumanReview,
    supplementalEvidence,
  };
}

function shouldRenderFinalConclusionReviewQueue(
  input: DiagnosisReviewQueueInput | null,
): input is DiagnosisReviewQueueInput {
  if (!input) {
    return false;
  }
  return (
    input.evidenceRequests.length > 0 ||
    input.missingEvidenceRequests.length > 0 ||
    input.evidenceCollectionSuggestions.length > 0 ||
    input.collectionResults.some(
      (result) => result.status.toLowerCase() !== "collected",
    )
  );
}

function formatSupplementalEvidenceSummary(
  planCount: number,
  supplementalCount: number,
): string {
  const parts: string[] = [];
  if (planCount > 0) {
    parts.push(`${planCount} executable request${planCount === 1 ? "" : "s"}`);
  }
  if (supplementalCount > 0) {
    parts.push(
      `${supplementalCount} supplemental request${supplementalCount === 1 ? "" : "s"}`,
    );
  }
  return parts.length > 0
    ? parts.join(" and ")
    : "No supplemental evidence requested.";
}

function evidencePlanFollowUpMessage(
  request: DiagnosisEvidenceRequest,
): string {
  const lines = [
    "Run planned evidence collection",
    "",
    `Tool: ${request.tool}`,
    `Reason: ${request.reason}`,
  ];
  if (request.query) {
    lines.push(`Query: ${request.query}`);
  }
  if (request.template_id) {
    lines.push(`Template: ${request.template_id}`);
  }
  if (request.alert_source_profile_id) {
    lines.push(`Alert source profile: ${request.alert_source_profile_id}`);
  }
  if (request.window_seconds) {
    lines.push(`Window: ${request.window_seconds}s`);
  }
  if (request.step_seconds) {
    lines.push(`Step: ${request.step_seconds}s`);
  }
  if (request.limit) {
    lines.push(`Limit: ${request.limit}`);
  }
  lines.push(
    "",
    "After collecting this evidence, reassess confidence and state whether the conclusion is ready for operator review.",
    "If only non-executable operator or DBA evidence remains unavailable, keep requires_human_review true and use ready_for_review with explicit caveats instead of repeating the same request.",
  );
  return lines.join("\n");
}

function operatorEvidenceFollowUpMessage(
  request: DiagnosisEvidenceRequest,
): string {
  return evidencePlanFollowUpMessage(request).replace(
    "Run planned evidence collection",
    "Run operator evidence collection",
  );
}

function operatorEvidenceRecommendations(
  templates: DiagnosisToolTemplate[],
  alertContext: DiagnosisAlertContext | undefined,
): OperatorEvidenceRecommendation[] {
  const alertSourceProfileID = alertContext?.alert.alert_source_profile_id ?? 0;
  return templates
    .slice()
    .sort((left, right) =>
      operatorEvidenceTemplateSort(left, right, alertSourceProfileID),
    )
    .slice(0, 5)
    .map((template) =>
      operatorEvidenceRecommendation(
        template,
        alertContext,
        alertSourceProfileID,
      ),
    );
}

function operatorEvidenceRecommendation(
  template: DiagnosisToolTemplate,
  alertContext: DiagnosisAlertContext | undefined,
  alertSourceProfileID: number,
): OperatorEvidenceRecommendation {
  const queryResult = operatorEvidenceTemplateQuery(template, alertContext);
  const formValues = {
    ...operatorEvidenceFormValuesFromTemplate(template),
    ...(queryResult.query !== undefined ? { query: queryResult.query } : {}),
  };
  const sourceDisabledReason = operatorEvidenceTemplateSourceDisabledReason(
    template,
    alertSourceProfileID,
  );
  const disabledReason = [
    queryResult.missing.length > 0
      ? `Missing ${queryResult.missing.join(", ")} in the current alert context.`
      : "",
    sourceDisabledReason,
  ]
    .filter((reason) => reason !== "")
    .join(" ");
  const ready = disabledReason === "";
  return {
    detail: operatorEvidenceRecommendationDetail(
      template,
      queryResult.query,
      alertSourceProfileID,
    ),
    disabledReason,
    formValues,
    key: `template-${template.id}`,
    ready,
    sourceMatches:
      alertSourceProfileID > 0 &&
      template.alert_source_profile_id === alertSourceProfileID,
    tag: template.tool,
    template,
    title: template.name,
  };
}

function operatorEvidenceRecommendationDetail(
  template: DiagnosisToolTemplate,
  query: string | undefined,
  alertSourceProfileID: number,
): string {
  const parts = [
    `Uses template #${template.id}`,
    `source #${template.alert_source_profile_id}`,
    `limit ${template.default_limit}`,
  ];
  if (alertSourceProfileID > 0) {
    parts.push(
      template.alert_source_profile_id === alertSourceProfileID
        ? "matches alert source"
        : `alert source #${alertSourceProfileID} context differs`,
    );
  }
  if (template.tool === "metric_range_query") {
    parts.push(
      `window ${template.default_window_seconds}s`,
      `step ${template.default_step_seconds}s`,
    );
  }
  if (query !== undefined && query.trim() !== "") {
    parts.push(`query ${query}`);
  }
  return parts.join(" / ");
}

function operatorEvidenceTemplateSort(
  left: DiagnosisToolTemplate,
  right: DiagnosisToolTemplate,
  alertSourceProfileID = 0,
): number {
  if (alertSourceProfileID > 0) {
    const sourcePriority =
      Number(left.alert_source_profile_id !== alertSourceProfileID) -
      Number(right.alert_source_profile_id !== alertSourceProfileID);
    if (sourcePriority !== 0) {
      return sourcePriority;
    }
  }
  const toolPriority =
    operatorEvidenceToolPriority(left.tool) -
    operatorEvidenceToolPriority(right.tool);
  if (toolPriority !== 0) {
    return toolPriority;
  }
  return left.id - right.id;
}

function operatorEvidenceToolPriority(tool: DiagnosisToolKind): number {
  switch (tool) {
    case "active_alerts":
      return 0;
    case "metric_range_query":
      return 1;
    case "metric_query":
      return 2;
  }
}

function enabledDiagnosisToolTemplates(
  result: ApiResult<DiagnosisToolTemplateListResponse> | undefined,
): DiagnosisToolTemplate[] {
  if (result?.ok !== true) {
    return [];
  }
  return result.data.items.filter((template) => template.enabled);
}

function notificationChannelItems(
  result: ApiResult<NotificationChannelProfileListResponse> | undefined,
) {
  if (result?.ok !== true) {
    return [];
  }
  return result.data.items;
}

function operatorEvidenceTemplateOptions(templates: DiagnosisToolTemplate[]) {
  return templates.map((template) => ({
    label: `#${template.id} ${template.name} / ${template.tool}`,
    title: operatorEvidenceTemplateSummary(template),
    value: template.id,
  }));
}

function applyOperatorEvidenceTemplate(
  form: FormInstance<OperatorEvidenceFormValues>,
  templates: DiagnosisToolTemplate[],
  templateID: number | undefined,
) {
  if (templateID === undefined) {
    form.setFieldsValue({
      selectedTemplateID: null,
      templateID: null,
    });
    return;
  }
  const template = templates.find((candidate) => candidate.id === templateID);
  if (template === undefined) {
    return;
  }
  form.setFieldsValue(operatorEvidenceFormValuesFromTemplate(template));
}

function operatorEvidenceFormValuesFromTemplate(
  template: DiagnosisToolTemplate,
): Partial<OperatorEvidenceFormValues> {
  const query =
    template.tool === "active_alerts" ||
    operatorEvidenceTemplateHasParameterizedQuery(template)
      ? ""
      : template.query_template;
  return {
    alertSourceProfileID: template.alert_source_profile_id,
    limit: template.default_limit > 0 ? template.default_limit : null,
    query,
    reason: `Collect ${template.name}.`,
    selectedTemplateID: template.id,
    stepSeconds:
      template.default_step_seconds > 0 ? template.default_step_seconds : null,
    templateID: template.id,
    tool: template.tool,
    windowSeconds:
      template.default_window_seconds > 0
        ? template.default_window_seconds
        : null,
  };
}

function operatorEvidenceTemplateSummary(
  template: DiagnosisToolTemplate,
): string {
  const parts = [
    `source #${template.alert_source_profile_id}`,
    template.tool,
    `limit ${template.default_limit}`,
  ];
  if (template.tool === "metric_range_query") {
    parts.push(
      `window ${template.default_window_seconds}s`,
      `step ${template.default_step_seconds}s`,
    );
  }
  if (operatorEvidenceTemplateHasParameterizedQuery(template)) {
    parts.push("requires concrete query");
  }
  return parts.join(" / ");
}

function validateOperatorSelectedTemplate(
  values: OperatorEvidenceFormValues,
  templates: DiagnosisToolTemplate[],
) {
  if (!isPositiveSafeInteger(values.selectedTemplateID)) {
    return;
  }
  const template = templates.find(
    (candidate) => candidate.id === values.selectedTemplateID,
  );
  if (template === undefined) {
    throw new Error("Selected template is no longer available.");
  }
  if (values.templateID !== template.id) {
    throw new Error("Template ID must match the selected template.");
  }
  if (values.tool !== template.tool) {
    throw new Error("Tool must match the selected template.");
  }
  if (
    values.alertSourceProfileID !== undefined &&
    values.alertSourceProfileID !== null &&
    values.alertSourceProfileID !== template.alert_source_profile_id
  ) {
    throw new Error("Alert source profile must match the selected template.");
  }
  if (
    operatorEvidenceTemplateHasParameterizedQuery(template) &&
    (values.query === undefined || values.query.trim() === "")
  ) {
    throw new Error("Concrete query is required for the selected template.");
  }
  if (
    template.tool !== "active_alerts" &&
    !operatorEvidenceTemplateHasParameterizedQuery(template)
  ) {
    const query = values.query?.trim() ?? "";
    const templateQuery = template.query_template.trim();
    if (query !== "" && query !== templateQuery) {
      throw new Error(
        "Query must match the selected template. Clear the selected template to use a custom query.",
      );
    }
  }
}

function operatorEvidenceRequest(
  values: OperatorEvidenceFormValues,
): DiagnosisEvidenceRequest {
  const tool = values.tool;
  if (!isOperatorEvidenceTool(tool)) {
    throw new Error("Tool is unsupported.");
  }
  const reason = operatorEvidenceRequiredLine(values.reason, "Reason", 500);
  const templateID = optionalOperatorEvidenceInteger(
    values.templateID,
    "Template ID",
  );
  const alertSourceProfileID = optionalOperatorEvidenceInteger(
    values.alertSourceProfileID,
    "Alert source profile",
  );
  const query = optionalOperatorEvidenceLine(values.query, "Query", 500);
  const limit = optionalOperatorEvidenceInteger(values.limit, "Limit");
  const request: DiagnosisEvidenceRequest = { reason, tool };
  if (templateID !== undefined) {
    request.template_id = templateID;
  }
  if (alertSourceProfileID !== undefined) {
    request.alert_source_profile_id = alertSourceProfileID;
  }
  if (limit !== undefined) {
    request.limit = limit;
  }

  if (tool === "active_alerts") {
    if (query !== undefined) {
      throw new Error("active_alerts does not accept a query.");
    }
    return request;
  }

  if (query !== undefined) {
    request.query = query;
  }
  if (templateID === undefined && query === undefined) {
    throw new Error("Metric evidence requires a query or template ID.");
  }
  if (tool === "metric_query") {
    return request;
  }

  const windowSeconds = optionalOperatorEvidenceInteger(
    values.windowSeconds,
    "Window seconds",
  );
  const stepSeconds = optionalOperatorEvidenceInteger(
    values.stepSeconds,
    "Step seconds",
  );
  if (
    templateID === undefined &&
    (windowSeconds === undefined || stepSeconds === undefined)
  ) {
    throw new Error(
      "Range metric evidence requires window and step without a template ID.",
    );
  }
  if (windowSeconds !== undefined || stepSeconds !== undefined) {
    if (windowSeconds === undefined || stepSeconds === undefined) {
      throw new Error("Window and step must be set together.");
    }
    validateOperatorEvidenceRange(windowSeconds, stepSeconds);
    request.window_seconds = windowSeconds;
    request.step_seconds = stepSeconds;
  }
  return request;
}

function formatCollectionProgressSummary(
  items: DiagnosisEvidenceCollectionResult[],
): string {
  const collected = items.filter((item) => item.status === "collected").length;
  const failed = items.filter((item) => item.status === "failed").length;
  const skipped = items.filter((item) => item.status === "skipped").length;
  const unsupported = items.filter(
    (item) => item.status === "unsupported",
  ).length;
  const details = [`${collected}/${items.length} collected`];
  if (failed > 0) {
    details.push(`${failed} failed`);
  }
  if (skipped > 0) {
    details.push(`${skipped} skipped`);
  }
  if (unsupported > 0) {
    details.push(`${unsupported} unsupported`);
  }
  return details.join(", ");
}

function evidenceCollectionSummary(
  items: DiagnosisEvidenceCollectionResult[],
): EvidenceCollectionSummaryStats {
  return items.reduce<EvidenceCollectionSummaryStats>(
    (summary, item) => {
      summary.total += 1;
      summary.observedAlerts += item.observed_alerts;
      summary.observedMetricSeries += item.observed_metric_series ?? 0;
      switch (item.status) {
        case "collected":
          summary.collected += 1;
          break;
        case "failed":
          summary.failed += 1;
          summary.unresolved += 1;
          break;
        case "skipped":
          summary.skipped += 1;
          summary.unresolved += 1;
          break;
        case "unsupported":
          summary.unsupported += 1;
          summary.unresolved += 1;
          break;
        default:
          summary.unresolved += 1;
          break;
      }
      return summary;
    },
    {
      collected: 0,
      failed: 0,
      observedAlerts: 0,
      observedMetricSeries: 0,
      skipped: 0,
      total: 0,
      unresolved: 0,
      unsupported: 0,
    },
  );
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

function diagnosisPageContext(
  searchParams: { get(name: string): string | null },
  alertContext?: DiagnosisAlertContext,
): DiagnosisPageContext {
  const evidenceSnapshotID = positiveIntegerSearchParam(
    searchParams,
    "evidence_snapshot_id",
  );
  const reportID = positiveIntegerSearchParam(searchParams, "report_id");
  const subReportID = positiveIntegerSearchParam(searchParams, "sub_report_id");
  const sessionID = boundedTextSearchParam(searchParams, "session_id", 128);
  const authError = diagnosisRoomWeComAuthErrorSearchParam(searchParams);
  const oidcAuthError = diagnosisRoomOIDCAuthErrorSearchParam(searchParams);
  const authMode = diagnosisAuthModeSearchParam(searchParams);
  const weComAutoLogin = diagnosisRoomWeComAutoLoginSearchParam(searchParams);
  const weComLaunchContext =
    diagnosisRoomWeComLaunchContextSearchParam(searchParams);
  const intent =
    searchParams.get("intent") === "review_conclusion"
      ? "review_conclusion"
      : "confidence_review";
  const supplementalFollowUp = supplementalFollowUpSearchParam(searchParams);
  const evidencePlan = diagnosisEvidencePlanSearchParam(searchParams);

  if (
    evidenceSnapshotID === undefined &&
    reportID === undefined &&
    sessionID === undefined
  ) {
    return {
      authError,
      authMode,
      description: "",
      hasContext: false,
      oidcAuthError,
      suggestedPrompt: "",
      title: "",
      weComAutoLogin,
      weComLaunchContext,
    };
  }

  if (evidenceSnapshotID === undefined && reportID === undefined) {
    return {
      authError,
      authMode,
      description: `Loaded diagnosis session ${sessionID}. Connect when operator review is ready.`,
      hasContext: true,
      oidcAuthError,
      sessionID,
      suggestedPrompt: "",
      title: "Existing diagnosis room",
      weComAutoLogin,
      weComLaunchContext,
    };
  }

  const hasReportRef = reportID !== undefined;
  const reportRef = hasReportRef ? `report #${reportID}` : "";
  const snapshotRef =
    evidenceSnapshotID !== undefined
      ? `evidence snapshot #${evidenceSnapshotID}`
      : "the linked evidence snapshot";
  const subReportRef =
    subReportID !== undefined ? `, subreport #${subReportID}` : "";
  const contextRef = hasReportRef
    ? `${snapshotRef} for ${reportRef}`
    : snapshotRef;
  const description = diagnosisContextDescription({
    alertContext,
    hasReportRef,
    reportRef,
    sessionID,
    snapshotRef,
    subReportRef,
  });
  const suggestedPrompt = diagnosisSuggestedPrompt({
    alertContext,
    contextRef,
    intent,
  });

  return {
    authError,
    authMode,
    backHref: diagnosisReportReturnHref(
      reportID !== undefined ? `/reports/${reportID}` : undefined,
    ),
    description,
    evidenceSnapshotID,
    evidencePlan,
    hasContext: true,
    oidcAuthError,
    sessionID,
    suggestedPrompt,
    supplementalFollowUp,
    title:
      reportID !== undefined
        ? `Report #${reportID} diagnosis`
        : alertContext
          ? "Alert diagnosis"
          : "Evidence snapshot diagnosis",
    weComAutoLogin,
    weComLaunchContext,
  };
}

function diagnosisWeComAuthErrorDetail(error: DiagnosisRoomWeComAuthError): {
  description: string;
  message: string;
} {
  switch (error) {
    case "wecom_auth_failed":
      return {
        description:
          "Enterprise WeChat returned to OpenClarion, but the backend could not establish a diagnosis-room identity. Retry from the intended Enterprise WeChat account or ask an administrator to check the Enterprise WeChat application configuration.",
        message: "Enterprise WeChat authentication was not accepted.",
      };
    case "wecom_role_unauthorized":
      return {
        description:
          "Enterprise WeChat identified the operator, but OpenClarion local RBAC did not grant diagnosis room access. OpenClarion cleared any previous browser session; assign the operator to the right local role, then sign in again.",
        message: "Enterprise WeChat identity is not authorized locally.",
      };
    case "wecom_callback_failed":
      return {
        description:
          "OpenClarion could not complete the Enterprise WeChat callback. Retry login; if it repeats, check the backend Enterprise WeChat application configuration and callback endpoint.",
        message: "Enterprise WeChat callback could not be completed.",
      };
    case "wecom_callback_missing":
      return {
        description:
          "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM, then retry the diagnosis-room action.",
        message: "Enterprise WeChat callback was incomplete.",
      };
    case "wecom_entry_unavailable":
      return {
        description:
          "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM, then retry the diagnosis-room action.",
        message: "Enterprise WeChat login entry was not available.",
      };
    case "wecom_login_failed":
      return {
        description:
          "OpenClarion could not start Enterprise WeChat login. Retry once; if it repeats, check the backend Enterprise WeChat app credentials, redirect URI, and provider endpoint reachability.",
        message: "Enterprise WeChat login could not be started.",
      };
  }
}

function diagnosisRoomOIDCAuthErrorSearchParam(searchParams: {
  get(name: string): string | null;
}): DiagnosisRoomOIDCAuthError | undefined {
  switch (searchParams.get("oidc_auth_error")) {
    case "oidc_auth_failed":
    case "oidc_callback_failed":
    case "oidc_callback_missing":
    case "oidc_login_failed":
    case "oidc_not_configured":
    case "oidc_role_unauthorized":
      return searchParams.get("oidc_auth_error") as DiagnosisRoomOIDCAuthError;
    default:
      return undefined;
  }
}

function diagnosisOIDCAuthErrorDetail(error: DiagnosisRoomOIDCAuthError): {
  description: string;
  message: string;
} {
  switch (error) {
    case "oidc_auth_failed":
      return {
        description:
          "IAM returned to OpenClarion, but the backend did not accept the identity token. Retry sign-in or ask an administrator to check the OIDC client, issuer, audience, and approved claims.",
        message: "IAM authentication was not accepted.",
      };
    case "oidc_role_unauthorized":
      return {
        description:
          "IAM identified the operator, but OpenClarion local RBAC did not grant diagnosis room access. Assign the operator to the right local role, then sign in again.",
        message: "IAM identity is not authorized locally.",
      };
    case "oidc_callback_failed":
      return {
        description:
          "OpenClarion could not complete the IAM callback. Retry sign-in; if it repeats, check OIDC discovery, token endpoint reachability, and redirect URI configuration.",
        message: "IAM callback could not be completed.",
      };
    case "oidc_callback_missing":
      return {
        description:
          "IAM did not return the code and state required to complete login, or the browser state cookie expired. Start IAM sign-in again from this console.",
        message: "IAM callback was incomplete.",
      };
    case "oidc_login_failed":
      return {
        description:
          "OpenClarion could not start IAM sign-in. Retry once; if it repeats, check OIDC discovery, client ID, redirect URI, and provider reachability.",
        message: "IAM sign-in could not be started.",
      };
    case "oidc_not_configured":
      return {
        description:
          "OpenClarion needs an OIDC issuer, client ID, redirect URI, and a state signing key before browser IAM sign-in can be used.",
        message: "IAM sign-in is not configured.",
      };
  }
}

function diagnosisContextDescription({
  alertContext,
  hasReportRef,
  reportRef,
  sessionID,
  snapshotRef,
  subReportRef,
}: {
  alertContext?: DiagnosisAlertContext;
  hasReportRef: boolean;
  reportRef: string;
  sessionID?: string;
  snapshotRef: string;
  subReportRef: string;
}): string {
  const action = sessionID
    ? `Connect to continue diagnosis session ${sessionID}.`
    : "Create the room when operator review is ready.";
  if (hasReportRef) {
    return `Prepared from ${reportRef}${subReportRef} using ${snapshotRef}. ${action}`;
  }
  if (alertContext) {
    const alert = alertContext.alert;
    return `Prepared from ${alertName(alert)} (${alertSeverity(alert)}, ${alert.status}) using ${snapshotRef}. ${action}`;
  }
  return `Loaded ${snapshotRef}. ${action}`;
}

function diagnosisSuggestedPrompt({
  alertContext,
  contextRef,
  intent,
}: {
  alertContext?: DiagnosisAlertContext;
  contextRef: string;
  intent: "confidence_review" | "review_conclusion";
}): string {
  if (!alertContext) {
    return intent === "review_conclusion"
      ? `Review ${contextRef}. Verify the current diagnosis conclusion, identify any operator-supplied evidence that can raise confidence, and state whether the conclusion is ready to finalize.`
      : `Review ${contextRef}. First identify the operator-supplied evidence still needed, then propose the next collection steps required to improve confidence before a final conclusion.`;
  }

  const alert = alertContext.alert;
  const alertRef = `${alertName(alert)} (${alertSeverity(alert)}, ${alert.status})`;
  const summary = alertSummary(alert);
  const source = alert.source;
  const sourceProfile =
    alert.alert_source_profile_id > 0
      ? `profile #${alert.alert_source_profile_id}`
      : "legacy provider-only profile";
  if (intent === "review_conclusion") {
    return `Review ${alertRef} using ${contextRef}. Summary: ${summary}. Source: ${source} (${sourceProfile}). Verify the current diagnosis conclusion, identify any operator-supplied evidence that can raise confidence, and state whether the conclusion is ready to finalize.`;
  }
  return `Start an AI diagnosis for ${alertRef} using ${contextRef}. Summary: ${summary}. Source: ${source} (${sourceProfile}). First explain the likely service impact, then list missing operator-supplied evidence and next collection steps before any final conclusion.`;
}

function diagnosisAuthModeSearchParam(searchParams: {
  get(name: string): string | null;
}): DiagnosisAuthMode | undefined {
  switch (searchParams.get("auth_mode")) {
    case "session":
    case "wecom":
      return "session";
    default:
      return undefined;
  }
}

function supplementalFollowUpSearchParam(searchParams: {
  get(name: string): string | null;
}): DiagnosisConsultationEvidenceRequest | undefined {
  const label = boundedTextSearchParam(searchParams, "follow_up_label", 120);
  const detail = boundedTextSearchParam(searchParams, "follow_up_detail", 1000);
  const priority = boundedTextSearchParam(
    searchParams,
    "follow_up_priority",
    40,
  );
  const normalizedPriority =
    priority === undefined
      ? undefined
      : supplementalEvidencePriorityFromText(priority);
  if (
    label === undefined ||
    detail === undefined ||
    normalizedPriority === undefined
  ) {
    return undefined;
  }
  return { detail, label, priority: normalizedPriority };
}

function boundedTextSearchParam(
  searchParams: { get(name: string): string | null },
  name: string,
  maxLength: number,
): string | undefined {
  return boundedURLTextValue(searchParams.get(name), maxLength);
}

function positiveIntegerSearchParam(
  searchParams: { get(name: string): string | null },
  name: string,
): number | undefined {
  const raw = searchParams.get(name);
  if (raw === null) {
    return undefined;
  }
  const parsed = Number(raw);
  return isPositiveSafeInteger(parsed) ? parsed : undefined;
}

function isOperatorEvidenceTool(value: unknown): value is OperatorEvidenceTool {
  return (
    value === "active_alerts" ||
    value === "metric_query" ||
    value === "metric_range_query"
  );
}

function optionalOperatorEvidenceInteger(
  value: unknown,
  label: string,
): number | undefined {
  if (value === undefined || value === null || value === "") {
    return undefined;
  }
  if (!isPositiveSafeInteger(value)) {
    throw new Error(`${label} must be a positive integer.`);
  }
  return value;
}

function operatorEvidenceRequiredLine(
  value: unknown,
  label: string,
  maxBytes: number,
): string {
  const line = optionalOperatorEvidenceLine(value, label, maxBytes);
  if (line === undefined) {
    throw new Error(`${label} is required.`);
  }
  return line;
}

function optionalOperatorEvidenceLine(
  value: unknown,
  label: string,
  maxBytes: number,
): string | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  if (typeof value !== "string") {
    throw new Error(`${label} must be text.`);
  }
  const trimmed = value.trim();
  if (trimmed === "") {
    return undefined;
  }
  if (/[\r\n\t]/.test(trimmed)) {
    throw new Error(`${label} must be a single line.`);
  }
  if (new TextEncoder().encode(trimmed).length > maxBytes) {
    throw new Error(`${label} is too long.`);
  }
  return trimmed;
}

function validateOperatorEvidenceRange(
  windowSeconds: number,
  stepSeconds: number,
) {
  if (windowSeconds < 15 || windowSeconds > 21_600) {
    throw new Error("Window seconds must be between 15 and 21600.");
  }
  if (stepSeconds < 15 || stepSeconds > 21_600) {
    throw new Error("Step seconds must be between 15 and 21600.");
  }
  if (stepSeconds > windowSeconds) {
    throw new Error("Step seconds must not exceed window seconds.");
  }
}

function operatorEvidenceSingleLine(
  value: unknown,
  label: string,
  maxBytes: number,
): Promise<void> {
  try {
    operatorEvidenceRequiredLine(value, label, maxBytes);
    return Promise.resolve();
  } catch (error) {
    return Promise.reject(error);
  }
}

function operatorEvidenceOptionalSingleLine(
  value: unknown,
  label: string,
  maxBytes: number,
): Promise<void> {
  try {
    optionalOperatorEvidenceLine(value, label, maxBytes);
    return Promise.resolve();
  } catch (error) {
    return Promise.reject(error);
  }
}

function operatorEvidenceOptionalInteger(
  value: unknown,
  label: string,
): Promise<void> {
  try {
    optionalOperatorEvidenceInteger(value, label);
    return Promise.resolve();
  } catch (error) {
    return Promise.reject(error);
  }
}

function operatorEvidenceRangeSeconds(
  value: unknown,
  peerValue: unknown,
  templateID: unknown,
  label: string,
): Promise<void> {
  try {
    const current = optionalOperatorEvidenceInteger(value, label);
    const peer = optionalOperatorEvidenceInteger(
      peerValue,
      label === "Window seconds" ? "Step seconds" : "Window seconds",
    );
    const hasTemplateID =
      optionalOperatorEvidenceInteger(templateID, "Template ID") !== undefined;
    if (!hasTemplateID && current === undefined) {
      throw new Error(`${label} is required without a template ID.`);
    }
    if (hasTemplateID && current === undefined && peer !== undefined) {
      throw new Error(`${label} must be set with its paired range field.`);
    }
    if (current !== undefined && peer !== undefined) {
      if (label === "Window seconds") {
        validateOperatorEvidenceRange(current, peer);
      } else {
        validateOperatorEvidenceRange(peer, current);
      }
    }
    return Promise.resolve();
  } catch (error) {
    return Promise.reject(error);
  }
}

function isPositiveSafeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
