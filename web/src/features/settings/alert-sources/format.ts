import type {
  AlertSourceFormState,
  AlertSourceLabels,
  AlertSourceProfile,
  AlertSourceProfileWriteRequest
} from "./types";

type ParseResult<T> =
  | { ok: true; value: T }
  | { ok: false; message: string };

export function emptyAlertSourceForm(): AlertSourceFormState {
  return {
    name: "",
    kind: "prometheus",
    baseURL: "",
    authMode: "none",
    secretRef: "",
    enabled: false,
    labelsText: ""
  };
}

export function profileToFormState(profile: AlertSourceProfile): AlertSourceFormState {
  return {
    name: profile.name,
    kind: profile.kind,
    baseURL: profile.base_url,
    authMode: profile.auth_mode,
    secretRef: profile.secret_ref,
    enabled: profile.enabled,
    labelsText: labelsToText(profile.labels)
  };
}

export function formStateToWriteRequest(form: AlertSourceFormState): ParseResult<AlertSourceProfileWriteRequest> {
  const name = form.name.trim();
  const baseURL = form.baseURL.trim();
  const secretRef = form.secretRef.trim();
  if (name === "") {
    return { ok: false, message: "Profile name is required." };
  }
  if (name.length > 120) {
    return { ok: false, message: "Profile name must be 120 characters or fewer." };
  }
  const urlResult = validateBaseURL(baseURL);
  if (!urlResult.ok) {
    return urlResult;
  }
  if (form.authMode === "none" && secretRef !== "") {
    return { ok: false, message: "Secret reference requires bearer auth." };
  }
  if (form.authMode === "bearer" && secretRef === "") {
    return { ok: false, message: "Bearer auth requires a secret reference." };
  }
  if (/\s/.test(secretRef)) {
    return { ok: false, message: "Secret reference must not contain whitespace." };
  }
  const labelsResult = parseLabelsText(form.labelsText);
  if (!labelsResult.ok) {
    return labelsResult;
  }
  return {
    ok: true,
    value: {
      name,
      kind: form.kind,
      base_url: baseURL,
      auth_mode: form.authMode,
      ...(secretRef === "" ? {} : { secret_ref: secretRef }),
      enabled: form.enabled,
      labels: labelsResult.value
    }
  };
}

export function parseLabelsText(value: string): ParseResult<AlertSourceLabels> {
  const labels: AlertSourceLabels = {};
  const lines = value.split(/\r?\n/);
  for (let index = 0; index < lines.length; index += 1) {
    const rawLine = lines[index] ?? "";
    const line = rawLine.trim();
    if (line === "") {
      continue;
    }
    const separator = line.indexOf("=");
    if (separator <= 0) {
      return { ok: false, message: `Label line ${index + 1} must use key=value.` };
    }
    const key = line.slice(0, separator).trim();
    const val = line.slice(separator + 1).trim();
    if (key === "") {
      return { ok: false, message: `Label line ${index + 1} has an empty key.` };
    }
    if (Object.hasOwn(labels, key)) {
      return { ok: false, message: `Label key "${key}" is duplicated.` };
    }
    if (key.length > 64 || val.length > 128) {
      return { ok: false, message: `Label line ${index + 1} exceeds the allowed length.` };
    }
    labels[key] = val;
  }
  return { ok: true, value: labels };
}

export function labelsToText(labels: AlertSourceLabels): string {
  return Object.entries(labels)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

export function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC"
  }).format(date);
}

function validateBaseURL(raw: string): ParseResult<string> {
  if (raw === "") {
    return { ok: false, message: "Base URL is required." };
  }
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return { ok: false, message: "Base URL must be a valid URL." };
  }
  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return { ok: false, message: "Base URL scheme must be http or https." };
  }
  if (parsed.username !== "" || parsed.password !== "") {
    return { ok: false, message: "Base URL must not include userinfo." };
  }
  if (parsed.search !== "" || parsed.hash !== "") {
    return { ok: false, message: "Base URL must not include query or fragment." };
  }
  return { ok: true, value: raw };
}
