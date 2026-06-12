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

export function formatDurationSeconds(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "0s";
  }
  const units = [
    { label: "d", value: 86400 },
    { label: "h", value: 3600 },
    { label: "m", value: 60 },
    { label: "s", value: 1 }
  ];
  let remaining = Math.floor(seconds);
  const parts: string[] = [];
  for (const unit of units) {
    const count = Math.floor(remaining / unit.value);
    if (count === 0) {
      continue;
    }
    parts.push(`${count}${unit.label}`);
    remaining -= count * unit.value;
  }
  return parts.length === 0 ? "0s" : parts.join(" ");
}
