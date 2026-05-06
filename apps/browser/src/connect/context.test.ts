import { describe, expect, it } from "vite-plus/test";
import { createBrowserServiceHandlers, type BrowserServiceOperations } from "./browser-service";
import type { BrowserContextSummary } from "../modules/context";

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

describe("BrowserService context handlers", () => {
  it("creates, lists, and closes contexts", async () => {
    const contexts: BrowserContextSummary[] = [
      {
        id: "ctx-1",
        name: "main",
        botId: "bot-1",
        core: "chromium" as const,
        config: { core: "chromium" as const, viewport: { width: 1280, height: 720 } },
      },
    ];
    const handlers = createBrowserServiceHandlers(
      makeOperations({
        createContext: async (input) => {
          expect(input).toMatchObject({
            id: "ctx-1",
            name: "main",
            botId: "bot-1",
            config: { core: "chromium" },
          });
          return contexts[0]!;
        },
        listContexts: () => contexts,
        closeContext: async (id) => {
          expect(id).toBe("ctx-1");
          contexts.length = 0;
          return true;
        },
      }),
    );

    const created = await handlers.createContext!(
      { id: "ctx-1", name: "main", botId: "bot-1", config: { core: "chromium" } } as never,
      undefined as never,
    );
    expect(created.context?.id).toBe("ctx-1");
    expect(created.context?.botId).toBe("bot-1");

    const listed = await handlers.listContexts!({} as never, undefined as never);
    expect((listed.contexts ?? []).map((context) => context.id)).toEqual(["ctx-1"]);

    const closed = await handlers.closeContext!({ id: "ctx-1" } as never, undefined as never);
    expect(closed.success).toBe(true);
    expect(contexts).toEqual([]);
  });
});
