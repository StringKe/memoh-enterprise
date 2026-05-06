import type { JsonObject, JsonValue, Message } from "@bufbuild/protobuf";
import type { GenFile, GenMessage, GenService } from "@bufbuild/protobuf/codegenv2";
import { fileDesc, messageDesc, serviceDesc } from "@bufbuild/protobuf/codegenv2";
import { file_google_protobuf_struct } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createConnectRouter, type ServiceImpl } from "@connectrpc/connect";
import { createFetchHandler } from "@connectrpc/connect/protocol";
import { ActionRequestModel, type ActionRequest as ModuleActionRequest } from "../models";
import { captureScreenshot, executeAction } from "../modules/action";
import {
  closeBrowserContext,
  createBrowserContext,
  listBrowserContexts,
  type BrowserContextSummary,
} from "../modules/context";
import { listCores } from "../modules/cores";
import { listDevices } from "../modules/devices";
import {
  closeRemoteSession,
  createRemoteSession,
  type CreateRemoteSessionResult,
} from "../modules/session";
import type { BrowserCore } from "../browser";

export const file_memoh_browser_v1_browser: GenFile = fileDesc(
  "Ch5tZW1vaC9icm93c2VyL3YxL2Jyb3dzZXIucHJvdG8SEG1lbW9oLmJyb3dzZXIudjEaHGdvb2dsZS9wcm90b2J1Zi9zdHJ1Y3QucHJvdG8ioAEKDkJyb3dzZXJDb250ZXh0Eg4KAmlkGAEgASgJUgJpZBISCgRuYW1lGAIgASgJUgRuYW1lEhIKBGNvcmUYAyABKAlSBGNvcmUSLwoGY29uZmlnGAQgASgLMhcuZ29vZ2xlLnByb3RvYnVmLlN0cnVjdFIGY29uZmlnEhoKBmJvdF9pZBgFIAEoCUgAUgVib3RJZIgBAUIJCgdfYm90X2lkIp4BChRDcmVhdGVDb250ZXh0UmVxdWVzdBITCgJpZBgBIAEoCUgAUgJpZIgBARISCgRuYW1lGAIgASgJUgRuYW1lEi8KBmNvbmZpZxgDIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RSBmNvbmZpZxIaCgZib3RfaWQYBCABKAlIAVIFYm90SWSIAQFCBQoDX2lkQgkKB19ib3RfaWQiUwoVQ3JlYXRlQ29udGV4dFJlc3BvbnNlEjoKB2NvbnRleHQYASABKAsyIC5tZW1vaC5icm93c2VyLnYxLkJyb3dzZXJDb250ZXh0Ugdjb250ZXh0IhUKE0xpc3RDb250ZXh0c1JlcXVlc3QiVAoUTGlzdENvbnRleHRzUmVzcG9uc2USPAoIY29udGV4dHMYASADKAsyIC5tZW1vaC5icm93c2VyLnYxLkJyb3dzZXJDb250ZXh0Ughjb250ZXh0cyIlChNDbG9zZUNvbnRleHRSZXF1ZXN0Eg4KAmlkGAEgASgJUgJpZCIwChRDbG9zZUNvbnRleHRSZXNwb25zZRIYCgdzdWNjZXNzGAEgASgIUgdzdWNjZXNzIrYBChRDcmVhdGVTZXNzaW9uUmVxdWVzdBIVCgZib3RfaWQYASABKAlSBWJvdElkEhcKBGNvcmUYAiABKAlIAFIEY29yZYgBARIaCgZ0dGxfbXMYAyABKA1IAVIFdHRsTXOIAQESPgoOY29udGV4dF9jb25maWcYBCABKAsyFy5nb29nbGUucHJvdG9idWYuU3RydWN0Ug1jb250ZXh0Q29uZmlnQgcKBV9jb3JlQgkKB190dGxfbXMijwIKFUNyZWF0ZVNlc3Npb25SZXNwb25zZRIOCgJpZBgBIAEoCVICaWQSHwoLd3NfZW5kcG9pbnQYAiABKAlSCndzRW5kcG9pbnQSIwoNc2Vzc2lvbl90b2tlbhgDIAEoCVIMc2Vzc2lvblRva2VuEi0KEnBsYXl3cmlnaHRfdmVyc2lvbhgEIAEoCVIRcGxheXdyaWdodFZlcnNpb24SEgoEY29yZRgFIAEoCVIEY29yZRI+Cg5jb250ZXh0X2NvbmZpZxgGIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RSDWNvbnRleHRDb25maWcSHQoKZXhwaXJlc19hdBgHIAEoCVIJZXhwaXJlc0F0IjsKE0Nsb3NlU2Vzc2lvblJlcXVlc3QSDgoCaWQYASABKAlSAmlkEhQKBXRva2VuGAIgASgJUgV0b2tlbiIwChRDbG9zZVNlc3Npb25SZXNwb25zZRIYCgdzdWNjZXNzGAEgASgIUgdzdWNjZXNzIm0KD05hdmlnYXRlUmVxdWVzdBIdCgpjb250ZXh0X2lkGAEgASgJUgljb250ZXh0SWQSEAoDdXJsGAIgASgJUgN1cmwSHQoHdGltZW91dBgDIAEoDUgAUgd0aW1lb3V0iAEBQgoKCF90aW1lb3V0It0ECg1BY3Rpb25SZXF1ZXN0Eh0KCmNvbnRleHRfaWQYASABKAlSCWNvbnRleHRJZBIWCgZhY3Rpb24YAiABKAlSBmFjdGlvbhIVCgN1cmwYAyABKAlIAFIDdXJsiAEBEh8KCHNlbGVjdG9yGAQgASgJSAFSCHNlbGVjdG9yiAEBEhcKBHRleHQYBSABKAlIAlIEdGV4dIgBARIbCgZzY3JpcHQYBiABKAlIA1IGc2NyaXB0iAEBEhUKA2tleRgHIAEoCUgEUgNrZXmIAQESGQoFdmFsdWUYCCABKAlIBVIFdmFsdWWIAQESLAoPdGFyZ2V0X3NlbGVjdG9yGAkgASgJSAZSDnRhcmdldFNlbGVjdG9yiAEBEhQKBWZpbGVzGAogAygJUgVmaWxlcxIgCglmdWxsX3BhZ2UYCyABKAhIB1IIZnVsbFBhZ2WIAQESIAoJdGFiX2luZGV4GAwgASgNSAhSCHRhYkluZGV4iAEBEiEKCWRpcmVjdGlvbhgNIAEoCUgJUglkaXJlY3Rpb26IAQESGwoGYW1vdW50GA4gASgNSApSBmFtb3VudIgBARIdCgd0aW1lb3V0GA8gASgNSAtSB3RpbWVvdXSIAQFCBgoEX3VybEILCglfc2VsZWN0b3JCBwoFX3RleHRCCQoHX3NjcmlwdEIGCgRfa2V5QggKBl92YWx1ZUISChBfdGFyZ2V0X3NlbGVjdG9yQgwKCl9mdWxsX3BhZ2VCDAoKX3RhYl9pbmRleEIMCgpfZGlyZWN0aW9uQgkKB19hbW91bnRCCgoIX3RpbWVvdXQiPQoOQWN0aW9uUmVzcG9uc2USKwoEZGF0YRgBIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RSBGRhdGEiYgoRU2NyZWVuc2hvdFJlcXVlc3QSHQoKY29udGV4dF9pZBgBIAEoCVIJY29udGV4dElkEiAKCWZ1bGxfcGFnZRgCIAEoCEgAUghmdWxsUGFnZYgBAUIMCgpfZnVsbF9wYWdlIkUKElNjcmVlbnNob3RSZXNwb25zZRISCgRkYXRhGAEgASgMUgRkYXRhEhsKCW1pbWVfdHlwZRgCIAEoCVIIbWltZVR5cGUiEAoORGV2aWNlc1JlcXVlc3QiWAoNQnJvd3NlckRldmljZRIOCgJpZBgBIAEoCVICaWQSNwoKZGVzY3JpcHRvchgCIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RSCmRlc2NyaXB0b3IiTAoPRGV2aWNlc1Jlc3BvbnNlEjkKB2RldmljZXMYASADKAsyHy5tZW1vaC5icm93c2VyLnYxLkJyb3dzZXJEZXZpY2VSB2RldmljZXMiDgoMQ29yZXNSZXF1ZXN0IiUKDUNvcmVzUmVzcG9uc2USFAoFY29yZXMYASADKAlSBWNvcmVzMoIHCg5Ccm93c2VyU2VydmljZRJgCg1DcmVhdGVDb250ZXh0EiYubWVtb2guYnJvd3Nlci52MS5DcmVhdGVDb250ZXh0UmVxdWVzdBonLm1lbW9oLmJyb3dzZXIudjEuQ3JlYXRlQ29udGV4dFJlc3BvbnNlEl0KDExpc3RDb250ZXh0cxIlLm1lbW9oLmJyb3dzZXIudjEuTGlzdENvbnRleHRzUmVxdWVzdBomLm1lbW9oLmJyb3dzZXIudjEuTGlzdENvbnRleHRzUmVzcG9uc2USXQoMQ2xvc2VDb250ZXh0EiUubWVtb2guYnJvd3Nlci52MS5DbG9zZUNvbnRleHRSZXF1ZXN0GiYubWVtb2guYnJvd3Nlci52MS5DbG9zZUNvbnRleHRSZXNwb25zZRJgCg1DcmVhdGVTZXNzaW9uEiYubWVtb2guYnJvd3Nlci52MS5DcmVhdGVTZXNzaW9uUmVxdWVzdBonLm1lbW9oLmJyb3dzZXIudjEuQ3JlYXRlU2Vzc2lvblJlc3BvbnNlEl0KDENsb3NlU2Vzc2lvbhIlLm1lbW9oLmJyb3dzZXIudjEuQ2xvc2VTZXNzaW9uUmVxdWVzdBomLm1lbW9oLmJyb3dzZXIudjEuQ2xvc2VTZXNzaW9uUmVzcG9uc2USTwoITmF2aWdhdGUSIS5tZW1vaC5icm93c2VyLnYxLk5hdmlnYXRlUmVxdWVzdBogLm1lbW9oLmJyb3dzZXIudjEuQWN0aW9uUmVzcG9uc2USSwoGQWN0aW9uEh8ubWVtb2guYnJvd3Nlci52MS5BY3Rpb25SZXF1ZXN0GiAubWVtb2guYnJvd3Nlci52MS5BY3Rpb25SZXNwb25zZRJXCgpTY3JlZW5zaG90EiMubWVtb2guYnJvd3Nlci52MS5TY3JlZW5zaG90UmVxdWVzdBokLm1lbW9oLmJyb3dzZXIudjEuU2NyZWVuc2hvdFJlc3BvbnNlEk4KB0RldmljZXMSIC5tZW1vaC5icm93c2VyLnYxLkRldmljZXNSZXF1ZXN0GiEubWVtb2guYnJvd3Nlci52MS5EZXZpY2VzUmVzcG9uc2USSAoFQ29yZXMSHi5tZW1vaC5icm93c2VyLnYxLkNvcmVzUmVxdWVzdBofLm1lbW9oLmJyb3dzZXIudjEuQ29yZXNSZXNwb25zZWIGcHJvdG8z",
  [file_google_protobuf_struct],
);

