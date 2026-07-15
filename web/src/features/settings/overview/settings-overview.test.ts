import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../../messages/en.json";

import type { AlertEventSummary } from "@/features/alerts/api";
import type { AlertSourceProfile } from "../alert-sources/types";
import type { DiagnosisToolTemplate } from "../diagnosis-tool-templates/types";
import type { GroupingPolicy } from "../grouping-policies/types";
import type { NotificationChannelProfile } from "../notification-channels/types";
import type { ReportWorkflowPolicy } from "../report-workflow-policies/types";
import type { ReportWorkflowSchedule } from "../report-workflow-schedules/types";
import {
  diagnosisAuthRoleMappingStatusDetail as diagnosisAuthRoleMappingStatusDetailWithTranslator,
  diagnosisAuthRoleMappingStatusReadiness as diagnosisAuthRoleMappingStatusReadinessWithTranslator,
  type DiagnosisAuthTranslator,
} from "@/features/diagnosis-room/auth-readiness";
import {
  alertIngestionStatus,
  buildWorkflowTopology,
  diagnosisAuthLiveProofFromBrowserSession,
  diagnosisAuthLiveProofFromProbeState as diagnosisAuthLiveProofFromProbeStateWithTranslator,
  diagnosisAuthProbeResultFromBrowserSessionStatus,
  diagnosisAuthProbeBrowserSessionBlockReason as diagnosisAuthProbeBrowserSessionBlockReasonWithTranslator,
  diagnosisAuthProbeCheckBlocked,
  diagnosisAuthProbeModeFromBackendStatus,
  diagnosisAuthProbeResultDetail,
  diagnosisAuthReadinessSummary as diagnosisAuthReadinessSummaryWithTranslator,
  diagnosisBrowserSessionNeedsIAMSignIn,
  diagnosisBrowserSessionStatusDetail as diagnosisBrowserSessionStatusDetailWithTranslator,
  diagnosisBrowserSessionStateFromResult,
  defaultDiagnosisAuthLiveProof,
  alertIngestionWebhookProofReadiness,
  autoDiagnosisProofHistoriesForAlerts,
  latestAutoDiagnosisProofHistory,
  latestAutoDiagnosisProofHistoryForSource,
  metricEvidenceConfigurationHref,
  settingsWeComAuthErrorDetail,
  settingsWeComAppCallbackGuidance,
  settingsWeComAppCallbackURL,
  settingsWeComBrowserSessionReadiness,
  settingsLDAPDirectorySearchGuidance,
  settingsOIDCBFFSetupReadiness as settingsOIDCBFFSetupReadinessWithTranslator,
  settingsCurrentRBACAccessSummary,
  settingsLocalAccessReadiness,
  settingsLDAPRoleMappingGuidance,
  settingsLDAPTransportGuidance,
  type DiagnosisAuthLiveProof,
  workflowIntegrationReadiness as workflowIntegrationReadinessWithTranslator,
  workflowLiveProofReadiness,
  workflowProofTargets,
  workflowTopologyActions,
} from "./settings-overview";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisAuth",
});

function bindAuthTranslator<TArgs extends unknown[], TResult>(
  fn: (...args: [...TArgs, DiagnosisAuthTranslator]) => TResult,
): (...args: TArgs) => TResult {
  return (...args) => fn(...args, tEn);
}

const diagnosisAuthLiveProofFromProbeState = bindAuthTranslator(
  diagnosisAuthLiveProofFromProbeStateWithTranslator,
);
const diagnosisAuthProbeBrowserSessionBlockReason = bindAuthTranslator(
  diagnosisAuthProbeBrowserSessionBlockReasonWithTranslator,
);
const diagnosisAuthReadinessSummary = bindAuthTranslator(
  diagnosisAuthReadinessSummaryWithTranslator,
);
const diagnosisBrowserSessionStatusDetail = bindAuthTranslator(
  diagnosisBrowserSessionStatusDetailWithTranslator,
);
const settingsOIDCBFFSetupReadiness = bindAuthTranslator(
  settingsOIDCBFFSetupReadinessWithTranslator,
);
const diagnosisAuthRoleMappingStatusDetail = (
  status: Parameters<
    typeof diagnosisAuthRoleMappingStatusDetailWithTranslator
  >[0],
  loading = false,
) => diagnosisAuthRoleMappingStatusDetailWithTranslator(status, tEn, loading);
const diagnosisAuthRoleMappingStatusReadiness = (
  status: Parameters<
    typeof diagnosisAuthRoleMappingStatusReadinessWithTranslator
  >[0],
  loading = false,
) =>
  diagnosisAuthRoleMappingStatusReadinessWithTranslator(status, tEn, loading);
const workflowIntegrationReadiness = (
  topology: Parameters<typeof workflowIntegrationReadinessWithTranslator>[0],
  diagnosisAuthProof?: Parameters<
    typeof workflowIntegrationReadinessWithTranslator
  >[2],
  localAccessReadiness?: Parameters<
    typeof workflowIntegrationReadinessWithTranslator
  >[3],
  currentRBACAccess?: Parameters<
    typeof workflowIntegrationReadinessWithTranslator
  >[4],
) =>
  workflowIntegrationReadinessWithTranslator(
    topology,
    tEn,
    diagnosisAuthProof,
    localAccessReadiness,
    currentRBACAccess,
  );

const timestamp = "2026-06-19T00:00:00Z";

