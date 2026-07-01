import { containsControlOrWhitespace } from "../settings/validation";

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

export type DiagnosisAuthWeComQuickSignInPrompt = {
  detail: string;
  label: string;
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

export function diagnosisAuthInputReadiness(
  values: DiagnosisAuthInputValues,
): DiagnosisAuthInputReadiness {
  const mode = values.authMode ?? "session";
  if (mode === "bearer") {
    return bearerInputReadiness(values.bearerToken ?? "");
  }
  if (mode === "session") {
    return browserSessionInputReadiness();
  }
  if (mode === "wecom") {
    return {
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
      label: "Enterprise WeChat browser login migrated.",
      mode: "wecom",
      status: "blocked",
    };
  }
  return ldapInputReadiness(
    values.ldapUsername ?? "",
    values.ldapPassword ?? "",
  );
}

export function diagnosisAuthLDAPBrowserSessionPromotionNotice(): DiagnosisAuthLDAPBrowserSessionPromotionNotice {
  return {
    detail:
      "After Check auth accepts explicitly configured LDAP credentials, OpenClarion exchanges them for an HttpOnly browser session, clears the LDAP password from this form, and uses local RBAC for diagnosis room create or connect actions.",
    message: "LDAP fallback creates a browser session",
  };
}

function browserSessionInputReadiness(): DiagnosisAuthInputReadiness {
  return {
    detail:
      "Use the current OpenClarion browser session from IAM OIDC. Check auth verifies the HttpOnly session cookie through the backend, and diagnosis room access is enforced by local RBAC.",
    label: "IAM browser session ready to check.",
    mode: "session",
    status: "ready",
  };
}

function ldapInputReadiness(
  rawUsername: string,
  password: string,
): DiagnosisAuthInputReadiness {
  const username = rawUsername.trim();
  if (username === "" || password === "") {
    return {
      detail:
        "Use LDAP only as a legacy fallback. Enter LDAP username and password, then run Check auth before creating or connecting to a diagnosis room.",
      label: "LDAP fallback credentials required.",
      mode: "ldap",
      status: "pending",
    };
  }
  if (containsControlOrWhitespace(username)) {
    return {
      detail:
        "LDAP username must not contain whitespace or control characters.",
      label: "LDAP username is invalid.",
      mode: "ldap",
      status: "blocked",
    };
  }
  if (/[\u0000\r\n]/.test(password)) {
    return {
      detail: "LDAP password must not contain null bytes or line breaks.",
      label: "LDAP password is invalid.",
      mode: "ldap",
      status: "blocked",
    };
  }
  return {
    detail:
      "Direct LDAP credentials are locally well-formed; Check auth verifies them against the explicitly configured backend LDAP provider.",
    label: "LDAP fallback credentials ready to check.",
    mode: "ldap",
    status: "ready",
  };
}

export function diagnosisAuthModeOptions(
  backendStatus?: DiagnosisAuthBackendStatusSnapshot,
): DiagnosisAuthModeOption[] {
  return [
    {
      disabled:
        diagnosisAuthBackendModeBlockReason("session", backendStatus) !== "",
      label: "IAM session",
      value: "session",
    },
    {
      disabled:
        diagnosisAuthBackendModeBlockReason("ldap", backendStatus) !== "",
      label: "LDAP fallback",
      value: "ldap",
    },
    {
      disabled:
        diagnosisAuthBackendModeBlockReason("bearer", backendStatus) !== "",
      label: "Dev bearer",
      value: "bearer",
    },
  ];
}

export function diagnosisAuthCoercedMode(
  mode: DiagnosisAuthMode,
  backendStatus?: DiagnosisAuthBackendStatusSnapshot,
): DiagnosisAuthMode {
  if (diagnosisAuthBackendModeBlockReason(mode, backendStatus) === "") {
    return mode;
  }
  const fallback = diagnosisAuthModeOptions(backendStatus).find(
    (option) => !option.disabled,
  );
  return fallback?.value ?? mode;
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
): string {
  const labels = diagnosisAuthBackendStatusModes(status).map(
    diagnosisAuthBackendShortModeLabel,
  );
  return diagnosisAuthListLabel(labels, "+");
}

export function diagnosisAuthBackendCredentialListLabel(
  status: DiagnosisAuthBackendStatusWithModes | null | undefined,
): string {
  if (status === null || status === undefined) {
    return "";
  }
  return diagnosisAuthBackendCredentialLabels(
    diagnosisAuthBackendStatusModes(status),
    status.mode,
  );
}

export function diagnosisAuthBackendModeDisplayItems(
  status: DiagnosisAuthBackendStatusWithModes | null | undefined,
): DiagnosisAuthBackendModeDisplayItem[] {
  return diagnosisAuthBackendStatusModes(status).map((mode) => ({
    color: diagnosisAuthBackendModeTagColor(mode),
    label: diagnosisAuthBackendShortModeLabel(mode),
    mode,
  }));
}

export function diagnosisAuthBackendReadiness({
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
}): DiagnosisAuthBackendReadiness {
  const input = diagnosisAuthInputReadiness(values);
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
    backendStatus,
  );
  if (backendModeBlockReason !== "") {
    return {
      color: "error",
      detail: backendModeBlockReason,
      label: "Auth mode does not match backend.",
      status: "blocked",
    };
  }
  if (checking) {
    return {
      color: "processing",
      detail: "Backend diagnosis auth check is running.",
      label: "Checking backend auth.",
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
      detail: diagnosisAuthBackendCheckRequiredDetail(input.mode),
      label: "Backend auth check required.",
      status: "needs_check",
    };
  }
  if (!authCheckMatchesExpectedSubject(input.mode, lastCheck, expectedSubject)) {
    return {
      color: "warning",
      detail: diagnosisAuthBackendCheckSubjectChangedDetail(
        lastCheck,
        expectedSubject,
      ),
      label: "Backend auth check required.",
      status: "needs_check",
    };
  }
  if (lastCheck.status === "failed") {
    return {
      color: "error",
      detail: lastCheck.message,
      label: "Backend auth check failed.",
      status: "failed",
    };
  }
  return {
    color: "success",
    detail: diagnosisAuthVerifiedDetail(lastCheck),
    label: "Backend auth verified.",
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
  return (
    diagnosisAuthBackendReadiness({
      backendStatus,
      checking,
      expectedSubject,
      inputRevision,
      lastCheck,
      values,
    }).status === "verified"
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
): DiagnosisAuthRolloutReadiness {
  const roleAuthorized = diagnosisAuthProofHasUsableRole(proof);
  if (proof.status === "verified" && diagnosisAuthRolloutSSOMode(proof.mode)) {
    const provider = diagnosisAuthProviderDisplayName(proof.mode);
    return {
      checkedAt: proof.checkedAt,
      detail:
        proof.detail ||
        `${provider} identity is verified against the running backend; diagnosis room access is enforced by local RBAC.`,
      label: `${provider} rollout proof ready.`,
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
        ? "Legacy Enterprise WeChat browser auth proof is no longer accepted for rollout. Use IAM OIDC browser sessions and keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks."
        : "Static bearer auth is acceptable for development checks, but operator rollout requires IAM or LDAP identity proof with OpenClarion local RBAC configured.",
      label: "Operator SSO proof required for rollout.",
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
      detail:
        proof.detail ||
        "Resolve backend diagnosis auth before accepting operator rollout.",
      label: "Operator auth rollout proof blocked.",
      mode: proof.mode,
      roleAuthorized,
      roles: proof.roles,
      status: "blocked",
      subject: proof.subject,
    };
  }
  return {
    checkedAt: proof.checkedAt,
    detail:
      proof.detail ||
      "Run Check auth with the current IAM browser session before accepting operator rollout. Direct LDAP and Enterprise WeChat auth remain explicit compatibility paths only where configured.",
    label: "Operator auth rollout proof pending.",
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
): string {
  if (readiness.roleAuthorized === true) {
    switch (readiness.mode) {
      case "ldap":
        return "LDAP provider role metadata is present. Diagnosis room permissions are still enforced by OpenClarion local RBAC.";
      case "session":
        return "The browser session includes legacy owner/admin metadata. Diagnosis room permissions are still enforced by OpenClarion local RBAC.";
      case "wecom":
        return "Enterprise WeChat provider role metadata is present. Diagnosis room permissions are still enforced by OpenClarion local RBAC.";
      case "bearer":
        return "Static bearer roles are development-only; operator rollout should use IAM, LDAP, or Enterprise WeChat identity proof plus OpenClarion local RBAC.";
    }
  }
  switch (readiness.mode) {
    case "ldap":
      return "LDAP provider role mapping is optional for identity proof. Assign diagnosis room access in OpenClarion local RBAC.";
    case "session":
      return "Browser-session provider roles are optional for identity proof. Assign diagnosis room access in OpenClarion local RBAC.";
    case "wecom":
      return "Enterprise WeChat provider role mapping is optional for identity proof. Assign diagnosis room access in OpenClarion local RBAC.";
    case "bearer":
      return "Static bearer auth is development-only; use IAM, LDAP, or Enterprise WeChat identity proof plus OpenClarion local RBAC for rollout.";
  }
}

export function diagnosisAuthRoleMappingStatusDetail(
  status: DiagnosisAuthStatusSummary | null | undefined,
  loading = false,
): string {
  return diagnosisAuthRoleMappingStatusReadiness(status, loading).detail;
}

export function diagnosisAuthRoleMappingStatusReadiness(
  status: DiagnosisAuthStatusSummary | null | undefined,
  loading = false,
): DiagnosisAuthRoleMappingStatusReadiness {
  if (loading) {
    return {
      color: "processing",
      detail: "Loading backend role mapping summary.",
      label: "Loading",
      status: "loading",
    };
  }
  if (status === null || status === undefined) {
    return {
      color: "default",
      detail: "Backend role mapping summary is unavailable.",
      label: "Unavailable",
      status: "unavailable",
    };
  }
  if (!status.configured || status.mode === "none") {
    return {
      color: "error",
      detail: "Diagnosis auth is not configured in the running backend.",
      label: "Not configured",
      status: "blocked",
    };
  }
  const mapping = status.role_mapping;
  if (mapping === undefined) {
    return {
      color: "warning",
      detail: "Backend did not report role mapping metadata.",
      label: "Not reported",
      status: "unavailable",
    };
  }
  if (!mapping.configured) {
    const provider = diagnosisAuthProviderDisplayNameForBackend(status.mode);
    return {
      color: "success",
      detail: `${provider} has no provider role mapping configured. Identity-only authentication is accepted; diagnosis room permissions are assigned through OpenClarion local RBAC.`,
      label: "Identity-only",
      status: "ready",
    };
  }
  const defaultRoles =
    mapping.default_roles.length === 0
      ? "none"
      : mapping.default_roles.join(", ");
  return {
    color: "success",
    detail: `${diagnosisAuthProviderDisplayNameForBackend(status.mode)} reports optional provider role metadata: ${mapping.owner_mapping_count} owner mapping(s), ${mapping.admin_mapping_count} admin mapping(s), and default roles: ${defaultRoles}. Diagnosis room permissions are assigned through OpenClarion local RBAC.`,
    label: "Metadata reported",
    status: "ready",
  };
}

function diagnosisAuthRolloutSSOMode(mode: DiagnosisAuthMode): boolean {
  return mode === "ldap" || mode === "session";
}

function diagnosisAuthProviderDisplayName(mode: DiagnosisAuthMode): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "session":
      return "Browser session";
    case "wecom":
      return "Enterprise WeChat";
    case "bearer":
      return "Static bearer";
  }
}

function diagnosisAuthProviderDisplayNameForBackend(
  mode: DiagnosisAuthBackendMode,
): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "wecom":
      return "Enterprise WeChat";
    case "static":
      return "Static bearer";
    case "oidc":
      return "IAM OIDC";
    case "unknown":
      return "Backend auth";
    case "none":
      return "Diagnosis auth";
  }
}

