import type { TimestampMessage } from "@stringke/sdk/connect";

export function timestampToISOString(value: TimestampMessage | undefined): string | undefined {
  if (!value) return undefined;
  const millis = Number(value.seconds) * 1000 + Math.floor(value.nanos / 1_000_000);
  const date = new Date(millis);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

export function int64ToNumber(value: bigint | number | undefined): number {
  if (typeof value === "bigint") return Number(value);
  return value ?? 0;
}

export function bytesToText(value: Uint8Array): string {
  return new TextDecoder().decode(value);
}

export function bytesToBlob(value: Uint8Array, mimeType = "application/octet-stream"): Blob {
  return new Blob([value.slice()], { type: mimeType });
}

export function downloadBytes(
  value: Uint8Array,
  filename: string,
  mimeType = "application/octet-stream",
) {
  const url = URL.createObjectURL(bytesToBlob(value, mimeType));
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 0);
}

export async function fileToBytes(file: File): Promise<Uint8Array> {
  return new Uint8Array(await file.arrayBuffer());
}

export function recordValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

export function stringFromRecord(value: Record<string, unknown>, key: string): string {
  const raw = value[key];
  return typeof raw === "string" ? raw : "";
}

export function booleanFromRecord(value: Record<string, unknown>, key: string): boolean {
  return value[key] === true;
}
