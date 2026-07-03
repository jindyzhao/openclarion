import type {
  GroupingPolicy,
  GroupingPolicyFormState,
  GroupingPolicyWriteRequest
} from "./types";
import {
  containsControlOrWhitespace,
  utf8ByteLength
} from "../validation";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

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
    return { ok: false, message: "Policy name is required." };
  }
  if (utf8ByteLength(name) > 120) {
    return { ok: false, message: "Policy name must be 120 bytes or fewer." };
  }
  const dimensions = parseTokenList(form.dimensionKeysText, {
    field: "Dimension key",
    maxEntries: 16,
    required: true
  });
  if (!dimensions.ok) {
    return { ok: false, message: dimensions.message };
  }
  if (severityKey === "") {
    return { ok: false, message: "Severity key is required." };
  }
  if (!validPolicyToken(severityKey)) {
    return { ok: false, message: "Severity key must not contain whitespace or control characters." };
  }
  if (severityKey.length > 64) {
    return { ok: false, message: "Severity key must be 64 characters or fewer." };
  }
  const sources = parseTokenList(form.sourceFilterText, {
    field: "Source filter",
    maxEntries: 16,
    required: false
  });
  if (!sources.ok) {
    return { ok: false, message: sources.message };
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
  options: { field: string; maxEntries: number; required: boolean }
): ParseResult<string[]> {
  const tokens = value
    .split(/[\r\n,]+/)
    .map((item) => item.trim())
    .filter((item) => item !== "");
  if (tokens.length === 0) {
    return options.required ? { ok: false, message: `${options.field} list is required.` } : { ok: true, value: [] };
  }
  const unique = [...new Set(tokens)].sort((left, right) => left.localeCompare(right));
  if (unique.length > options.maxEntries) {
    return { ok: false, message: `${options.field} list must contain ${options.maxEntries} entries or fewer.` };
  }
  for (const token of unique) {
    if (!validPolicyToken(token)) {
      return { ok: false, message: `${options.field} must not contain whitespace or control characters.` };
    }
    if (utf8ByteLength(token) > 64) {
      return { ok: false, message: `${options.field} must be 64 bytes or fewer.` };
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
