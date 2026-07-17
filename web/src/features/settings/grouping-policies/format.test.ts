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
      })
    ).toEqual({
      error: { code: "token_invalid_characters", field: "dimension_key" },
      ok: false
    });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Bad severity",
        severityKey: "severity level"
      })
    ).toEqual({
      error: { code: "severity_invalid_characters" },
      ok: false
    });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Bad source",
        sourceFilterText: "prometheus east"
      })
    ).toEqual({
      error: { code: "token_invalid_characters", field: "source_filter" },
      ok: false
    });
  });

  it("returns semantic required and list-limit validation errors", () => {
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: ""
      })
    ).toEqual({ error: { code: "policy_name_required" }, ok: false });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Missing dimensions",
        dimensionKeysText: ""
      })
    ).toEqual({
      error: { code: "token_list_required", field: "dimension_key" },
      ok: false
    });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Too many dimensions",
        dimensionKeysText: Array.from(
          { length: 17 },
          (_, index) => `dimension_${index}`
        ).join("\n")
      })
    ).toEqual({
      error: {
        code: "token_list_too_long",
        field: "dimension_key",
        limit: 16
      },
      ok: false
    });
  });

  it("enforces UTF-8 byte limits before submit", () => {
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "é".repeat(61)
      })
    ).toEqual({
      error: { code: "policy_name_too_long", limit: 120 },
      ok: false
    });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Wide dimension",
        dimensionKeysText: "é".repeat(33)
      })
    ).toEqual({
      error: {
        code: "token_too_long",
        field: "dimension_key",
        limit: 64
      },
      ok: false
    });
    expect(
      formStateToWriteRequest({
        ...emptyGroupingPolicyForm(),
        name: "Wide source",
        sourceFilterText: "é".repeat(33)
      })
    ).toEqual({
      error: {
        code: "token_too_long",
        field: "source_filter",
        limit: 64
      },
      ok: false
    });
  });
});
