import type { Interceptor, Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { createMemohConnectClients } from "@stringke/sdk/connect";
import type {
  StreamChatRequest,
  StreamContainerProgressRequest,
  StreamTerminalRequest,
} from "@stringke/sdk/connect";
import { connectBaseUrl } from "./runtime-url";

export function createAuthInterceptor(getToken = () => localStorage.getItem("token")): Interceptor {
  return (next) => async (req) => {
    const token = getToken();
    if (token) {
      req.header.set("Authorization", `Bearer ${token}`);
    }
    return await next(req);
  };
}

export function createMemohWebTransport(baseUrl: string): Transport {
  return createConnectTransport({
    baseUrl,
    interceptors: [createAuthInterceptor()],
    fetch: (input, init) => fetch(input, { ...init, credentials: "include" }),
  });
}

const connectUrl = connectBaseUrl();

const transport = createMemohWebTransport(connectUrl);

export const connectClients = createMemohConnectClients({
  baseUrl: connectUrl,
  transport,
});

export function streamConnectChat(input: StreamChatRequest, signal: AbortSignal) {
  return connectClients.chat.streamChat(input, { signal });
}

export function streamConnectContainerProgress(
  input: StreamContainerProgressRequest,
  signal: AbortSignal,
) {
  return connectClients.containers.streamContainerProgress(input, { signal });
}

export function streamConnectTerminal(input: StreamTerminalRequest, signal: AbortSignal) {
  return connectClients.containers.streamTerminal(input, { signal });
}
