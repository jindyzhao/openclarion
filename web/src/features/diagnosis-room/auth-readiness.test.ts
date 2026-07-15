import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";

import {
  diagnosisAuthActionBlockReason as diagnosisAuthActionBlockReasonWithTranslator,
  diagnosisAuthBackendCredentialListLabel as diagnosisAuthBackendCredentialListLabelWithTranslator,
  diagnosisAuthBackendModeDisplayItems as diagnosisAuthBackendModeDisplayItemsWithTranslator,
  diagnosisAuthBackendModeListLabel as diagnosisAuthBackendModeListLabelWithTranslator,
  diagnosisAuthBackendReadiness as diagnosisAuthBackendReadinessWithTranslator,
  diagnosisAuthBackendStatusModes,
  diagnosisAuthBackendVerified,
  diagnosisAuthBrowserSessionAuthenticatedSummary as diagnosisAuthBrowserSessionAuthenticatedSummaryWithTranslator,
  diagnosisAuthBrowserSessionBlockReason as diagnosisAuthBrowserSessionBlockReasonWithTranslator,
  diagnosisAuthBrowserSessionDisplaySummary as diagnosisAuthBrowserSessionDisplaySummaryWithTranslator,
  diagnosisAuthBrowserSessionShouldClearAfterError,
  diagnosisAuthCheckSuccessFeedback as diagnosisAuthCheckSuccessFeedbackWithTranslator,
  diagnosisAuthWeComSetupReadiness as diagnosisAuthWeComSetupReadinessWithTranslator,
  diagnosisAuthCheckBlockReason as diagnosisAuthCheckBlockReasonWithTranslator,
  diagnosisAuthCoercedMode,
  diagnosisAutoBrowserSessionConnectionPlan,
  diagnosisAutoBrowserSessionCreateRoomPlan,
  diagnosisAutoBrowserSessionAuthCheckPlan,
  diagnosisAuthInputFieldsChanged,
  diagnosisAuthInputReadiness as diagnosisAuthInputReadinessWithTranslator,
  diagnosisAuthLDAPBrowserSessionPromotionNotice as diagnosisAuthLDAPBrowserSessionPromotionNoticeWithTranslator,
  diagnosisAuthLDAPSetupReadiness as diagnosisAuthLDAPSetupReadinessWithTranslator,
  diagnosisAuthModeOptions as diagnosisAuthModeOptionsWithTranslator,
  diagnosisAuthRoleMappingGuidance as diagnosisAuthRoleMappingGuidanceWithTranslator,
  diagnosisAuthRolloutReadiness as diagnosisAuthRolloutReadinessWithTranslator,
  type DiagnosisAuthTranslator,
} from "./auth-readiness";

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

const diagnosisAuthActionBlockReason = bindAuthTranslator(
  diagnosisAuthActionBlockReasonWithTranslator,
);
const diagnosisAuthBackendCredentialListLabel = bindAuthTranslator(
  diagnosisAuthBackendCredentialListLabelWithTranslator,
);
const diagnosisAuthBackendModeDisplayItems = bindAuthTranslator(
  diagnosisAuthBackendModeDisplayItemsWithTranslator,
);
const diagnosisAuthBackendModeListLabel = bindAuthTranslator(
  diagnosisAuthBackendModeListLabelWithTranslator,
);
const diagnosisAuthBackendReadiness = bindAuthTranslator(
  diagnosisAuthBackendReadinessWithTranslator,
);
const diagnosisAuthBrowserSessionAuthenticatedSummary = bindAuthTranslator(
  diagnosisAuthBrowserSessionAuthenticatedSummaryWithTranslator,
);
const diagnosisAuthBrowserSessionBlockReason = bindAuthTranslator(
  diagnosisAuthBrowserSessionBlockReasonWithTranslator,
);
const diagnosisAuthBrowserSessionDisplaySummary = bindAuthTranslator(
  diagnosisAuthBrowserSessionDisplaySummaryWithTranslator,
);
const diagnosisAuthCheckSuccessFeedback = bindAuthTranslator(
  diagnosisAuthCheckSuccessFeedbackWithTranslator,
);
const diagnosisAuthCheckBlockReason = bindAuthTranslator(
  diagnosisAuthCheckBlockReasonWithTranslator,
);
const diagnosisAuthInputReadiness = bindAuthTranslator(
  diagnosisAuthInputReadinessWithTranslator,
);
const diagnosisAuthLDAPBrowserSessionPromotionNotice = bindAuthTranslator(
  diagnosisAuthLDAPBrowserSessionPromotionNoticeWithTranslator,
);
const diagnosisAuthModeOptions = bindAuthTranslator(
  diagnosisAuthModeOptionsWithTranslator,
);
const diagnosisAuthRoleMappingGuidance = bindAuthTranslator(
  diagnosisAuthRoleMappingGuidanceWithTranslator,
);
const diagnosisAuthRolloutReadiness = bindAuthTranslator(
  diagnosisAuthRolloutReadinessWithTranslator,
);
const diagnosisAuthLDAPSetupReadiness = (
  status: Parameters<typeof diagnosisAuthLDAPSetupReadinessWithTranslator>[0],
  loading = false,
) => diagnosisAuthLDAPSetupReadinessWithTranslator(status, tEn, loading);
const diagnosisAuthWeComSetupReadiness = (
  status: Parameters<typeof diagnosisAuthWeComSetupReadinessWithTranslator>[0],
  loading = false,
) => diagnosisAuthWeComSetupReadinessWithTranslator(status, tEn, loading);