export type BrowserContext = Message<"memoh.browser.v1.BrowserContext"> & {
  id: string;
  name: string;
  core: string;
  config?: JsonObject;
  botId?: string;
};
export const BrowserContextSchema: GenMessage<BrowserContext> = messageDesc(
  file_memoh_browser_v1_browser,
  0,
);

export type CreateContextRequest = Message<"memoh.browser.v1.CreateContextRequest"> & {
  id?: string;
  name: string;
  config?: JsonObject;
  botId?: string;
};
export const CreateContextRequestSchema: GenMessage<CreateContextRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  1,
);

export type CreateContextResponse = Message<"memoh.browser.v1.CreateContextResponse"> & {
  context?: BrowserContext;
};
export const CreateContextResponseSchema: GenMessage<CreateContextResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  2,
);

export type ListContextsRequest = Message<"memoh.browser.v1.ListContextsRequest"> & {};
export const ListContextsRequestSchema: GenMessage<ListContextsRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  3,
);

export type ListContextsResponse = Message<"memoh.browser.v1.ListContextsResponse"> & {
  contexts: BrowserContext[];
};
export const ListContextsResponseSchema: GenMessage<ListContextsResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  4,
);

export type CloseContextRequest = Message<"memoh.browser.v1.CloseContextRequest"> & { id: string };
export const CloseContextRequestSchema: GenMessage<CloseContextRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  5,
);

