export function normalizeForwardedAuthorization(raw: string): string | null {
  const fields = raw.trim().split(/[ \t\r\n]+/).filter(Boolean);
  const scheme = fields[0];
  const value = fields[1];
  if (fields.length !== 2 || scheme === undefined || value === undefined || value === "") {
    return null;
  }
  if (/^Bearer$/i.test(scheme)) {
    return `Bearer ${value}`;
  }
  if (/^Basic$/i.test(scheme)) {
    return `Basic ${value}`;
  }
  return null;
}