describe("diagnosis auth input readiness", () => {
  it("describes LDAP browser-session promotion without storing passwords", () => {
    expect(diagnosisAuthLDAPBrowserSessionPromotionNotice()).toEqual({
      detail:
        "After Check auth accepts explicitly configured LDAP credentials, OpenClarion exchanges them for an HttpOnly browser session, clears the LDAP password from this form, and uses local RBAC for diagnosis room create or connect actions.",
      message: "LDAP fallback creates a browser session",
    });
  });

  it("keeps browser session as the default auth mode", () => {
    expect(diagnosisAuthInputReadiness({})).toEqual({
      detail:
        "Use the current OpenClarion browser session from IAM OIDC. Check auth verifies the HttpOnly session cookie through the backend, and diagnosis room access is enforced by local RBAC.",
      label: "IAM browser session ready to check.",
      mode: "session",
      status: "ready",
    });
  });

  it("marks locally well-formed LDAP credentials ready for backend check", () => {
    expect(
      diagnosisAuthInputReadiness({
        authMode: "ldap",
        ldapPassword: "password-1",
        ldapUsername: " operator-1 ",
      }),
    ).toEqual({
      detail:
        "Direct LDAP credentials are locally well-formed; Check auth verifies them against the explicitly configured backend LDAP provider.",
      label: "LDAP fallback credentials ready to check.",
      mode: "ldap",
      status: "ready",
    });
  });

  it("blocks malformed LDAP username and password values", () => {
    expect(
      diagnosisAuthInputReadiness({
        authMode: "ldap",
        ldapPassword: "password-1",
        ldapUsername: "operator 1",
      }),
    ).toMatchObject({
      label: "LDAP username is invalid.",
      mode: "ldap",
      status: "blocked",
    });

    expect(
      diagnosisAuthInputReadiness({
        authMode: "ldap",
        ldapPassword: "password-1",
        ldapUsername: "operator\u0001",
      }),
    ).toMatchObject({
      label: "LDAP username is invalid.",
      mode: "ldap",
      status: "blocked",
    });

    expect(
      diagnosisAuthInputReadiness({
        authMode: "ldap",
        ldapPassword: "password\n1",
        ldapUsername: "operator-1",
      }),
    ).toMatchObject({
      label: "LDAP password is invalid.",
      mode: "ldap",
      status: "blocked",
    });
  });

  it("summarizes bearer token input without accepting whitespace", () => {
    expect(diagnosisAuthInputReadiness({ authMode: "bearer" })).toMatchObject({
      label: "Bearer token required.",
      mode: "bearer",
      status: "pending",
    });
    expect(
      diagnosisAuthInputReadiness({
        authMode: "bearer",
        bearerToken: "token 1",
      }),
    ).toMatchObject({
      label: "Bearer token is invalid.",
      mode: "bearer",
      status: "blocked",
    });
    expect(
      diagnosisAuthInputReadiness({
        authMode: "bearer",
        bearerToken: " token-1 ",
      }),
    ).toMatchObject({
      label: "Bearer token ready to check.",
      mode: "bearer",
      status: "ready",
    });
  });

  it("blocks legacy Enterprise WeChat browser login", () => {
    expect(diagnosisAuthInputReadiness({ authMode: "wecom" })).toEqual({
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
      label: "Enterprise WeChat browser login migrated.",
      mode: "wecom",
      status: "blocked",
    });
  });

  it("describes the OpenClarion browser session as a reusable auth mode", () => {
    expect(diagnosisAuthInputReadiness({ authMode: "session" })).toEqual({
      detail:
        "Use the current OpenClarion browser session from IAM OIDC. Check auth verifies the HttpOnly session cookie through the backend, and diagnosis room access is enforced by local RBAC.",
      label: "IAM browser session ready to check.",
      mode: "session",
      status: "ready",
    });
  });
});

describe("diagnosis LDAP setup readiness", () => {
  it("marks complete LDAP setup ready when encrypted transport is reported", () => {
    const readiness = diagnosisAuthLDAPSetupReadiness({
      configured: true,
      mode: "ldap",
      role_mapping: {
        admin_mapping_count: 1,
        configured: true,
        default_roles: [],
        owner_mapping_count: 1,
      },
      supported_modes: ["ldap"],
      transport_policy: { security: "tls" },
    });

    expect(readiness).toMatchObject({
      color: "success",
      label: "LDAP setup ready",
      status: "ready",
      items: [
        { key: "backend", status: "ready" },
        { key: "transport_policy", status: "ready" },
        { key: "role_mapping", status: "ready" },
      ],
    });
    expect(readiness.detail).toContain("encrypted credential transport");
  });

  it("keeps LDAP setup in review when transport metadata is not reported", () => {
    const readiness = diagnosisAuthLDAPSetupReadiness({
      configured: true,
      mode: "ldap",
      role_mapping: {
        admin_mapping_count: 1,
        configured: true,
        default_roles: [],
        owner_mapping_count: 1,
      },
      supported_modes: ["ldap"],
    });

    expect(readiness).toMatchObject({
      color: "warning",
      label: "LDAP setup needs review",
      status: "review",
      items: [
        { key: "backend", status: "ready" },
        { key: "transport_policy", status: "review" },
        { key: "role_mapping", status: "ready" },
      ],
    });
    expect(
      readiness.items.find((item) => item.key === "transport_policy")?.detail,
    ).toContain("did not report LDAP transport policy metadata");
  });

  it("keeps LDAP setup ready when only optional default provider roles are configured", () => {
    const readiness = diagnosisAuthLDAPSetupReadiness({
      configured: true,
      mode: "ldap",
      role_mapping: {
        admin_mapping_count: 0,
        configured: true,
        default_roles: ["owner"],
        owner_mapping_count: 0,
      },
      supported_modes: ["ldap"],
      transport_policy: { security: "start_tls" },
    });

    expect(readiness).toMatchObject({
      color: "success",
      label: "LDAP setup ready",
      status: "ready",
      items: [
        { key: "backend", status: "ready" },
        { key: "transport_policy", status: "ready" },
        { key: "role_mapping", status: "ready" },
      ],
    });
    expect(
      readiness.items.find((item) => item.key === "role_mapping")?.detail,
    ).toContain("only optional default provider roles");
  });

  it("blocks LDAP setup when plaintext transport is allowed", () => {
    const readiness = diagnosisAuthLDAPSetupReadiness({
      configured: true,
      mode: "ldap",
      role_mapping: {
        admin_mapping_count: 1,
        configured: true,
        default_roles: [],
        owner_mapping_count: 1,
      },
      supported_modes: ["ldap"],
      transport_policy: { security: "insecure_plaintext" },
    });

    expect(readiness).toMatchObject({
      color: "error",
      label: "LDAP setup blocked",
      status: "blocked",
      items: [
        { key: "backend", status: "ready" },
        { key: "transport_policy", status: "blocked" },
        { key: "role_mapping", status: "ready" },
      ],
    });
    expect(
      readiness.items.find((item) => item.key === "transport_policy")?.detail,
    ).toContain("plaintext transport");
  });

  it("blocks LDAP setup when the backend does not advertise LDAP", () => {
    expect(
      diagnosisAuthLDAPSetupReadiness({
        configured: true,
        mode: "wecom",
        role_mapping: {
          admin_mapping_count: 1,
          configured: true,
          default_roles: [],
          owner_mapping_count: 1,
        },
        supported_modes: ["wecom"],
      }),
    ).toMatchObject({
      color: "error",
      label: "LDAP setup blocked",
      status: "blocked",
      items: [
        { key: "backend", status: "blocked" },
        { key: "transport_policy", status: "blocked" },
        { key: "role_mapping", status: "ready" },
      ],
    });
  });
});

