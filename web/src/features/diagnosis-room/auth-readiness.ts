import type { useTranslations } from "next-intl";

import { containsControlOrWhitespace } from "../settings/validation";
import type { DiagnosisAuthStatus } from "./transport";
import type { DiagnosisApprovalMode } from "./types";

export type DiagnosisAuthTranslator = ReturnType<
  typeof useTranslations<"DiagnosisAuth">
>;

export type DiagnosisAuthMode = "ldap" | "bearer" | "session" | "wecom";
export type DiagnosisAuthBackendMode =
  | "ldap"
  | "none"
  | "oidc"
  | "static"
  | "wecom"
  | "unknown";

export type DiagnosisAuthInputValues = {
  authMode?: DiagnosisAuthMode;
  bearerToken?: string;
  ldapPassword?: string;
  ldapUsername?: string;
};

export type DiagnosisAuthInputReadiness = {
  detail: string;
  label: string;
  mode: DiagnosisAuthMode;
  status: "ready" | "pending" | "blocked";
};

type DiagnosisAuthInputIssue =
  | "bearer_invalid"
  | "bearer_required"
  | "ldap_password_invalid"
  | "ldap_required"
  | "ldap_username_invalid"
  | "wecom_migrated";

type DiagnosisAuthInputState = Pick<
  DiagnosisAuthInputReadiness,
  "mode" | "status"
> & {
  issue: DiagnosisAuthInputIssue | null;
};

export type DiagnosisAuthBackendCheck = {
  checkedAt?: string;
  inputRevision?: number;
  message: string;
  mode: DiagnosisAuthMode;
  roleAuthorized?: boolean;
  roles: string[];
  status: "success" | "failed";
  subject: string;
};

export type DiagnosisAuthBackendReadiness = {
  color: "success" | "processing" | "warning" | "error" | "default";
  detail: string;
  label: string;
  status:
    | "verified"
    | "checking"
    | "needs_check"
    | "failed"
    | "pending"
    | "blocked";
};

export type DiagnosisAuthRolloutProof = {
  checkedAt?: string;
  detail: string;
  mode: DiagnosisAuthMode;
  roleAuthorized?: boolean;
  roles: string[];
  status: DiagnosisAuthBackendReadiness["status"];
  subject: string;
};

export type DiagnosisAuthRolloutReadiness = {
  checkedAt?: string;
  detail: string;
  label: string;
  mode: DiagnosisAuthMode;
  roleAuthorized?: boolean;
  roles: string[];
  status: "ready" | "review" | "pending" | "blocked";
  subject: string;
};

export type DiagnosisAuthAction = "connect" | "create";

export type DiagnosisAutoBrowserSessionAuthCheckContext =
  | "connection"
  | "create";

export type DiagnosisAutoBrowserSessionAuthCheckPlan = {
  attemptKey: string;
  context: DiagnosisAutoBrowserSessionAuthCheckContext;
  inputRevision: number;
  values: DiagnosisAuthInputValues;
};

export type DiagnosisAutoBrowserSessionConnectionPlan = {
  attemptKey: string;
  sessionID: string;
};

export type DiagnosisAutoBrowserSessionCreateRoomPlan = {
  attemptKey: string;
  closeNotificationChannelProfileID?: number;
  evidenceSnapshotID: number;
};

export type DiagnosisAutoBrowserSessionConnectionStatus =
  | "idle"
  | "ticketing"
  | "connecting"
  | "connected"
  | "closed"
  | "error";

export type DiagnosisAuthBackendStatusSnapshot = {
  configured: boolean;
  mode: DiagnosisAuthBackendMode;
  supportedModes?: DiagnosisAuthBackendMode[];
};

export type DiagnosisAuthBrowserSessionIntent = "action" | "check";

export type DiagnosisAuthWeComSetupReadinessItem = {
  detail: string;
  key: "backend" | "callback" | "identity_checks" | "role_mapping";
  label: string;
  status: "ready" | "review" | "blocked" | "loading" | "unavailable";
};

export type DiagnosisAuthWeComSetupReadiness = {
  color: "success" | "processing" | "warning" | "error" | "default";
  detail: string;
  items: DiagnosisAuthWeComSetupReadinessItem[];
  label: string;
  status: "ready" | "review" | "blocked" | "loading" | "unavailable";
};

type DiagnosisAuthLDAPSetupReadinessItem = {
  detail: string;
  key: "backend" | "transport_policy" | "role_mapping";
  label: string;
  status: "ready" | "review" | "blocked" | "loading" | "unavailable";
};

export type DiagnosisAuthLDAPSetupReadiness = {
  color: "success" | "processing" | "warning" | "error" | "default";
  detail: string;
  items: DiagnosisAuthLDAPSetupReadinessItem[];
  label: string;
  status: "ready" | "review" | "blocked" | "loading" | "unavailable";
};

export type DiagnosisAuthBrowserSessionAuthenticatedSummary = {
  alertType: "success" | "warning";
  detail: string;
};

export type DiagnosisAuthBrowserSessionDisplaySummary = {
  active: boolean;
  alertType: "success" | "info" | "warning";
  detail: string;
};

export type DiagnosisAuthBrowserSessionErrorAuthMode =
  | "basic"
  | "bearer"
  | "session";

export type DiagnosisAuthLDAPBrowserSessionPromotionNotice = {
  detail: string;
  message: string;
};

type DiagnosisAuthRoleMappingStatusSummary = {
  admin_mapping_count: number;
  configured: boolean;
  default_roles: Array<"admin" | "owner">;
  owner_mapping_count: number;
};

type DiagnosisAuthTransportPolicyStatusSummary = {
  security: "tls" | "start_tls" | "insecure_plaintext";
};

export type DiagnosisAuthStatusSummary = {
  configured: boolean;
  mode: DiagnosisAuthBackendMode;
  role_mapping?: DiagnosisAuthRoleMappingStatusSummary;
  supported_modes?: DiagnosisAuthBackendMode[];
  transport_policy?: DiagnosisAuthTransportPolicyStatusSummary;
};

type DiagnosisAuthBackendStatusWithModes = {
  configured: boolean;
  mode: DiagnosisAuthBackendMode;
  supportedModes?: readonly DiagnosisAuthBackendMode[];
  supported_modes?: readonly DiagnosisAuthBackendMode[];
};

export type DiagnosisAuthRoleMappingStatusReadiness = {
  color: "success" | "processing" | "warning" | "error" | "default";
  detail: string;
  label: string;
  status: "ready" | "loading" | "unavailable" | "blocked";
};

export type DiagnosisAuthCheckSuccessFeedback = {
  logLevel: "error" | "info";
  logMessage: string;
  toastMessage: string;
  toastType: "success" | "warning";
};

export type DiagnosisAuthModeOption = {
  disabled: boolean;
  label: string;
  value: DiagnosisAuthMode;
};

export type DiagnosisAuthBackendModeDisplayItem = {
  color: string;
  label: string;
  mode: DiagnosisAuthBackendMode;
};

type DiagnosisAuthOIDCBFFReadiness = NonNullable<
  DiagnosisAuthStatus["oidc_bff"]
>;
export type DiagnosisAuthOIDCBFFMissingKey =
  DiagnosisAuthOIDCBFFReadiness["missing"][number];
