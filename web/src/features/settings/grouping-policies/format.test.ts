import { describe, expect, it } from "vitest";

import {
  emptyGroupingPolicyForm,
  formStateToWriteRequest,
  listToText
} from "./format";

describe("grouping policy settings formatting", () => {
  it("parses dimension and source lists in stable order", () => {
    const result = formStateToWriteRequest({
      ...emptyGroupingPolicyForm(),
      name: "Default alert grouping",
      dimensionKeysText: "service\nalertname\nservice",
      severityKey: "severity",
      sourceFilterText: "prometheus,alertmanager",
      enabled: true
    });

    expect(result).toEqual({
      ok: true,
      value: {
        name: "Default alert grouping",
        dimension_keys: ["alertname", "service"],
        severity_key: "severity",
        source_filter: ["alertmanager", "prometheus"],
        enabled: true
      }
    });
  });

  it("formats lists in stable key order", () => {
    expect(listToText(["service", "alertname"])).toBe("alertname\nservice");
  });

  it("rejects invalid tokens before submit", () => {
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Bad dimension",
        dimensionKeysText: "alert name"
      }).ok
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Bad severity",
        severityKey: "severity level"
      }).ok
    ).toBe(false);
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Bad source",
        sourceFilterText: "prometheus east"
      }).ok
    ).toBe(false);
  });
});