export type CloseContextResponse = Message<"memoh.browser.v1.CloseContextResponse"> & {
  success: boolean;
};
export const CloseContextResponseSchema: GenMessage<CloseContextResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  6,
);

export type CreateSessionRequest = Message<"memoh.browser.v1.CreateSessionRequest"> & {
  botId: string;
  core?: string;
  ttlMs?: number;
  contextConfig?: JsonObject;
};
export const CreateSessionRequestSchema: GenMessage<CreateSessionRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  7,
);

export type CreateSessionResponse = Message<"memoh.browser.v1.CreateSessionResponse"> & {
  id: string;
  wsEndpoint: string;
  sessionToken: string;
  playwrightVersion: string;
  core: string;
  contextConfig?: JsonObject;
  expiresAt: string;
};
export const CreateSessionResponseSchema: GenMessage<CreateSessionResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  8,
);

export type CloseSessionRequest = Message<"memoh.browser.v1.CloseSessionRequest"> & {
  id: string;
  token: string;
};
export const CloseSessionRequestSchema: GenMessage<CloseSessionRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  9,
);

export type CloseSessionResponse = Message<"memoh.browser.v1.CloseSessionResponse"> & {
  success: boolean;
};
export const CloseSessionResponseSchema: GenMessage<CloseSessionResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  10,
);

