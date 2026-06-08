import type {
  NotificationChannelFormState,
  NotificationChannelProfile,
  NotificationChannelProfileWriteRequest,
  NotificationDeliveryScope
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

export function emptyNotificationChannelForm(): NotificationChannelFormState {
  return {
    name: "",
    kind: "webhook",
    secretRef: "",
    deliveryScopes: ["report"],
    enabled: false,
    labelsText: ""
  };
}

export function channelToFormState(channel: NotificationChannelProfile): NotificationChannelFormState {
  return {
    name: channel.name,
    kind: channel.kind,
    secretRef: channel.secret_ref,
    deliveryScopes: channel.delivery_scopes,
    enabled: channel.enabled,
    labelsText: labelsToText(channel.labels)
  };
}

export function formStateToWriteRequest(
  form: NotificationChannelFormState
): ParseResult<NotificationChannelProfileWriteRequest> {
  const name = form.name.trim();
  if (name === "") {
    return { ok: false, message: "Channel name is required." };
  }
  if (name.length > 120) {
    return { ok: false, message: "Channel name must be 120 characters or fewer." };
  }

  const secretRef = form.secretRef.trim();
  if (secretRef === "") {
    return { ok: false, message: "Secret reference is required." };
  }
  if (/\s/.test(secretRef)) {
    return { ok: false, message: "Secret reference must not contain whitespace." };
  }

  const scopes = normalizeDeliveryScopes(form.deliveryScopes);
  if (scopes.length === 0) {
    return { ok: false, message: "Select at least one delivery scope." };
  }

  const labels = parseLabels(form.labelsText);
  if (!labels.ok) {
    return labels;
  }

  return {
    ok: true,
    value: {
      name,
      kind: form.kind,
      secret_ref: secretRef,
      delivery_scopes: scopes,
      enabled: form.enabled,
      labels: labels.value
    }
  };
}

function normalizeDeliveryScopes(scopes: NotificationDeliveryScope[]): NotificationDeliveryScope[] {
  return Array.from(new Set(scopes)).sort();
}

function parseLabels(text: string): ParseResult<Record<string, string>> {
  const trimmed = text.trim();
  if (trimmed === "") {
    return { ok: true, value: {} };
  }
  const labels: Record<string, string> = {};
  for (const rawLine of trimmed.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (line === "") {
      continue;
    }
    const eq = line.indexOf("=");
    if (eq <= 0) {
      return { ok: false, message: "Labels must use key=value lines." };
    }
    const key = line.slice(0, eq).trim();
    const value = line.slice(eq + 1).trim();
    if (key === "") {
      return { ok: false, message: "Label keys must be non-empty." };
    }
    labels[key] = value;
  }
  return { ok: true, value: labels };
}

function labelsToText(labels: Record<string, string>): string {
  return Object.entries(labels)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}