describe("settings overview diagnosis auth status", () => {
  it("distinguishes local auth blocks from backend rejection", () => {
    expect(
      diagnosisAuthProbeResultDetail(
        {
          localFailure: "browser_session",
          message: "",
          status: "failed",
        },
        tEn,
      ),
    ).toBe(
      "Authentication check was not sent because the required browser session is not ready.",
    );
    expect(
      diagnosisAuthProbeResultDetail(
        {
          message: "upstream identity rejected",
          status: "failed",
        },
        tEn,
      ),
    ).toBe("upstream identity rejected");
  });

  it("resets diagnosis auth proof to pending browser-session readiness", () => {
    expect(defaultDiagnosisAuthLiveProof()).toEqual({
      detail: "",
      mode: "session",
      roles: [],
      status: "pending",
      subject: "",
    });
  });

  it("can reset diagnosis auth proof to pending bearer readiness", () => {
    expect(defaultDiagnosisAuthLiveProof("bearer")).toEqual({
      detail: "",
      mode: "bearer",
      roles: [],
      status: "pending",
      subject: "",
    });
  });

  it("can reset diagnosis auth proof to pending Enterprise WeChat readiness", () => {
    expect(defaultDiagnosisAuthLiveProof("wecom")).toEqual({
      detail: "",
      mode: "wecom",
      roles: [],
      status: "pending",
      subject: "",
    });
  });

  it("can reset diagnosis auth proof to pending browser-session readiness", () => {
    expect(defaultDiagnosisAuthLiveProof("session")).toEqual({
      detail: "",
      mode: "session",
      roles: [],
      status: "pending",
      subject: "",
    });
  });

  it("builds Enterprise WeChat app callback URL from the current console origin", () => {
    expect(settingsWeComAppCallbackURL("https://console.example.com")).toBe(
      "https://console.example.com/api/v1/diagnosis/wecom/app-callback",
    );
    expect(
      settingsWeComAppCallbackURL(
        "https://operator:secret@console.example.com",
      ),
    ).toBeNull();
  });

  it("documents Enterprise WeChat app message callback env guidance", () => {
    expect(settingsWeComAppCallbackGuidance()).toEqual([
      {
        detail:
          "Shared verification token configured on the Enterprise WeChat app message receiver. It is used with timestamp, nonce, and encrypted payload data to validate callback signatures.",
        envVar: "OPENCLARION_WECOM_CALLBACK_TOKEN",
        label: "Callback token",
        value: "Signature verification",
      },
      {
        detail:
          "Base64 EncodingAESKey configured on the same Enterprise WeChat app message receiver. OpenClarion uses it to decrypt URL verification echoes and encrypted XML callbacks.",
        envVar: "OPENCLARION_WECOM_CALLBACK_ENCODING_AES_KEY",
        label: "Encoding AES key",
        value: "Encrypted callback body",
      },
      {
        detail:
          "Receive ID checked after callback decryption. Leave unset only when the Enterprise WeChat Corp ID is the expected receive ID.",
        envVar: "OPENCLARION_WECOM_CALLBACK_RECEIVE_ID",
        label: "Receive ID",
        value: "Corp or app receive ID",
      },
    ]);
  });

  it("documents LDAP transport env guidance", () => {
    expect(settingsLDAPTransportGuidance()).toEqual([
      {
        detail:
          "Directory endpoint. Prefer ldaps://; ldap:// requires StartTLS unless plaintext is explicitly allowed for local development.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_URL",
        label: "LDAP URL",
        value: "Directory endpoint",
      },
      {
        detail:
          "Upgrades ldap:// connections before service bind, search, and user bind.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_START_TLS",
        label: "StartTLS",
        value: "Encrypted ldap:// transport",
      },
      {
        detail:
          "Explicit local-only plaintext allowance. Keep unset for production rollout.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_ALLOW_INSECURE_PLAINTEXT",
        label: "Plaintext allowance",
        value: "Development fallback",
      },
    ]);
  });

  it("documents LDAP directory search env guidance", () => {
    expect(settingsLDAPDirectorySearchGuidance()).toEqual([
      {
        detail:
          "Search root used to locate the operator entry before the user bind.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_BASE_DN",
        label: "Base DN",
        value: "Search root",
      },
      {
        detail:
          "Optional service account DN used for the initial directory search.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_BIND_DN",
        label: "Bind DN",
        value: "Search account",
      },
      {
        detail:
          "Optional service account password. Configure only together with the bind DN.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_BIND_PASSWORD",
        label: "Bind password",
        value: "Search credential",
      },
      {
        detail:
          "LDAP filter template. Use {username} for the escaped login name.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_USER_FILTER",
        label: "User filter",
        value: "User lookup",
      },
      {
        detail:
          "Optional attribute used as the OpenClarion subject. Leave unset to use the entry DN.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_SUBJECT_ATTRIBUTE",
        label: "Subject attribute",
        value: "Principal subject",
      },
    ]);
  });

  it("documents LDAP role mapping env guidance", () => {
    expect(settingsLDAPRoleMappingGuidance()).toEqual([
      {
        detail:
          "LDAP attribute read from the matched user entry to evaluate owner/admin mapping values.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_ROLE_ATTRIBUTE",
        label: "Role attribute",
        value: "Directory role source",
      },
      {
        detail:
          "Role attribute values that grant OpenClarion owner access.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_OWNER_ROLE_VALUES",
        label: "Owner role values",
        value: "Owner mapping",
      },
      {
        detail:
          "Role attribute values that grant OpenClarion admin access.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_ADMIN_ROLE_VALUES",
        label: "Admin role values",
        value: "Admin mapping",
      },
      {
        detail:
          "Fallback OpenClarion roles for every authenticated LDAP user. Use only for controlled pilots or local checks.",
        envVar: "OPENCLARION_DIAGNOSIS_LDAP_DEFAULT_ROLES",
        label: "Default roles",
        value: "Fallback role mapping",
      },
    ]);
  });

  it("keeps Enterprise WeChat session readiness blocked before sign-in", () => {
    const readiness = settingsWeComBrowserSessionReadiness({
      loading: false,
      message: "",
      status: { authenticated: false },
    });

    expect(readiness).toMatchObject({
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM before accepting OpenClarion browser-session proof.",
      label: "WeCom session required",
      status: "blocked",
      items: [
        { key: "session", status: "blocked", value: "Not signed in" },
        {
          key: "provider",
          status: "blocked",
          value: "Enterprise WeChat required",
        },
        { key: "role", status: "blocked", value: "RBAC pending" },
        { key: "subject", status: "unavailable", value: "Not available" },
      ],
    });
  });

  it("blocks Enterprise WeChat session readiness when the browser session came from LDAP", () => {
    const readiness = settingsWeComBrowserSessionReadiness({
      loading: false,
      message: "",
      status: {
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "ldap",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-ldap",
      },
    });

    expect(readiness).toMatchObject({
      checkedAt: "2026-06-22T10:00:00Z",
      label: "WeCom browser session replaced",
      status: "blocked",
    });
    expect(readiness.detail).toContain(
      "Enterprise WeChat browser login has been replaced by IAM OIDC.",
    );
    expect(readiness.items).toEqual([
      {
        detail:
          "OpenClarion no longer accepts Enterprise WeChat as a browser session provider.",
        key: "session",
        label: "Session",
        status: "blocked",
        value: "LDAP session",
      },
      {
        detail:
          "Use IAM OIDC sign-in for browser authentication. Enterprise WeChat remains available for app messages and notification delivery.",
        key: "provider",
        label: "Provider",
        status: "blocked",
        value: "LDAP",
      },
      {
        detail:
          "Identity is verified. Diagnosis room permissions are assigned in OpenClarion local RBAC, not by provider role mapping.",
        key: "role",
        label: "Role",
        status: "ready",
        value: "owner",
      },
      {
        detail: "Subject returned by the active browser session.",
        key: "subject",
        label: "Subject",
        status: "ready",
        value: "operator-ldap",
      },
    ]);
  });

  it("marks Enterprise WeChat browser session readiness replaced by IAM", () => {
    const readiness = settingsWeComBrowserSessionReadiness({
      loading: false,
      message: "",
      status: {
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: false,
        roles: ["viewer"],
        subject: "operator-1",
      },
    });

    expect(readiness).toMatchObject({
      checkedAt: "2026-06-22T10:00:00Z",
      label: "WeCom browser session replaced",
      status: "blocked",
    });
    expect(readiness.detail).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the active IAM browser session for OpenClarion authorization and keep Enterprise WeChat only for app messages and notifications.",
    );
    expect(readiness.items.find((item) => item.key === "provider")).toMatchObject({
      status: "blocked",
      value: "IAM OIDC",
    });
    expect(readiness.items.find((item) => item.key === "role")).toMatchObject({
      detail:
        "Identity is verified. Diagnosis room permissions are assigned in OpenClarion local RBAC, not by provider role mapping.",
      status: "ready",
      value: "viewer",
    });
  });

  it("keeps Enterprise WeChat browser readiness blocked for IAM sessions", () => {
    const readiness = settingsWeComBrowserSessionReadiness({
      loading: false,
      message: "",
      status: {
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      },
    });

    expect(readiness).toMatchObject({
      checkedAt: "2026-06-22T10:00:00Z",
      label: "WeCom browser session replaced",
      status: "blocked",
      items: [
        { key: "session", status: "blocked", value: "IAM OIDC session" },
        { key: "provider", status: "blocked", value: "IAM OIDC" },
        { key: "role", status: "ready", value: "owner" },
        { key: "subject", status: "ready", value: "operator-1" },
      ],
    });
    expect(readiness.detail).toContain(
      "Enterprise WeChat browser login has been replaced by IAM OIDC.",
    );
  });

  it("explains recoverable Enterprise WeChat login entry mismatches", () => {
    expect(settingsWeComAuthErrorDetail("wecom_entry_unavailable")).toEqual({
      description:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Sign in with IAM, then retry the diagnosis-room action.",
      message: "Enterprise WeChat login entry was not available.",
    });
    expect(settingsWeComAuthErrorDetail("wecom_login_failed")).toEqual({
      description:
        "OpenClarion could not start Enterprise WeChat login. Check the Enterprise WeChat app credentials, redirect URI, and provider endpoint reachability.",
      message: "Enterprise WeChat login could not be started.",
    });
    expect(settingsWeComAuthErrorDetail("wecom_role_unauthorized")).toEqual({
      description:
        "Enterprise WeChat identified the operator, but OpenClarion local RBAC did not grant diagnosis room access. OpenClarion cleared any previous browser session; assign the operator to the right local role, then sign in again.",
      message: "Enterprise WeChat identity is not authorized locally.",
    });
  });

  it("uses browser-session probes for LDAP backends", () => {
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "ldap",
      }),
    ).toBe("session");
  });

  it("uses bearer probes for static backends", () => {
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "static",
      }),
    ).toBe("bearer");
  });

  it("uses browser-session probes for OIDC backends", () => {
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "oidc",
      }),
    ).toBe("session");
  });

  it("prefers browser-session probes when OIDC is layered onto another auth provider", () => {
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "ldap",
        supported_modes: ["ldap", "oidc"],
      }),
    ).toBe("session");
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "static",
        supported_modes: ["static", "oidc"],
      }),
    ).toBe("session");
  });

  it("uses browser-session probes for OIDC and does not guess when backend auth is missing or unnamed", () => {
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "oidc",
      }),
    ).toBe("session");
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: false,
        mode: "none",
      }),
    ).toBeNull();
    expect(
      diagnosisAuthProbeModeFromBackendStatus({
        configured: true,
        mode: "unknown",
      }),
    ).toBeNull();
    expect(diagnosisAuthProbeModeFromBackendStatus(null)).toBeNull();
  });

  it("keeps IAM OIDC backends on browser-session rollout proof instead of blocking them", () => {
    expect(
      diagnosisAuthLiveProofFromProbeState({
        backendStatus: {
          configured: true,
          mode: "oidc",
          supportedModes: ["oidc"],
        },
        browserSession: null,
        checking: false,
        result: null,
        values: { authMode: "session" },
      }),
    ).toEqual({
      detail: "",
      mode: "session",
      roles: [],
      status: "needs_check",
      subject: "",
    });
  });

  it("blocks auth checks while input is unavailable, running, or backend mode is unsupported", () => {
    expect(
      diagnosisAuthProbeCheckBlocked({
        backendStatusAvailable: true,
        backendStatus: "needs_check",
        checking: false,
        inputStatus: "ready",
      }),
    ).toBe(false);
    expect(
      diagnosisAuthProbeCheckBlocked({
        backendStatusAvailable: true,
        backendStatus: "blocked",
        checking: false,
        inputStatus: "ready",
      }),
    ).toBe(true);
    expect(
      diagnosisAuthProbeCheckBlocked({
        backendStatusAvailable: true,
        backendStatus: "needs_check",
        checking: true,
        inputStatus: "ready",
      }),
    ).toBe(true);
    expect(
      diagnosisAuthProbeCheckBlocked({
        backendStatusAvailable: true,
        backendStatus: "pending",
        checking: false,
        inputStatus: "pending",
      }),
    ).toBe(true);
    expect(
      diagnosisAuthProbeCheckBlocked({
        backendStatusAvailable: false,
        backendStatus: "needs_check",
        checking: false,
        inputStatus: "ready",
      }),
    ).toBe(true);
  });

  it("summarizes backend role mapping metadata without upstream values", () => {
    expect(
      diagnosisAuthRoleMappingStatusDetail({
        configured: true,
        mode: "wecom",
        role_mapping: {
          admin_mapping_count: 1,
          configured: true,
          default_roles: ["owner"],
          owner_mapping_count: 2,
        },
      }),
    ).toBe(
      "Enterprise WeChat reports optional provider role metadata: 2 owner mappings, 1 admin mapping, and default roles: owner. Diagnosis room permissions are assigned through OpenClarion local RBAC.",
    );
    expect(
      diagnosisAuthRoleMappingStatusReadiness({
        configured: true,
        mode: "wecom",
        role_mapping: {
          admin_mapping_count: 0,
          configured: false,
          default_roles: [],
          owner_mapping_count: 0,
        },
      }),
    ).toEqual({
      color: "success",
      detail:
        "Enterprise WeChat has no provider role mapping configured. Identity-only authentication is accepted; diagnosis room permissions are assigned through OpenClarion local RBAC.",
      label: "Identity-only",
      status: "ready",
    });
    expect(
      diagnosisAuthRoleMappingStatusDetail({
        configured: true,
        mode: "ldap",
      }),
    ).toBe("Backend did not report role mapping metadata.");
    expect(
      diagnosisAuthRoleMappingStatusDetail({
        configured: false,
        mode: "none",
      }),
    ).toBe("Diagnosis auth is not configured in the running backend.");
  });

  it("converts an active IAM OIDC browser session into rollout proof", () => {
    expect(
      diagnosisAuthLiveProofFromBrowserSession({
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      }),
    ).toEqual({
      checkedAt: "2026-06-22T10:00:00Z",
      detail: "",
      mode: "session",
      roleAuthorized: true,
      roles: ["owner"],
      status: "verified",
      subject: "operator-1",
    });
  });

  it("keeps IAM OIDC browser sessions without provider roles verified for local RBAC", () => {
    expect(
      diagnosisAuthLiveProofFromBrowserSession({
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: false,
        roles: ["viewer"],
        subject: "operator-1",
      }),
    ).toMatchObject({
      detail: "",
      mode: "session",
      roleAuthorized: false,
      status: "verified",
      subject: "operator-1",
    });
    expect(
      diagnosisAuthLiveProofFromBrowserSession({ authenticated: false }),
    ).toBeNull();
  });

  it("uses LDAP-backed browser sessions as session-mode rollout proof", () => {
    expect(
      diagnosisAuthLiveProofFromBrowserSession({
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "ldap",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-ldap",
      }),
    ).toMatchObject({
      detail: "",
      mode: "ldap",
      status: "verified",
      subject: "operator-ldap",
    });
    expect(
      diagnosisAuthLiveProofFromProbeState({
        backendStatus: {
          configured: true,
          mode: "ldap",
          supportedModes: ["ldap"],
        },
        browserSession: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-ldap",
        },
        checking: false,
        result: null,
        values: { authMode: "session" },
      }),
    ).toMatchObject({
      mode: "session",
      status: "verified",
      subject: "operator-ldap",
    });
    expect(
      diagnosisAuthProbeResultFromBrowserSessionStatus({
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "ldap",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-ldap",
      }),
    ).toEqual({
      checkedAt: "2026-06-22T10:00:00Z",
      message: "",
      mode: "session",
      roleAuthorized: true,
      roles: ["owner"],
      status: "success",
      subject: "operator-ldap",
    });
  });

  it("uses IAM OIDC browser sessions as session-mode rollout proof", () => {
    expect(
      diagnosisAuthLiveProofFromBrowserSession({
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-iam",
      }),
    ).toEqual({
      checkedAt: "2026-06-22T10:00:00Z",
      detail: "",
      mode: "session",
      roleAuthorized: true,
      roles: ["owner"],
      status: "verified",
      subject: "operator-iam",
    });
    expect(
      diagnosisAuthProbeResultFromBrowserSessionStatus({
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-iam",
      }),
    ).toEqual({
      checkedAt: "2026-06-22T10:00:00Z",
      message: "",
      mode: "session",
      roleAuthorized: true,
      roles: ["owner"],
      status: "success",
      subject: "operator-iam",
    });
  });

  it("maps IAM OIDC browser-session fetch results into UI state", () => {
    expect(
      diagnosisBrowserSessionStateFromResult({
        ok: true,
        data: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      }),
    ).toEqual({
      loading: false,
      message: "",
      status: {
        authenticated: true,
        tenant_id: 1,
        tenant_key: "default",
        checked_at: "2026-06-22T10:00:00Z",
        mode: "oidc",
        role_authorized: true,
        roles: ["owner"],
        subject: "operator-1",
      },
    });
    expect(
      diagnosisBrowserSessionStateFromResult({
        ok: true,
        data: { authenticated: false },
      }),
    ).toEqual({
      loading: false,
      message: "",
      status: { authenticated: false },
    });
    expect(
      diagnosisBrowserSessionStateFromResult({
        ok: false,
        error: { message: "session check failed", status: 502 },
      }),
    ).toEqual({
      loading: false,
      message: "session check failed",
      status: null,
    });
  });

  it("shows browser session source in settings readiness detail", () => {
    expect(
      diagnosisBrowserSessionStatusDetail({
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      }),
    ).toBe(
      "Signed in as operator-1 using IAM OIDC. Roles: owner. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting. This session can be used for diagnosis-room identity rollout proof.",
    );
    expect(
      diagnosisBrowserSessionStatusDetail({
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "ldap",
          role_authorized: false,
          roles: ["viewer"],
          subject: "operator-ldap",
        },
      }),
    ).toBe(
      "Signed in as operator-ldap using LDAP. Roles: viewer. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting. This session can be used for diagnosis-room identity rollout proof.",
    );
  });

  it("shows IAM sign-in only when no browser session is active", () => {
    expect(
      diagnosisBrowserSessionNeedsIAMSignIn({
        loading: false,
        message: "",
        status: { authenticated: false },
      }),
    ).toBe(true);
    expect(
      diagnosisBrowserSessionNeedsIAMSignIn({
        loading: true,
        message: "Checking",
        status: null,
      }),
    ).toBe(false);
    expect(
      diagnosisBrowserSessionNeedsIAMSignIn({
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: [],
          subject: "operator-1",
        },
      }),
    ).toBe(false);
  });

  it("summarizes ready Enterprise WeChat auth prerequisites", () => {
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      },
      ldapSetupReadiness: {
        color: "warning",
        detail: "LDAP setup is not selected.",
        items: [],
        label: "LDAP setup needs review",
        status: "review",
      },
      mode: "wecom",
      rolloutReadiness: {
        checkedAt: "2026-06-22T10:00:00Z",
        detail:
          "Enterprise WeChat identity is verified against the running backend; diagnosis room permissions are enforced by local RBAC.",
        label: "Enterprise WeChat rollout proof ready.",
        mode: "wecom",
        roleAuthorized: true,
        roles: ["owner"],
        status: "ready",
        subject: "operator-1",
      },
      weComSetupReadiness: {
        color: "success",
        detail: "Enterprise WeChat setup is ready.",
        items: [],
        label: "Enterprise WeChat setup ready",
        status: "ready",
      },
    });

    expect(summary).toMatchObject({
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Use IAM sign-in for OpenClarion browser sessions.",
      label: "Operator auth blocked",
      status: "blocked",
      items: [
        { key: "backend", status: "ready", value: "Verified" },
        { key: "setup", status: "ready", value: "Callback + RBAC" },
        { key: "session", status: "blocked", value: "IAM OIDC session" },
        { key: "rollout", status: "blocked", value: "Use IAM OIDC" },
      ],
    });
  });

  it("summarizes IAM OIDC browser BFF setup readiness without exposing env values", () => {
    expect(
      settingsOIDCBFFSetupReadiness(
        {
          configured: true,
          mode: "oidc",
          oidc_bff: {
            browser_session_signing_key_configured: true,
            client_auth_method: "client_secret_basic",
            client_id_configured: true,
            client_secret_configured: true,
            configured: true,
            issuer_configured: true,
            missing: [],
            pkce_enabled: true,
            redirect_url_configured: true,
            scopes_include_openid: true,
            state_signing_key_configured: true,
            status: "ready",
          },
          supported_modes: ["oidc"],
        },
        false,
      ),
    ).toEqual({
      detail:
        "IAM OIDC browser sign-in prerequisites are configured in the console BFF and backend session issuer.",
      label: "IAM browser sign-in ready",
      status: "ready",
      value: "BFF ready",
    });

    expect(
      settingsOIDCBFFSetupReadiness(
        {
          configured: true,
          mode: "oidc",
          oidc_bff: {
            browser_session_signing_key_configured: false,
            client_auth_method: "client_secret_basic",
            client_id_configured: true,
            client_secret_configured: true,
            configured: false,
            issuer_configured: true,
            missing: ["session_signing_key"],
            pkce_enabled: true,
            redirect_url_configured: true,
            scopes_include_openid: true,
            state_signing_key_configured: true,
            status: "blocked",
          },
          supported_modes: ["oidc"],
        },
        false,
      ),
    ).toMatchObject({
      detail:
        "IAM OIDC browser sign-in is blocked: missing browser session signing key.",
      label: "IAM browser sign-in blocked",
      status: "blocked",
      value: "Missing browser session signing key",
    });

    expect(
      settingsOIDCBFFSetupReadiness(
        {
          configured: true,
          mode: "oidc",
          supported_modes: ["oidc"],
        },
        false,
      ),
    ).toMatchObject({
      label: "IAM browser sign-in blocked",
      status: "blocked",
      value: "BFF status missing",
    });

    expect(
      settingsOIDCBFFSetupReadiness(
        {
          configured: true,
          mode: "ldap",
          supported_modes: ["ldap"],
        },
        false,
      ),
    ).toBeUndefined();
  });

  it("uses IAM OIDC BFF readiness as the session-mode setup summary", () => {
    const oidcSetupReadiness = settingsOIDCBFFSetupReadiness(
      {
        configured: true,
        mode: "oidc",
        oidc_bff: {
          browser_session_signing_key_configured: false,
          client_auth_method: "auto",
          client_id_configured: true,
          client_secret_configured: false,
          configured: false,
          issuer_configured: true,
          missing: ["session_signing_key"],
          pkce_enabled: true,
          redirect_url_configured: false,
          scopes_include_openid: true,
          state_signing_key_configured: true,
          status: "blocked",
        },
        supported_modes: ["oidc"],
      },
      false,
    );
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: { authenticated: false },
      },
      ldapSetupReadiness: {
        color: "success",
        detail: "LDAP fallback should not be shown for IAM OIDC.",
        items: [],
        label: "LDAP setup ready",
        status: "ready",
      },
      mode: "session",
      oidcSetupReadiness,
      rolloutReadiness: {
        detail:
          "Run Check auth to verify the current OpenClarion browser session identity against the backend provider.",
        label: "Browser session check pending.",
        mode: "session",
        roles: [],
        status: "pending",
        subject: "",
      },
      weComSetupReadiness: {
        color: "default",
        detail: "Enterprise WeChat is not selected.",
        items: [],
        label: "Enterprise WeChat unavailable",
        status: "unavailable",
      },
    });

    expect(summary).toMatchObject({
      detail:
        "IAM OIDC browser sign-in is blocked: missing browser session signing key.",
      label: "Operator auth blocked",
      status: "blocked",
    });
    expect(summary.items.find((item) => item.key === "setup")).toMatchObject({
      label: "IAM OIDC",
      status: "blocked",
      value: "Missing browser session signing key",
    });
  });

  it("blocks operator auth readiness when local access control is not prepared", () => {
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: false,
          roles: [],
          subject: "operator-1",
        },
      },
      ldapSetupReadiness: {
        color: "success",
        detail: "IAM OIDC is selected.",
        items: [],
        label: "IAM setup ready",
        status: "ready",
      },
      localAccessReadiness: settingsLocalAccessReadiness({
        directoryDepartments: [],
        directoryUsers: [],
        rbacAssignments: [],
      }),
      mode: "session",
      rolloutReadiness: {
        checkedAt: "2026-06-22T10:00:00Z",
        detail:
          "IAM identity is verified against the running backend; diagnosis room permissions are enforced by local RBAC.",
        label: "IAM rollout proof ready.",
        mode: "session",
        roleAuthorized: false,
        roles: [],
        status: "ready",
        subject: "operator-1",
      },
      weComSetupReadiness: {
        color: "default",
        detail: "Enterprise WeChat is not selected.",
        items: [],
        label: "Enterprise WeChat unavailable",
        status: "unavailable",
      },
    });

    expect(summary).toMatchObject({
      detail:
        "Local IAM directory projection has no active users. Run directory sync before accepting diagnosis-room authorization readiness.",
      label: "Operator auth blocked",
      status: "blocked",
    });
    expect(summary.items.find((item) => item.key === "access")).toMatchObject({
      status: "blocked",
      value: "0 enabled",
    });
  });

  it("summarizes local directory projection and RBAC assignment readiness", () => {
    expect(
      settingsLocalAccessReadiness({
        directoryDepartments: [directoryDepartment()],
        directoryUsers: [directoryUser()],
        directorySyncRuns: [directorySyncRun()],
        rbacAssignments: [rbacAssignment()],
      }),
    ).toMatchObject({
      detail:
        "Local directory projection and RBAC assignments are ready for diagnosis-room authorization.",
      label: "Local access ready",
      status: "ready",
      items: [
        {
          key: "directory-sync",
          status: "ready",
          value: expect.stringContaining("Succeeded"),
        },
        { key: "directory-users", status: "ready", value: "1 active" },
        {
          key: "directory-departments",
          status: "ready",
          value: "1 departments",
        },
        { key: "rbac-assignments", status: "ready", value: "1 enabled" },
      ],
    });

    expect(
      settingsLocalAccessReadiness({
        directoryDepartments: [],
        directoryUsers: [directoryUser()],
        directorySyncRuns: [],
        rbacAssignments: [rbacAssignment()],
      }),
    ).toMatchObject({
      label: "Local access review",
      status: "review",
      items: [
        { key: "directory-sync", status: "review", value: "No runs" },
        { key: "directory-users", status: "ready" },
        { key: "directory-departments", status: "review" },
        { key: "rbac-assignments", status: "ready" },
      ],
    });

    expect(
      settingsLocalAccessReadiness({
        directoryDepartments: [directoryDepartment()],
        directoryUsers: [directoryUser({ active: false })],
        directorySyncRuns: [
          directorySyncRun({
            failure_code: "provider_unavailable",
            failure_message: "Directory provider returned an unavailable status.",
            status: "failed",
            synced_at: "2026-06-26T09:00:00Z",
          }),
          directorySyncRun({ synced_at: "2026-06-26T08:00:00Z" }),
        ],
        rbacAssignments: [rbacAssignment({ enabled: false })],
      }),
    ).toMatchObject({
      label: "Local access blocked",
      status: "blocked",
      items: [
        {
          key: "directory-sync",
          status: "blocked",
          value: expect.stringContaining("Failed"),
        },
        { key: "directory-users", status: "blocked", value: "0/1 active" },
        { key: "directory-departments", status: "ready" },
        { key: "rbac-assignments", status: "blocked", value: "0/1 enabled" },
      ],
    });
  });

  it("uses current RBAC access in operator rollout readiness", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [workflowSchedule()],
    });
    const localAccessReadiness = settingsLocalAccessReadiness({
      directoryDepartments: [directoryDepartment()],
      directoryUsers: [directoryUser({ subject: "operator-1" })],
      directorySyncRuns: [directorySyncRun()],
      rbacAssignments: [rbacAssignment()],
    });

    const signInAccess = settingsCurrentRBACAccessSummary(
      {
        fingerprint: "current",
        kind: "error",
        message: "authorization is required",
        status: 401,
      },
      false,
    );
    expect(
      workflowIntegrationReadiness(
        topology,
        verifiedDiagnosisAuthProof(),
        localAccessReadiness,
        signInAccess,
      ).items.find((item) => item.key === "local-access"),
    ).toMatchObject({
      actionLabel: "Sign in",
      status: "blocked",
      value: "Current access sign-in",
    });

    const readyAccess = settingsCurrentRBACAccessSummary(
      {
        allowed: {
          "directory-read": true,
          "directory-sync": true,
          "operations-read": true,
          "rbac-manage": true,
        },
        departmentKeys: ["dep-1"],
        directoryUsers: [directoryUser({ subject: "operator-1" })],
        fingerprint: "current",
        kind: "ready",
        subject: "operator-1",
      },
      false,
    );
    expect(
      workflowIntegrationReadiness(
        topology,
        verifiedDiagnosisAuthProof(),
        localAccessReadiness,
        readyAccess,
      ).items.find((item) => item.key === "local-access"),
    ).toMatchObject({
      actionLabel: "Review access",
      status: "ready",
      value: "operator-1",
    });
  });

  it("summarizes current RBAC access for the signed-in operator", () => {
    expect(
      settingsCurrentRBACAccessSummary(
        { fingerprint: "current", kind: "loading" },
        true,
      ),
    ).toMatchObject({
      label: "Current access pending",
      status: "pending",
    });

    expect(
      settingsCurrentRBACAccessSummary(
        {
          fingerprint: "current",
          kind: "error",
          message: "authorization is required",
          status: 401,
        },
        false,
      ),
    ).toMatchObject({
      label: "Current access sign-in",
      needsSignIn: true,
      status: "blocked",
    });

    const limitedAccess = settingsCurrentRBACAccessSummary(
      {
        allowed: {
          "directory-read": true,
          "directory-sync": false,
          "operations-read": true,
          "rbac-manage": false,
        },
        departmentKeys: ["dep-1"],
        directoryUsers: [directoryUser({ subject: "operator-1" })],
        fingerprint: "current",
        kind: "ready",
        subject: "operator-1",
      },
      false,
    );
    expect(limitedAccess).toMatchObject({
      label: "Current access limited",
      status: "review",
      subject: "operator-1",
    });
    expect(limitedAccess.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "directory-sync",
          status: "blocked",
          value: "Denied",
        }),
        expect.objectContaining({
          key: "rbac-manage",
          status: "blocked",
          value: "Denied",
        }),
      ]),
    );

    const missingProfileAccess = settingsCurrentRBACAccessSummary(
      {
        allowed: {
          "directory-read": true,
          "directory-sync": true,
          "operations-read": true,
          "rbac-manage": true,
        },
        departmentKeys: ["dep-1"],
        directoryUsers: [],
        fingerprint: "current",
        kind: "ready",
        subject: "operator-1",
      },
      false,
    );
    expect(missingProfileAccess).toMatchObject({
      label: "Current access review",
      status: "review",
    });
    expect(missingProfileAccess.items).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          key: "directory-profile",
          status: "review",
          value: "Not synced",
        }),
      ]),
    );

    expect(
      settingsCurrentRBACAccessSummary(
        {
          allowed: {
            "directory-read": true,
            "directory-sync": true,
            "operations-read": true,
            "rbac-manage": true,
          },
          departmentKeys: ["dep-1"],
          directoryUsers: [directoryUser({ subject: "operator-1" })],
          fingerprint: "current",
          kind: "ready",
          subject: "operator-1",
        },
        false,
      ),
    ).toMatchObject({
      label: "Current access ready",
      needsSignIn: false,
      status: "ready",
      subject: "operator-1",
    });
  });

  it("keeps Enterprise WeChat auth under review when one login entry is missing", () => {
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      },
      ldapSetupReadiness: {
        color: "warning",
        detail: "LDAP setup is not selected.",
        items: [],
        label: "LDAP setup needs review",
        status: "review",
      },
      mode: "wecom",
      rolloutReadiness: {
        checkedAt: "2026-06-22T10:00:00Z",
        detail:
          "Enterprise WeChat identity is verified against the running backend; diagnosis room permissions are enforced by local RBAC.",
        label: "Enterprise WeChat rollout proof ready.",
        mode: "wecom",
        roleAuthorized: true,
        roles: ["owner"],
        status: "ready",
        subject: "operator-1",
      },
      weComSetupReadiness: {
        color: "success",
        detail: "Enterprise WeChat setup is otherwise ready.",
        items: [],
        label: "Enterprise WeChat setup ready",
        status: "ready",
      },
    });

    expect(summary.status).toBe("blocked");
    expect(summary.detail).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use IAM sign-in for OpenClarion browser sessions.",
    );
    expect(summary.items.find((item) => item.key === "session")).toMatchObject({
      status: "blocked",
      value: "IAM OIDC session",
    });
  });

  it("keeps Enterprise WeChat auth under review when application launch URLs need review", () => {
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      },
      ldapSetupReadiness: {
        color: "warning",
        detail: "LDAP setup is not selected.",
        items: [],
        label: "LDAP setup needs review",
        status: "review",
      },
      mode: "wecom",
      rolloutReadiness: {
        checkedAt: "2026-06-22T10:00:00Z",
        detail:
          "Enterprise WeChat identity is verified against the running backend; diagnosis room permissions are enforced by local RBAC.",
        label: "Enterprise WeChat rollout proof ready.",
        mode: "wecom",
        roleAuthorized: true,
        roles: ["owner"],
        status: "ready",
        subject: "operator-1",
      },
      weComSetupReadiness: {
        color: "success",
        detail: "Enterprise WeChat setup is ready.",
        items: [],
        label: "Enterprise WeChat setup ready",
        status: "ready",
      },
    });

    expect(summary.status).toBe("blocked");
    expect(summary.detail).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use IAM sign-in for OpenClarion browser sessions.",
    );
    expect(summary.items.find((item) => item.key === "session")).toMatchObject({
      status: "blocked",
      value: "IAM OIDC session",
    });
  });

  it("surfaces Enterprise WeChat callback readiness gaps in the setup summary", () => {
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
      },
      ldapSetupReadiness: {
        color: "warning",
        detail: "LDAP setup is not selected.",
        items: [],
        label: "LDAP setup needs review",
        status: "review",
      },
      mode: "wecom",
      rolloutReadiness: {
        checkedAt: "2026-06-22T10:00:00Z",
        detail:
          "Enterprise WeChat identity is verified against the running backend; diagnosis room permissions are enforced by local RBAC.",
        label: "Enterprise WeChat rollout proof ready.",
        mode: "wecom",
        roleAuthorized: true,
        roles: ["owner"],
        status: "ready",
        subject: "operator-1",
      },
      weComSetupReadiness: {
        color: "warning",
        detail:
          "Enterprise WeChat can be used, but rollout still needs callback capability metadata reviewed before acceptance.",
        items: [
          {
            detail:
              "Backend did not report callback capability metadata. Verify the Enterprise WeChat callback URL and browser-session signing key before rollout.",
            key: "callback",
            label: "Callback session",
            status: "review",
          },
        ],
        label: "Enterprise WeChat setup needs review",
        status: "review",
      },
    });

    expect(summary.status).toBe("blocked");
    expect(summary.detail).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use IAM sign-in for OpenClarion browser sessions.",
    );
    expect(summary.items.find((item) => item.key === "setup")).toMatchObject({
      detail:
        "Backend did not report callback capability metadata. Verify the Enterprise WeChat callback URL and browser-session signing key before rollout.",
      status: "review",
      value: "Callback session review",
    });
    expect(summary.items.find((item) => item.key === "session")).toMatchObject({
      status: "blocked",
      value: "IAM OIDC session",
    });
  });

  it("blocks Enterprise WeChat readiness when the active session came from LDAP", () => {
    const summary = diagnosisAuthReadinessSummary({
      backendReadiness: {
        color: "success",
        detail: "Backend auth verified.",
        label: "Backend auth verified.",
        status: "verified",
      },
      browserSession: {
        loading: false,
        message: "",
        status: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "ldap",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-ldap",
        },
      },
      ldapSetupReadiness: {
        color: "success",
        detail: "LDAP setup is ready.",
        items: [],
        label: "LDAP setup ready",
        status: "ready",
      },
      mode: "wecom",
      rolloutReadiness: {
        checkedAt: "2026-06-22T10:00:00Z",
        detail:
          "Enterprise WeChat identity is verified against the running backend; diagnosis room permissions are enforced by local RBAC.",
        label: "Enterprise WeChat rollout proof ready.",
        mode: "wecom",
        roleAuthorized: true,
        roles: ["owner"],
        status: "ready",
        subject: "operator-1",
      },
      weComSetupReadiness: {
        color: "success",
        detail: "Enterprise WeChat setup is ready.",
        items: [],
        label: "Enterprise WeChat setup ready",
        status: "ready",
      },
    });

    expect(summary.status).toBe("blocked");
    expect(summary.label).toBe("Operator auth blocked");
    expect(summary.detail).toContain(
      "Enterprise WeChat browser login has been replaced by IAM OIDC.",
    );
    expect(summary.items.find((item) => item.key === "session")).toMatchObject({
      status: "blocked",
      value: "LDAP session",
    });
    expect(summary.items.find((item) => item.key === "rollout")).toMatchObject({
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Use IAM sign-in and local RBAC proof for OpenClarion authorization.",
      label: "Check auth proof",
      status: "blocked",
      value: "Use IAM OIDC",
    });
  });

  it("drops Enterprise WeChat rollout proof when the browser session is gone", () => {
    const backendStatus = {
      configured: true,
      mode: "ldap" as const,
      supportedModes: ["ldap" as const, "oidc" as const],
    };
    expect(
      diagnosisAuthLiveProofFromProbeState({
        backendStatus,
        browserSession: {
          authenticated: true,
          tenant_id: 1,
          tenant_key: "default",
          checked_at: "2026-06-22T10:00:00Z",
          mode: "oidc",
          role_authorized: true,
          roles: ["owner"],
          subject: "operator-1",
        },
        checking: false,
        result: null,
        values: { authMode: "wecom" },
      }),
    ).toMatchObject({
      mode: "wecom",
      status: "blocked",
      subject: "",
    });
    expect(
      diagnosisAuthLiveProofFromProbeState({
        backendStatus,
        browserSession: { authenticated: false },
        checking: false,
        result: null,
        values: { authMode: "wecom" },
      }),
    ).toEqual({
      detail: "",
      mode: "wecom",
      roles: [],
      status: "blocked",
      subject: "",
    });
  });

  it("blocks Enterprise WeChat probes from using an LDAP browser session", () => {
    expect(
      diagnosisAuthProbeBrowserSessionBlockReason(
        { authMode: "wecom" },
        {
          loading: false,
          status: {
            authenticated: true,
            tenant_id: 1,
            tenant_key: "default",
            checked_at: "2026-06-22T10:00:00Z",
            mode: "ldap",
            role_authorized: true,
            roles: ["owner"],
            subject: "operator-ldap",
          },
        },
      ),
    ).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and use the active IAM session for Check auth.",
    );

    expect(
      diagnosisAuthProbeBrowserSessionBlockReason(
        { authMode: "session" },
        {
          loading: false,
          status: {
            authenticated: true,
            tenant_id: 1,
            tenant_key: "default",
            checked_at: "2026-06-22T10:00:00Z",
            mode: "ldap",
            role_authorized: true,
            roles: ["owner"],
            subject: "operator-ldap",
          },
        },
      ),
    ).toBe("");
  });
});

