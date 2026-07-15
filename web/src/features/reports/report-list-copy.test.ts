import { createTranslator } from "next-intl";
import { describe, expect, it } from "vitest";

import en from "../../../messages/en.json";
import zhCN from "../../../messages/zh-CN.json";

import {
  localizeReportConfidence,
  localizeReportSeverity,
} from "./report-list-copy";

const tEn = createTranslator({
  locale: "en",
  messages: en,
  namespace: "ReportList",
});
const tZhCN = createTranslator({
  locale: "zh-CN",
  messages: zhCN,
  namespace: "ReportList",
});

describe("report list presentation copy", () => {
  it("localizes known severity and confidence values", () => {
    expect(localizeReportSeverity("critical", tEn)).toBe("Critical");
    expect(localizeReportConfidence("medium", tEn)).toBe("Medium");
    expect(localizeReportSeverity("warning", tZhCN)).toBe("警告");
    expect(localizeReportConfidence("high", tZhCN)).toBe("高");
  });

  it("preserves unknown external values", () => {
    expect(localizeReportSeverity("vendor-severity", tZhCN)).toBe(
      "vendor-severity",
    );
    expect(localizeReportConfidence("vendor-confidence", tZhCN)).toBe(
      "vendor-confidence",
    );
  });

  it("formats report counts through each locale catalog", () => {
    expect(tEn("count", { count: 1 })).toBe("1 report");
    expect(tEn("count", { count: 2 })).toBe("2 reports");
    expect(tZhCN("count", { count: 2 })).toBe("共 2 份报告");
  });
});
