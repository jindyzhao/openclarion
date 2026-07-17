import type {
  GroupingPolicy,
  GroupingPolicyFormState,
  GroupingPolicyWriteRequest
} from "./types";
import {
  containsControlOrWhitespace,
  utf8ByteLength
} from "../validation";

type GroupingPolicyTokenField = "dimension_key" | "source_filter";

export type GroupingPolicyValidationError =
  | { code: "policy_name_required" }
  | { code: "policy_name_too_long"; limit: number }
  | { code: "token_list_required"; field: GroupingPolicyTokenField }
  | {
      code: "token_list_too_long";
      field: GroupingPolicyTokenField;
      limit: number;
    }
  | { code: "token_invalid_characters"; field: GroupingPolicyTokenField }
  | {
      code: "token_too_long";
      field: GroupingPolicyTokenField;
      limit: number;
    }
  | { code: "severity_required" }
  | { code: "severity_invalid_characters" }
  | { code: "severity_too_long"; limit: number };

type ParseResult<T> =
  | { ok: true; value: T }
  | { error: GroupingPolicyValidationError; ok: false };

type SearchParamValue = string | string[] | undefined;

export type GroupingPolicyLaunchIntentName = "default-alert-grouping";

export type GroupingPolicyLaunchIntent = {
  dimensionKeysText: string;
  message: string;
  name: string;
  severityKey: string;
  sourceFilterText: string;
};

export function emptyGroupingPolicyForm(): GroupingPolicyFormState {
  return {
    name: "",
    dimensionKeysText: "alertname",
    severityKey: "severity",
    sourceFilterText: "",
    enabled: false
  };
}

export function groupingPolicyLaunchHref({
  intent
}: {
  intent: GroupingPolicyLaunchIntentName;
}): string {
  const params = new URLSearchParams({ intent });
  return `/settings/grouping-policies?${params.toString()}`;
}

export function groupingPolicyLaunchIntentFromSearchParams(
  searchParams: Record<string, SearchParamValue>
): GroupingPolicyLaunchIntent | null {
  switch (firstSearchParamValue(searchParams.intent)) {
    case "default-alert-grouping":
      return {
        dimensionKeysText: "alertname\nservice\nnamespace\npod",
        message:
          "Prepared a default alert grouping policy for alert name, service, namespace, and pod dimensions.",
        name: "Default alert grouping",
        severityKey: "severity",
        sourceFilterText: ""
      };
    default:
      return null;
  }
}

export function groupingPolicyLaunchIntentKey(launchIntent: GroupingPolicyLaunchIntent | null): string {
  if (launchIntent === null) {
    return "default";
  }
  return `${launchIntent.name}:${launchIntent.dimensionKeysText}:${launchIntent.severityKey}`;
}

export function groupingPolicyLaunchInitialForm(
  launchIntent: GroupingPolicyLaunchIntent | null
): GroupingPolicyFormState {
  if (launchIntent === null) {
    return emptyGroupingPolicyForm();
  }
  return {
    ...emptyGroupingPolicyForm(),
    dimensionKeysText: launchIntent.dimensionKeysText,
    enabled: true,
    name: launchIntent.name,
    severityKey: launchIntent.severityKey,
    sourceFilterText: launchIntent.sourceFilterText
  };
}

export function policyToFormState(policy: GroupingPolicy): GroupingPolicyFormState {
  return {
    name: policy.name,
    dimensionKeysText: listToText(policy.dimension_keys),
    severityKey: policy.severity_key,
    sourceFilterText: listToText(policy.source_filter),
    enabled: policy.enabled
  };
}

export function formStateToWriteRequest(form: GroupingPolicyFormState): ParseResult<GroupingPolicyWriteRequest> {
  const name = form.name.trim();
  const severityKey = form.severityKey.trim();
  if (name === "") {
    return { error: { code: "policy_name_required" }, ok: false };
  }
  if (utf8ByteLength(name) > 120) {
    return {
      error: { code: "policy_name_too_long", limit: 120 },
      ok: false
    };
  }
  const dimensions = parseTokenList(form.dimensionKeysText, {
    field: "dimension_key",
    maxEntries: 16,
    required: true
  });
  if (!dimensions.ok) {
    return dimensions;
  }
  if (severityKey === "") {
    return { error: { code: "severity_required" }, ok: false };
  }
  if (!validPolicyToken(severityKey)) {
    return {
      error: { code: "severity_invalid_characters" },
      ok: false
    };
  }
  if (severityKey.length > 64) {
    return {
      error: { code: "severity_too_long", limit: 64 },
      ok: false
    };
  }
  const sources = parseTokenList(form.sourceFilterText, {
    field: "source_filter",
    maxEntries: 16,
    required: false
  });
  if (!sources.ok) {
    return sources;
  }
  return {
    ok: true,
    value: {
      name,
      dimension_keys: dimensions.value,
      severity_key: severityKey,
      source_filter: sources.value,
      enabled: form.enabled
    }
  };
}

export function listToText(values: string[]): string {
  return [...values].sort((left, right) => left.localeCompare(right)).join("\n");
}

function parseTokenList(
  value: string,
  options: {
    field: GroupingPolicyTokenField;
    maxEntries: number;
    required: boolean;
  }
): ParseResult<string[]> {
  const tokens = value
    .split(/[\r\n,]+/)
    .map((item) => item.trim())
    .filter((item) => item !== "");
  if (tokens.length === 0) {
    return options.required
      ? {
          error: { code: "token_list_required", field: options.field },
          ok: false
        }
      : { ok: true, value: [] };
  }
  const unique = [...new Set(tokens)].sort((left, right) => left.localeCompare(right));
  if (unique.length > options.maxEntries) {
    return {
      error: {
        code: "token_list_too_long",
        field: options.field,
        limit: options.maxEntries
      },
      ok: false
    };
  }
  for (const token of unique) {
    if (!validPolicyToken(token)) {
      return {
        error: {
          code: "token_invalid_characters",
          field: options.field
        },
        ok: false
      };
    }
    if (utf8ByteLength(token) > 64) {
      return {
        error: {
          code: "token_too_long",
          field: options.field,
          limit: 64
        },
        ok: false
      };
    }
  }
  return { ok: true, value: unique };
}

function validPolicyToken(value: string): boolean {
  return !containsControlOrWhitespace(value);
}

function firstSearchParamValue(value: SearchParamValue): string | null {
  if (Array.isArray(value)) {
    return value[0] ?? null;
  }
  return value ?? null;
}