export type NavigateRequest = Message<"memoh.browser.v1.NavigateRequest"> & {
  contextId: string;
  url: string;
  timeout?: number;
};
export const NavigateRequestSchema: GenMessage<NavigateRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  11,
);

export type ActionRequest = Message<"memoh.browser.v1.ActionRequest"> & {
  contextId: string;
  action: string;
  url?: string;
  selector?: string;
  text?: string;
  script?: string;
  key?: string;
  value?: string;
  targetSelector?: string;
  files: string[];
  fullPage?: boolean;
  tabIndex?: number;
  direction?: string;
  amount?: number;
  timeout?: number;
};
export const ActionRequestSchema: GenMessage<ActionRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  12,
);

export type ActionResponse = Message<"memoh.browser.v1.ActionResponse"> & {
  data?: JsonObject;
};
export const ActionResponseSchema: GenMessage<ActionResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  13,
);

export type ScreenshotRequest = Message<"memoh.browser.v1.ScreenshotRequest"> & {
  contextId: string;
  fullPage?: boolean;
};
export const ScreenshotRequestSchema: GenMessage<ScreenshotRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  14,
);

export type ScreenshotResponse = Message<"memoh.browser.v1.ScreenshotResponse"> & {
  data: Uint8Array;
  mimeType: string;
};
export const ScreenshotResponseSchema: GenMessage<ScreenshotResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  15,
);

export type DevicesRequest = Message<"memoh.browser.v1.DevicesRequest"> & {};
export const DevicesRequestSchema: GenMessage<DevicesRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  16,
);

export type BrowserDevice = Message<"memoh.browser.v1.BrowserDevice"> & {
  id: string;
  descriptor?: JsonObject;
};
export const BrowserDeviceSchema: GenMessage<BrowserDevice> = messageDesc(
  file_memoh_browser_v1_browser,
  17,
);

export type DevicesResponse = Message<"memoh.browser.v1.DevicesResponse"> & {
  devices: BrowserDevice[];
};
export const DevicesResponseSchema: GenMessage<DevicesResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  18,
);

export type CoresRequest = Message<"memoh.browser.v1.CoresRequest"> & {};
export const CoresRequestSchema: GenMessage<CoresRequest> = messageDesc(
  file_memoh_browser_v1_browser,
  19,
);

export type CoresResponse = Message<"memoh.browser.v1.CoresResponse"> & {
  cores: string[];
};
export const CoresResponseSchema: GenMessage<CoresResponse> = messageDesc(
  file_memoh_browser_v1_browser,
  20,
);

