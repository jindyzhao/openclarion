import { describe, expect, it } from "vitest";

import {
  emptyGroupingPolicyForm,
  formStateToWriteRequest,
  groupingPolicyLaunchHref,
  groupingPolicyLaunchInitialForm,
  groupingPolicyLaunchIntentFromSearchParams,
  groupingPolicyLaunchIntentKey,
  listToText
} from "./format";

describe("grouping policy settings formatting", () => {
  it("parses grouping launch intents from settings overview actions", () => {
    const launch = groupingPolicyLaunchIntentFromSearchParams({ intent: "default-alert-grouping" });

    expect(launch).toEqual({
      dimensionKeysText: "alertname\nservice\nnamespace\npod",
      message: "Prepared a default alert grouping policy for alert name, service, namespace, and pod dimensions.",
      name: "Default alert grouping",
      severityKey: "severity",
      sourceFilterText: ""
    });
    expect(groupingPolicyLaunchInitialForm(launch)).toEqual({
      ...emptyGroupingPolicyForm(),
      dimensionKeysText: "alertname\nservice\nnamespace\npod",
      enabled: true,
      name: "Default alert grouping",
      severityKey: "severity",
      sourceFilterText: ""
    });
    expect(groupingPolicyLaunchIntentFromSearchParams({ intent: "unknown" })).toBeNull();
  });

  it("builds stable grouping launch hrefs and keys", () => {
    const launch = groupingPolicyLaunchIntentFromSearchParams({ intent: "default-alert-grouping" });

    expect(groupingPolicyLaunchHref({ intent: "default-alert-grouping" })).toBe(
      "/settings/grouping-policies?intent=default-alert-grouping"
    );
    expect(groupingPolicyLaunchIntentKey(launch)).toBe(
      "Default alert grouping:alertname\nservice\nnamespace\npod:severity"
    );
    expect(groupingPolicyLaunchIntentKey(null)).toBe("default");
  });

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

  it("enforces UTF-8 byte limits before submit", () => {
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "é".repeat(61)
      })
    ).toEqual({ ok: false, message: "Policy name must be 120 bytes or fewer." });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Wide dimension",
        dimensionKeysText: "é".repeat(33)
      })
    ).toEqual({ ok: false, message: "Dimension key must be 64 bytes or fewer." });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Wide source",
        sourceFilterText: "é".repeat(33)
      })
    ).toEqual({ ok: false, message: "Source filter must be 64 bytes or fewer." });
  });
});