export type DiagnosisAuthOIDCBFFReadinessSummary = Pick<
  DiagnosisAuthOIDCBFFReadiness,
  "missing" | "status"
>;

export function diagnosisAuthInputReadiness(
  values: DiagnosisAuthInputValues,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthInputReadiness {
  const state = diagnosisAuthInputState(values);
  const copy = diagnosisAuthInputCopy(state, t);
  return { ...copy, mode: state.mode, status: state.status };
}

export function diagnosisAuthLDAPBrowserSessionPromotionNotice(
  t: DiagnosisAuthTranslator,
): DiagnosisAuthLDAPBrowserSessionPromotionNotice {
  return {
    detail: t("ldapPromotion.detail"),
    message: t("ldapPromotion.message"),
  };
}

function diagnosisAuthInputState(
  values: DiagnosisAuthInputValues,
): DiagnosisAuthInputState {
  const mode = values.authMode ?? "session";
  if (mode === "session") {
    return { issue: null, mode, status: "ready" };
  }
  if (mode === "wecom") {
    return { issue: "wecom_migrated", mode, status: "blocked" };
  }
  if (mode === "bearer") {
    const token = (values.bearerToken ?? "").trim();
    if (token === "") {
      return { issue: "bearer_required", mode, status: "pending" };
    }
    if (/\s/.test(token)) {
      return { issue: "bearer_invalid", mode, status: "blocked" };
    }
    return { issue: null, mode, status: "ready" };
  }

  const username = (values.ldapUsername ?? "").trim();
  const password = values.ldapPassword ?? "";
  if (username === "" || password === "") {
    return { issue: "ldap_required", mode, status: "pending" };
  }
  if (containsControlOrWhitespace(username)) {
    return { issue: "ldap_username_invalid", mode, status: "blocked" };
  }
  if (/[\u0000\r\n]/.test(password)) {
    return { issue: "ldap_password_invalid", mode, status: "blocked" };
  }
  return { issue: null, mode, status: "ready" };
}

function diagnosisAuthInputCopy(
  state: DiagnosisAuthInputState,
  t: DiagnosisAuthTranslator,
): Pick<DiagnosisAuthInputReadiness, "detail" | "label"> {
  switch (state.issue) {
    case "bearer_invalid":
      return {
        detail: t("input.bearerInvalidDetail"),
        label: t("input.bearerInvalidLabel"),
      };
    case "bearer_required":
      return {
        detail: t("input.bearerRequiredDetail"),
        label: t("input.bearerRequiredLabel"),
      };
    case "ldap_password_invalid":
      return {
        detail: t("input.ldapPasswordInvalidDetail"),
        label: t("input.ldapPasswordInvalidLabel"),
      };
    case "ldap_required":
      return {
        detail: t("input.ldapRequiredDetail"),
        label: t("input.ldapRequiredLabel"),
      };
    case "ldap_username_invalid":
      return {
        detail: t("input.ldapUsernameInvalidDetail"),
        label: t("input.ldapUsernameInvalidLabel"),
      };
    case "wecom_migrated":
      return {
        detail: t("input.weComMigratedDetail"),
        label: t("input.weComMigratedLabel"),
      };
    case null:
      switch (state.mode) {
        case "bearer":
          return {
            detail: t("input.bearerReadyDetail"),
            label: t("input.bearerReadyLabel"),
          };
        case "ldap":
          return {
            detail: t("input.ldapReadyDetail"),
            label: t("input.ldapReadyLabel"),
          };
        case "session":
          return {
            detail: t("input.sessionReadyDetail"),
            label: t("input.sessionReadyLabel"),
          };
        case "wecom":
          throw new Error("migrated WeCom input must carry an issue");
      }
  }
}

export function diagnosisAuthModeOptions(
  backendStatus: DiagnosisAuthBackendStatusSnapshot | undefined,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthModeOption[] {
  return [
    {
      disabled:
        diagnosisAuthBackendModeIssue("session", backendStatus) !== null,
      label: t("mode.session"),
      value: "session",
    },
    {
      disabled: diagnosisAuthBackendModeIssue("ldap", backendStatus) !== null,
      label: t("mode.ldap"),
      value: "ldap",
    },
    {
      disabled: diagnosisAuthBackendModeIssue("bearer", backendStatus) !== null,
      label: t("mode.bearer"),
      value: "bearer",
    },
  ];
}

export function diagnosisAuthCoercedMode(
  mode: DiagnosisAuthMode,
  backendStatus?: DiagnosisAuthBackendStatusSnapshot,
): DiagnosisAuthMode {
  if (diagnosisAuthBackendModeIssue(mode, backendStatus) === null) {
    return mode;
  }
  const fallback = (["session", "ldap", "bearer"] as const).find(
    (candidate) =>
      diagnosisAuthBackendModeIssue(candidate, backendStatus) === null,
  );
  return fallback ?? mode;
}

export function diagnosisAuthBackendStatusModes(
  status: DiagnosisAuthBackendStatusWithModes | null | undefined,
): DiagnosisAuthBackendMode[] {
  if (
    status === null ||
    status === undefined ||
    !status.configured ||
    status.mode === "none"
  ) {
    return [];
  }
  const supportedModes =
    status.supportedModes === undefined || status.supportedModes.length === 0
      ? status.supported_modes
      : status.supportedModes;
  const modes =
    supportedModes === undefined || supportedModes.length === 0
      ? [status.mode]
      : supportedModes;
  const out: DiagnosisAuthBackendMode[] = [];
  modes.forEach((mode) => {
    if (mode !== "none" && !out.includes(mode)) {
      out.push(mode);
    }
  });
  return out;
}

export function diagnosisAuthBackendModeListLabel(
  status: DiagnosisAuthBackendStatusWithModes | null | undefined,
  t: DiagnosisAuthTranslator,
): string {
  const labels = diagnosisAuthBackendStatusModes(status).map((mode) =>
    diagnosisAuthBackendShortModeLabel(mode, t),
  );
  return diagnosisAuthListLabel(labels, t, "+");
}

export function diagnosisAuthBackendCredentialListLabel(
  status: DiagnosisAuthBackendStatusWithModes | null | undefined,
  t: DiagnosisAuthTranslator,
): string {
  if (status === null || status === undefined) {
    return "";
  }
  return diagnosisAuthBackendCredentialLabels(
    diagnosisAuthBackendStatusModes(status),
    status.mode,
    t,
  );
}

export function diagnosisAuthBackendModeDisplayItems(
  status: DiagnosisAuthBackendStatusWithModes | null | undefined,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthBackendModeDisplayItem[] {
  return diagnosisAuthBackendStatusModes(status).map((mode) => ({
    color: diagnosisAuthBackendModeTagColor(mode),
    label: diagnosisAuthBackendShortModeLabel(mode, t),
    mode,
  }));
}

export function diagnosisAuthBackendReadinessStatusLabel(
  status: DiagnosisAuthBackendReadiness["status"],
  t: DiagnosisAuthTranslator,
): string {
  switch (status) {
    case "pending":
      return t("ui.backendStatus.pending");
    case "blocked":
      return t("ui.backendStatus.blocked");
    case "checking":
      return t("ui.backendStatus.checking");
    case "needs_check":
      return t("ui.backendStatus.needsCheck");
    case "failed":
      return t("ui.backendStatus.failed");
    case "verified":
      return t("ui.backendStatus.verified");
  }
}

export function diagnosisAuthInputReadinessStatusLabel(
  status: DiagnosisAuthInputReadiness["status"],
  t: DiagnosisAuthTranslator,
): string {
  switch (status) {
    case "ready":
      return t("ui.ready");
    case "pending":
      return t("ui.pending");
    case "blocked":
      return t("ui.blocked");
  }
}

export function diagnosisAuthOIDCBFFReadinessDetail(
  readiness: DiagnosisAuthOIDCBFFReadinessSummary,
  t: DiagnosisAuthTranslator,
): string {
  if (readiness.status === "ready") {
    return t("oidc.ready");
  }
  const labels = readiness.missing.map((key) =>
    diagnosisAuthOIDCBFFMissingLabel(key, t),
  );
  return t("oidc.missing", { items: labels.join(t("list.separator")) });
}

export function diagnosisAuthOIDCBFFMissingLabel(
  key: DiagnosisAuthOIDCBFFMissingKey,
  t: DiagnosisAuthTranslator,
): string {
  switch (key) {
    case "client_auth_method":
      return t("oidc.clientAuthMethod");
    case "client_id":
      return t("oidc.clientID");
    case "client_secret":
      return t("oidc.clientSecret");
    case "email_scope":
      return t("oidc.emailScope");
    case "issuer":
      return t("oidc.issuer");
    case "openid_scope":
      return t("oidc.openIDScope");
    case "pkce":
      return t("oidc.pkce");
    case "profile_scope":
      return t("oidc.profileScope");
    case "session_signing_key":
      return t("oidc.sessionSigningKey");
    case "state_signing_key":
      return t("oidc.stateSigningKey");
  }
}

export function diagnosisAuthBackendReadiness(
  {
    backendStatus,
    checking,
    expectedSubject,
    inputRevision,
    lastCheck,
    values,
  }: {
    backendStatus?: DiagnosisAuthBackendStatusSnapshot;
    checking: boolean;
    expectedSubject?: string;
    inputRevision?: number;
    lastCheck: DiagnosisAuthBackendCheck | null;
    values: DiagnosisAuthInputValues;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthBackendReadiness {
  const input = diagnosisAuthInputReadiness(values, t);
  if (input.status === "pending") {
    return {
      color: "default",
      detail: input.detail,
      label: input.label,
      status: "pending",
    };
  }
  if (input.status === "blocked") {
    return {
      color: "error",
      detail: input.detail,
      label: input.label,
      status: "blocked",
    };
  }
  const backendModeBlockReason = diagnosisAuthBackendModeBlockReason(
    input.mode,
    t,
    backendStatus,
  );
  if (backendModeBlockReason !== "") {
    return {
      color: "error",
      detail: backendModeBlockReason,
      label: t("backend.modeMismatchLabel"),
      status: "blocked",
    };
  }
  if (checking) {
    return {
      color: "processing",
      detail: t("backend.checkingDetail"),
      label: t("backend.checkingLabel"),
      status: "checking",
    };
  }
  if (
    lastCheck === null ||
    lastCheck.mode !== input.mode ||
    authCheckInputRevisionChanged(lastCheck, inputRevision)
  ) {
    return {
      color: "warning",
      detail: diagnosisAuthBackendCheckRequiredDetail(input.mode, t),
      label: t("backend.checkRequiredLabel"),
      status: "needs_check",
    };
  }
  if (!authCheckMatchesExpectedSubject(input.mode, lastCheck, expectedSubject)) {
    return {
      color: "warning",
      detail: diagnosisAuthBackendCheckSubjectChangedDetail(
        lastCheck,
        expectedSubject,
        t,
      ),
      label: t("backend.checkRequiredLabel"),
      status: "needs_check",
    };
  }
  if (lastCheck.status === "failed") {
    return {
      color: "error",
      detail: lastCheck.message || t("backend.checkFailedDetail"),
      label: t("backend.checkFailedLabel"),
      status: "failed",
    };
  }
  return {
    color: "success",
    detail: diagnosisAuthVerifiedDetail(lastCheck, t),
    label: t("backend.verifiedLabel"),
    status: "verified",
  };
}

export function diagnosisAuthBackendVerified({
  backendStatus,
  checking,
  expectedSubject,
  inputRevision,
  lastCheck,
  values,
}: {
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  checking: boolean;
  expectedSubject?: string;
  inputRevision?: number;
  lastCheck: DiagnosisAuthBackendCheck | null;
  values: DiagnosisAuthInputValues;
}): boolean {
  const input = diagnosisAuthInputState(values);
  return (
    input.status === "ready" &&
    diagnosisAuthBackendModeIssue(input.mode, backendStatus) === null &&
    !checking &&
    lastCheck !== null &&
    lastCheck.mode === input.mode &&
    !authCheckInputRevisionChanged(lastCheck, inputRevision) &&
    authCheckMatchesExpectedSubject(input.mode, lastCheck, expectedSubject) &&
    lastCheck.status === "success"
  );
}

export function diagnosisAutoBrowserSessionAuthCheckPlan({
  authenticatedSubject,
  backendStatus,
  checking,
  connectionDisabledReason,
  connectionLastCheck,
  connectionValues,
  createDisabledReason,
  createLastCheck,
  createValues,
  inputRevisions,
  previousAttemptKey,
  selectedSessionID,
}: {
  authenticatedSubject?: string;
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  checking: boolean;
  connectionDisabledReason: string;
  connectionLastCheck: DiagnosisAuthBackendCheck | null;
  connectionValues: DiagnosisAuthInputValues;
  createDisabledReason: string;
  createLastCheck: DiagnosisAuthBackendCheck | null;
  createValues: DiagnosisAuthInputValues;
  inputRevisions: Record<DiagnosisAutoBrowserSessionAuthCheckContext, number>;
  previousAttemptKey: string;
  selectedSessionID: string;
}): DiagnosisAutoBrowserSessionAuthCheckPlan | null {
  const subject = authenticatedSubject?.trim() ?? "";
  if (subject === "" || checking) {
    return null;
  }
  const context: DiagnosisAutoBrowserSessionAuthCheckContext =
    selectedSessionID.trim() === "" ? "create" : "connection";
  const values = context === "connection" ? connectionValues : createValues;
  if (!diagnosisAuthModeUsesBrowserSession(values.authMode)) {
    return null;
  }
  const disabledReason =
    context === "connection" ? connectionDisabledReason : createDisabledReason;
  if (disabledReason !== "") {
    return null;
  }
  const inputRevision = inputRevisions[context];
  const lastCheck =
    context === "connection" ? connectionLastCheck : createLastCheck;
  if (
    diagnosisAuthBackendVerified({
      backendStatus,
      checking: false,
      expectedSubject: subject,
      inputRevision,
      lastCheck,
      values,
    })
  ) {
    return null;
  }
  const attemptKey = [context, subject, inputRevision].join(":");
  if (attemptKey === previousAttemptKey) {
    return null;
  }
  return {
    attemptKey,
    context,
    inputRevision,
    values,
  };
}

export function diagnosisAutoBrowserSessionConnectionPlan({
  authenticatedSubject,
  backendStatus,
  connectionDisabledReason,
  connectionStatus,
  inputRevision,
  lastCheck,
  manualDisconnected,
  previousAttemptKey,
  selectedSessionID,
  values,
}: {
  authenticatedSubject?: string;
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  connectionDisabledReason: string;
  connectionStatus: DiagnosisAutoBrowserSessionConnectionStatus;
  inputRevision: number;
  lastCheck: DiagnosisAuthBackendCheck | null;
  manualDisconnected: boolean;
  previousAttemptKey: string;
  selectedSessionID: string;
  values: DiagnosisAuthInputValues;
}): DiagnosisAutoBrowserSessionConnectionPlan | null {
  const subject = authenticatedSubject?.trim() ?? "";
  const sessionID = selectedSessionID.trim();
  if (
    subject === "" ||
    sessionID === "" ||
    !diagnosisAuthModeUsesBrowserSession(values.authMode) ||
    connectionDisabledReason !== "" ||
    manualDisconnected
  ) {
    return null;
  }
  if (connectionStatus !== "idle" && connectionStatus !== "error") {
    return null;
  }
  if (
    lastCheck?.status !== "success" ||
    lastCheck.subject !== subject ||
    !diagnosisAuthBackendVerified({
      backendStatus,
      checking: false,
      expectedSubject: subject,
      inputRevision,
      lastCheck,
      values,
    })
  ) {
    return null;
  }
  const attemptKey = ["connection", subject, sessionID, inputRevision].join(
    ":",
  );
  if (attemptKey === previousAttemptKey) {
    return null;
  }
  return { attemptKey, sessionID };
}

export function diagnosisAutoBrowserSessionCreateRoomPlan({
  approvalMode = "single",
  authenticatedSubject,
  backendStatus,
  closeNotificationChannelProfileID,
  createDisabledReason,
  evidenceSnapshotID,
  inputRevision,
  lastCheck,
  previousAttemptKey,
  selectedSessionID,
  snapshotNeedsRoom,
  values,
}: {
  approvalMode?: DiagnosisApprovalMode;
  authenticatedSubject?: string;
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  closeNotificationChannelProfileID?: number | null;
  createDisabledReason: string;
  evidenceSnapshotID?: number | null;
  inputRevision: number;
  lastCheck: DiagnosisAuthBackendCheck | null;
  previousAttemptKey: string;
  selectedSessionID: string;
  snapshotNeedsRoom: boolean;
  values: DiagnosisAuthInputValues;
}): DiagnosisAutoBrowserSessionCreateRoomPlan | null {
  const subject = authenticatedSubject?.trim() ?? "";
  if (
    subject === "" ||
    !snapshotNeedsRoom ||
    selectedSessionID.trim() !== "" ||
    !diagnosisAuthModeUsesBrowserSession(values.authMode) ||
    createDisabledReason !== "" ||
    !positiveSafeInteger(evidenceSnapshotID)
  ) {
    return null;
  }
  if (
    lastCheck?.status !== "success" ||
    lastCheck.subject !== subject ||
    !diagnosisAuthBackendVerified({
      backendStatus,
      checking: false,
      expectedSubject: subject,
      inputRevision,
      lastCheck,
      values,
    })
  ) {
    return null;
  }
  const channelID = positiveSafeInteger(closeNotificationChannelProfileID)
    ? closeNotificationChannelProfileID
    : undefined;
  const channelKey = channelID === undefined ? "none" : String(channelID);
  const attemptKey = [
    "create",
    subject,
    evidenceSnapshotID,
    channelKey,
    approvalMode,
    inputRevision,
  ].join(":");
  if (attemptKey === previousAttemptKey) {
    return null;
  }
  return {
    attemptKey,
    closeNotificationChannelProfileID: channelID,
    evidenceSnapshotID,
  };
}

function diagnosisAuthModeUsesBrowserSession(
  mode: DiagnosisAuthInputValues["authMode"],
): boolean {
  return mode === "session" || mode === "wecom";
}

function positiveSafeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && typeof value === "number" && value > 0;
}

export function diagnosisAuthRolloutReadiness(
  proof: DiagnosisAuthRolloutProof,
  t: DiagnosisAuthTranslator,
): DiagnosisAuthRolloutReadiness {
  const roleAuthorized = diagnosisAuthProofHasUsableRole(proof);
  if (proof.status === "verified" && diagnosisAuthRolloutSSOMode(proof.mode)) {
    const provider = diagnosisAuthProviderDisplayName(proof.mode, t);
    return {
      checkedAt: proof.checkedAt,
      detail: proof.detail || t("rollout.readyDetail", { provider }),
      label: t("rollout.readyLabel", { provider }),
      mode: proof.mode,
      roleAuthorized,
      roles: proof.roles,
      status: "ready",
      subject: proof.subject,
    };
  }
  if (proof.status === "verified") {
    const legacyWeComBrowserAuth = proof.mode === "wecom";
    return {
      checkedAt: proof.checkedAt,
      detail: legacyWeComBrowserAuth
        ? t("rollout.legacyWeComReviewDetail")
        : t("rollout.staticBearerReviewDetail"),
      label: t("rollout.reviewLabel"),
      mode: proof.mode,
      roleAuthorized,
      roles: proof.roles,
      status: "review",
      subject: proof.subject,
    };
  }
  if (proof.status === "failed" || proof.status === "blocked") {
    return {
      checkedAt: proof.checkedAt,
      detail: proof.detail || t("rollout.blockedDetail"),
      label: t("rollout.blockedLabel"),
      mode: proof.mode,
      roleAuthorized,
      roles: proof.roles,
      status: "blocked",
      subject: proof.subject,
    };
  }
  return {
    checkedAt: proof.checkedAt,
    detail: proof.detail || t("rollout.pendingDetail"),
    label: t("rollout.pendingLabel"),
    mode: proof.mode,
    roleAuthorized,
    roles: proof.roles,
    status: "pending",
    subject: proof.subject,
  };
}

export function diagnosisAuthRoleMappingGuidance(
  readiness: Pick<
    DiagnosisAuthRolloutReadiness,
    "mode" | "roleAuthorized" | "status"
  >,
  t: DiagnosisAuthTranslator,
): string {
  if (readiness.roleAuthorized === true) {
    switch (readiness.mode) {
      case "ldap":
        return t("roleGuidance.ldapPresent");
      case "session":
        return t("roleGuidance.sessionPresent");
      case "wecom":
        return t("roleGuidance.weComPresent");
      case "bearer":
        return t("roleGuidance.bearerPresent");
    }
  }
  switch (readiness.mode) {
    case "ldap":
      return t("roleGuidance.ldapAbsent");
    case "session":
      return t("roleGuidance.sessionAbsent");
    case "wecom":
      return t("roleGuidance.weComAbsent");
    case "bearer":
      return t("roleGuidance.bearerAbsent");
  }
}

export function diagnosisAuthRoleMappingStatusDetail(
  status: DiagnosisAuthStatusSummary | null | undefined,
  t: DiagnosisAuthTranslator,
  loading = false,
): string {
  return diagnosisAuthRoleMappingStatusReadiness(status, t, loading).detail;
}

export function diagnosisAuthRoleMappingStatusReadiness(
  status: DiagnosisAuthStatusSummary | null | undefined,
  t: DiagnosisAuthTranslator,
  loading = false,
): DiagnosisAuthRoleMappingStatusReadiness {
  if (loading) {
    return {
      color: "processing",
      detail: t("roleStatus.loadingDetail"),
      label: t("roleStatus.loadingLabel"),
      status: "loading",
    };
  }
  if (status === null || status === undefined) {
    return {
      color: "default",
      detail: t("roleStatus.unavailableDetail"),
      label: t("roleStatus.unavailableLabel"),
      status: "unavailable",
    };
  }
  if (!status.configured || status.mode === "none") {
    return {
      color: "error",
      detail: t("roleStatus.notConfiguredDetail"),
      label: t("roleStatus.notConfiguredLabel"),
      status: "blocked",
    };
  }
  const mapping = status.role_mapping;
  if (mapping === undefined) {
    return {
      color: "warning",
      detail: t("roleStatus.notReportedDetail"),
      label: t("roleStatus.notReportedLabel"),
      status: "unavailable",
    };
  }
  if (!mapping.configured) {
    const provider = diagnosisAuthProviderDisplayNameForBackend(status.mode, t);
    return {
      color: "success",
      detail: t("roleStatus.identityOnlyDetail", { provider }),
      label: t("roleStatus.identityOnlyLabel"),
      status: "ready",
    };
  }
  const defaultRoles =
    mapping.default_roles.length === 0
      ? t("common.none")
      : mapping.default_roles.join(", ");
  return {
    color: "success",
    detail: t("roleStatus.metadataDetail", {
      adminCount: mapping.admin_mapping_count,
      defaultRoles,
      ownerCount: mapping.owner_mapping_count,
      provider: diagnosisAuthProviderDisplayNameForBackend(status.mode, t),
    }),
    label: t("roleStatus.metadataLabel"),
    status: "ready",
  };
}

function diagnosisAuthRolloutSSOMode(mode: DiagnosisAuthMode): boolean {
  return mode === "ldap" || mode === "session";
}

function diagnosisAuthProviderDisplayName(
  mode: DiagnosisAuthMode,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("provider.ldap");
    case "session":
      return t("provider.session");
    case "wecom":
      return t("provider.weCom");
    case "bearer":
      return t("provider.staticBearer");
  }
}

function diagnosisAuthProviderDisplayNameForBackend(
  mode: DiagnosisAuthBackendMode,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("provider.ldap");
    case "wecom":
      return t("provider.weCom");
    case "static":
      return t("provider.staticBearer");
    case "oidc":
      return t("provider.iamOIDC");
    case "unknown":
      return t("provider.backendAuth");
    case "none":
      return t("provider.diagnosisAuth");
  }
}

export function diagnosisAuthCheckBlockReason(
  {
    backendStatus,
    values,
  }: {
    backendStatus?: DiagnosisAuthBackendStatusSnapshot;
    values: DiagnosisAuthInputValues;
  },
  t: DiagnosisAuthTranslator,
): string {
  const input = diagnosisAuthInputReadiness(values, t);
  if (input.status !== "ready") {
    return input.detail;
  }
  return diagnosisAuthBackendModeBlockReason(input.mode, t, backendStatus);
}

export function diagnosisAuthActionBlockReason(
  {
    action,
    backendStatus,
    checking,
    expectedSubject,
    inputRevision,
    lastCheck,
    values,
  }: {
    action: DiagnosisAuthAction;
    backendStatus?: DiagnosisAuthBackendStatusSnapshot;
    checking: boolean;
    expectedSubject?: string;
    inputRevision?: number;
    lastCheck: DiagnosisAuthBackendCheck | null;
    values: DiagnosisAuthInputValues;
  },
  t: DiagnosisAuthTranslator,
): string {
  const readiness = diagnosisAuthBackendReadiness(
    {
      backendStatus,
      checking,
      expectedSubject,
      inputRevision,
      lastCheck,
      values,
    },
    t,
  );
  if (readiness.status === "verified") {
    return "";
  }
  if (readiness.status === "blocked") {
    return readiness.detail;
  }
  switch (action) {
    case "connect":
      if (values.authMode === "wecom") {
        return t("action.connectWeComMigrated");
      }
      if (values.authMode === "session") {
        return t("action.connectSessionCheckRequired");
      }
      return t("action.connectCheckRequired");
    case "create":
      if (values.authMode === "wecom") {
        return t("action.createWeComMigrated");
      }
      if (values.authMode === "session") {
        return t("action.createSessionCheckRequired");
      }
      return t("action.createCheckRequired");
  }
}

export function diagnosisAuthInputFieldsChanged(
  changedValues: Record<string, unknown>,
): boolean {
  return (
    Object.prototype.hasOwnProperty.call(changedValues, "authMode") ||
    Object.prototype.hasOwnProperty.call(changedValues, "bearerToken") ||
    Object.prototype.hasOwnProperty.call(changedValues, "ldapPassword") ||
    Object.prototype.hasOwnProperty.call(changedValues, "ldapUsername")
  );
}

function diagnosisAuthVerifiedDetail(
  check: DiagnosisAuthBackendCheck,
  t: DiagnosisAuthTranslator,
): string {
  const roles =
    check.roles.length === 0 ? t("common.noRoles") : check.roles.join(", ");
  const checkedAt = check.checkedAt?.trim();
  if (checkedAt !== undefined && checkedAt !== "") {
    return t("backend.verifiedDetailWithTime", {
      checkedAt,
      roles,
      subject: check.subject,
    });
  }
  return t("backend.verifiedDetail", { roles, subject: check.subject });
}

function diagnosisAuthProofHasUsableRole({
  roleAuthorized,
  roles,
}: {
  roleAuthorized?: boolean;
  roles: readonly string[];
}): boolean {
  if (roleAuthorized !== undefined) {
    return roleAuthorized;
  }
  return roles.some((role) => role === "owner" || role === "admin");
}

function authCheckInputRevisionChanged(
  check: DiagnosisAuthBackendCheck,
  inputRevision: number | undefined,
): boolean {
  return inputRevision !== undefined && check.inputRevision !== inputRevision;
}

function authCheckMatchesExpectedSubject(
  mode: DiagnosisAuthMode,
  check: DiagnosisAuthBackendCheck,
  expectedSubject: string | undefined,
): boolean {
  if (!diagnosisAuthModeUsesBrowserSession(mode)) {
    return true;
  }
  const subject = expectedSubject?.trim() ?? "";
  if (subject === "") {
    return true;
  }
  return check.subject === subject;
}

function diagnosisAuthBackendCheckSubjectChangedDetail(
  check: DiagnosisAuthBackendCheck,
  expectedSubject: string | undefined,
  t: DiagnosisAuthTranslator,
): string {
  const subject = expectedSubject?.trim() ?? "";
  if (subject === "") {
    return t("backend.subjectChangedCurrent");
  }
  return t("backend.subjectChanged", {
    currentSubject: subject,
    previousSubject: check.subject,
  });
}

type DiagnosisAuthBackendModeIssue =
  | { kind: "not_configured" }
  | { kind: "unknown" }
  | { kind: "wecom_migrated" }
  | {
      fallback: DiagnosisAuthBackendMode;
      kind: "session_mismatch" | "bearer_mismatch";
      modes: DiagnosisAuthBackendMode[];
    }
  | { kind: "legacy_wecom" }
  | { kind: "ldap_mismatch"; mode: DiagnosisAuthBackendMode };

function diagnosisAuthBackendModeIssue(
  mode: DiagnosisAuthMode,
  backendStatus?: DiagnosisAuthBackendStatusSnapshot,
): DiagnosisAuthBackendModeIssue | null {
  if (backendStatus === undefined) {
    return null;
  }
  if (!backendStatus.configured || backendStatus.mode === "none") {
    return { kind: "not_configured" };
  }
  const supportedModes = diagnosisAuthBackendSupportedModes(backendStatus);
  if (supportedModes.length === 1 && supportedModes[0] === "unknown") {
    return { kind: "unknown" };
  }
  if (mode === "wecom") {
    return { kind: "wecom_migrated" };
  }
  if (mode === "session") {
    if (supportedModes.includes("ldap") || supportedModes.includes("oidc")) {
      return null;
    }
    return {
      fallback: backendStatus.mode,
      kind: "session_mismatch",
      modes: supportedModes,
    };
  }
  const requestedMode = diagnosisAuthBackendModeForInput(mode);
  if (supportedModes.includes(requestedMode)) {
    return null;
  }
  if (
    supportedModes.length === 1 &&
    supportedModes[0] === "wecom"
  ) {
    return { kind: "legacy_wecom" };
  }
  if (mode === "ldap") {
    return {
      kind: "ldap_mismatch",
      mode: supportedModes[0] ?? backendStatus.mode,
    };
  }
  return {
    fallback: backendStatus.mode,
    kind: "bearer_mismatch",
    modes: supportedModes,
  };
}

function diagnosisAuthBackendModeBlockReason(
  mode: DiagnosisAuthMode,
  t: DiagnosisAuthTranslator,
  backendStatus?: DiagnosisAuthBackendStatusSnapshot,
): string {
  const issue = diagnosisAuthBackendModeIssue(mode, backendStatus);
  if (issue === null) {
    return "";
  }
  switch (issue.kind) {
    case "not_configured":
      return t("backend.notConfigured");
    case "unknown":
      return t("backend.unknownMode");
    case "wecom_migrated":
      return t("input.weComMigratedDetail");
    case "session_mismatch":
      return t("backend.sessionMismatch", {
        credentials: diagnosisAuthBackendCredentialLabels(
          issue.modes,
          issue.fallback,
          t,
        ),
      });
    case "legacy_wecom":
      return t("backend.legacyWeCom");
    case "ldap_mismatch":
      return t("backend.ldapMismatch", {
        credentials: backendAuthCredentialLabel(issue.mode, t),
      });
    case "bearer_mismatch":
      return t("backend.bearerMismatch", {
        credentials: diagnosisAuthBackendCredentialLabels(
          issue.modes,
          issue.fallback,
          t,
        ),
      });
  }
}

function backendAuthCredentialLabel(
  mode: DiagnosisAuthBackendMode,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("credential.ldap");
    case "static":
      return t("credential.staticBearer");
    case "oidc":
      return t("credential.iamOIDC");
    case "wecom":
      return t("credential.weCom");
    case "none":
      return t("credential.none");
    case "unknown":
      return t("credential.unknown");
  }
}

function diagnosisAuthBackendModeForInput(
  mode: Exclude<DiagnosisAuthMode, "session">,
): DiagnosisAuthBackendMode {
  if (mode === "bearer") {
    return "static";
  }
  return mode;
}

function diagnosisAuthBackendSupportedModes(
  backendStatus: DiagnosisAuthBackendStatusSnapshot,
): DiagnosisAuthBackendMode[] {
  return diagnosisAuthBackendStatusModes(backendStatus);
}

function diagnosisAuthBackendCredentialLabels(
  modes: DiagnosisAuthBackendMode[],
  fallback: DiagnosisAuthBackendMode,
  t: DiagnosisAuthTranslator,
): string {
  const labels = (modes.length === 0 ? [fallback] : modes)
    .filter((mode) => mode !== "none")
    .map((mode) => backendAuthCredentialLabel(mode, t));
  if (labels.length === 0) {
    return backendAuthCredentialLabel(fallback, t);
  }
  if (labels.length === 1) {
    return labels[0] ?? backendAuthCredentialLabel(fallback, t);
  }
  const lastLabel = labels.at(-1) ?? backendAuthCredentialLabel(fallback, t);
  return t("list.or", {
    items: labels.slice(0, -1).join(t("list.separator")),
    last: lastLabel,
  });
}

function diagnosisAuthBackendShortModeLabel(
  mode: DiagnosisAuthBackendMode,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode) {
    case "ldap":
      return t("backendMode.ldap");
    case "static":
      return t("backendMode.static");
    case "oidc":
      return t("backendMode.oidc");
    case "wecom":
      return t("backendMode.weCom");
    case "unknown":
      return t("backendMode.unknown");
    case "none":
      return t("backendMode.notConfigured");
  }
}

