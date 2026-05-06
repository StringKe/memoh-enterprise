import { create, fromJsonString, toJsonString } from "@bufbuild/protobuf";

import {
  AckRequestSchema,
  type AckResponse,
  AuthRequestSchema,
  type AuthResponse,
  CreateSessionRequestSchema,
  type CreateSessionResponse,
  EnvelopeSchema,
  type Envelope,
  GetSessionStatusRequestSchema,
  type GetSessionStatusResponse,
  GetBotStatusRequestSchema,
  type GetBotStatusResponse,
  PingSchema,
  type Pong,
  RequestActionRequestSchema,
  type RequestActionResponse,
  SendBotMessageRequestSchema,
  type SendBotMessageResponse,
  SubscribeRequestSchema,
  type SubscribeResponse,
} from "@stringke/sdk/gen/memoh/integration/v1/envelope_pb";

export type WebSocketLike = {
  readyState: number;
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent<string>) => void) | null;
  onerror: ((event: Event) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  send(data: string): void;
  close(code?: number, reason?: string): void;
};

export type WebSocketFactoryInit = {
  protocols?: string | string[];
  headers: Record<string, string>;
};

export type MemohIntegrationClientOptions = {
  url: string;
  token: string;
  clientId?: string;
  clientSecret?: string;
  headers?: Record<string, string>;
  protocols?: string | string[];
  protocolVersion?: string;
  requestTimeoutMs?: number;
  heartbeatIntervalMs?: number;
  reconnectBackoffMs?: number;
  maxReconnectAttempts?: number;
  webSocketFactory?: (url: string, init: WebSocketFactoryInit) => WebSocketLike;
  idFactory?: () => string;
};

export type SubscribeOptions = {
  eventTypes?: string[];
  botIds?: string[];
  botGroupIds?: string[];
};

export type SendBotMessageOptions = {
  botId: string;
  sessionId?: string;
  text: string;
  metadata?: Record<string, string>;
};

export type CreateSessionOptions = {
  botId: string;
  externalSessionId?: string;
  metadata?: Record<string, string>;
};

export type RequestActionOptions = {
  botId: string;
  actionType: string;
  payloadJson?: string;
  metadata?: Record<string, string>;
};

export type ConnectionInfo = {
  integrationId: string;
  scopeType: string;
  scopeBotId: string;
  scopeBotGroupId: string;
};

type PendingRequest<T> = {
  expectedCase: Envelope["payload"]["case"];
  resolve: (value: T) => void;
  reject: (reason: unknown) => void;
  timer: ReturnType<typeof setTimeout>;
};

type RequestPayload = Extract<
  Envelope["payload"],
  {
    case:
      | "subscribeRequest"
      | "ackRequest"
      | "ping"
      | "sendBotMessageRequest"
      | "createSessionRequest"
      | "getSessionStatusRequest"
      | "getBotStatusRequest"
      | "requestActionRequest";
  }
>;

type EventWaiter = {
  resolve: (value: IteratorResult<Envelope>) => void;
  reject: (reason: unknown) => void;
};

export class MemohIntegrationError extends Error {
  constructor(
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "MemohIntegrationError";
  }
}

export class MemohIntegrationClient {
  private readonly protocolVersion: string;
  private readonly requestTimeoutMs: number;
  private readonly heartbeatIntervalMs: number;
  private readonly reconnectBackoffMs: number;
  private readonly maxReconnectAttempts: number;
  private readonly webSocketFactory: (url: string, init: WebSocketFactoryInit) => WebSocketLike;
  private readonly idFactory: () => string;
  private socket: WebSocketLike | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private connectionInfo: ConnectionInfo | null = null;
  private authWaiter: PendingRequest<AuthResponse> | null = null;
  private pending = new Map<string, PendingRequest<unknown>>();
  private eventQueue: Envelope[] = [];
  private seenEventIds = new Set<string>();
  private eventWaiter: EventWaiter | null = null;
  private closed = false;

