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

describe("BrowserService session handlers", () => {
  it("creates and closes sessions", async () => {
    const handlers = createBrowserServiceHandlers(
      makeOperations({
        createSession: async (input) => {
          expect(input).toEqual({
            botId: "bot-1",
            core: "firefox",
            ttlMs: 60_000,
            contextConfig: { locale: "en-US" },
          });
          return {
            id: "session-1",
            wsEndpoint: "ws://127.0.0.1/session-1",
            sessionToken: "secret",
            playwrightVersion: "1.50.0",
            core: "firefox",
            contextConfig: { locale: "en-US" },
            expiresAt: "2026-05-06T00:01:00.000Z",
          };
        },
        closeSession: (id, token) => {
          expect({ id, token }).toEqual({ id: "session-1", token: "secret" });
          return true;
        },
      }),
    );

    const created = await handlers.createSession!(
      {
        botId: "bot-1",
        core: "firefox",
        ttlMs: 60_000,
        contextConfig: { locale: "en-US" },
      } as never,
      undefined as never,
    );
    expect(created).toMatchObject({
      id: "session-1",
      wsEndpoint: "ws://127.0.0.1/session-1",
      sessionToken: "secret",
      core: "firefox",
      contextConfig: { locale: "en-US" },
    });

    const closed = await handlers.closeSession!(
      { id: "session-1", token: "secret" } as never,
      undefined as never,
    );
    expect(closed.success).toBe(true);
  });
});
