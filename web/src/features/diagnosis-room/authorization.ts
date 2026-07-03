export type DiagnosisAuthorization =
  | { mode: "bearer"; token: string }
  | { mode: "basic"; username: string; password: string }
  | { mode: "session" };

export function normalizedDiagnosisAuthorization(
  authorization: DiagnosisAuthorization,
): DiagnosisAuthorization | null {
  if (authorization.mode === "bearer") {
    const token = authorization.token.trim();
    if (token === "" || /[\s]/.test(token)) {
      return null;
    }
    return { mode: "bearer", token };
  }
  if (authorization.mode === "session") {
    return authorization;
  }
  const username = authorization.username.trim();
  const password = authorization.password;
  if (
    username === "" ||
    password === "" ||
    /[\u0000\s]/.test(username) ||
    /[\u0000\r\n]/.test(password)
  ) {
    return null;
  }
  return { mode: "basic", username, password };
}

export function diagnosisAuthorizationHeader(
  authorization: DiagnosisAuthorization,
): string | null {
  const normalized = normalizedDiagnosisAuthorization(authorization);
  if (normalized === null) {
    return null;
  }
  if (normalized.mode === "session") {
    return "";
  }
  if (normalized.mode === "bearer") {
    return `Bearer ${normalized.token}`;
  }
  return `Basic ${base64Encode(`${normalized.username}:${normalized.password}`)}`;
}

export function diagnosisAuthorizationHeaders(
  authorization: DiagnosisAuthorization,
): HeadersInit | null {
  const header = diagnosisAuthorizationHeader(authorization);
  if (header === null) {
    return null;
  }
  return header === "" ? {} : { authorization: header };
}

function base64Encode(value: string): string {
  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}
