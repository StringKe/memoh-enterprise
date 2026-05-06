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

describe("BrowserService action handlers", () => {
  it("maps navigate and generic action requests", async () => {
    const calls: unknown[] = [];
    const handlers = createBrowserServiceHandlers(
      makeOperations({
        action: async (contextId, request) => {
          calls.push({ contextId, request });
          if (request.action === "navigate") return { url: request.url, status: 200 };
          return { clicked: request.selector };
        },
      }),
    );

    const navigate = await handlers.navigate!(
      { contextId: "ctx-1", url: "https://example.test", timeout: 1234 } as never,
      undefined as never,
    );
    expect(navigate.data).toEqual({ url: "https://example.test", status: 200 });

    const action = await handlers.action!(
      { contextId: "ctx-1", action: "click", selector: "#submit", timeout: 5000 } as never,
      undefined as never,
    );
    expect(action.data).toEqual({ clicked: "#submit" });
    expect(calls).toEqual([
      {
        contextId: "ctx-1",
        request: { action: "navigate", url: "https://example.test", timeout: 1234 },
      },
      {
        contextId: "ctx-1",
        request: { action: "click", selector: "#submit", timeout: 5000 },
      },
    ]);
  });
});
