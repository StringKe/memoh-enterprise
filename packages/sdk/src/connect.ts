import { create, type MessageShape } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import {
  Code,
  ConnectError,
  createClient,
  type Client,
  type Interceptor,
  type Transport,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

import { BrowserService } from "./gen/memoh/browser/v1/browser_pb";
import { ConnectorService } from "./gen/memoh/connector/v1/connector_pb";
import { ConnectorEventService } from "./gen/memoh/connector/v1/events_pb";
import { AclService } from "./gen/memoh/private/v1/acl_pb";
import { AuthService } from "./gen/memoh/private/v1/auth_pb";
import { BotGroupService } from "./gen/memoh/private/v1/bot_groups_pb";
import { BotService } from "./gen/memoh/private/v1/bots_pb";
import { BrowserContextService } from "./gen/memoh/private/v1/browser_contexts_pb";
import { ChannelService } from "./gen/memoh/private/v1/channels_pb";
import {
  EmailBindingService,
  EmailOutboxService,
  EmailProviderService,
} from "./gen/memoh/private/v1/email_pb";
import { IamService } from "./gen/memoh/private/v1/iam_pb";
import { HealthService } from "./gen/memoh/private/v1/health_pb";
import { IntegrationAdminService } from "./gen/memoh/private/v1/integrations_pb";
import { InternalAuthService } from "./gen/memoh/private/v1/internal_auth_pb";
import { McpService } from "./gen/memoh/private/v1/mcp_pb";
import { MemoryProviderService, MemoryService } from "./gen/memoh/private/v1/memory_pb";
import { ModelService } from "./gen/memoh/private/v1/models_pb";
import { NetworkService } from "./gen/memoh/private/v1/network_pb";
import { ProviderService } from "./gen/memoh/private/v1/providers_pb";
import { ScheduleService } from "./gen/memoh/private/v1/schedule_pb";
import { SearchProviderService } from "./gen/memoh/private/v1/search_providers_pb";
import { SettingsService } from "./gen/memoh/private/v1/settings_pb";
import { SkillService } from "./gen/memoh/private/v1/skills_pb";
import { SpeechService } from "./gen/memoh/private/v1/speech_pb";
import { ChatService, ContainerService } from "./gen/memoh/private/v1/streaming_pb";
import { StructuredDataService } from "./gen/memoh/private/v1/structured_data_pb";
import { SupermarketService } from "./gen/memoh/private/v1/supermarket_pb";
import { ToolApprovalService } from "./gen/memoh/private/v1/tool_approval_pb";
import { UsageService } from "./gen/memoh/private/v1/usage_pb";
import { UserService } from "./gen/memoh/private/v1/users_pb";
import { RunnerService } from "./gen/memoh/runner/v1/run_pb";
import { RunnerSupportService } from "./gen/memoh/runner/v1/support_pb";
import { WorkspaceExecutorService } from "./gen/memoh/workspace/v1/executor_pb";

export type MemohConnectClients = {
  acl: Client<typeof AclService>;
  auth: Client<typeof AuthService>;
  browser: Client<typeof BrowserService>;
  bots: Client<typeof BotService>;
  botGroups: Client<typeof BotGroupService>;
  browserContexts: Client<typeof BrowserContextService>;
  channels: Client<typeof ChannelService>;
  chat: Client<typeof ChatService>;
  connectorEvents: Client<typeof ConnectorEventService>;
  connectors: Client<typeof ConnectorService>;
  containers: Client<typeof ContainerService>;
  emailBindings: Client<typeof EmailBindingService>;
  emailOutbox: Client<typeof EmailOutboxService>;
  emailProviders: Client<typeof EmailProviderService>;
  health: Client<typeof HealthService>;
  iam: Client<typeof IamService>;
  integrationAdmin: Client<typeof IntegrationAdminService>;
  internalAuth: Client<typeof InternalAuthService>;
  mcp: Client<typeof McpService>;
  memory: Client<typeof MemoryService>;
  memoryProviders: Client<typeof MemoryProviderService>;
  models: Client<typeof ModelService>;
  network: Client<typeof NetworkService>;
  providers: Client<typeof ProviderService>;
  runner: Client<typeof RunnerService>;
  runnerSupport: Client<typeof RunnerSupportService>;
  schedule: Client<typeof ScheduleService>;
  searchProviders: Client<typeof SearchProviderService>;
  settings: Client<typeof SettingsService>;
  skills: Client<typeof SkillService>;
  speech: Client<typeof SpeechService>;
  structuredData: Client<typeof StructuredDataService>;
  supermarket: Client<typeof SupermarketService>;
  toolApproval: Client<typeof ToolApprovalService>;
  usage: Client<typeof UsageService>;
  users: Client<typeof UserService>;
  workspaceExecutor: Client<typeof WorkspaceExecutorService>;
};

export type CreateMemohConnectClientOptions = {
  baseUrl: string;
  transport?: Transport;
  fetch?: typeof globalThis.fetch;
  useBinaryFormat?: boolean;
  defaultTimeoutMs?: number;
  getAuthToken?: () => string | null | undefined;
  requestIdFactory?: () => string;
  interceptors?: Interceptor[];
  logger?: MemohConnectLogger;
};

export type TimestampMessage = MessageShape<typeof TimestampSchema>;

export type MemohConnectLogger = (event: {
  phase: "request" | "response" | "error";
  url: string;
  service: string;
  method: string;
  error?: unknown;
}) => void;

export type MemohConnectInterceptorsOptions = Pick<
  CreateMemohConnectClientOptions,
  "getAuthToken" | "requestIdFactory" | "logger" | "interceptors"
>;

export type MemohConnectError = {
  code: Code;
  message: string;
  metadata: Headers;
  cause: unknown;
};

export type ConnectQueryService = {
  typeName: string;
};

export function connectQueryKey(
  service: ConnectQueryService | string,
  method: string,
  input: unknown = null,
): readonly unknown[] {
  const serviceName = typeof service === "string" ? service : service.typeName;
  return ["connect", serviceName, method, input];
}

export function createMemohAuthInterceptor(
  getAuthToken: () => string | null | undefined,
): Interceptor {
  return (next) => async (request) => {
    const token = getAuthToken();
    if (token) {
      request.header.set("authorization", `Bearer ${token}`);
    }
    return next(request);
  };
}

export function createMemohRequestIdInterceptor(requestIdFactory: () => string): Interceptor {
  return (next) => async (request) => {
    request.header.set("x-request-id", requestIdFactory());
    return next(request);
  };
}

export function createMemohErrorInterceptor(): Interceptor {
  return (next) => async (request) => {
    try {
      return await next(request);
    } catch (error) {
      throw normalizeConnectError(error);
    }
  };
}

export function createMemohLoggingInterceptor(logger: MemohConnectLogger): Interceptor {
  return (next) => async (request) => {
    const event = {
      url: request.url,
      service: request.service.typeName,
      method: request.method.name,
    };
    logger({ phase: "request", ...event });

    try {
      const response = await next(request);
      logger({ phase: "response", ...event });
      return response;
    } catch (error) {
      logger({ phase: "error", error, ...event });
      throw error;
    }
  };
}

export function createMemohInterceptors({
  getAuthToken,
  requestIdFactory,
  logger,
  interceptors = [],
}: MemohConnectInterceptorsOptions): Interceptor[] {
  const result: Interceptor[] = [];

  if (getAuthToken) {
    result.push(createMemohAuthInterceptor(getAuthToken));
  }
  if (requestIdFactory) {
    result.push(createMemohRequestIdInterceptor(requestIdFactory));
  }
  if (logger) {
    result.push(createMemohLoggingInterceptor(logger));
  }

  result.push(createMemohErrorInterceptor(), ...interceptors);
  return result;
}

export function createMemohConnectTransport(options: CreateMemohConnectClientOptions): Transport {
  return (
    options.transport ??
    createConnectTransport({
      baseUrl: options.baseUrl,
      defaultTimeoutMs: options.defaultTimeoutMs,
      fetch: (input, init) => (options.fetch ?? fetch)(input, { ...init, credentials: "include" }),
      interceptors: createMemohInterceptors(options),
      useBinaryFormat: options.useBinaryFormat,
    })
  );
}

export function createMemohConnectClients(
  options: CreateMemohConnectClientOptions,
): MemohConnectClients {
  const transport = createMemohConnectTransport(options);

  return {
    acl: createClient(AclService, transport),
    auth: createClient(AuthService, transport),
    browser: createClient(BrowserService, transport),
    bots: createClient(BotService, transport),
    botGroups: createClient(BotGroupService, transport),
    browserContexts: createClient(BrowserContextService, transport),
    channels: createClient(ChannelService, transport),
    chat: createClient(ChatService, transport),
    connectorEvents: createClient(ConnectorEventService, transport),
    connectors: createClient(ConnectorService, transport),
    containers: createClient(ContainerService, transport),
    emailBindings: createClient(EmailBindingService, transport),
    emailOutbox: createClient(EmailOutboxService, transport),
    emailProviders: createClient(EmailProviderService, transport),
    health: createClient(HealthService, transport),
    iam: createClient(IamService, transport),
    integrationAdmin: createClient(IntegrationAdminService, transport),
    internalAuth: createClient(InternalAuthService, transport),
    mcp: createClient(McpService, transport),
    memory: createClient(MemoryService, transport),
    memoryProviders: createClient(MemoryProviderService, transport),
    models: createClient(ModelService, transport),
    network: createClient(NetworkService, transport),
    providers: createClient(ProviderService, transport),
    runner: createClient(RunnerService, transport),
    runnerSupport: createClient(RunnerSupportService, transport),
    schedule: createClient(ScheduleService, transport),
    searchProviders: createClient(SearchProviderService, transport),
    settings: createClient(SettingsService, transport),
    skills: createClient(SkillService, transport),
    speech: createClient(SpeechService, transport),
    structuredData: createClient(StructuredDataService, transport),
    supermarket: createClient(SupermarketService, transport),
    toolApproval: createClient(ToolApprovalService, transport),
    usage: createClient(UsageService, transport),
    users: createClient(UserService, transport),
    workspaceExecutor: createClient(WorkspaceExecutorService, transport),
  };
}

export function createTimestampFromDate(date: Date): TimestampMessage {
  const milliseconds = date.getTime();
  const seconds = Math.floor(milliseconds / 1000);
  return create(TimestampSchema, {
    seconds: BigInt(seconds),
    nanos: (milliseconds - seconds * 1000) * 1_000_000,
  });
}

export function isConnectError(error: unknown): error is ConnectError {
  return error instanceof ConnectError;
}

export function normalizeConnectError(error: unknown): MemohConnectError {
  if (error instanceof ConnectError) {
    return {
      code: error.code,
      message: error.rawMessage,
      metadata: error.metadata,
      cause: error.cause,
    };
  }

  if (error instanceof Error) {
    return {
      code: Code.Unknown,
      message: error.message,
      metadata: new Headers(),
      cause: error,
    };
  }

  return {
    code: Code.Unknown,
    message: String(error),
    metadata: new Headers(),
    cause: error,
  };
}

export * from "./gen/memoh/private/v1/acl_pb";
export * from "./gen/memoh/private/v1/auth_pb";
export * from "./gen/memoh/private/v1/bot_groups_pb";
export * from "./gen/memoh/private/v1/bots_pb";
export * from "./gen/memoh/private/v1/browser_contexts_pb";
export * from "./gen/memoh/private/v1/channels_pb";
export * from "./gen/memoh/private/v1/common_pb";
export * from "./gen/memoh/private/v1/email_pb";
export * from "./gen/memoh/private/v1/health_pb";
export * from "./gen/memoh/private/v1/iam_pb";
export * from "./gen/memoh/private/v1/integrations_pb";
export * from "./gen/memoh/private/v1/mcp_pb";
export * from "./gen/memoh/private/v1/memory_pb";
export * from "./gen/memoh/private/v1/models_pb";
export * from "./gen/memoh/private/v1/network_pb";
export * from "./gen/memoh/private/v1/providers_pb";
export * from "./gen/memoh/private/v1/schedule_pb";
export * from "./gen/memoh/private/v1/search_providers_pb";
export * from "./gen/memoh/private/v1/settings_pb";
export * from "./gen/memoh/private/v1/skills_pb";
export * from "./gen/memoh/private/v1/speech_pb";
export * from "./gen/memoh/private/v1/streaming_pb";
export * from "./gen/memoh/private/v1/structured_data_pb";
export * from "./gen/memoh/private/v1/supermarket_pb";
export * from "./gen/memoh/private/v1/tool_approval_pb";
export * from "./gen/memoh/private/v1/usage_pb";
export * from "./gen/memoh/private/v1/users_pb";
export * from "./gen/memoh/private/v1/internal_auth_pb";
export * from "./gen/memoh/browser/v1/browser_pb";
export * from "./gen/memoh/connector/v1/connector_pb";
export * from "./gen/memoh/connector/v1/events_pb";
export * from "./gen/memoh/event/v1/events_pb";
export * from "./gen/memoh/runner/v1/run_pb";
export * from "./gen/memoh/runner/v1/support_pb";
export * from "./gen/memoh/workspace/v1/executor_pb";
