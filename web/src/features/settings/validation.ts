const textEncoder = new TextEncoder();

export function utf8ByteLength(value: string): number {
  return textEncoder.encode(value).length;
}

export function containsControl(value: string): boolean {
  return /[\u0000-\u001f\u007f]/.test(value);
}

export function containsControlOrWhitespace(value: string): boolean {
  return /[\s\u0000-\u001f\u007f]/.test(value);
}
