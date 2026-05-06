import { describe, expect, it } from "vite-plus/test";
import { createBrowserServiceHandlers, type BrowserServiceOperations } from "./browser-service";

function makeOperations(overrides: Partial<BrowserServiceOperations>): BrowserServiceOperations {
  return {
    createContext: async () => ({
      id: "ctx-default",
      name: "",
      core: "chromium",
      config: { core: "chromium" },
    }),
    listContexts: () => [],
    closeContext: async () => true,
    createSession: async () => ({
      id: "session-default",
      wsEndpoint: "ws://127.0.0.1/default",
      sessionToken: "token-default",
      playwrightVersion: "1.50.0",
      core: "chromium",
      contextConfig: {},
      expiresAt: "2026-05-06T00:30:00.000Z",
    }),
    closeSession: () => true,
    action: async () => ({}),
    screenshot: async () => ({ data: new Uint8Array(), mimeType: "image/png" }),
    devices: () => ({}),
    cores: () => [],
    ...overrides,
  };
}

describe("BrowserService screenshot handler", () => {
  it("returns screenshot bytes and mime type", async () => {
    const bytes = new Uint8Array([137, 80, 78, 71]);
    const handlers = createBrowserServiceHandlers(
      makeOperations({
        screenshot: async (contextId, fullPage) => {
          expect({ contextId, fullPage }).toEqual({ contextId: "ctx-1", fullPage: true });
          return { data: bytes, mimeType: "image/png" };
        },
      }),
    );

    const response = await handlers.screenshot!(
      { contextId: "ctx-1", fullPage: true } as never,
      undefined as never,
    );
    expect([...(response.data ?? new Uint8Array())]).toEqual([137, 80, 78, 71]);
    expect(response.mimeType).toBe("image/png");
  });
});