function diagnosisAuthBackendModeTagColor(mode: DiagnosisAuthBackendMode): string {
  switch (mode) {
    case "ldap":
      return "blue";
    case "static":
      return "default";
    case "oidc":
      return "gold";
    case "wecom":
      return "green";
    case "unknown":
      return "default";
    case "none":
      return "red";
  }
}

function diagnosisAuthListLabel(
  labels: string[],
  t: DiagnosisAuthTranslator,
  separator: "and" | "+" = "and",
): string {
  if (labels.length === 0) {
    return "";
  }
  if (separator === "+") {
    return labels.join(" + ");
  }
  if (labels.length === 1) {
    return labels[0] ?? "";
  }
  return t("list.and", {
    items: labels.slice(0, -1).join(t("list.separator")),
    last: labels[labels.length - 1] ?? "",
  });
}

export function diagnosisAuthLDAPSetupReadiness(
  status: DiagnosisAuthStatusSummary | null | undefined,
  t: DiagnosisAuthTranslator,
  loading = false,
): DiagnosisAuthLDAPSetupReadiness {
  if (loading) {
    return {
      color: "processing",
      detail: t("ldapSetup.loadingDetail"),
      items: [
        {
          detail: t("setup.backendLoadingDetail"),
          key: "backend",
          label: t("ldapSetup.backendLabel"),
          status: "loading",
        },
      ],
      label: t("ldapSetup.loadingLabel"),
      status: "loading",
    };
  }
  if (status === null || status === undefined) {
    return {
      color: "default",
      detail: t("ldapSetup.unavailableDetail"),
      items: [
        {
          detail: t("setup.backendUnavailableDetail"),
          key: "backend",
          label: t("ldapSetup.backendLabel"),
          status: "unavailable",
        },
      ],
      label: t("ldapSetup.unavailableLabel"),
      status: "unavailable",
    };
  }

  const supportedModes = diagnosisAuthBackendStatusModes(status);
  const ldapSupported = status.configured && supportedModes.includes("ldap");
  const transportPolicy = diagnosisAuthLDAPTransportPolicySetupItem(
    status.transport_policy,
    ldapSupported,
    t,
  );
  const roleMapping = diagnosisAuthRoleMappingStatusReadiness(status, t);
  const roleMappingItem = diagnosisAuthLDAPRoleMappingSetupItem(
    status.role_mapping,
    roleMapping,
    t,
  );
  const items: DiagnosisAuthLDAPSetupReadinessItem[] = [
    {
      detail: ldapSupported
        ? t("ldapSetup.backendReadyDetail")
        : t("ldapSetup.backendBlockedDetail"),
      key: "backend",
      label: t("ldapSetup.backendLabel"),
      status: ldapSupported ? "ready" : "blocked",
    },
    {
      detail: transportPolicy.detail,
      key: "transport_policy",
      label: t("ldapSetup.transportLabel"),
      status: transportPolicy.status,
    },
    {
      detail: roleMappingItem.detail,
      key: "role_mapping",
      label: t("setup.roleMappingLabel"),
      status: roleMappingItem.status,
    },
  ];

  if (
    !ldapSupported ||
    transportPolicy.status === "blocked"
  ) {
    return {
      color: "error",
      detail: t("ldapSetup.blockedDetail"),
      items,
      label: t("ldapSetup.blockedLabel"),
      status: "blocked",
    };
  }
  if (
    transportPolicy.status === "review" ||
    transportPolicy.status === "unavailable" ||
    roleMappingItem.status === "review" ||
    roleMappingItem.status === "unavailable" ||
    roleMappingItem.status === "loading"
  ) {
    return {
      color: "warning",
      detail: t("ldapSetup.reviewDetail"),
      items,
      label: t("ldapSetup.reviewLabel"),
      status: "review",
    };
  }
  return {
    color: "success",
    detail: t("ldapSetup.readyDetail"),
    items,
    label: t("ldapSetup.readyLabel"),
    status: "ready",
  };
}

