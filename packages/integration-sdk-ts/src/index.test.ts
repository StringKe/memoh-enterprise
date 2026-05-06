import { create, fromJsonString, toJsonString } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";

import {
  AckResponseSchema,
  AuthResponseSchema,
  CreateSessionResponseSchema,
  EnvelopeSchema,
  ErrorSchema,
  GetBotStatusResponseSchema,
  GetSessionStatusResponseSchema,
  PongSchema,
  RequestActionResponseSchema,
  SendBotMessageResponseSchema,
  SubscribeResponseSchema,
  type Envelope,
} from "@stringke/sdk/gen/memoh/integration/v1/envelope_pb";

import { MemohIntegrationClient, MemohIntegrationError, type WebSocketLike } from "./index";

class FakeWebSocket implements WebSocketLike {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  sent: Envelope[] = [];

  open(): void {
    this.readyState = 1;
    this.onopen?.(new Event("open"));
  }

  send(data: string): void {
    this.sent.push(fromJsonString(EnvelopeSchema, data, { ignoreUnknownFields: true }));
  }

  close(): void {
    this.readyState = 3;
    this.onclose?.(new Event("close") as CloseEvent);
  }

  emit(envelope: Envelope): void {
    this.onmessage?.(
      new MessageEvent("message", {
        data: toJsonString(EnvelopeSchema, envelope),
      }),
    );
  }
}

function makeClient(socket: FakeWebSocket): MemohIntegrationClient {
  let nextID = 0;
  return new MemohIntegrationClient({
    url: "ws://localhost/integration/v1/ws",
    token: "memoh_it_test",
    requestTimeoutMs: 500,
    webSocketFactory: () => socket,
    idFactory: () => `id-${++nextID}`,
  });
}

