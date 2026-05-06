import { describe, expect, it } from "vite-plus/test";
import { createTerminalSize, terminalClosedMessage, terminalOutputData } from "./connect-terminal";

describe("connect-terminal", () => {
  it("normalizes terminal dimensions", () => {
    expect(createTerminalSize(0, 24.8)).toMatchObject({ cols: 1, rows: 24 });
  });

  it("returns terminal output bytes", () => {
    const data = new Uint8Array([65, 66]);
    expect(
      terminalOutputData({
        $typeName: "memoh.private.v1.StreamTerminalResponse",
        data,
        exited: false,
        exitCode: 0,
      }),
    ).toBe(data);
  });

  it("formats close message with exit code", () => {
    expect(terminalClosedMessage(2)).toContain("2");
  });
});