  constructor(private readonly options: MemohIntegrationClientOptions) {
    this.protocolVersion = options.protocolVersion ?? "2026-05-05";
    this.requestTimeoutMs = options.requestTimeoutMs ?? 10_000;
    this.heartbeatIntervalMs = options.heartbeatIntervalMs ?? 0;
    this.reconnectBackoffMs = options.reconnectBackoffMs ?? 500;
    this.maxReconnectAttempts = options.maxReconnectAttempts ?? 0;
    this.webSocketFactory =
      options.webSocketFactory ??
      ((url, init) => {
        if (typeof WebSocket === "undefined") {
          throw new Error("WebSocket is not available in this runtime");
        }
        return init.protocols ? new WebSocket(url, init.protocols) : new WebSocket(url);
      });
    this.idFactory = options.idFactory ?? defaultIdFactory;
  }

  get info(): ConnectionInfo | null {
    return this.connectionInfo;
  }

  async connect(): Promise<ConnectionInfo> {
    this.closed = false;
    this.stopHeartbeat();
    const socket = this.webSocketFactory(this.options.url, this.webSocketInit());
    this.socket = socket;

    await new Promise<void>((resolve, reject) => {
      socket.onopen = () => resolve();
      socket.onerror = () => reject(new Error("integration websocket open failed"));
    });

    socket.onmessage = (event) => this.handleMessage(event.data);
    socket.onerror = () => this.rejectAll(new Error("integration websocket error"));
    socket.onclose = () => this.handleClose();

    const info = await this.authenticate();
    this.connectionInfo = {
      integrationId: info.integrationId,
      scopeType: info.scopeType,
      scopeBotId: info.scopeBotId,
      scopeBotGroupId: info.scopeBotGroupId,
    };
    this.startHeartbeat();
    return this.connectionInfo;
  }

  async reconnect(): Promise<ConnectionInfo> {
    let attempt = 0;
    let lastError: unknown;
    while (attempt <= this.maxReconnectAttempts) {
      try {
        return await this.connect();
      } catch (error) {
        lastError = error;
        attempt += 1;
        if (attempt > this.maxReconnectAttempts) {
          break;
        }
        await sleep(this.reconnectBackoffMs * attempt);
      }
    }
    throw lastError;
  }

  async subscribe(options: SubscribeOptions = {}): Promise<SubscribeResponse> {
    return await this.request<SubscribeResponse>("subscribeResponse", {
      case: "subscribeRequest",
      value: create(SubscribeRequestSchema, {
        eventTypes: options.eventTypes ?? [],
        botIds: options.botIds ?? [],
        botGroupIds: options.botGroupIds ?? [],
      }),
    });
  }

  async ackEvent(eventId: string): Promise<string> {
    const response = await this.request<AckResponse>("ackResponse", {
      case: "ackRequest",
      value: create(AckRequestSchema, { eventId }),
    });
    return response.eventId;
  }

  async sendBotMessage(options: SendBotMessageOptions): Promise<SendBotMessageResponse> {
    return await this.request<SendBotMessageResponse>("sendBotMessageResponse", {
      case: "sendBotMessageRequest",
      value: create(SendBotMessageRequestSchema, {
        botId: options.botId,
        sessionId: options.sessionId ?? "",
        text: options.text,
        metadata: options.metadata ?? {},
      }),
    });
  }

  async createSession(options: CreateSessionOptions): Promise<CreateSessionResponse> {
    return await this.request<CreateSessionResponse>("createSessionResponse", {
      case: "createSessionRequest",
      value: create(CreateSessionRequestSchema, {
        botId: options.botId,
        externalSessionId: options.externalSessionId ?? "",
        metadata: options.metadata ?? {},
      }),
    });
  }

  async getSessionStatus(sessionId: string): Promise<GetSessionStatusResponse> {
    return await this.request<GetSessionStatusResponse>("getSessionStatusResponse", {
      case: "getSessionStatusRequest",
      value: create(GetSessionStatusRequestSchema, { sessionId }),
    });
  }

