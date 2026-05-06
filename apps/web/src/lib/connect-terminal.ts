import type { StreamTerminalResponse, TerminalSize } from "@stringke/sdk/connect";

export function createTerminalSize(cols: number, rows: number): TerminalSize {
  return {
    $typeName: "memoh.private.v1.TerminalSize",
    cols: Math.max(1, Math.floor(cols)),
    rows: Math.max(1, Math.floor(rows)),
  };
}

export function terminalOutputData(event: StreamTerminalResponse): Uint8Array | string {
  return event.data.length > 0 ? event.data : "";
}

export function terminalClosedMessage(exitCode: number): string {
  return `\r\n\x1b[31m[Connection closed: ${exitCode}]\x1b[0m\r\n`;
}
