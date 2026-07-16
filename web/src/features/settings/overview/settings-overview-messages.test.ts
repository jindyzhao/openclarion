import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../../messages/en.json";
import zhCN from "../../../../messages/zh-CN.json";

describe("settings overview messages", () => {
  it("keeps message keys aligned across supported locales", () => {
    expect(
      flattenedMessages(zhCN.SettingsOverview).map(([key]) => key),
    ).toEqual(flattenedMessages(en.SettingsOverview).map(([key]) => key));
  });

  it.each([
    ["en", en],
    ["zh-CN", zhCN],
  ] as const)("compiles every %s message", (locale, messages) => {
    const translate = createTranslator({
      locale,
      messages,
      namespace: "SettingsOverview",
    }) as (key: string, values?: Record<string, number | string>) => string;

    for (const [key, message] of flattenedMessages(messages.SettingsOverview)) {
      expect(() => translate(key, messageValues(message))).not.toThrow();
    }
  });

  it("localizes bounded rollout states and preserves external values", () => {
    const translate = createTranslator({
      locale: "zh-CN",
      messages: zhCN,
      namespace: "SettingsOverview",
    });

    expect(translate("topology.status.blocked")).toBe("已阻塞");
    expect(
      translate("proofTargets.autoDiagnosis.historyDetail", {
        alert: "HighLatency",
        detail: "provider status: accepted",
        room: "diagnosis-session-42",
      }),
    ).toContain("HighLatency");
    expect(
      translate("proofTargets.autoDiagnosis.historyDetail", {
        alert: "HighLatency",
        detail: "provider status: accepted",
        room: "diagnosis-session-42",
      }),
    ).toContain("provider status: accepted");
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