describe("settings overview workflow topology", () => {
  it("links missing workflow policy to an automatic diagnosis workflow preset", () => {
    const topology = buildWorkflowTopology({
      alertSources: [],
      diagnosisToolTemplates: [],
      groupingPolicies: [],
      notificationChannels: [],
      workflowPolicies: [],
      workflowSchedules: [],
    });

    expect(workflowTopologyActions(topology)).toEqual([
      {
        detail:
          "Bind an Alertmanager source, grouping, and Enterprise WeChat delivery settings before replaying alert windows.",
        href: "/settings/report-workflow-policies?intent=create-auto-room-policy",
        key: "create-policy",
        priority: "high",
        title: "Create automatic diagnosis workflow",
      },
    ]);
  });

  it("blocks auto-room topology when the selected alert source is not Alertmanager", () => {
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Primary Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: prometheus.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(topology.status).toBe("blocked");
    expect(
      alertIngestionStatus(
        topology,
        topology.activeAlertTools.length,
        topology.metricTools.length,
      ),
    ).toBe("blocked");
    const alertmanagerAction = workflowTopologyActions(topology).find(
      (action) => action.key === "auto-room-alertmanager-source",
    );
    expect(alertmanagerAction?.href).toBe(
      "/settings/alert-sources?intent=alertmanager-source",
    );
    expect(alertmanagerAction).toMatchObject({
      detail:
        "Automatic diagnosis room starts require an enabled Alertmanager-compatible webhook source. Keep Thanos Rule for active-alert evidence and use Alertmanager for webhook intake.",
      title: "Bind an Alertmanager webhook source",
    });
    expect(
      workflowIntegrationReadiness(topology).items.find(
        (item) => item.key === "alertmanager-auto-room",
      )?.actionHref,
    ).toBe("/settings/alert-sources?intent=alertmanager-source");
    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      )?.actionHref,
    ).toBe(
      "/settings/alert-sources?intent=alertmanager-source",
    );
  });

  it("blocks auto-room topology when the report channel lacks diagnosis_close scope", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_range_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({ deliveryScopes: ["report"] }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(topology.status).toBe("blocked");
    const scopeAction = workflowTopologyActions(topology).find(
      (action) => action.key === "notification-channel-scope",
    );
    expect(scopeAction?.href).toBe(
      "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=3",
    );
  });

  it("blocks auto-room topology when no Enterprise WeChat channel is bound", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(topology.status).toBe("blocked");
    const bindAction = workflowTopologyActions(topology).find(
      (action) => action.key === "notification-channel-bind",
    );
    expect(bindAction?.href).toBe(
      "/settings/notification-channels?intent=report-close-channel&workflow_return=auto-room-enable&workflow_source_id=3",
    );
    expect(bindAction?.priority).toBe("high");
    expect(workflowLiveProofReadiness(topology)).toMatchObject({
      status: "blocked",
      items: [
        { key: "policy-replay", status: "blocked" },
        { key: "ai-diagnosis", status: "blocked" },
        { key: "diagnosis-auth", status: "pending", value: "Browser session not checked" },
        { key: "notification", status: "blocked", value: "No channel" },
        { key: "scheduled-trigger", status: "blocked" },
      ],
    });
  });

  it("blocks auto-room topology when a generic webhook is bound instead of WeCom", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
          kind: "webhook",
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(topology.status).toBe("blocked");
    expect(
      workflowTopologyActions(topology).find(
        (action) => action.key === "notification-channel-kind",
      ),
    ).toMatchObject({
      href: "/settings/notification-channels?intent=report-close-channel&workflow_return=auto-room-enable&workflow_source_id=3",
      priority: "high",
      title: "Switch AI delivery to Enterprise WeChat",
    });
    expect(workflowLiveProofReadiness(topology)).toMatchObject({
      status: "blocked",
      items: [
        { key: "policy-replay", status: "blocked" },
        { key: "ai-diagnosis", status: "blocked" },
        { key: "diagnosis-auth", status: "pending", value: "Browser session not checked" },
        { key: "notification", status: "blocked", value: "Operations webhook" },
        { key: "scheduled-trigger", status: "blocked" },
      ],
    });
  });

  it("marks a complete auto-room topology ready", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
          latestTestResults: notificationChannelAIProofs(),
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [workflowSchedule()],
    });

    expect(topology.status).toBe("ready");
    expect(
      alertIngestionStatus(
        topology,
        topology.activeAlertTools.length,
        topology.metricTools.length,
      ),
    ).toBe("ready");
    expect(
      workflowTopologyActions(topology).map((action) => action.key),
    ).toEqual(["impact-preview"]);
    expect(
      workflowProofTargets(topology).map((target) => [
        target.key,
        target.status,
        target.actionLabel,
      ]),
    ).toEqual([
      ["policy-replay", "ready", "Run Replay"],
      ["alertmanager-auto-diagnosis", "ready", "Open Proof Path"],
      ["scheduled-trigger", "ready", "Review Schedule"],
    ]);
    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      ),
    ).toMatchObject({
      actionHref:
        "/settings/report-workflow-policies?intent=alertmanager-auto-diagnosis-proof&source_id=3",
      detail: expect.stringContaining(
        "retained Alertmanager webhook proof",
      ),
      evidence: expect.arrayContaining([
        "Alertmanager webhook",
        "Assistant message",
        "Final conclusion",
        "Enterprise WeChat",
      ]),
      status: "ready",
    });
    expect(workflowLiveProofReadiness(topology)).toMatchObject({
      status: "pending",
      items: [
        { key: "policy-replay", status: "ready" },
        { key: "ai-diagnosis", status: "ready", value: "Auto room" },
        { key: "diagnosis-auth", status: "pending", value: "Browser session not checked" },
        { key: "notification", status: "ready", value: "Operations WeCom" },
        { key: "scheduled-trigger", status: "ready" },
      ],
    });
    expect(
      workflowLiveProofReadiness(topology, verifiedDiagnosisAuthProof()),
    ).toMatchObject({
      status: "ready",
      items: [
        { key: "policy-replay", status: "ready" },
        { key: "ai-diagnosis", status: "ready", value: "Auto room" },
        {
          key: "diagnosis-auth",
          status: "ready",
          value: "operator@example.test",
        },
        { key: "notification", status: "ready", value: "Operations WeCom" },
        { key: "scheduled-trigger", status: "ready" },
      ],
    });
    expect(
      workflowIntegrationReadiness(topology, verifiedDiagnosisAuthProof()),
    ).toMatchObject({
      status: "ready",
      items: [
        {
          key: "operator-auth",
          status: "ready",
          value: "operator@example.test",
        },
        {
          key: "enterprise-wechat",
          status: "ready",
          value: "Operations WeCom",
        },
        {
          key: "alertmanager-auto-room",
          status: "ready",
          value: "Primary Alertmanager",
        },
      ],
    });
  });

  it("keeps operator rollout in review when only bearer auth is verified", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
          latestTestResults: notificationChannelAIProofs(),
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [workflowSchedule()],
    });

    expect(
      workflowIntegrationReadiness(topology, {
        detail: "Backend diagnosis auth check succeeded.",
        mode: "bearer",
        roles: ["owner"],
        status: "verified",
        subject: "operator-static",
      }),
    ).toMatchObject({
      status: "review",
      items: [
        {
          key: "operator-auth",
          status: "review",
          value: "Bearer verified",
        },
        { key: "enterprise-wechat", status: "ready" },
        { key: "alertmanager-auto-room", status: "ready" },
      ],
    });
  });

  it("allows operator rollout identity proof before local RBAC authorization", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
          latestTestResults: notificationChannelAIProofs(),
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [workflowSchedule()],
    });

    expect(
      workflowIntegrationReadiness(topology, {
        detail: "Backend diagnosis auth check succeeded.",
        mode: "ldap",
        roleAuthorized: false,
        roles: ["viewer"],
        status: "verified",
        subject: "operator@example.test",
      }),
    ).toMatchObject({
      status: "ready",
      items: [
        { key: "operator-auth", status: "ready", value: "operator@example.test" },
        { key: "enterprise-wechat", status: "ready" },
        { key: "alertmanager-auto-room", status: "ready" },
      ],
    });
  });

  it("keeps auto-room live proof in review until Enterprise WeChat AI samples are tested", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [workflowSchedule()],
    });

    expect(
      workflowLiveProofReadiness(topology, verifiedDiagnosisAuthProof()),
    ).toMatchObject({
      status: "review",
      items: [
        { key: "policy-replay", status: "ready" },
        { key: "ai-diagnosis", status: "ready", value: "Auto room" },
        {
          key: "diagnosis-auth",
          status: "ready",
          value: "operator@example.test",
        },
        {
          key: "notification",
          status: "review",
          value: "Operations WeCom",
        },
        { key: "scheduled-trigger", status: "ready" },
      ],
    });
    expect(
      workflowLiveProofReadiness(
        topology,
        verifiedDiagnosisAuthProof(),
      ).items.find((item) => item.key === "notification")?.detail,
    ).toContain("AI diagnosis sample and Diagnosis close sample tests");
    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      ),
    ).toMatchObject({
      actionHref:
        "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=3",
      actionLabel: "Test Channel",
      detail: expect.stringContaining(
        "current AI diagnosis sample and diagnosis close sample tests",
      ),
      status: "review",
    });
    expect(
      workflowIntegrationReadiness(topology, verifiedDiagnosisAuthProof()),
    ).toMatchObject({
      status: "review",
      items: [
        { key: "operator-auth", status: "ready" },
        {
          actionHref:
            "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=3",
          key: "enterprise-wechat",
          status: "review",
          value: "Operations WeCom",
        },
        { key: "alertmanager-auto-room", status: "ready" },
      ],
    });
  });

  it("keeps Enterprise WeChat rollout blocked when auto-room scopes are missing", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: ["report", "diagnosis_consultation"],
          latestTestResults: notificationChannelAIProofs(),
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [workflowSchedule()],
    });

    expect(
      workflowIntegrationReadiness(topology, verifiedDiagnosisAuthProof()),
    ).toMatchObject({
      status: "blocked",
      items: [
        { key: "operator-auth", status: "ready" },
        {
          actionHref:
            "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=3",
          key: "enterprise-wechat",
          status: "blocked",
          value: "Operations WeCom",
        },
        { key: "alertmanager-auto-room", status: "ready" },
      ],
    });
    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      ),
    ).toMatchObject({
      actionHref:
        "/settings/notification-channels?channel_id=3&workflow_return=auto-room-enable&workflow_source_id=3",
      actionLabel: "Review Scopes",
      status: "blocked",
    });
  });

  it("links auto-diagnosis proof targets to the latest retained alert delivery history", () => {
    const topology = readyAutoDiagnosisTopology();
    const history = latestAutoDiagnosisProofHistory([
      alertEvent({
        id: 40,
        snapshotWorkflow: "ManualDiagnosis",
      }),
      alertEvent({
        id: 42,
        labels: { alertname: "HighLatency" },
        roomTimeline: [
          notificationTimelineEntry({
            content_kind: "assistant_message",
            content_sha256: "a".repeat(64),
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            occurred_at: "2026-06-19T00:01:00Z",
          }),
          notificationTimelineEntry({
            content_kind: "final_conclusion",
            content_sha256: "b".repeat(64),
            event_kind: "diagnosis_room.final_ready_notification_sent",
            occurred_at: "2026-06-19T00:02:00Z",
          }),
          notificationTimelineEntry({
            content_kind: "final_conclusion",
            content_sha256: "c".repeat(64),
            event_kind: "diagnosis_room.close_notification_sent",
            occurred_at: "2026-06-19T00:03:00Z",
          }),
        ],
      }),
    ]);

    expect(history).toMatchObject({
      alertID: 42,
      alertName: "HighLatency",
      coverage: { status: "ready" },
      href: "/diagnosis-room?evidence_snapshot_id=420&session_id=diagnosis-session-42#diagnosis-notification-timeline",
      roomSessionID: "diagnosis-session-42",
      snapshotID: 420,
    });
    expect(
      workflowProofTargets(topology, history).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      ),
    ).toMatchObject({
      actionHref:
        "/diagnosis-room?evidence_snapshot_id=420&session_id=diagnosis-session-42#diagnosis-notification-timeline",
      actionLabel: "Review Proof",
      detail: expect.stringContaining("HighLatency"),
      evidence: expect.arrayContaining([
        "alert #42",
        "snapshot #420",
        "AI delivery complete",
      ]),
      status: "ready",
    });
    expect(
      alertIngestionWebhookProofReadiness(topology, history),
    ).toMatchObject({
      href: "/diagnosis-room?evidence_snapshot_id=420&session_id=diagnosis-session-42#diagnosis-notification-timeline",
      status: "ready",
      value: "AI delivery complete",
    });
  });

  it("keeps auto-diagnosis proof scoped to the topology alert source", () => {
    const topology = readyAutoDiagnosisTopology();
    const alerts = [
      alertEvent({
        alertSourceProfileID: 9,
        id: 50,
        labels: { alertname: "SecondaryAlertmanagerAlert" },
        roomTimeline: [
          notificationTimelineEntry({
            content_kind: "assistant_message",
            content_sha256: "d".repeat(64),
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            occurred_at: "2026-06-19T00:04:00Z",
          }),
        ],
      }),
      alertEvent({
        alertSourceProfileID: 3,
        id: 51,
        labels: { alertname: "PrimaryAlertmanagerAlert" },
        roomTimeline: [
          notificationTimelineEntry({
            content_kind: "assistant_message",
            content_sha256: "e".repeat(64),
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            occurred_at: "2026-06-19T00:02:00Z",
          }),
        ],
      }),
    ];

    expect(latestAutoDiagnosisProofHistory(alerts)).toMatchObject({
      alertID: 50,
      alertSourceProfileID: 9,
    });
    expect(
      latestAutoDiagnosisProofHistoryForSource(
        alerts,
        topology.alertSource?.id ?? null,
      ),
    ).toMatchObject({
      alertID: 51,
      alertSourceProfileID: 3,
    });
    expect(
      workflowProofTargets(
        topology,
        latestAutoDiagnosisProofHistoryForSource(
          alerts.slice(0, 1),
          topology.alertSource?.id ?? null,
        ),
      ).find((target) => target.key === "alertmanager-auto-diagnosis"),
    ).toMatchObject({
      actionLabel: "Open Proof Path",
      actionHref:
        "/settings/report-workflow-policies?intent=alertmanager-auto-diagnosis-proof&source_id=3",
      status: "ready",
    });
    expect(
      alertIngestionWebhookProofReadiness(
        topology,
        latestAutoDiagnosisProofHistoryForSource(
          alerts.slice(0, 1),
          topology.alertSource?.id ?? null,
        ),
      ),
    ).toMatchObject({
      href: "/settings/report-workflow-policies?intent=alertmanager-auto-diagnosis-proof&source_id=3",
      status: "review",
      value: "missing",
    });
  });

  it("surfaces the newest failed auto-diagnosis delivery history as blocked", () => {
    const topology = readyAutoDiagnosisTopology();
    const history = latestAutoDiagnosisProofHistory([
      alertEvent({
        id: 41,
        roomTimeline: [
          notificationTimelineEntry({
            content_kind: "assistant_message",
            content_sha256: "a".repeat(64),
            event_kind: "diagnosis_room.assistant_turn_notification_sent",
            occurred_at: "2026-06-19T00:01:00Z",
          }),
          notificationTimelineEntry({
            content_kind: "final_conclusion",
            content_sha256: "b".repeat(64),
            event_kind: "diagnosis_room.final_ready_notification_sent",
            occurred_at: "2026-06-19T00:02:00Z",
          }),
          notificationTimelineEntry({
            content_kind: "final_conclusion",
            content_sha256: "c".repeat(64),
            event_kind: "diagnosis_room.close_notification_sent",
            occurred_at: "2026-06-19T00:03:00Z",
          }),
        ],
      }),
      alertEvent({
        id: 43,
        roomTimeline: [
          notificationTimelineEntry({
            event_kind: "diagnosis_room.final_ready_notification_sent",
            occurred_at: "2026-06-19T00:04:00Z",
            provider_status: "failed",
          }),
        ],
      }),
    ]);

    expect(history).toMatchObject({
      alertID: 43,
      coverage: { status: "blocked" },
    });
    expect(
      workflowProofTargets(topology, history).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      ),
    ).toMatchObject({
      actionLabel: "Review Delivery",
      status: "blocked",
    });
  });

  it("lists recent auto-diagnosis proof histories in reverse occurrence order", () => {
    expect(
      autoDiagnosisProofHistoriesForAlerts(
        [
          alertEvent({
            id: 44,
            roomTimeline: [
              notificationTimelineEntry({
                content_kind: "assistant_message",
                content_sha256: "a".repeat(64),
                event_kind: "diagnosis_room.assistant_turn_notification_sent",
                occurred_at: "2026-06-19T00:05:00Z",
              }),
            ],
          }),
          alertEvent({
            id: 45,
            roomTimeline: [
              notificationTimelineEntry({
                content_kind: "assistant_message",
                content_sha256: "b".repeat(64),
                event_kind: "diagnosis_room.assistant_turn_notification_sent",
                occurred_at: "2026-06-19T00:07:00Z",
              }),
            ],
          }),
          alertEvent({
            id: 46,
            snapshotWorkflow: "ManualDiagnosis",
          }),
          alertEvent({
            id: 47,
            roomTimeline: [
              notificationTimelineEntry({
                content_kind: "assistant_message",
                content_sha256: "c".repeat(64),
                event_kind: "diagnosis_room.assistant_turn_notification_sent",
                occurred_at: "2026-06-19T00:06:00Z",
              }),
            ],
          }),
        ],
        2,
      ).map((history) => history.alertID),
    ).toEqual([45, 47]);
  });

  it("keeps suggest-room topology in review because it still needs operator handoff", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({ deliveryScopes: ["report"] }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "suggest_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(topology.status).toBe("review");
    expect(
      alertIngestionStatus(
        topology,
        topology.activeAlertTools.length,
        topology.metricTools.length,
      ),
    ).toBe("review");
    const autoRoomAction = workflowTopologyActions(topology).find(
      (action) => action.key === "auto-room-follow-up",
    );
    expect(autoRoomAction?.href).toBe(
      "/settings/report-workflow-policies?intent=auto-room-follow-up&source_id=3",
    );
    expect(workflowLiveProofReadiness(topology)).toMatchObject({
      status: "review",
      items: [
        { key: "policy-replay", status: "review" },
        { key: "ai-diagnosis", status: "review", value: "Operator handoff" },
        { key: "diagnosis-auth", status: "pending", value: "Browser session not checked" },
        { key: "notification", status: "ready" },
        { key: "scheduled-trigger", status: "pending" },
      ],
    });
  });

  it("links missing diagnosis tool actions to launch intents", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [
        alertmanager,
        alertSource({ id: 5, kind: "prometheus", name: "Metrics Prometheus" }),
      ],
      diagnosisToolTemplates: [],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });
    const actions = workflowTopologyActions(topology);

    expect(
      actions.find((action) => action.key === "active-alert-tool")?.href,
    ).toBe(
      "/settings/diagnosis-tool-templates?intent=active-alert-tool&source_id=3",
    );
    expect(
      actions.find((action) => action.key === "metric-evidence-tool")?.href,
    ).toBe(
      "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=5",
    );
  });

  it("uses the single metric source for rollout and proof tool actions", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(
      workflowIntegrationReadiness(topology).items.find(
        (item) => item.key === "alertmanager-auto-room",
      )?.actionHref,
    ).toBe(
      "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=5",
    );
    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      )?.actionHref,
    ).toBe(
      "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=5",
    );
    expect(metricEvidenceConfigurationHref(topology)).toBe(
      "/settings/diagnosis-tool-templates?intent=metric-evidence-tool&source_id=5",
    );
  });

  it("keeps metric evidence source selection open when multiple enabled metric sources exist", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [
        alertmanager,
        alertSource({ id: 5, kind: "prometheus", name: "Primary Metrics" }),
        alertSource({ id: 6, kind: "prometheus", name: "Secondary Metrics" }),
      ],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(
      workflowTopologyActions(topology).find(
        (action) => action.key === "metric-evidence-tool",
      )?.href,
    ).toBe("/settings/diagnosis-tool-templates?intent=metric-evidence-tool");
    expect(
      workflowIntegrationReadiness(topology).items.find(
        (item) => item.key === "alertmanager-auto-room",
      )?.actionHref,
    ).toBe("/settings/diagnosis-tool-templates?intent=metric-evidence-tool");
    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "alertmanager-auto-diagnosis",
      )?.actionHref,
    ).toBe("/settings/diagnosis-tool-templates?intent=metric-evidence-tool");
  });

  it("links missing metric evidence to existing disabled metric sources before creating another source", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [
        alertmanager,
        alertSource({
          enabled: false,
          id: 5,
          kind: "prometheus",
          name: "Draft Metrics",
        }),
      ],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });
    const actions = workflowTopologyActions(topology);

    expect(
      actions.find((action) => action.key === "enable-prometheus-metric-source")
        ?.href,
    ).toBe("/settings/alert-sources");
    expect(
      actions.find((action) => action.key === "thanos-metric-source"),
    ).toBeUndefined();
    expect(
      actions.find((action) => action.key === "metric-evidence-tool"),
    ).toBeUndefined();
    expect(metricEvidenceConfigurationHref(topology)).toBe(
      "/settings/alert-sources",
    );
  });

  it("links missing metric evidence to a Thanos source preset when no metric source exists", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });
    const actions = workflowTopologyActions(topology);

    expect(
      actions.find((action) => action.key === "thanos-metric-source")?.href,
    ).toBe("/settings/alert-sources?intent=thanos-source");
    expect(
      actions.find((action) => action.key === "metric-evidence-tool"),
    ).toBeUndefined();
    expect(metricEvidenceConfigurationHref(topology)).toBe(
      "/settings/alert-sources?intent=thanos-source",
    );
  });

  it("does not treat Thanos Rule active-alert sources as metric evidence sources", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const thanosRule = alertSource({
      id: 5,
      kind: "prometheus",
      labels: { role: "alert-intake", source: "thanos-rule" },
      name: "Thanos Rule active alerts",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, thanosRule],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });
    const actions = workflowTopologyActions(topology);

    expect(topology.metricSources).toEqual([]);
    expect(
      actions.find((action) => action.key === "thanos-metric-source")?.href,
    ).toBe("/settings/alert-sources?intent=thanos-source");
    expect(
      actions.find((action) => action.key === "metric-evidence-tool"),
    ).toBeUndefined();
  });

  it("does not count metric templates bound to Thanos Rule active-alert sources", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const thanosRule = alertSource({
      id: 5,
      kind: "prometheus",
      labels: { role: "alert-intake", source: "thanos-rule" },
      name: "Thanos Rule active alerts",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, thanosRule],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: thanosRule.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(topology.metricTools).toEqual([]);
    expect(workflowTopologyActions(topology)[0]?.key).toBe(
      "thanos-metric-source",
    );
  });

  it("links missing grouping policy to a default grouping preset", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager],
      diagnosisToolTemplates: [],
      groupingPolicies: [],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });
    const groupingAction = workflowTopologyActions(topology).find(
      (action) => action.key === "grouping",
    );

    expect(groupingAction?.href).toBe(
      "/settings/grouping-policies?intent=default-alert-grouping",
    );
  });

  it("keeps disabled grouping policy actions on the grouping policy page", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager],
      diagnosisToolTemplates: [],
      groupingPolicies: [groupingPolicy({ enabled: false })],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });
    const groupingAction = workflowTopologyActions(topology).find(
      (action) => action.key === "grouping",
    );

    expect(groupingAction?.href).toBe("/settings/grouping-policies");
  });

  it("marks scheduled proof pending when no schedule is configured", () => {
    const alertmanager = alertSource({
      id: 3,
      kind: "alertmanager",
      name: "Primary Alertmanager",
    });
    const prometheus = alertSource({
      id: 5,
      kind: "prometheus",
      name: "Metrics Prometheus",
    });
    const topology = buildWorkflowTopology({
      alertSources: [alertmanager, prometheus],
      diagnosisToolTemplates: [
        diagnosisToolTemplate({
          alertSourceProfileID: alertmanager.id,
          id: 10,
          tool: "active_alerts",
        }),
        diagnosisToolTemplate({
          alertSourceProfileID: prometheus.id,
          id: 11,
          tool: "metric_query",
        }),
      ],
      groupingPolicies: [groupingPolicy()],
      notificationChannels: [
        notificationChannel({
          deliveryScopes: [
            "report",
            "diagnosis_consultation",
            "diagnosis_close",
          ],
        }),
      ],
      workflowPolicies: [
        workflowPolicy({
          alertSourceProfileID: alertmanager.id,
          diagnosisFollowUp: "auto_room",
        }),
      ],
      workflowSchedules: [],
    });

    expect(
      workflowProofTargets(topology).find(
        (target) => target.key === "scheduled-trigger",
      ),
    ).toMatchObject({
      actionHref:
        "/settings/report-workflow-schedules?intent=create-schedule&policy_id=1",
      actionLabel: "Create Schedule",
      status: "pending",
    });
    expect(
      workflowTopologyActions(topology).find(
        (action) => action.key === "create-schedule",
      )?.href,
    ).toBe(
      "/settings/report-workflow-schedules?intent=create-schedule&policy_id=1",
    );
  });
});