function diagnosisAuthLDAPTransportPolicySetupItem(
  policy: DiagnosisAuthTransportPolicyStatusSummary | undefined,
  ldapSupported: boolean,
  t: DiagnosisAuthTranslator,
): Pick<DiagnosisAuthLDAPSetupReadinessItem, "detail" | "status"> {
  if (!ldapSupported) {
    return {
      detail: t("ldapSetup.transportBlockedDetail"),
      status: "blocked",
    };
  }
  if (policy === undefined) {
    return {
      detail: t("ldapSetup.transportReviewDetail"),
      status: "review",
    };
  }
  switch (policy.security) {
    case "tls":
      return {
        detail: t("ldapSetup.transportTLSDetail"),
        status: "ready",
      };
    case "start_tls":
      return {
        detail: t("ldapSetup.transportStartTLSDetail"),
        status: "ready",
      };
    case "insecure_plaintext":
      return {
        detail: t("ldapSetup.transportInsecureDetail"),
        status: "blocked",
      };
  }
}

function diagnosisAuthLDAPRoleMappingSetupItem(
  mapping: DiagnosisAuthRoleMappingStatusSummary | undefined,
  readiness: DiagnosisAuthRoleMappingStatusReadiness,
  t: DiagnosisAuthTranslator,
): Pick<DiagnosisAuthLDAPSetupReadinessItem, "detail" | "status"> {
  if (readiness.status !== "ready") {
    return {
      detail: readiness.detail,
      status: readiness.status,
    };
  }
  const mappedRoles =
    (mapping?.owner_mapping_count ?? 0) + (mapping?.admin_mapping_count ?? 0);
  const defaultRoles = mapping?.default_roles ?? [];
  if (mappedRoles === 0 && defaultRoles.length > 0) {
    return {
      detail: t("ldapSetup.defaultRolesOnlyDetail"),
      status: "ready",
    };
  }
  return {
    detail: readiness.detail,
    status: "ready",
  };
}

