import { beforeEach, describe, expect, it, vi } from "vite-plus/test";

import { resolveAuthRedirect } from "./router-auth";

describe("resolveAuthRedirect", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it("redirects unauthenticated settings routes to login", () => {
    vi.stubGlobal("localStorage", { getItem: vi.fn(() => "") });

    expect(resolveAuthRedirect({ fullPath: "/settings/bots", path: "/settings/bots" })).toEqual({
      name: "Login",
    });
  });

  it("allows unauthenticated OAuth callbacks", () => {
    vi.stubGlobal("localStorage", { getItem: vi.fn(() => "") });

    expect(
      resolveAuthRedirect({
        fullPath: "/oauth/mcp/callback?code=1",
        path: "/oauth/mcp/callback",
      }),
    ).toBe(true);
  });

  it("redirects authenticated login route to home", () => {
    vi.stubGlobal("localStorage", { getItem: vi.fn(() => "token-1") });

    expect(resolveAuthRedirect({ fullPath: "/login", path: "/login" })).toEqual({ path: "/" });
  });
});