export const BrowserService: GenService<{
  createContext: {
    methodKind: "unary";
    input: typeof CreateContextRequestSchema;
    output: typeof CreateContextResponseSchema;
  };
  listContexts: {
    methodKind: "unary";
    input: typeof ListContextsRequestSchema;
    output: typeof ListContextsResponseSchema;
  };
  closeContext: {
    methodKind: "unary";
    input: typeof CloseContextRequestSchema;
    output: typeof CloseContextResponseSchema;
  };
  createSession: {
    methodKind: "unary";
    input: typeof CreateSessionRequestSchema;
    output: typeof CreateSessionResponseSchema;
  };
  closeSession: {
    methodKind: "unary";
    input: typeof CloseSessionRequestSchema;
    output: typeof CloseSessionResponseSchema;
  };
  navigate: {
    methodKind: "unary";
    input: typeof NavigateRequestSchema;
    output: typeof ActionResponseSchema;
  };
  action: {
    methodKind: "unary";
    input: typeof ActionRequestSchema;
    output: typeof ActionResponseSchema;
  };
  screenshot: {
    methodKind: "unary";
    input: typeof ScreenshotRequestSchema;
    output: typeof ScreenshotResponseSchema;
  };
  devices: {
    methodKind: "unary";
    input: typeof DevicesRequestSchema;
    output: typeof DevicesResponseSchema;
  };
  cores: {
    methodKind: "unary";
    input: typeof CoresRequestSchema;
    output: typeof CoresResponseSchema;
  };
}> = serviceDesc(file_memoh_browser_v1_browser, 0);

export interface BrowserServiceOperations {
  createContext(input: {
    id?: string;
    name?: string;
    botId?: string;
    config?: JsonObject;
  }): Promise<BrowserContextSummary>;
  listContexts(): BrowserContextSummary[];
  closeContext(id: string): Promise<boolean>;
  createSession(input: {
    botId: string;
    core?: BrowserCore;
    ttlMs?: number;
    contextConfig?: Record<string, unknown>;
  }): Promise<CreateRemoteSessionResult>;
  closeSession(id: string, token: string): boolean;
  action(contextId: string, request: ModuleActionRequest): Promise<Record<string, unknown>>;
  screenshot(
    contextId: string,
    fullPage?: boolean,
  ): Promise<{ data: Uint8Array; mimeType: string }>;
  devices(): Record<string, unknown>;
  cores(): string[];
}

export const defaultBrowserServiceOperations: BrowserServiceOperations = {
  createContext: createBrowserContext,
  listContexts: listBrowserContexts,
  closeContext: closeBrowserContext,
  createSession: createRemoteSession,
  closeSession: closeRemoteSession,
  action: executeAction,
  screenshot: captureScreenshot,
  devices: listDevices,
  cores: listCores,
};

export function createBrowserServiceHandlers(
  operations: BrowserServiceOperations = defaultBrowserServiceOperations,
): Partial<ServiceImpl<typeof BrowserService>> {
  return {
    async createContext(request) {
      return withConnectErrors(async () => {
        const context = await operations.createContext({
          id: request.id,
          name: request.name,
          botId: request.botId,
          config: request.config,
        });
        return { context: toProtoContext(context) };
      });
    },
    listContexts() {
      return { contexts: operations.listContexts().map(toProtoContext) };
    },
    async closeContext(request) {
      return withConnectErrors(async () => {
        return { success: await operations.closeContext(request.id) };
      });
    },
    async createSession(request) {
      return withConnectErrors(async () => {
        const session = await operations.createSession({
          botId: request.botId,
          core: toBrowserCore(request.core),
          ttlMs: request.ttlMs,
          contextConfig: request.contextConfig,
        });
        return toCreateSessionResponse(session);
      });
    },
    closeSession(request) {
      return withConnectErrors(() => {
        return { success: operations.closeSession(request.id, request.token) };
      });
    },
    async navigate(request) {
      return withConnectErrors(async () => {
        const result = await operations.action(request.contextId, {
          action: "navigate",
          url: request.url,
          timeout: request.timeout,
        });
        return { data: toJsonObject(result) };
      });
    },
    async action(request) {
      return withConnectErrors(async () => {
        const result = await operations.action(request.contextId, toModuleActionRequest(request));
        return { data: toJsonObject(result) };
      });
    },
    async screenshot(request) {
      return withConnectErrors(async () => {
        const screenshot = await operations.screenshot(request.contextId, request.fullPage);
        return { data: screenshot.data, mimeType: screenshot.mimeType };
      });
    },
    devices() {
      return {
        devices: Object.entries(operations.devices()).map(([id, descriptor]) => ({
          id,
          descriptor: toJsonObject(descriptor),
        })),
      };
    },
    cores() {
      return { cores: operations.cores() };
    },
  };
}

