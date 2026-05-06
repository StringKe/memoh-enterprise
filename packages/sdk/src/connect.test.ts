import {
  Code,
  ConnectError,
  type Interceptor,
  type UnaryRequest,
  type UnaryResponse,
} from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { createMemohAuthInterceptor, normalizeConnectError } from "./connect";
import { connectQueryKey } from "./@pinia/colada.gen";

describe("createMemohAuthInterceptor", () => {
  it("attaches bearer token", async () => {
    const interceptor = createMemohAuthInterceptor(() => "token-1");
    let request: UnaryRequest | undefined;
    const next = (async (req: UnaryRequest): Promise<UnaryResponse> => {
      request = req;
      return {
        stream: false,
        service: req.service,
        method: req.method,
        header: new Headers(),
        trailer: new Headers(),
        message: req.message,
      } as unknown as UnaryResponse;
    }) as Parameters<Interceptor>[0];

    await interceptor(next)({ header: new Headers() } as unknown as UnaryRequest);

    expect(request?.header.get("authorization")).toBe("Bearer token-1");
  });

  it("does not attach authorization when token is empty", async () => {
    const interceptor = createMemohAuthInterceptor(() => "");
    let request: UnaryRequest | undefined;
    const next = (async (req: UnaryRequest): Promise<UnaryResponse> => {
      request = req;
      return {
        stream: false,
        service: req.service,
        method: req.method,
        header: new Headers(),
        trailer: new Headers(),
        message: req.message,
      } as unknown as UnaryResponse;
    }) as Parameters<Interceptor>[0];

    await interceptor(next)({ header: new Headers() } as unknown as UnaryRequest);

    expect(request?.header.has("authorization")).toBe(false);
  });
});

describe("normalizeConnectError", () => {
  it("normalizes Connect errors", () => {
    const source = new ConnectError("expired", Code.Unauthenticated);
    const normalized = normalizeConnectError(source);

    expect(normalized.code).toBe(Code.Unauthenticated);
    expect(normalized.message).toBe("expired");
    expect(normalized.metadata).toBe(source.metadata);
    expect(normalized.cause).toBe(source.cause);
  });

  it("normalizes plain errors", () => {
    const source = new Error("failed");
    const normalized = normalizeConnectError(source);

    expect(normalized.code).toBe(Code.Unknown);
    expect(normalized.message).toBe("failed");
    expect(normalized.cause).toBe(source);
  });
});

describe("connectQueryKey", () => {
  it("keeps key parts stable", () => {
    expect(connectQueryKey("bots", 1, true, null)).toEqual(["bots", 1, true, null]);
  });
});
