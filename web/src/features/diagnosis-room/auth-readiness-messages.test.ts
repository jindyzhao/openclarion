import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";

import {
  diagnosisAuthBackendReadinessStatusLabel,
  diagnosisAuthBrowserSessionAuthenticatedSummary,
  diagnosisAuthInputReadiness,
  diagnosisAuthOIDCBFFReadinessDetail,
  diagnosisAuthRolloutReadiness,
} from "./auth-readiness";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "DiagnosisAuth",
});
const tZhCN = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "DiagnosisAuth",
});

describe("diagnosis auth messages", () => {
  it("keeps diagnosis auth message keys aligned across locales", () => {
    expect(
      flattenedMessages(zhCN.DiagnosisAuth).map(([key]) => key),
    ).toEqual(flattenedMessages(en.DiagnosisAuth).map(([key]) => key));
  });

  it.each([
    ["en", en],
    ["zh-CN", zhCN],
  ] as const)(
    "compiles every %s diagnosis auth message",
    (locale, messages) => {
      const translate = createTranslator({
        locale,
        messages,
        namespace: "DiagnosisAuth",
      }) as (key: string, values?: Record<string, number | string>) => string;

      for (const [key, message] of flattenedMessages(messages.DiagnosisAuth)) {
        expect(() => translate(key, messageValues(message))).not.toThrow();
      }
    },
  );

  it("localizes stable readiness states in both supported locales", () => {
    expect(
      diagnosisAuthInputReadiness({ authMode: "session" }, tEn).label,
    ).toBe("IAM browser session ready to check.");
    expect(
      diagnosisAuthInputReadiness({ authMode: "session" }, tZhCN).label,
    ).toBe("IAM 浏览器会话已可检查。");
    expect(diagnosisAuthBackendReadinessStatusLabel("needs_check", tZhCN)).toBe(
      "需要检查",
    );
    expect(
      diagnosisAuthRolloutReadiness(
        {
          detail: "",
          mode: "session",
          roles: [],
          status: "pending",
          subject: "",
        },
        tZhCN,
      ).detail,
    ).toContain("当前 IAM 浏览器会话");
  });

  it("preserves external identity values in localized session summaries", () => {
    const summary = diagnosisAuthBrowserSessionAuthenticatedSummary(
      {
        mode: "oidc",
        roles: ["incident-commander", "auditor"],
        subject: "operator@example.com",
      },
      tZhCN,
    );

    expect(summary.detail).toContain("operator@example.com");
    expect(summary.detail).toContain("incident-commander, auditor");
    expect(summary.detail).toContain("IAM OIDC");
    expect(summary.detail).not.toContain("Signed in as");
  });

  it("localizes every OIDC BFF prerequisite without dropping scope gaps", () => {
    expect(
      diagnosisAuthOIDCBFFReadinessDetail(
        {
          missing: [
            "client_auth_method",
            "client_id",
            "client_secret",
            "email_scope",
            "issuer",
            "openid_scope",
            "pkce",
            "profile_scope",
            "session_signing_key",
            "state_signing_key",
          ],
          status: "blocked",
        },
        tZhCN,
      ),
    ).toBe(
      "缺少客户端认证方式、客户端 ID、客户端密钥、email 范围、签发方、openid 范围、公共客户端 PKCE、profile 范围、浏览器会话签名密钥、状态签名密钥",
    );
  });
});

function flattenedMessages(
  value: Record<string, unknown>,
  prefix = "",
): Array<[key: string, message: string]> {
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix === "" ? key : `${prefix}.${key}`;
    return typeof child === "string"
      ? [[path, child]]
      : flattenedMessages(child as Record<string, unknown>, path);
  });
}

function messageValues(message: string): Record<string, number | string> {
  const values: Record<string, number | string> = {};
  for (const match of message.matchAll(
    /\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:,\s*(plural|select))?/g,
  )) {
    const [, name, format] = match;
    if (name === undefined || name in values) {
      continue;
    }
    values[name] =
      format === "plural" ? 2 : format === "select" ? "other" : "sample";
  }
  return values;
}
