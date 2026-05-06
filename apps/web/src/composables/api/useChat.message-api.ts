import { apiHttpUrl } from "../../lib/runtime-url";
import type {
  ChatAttachment,
  FetchMessagesOptions,
  Message,
  MessageStreamEvent,
  StreamEventHandler,
  UITurn,
} from "./useChat.types";
import { parseStreamPayload, readSSEStream } from "./useChat.sse";

interface ChannelAttachmentPayload {
  type: string;
  base64: string;
  mime: string;
  name: string;
}

interface ChannelMessagePayload {
  text?: string;
  attachments?: ChannelAttachmentPayload[];
}

function authHeaders(extra?: HeadersInit): HeadersInit {
  const token = localStorage.getItem("token") || "";
  return {
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...extra,
  };
}

function botRuntimeUrl(botId: string, path: string, query?: URLSearchParams): string {
  const url = apiHttpUrl(`/bots/${encodeURIComponent(botId)}${path}`);
  const queryString = query?.toString();
  return queryString ? `${url}?${queryString}` : url;
}

function messagesQuery(sessionId?: string, options?: FetchMessagesOptions): URLSearchParams {
  const query = new URLSearchParams();
  query.set("limit", String(options?.limit ?? 30));
  if (options?.before?.trim()) query.set("before", options.before.trim());
  if (sessionId?.trim()) query.set("session_id", sessionId.trim());
  return query;
}

async function readJsonResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  return response.json() as Promise<T>;
}

async function readStreamResponse(response: Response): Promise<ReadableStream<Uint8Array>> {
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
  if (!response.body) throw new Error("No response body");
  return response.body;
}

export async function fetchMessages(
  botId: string,
  sessionId?: string,
  options?: FetchMessagesOptions,
): Promise<Message[]> {
  const response = await fetch(
    botRuntimeUrl(botId, "/messages", messagesQuery(sessionId, options)),
    {
      headers: authHeaders(),
    },
  );
  const data = await readJsonResponse<{ items?: Message[] }>(response);
  return data.items ?? [];
}

export async function fetchMessagesUI(
  botId: string,
  sessionId?: string,
  options?: FetchMessagesOptions,
): Promise<UITurn[]> {
  const query = messagesQuery(sessionId, options);
  query.set("format", "ui");
  const response = await fetch(botRuntimeUrl(botId, "/messages", query), {
    headers: authHeaders(),
  });
  const data = await readJsonResponse<{ items?: UITurn[] }>(response);
  return data.items ?? [];
}

export interface SendMessageOverrides {
  modelId?: string;
  reasoningEffort?: string;
}

export async function sendLocalChannelMessage(
  botId: string,
  text: string,
  attachments?: ChatAttachment[],
  overrides?: SendMessageOverrides,
): Promise<void> {
  const msg: ChannelMessagePayload = {};
  const trimmedText = text.trim();
  if (trimmedText) {
    msg.text = trimmedText;
  }
  if (attachments?.length) {
    msg.attachments = attachments.map(
      (item): ChannelAttachmentPayload => ({
        type: item.type,
        base64: item.base64,
        mime: item.mime ?? "",
        name: item.name ?? "",
      }),
    );
  }
  const body: Record<string, unknown> = { message: msg };
  if (overrides?.modelId) body.model_id = overrides.modelId;
  if (overrides?.reasoningEffort) body.reasoning_effort = overrides.reasoningEffort;

  const response = await fetch(botRuntimeUrl(botId, "/local/messages"), {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    const message = await response.text();
    throw new Error(message || `Request failed with status ${response.status}`);
  }
}

export async function streamLocalChannel(
  botId: string,
  signal: AbortSignal,
  onEvent: StreamEventHandler,
): Promise<void> {
  const id = botId.trim();
  if (!id) throw new Error("bot id is required");

  const response = await fetch(botRuntimeUrl(id, "/local/stream"), {
    headers: authHeaders({ Accept: "text/event-stream" }),
    signal,
  });
  const body = await readStreamResponse(response);

  await readSSEStream(body, (payload) => {
    const event = parseStreamPayload(payload);
    if (event) onEvent(event);
  });
}

export async function streamMessageEvents(
  botId: string,
  signal: AbortSignal,
  onEvent: (event: MessageStreamEvent) => void,
  since?: string,
): Promise<void> {
  const id = botId.trim();
  if (!id) throw new Error("bot id is required");

  const query: Record<string, string> = {};
  if (since?.trim()) query.since = since.trim();

  const response = await fetch(botRuntimeUrl(id, "/messages/events", new URLSearchParams(query)), {
    headers: authHeaders({ Accept: "text/event-stream" }),
    signal,
  });
  const body = await readStreamResponse(response);

  await readSSEStream(body, (payload) => {
    try {
      const parsed = JSON.parse(payload);
      if (!parsed || typeof parsed !== "object" || !("type" in parsed)) return;
      if (typeof parsed.type !== "string" || !parsed.type.trim()) return;
      onEvent(parsed as MessageStreamEvent);
    } catch {
      // Ignore unparsable payloads
    }
  });
}