async function flushMicrotasks(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

async function sleep(ms: number): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("MemohIntegrationClient", () => {
  it("authenticates after websocket open", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();

    socket.open();
    await flushMicrotasks();
    expect(socket.sent[0]?.payload.case).toBe("authRequest");
    socket.emit(
      create(EnvelopeSchema, {
        payload: {
          case: "authResponse",
          value: create(AuthResponseSchema, {
            integrationId: "token-1",
            scopeType: "global",
          }),
        },
      }),
    );

    await expect(connected).resolves.toMatchObject({
      integrationId: "token-1",
      scopeType: "global",
    });
    expect(client.info?.integrationId).toBe("token-1");
  });

  it("subscribes with request correlation", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();
    socket.open();
    await flushMicrotasks();
    socket.emit(
      create(EnvelopeSchema, {
        payload: { case: "authResponse", value: create(AuthResponseSchema) },
      }),
    );
    await connected;

    const subscribed = client.subscribe({ eventTypes: ["message"], botIds: ["bot-1"] });
    const request = socket.sent[1];
    expect(request?.correlationId).toBe("id-2");
    expect(request?.payload.case).toBe("subscribeRequest");

    socket.emit(
      create(EnvelopeSchema, {
        correlationId: request?.correlationId,
        payload: {
          case: "subscribeResponse",
          value: create(SubscribeResponseSchema, { eventTypes: ["message"] }),
        },
      }),
    );

    await expect(subscribed).resolves.toMatchObject({ eventTypes: ["message"] });
  });

  it("acks and pings", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();
    socket.open();
    await flushMicrotasks();
    socket.emit(
      create(EnvelopeSchema, {
        payload: { case: "authResponse", value: create(AuthResponseSchema) },
      }),
    );
    await connected;

    const acked = client.ackEvent("event-1");
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-2",
        payload: {
          case: "ackResponse",
          value: create(AckResponseSchema, { eventId: "event-1" }),
        },
      }),
    );
    await expect(acked).resolves.toBe("event-1");

    const pong = client.ping();
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-3",
        payload: { case: "pong", value: create(PongSchema) },
      }),
    );
    await expect(pong).resolves.toBeUndefined();
  });

  it("sends bot messages and manages sessions", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();
    socket.open();
    await flushMicrotasks();
    socket.emit(
      create(EnvelopeSchema, {
        payload: { case: "authResponse", value: create(AuthResponseSchema) },
      }),
    );
    await connected;

    const sent = client.sendBotMessage({
      botId: "bot-1",
      sessionId: "session-1",
      text: "hello",
      metadata: { source: "test" },
    });
    expect(socket.sent[1]?.payload.case).toBe("sendBotMessageRequest");
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-2",
        payload: {
          case: "sendBotMessageResponse",
          value: create(SendBotMessageResponseSchema, {
            messageId: "message-1",
            sessionId: "session-1",
          }),
        },
      }),
    );
    await expect(sent).resolves.toMatchObject({ messageId: "message-1" });

    const created = client.createSession({ botId: "bot-1", externalSessionId: "external-1" });
    expect(socket.sent[2]?.payload.case).toBe("createSessionRequest");
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-3",
        payload: {
          case: "createSessionResponse",
          value: create(CreateSessionResponseSchema, {
            botId: "bot-1",
            sessionId: "session-2",
          }),
        },
      }),
    );
    await expect(created).resolves.toMatchObject({ sessionId: "session-2" });

    const status = client.getSessionStatus("session-2");
    expect(socket.sent[3]?.payload.case).toBe("getSessionStatusRequest");
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-4",
        payload: {
          case: "getSessionStatusResponse",
          value: create(GetSessionStatusResponseSchema, {
            botId: "bot-1",
            sessionId: "session-2",
            status: "active",
          }),
        },
      }),
    );
    await expect(status).resolves.toMatchObject({ status: "active" });

    const botStatus = client.getBotStatus("bot-1");
    expect(socket.sent[4]?.payload.case).toBe("getBotStatusRequest");
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-5",
        payload: {
          case: "getBotStatusResponse",
          value: create(GetBotStatusResponseSchema, {
            botId: "bot-1",
            status: "available",
          }),
        },
      }),
    );
    await expect(botStatus).resolves.toMatchObject({ status: "available" });

    const action = client.requestAction({
      botId: "bot-1",
      actionType: "run_task",
      payloadJson: '{"task":"sync"}',
    });
    expect(socket.sent[5]?.payload.case).toBe("requestActionRequest");
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-6",
        payload: {
          case: "requestActionResponse",
          value: create(RequestActionResponseSchema, {
            actionId: "action-1",
            botId: "bot-1",
            actionType: "run_task",
            status: "accepted",
          }),
        },
      }),
    );
    await expect(action).resolves.toMatchObject({ actionId: "action-1" });
  });

  it("rejects correlated protocol errors", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();
    socket.open();
    await flushMicrotasks();
    socket.emit(
      create(EnvelopeSchema, {
        payload: { case: "authResponse", value: create(AuthResponseSchema) },
      }),
    );
    await connected;

    const action = client.requestAction({ botId: "bot-2", actionType: "run_task" });
    socket.emit(
      create(EnvelopeSchema, {
        correlationId: "id-2",
        payload: {
          case: "error",
          value: create(ErrorSchema, {
            code: "permission_denied",
            message: "denied",
          }),
        },
      }),
    );

    await expect(action).rejects.toBeInstanceOf(MemohIntegrationError);
    await expect(action).rejects.toMatchObject({ code: "permission_denied" });
  });

  it("yields unsolicited envelopes through async iterator", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();
    socket.open();
    await flushMicrotasks();
    socket.emit(
      create(EnvelopeSchema, {
        payload: { case: "authResponse", value: create(AuthResponseSchema) },
      }),
    );
    await connected;

    const iterator = client.events()[Symbol.asyncIterator]();
    socket.emit(
      create(EnvelopeSchema, {
        messageId: "server-event-1",
        payload: {
          case: "error",
          value: create(ErrorSchema, { code: "event", message: "queued event" }),
        },
      }),
    );

    await expect(iterator.next()).resolves.toMatchObject({
      done: false,
      value: { messageId: "server-event-1" },
    });
    client.close();
  });

  it("deduplicates unsolicited envelopes by message id", async () => {
    const socket = new FakeWebSocket();
    const client = makeClient(socket);
    const connected = client.connect();
    socket.open();
    await flushMicrotasks();
    socket.emit(
      create(EnvelopeSchema, {
        payload: { case: "authResponse", value: create(AuthResponseSchema) },
      }),
    );
    await connected;

    const duplicate = create(EnvelopeSchema, {
      messageId: "server-event-1",
      payload: {
        case: "error",
        value: create(ErrorSchema, { code: "event", message: "queued event" }),
      },
    });
    socket.emit(duplicate);
    socket.emit(duplicate);

    const iterator = client.events()[Symbol.asyncIterator]();
    await expect(iterator.next()).resolves.toMatchObject({
      done: false,
      value: { messageId: "server-event-1" },
    });

    const next = iterator.next();
    await flushMicrotasks();
    client.close();
    await expect(next).resolves.toMatchObject({ done: true });
  });

  it("reconnects after a failed attempt", async () => {
    const first = new FakeWebSocket();
    const second = new FakeWebSocket();
    let created = 0;
    const client = new MemohIntegrationClient({
      url: "ws://localhost/integration/v1/ws",
      token: "memoh_it_test",
      requestTimeoutMs: 500,
      reconnectBackoffMs: 1,
      maxReconnectAttempts: 1,
      webSocketFactory: () => {
        created += 1;
        return created === 1 ? first : second;
      },
      idFactory: () => `id-${created}`,
    });

    const reconnected = client.reconnect();
    first.onerror?.(new Event("error"));
    await sleep(5);
    second.open();
    await flushMicrotasks();
    second.emit(
      create(EnvelopeSchema, {
        payload: {
          case: "authResponse",
          value: create(AuthResponseSchema, {
            integrationId: "token-2",
            scopeType: "global",
          }),
        },
      }),
    );

    await expect(reconnected).resolves.toMatchObject({ integrationId: "token-2" });
  });
});