export function diagnosisAuthCheckBlockReason({
  backendStatus,
  values,
}: {
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  values: DiagnosisAuthInputValues;
}): string {
  const input = diagnosisAuthInputReadiness(values);
  if (input.status !== "ready") {
    return input.detail;
  }
  return diagnosisAuthBackendModeBlockReason(input.mode, backendStatus);
}

export function diagnosisAuthActionBlockReason({
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
}): string {
  const readiness = diagnosisAuthBackendReadiness({
    backendStatus,
    checking,
    expectedSubject,
    inputRevision,
    lastCheck,
    values,
  });
  if (readiness.status === "verified") {
    return "";
  }
  if (readiness.status === "blocked") {
    return readiness.detail;
  }
  switch (action) {
    case "connect":
      if (values.authMode === "wecom") {
        return "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and run Check auth successfully before connecting to a diagnosis room.";
      }
      if (values.authMode === "session") {
        return "Run Check auth successfully with the current browser session before connecting to a diagnosis room.";
      }
      return "Run Check auth successfully before connecting to a diagnosis room.";
    case "create":
      if (values.authMode === "wecom") {
        return "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and run Check auth successfully before creating a diagnosis room.";
      }
      if (values.authMode === "session") {
        return "Run Check auth successfully with the current browser session before creating a diagnosis room.";
      }
      return "Run Check auth successfully before creating a diagnosis room.";
  }
}

