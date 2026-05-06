import type { Interceptor, UnaryRequest, UnaryResponse } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { createAuthInterceptor } from "./connect-client";

describe("createAuthInterceptor", () => {
  it("attaches bearer token from token storage", async () => {
    const interceptor = createAuthInterceptor(() => "token-1");
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

    expect(request?.header.get("Authorization")).toBe("Bearer token-1");
  });

  it("leaves authorization unset when token storage is empty", async () => {
    const interceptor = createAuthInterceptor(() => "");
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

    expect(request?.header.has("Authorization")).toBe(false);
  });
});
