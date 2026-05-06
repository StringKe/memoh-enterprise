import type { StreamChatResponse } from "@stringke/sdk/connect";
import { describe, expect, it } from "vite-plus/test";

import { createConnectChatStreamNormalizer } from "./useChat.connect-stream";

function streamResponse(input: Partial<StreamChatResponse>): StreamChatResponse {
  return {
    $typeName: "memoh.private.v1.StreamChatResponse",
    id: "",
    type: "",
    text: "",
    payload: {},
    ...input,
  } as StreamChatResponse;
}

describe("createConnectChatStreamNormalizer", () => {
  it("passes through direct UI stream events", () => {
    const normalize = createConnectChatStreamNormalizer();

    expect(
      normalize(
        streamResponse({
          payload: {
            type: "message",
            data: { id: 1, type: "text", content: "hello" },
          },
        }),
      ),
    ).toEqual([{ type: "message", data: { id: 1, type: "text", content: "hello" } }]);
  });

  it("accumulates Connect text deltas", () => {
    const normalize = createConnectChatStreamNormalizer();

    expect(normalize(streamResponse({ type: "text_delta", text: "he" }))).toEqual([
      { type: "message", data: { id: 0, type: "text", content: "he" } },
    ]);
    expect(normalize(streamResponse({ type: "text_delta", text: "llo" }))).toEqual([
      { type: "message", data: { id: 0, type: "text", content: "hello" } },
    ]);
  });

  it("normalizes tool call lifecycle events", () => {
    const normalize = createConnectChatStreamNormalizer();

    expect(
      normalize(
        streamResponse({
          type: "tool_call_start",
          payload: {
            tool_call_id: "call-1",
            tool_name: "write",
            input: { path: "/data/a.txt" },
          },
        }),
      ),
    ).toEqual([
      {
        type: "message",
        data: {
          id: 0,
          type: "tool",
          name: "write",
          input: { path: "/data/a.txt" },
          tool_call_id: "call-1",
          running: true,
        },
      },
    ]);

    expect(
      normalize(
        streamResponse({
          type: "tool_call_end",
          payload: {
            tool_call_id: "call-1",
            output: { ok: true },
          },
        }),
      ),
    ).toEqual([
      {
        type: "message",
        data: {
          id: 0,
          type: "tool",
          name: "write",
          input: { path: "/data/a.txt" },
          output: { ok: true },
          tool_call_id: "call-1",
          running: false,
        },
      },
    ]);
  });
});