function alertSource({
  id = 3,
  kind = "alertmanager",
  labels = {},
  name = "Alertmanager",
  enabled = true,
}: {
  enabled?: boolean;
  id?: number;
  kind?: AlertSourceProfile["kind"];
  labels?: AlertSourceProfile["labels"];
  name?: string;
} = {}): AlertSourceProfile {
  return {
    auth_mode: "none",
    base_url: "https://example.invalid",
    created_at: timestamp,
    enabled,
    id,
    kind,
    labels,
    name,
    secret_ref: "",
    updated_at: timestamp,
  };
}

function readyAutoDiagnosisTopology() {
  const alertmanager = alertSource({
    id: 3,
    kind: "alertmanager",
    name: "Primary Alertmanager",
  });
  const prometheus = alertSource({
    id: 5,
    kind: "prometheus",
    name: "Metrics Prometheus",
  });
  return buildWorkflowTopology({
    alertSources: [alertmanager, prometheus],
    diagnosisToolTemplates: [
      diagnosisToolTemplate({
        alertSourceProfileID: alertmanager.id,
        id: 10,
        tool: "active_alerts",
      }),
      diagnosisToolTemplate({
        alertSourceProfileID: prometheus.id,
        id: 11,
        tool: "metric_query",
      }),
    ],
    groupingPolicies: [groupingPolicy()],
    notificationChannels: [
      notificationChannel({
        deliveryScopes: ["report", "diagnosis_consultation", "diagnosis_close"],
        latestTestResults: notificationChannelAIProofs(),
      }),
    ],
    workflowPolicies: [
      workflowPolicy({
        alertSourceProfileID: alertmanager.id,
        diagnosisFollowUp: "auto_room",
      }),
    ],
    workflowSchedules: [workflowSchedule()],
  });
}