describe("diagnosis Enterprise WeChat collaboration readiness", () => {
  it("blocks legacy Enterprise WeChat browser authentication after IAM migration", () => {
    const readiness = diagnosisAuthWeComSetupReadiness({
      configured: true,
      mode: "wecom",
      role_mapping: {
        admin_mapping_count: 0,
        configured: false,
        default_roles: [],
        owner_mapping_count: 0,
      },
      supported_modes: ["wecom"],
    });

    expect(readiness).toMatchObject({
      color: "error",
      label: "Enterprise WeChat browser auth migrated",
      status: "blocked",
      items: [
        { key: "backend", status: "blocked" },
        { key: "callback", status: "blocked" },
        { key: "identity_checks", status: "blocked" },
        { key: "role_mapping", status: "ready" },
      ],
    });
    expect(readiness.detail).toContain("migrated to IAM OIDC");
    expect(
      readiness.items.find((item) => item.key === "role_mapping")?.detail,
    ).toBe(
      "Enterprise WeChat has no provider role mapping configured. Identity-only authentication is accepted; diagnosis room permissions are assigned through OpenClarion local RBAC.",
    );
  });

  it("keeps IAM-backed deployments in review for independent app callback verification", () => {
    expect(
      diagnosisAuthWeComSetupReadiness({
        configured: true,
        mode: "oidc",
        role_mapping: {
          admin_mapping_count: 1,
          configured: true,
          default_roles: [],
          owner_mapping_count: 1,
        },
        supported_modes: ["oidc"],
      }),
    ).toMatchObject({
      color: "warning",
      label: "Enterprise WeChat collaboration needs review",
      status: "review",
      items: [
        { key: "backend", status: "ready" },
        { key: "callback", status: "review" },
        { key: "identity_checks", status: "ready" },
        { key: "role_mapping", status: "ready" },
      ],
    });
  });

});

describe("diagnosis auth rollout readiness", () => {
  it("accepts verified LDAP credentials with an owner or admin role", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        checkedAt: "2026-06-21T04:00:00Z",
        detail: "Backend diagnosis auth check succeeded.",
        mode: "ldap",
        roleAuthorized: true,
        roles: ["owner"],
        status: "verified",
        subject: "operator-1",
      }),
    ).toMatchObject({
      checkedAt: "2026-06-21T04:00:00Z",
      label: "LDAP rollout proof ready.",
      mode: "ldap",
      roleAuthorized: true,
      status: "ready",
      subject: "operator-1",
    });
  });

  it("keeps verified legacy Enterprise WeChat browser authentication in review", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        checkedAt: "2026-06-21T04:05:00Z",
        detail: "Backend diagnosis auth check succeeded.",
        mode: "wecom",
        roleAuthorized: true,
        roles: ["admin"],
        status: "verified",
        subject: "wecom-user-1",
      }),
    ).toMatchObject({
      checkedAt: "2026-06-21T04:05:00Z",
      detail:
        "Legacy Enterprise WeChat browser auth proof is no longer accepted for rollout. Use IAM OIDC browser sessions and keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
      label: "Operator SSO proof required for rollout.",
      mode: "wecom",
      roleAuthorized: true,
      status: "review",
      subject: "wecom-user-1",
    });
  });

  it("keeps static bearer auth in review for operator rollout", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        detail: "Backend diagnosis auth check succeeded.",
        mode: "bearer",
        roles: ["owner"],
        status: "verified",
        subject: "operator-static",
      }),
    ).toMatchObject({
      detail:
        "Static bearer auth is acceptable for development checks, but operator rollout requires IAM or LDAP identity proof with OpenClarion local RBAC configured.",
      label: "Operator SSO proof required for rollout.",
      mode: "bearer",
      status: "review",
      subject: "operator-static",
    });
  });

  it("accepts verified LDAP identity proof before local RBAC", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        detail: "Backend diagnosis auth check succeeded.",
        mode: "ldap",
        roleAuthorized: false,
        roles: ["viewer"],
        status: "verified",
        subject: "operator-1",
      }),
    ).toMatchObject({
      detail: "Backend diagnosis auth check succeeded.",
      label: "LDAP rollout proof ready.",
      roleAuthorized: false,
      status: "ready",
    });
  });

  it("keeps verified legacy Enterprise WeChat identity proof in review before local RBAC", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        detail: "Backend diagnosis auth check succeeded.",
        mode: "wecom",
        roleAuthorized: false,
        roles: ["viewer"],
        status: "verified",
        subject: "wecom-user-1",
      }),
    ).toMatchObject({
      detail:
        "Legacy Enterprise WeChat browser auth proof is no longer accepted for rollout. Use IAM OIDC browser sessions and keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
      label: "Operator SSO proof required for rollout.",
      roleAuthorized: false,
      status: "review",
    });
  });

  it("preserves pending proof details before backend auth is checked", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        detail:
          "Run Check auth with LDAP basic auth before accepting live proof.",
        mode: "ldap",
        roles: [],
        status: "pending",
        subject: "",
      }),
    ).toMatchObject({
      label: "Operator auth rollout proof pending.",
      status: "pending",
    });
  });

  it("uses provider-neutral copy when rollout proof is blocked", () => {
    expect(
      diagnosisAuthRolloutReadiness({
        detail: "",
        mode: "wecom",
        roles: [],
        status: "blocked",
        subject: "",
      }),
    ).toMatchObject({
      label: "Operator auth rollout proof blocked.",
      status: "blocked",
    });
  });

  it("describes provider-specific role mapping guidance", () => {
    expect(
      diagnosisAuthRoleMappingGuidance({
        mode: "wecom",
        roleAuthorized: false,
        status: "blocked",
      }),
    ).toBe(
      "Enterprise WeChat provider role mapping is optional for identity proof. Assign diagnosis room access in OpenClarion local RBAC.",
    );
    expect(
      diagnosisAuthRoleMappingGuidance({
        mode: "ldap",
        roleAuthorized: false,
        status: "blocked",
      }),
    ).toBe(
      "LDAP provider role mapping is optional for identity proof. Assign diagnosis room access in OpenClarion local RBAC.",
    );
    expect(
      diagnosisAuthRoleMappingGuidance({
        mode: "bearer",
        roleAuthorized: true,
        status: "review",
      }),
    ).toBe(
      "Static bearer roles are development-only; operator rollout should use IAM, LDAP, or Enterprise WeChat identity proof plus OpenClarion local RBAC.",
    );
  });
});

