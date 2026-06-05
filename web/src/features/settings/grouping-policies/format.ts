import type {
  GroupingPolicy,
  GroupingPolicyFormState,
  GroupingPolicyWriteRequest
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

export function emptyGroupingPolicyForm(): GroupingPolicyFormState {
  return {
    name: "",
    dimensionKeysText: "alertname",
    severityKey: "severity",
    sourceFilterText: "",
    enabled: false
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
  if (name.length > 120) {
    return { ok: false, message: "Policy name must be 120 characters or fewer." };
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
    if (token.length > 64) {
      return { ok: false, message: `${options.field} must be 64 characters or fewer.` };
    }
  }
  return { ok: true, value: unique };
}

function validPolicyToken(value: string): boolean {
  return !/[\s\u0000-\u001f\u007f]/.test(value);
}
