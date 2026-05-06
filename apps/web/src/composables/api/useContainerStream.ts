import type { StreamContainerProgressRequest } from "@stringke/sdk/connect";
import { streamConnectContainerProgress } from "@/lib/connect-client";
import { recordValue } from "@/lib/connect-runtime";

export type ContainerCreateLayerStatus = {
  ref: string;
  offset: number;
  total: number;
};

export type ContainerCreateResponse = {
  data_restored?: boolean;
  [key: string]: unknown;
};

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
  signal?: AbortSignal;
  throwOnError?: boolean;
};

function layerStatusFromValue(value: unknown): ContainerCreateLayerStatus | null {
  const item = recordValue(value);
  const ref = typeof item.ref === "string" ? item.ref : "";
  const offset = typeof item.offset === "number" ? item.offset : 0;
  const total = typeof item.total === "number" ? item.total : 0;
  return ref ? { ref, offset, total } : null;
}

function normalizeContainerProgressEvent(
  type: string,
  message: string,
  payload: Record<string, unknown>,
): ContainerCreateStreamEvent | null {
  switch (type) {
    case "pulling":
      return { type, image: String(payload.image ?? message ?? "") };
    case "pull_progress":
      return {
        type,
        layers: Array.isArray(payload.layers)
          ? payload.layers
              .map(layerStatusFromValue)
              .filter((item): item is ContainerCreateLayerStatus => !!item)
          : [],
      };
    case "pull_skipped":
    case "pull_delegated":
      return { type, image: String(payload.image ?? ""), message };
    case "creating":
    case "restoring":
      return { type };
    case "complete":
      return { type, container: recordValue(payload.container ?? payload) };
    case "error":
      return { type, message: message || String(payload.error ?? "Container operation failed") };
    default:
      return null;
  }
}

export async function postBotsByBotIdContainerStream(
  options: PostBotsByBotIdContainerStreamOptions,
): Promise<ContainerCreateStreamResult> {
  const botID = options.path.bot_id.trim();
  if (!botID) throw new Error("bot id is required");

  return {
    stream: (async function* () {
      const controller = new AbortController();
      const abort = () => controller.abort(options.signal?.reason);
      if (options.signal?.aborted) {
        abort();
      } else {
        options.signal?.addEventListener("abort", abort, { once: true });
      }
      try {
        const stream = streamConnectContainerProgress(
          {
            $typeName: "memoh.private.v1.StreamContainerProgressRequest",
            botId: botID,
            operation: "create",
            options: recordValue(options.body) as unknown as NonNullable<
              StreamContainerProgressRequest["options"]
            >,
          },
          controller.signal,
        );
        for await (const event of stream) {
          const normalized = normalizeContainerProgressEvent(
            event.type,
            event.message,
            recordValue(event.payload),
          );
          if (normalized) yield normalized;
        }
      } finally {
        options.signal?.removeEventListener("abort", abort);
      }
    })(),
  };
}