describe("diagnosis auth backend readiness", () => {
  it("derives selectable auth modes from backend mode", () => {
    expect(
      diagnosisAuthModeOptions({ configured: true, mode: "ldap" }),
    ).toEqual([
      { disabled: false, label: "IAM session", value: "session" },
      { disabled: false, label: "LDAP fallback", value: "ldap" },
      { disabled: true, label: "Dev bearer", value: "bearer" },
    ]);
    expect(
      diagnosisAuthModeOptions({ configured: true, mode: "static" }),
    ).toEqual([
      { disabled: true, label: "IAM session", value: "session" },
      { disabled: true, label: "LDAP fallback", value: "ldap" },
      { disabled: false, label: "Dev bearer", value: "bearer" },
    ]);
    expect(
      diagnosisAuthModeOptions({ configured: true, mode: "wecom" }),
    ).toEqual([
      { disabled: true, label: "IAM session", value: "session" },
      { disabled: true, label: "LDAP fallback", value: "ldap" },
      { disabled: true, label: "Dev bearer", value: "bearer" },
    ]);
    expect(
      diagnosisAuthModeOptions({
        configured: true,
        mode: "ldap",
        supportedModes: ["ldap", "wecom"],
      }),
    ).toEqual([
      { disabled: false, label: "IAM session", value: "session" },
      { disabled: false, label: "LDAP fallback", value: "ldap" },
      { disabled: true, label: "Dev bearer", value: "bearer" },
    ]);
    expect(
      diagnosisAuthModeOptions({ configured: false, mode: "none" }),
    ).toEqual([
      { disabled: true, label: "IAM session", value: "session" },
      { disabled: true, label: "LDAP fallback", value: "ldap" },
      { disabled: true, label: "Dev bearer", value: "bearer" },
    ]);
    expect(
      diagnosisAuthModeOptions({ configured: true, mode: "oidc" }),
    ).toEqual([
      { disabled: false, label: "IAM session", value: "session" },
      { disabled: true, label: "LDAP fallback", value: "ldap" },
      { disabled: true, label: "Dev bearer", value: "bearer" },
    ]);
  });

  it("summarizes mixed backend auth modes for status displays", () => {
    const status = {
      configured: true,
      mode: "ldap",
      supportedModes: ["ldap", "wecom"],
    } as const;

    expect(diagnosisAuthBackendStatusModes(status)).toEqual(["ldap", "wecom"]);
    expect(diagnosisAuthBackendModeListLabel(status)).toBe("LDAP + WeCom");
    expect(diagnosisAuthBackendCredentialListLabel(status)).toBe(
      "LDAP Basic credentials or Enterprise WeChat authentication",
    );
    expect(diagnosisAuthBackendModeDisplayItems(status)).toEqual([
      { color: "blue", label: "LDAP", mode: "ldap" },
      { color: "green", label: "WeCom", mode: "wecom" },
    ]);
    expect(
      diagnosisAuthBackendStatusModes({
        configured: false,
        mode: "none",
      }),
    ).toEqual([]);
  });

  it("coerces selected auth mode only when backend reports a usable alternative", () => {
    expect(
      diagnosisAuthCoercedMode("bearer", { configured: true, mode: "ldap" }),
    ).toBe("session");
    expect(
      diagnosisAuthCoercedMode("ldap", { configured: true, mode: "oidc" }),
    ).toBe("session");
    expect(
      diagnosisAuthCoercedMode("ldap", { configured: true, mode: "wecom" }),
    ).toBe("ldap");
    expect(
      diagnosisAuthCoercedMode("wecom", {
        configured: true,
        mode: "ldap",
        supportedModes: ["ldap", "wecom"],
      }),
    ).toBe("session");
    expect(
      diagnosisAuthCoercedMode("bearer", {
        configured: true,
        mode: "ldap",
        supportedModes: ["ldap", "wecom"],
      }),
    ).toBe("session");
    expect(
      diagnosisAuthCoercedMode("ldap", { configured: false, mode: "none" }),
    ).toBe("ldap");
    expect(diagnosisAuthCoercedMode("ldap", undefined)).toBe("ldap");
  });

  it("requires backend check after local LDAP inputs are ready", () => {
    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck: null,
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toEqual({
      color: "warning",
      detail:
        "Run Check auth to verify these credentials against the configured backend provider.",
      label: "Backend auth check required.",
      status: "needs_check",
    });
  });

  it("summarizes authenticated browser sessions by source and local roles", () => {
    expect(
      diagnosisAuthBrowserSessionAuthenticatedSummary({
        mode: "wecom",
        roles: ["owner"],
        subject: "wecom-user-1",
      }),
    ).toEqual({
      alertType: "success",
      detail:
        "Signed in as wecom-user-1 using Enterprise WeChat. Roles: owner. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.",
    });

    expect(
      diagnosisAuthBrowserSessionAuthenticatedSummary({
        expectedMode: "wecom",
        mode: "ldap",
        roles: ["owner"],
        subject: "operator-ldap",
      }),
    ).toEqual({
      alertType: "warning",
      detail:
        "Signed in as operator-ldap using LDAP. Roles: owner. Enterprise WeChat browser login has been replaced by IAM OIDC; select IAM browser session before running Check auth.",
    });

    expect(
      diagnosisAuthBrowserSessionAuthenticatedSummary({
        expectedMode: "wecom",
        mode: "wecom",
        roles: [],
        subject: "wecom-user-2",
      }),
    ).toEqual({
      alertType: "success",
      detail:
        "Signed in as wecom-user-2 using Enterprise WeChat. Roles: no roles. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.",
    });

    expect(
      diagnosisAuthBrowserSessionAuthenticatedSummary({
        roles: [],
        subject: "wecom-user-2",
      }),
    ).toEqual({
      alertType: "success",
      detail:
        "Signed in as wecom-user-2. Roles: no roles. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.",
    });

    expect(
      diagnosisAuthBrowserSessionAuthenticatedSummary({
        roles: ["auditor"],
        subject: "",
      }),
    ).toEqual({
      alertType: "success",
      detail:
        "Signed in as the current user. Roles: auditor. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.",
    });
  });

  it("summarizes browser session display states without hiding check failures", () => {
    const unauthenticatedDetail =
      "No OpenClarion browser session is active.";

    expect(
      diagnosisAuthBrowserSessionDisplaySummary({
        authenticated: false,
        checkFailed: false,
        loading: true,
        roles: [],
        subject: "",
        unauthenticatedDetail,
      }),
    ).toEqual({
      active: false,
      alertType: "info",
      detail: "Checking the current OpenClarion browser session.",
    });

    expect(
      diagnosisAuthBrowserSessionDisplaySummary({
        authenticated: false,
        checkFailed: true,
        loading: false,
        roles: [],
        subject: "",
        unauthenticatedDetail,
      }),
    ).toEqual({
      active: false,
      alertType: "warning",
      detail:
        "OpenClarion browser session could not be checked. Reload this page or sign in again before continuing.",
    });

    expect(
      diagnosisAuthBrowserSessionDisplaySummary({
        authenticated: false,
        checkFailed: false,
        loading: false,
        roles: [],
        subject: "",
        unauthenticatedDetail,
      }),
    ).toEqual({
      active: false,
      alertType: "info",
      detail: unauthenticatedDetail,
    });

    expect(
      diagnosisAuthBrowserSessionDisplaySummary({
        authenticated: true,
        checkFailed: false,
        expectedMode: "wecom",
        loading: false,
        mode: "wecom",
        roles: ["owner"],
        subject: "wecom-user-1",
        unauthenticatedDetail,
      }),
    ).toEqual({
      active: true,
      alertType: "success",
      detail:
        "Signed in as wecom-user-1 using Enterprise WeChat. Roles: owner. Identity is verified; diagnosis room access is enforced by local RBAC when creating or connecting.",
    });
  });

  it("clears cached browser session state only after session auth rejections", () => {
    expect(
      diagnosisAuthBrowserSessionShouldClearAfterError({
        authMode: "session",
        status: 401,
      }),
    ).toBe(true);
    expect(
      diagnosisAuthBrowserSessionShouldClearAfterError({
        authMode: "session",
        status: 403,
      }),
    ).toBe(true);
    expect(
      diagnosisAuthBrowserSessionShouldClearAfterError({
        authMode: "session",
        status: 500,
      }),
    ).toBe(false);
    expect(
      diagnosisAuthBrowserSessionShouldClearAfterError({
        authMode: "basic",
        status: 401,
      }),
    ).toBe(false);
    expect(
      diagnosisAuthBrowserSessionShouldClearAfterError({
        authMode: "bearer",
        status: 403,
      }),
    ).toBe(false);
  });

  it("treats successful backend auth as identity proof before local RBAC", () => {
    expect(
      diagnosisAuthCheckSuccessFeedback({
        roles: [],
        subject: "wecom-user-2",
      }),
    ).toEqual({
      logLevel: "info",
      logMessage:
        "Authentication checked for wecom-user-2 (no roles).",
      toastMessage:
        "Authenticated as wecom-user-2. Local RBAC will authorize diagnosis room actions.",
      toastType: "success",
    });

    expect(
      diagnosisAuthCheckSuccessFeedback({
        roles: ["owner"],
        subject: "operator-1",
      }),
    ).toEqual({
      logLevel: "info",
      logMessage: "Authentication checked for operator-1 (owner).",
      toastMessage:
        "Authenticated as operator-1. Local RBAC will authorize diagnosis room actions.",
      toastType: "success",
    });
  });

  it("requires Enterprise WeChat login before browser-session auth is usable", () => {
    expect(
      diagnosisAuthBackendReadiness({
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        lastCheck: null,
        values: { authMode: "wecom" },
      }),
    ).toEqual({
      color: "error",
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
      label: "Enterprise WeChat browser login migrated.",
      status: "blocked",
    });
    expect(
      diagnosisAuthActionBlockReason({
        action: "create",
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        lastCheck: null,
        values: { authMode: "wecom" },
      }),
    ).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
    );
    expect(
      diagnosisAuthActionBlockReason({
        action: "connect",
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        lastCheck: null,
        values: { authMode: "wecom" },
      }),
    ).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
    );
  });

  it("requires an active OpenClarion browser session before session auth is usable", () => {
    expect(
      diagnosisAuthBackendReadiness({
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        lastCheck: null,
        values: { authMode: "session" },
      }),
    ).toEqual({
      color: "warning",
      detail:
        "Run Check auth to verify the current OpenClarion browser session identity against the backend provider. Diagnosis room permissions are enforced by local RBAC.",
      label: "Backend auth check required.",
      status: "needs_check",
    });
    expect(
      diagnosisAuthActionBlockReason({
        action: "connect",
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        lastCheck: null,
        values: { authMode: "session" },
      }),
    ).toBe(
      "Run Check auth successfully with the current browser session before connecting to a diagnosis room.",
    );
  });

  it("requires a fresh browser-session check when the session subject changes", () => {
    const lastCheck = {
      inputRevision: 3,
      message: "Authenticated as operator-a.",
      mode: "session" as const,
      roleAuthorized: true,
      roles: ["owner"],
      status: "success" as const,
      subject: "operator-a",
    };

    expect(
      diagnosisAuthBackendReadiness({
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        expectedSubject: "operator-b",
        inputRevision: 3,
        lastCheck,
        values: { authMode: "session" },
      }),
    ).toMatchObject({
      detail:
        "Run Check auth again for the current OpenClarion browser session subject operator-b; the last backend check was for operator-a.",
      label: "Backend auth check required.",
      status: "needs_check",
    });
    expect(
      diagnosisAuthBackendVerified({
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        expectedSubject: "operator-b",
        inputRevision: 3,
        lastCheck,
        values: { authMode: "session" },
      }),
    ).toBe(false);
    expect(
      diagnosisAuthActionBlockReason({
        action: "create",
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        expectedSubject: "operator-b",
        inputRevision: 3,
        lastCheck,
        values: { authMode: "session" },
      }),
    ).toBe(
      "Run Check auth successfully with the current browser session before creating a diagnosis room.",
    );
  });

  it("blocks Enterprise WeChat checks and actions until the browser session is ready", () => {
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "check",
        sessionAuthenticated: false,
        values: { authMode: "wecom" },
      }),
    ).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and sign in before running Check auth.",
    );
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "action",
        sessionAuthenticated: false,
        sessionStatusAvailable: false,
        values: { authMode: "wecom" },
      }),
    ).toBe(
      "OpenClarion browser session could not be checked. Reload this page or sign in again before creating or connecting to a diagnosis room.",
    );
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "check",
        sessionAuthenticated: true,
        sessionMode: "wecom",
        values: { authMode: "wecom" },
      }),
    ).toBe("");
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "check",
        sessionAuthenticated: true,
        sessionMode: "ldap",
        values: { authMode: "wecom" },
      }),
    ).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Select IAM browser session and use the active IAM session for Check auth.",
    );
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "action",
        sessionAuthenticated: true,
        sessionMode: "wecom",
        values: { authMode: "wecom" },
      }),
    ).toBe("");
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "action",
        sessionAuthenticated: true,
        values: { authMode: "session" },
      }),
    ).toBe("");
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "check",
        sessionAuthenticated: false,
        values: { authMode: "session" },
      }),
    ).toBe(
      "Sign in with IAM before running Check auth with a browser session.",
    );
    expect(
      diagnosisAuthBrowserSessionBlockReason({
        intent: "check",
        sessionAuthenticated: false,
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toBe("");
  });

  it("blocks auth modes that do not match the running backend", () => {
    expect(
      diagnosisAuthBackendReadiness({
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        lastCheck: null,
        values: { authMode: "bearer", bearerToken: "token-1" },
      }),
    ).toEqual({
      color: "error",
      detail:
        "The running backend expects LDAP Basic credentials, not Bearer credentials.",
      label: "Auth mode does not match backend.",
      status: "blocked",
    });

    expect(
      diagnosisAuthBackendReadiness({
        backendStatus: { configured: true, mode: "static" },
        checking: false,
        lastCheck: null,
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toMatchObject({
      detail:
        "The running backend expects a static Bearer token, not LDAP Basic credentials.",
      label: "Auth mode does not match backend.",
      status: "blocked",
    });

    expect(
      diagnosisAuthActionBlockReason({
        action: "connect",
        backendStatus: { configured: true, mode: "ldap" },
        checking: false,
        lastCheck: null,
        values: { authMode: "bearer", bearerToken: "token-1" },
      }),
    ).toBe(
      "The running backend expects LDAP Basic credentials, not Bearer credentials.",
    );

    expect(
      diagnosisAuthCheckBlockReason({
        backendStatus: { configured: true, mode: "wecom" },
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toBe(
      "The running backend advertises legacy Enterprise WeChat browser authentication. OpenClarion browser login is now handled by IAM OIDC; update the backend auth mode before accepting rollout.",
    );

    expect(
      diagnosisAuthCheckBlockReason({
        backendStatus: { configured: true, mode: "static" },
        values: { authMode: "wecom" },
      }),
    ).toBe(
      "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
    );

    expect(
      diagnosisAuthCheckBlockReason({
        backendStatus: { configured: true, mode: "static" },
        values: { authMode: "session" },
      }),
    ).toBe(
      "The running backend expects a static Bearer token, not an OpenClarion browser session.",
    );
  });

  it("blocks credential checks when backend auth is not usable", () => {
    expect(
      diagnosisAuthCheckBlockReason({
        backendStatus: { configured: false, mode: "none" },
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toBe("Diagnosis auth is not configured in the running backend.");

    expect(
      diagnosisAuthCheckBlockReason({
        backendStatus: { configured: true, mode: "unknown" },
        values: { authMode: "bearer", bearerToken: "token-1" },
      }),
    ).toBe(
      "Backend diagnosis auth mode is unknown; reload or inspect deployment before sending credentials.",
    );

    expect(
      diagnosisAuthCheckBlockReason({
        backendStatus: { configured: true, mode: "oidc" },
        values: { authMode: "bearer", bearerToken: "token-1" },
      }),
    ).toBe(
      "The running backend expects IAM OIDC authentication, not Bearer credentials.",
    );
  });

  it("renders verified backend auth from semantic subject and roles", () => {
    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck: {
          checkedAt: "2026-06-21T04:00:00Z",
          message: "",
          mode: "ldap",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toEqual({
      color: "success",
      detail:
        "Authenticated as operator-1. Roles: owner. Checked at 2026-06-21T04:00:00Z.",
      label: "Backend auth verified.",
      status: "verified",
    });
  });

  it("verifies authenticated principals before local RBAC authorization", () => {
    const values = {
      authMode: "ldap" as const,
      ldapPassword: "password-1",
      ldapUsername: "operator-1",
    };
    const lastCheck = {
      message: "Authenticated as operator-1.",
      mode: "ldap" as const,
      roleAuthorized: false,
      roles: ["viewer"],
      status: "success" as const,
      subject: "operator-1",
    };

    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck,
        values,
      }),
    ).toEqual({
      color: "success",
      detail: "Authenticated as operator-1. Roles: viewer.",
      label: "Backend auth verified.",
      status: "verified",
    });
    expect(
      diagnosisAuthBackendVerified({
        checking: false,
        lastCheck,
        values,
      }),
    ).toBe(true);
    expect(
      diagnosisAuthActionBlockReason({
        action: "create",
        checking: false,
        lastCheck,
        values,
      }),
    ).toBe("");
  });

  it("blocks Enterprise WeChat backend auth after IAM migration", () => {
    expect(
      diagnosisAuthBackendReadiness({
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        lastCheck: {
          message: "Authenticated as wecom-user-1.",
          mode: "wecom",
          roleAuthorized: false,
          roles: ["viewer"],
          status: "success",
          subject: "wecom-user-1",
        },
        values: { authMode: "wecom" },
      }),
    ).toEqual({
      color: "error",
      detail:
        "Enterprise WeChat browser login has been replaced by IAM OIDC. Use the current IAM browser session for OpenClarion authorization; keep Enterprise WeChat for app messages, notifications, and diagnosis-room collaboration callbacks.",
      label: "Enterprise WeChat browser login migrated.",
      status: "blocked",
    });
  });

  it("marks failed backend checks and changed modes as not verified", () => {
    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck: {
          message: "ldap auth: invalid credentials",
          mode: "ldap",
          roles: [],
          status: "failed",
          subject: "",
        },
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator-1",
        },
      }),
    ).toMatchObject({
      color: "error",
      label: "Backend auth check failed.",
      status: "failed",
    });

    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck: {
          message: "Authenticated as operator-1.",
          mode: "ldap",
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        values: {
          authMode: "bearer",
          bearerToken: "token-1",
        },
      }),
    ).toMatchObject({
      label: "Backend auth check required.",
      status: "needs_check",
    });
  });

  it("requires a new backend check when auth input revision changes", () => {
    const values = {
      authMode: "ldap" as const,
      ldapPassword: "password-2",
      ldapUsername: "operator-1",
    };
    const lastCheck = {
      inputRevision: 1,
      message: "Authenticated as operator-1.",
      mode: "ldap" as const,
      roles: ["owner"],
      status: "success" as const,
      subject: "operator-1",
    };

    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        inputRevision: 2,
        lastCheck,
        values,
      }),
    ).toMatchObject({
      label: "Backend auth check required.",
      status: "needs_check",
    });
    expect(
      diagnosisAuthBackendVerified({
        checking: false,
        inputRevision: 2,
        lastCheck,
        values,
      }),
    ).toBe(false);
    expect(
      diagnosisAuthActionBlockReason({
        action: "connect",
        checking: false,
        inputRevision: 2,
        lastCheck,
        values,
      }),
    ).toBe(
      "Run Check auth successfully before connecting to a diagnosis room.",
    );
  });

  it("accepts backend checks that match the current auth input revision", () => {
    const values = {
      authMode: "ldap" as const,
      ldapPassword: "password-1",
      ldapUsername: "operator-1",
    };
    const lastCheck = {
      inputRevision: 3,
      message: "Authenticated as operator-1.",
      mode: "ldap" as const,
      roles: ["owner"],
      status: "success" as const,
      subject: "operator-1",
    };

    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        inputRevision: 3,
        lastCheck,
        values,
      }),
    ).toMatchObject({
      label: "Backend auth verified.",
      status: "verified",
    });
    expect(
      diagnosisAuthActionBlockReason({
        action: "create",
        checking: false,
        inputRevision: 3,
        lastCheck,
        values,
      }),
    ).toBe("");
  });

  it("does not ask the backend while local input is incomplete or blocked", () => {
    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck: null,
        values: { authMode: "ldap" },
      }),
    ).toMatchObject({
      label: "LDAP fallback credentials required.",
      status: "pending",
    });
    expect(
      diagnosisAuthBackendReadiness({
        checking: false,
        lastCheck: null,
        values: {
          authMode: "ldap",
          ldapPassword: "password-1",
          ldapUsername: "operator 1",
        },
      }),
    ).toMatchObject({
      label: "LDAP username is invalid.",
      status: "blocked",
    });
  });

  it("exposes a strict verified boolean for submit gating", () => {
    const values = {
      authMode: "ldap" as const,
      ldapPassword: "password-1",
      ldapUsername: "operator-1",
    };
    expect(
      diagnosisAuthBackendVerified({
        checking: false,
        lastCheck: {
          message: "Authenticated as operator-1.",
          mode: "ldap",
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        values,
      }),
    ).toBe(true);
    expect(
      diagnosisAuthBackendVerified({
        checking: false,
        lastCheck: null,
        values,
      }),
    ).toBe(false);
    expect(
      diagnosisAuthBackendVerified({
        checking: false,
        lastCheck: {
          message: "Authenticated as operator-1.",
          mode: "ldap",
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        values: {
          authMode: "bearer",
          bearerToken: "token-1",
        },
      }),
    ).toBe(false);
  });

  it("returns action-specific submit blockers until backend auth is verified", () => {
    const values = {
      authMode: "ldap" as const,
      ldapPassword: "password-1",
      ldapUsername: "operator-1",
    };
    expect(
      diagnosisAuthActionBlockReason({
        action: "create",
        checking: false,
        lastCheck: null,
        values,
      }),
    ).toBe("Run Check auth successfully before creating a diagnosis room.");
    expect(
      diagnosisAuthActionBlockReason({
        action: "connect",
        checking: false,
        lastCheck: null,
        values,
      }),
    ).toBe(
      "Run Check auth successfully before connecting to a diagnosis room.",
    );
    expect(
      diagnosisAuthActionBlockReason({
        action: "create",
        checking: false,
        lastCheck: {
          message: "Authenticated as operator-1.",
          mode: "ldap",
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        values,
      }),
    ).toBe("");
  });

  it("identifies only auth input changes as auth-check invalidation triggers", () => {
    expect(
      diagnosisAuthInputFieldsChanged({ ldapUsername: "operator-1" }),
    ).toBe(true);
    expect(
      diagnosisAuthInputFieldsChanged({ ldapPassword: "password-1" }),
    ).toBe(true);
    expect(diagnosisAuthInputFieldsChanged({ bearerToken: "token-1" })).toBe(
      true,
    );
    expect(diagnosisAuthInputFieldsChanged({ authMode: "bearer" })).toBe(true);
    expect(diagnosisAuthInputFieldsChanged({ evidenceSnapshotID: 7 })).toBe(
      false,
    );
    expect(diagnosisAuthInputFieldsChanged({ sessionID: "session-1" })).toBe(
      false,
    );
  });
});

describe("diagnosis automatic browser-session auth checks", () => {
  it("plans a create auth check for an authenticated browser session", () => {
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        authenticatedSubject: "wecom-user-1",
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        connectionDisabledReason: "",
        connectionLastCheck: null,
        connectionValues: { authMode: "ldap" },
        createDisabledReason: "",
        createLastCheck: null,
        createValues: { authMode: "session" },
        inputRevisions: { connection: 2, create: 3 },
        previousAttemptKey: "",
        selectedSessionID: "",
      }),
    ).toEqual({
      attemptKey: "create:wecom-user-1:3",
      context: "create",
      inputRevision: 3,
      values: { authMode: "session" },
    });
  });

  it("plans a create auth check for Enterprise WeChat mode with an authenticated browser session", () => {
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        authenticatedSubject: "wecom-user-1",
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        connectionDisabledReason: "",
        connectionLastCheck: null,
        connectionValues: { authMode: "ldap" },
        createDisabledReason: "",
        createLastCheck: null,
        createValues: { authMode: "wecom" },
        inputRevisions: { connection: 2, create: 3 },
        previousAttemptKey: "",
        selectedSessionID: "",
      }),
    ).toEqual({
      attemptKey: "create:wecom-user-1:3",
      context: "create",
      inputRevision: 3,
      values: { authMode: "wecom" },
    });
  });

  it("plans a connection auth check when a diagnosis room is selected", () => {
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        authenticatedSubject: "wecom-user-1",
        backendStatus: { configured: true, mode: "wecom" },
        checking: false,
        connectionDisabledReason: "",
        connectionLastCheck: null,
        connectionValues: { authMode: "session" },
        createDisabledReason: "",
        createLastCheck: null,
        createValues: { authMode: "session" },
        inputRevisions: { connection: 5, create: 3 },
        previousAttemptKey: "",
        selectedSessionID: " diagnosis-session-1 ",
      }),
    ).toMatchObject({
      attemptKey: "connection:wecom-user-1:5",
      context: "connection",
      inputRevision: 5,
    });
  });

  it("does not plan checks while unavailable, blocked, verified, or already attempted", () => {
    const base = {
      authenticatedSubject: "wecom-user-1",
      backendStatus: { configured: true, mode: "oidc" } as const,
      checking: false,
      connectionDisabledReason: "",
      connectionLastCheck: null,
      connectionValues: { authMode: "session" as const },
      createDisabledReason: "",
      createLastCheck: null,
      createValues: { authMode: "session" as const },
      inputRevisions: { connection: 2, create: 3 },
      previousAttemptKey: "",
      selectedSessionID: "",
    };

    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        ...base,
        authenticatedSubject: " ",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        ...base,
        checking: true,
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        ...base,
        createValues: { authMode: "ldap" },
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        ...base,
        backendStatus: { configured: true, mode: "ldap" },
        createDisabledReason:
          "OpenClarion browser session could not be checked.",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        ...base,
        createLastCheck: {
          inputRevision: 3,
          message: "Authenticated as wecom-user-1.",
          mode: "session",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "wecom-user-1",
        },
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        ...base,
        previousAttemptKey: "create:wecom-user-1:3",
      }),
    ).toBeNull();
  });

  it("plans a new browser-session auth check when the authenticated subject changes", () => {
    expect(
      diagnosisAutoBrowserSessionAuthCheckPlan({
        authenticatedSubject: "wecom-user-2",
        backendStatus: { configured: true, mode: "oidc" },
        checking: false,
        connectionDisabledReason: "",
        connectionLastCheck: null,
        connectionValues: { authMode: "session" },
        createDisabledReason: "",
        createLastCheck: {
          inputRevision: 3,
          message: "Authenticated as wecom-user-1.",
          mode: "session",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "wecom-user-1",
        },
        createValues: { authMode: "session" },
        inputRevisions: { connection: 2, create: 3 },
        previousAttemptKey: "",
        selectedSessionID: "",
      }),
    ).toEqual({
      attemptKey: "create:wecom-user-2:3",
      context: "create",
      inputRevision: 3,
      values: { authMode: "session" },
    });
  });
});

