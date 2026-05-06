import type { StreamChatResponse } from "@stringke/sdk/connect";
import type { UIAttachment, UIMessage, UIStreamEvent, UIToolApproval } from "./useChat.types";

type LowLevelStreamEvent = {
  type: string;
  delta?: string;
  toolCallId?: string;
  toolName?: string;
  input?: unknown;
  progress?: unknown;
  output?: unknown;
  attachments?: UIAttachment[];
  approval?: UIToolApproval;
  error?: string;
  message?: string;
  status?: string;
};

type TextState = {
  id: number;
  content: string;
};

type ToolState = {
  message: Extract<UIMessage, { type: "tool" }>;
};

export function createConnectChatStreamNormalizer() {
  let nextId = 0;
  let text: TextState | null = null;
  let reasoning: TextState | null = null;
  const tools = new Map<string, ToolState>();

  function nextMessageId() {
    const id = nextId;
    nextId += 1;
    return id;
  }

  function toolState(toolCallId: string | undefined, toolName: string | undefined): ToolState {
    const trimmedId = (toolCallId ?? "").trim();
    const trimmedName = (toolName ?? "").trim();
    if (trimmedId && tools.has(trimmedId)) return tools.get(trimmedId)!;

    const state: ToolState = {
      message: {
        id: nextMessageId(),
        type: "tool",
        name: trimmedName,
        input: undefined,
        tool_call_id: trimmedId,
        running: true,
      },
    };
    if (trimmedId) tools.set(trimmedId, state);
    return state;
  }

  function fromLowLevel(event: LowLevelStreamEvent): UIStreamEvent[] {
    const type = event.type.trim().toLowerCase();
    switch (type) {
      case "processing_started":
      case "agent_start":
        return [{ type: "start" }];
      case "processing_completed":
      case "agent_end":
      case "final":
        return [{ type: "end" }];
      case "processing_failed":
      case "error":
        return [{ type: "error", message: event.error || event.message || "stream error" }];
      case "text_start":
        text = { id: nextMessageId(), content: "" };
        return [];
      case "text_delta": {
        if (!text) text = { id: nextMessageId(), content: "" };
        text.content += event.delta ?? "";
        return [{ type: "message", data: { id: text.id, type: "text", content: text.content } }];
      }
      case "text_end":
        text = null;
        return [];
      case "reasoning_start":
        reasoning = { id: nextMessageId(), content: "" };
        return [];
      case "reasoning_delta": {
        if (!reasoning) reasoning = { id: nextMessageId(), content: "" };
        reasoning.content += event.delta ?? "";
        return [
          {
            type: "message",
            data: { id: reasoning.id, type: "reasoning", content: reasoning.content },
          },
        ];
      }
      case "reasoning_end":
        reasoning = null;
        return [];
      case "tool_call_start":
      case "tool_call_progress":
      case "tool_call_end":
      case "tool_approval_request": {
        const state = toolState(event.toolCallId, event.toolName);
        if (event.toolName) state.message.name = event.toolName;
        if (event.toolCallId) state.message.tool_call_id = event.toolCallId;
        if (event.input !== undefined) state.message.input = event.input;
        if (type === "tool_call_progress") {
          state.message.progress = [...(state.message.progress ?? []), event.progress];
        }
        if (type === "tool_call_end") {
          state.message.output = event.output;
          state.message.running = false;
          if (state.message.tool_call_id) tools.delete(state.message.tool_call_id);
        } else if (type === "tool_approval_request") {
          state.message.running = false;
          state.message.approval = event.approval;
        } else {
          state.message.running = true;
        }
        text = null;
        return [{ type: "message", data: { ...state.message } }];
      }
      case "attachment_delta":
        if (!event.attachments?.length) return [];
        return [
          {
            type: "message",
            data: { id: nextMessageId(), type: "attachments", attachments: event.attachments },
          },
        ];
      default:
        return [];
    }
  }

  return (response: StreamChatResponse): UIStreamEvent[] => {
    const raw = response.payload ?? {};
    const direct = normalizeDirectUIEvent(raw);
    if (direct) return [direct];

    const lowLevel = normalizeLowLevelEvent(response);
    return lowLevel ? fromLowLevel(lowLevel) : [];
  };
}

