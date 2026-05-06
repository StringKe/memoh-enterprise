import { describe, expect, it, vi } from "vite-plus/test";

import { consumeConnectServerStream } from "./connect-colada";

async function* signalAwareStream(signal: AbortSignal): AsyncGenerator<number> {
  yield 1;
  if (signal.aborted) return;
  await new Promise<void>((resolve) => {
    signal.addEventListener("abort", () => resolve(), { once: true });
  });
  if (!signal.aborted) yield 2;
}

describe("consumeConnectServerStream", () => {
  it("forwards stream events", async () => {
    const events: number[] = [];

    await consumeConnectServerStream({
      stream: async function* () {
        yield 1;
        yield 2;
      },
      onEvent: (event) => {
        events.push(event);
      },
    });

    expect(events).toEqual([1, 2]);
  });

  it("propagates cancellation to the stream signal", async () => {
    const controller = new AbortController();
    const onEvent = vi.fn((event: number) => {
      if (event === 1) controller.abort("stop");
    });

    await consumeConnectServerStream({
      signal: controller.signal,
      stream: signalAwareStream,
      onEvent,
    });

    expect(onEvent).toHaveBeenCalledTimes(1);
    expect(onEvent).toHaveBeenCalledWith(1);
  });
});
