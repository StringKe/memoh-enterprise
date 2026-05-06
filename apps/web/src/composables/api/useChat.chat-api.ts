import { connectClients } from "../../lib/connect-client";
import type { Bot, SessionSummary } from "./useChat.types";

const localSessionsByBot = new Map<string, SessionSummary[]>();

function createLocalId(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.floor(Math.random() * 1000)}`;
}

function upsertLocalSession(botId: string, session: SessionSummary) {
  const list = localSessionsByBot.get(botId) ?? [];
  localSessionsByBot.set(botId, [session, ...list.filter((item) => item.id !== session.id)]);
}

export async function fetchBots(): Promise<Bot[]> {
  const response = await connectClients.bots.listBots({});
  return response.bots;
}

export async function fetchSessions(botId: string): Promise<SessionSummary[]> {
  const id = botId.trim();
  if (!id) return [];
  return localSessionsByBot.get(id) ?? [];
}

export async function createSession(botId: string, title?: string): Promise<SessionSummary> {
  const id = botId.trim();
  if (!id) throw new Error("bot id is required");
  const now = new Date().toISOString();
  const session: SessionSummary = {
    id: createLocalId(),
    bot_id: id,
    channel_type: "local",
    type: "chat",
    title: title?.trim() || "New chat",
    created_at: now,
    updated_at: now,
  };
  upsertLocalSession(id, session);
  return session;
}

export async function updateSessionTitle(
  botId: string,
  sessionId: string,
  title: string,
): Promise<SessionSummary> {
  const id = botId.trim();
  const sid = sessionId.trim();
  const list = localSessionsByBot.get(id) ?? [];
  const current = list.find((item) => item.id === sid) ?? (await createSession(id, title));
  const updated = { ...current, title, updated_at: new Date().toISOString() };
  upsertLocalSession(id, updated);
  return updated;
}

export async function deleteSession(botId: string, sessionId: string): Promise<void> {
  const id = botId.trim();
  const sid = sessionId.trim();
  const list = localSessionsByBot.get(id) ?? [];
  localSessionsByBot.set(
    id,
    list.filter((item) => item.id !== sid),
  );
}

export async function deleteAllMessages(): Promise<void> {}