describe("diagnosis automatic browser-session room connection", () => {
  it("plans a WebSocket connection after browser-session auth is verified for a selected room", () => {
    expect(
      diagnosisAutoBrowserSessionConnectionPlan({
        authenticatedSubject: "wecom-user-1",
        backendStatus: { configured: true, mode: "oidc" },
        connectionDisabledReason: "",
        connectionStatus: "idle",
        inputRevision: 5,
        lastCheck: {
          inputRevision: 5,
          message: "Authenticated as wecom-user-1.",
          mode: "session",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "wecom-user-1",
        },
        manualDisconnected: false,
        previousAttemptKey: "",
        selectedSessionID: " diagnosis-session-1 ",
        values: { authMode: "session" },
      }),
    ).toEqual({
      attemptKey: "connection:wecom-user-1:diagnosis-session-1:5",
      sessionID: "diagnosis-session-1",
    });
  });

  it("does not plan a WebSocket connection from legacy Enterprise WeChat auth", () => {
    expect(
      diagnosisAutoBrowserSessionConnectionPlan({
        authenticatedSubject: "wecom-user-1",
        backendStatus: { configured: true, mode: "wecom" },
        connectionDisabledReason: "",
        connectionStatus: "idle",
        inputRevision: 5,
        lastCheck: {
          inputRevision: 5,
          message: "Authenticated as wecom-user-1.",
          mode: "wecom",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "wecom-user-1",
        },
        manualDisconnected: false,
        previousAttemptKey: "",
        selectedSessionID: " diagnosis-session-1 ",
        values: { authMode: "wecom" },
      }),
    ).toBeNull();
  });

  it("does not auto-connect before verification, after manual disconnect, or after an existing attempt", () => {
    const base = {
      authenticatedSubject: "wecom-user-1",
      backendStatus: { configured: true, mode: "wecom" } as const,
      connectionDisabledReason: "",
      connectionStatus: "idle" as const,
      inputRevision: 5,
      lastCheck: {
        inputRevision: 5,
        message: "Authenticated as wecom-user-1.",
        mode: "session" as const,
        roleAuthorized: true,
        roles: ["owner"],
        status: "success" as const,
        subject: "wecom-user-1",
      },
      manualDisconnected: false,
      previousAttemptKey: "",
      selectedSessionID: "diagnosis-session-1",
      values: { authMode: "session" as const },
    };

    expect(
      diagnosisAutoBrowserSessionConnectionPlan({
        ...base,
        lastCheck: null,
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionConnectionPlan({
        ...base,
        manualDisconnected: true,
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionConnectionPlan({
        ...base,
        connectionStatus: "connected",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionConnectionPlan({
        ...base,
        previousAttemptKey: "connection:wecom-user-1:diagnosis-session-1:5",
      }),
    ).toBeNull();
  });
});

describe("diagnosis automatic browser-session room creation", () => {
  it("plans room creation after browser-session auth is verified for an alert snapshot", () => {
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        authenticatedSubject: "operator-1",
        backendStatus: { configured: true, mode: "oidc" },
        closeNotificationChannelProfileID: 17,
        createDisabledReason: "",
        evidenceSnapshotID: 42,
        inputRevision: 3,
        lastCheck: {
          inputRevision: 3,
          message: "Authenticated as operator-1.",
          mode: "session",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        previousAttemptKey: "",
        selectedSessionID: "",
        snapshotNeedsRoom: true,
        values: { authMode: "session" },
      }),
    ).toEqual({
      attemptKey: "create:operator-1:42:17:single:3",
      closeNotificationChannelProfileID: 17,
      evidenceSnapshotID: 42,
    });
  });

  it("plans room creation without an optional notification channel", () => {
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        authenticatedSubject: "operator-1",
        backendStatus: { configured: true, mode: "oidc" },
        closeNotificationChannelProfileID: null,
        createDisabledReason: "",
        evidenceSnapshotID: 42,
        inputRevision: 3,
        lastCheck: {
          inputRevision: 3,
          message: "Authenticated as operator-1.",
          mode: "session",
          roleAuthorized: true,
          roles: ["owner"],
          status: "success",
          subject: "operator-1",
        },
        previousAttemptKey: "",
        selectedSessionID: "",
        snapshotNeedsRoom: true,
        values: { authMode: "session" },
      }),
    ).toEqual({
      attemptKey: "create:operator-1:42:none:single:3",
      evidenceSnapshotID: 42,
    });
  });

  it("does not auto-create without a verified browser-session handoff target", () => {
    const base = {
      authenticatedSubject: "operator-1",
      backendStatus: { configured: true, mode: "oidc" } as const,
      closeNotificationChannelProfileID: 17,
      createDisabledReason: "",
      evidenceSnapshotID: 42,
      inputRevision: 3,
      lastCheck: {
        inputRevision: 3,
        message: "Authenticated as operator-1.",
        mode: "session" as const,
        roleAuthorized: true,
        roles: ["owner"],
        status: "success" as const,
        subject: "operator-1",
      },
      previousAttemptKey: "",
      selectedSessionID: "",
      snapshotNeedsRoom: true,
      values: { authMode: "session" as const },
    };

    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        authenticatedSubject: " ",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        snapshotNeedsRoom: false,
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        selectedSessionID: "diagnosis-session-1",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        values: { authMode: "ldap" },
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        createDisabledReason: "Notification channel is required.",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        evidenceSnapshotID: 0,
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        lastCheck: null,
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        lastCheck: { ...base.lastCheck, subject: "operator-2" },
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        previousAttemptKey: "create:operator-1:42:17:single:3",
      }),
    ).toBeNull();
    expect(
      diagnosisAutoBrowserSessionCreateRoomPlan({
        ...base,
        approvalMode: "owner_and_leader",
        previousAttemptKey: "create:operator-1:42:17:single:3",
      }),
    ).toMatchObject({
      attemptKey: "create:operator-1:42:17:owner_and_leader:3",
    });
  });
});
