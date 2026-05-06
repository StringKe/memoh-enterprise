import { Elysia } from "elysia";
import { z } from "zod";
import { getBrowser, getOrCreateBotBrowser } from "../browser";
import { BrowserContextConfigModel, type BrowserContextConfig } from "../models";
import { storage } from "../storage";
import { actionModule } from "./action";

export interface BrowserContextSummary {
  id: string;
  name: string;
  botId?: string;
  core: "chromium" | "firefox";
  config: BrowserContextConfig;
}

export interface CreateBrowserContextInput {
  id?: string;
  name?: string;
  botId?: string;
  config?: unknown;
}

function toBrowserContextSummary(
  entry: NonNullable<ReturnType<typeof storage.get>>,
): BrowserContextSummary {
  return {
    id: entry.id,
    name: entry.name,
    botId: entry.botId,
    core: entry.core,
    config: entry.config,
  };
}

export function getBrowserContext(id: string): BrowserContextSummary | null {
  const entry = storage.get(id);
  return entry ? toBrowserContextSummary(entry) : null;
}

export function listBrowserContexts(): BrowserContextSummary[] {
  return [...storage.values()].map(toBrowserContextSummary);
}

export async function createBrowserContext({
  id = crypto.randomUUID(),
  name = "",
  botId,
  config,
}: CreateBrowserContextInput): Promise<BrowserContextSummary> {
  const parsedConfig = BrowserContextConfigModel.parse(config ?? { core: "chromium" });
  const core = parsedConfig.core ?? "chromium";

  if (storage.has(id)) {
    throw new Error(`context with id "${id}" already exists`);
  }

  const browser = botId ? (await getOrCreateBotBrowser(botId, core)).browser : getBrowser(core);
  const context = await browser.newContext({
    viewport: parsedConfig.viewport,
    userAgent: parsedConfig.userAgent,
    deviceScaleFactor: parsedConfig.deviceScaleFactor,
    isMobile: parsedConfig.isMobile,
    locale: parsedConfig.locale,
    timezoneId: parsedConfig.timezoneId,
    geolocation: parsedConfig.geolocation,
    permissions: parsedConfig.permissions,
    extraHTTPHeaders: parsedConfig.extraHTTPHeaders,
    ignoreHTTPSErrors: parsedConfig.ignoreHTTPSErrors,
    proxy: parsedConfig.proxy,
  });

  storage.set(id, { id, name, botId, core, context, config: parsedConfig });
  return { id, name, botId, core, config: parsedConfig };
}

export async function closeBrowserContext(id: string): Promise<boolean> {
  const entry = storage.get(id);
  if (entry) {
    await entry.context.close();
    storage.delete(id);
  }
  return true;
}

export const contextModule = new Elysia({ prefix: "/context" })
  .use(actionModule)
  .get("/:id/exists", ({ params }) => {
    return { exists: storage.has(params.id) };
  })
  .get(
    "/",
    ({ query }) => {
      const entry = getBrowserContext(query.id);
      if (!entry) return null;
      return { id: entry.id, name: entry.name, core: entry.core, config: entry.config };
    },
    {
      query: z.object({
        id: z.string(),
      }),
    },
  )
  .post(
    "/",
    async ({ body, set }) => {
      try {
        const entry = await createBrowserContext({
          id: body.id,
          name: body.name,
          botId: body.bot_id,
          config: body.config,
        });
        return { id: entry.id, name: entry.name, core: entry.core, config: entry.config };
      } catch (error) {
        if (error instanceof Error && error.message.includes("already exists")) {
          set.status = 409;
          return { error: error.message };
        }
        throw error;
      }
    },
    {
      body: z.object({
        name: z.string().default(""),
        config: BrowserContextConfigModel.default({ core: "chromium" }),
        id: z.string().default(crypto.randomUUID()),
        bot_id: z.string().optional(),
      }),
    },
  )
  .delete("/:id", async ({ params }) => {
    await closeBrowserContext(params.id);
    return { success: true };
  })
  .get("/:id/storage-state", async ({ params, set }) => {
    const entry = storage.get(params.id);
    if (!entry) {
      set.status = 404;
      return { error: "context not found" };
    }
    return await entry.context.storageState();
  })
  .post(
    "/:id/storage-state",
    async ({ params, body, set }) => {
      const entry = storage.get(params.id);
      if (!entry) {
        set.status = 404;
        return { error: "context not found" };
      }
      if (body.cookies && Array.isArray(body.cookies)) {
        await entry.context.addCookies(body.cookies);
      }
      return { success: true };
    },
    {
      body: z.object({
        cookies: z.array(z.any()).optional(),
      }),
    },
  );
