import {
  Code,
  ConnectError,
  type Transport,
  type Interceptor,
  type UnaryRequest,
  type UnaryResponse,
} from "@connectrpc/connect";
import { describe, expect, it } from "vite-plus/test";

import {
  BotService,
  BrowserService,
  ConnectorEventService,
  ConnectorService,
  connectQueryKey,
  createMemohAuthInterceptor,
  createMemohConnectClients,
  normalizeConnectError,
  RunnerService,
  RunnerSupportService,
  WorkspaceExecutorService,
} from "./connect";

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
  it("uses ConnectRPC service and method names", () => {
    expect(connectQueryKey(BotService, "ListBots", { pageSize: 20 })).toEqual([
      "connect",
      "memoh.private.v1.BotService",
      "ListBots",
      { pageSize: 20 },
    ]);
  });
});

describe("createMemohConnectClients", () => {
  it("exports protocol foundation clients", () => {
    const clients = createMemohConnectClients({
      baseUrl: "http://127.0.0.1:8080",
      transport: {} as Transport,
    });

    expect(BrowserService.typeName).toBe("memoh.browser.v1.BrowserService");
    expect(ConnectorService.typeName).toBe("memoh.connector.v1.ConnectorService");
    expect(ConnectorEventService.typeName).toBe("memoh.connector.v1.ConnectorEventService");
    expect(RunnerService.typeName).toBe("memoh.runner.v1.RunnerService");
    expect(RunnerSupportService.typeName).toBe("memoh.runner.v1.RunnerSupportService");
    expect(WorkspaceExecutorService.typeName).toBe("memoh.workspace.v1.WorkspaceExecutorService");
    expect(typeof clients.browser.listCores).toBe("function");
    expect(typeof clients.connectors.registerConnector).toBe("function");
    expect(typeof clients.connectorEvents.streamOutboundCommands).toBe("function");
    expect(typeof clients.runner.startRun).toBe("function");
    expect(typeof clients.runnerSupport.resolveRunContext).toBe("function");
    expect(typeof clients.workspaceExecutor.getWorkspaceInfo).toBe("function");
  });
});