export function diagnosisAuthWeComQuickSignInPrompt({
  selectedModes,
}: {
  backendStatus?: DiagnosisAuthBackendStatusSnapshot;
  selectedModes: readonly DiagnosisAuthMode[];
}): DiagnosisAuthWeComQuickSignInPrompt | null {
  if (selectedModes.includes("wecom") || selectedModes.includes("session")) {
    return null;
  }
  return null;
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

function diagnosisAuthVerifiedDetail(check: DiagnosisAuthBackendCheck): string {
  const roles = check.roles.length === 0 ? "no roles" : check.roles.join(", ");
  const checkedAt = check.checkedAt?.trim();
  if (checkedAt !== undefined && checkedAt !== "") {
    return `Authenticated as ${check.subject}. Roles: ${roles}. Checked at ${checkedAt}.`;
  }
  return `Authenticated as ${check.subject}. Roles: ${roles}.`;
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
): string {
  const subject = expectedSubject?.trim() ?? "";
  if (subject === "") {
    return "Run Check auth again for the current OpenClarion browser session.";
  }
  return `Run Check auth again for the current OpenClarion browser session subject ${subject}; the last backend check was for ${check.subject}.`;
}

function diagnosisAuthBackendModeBlockReason(
  mode: DiagnosisAuthMode,
  backendStatus?: DiagnosisAuthBackendStatusSnapshot,
): string {
  if (backendStatus === undefined) {
    return "";
  }
  if (!backendStatus.configured || backendStatus.mode === "none") {
    return "Diagnosis auth is not configured in the running backend.";
  }
  const supportedModes = diagnosisAuthBackendSupportedModes(backendStatus);
  if (supportedModes.length === 1 && supportedModes[0] === "unknown") {
    return "Backend diagnosis auth mode is unknown; reload or inspect deployment before sending credentials.";
  }
  if (mode === "wecom") {
    return "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.";
  }
  if (mode === "session") {
    if (supportedModes.includes("ldap") || supportedModes.includes("oidc")) {
      return "";
    }
    return `The running backend expects ${diagnosisAuthBackendCredentialLabels(
      supportedModes,
      backendStatus.mode,
    )}, not an OpenClarion browser session.`;
  }
  const requestedMode = diagnosisAuthBackendModeForInput(mode);
  if (supportedModes.includes(requestedMode)) {
    return "";
  }
  if (
    supportedModes.length === 1 &&
    supportedModes[0] === "wecom"
  ) {
    return "The running backend advertises legacy Enterprise WeChat browser authentication. OpenClarion browser login is now handled by IAM OIDC; update the backend auth mode before accepting rollout.";
  }
  if (mode === "ldap") {
    return `The running backend expects ${backendAuthCredentialLabel(
      supportedModes[0] ?? backendStatus.mode,
    )}, not LDAP Basic credentials.`;
  }
  return `The running backend expects ${diagnosisAuthBackendCredentialLabels(
    supportedModes,
    backendStatus.mode,
  )}, not Bearer credentials.`;
}

function backendAuthCredentialLabel(mode: DiagnosisAuthBackendMode): string {
  switch (mode) {
    case "ldap":
      return "LDAP Basic credentials";
    case "static":
      return "a static Bearer token";
    case "oidc":
      return "IAM OIDC authentication";
    case "wecom":
      return "Enterprise WeChat authentication";
    case "none":
      return "no credentials";
    case "unknown":
      return "an unknown credential type";
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
): string {
  const labels = (modes.length === 0 ? [fallback] : modes)
    .filter((mode) => mode !== "none")
    .map(backendAuthCredentialLabel);
  if (labels.length === 0) {
    return backendAuthCredentialLabel(fallback);
  }
  if (labels.length === 1) {
    return labels[0] ?? backendAuthCredentialLabel(fallback);
  }
  const lastLabel = labels.at(-1) ?? backendAuthCredentialLabel(fallback);
  return `${labels.slice(0, -1).join(", ")} or ${lastLabel}`;
}

function diagnosisAuthBackendShortModeLabel(
  mode: DiagnosisAuthBackendMode,
): string {
  switch (mode) {
    case "ldap":
      return "LDAP";
    case "static":
      return "static";
    case "oidc":
      return "OIDC";
    case "wecom":
      return "WeCom";
    case "unknown":
      return "unknown";
    case "none":
      return "not configured";
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
  return `${labels.slice(0, -1).join(", ")} and ${labels[labels.length - 1] ?? ""}`;
}

export function diagnosisAuthLDAPSetupReadiness(
  status: DiagnosisAuthStatusSummary | null | undefined,
  loading = false,
): DiagnosisAuthLDAPSetupReadiness {
  if (loading) {
    return {
      color: "processing",
      detail: "Loading LDAP authentication setup summary.",
      items: [
        {
          detail: "Backend auth status is still loading.",
          key: "backend",
          label: "Backend LDAP mode",
          status: "loading",
        },
      ],
      label: "LDAP setup loading",
      status: "loading",
    };
  }
  if (status === null || status === undefined) {
    return {
      color: "default",
      detail: "LDAP authentication setup could not be loaded.",
      items: [
        {
          detail: "Backend auth status is unavailable.",
          key: "backend",
          label: "Backend LDAP mode",
          status: "unavailable",
        },
      ],
      label: "LDAP setup unavailable",
      status: "unavailable",
    };
  }

  const supportedModes = diagnosisAuthBackendStatusModes(status);
  const ldapSupported = status.configured && supportedModes.includes("ldap");
  const transportPolicy = diagnosisAuthLDAPTransportPolicySetupItem(
    status.transport_policy,
    ldapSupported,
  );
  const roleMapping = diagnosisAuthRoleMappingStatusReadiness(status);
  const roleMappingItem = diagnosisAuthLDAPRoleMappingSetupItem(
    status.role_mapping,
    roleMapping,
  );
  const items: DiagnosisAuthLDAPSetupReadinessItem[] = [
    {
      detail: ldapSupported
        ? "Backend advertises LDAP diagnosis authentication."
        : "Backend does not advertise LDAP diagnosis authentication. Configure LDAP bind/search settings and a session signing key before operator rollout.",
      key: "backend",
      label: "Backend LDAP mode",
      status: ldapSupported ? "ready" : "blocked",
    },
    {
      detail: transportPolicy.detail,
      key: "transport_policy",
      label: "Transport policy",
      status: transportPolicy.status,
    },
    {
      detail: roleMappingItem.detail,
      key: "role_mapping",
      label: "Role mapping",
      status: roleMappingItem.status,
    },
  ];

  if (
    !ldapSupported ||
    transportPolicy.status === "blocked"
  ) {
    return {
      color: "error",
      detail:
        "LDAP rollout is blocked until backend LDAP mode and encrypted credential transport are configured.",
      items,
      label: "LDAP setup blocked",
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
      detail:
        "LDAP can be checked from this console, but rollout still needs LDAP transport policy and optional provider role metadata reviewed before acceptance.",
      items,
      label: "LDAP setup needs review",
      status: "review",
    };
  }
  return {
    color: "success",
    detail:
      "LDAP setup advertises backend LDAP auth and encrypted credential transport. Diagnosis room permissions are enforced by OpenClarion local RBAC.",
    items,
    label: "LDAP setup ready",
    status: "ready",
  };
}

function diagnosisAuthLDAPTransportPolicySetupItem(
  policy: DiagnosisAuthTransportPolicyStatusSummary | undefined,
  ldapSupported: boolean,
): Pick<DiagnosisAuthLDAPSetupReadinessItem, "detail" | "status"> {
  if (!ldapSupported) {
    return {
      detail:
        "LDAP transport policy cannot be reviewed until the backend advertises LDAP diagnosis authentication.",
      status: "blocked",
    };
  }
  if (policy === undefined) {
    return {
      detail:
        "Backend did not report LDAP transport policy metadata. Verify privately that production configuration uses LDAPS or LDAP with StartTLS before acceptance.",
      status: "review",
    };
  }
  switch (policy.security) {
    case "tls":
      return {
        detail:
          "Backend reports LDAP credentials use TLS transport, such as LDAPS.",
        status: "ready",
      };
    case "start_tls":
      return {
        detail:
          "Backend reports LDAP credentials use LDAP with a StartTLS upgrade before binds.",
        status: "ready",
      };
    case "insecure_plaintext":
      return {
        detail:
          "Backend reports LDAP plaintext transport is explicitly allowed. Use LDAPS or LDAP with StartTLS before production rollout.",
        status: "blocked",
      };
  }
}

function diagnosisAuthLDAPRoleMappingSetupItem(
  mapping: DiagnosisAuthRoleMappingStatusSummary | undefined,
  readiness: DiagnosisAuthRoleMappingStatusReadiness,
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
      detail:
        "LDAP reports only optional default provider roles. Identity proof is accepted; assign diagnosis room access in OpenClarion local RBAC.",
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
  loading = false,
): DiagnosisAuthWeComSetupReadiness {
  if (loading) {
    return {
      color: "processing",
      detail: "Loading Enterprise WeChat collaboration setup summary.",
      items: [
        {
          detail: "Backend auth status is still loading.",
          key: "backend",
          label: "Browser auth mode",
          status: "loading",
        },
      ],
      label: "Enterprise WeChat collaboration loading",
      status: "loading",
    };
  }
  if (status === null || status === undefined) {
    return {
      color: "default",
      detail: "Enterprise WeChat collaboration setup could not be loaded.",
      items: [
        {
          detail: "Backend auth status is unavailable.",
          key: "backend",
          label: "Browser auth mode",
          status: "unavailable",
        },
      ],
      label: "Enterprise WeChat collaboration unavailable",
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
      ? "Browser identity cannot be reviewed until diagnosis authentication is configured."
      : legacyWeComAuthAdvertised
        ? "Backend still advertises legacy Enterprise WeChat browser identity. Browser sessions must come from IAM OIDC; keep Enterprise WeChat identity only for app-message callbacks and collaboration context."
        : "Browser identity comes from IAM OIDC claims. Enterprise WeChat callbacks should resolve participants against the local directory projection and OpenClarion RBAC.";
  const roleMapping = diagnosisAuthRoleMappingStatusReadiness(status);
  const roleMappingItem = diagnosisAuthWeComRoleMappingSetupItem(
    status.role_mapping,
    roleMapping,
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
      ? "Enterprise WeChat app-message callbacks cannot be reviewed until diagnosis authentication is configured."
      : legacyWeComAuthAdvertised
        ? "Backend still advertises legacy Enterprise WeChat browser callback handling. OpenClarion browser sessions must be minted by IAM OIDC, not Enterprise WeChat SSO callbacks."
        : "Enterprise WeChat app-message callbacks are outside diagnosis auth status. Verify the dedicated /api/v1/diagnosis/wecom/app-callback endpoint from notification and collaboration settings.";
  const items: DiagnosisAuthWeComSetupReadinessItem[] = [
    {
      detail: !authConfigured
        ? "Diagnosis browser authentication is not configured in the running backend."
        : legacyWeComAuthAdvertised
          ? "Backend still advertises legacy Enterprise WeChat browser authentication. OpenClarion browser login is now handled by IAM OIDC; remove legacy WeCom auth before rollout acceptance."
          : oidcSupported
            ? "Backend advertises IAM OIDC for OpenClarion browser sessions. Enterprise WeChat remains a message and collaboration integration."
            : "Backend no longer advertises legacy Enterprise WeChat browser authentication, but IAM OIDC is not advertised. Verify the intended browser auth provider before rollout acceptance.",
      key: "backend",
      label: "Browser auth mode",
      status: backendStatus,
    },
    {
      detail: callbackDetail,
      key: "callback",
      label: "App callback",
      status: callbackStatus,
    },
    {
      detail: identityCheckDetail,
      key: "identity_checks",
      label: "Identity source",
      status: identityCheckStatus,
    },
    {
      detail: roleMappingItem.detail,
      key: "role_mapping",
      label: "Role mapping",
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
      detail:
        "Enterprise WeChat browser authentication is migrated to IAM OIDC. Remove legacy WeCom browser auth and keep Enterprise WeChat for app-message callbacks, notifications, and collaboration context.",
      items,
      label: "Enterprise WeChat browser auth migrated",
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
      detail:
        "Enterprise WeChat browser login is no longer a rollout target. Review app-message callback handling separately and keep browser authorization on IAM OIDC plus OpenClarion local RBAC.",
      items,
      label: "Enterprise WeChat collaboration needs review",
      status: "review",
    };
  }
  return {
    color: "success",
    detail:
      "Browser authentication uses IAM OIDC, and Enterprise WeChat is limited to app messages, notifications, and diagnosis-room collaboration context. Diagnosis room permissions are enforced by OpenClarion local RBAC.",
    items,
    label: "Enterprise WeChat collaboration ready",
    status: "ready",
  };
}

function diagnosisAuthWeComRoleMappingSetupItem(
  mapping: DiagnosisAuthRoleMappingStatusSummary | undefined,
  readiness: DiagnosisAuthRoleMappingStatusReadiness,
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
      detail:
        "Enterprise WeChat reports only optional default provider roles. Identity proof is accepted; assign diagnosis room access in OpenClarion local RBAC.",
      status: "ready",
    };
  }
  return {
    detail: readiness.detail,
    status: "ready",
  };
}

export function diagnosisAuthBrowserSessionBlockReason({
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
}): string {
  const mode = values.authMode ?? "session";
  if (mode !== "session" && mode !== "wecom") {
    return "";
  }
  if (sessionLoading) {
    return intent === "check"
      ? "Checking OpenClarion browser session before running Check auth."
      : "Checking OpenClarion browser session before continuing.";
  }
  if (!sessionStatusAvailable) {
    return intent === "check"
      ? "OpenClarion browser session could not be checked. Reload this page or sign in again before running Check auth."
      : "OpenClarion browser session could not be checked. Reload this page or sign in again before creating or connecting to a diagnosis room.";
  }
  if (!sessionAuthenticated) {
    if (mode === "session") {
      return intent === "check"
        ? "Sign in with IAM before running Check auth with a browser session."
        : "Sign in with IAM before creating or connecting to a diagnosis room with a browser session.";
    }
    return intent === "check"
      ? "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and sign in before running Check auth."
      : "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and sign in before creating or connecting to a diagnosis room.";
  }
  if (
    mode === "wecom" &&
    sessionMode !== undefined &&
    sessionMode.trim() !== "" &&
    sessionMode !== "wecom"
  ) {
    return "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and use the active IAM session for Check auth.";
  }
  return "";
}

export function diagnosisAuthBrowserSessionDisplaySummary({
  authenticated,
  checkFailed,
  expectedMode,
  loading,
  mode,
  roleAuthorized,
  roles,
  subject,
  unauthenticatedDetail,
}: {
  authenticated: boolean;
  checkFailed: boolean;
  expectedMode?: "wecom";
  loading: boolean;
  mode?: string;
  roleAuthorized?: boolean;
  roles: readonly string[];
  subject: string;
  unauthenticatedDetail: string;
}): DiagnosisAuthBrowserSessionDisplaySummary {
  if (loading) {
    return {
      active: false,
      alertType: "info",
      detail: "Checking the current OpenClarion browser session.",
    };
  }
  if (checkFailed) {
    return {
      active: false,
      alertType: "warning",
      detail:
        "OpenClarion browser session could not be checked. Reload this page or sign in again before continuing.",
    };
  }
  if (!authenticated) {
    return {
      active: false,
      alertType: "info",
      detail: unauthenticatedDetail,
    };
  }
  const summary = diagnosisAuthBrowserSessionAuthenticatedSummary({
    expectedMode,
    mode,
    roleAuthorized,
    roles,
    subject,
  });
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

export function diagnosisAuthBrowserSessionAuthenticatedSummary({
  expectedMode,
  mode,
  roleAuthorized,
  roles,
  subject,
}: {
  expectedMode?: "wecom";
  mode?: string;
  roleAuthorized?: boolean;
  roles: readonly string[];
  subject: string;
}): DiagnosisAuthBrowserSessionAuthenticatedSummary {
  const subjectLabel = subject.trim() === "" ? "the current user" : subject;
  const rolesLabel = roles.length === 0 ? "no roles" : roles.join(", ");
  const sourceLabel = diagnosisAuthBrowserSessionSourceLabel(mode);
  const sourceClause = sourceLabel === "" ? "" : ` using ${sourceLabel}`;
  if (
    expectedMode === "wecom" &&
    mode !== undefined &&
    mode.trim() !== "" &&
    mode !== "wecom"
  ) {
    return {
      alertType: "warning",
      detail: `Signed in as ${subjectLabel}${sourceClause}. Roles: ${rolesLabel}. Enterprise WeChat browser login has been replaced by IAM OIDC; select IAM browser session before running Check auth.`,
    };
  }
  if (roleAuthorized === false) {
    return {
      alertType: "success",
      detail: `Signed in as ${subjectLabel}${sourceClause}. Roles: ${rolesLabel}. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.`,
    };
  }
  if (roleAuthorized === true) {
    return {
      alertType: "success",
      detail: `Signed in as ${subjectLabel}${sourceClause}. Roles: ${rolesLabel}. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.`,
    };
  }
  return {
    alertType: "success",
    detail: `Signed in as ${subjectLabel}${sourceClause}. Roles: ${rolesLabel}. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.`,
  };
}

function diagnosisAuthBrowserSessionSourceLabel(mode: string | undefined): string {
  switch (mode?.trim()) {
    case "ldap":
      return "LDAP";
    case "wecom":
      return "Enterprise WeChat";
    case "static":
    case "bearer":
      return "static bearer auth";
    case "oidc":
      return "IAM OIDC";
    case "":
    case undefined:
      return "";
    default:
      return "the configured backend provider";
  }
}

export function diagnosisAuthCheckSuccessFeedback({
  roles,
  subject,
}: {
  mode: DiagnosisAuthMode;
  roleAuthorized?: boolean;
  roles: readonly string[];
  subject: string;
}): DiagnosisAuthCheckSuccessFeedback {
  const rolesLabel = roles.length === 0 ? "no roles" : roles.join(", ");
  return {
    logLevel: "info",
    logMessage: `Authentication checked for ${subject} (${rolesLabel}).`,
    toastMessage: `Authenticated as ${subject}. Local RBAC will authorize diagnosis room actions.`,
    toastType: "success",
  };
}

function diagnosisAuthBackendCheckRequiredDetail(
  mode: DiagnosisAuthMode,
): string {
  if (mode === "session") {
    return "Run Check auth to verify the current OpenClarion browser session identity against the backend provider. Diagnosis room permissions are enforced by local RBAC.";
  }
  if (mode === "wecom") {
    return "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session, then run Check auth to verify the HttpOnly browser session identity. Diagnosis room permissions are enforced by local RBAC.";
  }
  return "Run Check auth to verify these credentials against the configured backend provider.";
}

function bearerInputReadiness(token: string): DiagnosisAuthInputReadiness {
  const trimmed = token.trim();
  if (trimmed === "") {
    return {
      detail:
        "Enter a bearer token only when the backend is configured for static bearer diagnosis auth.",
      label: "Bearer token required.",
      mode: "bearer",
      status: "pending",
    };
  }
  if (/[\s]/.test(trimmed)) {
    return {
      detail: "Bearer token must not contain whitespace.",
      label: "Bearer token is invalid.",
      mode: "bearer",
      status: "blocked",
    };
  }
  return {
    detail:
      "Token is locally well-formed; Check auth verifies it against the configured backend provider.",
    label: "Bearer token ready to check.",
    mode: "bearer",
    status: "ready",
  };
}