export function diagnosisAuthWeComSetupReadiness(
  status: DiagnosisAuthStatusSummary | null | undefined,
  t: DiagnosisAuthTranslator,
  loading = false,
): DiagnosisAuthWeComSetupReadiness {
  if (loading) {
    return {
      color: "processing",
      detail: t("weComSetup.loadingDetail"),
      items: [
        {
          detail: t("setup.backendLoadingDetail"),
          key: "backend",
          label: t("weComSetup.backendLabel"),
          status: "loading",
        },
      ],
      label: t("weComSetup.loadingLabel"),
      status: "loading",
    };
  }
  if (status === null || status === undefined) {
    return {
      color: "default",
      detail: t("weComSetup.unavailableDetail"),
      items: [
        {
          detail: t("setup.backendUnavailableDetail"),
          key: "backend",
          label: t("weComSetup.backendLabel"),
          status: "unavailable",
        },
      ],
      label: t("weComSetup.unavailableLabel"),
      status: "unavailable",
    };
  }

  const supportedModes = diagnosisAuthBackendStatusModes(status);
  const authConfigured = status.configured && status.mode !== "none";
  const oidcSupported = authConfigured && supportedModes.includes("oidc");
  const legacyWeComAuthAdvertised =
    authConfigured && supportedModes.includes("wecom");
  const callbackStatus: DiagnosisAuthWeComSetupReadinessItem["status"] =
    !authConfigured || legacyWeComAuthAdvertised ? "blocked" : "review";
  const identityCheckStatus: DiagnosisAuthWeComSetupReadinessItem["status"] =
    !authConfigured || legacyWeComAuthAdvertised ? "blocked" : "ready";
  const identityCheckDetail =
    !authConfigured
      ? t("weComSetup.identityNotConfigured")
      : legacyWeComAuthAdvertised
        ? t("weComSetup.identityLegacy")
        : t("weComSetup.identityReady");
  const roleMapping = diagnosisAuthRoleMappingStatusReadiness(status, t);
  const roleMappingItem = diagnosisAuthWeComRoleMappingSetupItem(
    status.role_mapping,
    roleMapping,
    t,
  );
  const backendStatus: DiagnosisAuthWeComSetupReadinessItem["status"] =
    !authConfigured
      ? "blocked"
      : legacyWeComAuthAdvertised
        ? "blocked"
        : oidcSupported
          ? "ready"
          : "review";
  const callbackDetail =
    !authConfigured
      ? t("weComSetup.callbackNotConfigured")
      : legacyWeComAuthAdvertised
        ? t("weComSetup.callbackLegacy")
        : t("weComSetup.callbackReview");
  const items: DiagnosisAuthWeComSetupReadinessItem[] = [
    {
      detail: !authConfigured
        ? t("weComSetup.backendNotConfigured")
        : legacyWeComAuthAdvertised
          ? t("weComSetup.backendLegacy")
          : oidcSupported
            ? t("weComSetup.backendReady")
            : t("weComSetup.backendReview"),
      key: "backend",
      label: t("weComSetup.backendLabel"),
      status: backendStatus,
    },
    {
      detail: callbackDetail,
      key: "callback",
      label: t("weComSetup.callbackLabel"),
      status: callbackStatus,
    },
    {
      detail: identityCheckDetail,
      key: "identity_checks",
      label: t("weComSetup.identityLabel"),
      status: identityCheckStatus,
    },
    {
      detail: roleMappingItem.detail,
      key: "role_mapping",
      label: t("setup.roleMappingLabel"),
      status: roleMappingItem.status,
    },
  ];

  if (
    !authConfigured ||
    legacyWeComAuthAdvertised ||
    callbackStatus === "blocked" ||
    identityCheckStatus === "blocked"
  ) {
    return {
      color: "error",
      detail: t("weComSetup.blockedDetail"),
      items,
      label: t("weComSetup.blockedLabel"),
      status: "blocked",
    };
  }
  if (
    callbackStatus === "review" ||
    roleMappingItem.status === "review" ||
    roleMappingItem.status === "unavailable" ||
    roleMappingItem.status === "loading" ||
    backendStatus === "review"
  ) {
    return {
      color: "warning",
      detail: t("weComSetup.reviewDetail"),
      items,
      label: t("weComSetup.reviewLabel"),
      status: "review",
    };
  }
  return {
    color: "success",
    detail: t("weComSetup.readyDetail"),
    items,
    label: t("weComSetup.readyLabel"),
    status: "ready",
  };
}