  async getBotStatus(botId: string): Promise<GetBotStatusResponse> {
    return await this.request<GetBotStatusResponse>("getBotStatusResponse", {
      case: "getBotStatusRequest",
      value: create(GetBotStatusRequestSchema, { botId }),
    });
  }

  async requestAction(options: RequestActionOptions): Promise<RequestActionResponse> {
    return await this.request<RequestActionResponse>("requestActionResponse", {
      case: "requestActionRequest",
      value: create(RequestActionRequestSchema, {
        botId: options.botId,
        actionType: options.actionType,
        payloadJson: options.payloadJson ?? "",
        metadata: options.metadata ?? {},
      }),
    });
  }

  async ping(): Promise<void> {
    await this.request<Pong>("pong", {
      case: "ping",
      value: create(PingSchema),
    });
  }

  async *events(): AsyncIterable<Envelope> {
    while (!this.closed) {
      const next = await this.nextEvent();
      if (next.done) {
        return;
      }
      yield next.value;
    }
  }

  close(code = 1000, reason = "client closed"): void {
    this.closed = true;
    this.stopHeartbeat();
    this.socket?.close(code, reason);
    this.socket = null;
    this.rejectAll(new Error("integration websocket closed"));
    this.resolveEventDone();
  }

  private async authenticate(): Promise<AuthResponse> {
    const message = create(EnvelopeSchema, {
      version: this.protocolVersion,
      messageId: this.idFactory(),
      payload: {
        case: "authRequest",
        value: create(AuthRequestSchema, { token: this.options.token }),
      },
    });
    const response = new Promise<AuthResponse>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.authWaiter = null;
        reject(new Error("integration auth timed out"));
      }, this.requestTimeoutMs);
      this.authWaiter = {
        expectedCase: "authResponse",
        resolve,
        reject,
        timer,
      };
    });
    this.send(message);
    return await response;
  }

  private async request<T>(
    expectedCase: Envelope["payload"]["case"],
    payload: RequestPayload,
  ): Promise<T> {
    const id = this.idFactory();
    const envelope = create(EnvelopeSchema, {
      version: this.protocolVersion,
      messageId: id,
      correlationId: id,
      payload,
    });
    const response = new Promise<unknown>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`integration request timed out: ${String(expectedCase)}`));
      }, this.requestTimeoutMs);
      this.pending.set(id, {
        expectedCase,
        resolve,
        reject,
        timer,
      });
    });
    this.send(envelope);
    return (await response) as T;
  }

  private handleMessage(data: string): void {
    const envelope = fromJsonString(EnvelopeSchema, data, { ignoreUnknownFields: true });
    const payload = envelope.payload;
    if (payload.case === "authResponse" && this.authWaiter) {
      this.resolvePending(this.authWaiter, payload.value);
      this.authWaiter = null;
      return;
    }
    if (payload.case === "error") {
      const error = new MemohIntegrationError(payload.value.code, payload.value.message);
      if (envelope.correlationId) {
        const pending = this.pending.get(envelope.correlationId);
        if (pending) {
          this.pending.delete(envelope.correlationId);
          this.rejectPending(pending, error);
          return;
        }
      }
      if (this.authWaiter) {
        this.rejectPending(this.authWaiter, error);
        this.authWaiter = null;
        return;
      }
      this.enqueueEvent(envelope);
      return;
    }
    if (envelope.correlationId) {
      const pending = this.pending.get(envelope.correlationId);
      if (pending) {
        this.pending.delete(envelope.correlationId);
        if (payload.case === pending.expectedCase) {
          this.resolvePending(pending, payload.value);
        } else {
          this.rejectPending(
            pending,
            new Error(`unexpected integration response: ${String(payload.case)}`),
          );
        }
        return;
      }
    }
    this.enqueueEvent(envelope);
  }

  private handleClose(): void {
    this.stopHeartbeat();
    if (this.closed) {
      this.resolveEventDone();
      return;
    }
    this.rejectAll(new Error("integration websocket closed"));
    this.resolveEventDone();
  }

  private send(envelope: Envelope): void {
    if (!this.socket || this.socket.readyState !== 1) {
      throw new Error("integration websocket is not open");
    }
    this.socket.send(toJsonString(EnvelopeSchema, envelope));
  }

  private nextEvent(): Promise<IteratorResult<Envelope>> {
    if (this.eventQueue.length > 0) {
      return Promise.resolve({ done: false, value: this.eventQueue.shift()! });
    }
    if (this.closed) {
      return Promise.resolve({ done: true, value: undefined });
    }
    return new Promise((resolve, reject) => {
      this.eventWaiter = { resolve, reject };
    });
  }

  private enqueueEvent(envelope: Envelope): void {
    if (envelope.messageId) {
      if (this.seenEventIds.has(envelope.messageId)) {
        return;
      }
      this.seenEventIds.add(envelope.messageId);
    }
    if (this.eventWaiter) {
      const waiter = this.eventWaiter;
      this.eventWaiter = null;
      waiter.resolve({ done: false, value: envelope });
      return;
    }
    this.eventQueue.push(envelope);
  }

  private resolveEventDone(): void {
    if (!this.eventWaiter) {
      return;
    }
    const waiter = this.eventWaiter;
    this.eventWaiter = null;
    waiter.resolve({ done: true, value: undefined });
  }

  private resolvePending<T>(pending: PendingRequest<T>, value: T): void {
    clearTimeout(pending.timer);
    pending.resolve(value);
  }

  private rejectPending<T>(pending: PendingRequest<T>, reason: unknown): void {
    clearTimeout(pending.timer);
    pending.reject(reason);
  }

  private rejectAll(reason: unknown): void {
    if (this.authWaiter) {
      this.rejectPending(this.authWaiter, reason);
      this.authWaiter = null;
    }
    for (const pending of this.pending.values()) {
      this.rejectPending(pending, reason);
    }
    this.pending.clear();
    if (this.eventWaiter) {
      const waiter = this.eventWaiter;
      this.eventWaiter = null;
      waiter.reject(reason);
    }
  }

  private webSocketInit(): WebSocketFactoryInit {
    const headers: Record<string, string> = { ...this.options.headers };
    if (!hasHeader(headers, "authorization")) {
      headers.authorization = `Bearer ${this.options.token}`;
    }
    if (this.options.clientId) {
      headers["x-memoh-client-id"] = this.options.clientId;
    }
    if (this.options.clientSecret) {
      headers["x-memoh-client-secret"] = this.options.clientSecret;
    }
    return {
      protocols: this.options.protocols,
      headers,
    };
  }

  private startHeartbeat(): void {
    if (this.heartbeatIntervalMs <= 0 || this.heartbeatTimer) {
      return;
    }
    this.heartbeatTimer = setInterval(() => {
      void this.ping().catch(() => {
        this.close(4000, "heartbeat failed");
      });
    }, this.heartbeatIntervalMs);
  }

  private stopHeartbeat(): void {
    if (!this.heartbeatTimer) {
      return;
    }
    clearInterval(this.heartbeatTimer);
    this.heartbeatTimer = null;
  }
}

function defaultIdFactory(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

async function sleep(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

function hasHeader(headers: Record<string, string>, name: string): boolean {
  const normalized = name.toLowerCase();
  return Object.keys(headers).some((key) => key.toLowerCase() === normalized);
}

export type {
  AckRequest,
  AckResponse,
  AuthRequest,
  AuthResponse,
  CreateSessionRequest,
  CreateSessionResponse,
  Envelope,
  Error as IntegrationError,
  GetSessionStatusRequest,
  GetSessionStatusResponse,
  GetBotStatusRequest,
  GetBotStatusResponse,
  Ping,
  Pong,
  RequestActionRequest,
  RequestActionResponse,
  SendBotMessageRequest,
  SendBotMessageResponse,
  SubscribeRequest,
  SubscribeResponse,
} from "@stringke/sdk/gen/memoh/integration/v1/envelope_pb";