function alertEvent({
  alertSourceProfileID = 3,
  id,
  labels = { alertname: `Alert${id}` },
  roomTimeline = [],
  snapshotWorkflow = "AlertmanagerWebhookAutoDiagnosis",
}: {
  alertSourceProfileID?: number;
  id: number;
  labels?: AlertEventSummary["labels"];
  roomTimeline?: NonNullable<
    AlertEventSummary["linked_evidence_snapshots"][number]["diagnosis_rooms"][number]["notification_timeline"]
  >;
  snapshotWorkflow?: string;
}): AlertEventSummary {
  return {
    alert_source_profile_id: alertSourceProfileID,
    annotations: { summary: `Alert ${id}` },
    canonical_fingerprint: `sha256:${id}`,
    created_at: "2026-06-19T00:00:00Z",
    ends_at: null,
    id,
    labels,
    linked_evidence_snapshots: [
      {
        alert_group_id: id,
        created_at: "2026-06-19T00:00:00Z",
        created_by_workflow: snapshotWorkflow,
        diagnosis_rooms: [diagnosisRoom(id, roomTimeline)],
        digest: `sha256:${id}`,
        id: id * 10,
        status: "complete",
      },
    ],
    source: "alertmanager",
    source_fingerprint: `alertmanager:${id}`,
    starts_at: "2026-06-19T00:00:00Z",
    status: "firing",
  };
}

