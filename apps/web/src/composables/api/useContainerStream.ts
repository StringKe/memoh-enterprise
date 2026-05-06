import { apiHttpUrl } from "@/lib/runtime-url";

export type ContainerCreateLayerStatus = {
  ref: string;
  offset: number;
  total: number;
};

export type ContainerCreateResponse = {
  data_restored?: boolean;
  [key: string]: unknown;
};

// codesync(container-create-stream): keep these manual SSE payload types in sync
// with internal/handlers/containerd.go.
export type ContainerCreateStreamEvent =
  | { type: "pulling"; image: string }
  | { type: "pull_progress"; layers: ContainerCreateLayerStatus[] }
  | { type: "pull_skipped"; image: string; message?: string }
  | { type: "pull_delegated"; image: string; message?: string }
  | { type: "creating" }
  | { type: "restoring" }
  | { type: "complete"; container: ContainerCreateResponse }
  | { type: "error"; message: string };

export type ContainerCreateStreamResult = {
  stream: AsyncGenerator<ContainerCreateStreamEvent, void, unknown>;
};

export type PostBotsByBotIdContainerStreamOptions = {
  path: { bot_id: string };
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
  throwOnError?: boolean;
};

function isLayerStatus(value: unknown): value is ContainerCreateLayerStatus {
  return (
    !!value &&
    typeof value === "object" &&
    typeof (value as { ref?: unknown }).ref === "string" &&
    typeof (value as { offset?: unknown }).offset === "number" &&
    typeof (value as { total?: unknown }).total === "number"
  );
}

function isContainerCreateStreamEvent(value: unknown): value is ContainerCreateStreamEvent {
  if (!value || typeof value !== "object") return false;

  const event = value as Record<string, unknown>;
  switch (event.type) {
    case "pulling":
      return typeof event.image === "string";
    case "pull_progress":
      return Array.isArray(event.layers) && event.layers.every(isLayerStatus);
    case "pull_skipped":
    case "pull_delegated":
      return (
        typeof event.image === "string" &&
        (event.message === undefined || typeof event.message === "string")
      );
    case "creating":
    case "restoring":
      return true;
    case "complete":
      return !!event.container && typeof event.container === "object";
    case "error":
      return typeof event.message === "string";
    default:
      return false;
  }
}

function toError(error: unknown): Error {
  if (error instanceof Error) return error;
  if (typeof error === "string" && error.trim()) return new Error(error);
  return new Error("Container create stream failed");
}

export async function postBotsByBotIdContainerStream(
  options: PostBotsByBotIdContainerStreamOptions,
): Promise<ContainerCreateStreamResult> {
  const botID = options.path.bot_id.trim();
  if (!botID) throw new Error("bot id is required");

  const response = await fetch(apiHttpUrl(`/bots/${encodeURIComponent(botID)}/container`), {
    method: "POST",
    headers: {
      Authorization: `Bearer ${localStorage.getItem("token") || ""}`,
      Accept: "text/event-stream",
      "Content-Type": "application/json",
      ...options.headers,
    },
    body: JSON.stringify(options.body ?? {}),
    signal: options.signal,
  });
  if (!response.ok) {
    throw new Error((await response.text()) || `request failed: ${response.status}`);
  }
  if (!response.body) throw new Error("No response body");

  return {
    stream: (async function* () {
      for await (const event of readContainerSSE(response.body!)) {
        if (!isContainerCreateStreamEvent(event)) {
          throw new Error("Invalid container create stream event");
        }
        yield event;
      }
    })(),
  };
}

async function* readContainerSSE(body: ReadableStream<Uint8Array>): AsyncGenerator<unknown> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      const chunks = buffer.split("\n\n");
      buffer = chunks.pop() ?? "";

      for (const chunk of chunks) {
        const event = parseContainerSSEChunk(chunk);
        if (event !== undefined) yield event;
      }
    }

    const event = parseContainerSSEChunk(buffer);
    if (event !== undefined) yield event;
  } finally {
    reader.releaseLock();
  }
}

function parseContainerSSEChunk(chunk: string): unknown {
  for (const line of chunk.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("data:")) continue;
    const payload = trimmed.replace(/^data:\s*/, "").trim();
    if (!payload || payload === "[DONE]") continue;
    try {
      return JSON.parse(payload);
    } catch (error) {
      throw toError(error);
    }
  }
  return undefined;
}