function normalizeDirectUIEvent(raw: Record<string, unknown>): UIStreamEvent | null {
  const type = String(raw.type ?? "").trim();
  if (type === "start" || type === "end") return { type };
  if (type === "error") return { type: "error", message: String(raw.message ?? "stream error") };
  if (type === "message" && raw.data && typeof raw.data === "object") {
    return { type: "message", data: raw.data as UIMessage };
  }
  return null;
}

function normalizeLowLevelEvent(response: StreamChatResponse): LowLevelStreamEvent | null {
  const raw = response.payload ?? {};
  const type = String(raw.type || response.type || "")
    .trim()
    .toLowerCase();
  if (!type) {
    return response.text ? { type: "text_delta", delta: response.text } : null;
  }

  if (type === "status") {
    const status = String(raw.status ?? "")
      .trim()
      .toLowerCase();
    if (status === "started") return { type: "processing_started" };
    if (status === "completed") return { type: "processing_completed" };
    if (status === "failed") {
      const error = String(raw.error ?? raw.message ?? response.text ?? "").trim();
      return { type: "processing_failed", error };
    }
    return null;
  }

  if (type === "delta") {
    const phase = String(raw.phase ?? "")
      .trim()
      .toLowerCase();
    return {
      type: phase === "reasoning" ? "reasoning_delta" : "text_delta",
      delta: String(raw.delta ?? response.text ?? ""),
    };
  }

  if (type === "phase_start" || type === "phase_end") {
    const phase = String(raw.phase ?? "")
      .trim()
      .toLowerCase();
    if (phase !== "text" && phase !== "reasoning") return null;
    return { type: `${phase}_${type === "phase_start" ? "start" : "end"}` };
  }

  if (type === "tool_call_start" || type === "tool_call_progress" || type === "tool_call_end") {
    const toolCall =
      raw.tool_call && typeof raw.tool_call === "object"
        ? (raw.tool_call as Record<string, unknown>)
        : raw;
    return {
      type,
      toolCallId: String(toolCall.call_id ?? toolCall.toolCallId ?? toolCall.tool_call_id ?? ""),
      toolName: String(toolCall.name ?? toolCall.toolName ?? toolCall.tool_name ?? ""),
      input: toolCall.input,
      progress: toolCall.progress,
      output: toolCall.output ?? toolCall.result,
    };
  }

  if (type === "tool_approval_request") {
    return {
      type,
      toolCallId: String(raw.tool_call_id ?? raw.toolCallId ?? ""),
      toolName: String(raw.tool_name ?? raw.toolName ?? ""),
      input: raw.input,
      approval: {
        approval_id: String(raw.approval_id ?? ""),
        short_id: typeof raw.short_id === "number" ? raw.short_id : undefined,
        status: String(raw.status ?? "pending"),
        can_approve: true,
      },
    };
  }

  if (type === "attachment_delta" || type === "attachment") {
    return {
      type: "attachment_delta",
      attachments: Array.isArray(raw.attachments)
        ? (raw.attachments as unknown as UIAttachment[])
        : [],
    };
  }

  if (
    type === "text_start" ||
    type === "text_end" ||
    type === "reasoning_start" ||
    type === "reasoning_end" ||
    type === "processing_started" ||
    type === "processing_completed" ||
    type === "agent_start" ||
    type === "agent_end" ||
    type === "final"
  ) {
    return { type };
  }

  if (type === "text_delta" || type === "reasoning_delta") {
    return { type, delta: String(raw.delta ?? response.text ?? "") };
  }

  if (type === "processing_failed" || type === "error") {
    return { type, error: String(raw.error ?? raw.message ?? response.text ?? "stream error") };
  }

  return response.text ? { type: "text_delta", delta: response.text } : null;
}