function diagnosisAuthWeComRoleMappingSetupItem(
  mapping: DiagnosisAuthRoleMappingStatusSummary | undefined,
  readiness: DiagnosisAuthRoleMappingStatusReadiness,
  t: DiagnosisAuthTranslator,
): Pick<DiagnosisAuthWeComSetupReadinessItem, "detail" | "status"> {
  if (readiness.status !== "ready") {
    return {
      detail: readiness.detail,
      status: readiness.status,
    };
  }
  const mappedUsers =
    (mapping?.owner_mapping_count ?? 0) + (mapping?.admin_mapping_count ?? 0);
  const defaultRoles = mapping?.default_roles ?? [];
  if (mappedUsers === 0 && defaultRoles.length > 0) {
    return {
      detail: t("weComSetup.defaultRolesOnlyDetail"),
      status: "ready",
    };
  }
  return {
    detail: readiness.detail,
    status: "ready",
  };
}

export function diagnosisAuthBrowserSessionBlockReason(
  {
    intent,
    sessionAuthenticated,
    sessionLoading = false,
    sessionMode,
    sessionStatusAvailable = true,
    values,
  }: {
    intent: DiagnosisAuthBrowserSessionIntent;
    sessionAuthenticated: boolean;
    sessionLoading?: boolean;
    sessionMode?: string;
    sessionStatusAvailable?: boolean;
    values: DiagnosisAuthInputValues;
  },
  t: DiagnosisAuthTranslator,
): string {
  const mode = values.authMode ?? "session";
  if (mode !== "session" && mode !== "wecom") {
    return "";
  }
  if (sessionLoading) {
    return intent === "check"
      ? t("browserBlock.checkingBeforeCheck")
      : t("browserBlock.checkingBeforeAction");
  }
  if (!sessionStatusAvailable) {
    return intent === "check"
      ? t("browserBlock.unavailableBeforeCheck")
      : t("browserBlock.unavailableBeforeAction");
  }
  if (!sessionAuthenticated) {
    if (mode === "session") {
      return intent === "check"
        ? t("browserBlock.signInBeforeCheck")
        : t("browserBlock.signInBeforeAction");
    }
    return intent === "check"
      ? t("browserBlock.weComSignInBeforeCheck")
      : t("browserBlock.weComSignInBeforeAction");
  }
  if (
    mode === "wecom" &&
    sessionMode !== undefined &&
    sessionMode.trim() !== "" &&
    sessionMode !== "wecom"
  ) {
    return t("browserBlock.weComUseIAMSession");
  }
  return "";
}

