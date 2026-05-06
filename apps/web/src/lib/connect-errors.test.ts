import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { resolveConnectErrorMessage } from "./connect-errors";

describe("resolveConnectErrorMessage", () => {
  it("uses raw Connect error messages", () => {
    expect(
      resolveConnectErrorMessage(new ConnectError("token expired", Code.Unauthenticated)),
    ).toBe("token expired");
  });

  it("falls back for unknown values", () => {
    expect(resolveConnectErrorMessage(null, "fallback")).toBe("fallback");
  });
});
