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