export function diagnosisAuthBrowserSessionDisplaySummary(
  {
    authenticated,
    checkFailed,
    expectedMode,
    loading,
    mode,
    roles,
    subject,
    unauthenticatedDetail,
  }: {
    authenticated: boolean;
    checkFailed: boolean;
    expectedMode?: "wecom";
    loading: boolean;
    mode?: string;
    roles: readonly string[];
    subject: string;
    unauthenticatedDetail: string;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthBrowserSessionDisplaySummary {
  if (loading) {
    return {
      active: false,
      alertType: "info",
      detail: t("browserSummary.loading"),
    };
  }
  if (checkFailed) {
    return {
      active: false,
      alertType: "warning",
      detail: t("browserSummary.unavailable"),
    };
  }
  if (!authenticated) {
    return {
      active: false,
      alertType: "info",
      detail: unauthenticatedDetail,
    };
  }
  const summary = diagnosisAuthBrowserSessionAuthenticatedSummary(
    {
      expectedMode,
      mode,
      roles,
      subject,
    },
    t,
  );
  return {
    active: true,
    alertType: summary.alertType,
    detail: summary.detail,
  };
}

export function diagnosisAuthBrowserSessionShouldClearAfterError({
  authMode,
  status,
}: {
  authMode: DiagnosisAuthBrowserSessionErrorAuthMode;
  status?: number;
}): boolean {
  return authMode === "session" && (status === 401 || status === 403);
}

export function diagnosisAuthBrowserSessionAuthenticatedSummary(
  {
    expectedMode,
    mode,
    roles,
    subject,
  }: {
    expectedMode?: "wecom";
    mode?: string;
    roles: readonly string[];
    subject: string;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthBrowserSessionAuthenticatedSummary {
  const subjectLabel =
    subject.trim() === "" ? t("common.currentUser") : subject;
  const rolesLabel =
    roles.length === 0 ? t("common.noRoles") : roles.join(", ");
  const sourceLabel = diagnosisAuthBrowserSessionSourceLabel(mode, t);
  if (
    expectedMode === "wecom" &&
    mode !== undefined &&
    mode.trim() !== "" &&
    mode !== "wecom"
  ) {
    return {
      alertType: "warning",
      detail:
        sourceLabel === ""
          ? t("browserSummary.migrated", {
              roles: rolesLabel,
              subject: subjectLabel,
            })
          : t("browserSummary.migratedWithSource", {
              roles: rolesLabel,
              source: sourceLabel,
              subject: subjectLabel,
            }),
    };
  }
  return {
    alertType: "success",
    detail:
      sourceLabel === ""
        ? t("browserSummary.authenticated", {
            roles: rolesLabel,
            subject: subjectLabel,
          })
        : t("browserSummary.authenticatedWithSource", {
            roles: rolesLabel,
            source: sourceLabel,
            subject: subjectLabel,
          }),
  };
}

function diagnosisAuthBrowserSessionSourceLabel(
  mode: string | undefined,
  t: DiagnosisAuthTranslator,
): string {
  switch (mode?.trim()) {
    case "ldap":
      return t("provider.ldap");
    case "wecom":
      return t("provider.weCom");
    case "static":
    case "bearer":
      return t("provider.staticBearerAuth");
    case "oidc":
      return t("provider.iamOIDC");
    case "":
    case undefined:
      return "";
    default:
      return t("provider.configuredBackend");
  }
}

export function diagnosisAuthCheckSuccessFeedback(
  {
    roles,
    subject,
  }: {
    roles: readonly string[];
    subject: string;
  },
  t: DiagnosisAuthTranslator,
): DiagnosisAuthCheckSuccessFeedback {
  const rolesLabel =
    roles.length === 0 ? t("common.noRoles") : roles.join(", ");
  return {
    logLevel: "info",
    logMessage: t("feedback.log", { roles: rolesLabel, subject }),
    toastMessage: t("feedback.toast", { subject }),
    toastType: "success",
  };
}

function diagnosisAuthBackendCheckRequiredDetail(
  mode: DiagnosisAuthMode,
  t: DiagnosisAuthTranslator,
): string {
  if (mode === "session") {
    return t("backend.checkRequiredSession");
  }
  if (mode === "wecom") {
    return t("backend.checkRequiredWeCom");
  }
  return t("backend.checkRequiredCredentials");
}