function diagnosisRoom(
  id: number,
  notificationTimeline: NonNullable<
    AlertEventSummary["linked_evidence_snapshots"][number]["diagnosis_rooms"][number]["notification_timeline"]
  >,
): AlertEventSummary["linked_evidence_snapshots"][number]["diagnosis_rooms"][number] {
  return {
    approval_mode: "single",
    chat_session_id: id * 100,
    close_reason: "",
    closed_at: null,
    created_at: "2026-06-19T00:00:00Z",
    diagnosis_task_id: id * 1000,
    evidence_snapshot_id: id * 10,
    last_activity_at: "2026-06-19T00:05:00Z",
    latest_conclusion: undefined,
    latest_progress: undefined,
    notification_timeline: notificationTimeline,
    room_status: "open",
    run_id: `run-${id}`,
    session_id: `diagnosis-session-${id}`,
    started_at: "2026-06-19T00:00:00Z",
    task_status: "running",
    turn_count: 1,
    updated_at: "2026-06-19T00:05:00Z",
    workflow_id: `diagnosis-room-diagnosis-session-${id}`,
    workflow_visibility: { status: "running" },
  };
}

function notificationTimelineEntry(
  overrides: Partial<
    NonNullable<
      AlertEventSummary["linked_evidence_snapshots"][number]["diagnosis_rooms"][number]["notification_timeline"]
    >[number]
  >,
): NonNullable<
  AlertEventSummary["linked_evidence_snapshots"][number]["diagnosis_rooms"][number]["notification_timeline"]
