import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import enMessages from "../../messages/en.json";
import zhCNMessages from "../../messages/zh-CN.json";

import {
  appLocaleFromAcceptLanguage,
  defaultAppLocale,
  isAppLocale,
} from "./config";

describe("app locale configuration", () => {
  it("keeps English and Chinese message catalogs structurally aligned", () => {
    expect(messagePaths(zhCNMessages)).toEqual(messagePaths(enMessages));
  });

  it.each([
    ["en", enMessages],
    ["zh-CN", zhCNMessages],
  ] as const)("compiles every %s catalog message", (locale, messages) => {
    const translate = createTranslator({
      locale,
      messages,
      onError(error) {
        throw error;
      },
    }) as unknown as (
      key: string,
      values?: Record<string, Date | number | string>,
    ) => string;

    for (const [key, message] of messageEntries(messages)) {
      expect(() => translate(key, messageValues(message))).not.toThrow();
    }
  });

  it("recognizes only supported locales", () => {
    expect(isAppLocale("en")).toBe(true);
    expect(isAppLocale("zh-CN")).toBe(true);
    expect(isAppLocale("zh")).toBe(false);
    expect(isAppLocale(undefined)).toBe(false);
  });

  it("honors quality-weighted Chinese and English preferences", () => {
    expect(appLocaleFromAcceptLanguage("zh-HK,zh;q=0.9,en;q=0.8")).toBe(
      "zh-CN",
    );
    expect(appLocaleFromAcceptLanguage("zh;q=0.5, en-US;q=0.9")).toBe("en");
    expect(appLocaleFromAcceptLanguage("fr, zh-CN;q=0")).toBe(
      defaultAppLocale,
    );
  });

  it("falls back for missing and malformed preferences", () => {
    expect(appLocaleFromAcceptLanguage(null)).toBe(defaultAppLocale);
    expect(appLocaleFromAcceptLanguage("fr-FR, *;q=0.5")).toBe(
      defaultAppLocale,
    );
    expect(appLocaleFromAcceptLanguage("zh;q=invalid,en;q=0.7")).toBe("en");
    expect(appLocaleFromAcceptLanguage("zh;q=2,en;q=0.7")).toBe("en");
  });
});

function messagePaths(value: unknown, prefix = ""): string[] {
  if (typeof value === "string") {
    expect(value.trim()).not.toBe("");
    return [prefix];
  }
  expect(value).toBeTypeOf("object");
  expect(value).not.toBeNull();
  expect(Array.isArray(value)).toBe(false);
  return Object.entries(value as Record<string, unknown>)
    .flatMap(([key, child]) =>
      messagePaths(child, prefix === "" ? key : `${prefix}.${key}`),
    )
    .sort();
}

function messageEntries(
  value: unknown,
  prefix = "",
): Array<[key: string, message: string]> {
  if (typeof value === "string") {
    return [[prefix, value]];
  }
  return Object.entries(value as Record<string, unknown>).flatMap(
    ([key, child]) =>
      messageEntries(child, prefix === "" ? key : `${prefix}.${key}`),
  );
}

function messageValues(
  message: string,
): Record<string, Date | number | string> {
  const values: Record<string, Date | number | string> = {};
  for (const match of message.matchAll(
    /\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*(?:,\s*(plural|selectordinal|select|number|date|time))?/g,
  )) {
    const [, name, format] = match;
    if (name === undefined) {
      continue;
    }
    switch (format) {
      case "plural":
      case "selectordinal":
      case "number":
        values[name] = 2;
        break;
      case "date":
      case "time":
        values[name] = new Date(0);
        break;
      case "select":
        values[name] = "other";
        break;
      default:
        values[name] = "sample";
    }
  }
  return values;
}