export const browserServiceHandlers = createBrowserServiceHandlers();

export function createBrowserConnectFetchHandler(
  implementation: Partial<ServiceImpl<typeof BrowserService>> = browserServiceHandlers,
): (request: Request) => Promise<Response> {
  const router = createConnectRouter({
    connect: true,
    grpc: false,
    grpcWeb: false,
    readMaxBytes: 32 * 1024 * 1024,
    writeMaxBytes: 32 * 1024 * 1024,
  });
  router.service(BrowserService, implementation);

  const handlers = new Map(
    router.handlers.map((handler) => [
      handler.requestPath,
      createFetchHandler(handler, { httpVersion: "1.1" }),
    ]),
  );

  return async (request) => {
    const path = new URL(request.url).pathname;
    const handler = handlers.get(path);
    if (!handler) return new Response("not found", { status: 404 });
    return handler(request);
  };
}

function toProtoContext(context: BrowserContextSummary): BrowserContext {
  return {
    $typeName: "memoh.browser.v1.BrowserContext",
    id: context.id,
    name: context.name,
    core: context.core,
    config: toJsonObject(context.config),
    botId: context.botId,
  };
}

function toCreateSessionResponse(session: CreateRemoteSessionResult): CreateSessionResponse {
  return {
    $typeName: "memoh.browser.v1.CreateSessionResponse",
    id: session.id,
    wsEndpoint: session.wsEndpoint,
    sessionToken: session.sessionToken,
    playwrightVersion: session.playwrightVersion,
    core: session.core,
    contextConfig: toJsonObject(session.contextConfig),
    expiresAt: session.expiresAt,
  };
}

function toBrowserCore(core: string | undefined): BrowserCore | undefined {
  if (core === undefined || core === "") return undefined;
  if (core === "chromium" || core === "firefox") return core;
  throw new ConnectError(`unsupported browser core: ${core}`, Code.InvalidArgument);
}

function toModuleActionRequest(request: ActionRequest): ModuleActionRequest {
  const parsed = ActionRequestModel.safeParse({
    action: request.action,
    url: request.url,
    selector: request.selector,
    text: request.text,
    script: request.script,
    key: request.key,
    value: request.value,
    target_selector: request.targetSelector,
    files: request.files,
    full_page: request.fullPage,
    tab_index: request.tabIndex,
    direction: request.direction,
    amount: request.amount,
    timeout: request.timeout,
  });
  if (!parsed.success) {
    throw new ConnectError(parsed.error.message, Code.InvalidArgument);
  }
  return parsed.data;
}

async function withConnectErrors<T>(fn: () => T | Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (error) {
    throw normalizeConnectError(error);
  }
}

function normalizeConnectError(error: unknown): ConnectError {
  if (error instanceof ConnectError) return error;
  const message = error instanceof Error ? error.message : String(error);
  if (message.includes("already exists")) {
    return new ConnectError(message, Code.AlreadyExists, undefined, undefined, error);
  }
  if (message.includes("not found") || message.includes("not reachable")) {
    return new ConnectError(message, Code.NotFound, undefined, undefined, error);
  }
  if (message.includes("required") || message.includes("unsupported")) {
    return new ConnectError(message, Code.InvalidArgument, undefined, undefined, error);
  }
  return new ConnectError(message, Code.Unknown, undefined, undefined, error);
}

function toJsonObject(value: unknown): JsonObject {
  const converted = toJsonValue(value);
  if (converted && typeof converted === "object" && !Array.isArray(converted)) {
    return converted as JsonObject;
  }
  return {};
}

function toJsonValue(value: unknown): JsonValue | undefined {
  if (value === undefined || typeof value === "function" || typeof value === "symbol") {
    return undefined;
  }
  if (value === null || typeof value === "string" || typeof value === "number") {
    return value;
  }
  if (typeof value === "boolean") return value;
  if (value instanceof Uint8Array) return Buffer.from(value).toString("base64");
  if (Array.isArray(value)) {
    return value
      .map((item) => toJsonValue(item))
      .filter((item): item is JsonValue => item !== undefined);
  }
  if (typeof value === "object") {
    const result: JsonObject = {};
    for (const [key, item] of Object.entries(value)) {
      const converted = toJsonValue(item);
      if (converted !== undefined) result[key] = converted;
    }
    return result;
  }
  return String(value);
}