>[number] {
  return {
    event_kind: "diagnosis_room.assistant_turn_notification_sent",
    occurred_at: "2026-06-19T00:01:00Z",
    provider_status: "delivered",
    ...overrides,
  };
}

function diagnosisToolTemplate({
  alertSourceProfileID,
  id,
  tool,
}: {
  alertSourceProfileID: number;
  id: number;
  tool: DiagnosisToolTemplate["tool"];
}): DiagnosisToolTemplate {
  return {
    alert_source_profile_id: alertSourceProfileID,
    created_at: timestamp,
    default_limit: 5,
    default_step_seconds: tool === "metric_range_query" ? 60 : 0,
    default_window_seconds: tool === "metric_range_query" ? 300 : 0,
    disabled_at: null,
    enabled: true,
    enabled_at: timestamp,
    id,
    max_window_seconds: tool === "metric_range_query" ? 1800 : 0,
    name: `Tool ${id}`,
    query_template: tool === "active_alerts" ? "" : "up",
    tool,
    updated_at: timestamp,
  };
}

function groupingPolicy({
  enabled = true,
}: { enabled?: boolean } = {}): GroupingPolicy {
  return {
    created_at: timestamp,
    dimension_keys: ["alertname", "service"],
    enabled,
    id: 1,
    name: "Default grouping",
    severity_key: "severity",
    source_filter: [],
    updated_at: timestamp,
  };
}

