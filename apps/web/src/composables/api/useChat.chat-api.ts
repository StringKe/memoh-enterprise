import { connectClients } from "../../lib/connect-client";
import { apiHttpUrl } from "../../lib/runtime-url";
import type { Bot, SessionSummary } from "./useChat.types";

function authHeaders(extra?: HeadersInit): HeadersInit {
  const token = localStorage.getItem("token") || "";
  return {
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...extra,
  };
}

function botRuntimeUrl(botId: string, path: string): string {
  return apiHttpUrl(`/bots/${encodeURIComponent(botId)}${path}`);
}

async function readJsonResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function ensureOk(response: Response): Promise<void> {
  if (response.ok) return;
  const message = await response.text();
  throw new Error(message || `Request failed with status ${response.status}`);
}

export async function fetchBots(): Promise<Bot[]> {
  const response = await connectClients.bots.listBots({});
  return response.bots;
}

export async function fetchSessions(botId: string): Promise<SessionSummary[]> {
  const id = botId.trim();
  if (!id) return [];
  const response = await fetch(botRuntimeUrl(id, "/sessions"), {
    headers: authHeaders(),
  });
  const data = await readJsonResponse<{ items?: SessionSummary[] }>(response);
  return data.items ?? [];
}

export async function createSession(botId: string, title?: string): Promise<SessionSummary> {
  const id = botId.trim();
  if (!id) throw new Error("bot id is required");
  const response = await fetch(botRuntimeUrl(id, "/sessions"), {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ title: title ?? "", channel_type: "local" }),
  });
  return readJsonResponse<SessionSummary>(response);
}

export async function updateSessionTitle(
  botId: string,
  sessionId: string,
  title: string,
): Promise<SessionSummary> {
  const response = await fetch(
    botRuntimeUrl(botId.trim(), `/sessions/${encodeURIComponent(sessionId.trim())}`),
    {
      method: "PATCH",
      headers: authHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ title }),
    },
  );
  return readJsonResponse<SessionSummary>(response);
}

export async function deleteSession(botId: string, sessionId: string): Promise<void> {
  const response = await fetch(
    botRuntimeUrl(botId.trim(), `/sessions/${encodeURIComponent(sessionId.trim())}`),
    {
      method: "DELETE",
      headers: authHeaders(),
    },
  );
  await ensureOk(response);
}

export async function deleteAllMessages(botId: string): Promise<void> {
  const response = await fetch(botRuntimeUrl(botId, "/messages"), {
    method: "DELETE",
    headers: authHeaders(),
  });
  await ensureOk(response);
}
