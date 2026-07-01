export function normalizeForwardedAuthorization(raw: string): string | null {
  const value = raw.trim();
  if (value === "") {
    return null;
  }
  const parts = value.split(/[ \t\r\n]+/);
  const scheme = parts[0] ?? "";
  const token = parts[1] ?? "";
  if (parts.length !== 2 || scheme.toLowerCase() !== "bearer" || token === "") {
    return null;
  }
  return `Bearer ${token}`;
}