function notificationChannel({
  deliveryScopes,
  enabled = true,
  kind = "wecom",
  latestTestResults = [],
}: {
  deliveryScopes: NotificationChannelProfile["delivery_scopes"];
  enabled?: boolean;
  kind?: NotificationChannelProfile["kind"];
  latestTestResults?: NotificationChannelProfile["latest_test_results"];
}): NotificationChannelProfile {
  return {
    created_at: timestamp,
    delivery_scopes: deliveryScopes,
    enabled,
    id: 3,
    kind,
    labels: {},
    latest_test_results: latestTestResults,
    name: kind === "wecom" ? "Operations WeCom" : "Operations webhook",
    secret_ref:
      kind === "wecom"
        ? "secret/example/ops-wecom"
        : "secret/example/ops-webhook",
    updated_at: timestamp,
  };
}

function notificationChannelAIProofs(): NotificationChannelProfile["latest_test_results"] {
  return [
    notificationChannelTestResult("ai_diagnosis_sample"),
    notificationChannelTestResult("diagnosis_close_sample"),
  ];
}

function notificationChannelTestResult(
  contentKind: NonNullable<
    NotificationChannelProfile["latest_test_results"][number]["content_kind"]
  >,
): NotificationChannelProfile["latest_test_results"][number] {
  return {
    channel_id: 3,
    checked_at: timestamp,
    content_kind: contentKind,
    content_sha256: "a".repeat(64),
    kind: "wecom",
    message: "Notification channel test delivery succeeded.",
    provider_message_id: `wecom-${contentKind}`,
    provider_status: "delivered",
    reason_code: "ok",
    status: "success",
  };
}

type LocalAccessReadinessInput = Parameters<
  typeof settingsLocalAccessReadiness
>[0];

function directoryUser(
  overrides: Partial<LocalAccessReadinessInput["directoryUsers"][number]> = {},
): LocalAccessReadinessInput["directoryUsers"][number] {
  return {
    active: true,
    subject: "operator-1",
    ...overrides,
  } as LocalAccessReadinessInput["directoryUsers"][number];
}

function directoryDepartment(
  overrides: Partial<LocalAccessReadinessInput["directoryDepartments"][number]> = {},
): LocalAccessReadinessInput["directoryDepartments"][number] {
  return {
    external_id: "dep-1",
    name: "Operations",
    ...overrides,
  } as LocalAccessReadinessInput["directoryDepartments"][number];
}

function directorySyncRun(
  overrides: Partial<
    NonNullable<LocalAccessReadinessInput["directorySyncRuns"]>[number]
  > = {},
): NonNullable<LocalAccessReadinessInput["directorySyncRuns"]>[number] {
  return {
    created_at: "2026-06-26T08:00:00Z",
    department_pages: 1,
    departments_upserted: 1,
    failure_code: "",
    failure_message: "",
    id: 1,
    page_size: 100,
    provider: "ops_iam",
    status: "succeeded",
    synced_at: "2026-06-26T08:00:00Z",
    user_pages: 1,
    users_upserted: 1,
    ...overrides,
  } as NonNullable<LocalAccessReadinessInput["directorySyncRuns"]>[number];
}

function rbacAssignment(
  overrides: Partial<LocalAccessReadinessInput["rbacAssignments"][number]> = {},
): LocalAccessReadinessInput["rbacAssignments"][number] {
  return {
    enabled: true,
    role: "responder",
    scope_kind: "global",
    scope_key: "",
    subject_key: "operator-1",
    subject_kind: "user",
    ...overrides,
  } as LocalAccessReadinessInput["rbacAssignments"][number];
}

function verifiedDiagnosisAuthProof(): DiagnosisAuthLiveProof {
  return {
    detail: "Backend diagnosis auth check succeeded.",
    mode: "ldap",
    roles: ["owner"],
    status: "verified",
    subject: "operator@example.test",
  };
}

function workflowPolicy({
  alertSourceProfileID,
  diagnosisFollowUp,
  enabled = true,
}: {
  alertSourceProfileID: number;
  diagnosisFollowUp: ReportWorkflowPolicy["diagnosis_follow_up"];
  enabled?: boolean;
}): ReportWorkflowPolicy {
  return {
    alert_source_profile_id: alertSourceProfileID,
    created_at: timestamp,
    diagnosis_follow_up: diagnosisFollowUp,
    disabled_at: null,
    enabled,
    enabled_at: enabled ? timestamp : null,
    grouping_policy_id: 1,
    id: 1,
    name: "Default workflow",
    report_notification_channel_profile_id: 3,
    report_scenario: "single_alert",
    trigger_mode: "manual_replay",
    updated_at: timestamp,
  };
}

function workflowSchedule(): ReportWorkflowSchedule {
  return {
    cadence: "interval",
    calendar_day_of_month: 0,
    calendar_day_of_week: 0,
    calendar_hour: 0,
    calendar_minute: 0,
    catchup_window_seconds: 3600,
    created_at: timestamp,
    disabled_at: null,
    enabled: true,
    enabled_at: timestamp,
    id: 1,
    interval_seconds: 3600,
    name: "Hourly replay",
    offset_seconds: 0,
    replay_delay_seconds: 300,
    replay_limit: 10000,
    replay_window_seconds: 3600,
    report_workflow_policy_id: 1,
    temporal_schedule_id: "openclarion-report-policy-1-hourly",
    updated_at: timestamp,
  };
}
