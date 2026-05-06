import type { BotSessionMessage, StreamChatRequest } from "@stringke/sdk/connect";
import { streamConnectChat, connectClients } from "../../lib/connect-client";
import { consumeConnectServerStream } from "../../lib/connect-colada";
import { timestampToISOString } from "../../lib/connect-runtime";
import { createConnectChatStreamNormalizer } from "./useChat.connect-stream";
import type {
  ChatAttachment,
  FetchMessagesOptions,
  Message,
  MessageStreamEvent,
  UIMessage,
  UIStreamEvent,
  UITurn,
} from "./useChat.types";

export interface SendMessageOverrides {
  modelId?: string;
  reasoningEffort?: string;
}

function botSessionMessageToMessage(message: BotSessionMessage): Message {
  const payload = message.payload as Record<string, unknown> | undefined;
  return {
    id: message.id,
    bot_id: message.botId,
    session_id: message.sessionId,
    role: message.role,
    content: payload ?? message.text,
    display_content: message.text,
    created_at: timestampToISOString(message.createdAt),
  };
}

function messageToUITurn(message: Message): UITurn {
  const timestamp = message.created_at ?? new Date().toISOString();
  if (message.role === "user") {
    return {
      id: message.id,
      role: "user",
      text: typeof message.display_content === "string" ? message.display_content : "",
      attachments: [],
      timestamp,
      platform: message.platform,
      sender_display_name: message.sender_display_name,
      sender_avatar_url: message.sender_avatar_url,
      sender_user_id: message.sender_user_id,
    };
  }

  const text =
    typeof message.display_content === "string"
      ? message.display_content
      : typeof message.content === "string"
        ? message.content
        : "";
  const data: UIMessage = { id: 0, type: "text", content: text };
  return {
    id: message.id,
    role: "assistant",
    messages: [data],
    timestamp,
    platform: message.platform,
  };
}

export async function fetchMessages(
  botId: string,
  sessionId?: string,
  options?: FetchMessagesOptions,
): Promise<Message[]> {
  const id = botId.trim();
  const sid = sessionId?.trim() ?? "";
  if (!id || !sid) return [];
  const response = await connectClients.bots.listBotSessionMessages({
    botId: id,
    sessionId: sid,
    page: {
      pageSize: options?.limit ?? 30,
      pageToken: options?.before?.trim() ?? "",
    },
  });
  return response.messages.map(botSessionMessageToMessage);
}

export async function fetchMessagesUI(
  botId: string,
  sessionId?: string,
  options?: FetchMessagesOptions,
): Promise<UITurn[]> {
  const messages = await fetchMessages(botId, sessionId, options);
  return messages.map(messageToUITurn);
}

export async function sendLocalChannelMessage(
  botId: string,
  text: string,
  attachments?: ChatAttachment[],
  overrides?: SendMessageOverrides,
): Promise<void> {
  const controller = new AbortController();
  await streamLocalChannelMessage(
    {
      botId,
      sessionId: "",
      text,
      attachments,
      overrides,
    },
    controller.signal,
    () => {},
  );
}

export async function streamLocalChannelMessage(
  input: {
    botId: string;
    sessionId: string;
    text: string;
    attachments?: ChatAttachment[];
    overrides?: SendMessageOverrides;
  },
  signal: AbortSignal,
  onEvent: (event: UIStreamEvent) => void,
): Promise<void> {
  const id = input.botId.trim();
  if (!id) throw new Error("bot id is required");

  const request: StreamChatRequest = {
    $typeName: "memoh.private.v1.StreamChatRequest",
    botId: id,
    sessionId: input.sessionId.trim(),
    message: input.text.trim(),
    attachmentIds: [],
    options: {
      attachments: input.attachments ?? [],
      model_id: input.overrides?.modelId ?? "",
      reasoning_effort: input.overrides?.reasoningEffort ?? "",
    } as unknown as NonNullable<StreamChatRequest["options"]>,
  };
  const normalize = createConnectChatStreamNormalizer();

  await consumeConnectServerStream({
    signal,
    stream: (nextSignal) => streamConnectChat(request, nextSignal),
    onEvent: (event) => {
      for (const normalized of normalize(event)) {
        onEvent(normalized);
      }
    },
  });
}

export async function streamMessageEvents(
  botId: string,
  signal: AbortSignal,
  onEvent: (event: MessageStreamEvent) => void,
  since?: string,
): Promise<void> {
  const id = botId.trim();
  if (!id || signal.aborted) return;
  const messages = await fetchMessages(id, undefined, { before: since, limit: 1 });
  for (const message of messages) {
    if (signal.aborted) return;
    onEvent({ type: "message_created", bot_id: id, message, session_id: message.session_id });
  }
}
